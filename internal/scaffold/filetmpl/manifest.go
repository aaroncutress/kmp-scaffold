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
	// FeatureNoun is what `add` calls the thing this template can add. Recipes
	// are not implemented yet; the field is reserved so a template that sets it
	// is not rejected out of hand.
	FeatureNoun string `toml:"feature_noun"`
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

	seen := map[string]bool{}
	for i := range m.Question {
		q := &m.Question[i]
		if q.ID == "" {
			return fmt.Errorf("question %d has no id", i+1)
		}
		if seen[q.ID] {
			return fmt.Errorf("two questions share the id %q", q.ID)
		}
		seen[q.ID] = true
		if q.Prompt == "" {
			return fmt.Errorf("question %q has no prompt", q.ID)
		}
		kind, err := parseKind(q.Kind)
		if err != nil {
			return fmt.Errorf("question %q: %w", q.ID, err)
		}
		q.Kind = string(kind)
		if kind == scaffold.KindSelect || kind == scaffold.KindMultiSelect {
			if len(q.Options) == 0 {
				return fmt.Errorf("question %q is a %s but has no options", q.ID, kind)
			}
			for j, o := range q.Options {
				if o.ID == "" {
					return fmt.Errorf("question %q: option %d has no id", q.ID, j+1)
				}
			}
		}
		if q.Pattern != "" {
			if _, err := compilePattern(q.Pattern); err != nil {
				return fmt.Errorf("question %q: pattern %q: %w", q.ID, q.Pattern, err)
			}
		}
	}

	if len(m.File) == 0 {
		return fmt.Errorf("[[files]] is empty, so this template would generate nothing")
	}
	for i := range m.File {
		f := &m.File[i]
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
