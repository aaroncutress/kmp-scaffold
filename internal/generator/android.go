package generator

import (
	"fmt"
	"path"

	"github.com/aaroncutress/kmp-scaffold/internal/model"
)

func init() {
	Register(androidGenerator{shell: true})
	Register(androidGenerator{shell: false})
}

// androidGenerator writes androidApp, core/navigation, core/ui and the
// per-feature modules. The shell variant adds the responsive navigation shell
// (floating bar / rail / drawer) and one root-tab module per tab; the single
// variant has one screen stack and no tabs.
type androidGenerator struct{ shell bool }

func (g androidGenerator) ID() string {
	if g.shell {
		return "nav3-shell"
	}
	return "nav3-single"
}

func (g androidGenerator) Label() string {
	if g.shell {
		return "Navigation 3 + adaptive shell"
	}
	return "Navigation 3, single stack"
}

func (g androidGenerator) Description() string {
	if g.shell {
		return "Root tabs in a floating bar that becomes a rail then a drawer as the window grows"
	}
	return "One back stack, no tab shell - for apps that navigate from a single entry point"
}

func (g androidGenerator) Kind() Kind { return KindAndroid }

func (g androidGenerator) Generate(env *Env) error {
	spec := env.Ctx.Spec
	pkg := spec.PackagePath()

	app := func(p string) string {
		return path.Join("androidApp/src/main/kotlin", pkg, p)
	}
	nav := func(p string) string {
		return path.Join("core/navigation/src/main/kotlin", pkg, "core/navigation", p)
	}
	ui := func(p string) string {
		return path.Join("core/ui/src/main/kotlin", pkg, "core/ui", p)
	}

	type file struct {
		tpl, path string
		when      bool
	}

	files := []file{
		// androidApp
		{"android/app.build.gradle.kts", "androidApp/build.gradle.kts", true},
		{"android/AndroidManifest.xml", "androidApp/src/main/AndroidManifest.xml", true},
		{"android/proguard-rules.pro", "androidApp/proguard-rules.pro", true},
		{"android/strings.xml", "androidApp/src/main/res/values/strings.xml", true},
		{"android/themes.xml", "androidApp/src/main/res/values/themes.xml", true},
		{"android/ic_launcher_background.xml", "androidApp/src/main/res/values/ic_launcher_background.xml", true},
		{"android/ic_launcher.xml", "androidApp/src/main/res/mipmap-anydpi-v26/ic_launcher.xml", true},
		{"android/ic_launcher_round.xml", "androidApp/src/main/res/mipmap-anydpi-v26/ic_launcher_round.xml", true},
		{"android/ic_launcher_foreground.xml", "androidApp/src/main/res/drawable/ic_launcher_foreground.xml", true},
		{"android/Application.kt", app(spec.AppClassName() + ".kt"), true},
		{"android/MainActivity.kt", app("MainActivity.kt"), true},
		{"android/App.kt", app("App.kt"), true},
		{"android/AppSerializers.kt", app("AppSerializers.kt"), true},

		// core/navigation
		{"android/core-navigation.build.gradle.kts", "core/navigation/build.gradle.kts", true},
		{"android/Route.kt", nav("Route.kt"), true},
		{"android/Navigator.kt", nav("Navigator.kt"), true},
		{"android/NavigationState.kt", nav("NavigationState.kt"), true},
		{"android/NavigationScenes.kt", nav("NavigationScenes.kt"), spec.HasAndroidExtra("nav3-scenes")},

		// core/ui
		{"android/core-ui.build.gradle.kts", "core/ui/build.gradle.kts", true},
		{"android/Theme.kt", ui("theme/Theme.kt"), true},
		{"android/Colours.kt", ui("theme/Colours.kt"), true},
		{"android/Typography.kt", ui("theme/Typography.kt"), true},
		{"android/Shapes.kt", ui("theme/Shapes.kt"), true},
		{"android/Tokens.kt", ui("theme/Tokens.kt"), true},
		{"android/Navigation.kt", ui("components/Navigation.kt"), g.shell},
		{"android/Snackbar.kt", ui("components/Snackbar.kt"), spec.HasAndroidExtra("snackbar-host")},
		{"android/ConnectivityBanner.kt", ui("components/ConnectivityBanner.kt"),
			spec.HasAndroidExtra("connectivity-banner")},
	}

	for _, f := range files {
		if !f.when {
			continue
		}
		if err := env.Render(f.tpl, f.path); err != nil {
			return err
		}
	}

	// Root tab modules (shell layout) or a single Home module (single layout).
	tabs := env.Ctx.Tabs
	if !g.shell {
		tabs = BuildTabs([]string{"Home"})
	}
	for _, tab := range tabs {
		fc := &FeatureCtx{
			Name:            tab.Name,
			Pascal:          tab.Pascal,
			Camel:           tab.Camel,
			Pkg:             tab.Pkg,
			Presentation:    "shell",
			RootTab:         g.shell,
			Android:         true,
			ProjectAccessor: model.Camel(tab.Name),
		}
		sub := *env
		sub.Ctx.Feature = fc
		if err := g.GenerateFeature(&sub); err != nil {
			return fmt.Errorf("generating %s tab: %w", tab.Label, err)
		}
	}

	return nil
}

