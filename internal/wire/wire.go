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
		updated, added, found := insertBeforeAnchor(content, e.Anchor, e.Lines)
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
func insertBeforeAnchor(content, anchor string, lines []string) (string, int, bool) {
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

	existing := make(map[string]bool, len(fileLines))
	for _, l := range fileLines {
		existing[strings.TrimSpace(l)] = true
	}

	var toInsert []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" || existing[trimmed] {
			continue
		}
		existing[trimmed] = true
		toInsert = append(toInsert, indent+trimmed)
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

func leadingWhitespace(s string) string {
	for i, r := range s {
		if r != ' ' && r != '\t' {
			return s[:i]
		}
	}
	return s
}
