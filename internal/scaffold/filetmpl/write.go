package filetmpl

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/render"
	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
)

// Ctx is what a file-based template's Go templates see.
type Ctx struct {
	// Project is the identity the tool always asks for.
	Project model.Project
	// Vars is one entry per question, keyed by its id.
	Vars map[string]any
	// Versions maps a resolved version key to the version chosen.
	Versions Versions
	// Gradle is the resolved Gradle distribution, when one was asked for.
	Gradle resolve.GradleRelease
	// Gen is the kmp-scaffold version that generated this.
	Gen string

	// ProjectVars is what the project was generated with, set while a recipe
	// runs. It is how a recipe asks whether the project has the thing it needs:
	// `{{ if has .ProjectVars.extras "auth" }}`.
	ProjectVars map[string]any

	// Item, Index, First and Last are bound inside a for_each.
	Item  any
	Index int
	First bool
	Last  bool

	// RelPath is the path under a glob's base, with any .tmpl suffix removed.
	RelPath string

	// Feature is the thing being added, set while a recipe runs.
	Feature *FeatureCtx
}

// Versions is the resolved version set.
//
// It is a named map so that keys can be read either way: `{{ .Versions.kotlin }}`
// for an ordinary key, and `{{ .Versions.Of "kotlinx-coroutines" }}` for one
// with a dash in it, which a Go template cannot address with a dot.
type Versions map[string]string

// Of returns the version for a key, or "" if it was not resolved.
func (v Versions) Of(key string) string { return v[key] }

// renderSafePath is render.SafePath, re-exported here so the recipe code has
// one place to reach for it.
func renderSafePath(p string) (string, error) { return render.SafePath(p) }

// FeatureCtx describes the thing a recipe is adding.
type FeatureCtx struct {
	Name   string
	Kebab  string
	Pascal string
	Camel  string
	Pkg    string
	Vars   map[string]any
}

// ctx builds the render context for the current answers.
func (t *Template) ctx(a *scaffold.Answers, res *resolve.Result) Ctx {
	c := Ctx{
		Project:  a.Project,
		Vars:     a.Values,
		Versions: Versions{},
	}
	if c.Vars == nil {
		c.Vars = map[string]any{}
	}
	if res != nil {
		c.Versions = res.Versions
		c.Gradle = res.Gradle
	}
	return c
}

// write renders every [[files]] entry.
func (t *Template) write(a *scaffold.Answers, res *resolve.Result, version string,
	w *render.Writer, feature *FeatureCtx) error {

	base := t.ctx(a, res)
	base.Gen = version
	base.Feature = feature

	for _, def := range t.manifest.File {
		if err := t.writeOne(def, base, w); err != nil {
			return fmt.Errorf("%s: %w", def.From, err)
		}
	}
	return nil
}

func (t *Template) writeOne(def FileDef, base Ctx, w *render.Writer) error {
	if def.When != "" {
		ok, err := t.engine.Truthy(def.When, base)
		if err != nil {
			return fmt.Errorf("`when`: %w", err)
		}
		if !ok {
			return nil
		}
	}

	sources, err := t.expand(def.From)
	if err != nil {
		return err
	}

	items, err := t.forEach(def, base)
	if err != nil {
		return err
	}

	for _, src := range sources {
		for i, item := range items {
			ctx := base
			ctx.RelPath = src.rel
			if def.ForEach != "" {
				ctx.Item = item
				ctx.Index = i
				ctx.First = i == 0
				ctx.Last = i == len(items)-1
			}
			if err := t.writeFile(def, src, ctx, w); err != nil {
				return err
			}
		}
	}
	return nil
}

