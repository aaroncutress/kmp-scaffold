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
		"iosApp/iosApp/App/iOSApp.swift",
		"iosApp/Packages/Features/Package.swift",
		".run/androidApp.run.xml",
		".run/iosApp.run.xml",
		".run/Generate Build Konfig.run.xml",
		".run/Link iOS Framework (Debug).run.xml",
		".run/Build iOS XCFramework (Debug).run.xml",
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
		"iosApp/iosApp/App/iOSApp.swift",
		"iosApp/Packages/Features/Package.swift",
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

// ---------------------------------------------------------------------------
// .run configurations
// ---------------------------------------------------------------------------

func TestRunConfigurationsMatchTheProject(t *testing.T) {
	root := generate(t, testSpec("tunesic"))

	// The Android configuration names the module as the IDE sees it:
	// <rootProject.name>.<module>.
	android := mustRead(t, root, ".run/androidApp.run.xml")
	if !strings.Contains(android, `<module name="Tunesic.androidApp" />`) {
		t.Errorf("the Android run configuration does not name Tunesic.androidApp:\n%s", android)
	}

	konfig := mustRead(t, root, ".run/Generate Build Konfig.run.xml")
	if !strings.Contains(konfig, ":sharedLogic:generateBuildKonfig") {
		t.Error("the BuildKonfig configuration does not run generateBuildKonfig")
	}

	xcf := mustRead(t, root, ".run/Build iOS XCFramework (Debug).run.xml")
	if !strings.Contains(xcf, ":sharedLogic:assembleSharedLogicDebugXCFramework") {
		t.Errorf("the XCFramework configuration does not assemble the framework:\n%s", xcf)
	}
}

func TestRunConfigurationsFollowTheSpec(t *testing.T) {
	// No secrets pack, no BuildKonfig configuration.
	spec := testSpec("tunesic")
	spec.Packs = nil
	catalog.Normalise(&spec)
	root := generate(t, spec)
	if _, err := os.Stat(filepath.Join(root, ".run", "Generate Build Konfig.run.xml")); err == nil {
		t.Error("the BuildKonfig configuration was written without the secrets pack")
	}

	// The single-file iOS layout has no XCFramework to assemble, but still
	// links the framework Xcode embeds.
	simple := testSpec("simple")
	simple.IOSLayout = "swiftui-simple"
	catalog.Normalise(&simple)
	root = generate(t, simple)
	if _, err := os.Stat(filepath.Join(root, ".run", "Build iOS XCFramework (Debug).run.xml")); err == nil {
		t.Error("the XCFramework configuration should be specific to the modular iOS layout")
	}
	for _, rel := range []string{".run/iosApp.run.xml", ".run/Link iOS Framework (Debug).run.xml"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}

	// An Android-only project gets no iOS configurations at all.
	androidOnly := testSpec("androidonly")
	androidOnly.IOS = false
	catalog.Normalise(&androidOnly)
	root = generate(t, androidOnly)
	for _, rel := range []string{
		".run/iosApp.run.xml",
		".run/Link iOS Framework (Debug).run.xml",
		".run/Build iOS XCFramework (Debug).run.xml",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			t.Errorf("%s was written for an Android-only project", rel)
		}
	}
}

