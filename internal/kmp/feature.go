package kmp

import (
	"context"
	"fmt"

	"github.com/aaroncutress/kmp-scaffold/internal/generator"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
)

// Feature question ids, matching the JSON names in FeatureVars.
const (
	QFeatureTargets      = "targets"
	QFeaturePresentation = "presentation"
)

// Recipes is what `kmp-scaffold add` can do to a project this template made.
func (Template) Recipes() []scaffold.Recipe {
	return []scaffold.Recipe{{
		Name:        "feature",
		Noun:        "feature",
		Label:       "Feature module",
		Description: "api and impl modules, shared logic and an iOS target, wired into every place that needs to know.",
		NameHint:    "Lowercase kebab-case, e.g. firmware-update. Becomes feature/<name>/{api,impl}.",
		Questions:   featureQuestions,
		Summary:     featureSummary,
		Apply:       addFeature,
	}}
}

// featureQuestions asks what the new module should cover and how its screen
// appears. Which options are offered depends on how the project was generated -
// there is no point offering an iOS target to a project with no iOS app.
func featureQuestions(m *model.Manifest) []scaffold.Question {
	spec, _, err := SpecFrom(m)
	if err != nil {
		// The caller has already reported this; asking nothing is better than
		// asking questions built from a half-read manifest.
		return nil
	}

	return []scaffold.Question{
		{
			ID:     QFeatureTargets,
			Kind:   scaffold.KindMultiSelect,
			Prompt: "What should be generated?",
			Hint:   "Most features want both: UI on Android, logic and state in sharedLogic.",
			OptionsFor: func(*scaffold.Answers) []scaffold.Option {
				android := scaffold.Option{
					ID:    "android",
					Label: "Android feature module",
					Desc:  "api (routes) and impl (screens), wired into the entry provider and serializers.",
				}
				if !spec.Android {
					android.Disabled = true
					android.DisabledNote = "this project has no Android app"
				}

				ios := scaffold.Option{
					ID:    "ios",
					Label: "iOS feature target",
					Desc: "A Swift target with its route in CoreNavigation, wired into the Features " +
						"package and the coordinator.",
				}
				switch {
				case !spec.IOS:
					ios.Disabled = true
					ios.DisabledNote = "this project has no iOS app"
				case !generator.SupportsFeatures(generator.KindIOS, spec.IOSLayout):
					ios.Disabled = true
					ios.DisabledNote = "the " + spec.IOSLayout +
						" layout has a single entry point, so it has no feature targets"
				}

				return []scaffold.Option{
					android,
					ios,
					{ID: "shared", Label: "Shared logic",
						Desc: "Repository, ViewModel and Koin module in sharedLogic, used by both platforms."},
				}
			},
			DefaultFor: func(a *scaffold.Answers) any { return a.Strs(QFeatureTargets) },
		},

		{
			ID:     QFeaturePresentation,
			Kind:   scaffold.KindSelect,
			Prompt: "How does this screen appear?",
			Hint:   "Sets the Android route's Presentation, and where the iOS destination is registered.",
			SkipFor: func(a *scaffold.Answers) bool {
				targets := a.Strs(QFeatureTargets)
				return !model.Has(targets, "android") && !model.Has(targets, "ios")
			},
			OptionsFor: func(*scaffold.Answers) []scaffold.Option {
				opts := []scaffold.Option{
					{ID: "above-nav", Label: "Full screen",
						Desc: "Pushed on top of the shell, covering the navigation bar. The usual choice."},
					{ID: "overlay", Label: "Bottom sheet",
						Desc: "A modal sheet over the current screen."},
					{ID: "dialog", Label: "Dialog",
						Desc: "A modal dialog over the current screen."},
				}
				rootTab := scaffold.Option{ID: "shell", Label: "New root tab",
					Desc: "Adds a tab to the navigation shell, with its own back stack."}
				if spec.AndroidLayout != "nav3-shell" {
					rootTab.Disabled = true
					rootTab.DisabledNote = "this project uses the single-stack layout, which has no tabs"
				}
				return append(opts, rootTab)
			},
			DefaultFor: func(a *scaffold.Answers) any {
				if p := a.Str(QFeaturePresentation); p != "" {
					return p
				}
				return "above-nav"
			},
		},
	}
}

// featureRequest reads the answers into what the generators need.
func featureRequest(name string, a *scaffold.Answers) generator.FeatureRequest {
	targets := a.Strs(QFeatureTargets)
	presentation := a.Str(QFeaturePresentation)
	if presentation == "" {
		presentation = "above-nav"
	}
	return generator.FeatureRequest{
		Name:         model.Kebab(name),
		Android:      model.Has(targets, "android"),
		IOS:          model.Has(targets, "ios"),
		Shared:       model.Has(targets, "shared"),
		Presentation: presentation,
		RootTab:      presentation == "shell",
	}
}

