package kmp_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
	"github.com/aaroncutress/kmp-scaffold/internal/generator"
	"github.com/aaroncutress/kmp-scaffold/internal/kmp"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
)

// ---------------------------------------------------------------------------
// Tests as an option
// ---------------------------------------------------------------------------

// testFiles are every path the tests option is responsible for.
var testFiles = []string{
	"sharedLogic/src/commonTest/kotlin/io/kontour/tunesic/core/util/PlatformTest.kt",
	"sharedLogic/src/commonTest/kotlin/io/kontour/tunesic/feature/settings/presentation/SettingsViewModelTest.kt",
	"androidApp/src/test/kotlin/io/kontour/tunesic/ExampleUnitTest.kt",
	"iosApp/Packages/Features/Tests/FeatureTests/RouteTests.swift",
}

func TestTestsOptionWritesEveryLayer(t *testing.T) {
	spec := testSpec("tunesic")
	spec.Tests = true
	catalog.Normalise(&spec)
	root := generate(t, spec)

	for _, rel := range testFiles {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}
	if !strings.Contains(mustRead(t, root, "iosApp/Packages/Features/Package.swift"), ".testTarget(") {
		t.Error("the Swift test sources exist but no target builds them")
	}
}

// Turning tests off has to take the dependencies with it. A declared
// dependency with no test source set to use it is the state this option exists
// to fix, so leaving one behind would make the option a half-measure.
func TestNoTestsLeavesNothingBehind(t *testing.T) {
	spec := testSpec("tunesic")
	spec.Tests = false
	catalog.Normalise(&spec)
	root := generate(t, spec)

	for _, rel := range testFiles {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			t.Errorf("%s was written for a project that asked for no tests", rel)
		}
	}

	catalogText := mustRead(t, root, "gradle/libs.versions.toml")
	for _, alias := range []string{"junit", "turbine", "espresso", "kotlin-test", "koin-test", "testExt", "ktor-client-mock"} {
		if strings.Contains(catalogText, alias) {
			t.Errorf("libs.versions.toml still declares %q with nothing to use it", alias)
		}
	}

	for _, rel := range []string{"androidApp/build.gradle.kts", "sharedLogic/build.gradle.kts"} {
		body := mustRead(t, root, rel)
		for _, marker := range []string{"testImplementation", "androidTestImplementation", "commonTest"} {
			if strings.Contains(body, marker) {
				t.Errorf("%s still has a %s block", rel, marker)
			}
		}
	}

	if strings.Contains(mustRead(t, root, "iosApp/Packages/Features/Package.swift"), "testTarget") {
		t.Error("Package.swift declares a test target with no sources")
	}
}

// The one test that has to survive every other option being switched off,
// because otherwise `allTests` runs nothing and looks like it passed.
func TestASharedTestSurvivesAMinimalProject(t *testing.T) {
	spec := testSpec("tunesic")
	spec.Packs = nil
	spec.SharedUtils = nil
	spec.AndroidExtras = nil
	catalog.Normalise(&spec)
	root := generate(t, spec)

	body := mustRead(t, root, "sharedLogic/src/commonTest/kotlin/io/kontour/tunesic/core/util/PlatformTest.kt")
	if !strings.Contains(body, "currentPlatform()") {
		t.Errorf("the platform test does not test the platform: %q", body)
	}
	// The settings test depends on a utility this project does not have.
	settings := "sharedLogic/src/commonTest/kotlin/io/kontour/tunesic/feature/settings/presentation/SettingsViewModelTest.kt"
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(settings))); err == nil {
		t.Error("the settings test was written without the settings utility")
	}
}

// ---------------------------------------------------------------------------
// CI as an option
// ---------------------------------------------------------------------------

const workflow = ".github/workflows/ci.yml"

func TestCIOptionWritesAWorkflow(t *testing.T) {
	spec := testSpec("tunesic")
	catalog.Normalise(&spec)
	body := mustRead(t, generate(t, spec), workflow)

	// Triggers, which is the part most easily got wrong: a workflow on every
	// push burns minutes on work in progress.
	for _, want := range []string{"pull_request:", "workflow_dispatch:", "cancel-in-progress: true"} {
		if !strings.Contains(body, want) {
			t.Errorf("the workflow is missing %q", want)
		}
	}
	// Both halves of the project get built.
	for _, want := range []string{"runs-on: ubuntu-latest", "runs-on: macos-latest", "xcodebuild build"} {
		if !strings.Contains(body, want) {
			t.Errorf("the workflow is missing %q", want)
		}
	}
	// The Java version is the project's, not a hard-coded one.
	if !strings.Contains(body, "java-version: '"+spec.JVMTarget+"'") {
		t.Errorf("the workflow does not set up Java %s", spec.JVMTarget)
	}
}