// forEach returns the list a file is repeated over, or a single nil item when
// the file is written once.
func (t *Template) forEach(def FileDef, ctx Ctx) ([]any, error) {
	if def.ForEach == "" {
		return []any{nil}, nil
	}
	// The list is rendered to text and split on newlines, because a Go template
	// has no way to hand back a value. `{{ range .Vars.routes }}{{ . }}
	// {{ end }}` is the idiom, and range over a list of one line is the same.
	out, err := t.engine.RenderString("for_each", "{{ range "+listExpr(def.ForEach)+" }}{{ . }}\n{{ end }}", ctx)
	if err != nil {
		return nil, fmt.Errorf("`for_each`: %w", err)
	}
	var items []any
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			items = append(items, line)
		}
	}
	return items, nil
}

// listExpr accepts both a bare path (`.Vars.routes`) and a full action
// (`{{ .Vars.routes }}`), because both read naturally in a manifest.
func listExpr(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "{{")
	s = strings.TrimSuffix(s, "}}")
	return strings.TrimSpace(s)
}

// outputName turns a path inside the template into the path it is written to.
//
// Two conventions, both there to keep a template's own files ordinary files:
// the .tmpl suffix is dropped, and a `dot-` prefix on any segment becomes a
// leading dot. Without the second, a template could not carry a .gitignore -
// git would honour it inside the template itself, and //go:embed skips
// dotfiles.
func outputName(rel string) string {
	segments := strings.Split(strings.TrimSuffix(rel, ".tmpl"), "/")
	for i, s := range segments {
		if after, ok := strings.CutPrefix(s, "dot-"); ok {
			segments[i] = "." + after
		}
	}
	return strings.Join(segments, "/")
}

// source is one file inside the template.
type source struct {
	// path is relative to the template directory.
	path string
	// rel is the path under a glob's base, with .tmpl removed. It is empty for
	// a plain (non-glob) entry.
	rel string
}

// expand resolves a `from` into the files it names. A trailing /** takes the
// whole subtree; anything else is a single file.
func (t *Template) expand(from string) ([]source, error) {
	clean := path.Clean(from)
	if !strings.HasSuffix(clean, "/**") && !strings.HasSuffix(clean, "/*") {
		return []source{{path: clean}}, nil
	}

	base := strings.TrimSuffix(strings.TrimSuffix(clean, "/**"), "/*")
	recursive := strings.HasSuffix(clean, "/**")

	var out []source
	root := filepath.Join(t.dir, filepath.FromSlash(base))
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if !recursive && p != root {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		out = append(out, source{
			path: path.Join(base, rel),
			rel:  outputName(rel),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("matched no files")
	}
	return out, nil
}

func (t *Template) writeFile(def FileDef, src source, ctx Ctx, w *render.Writer) error {
	data, err := os.ReadFile(filepath.Join(t.dir, filepath.FromSlash(src.path)))
	if err != nil {
		return err
	}

	out, err := t.engine.RenderString("path:"+def.To, def.To, ctx)
	if err != nil {
		return fmt.Errorf("`to`: %w", err)
	}
	dest := strings.TrimSpace(string(out))
	if dest == "" {
		return fmt.Errorf("`to` rendered to nothing")
	}
	// A directory destination keeps the source's own name under it, which is
	// what makes `to = "src/{{ .Project.PackagePath }}/"` do the obvious thing.
	if strings.HasSuffix(dest, "/") {
		name := src.rel
		if name == "" {
			name = outputName(path.Base(src.path))
		}
		dest = path.Join(dest, name)
	}

	content := data
	if !def.Copy {
		content, err = t.engine.RenderString(src.path, string(data), ctx)
		if err != nil {
			return err
		}
	}
	if def.Normalise == nil || *def.Normalise {
		if !def.Copy {
			content = render.Normalise(content)
		}
	}
	if len(content) == 0 {
		return nil
	}

	mode, err := parseMode(def.Mode)
	if err != nil {
		return err
	}
	if mode == 0 {
		mode = render.ExecutableMode(dest)
		// A source file that is executable stays executable, so a template can
		// ship a script without spelling out its mode.
		if info, err := os.Stat(filepath.Join(t.dir, filepath.FromSlash(src.path))); err == nil {
			if info.Mode()&0o111 != 0 {
				mode = 0o755
			}
		}
	}
	return w.WriteBytes(dest, content, mode)
}
