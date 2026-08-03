package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aaroncutress/kmp-scaffold/internal/model"
)

// FeatureDraft is what the add-feature wizard collects.
type FeatureDraft struct {
	Name         string
	Android      bool
	Shared       bool
	Presentation string
	RootTab      bool
}

// FeatureFlow builds the wizard for `kmp-scaffold add feature`.
//
// The steps take a model.Spec because that is the Step contract, but they write
// into the returned draft: the spec here is a read-only view of how the project
// was generated, used to decide which questions are worth asking.
func FeatureFlow(spec model.Spec, manifest model.Manifest, initial FeatureDraft) (*Wizard, *FeatureDraft) {
	draft := &initial
	if draft.Presentation == "" {
		draft.Presentation = "above-nav"
	}

	steps := []Step{
		&TextStep{
			Prompt:  "What is the feature called?",
			Hint:    "Lowercase kebab-case, e.g. firmware-update. Becomes feature/<name>/{api,impl}.",
			Default: func(model.Spec) string { return draft.Name },
			Validate: func(v string, _ model.Spec) error {
				if err := model.ValidateFeatureName(v); err != nil {
					return err
				}
				if existing := manifest.FindFeature(model.Kebab(v)); existing != nil {
					return fmt.Errorf("%q already exists in this project", v)
				}
				return nil
			},
			Apply: func(_ *model.Spec, v string) { draft.Name = model.Kebab(v) },
		},

		&CheckStep{
			Prompt: "What should be generated?",
			Hint:   "Most features want both: UI on Android, logic and state in sharedLogic.",
			Options: func(spec model.Spec) []Option {
				android := Option{
					ID:    "android",
					Label: "Android feature module",
					Desc:  "api (routes) and impl (screens), wired into the entry provider and serializers.",
				}
				if !spec.Android {
					android.Disabled = true
					android.DisabledNote = "this project has no Android app"
				}
				return []Option{
					android,
					{ID: "shared", Label: "Shared logic",
						Desc: "Repository, ViewModel and Koin module in sharedLogic, added to the DI graph."},
				}
			},
			Current: func(model.Spec) []string {
				var out []string
				if draft.Android {
					out = append(out, "android")
				}
				if draft.Shared {
					out = append(out, "shared")
				}
				return out
			},
			Apply: func(_ *model.Spec, ids []string) {
				draft.Android = model.Has(ids, "android")
				draft.Shared = model.Has(ids, "shared")
			},
		},

		&SelectStep{
			Prompt: "How does this screen appear?",
			Hint:   "This sets the route's Presentation, which decides which back stack it lands on.",
			SkipIf: func(model.Spec) bool { return !draft.Android },
			Options: func(spec model.Spec) []Option {
				opts := []Option{
					{ID: "above-nav", Label: "Full screen",
						Desc: "Pushed on top of the shell, covering the navigation bar. The usual choice."},
					{ID: "overlay", Label: "Bottom sheet",
						Desc: "A modal sheet over the current screen."},
					{ID: "dialog", Label: "Dialog",
						Desc: "A modal dialog over the current screen."},
				}
				rootTab := Option{ID: "shell", Label: "New root tab",
					Desc: "Adds a tab to the navigation shell, with its own back stack."}
				if spec.AndroidLayout != "nav3-shell" {
					rootTab.Disabled = true
					rootTab.DisabledNote = "this project uses the single-stack layout, which has no tabs"
				}
				return append(opts, rootTab)
			},
			Current: func(model.Spec) string { return draft.Presentation },
			Apply: func(_ *model.Spec, id string) {
				draft.Presentation = id
				draft.RootTab = id == "shell"
			},
		},

		&FeatureReviewStep{Draft: draft, Manifest: manifest},
	}

	return NewWizard("kmp-scaffold · add feature", spec, steps), draft
}

// FeatureReviewStep lists exactly which files will be written and which will be
// edited, before anything happens.
type FeatureReviewStep struct {
	Draft    *FeatureDraft
	Manifest model.Manifest
}

