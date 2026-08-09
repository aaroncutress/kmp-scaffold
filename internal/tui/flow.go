package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
	"github.com/aaroncutress/kmp-scaffold/internal/vcs"
)

// NewFlow builds the wizard for `kmp-scaffold new`.
//
// The steps are the same for every template: the questions the tool always asks
// (which template, what is it called, where does it go, what package), then the
// template's own, then resolution and review. Nothing here knows what kind of
// project is being generated.
//
// The returned Holder carries the resolution result, which the caller needs in
// order to generate.
func NewFlow(ctx context.Context, t scaffold.Template, answers *scaffold.Answers) (*Wizard, *Holder) {
	holder := &Holder{}

	steps := universalSteps(t.Meta())
	steps = append(steps, StepsFor(t.Questions())...)
	steps = append(steps, NewResolveStep(ctx, t, holder))
	steps = append(steps, &SummaryStep{
		Heading: "Ready to generate",
		Action:  "create",
		Sections: func(a *scaffold.Answers) []scaffold.Section {
			return t.Summary(a, holder.Result)
		},
	})

	return NewWizard("kmp-scaffold · new project", answers, steps), holder
}

// TemplateQuestion is the id the chosen template is recorded under.
const TemplateQuestion = "template"

// PickTemplate asks which template to generate from.
//
// It is a wizard of its own rather than the first step of the main one, because
// the answer decides what the rest of the questions are: the flow cannot be
// built until it is known.
func PickTemplate(choices []scaffold.Template) *Wizard {
	return NewWizard("kmp-scaffold · new project",
		scaffold.NewAnswers(), []Step{templateStep(choices)})
}

func templateStep(choices []scaffold.Template) Step {
	return &SelectStep{
		Prompt: "Which template?",
		Hint:   "What kind of project to generate.",
		Options: func(*scaffold.Answers) []Option {
			var out []Option
			for _, c := range choices {
				m := c.Meta()
				label := m.Label
				if m.Source.Kind != scaffold.SourceBuiltin {
					label += "  (" + string(m.Source.Kind) + ")"
				}
				out = append(out, Option{ID: m.ID, Label: label, Desc: m.Description})
			}
			return out
		},
		Current: func(a *scaffold.Answers) string {
			if id := a.Str(TemplateQuestion); id != "" {
				return id
			}
			return scaffold.DefaultTemplate
		},
		Apply: func(a *scaffold.Answers, id string) { a.Set(TemplateQuestion, id) },
	}
}

