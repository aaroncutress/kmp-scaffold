// Package model holds the description of a project that the generators consume.
//
// A Spec is the Kotlin Multiplatform template's own answer type - what its
// wizard fills in and what its templates render against. A Manifest is the
// template-agnostic file written into the generated project
// (.kmp-scaffold.json), so that later `kmp-scaffold add` runs know which
// template built it and how it was laid out.
package model

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Spec is the complete description of a project to generate.
type Spec struct {
	// Identity
	Name          string `json:"name"`          // "Tunesic" - rootProject.name
	Dir           string `json:"dir"`           // where to write
	Package       string `json:"package"`       // io.kontour.tunesic
	ApplicationID string `json:"applicationId"` // io.kontour.tunesic

	// Targets
	Android bool `json:"android"`
	IOS     bool `json:"ios"`

	// Layout strategies, resolved against the generator registry. These are
	// plain strings so that new strategies (e.g. a feature-per-module iOS
	// layout) can be registered without changing this struct.
	AndroidLayout string `json:"androidLayout"`
	IOSLayout     string `json:"iosLayout"`

	// Selected shared-logic utilities (ids from catalog.SharedUtilities).
	SharedUtils []string `json:"sharedUtils"`

	// Selected Android extras (ids from catalog.AndroidExtras).
	AndroidExtras []string `json:"androidExtras"`

	// Selected library packs (ids from catalog.Packs). Core packs are always on.
	Packs []string `json:"packs"`

	// Root tabs for the shell layout, in order. Display labels; module names
	// are derived from them.
	RootTabs []string `json:"rootTabs"`

	// Tests writes the test source sets, the dependencies they need, and one
	// worked example per platform. CI writes a GitHub Actions workflow.
	Tests bool `json:"tests"`
	CI    bool `json:"ci"`

	// Version policy
	Channel      string `json:"channel"`      // stable | preview | bleeding
	Offline      bool   `json:"offline"`      // skip network, use baselines
	MinSDK       int    `json:"minSdk"`       //
	CompileSDK   int    `json:"compileSdk"`   // 0 = resolve
	AGP          string `json:"agp"`          // "" = resolve
	KotlinVer    string `json:"kotlin"`       // "" = resolve
	GradleVer    string `json:"gradle"`       // "" = resolve
	JVMTarget    string `json:"jvmTarget"`    // "17"
	IOSDeployTgt string `json:"iosDeployTgt"` // "18.0"

	// SwiftMode is the Swift language mode the iOS side compiles in, "5" or
	// "6". Not a version to resolve: 6 turns on strict concurrency, which is a
	// decision about the code rather than about being current.
	SwiftMode string `json:"swiftMode"` // "5"
}

// Defaults returns a Spec pre-filled with the recommended answers. The wizard
// starts from this, and --yes uses it verbatim.
func Defaults() Spec {
	return Spec{
		Android:       true,
		IOS:           true,
		AndroidLayout: "nav3-shell",
		IOSLayout:     "swiftui-features",
		// SharedUtils, AndroidExtras and Packs are filled from the catalog by
		// the caller, so a pack and the utility that depends on it cannot drift
		// apart as the catalog changes.
		RootTabs: []string{"Home", "Settings"},
		Tests:    true,
		CI:       true,
		Channel:  "preview",
		MinSDK:   26,
		// 17 is what AGP 9 and Gradle 9 are built around; 11 is behind the
		// toolchain everything else here resolves to.
		JVMTarget: "17",
		// A whole major version, not a point release: a deployment target of
		// 18.2 excludes 18.0 and 18.1 devices for no stated reason.
		IOSDeployTgt: "18.0",
		// Swift 5 language mode. Kotlin/Native's exported classes are not
		// Sendable, so 6 is offered rather than assumed.
		SwiftMode: "5",
	}
}

// Has reports whether the named item is in the list.
func Has(list []string, id string) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}

// HasSharedUtil is a convenience for templates.
func (s Spec) HasSharedUtil(id string) bool { return Has(s.SharedUtils, id) }

// HasAndroidExtra is a convenience for templates.
func (s Spec) HasAndroidExtra(id string) bool { return Has(s.AndroidExtras, id) }

// HasPack is a convenience for templates.
func (s Spec) HasPack(id string) bool { return Has(s.Packs, id) }

// PackagePath returns the package as a directory path: io/kontour/tunesic.
func (s Spec) PackagePath() string { return strings.ReplaceAll(s.Package, ".", "/") }

// Namespace is the prefix used for buildSrc convention plugin ids, e.g.
// "tunesic" in `id("tunesic.android.library")`.
func (s Spec) Namespace() string { return LowerAlnum(s.Name) }

