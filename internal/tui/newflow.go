package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
	"github.com/aaroncutress/kmp-scaffold/internal/generator"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
)

// NewFlow builds the wizard for `kmp-scaffold new`.
//
// The returned Holder carries the resolution result, which the caller needs in
// order to generate.
func NewFlow(ctx context.Context, defaults model.Spec) (*Wizard, *Holder) {
	holder := &Holder{}

	steps := []Step{
		&TextStep{
			Prompt: "What is your project called?",
			Hint:   "Used for rootProject.name, the app label and the generated Kotlin type names.",
			Default: func(spec model.Spec) string {
				if spec.Name != "" {
					return spec.Name
				}
				return "My App"
			},
			Validate: func(v string, _ model.Spec) error { return model.ValidateProjectName(v) },
			Apply:    func(spec *model.Spec, v string) { spec.Name = v },
		},

		&TextStep{
			Prompt: "Where should it go?",
			Hint:   "A directory, relative to where you are now. It will be created if it does not exist.",
			Default: func(spec model.Spec) string {
				if spec.Dir != "" {
					return spec.Dir
				}
				return model.Kebab(spec.Name)
			},
			Validate: func(v string, _ model.Spec) error {
				if strings.TrimSpace(v) == "" {
					return errors.New("give a directory, or . for the current one")
				}
				return nil
			},
			Apply: func(spec *model.Spec, v string) { spec.Dir = filepath.Clean(v) },
		},

		&TextStep{
			Prompt: "Package name?",
			Hint:   "Reverse-DNS, e.g. com.example.myapp. Also used as the Android applicationId.",
			Default: func(spec model.Spec) string {
				if spec.Package != "" {
					return spec.Package
				}
				return "com.example." + model.LowerAlnum(spec.Name)
			},
			Validate: func(v string, _ model.Spec) error { return model.ValidatePackage(v) },
			Apply: func(spec *model.Spec, v string) {
				spec.Package = v
				spec.ApplicationID = v
			},
		},

		&CheckStep{
			Prompt: "Which platforms?",
			Hint:   "sharedLogic (the Kotlin Multiplatform module) is always generated.",
			Options: func(model.Spec) []Option {
				return []Option{
					{ID: "android", Label: "Android app", Desc: "androidApp, core modules and feature modules, all Compose."},
					{ID: "ios", Label: "iOS app", Desc: "iosApp (SwiftUI) linking the shared framework built by Gradle."},
				}
			},
			Current: func(spec model.Spec) []string {
				var out []string
				if spec.Android {
					out = append(out, "android")
				}
				if spec.IOS {
					out = append(out, "ios")
				}
				return out
			},
			Apply: func(spec *model.Spec, ids []string) {
				spec.Android = model.Has(ids, "android")
				spec.IOS = model.Has(ids, "ios")
			},
		},

		&SelectStep{
			Prompt:  "How should Android navigation be laid out?",
			Hint:    "Both options use Navigation 3 with a serialisable back stack.",
			SkipIf:  func(spec model.Spec) bool { return !spec.Android },
			Options: layoutOptions(generator.KindAndroid),
			Current: func(spec model.Spec) string { return spec.AndroidLayout },
			Apply:   func(spec *model.Spec, id string) { spec.AndroidLayout = id },
		},

		&TextStep{
			Prompt: "Which root tabs?",
			Hint:   "Comma separated, in order. Each becomes a feature module with its own back stack.",
			SkipIf: func(spec model.Spec) bool {
				return !spec.Android || spec.AndroidLayout != "nav3-shell"
			},
			Default: func(spec model.Spec) string { return strings.Join(spec.RootTabs, ", ") },
			Validate: func(v string, _ model.Spec) error {
				tabs := splitTabs(v)
				if len(tabs) == 0 {
					return errors.New("name at least one tab, e.g. Home, Settings")
				}
				if len(tabs) > 6 {
					return errors.New("more than six root tabs will not fit a phone-width bar")
				}
				seen := map[string]bool{}
				for _, t := range tabs {
					if err := model.ValidateProjectName(t); err != nil {
						return fmt.Errorf("%q: %w", t, err)
					}
					key := model.Kebab(t)
					if seen[key] {
						return fmt.Errorf("%q appears twice", t)
					}
					seen[key] = true
				}
				return nil
			},
			Apply: func(spec *model.Spec, v string) { spec.RootTabs = splitTabs(v) },
		},

		&SelectStep{
			Prompt:  "How should the iOS app be laid out?",
			SkipIf:  func(spec model.Spec) bool { return !spec.IOS },
			Options: layoutOptions(generator.KindIOS),
			Current: func(spec model.Spec) string { return spec.IOSLayout },
			Apply:   func(spec *model.Spec, id string) { spec.IOSLayout = id },
		},

		&CheckStep{
			Prompt:     "Which shared utilities do you want?",
			Hint:       "These live in sharedLogic and are used by both platforms. All are on by default.",
			AllowEmpty: true,
			Options: func(spec model.Spec) []Option {
				var out []Option
				for _, u := range catalog.SharedUtilities() {
					opt := Option{ID: u.ID, Label: u.Label, Desc: u.Description}
					if u.ID == "ios-koin-helper" && !spec.IOS {
						continue
					}
					for _, pack := range u.RequiresPack {
						if !spec.HasPack(pack) {
							def, _ := catalog.PackByID(pack)
							opt.Disabled = true
							opt.DisabledNote = "needs the " + def.Label + " library pack, chosen later"
						}
					}
					out = append(out, opt)
				}
				return out
			},
			Current: func(spec model.Spec) []string { return spec.SharedUtils },
			Apply:   func(spec *model.Spec, ids []string) { spec.SharedUtils = ids },
		},

		&CheckStep{
			Prompt:     "Which Android extras?",
			Hint:       "Wiring in androidApp and core/ui that most apps end up writing anyway.",
			AllowEmpty: true,
			SkipIf:     func(spec model.Spec) bool { return !spec.Android },
			Options: func(spec model.Spec) []Option {
				var out []Option
				for _, e := range catalog.AndroidExtras() {
					opt := Option{ID: e.ID, Label: e.Label, Desc: e.Description}
					for _, u := range e.RequiresUtil {
						if !spec.HasSharedUtil(u) {
							def, _ := catalog.UtilityByID(u)
							opt.Disabled = true
							opt.DisabledNote = "needs the " + def.Label + " shared utility"
						}
					}
					out = append(out, opt)
				}
				return out
			},
			Current: func(spec model.Spec) []string { return spec.AndroidExtras },
			Apply:   func(spec *model.Spec, ids []string) { spec.AndroidExtras = ids },
		},

		&SelectStep{
			Prompt: "Which libraries to start with?",
			Hint:   "You can fine-tune the list on the next screen either way.",
			Options: func(model.Spec) []Option {
				return []Option{
					{ID: "basic", Label: "Basic",
						Desc: "Images, local database, secrets and logging on top of the always-present core."},
					{ID: "minimal", Label: "Minimal",
						Desc: "Only the core: Compose, Navigation 3, Koin, Ktor, coroutines, serialisation."},
					{ID: "everything", Label: "Everything",
						Desc: "Every pack in the catalog. Useful to see what is on offer, rarely what you want."},
				}
			},
			Current: func(model.Spec) string { return "basic" },
			Apply: func(spec *model.Spec, id string) {
				switch id {
				case "minimal":
					spec.Packs = nil
				case "everything":
					var all []string
					for _, p := range catalog.Packs() {
						all = append(all, p.ID)
					}
					spec.Packs = all
				default:
					spec.Packs = catalog.BasicPacks()
				}
			},
		},

		&CheckStep{
			Prompt:     "Confirm the libraries",
			Hint:       "Versions are resolved after this, so nothing here pins you to a release.",
			AllowEmpty: true,
			Options: func(spec model.Spec) []Option {
				var out []Option
				for _, p := range catalog.Packs() {
					opt := Option{ID: p.ID, Label: p.Label, Desc: p.Description}
					if p.RequiresIOS && !spec.IOS {
						opt.Disabled = true
						opt.DisabledNote = "needs an iOS target"
					}
					if p.RequiresAndroid && !spec.Android {
						opt.Disabled = true
						opt.DisabledNote = "needs an Android target"
					}
					out = append(out, opt)
				}
				return out
			},
			Current: func(spec model.Spec) []string { return spec.Packs },
			Apply:   func(spec *model.Spec, ids []string) { spec.Packs = ids },
		},

		&SelectStep{
			Prompt: "How current should the versions be?",
			Hint:   "Libraries with no stable release (Navigation 3, adaptive Material) always use their newest track.",
			Options: func(model.Spec) []Option {
				return []Option{
					{ID: "preview", Label: "Latest, including release candidates",
						Desc: "Final releases plus -rc and -beta. A good default for a new project."},
					{ID: "stable", Label: "Stable only",
						Desc: "Final releases. The safest choice for something going to production soon."},
					{ID: "bleeding", Label: "Bleeding edge",
						Desc: "Alphas too. Expect API churn between builds."},
				}
			},
			Current: func(spec model.Spec) string { return spec.Channel },
			Apply:   func(spec *model.Spec, id string) { spec.Channel = id },
		},

		&TextStep{
			Prompt: "Minimum Android SDK?",
			Hint:   "26 (Android 8) is the floor this catalog is tested against. compileSdk is resolved for you.",
			SkipIf: func(spec model.Spec) bool { return !spec.Android },
			Default: func(spec model.Spec) string {
				if spec.MinSDK > 0 {
					return strconv.Itoa(spec.MinSDK)
				}
				return "26"
			},
			Validate: func(v string, _ model.Spec) error {
				n, err := strconv.Atoi(v)
				if err != nil {
					return errors.New("enter an API level, e.g. 26")
				}
				if n < 21 || n > 40 {
					return errors.New("that is not a plausible API level")
				}
				return nil
			},
			Apply: func(spec *model.Spec, v string) {
				n, _ := strconv.Atoi(v)
				spec.MinSDK = n
			},
		},

		NewResolveStep(ctx, holder),

		&ReviewStep{Holder: holder},
	}

	return NewWizard("kmp-scaffold · new project", defaults, steps), holder
}

