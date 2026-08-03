package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/render"
	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
)

// testSpec is a fully-featured project, generated offline so the test never
// touches the network.
func testSpec(dir string) model.Spec {
	spec := model.Defaults()
	spec.Name = "Tunesic"
	spec.Dir = dir
	spec.Package = "io.kontour.tunesic"
	spec.ApplicationID = "io.kontour.tunesic"
	spec.Packs = catalog.BasicPacks()
	spec.SharedUtils = catalog.DefaultUtilities()
	spec.AndroidExtras = catalog.DefaultExtras()
	spec.Offline = true
	catalog.Normalise(&spec)
	return spec
}

func generate(t *testing.T, spec model.Spec) string {
	t.Helper()
	root := t.TempDir()
	res := resolve.Run(context.Background(), spec, resolve.Options{Offline: true})
	writer := render.NewWriter(root, false, false)
	if _, err := NewProject(context.Background(), spec, res, "test", writer); err != nil {
		t.Fatalf("generating: %v", err)
	}
	return root
}

func mustRead(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return string(data)
}

func TestGenerateFullProject(t *testing.T) {
	spec := testSpec("tunesic")
	root := generate(t, spec)

	// Every file a project needs to open in an IDE.
	for _, rel := range []string{
		"settings.gradle.kts",
		"build.gradle.kts",
		"gradle.properties",
		"gradle/libs.versions.toml",
		"gradle/wrapper/gradle-wrapper.properties",
		"buildSrc/build.gradle.kts",
		"buildSrc/src/main/kotlin/tunesic.android.library.gradle.kts",
		"buildSrc/src/main/kotlin/tunesic.android.compose.gradle.kts",
		"buildSrc/src/main/kotlin/tunesic.android.feature.gradle.kts",
		"buildSrc/src/main/kotlin/tunesic.secrets.gradle.kts",
		"sharedLogic/build.gradle.kts",
		"sharedLogic/src/commonMain/kotlin/io/kontour/tunesic/di/SharedModules.kt",
		"sharedLogic/src/androidMain/kotlin/io/kontour/tunesic/di/SharedModules.android.kt",
		"sharedLogic/src/iosMain/kotlin/io/kontour/tunesic/di/SharedModules.ios.kt",
		"sharedLogic/src/iosMain/kotlin/io/kontour/tunesic/KoinHelper.kt",
		"androidApp/build.gradle.kts",
		"androidApp/src/main/AndroidManifest.xml",
		"androidApp/src/main/kotlin/io/kontour/tunesic/App.kt",
		"androidApp/src/main/kotlin/io/kontour/tunesic/AppSerializers.kt",
		"androidApp/src/main/kotlin/io/kontour/tunesic/TunesicApplication.kt",
		"core/navigation/src/main/kotlin/io/kontour/tunesic/core/navigation/Route.kt",
		"core/ui/src/main/kotlin/io/kontour/tunesic/core/ui/components/Navigation.kt",
		"feature/home/api/src/main/kotlin/io/kontour/tunesic/feature/home/api/HomeRoute.kt",
		"feature/home/impl/src/main/kotlin/io/kontour/tunesic/feature/home/impl/HomeScreen.kt",
		"iosApp/iosApp.xcodeproj/project.pbxproj",
		"iosApp/iosApp/iOSApp.swift",
		model.ManifestFile,
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}
}

