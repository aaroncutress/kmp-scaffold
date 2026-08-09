package wire

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const settingsFile = `rootProject.name = "Tunesic"

include(":sharedLogic")
include(":androidApp")

include(":feature:home:api")
include(":feature:home:impl")

// kmp-scaffold:features - new modules go here.
`

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInsertBeforeAnchor(t *testing.T) {
	root := t.TempDir()
	write(t, root, "settings.gradle.kts", settingsFile)

	applier := NewApplier(root, false)
	err := applier.Apply(Edit{
		Path:   "settings.gradle.kts",
		Anchor: AnchorFeatures,
		Lines: []string{
			`include(":feature:billing:api")`,
			`include(":feature:billing:impl")`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := read(t, root, "settings.gradle.kts")
	if !strings.Contains(got, `include(":feature:billing:api")`) {
		t.Fatalf("insertion missing:\n%s", got)
	}
	// The anchor must survive so the next feature can use it.
	if !strings.Contains(got, AnchorFeatures) {
		t.Error("the anchor comment was consumed")
	}
	// Insertions go above the anchor, not below.
	if strings.Index(got, "billing:impl") > strings.Index(got, AnchorFeatures) {
		t.Error("lines were inserted after the anchor")
	}
}

func TestInsertIsIdempotent(t *testing.T) {
	root := t.TempDir()
	write(t, root, "settings.gradle.kts", settingsFile)

	edit := Edit{
		Path:   "settings.gradle.kts",
		Anchor: AnchorFeatures,
		Lines:  []string{`include(":feature:billing:api")`},
	}

	first := NewApplier(root, false)
	if err := first.Apply(edit); err != nil {
		t.Fatal(err)
	}
	after := read(t, root, "settings.gradle.kts")

	second := NewApplier(root, false)
	if err := second.Apply(edit); err != nil {
		t.Fatal(err)
	}
	if got := read(t, root, "settings.gradle.kts"); got != after {
		t.Error("applying the same edit twice changed the file the second time")
	}
	if n := second.Results()[0].Inserted; n != 0 {
		t.Errorf("second run inserted %d line(s), want 0", n)
	}
}

func TestInsertPreservesIndentation(t *testing.T) {
	root := t.TempDir()
	write(t, root, "App.kt", `fun App() {
    entryProvider {
        homeEntries()
        // kmp-scaffold:entries
    }
}
`)

	applier := NewApplier(root, false)
	if err := applier.Apply(Edit{
		Path:   "App.kt",
		Anchor: AnchorEntries,
		Lines:  []string{"billingEntries()"},
	}); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(read(t, root, "App.kt"), "\n        billingEntries()\n") {
		t.Errorf("indentation was not matched:\n%s", read(t, root, "App.kt"))
	}
}

func TestAddImportKeepsBlockSorted(t *testing.T) {
	root := t.TempDir()
	write(t, root, "App.kt", `package com.example

import androidx.compose.runtime.Composable
import com.example.feature.zzz.impl.zzzEntries

fun App() {
    // kmp-scaffold:entries
}
`)

	applier := NewApplier(root, false)
	if err := applier.Apply(Edit{
		Path:    "App.kt",
		Anchor:  AnchorEntries,
		Lines:   []string{"billingEntries()"},
		Imports: []string{"import com.example.feature.billing.impl.billingEntries"},
	}); err != nil {
		t.Fatal(err)
	}

	got := read(t, root, "App.kt")
	billing := strings.Index(got, "import com.example.feature.billing")
	zzz := strings.Index(got, "import com.example.feature.zzz")
	if billing < 0 {
		t.Fatalf("import was not added:\n%s", got)
	}
	if billing > zzz {
		t.Error("imports are not sorted")
	}
}

func TestMissingAnchorIsReportedNotFatal(t *testing.T) {
	root := t.TempDir()
	write(t, root, "settings.gradle.kts", "rootProject.name = \"Tunesic\"\n")

	applier := NewApplier(root, false)
	if err := applier.Apply(Edit{
		Path:   "settings.gradle.kts",
		Anchor: AnchorFeatures,
		Lines:  []string{`include(":feature:billing:api")`},
	}); err != nil {
		t.Fatalf("a missing anchor should not be an error: %v", err)
	}
	if missing := applier.MissingAnchors(); len(missing) != 1 {
		t.Errorf("MissingAnchors() = %v, want one entry", missing)
	}
}

func TestDryRunLeavesFileAlone(t *testing.T) {
	root := t.TempDir()
	write(t, root, "settings.gradle.kts", settingsFile)

	applier := NewApplier(root, true)
	if err := applier.Apply(Edit{
		Path:   "settings.gradle.kts",
		Anchor: AnchorFeatures,
		Lines:  []string{`include(":feature:billing:api")`},
	}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, root, "settings.gradle.kts"); got != settingsFile {
		t.Error("a dry run modified the file")
	}
	if n := applier.Results()[0].Inserted; n != 1 {
		t.Errorf("dry run reported %d insertion(s), want 1", n)
	}
}

func TestInsertPreservesBlockShape(t *testing.T) {
	root := t.TempDir()
	write(t, root, "AppCoordinator.swift", `        TabView(selection: $selectedTab) {
            NavigationStack(path: $homePath) {
                EmptyView()
            }

            // kmp-scaffold:tabs
        }
`)

	applier := NewApplier(root, false)
	if err := applier.Apply(Edit{
		Path:   "AppCoordinator.swift",
		Anchor: "kmp-scaffold:tabs",
		Lines: []string{
			"NavigationStack(path: $billingPath) {",
			"    EmptyView()",
			"        .billingDestination(for: BillingRoute())",
			"}",
			`.tabItem { Label("Billing", systemImage: "creditcard") }`,
			".tag(AppTab.billing)",
			"",
		},
	}); err != nil {
		t.Fatal(err)
	}

	got := read(t, root, "AppCoordinator.swift")
	// The block's own nesting survives, offset by the anchor's indentation.
	for _, want := range []string{
		"\n            NavigationStack(path: $billingPath) {\n",
		"\n                EmptyView()\n",
		"\n                    .billingDestination(for: BillingRoute())\n",
		"\n            }\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}
}

// A block whose inner lines ("}", "EmptyView()") appear elsewhere must still be
// recognised as already inserted on a second run.
func TestBlockInsertionIsIdempotent(t *testing.T) {
	root := t.TempDir()
	write(t, root, "AppCoordinator.swift", `        TabView {
            NavigationStack(path: $homePath) {
                EmptyView()
            }
            // kmp-scaffold:tabs
        }
`)

	edit := Edit{
		Path:   "AppCoordinator.swift",
		Anchor: "kmp-scaffold:tabs",
		Lines: []string{
			"NavigationStack(path: $billingPath) {",
			"    EmptyView()",
			"}",
		},
	}

	first := NewApplier(root, false)
	if err := first.Apply(edit); err != nil {
		t.Fatal(err)
	}
	after := read(t, root, "AppCoordinator.swift")

	second := NewApplier(root, false)
	if err := second.Apply(edit); err != nil {
		t.Fatal(err)
	}
	if got := read(t, root, "AppCoordinator.swift"); got != after {
		t.Errorf("the block was inserted twice:\n%s", got)
	}
}
