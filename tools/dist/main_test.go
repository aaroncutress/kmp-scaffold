package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These run on every platform CI covers, which is the point: the archiving used
// to be tar and zip in a shell block, and could only be exercised where those
// existed. Testing the Go directly is both faster and truer than cross-
// compiling six targets on three runners to find out.

// staged is a release directory as build() leaves it: a binary and a README.
func staged(t *testing.T, name, exe string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for file, body := range map[string]string{
		exe:         "a binary",
		"README.md": "docs",
	} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The binary has to unpack executable. Nothing else does - and on Windows the
// staged files carry no executable bit at all, so this cannot be read off the
// filesystem; the archive has to assert it.
func TestTarCarriesTheExecutableBit(t *testing.T) {
	const name = "kmp-scaffold_v1_linux_amd64"
	dir := staged(t, name, "kmp-scaffold")

	path := filepath.Join(t.TempDir(), name+".tar.gz")
	if err := writeTarGz(dir, name, path); err != nil {
		t.Fatal(err)
	}

	modes := map[string]int64{}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		modes[h.Name] = h.Mode
		if h.ModTime.IsZero() {
			t.Errorf("%s has no modification time", h.Name)
		}
	}

	if got := modes[name+"/kmp-scaffold"]; got != 0o755 {
		t.Errorf("the binary is mode %o, want 755", got)
	}
	if got := modes[name+"/README.md"]; got != 0o644 {
		t.Errorf("README.md is mode %o, want 644", got)
	}
	// Everything under one directory, so unpacking does not scatter files.
	for member := range modes {
		if !strings.HasPrefix(member, name+"/") {
			t.Errorf("%q is not under the release directory", member)
		}
	}
}

func TestZipCarriesTheExecutableBit(t *testing.T) {
	const name = "kmp-scaffold_v1_windows_amd64"
	dir := staged(t, name, "kmp-scaffold.exe")

	path := filepath.Join(t.TempDir(), name+".zip")
	if err := writeZip(dir, name, path); err != nil {
		t.Fatal(err)
	}

	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	found := map[string]os.FileMode{}
	for _, f := range r.File {
		found[f.Name] = f.Mode().Perm()
		// Zip cannot represent a time before 1980, and a zero one renders as
		// "1980-00-00" in every listing tool.
		if f.Modified.Year() < 1980 {
			t.Errorf("%s has an unrepresentable modification time: %v", f.Name, f.Modified)
		}
	}
	if got := found[name+"/kmp-scaffold.exe"]; got != 0o755 {
		t.Errorf("the binary is mode %o, want 755", got)
	}
	if got := found[name+"/README.md"]; got != 0o644 {
		t.Errorf("README.md is mode %o, want 644", got)
	}
}

// A checksum file that includes itself is the classic way to get this wrong,
// and it happened here once already with the shell version.
func TestChecksumsCoverTheArchivesAndNotThemselves(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatal(err)
	}

	var archives []string
	bodies := map[string]string{"one.tar.gz": "first", "two.zip": "second"}
	for name, body := range bodies {
		path := filepath.Join(distDir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		archives = append(archives, path)
	}

	if err := writeChecksums(archives); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(distDir, "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(got)), "\n")
	if len(lines) != len(bodies) {
		t.Fatalf("checksums.txt has %d lines, want one per archive:\n%s", len(lines), got)
	}
	if strings.Contains(string(got), "checksums.txt") {
		t.Error("checksums.txt hashes itself")
	}

	// The format is the one `sha256sum -c` reads: hash, two spaces, base name.
	for _, line := range lines {
		hash, name, ok := strings.Cut(line, "  ")
		if !ok {
			t.Fatalf("%q is not `<hash>  <name>`", line)
		}
		want := sha256.Sum256([]byte(bodies[name]))
		if hash != hex.EncodeToString(want[:]) {
			t.Errorf("%s: hash is %s, want %s", name, hash, hex.EncodeToString(want[:]))
		}
		if filepath.Base(name) != name {
			t.Errorf("%q should be a bare name, not a path", name)
		}
	}
}

// Member names are archive paths, not OS paths, so they are slash-separated
// wherever the archive was built.
func TestMemberNamesUseSlashes(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "sub", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var seen []string
	if err := walk(dir, func(rel string, _ []byte, _ os.FileInfo) error {
		seen = append(seen, rel)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if len(seen) != 1 || seen[0] != "sub/deep/file.txt" {
		t.Errorf("walked %v, want [sub/deep/file.txt]", seen)
	}
	for _, rel := range seen {
		if strings.Contains(rel, `\`) {
			t.Errorf("%q contains a backslash", rel)
		}
	}
}

func TestExecutableOnlyTheBinary(t *testing.T) {
	for _, c := range []struct {
		name string
		want bool
	}{
		{"kmp-scaffold", true},
		{"kmp-scaffold.exe", true},
		{"README.md", false},
		{"LICENSE", false},
	} {
		if got := executable(c.name); got != c.want {
			t.Errorf("executable(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

// -clean removes the build output and does not mind it being absent already.
func TestCleanIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.MkdirAll(filepath.Join(distDir, "staging"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("kmp-scaffold", []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if err := cleanUp("kmp-scaffold"); err != nil {
			t.Fatalf("clean: %v", err)
		}
	}
	if _, err := os.Stat(distDir); !os.IsNotExist(err) {
		t.Error("dist/ survived")
	}
}