func (s *FeatureReviewStep) Title() string             { return "Ready to add the feature" }
func (s *FeatureReviewStep) Help() string              { return "enter create · esc back · ctrl+c quit" }
func (s *FeatureReviewStep) Skip(model.Spec) bool      { return false }
func (s *FeatureReviewStep) Enter(*model.Spec) tea.Cmd { return nil }

func (s *FeatureReviewStep) Update(msg tea.Msg, _ *model.Spec) (tea.Cmd, Outcome) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, StayHere
	}
	switch key.String() {
	case "ctrl+c":
		return nil, Cancel
	case "esc":
		return nil, GoBack
	case "enter":
		return nil, Finish
	}
	return nil, StayHere
}

func (s *FeatureReviewStep) View(spec model.Spec, width int) string {
	d := s.Draft
	pascal := model.Pascal(d.Name)
	pkg := model.PackageSegment(d.Name)
	pkgPath := strings.ReplaceAll(s.Manifest.Package, ".", "/")

	var created, edited []string

	if d.Android {
		created = append(created,
			fmt.Sprintf("feature/%s/api/build.gradle.kts", d.Name),
			fmt.Sprintf("feature/%s/api/.../%sRoute.kt", d.Name, pascal),
			fmt.Sprintf("feature/%s/impl/build.gradle.kts", d.Name),
			fmt.Sprintf("feature/%s/impl/.../%sNavigation.kt", d.Name, pascal),
			fmt.Sprintf("feature/%s/impl/.../%sScreen.kt", d.Name, pascal),
		)
		edited = append(edited,
			"settings.gradle.kts",
			"androidApp/build.gradle.kts",
			fmt.Sprintf("androidApp/src/main/kotlin/%s/AppSerializers.kt", pkgPath),
			fmt.Sprintf("androidApp/src/main/kotlin/%s/App.kt", pkgPath),
			"buildSrc/.../"+model.LowerAlnum(s.Manifest.Name)+".android.feature.gradle.kts",
		)
		if d.RootTab {
			edited = append(edited,
				"core/ui/build.gradle.kts",
				fmt.Sprintf("core/ui/src/main/kotlin/%s/core/ui/components/Navigation.kt", pkgPath))
		}
	}
	if d.Shared {
		created = append(created,
			fmt.Sprintf("sharedLogic/.../feature/%s/domain/%sRepository.kt", pkg, pascal),
			fmt.Sprintf("sharedLogic/.../feature/%s/data/%sRepositoryImpl.kt", pkg, pascal),
			fmt.Sprintf("sharedLogic/.../feature/%s/presentation/%sViewModel.kt", pkg, pascal),
			fmt.Sprintf("sharedLogic/.../feature/%s/di/%sModule.kt", pkg, pascal),
		)
		edited = append(edited,
			fmt.Sprintf("sharedLogic/src/commonMain/kotlin/%s/di/SharedModules.kt", pkgPath))
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("  %s %-14s %s\n", styleOK.Render(glyphDone), "Feature",
		styleText.Render(d.Name)))
	b.WriteString(fmt.Sprintf("  %s %-14s %s\n", styleOK.Render(glyphDone), "Presentation",
		styleText.Render(presentationLabel(d))))

	b.WriteString("\n" + styleMuted.Render("  New files") + "\n")
	for _, f := range created {
		b.WriteString("    " + styleOK.Render("+") + " " + styleText.Render(f) + "\n")
	}
	b.WriteString("\n" + styleMuted.Render("  Wired into") + "\n")
	for _, f := range edited {
		b.WriteString("    " + styleWarn.Render("~") + " " + styleText.Render(f) + "\n")
	}
	return b.String()
}

func presentationLabel(d *FeatureDraft) string {
	switch {
	case !d.Android:
		return "shared logic only"
	case d.RootTab:
		return "root tab in the navigation shell"
	case d.Presentation == "overlay":
		return "bottom sheet"
	case d.Presentation == "dialog":
		return "dialog"
	default:
		return "full screen, pushed above the shell"
	}
}