func TestNoCIWritesNoWorkflow(t *testing.T) {
	spec := testSpec("tunesic")
	spec.CI = false
	catalog.Normalise(&spec)
	if _, err := os.Stat(filepath.Join(generate(t, spec), filepath.FromSlash(workflow))); err == nil {
		t.Error("a workflow was written for a project that asked for none")
	}
}

// An Android-only project has no iOS job to run, and vice versa.
func TestTheWorkflowMatchesTheProjectShape(t *testing.T) {
	android := testSpec("tunesic")
	android.IOS = false
	catalog.Normalise(&android)
	if body := mustRead(t, generate(t, android), workflow); strings.Contains(body, "macos-latest") {
		t.Error("an Android-only project got a macOS job")
	}

	noTests := testSpec("tunesic")
	noTests.Tests = false
	catalog.Normalise(&noTests)
	if body := mustRead(t, generate(t, noTests), workflow); strings.Contains(body, "allTests") {
		t.Error("a project with no tests got a test step")
	}
}

// ---------------------------------------------------------------------------
// The toolchain
// ---------------------------------------------------------------------------

// swiftToolsPlatforms is the first swift-tools-version that knows each iOS
// platform case, from swiftlang/swift-package-manager's SupportedPlatforms.swift.
// A manifest naming a case its declared tools version does not have fails to
// parse - which is exactly what shipped with `.v18` under tools 5.9.
var swiftToolsPlatforms = map[int]string{
	17: "5.7",
	18: "6.0",
	19: "6.2",
	26: "6.2",
}

func TestPackageSwiftPlatformIsLegalForItsToolsVersion(t *testing.T) {
	for _, target := range []string{"17.0", "18.0", "26.0"} {
		t.Run(target, func(t *testing.T) {
			spec := testSpec("tunesic")
			spec.IOSLayout = generator.IOSFeaturesLayout
			spec.IOSDeployTgt = target
			catalog.Normalise(&spec)

			body := mustRead(t, generate(t, spec), "iosApp/Packages/Features/Package.swift")

			tools := findOne(t, body, `swift-tools-version:\s*([0-9]+\.[0-9]+)`)
			platform := findOne(t, body, `platforms:\s*\[\.iOS\(\.v([0-9]+)\)\]`)

			major, err := strconv.Atoi(platform)
			if err != nil {
				t.Fatalf("platform case .v%s is not a version", platform)
			}
			need, known := swiftToolsPlatforms[major]
			if !known {
				t.Fatalf("no recorded tools version introduces .v%d - add it to swiftToolsPlatforms", major)
			}
			if resolve.ParseVersion(tools).Less(resolve.ParseVersion(need)) {
				t.Errorf("Package.swift declares swift-tools-version %s but uses .v%d, "+
					"which needs %s or newer - the manifest will not parse", tools, major, need)
			}
		})
	}
}

// The language mode is one decision written in two files. They cannot disagree:
// a package compiled in one mode and an app target in another is a build that
// fails halfway through for reasons neither file explains.
func TestSwiftModeIsConsistent(t *testing.T) {
	for _, mode := range []string{"5", "6"} {
		t.Run("swift"+mode, func(t *testing.T) {
			spec := testSpec("tunesic")
			spec.IOSLayout = generator.IOSFeaturesLayout
			spec.SwiftMode = mode
			catalog.Normalise(&spec)
			root := generate(t, spec)

			pkg := mustRead(t, root, "iosApp/Packages/Features/Package.swift")
			if want := "swiftLanguageModes: [.v" + mode + "]"; !strings.Contains(pkg, want) {
				t.Errorf("Package.swift does not declare %s", want)
			}

			pbxproj := mustRead(t, root, "iosApp/iosApp.xcodeproj/project.pbxproj")
			if want := "SWIFT_VERSION = " + mode + ".0;"; !strings.Contains(pbxproj, want) {
				t.Errorf("the Xcode project does not set %s", want)
			}
			if other := map[string]string{"5": "6", "6": "5"}[mode]; strings.Contains(
				pbxproj, "SWIFT_VERSION = "+other+".0;") {
				t.Errorf("the Xcode project sets both Swift %s and Swift %s", mode, other)
			}
		})
	}
}

