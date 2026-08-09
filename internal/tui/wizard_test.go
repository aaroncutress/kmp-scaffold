package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aaroncutress/kmp-scaffold/internal/kmp"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
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

// A step that records how many times it was entered, so the tests can assert on
// wizard navigation without a terminal.
type probeStep struct {
	title   string
	entered int
	skip    bool
}

func (p *probeStep) Title() string                      { return p.title }
func (p *probeStep) Help() string                       { return "" }
func (p *probeStep) Skip(*scaffold.Answers) bool        { return p.skip }
func (p *probeStep) View(*scaffold.Answers, int) string { return p.title }

func (p *probeStep) Enter(*scaffold.Answers) tea.Cmd {
	p.entered++
	return nil
}

func (p *probeStep) Update(msg tea.Msg, _ *scaffold.Answers) (tea.Cmd, Outcome) {
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

func TestWizardSkipsDisabledSteps(t *testing.T) {
	first := &probeStep{title: "first"}
	skipped := &probeStep{title: "skipped", skip: true}
	last := &probeStep{title: "last"}

	w := NewWizard("test", scaffold.NewAnswers(), []Step{first, skipped, last})
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

	w := NewWizard("test", scaffold.NewAnswers(), []Step{first, second})
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
	w := NewWizard("test", scaffold.NewAnswers(), []Step{&probeStep{title: "only"}})
	w.Init()
	w.Update(key("esc"))

	if !w.cancelled {
		t.Error("esc on the first step should cancel the wizard")
	}
}

func TestTextStepValidates(t *testing.T) {
	step := StepFor(scaffold.Question{
		ID:       "name",
		Kind:     scaffold.KindText,
		Prompt:   "Name?",
		Validate: func(v any, _ *scaffold.Answers) error { return model.ValidateProjectName(v.(string)) },
	})
	w := NewWizard("test", scaffold.NewAnswers(), []Step{step, &probeStep{title: "next"}})
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

	// Correcting it lets us through, and the answer lands in the bag.
	for range 4 {
		w.Update(key("backspace"))
	}
	typeText(w, "Tunesic")
	w.Update(key("enter"))

	if w.idx != 1 {
		t.Error("a valid value did not advance the wizard")
	}
	if got := w.Answers().Str("name"); got != "Tunesic" {
		t.Errorf("name = %q, want Tunesic", got)
	}
}

func TestCheckStepTogglesAndApplies(t *testing.T) {
	step := StepFor(scaffold.Question{
		ID:     "picks",
		Kind:   scaffold.KindMultiSelect,
		Prompt: "Pick",
		Options: []scaffold.Option{
			{ID: "a", Label: "A"}, {ID: "b", Label: "B"}, {ID: "c", Label: "C"},
		},
		Default: []string{"a"},
	})
	w := NewWizard("test", scaffold.NewAnswers(), []Step{step, &probeStep{title: "next"}})
	w.Init()

	w.Update(key("down"))  // cursor on B
	w.Update(key("space")) // tick B
	w.Update(key("down"))  // cursor on C
	w.Update(key("space")) // tick C
	w.Update(key("space")) // untick C again
	w.Update(key("enter"))

	got := w.Answers().Strs("picks")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("picks = %v, want [a b]", got)
	}
}

func TestCheckStepSelectAllAndNone(t *testing.T) {
	step := StepFor(scaffold.Question{
		ID:         "picks",
		Kind:       scaffold.KindMultiSelect,
		Prompt:     "Pick",
		AllowEmpty: true,
		Options:    []scaffold.Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
	})
	w := NewWizard("test", scaffold.NewAnswers(), []Step{step, &probeStep{title: "next"}})
	w.Init()

	w.Update(key("a"))
	w.Update(key("enter"))
	if got := w.Answers().Strs("picks"); len(got) != 2 {
		t.Errorf("after 'a', picks = %v, want both", got)
	}

	w.Update(key("esc"))
	w.Update(key("n"))
	w.Update(key("enter"))
	if got := w.Answers().Strs("picks"); len(got) != 0 {
		t.Errorf("after 'n', picks = %v, want none", got)
	}
}

func TestCheckStepRefusesEmptyUnlessAllowed(t *testing.T) {
	step := StepFor(scaffold.Question{
		ID:      "platforms",
		Kind:    scaffold.KindMultiSelect,
		Prompt:  "Platforms",
		Options: []scaffold.Option{{ID: "android", Label: "Android"}},
	})
	w := NewWizard("test", scaffold.NewAnswers(), []Step{step, &probeStep{title: "next"}})
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
	step := StepFor(scaffold.Question{
		ID:     "layout",
		Kind:   scaffold.KindSelect,
		Prompt: "Layout",
		Options: []scaffold.Option{
			{ID: "a", Label: "A"},
			{ID: "b", Label: "B", Disabled: true, DisabledNote: "not available"},
			{ID: "c", Label: "C"},
		},
		Default: "a",
	})
	w := NewWizard("test", scaffold.NewAnswers(), []Step{step, &probeStep{title: "next"}})
	w.Init()

	w.Update(key("down")) // must land on C, not the disabled B
	w.Update(key("enter"))

	if got := w.Answers().Str("layout"); got != "c" {
		t.Errorf("layout = %q, want c (the disabled option was selectable)", got)
	}
}

