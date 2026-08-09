package kmp

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
	"github.com/aaroncutress/kmp-scaffold/internal/generator"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/render"
	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
	"github.com/aaroncutress/kmp-scaffold/internal/wire"
)

// Retrofit recipes turn on something the project was generated without.
//
// A project generated with --no-tests is not a project that can never have
// tests; it is one that does not have them yet, and six months later that is
// often the wrong answer. Each recipe here writes the files that option owns,
// wires its dependencies into the build scripts that already exist, and records
// the new answer in the manifest so `add feature` and the next `versions` run
// agree with it.
//
// They are singletons: there is one test setup in a project, not one per name.

// retrofitRecipes are the options that can be turned on after the fact.
func retrofitRecipes() []scaffold.Recipe {
	return []scaffold.Recipe{
		{
			Name:        "tests",
			Noun:        "tests",
			Label:       "Test source sets",
			Description: "Test source sets, the dependencies they need, and one worked example per platform.",
			Singleton:   true,
			AppliedTo:   func(m *model.Manifest) bool { return varsBool(m, "tests") },
			Summary:     retrofitSummary("tests", testsSummaryItems),
			Apply:       applyTests,
		},
		{
			Name:        "ci",
			Noun:        "a CI workflow",
			Label:       "CI workflow",
			Description: "A GitHub Actions workflow building this project on every pull request.",
			Singleton:   true,
			AppliedTo:   func(m *model.Manifest) bool { return varsBool(m, "ci") },
			Summary:     retrofitSummary("a CI workflow", ciSummaryItems),
			Apply:       applyCI,
		},
		{
			Name:        "editorconfig",
			Noun:        "editor configuration",
			Label:       "Editor configuration",
			Description: "The .editorconfig and .gitattributes a project generated before they existed has not got.",
			Singleton:   true,
			Summary:     retrofitSummary("editor configuration", editorSummaryItems),
			Apply:       applyEditorConfig,
		},
	}
}

// varsBool reads a boolean out of the manifest's vars blob.
//
// It is deliberately not typed through Vars: a project generated before the key
// existed has no entry, and that has to read as false rather than as an error.
func varsBool(m *model.Manifest, key string) bool {
	if m == nil {
		return false
	}
	var v map[string]any
	if err := m.DecodeVars(&v); err != nil {
		return false
	}
	b, _ := v[key].(bool)
	return b
}

func retrofitSummary(noun string, items func(model.Spec) []string) func(*model.Manifest, *scaffold.Answers) []scaffold.Section {
	return func(m *model.Manifest, _ *scaffold.Answers) []scaffold.Section {
		spec, _, err := SpecFrom(m)
		if err != nil {
			return nil
		}
		return []scaffold.Section{{
			Title: "Adding " + noun,
			Items: items(spec),
		}}
	}
}

func testsSummaryItems(spec model.Spec) []string {
	items := []string{"sharedLogic/src/commonTest/ - one test that runs on every target"}
	if spec.HasSharedUtil("settings") {
		items = append(items, "a test for the settings ViewModel")
	}
	if spec.Android {
		items = append(items, "androidApp/src/test/ - a JVM unit test")
	}
	if spec.IOSLayout == generator.IOSFeaturesLayout {
		items = append(items, "iosApp/Packages/Features/Tests/ - a Swift test target")
	}
	return append(items, "the testing entries in gradle/libs.versions.toml")
}

func ciSummaryItems(spec model.Spec) []string {
	items := []string{".github/workflows/ci.yml"}
	if spec.Android {
		items = append(items, "an Ubuntu job assembling the Android app")
	}
	if spec.IOS {
		items = append(items, "a macOS job building the iOS app")
	}
	if spec.Tests {
		items = append(items, "a step running the shared tests")
	}
	return items
}

func editorSummaryItems(model.Spec) []string {
	return []string{".editorconfig", ".gitattributes"}
}

// ---------------------------------------------------------------------------
// Applying
// ---------------------------------------------------------------------------

// retrofit is the shape all three share: read the project, flip one answer,
// render what that answer owns, wire in what the existing files need, save.
type retrofit struct {
	// set flips the answer on the spec before anything is rendered, so the
	// templates see the project as it is about to be rather than as it was.
	set func(*model.Spec)
	// files are rendered against the updated spec.
	files func(model.Spec) []retrofitFile
	// edits are applied to files that already exist.
	edits func(model.Spec) []wire.Edit
	// refresh names files the recipe does not own but does affect - a CI
	// workflow that should now run the tests being added.
	//
	// They are only rewritten when what is on disk is exactly what this tool
	// last wrote. A file the user has touched is theirs, and gets the same
	// treatment as any other collision: left alone, with the new version
	// beside it.
	refresh func(model.Spec) []retrofitFile
	// catalogKeys are version keys whose entries have to join
	// libs.versions.toml, resolved live so they are current rather than
	// whatever the project was generated with.
	catalogKeys func(model.Spec) bool
}

