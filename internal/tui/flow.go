package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
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

// FeatureFlow builds the wizard for `kmp-scaffold add feature`.
//
// The name question is the tool's, because the name is what stops the same
// thing being added twice; everything after it belongs to the template.
func FeatureFlow(t scaffold.FeatureTemplate, m *model.Manifest, answers *scaffold.Answers) *Wizard {
	noun := t.FeatureNoun()

	steps := []Step{
		&TextStep{
			Prompt: fmt.Sprintf("What is the %s called?", noun),
			Hint:   "Lowercase kebab-case, e.g. firmware-update.",
			Default: func(a *scaffold.Answers) string {
				return a.Project.Name
			},
			Validate: func(v string, _ *scaffold.Answers) error {
				if err := model.ValidateFeatureName(v); err != nil {
					return err
				}
				if m.FindFeature(model.Kebab(v)) != nil {
					return fmt.Errorf("%q already exists in this project", v)
				}
				return nil
			},
			Apply: func(a *scaffold.Answers, v string) { a.Project.Name = model.Kebab(v) },
		},
	}

	steps = append(steps, StepsFor(t.FeatureQuestions(m))...)
	steps = append(steps, &SummaryStep{
		Heading:  fmt.Sprintf("Ready to add the %s", noun),
		Action:   "create",
		Sections: func(a *scaffold.Answers) []scaffold.Section { return t.FeatureSummary(m, a) },
	})

	return NewWizard("kmp-scaffold · add "+noun, answers, steps)
}