// layoutOptions turns the registered generators of a kind into wizard choices,
// which is what makes a newly registered layout appear without touching the
// wizard.
func layoutOptions(kind generator.Kind) func(model.Spec) []Option {
	return func(model.Spec) []Option {
		var out []Option
		for _, g := range generator.Layouts(kind) {
			out = append(out, Option{ID: g.ID(), Label: g.Label(), Desc: g.Description()})
		}
		if kind == generator.KindIOS {
			out = append(out, Option{
				ID: "none", Label: "No iOS project files",
				Desc: "Still builds the shared framework - add the Xcode project yourself later.",
			})
		}
		return out
	}
}

func splitTabs(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Review
// ---------------------------------------------------------------------------

// ReviewStep is the final confirmation: everything chosen, in one screen.
type ReviewStep struct {
	Holder *Holder
}

func (s *ReviewStep) Title() string             { return "Ready to generate" }
func (s *ReviewStep) Help() string              { return "enter create · esc back · ctrl+c quit" }
func (s *ReviewStep) Skip(model.Spec) bool      { return false }
func (s *ReviewStep) Enter(*model.Spec) tea.Cmd { return nil }

func (s *ReviewStep) Update(msg tea.Msg, spec *model.Spec) (tea.Cmd, Outcome) {
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

func (s *ReviewStep) View(spec model.Spec, width int) string {
	var b strings.Builder

	row := func(label, value string) {
		b.WriteString(fmt.Sprintf("  %s %-16s %s\n",
			styleOK.Render(glyphDone), label, styleText.Render(value)))
	}

	row("Project", spec.Name)
	row("Directory", spec.Dir)
	row("Package", spec.Package)

	targets := []string{"sharedLogic"}
	if spec.Android {
		targets = append(targets, "androidApp")
	}
	if spec.IOS && spec.IOSLayout != "none" {
		targets = append(targets, "iosApp")
	}
	row("Modules", strings.Join(targets, ", "))

	if spec.Android {
		row("Android layout", spec.AndroidLayout)
		if spec.AndroidLayout == "nav3-shell" {
			row("Root tabs", strings.Join(spec.RootTabs, ", "))
		}
	}
	if spec.IOS {
		row("iOS layout", spec.IOSLayout)
	}

	b.WriteString("\n" + styleMuted.Render("  Shared utilities") + "\n")
	b.WriteString(bullets(labelsForUtils(spec.SharedUtils), width))

	if spec.Android {
		b.WriteString("\n" + styleMuted.Render("  Android extras") + "\n")
		b.WriteString(bullets(labelsForExtras(spec.AndroidExtras), width))
	}

	b.WriteString("\n" + styleMuted.Render("  Libraries") + "\n")
	b.WriteString(bullets(append([]string{"Core (Compose, Navigation 3, Koin, Ktor, coroutines, serialisation)"},
		labelsForPacks(spec.Packs)...), width))

	if res := s.Holder.Result; res != nil {
		b.WriteString("\n" + styleMuted.Render("  Toolchain") + "\n")
		b.WriteString("    " + styleText.Render(fmt.Sprintf(
			"Gradle %s · AGP %s · Kotlin %s", res.Gradle.Version,
			res.V(catalog.KeyAGP), res.V(catalog.KeyKotlin))) + "\n")
		if res.HasErrors() {
			b.WriteString("\n  " + styleErr.Render(glyphWarn+
				" The resolver flagged an incompatibility above - go back and check before generating.") + "\n")
		}
	}

	return b.String()
}

func bullets(labels []string, width int) string {
	if len(labels) == 0 {
		return "    " + styleMuted.Render("(none)") + "\n"
	}
	var b strings.Builder
	for _, l := range labels {
		b.WriteString("    " + styleMuted.Render(glyphInfo+" ") + styleText.Render(l) + "\n")
	}
	return b.String()
}

func labelsForUtils(ids []string) []string {
	var out []string
	for _, id := range ids {
		if def, ok := catalog.UtilityByID(id); ok {
			out = append(out, def.Label)
		}
	}
	return out
}

func labelsForExtras(ids []string) []string {
	var out []string
	for _, id := range ids {
		if def, ok := catalog.ExtraByID(id); ok {
			out = append(out, def.Label)
		}
	}
	return out
}

func labelsForPacks(ids []string) []string {
	var out []string
	for _, id := range ids {
		if def, ok := catalog.PackByID(id); ok {
			out = append(out, def.Label)
		}
	}
	return out
}
