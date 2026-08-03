package resolve

import (
	"testing"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
)

func TestParseVersionChannel(t *testing.T) {
	cases := []struct {
		raw  string
		want catalog.Channel
	}{
		{"2.4.10", catalog.Stable},
		{"1.13.0", catalog.Stable},
		{"2.4.20-Beta2", catalog.Preview},
		{"1.12.0-rc01", catalog.Preview},
		{"1.5.0-alpha25", catalog.Bleeding},
		{"1.0.0-SNAPSHOT", catalog.Bleeding},
		{"9.4.0-alpha07", catalog.Bleeding},
		{"3.3.0-next.1", catalog.Preview},
		{"1.0.0-preview02", catalog.Preview},
		{"2.0.0-eap1", catalog.Preview},
		// Variant suffixes are real releases, not prereleases.
		{"0.8.0-0.6.x-compat", catalog.Stable},
		{"1.0.6-kotlin-2.4.20", catalog.Stable},
	}
	for _, c := range cases {
		if got := ParseVersion(c.raw).Channel(); got != c.want {
			t.Errorf("ParseVersion(%q).Channel() = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestVersionOrdering(t *testing.T) {
	cases := []struct{ lower, higher string }{
		{"1.0.0", "1.0.1"},
		{"1.9.0", "1.10.0"},
		{"2.4.10", "2.4.20-Beta1"},
		{"1.0.0-alpha01", "1.0.0-alpha02"},
		{"1.0.0-alpha09", "1.0.0-beta01"},
		{"1.0.0-beta01", "1.0.0-rc01"},
		{"3.3.0-next.1", "3.3.0"},
		{"1.0.0-rc01", "1.0.0"},
		// A plain release beats a variant of the same number.
		{"0.8.0-0.6.x-compat", "0.8.0"},
		{"1.0.6-kotlin-2.4.20-Beta2", "1.0.6"},
	}
	for _, c := range cases {
		lo, hi := ParseVersion(c.lower), ParseVersion(c.higher)
		if !lo.Less(hi) {
			t.Errorf("expected %q < %q", c.lower, c.higher)
		}
		if hi.Less(lo) {
			t.Errorf("expected %q > %q", c.higher, c.lower)
		}
	}
}

func TestPickRespectsChannel(t *testing.T) {
	candidates := []string{"1.0.0", "1.1.0-beta01", "1.2.0-alpha03"}

	if got, relaxed := Pick(candidates, catalog.Stable); got != "1.0.0" || relaxed {
		t.Errorf("stable: got %q relaxed=%v, want 1.0.0", got, relaxed)
	}
	if got, _ := Pick(candidates, catalog.Preview); got != "1.1.0-beta01" {
		t.Errorf("preview: got %q, want 1.1.0-beta01", got)
	}
	if got, _ := Pick(candidates, catalog.Bleeding); got != "1.2.0-alpha03" {
		t.Errorf("bleeding: got %q, want 1.2.0-alpha03", got)
	}
}

// A library that has never had a stable release must still resolve, with the
// caller told that the channel was relaxed.
func TestPickRelaxesWhenNothingQualifies(t *testing.T) {
	got, relaxed := Pick([]string{"1.2.0-alpha01", "1.2.0-alpha07"}, catalog.Stable)
	if got != "1.2.0-alpha07" {
		t.Errorf("got %q, want 1.2.0-alpha07", got)
	}
	if !relaxed {
		t.Error("expected relaxed = true when no stable release exists")
	}
}

func TestUsesLegacyKSPScheme(t *testing.T) {
	legacy := []string{"2.2.20-2.0.4", "2.0.21-1.0.28"}
	modern := []string{"2.3.10", "2.3.0", "2.3.1-rc01"}

	for _, v := range legacy {
		if !usesLegacyKSPScheme(v) {
			t.Errorf("%q should be recognised as the Kotlin-prefixed KSP scheme", v)
		}
	}
	for _, v := range modern {
		if usesLegacyKSPScheme(v) {
			t.Errorf("%q should not be recognised as the Kotlin-prefixed KSP scheme", v)
		}
	}
}

func TestHighestWithPrefix(t *testing.T) {
	kspVersions := []string{
		"2.2.20-2.0.2", "2.2.20-2.0.4", "2.2.21-2.0.5", "2.2.21-RC-2.0.4",
	}
	got := HighestWithPrefix(kspVersions, "2.2.20-", catalog.Stable)
	if got != "2.2.20-2.0.4" {
		t.Errorf("got %q, want 2.2.20-2.0.4", got)
	}
	if got := HighestWithPrefix(kspVersions, "2.9.99-", catalog.Stable); got != "" {
		t.Errorf("got %q, want no match", got)
	}
}

func TestEffectiveChannel(t *testing.T) {
	// A key that is alpha-only pulls a stable request up to bleeding.
	if got := EffectiveChannel(catalog.Stable, catalog.Bleeding); got != catalog.Bleeding {
		t.Errorf("got %q, want bleeding", got)
	}
	// The user's more adventurous choice is never pulled back down.
	if got := EffectiveChannel(catalog.Bleeding, catalog.Preview); got != catalog.Bleeding {
		t.Errorf("got %q, want bleeding", got)
	}
}
