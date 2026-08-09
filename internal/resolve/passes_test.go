package resolve

import (
	"testing"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
)

// The compatibility passes mutate a shared Result in a specific order, and the
// order is load-bearing: overrides are written last so they win, and the
// Kotlin/KSP pass can move Kotlin off the version the candidate loop chose.
// These tests pin that behaviour with fixed candidate lists, so a refactor that
// reorders the passes fails here rather than silently changing what people get
// in their libs.versions.toml.

func newResult(versions map[string]string) *Result {
	res := &Result{Versions: map[string]string{}, Sources: map[string]string{}}
	for k, v := range versions {
		res.Versions[k] = v
		res.Sources[k] = "baseline"
	}
	return res
}

func TestApplyFixedOverridesWinLast(t *testing.T) {
	res := newResult(map[string]string{catalog.KeyKotlin: "2.4.0"})
	applyFixed(res, Request{Android: true, MinSDK: 28, Overrides: map[string]string{
		catalog.KeyKotlin: "2.3.10",
		catalog.KeyMinSDK: "31",
	}})

	// An override beats both the resolved value and the project setting
	// applyFixed itself just wrote.
	if got := res.V(catalog.KeyKotlin); got != "2.3.10" {
		t.Errorf("kotlin = %q, want the override 2.3.10", got)
	}
	if got := res.V(catalog.KeyMinSDK); got != "31" {
		t.Errorf("minSdk = %q, want the override 31 to beat the project setting 28", got)
	}
	if got := res.Sources[catalog.KeyKotlin]; got != "pinned" {
		t.Errorf("kotlin source = %q, want pinned", got)
	}
	if got := res.V(catalog.KeyVersionName); got != "1.0" {
		t.Errorf("versionName = %q, want 1.0", got)
	}
}

func TestApplyFixedRejectsUnknownKeys(t *testing.T) {
	res := newResult(nil)
	applyFixed(res, Request{Overrides: map[string]string{"not-a-key": "1.0"}})

	if _, ok := res.Versions["not-a-key"]; ok {
		t.Error("an unknown override key was written into the catalog")
	}
	if len(res.Notes) == 0 {
		t.Error("an unknown override key should be reported")
	}
}

func TestApplyFixedSkipsMinSDKWithoutAndroid(t *testing.T) {
	res := newResult(nil)
	applyFixed(res, Request{Android: false})

	if _, ok := res.Versions[catalog.KeyMinSDK]; ok {
		t.Error("minSdk was written for a project with no Android side")
	}
}

func TestApplyAndroidBaselineFollowsThePin(t *testing.T) {
	res := newResult(nil)
	applyAndroidBaseline(res, Request{Android: true, CompileSDK: 35})

	for _, key := range []string{catalog.KeyCompileSDK, catalog.KeyTargetSDK} {
		if got := res.V(key); got != "35" {
			t.Errorf("%s = %q, want 35", key, got)
		}
	}
	if got := res.V(catalog.KeyBuildTools); got != "35.0.0" {
		t.Errorf("buildTools = %q, want 35.0.0", got)
	}
}

func TestApplyKotlinKSPIndependentScheme(t *testing.T) {
	// KSP2 versions independently, so the newest of each is simply taken.
	res := newResult(map[string]string{catalog.KeyKotlin: "2.4.10", catalog.KeyKSP: "2.3.0"})
	applyKotlinKSP(res, map[string][]string{
		catalog.KeyKotlin: {"2.4.0", "2.4.10"},
		catalog.KeyKSP:    {"2.3.0", "2.4.10"},
	}, Request{Channel: catalog.Stable})

	if got := res.V(catalog.KeyKotlin); got != "2.4.10" {
		t.Errorf("kotlin = %q, want 2.4.10 - independent KSP versioning must not move Kotlin", got)
	}
	if got := res.V(catalog.KeyKSP); got != "2.4.10" {
		t.Errorf("ksp = %q, want 2.4.10", got)
	}
}

func TestApplyKotlinKSPLegacySchemeStepsKotlinBack(t *testing.T) {
	// Under the classic scheme KSP embeds the Kotlin version, so a Kotlin
	// release with no KSP build cannot be used.
	res := newResult(map[string]string{catalog.KeyKotlin: "2.2.21", catalog.KeyKSP: "2.2.20-2.0.4"})
	applyKotlinKSP(res, map[string][]string{
		catalog.KeyKotlin: {"2.2.20", "2.2.21"},
		catalog.KeyKSP:    {"2.2.20-2.0.3", "2.2.20-2.0.4"},
	}, Request{Channel: catalog.Stable})

	if got := res.V(catalog.KeyKotlin); got != "2.2.20" {
		t.Errorf("kotlin = %q, want 2.2.20 - the newest Kotlin with a matching KSP", got)
	}
	if got := res.V(catalog.KeyKSP); got != "2.2.20-2.0.4" {
		t.Errorf("ksp = %q, want 2.2.20-2.0.4", got)
	}
	if len(res.Notes) == 0 {
		t.Error("stepping Kotlin back should be explained")
	}
}

func TestApplyKotlinKSPLeavesPinnedVersionsAlone(t *testing.T) {
	res := newResult(map[string]string{catalog.KeyKotlin: "2.2.21", catalog.KeyKSP: "2.2.20-2.0.4"})
	applyKotlinKSP(res, map[string][]string{
		catalog.KeyKotlin: {"2.2.20", "2.2.21"},
		catalog.KeyKSP:    {"2.2.20-2.0.3", "2.2.20-2.0.4"},
	}, Request{Channel: catalog.Stable, Overrides: map[string]string{catalog.KeyKotlin: "2.2.21"}})

	if got := res.V(catalog.KeyKotlin); got != "2.2.21" {
		t.Errorf("kotlin = %q, want the pinned 2.2.21 to survive the KSP pass", got)
	}
}

func TestApplyAGPGradleFlagsAnOldPin(t *testing.T) {
	res := newResult(map[string]string{catalog.KeyAGP: "9.0.0"})
	res.Gradle = GradleRelease{Version: "8.5"}
	applyAGPGradle(res, nil, nil, nil, Request{})

	if !res.HasErrors() {
		t.Error("AGP 9 on Gradle 8.5 should be reported as an error")
	}

	ok := newResult(map[string]string{catalog.KeyAGP: "9.0.0"})
	ok.Gradle = GradleRelease{Version: "9.2"}
	applyAGPGradle(ok, nil, nil, nil, Request{})
	if ok.HasErrors() {
		t.Errorf("AGP 9 on Gradle 9.2 is fine: %v", ok.Notes)
	}
}

func TestRequestSignatureIgnoresKeyOrder(t *testing.T) {
	a := Request{Keys: []string{"kotlin", "ktor"}, Channel: catalog.Stable}
	b := Request{Keys: []string{"ktor", "kotlin"}, Channel: catalog.Stable}
	if a.Signature() != b.Signature() {
		t.Error("the same keys in a different order should not force a re-resolve")
	}

	c := Request{Keys: []string{"kotlin", "ktor"}, Channel: catalog.Bleeding}
	if a.Signature() == c.Signature() {
		t.Error("a different channel must change the signature")
	}

	d := Request{Keys: []string{"kotlin", "ktor"}, Channel: catalog.Stable, MinSDK: 24}
	if a.Signature() == d.Signature() {
		t.Error("a different minSdk must change the signature")
	}
}
