package kmp

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
	"github.com/aaroncutress/kmp-scaffold/internal/generator"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
)

// Question ids. These match the JSON names in Vars, so an answer and the thing
// it is remembered as are never out of step.
const (
	QPlatforms     = "platforms"
	QAndroidLayout = "androidLayout"
	QRootTabs      = "rootTabs"
	QIOSLayout     = "iosLayout"
	QSharedUtils   = "sharedUtils"
	QAndroidExtras = "androidExtras"
	QPreset        = "preset"
	QPacks         = "packs"
	QChannel       = "channel"
	QMinSDK        = "minSdk"
	QJVMTarget     = "jvmTarget"
	QIOSDeployTgt  = "iosDeployTgt"
	QSwiftMode     = "swiftMode"
)

// Questions is the wizard for this template, asked after the universal name,
// directory and package questions.
//
// Every answer is applied to the underlying Spec as well as being recorded in
// the bag, because the generators and the catalog predicates work in terms of
// the Spec.
func (Template) Questions() []scaffold.Question {
	return []scaffold.Question{
		{
			ID:     QPlatforms,
			Kind:   scaffold.KindMultiSelect,
			Prompt: "Which platforms?",
			Hint:   "sharedLogic (the Kotlin Multiplatform module) is always generated.",
			Options: []scaffold.Option{
				{ID: "android", Label: "Android app",
					Desc: "androidApp, core modules and feature modules, all Compose."},
				{ID: "ios", Label: "iOS app",
					Desc: "iosApp (SwiftUI) linking the shared framework built by Gradle."},
			},
			DefaultFor: func(a *scaffold.Answers) any {
				spec := Spec(a)
				var out []string
				if spec.Android {
					out = append(out, "android")
				}
				if spec.IOS {
					out = append(out, "ios")
				}
				return out
			},
			Apply: func(a *scaffold.Answers, v any) {
				ids := a.Strs(QPlatforms)
				spec := Spec(a)
				spec.Android = model.Has(ids, "android")
				spec.IOS = model.Has(ids, "ios")
			},
		},

		{
			ID:         QAndroidLayout,
			Kind:       scaffold.KindSelect,
			Prompt:     "How should Android navigation be laid out?",
			Hint:       "Both options use Navigation 3 with a serialisable back stack.",
			SkipFor:    func(a *scaffold.Answers) bool { return !Spec(a).Android },
			OptionsFor: layoutOptions(generator.KindAndroid),
			DefaultFor: func(a *scaffold.Answers) any { return Spec(a).AndroidLayout },
			Apply:      func(a *scaffold.Answers, v any) { Spec(a).AndroidLayout = a.Str(QAndroidLayout) },
		},

		{
			ID:     QRootTabs,
			Kind:   scaffold.KindList,
			Prompt: "Which root tabs?",
			Hint:   "Comma separated, in order. Each becomes a feature module with its own back stack.",
			SkipFor: func(a *scaffold.Answers) bool {
				spec := Spec(a)
				return !spec.Android || spec.AndroidLayout != "nav3-shell"
			},
			DefaultFor: func(a *scaffold.Answers) any { return Spec(a).RootTabs },
			Validate: func(v any, _ *scaffold.Answers) error {
				tabs, _ := v.([]string)
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
			Apply: func(a *scaffold.Answers, v any) { Spec(a).RootTabs = a.Strs(QRootTabs) },
		},

		{
			ID:         QIOSLayout,
			Kind:       scaffold.KindSelect,
			Prompt:     "How should the iOS app be laid out?",
			SkipFor:    func(a *scaffold.Answers) bool { return !Spec(a).IOS },
			OptionsFor: layoutOptions(generator.KindIOS),
			DefaultFor: func(a *scaffold.Answers) any { return Spec(a).IOSLayout },
			Apply:      func(a *scaffold.Answers, v any) { Spec(a).IOSLayout = a.Str(QIOSLayout) },
		},

		{
			ID:         QSharedUtils,
			Kind:       scaffold.KindMultiSelect,
			Prompt:     "Which shared utilities do you want?",
			Hint:       "These live in sharedLogic and are used by both platforms. All are on by default.",
			AllowEmpty: true,
			OptionsFor: func(a *scaffold.Answers) []scaffold.Option {
				spec := Spec(a)
				var out []scaffold.Option
				for _, u := range catalog.SharedUtilities() {
					opt := scaffold.Option{ID: u.ID, Label: u.Label, Desc: u.Description}
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
			DefaultFor: func(a *scaffold.Answers) any { return Spec(a).SharedUtils },
			Apply:      func(a *scaffold.Answers, v any) { Spec(a).SharedUtils = a.Strs(QSharedUtils) },
		},

		{
			ID:         QAndroidExtras,
			Kind:       scaffold.KindMultiSelect,
			Prompt:     "Which Android extras?",
			Hint:       "Wiring in androidApp and core/ui that most apps end up writing anyway.",
			AllowEmpty: true,
			SkipFor:    func(a *scaffold.Answers) bool { return !Spec(a).Android },
			OptionsFor: func(a *scaffold.Answers) []scaffold.Option {
				spec := Spec(a)
				var out []scaffold.Option
				for _, e := range catalog.AndroidExtras() {
					opt := scaffold.Option{ID: e.ID, Label: e.Label, Desc: e.Description}
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
			DefaultFor: func(a *scaffold.Answers) any { return Spec(a).AndroidExtras },
			Apply:      func(a *scaffold.Answers, v any) { Spec(a).AndroidExtras = a.Strs(QAndroidExtras) },
		},

		{
			ID:     QPreset,
			Kind:   scaffold.KindSelect,
			Prompt: "Which libraries to start with?",
			Hint:   "You can fine-tune the list on the next screen either way.",
			Options: []scaffold.Option{
				{ID: "basic", Label: "Basic",
					Desc: "Images, local database, secrets and logging on top of the always-present core."},
				{ID: "minimal", Label: "Minimal",
					Desc: "Only the core: Compose, Navigation 3, Koin, Ktor, coroutines, serialisation."},
				{ID: "everything", Label: "Everything",
					Desc: "Every pack in the catalog. Useful to see what is on offer, rarely what you want."},
			},
			Default: "basic",
			// The preset only seeds the next screen, which is where the real
			// answer is given, so it is not part of Vars.
			Apply: func(a *scaffold.Answers, v any) {
				spec := Spec(a)
				switch a.Str(QPreset) {
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
				a.Set(QPacks, spec.Packs)
			},
		},

		{
			ID:         QPacks,
			Kind:       scaffold.KindMultiSelect,
			Prompt:     "Confirm the libraries",
			Hint:       "Versions are resolved after this, so nothing here pins you to a release.",
			AllowEmpty: true,
			OptionsFor: func(a *scaffold.Answers) []scaffold.Option {
				spec := Spec(a)
				var out []scaffold.Option
				for _, p := range catalog.Packs() {
					opt := scaffold.Option{ID: p.ID, Label: p.Label, Desc: p.Description}
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
			DefaultFor: func(a *scaffold.Answers) any { return Spec(a).Packs },
			Apply:      func(a *scaffold.Answers, v any) { Spec(a).Packs = a.Strs(QPacks) },
		},

		{
			ID:     QChannel,
			Kind:   scaffold.KindSelect,
			Prompt: "How current should the versions be?",
			Hint:   "Libraries with no stable release (Navigation 3, adaptive Material) always use their newest track.",
			Options: []scaffold.Option{
				{ID: "preview", Label: "Latest, including release candidates",
					Desc: "Final releases plus -rc and -beta. A good default for a new project."},
				{ID: "stable", Label: "Stable only",
					Desc: "Final releases. The safest choice for something going to production soon."},
				{ID: "bleeding", Label: "Bleeding edge",
					Desc: "Alphas too. Expect API churn between builds."},
			},
			DefaultFor: func(a *scaffold.Answers) any { return Spec(a).Channel },
			Apply:      func(a *scaffold.Answers, v any) { Spec(a).Channel = a.Str(QChannel) },
		},

		{
			ID:      QMinSDK,
			Kind:    scaffold.KindText,
			Prompt:  "Minimum Android SDK?",
			Hint:    "26 (Android 8) is the floor this catalog is tested against. compileSdk is resolved for you.",
			SkipFor: func(a *scaffold.Answers) bool { return !Spec(a).Android },
			DefaultFor: func(a *scaffold.Answers) any {
				if n := Spec(a).MinSDK; n > 0 {
					return strconv.Itoa(n)
				}
				return "26"
			},
			Validate: func(v any, _ *scaffold.Answers) error {
				n, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(v)))
				if err != nil {
					return errors.New("enter an API level, e.g. 26")
				}
				if n < 21 || n > 40 {
					return errors.New("that is not a plausible API level")
				}
				return nil
			},
			Apply: func(a *scaffold.Answers, v any) { Spec(a).MinSDK = a.Int(QMinSDK) },
		},

		{
			ID:      QIOSDeployTgt,
			Kind:    scaffold.KindSelect,
			Prompt:  "Minimum iOS version?",
			Hint:    "The counterpart to minSdk: the oldest iOS a device can be on and still run this.",
			SkipFor: func(a *scaffold.Answers) bool { return !Spec(a).IOS },
			Options: []scaffold.Option{
				{ID: "18.0", Label: "iOS 18",
					Desc: "Two releases back. Covers almost every device still getting updates."},
				{ID: "26.0", Label: "iOS 26",
					Desc: "The current release. Every API available, the smallest audience."},
				{ID: "17.0", Label: "iOS 17",
					Desc: "Three releases back, for an app that has to reach older hardware."},
			},
			DefaultFor: func(a *scaffold.Answers) any { return Spec(a).IOSDeployTgt },
			Apply:      func(a *scaffold.Answers, v any) { Spec(a).IOSDeployTgt = a.Str(QIOSDeployTgt) },
		},

		{
			ID:      QSwiftMode,
			Kind:    scaffold.KindSelect,
			Prompt:  "Which Swift language mode?",
			Hint:    "Swift 6 checks concurrency at compile time. The Kotlin framework is not annotated for it.",
			SkipFor: func(a *scaffold.Answers) bool { return !Spec(a).IOS },
			Options: []scaffold.Option{
				{ID: "5", Label: "Swift 5",
					Desc: "Concurrency warnings, not errors. What the generated code is written against."},
				{ID: "6", Label: "Swift 6",
					Desc: "Strict concurrency. Types crossing an actor boundary from Kotlin will need " +
						"@unchecked Sendable annotations you write yourself."},
			},
			DefaultFor: func(a *scaffold.Answers) any { return Spec(a).SwiftMode },
			Apply:      func(a *scaffold.Answers, v any) { Spec(a).SwiftMode = a.Str(QSwiftMode) },
		},

		{
			ID:     QJVMTarget,
			Kind:   scaffold.KindSelect,
			Prompt: "Which Java version should the JVM targets compile to?",
			Hint:   "Applies to the Android app and sharedLogic's JVM output, not to the Gradle daemon.",
			Options: []scaffold.Option{
				{ID: "17", Label: "Java 17",
					Desc: "What Android Gradle Plugin 9 and Gradle 9 are built around."},
				{ID: "21", Label: "Java 21",
					Desc: "The current long-term release. Needs a JDK 21 toolchain to build."},
				{ID: "11", Label: "Java 11",
					Desc: "For a codebase that still has to interoperate with something older."},
			},
			DefaultFor: func(a *scaffold.Answers) any { return Spec(a).JVMTarget },
			Apply:      func(a *scaffold.Answers, v any) { Spec(a).JVMTarget = a.Str(QJVMTarget) },
		},
	}
}

// layoutOptions turns the registered generators of a kind into wizard choices,
// which is what makes a newly registered layout appear without touching the
// question list.
func layoutOptions(kind generator.Kind) func(*scaffold.Answers) []scaffold.Option {
	return func(*scaffold.Answers) []scaffold.Option {
		var out []scaffold.Option
		for _, g := range generator.Layouts(kind) {
			out = append(out, scaffold.Option{ID: g.ID(), Label: g.Label(), Desc: g.Description()})
		}
		if kind == generator.KindIOS {
			out = append(out, scaffold.Option{
				ID: "none", Label: "No iOS project files",
				Desc: "Still builds the shared framework - add the Xcode project yourself later.",
			})
		}
		return out
	}
}

// LayoutIDs lists the registered layout ids of a kind, for flag help and
// validation.
func LayoutIDs(kind generator.Kind) []string {
	var out []string
	for _, g := range generator.Layouts(kind) {
		out = append(out, g.ID())
	}
	if kind == generator.KindIOS {
		out = append(out, "none")
	}
	return out
}
