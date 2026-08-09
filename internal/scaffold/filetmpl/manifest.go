// Package filetmpl implements templates written as files rather than as Go
// code: a directory with a template.toml manifest and a tree of Go templates.
//
// The trade is deliberate. A file-based template can be written, read and
// shared without a compiler, and it can do everything most templates need: ask
// questions, resolve versions, and write files whose paths and contents depend
// on the answers. What it cannot do is compute a question's options from a
// registry, express dependency rules beyond `requires`, or reach into the
// resolver's compatibility passes. A template that needs those is a Go
// template - see internal/kmp.
package filetmpl

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
)

// ManifestFile is the file that makes a directory a template.
const ManifestFile = "template.toml"

// ManifestSchema is the manifest format this build understands.
const ManifestSchema = 1

// Manifest is the parsed template.toml.
type Manifest struct {
	Schema   int            `toml:"schema"`
	Template TemplateBlock  `toml:"template"`
	Question []QuestionDef  `toml:"questions"`
	Versions VersionsBlock  `toml:"versions"`
	File     []FileDef      `toml:"files"`
	NextStep []NextStepDef  `toml:"next_steps"`
	Summary  []SummaryBlock `toml:"summary"`
	// Recipes are the named things `kmp-scaffold add` can apply later, keyed by
	// name. TOML tables are unordered, so they are sorted before use.
	Recipes map[string]RecipeDef `toml:"recipes"`
}

// TemplateBlock is the template's identity.
type TemplateBlock struct {
	ID          string `toml:"id"`
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Version     string `toml:"version"`
	// UsesPackage adds the universal "package name?" question.
	UsesPackage bool `toml:"uses_package"`
	// Sentinels are the files whose presence means a directory already holds a
	// project of this kind.
	Sentinels []string `toml:"sentinels"`
}

// RecipeDef is one [recipes.<name>] block: something `kmp-scaffold add` can
// apply to a project this template generated.
type RecipeDef struct {
	Label       string `toml:"label"`
	Description string `toml:"description"`
	// Noun is what to call the thing being added in prompts. Defaults to the
	// recipe's name.
	Noun string `toml:"noun"`
	// NameHint is shown under the "what is it called?" question.
	NameHint string `toml:"name_hint"`

	Question []QuestionDef  `toml:"questions"`
	File     []FileDef      `toml:"files"`
	Edit     []EditDef      `toml:"edits"`
	Summary  []SummaryBlock `toml:"summary"`
}

// EditDef is one insertion into a file that already exists. Every field is a Go
// template, and the whole thing maps onto one wire.Edit.
type EditDef struct {
	// Path is the file to edit, relative to the project root.
	Path string `toml:"path"`
	// Anchor is the comment to insert above.
	Anchor string `toml:"anchor"`
	// Lines are inserted, indented to match the anchor.
	Lines []string `toml:"lines"`
	// Imports are sorted into the file's import block. Kotlin and Swift only -
	// the insertion looks for a line starting with `import`.
	Imports []string `toml:"imports"`
	// Key decides whether this block is already there, instead of its first
	// line. Give one when the first line is not distinctive.
	Key string `toml:"key"`
	// When is a Go template; the edit is applied when it renders truthy.
	When string `toml:"when"`
}

// QuestionDef is one wizard question.
type QuestionDef struct {
	ID     string `toml:"id"`
	Kind   string `toml:"kind"`
	Prompt string `toml:"prompt"`
	Hint   string `toml:"hint"`

	Default    any         `toml:"default"`
	Options    []OptionDef `toml:"options"`
	AllowEmpty bool        `toml:"allow_empty"`

	// When is a Go template; the question is asked when it renders to something
	// truthy.
	When string `toml:"when"`

	// Pattern is a regexp every answer (or every item of a list answer) must
	// match, and Invalid is what to say when it does not.
	Pattern string `toml:"pattern"`
	Invalid string `toml:"invalid"`

	// Min and Max bound a list or checklist answer. Zero means no bound.
	Min int `toml:"min"`
	Max int `toml:"max"`
}

// OptionDef is one choice of a select or checklist question.
type OptionDef struct {
	ID    string `toml:"id"`
	Label string `toml:"label"`
	Desc  string `toml:"desc"`
	// DisabledWhen greys the option out; DisabledNote says why.
	DisabledWhen string `toml:"disabled_when"`
	DisabledNote string `toml:"disabled_note"`
}

