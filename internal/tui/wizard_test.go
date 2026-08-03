package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func typeText(w *Wizard, s string) {
	for _, r := range s {
		if r == ' ' {
			w.Update(key("space"))
			continue
		}
		w.Update(key(string(r)))
	}
}

func defaultSpec() model.Spec {
	spec := model.Defaults()
	spec.Packs = catalog.BasicPacks()
	spec.SharedUtils = catalog.DefaultUtilities()
	spec.AndroidExtras = catalog.DefaultExtras()
	return spec
}

// A step that records how many times it was entered, so the tests can assert on
// wizard navigation without a terminal.
type probeStep struct {
	title   string
	entered int
	skip    bool
}

func (p *probeStep) Title() string        { return p.title }
func (p *probeStep) Help() string         { return "" }
func (p *probeStep) Skip(model.Spec) bool { return p.skip }
func (p *probeStep) Enter(*model.Spec) tea.Cmd {
	p.entered++
	return nil
}

func (p *probeStep) Update(msg tea.Msg, _ *model.Spec) (tea.Cmd, Outcome) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, StayHere
	}
	switch k.String() {
	case "enter":
		return nil, GoNext
	case "esc":
		return nil, GoBack
	}
	return nil, StayHere
}

func (p *probeStep) View(model.Spec, int) string { return p.title }

func TestWizardSkipsDisabledSteps(t *testing.T) {
	first := &probeStep{title: "first"}
	skipped := &probeStep{title: "skipped", skip: true}
	last := &probeStep{title: "last"}

	w := NewWizard("test", defaultSpec(), []Step{first, skipped, last})
	w.Init()
	w.Update(key("enter"))

	if skipped.entered != 0 {
		t.Error("a skipped step was entered")
	}
	if last.entered != 1 {
		t.Errorf("last step entered %d times, want 1", last.entered)
	}
}

func TestWizardGoesBack(t *testing.T) {
	first := &probeStep{title: "first"}
	second := &probeStep{title: "second"}

	w := NewWizard("test", defaultSpec(), []Step{first, second})
	w.Init()
	w.Update(key("enter")) // -> second
	w.Update(key("esc"))   // -> back to first

	if first.entered != 2 {
		t.Errorf("first step entered %d times, want 2 (initial + return)", first.entered)
	}
	if !strings.Contains(w.View(), "first") {
		t.Error("the view does not show the step we went back to")
	}
}

// Escape on the very first step means "give up", not "go nowhere".
func TestWizardCancelsFromFirstStep(t *testing.T) {
	w := NewWizard("test", defaultSpec(), []Step{&probeStep{title: "only"}})
	w.Init()
	w.Update(key("esc"))

	if !w.cancelled {
		t.Error("esc on the first step should cancel the wizard")
	}
}

func TestTextStepValidates(t *testing.T) {
	step := &TextStep{
		Prompt:   "Name?",
		Default:  func(model.Spec) string { return "" },
		Validate: func(v string, _ model.Spec) error { return model.ValidateProjectName(v) },
		Apply:    func(spec *model.Spec, v string) { spec.Name = v },
	}
	w := NewWizard("test", defaultSpec(), []Step{step, &probeStep{title: "next"}})
	w.Init()

	// An invalid value keeps us on the step and shows the reason.
	typeText(w, "9bad")
	w.Update(key("enter"))
	if w.idx != 0 {
		t.Error("an invalid value advanced the wizard")
	}
	if !strings.Contains(w.View(), "must start with a letter") {
		t.Errorf("the validation error is not shown:\n%s", w.View())
	}

	// Correcting it lets us through, and the answer lands in the spec.
	for range 4 {
		w.Update(key("backspace"))
	}
	typeText(w, "Tunesic")
	w.Update(key("enter"))

	if w.idx != 1 {
		t.Error("a valid value did not advance the wizard")
	}
	if w.Spec().Name != "Tunesic" {
		t.Errorf("Name = %q, want Tunesic", w.Spec().Name)
	}
}

func TestCheckStepTogglesAndApplies(t *testing.T) {
	step := &CheckStep{
		Prompt: "Pick",
		Options: func(model.Spec) []Option {
			return []Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}, {ID: "c", Label: "C"}}
		},
		Current: func(model.Spec) []string { return []string{"a"} },
		Apply:   func(spec *model.Spec, ids []string) { spec.Packs = ids },
	}
	w := NewWizard("test", defaultSpec(), []Step{step, &probeStep{title: "next"}})
	w.Init()

	w.Update(key("down"))  // cursor on B
	w.Update(key("space")) // tick B
	w.Update(key("down"))  // cursor on C
	w.Update(key("space")) // tick C
	w.Update(key("space")) // untick C again
	w.Update(key("enter"))

	got := w.Spec().Packs
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("Packs = %v, want [a b]", got)
	}
}

func TestCheckStepSelectAllAndNone(t *testing.T) {
	step := &CheckStep{
		Prompt:     "Pick",
		AllowEmpty: true,
		Options: func(model.Spec) []Option {
			return []Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}
		},
		Current: func(model.Spec) []string { return nil },
		Apply:   func(spec *model.Spec, ids []string) { spec.Packs = ids },
	}
	w := NewWizard("test", defaultSpec(), []Step{step, &probeStep{title: "next"}})
	w.Init()

	w.Update(key("a"))
	w.Update(key("enter"))
	if len(w.Spec().Packs) != 2 {
		t.Errorf("after 'a', Packs = %v, want both", w.Spec().Packs)
	}

	w.Update(key("esc"))
	w.Update(key("n"))
	w.Update(key("enter"))
	if len(w.Spec().Packs) != 0 {
		t.Errorf("after 'n', Packs = %v, want none", w.Spec().Packs)
	}
}

