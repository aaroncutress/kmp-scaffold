package cli

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"

	"github.com/aaroncutress/kmp-scaffold/internal/render"
	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
)

var (
	sAccent = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#3B5BDB", Dark: "#91A7FF"})
	sMuted  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6C757D", Dark: "#909296"})
	sOK     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#2B8A3E", Dark: "#8CE99A"})
	sWarn   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#E67700", Dark: "#FFD43B"})
	sErr    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#C92A2A", Dark: "#FF8787"}).Bold(true)
	sBold   = lipgloss.NewStyle().Bold(true)
)

func dim(s string) string       { return sMuted.Render(s) }
func errorLine(s string) string { return sErr.Render("error: ") + s }

// interactive reports whether we can run a full-screen wizard.
func interactive() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}

// printWriteSummary reports what a render.Writer did.
func printWriteSummary(w *render.Writer, verbose bool) {
	created := w.Count(render.Created)
	overwritten := w.Count(render.Overwritten)
	skipped := w.Count(render.Skipped)
	unchanged := w.Count(render.Unchanged)
	planned := w.Count(render.Planned)
	sidecars := w.Count(render.Sidecar)

	if verbose || planned > 0 {
		for _, a := range w.Actions() {
			switch a.Status {
			case render.Created, render.Planned:
				fmt.Printf("  %s %s\n", sOK.Render("+"), a.Path)
			case render.Overwritten:
				fmt.Printf("  %s %s\n", sWarn.Render("~"), a.Path)
			case render.Skipped:
				fmt.Printf("  %s %s %s\n", sWarn.Render("!"), a.Path, sMuted.Render("(exists, left alone)"))
			case render.Sidecar:
				fmt.Printf("  %s %s %s\n", sWarn.Render("!"), a.Path+".new",
					sMuted.Render("(yours was left alone)"))
			}
		}
		fmt.Println()
	}

	var parts []string
	if planned > 0 {
		parts = append(parts, fmt.Sprintf("%d file(s) would be written", planned))
	}
	if created > 0 {
		parts = append(parts, fmt.Sprintf("%d created", created))
	}
	if overwritten > 0 {
		parts = append(parts, fmt.Sprintf("%d overwritten", overwritten))
	}
	if unchanged > 0 {
		parts = append(parts, fmt.Sprintf("%d unchanged", unchanged))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	if sidecars > 0 {
		parts = append(parts, fmt.Sprintf("%d written alongside", sidecars))
	}
	if len(parts) > 0 {
		fmt.Println(sMuted.Render(strings.Join(parts, ", ")))
	}

	if skipped > 0 {
		fmt.Println()
		fmt.Println(sWarn.Render("Some files already existed and were left untouched:"))
		for _, p := range w.Conflicts() {
			fmt.Println("  " + p)
		}
		fmt.Println(sMuted.Render("Re-run with --force to overwrite them."))
	}

	if sidecars > 0 {
		fmt.Println()
		fmt.Println(sWarn.Render("These already existed, so what would have been written is beside them:"))
		for _, p := range w.SidecarPaths() {
			fmt.Printf("  %s  %s\n", p+".new", sMuted.Render("← compare against "+path.Base(p)))
		}
		fmt.Println(sMuted.Render(
			"Diff them, take what you want, and delete the .new files. --force overwrites instead."))
	}
}

// printResolveNotes prints the compatibility report.
func printResolveNotes(res *resolve.Result) {
	if len(res.Notes) == 0 {
		return
	}
	fmt.Println()
	for _, n := range res.Notes {
		switch n.Level {
		case resolve.Error:
			fmt.Println("  " + sErr.Render("! "+n.Text))
		case resolve.Warn:
			fmt.Println("  " + sWarn.Render("! "+n.Text))
		default:
			fmt.Println("  " + sMuted.Render("· "+n.Text))
		}
	}
}

// printVersionTable prints resolved versions, sorted by key.
func printVersionTable(res *resolve.Result) {
	keys := make([]string, 0, len(res.Versions))
	for k := range res.Versions {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	width := 0
	for _, k := range keys {
		if len(k) > width {
			width = len(k)
		}
	}
	for _, k := range keys {
		fmt.Printf("  %-*s  %s  %s\n", width, k,
			sAccent.Render(res.Versions[k]), sMuted.Render(res.Sources[k]))
	}
}
