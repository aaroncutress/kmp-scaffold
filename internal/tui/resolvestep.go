package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
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
	Ctx    context.Context
	Holder *Holder

	spinner   spinner.Model
	events    chan tea.Msg
	signature string
	running   bool
	done      bool
	progress  resolveProgressMsg
}

// NewResolveStep builds the resolution step.
func NewResolveStep(ctx context.Context, holder *Holder) *ResolveStep {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styleFocus
	return &ResolveStep{Ctx: ctx, Holder: holder, spinner: sp}
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

func (s *ResolveStep) Skip(model.Spec) bool { return false }

// specSignature captures the answers that change the resolution result, so
// going back and editing them re-runs it while going back and forth does not.
func specSignature(spec model.Spec) string {
	return fmt.Sprintf("%v|%v|%v|%v|%d|%d|%s|%s",
		spec.Packs, spec.SharedUtils, spec.Android, spec.IOS,
		spec.MinSDK, spec.CompileSDK, spec.Channel, spec.GradleVer)
}

func (s *ResolveStep) Enter(spec *model.Spec) tea.Cmd {
	sig := specSignature(*spec)
	if s.done && sig == s.signature {
		return nil
	}
	s.signature = sig
	s.done = false
	s.running = true
	s.progress = resolveProgressMsg{}
	s.events = make(chan tea.Msg, 512)

	snapshot := *spec
	events := s.events
	go func() {
		opts := resolve.Options{
			Channel: catalog.ParseChannel(snapshot.Channel),
			Offline: snapshot.Offline,
			Timeout: 20 * time.Second,
			Progress: func(done, total int, label string) {
				select {
				case events <- resolveProgressMsg{done, total, label}:
				default: // never block resolution on a slow renderer
				}
			},
		}
		result := resolve.Run(s.Ctx, snapshot, opts)
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

func (s *ResolveStep) Update(msg tea.Msg, spec *model.Spec) (tea.Cmd, Outcome) {
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

func (s *ResolveStep) View(spec model.Spec, width int) string {
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

	rows := headlineVersions(spec, res)
	// The value is padded before styling: lipgloss does not pad for us, and
	// styled text cannot be padded by fmt's width verb.
	valueWidth := 0
	for _, r := range rows {
		if n := len(r.value); n > valueWidth {
			valueWidth = n
		}
	}
	for _, r := range rows {
		padding := strings.Repeat(" ", valueWidth-len(r.value)+2)
		b.WriteString(fmt.Sprintf("  %s %-22s %s%s%s\n",
			styleOK.Render(glyphDone),
			r.label,
			styleFocus.Render(r.value),
			padding,
			styleMuted.Render(r.note)))
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

type versionRow struct{ label, value, note string }

// headlineVersions is the short list worth showing on screen; the full set goes
// into the generated version catalog.
func headlineVersions(spec model.Spec, res *resolve.Result) []versionRow {
	add := func(rows []versionRow, label, key string) []versionRow {
		if v := res.V(key); v != "" {
			return append(rows, versionRow{label, v, res.Sources[key]})
		}
		return rows
	}

	var rows []versionRow
	rows = append(rows, versionRow{"Gradle", res.Gradle.Version, "current release"})
	rows = add(rows, "Android Gradle Plugin", catalog.KeyAGP)
	rows = add(rows, "Kotlin", catalog.KeyKotlin)
	rows = add(rows, "KSP", catalog.KeyKSP)
	if spec.Android {
		rows = add(rows, "compileSdk / targetSdk", catalog.KeyCompileSDK)
		rows = add(rows, "minSdk", catalog.KeyMinSDK)
		rows = add(rows, "Compose UI", catalog.KeyComposeCore)
		rows = add(rows, "Material 3", catalog.KeyComposeM3)
		rows = add(rows, "Navigation 3", catalog.KeyNavigation3)
	}
	rows = add(rows, "Koin", catalog.KeyKoin)
	rows = add(rows, "Ktor", catalog.KeyKtor)
	if spec.HasPack("database") {
		rows = add(rows, "Room", catalog.KeyRoom)
	}
	if spec.IOS && spec.HasPack("skie") {
		rows = add(rows, "SKIE", catalog.KeySkie)
	}
	return rows
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
