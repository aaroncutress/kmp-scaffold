package filetmpl

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/render"
	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
)

var templateIDRe = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// Template is a template read from a directory.
type Template struct {
	dir      string
	manifest *Manifest
	source   scaffold.Source
	engine   *render.Engine
}

// Open reads the template in dir.
func Open(dir string, source scaffold.Source) (*Template, error) {
	manifest, err := ReadManifest(dir)
	if err != nil {
		return nil, err
	}

	// Partials are optional; without them the engine still renders each file's
	// own text through RenderString.
	engine := render.NewEmptyEngine()
	if _, err := os.Stat(dir + "/partials"); err == nil {
		engine, err = render.NewEngineFS(os.DirFS(dir), "partials/*.tmpl")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
	}

	return &Template{dir: dir, manifest: manifest, source: source, engine: engine}, nil
}

// Dir is where the template was read from.
func (t *Template) Dir() string { return t.dir }

// Meta describes this template.
func (t *Template) Meta() scaffold.Meta {
	b := t.manifest.Template
	sentinels := b.Sentinels
	if len(sentinels) == 0 {
		sentinels = []string{model.ManifestFile}
	}
	return scaffold.Meta{
		ID:          b.ID,
		Label:       b.Name,
		Description: b.Description,
		Version:     b.Version,
		Source:      t.source,
		AsksPackage: b.UsesPackage,
		Sentinels:   sentinels,
	}
}

// NewAnswers seeds the manifest's defaults, so a non-interactive run generates
// the same project the wizard's default path would.
func (t *Template) NewAnswers() *scaffold.Answers {
	a := scaffold.NewAnswers()
	for _, q := range t.manifest.Question {
		if q.Default != nil {
			a.Set(q.ID, q.Default)
		}
	}
	return a
}

// Questions turns the manifest's questions into the tool's.
func (t *Template) Questions() []scaffold.Question {
	out := make([]scaffold.Question, 0, len(t.manifest.Question))
	for _, def := range t.manifest.Question {
		out = append(out, t.question(def))
	}
	return out
}

func (t *Template) question(def QuestionDef) scaffold.Question {
	kind := scaffold.Kind(def.Kind)

	q := scaffold.Question{
		ID:         def.ID,
		Kind:       kind,
		Prompt:     def.Prompt,
		Hint:       def.Hint,
		Default:    def.Default,
		AllowEmpty: def.AllowEmpty,
	}

	if def.When != "" {
		when := def.When
		q.SkipFor = func(a *scaffold.Answers) bool {
			ok, err := t.engine.Truthy(when, t.ctx(a, nil))
			// A condition that will not render is a template bug. Asking the
			// question is the recoverable choice: the user can still answer it,
			// and the answer is recorded either way.
			return err == nil && !ok
		}
	}

	if len(def.Options) > 0 {
		options := def.Options
		q.OptionsFor = func(a *scaffold.Answers) []scaffold.Option {
			ctx := t.ctx(a, nil)
			out := make([]scaffold.Option, 0, len(options))
			for _, o := range options {
				opt := scaffold.Option{ID: o.ID, Label: o.Label, Desc: o.Desc}
				if opt.Label == "" {
					opt.Label = o.ID
				}
				if o.DisabledWhen != "" {
					if off, err := t.engine.Truthy(o.DisabledWhen, ctx); err == nil && off {
						opt.Disabled = true
						opt.DisabledNote = o.DisabledNote
					}
				}
				out = append(out, opt)
			}
			return out
		}
	}

	if v := validatorFor(def, kind); v != nil {
		q.Validate = v
	}
	return q
}

func validatorFor(def QuestionDef, kind scaffold.Kind) func(any, *scaffold.Answers) error {
	var re *regexp.Regexp
	if def.Pattern != "" {
		re, _ = compilePattern(def.Pattern)
	}
	if re == nil && def.Min == 0 && def.Max == 0 {
		return nil
	}

	invalid := def.Invalid
	if invalid == "" && def.Pattern != "" {
		invalid = "must match " + def.Pattern
	}

	return func(v any, _ *scaffold.Answers) error {
		var items []string
		switch t := v.(type) {
		case []string:
			items = t
		case string:
			items = []string{t}
		}

		if kind == scaffold.KindList || kind == scaffold.KindMultiSelect {
			if def.Min > 0 && len(items) < def.Min {
				return fmt.Errorf("pick at least %d", def.Min)
			}
			if def.Max > 0 && len(items) > def.Max {
				return fmt.Errorf("pick at most %d", def.Max)
			}
		}
		if re == nil {
			return nil
		}
		for _, item := range items {
			if !re.MatchString(item) {
				if len(items) > 1 {
					return fmt.Errorf("%q: %s", item, invalid)
				}
				return fmt.Errorf("%s", invalid)
			}
		}
		return nil
	}
}

