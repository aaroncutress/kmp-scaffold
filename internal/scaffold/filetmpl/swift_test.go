package filetmpl_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaroncutress/kmp-scaffold/internal/render"
	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
)

// swiftPackage builds a real git repository with real tags and returns its
// file:// URL. `git ls-remote` treats a local path exactly like a remote one,
// so the Swift probe can be tested end to end without a network.
func swiftPackage(t *testing.T, tags ...string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL="+filepath.Join(dir, "nonexistent"),
			"GIT_CONFIG_SYSTEM="+filepath.Join(dir, "nonexistent-system"),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
			"GIT_TERMINAL_PROMPT=0",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "Package.swift"), []byte("// swift-tools-version:5.9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "Initial commit")
	for _, tag := range tags {
		run("tag", tag)
	}
	return dir
}

// swiftTemplate is a minimal file-based template whose only job is to write the
// resolved version out, so the test can read what the resolver decided.
func swiftTemplate(t *testing.T, manifest string) string {
	t.Helper()
	return write(t, map[string]string{
		"template.toml":     manifest,
		"files/version.txt": "{{ .Versions.swiftpkg }}\n",
	})
}

const swiftFiles = `
[[files]]
from = "files/version.txt"
to = "version.txt"
`

// A template author gets Swift resolution by naming the repository as a probe:
// no network, no special casing, the same shape as a Maven one.
func TestSwiftProbeResolvesFromTags(t *testing.T) {
	repo := swiftPackage(t, "v0.9.0", "1.0.0", "1.2.0", "1.10.0", "2.0.0-beta.1")

	dir := swiftTemplate(t, `
schema = 1

[template]
id = "swiftprobe"
name = "Swift probe"

[versions]
channel = "stable"

[[versions.probe]]
key = "swiftpkg"
group = "`+filepath.ToSlash(filepath.Dir(repo))+`"
artifact = "`+filepath.Base(repo)+`"
repo = "swift"
baseline = "0.0.1"
`+swiftFiles)

	// Not offline: this probe reaches a local repository, not the internet.
	got := strings.TrimSpace(resolveVersion(t, dir))
	if got != "1.10.0" {
		t.Errorf("resolved %q, want 1.10.0 - the highest stable tag, with the v stripped", got)
	}
}

// The channel applies to tags exactly as it does to Maven versions.
func TestSwiftProbeHonoursTheChannel(t *testing.T) {
	repo := swiftPackage(t, "1.0.0", "2.0.0-beta.1")

	dir := swiftTemplate(t, `
schema = 1

[template]
id = "swiftprobe"
name = "Swift probe"

[versions]
channel = "preview"

[[versions.probe]]
key = "swiftpkg"
group = "`+filepath.ToSlash(filepath.Dir(repo))+`"
artifact = "`+filepath.Base(repo)+`"
repo = "swift"
baseline = "0.0.1"
`+swiftFiles)

	if got := strings.TrimSpace(resolveVersion(t, dir)); got != "2.0.0-beta.1" {
		t.Errorf("resolved %q, want the beta on the preview channel", got)
	}
}

// A repository that is not there falls back to the baseline rather than failing
// the run - the same as an unreachable Maven coordinate.
func TestSwiftProbeFallsBackToTheBaseline(t *testing.T) {
	dir := swiftTemplate(t, `
schema = 1

[template]
id = "swiftprobe"
name = "Swift probe"

[versions]
channel = "stable"

[[versions.probe]]
key = "swiftpkg"
group = "`+filepath.ToSlash(t.TempDir())+`"
artifact = "does-not-exist"
repo = "swift"
baseline = "0.0.1"
`+swiftFiles)

	if got := strings.TrimSpace(resolveVersion(t, dir)); got != "0.0.1" {
		t.Errorf("resolved %q, want the baseline", got)
	}
}

// resolveVersion generates the template and returns what it wrote. It runs the
// resolver online, because the probe points at the local filesystem.
func resolveVersion(t *testing.T, dir string) string {
	t.Helper()
	tmpl := open(t, dir)

	a := tmpl.NewAnswers()
	a.Project.Name = "Thing"
	a.Project.Dir = t.TempDir()

	req := tmpl.Versions(a)
	if req.Empty() {
		t.Fatal("the template declares a probe but asks for no resolution")
	}
	res := resolve.Run(context.Background(), req)

	root := a.Project.Dir
	writer := render.NewWriter(root, false, false)
	if _, err := tmpl.Generate(context.Background(), scaffold.GenRequest{
		Answers: a, Result: res, Writer: writer, Version: "test",
	}); err != nil {
		t.Fatalf("generating: %v", err)
	}
	return read(t, root, "version.txt")
}

// An omitted min_channel is not a channel. ParseChannel answers "preview" for
// anything it does not recognise, so reading it unconditionally would put every
// probe in every file-based template on the preview track.
func TestProbeWithoutAMinChannelStaysOnTheRequestedChannel(t *testing.T) {
	repo := swiftPackage(t, "1.0.0", "2.0.0-beta.1")

	dir := swiftTemplate(t, `
schema = 1

[template]
id = "swiftprobe"
name = "Swift probe"

[versions]
channel = "stable"

[[versions.probe]]
key = "swiftpkg"
group = "`+filepath.ToSlash(filepath.Dir(repo))+`"
artifact = "`+filepath.Base(repo)+`"
repo = "swift"
baseline = "0.0.1"
`+swiftFiles)

	if got := strings.TrimSpace(resolveVersion(t, dir)); got != "1.0.0" {
		t.Errorf("resolved %q, want 1.0.0 - the template asked for stable and set no floor", got)
	}
}

// A floor that is set still raises the channel for that key alone.
func TestProbeMinChannelRaisesTheFloor(t *testing.T) {
	repo := swiftPackage(t, "1.0.0", "2.0.0-beta.1")

	dir := swiftTemplate(t, `
schema = 1

[template]
id = "swiftprobe"
name = "Swift probe"

[versions]
channel = "stable"

[[versions.probe]]
key = "swiftpkg"
group = "`+filepath.ToSlash(filepath.Dir(repo))+`"
artifact = "`+filepath.Base(repo)+`"
repo = "swift"
baseline = "0.0.1"
min_channel = "preview"
`+swiftFiles)

	if got := strings.TrimSpace(resolveVersion(t, dir)); got != "2.0.0-beta.1" {
		t.Errorf("resolved %q, want the beta - min_channel raises this key to preview", got)
	}
}
