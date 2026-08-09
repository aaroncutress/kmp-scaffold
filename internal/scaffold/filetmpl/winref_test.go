package filetmpl_test

import (
	"strings"
	"testing"

	"github.com/aaroncutress/kmp-scaffold/internal/scaffold/filetmpl"
)

// A local repository can be named the way each platform names paths. Windows
// is the one that forces the issue: a drive letter is fine in a file:// path
// and illegal in a directory name, so it cannot reach the cache unchanged.
func TestFileRefsAcceptWindowsPaths(t *testing.T) {
	for _, in := range []string{
		`file://C:\Users\a\src\tmpl`,
		`file:///C:/Users/a/src/tmpl`,
	} {
		ref, ok := filetmpl.ParseRef(in)
		if !ok {
			t.Fatalf("ParseRef(%q) did not parse", in)
		}
		if ref.URL != "file:///C:/Users/a/src/tmpl" {
			t.Errorf("ParseRef(%q).URL = %q, want the three-slash form git wants", in, ref.URL)
		}
		if strings.ContainsAny(ref.Slug(), `:\`) {
			t.Errorf("ParseRef(%q).Slug() = %q, which is not a legal directory name", in, ref.Slug())
		}
	}
}

// The same goes for a repository path with a colon in it, whatever the host.
func TestSlugIsAlwaysALegalPath(t *testing.T) {
	for _, in := range []string{
		`github:acme:evil/repo`,
		`file://C:\src\tmpl`,
		`https://git.example.com/team/templates.git`,
	} {
		ref, ok := filetmpl.ParseRef(in)
		if !ok {
			t.Fatalf("ParseRef(%q) did not parse", in)
		}
		if strings.ContainsAny(ref.Slug(), `<>:"\|?*`) {
			t.Errorf("ParseRef(%q).Slug() = %q, which some filesystem will refuse", in, ref.Slug())
		}
	}
}

// A subdirectory is still found past the path's own leading slashes.
func TestFileRefsKeepTheirSubdirectory(t *testing.T) {
	ref, ok := filetmpl.ParseRef(`file:///srv/templates//services/ktor@v2`)
	if !ok {
		t.Fatal("the ref did not parse")
	}
	if ref.Subdir != "services/ktor" {
		t.Errorf("subdir = %q, want services/ktor", ref.Subdir)
	}
	if ref.Rev != "v2" {
		t.Errorf("rev = %q, want v2", ref.Rev)
	}
	if ref.URL != "file:///srv/templates" {
		t.Errorf("url = %q", ref.URL)
	}
}