// .run is a shared JetBrains directory: unlike .idea it belongs in version
// control, so the generated .gitignore must leave it alone.
func TestRunDirectoryIsNotGitignored(t *testing.T) {
	root := generate(t, testSpec("tunesic"))
	for _, line := range strings.Split(mustRead(t, root, ".gitignore"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), ".run") {
			t.Errorf(".gitignore excludes the run configurations: %q", line)
		}
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

// ---------------------------------------------------------------------------
// The modular iOS layout
// ---------------------------------------------------------------------------

func iosFeaturesSpec(dir string) model.Spec {
	spec := testSpec(dir)
	spec.IOSLayout = IOSFeaturesLayout
	spec.RootTabs = []string{"Home", "Library"}
	catalog.Normalise(&spec)
	return spec
}

func TestIOSFeaturesLayoutGeneratesThePackage(t *testing.T) {
	root := generate(t, iosFeaturesSpec("tunesic"))

	for _, rel := range []string{
		"iosApp/Packages/Features/Package.swift",
		"iosApp/Packages/Features/Sources/CoreNavigation/Resolve.swift",
		"iosApp/Packages/Features/Sources/CoreNavigation/HomeRoute.swift",
		"iosApp/Packages/Features/Sources/CoreNavigation/LibraryRoute.swift",
		"iosApp/Packages/Features/Sources/Home/HomeScreen.swift",
		"iosApp/Packages/Features/Sources/Home/HomeDestination.swift",
		"iosApp/Packages/Features/Sources/Library/LibraryScreen.swift",
		"iosApp/iosApp/App/iOSApp.swift",
		"iosApp/iosApp/App/AppCoordinator.swift",
		"iosApp/build-framework.sh",
		"iosApp/README.md",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}

	// The old single-file layout's entry point must not linger.
	if _, err := os.Stat(filepath.Join(root, "iosApp/iosApp/ContentView.swift")); err == nil {
		t.Error("ContentView.swift belongs to the single-entry-point layout")
	}
}

func TestIOSFeaturesPackageDeclaresEveryTarget(t *testing.T) {
	root := generate(t, iosFeaturesSpec("tunesic"))
	pkg := mustRead(t, root, "iosApp/Packages/Features/Package.swift")

	for _, want := range []string{
		`.library(name: "Home", targets: ["Home"])`,
		`.library(name: "Library", targets: ["Library"])`,
		`.target(name: "Home", dependencies: featureDependencies)`,
		`.target(name: "Library", dependencies: featureDependencies)`,
		`.binaryTarget(`,
		`path: "../../Frameworks/SharedLogic.xcframework"`,
		`.library(name: "AppFeatures", targets: [`,
	} {
		if !strings.Contains(pkg, want) {
			t.Errorf("Package.swift is missing %q", want)
		}
	}

	// Only the umbrella product is linked by Xcode, so a new feature never has
	// to touch project.pbxproj - which Xcode rewrites on save.
	pbx := mustRead(t, root, "iosApp/iosApp.xcodeproj/project.pbxproj")
	if !strings.Contains(pbx, "productName = AppFeatures") {
		t.Error("the Xcode project does not link the AppFeatures product")
	}
	if strings.Contains(pbx, "productName = Home") {
		t.Error("the Xcode project links an individual feature, which add-feature cannot maintain")
	}
}

func TestIOSFeaturesCoordinatorCoversEveryTab(t *testing.T) {
	root := generate(t, iosFeaturesSpec("tunesic"))
	coordinator := mustRead(t, root, "iosApp/iosApp/App/AppCoordinator.swift")

	for _, want := range []string{
		"import Home", "import Library", "import CoreNavigation",
		"case home", "case library",
		"@State private var homePath = NavigationPath()",
		"@State private var libraryPath = NavigationPath()",
		".homeDestination(for: HomeRoute())",
		".libraryDestination(for: LibraryRoute())",
		`.tabItem { Label("Home", systemImage: "house") }`,
	} {
		if !strings.Contains(coordinator, want) {
			t.Errorf("AppCoordinator.swift is missing %q", want)
		}
	}

	// Each tab gets its own path, which is what preserves per-tab history.
	if strings.Count(coordinator, "NavigationStack(path:") != 2 {
		t.Errorf("expected one NavigationStack per tab:\n%s", coordinator)
	}
}

func TestIOSFeaturesProjectHasEveryAnchor(t *testing.T) {
	root := generate(t, iosFeaturesSpec("tunesic"))

	cases := []struct{ rel, anchor string }{
		{"iosApp/Packages/Features/Package.swift", "kmp-scaffold:ios-products"},
		{"iosApp/Packages/Features/Package.swift", "kmp-scaffold:ios-app-features"},
		{"iosApp/Packages/Features/Package.swift", "kmp-scaffold:ios-targets"},
		{"iosApp/iosApp/App/AppCoordinator.swift", "kmp-scaffold:ios-tab-cases"},
		{"iosApp/iosApp/App/AppCoordinator.swift", "kmp-scaffold:ios-tab-paths"},
		{"iosApp/iosApp/App/AppCoordinator.swift", "kmp-scaffold:ios-tabs"},
		{"iosApp/iosApp/App/AppCoordinator.swift", "kmp-scaffold:ios-destinations"},
	}
	for _, c := range cases {
		if !strings.Contains(mustRead(t, root, c.rel), c.anchor) {
			t.Errorf("%s is missing the %s anchor", c.rel, c.anchor)
		}
	}
}

// The shared module has to publish an XCFramework for SPM to consume, but only
// for this layout - the single-entry-point one links the framework directly.
func TestXCFrameworkOnlyForTheModularLayout(t *testing.T) {
	modular := mustRead(t, generate(t, iosFeaturesSpec("tunesic")), "sharedLogic/build.gradle.kts")
	if !strings.Contains(modular, "XCFramework(\"SharedLogic\")") ||
		!strings.Contains(modular, "xcframework.add(this)") {
		t.Errorf("the modular layout needs an XCFramework:\n%s", modular)
	}

	simple := testSpec("tunesic")
	simple.IOSLayout = "swiftui-simple"
	catalog.Normalise(&simple)
	plain := mustRead(t, generate(t, simple), "sharedLogic/build.gradle.kts")
	if strings.Contains(plain, "XCFramework") {
		t.Error("the single-entry-point layout should not build an XCFramework")
	}
}

func TestAddIOSFeatureWiresPackageAndCoordinator(t *testing.T) {
	root := generate(t, iosFeaturesSpec("tunesic"))
	manifest, _, err := model.LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}

	writer := render.NewWriter(root, false, false)
	report, err := AddFeature(manifest, root, FeatureRequest{
		Name: "now-playing", Android: true, Shared: true, IOS: true, Presentation: "above-nav",
	}, "test", writer, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Warnings) > 0 {
		t.Errorf("unexpected warnings: %v", report.Warnings)
	}

	// A kebab-case name becomes a PascalCase Swift module.
	for _, rel := range []string{
		"iosApp/Packages/Features/Sources/CoreNavigation/NowPlayingRoute.swift",
		"iosApp/Packages/Features/Sources/NowPlaying/NowPlayingScreen.swift",
		"iosApp/Packages/Features/Sources/NowPlaying/NowPlayingDestination.swift",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}

	pkg := mustRead(t, root, "iosApp/Packages/Features/Package.swift")
	for _, want := range []string{
		`.library(name: "NowPlaying", targets: ["NowPlaying"]),`,
		`.target(name: "NowPlaying", dependencies: featureDependencies),`,
		`"NowPlaying",`,
	} {
		if !strings.Contains(pkg, want) {
			t.Errorf("Package.swift is missing %q", want)
		}
	}

	coordinator := mustRead(t, root, "iosApp/iosApp/App/AppCoordinator.swift")
	if !strings.Contains(coordinator, "import NowPlaying") {
		t.Error("the coordinator is missing the new module's import")
	}
	if !strings.Contains(coordinator, ".navigationDestination(for: NowPlayingRoute.self) { route in") {
		t.Errorf("the pushed destination was not registered:\n%s", coordinator)
	}
	// A pushed screen goes on the shared modifier, not into the tab list.
	if strings.Contains(coordinator, ".tag(AppTab.nowPlaying)") {
		t.Error("a pushed screen should not become a tab")
	}

	// A feature paired with shared logic drives its screen from the shared ViewModel.
	screen := mustRead(t, root, "iosApp/Packages/Features/Sources/NowPlaying/NowPlayingScreen.swift")
	if !strings.Contains(screen, "resolveShared(NowPlayingViewModel.self)") {
		t.Errorf("the screen does not resolve its shared ViewModel:\n%s", screen)
	}

	if f := manifest.FindFeature("now-playing"); f == nil || !f.IOS {
		t.Errorf("the manifest does not record the iOS half: %+v", f)
	}
}