// VersionsBlock says what to resolve. An empty block skips resolution.
type VersionsBlock struct {
	// Keys are catalog version keys.
	Keys []string `toml:"keys"`
	// Probe declares coordinates the catalog does not already know about.
	Probe []ProbeDef `toml:"probe"`
	// Android turns on the Android SDK passes.
	Android bool `toml:"android"`
	// MinSDKFrom names the question holding the minSdk answer.
	MinSDKFrom string `toml:"min_sdk_from"`
	// ChannelFrom names the question holding the release channel.
	ChannelFrom string `toml:"channel_from"`
	// Channel is the release channel when no question supplies one.
	Channel string `toml:"channel"`
}

// ProbeDef is a coordinate to look up.
type ProbeDef struct {
	Key        string `toml:"key"`
	Group      string `toml:"group"`
	Artifact   string `toml:"artifact"`
	Repo       string `toml:"repo"`
	Baseline   string `toml:"baseline"`
	MinChannel string `toml:"min_channel"`
}

// FileDef is one output, or - with a glob - a subtree of them.
type FileDef struct {
	// From is a path inside the template. A trailing /** takes the subtree.
	From string `toml:"from"`
	// To is the output path, itself a Go template. Inside a glob or a for_each
	// it is rendered per item.
	To string `toml:"to"`
	// When is a Go template; the file is written when it renders truthy.
	When string `toml:"when"`
	// ForEach is a Go template producing a list; the file is rendered once per
	// item, with .Item, .Index, .First and .Last bound.
	ForEach string `toml:"for_each"`
	// Copy writes the source verbatim instead of rendering it, for anything
	// that is not a Go template.
	Copy bool `toml:"copy"`
	// Mode is an octal file mode, e.g. "0755".
	Mode string `toml:"mode"`
	// Normalise tidies whitespace and collapses blank runs. On by default;
	// turn it off for output where every byte matters.
	Normalise *bool `toml:"normalise"`
}

// NextStepDef is one line of the "what to do now" list.
type NextStepDef struct {
	Command string `toml:"command"`
	Note    string `toml:"note"`
	When    string `toml:"when"`
}

// SummaryBlock is one section of the review screen. Without any, the review is
// built from the questions themselves.
type SummaryBlock struct {
	Title string   `toml:"title"`
	Rows  []RowDef `toml:"rows"`
	Items string   `toml:"items"`
	When  string   `toml:"when"`
}

// RowDef is one label/value pair, the value being a Go template.
type RowDef struct {
	Label string `toml:"label"`
	Value string `toml:"value"`
}

// ReadManifest parses a template.toml.
//
// Unknown keys are an error rather than being ignored: a mistyped `when` that
// silently did nothing would be far harder to find than a message naming the
// line it is on.
func ReadManifest(dir string) (*Manifest, error) {
	file := filepath.Join(dir, ManifestFile)
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%s has no %s, so it is not a template", dir, ManifestFile)
		}
		return nil, err
	}

	var m Manifest
	meta, err := toml.Decode(string(data), &m)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		var keys []string
		for _, k := range undecoded {
			keys = append(keys, k.String())
		}
		return nil, fmt.Errorf("%s: unknown key(s): %s", file, strings.Join(keys, ", "))
	}

	if err := m.validate(dir); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return &m, nil
}

func (m *Manifest) validate(dir string) error {
	switch {
	case m.Schema == 0:
		return fmt.Errorf("no schema; add `schema = %d` at the top", ManifestSchema)
	case m.Schema > ManifestSchema:
		return fmt.Errorf("schema %d is newer than this build understands (%d)", m.Schema, ManifestSchema)
	}

	if m.Template.ID == "" {
		return fmt.Errorf("[template] has no id")
	}
	if !templateIDRe.MatchString(m.Template.ID) {
		return fmt.Errorf("template id %q: use lowercase letters, digits and dashes", m.Template.ID)
	}
	if m.Template.Name == "" {
		m.Template.Name = m.Template.ID
	}

	if err := validateQuestions(m.Question, ""); err != nil {
		return err
	}

	if len(m.File) == 0 {
		return fmt.Errorf("[[files]] is empty, so this template would generate nothing")
	}
	if err := validateFiles(dir, m.File); err != nil {
		return err
	}

	for _, name := range m.RecipeNames() {
		r := m.Recipes[name]
		if !templateIDRe.MatchString(name) {
			return fmt.Errorf("recipe %q: use lowercase letters, digits and dashes", name)
		}
		if len(r.File) == 0 && len(r.Edit) == 0 {
			return fmt.Errorf("recipe %q writes no files and makes no edits, so it would do nothing", name)
		}
		if err := validateQuestions(r.Question, "recipe "+name+": "); err != nil {
			return err
		}
		if err := validateFiles(dir, r.File); err != nil {
			return fmt.Errorf("recipe %q: %w", name, err)
		}
		for i, e := range r.Edit {
			if e.Path == "" {
				return fmt.Errorf("recipe %q: edit %d has no `path`", name, i+1)
			}
			if len(e.Lines) == 0 && len(e.Imports) == 0 {
				return fmt.Errorf("recipe %q: edit on %q has neither `lines` nor `imports`", name, e.Path)
			}
			if len(e.Lines) > 0 && e.Anchor == "" {
				return fmt.Errorf("recipe %q: edit on %q has `lines` but no `anchor` to insert them above",
					name, e.Path)
			}
		}
	}
	return nil
}

