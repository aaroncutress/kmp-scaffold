package kmp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
	"github.com/aaroncutress/kmp-scaffold/internal/kmp"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/render"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
)

// retrofit runs `add <name>` the way the CLI does for a singleton recipe: no
// name, no questions, applied to a project that already exists.
func retrofit(t *testing.T, root, recipeName string, force bool) *scaffold.Report {
	t.Helper()
	manifest, _, err := model.LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	recipe, ok := scaffold.FindRecipe(tmpl, recipeName)
	if !ok {
		t.Fatalf("the kmp template has no %q recipe", recipeName)
	}
	if !recipe.Singleton {
		t.Fatalf("%q should be a singleton - it adds one thing, not a named one", recipeName)
	}

	a := scaffold.NewAnswers()
	a.Project = manifest.Project
	writer := render.NewWriter(root, false, force)
	writer.Sidecars = true

	report, err := recipe.Apply(context.Background(), scaffold.RecipeRequest{
		Recipe:   recipe.Name,
		Manifest: manifest,
		Root:     root,
		Name:     recipe.Name,
		Answers:  a,
		Writer:   writer,
		Version:  "test",
	})
	if err != nil {
		t.Fatalf("applying %s: %v", recipeName, err)
	}
	report.Writer = writer
	return report
}

// specWithout is a project generated with one of the options switched off.
func specWithout(name string, off func(*model.Spec)) model.Spec {
	spec := testSpec(name)
	off(&spec)
	catalog.Normalise(&spec)
	return spec
}

// The point of the whole feature: a project generated without tests, then given
// tests, should be the project it would have been if it had asked for them.
//
// Comparing whole trees rather than spot-checking is what makes this worth
// having - it catches a file the recipe forgot as readily as one it got wrong.
func TestRetrofittingTestsMatchesGeneratingThem(t *testing.T) {
	retro := generate(t, specWithout("tunesic", func(s *model.Spec) { s.Tests = false }))
	retrofit(t, retro, "tests", false)

	native := generate(t, testSpec("tunesic"))

	compareTrees(t, retro, native, map[string]string{
		// Appended in a marked block rather than woven into the sections, which
		// is honest about what was added later. Contents are compared below.
		"gradle/libs.versions.toml": "catalog entries are appended, not interleaved",
		// wire strips a block's leading blank line, so the inserted target sits
		// one line closer to the anchor than the generated one does.
		"iosApp/Packages/Features/Package.swift": "one blank line",
	})

	// The catalog has to end up declaring the same set of keys, wherever they sit.
	if got, want := catalogKeys(t, retro), catalogKeys(t, native); !sameSet(got, want) {
		t.Errorf("the retrofitted catalog declares a different set of keys:\n got %v\nwant %v", got, want)
	}
}

func TestRetrofittingCIMatchesGeneratingIt(t *testing.T) {
	retro := generate(t, specWithout("tunesic", func(s *model.Spec) { s.CI = false }))
	retrofit(t, retro, "ci", false)

	native := generate(t, testSpec("tunesic"))
	compareTrees(t, retro, native, nil)
}

// A project generated before .editorconfig existed has neither file. The recipe
// exists for exactly that project, so it has to work on one.
func TestRetrofittingEditorConfig(t *testing.T) {
	root := generate(t, testSpec("tunesic"))
	for _, rel := range []string{".editorconfig", ".gitattributes"} {
		if err := os.Remove(filepath.Join(root, rel)); err != nil {
			t.Fatal(err)
		}
	}

	retrofit(t, root, "editorconfig", false)
	for _, rel := range []string{".editorconfig", ".gitattributes"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("%s was not restored: %v", rel, err)
		}
	}
}

// Applying twice must not double anything: the files are already right, the
// wire blocks are already there, and the catalog already declares the keys.
func TestRetrofittingIsIdempotent(t *testing.T) {
	root := generate(t, specWithout("tunesic", func(s *model.Spec) { s.Tests = false }))
	retrofit(t, root, "tests", false)

	before := mustRead(t, root, "sharedLogic/build.gradle.kts")
	catalogBefore := mustRead(t, root, "gradle/libs.versions.toml")

	report := retrofit(t, root, "tests", true)

	if got := mustRead(t, root, "sharedLogic/build.gradle.kts"); got != before {
		t.Error("a second run changed sharedLogic/build.gradle.kts")
	}
	if got := mustRead(t, root, "gradle/libs.versions.toml"); got != catalogBefore {
		t.Error("a second run changed the version catalog")
	}
	if n := strings.Count(before, "commonTest.dependencies"); n != 1 {
		t.Errorf("commonTest.dependencies appears %d times, want 1", n)
	}
	if len(report.Notes) != 0 {
		t.Errorf("a second run reported adding something: %v", report.Notes)
	}
}