// FrameworkName is the iOS framework name produced by sharedLogic.
func (s Spec) FrameworkName() string { return "SharedLogic" }

// ---------------------------------------------------------------------------
// Validation and naming helpers
// ---------------------------------------------------------------------------

var (
	projectNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9 _-]*$`)
	packageRe     = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
	featureNameRe = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
)

// Kotlin keywords that would break a generated package segment.
var kotlinReserved = map[string]bool{
	"as": true, "break": true, "class": true, "continue": true, "do": true,
	"else": true, "false": true, "for": true, "fun": true, "if": true,
	"in": true, "interface": true, "is": true, "null": true, "object": true,
	"package": true, "return": true, "super": true, "this": true, "throw": true,
	"true": true, "try": true, "typealias": true, "typeof": true, "val": true,
	"var": true, "when": true, "while": true,
}

// ValidateProjectName checks a rootProject.name value.
func ValidateProjectName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("project name cannot be empty")
	}
	if !projectNameRe.MatchString(name) {
		return errors.New("must start with a letter and contain only letters, digits, spaces, - or _")
	}
	return nil
}

// ValidatePackage checks a Kotlin/Android package name.
func ValidatePackage(pkg string) error {
	if !packageRe.MatchString(pkg) {
		return errors.New("must look like com.example.app (at least two lowercase segments)")
	}
	for _, seg := range strings.Split(pkg, ".") {
		if kotlinReserved[seg] {
			return fmt.Errorf("%q is a Kotlin keyword and cannot be a package segment", seg)
		}
	}
	return nil
}

// ValidateFeatureName checks a feature module name.
func ValidateFeatureName(name string) error {
	if !featureNameRe.MatchString(name) {
		return errors.New("use lowercase kebab-case, e.g. firmware-update")
	}
	if kotlinReserved[strings.ReplaceAll(name, "-", "")] {
		return fmt.Errorf("%q collides with a Kotlin keyword", name)
	}
	return nil
}

// LowerAlnum lowercases a string and strips everything that is not a letter or
// digit: "My App!" -> "myapp". Used for convention-plugin prefixes.
func LowerAlnum(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "app"
	}
	out := b.String()
	if out[0] >= '0' && out[0] <= '9' {
		return "app" + out
	}
	return out
}

// Pascal converts kebab/snake/space separated text to PascalCase.
func Pascal(s string) string {
	parts := splitWords(s)
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(strings.ToUpper(p[:1]))
		b.WriteString(p[1:])
	}
	if b.Len() == 0 {
		return ""
	}
	return b.String()
}

// Camel converts kebab/snake/space separated text to camelCase.
func Camel(s string) string {
	p := Pascal(s)
	if p == "" {
		return ""
	}
	return strings.ToLower(p[:1]) + p[1:]
}

// Kebab converts arbitrary text to kebab-case.
func Kebab(s string) string {
	parts := splitWords(s)
	for i, p := range parts {
		parts[i] = strings.ToLower(p)
	}
	return strings.Join(parts, "-")
}

// PackageSegment converts a feature name into a package segment: a kebab-case
// name loses its dashes ("firmware-update" -> "firmwareupdate"), matching the
// convention used for feature packages.
func PackageSegment(s string) string {
	return strings.ToLower(strings.ReplaceAll(Pascal(s), "-", ""))
}

// splitWords breaks a string on separators and camelCase boundaries.
func splitWords(s string) []string {
	var words []string
	var cur strings.Builder
	var prevLower bool
	for _, r := range s {
		switch {
		case r == '-' || r == '_' || r == ' ' || r == '.' || r == '/':
			if cur.Len() > 0 {
				words = append(words, cur.String())
				cur.Reset()
			}
			prevLower = false
		case r >= 'A' && r <= 'Z':
			if prevLower && cur.Len() > 0 {
				words = append(words, cur.String())
				cur.Reset()
			}
			cur.WriteRune(r)
			prevLower = false
		default:
			cur.WriteRune(r)
			prevLower = true
		}
	}
	if cur.Len() > 0 {
		words = append(words, cur.String())
	}
	return words
}

// TypePrefix is the PascalCase form of the project name, used to name
// generated Kotlin types (AppTheme, AppApplication, ...).
func (s Spec) TypePrefix() string { return Pascal(s.Name) }

// ThemeName is the generated Compose theme function/object name.
func (s Spec) ThemeName() string { return Pascal(s.Name) + "Theme" }

// AppClassName is the generated android.app.Application subclass name.
func (s Spec) AppClassName() string { return Pascal(s.Name) + "Application" }
