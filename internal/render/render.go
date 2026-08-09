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
func NewEngine(fsys fs.FS) (*Engine, error) {
	tpl := template.New("kmp-scaffold").Funcs(FuncMap())
	tpl, err := tpl.ParseFS(fsys, "*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}
	return &Engine{tpl: tpl}, nil
}

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
	abs := filepath.Join(w.Root, filepath.FromSlash(rel))

	existing, err := os.ReadFile(abs)
	switch {
	case err == nil && bytes.Equal(existing, content):
		w.actions = append(w.actions, Action{rel, Unchanged})
		return nil
	case err == nil && !w.Force:
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

// Render renders a template and writes it in one step.
func (w *Writer) Render(e *Engine, tplName, rel string, data any) error {
	content, err := e.Render(tplName, data)
	if err != nil {
		return err
	}
	if len(content) == 0 {
		return nil
	}
	mode := fs.FileMode(0o644)
	if strings.HasSuffix(rel, "gradlew") || strings.HasSuffix(rel, ".sh") {
		mode = 0o755
	}
	return w.WriteBytes(rel, content, mode)
}

// ---------------------------------------------------------------------------
// Template functions
// ---------------------------------------------------------------------------

// FuncMap is the function set available inside every template. It is
// deliberately small: anything more elaborate belongs in Go, where it can be
// tested, rather than in a template.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"pascal":     model.Pascal,
		"camel":      model.Camel,
		"lowerAlnum": model.LowerAlnum,
		// first is for "the start destination is the first root tab".
		"first": func(list []string) string {
			if len(list) == 0 {
				return ""
			}
			return list[0]
		},
	}
}