// featureRecords encodes what the generators created into manifest entries.
func featureRecords(recipe string, reqs []generator.FeatureRequest) ([]model.Feature, error) {
	if recipe == "" {
		recipe = "feature"
	}
	var out []model.Feature
	for _, r := range reqs {
		rec, err := model.NewFeature(recipe, r.Name, FeatureVars{
			Android:      r.Android,
			Shared:       r.Shared,
			IOS:          r.IOS,
			Presentation: r.Presentation,
			RootTab:      r.RootTab,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

// addFeature generates the feature and wires it into the project.
func addFeature(ctx context.Context, req scaffold.RecipeRequest) (*scaffold.Report, error) {
	spec, _, err := SpecFrom(req.Manifest)
	if err != nil {
		return nil, err
	}

	fr := featureRequest(req.Name, req.Answers)
	if !fr.Android && !fr.IOS && !fr.Shared {
		return nil, fmt.Errorf("nothing to generate - pick at least one of android, ios or shared")
	}

	report, err := generator.AddFeature(spec, req.Root, fr, req.Version, req.Writer, req.DryRun)
	if err != nil {
		return nil, err
	}

	records, err := featureRecords(req.Recipe, report.Features)
	if err != nil {
		return nil, err
	}
	for _, rec := range records {
		req.Manifest.PutFeature(rec)
	}

	// A root tab joins the shell, so it has to be remembered as one: the tab
	// list is what the next `add feature` builds its navigation from.
	if fr.RootTab && !model.Has(spec.RootTabs, model.Pascal(fr.Name)) {
		spec.RootTabs = append(spec.RootTabs, model.Pascal(fr.Name))
	}
	if err := req.Manifest.SetVars(VarsOf(spec)); err != nil {
		return nil, err
	}

	if !req.DryRun {
		if err := req.Manifest.Save(req.Root); err != nil {
			return nil, fmt.Errorf("updating the project manifest: %w", err)
		}
	}

	return &scaffold.Report{
		Writer:   req.Writer,
		Wire:     report.Wire,
		Warnings: report.Warnings,
		Manifest: *req.Manifest,
	}, nil
}

// featureSummary lists exactly which files will be written and which will be
// edited, before anything happens.
func featureSummary(m *model.Manifest, a *scaffold.Answers) []scaffold.Section {
	fr := featureRequest(a.Str(scaffold.NameAnswer), a)
	pascal := model.Pascal(fr.Name)
	pkg := model.PackageSegment(fr.Name)
	pkgPath := m.Project.PackagePath()

	var created, edited []string

	if fr.Android {
		created = append(created,
			fmt.Sprintf("feature/%s/api/build.gradle.kts", fr.Name),
			fmt.Sprintf("feature/%s/api/.../%sRoute.kt", fr.Name, pascal),
			fmt.Sprintf("feature/%s/impl/build.gradle.kts", fr.Name),
			fmt.Sprintf("feature/%s/impl/.../%sNavigation.kt", fr.Name, pascal),
			fmt.Sprintf("feature/%s/impl/.../%sScreen.kt", fr.Name, pascal),
		)
		edited = append(edited,
			"settings.gradle.kts",
			"androidApp/build.gradle.kts",
			fmt.Sprintf("androidApp/src/main/kotlin/%s/AppSerializers.kt", pkgPath),
			fmt.Sprintf("androidApp/src/main/kotlin/%s/App.kt", pkgPath),
			"buildSrc/.../"+m.Project.Namespace()+".android.feature.gradle.kts",
		)
		if fr.RootTab {
			edited = append(edited,
				"core/ui/build.gradle.kts",
				fmt.Sprintf("core/ui/src/main/kotlin/%s/core/ui/components/Navigation.kt", pkgPath))
		}
	}
	if fr.IOS {
		created = append(created,
			fmt.Sprintf("iosApp/Packages/Features/Sources/CoreNavigation/%sRoute.swift", pascal),
			fmt.Sprintf("iosApp/Packages/Features/Sources/%s/%sScreen.swift", pascal, pascal),
			fmt.Sprintf("iosApp/Packages/Features/Sources/%s/%sDestination.swift", pascal, pascal),
		)
		edited = append(edited,
			"iosApp/Packages/Features/Package.swift",
			"iosApp/iosApp/App/AppCoordinator.swift")
	}
	if fr.Shared {
		created = append(created,
			fmt.Sprintf("sharedLogic/.../feature/%s/domain/%sRepository.kt", pkg, pascal),
			fmt.Sprintf("sharedLogic/.../feature/%s/data/%sRepositoryImpl.kt", pkg, pascal),
			fmt.Sprintf("sharedLogic/.../feature/%s/presentation/%sViewModel.kt", pkg, pascal),
			fmt.Sprintf("sharedLogic/.../feature/%s/di/%sModule.kt", pkg, pascal),
		)
		edited = append(edited,
			fmt.Sprintf("sharedLogic/src/commonMain/kotlin/%s/di/SharedModules.kt", pkgPath))
	}

	return []scaffold.Section{
		{Rows: []scaffold.Row{
			{Label: "Feature", Value: fr.Name},
			{Label: "Presentation", Value: presentationLabel(fr)},
		}},
		{Title: "New files", Items: created},
		{Title: "Wired into", Items: edited},
	}
}

func presentationLabel(fr generator.FeatureRequest) string {
	switch {
	case !fr.Android && !fr.IOS:
		return "shared logic only"
	case fr.RootTab:
		return "root tab in the navigation shell"
	case fr.Presentation == "overlay":
		return "bottom sheet"
	case fr.Presentation == "dialog":
		return "dialog"
	default:
		return "full screen, pushed above the shell"
	}
}

// DefaultTargets is what a new feature covers unless the user says otherwise:
// every side of the project that can hold one.
func DefaultTargets(m *model.Manifest) []string {
	spec, _, err := SpecFrom(m)
	if err != nil {
		return []string{"shared"}
	}
	var out []string
	if spec.Android {
		out = append(out, "android")
	}
	if spec.IOS && generator.SupportsFeatures(generator.KindIOS, spec.IOSLayout) {
		out = append(out, "ios")
	}
	return append(out, "shared")
}

// ValidateTargets checks a --targets list.
func ValidateTargets(targets []string) error {
	for _, t := range targets {
		switch t {
		case "android", "ios", "shared":
		default:
			return fmt.Errorf("unknown target %q - use android, ios or shared", t)
		}
	}
	return nil
}
