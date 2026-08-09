// Package wire performs surgical, idempotent edits on files that already exist
// in a generated project.
//
// Adding a feature module means touching half a dozen shared files (settings,
// the app's dependencies, the serializer sum, the entry provider, the DI
// graph). Rather than regenerating those files - which would discard the user's
// own edits - the generator writes anchor comments into them, and this package
// inserts before those anchors. Every insertion checks first, so running the
// same command twice changes nothing the second time.
package wire

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Anchor comments written by the templates.
const (
	// AnchorFeatures is in settings.gradle.kts.
	AnchorFeatures = "kmp-scaffold:features"
	// AnchorFeatureDeps is in androidApp/build.gradle.kts.
	AnchorFeatureDeps = "kmp-scaffold:feature-deps"
	// AnchorFeatureAPIs is in the android.feature convention plugin.
	AnchorFeatureAPIs = "kmp-scaffold:feature-apis"
	// AnchorSerializers is in AppSerializers.kt.
	AnchorSerializers = "kmp-scaffold:serializers"
	// AnchorEntries is in App.kt, for screens pushed above the shell.
	AnchorEntries = "kmp-scaffold:entries"
	// AnchorRootEntries is in App.kt, for the shell's own root tabs.
	AnchorRootEntries = "kmp-scaffold:root-entries"
	// AnchorRoots is in App.kt, for the set of root routes.
	AnchorRoots = "kmp-scaffold:roots"
	// AnchorNavItems is in core/ui's Navigation.kt.
	AnchorNavItems = "kmp-scaffold:nav-items"
	// AnchorSharedModules is in sharedLogic's SharedModules.kt.
	AnchorSharedModules = "kmp-scaffold:shared-modules"

	// The iOS Features package and its coordinator.

	// AnchorIOSProducts is the per-target product list in Package.swift.
	AnchorIOSProducts = "kmp-scaffold:ios-products"
	// AnchorIOSAppFeatures is the umbrella product the app target links.
	AnchorIOSAppFeatures = "kmp-scaffold:ios-app-features"
	// AnchorIOSTargets is the target list in Package.swift.
	AnchorIOSTargets = "kmp-scaffold:ios-targets"
	// AnchorIOSTabCases is the AppTab enum in AppCoordinator.swift.
	AnchorIOSTabCases = "kmp-scaffold:ios-tab-cases"
	// AnchorIOSTabPaths is the per-tab NavigationPath state.
	AnchorIOSTabPaths = "kmp-scaffold:ios-tab-paths"
	// AnchorIOSTabs is the TabView body.
	AnchorIOSTabs = "kmp-scaffold:ios-tabs"
	// AnchorIOSDestinations is the shared destination modifier every tab applies.
	AnchorIOSDestinations = "kmp-scaffold:ios-destinations"
)

// Edit is one pending change to one file.
type Edit struct {
	// Path is relative to the project root.
	Path string
	// Anchor is the anchor comment to insert before.
	Anchor string
	// Lines are inserted, indented to match the anchor.
	Lines []string
	// Imports are added to the file's import block (Kotlin files only).
	Imports []string
	// Key decides whether this block is already present, instead of its first
	// meaningful line.
	//
	// The default is right when that first line is distinctive - which is why
	// the built-in template does not set this - and wrong when a template
	// renders many blocks that open the same way, where the second insertion
	// would silently do nothing.
	Key string
}

// Result records what an Apply run did to one file.
type Result struct {
	Path     string
	Inserted int
	Skipped  int
	Missing  bool
}

// Applier batches edits so a dry run can report them without writing.
type Applier struct {
	Root   string
	DryRun bool

	results []Result
}

// NewApplier builds an Applier rooted at a project directory.
func NewApplier(root string, dryRun bool) *Applier {
	return &Applier{Root: root, DryRun: dryRun}
}

// Results returns the record of everything applied, in order.
func (a *Applier) Results() []Result { return a.results }

// Apply performs one edit.
//
// A missing anchor is reported rather than treated as fatal: the user may have
// reorganised the file, and telling them exactly what to paste where is more
// useful than refusing to continue.
func (a *Applier) Apply(e Edit) error {
	abs := filepath.Join(a.Root, filepath.FromSlash(e.Path))
	raw, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			a.results = append(a.results, Result{Path: e.Path, Missing: true})
			return nil
		}
		return fmt.Errorf("reading %s: %w", e.Path, err)
	}

	content := string(raw)
	res := Result{Path: e.Path}

	// Imports first, so the inserted code compiles.
	for _, imp := range e.Imports {
		updated, added := addImport(content, imp)
		content = updated
		if added {
			res.Inserted++
		} else {
			res.Skipped++
		}
	}

	if e.Anchor != "" && len(e.Lines) > 0 {
		updated, added, found := insertBeforeAnchor(content, e.Anchor, e.Lines, e.Key)
		if !found {
			a.results = append(a.results, Result{Path: e.Path, Missing: true})
			return nil
		}
		content = updated
		res.Inserted += added
		res.Skipped += len(e.Lines) - added
	}

	if res.Inserted > 0 && !a.DryRun {
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", e.Path, err)
		}
	}
	a.results = append(a.results, res)
	return nil
}

