package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
)

// Outcome is what a step wants the wizard to do next.
type Outcome int

const (
	// StayHere means the step handled the key itself.
	StayHere Outcome = iota
	// GoNext advances to the following step.
	GoNext
	// GoBack returns to the previous step.
	GoBack
	// Cancel aborts the whole wizard.
	Cancel
	// Finish ends the wizard successfully from this step.
	Finish
)

// Step is one screen of the wizard.
//
// Steps work on a scaffold.Answers rather than any particular project type, so
// the same four widgets answer a built-in template's questions and a
// file-based one's.
type Step interface {
	// Title is shown in the header.
	Title() string
	// Enter is called each time the step becomes active, so it can seed its
	// state from answers given earlier.
	Enter(a *scaffold.Answers) tea.Cmd
	// Update handles a message and writes any answer back into a.
	Update(msg tea.Msg, a *scaffold.Answers) (tea.Cmd, Outcome)
	// View renders the step body (the wizard draws the header and footer).
	View(a *scaffold.Answers, width int) string
	// Help is the footer key hint.
	Help() string
	// Skip hides the step when earlier answers make it irrelevant.
	Skip(a *scaffold.Answers) bool
}

// Option is one choice in a select or checklist step.
type Option = scaffold.Option

// ---------------------------------------------------------------------------
// Text input
// ---------------------------------------------------------------------------

// TextStep asks for a single line of text.
type TextStep struct {
	Prompt   string
	Hint     string
	Default  func(a *scaffold.Answers) string
	Validate func(string, *scaffold.Answers) error
	Apply    func(*scaffold.Answers, string)
	SkipIf   func(*scaffold.Answers) bool

	// value is what the user has typed. It starts empty even when there is a
	// default: the default is shown as a placeholder and accepted by pressing
	// enter, so the first keystroke does not have to be a backspace.
	value    string
	err      error
	fallback string
	// answered records that this step has been confirmed once, so returning to
	// it shows the answer as editable text rather than as a placeholder.
	answered bool
}

func (s *TextStep) Title() string { return s.Prompt }
func (s *TextStep) Help() string  { return "enter confirm · esc back · ctrl+c quit" }

func (s *TextStep) Skip(a *scaffold.Answers) bool {
	return s.SkipIf != nil && s.SkipIf(a)
}

func (s *TextStep) Enter(a *scaffold.Answers) tea.Cmd {
	if s.Default != nil {
		s.fallback = s.Default(a)
	}
	if s.answered && s.value == "" {
		// The user confirmed the default, then came back: show it so they can
		// edit it rather than making them retype it.
		s.value = s.fallback
	}
	return nil
}

// effective is the value that would be submitted right now.
func (s *TextStep) effective() string {
	if s.value != "" {
		return s.value
	}
	return s.fallback
}

func (s *TextStep) Update(msg tea.Msg, a *scaffold.Answers) (tea.Cmd, Outcome) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, StayHere
	}
	switch key.Type {
	case tea.KeyCtrlC:
		return nil, Cancel
	case tea.KeyEsc:
		return nil, GoBack
	case tea.KeyEnter:
		value := strings.TrimSpace(s.effective())
		if s.Validate != nil {
			if err := s.Validate(value, a); err != nil {
				s.err = err
				return nil, StayHere
			}
		}
		s.err = nil
		s.answered = true
		s.Apply(a, value)
		return nil, GoNext
	case tea.KeyBackspace:
		// The first backspace on a placeholder makes it editable, minus its
		// last character - which is what you want when tweaking a default.
		if s.value == "" {
			s.value = s.fallback
		}
		if len(s.value) > 0 {
			runes := []rune(s.value)
			s.value = string(runes[:len(runes)-1])
			s.err = nil
		}
	case tea.KeyCtrlU:
		s.value = ""
		s.fallback = ""
		s.err = nil
	case tea.KeySpace:
		s.value += " "
	case tea.KeyRunes:
		s.value += string(key.Runes)
		s.err = nil
	}
	return nil, StayHere
}