func TestGeneratedFilesHaveNoLeadingBlankLine(t *testing.T) {
	root := generate(t, testSpec("tunesic"))

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		switch filepath.Ext(path) {
		case ".kt", ".kts", ".swift", ".toml", ".xml":
		default:
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(string(data), "\n") {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s starts with a blank line", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The generated App.kt, Route.kt and version catalog must agree about names,
// or the project will not compile.
func TestGeneratedNamesAreConsistent(t *testing.T) {
	spec := testSpec("tunesic")
	spec.RootTabs = []string{"Home", "Settings"}
	catalog.Normalise(&spec)
	root := generate(t, spec)

	app := mustRead(t, root, "androidApp/src/main/kotlin/io/kontour/tunesic/App.kt")
	for _, want := range []string{
		"HomeRoute", "SettingsRoute", "homeEntries()", "settingsEntries()",
		"TunesicTheme", "ResponsiveNavigationShell",
	} {
		if !strings.Contains(app, want) {
			t.Errorf("App.kt is missing %q", want)
		}
	}

	serializers := mustRead(t, root, "androidApp/src/main/kotlin/io/kontour/tunesic/AppSerializers.kt")
	for _, want := range []string{"homeNavSerializers", "settingsNavSerializers", "coreNavSerializers"} {
		if !strings.Contains(serializers, want) {
			t.Errorf("AppSerializers.kt is missing %q", want)
		}
	}

	nav := mustRead(t, root, "core/ui/src/main/kotlin/io/kontour/tunesic/core/ui/components/Navigation.kt")
	if !strings.Contains(nav, "AppNavigationItem(HomeRoute") ||
		!strings.Contains(nav, "AppNavigationItem(SettingsRoute") {
		t.Error("the navigation item list does not cover every root tab")
	}
}

// Every anchor the add-feature flow relies on must be present in a fresh
// project, otherwise `add feature` silently half-wires the module.
func TestGeneratedProjectHasEveryAnchor(t *testing.T) {
	root := generate(t, testSpec("tunesic"))

	cases := []struct{ rel, anchor string }{
		{"settings.gradle.kts", "kmp-scaffold:features"},
		{"androidApp/build.gradle.kts", "kmp-scaffold:feature-deps"},
		{"buildSrc/src/main/kotlin/tunesic.android.feature.gradle.kts", "kmp-scaffold:feature-apis"},
		{"core/ui/build.gradle.kts", "kmp-scaffold:feature-apis"},
		{"androidApp/src/main/kotlin/io/kontour/tunesic/AppSerializers.kt", "kmp-scaffold:serializers"},
		{"androidApp/src/main/kotlin/io/kontour/tunesic/App.kt", "kmp-scaffold:entries"},
		{"androidApp/src/main/kotlin/io/kontour/tunesic/App.kt", "kmp-scaffold:root-entries"},
		{"androidApp/src/main/kotlin/io/kontour/tunesic/App.kt", "kmp-scaffold:roots"},
		{"core/ui/src/main/kotlin/io/kontour/tunesic/core/ui/components/Navigation.kt", "kmp-scaffold:nav-items"},
		{"sharedLogic/src/commonMain/kotlin/io/kontour/tunesic/di/SharedModules.kt", "kmp-scaffold:shared-modules"},
	}
	for _, c := range cases {
		if !strings.Contains(mustRead(t, root, c.rel), c.anchor) {
			t.Errorf("%s is missing the %s anchor", c.rel, c.anchor)
		}
	}
}

func TestAddFeatureWiresEverything(t *testing.T) {
	root := generate(t, testSpec("tunesic"))

	manifest, _, err := model.LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}

	writer := render.NewWriter(root, false, false)
	report, err := AddFeature(manifest, root, FeatureRequest{
		Name: "firmware-update", Android: true, Shared: true, Presentation: "above-nav",
	}, "test", writer, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Warnings) > 0 {
		t.Errorf("unexpected warnings: %v", report.Warnings)
	}

	// The kebab-case module name becomes a run-together package segment.
	for _, rel := range []string{
		"feature/firmware-update/api/build.gradle.kts",
		"feature/firmware-update/api/src/main/kotlin/io/kontour/tunesic/feature/firmwareupdate/api/FirmwareUpdateRoute.kt",
		"feature/firmware-update/impl/src/main/kotlin/io/kontour/tunesic/feature/firmwareupdate/impl/FirmwareUpdateScreen.kt",
		"sharedLogic/src/commonMain/kotlin/io/kontour/tunesic/feature/firmwareupdate/di/FirmwareUpdateModule.kt",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}

	// Gradle's type-safe accessors camel-case a kebab module name.
	appBuild := mustRead(t, root, "androidApp/build.gradle.kts")
	if !strings.Contains(appBuild, "projects.feature.firmwareUpdate.api") {
		t.Errorf("androidApp/build.gradle.kts is missing the type-safe accessor:\n%s", appBuild)
	}

	settings := mustRead(t, root, "settings.gradle.kts")
	if !strings.Contains(settings, `include(":feature:firmware-update:api")`) {
		t.Error("settings.gradle.kts is missing the new module")
	}

	app := mustRead(t, root, "androidApp/src/main/kotlin/io/kontour/tunesic/App.kt")
	if !strings.Contains(app, "firmwareUpdateEntries()") {
		t.Error("App.kt does not call the new entries function")
	}
	if !strings.Contains(app, "import io.kontour.tunesic.feature.firmwareupdate.impl.firmwareUpdateEntries") {
		t.Error("App.kt is missing the import for the new entries function")
	}

	shared := mustRead(t, root, "sharedLogic/src/commonMain/kotlin/io/kontour/tunesic/di/SharedModules.kt")
	if !strings.Contains(shared, "firmwareUpdateModule,") {
		t.Error("the shared Koin graph does not include the new module")
	}

	if f := manifest.FindFeature("firmware-update"); f == nil {
		t.Error("the manifest does not record the new feature")
	}
}

func TestAddRootTabJoinsTheShell(t *testing.T) {
	root := generate(t, testSpec("tunesic"))
	manifest, _, err := model.LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}

	writer := render.NewWriter(root, false, false)
	if _, err := AddFeature(manifest, root, FeatureRequest{
		Name: "library", Android: true, Shared: false, Presentation: "shell", RootTab: true,
	}, "test", writer, false); err != nil {
		t.Fatal(err)
	}

	app := mustRead(t, root, "androidApp/src/main/kotlin/io/kontour/tunesic/App.kt")
	if !strings.Contains(app, "LibraryRoute,") {
		t.Error("the new tab was not added to the roots set")
	}

	nav := mustRead(t, root, "core/ui/src/main/kotlin/io/kontour/tunesic/core/ui/components/Navigation.kt")
	if !strings.Contains(nav, "AppNavigationItem(LibraryRoute") {
		t.Error("the new tab was not added to the navigation item list")
	}
	if !strings.Contains(nav, "import com.composables.icons.materialsymbols.rounded.Library_music") {
		t.Error("the icon import for the new tab is missing")
	}

	// A root tab's entries belong to the inner NavDisplay, not the outer one.
	rootEntriesIdx := strings.Index(app, "kmp-scaffold:root-entries")
	callIdx := strings.Index(app, "libraryEntries()")
	entriesIdx := strings.Index(app, "kmp-scaffold:entries")
	if callIdx < 0 || callIdx > rootEntriesIdx || callIdx < entriesIdx {
		t.Error("a root tab's entries were not added to the per-tab entry provider")
	}
}

func TestMinimalProjectSkipsOptionalFiles(t *testing.T) {
	spec := testSpec("minimal")
	spec.Packs = nil
	spec.SharedUtils = []string{"koin-di", "base-viewmodel"}
	spec.AndroidExtras = nil
	spec.IOS = false
	catalog.Normalise(&spec)
	root := generate(t, spec)

	for _, rel := range []string{
		"iosApp/iosApp/iOSApp.swift",
		"secrets.properties.template",
		"sharedLogic/src/commonMain/kotlin/io/kontour/tunesic/core/database/AppDatabase.kt",
		"sharedLogic/src/commonMain/kotlin/io/kontour/tunesic/core/util/NetworkObserver.kt",
		"core/ui/src/main/kotlin/io/kontour/tunesic/core/ui/components/ConnectivityBanner.kt",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			t.Errorf("%s should not have been generated", rel)
		}
	}

	// The version catalog should not carry entries nothing depends on.
	toml := mustRead(t, root, "gradle/libs.versions.toml")
	for _, unwanted := range []string{"coil-compose", "androidx-room-runtime", "maps-compose", "ktor-client-darwin"} {
		if strings.Contains(toml, unwanted) {
			t.Errorf("the version catalog contains %q, which this project does not use", unwanted)
		}
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	spec := testSpec("tunesic")
	root := t.TempDir()
	res := resolve.Run(context.Background(), spec, resolve.Options{Offline: true})

	writer := render.NewWriter(root, true, false)
	if _, err := NewProject(context.Background(), spec, res, "test", writer); err != nil {
		t.Fatal(err)
	}
	if writer.Count(render.Planned) == 0 {
		t.Error("a dry run should still report planned files")
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("a dry run wrote %d entries", len(entries))
	}
}

func TestExistingFilesAreNotClobbered(t *testing.T) {
	spec := testSpec("tunesic")
	root := generate(t, spec)

	custom := "// my own settings\n"
	if err := os.WriteFile(filepath.Join(root, "settings.gradle.kts"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}

	res := resolve.Run(context.Background(), spec, resolve.Options{Offline: true})
	writer := render.NewWriter(root, false, false)
	if _, err := NewProject(context.Background(), spec, res, "test", writer); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, root, "settings.gradle.kts"); got != custom {
		t.Error("an existing file was overwritten without --force")
	}
	if len(writer.Conflicts()) == 0 {
		t.Error("the conflict was not reported")
	}
}

func TestLayoutRegistry(t *testing.T) {
	if len(Layouts(KindAndroid)) < 2 {
		t.Error("expected at least two Android layouts to be registered")
	}
	if len(Layouts(KindIOS)) < 1 {
		t.Error("expected at least one iOS layout to be registered")
	}
	if _, err := Get(KindAndroid, "nav3-shell"); err != nil {
		t.Errorf("nav3-shell should be registered: %v", err)
	}
	if _, err := Get(KindIOS, "does-not-exist"); err == nil {
		t.Error("an unknown layout should be an error")
	}
}