// GenerateFeature writes one feature module's api and impl subprojects.
func (g androidGenerator) GenerateFeature(env *Env) error {
	f := env.Ctx.Feature
	if f == nil {
		return fmt.Errorf("GenerateFeature called without a feature context")
	}
	spec := env.Ctx.Spec
	pkg := spec.PackagePath()

	apiSrc := path.Join("feature", f.Name, "api/src/main/kotlin", pkg, "feature", f.Pkg, "api")
	implSrc := path.Join("feature", f.Name, "impl/src/main/kotlin", pkg, "feature", f.Pkg, "impl")

	files := []struct{ tpl, path string }{
		{"feature/api.build.gradle.kts", path.Join("feature", f.Name, "api/build.gradle.kts")},
		{"feature/Route.kt", path.Join(apiSrc, f.Pascal+"Route.kt")},
		{"feature/impl.build.gradle.kts", path.Join("feature", f.Name, "impl/build.gradle.kts")},
		{"feature/Navigation.kt", path.Join(implSrc, f.Pascal+"Navigation.kt")},
		{"feature/Screen.kt", path.Join(implSrc, f.Pascal+"Screen.kt")},
	}
	for _, file := range files {
		if err := env.Render(file.tpl, file.path); err != nil {
			return err
		}
	}
	return nil
}

// GenerateSharedFeature writes the sharedLogic half of a feature: a repository,
// a ViewModel and a Koin module. It is a package-level function rather than a
// method because it belongs to the shared module regardless of which Android
// or iOS layout is in use.
func GenerateSharedFeature(env *Env) error {
	f := env.Ctx.Feature
	if f == nil {
		return fmt.Errorf("GenerateSharedFeature called without a feature context")
	}
	base := path.Join("sharedLogic/src/commonMain/kotlin", env.Ctx.Spec.PackagePath(), "feature", f.Pkg)

	files := []struct{ tpl, path string }{
		{"feature/shared.Repository.kt", path.Join(base, "domain", f.Pascal+"Repository.kt")},
		{"feature/shared.RepositoryImpl.kt", path.Join(base, "data", f.Pascal+"RepositoryImpl.kt")},
		{"feature/shared.ViewModel.kt", path.Join(base, "presentation", f.Pascal+"ViewModel.kt")},
		{"feature/shared.Module.kt", path.Join(base, "di", f.Pascal+"Module.kt")},
	}
	for _, file := range files {
		if err := env.Render(file.tpl, file.path); err != nil {
			return err
		}
	}
	return nil
}
