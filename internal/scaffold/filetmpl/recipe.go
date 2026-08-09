package filetmpl

import (
	"context"
	"fmt"
	"strings"

	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
	"github.com/aaroncutress/kmp-scaffold/internal/wire"
)

// Recipes turns each [recipes.<name>] block into something `add` can apply.
func (t *Template) Recipes() []scaffold.Recipe {
	names := t.manifest.RecipeNames()
	out := make([]scaffold.Recipe, 0, len(names))
	for _, name := range names {
		out = append(out, t.recipe(name, t.manifest.Recipes[name]))
	}
	return out
}

func (t *Template) recipe(name string, def RecipeDef) scaffold.Recipe {
	label := def.Label
	if label == "" {
		label = name
	}
	return scaffold.Recipe{
		Name:        name,
		Noun:        def.Noun,
		Label:       label,
		Description: def.Description,
		NameHint:    def.NameHint,

		Questions: func(m *model.Manifest) []scaffold.Question {
			// The conditions are evaluated against the project as it was
			// generated, so `when` can ask whether it has what this recipe
			// needs.
			ctxFor := func(a *scaffold.Answers) Ctx {
				ctx, err := t.recipeCtx(m, a, a.Str(scaffold.NameAnswer), "")
				if err != nil {
					return t.ctx(a, nil)
				}
				return ctx
			}
			out := make([]scaffold.Question, 0, len(def.Question))
			for _, q := range def.Question {
				out = append(out, t.questionWith(q, ctxFor))
			}
			return out
		},

		Summary: func(m *model.Manifest, a *scaffold.Answers) []scaffold.Section {
			return t.recipeSummary(name, def, m, a)
		},

		Apply: func(_ context.Context, req scaffold.RecipeRequest) (*scaffold.Report, error) {
			return t.applyRecipe(name, def, req)
		},
	}
}

// applyRecipe writes the recipe's files and wires them into the ones that are
// already there.
func (t *Template) applyRecipe(name string, def RecipeDef, req scaffold.RecipeRequest) (*scaffold.Report, error) {
	ctx, err := t.recipeCtx(req.Manifest, req.Answers, req.Name, req.Version)
	if err != nil {
		return nil, err
	}

	for _, f := range def.File {
		if err := t.writeOne(f, ctx, req.Writer); err != nil {
			return nil, fmt.Errorf("%s: %w", f.From, err)
		}
	}

	report := &scaffold.Report{Writer: req.Writer}

	applier := wire.NewApplier(req.Root, req.DryRun)
	for _, e := range def.Edit {
		edit, skip, err := t.edit(e, ctx)
		if err != nil {
			return nil, err
		}
		if skip {
			continue
		}
		if err := applier.Apply(edit); err != nil {
			return nil, err
		}
	}
	report.Wire = applier.Results()

	for _, missing := range applier.MissingAnchors() {
		report.Warnings = append(report.Warnings, fmt.Sprintf(
			"could not find the anchor comment in %s - wire the new %s in by hand",
			missing, scaffold.Recipe{Name: name, Noun: def.Noun}.NounOr()))
	}

	// What was added is remembered by name and recipe, which is what stops it
	// being added twice.
	record, err := model.NewFeature(name, req.Name, req.Answers.Values)
	if err != nil {
		return nil, err
	}
	req.Manifest.PutFeature(record)

	if !req.DryRun {
		if err := req.Manifest.Save(req.Root); err != nil {
			return nil, fmt.Errorf("updating the project manifest: %w", err)
		}
	}
	report.Manifest = *req.Manifest
	return report, nil
}