// universalSteps are the questions the tool asks whatever the template.
func universalSteps(meta scaffold.Meta) []Step {
	steps := []Step{
		&TextStep{
			Prompt: "What is your project called?",
			Hint:   "Used for the project name, the app label and the generated type names.",
			Default: func(a *scaffold.Answers) string {
				if a.Project.Name != "" {
					return a.Project.Name
				}
				return "My App"
			},
			Validate: func(v string, _ *scaffold.Answers) error { return model.ValidateProjectName(v) },
			Apply:    func(a *scaffold.Answers, v string) { a.Project.Name = v },
		},

		&TextStep{
			Prompt: "Where should it go?",
			Hint:   "A directory, relative to where you are now. It will be created if it does not exist.",
			Default: func(a *scaffold.Answers) string {
				if a.Project.Dir != "" {
					return a.Project.Dir
				}
				return model.Kebab(a.Project.Name)
			},
			Validate: func(v string, _ *scaffold.Answers) error {
				if strings.TrimSpace(v) == "" {
					return errors.New("give a directory, or . for the current one")
				}
				return nil
			},
			Apply: func(a *scaffold.Answers, v string) { a.Project.Dir = filepath.Clean(v) },
		},
	}

	// Where the project will be written decides whether offering a repository
	// makes any sense, and that is only known once the directory is answered.
	steps = append(steps, &ConfirmStep{
		Prompt:  "Set up a git repository?",
		Hint:    "git init, then an initial commit - so you can see what you change next.",
		Default: true,
		SkipIf: func(a *scaffold.Answers) bool {
			return !vcs.Available() || vcs.InsideRepo(projectDir(a))
		},
		Apply: func(a *scaffold.Answers, v bool) { a.InitGit = v },
	})

	// Asked here rather than by each template, because wanting tests and wanting
	// CI are properties of the project rather than of what kind of project it
	// is. A template that has no files to write for either says so in its meta
	// and is not asked.
	if meta.SupportsTests {
		steps = append(steps, &ConfirmStep{
			Prompt:  "Generate tests?",
			Hint:    "Test source sets, the dependencies they need, and one worked example per platform.",
			Default: true,
			Apply:   func(a *scaffold.Answers, v bool) { a.Tests = v },
		})
	}
	if meta.SupportsCI {
		steps = append(steps, &ConfirmStep{
			Prompt:  "Generate a CI workflow?",
			Hint:    "GitHub Actions: build on every pull request, and on the default branch.",
			Default: true,
			Apply:   func(a *scaffold.Answers, v bool) { a.CI = v },
		})
	}

	if meta.AsksPackage {
		steps = append(steps, &TextStep{
			Prompt: "Package name?",
			Hint:   "Reverse-DNS, e.g. com.example.myapp. Also used as the Android applicationId.",
			Default: func(a *scaffold.Answers) string {
				if a.Project.Package != "" {
					return a.Project.Package
				}
				return "com.example." + model.LowerAlnum(a.Project.Name)
			},
			Validate: func(v string, _ *scaffold.Answers) error { return model.ValidatePackage(v) },
			Apply: func(a *scaffold.Answers, v string) {
				a.Project.Package = v
				a.Project.ApplicationID = v
			},
		})
	}

	return steps
}

// RecipeFlow builds the wizard for `kmp-scaffold add <recipe>`.
//
// The name question is the tool's, because the name is what stops the same
// thing being added twice; everything after it belongs to the recipe.
func RecipeFlow(r scaffold.Recipe, m *model.Manifest, answers *scaffold.Answers) *Wizard {
	noun := r.NounOr()

	hint := r.NameHint
	if hint == "" {
		hint = "Lowercase kebab-case, e.g. firmware-update."
	}

	steps := []Step{
		&TextStep{
			Prompt:  fmt.Sprintf("What is the %s called?", noun),
			Hint:    hint,
			Default: func(a *scaffold.Answers) string { return a.Str(scaffold.NameAnswer) },
			Validate: func(v string, _ *scaffold.Answers) error {
				if err := model.ValidateFeatureName(v); err != nil {
					return err
				}
				if m.FindFeatureOf(r.Name, model.Kebab(v)) != nil {
					return fmt.Errorf("this project already has a %s called %q", noun, v)
				}
				return nil
			},
			Apply: func(a *scaffold.Answers, v string) { a.Set(scaffold.NameAnswer, model.Kebab(v)) },
		},
	}

	if r.Questions != nil {
		steps = append(steps, StepsFor(r.Questions(m))...)
	}
	steps = append(steps, &SummaryStep{
		Heading: fmt.Sprintf("Ready to add the %s", noun),
		Action:  "create",
		Sections: func(a *scaffold.Answers) []scaffold.Section {
			if r.Summary == nil {
				return nil
			}
			return r.Summary(m, a)
		},
	})

	return NewWizard("kmp-scaffold · add "+noun, answers, steps)
}

// projectDir is where the project will be written, as an absolute path.
//
// The wizard has to answer "would a repository here be nested inside another
// one?" before anything is generated, so this works from the answers rather
// than from what is on disk.
func projectDir(a *scaffold.Answers) string {
	dir := a.Project.Dir
	if dir == "" {
		dir = model.Kebab(a.Project.Name)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return abs
}