type retrofitFile struct {
	tpl, path string
}

func applyTests(ctx context.Context, req scaffold.RecipeRequest) (*scaffold.Report, error) {
	return runRetrofit(ctx, req, retrofit{
		set:         func(s *model.Spec) { s.Tests = true },
		files:       testFiles,
		edits:       testEdits,
		catalogKeys: func(model.Spec) bool { return true },
		// The workflow decides whether to run the tests by looking at whether
		// there are any. Adding tests without refreshing it leaves CI quietly
		// not running the thing that was just added.
		refresh: func(spec model.Spec) []retrofitFile {
			if !spec.CI {
				return nil
			}
			return []retrofitFile{{"ci/workflow.yml", ".github/workflows/ci.yml"}}
		},
	})
}

func applyCI(ctx context.Context, req scaffold.RecipeRequest) (*scaffold.Report, error) {
	return runRetrofit(ctx, req, retrofit{
		set: func(s *model.Spec) { s.CI = true },
		files: func(model.Spec) []retrofitFile {
			return []retrofitFile{{"ci/workflow.yml", ".github/workflows/ci.yml"}}
		},
	})
}

func applyEditorConfig(ctx context.Context, req scaffold.RecipeRequest) (*scaffold.Report, error) {
	return runRetrofit(ctx, req, retrofit{
		files: func(model.Spec) []retrofitFile {
			return []retrofitFile{
				{"root/editorconfig", ".editorconfig"},
				{"root/gitattributes", ".gitattributes"},
			}
		},
	})
}

// testFiles is what the tests option owns, filtered to what this project can
// hold: no Android unit test without an Android app, no Swift tests without the
// modular iOS layout.
func testFiles(spec model.Spec) []retrofitFile {
	pkg := spec.PackagePath()
	files := []retrofitFile{
		{"shared/PlatformTest.kt",
			path.Join("sharedLogic/src/commonTest/kotlin", pkg, "core/util/PlatformTest.kt")},
	}
	if spec.HasSharedUtil("settings") {
		files = append(files, retrofitFile{"shared/SettingsViewModelTest.kt",
			path.Join("sharedLogic/src/commonTest/kotlin", pkg,
				"feature/settings/presentation/SettingsViewModelTest.kt")})
	}
	if spec.Android {
		files = append(files, retrofitFile{"android/ExampleUnitTest.kt",
			path.Join("androidApp/src/test/kotlin", pkg, "ExampleUnitTest.kt")})
	}
	if spec.IOSLayout == generator.IOSFeaturesLayout {
		files = append(files, retrofitFile{"iosfeatures/RouteTests.swift",
			"iosApp/Packages/Features/Tests/FeatureTests/RouteTests.swift"})
	}
	return files
}

// testEdits are the dependency blocks the existing build scripts need. The
// anchors they insert before are written whatever the tests answer was, which
// is what makes this possible at all.
func testEdits(spec model.Spec) []wire.Edit {
	var edits []wire.Edit

	shared := []string{
		"commonTest.dependencies {",
		"    implementation(libs.kotlin.test)",
		"    implementation(libs.koin.test)",
		"    implementation(libs.kotlinx.coroutines.test)",
		"    implementation(libs.turbine)",
	}
	if spec.HasSharedUtil("ktor-engine") {
		shared = append(shared, "    implementation(libs.ktor.client.mock)")
	}
	shared = append(shared, "}", "")
	edits = append(edits, wire.Edit{
		Path:   "sharedLogic/build.gradle.kts",
		Anchor: wire.AnchorSharedTestDeps,
		Lines:  shared,
		Key:    "commonTest.dependencies",
	})

	if spec.Android {
		edits = append(edits, wire.Edit{
			Path:   "androidApp/build.gradle.kts",
			Anchor: wire.AnchorTestDeps,
			Lines: []string{
				"testImplementation(libs.junit)",
				"androidTestImplementation(libs.androidx.testExt.junit)",
				"androidTestImplementation(libs.androidx.espresso.core)",
				"",
			},
			Key: "testImplementation(libs.junit)",
		})
	}

	if spec.IOSLayout == generator.IOSFeaturesLayout {
		edits = append(edits, wire.Edit{
			Path:   "iosApp/Packages/Features/Package.swift",
			Anchor: wire.AnchorIOSTestTarget,
			// The comment goes in too, so a retrofitted Package.swift reads the
			// same as a generated one - it explains a decision that is not
			// obvious from the line below it.
			Lines: []string{
				"",
				"// Tests for the package, run from Xcode's Test navigator.",
				"//",
				"// It depends on CoreNavigation alone, which is where the routes live -",
				"// so adding a feature never changes this target. To test a feature's",
				"// own views, add it to this list yourself.",
				`.testTarget(name: "FeatureTests", dependencies: ["CoreNavigation"]),`,
			},
			Key: "FeatureTests",
		})
	}

	return edits
}