// A list question is one comma-separated line, and comes back as a list.
func TestListStepSplitsAndValidates(t *testing.T) {
	step := StepFor(scaffold.Question{
		ID:     "tabs",
		Kind:   scaffold.KindList,
		Prompt: "Tabs?",
		Validate: func(v any, _ *scaffold.Answers) error {
			if len(v.([]string)) < 2 {
				return errors.New("name at least two")
			}
			return nil
		},
	})
	w := NewWizard("test", scaffold.NewAnswers(), []Step{step, &probeStep{title: "next"}})
	w.Init()

	typeText(w, "Home")
	w.Update(key("enter"))
	if w.idx != 0 {
		t.Error("a list that failed validation advanced the wizard")
	}

	typeText(w, ", Settings ,")
	w.Update(key("enter"))

	got := w.Answers().Strs("tabs")
	if len(got) != 2 || got[0] != "Home" || got[1] != "Settings" {
		t.Errorf("tabs = %#v, want [Home Settings] with the trailing comma dropped", got)
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
			template := kmp.Template{}
			answers := template.NewAnswers()
			answers.Project.Name = "Tunesic"
			answers.Project.Dir = "tunesic"
			answers.Project.Package = "io.kontour.tunesic"

			spec := kmp.Spec(answers)
			spec.Android = tc.android
			spec.IOS = tc.ios
			spec.Offline = true
			template.Bind(answers)

			w, _ := NewFlow(context.Background(), template, answers)
			w.Init()

			for i, step := range w.steps {
				if step.Skip(w.answers) {
					continue
				}
				step.Enter(w.answers)
				if got := step.View(w.answers, 80); got == "" && step.Title() == "" {
					t.Errorf("step %d rendered nothing", i)
				}
				if step.Title() == "" {
					t.Errorf("step %d has no title", i)
				}
			}
		})
	}
}

// Picking a template is its own screen, because the answer decides what the
// rest of the wizard asks.
func TestPickTemplate(t *testing.T) {
	template := kmp.Template{}
	w := PickTemplate([]scaffold.Template{template})
	w.Init()
	w.Update(key("enter"))

	if got := w.Answers().Str(TemplateQuestion); got != kmp.ID {
		t.Errorf("template = %q, want %q", got, kmp.ID)
	}
}

func kmpManifest(t *testing.T) *model.Manifest {
	t.Helper()
	m, err := model.NewManifest("test", kmp.Template{}.Meta().Ref(),
		model.Project{Name: "Tunesic", Package: "io.kontour.tunesic"},
		kmp.Vars{
			Android: true, IOS: true, AndroidLayout: "nav3-shell",
			IOSLayout: "swiftui-features", RootTabs: []string{"Home"},
		}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := model.NewFeature("home", kmp.FeatureVars{Android: true, RootTab: true})
	if err != nil {
		t.Fatal(err)
	}
	m.PutFeature(rec)
	return &m
}

func TestFeatureFlowCollectsAnswers(t *testing.T) {
	template := kmp.Template{}
	manifest := kmpManifest(t)

	answers := scaffold.NewAnswers()
	answers.Set(kmp.QFeatureTargets, []string{"android", "shared"})

	w := FeatureFlow(template, manifest, answers)
	w.Init()

	typeText(w, "billing")
	w.Update(key("enter")) // name -> what to generate
	w.Update(key("enter")) // keep both -> presentation
	w.Update(key("enter")) // full screen -> review

	if got := w.Answers().Project.Name; got != "billing" {
		t.Errorf("name = %q, want billing", got)
	}
	targets := w.Answers().Strs(kmp.QFeatureTargets)
	if !model.Has(targets, "android") || !model.Has(targets, "shared") {
		t.Errorf("targets = %v, want both halves", targets)
	}
	if got := w.Answers().Str(kmp.QFeaturePresentation); got != "above-nav" {
		t.Errorf("presentation = %q, want above-nav", got)
	}

	// The review screen must name every file it is about to touch.
	view := w.steps[len(w.steps)-1].View(w.answers, 100)
	for _, want := range []string{"feature/billing/api", "settings.gradle.kts", "SharedModules.kt"} {
		if !strings.Contains(view, want) {
			t.Errorf("the review screen does not mention %q:\n%s", want, view)
		}
	}
}

func TestFeatureFlowRejectsDuplicateNames(t *testing.T) {
	w := FeatureFlow(kmp.Template{}, kmpManifest(t), scaffold.NewAnswers())
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