// The manifest is what the next `add` reads. A retrofit that writes the files
// but does not record the answer leaves the project disagreeing with itself.
func TestRetrofitRecordsTheAnswer(t *testing.T) {
	root := generate(t, specWithout("tunesic", func(s *model.Spec) {
		s.Tests = false
		s.CI = false
	}))

	before, _, err := model.LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	spec, _, err := kmpSpecFrom(before)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Tests {
		t.Fatal("the project was generated without tests")
	}

	retrofit(t, root, "tests", false)

	after, _, err := model.LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	spec, _, err = kmpSpecFrom(after)
	if err != nil {
		t.Fatal(err)
	}
	if !spec.Tests {
		t.Error("the manifest still says this project has no tests")
	}
	if spec.CI {
		t.Error("adding tests turned CI on as well")
	}

	recipe, _ := scaffold.FindRecipe(tmpl, "tests")
	if !recipe.Applied(after) {
		t.Error("the recipe does not consider itself applied")
	}
	ci, _ := scaffold.FindRecipe(tmpl, "ci")
	if ci.Applied(after) {
		t.Error("the ci recipe considers itself applied after `add tests`")
	}
}

// A file the user has written themselves is theirs. The recipe leaves it alone
// and puts what it would have written beside it.
func TestRetrofitWritesASidecarRatherThanClobbering(t *testing.T) {
	root := generate(t, specWithout("tunesic", func(s *model.Spec) { s.CI = false }))

	mine := "name: mine\non: push\n"
	workflow := filepath.Join(root, ".github", "workflows", "ci.yml")
	if err := os.MkdirAll(filepath.Dir(workflow), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workflow, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}

	report := retrofit(t, root, "ci", false)

	if got := mustRead(t, root, ".github/workflows/ci.yml"); got != mine {
		t.Error("the user's workflow was overwritten")
	}
	sidecar := mustRead(t, root, ".github/workflows/ci.yml.new")
	if !strings.Contains(sidecar, "runs-on:") {
		t.Errorf("the sidecar does not look like a workflow: %q", sidecar)
	}
	if paths := report.Writer.SidecarPaths(); len(paths) != 1 {
		t.Errorf("sidecar paths = %v, want exactly the workflow", paths)
	}
}

// A project generated before the anchors existed cannot be edited. Saying so,
// with the lines that would have gone in, is the difference between a warning
// and something the reader can act on.
func TestRetrofitReportsWhatItCouldNotWire(t *testing.T) {
	root := generate(t, specWithout("tunesic", func(s *model.Spec) { s.Tests = false }))

	// Strip the anchors, as a project generated by an older build would have.
	stripAnchor(t, root, "sharedLogic/build.gradle.kts", "kmp-scaffold:shared-test-deps")
	stripAnchor(t, root, "androidApp/build.gradle.kts", "kmp-scaffold:test-deps")

	report := retrofit(t, root, "tests", false)

	var unapplied []string
	for _, r := range report.Wire {
		if r.Missing {
			unapplied = append(unapplied, r.Path)
			if len(r.Lines) == 0 {
				t.Errorf("%s: nothing to tell the user to paste", r.Path)
			}
			if r.Anchor == "" {
				t.Errorf("%s: the report does not name the missing anchor", r.Path)
			}
		}
	}
	if len(unapplied) != 2 {
		t.Errorf("unapplied edits = %v, want both build scripts", unapplied)
	}

	// The files it *could* write still got written: a wiring problem in one
	// file must not abandon the rest of the job.
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(
		"sharedLogic/src/commonTest/kotlin/io/kontour/tunesic/core/util/PlatformTest.kt"))); err != nil {
		t.Errorf("the test files were not written: %v", err)
	}
}

// The anchors only earn their keep if they are there when tests are off, which
// is the only case that needs them.
func TestRetrofitAnchorsExistWithoutTests(t *testing.T) {
	root := generate(t, specWithout("tunesic", func(s *model.Spec) { s.Tests = false }))

	for _, c := range []struct{ rel, anchor string }{
		{"sharedLogic/build.gradle.kts", "kmp-scaffold:shared-test-deps"},
		{"androidApp/build.gradle.kts", "kmp-scaffold:test-deps"},
		{"iosApp/Packages/Features/Package.swift", "kmp-scaffold:ios-test-target"},
	} {
		if !strings.Contains(mustRead(t, root, c.rel), c.anchor) {
			t.Errorf("%s has no %s anchor, so `add tests` could never edit it", c.rel, c.anchor)
		}
	}
}