func TestAddIOSRootTabJoinsTheTabView(t *testing.T) {
	root := generate(t, iosFeaturesSpec("tunesic"))
	manifest, _, err := model.LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}

	writer := render.NewWriter(root, false, false)
	if _, err := AddFeature(manifest, root, FeatureRequest{
		Name: "explore", Android: true, IOS: true, Presentation: "shell", RootTab: true,
	}, "test", writer, false); err != nil {
		t.Fatal(err)
	}

	coordinator := mustRead(t, root, "iosApp/iosApp/App/AppCoordinator.swift")
	for _, want := range []string{
		"case explore",
		"@State private var explorePath = NavigationPath()",
		"NavigationStack(path: $explorePath) {",
		".exploreDestination(for: ExploreRoute())",
		`.tabItem { Label("Explore", systemImage: "safari") }`,
		".tag(AppTab.explore)",
	} {
		if !strings.Contains(coordinator, want) {
			t.Errorf("AppCoordinator.swift is missing %q:\n%s", want, coordinator)
		}
	}

	// The inserted block keeps its nesting.
	if !strings.Contains(coordinator, "\n            NavigationStack(path: $explorePath) {\n                EmptyView()\n") {
		t.Errorf("the inserted tab lost its shape:\n%s", coordinator)
	}
}

func TestAddIOSFeatureIsRejectedOnTheSimpleLayout(t *testing.T) {
	spec := testSpec("tunesic")
	spec.IOSLayout = "swiftui-simple"
	catalog.Normalise(&spec)
	root := generate(t, spec)

	manifest, _, err := model.LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}

	writer := render.NewWriter(root, false, false)
	_, err = AddFeature(manifest, root, FeatureRequest{
		Name: "billing", IOS: true, Presentation: "above-nav",
	}, "test", writer, false)
	if err == nil {
		t.Fatal("expected an error: the single-entry-point layout has no feature targets")
	}
	if !strings.Contains(err.Error(), "single entry point") {
		t.Errorf("the error should explain why: %v", err)
	}

	if SupportsFeatures(KindIOS, "swiftui-simple") {
		t.Error("SupportsFeatures should be false for the single-entry-point layout")
	}
	if !SupportsFeatures(KindIOS, IOSFeaturesLayout) {
		t.Error("SupportsFeatures should be true for the modular layout")
	}
}