// RecipeNames lists the recipe names in a stable order. TOML tables decode into
// a Go map, whose iteration order would otherwise change between runs.
func (m *Manifest) RecipeNames() []string {
	out := make([]string, 0, len(m.Recipes))
	for name := range m.Recipes {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func validateFiles(dir string, files []FileDef) error {
	for i := range files {
		f := &files[i]
		if f.From == "" {
			return fmt.Errorf("file %d has no `from`", i+1)
		}
		if f.To == "" {
			return fmt.Errorf("file %q has no `to`", f.From)
		}
		if _, err := parseMode(f.Mode); err != nil {
			return fmt.Errorf("file %q: %w", f.From, err)
		}
		if err := checkSourcePath(dir, f.From); err != nil {
			return err
		}
	}
	return nil
}

// checkSourcePath keeps a template's `from` inside its own directory, and says
// so early rather than at generation time.
func checkSourcePath(dir, from string) error {
	clean := path.Clean(from)
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("file %q is outside the template directory", from)
	}
	base := strings.TrimSuffix(clean, "/**")
	base = strings.TrimSuffix(base, "/*")
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(base))); err != nil {
		return fmt.Errorf("file %q does not exist in the template", from)
	}
	return nil
}

// validateQuestions checks a question list and canonicalises each kind. The
// prefix names which list, so an error in a recipe says which recipe.
func validateQuestions(questions []QuestionDef, prefix string) error {
	seen := map[string]bool{}
	for i := range questions {
		q := &questions[i]
		if q.ID == "" {
			return fmt.Errorf("%squestion %d has no id", prefix, i+1)
		}
		if seen[q.ID] {
			return fmt.Errorf("%stwo questions share the id %q", prefix, q.ID)
		}
		seen[q.ID] = true
		if q.ID == scaffold.NameAnswer {
			return fmt.Errorf("%squestion id %q is reserved - the tool asks for the name itself",
				prefix, scaffold.NameAnswer)
		}
		if q.Prompt == "" {
			return fmt.Errorf("%squestion %q has no prompt", prefix, q.ID)
		}
		kind, err := parseKind(q.Kind)
		if err != nil {
			return fmt.Errorf("%squestion %q: %w", prefix, q.ID, err)
		}
		q.Kind = string(kind)
		if kind == scaffold.KindSelect || kind == scaffold.KindMultiSelect {
			if len(q.Options) == 0 {
				return fmt.Errorf("%squestion %q is a %s but has no options", prefix, q.ID, kind)
			}
			for j, o := range q.Options {
				if o.ID == "" {
					return fmt.Errorf("%squestion %q: option %d has no id", prefix, q.ID, j+1)
				}
			}
		}
		if q.Pattern != "" {
			if _, err := compilePattern(q.Pattern); err != nil {
				return fmt.Errorf("%squestion %q: pattern %q: %w", prefix, q.ID, q.Pattern, err)
			}
		}
	}
	return nil
}

func parseKind(s string) (scaffold.Kind, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "text":
		return scaffold.KindText, nil
	case "list":
		return scaffold.KindList, nil
	case "select":
		return scaffold.KindSelect, nil
	case "multiselect", "checklist":
		return scaffold.KindMultiSelect, nil
	case "confirm", "bool":
		return scaffold.KindConfirm, nil
	default:
		return "", fmt.Errorf("unknown kind %q - use text, list, select, multiselect or confirm", s)
	}
}

func parseMode(s string) (os.FileMode, error) {
	if s == "" {
		return 0, nil
	}
	var mode uint32
	if _, err := fmt.Sscanf(s, "%o", &mode); err != nil || mode == 0 || mode > 0o7777 {
		return 0, fmt.Errorf("mode %q is not an octal file mode like \"0755\"", s)
	}
	return os.FileMode(mode), nil
}
