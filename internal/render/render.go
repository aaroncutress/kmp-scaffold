// Package render turns templates into files on disk.
//
// Every generator produces its output through an Engine (which owns the parsed
// template set and the function map available to templates) and a Writer
// (which owns collision handling, dry runs and the record of what changed).
package render

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/aaroncutress/kmp-scaffold/internal/model"
)

// Engine holds the parsed templates and renders them by name.
type Engine struct {
	tpl *template.Template
}

// NewEngine parses every *.tmpl file in the given filesystem.
func NewEngine(fsys fs.FS) (*Engine, error) { return NewEngineFS(fsys, "*.tmpl") }

// NewEngineFS parses the files matching the given patterns. A template made of
// loose files rather than one big set of {{define}} blocks uses this with its
// own layout.
func NewEngineFS(fsys fs.FS, patterns ...string) (*Engine, error) {
	tpl := template.New("kmp-scaffold").Funcs(FuncMap())
	if len(patterns) == 0 {
		return &Engine{tpl: tpl}, nil
	}
	tpl, err := tpl.ParseFS(fsys, patterns...)
	if err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}
	return &Engine{tpl: tpl}, nil
}

// NewEmptyEngine builds an engine with no templates parsed, for callers that
// only render ad-hoc text.
func NewEmptyEngine() *Engine {
	return &Engine{tpl: template.New("kmp-scaffold").Funcs(FuncMap())}
}

