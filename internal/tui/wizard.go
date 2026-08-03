package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aaroncutress/kmp-scaffold/internal/model"
)

// ErrCancelled is returned when the user aborts the wizard.
var ErrCancelled = errors.New("cancelled")

// Wizard drives a list of steps over a shared spec.
type Wizard struct {
	title string
	steps []Step
	idx   int
	spec  model.Spec

	width  int
	height int

	cancelled bool
	finished  bool
	fatal     error
}

// NewWizard builds a wizard over the given steps.
func NewWizard(title string, spec model.Spec, steps []Step) *Wizard {
	return &Wizard{title: title, spec: spec, steps: steps, width: 80}
}

// Spec returns the answers collected so far.
func (w *Wizard) Spec() model.Spec { return w.spec }

// Init implements tea.Model.
func (w *Wizard) Init() tea.Cmd {
	w.idx = w.firstEnabled(0, +1)
	if w.idx < 0 {
		w.finished = true
		return tea.Quit
	}
	return w.steps[w.idx].Enter(&w.spec)
}

// Update implements tea.Model.
func (w *Wizard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		w.width, w.height = sz.Width, sz.Height
		if w.width < 40 {
			w.width = 40
		}
	}
	if w.finished || w.cancelled {
		return w, nil
	}
	if w.idx < 0 || w.idx >= len(w.steps) {
		w.finished = true
		return w, tea.Quit
	}

	cmd, outcome := w.steps[w.idx].Update(msg, &w.spec)

	switch outcome {
	case Cancel:
		w.cancelled = true
		return w, tea.Quit
	case Finish:
		w.finished = true
		return w, tea.Quit
	case GoNext:
		next := w.firstEnabled(w.idx+1, +1)
		if next < 0 {
			w.finished = true
			return w, tea.Quit
		}
		w.idx = next
		return w, tea.Batch(cmd, w.steps[w.idx].Enter(&w.spec))
	case GoBack:
		prev := w.firstEnabled(w.idx-1, -1)
		if prev < 0 {
			// Already at the first step: esc there means "give up".
			w.cancelled = true
			return w, tea.Quit
		}
		w.idx = prev
		return w, tea.Batch(cmd, w.steps[w.idx].Enter(&w.spec))
	}
	return w, cmd
}

// firstEnabled walks from idx in the given direction to the first step that is
// not skipped, or -1 when there is none.
func (w *Wizard) firstEnabled(idx, dir int) int {
	for idx >= 0 && idx < len(w.steps) {
		if !w.steps[idx].Skip(w.spec) {
			return idx
		}
		idx += dir
	}
	return -1
}

// View implements tea.Model.
func (w *Wizard) View() string {
	if w.cancelled {
		return styleMuted.Render("Cancelled - nothing was written.") + "\n"
	}
	if w.finished {
		return ""
	}
	if w.idx < 0 || w.idx >= len(w.steps) {
		return ""
	}

	step := w.steps[w.idx]

	var b strings.Builder
	b.WriteString(Banner(w.title) + "\n")
	b.WriteString(styleStep.Render(fmt.Sprintf("Step %d of %d", w.visibleIndex()+1, w.visibleTotal())) + "\n\n")
	b.WriteString(styleText.Bold(true).Render(step.Title()) + "\n\n")
	b.WriteString(step.View(w.spec, w.width))
	b.WriteString("\n\n" + styleHelp.Render(step.Help()) + "\n")
	return b.String()
}

func (w *Wizard) visibleTotal() int {
	n := 0
	for _, s := range w.steps {
		if !s.Skip(w.spec) {
			n++
		}
	}
	return n
}

func (w *Wizard) visibleIndex() int {
	n := 0
	for i, s := range w.steps {
		if i == w.idx {
			return n
		}
		if !s.Skip(w.spec) {
			n++
		}
	}
	return n
}

// Run executes the wizard and returns the completed spec.
func (w *Wizard) Run(ctx context.Context) (model.Spec, error) {
	prog := tea.NewProgram(w, tea.WithContext(ctx))
	final, err := prog.Run()
	if err != nil {
		return w.spec, err
	}
	done, ok := final.(*Wizard)
	if !ok {
		return w.spec, fmt.Errorf("unexpected wizard state")
	}
	if done.fatal != nil {
		return done.spec, done.fatal
	}
	if done.cancelled {
		return done.spec, ErrCancelled
	}
	return done.spec, nil
}