// runRetrofit is the common body. Everything it does is idempotent: rendering
// skips a file whose content already matches, wire skips a block already there,
// and the catalog pass adds only keys that are missing.
func runRetrofit(ctx context.Context, req scaffold.RecipeRequest, r retrofit) (*scaffold.Report, error) {
	before, _, err := SpecFrom(req.Manifest)
	if err != nil {
		return nil, err
	}
	spec := before
	if r.set != nil {
		r.set(&spec)
	}

	// Versions are resolved rather than read from the catalog, because the
	// entries about to be added should be current - the project's other
	// versions are whenever it was generated, and this is a new decision.
	var result *resolve.Result
	if r.catalogKeys != nil && r.catalogKeys(spec) {
		result = resolve.Run(ctx, RequestFor(spec))
	}

	env, err := generator.NewEnv(spec, result, req.Version, req.Writer)
	if err != nil {
		return nil, err
	}

	report := &scaffold.Report{}
	if r.files != nil {
		for _, f := range r.files(spec) {
			if err := env.Render(f.tpl, f.path); err != nil {
				return nil, err
			}
		}
	}

	// Files this recipe affects but does not own, updated only when untouched.
	if r.refresh != nil {
		for _, f := range r.refresh(spec) {
			if err := refreshFile(env, before, f, req.Root, req.Writer); err != nil {
				return nil, err
			}
		}
	}

	if r.edits != nil {
		applier := wire.NewApplier(req.Root, req.DryRun)
		for _, e := range r.edits(spec) {
			if err := applier.Apply(e); err != nil {
				return nil, err
			}
		}
		report.Wire = applier.Results()
	}

	if result != nil {
		added, err := addCatalogEntries(req.Root, spec, result, req.DryRun)
		if err != nil {
			return nil, err
		}
		if len(added) > 0 {
			report.Notes = append(report.Notes,
				fmt.Sprintf("added %d entr%s to gradle/libs.versions.toml: %s",
					len(added), plural(len(added), "y", "ies"), strings.Join(added, ", ")))
		}
	}

	if req.DryRun {
		return report, nil
	}

	// The manifest is what `add feature` and the next retrofit read, so the new
	// answer has to be recorded even when every file was already there.
	if err := req.Manifest.SetVars(VarsOf(spec)); err != nil {
		return nil, err
	}
	req.Manifest.Features = append(req.Manifest.Features, model.Feature{
		Name: req.Recipe, Recipe: req.Recipe,
	})
	if err := req.Manifest.Save(req.Root); err != nil {
		return nil, err
	}
	return report, nil
}

// refreshFile rewrites a file whose content depends on the answer being
// changed, but only when what is on disk is what this tool last wrote.
//
// "What we last wrote" is knowable: render the same template against the spec
// as it was before this recipe touched it. If that matches the file, nobody has
// edited it and updating it is safe. If it does not, the file is the user's and
// the ordinary collision handling applies - which for a recipe means the new
// version lands beside it as .new.
func refreshFile(env *generator.Env, before model.Spec, f retrofitFile, root string, w *render.Writer) error {
	current, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f.path)))
	if err != nil {
		// Not there at all, so there is nothing to preserve and nothing this
		// recipe promised to write either.
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	previous, err := env.Engine.Render(f.tpl, generator.NewCtx(before, env.Ctx.Res, env.Ctx.Gen))
	if err != nil {
		return err
	}

	if string(current) == string(previous) {
		// Ours, unmodified. Overwriting is an update, not a clobber.
		force := w.Force
		w.Force = true
		defer func() { w.Force = force }()
	}
	return env.Render(f.tpl, f.path)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// addCatalogEntries adds the version keys and libraries the updated spec needs
// and the catalog has not got, leaving everything already there alone.
func addCatalogEntries(root string, spec model.Spec, res *resolve.Result, dryRun bool) ([]string, error) {
	existing, err := readCatalog(root)
	if err != nil {
		return nil, err
	}

	var versionLines, libraryLines, added []string
	usedKeys := map[string]bool{}
	for _, lib := range catalog.Libraries() {
		if !lib.When(spec) || existing[lib.Alias] {
			continue
		}
		added = append(added, lib.Alias)
		if lib.Version != "" {
			usedKeys[lib.Version] = true
			libraryLines = append(libraryLines,
				fmt.Sprintf("%s = { module = %q, version.ref = %q }", lib.Alias, lib.Module, lib.Version))
		} else {
			libraryLines = append(libraryLines,
				fmt.Sprintf("%s = { module = %q }", lib.Alias, lib.Module))
		}
	}
	// Catalog order, not library order, so the block reads like the rest of it.
	for _, def := range catalog.VersionKeys() {
		if usedKeys[def.Key] && !existing[def.Key] {
			versionLines = append(versionLines, fmt.Sprintf("%s = %q", def.Key, res.V(def.Key)))
		}
	}

	if len(added) == 0 || dryRun {
		return added, nil
	}
	return added, AppendToCatalog(root, versionLines, libraryLines)
}
