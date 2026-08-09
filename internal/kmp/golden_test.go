package kmp_test

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// The golden test is the safety net for refactors that are meant to change no
// output at all. It fingerprints every generated file, so a stray whitespace
// change in a template or a reordered resolver pass shows up as a diff rather
// than as a build failure in someone's IDE three weeks later.
//
// When output is *meant* to change, read the diff, satisfy yourself that every
// line of it was intended, then re-run with -update.
func TestGoldenProjects(t *testing.T) {
	cases := []struct {
		name string
		spec func() model.Spec
	}{
		{"full", func() model.Spec { return testSpec("tunesic") }},
		{"ios-simple", func() model.Spec {
			spec := testSpec("tunesic")
			spec.IOSLayout = "swiftui-simple"
			catalog.Normalise(&spec)
			return spec
		}},
		{"android-only", func() model.Spec {
			spec := testSpec("tunesic")
			spec.IOS = false
			spec.Packs = nil
			spec.SharedUtils = nil
			spec.AndroidExtras = nil
			catalog.Normalise(&spec)
			return spec
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fingerprint(t, generate(t, tc.spec()))
			golden := filepath.Join("testdata", "golden-"+tc.name+".txt")

			if *update {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}

			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("reading the golden file (run `go test ./internal/kmp -update` to create it): %v", err)
			}
			if got != string(want) {
				t.Errorf("the generated project differs from %s:\n%s\n\nIf every change was intended, re-run with -update.",
					golden, diffLines(string(want), got))
			}
		})
	}
}

// fingerprint is one line per file: the path, then a hash of its contents.
// Hashing rather than embedding keeps the golden files reviewable while still
// catching a one-character change anywhere in 200 files.
func fingerprint(t *testing.T, root string) string {
	t.Helper()

	var lines []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		lines = append(lines, fmt.Sprintf("%s  %s", filepath.ToSlash(rel), hex.EncodeToString(sum[:])[:16]))
		return nil
	})
	if err != nil {
		t.Fatalf("walking the generated project: %v", err)
	}

	// The manifest records the generator version, which is "test" here, so it
	// is stable - but sort so the order does not depend on the filesystem.
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

// diffLines reports only what changed, because a whole-tree dump buries it.
func diffLines(want, got string) string {
	index := func(s string) map[string]string {
		m := map[string]string{}
		for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
			if path, sum, ok := strings.Cut(line, "  "); ok {
				m[path] = sum
			}
		}
		return m
	}
	a, b := index(want), index(got)

	var out []string
	for path, sum := range a {
		switch other, ok := b[path]; {
		case !ok:
			out = append(out, "  - "+path+" (no longer generated)")
		case other != sum:
			out = append(out, "  ~ "+path+" (contents changed)")
		}
	}
	for path := range b {
		if _, ok := a[path]; !ok {
			out = append(out, "  + "+path+" (newly generated)")
		}
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}
