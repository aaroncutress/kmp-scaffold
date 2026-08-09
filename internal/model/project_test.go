package model

import (
	"os"
	"testing"
)

func TestNaming(t *testing.T) {
	cases := []struct {
		in, pascal, camel, kebab, pkgseg string
	}{
		{"firmware-update", "FirmwareUpdate", "firmwareUpdate", "firmware-update", "firmwareupdate"},
		{"home", "Home", "home", "home", "home"},
		{"My App", "MyApp", "myApp", "my-app", "myapp"},
		{"drive_logger", "DriveLogger", "driveLogger", "drive-logger", "drivelogger"},
		{"PidRegistry", "PidRegistry", "pidRegistry", "pid-registry", "pidregistry"},
	}
	for _, c := range cases {
		if got := Pascal(c.in); got != c.pascal {
			t.Errorf("Pascal(%q) = %q, want %q", c.in, got, c.pascal)
		}
		if got := Camel(c.in); got != c.camel {
			t.Errorf("Camel(%q) = %q, want %q", c.in, got, c.camel)
		}
		if got := Kebab(c.in); got != c.kebab {
			t.Errorf("Kebab(%q) = %q, want %q", c.in, got, c.kebab)
		}
		if got := PackageSegment(c.in); got != c.pkgseg {
			t.Errorf("PackageSegment(%q) = %q, want %q", c.in, got, c.pkgseg)
		}
	}
}

func TestLowerAlnum(t *testing.T) {
	cases := map[string]string{
		"Tunesic":   "tunesic",
		"My App!":   "myapp",
		"G-Turbo":   "gturbo",
		"":          "app",
		"123":       "app123",
		"2Fast4You": "app2fast4you",
	}
	for in, want := range cases {
		if got := LowerAlnum(in); got != want {
			t.Errorf("LowerAlnum(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidatePackage(t *testing.T) {
	valid := []string{"com.example.app", "io.kontour.tunesic", "a.b"}
	for _, v := range valid {
		if err := ValidatePackage(v); err != nil {
			t.Errorf("ValidatePackage(%q) = %v, want nil", v, err)
		}
	}

	invalid := []string{
		"",             // empty
		"single",       // one segment
		"Com.Example",  // uppercase
		"com..example", // empty segment
	}
	for _, v := range invalid {
		if err := ValidatePackage(v); err == nil {
			t.Errorf("ValidatePackage(%q) = nil, want an error", v)
		}
	}

	// Kotlin keywords cannot be package segments.
	if err := ValidatePackage("com.example.object"); err == nil {
		t.Error("a Kotlin keyword segment should be rejected")
	}
}

func TestValidateFeatureName(t *testing.T) {
	for _, v := range []string{"billing", "firmware-update", "a1"} {
		if err := ValidateFeatureName(v); err != nil {
			t.Errorf("ValidateFeatureName(%q) = %v, want nil", v, err)
		}
	}
	for _, v := range []string{"", "Billing", "firmware_update", "-lead", "trail-"} {
		if err := ValidateFeatureName(v); err == nil {
			t.Errorf("ValidateFeatureName(%q) = nil, want an error", v)
		}
	}
}

// `add` should work from anywhere inside the project, not just its root.
func TestLoadManifestWalksUp(t *testing.T) {
	dir := t.TempDir()
	manifest := Manifest{Schema: ManifestSchema, Project: Project{Name: "Tunesic"}}
	if err := manifest.Save(dir); err != nil {
		t.Fatal(err)
	}

	nested := dir + "/feature/home/impl"
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	loaded, root, err := LoadManifest(nested)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Project.Name != "Tunesic" || root != dir {
		t.Errorf("walked to %q (%q), want %q", root, loaded.Project.Name, dir)
	}
}