// RenderString parses and executes ad-hoc text. The engine's own templates are
// in scope, so a file-based template's files can call shared partials.
//
// The tree is cloned per call because parsing into the shared one would let a
// file's {{define}} blocks leak into the next file rendered.
func (e *Engine) RenderString(name, text string, data any) ([]byte, error) {
	clone, err := e.tpl.Clone()
	if err != nil {
		return nil, err
	}
	t, err := clone.New(name).Parse(text)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("rendering %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

// Truthy renders text and reports whether the result means yes. It is how a
// declarative `when = "..."` condition is evaluated: one template language for
// the whole template rather than a second expression syntax to learn.
func (e *Engine) Truthy(text string, data any) (bool, error) {
	if strings.TrimSpace(text) == "" {
		return true, nil
	}
	out, err := e.RenderString("condition", text, data)
	if err != nil {
		return false, err
	}
	switch strings.TrimSpace(string(out)) {
	case "", "false", "0", "no", "off", "[]", "map[]", "<no value>":
		return false, nil
	default:
		return true, nil
	}
}

// Normalise is the whitespace tidy-up applied to rendered output: trailing
// whitespace trimmed, runs of blank lines collapsed, exactly one final newline.
// It is exported so a template can render its own text through the same rules.
func Normalise(b []byte) []byte { return normalise(b) }

// Render executes a named template block against data.
func (e *Engine) Render(name string, data any) ([]byte, error) {
	t := e.tpl.Lookup(name)
	if t == nil {
		return nil, fmt.Errorf("no template named %q", name)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("rendering %s: %w", name, err)
	}
	return normalise(buf.Bytes()), nil
}

// normalise trims trailing whitespace on each line and ensures the file ends
// with exactly one newline. Go templates leave a lot of ragged whitespace
// behind when conditionals are involved.
func normalise(b []byte) []byte {
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	out := strings.Join(lines, "\n")
	// Conditionals leave ragged gaps behind, so collapse any run of blank lines
	// to a single one.
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	// A {{define}} block starts with the newline after the tag, so every
	// rendered file would otherwise begin with a blank line.
	out = strings.Trim(out, "\n")
	if out == "" {
		return nil
	}
	return []byte(out + "\n")
}

// ---------------------------------------------------------------------------
// Writer
// ---------------------------------------------------------------------------

// Action records what happened to one output path.
type Action struct {
	Path   string
	Status Status
}

// Status is the outcome of writing a single file.
type Status string

const (
	// Created means the file did not exist and was written.
	Created Status = "created"
	// Overwritten means an existing file was replaced.
	Overwritten Status = "overwritten"
	// Skipped means the file already existed and was left alone.
	Skipped Status = "skipped"
	// Sidecar means the file already existed and differed, so what would have
	// been written was put beside it with a .new suffix instead.
	Sidecar Status = "sidecar"
	// Unchanged means the file existed with identical content.
	Unchanged Status = "unchanged"
	// Planned means a dry run would have written the file.
	Planned Status = "planned"
)

// Writer writes rendered files under a root directory.
type Writer struct {
	Root string
	// DryRun reports what would happen without touching the filesystem.
	DryRun bool
	// Force overwrites files that already exist.
	Force bool
	// Sidecars writes "<path>.new" beside a file that already exists and
	// differs, rather than only reporting the collision.
	//
	// It is off for `new`, where a collision means the directory was not empty
	// and the answer is to pick another one. It is on for a recipe retrofitting
	// something into a project that has been worked on for months, where the
	// useful thing is to be able to diff what you have against what this would
	// have written.
	Sidecars bool

	actions []Action
}

// NewWriter builds a Writer rooted at dir.
func NewWriter(root string, dryRun, force bool) *Writer {
	return &Writer{Root: root, DryRun: dryRun, Force: force}
}

// Actions returns the record of everything the writer did, in order.
func (w *Writer) Actions() []Action { return w.actions }

// Count returns how many actions had the given status.
func (w *Writer) Count(s Status) int {
	n := 0
	for _, a := range w.actions {
		if a.Status == s {
			n++
		}
	}
	return n
}

// Conflicts lists paths that were skipped because they already existed.
func (w *Writer) Conflicts() []string {
	var out []string
	for _, a := range w.actions {
		if a.Status == Skipped {
			out = append(out, a.Path)
		}
	}
	return out
}

// WriteBytes writes content to a path relative to the writer root.
func (w *Writer) WriteBytes(rel string, content []byte, mode fs.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	clean, err := SafePath(rel)
	if err != nil {
		return err
	}
	abs := filepath.Join(w.Root, filepath.FromSlash(clean))
	rel = clean

	existing, err := os.ReadFile(abs)
	switch {
	case err == nil && bytes.Equal(existing, content):
		w.actions = append(w.actions, Action{rel, Unchanged})
		return nil
	case err == nil && !w.Force:
		if w.Sidecars {
			return w.writeSidecar(rel, abs, content, mode)
		}
		w.actions = append(w.actions, Action{rel, Skipped})
		return nil
	}

	if w.DryRun {
		w.actions = append(w.actions, Action{rel, Planned})
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", rel, err)
	}
	if err := os.WriteFile(abs, content, mode); err != nil {
		return fmt.Errorf("writing %s: %w", rel, err)
	}
	status := Created
	if err == nil {
		status = Overwritten
	}
	w.actions = append(w.actions, Action{rel, status})
	return nil
}

// writeSidecar puts content beside an existing file rather than over it.
//
// The original is never touched. A stale .new from a previous run is replaced,
// because it is this tool's file and the point of it is to describe the current
// state rather than some earlier one.
func (w *Writer) writeSidecar(rel, abs string, content []byte, mode fs.FileMode) error {
	if w.DryRun {
		w.actions = append(w.actions, Action{rel, Sidecar})
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", rel, err)
	}
	if err := os.WriteFile(abs+".new", content, mode); err != nil {
		return fmt.Errorf("writing %s.new: %w", rel, err)
	}
	w.actions = append(w.actions, Action{rel, Sidecar})
	return nil
}

// Sidecars lists the paths written beside an existing file rather than over it.
func (w *Writer) SidecarPaths() []string {
	var out []string
	for _, a := range w.actions {
		if a.Status == Sidecar {
			out = append(out, a.Path)
		}
	}
	return out
}

// Render renders a template and writes it in one step.
func (w *Writer) Render(e *Engine, tplName, rel string, data any) error {
	content, err := e.Render(tplName, data)
	if err != nil {
		return err
	}
	if len(content) == 0 {
		return nil
	}
	return w.WriteBytes(rel, content, ExecutableMode(rel))
}

// ExecutableMode is the file mode for a generated path, guessed from its name.
// A template that knows better says so explicitly instead.
func ExecutableMode(rel string) fs.FileMode {
	if strings.HasSuffix(rel, "gradlew") || strings.HasSuffix(rel, ".sh") {
		return 0o755
	}
	return 0o644
}

// SafePath cleans an output path and refuses anything that would escape the
// project root. This is the single choke point every generated file goes
// through, which is what makes a template's paths safe to template.
func SafePath(rel string) (string, error) {
	clean := path.Clean(filepath.ToSlash(rel))
	switch {
	case clean == "" || clean == ".":
		return "", fmt.Errorf("empty output path")
	case path.IsAbs(clean) || filepath.IsAbs(rel):
		return "", fmt.Errorf("output path %q is absolute; paths are relative to the project", rel)
	case clean == ".." || strings.HasPrefix(clean, "../"):
		return "", fmt.Errorf("output path %q escapes the project directory", rel)
	}
	return clean, nil
}

// ---------------------------------------------------------------------------
// Template functions
// ---------------------------------------------------------------------------

// FuncMap is the function set available inside every template. It is
// deliberately small: anything more elaborate belongs in Go, where it can be
// tested, rather than in a template.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"pascal":      model.Pascal,
		"camel":       model.Camel,
		"kebab":       model.Kebab,
		"lowerAlnum":  model.LowerAlnum,
		"packagePath": func(pkg string) string { return strings.ReplaceAll(pkg, ".", "/") },

		// first is for "the start destination is the first root tab".
		"first": func(list []string) string {
			if len(list) == 0 {
				return ""
			}
			return list[0]
		},

		"upper":   strings.ToUpper,
		"lower":   strings.ToLower,
		"trim":    strings.TrimSpace,
		"replace": func(old, new, s string) string { return strings.ReplaceAll(s, old, new) },
		"join":    func(sep string, list any) string { return strings.Join(toStrings(list), sep) },
		"split":   func(sep, s string) []string { return strings.Split(s, sep) },
		"quote":   func(v any) string { return fmt.Sprintf("%q", fmt.Sprint(v)) },

		// has answers "did they pick this?", which is most of what a
		// declarative condition needs to ask.
		"has": func(list any, want string) bool {
			for _, v := range toStrings(list) {
				if v == want {
					return true
				}
			}
			return false
		},
		"default": func(fallback, v any) any {
			if v == nil || v == "" || v == false {
				return fallback
			}
			return v
		},

		"indent":  func(n int, s string) string { return indent(n, s) },
		"nindent": func(n int, s string) string { return "\n" + indent(n, s) },
	}
}

// toStrings accepts both []string and the []any a JSON round trip leaves
// behind, so a condition works the same on a fresh answer and a remembered one.
func toStrings(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, fmt.Sprint(item))
		}
		return out
	case string:
		return []string{t}
	case nil:
		return nil
	default:
		return []string{fmt.Sprint(t)}
	}
}

func indent(n int, s string) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = pad + l
		}
	}
	return strings.Join(lines, "\n")
}
