package catalog

import (
	"strings"
	"testing"

	"github.com/aaroncutress/kmp-scaffold/internal/model"
)

func baseSpec() model.Spec {
	spec := model.Defaults()
	spec.Name = "Tunesic"
	spec.Package = "io.kontour.tunesic"
	spec.Packs = BasicPacks()
	spec.SharedUtils = DefaultUtilities()
	spec.AndroidExtras = DefaultExtras()
	return spec
}

func TestNormaliseDropsIOSOnlyPacksWithoutIOS(t *testing.T) {
	spec := baseSpec()
	spec.IOS = false

	notes := Normalise(&spec)

	if model.Has(spec.Packs, "skie") {
		t.Error("SKIE should be dropped when there is no iOS target")
	}
	if model.Has(spec.SharedUtils, "ios-koin-helper") {
		t.Error("the iOS Koin helper should be dropped when there is no iOS target")
	}
	if len(notes) == 0 {
		t.Error("dropping a selection should be explained in a note")
	}
}

func TestNormaliseDropsAndroidSelectionsWithoutAndroid(t *testing.T) {
	spec := baseSpec()
	spec.Android = false

	Normalise(&spec)

	if len(spec.AndroidExtras) != 0 {
		t.Errorf("AndroidExtras = %v, want none", spec.AndroidExtras)
	}
	if len(spec.RootTabs) != 0 {
		t.Errorf("RootTabs = %v, want none", spec.RootTabs)
	}
	if spec.AndroidLayout != "none" {
		t.Errorf("AndroidLayout = %q, want none", spec.AndroidLayout)
	}
}

func TestNormalisePullsInRequiredUtilities(t *testing.T) {
	spec := baseSpec()
	// The network observer needs BaseViewModel, but the user only asked for one.
	spec.SharedUtils = []string{"network-observer"}

	Normalise(&spec)

	if !model.Has(spec.SharedUtils, "base-viewmodel") {
		t.Errorf("SharedUtils = %v, want base-viewmodel to be pulled in", spec.SharedUtils)
	}
}

func TestNormaliseDropsUtilitiesWhoseLibraryPackIsMissing(t *testing.T) {
	spec := baseSpec()
	spec.SharedUtils = append(spec.SharedUtils, "database")
	spec.Packs = nil // no database pack

	notes := Normalise(&spec)

	if model.Has(spec.SharedUtils, "database") {
		t.Error("the Room utility should be dropped without the database pack")
	}
	if !containsSubstring(notes, "database") && !containsSubstring(notes, "Local database") {
		t.Errorf("notes = %v, want an explanation mentioning the database pack", notes)
	}
}

func TestNormaliseDropsExtrasWhoseUtilityIsMissing(t *testing.T) {
	spec := baseSpec()
	spec.SharedUtils = []string{"koin-di"} // no network observer

	Normalise(&spec)

	if model.Has(spec.AndroidExtras, "connectivity-banner") {
		t.Error("the connectivity banner should be dropped without the network observer")
	}
}

func TestNormaliseGivesSingleStackLayoutOneRoot(t *testing.T) {
	spec := baseSpec()
	spec.AndroidLayout = "nav3-single"
	spec.RootTabs = []string{"Home", "Settings", "More"}

	Normalise(&spec)

	if len(spec.RootTabs) != 1 || spec.RootTabs[0] != "Home" {
		t.Errorf("RootTabs = %v, want exactly [Home]", spec.RootTabs)
	}
}

func TestEveryPredicateIsSafeOnAnEmptySpec(t *testing.T) {
	// Catalog predicates run before normalisation in some paths, so none of
	// them may assume a populated spec.
	var empty model.Spec
	for _, lib := range Libraries() {
		lib.When(empty)
	}
	for _, b := range Bundles() {
		b.When(empty)
	}
	for _, p := range Plugins() {
		p.When(empty)
	}
}

func TestVersionKeysAreUniqueAndComplete(t *testing.T) {
	seen := map[string]bool{}
	sections := map[string]bool{}
	for _, s := range SectionOrder() {
		sections[s] = true
	}
	for _, k := range VersionKeys() {
		if seen[k.Key] {
			t.Errorf("duplicate version key %q", k.Key)
		}
		seen[k.Key] = true
		if k.Baseline == "" {
			t.Errorf("version key %q has no baseline, so offline generation would emit an empty version", k.Key)
		}
		if !sections[k.Section] {
			t.Errorf("version key %q is in section %q, which is not in SectionOrder()", k.Key, k.Section)
		}
	}
}

// Every library and plugin must reference a version key that exists, or the
// generated catalog will not resolve.
func TestCatalogReferencesResolve(t *testing.T) {
	keys := map[string]bool{}
	for _, k := range VersionKeys() {
		keys[k.Key] = true
	}
	for _, lib := range Libraries() {
		if lib.Version != "" && !keys[lib.Version] {
			t.Errorf("library %q references unknown version key %q", lib.Alias, lib.Version)
		}
		if !strings.Contains(lib.Module, ":") {
			t.Errorf("library %q has a malformed module %q", lib.Alias, lib.Module)
		}
	}
	for _, p := range Plugins() {
		if p.Version != "" && !keys[p.Version] {
			t.Errorf("plugin %q references unknown version key %q", p.Alias, p.Version)
		}
	}

	aliases := map[string]bool{}
	for _, lib := range Libraries() {
		aliases[lib.Alias] = true
	}
	for _, b := range Bundles() {
		for _, member := range b.Libs {
			if !aliases[member] {
				t.Errorf("bundle %q references unknown library %q", b.Name, member)
			}
		}
	}
}

func TestAccessor(t *testing.T) {
	cases := map[string]string{
		"koin-core":                   "libs.koin.core",
		"androidx-compose-ui-tooling": "libs.androidx.compose.ui.tooling",
		"turbine":                     "libs.turbine",
	}
	for alias, want := range cases {
		if got := Accessor(alias); got != want {
			t.Errorf("Accessor(%q) = %q, want %q", alias, got, want)
		}
	}
}

func containsSubstring(list []string, want string) bool {
	for _, s := range list {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
}
