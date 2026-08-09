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

// A cache key becomes a directory, and git then writes its own deep paths
// inside it - an object pack's .keep file is another 60 characters on top.
// Windows refuses a path over 260, so a key that embedded a whole source path
// blew the limit before git had started. This is what that looked like:
//
//	.../cache/file/C__Users_a_AppData_Local_Temp_TestNameNNNNN/.fetch-NNNN/repo/
//	    .git/objects/pack/pack-<40 hex>.keep
func TestSlugComponentsAreBounded(t *testing.T) {
	// A file:// ref of the shape a temp directory produces: the whole parent
	// path becomes one component.
	deep := `file:///C:/Users/somebody/AppData/Local/Temp/` +
		`TestCachedTemplatesAreDiscovered3319318861/004`
	ref, ok := filetmpl.ParseRef(deep)
	if !ok {
		t.Fatalf("ParseRef(%q) did not parse", deep)
	}

	for _, part := range strings.Split(ref.Slug(), "/") {
		if len(part) > 40 {
			t.Errorf("slug component %q is %d characters; a cache path built from "+
				"it will not fit in Windows' 260-character limit", part, len(part))
		}
	}
}

// Bounding a component must not merge two sources into one cache entry. The
// tail is kept because it is the distinctive part of a path, and a hash of the
// whole is what actually separates them.
func TestBoundedSlugsStayDistinct(t *testing.T) {
	base := `file:///C:/Users/somebody/AppData/Local/VeryLongDirectoryNameIndeed/`

	seen := map[string]string{}
	for _, suffix := range []string{"alpha", "beta", "gamma"} {
		ref, ok := filetmpl.ParseRef(base + suffix + "/repo")
		if !ok {
			t.Fatalf("ParseRef did not parse %q", base+suffix)
		}
		slug := ref.Slug()
		if other, clash := seen[slug]; clash {
			t.Errorf("%q and %q share the cache key %q", suffix, other, slug)
		}
		seen[slug] = suffix
	}

	// Two paths differing only where the truncation cuts must still differ.
	long := `file:///C:/` + strings.Repeat("a", 60)
	one, _ := filetmpl.ParseRef(long + "1/repo")
	two, _ := filetmpl.ParseRef(long + "2/repo")
	if one.Slug() == two.Slug() {
		t.Errorf("two sources collapsed onto %q", one.Slug())
	}
}

// An ordinary ref is nowhere near the bound, so its cache key is unchanged by
// it - the same directory an older build used.
func TestOrdinaryRefsAreUntouchedByTheBound(t *testing.T) {
	ref, ok := filetmpl.ParseRef("github:acme/templates")
	if !ok {
		t.Fatal("the ref did not parse")
	}
	if got, want := ref.Slug(), "github.com/acme/templates"; got != want {
		t.Errorf("slug = %q, want %q", got, want)
	}
}
