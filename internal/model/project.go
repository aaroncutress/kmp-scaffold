// Package model holds the description of a project that the generators consume.
//
// A Spec is what the wizard fills in. A Manifest is the subset of that which is
// written into the generated project (as .kmp-scaffold.json) so that later
// `kmp-scaffold add` runs know how the project was laid out.
package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ManifestFile is the per-project state file written at the project root.
const ManifestFile = ".kmp-scaffold.json"

// ManifestSchema is bumped whenever the manifest layout changes incompatibly.
const ManifestSchema = 1

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

	// Version policy
	Channel      string `json:"channel"`      // stable | preview | bleeding
	Offline      bool   `json:"offline"`      // skip network, use baselines
	MinSDK       int    `json:"minSdk"`       //
	CompileSDK   int    `json:"compileSdk"`   // 0 = resolve
	AGP          string `json:"agp"`          // "" = resolve
	KotlinVer    string `json:"kotlin"`       // "" = resolve
	GradleVer    string `json:"gradle"`       // "" = resolve
	JVMTarget    string `json:"jvmTarget"`    // "11"
	IOSDeployTgt string `json:"iosDeployTgt"` // "18.2"
}

// Manifest is what gets written into the generated project.
type Manifest struct {
	Schema        int       `json:"schema"`
	GeneratorVer  string    `json:"generatorVersion"`
	Name          string    `json:"name"`
	Package       string    `json:"package"`
	ApplicationID string    `json:"applicationId"`
	Android       bool      `json:"android"`
	IOS           bool      `json:"ios"`
	AndroidLayout string    `json:"androidLayout"`
	IOSLayout     string    `json:"iosLayout"`
	SharedUtils   []string  `json:"sharedUtils"`
	AndroidExtras []string  `json:"androidExtras"`
	Packs         []string  `json:"packs"`
	RootTabs      []string  `json:"rootTabs"`
	Features      []Feature `json:"features"`
}

// Feature records a feature module added to the project.
type Feature struct {
	Name         string `json:"name"`          // kebab-case, e.g. "firmware-update"
	Android      bool   `json:"android"`       // has feature/<name>/{api,impl}
	Shared       bool   `json:"shared"`        // has sharedLogic feature package
	IOS          bool   `json:"ios,omitempty"` // has a target in the iOS Features package
	Presentation string `json:"presentation"`  // shell | above-nav | overlay | dialog
	RootTab      bool   `json:"rootTab"`
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
		RootTabs:     []string{"Home", "Settings"},
		Channel:      "preview",
		MinSDK:       26,
		JVMTarget:    "11",
		IOSDeployTgt: "18.2",
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

// ToManifest projects the spec into the file written to the generated project.
func (s Spec) ToManifest(version string, features []Feature) Manifest {
	return Manifest{
		Schema:        ManifestSchema,
		GeneratorVer:  version,
		Name:          s.Name,
		Package:       s.Package,
		ApplicationID: s.ApplicationID,
		Android:       s.Android,
		IOS:           s.IOS,
		AndroidLayout: s.AndroidLayout,
		IOSLayout:     s.IOSLayout,
		SharedUtils:   s.SharedUtils,
		AndroidExtras: s.AndroidExtras,
		Packs:         s.Packs,
		RootTabs:      s.RootTabs,
		Features:      features,
	}
}

// LoadManifest reads .kmp-scaffold.json from dir, walking up to the filesystem
// root so that `add` works from any subdirectory of a project.
func LoadManifest(dir string) (*Manifest, string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, "", err
	}
	for {
		candidate := filepath.Join(abs, ManifestFile)
		if data, err := os.ReadFile(candidate); err == nil {
			var m Manifest
			if err := json.Unmarshal(data, &m); err != nil {
				return nil, "", fmt.Errorf("parsing %s: %w", candidate, err)
			}
			if m.Schema > ManifestSchema {
				return nil, "", fmt.Errorf(
					"%s was written by a newer kmp-scaffold (schema %d, this build understands %d)",
					candidate, m.Schema, ManifestSchema)
			}
			return &m, abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return nil, "", fmt.Errorf("no %s found in %s or any parent directory - "+
				"run this from inside a project created by kmp-scaffold", ManifestFile, dir)
		}
		abs = parent
	}
}

// Save writes the manifest to the given project root.
func (m Manifest) Save(root string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(root, ManifestFile), data, 0o644)
}

// PackagePath returns the package as a directory path.
func (m Manifest) PackagePath() string { return strings.ReplaceAll(m.Package, ".", "/") }

// Namespace mirrors Spec.Namespace for manifests.
func (m Manifest) Namespace() string { return LowerAlnum(m.Name) }

// HasSharedUtil is a convenience for templates.
func (m Manifest) HasSharedUtil(id string) bool { return Has(m.SharedUtils, id) }

// HasPack is a convenience for templates.
func (m Manifest) HasPack(id string) bool { return Has(m.Packs, id) }

// FindFeature returns the recorded feature with the given name, if any.
func (m Manifest) FindFeature(name string) *Feature {
	for i := range m.Features {
		if m.Features[i].Name == name {
			return &m.Features[i]
		}
	}
	return nil
}

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

// TypePrefix mirrors Spec.TypePrefix for manifests.
func (m Manifest) TypePrefix() string { return Pascal(m.Name) }

// ThemeName mirrors Spec.ThemeName for manifests.
func (m Manifest) ThemeName() string { return Pascal(m.Name) + "Theme" }
