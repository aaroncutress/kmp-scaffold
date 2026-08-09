package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()

	type vars struct {
		AndroidLayout string   `json:"androidLayout"`
		RootTabs      []string `json:"rootTabs"`
	}

	feature, err := NewFeature("home", map[string]any{"rootTab": true})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := NewManifest("test",
		TemplateRef{ID: "kmp-mobile", Source: "builtin"},
		Project{Name: "Tunesic", Package: "io.kontour.tunesic", ApplicationID: "io.kontour.tunesic"},
		vars{AndroidLayout: "nav3-shell", RootTabs: []string{"Home", "Settings"}},
		[]Feature{feature})
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.Save(dir); err != nil {
		t.Fatal(err)
	}

	loaded, root, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if root != dir {
		t.Errorf("root = %q, want %q", root, dir)
	}
	if loaded.Project.Name != "Tunesic" || loaded.Template.ID != "kmp-mobile" {
		t.Errorf("round trip lost data: %+v", loaded)
	}

	var got vars
	if err := loaded.DecodeVars(&got); err != nil {
		t.Fatal(err)
	}
	if got.AndroidLayout != "nav3-shell" || len(got.RootTabs) != 2 {
		t.Errorf("vars = %+v, want the layout and both tabs", got)
	}

	if f := loaded.FindFeature("home"); f == nil {
		t.Error("FindFeature(home) found nothing")
	}
	if loaded.Migrated {
		t.Error("a current manifest should not be marked as migrated")
	}
}

// A project generated before templates existed still has to work. This is the
// exact file the tool used to write.
const schema1Manifest = `{
  "schema": 1,
  "generatorVersion": "dev",
  "name": "Tunesic",
  "package": "io.kontour.tunesic",
  "applicationId": "io.kontour.tunesic",
  "android": true,
  "ios": true,
  "androidLayout": "nav3-shell",
  "iosLayout": "swiftui-features",
  "sharedUtils": ["network-observer", "settings"],
  "androidExtras": ["connectivity-banner"],
  "packs": ["images", "database", "secrets"],
  "rootTabs": ["Home", "Settings"],
  "features": [
    {
      "name": "home",
      "android": true,
      "shared": false,
      "ios": true,
      "presentation": "shell",
      "rootTab": true
    },
    {
      "name": "billing",
      "android": true,
      "shared": true,
      "presentation": "above-nav",
      "rootTab": false
    }
  ]
}
`

func TestSchema1MigratesLosslessly(t *testing.T) {
	m, err := ParseManifest([]byte(schema1Manifest))
	if err != nil {
		t.Fatalf("migrating: %v", err)
	}

	if m.Schema != ManifestSchema {
		t.Errorf("schema = %d, want %d", m.Schema, ManifestSchema)
	}
	if !m.Migrated {
		t.Error("a migrated manifest should say so, so a command can report the upgrade")
	}
	if m.Template.ID != BuiltinKMPTemplate || m.Template.Source != "builtin" {
		t.Errorf("template = %+v, want the built-in KMP template", m.Template)
	}
	if m.GeneratorVer != "dev" {
		t.Errorf("generatorVersion = %q, want dev", m.GeneratorVer)
	}
	if m.Project.Name != "Tunesic" || m.Project.Package != "io.kontour.tunesic" ||
		m.Project.ApplicationID != "io.kontour.tunesic" {
		t.Errorf("project = %+v", m.Project)
	}

	// Every structural field becomes a var, under the same name.
	var vars struct {
		Android       bool     `json:"android"`
		IOS           bool     `json:"ios"`
		AndroidLayout string   `json:"androidLayout"`
		IOSLayout     string   `json:"iosLayout"`
		SharedUtils   []string `json:"sharedUtils"`
		AndroidExtras []string `json:"androidExtras"`
		Packs         []string `json:"packs"`
		RootTabs      []string `json:"rootTabs"`
	}
	if err := m.DecodeVars(&vars); err != nil {
		t.Fatal(err)
	}
	switch {
	case !vars.Android || !vars.IOS:
		t.Errorf("platforms lost: %+v", vars)
	case vars.AndroidLayout != "nav3-shell", vars.IOSLayout != "swiftui-features":
		t.Errorf("layouts lost: %+v", vars)
	case len(vars.SharedUtils) != 2, len(vars.AndroidExtras) != 1, len(vars.Packs) != 3:
		t.Errorf("selections lost: %+v", vars)
	case len(vars.RootTabs) != 2 || vars.RootTabs[0] != "Home":
		t.Errorf("root tabs lost: %+v", vars)
	}

	// Features keep their names and everything recorded about them.
	if len(m.Features) != 2 {
		t.Fatalf("got %d features, want 2", len(m.Features))
	}
	var home struct {
		Android      bool   `json:"android"`
		Shared       bool   `json:"shared"`
		IOS          bool   `json:"ios"`
		Presentation string `json:"presentation"`
		RootTab      bool   `json:"rootTab"`
	}
	f := m.FindFeature("home")
	if f == nil {
		t.Fatal("the home feature was lost")
	}
	if err := f.DecodeVars(&home); err != nil {
		t.Fatal(err)
	}
	if !home.Android || !home.IOS || !home.RootTab || home.Presentation != "shell" {
		t.Errorf("home = %+v, want an iOS-backed root tab", home)
	}
	if home.Shared {
		t.Error("home was not a shared feature; the migration invented one")
	}
}

// Reading a project must never rewrite it: a schema-2 file an older
// kmp-scaffold cannot open should only appear once the user asks for a change.
func TestLoadDoesNotRewriteAnOldManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ManifestFile)
	if err := os.WriteFile(path, []byte(schema1Manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := LoadManifest(dir); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != schema1Manifest {
		t.Errorf("loading rewrote the manifest:\n%s", after)
	}
}

func TestManifestRejectsANewerSchema(t *testing.T) {
	data, err := json.Marshal(map[string]any{"schema": ManifestSchema + 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseManifest(data); err == nil {
		t.Error("a manifest from a newer kmp-scaffold should be refused, not guessed at")
	}
}

func TestPutFeatureReplacesByName(t *testing.T) {
	m := Manifest{}
	first, _ := NewFeature("billing", map[string]any{"shared": false})
	second, _ := NewFeature("billing", map[string]any{"shared": true})

	m.PutFeature(first)
	m.PutFeature(second)

	if len(m.Features) != 1 {
		t.Fatalf("got %d features, want the second to replace the first", len(m.Features))
	}
	var got struct {
		Shared bool `json:"shared"`
	}
	if err := m.Features[0].DecodeVars(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Shared {
		t.Error("the replacement did not take")
	}
}