func (s *TextStep) View(_ *scaffold.Answers, width int) string {
	var b strings.Builder
	if s.Hint != "" {
		b.WriteString(styleMuted.Render(s.Hint) + "\n\n")
	}
	var shown string
	switch {
	case s.value != "":
		shown = styleText.Render(s.value) + styleFocus.Render("▏")
	case s.fallback != "":
		// Dimmed: pressing enter takes it, typing replaces it.
		shown = styleFocus.Render("▏") + styleMuted.Render(s.fallback)
	default:
		shown = styleFocus.Render("▏")
	}
	b.WriteString(styleBox.Width(min(width-4, 68)).Render(shown))
	if s.err != nil {
		b.WriteString("\n\n" + styleErr.Render(glyphWarn+" "+s.err.Error()))
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Single select
// ---------------------------------------------------------------------------

// SelectStep asks the user to pick exactly one option.
type SelectStep struct {
	Prompt  string
	Hint    string
	Options func(a *scaffold.Answers) []Option
	Current func(a *scaffold.Answers) string
	Apply   func(*scaffold.Answers, string)
	SkipIf  func(*scaffold.Answers) bool

	options []Option
	cursor  int
}

func (s *SelectStep) Title() string { return s.Prompt }
func (s *SelectStep) Help() string  { return "↑/↓ move · enter select · esc back · ctrl+c quit" }

func (s *SelectStep) Skip(a *scaffold.Answers) bool {
	return s.SkipIf != nil && s.SkipIf(a)
}

func (s *SelectStep) Enter(a *scaffold.Answers) tea.Cmd {
	s.options = s.Options(a)
	if s.Current != nil {
		current := s.Current(a)
		for i, o := range s.options {
			if o.ID == current {
				s.cursor = i
			}
		}
	}
	return nil
}

func (s *SelectStep) Update(msg tea.Msg, a *scaffold.Answers) (tea.Cmd, Outcome) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, StayHere
	}
	switch key.String() {
	case "ctrl+c":
		return nil, Cancel
	case "esc", "left", "h":
		return nil, GoBack
	case "up", "k":
		s.move(-1)
	case "down", "j":
		s.move(1)
	case "enter", "right", "l":
		if len(s.options) == 0 {
			return nil, GoNext
		}
		if s.options[s.cursor].Disabled {
			return nil, StayHere
		}
		s.Apply(a, s.options[s.cursor].ID)
		return nil, GoNext
	}
	return nil, StayHere
}

func (s *SelectStep) move(delta int) {
	if len(s.options) == 0 {
		return
	}
	for range len(s.options) {
		s.cursor = (s.cursor + delta + len(s.options)) % len(s.options)
		if !s.options[s.cursor].Disabled {
			return
		}
	}
}