// An iOS-only or Android-only project must get only what it can hold.
func TestRetrofitFollowsTheProjectShape(t *testing.T) {
	android := generate(t, specWithout("tunesic", func(s *model.Spec) {
		s.Tests = false
		s.IOS = false
	}))
	retrofit(t, android, "tests", false)
	if _, err := os.Stat(filepath.Join(android, "iosApp")); err == nil {
		t.Error("an Android-only project grew an iosApp directory")
	}

	simple := generate(t, specWithout("tunesic", func(s *model.Spec) {
		s.Tests = false
		s.IOSLayout = "swiftui-simple"
	}))
	retrofit(t, simple, "tests", false)
	if _, err := os.Stat(filepath.Join(simple, filepath.FromSlash(
		"iosApp/Packages/Features/Tests"))); err == nil {
		t.Error("the single-entry-point layout got a Swift test target it has no package for")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func stripAnchor(t *testing.T, root, rel, anchor string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	body := mustRead(t, root, rel)

	var kept []string
	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, anchor) {
			kept = append(kept, line)
		}
	}
	if err := os.WriteFile(path, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}

// compareTrees asserts the two projects hold the same files with the same
// contents, except for paths named in `expected` along with why.
func compareTrees(t *testing.T, got, want string, expected map[string]string) {
	t.Helper()

	list := func(root string) map[string]string {
		out := map[string]string{}
		err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			body, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(rel)] = string(body)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		// The manifest records how the project came to be, which is exactly the
		// thing that legitimately differs.
		delete(out, model.ManifestFile)
		return out
	}

	a, b := list(got), list(want)
	for rel := range b {
		if _, ok := a[rel]; !ok {
			t.Errorf("the retrofitted project is missing %s", rel)
		}
	}
	for rel, body := range a {
		other, ok := b[rel]
		if !ok {
			t.Errorf("the retrofitted project has %s, which generating never writes", rel)
			continue
		}
		if body == other {
			continue
		}
		if why, allowed := expected[rel]; allowed {
			t.Logf("%s differs as expected (%s)", rel, why)
			continue
		}
		t.Errorf("%s differs between retrofitting and generating", rel)
	}
}

func catalogKeys(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	for _, line := range strings.Split(mustRead(t, root, "gradle/libs.versions.toml"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		if name, _, ok := strings.Cut(line, "="); ok {
			out = append(out, strings.TrimSpace(name))
		}
	}
	return out
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, v := range a {
		seen[v]++
	}
	for _, v := range b {
		seen[v]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

// kmpSpecFrom is SpecFrom, named apart so the test reads as what it is.
var kmpSpecFrom = kmp.SpecFrom

// A file the recipe affects but does not own is updated only when it is still
// exactly what this tool wrote. That rule is what lets `add tests` refresh the
// CI workflow so it runs them, without ever overwriting one someone has taken
// over.
func TestRetrofitRefreshesAnUntouchedWorkflowOnly(t *testing.T) {
	const workflow = ".github/workflows/ci.yml"

	// Untouched: it should gain the test steps.
	untouched := generate(t, specWithout("tunesic", func(s *model.Spec) { s.Tests = false }))
	if strings.Contains(mustRead(t, untouched, workflow), "allTests") {
		t.Fatal("a project with no tests already had a test step")
	}
	retrofit(t, untouched, "tests", false)
	if !strings.Contains(mustRead(t, untouched, workflow), "allTests") {
		t.Error("the workflow was not refreshed, so CI will not run the tests just added")
	}
	if _, err := os.Stat(filepath.Join(untouched, filepath.FromSlash(workflow+".new"))); err == nil {
		t.Error("an untouched workflow got a sidecar instead of being updated")
	}

	// Edited: it is the user's now, so it must not change.
	edited := generate(t, specWithout("tunesic", func(s *model.Spec) { s.Tests = false }))
	path := filepath.Join(edited, filepath.FromSlash(workflow))
	mine := mustRead(t, edited, workflow) + "\n# my own step\n"
	if err := os.WriteFile(path, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}

	retrofit(t, edited, "tests", false)

	if got := mustRead(t, edited, workflow); got != mine {
		t.Error("an edited workflow was overwritten")
	}
	sidecar := mustRead(t, edited, workflow+".new")
	if !strings.Contains(sidecar, "allTests") {
		t.Error("the sidecar does not contain the refreshed workflow")
	}
}
