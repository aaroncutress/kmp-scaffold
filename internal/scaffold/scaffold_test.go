package scaffold

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
)

// stub is the smallest thing that satisfies Template. The real ones live in
// packages that import this one, so the registry is exercised with a local
// stand-in rather than by reaching back up the dependency graph.
type stub struct{ id string }

func (s stub) Meta() Meta {
	return Meta{ID: s.id, Label: "Stub", Source: Source{Kind: SourceBuiltin}}
}
func (stub) NewAnswers() *Answers                                  { return NewAnswers() }
func (stub) Questions() []Question                                 { return nil }
func (stub) Normalise(*Answers) []string                           { return nil }
func (stub) Versions(*Answers) resolve.Request                     { return resolve.Request{} }
func (stub) Check(*Answers, *resolve.Result)                       {}
func (stub) Headlines(*Answers, *resolve.Result) []Headline        { return nil }
func (stub) Summary(*Answers, *resolve.Result) []Section           { return nil }
func (stub) Vars(*Answers) any                                     { return nil }
func (stub) NextSteps(*Answers) []NextStep                         { return nil }
func (stub) Generate(context.Context, GenRequest) (*Report, error) { return &Report{}, nil }

func init() {
	Register(DefaultTemplate, func() Template { return stub{DefaultTemplate} })
	Register("other", func() Template { return stub{"other"} })
}

// Anything that has been through the manifest, or through a saved set of
// answers, comes back from encoding/json as []any and float64. A template that
// only understood the Go types would silently see empty lists and zeros.
func TestAnswersSurviveAJSONRoundTrip(t *testing.T) {
	a := NewAnswers()
	a.Set("packs", []string{"images", "database"})
	a.Set("minSdk", 26)
	a.Set("android", true)
	a.Set("layout", "nav3-shell")

	data, err := json.Marshal(a.Values)
	if err != nil {
		t.Fatal(err)
	}

	back := NewAnswers()
	if err := json.Unmarshal(data, &back.Values); err != nil {
		t.Fatal(err)
	}

	packs := back.Strs("packs")
	if len(packs) != 2 || packs[0] != "images" || packs[1] != "database" {
		t.Errorf("packs = %#v, want the original two", packs)
	}
	if got := back.Int("minSdk"); got != 26 {
		t.Errorf("minSdk = %d, want 26 (JSON numbers arrive as float64)", got)
	}
	if !back.Bool("android") {
		t.Error("android came back false")
	}
	if got := back.Str("layout"); got != "nav3-shell" {
		t.Errorf("layout = %q", got)
	}
}

func TestAnswersReadMissingValuesAsZero(t *testing.T) {
	a := NewAnswers()
	if a.Has("nothing") {
		t.Error("an unanswered question should not report as answered")
	}
	if a.Str("nothing") != "" || a.Int("nothing") != 0 || a.Bool("nothing") || a.Strs("nothing") != nil {
		t.Error("an unanswered question should read as the zero value")
	}
}

// A single string is a list of one. This is what makes `--set tabs=Home` do the
// obvious thing.
func TestStrsAcceptsASingleString(t *testing.T) {
	a := NewAnswers()
	a.Set("tabs", "Home")
	if got := a.Strs("tabs"); len(got) != 1 || got[0] != "Home" {
		t.Errorf("tabs = %#v, want [Home]", got)
	}
}

func TestQuestionHooksWinOverStaticFields(t *testing.T) {
	a := NewAnswers()
	q := Question{
		ID:         "layout",
		Default:    "static",
		Options:    []Option{{ID: "static"}},
		DefaultFor: func(*Answers) any { return "computed" },
		OptionsFor: func(*Answers) []Option { return []Option{{ID: "computed"}} },
	}

	if got := q.DefaultValue(a); got != "computed" {
		t.Errorf("default = %v, want the hook's answer", got)
	}
	if opts := q.OptionList(a); len(opts) != 1 || opts[0].ID != "computed" {
		t.Errorf("options = %+v, want the hook's answer", opts)
	}

	// Without hooks, the declarative fields are used as-is - which is all a
	// file-based template will ever set.
	plain := Question{ID: "layout", Default: "static", Options: []Option{{ID: "static"}}}
	if got := plain.DefaultValue(a); got != "static" {
		t.Errorf("default = %v, want static", got)
	}
	if opts := plain.OptionList(a); len(opts) != 1 || opts[0].ID != "static" {
		t.Errorf("options = %+v, want static", opts)
	}
}

func TestLoadFallsBackToTheDefaultTemplate(t *testing.T) {
	if _, err := Load(""); err != nil {
		t.Errorf("an empty ref should load the default template: %v", err)
	}
	if _, err := Load("no-such-template"); err == nil {
		t.Error("an unknown template should be an error")
	}
}

// An external ref is recognised even though it cannot be loaded yet, so the
// message says what is missing rather than "unknown template".
func TestLoadExplainsExternalRefs(t *testing.T) {
	for _, ref := range []string{"./my-template", "/tmp/my-template", "github:acme/tmpl@v1",
		"https://example.com/tmpl.git"} {
		_, err := Load(ref)
		if err == nil {
			t.Errorf("Load(%q) should fail for now", ref)
			continue
		}
		if !strings.Contains(err.Error(), "external template") {
			t.Errorf("Load(%q) = %v, want an explanation that external templates are not loadable yet", ref, err)
		}
	}
}

func TestSourceStringIsWhatTheManifestRecords(t *testing.T) {
	if got := (Source{Kind: SourceBuiltin}).String(); got != "builtin" {
		t.Errorf("builtin source = %q", got)
	}
	if got := (Source{Kind: SourceRemote, Ref: "github:acme/tmpl@v1"}).String(); got != "github:acme/tmpl@v1" {
		t.Errorf("remote source = %q, want the ref the user typed", got)
	}
}