// MissingAnchors lists files where the anchor could not be found.
func (a *Applier) MissingAnchors() []string {
	var out []string
	for _, r := range a.results {
		if r.Missing {
			out = append(out, r.Path)
		}
	}
	return out
}

// insertBeforeAnchor inserts lines immediately above the anchor line, matching
// its indentation. Lines already present anywhere in the file are skipped.
func insertBeforeAnchor(content, anchor string, lines []string, key string) (string, int, bool) {
	fileLines := strings.Split(content, "\n")
	anchorIdx := -1
	for i, l := range fileLines {
		if strings.Contains(l, anchor) {
			anchorIdx = i
			break
		}
	}
	if anchorIdx < 0 {
		return content, 0, false
	}

	indent := leadingWhitespace(fileLines[anchorIdx])

	// A multi-line insertion (a whole SwiftUI tab, say) carries its own nesting.
	// Strip the block's common indent and re-apply the anchor's, so the inserted
	// code keeps its shape wherever the anchor happens to sit.
	base := commonIndent(lines)

	// Duplicate detection is per block rather than per line: a block's inner
	// lines ("}", "content") repeat all over a file, so one thing decides
	// whether the whole block is already there.
	//
	// By default that is the block's first meaningful line, matched exactly. An
	// explicit key is matched as a fragment instead, because the point of
	// giving one is to name what is distinctive about this block - a route
	// name, say - rather than to repeat a whole line of it.
	if key = strings.TrimSpace(key); key != "" {
		for _, l := range fileLines {
			if strings.Contains(l, key) {
				return content, 0, true
			}
		}
	} else {
		first := firstMeaningful(lines)
		if first == "" {
			return content, 0, true
		}
		for _, l := range fileLines {
			if strings.TrimSpace(l) == first {
				return content, 0, true
			}
		}
	}

	var toInsert []string
	blank := true
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			// Keep interior blank lines, drop leading and trailing ones.
			if !blank {
				toInsert = append(toInsert, "")
			}
			continue
		}
		blank = false
		toInsert = append(toInsert, indent+strings.TrimPrefix(l, base))
	}
	// One trailing blank line is kept - that is how a caller asks for a gap
	// between this block and whatever the anchor sits above - but no more.
	for len(toInsert) > 1 && toInsert[len(toInsert)-1] == "" && toInsert[len(toInsert)-2] == "" {
		toInsert = toInsert[:len(toInsert)-1]
	}
	if len(toInsert) == 0 {
		return content, 0, true
	}

	out := make([]string, 0, len(fileLines)+len(toInsert))
	out = append(out, fileLines[:anchorIdx]...)
	out = append(out, toInsert...)
	out = append(out, fileLines[anchorIdx:]...)
	return strings.Join(out, "\n"), len(toInsert), true
}

// addImport inserts an import line into a Kotlin file's import block, keeping
// it sorted. Returns false when the import is already there.
func addImport(content, importLine string) (string, bool) {
	importLine = strings.TrimSpace(importLine)
	if importLine == "" {
		return content, false
	}
	lines := strings.Split(content, "\n")

	first, last := -1, -1
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "import ") {
			if first < 0 {
				first = i
			}
			last = i
			if strings.TrimSpace(l) == importLine {
				return content, false
			}
		}
	}

	if first < 0 {
		// No imports yet: put one right after the package declaration.
		for i, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), "package ") {
				out := make([]string, 0, len(lines)+2)
				out = append(out, lines[:i+1]...)
				out = append(out, "", importLine)
				out = append(out, lines[i+1:]...)
				return strings.Join(out, "\n"), true
			}
		}
		return content, false
	}

	block := append([]string(nil), lines[first:last+1]...)
	block = append(block, importLine)
	// Keep only real imports in the sort; blank lines inside the block are
	// dropped, which matches how Kotlin files are normally formatted.
	var imports []string
	for _, l := range block {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, "import ") {
			imports = append(imports, t)
		}
	}
	sort.Strings(imports)

	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:first]...)
	out = append(out, imports...)
	out = append(out, lines[last+1:]...)
	return strings.Join(out, "\n"), true
}

// commonIndent returns the longest whitespace prefix shared by every non-blank
// line, so a block can be re-indented without losing its internal structure.
func commonIndent(lines []string) string {
	common := ""
	first := true
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		indent := leadingWhitespace(l)
		if first {
			common, first = indent, false
			continue
		}
		for !strings.HasPrefix(indent, common) {
			common = common[:len(common)-1]
		}
	}
	return common
}

// firstMeaningful returns the first non-blank, non-comment line, trimmed.
func firstMeaningful(lines []string) string {
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "//") {
			continue
		}
		return t
	}
	return ""
}

func leadingWhitespace(s string) string {
	for i, r := range s {
		if r != ' ' && r != '\t' {
			return s[:i]
		}
	}
	return s
}
