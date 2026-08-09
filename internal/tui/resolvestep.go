package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
)

type resolveProgressMsg struct {
	done, total int
	label       string
}

type resolveDoneMsg struct{ result *resolve.Result }

// Holder carries the resolution result out of the wizard.
type Holder struct{ Result *resolve.Result }

// ResolveStep queries the repositories for the newest compatible versions and
// then shows what it found, so nothing is generated against versions the user
// has not seen.
type ResolveStep struct {
	Ctx      context.Context
	Template scaffold.Template
	Holder   *Holder

	spinner   spinner.Model
	events    chan tea.Msg
	signature string
	running   bool
	done      bool
	progress  resolveProgressMsg
}

// NewResolveStep builds the resolution step.
func NewResolveStep(ctx context.Context, t scaffold.Template, holder *Holder) *ResolveStep {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styleFocus
	return &ResolveStep{Ctx: ctx, Template: t, Holder: holder, spinner: sp}
}

func (s *ResolveStep) Title() string {
	if s.done {
		return "Resolved versions"
	}
	return "Working out the latest compatible versions"
}

func (s *ResolveStep) Help() string {
	if s.done {
		return "enter continue · esc back · ctrl+c quit"
	}
	return "ctrl+c quit"
}

// Skip drops the step entirely for a template that resolves nothing.
func (s *ResolveStep) Skip(a *scaffold.Answers) bool {
	return len(s.Template.Versions(a).Keys) == 0
}

func (s *ResolveStep) Enter(a *scaffold.Answers) tea.Cmd {
	req := s.Template.Versions(a)

	// The request itself is the signature, so editing an answer that changes
	// the outcome re-runs resolution while going back and forth does not.
	sig := req.Signature()
	if s.done && sig == s.signature {
		return nil
	}
	s.signature = sig
	s.done = false
	s.running = true
	s.progress = resolveProgressMsg{}
	s.events = make(chan tea.Msg, 512)

	events := s.events
	req.Progress = func(done, total int, label string) {
		select {
		case events <- resolveProgressMsg{done, total, label}:
		default: // never block resolution on a slow renderer
		}
	}
	template := s.Template
	go func() {
		result := resolve.Run(s.Ctx, req)
		template.Check(a, result)
		events <- resolveDoneMsg{result}
	}()

	return tea.Batch(s.spinner.Tick, waitForEvent(events))
}

func waitForEvent(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (s *ResolveStep) Update(msg tea.Msg, a *scaffold.Answers) (tea.Cmd, Outcome) {
	switch m := msg.(type) {
	case resolveProgressMsg:
		s.progress = m
		return waitForEvent(s.events), StayHere

	case resolveDoneMsg:
		s.Holder.Result = m.result
		s.running = false
		s.done = true
		return nil, StayHere

	case spinner.TickMsg:
		if !s.running {
			return nil, StayHere
		}
		sp, cmd := s.spinner.Update(m)
		s.spinner = sp
		return cmd, StayHere

	case tea.KeyMsg:
		switch m.String() {
		case "ctrl+c":
			return nil, Cancel
		case "esc":
			if s.running {
				return nil, StayHere
			}
			return nil, GoBack
		case "enter":
			if s.done {
				return nil, GoNext
			}
		}
	}
	return nil, StayHere
}

func (s *ResolveStep) View(a *scaffold.Answers, width int) string {
	var b strings.Builder

	if s.running {
		b.WriteString(s.spinner.View() + " ")
		if s.progress.total > 0 {
			b.WriteString(styleText.Render(fmt.Sprintf(
				"%d/%d  ", s.progress.done, s.progress.total)))
			b.WriteString(styleMuted.Render(s.progress.label))
			b.WriteString("\n\n" + progressBar(s.progress.done, s.progress.total, min(width-4, 48)))
		} else {
			b.WriteString(styleMuted.Render("contacting Maven Central, Google Maven and services.gradle.org"))
		}
		return b.String()
	}

	res := s.Holder.Result
	if res == nil {
		return styleMuted.Render("nothing resolved")
	}

	rows := s.Template.Headlines(a, res)
	// The value is padded before styling: lipgloss does not pad for us, and
	// styled text cannot be padded by fmt's width verb.
	valueWidth := 0
	for _, r := range rows {
		if n := len(r.Value); n > valueWidth {
			valueWidth = n
		}
	}
	for _, r := range rows {
		padding := strings.Repeat(" ", valueWidth-len(r.Value)+2)
		b.WriteString(fmt.Sprintf("  %s %-22s %s%s%s\n",
			styleOK.Render(glyphDone),
			r.Label,
			styleFocus.Render(r.Value),
			padding,
			styleMuted.Render(r.Note)))
	}

	if len(res.Notes) > 0 {
		b.WriteString("\n")
		for _, n := range res.Notes {
			switch n.Level {
			case resolve.Error:
				b.WriteString("  " + styleErr.Render(glyphWarn+" "+wrapIndent(n.Text, width-6)) + "\n")
			case resolve.Warn:
				b.WriteString("  " + styleWarn.Render(glyphWarn+" "+wrapIndent(n.Text, width-6)) + "\n")
			default:
				b.WriteString("  " + styleMuted.Render(glyphInfo+" "+wrapIndent(n.Text, width-6)) + "\n")
			}
		}
	}

	b.WriteString("\n" + styleMuted.Render(fmt.Sprintf(
		"%d artifacts checked in %s", res.Probed, res.Elapsed.Round(time.Millisecond))))
	return b.String()
}

func progressBar(done, total, width int) string {
	if total <= 0 || width <= 0 {
		return ""
	}
	filled := done * width / total
	if filled > width {
		filled = width
	}
	return styleFocus.Render(strings.Repeat("━", filled)) +
		styleMuted.Render(strings.Repeat("━", width-filled))
}

func wrapIndent(s string, width int) string {
	return strings.ReplaceAll(wrap(s, width), "\n", "\n    ")
}
