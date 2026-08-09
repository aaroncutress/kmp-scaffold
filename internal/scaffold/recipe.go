package scaffold

import (
	"context"

	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/render"
)

// NameAnswer is the id the thing being added is named under.
//
// It is a reserved answer rather than a field, so a recipe's templates can read
// it the same way they read every other answer, and so Answers.Project keeps
// meaning the *project* - which is what a recipe's files need when they build a
// package path.
const NameAnswer = "name"

// Recipe is a named thing `kmp-scaffold add` can apply to a project.
//
// What a project can be extended with is the business of the template that
// generated it: a Kotlin Multiplatform app grows features, a service grows
// routes, and something else again grows whatever it calls them. The tool only
// knows that each one has a name, some questions, and an apply step.
type Recipe struct {
	// Name is what `kmp-scaffold add <name>` matches.
	Name string
	// Noun is what this recipe calls the thing being added, so the prompts read
	// naturally. Defaults to Name.
	Noun string
	// Label and Description describe it in help and in `kmp-scaffold templates`.
	Label       string
	Description string
	// NameHint is shown under the universal name question.
	NameHint string

	// Questions are asked after the name.
	//
	// The project's manifest is passed so a recipe can offer only what this
	// project can actually hold - there is no point asking about an iOS target
	// in a project with no iOS app.
	Questions func(*model.Manifest) []Question

	// Summary drives the review screen.
	Summary func(*model.Manifest, *Answers) []Section

	// Apply writes the new files and wires them into the ones that already
	// exist. It is responsible for recording what it did in the manifest and
	// saving it.
	Apply func(context.Context, RecipeRequest) (*Report, error)
}

// NounOr returns the recipe's noun, falling back to its name.
func (r Recipe) NounOr() string {
	if r.Noun != "" {
		return r.Noun
	}
	return r.Name
}

// RecipeRequest is everything Apply needs.
type RecipeRequest struct {
	// Recipe is the name being applied, for the record written to the manifest.
	Recipe string
	// Manifest is the project's, already loaded. Apply updates and saves it.
	Manifest *model.Manifest
	Root     string
	// Name is what is being added, validated as kebab-case by the tool.
	Name    string
	Answers *Answers
	Writer  *render.Writer
	Version string
	DryRun  bool
}

// FindRecipe returns the named recipe, if the template has one.
func FindRecipe(t Template, name string) (Recipe, bool) {
	for _, r := range t.Recipes() {
		if r.Name == name {
			return r, true
		}
	}
	return Recipe{}, false
}

// RecipeNames lists what a template can add.
func RecipeNames(t Template) []string {
	recipes := t.Recipes()
	out := make([]string, 0, len(recipes))
	for _, r := range recipes {
		out = append(out, r.Name)
	}
	return out
}