// edit renders every field of a declared edit. It reports skip=true when the
// edit's `when` says this project does not want it.
func (t *Template) edit(def EditDef, ctx Ctx) (wire.Edit, bool, error) {
	if def.When != "" {
		ok, err := t.engine.Truthy(def.When, ctx)
		if err != nil {
			return wire.Edit{}, false, fmt.Errorf("edit on %q: `when`: %w", def.Path, err)
		}
		if !ok {
			return wire.Edit{}, true, nil
		}
	}

	render1 := func(what, text string) (string, error) {
		out, err := t.engine.RenderString("edit", text, ctx)
		if err != nil {
			return "", fmt.Errorf("edit on %q: `%s`: %w", def.Path, what, err)
		}
		return strings.TrimSpace(string(out)), nil
	}
	renderAll := func(what string, texts []string) ([]string, error) {
		out := make([]string, 0, len(texts))
		for _, text := range texts {
			// Lines keep their leading whitespace: a multi-line block carries
			// its own nesting, which wire re-indents to match the anchor.
			rendered, err := t.engine.RenderString("edit", text, ctx)
			if err != nil {
				return nil, fmt.Errorf("edit on %q: `%s`: %w", def.Path, what, err)
			}
			out = append(out, strings.TrimRight(string(rendered), "\n"))
		}
		return out, nil
	}

	path, err := render1("path", def.Path)
	if err != nil {
		return wire.Edit{}, false, err
	}
	if path == "" {
		return wire.Edit{}, false, fmt.Errorf("an edit's `path` rendered to nothing")
	}
	if path, err = safeEditPath(path); err != nil {
		return wire.Edit{}, false, err
	}

	anchor, err := render1("anchor", def.Anchor)
	if err != nil {
		return wire.Edit{}, false, err
	}
	key, err := render1("key", def.Key)
	if err != nil {
		return wire.Edit{}, false, err
	}
	lines, err := renderAll("lines", def.Lines)
	if err != nil {
		return wire.Edit{}, false, err
	}
	imports, err := renderAll("imports", def.Imports)
	if err != nil {
		return wire.Edit{}, false, err
	}

	return wire.Edit{Path: path, Anchor: anchor, Lines: lines, Imports: imports, Key: key}, false, nil
}

// recipeSummary is the review screen: what will be written and what will be
// edited, before anything happens.
func (t *Template) recipeSummary(name string, def RecipeDef,
	m *model.Manifest, a *scaffold.Answers) []scaffold.Section {

	noun := scaffold.Recipe{Name: name, Noun: def.Noun}.NounOr()

	ctx, err := t.recipeCtx(m, a, a.Str(scaffold.NameAnswer), "")
	if err != nil {
		return []scaffold.Section{{Note: err.Error()}}
	}

	rows := []scaffold.Row{{Label: strings.ToUpper(noun[:1]) + noun[1:], Value: ctx.Feature.Name}}
	for _, q := range def.Question {
		if !a.Has(q.ID) {
			continue
		}
		rows = append(rows, scaffold.Row{Label: q.ID, Value: answerText(q, a)})
	}
	sections := []scaffold.Section{{Rows: rows}}

	if len(def.Summary) > 0 {
		return append(sections, t.sectionsFrom(def.Summary, ctx)...)
	}

	var created []string
	for _, f := range def.File {
		if f.When != "" {
			if ok, err := t.engine.Truthy(f.When, ctx); err != nil || !ok {
				continue
			}
		}
		if out, err := t.engine.RenderString("path", f.To, ctx); err == nil {
			created = append(created, strings.TrimSpace(string(out)))
		}
	}

	var edited []string
	seen := map[string]bool{}
	for _, e := range def.Edit {
		edit, skip, err := t.edit(e, ctx)
		if err != nil || skip || seen[edit.Path] {
			continue
		}
		seen[edit.Path] = true
		edited = append(edited, edit.Path)
	}

	return append(sections,
		scaffold.Section{Title: "New files", Items: created},
		scaffold.Section{Title: "Wired into", Items: edited})
}

func answerText(q QuestionDef, a *scaffold.Answers) string {
	switch scaffold.Kind(q.Kind) {
	case scaffold.KindMultiSelect, scaffold.KindList:
		return strings.Join(labelled(q, a.Strs(q.ID)), ", ")
	case scaffold.KindConfirm:
		return yesNo(a.Bool(q.ID))
	default:
		return strings.Join(labelled(q, []string{a.Str(q.ID)}), ", ")
	}
}

// recipeCtx builds the render context for a recipe: the project as it was
// generated, plus the thing being added.
func (t *Template) recipeCtx(m *model.Manifest, a *scaffold.Answers, name, version string) (Ctx, error) {
	var projectVars map[string]any
	if err := m.DecodeVars(&projectVars); err != nil {
		return Ctx{}, err
	}

	ctx := t.ctx(a, nil)
	ctx.Project = m.Project
	ctx.ProjectVars = projectVars
	ctx.Gen = version
	ctx.Feature = &FeatureCtx{
		Name:   model.Kebab(name),
		Kebab:  model.Kebab(name),
		Pascal: model.Pascal(name),
		Camel:  model.Camel(name),
		Pkg:    model.PackageSegment(name),
		Vars:   a.Values,
	}
	return ctx, nil
}

// safeEditPath keeps a rendered edit path inside the project, the same rule
// every generated file goes through.
func safeEditPath(p string) (string, error) {
	clean, err := renderSafePath(p)
	if err != nil {
		return "", fmt.Errorf("edit path %q: %w", p, err)
	}
	return clean, nil
}
