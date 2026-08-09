package resolve

import (
	"strings"
	"testing"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
)

// A trimmed transcript of `git ls-remote --tags --refs`, with the awkward parts
// real repositories have: a "v" prefix on some tags, tags that are not versions
// at all, and the lexicographic order the remote answers in.
const lsRemote = `9a1c0f4e1b2d3c4e5f60718293a4b5c6d7e8f901	refs/tags/1.0.10
1b2c3d4e5f60718293a4b5c6d7e8f9012345678a	refs/tags/1.0.6
2c3d4e5f60718293a4b5c6d7e8f9012345678abc	refs/tags/1.0.6-kotlin-2.4.20-Beta2
3d4e5f60718293a4b5c6d7e8f9012345678abcde	refs/tags/1.0.9
4e5f60718293a4b5c6d7e8f9012345678abcdef0	refs/tags/v0.9.0
5f60718293a4b5c6d7e8f9012345678abcdef012	refs/tags/latest
60718293a4b5c6d7e8f9012345678abcdef01234	refs/tags/docs-2024
`

func TestParseTags(t *testing.T) {
	got := ParseTags(lsRemote)
	want := []string{"0.9.0", "1.0.6-kotlin-2.4.20-Beta2", "1.0.6", "1.0.9", "1.0.10"}

	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// The two things that make a tag list a version list: 1.0.10 is newer than
// 1.0.9 despite sorting before it, and a suffixed tag is not preferred over the
// plain release it accompanies.
func TestParseTagsOrdersAndPicksLikeMaven(t *testing.T) {
	versions := ParseTags(lsRemote)
	best, relaxed := Pick(versions, catalog.Stable)
	if best != "1.0.10" {
		t.Errorf("picked %q, want 1.0.10", best)
	}
	if relaxed {
		t.Error("1.0.10 is a stable release; nothing was relaxed")
	}

	// The suffixed variant is still a candidate, just not the winner.
	if !contains(versions, "1.0.6-kotlin-2.4.20-Beta2") {
		t.Error("the Kotlin-suffixed tag was dropped, but it is a real published version")
	}
}

func TestParseTagsIgnoresNonVersions(t *testing.T) {
	for _, unwanted := range []string{"latest", "docs-2024"} {
		if contains(ParseTags(lsRemote), unwanted) {
			t.Errorf("%q is not a version", unwanted)
		}
	}
	if got := ParseTags("not a transcript at all"); len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}

// An annotated tag's peeled entry names the same version twice; --refs usually
// drops them, but a transcript from an older git may not.
func TestParseTagsDeduplicates(t *testing.T) {
	got := ParseTags("aaa\trefs/tags/2.0.0\nbbb\trefs/tags/2.0.0^{}\n")
	if len(got) != 1 || got[0] != "2.0.0" {
		t.Errorf("got %v, want [2.0.0]", got)
	}
}

func TestPackageURL(t *testing.T) {
	c := catalog.Coordinate{Group: "https://github.com/rickclephas", Artifact: "KMP-ObservableViewModel", Repo: catalog.Swift}
	if got := PackageURL(c); got != "https://github.com/rickclephas/KMP-ObservableViewModel" {
		t.Errorf("PackageURL = %q", got)
	}
	// The coordinate prints as the thing you would paste into a browser, not
	// as group:artifact.
	if got := c.String(); !strings.HasPrefix(got, "https://") {
		t.Errorf("String() = %q, want a URL for a Swift coordinate", got)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Pairing
// ---------------------------------------------------------------------------

func pairResult(kotlin, swift string) *Result {
	return &Result{
		Versions: map[string]string{
			catalog.KeyObservableVM:      kotlin,
			catalog.KeyObservableVMSwift: swift,
		},
		Sources: map[string]string{},
	}
}

var pairRule = PairRule{
	Lead:   catalog.KeyObservableVM,
	Follow: catalog.KeyObservableVMSwift,
	Label:  "KMP-ObservableViewModel",
}

// The whole point: the follower gives up its own newer pick to match the lead.
func TestApplyPairsTakesTheLeadVersion(t *testing.T) {
	res := pairResult("1.0.6", "1.0.9")
	candidates := map[string][]string{
		catalog.KeyObservableVMSwift: {"1.0.6", "1.0.9"},
	}
	applyPairs(res, candidates, Request{Pair: []PairRule{pairRule}})

	if got := res.V(catalog.KeyObservableVMSwift); got != "1.0.6" {
		t.Errorf("swift = %q, want 1.0.6 to match the Kotlin side", got)
	}
	if got := res.Sources[catalog.KeyObservableVMSwift]; got != "matched "+catalog.KeyObservableVM {
		t.Errorf("source = %q, want it to say where the version came from", got)
	}
	if len(res.Notes) != 0 {
		t.Errorf("a successful match should be silent: %v", res.Notes)
	}
}

// Matching a version the follower never published would be worse than not
// matching: it keeps its own pick and the mismatch is said out loud.
func TestApplyPairsWarnsWhenTheVersionIsMissing(t *testing.T) {
	res := pairResult("1.0.7", "1.0.6")
	candidates := map[string][]string{
		catalog.KeyObservableVMSwift: {"1.0.5", "1.0.6"},
	}
	applyPairs(res, candidates, Request{Pair: []PairRule{pairRule}})

	if got := res.V(catalog.KeyObservableVMSwift); got != "1.0.6" {
		t.Errorf("swift = %q, want its own pick kept", got)
	}
	if len(res.Notes) != 1 {
		t.Fatalf("want one note explaining the mismatch, got %v", res.Notes)
	}
	note := res.Notes[0]
	if note.Level != Warn {
		t.Errorf("level = %v, want a warning", note.Level)
	}
	for _, want := range []string{"KMP-ObservableViewModel", "1.0.7", "1.0.6"} {
		if !strings.Contains(note.Text, want) {
			t.Errorf("note %q does not mention %q", note.Text, want)
		}
	}
}

// A version the user pinned themselves is not overwritten by a rule.
func TestApplyPairsLeavesAnOverrideAlone(t *testing.T) {
	res := pairResult("1.0.6", "1.0.9")
	candidates := map[string][]string{catalog.KeyObservableVMSwift: {"1.0.6", "1.0.9"}}
	applyPairs(res, candidates, Request{
		Pair:      []PairRule{pairRule},
		Overrides: map[string]string{catalog.KeyObservableVMSwift: "1.0.9"},
	})

	if got := res.V(catalog.KeyObservableVMSwift); got != "1.0.9" {
		t.Errorf("swift = %q, want the pinned version untouched", got)
	}
	if len(res.Notes) != 0 {
		t.Errorf("nothing to report when the user chose the version: %v", res.Notes)
	}
}

// Offline, or after a failed lookup, there is nothing to pair against and the
// pass must not invent anything.
func TestApplyPairsDoesNothingWithoutALead(t *testing.T) {
	res := pairResult("", "1.0.6")
	applyPairs(res, nil, Request{Pair: []PairRule{pairRule}})

	if got := res.V(catalog.KeyObservableVMSwift); got != "1.0.6" {
		t.Errorf("swift = %q, want it left alone", got)
	}
	if len(res.Notes) != 0 {
		t.Errorf("no lead means nothing to say: %v", res.Notes)
	}
}