func (s *SelectStep) View(_ *scaffold.Answers, width int) string {
	var b strings.Builder
	if s.Hint != "" {
		b.WriteString(styleMuted.Render(s.Hint) + "\n\n")
	}
	for i, o := range s.options {
		mark := glyphRadioOff
		line := styleText
		prefix := "  "
		if i == s.cursor {
			prefix = styleFocus.Render(glyphCursor) + " "
			mark = glyphRadioOn
			line = styleFocus
		}
		if o.Disabled {
			line = styleMuted
		}
		b.WriteString(prefix + line.Render(mark+" "+o.Label) + "\n")
		desc := o.Desc
		if o.Disabled && o.DisabledNote != "" {
			desc = o.DisabledNote
		}
		if desc != "" {
			b.WriteString(styleMuted.Render(describe(desc, width)) + "\n")
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Checklist
// ---------------------------------------------------------------------------

// CheckStep is a multi-select checklist - the wizard's signature screen.
type CheckStep struct {
	Prompt  string
	Hint    string
	Options func(a *scaffold.Answers) []Option
	Current func(a *scaffold.Answers) []string
	Apply   func(*scaffold.Answers, []string)
	SkipIf  func(*scaffold.Answers) bool
	// AllowEmpty permits confirming with nothing ticked.
	AllowEmpty bool

	options  []Option
	selected map[string]bool
	cursor   int
	err      error
}

func (s *CheckStep) Title() string { return s.Prompt }
func (s *CheckStep) Help() string {
	return "↑/↓ move · space toggle · a all · n none · enter confirm · esc back"
}

func (s *CheckStep) Skip(a *scaffold.Answers) bool {
	return s.SkipIf != nil && s.SkipIf(a)
}

func (s *CheckStep) Enter(a *scaffold.Answers) tea.Cmd {
	s.options = s.Options(a)
	if s.selected == nil {
		s.selected = map[string]bool{}
		for _, id := range s.Current(a) {
			s.selected[id] = true
		}
	}
	if s.cursor >= len(s.options) {
		s.cursor = 0
	}
	return nil
}

func (s *CheckStep) Update(msg tea.Msg, a *scaffold.Answers) (tea.Cmd, Outcome) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, StayHere
	}
	switch key.String() {
	case "ctrl+c":
		return nil, Cancel
	case "esc", "left":
		return nil, GoBack
	case "up", "k":
		s.move(-1)
	case "down", "j":
		s.move(1)
	case " ", "x":
		if len(s.options) > 0 && !s.options[s.cursor].Disabled {
			id := s.options[s.cursor].ID
			s.selected[id] = !s.selected[id]
			s.err = nil
		}
	case "a":
		for _, o := range s.options {
			if !o.Disabled {
				s.selected[o.ID] = true
			}
		}
	case "n":
		s.selected = map[string]bool{}
	case "enter":
		var chosen []string
		for _, o := range s.options {
			if s.selected[o.ID] && !o.Disabled {
				chosen = append(chosen, o.ID)
			}
		}
		if len(chosen) == 0 && !s.AllowEmpty {
			s.err = fmt.Errorf("pick at least one, or press esc to go back")
			return nil, StayHere
		}
		s.Apply(a, chosen)
		return nil, GoNext
	}
	return nil, StayHere
}

func (s *CheckStep) move(delta int) {
	if len(s.options) == 0 {
		return
	}
	s.cursor = (s.cursor + delta + len(s.options)) % len(s.options)
}

func (s *CheckStep) View(_ *scaffold.Answers, width int) string {
	var b strings.Builder
	if s.Hint != "" {
		b.WriteString(styleMuted.Render(s.Hint) + "\n\n")
	}
	for i, o := range s.options {
		mark := glyphUnchecked
		if s.selected[o.ID] {
			mark = glyphChecked
		}
		prefix := "  "
		line := styleText
		if i == s.cursor {
			prefix = styleFocus.Render(glyphCursor) + " "
			line = styleFocus
		}
		if o.Disabled {
			line = styleMuted
			mark = "-"
		}
		markStyle := styleMuted
		if s.selected[o.ID] && !o.Disabled {
			markStyle = styleOK
		}
		b.WriteString(prefix + markStyle.Render(mark) + " " + line.Render(o.Label) + "\n")
		desc := o.Desc
		if o.Disabled && o.DisabledNote != "" {
			desc = o.DisabledNote
		}
		if desc != "" {
			b.WriteString(styleMuted.Render(describe(desc, width)) + "\n")
		}
	}
	if s.err != nil {
		b.WriteString("\n" + styleErr.Render(glyphWarn+" "+s.err.Error()))
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Confirm
// ---------------------------------------------------------------------------

// ConfirmStep is a yes/no question.
type ConfirmStep struct {
	Prompt  string
	Hint    string
	Default bool
	Apply   func(*scaffold.Answers, bool)
	SkipIf  func(*scaffold.Answers) bool

	value   bool
	entered bool
}

func (s *ConfirmStep) Title() string { return s.Prompt }
func (s *ConfirmStep) Help() string  { return "y/n · enter confirm · esc back · ctrl+c quit" }

func (s *ConfirmStep) Skip(a *scaffold.Answers) bool {
	return s.SkipIf != nil && s.SkipIf(a)
}

func (s *ConfirmStep) Enter(*scaffold.Answers) tea.Cmd {
	if !s.entered {
		s.value = s.Default
		s.entered = true
	}
	return nil
}

func (s *ConfirmStep) Update(msg tea.Msg, a *scaffold.Answers) (tea.Cmd, Outcome) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, StayHere
	}
	switch key.String() {
	case "ctrl+c":
		return nil, Cancel
	case "esc":
		return nil, GoBack
	case "y", "Y":
		s.value = true
		s.Apply(a, true)
		return nil, GoNext
	case "n", "N":
		s.value = false
		s.Apply(a, false)
		return nil, GoNext
	case "left", "right", "tab", "h", "l":
		s.value = !s.value
	case "enter":
		s.Apply(a, s.value)
		return nil, GoNext
	}
	return nil, StayHere
}

func (s *ConfirmStep) View(_ *scaffold.Answers, width int) string {
	var b strings.Builder
	if s.Hint != "" {
		b.WriteString(styleMuted.Render(s.Hint) + "\n\n")
	}
	yes, no := "  Yes  ", "  No  "
	if s.value {
		b.WriteString(styleFocus.Render("["+yes+"]") + "  " + styleMuted.Render(" "+no+" "))
	} else {
		b.WriteString(styleMuted.Render(" "+yes+" ") + "  " + styleFocus.Render("["+no+"]"))
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// wrap breaks text on word boundaries at the given width.
//
// lipgloss's Width() is not used here because it pads every line out to the
// full width, and lipgloss.Render then strips the trailing newline - which
// leaves the next line starting after a run of spaces.
func wrap(s string, width int) string {
	if width < 20 {
		width = 20
	}
	var out strings.Builder
	col := 0
	for i, word := range strings.Fields(s) {
		w := len([]rune(word))
		switch {
		case i == 0:
			out.WriteString(word)
			col = w
		case col+1+w > width:
			out.WriteString("\n" + word)
			col = w
		default:
			out.WriteString(" " + word)
			col += 1 + w
		}
	}
	return out.String()
}

// describe renders an option's description indented under its label, keeping
// wrapped continuation lines lined up with the first. The caller adds the
// trailing newline, since lipgloss strips one from the end of a Render.
func describe(text string, width int) string {
	const indent = "    "
	wrapped := wrap(text, width-len(indent)-2)
	return indent + strings.ReplaceAll(wrapped, "\n", "\n"+indent)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