func TestJVMTargetReachesEveryBuildScript(t *testing.T) {
	spec := testSpec("tunesic")
	spec.JVMTarget = "21"
	catalog.Normalise(&spec)
	root := generate(t, spec)

	for _, rel := range []string{
		"androidApp/build.gradle.kts",
		"sharedLogic/build.gradle.kts",
		"buildSrc/src/main/kotlin/tunesic.android.library.gradle.kts",
	} {
		body := mustRead(t, root, rel)
		if !strings.Contains(body, "_21") {
			t.Errorf("%s does not compile to Java 21", rel)
		}
		if strings.Contains(body, "_17") || strings.Contains(body, "_11") {
			t.Errorf("%s still names a Java version the project did not ask for", rel)
		}
	}
}

// ---------------------------------------------------------------------------
// The manifest round trip
// ---------------------------------------------------------------------------

// `add` rebuilds a spec from the manifest. Anything the manifest does not
// record is a value `add` has to invent - and it used to invent Java 11 and an
// iOS deployment target of "", whatever the project was really generated with.
func TestToolchainAnswersSurviveTheManifest(t *testing.T) {
	spec := testSpec("tunesic")
	spec.Tests = false
	spec.CI = false
	spec.JVMTarget = "21"
	spec.IOSDeployTgt = "26.0"
	spec.SwiftMode = "6"

	manifest := &model.Manifest{
		Schema:   model.ManifestSchema,
		Project:  model.Project{Name: spec.Name, Package: spec.Package, ApplicationID: spec.ApplicationID},
		Template: model.TemplateRef{ID: kmp.ID},
	}
	if err := manifest.SetVars(kmp.VarsOf(spec)); err != nil {
		t.Fatal(err)
	}

	got, _, err := kmp.SpecFrom(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, want, got string }{
		{"jvmTarget", spec.JVMTarget, got.JVMTarget},
		{"iosDeployTgt", spec.IOSDeployTgt, got.IOSDeployTgt},
		{"swiftMode", spec.SwiftMode, got.SwiftMode},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q after a round trip, want %q", c.name, c.got, c.want)
		}
	}
	if got.Tests || got.CI {
		t.Errorf("tests=%v ci=%v, want both false as generated", got.Tests, got.CI)
	}
}

// A project generated before these were recorded has no answer for them. The
// defaults are what it was built with, so that is what a rebuilt spec gets -
// not the zero value, which would render as a Swift platform of ".v".
func TestAnOlderManifestFallsBackToTheDefaults(t *testing.T) {
	manifest := &model.Manifest{
		Schema:   model.ManifestSchema,
		Project:  model.Project{Name: "Tunesic", Package: "io.kontour.tunesic"},
		Template: model.TemplateRef{ID: kmp.ID},
	}
	// Exactly what an older build wrote: the structural keys and nothing else.
	if err := manifest.SetVars(map[string]any{
		"android": true, "ios": true,
		"androidLayout": "nav3-shell", "iosLayout": generator.IOSFeaturesLayout,
	}); err != nil {
		t.Fatal(err)
	}

	got, _, err := kmp.SpecFrom(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defaults := model.Defaults()
	if got.JVMTarget != defaults.JVMTarget {
		t.Errorf("jvmTarget = %q, want the default %q", got.JVMTarget, defaults.JVMTarget)
	}
	if got.IOSDeployTgt != defaults.IOSDeployTgt {
		t.Errorf("iosDeployTgt = %q, want the default %q", got.IOSDeployTgt, defaults.IOSDeployTgt)
	}
	if got.SwiftMode != defaults.SwiftMode {
		t.Errorf("swiftMode = %q, want the default %q", got.SwiftMode, defaults.SwiftMode)
	}
}

func findOne(t *testing.T, body, pattern string) string {
	t.Helper()
	m := regexp.MustCompile(pattern).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("nothing in the file matches %s", pattern)
	}
	return m[1]
}