func compilePattern(p string) (*regexp.Regexp, error) { return regexp.Compile(p) }

// Normalise has nothing to do: a file-based template's questions stand alone,
// with `when` and `disabled_when` expressing what depends on what. Rules that
// need a fixpoint are the reason to write a Go template instead.
func (t *Template) Normalise(*scaffold.Answers) []string { return nil }

// Vars is every answer, verbatim. A file-based template has no tidier
// projection to make: the answers *are* its state.
func (t *Template) Vars(a *scaffold.Answers) any { return a.Values }

// Versions describes what to resolve, or an empty request when the template
// declares no versions at all.
func (t *Template) Versions(a *scaffold.Answers) resolve.Request {
	v := t.manifest.Versions
	if len(v.Keys) == 0 && len(v.Probe) == 0 {
		return resolve.Request{}
	}

	channel := v.Channel
	if v.ChannelFrom != "" {
		if answered := a.Str(v.ChannelFrom); answered != "" {
			channel = answered
		}
	}

	minSDK := 0
	if v.MinSDKFrom != "" {
		minSDK = a.Int(v.MinSDKFrom)
	}

	req := resolve.Request{
		Keys:    v.Keys,
		Channel: catalog.ParseChannel(channel),
		Offline: a.Offline,
		Timeout: 20 * time.Second,
		Android: v.Android,
		MinSDK:  minSDK,
	}
	for _, p := range v.Probe {
		req.Extra = append(req.Extra, catalog.VersionKey{
			Key:        p.Key,
			Probe:      catalog.Coordinate{Group: p.Group, Artifact: p.Artifact, Repo: parseRepo(p.Repo)},
			Baseline:   p.Baseline,
			MinChannel: catalog.ParseChannel(p.MinChannel),
		})
	}
	return req
}

func parseRepo(s string) catalog.Repo {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "google":
		return catalog.Google
	case "portal", "gradle", "plugins":
		return catalog.Portal
	default:
		return catalog.Central
	}
}

// Check has nothing to add: a file-based template gets the resolver's output,
// not a hook into its compatibility rules.
func (t *Template) Check(*scaffold.Answers, *resolve.Result) {}

// Headlines shows every version that was resolved, in catalog order. A Go
// template picks a shorter list; there is nothing here to pick it with.
func (t *Template) Headlines(a *scaffold.Answers, res *resolve.Result) []scaffold.Headline {
	if res == nil {
		return nil
	}
	var out []scaffold.Headline
	if res.Gradle.Version != "" {
		out = append(out, scaffold.Headline{Label: "Gradle", Value: res.Gradle.Version, Note: "current release"})
	}
	for _, key := range t.Versions(a).Keys {
		if v := res.V(key); v != "" {
			out = append(out, scaffold.Headline{Label: key, Value: v, Note: res.Sources[key]})
		}
	}
	for _, p := range t.manifest.Versions.Probe {
		if v := res.V(p.Key); v != "" {
			out = append(out, scaffold.Headline{Label: p.Key, Value: v, Note: res.Sources[p.Key]})
		}
	}
	return out
}

// Summary is the review screen. Without a [[summary]] block it is built from
// the questions, which is what most templates want and none have to write.
func (t *Template) Summary(a *scaffold.Answers, res *resolve.Result) []scaffold.Section {
	rows := []scaffold.Row{
		{Label: "Project", Value: a.Project.Name},
		{Label: "Directory", Value: a.Project.Dir},
	}
	if t.manifest.Template.UsesPackage {
		rows = append(rows, scaffold.Row{Label: "Package", Value: a.Project.Package})
	}
	sections := []scaffold.Section{{Rows: rows}}

	if len(t.manifest.Summary) > 0 {
		sections = append(sections, t.declaredSummary(a)...)
	} else {
		sections = append(sections, t.questionSummary(a)...)
	}

	if res != nil && len(res.Versions) > 0 {
		var items []string
		for _, h := range t.Headlines(a, res) {
			items = append(items, h.Label+" "+h.Value)
		}
		toolchain := scaffold.Section{Title: "Versions", Items: items}
		if res.HasErrors() {
			toolchain.Note = "The resolver flagged an incompatibility above - " +
				"go back and check before generating."
		}
		sections = append(sections, toolchain)
	}
	return sections
}

