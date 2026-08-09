package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
)

// SummaryStep is the final confirmation: everything chosen, in one screen.
//
// What it shows comes from the template, so this step knows nothing about the
// kind of project being generated - only how to lay a summary out.
type SummaryStep struct {
	Heading  string
	Sections func(*scaffold.Answers) []scaffold.Section
	Action   string // "create", "add the feature"
}

func (s *SummaryStep) Title() string { return s.Heading }
func (s *SummaryStep) Help() string {
	return fmt.Sprintf("enter %s · esc back · ctrl+c quit", s.Action)
}
func (s *SummaryStep) Skip(*scaffold.Answers) bool     { return false }
func (s *SummaryStep) Enter(*scaffold.Answers) tea.Cmd { return nil }

func (s *SummaryStep) Update(msg tea.Msg, _ *scaffold.Answers) (tea.Cmd, Outcome) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, StayHere
	}
	switch key.String() {
	case "ctrl+c":
		return nil, Cancel
	case "esc":
		return nil, GoBack
	case "enter":
		return nil, Finish
	}
	return nil, StayHere
}

func (s *SummaryStep) View(a *scaffold.Answers, width int) string {
	return RenderSections(s.Sections(a), width)
}

// RenderSections draws a summary. It is shared with the non-interactive path,
// so what `--yes` prints and what the review screen shows cannot drift apart.
func RenderSections(sections []scaffold.Section, width int) string {
	var b strings.Builder

	labelWidth := 0
	for _, sec := range sections {
		for _, r := range sec.Rows {
			if n := len(r.Label); n > labelWidth {
				labelWidth = n
			}
		}
	}

	for i, sec := range sections {
		if sec.Title != "" {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(styleMuted.Render("  "+sec.Title) + "\n")
		}
		for _, r := range sec.Rows {
			b.WriteString(fmt.Sprintf("  %s %-*s %s\n",
				styleOK.Render(glyphDone), labelWidth+2, r.Label, styleText.Render(r.Value)))
		}
		// A titled section with nothing in it says so, rather than looking like
		// a heading whose contents failed to render.
		if len(sec.Items) == 0 && sec.Title != "" && len(sec.Rows) == 0 {
			b.WriteString("    " + styleMuted.Render("(none)") + "\n")
		} else {
			b.WriteString(bullets(sec.Items))
		}
		if sec.Note != "" {
			b.WriteString("\n  " + styleErr.Render(glyphWarn+" "+wrapIndent(sec.Note, width-6)) + "\n")
		}
	}
	return b.String()
}

func bullets(items []string) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	for _, item := range items {
		b.WriteString("    " + styleMuted.Render(glyphInfo+" ") + styleText.Render(item) + "\n")
	}
	return b.String()
}