func TestCheckStepRefusesEmptyUnlessAllowed(t *testing.T) {
	step := &CheckStep{
		Prompt: "Platforms",
		Options: func(model.Spec) []Option {
			return []Option{{ID: "android", Label: "Android"}}
		},
		Current: func(model.Spec) []string { return nil },
		Apply:   func(spec *model.Spec, ids []string) { spec.Android = model.Has(ids, "android") },
	}
	w := NewWizard("test", defaultSpec(), []Step{step, &probeStep{title: "next"}})
	w.Init()

	w.Update(key("enter"))
	if w.idx != 0 {
		t.Error("confirming an empty required checklist advanced the wizard")
	}
	if !strings.Contains(w.View(), "at least one") {
		t.Errorf("no explanation shown:\n%s", w.View())
	}
}

func TestSelectStepSkipsDisabledOptions(t *testing.T) {
	step := &SelectStep{
		Prompt: "Layout",
		Options: func(model.Spec) []Option {
			return []Option{
				{ID: "a", Label: "A"},
				{ID: "b", Label: "B", Disabled: true, DisabledNote: "not available"},
				{ID: "c", Label: "C"},
			}
		},
		Current: func(model.Spec) string { return "a" },
		Apply:   func(spec *model.Spec, id string) { spec.AndroidLayout = id },
	}
	w := NewWizard("test", defaultSpec(), []Step{step, &probeStep{title: "next"}})
	w.Init()

	w.Update(key("down")) // must land on C, not the disabled B
	w.Update(key("enter"))

	if w.Spec().AndroidLayout != "c" {
		t.Errorf("AndroidLayout = %q, want c (the disabled option was selectable)", w.Spec().AndroidLayout)
	}
}

// The whole `new` flow must render every screen without panicking, whatever the
// answers are - including the paths where later screens are skipped.
func TestNewFlowRendersEveryStep(t *testing.T) {
	for _, tc := range []struct {
		name    string
		android bool
		ios     bool
	}{
		{"both", true, true},
		{"android only", true, false},
		{"ios only", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := defaultSpec()
			spec.Name = "Tunesic"
			spec.Dir = "tunesic"
			spec.Package = "io.kontour.tunesic"
			spec.Android = tc.android
			spec.IOS = tc.ios

			w, _ := NewFlow(context.Background(), spec)
			w.Init()

			for i, step := range w.steps {
				if step.Skip(w.spec) {
					continue
				}
				step.Enter(&w.spec)
				if got := step.View(w.spec, 80); got == "" && step.Title() == "" {
					t.Errorf("step %d rendered nothing", i)
				}
				if step.Title() == "" {
					t.Errorf("step %d has no title", i)
				}
			}
		})
	}
}

func TestFeatureFlowCollectsADraft(t *testing.T) {
	manifest := model.Manifest{
		Schema: model.ManifestSchema, Name: "Tunesic", Package: "io.kontour.tunesic",
		Android: true, IOS: true, AndroidLayout: "nav3-shell",
		RootTabs: []string{"Home"},
		Features: []model.Feature{{Name: "home", Android: true, RootTab: true}},
	}
	spec := defaultSpec()
	spec.Name = manifest.Name
	spec.AndroidLayout = manifest.AndroidLayout

	w, draft := FeatureFlow(spec, manifest, FeatureDraft{Android: true, Shared: true})
	w.Init()

	typeText(w, "billing")
	w.Update(key("enter")) // name -> what to generate
	w.Update(key("enter")) // keep both -> presentation
	w.Update(key("enter")) // full screen -> review

	if draft.Name != "billing" {
		t.Errorf("Name = %q, want billing", draft.Name)
	}
	if !draft.Android || !draft.Shared {
		t.Errorf("draft = %+v, want both halves", draft)
	}
	if draft.Presentation != "above-nav" {
		t.Errorf("Presentation = %q, want above-nav", draft.Presentation)
	}

	// The review screen must name every file it is about to touch.
	view := w.steps[len(w.steps)-1].View(w.spec, 100)
	for _, want := range []string{"feature/billing/api", "settings.gradle.kts", "SharedModules.kt"} {
		if !strings.Contains(view, want) {
			t.Errorf("the review screen does not mention %q:\n%s", want, view)
		}
	}
}

func TestFeatureFlowRejectsDuplicateNames(t *testing.T) {
	manifest := model.Manifest{
		Schema: model.ManifestSchema, Name: "Tunesic", Android: true,
		Features: []model.Feature{{Name: "home", Android: true}},
	}
	w, _ := FeatureFlow(defaultSpec(), manifest, FeatureDraft{Android: true})
	w.Init()

	typeText(w, "home")
	w.Update(key("enter"))

	if w.idx != 0 {
		t.Error("a duplicate feature name was accepted")
	}
	if !strings.Contains(w.View(), "already exists") {
		t.Errorf("no explanation shown:\n%s", w.View())
	}
}