func (t *Template) declaredSummary(a *scaffold.Answers) []scaffold.Section {
	ctx := t.ctx(a, nil)
	var out []scaffold.Section
	for _, block := range t.manifest.Summary {
		if block.When != "" {
			if ok, err := t.engine.Truthy(block.When, ctx); err != nil || !ok {
				continue
			}
		}
		sec := scaffold.Section{Title: block.Title}
		for _, r := range block.Rows {
			value, err := t.engine.RenderString("summary", r.Value, ctx)
			if err != nil {
				continue
			}
			sec.Rows = append(sec.Rows, scaffold.Row{Label: r.Label, Value: strings.TrimSpace(string(value))})
		}
		if block.Items != "" {
			if out, err := t.engine.RenderString("summary", block.Items, ctx); err == nil {
				for _, line := range strings.Split(string(out), "\n") {
					if line = strings.TrimSpace(line); line != "" {
						sec.Items = append(sec.Items, line)
					}
				}
			}
		}
		out = append(out, sec)
	}
	return out
}

// questionSummary reads back every answered question, using each option's label
// rather than its id so the review says what the user chose, not what the
// template calls it.
func (t *Template) questionSummary(a *scaffold.Answers) []scaffold.Section {
	var rows []scaffold.Row
	var sections []scaffold.Section

	for _, def := range t.manifest.Question {
		if !a.Has(def.ID) {
			continue
		}
		label := def.ID
		switch scaffold.Kind(def.Kind) {
		case scaffold.KindMultiSelect, scaffold.KindList:
			items := labelled(def, a.Strs(def.ID))
			sections = append(sections, scaffold.Section{Title: label, Items: items})
		case scaffold.KindConfirm:
			rows = append(rows, scaffold.Row{Label: label, Value: yesNo(a.Bool(def.ID))})
		default:
			rows = append(rows, scaffold.Row{Label: label,
				Value: strings.Join(labelled(def, []string{a.Str(def.ID)}), ", ")})
		}
	}
	if len(rows) > 0 {
		sections = append([]scaffold.Section{{Rows: rows}}, sections...)
	}
	return sections
}

func labelled(def QuestionDef, ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		label := id
		for _, o := range def.Options {
			if o.ID == id && o.Label != "" {
				label = o.Label
			}
		}
		out = append(out, label)
	}
	return out
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// NextSteps is what to print once the project exists.
func (t *Template) NextSteps(a *scaffold.Answers) []scaffold.NextStep {
	ctx := t.ctx(a, nil)
	var out []scaffold.NextStep
	for _, def := range t.manifest.NextStep {
		if def.When != "" {
			if ok, err := t.engine.Truthy(def.When, ctx); err != nil || !ok {
				continue
			}
		}
		command, err := t.engine.RenderString("next-step", def.Command, ctx)
		if err != nil {
			continue
		}
		note, err := t.engine.RenderString("next-step-note", def.Note, ctx)
		if err != nil {
			continue
		}
		out = append(out, scaffold.NextStep{
			Command: strings.TrimSpace(string(command)),
			Note:    strings.TrimSpace(string(note)),
		})
	}
	return out
}

// Outputs lists what this template writes, as declared. The paths still hold
// their placeholders, which is the useful form for "what would this give me?".
func (t *Template) Outputs() []string {
	var out []string
	for _, f := range t.manifest.File {
		to := f.To
		if strings.HasSuffix(to, "/") {
			to += "*"
		}
		if f.When != "" {
			to += "   (only when " + strings.TrimSpace(f.When) + ")"
		}
		out = append(out, to)
	}
	return out
}

// Generate writes the project.
func (t *Template) Generate(_ context.Context, req scaffold.GenRequest) (*scaffold.Report, error) {
	if err := t.write(req.Answers, req.Result, req.Version, req.Writer, nil); err != nil {
		return nil, err
	}

	manifest, err := model.NewManifest(req.Version, t.Meta().Ref(), req.Answers.Project,
		t.Vars(req.Answers), nil)
	if err != nil {
		return nil, err
	}
	if !req.Writer.DryRun {
		if err := manifest.Save(req.Writer.Root); err != nil {
			return nil, fmt.Errorf("writing the project manifest: %w", err)
		}
	}

	return &scaffold.Report{Writer: req.Writer, Manifest: manifest}, nil
}
