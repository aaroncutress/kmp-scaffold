package filetmpl_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold/filetmpl"
)

func TestParseRef(t *testing.T) {
	for _, tc := range []struct {
		in     string
		host   string
		owner  string
		repo   string
		subdir string
		rev    string
		url    string
	}{
		{in: "github:acme/templates",
			host: "github.com", owner: "acme", repo: "templates",
			url: "https://github.com/acme/templates.git"},

		{in: "github:acme/templates@v2.1",
			host: "github.com", owner: "acme", repo: "templates", rev: "v2.1",
			url: "https://github.com/acme/templates.git"},

		{in: "github:acme/templates/services/ktor@main",
			host: "github.com", owner: "acme", repo: "templates",
			subdir: "services/ktor", rev: "main",
			url: "https://github.com/acme/templates.git"},

		{in: "gitlab:acme/templates",
			host: "gitlab.com", owner: "acme", repo: "templates",
			url: "https://gitlab.com/acme/templates.git"},

		// The double slash separates the repository from a directory in it; a
		// single one would be ambiguous with the repository path.
		{in: "https://git.example.com/team/templates.git//service@v2",
			host: "git.example.com", owner: "team", repo: "templates",
			subdir: "service", rev: "v2",
			// The clone URL keeps its .git; only the repository name drops it.
			url: "https://git.example.com/team/templates.git"},

		{in: "https://git.example.com/team/templates.git",
			host: "git.example.com", owner: "team", repo: "templates",
			url: "https://git.example.com/team/templates.git"},

		{in: "git@github.com:acme/templates.git@v2",
			host: "github.com", owner: "acme", repo: "templates", rev: "v2",
			url: "git@github.com:acme/templates.git"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			ref, ok := filetmpl.ParseRef(tc.in)
			if !ok {
				t.Fatalf("ParseRef(%q) did not parse", tc.in)
			}
			if ref.Host != tc.host || ref.Owner != tc.owner || ref.Repo != tc.repo {
				t.Errorf("repository = %s/%s/%s, want %s/%s/%s",
					ref.Host, ref.Owner, ref.Repo, tc.host, tc.owner, tc.repo)
			}
			if ref.Subdir != tc.subdir {
				t.Errorf("subdir = %q, want %q", ref.Subdir, tc.subdir)
			}
			if ref.Rev != tc.rev {
				t.Errorf("rev = %q, want %q", ref.Rev, tc.rev)
			}
			if ref.URL != tc.url {
				t.Errorf("url = %q, want %q", ref.URL, tc.url)
			}
			if ref.Raw != tc.in {
				t.Errorf("raw = %q, want what was typed", ref.Raw)
			}
		})
	}
}

func TestParseRefRejectsWhatIsNotARef(t *testing.T) {
	for _, in := range []string{"", "kmp-mobile", "./dir", "/abs/dir", "github:acme", "https://"} {
		if _, ok := filetmpl.ParseRef(in); ok {
			t.Errorf("ParseRef(%q) should not parse", in)
		}
	}
}

// Two repositories on disk can end in the same name, so the whole path is the
// cache key rather than the last segment.
func TestFileRefsAreKeyedByWholePath(t *testing.T) {
	a, _ := filetmpl.ParseRef("file:///srv/one/templates")
	b, _ := filetmpl.ParseRef("file:///srv/two/templates")
	if a.Slug() == b.Slug() {
		t.Errorf("both slugs are %q; two different repositories would share a cache", a.Slug())
	}
}

// ---------------------------------------------------------------------------
// Fetching
// ---------------------------------------------------------------------------

// gitRepo makes a real repository from the example template, so the fetch path
// is exercised end to end without a network.
func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}

	dir := t.TempDir()
	if err := copyTree(exampleDir, filepath.Join(dir, "ktor-service")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"add", "-A"},
		{"commit", "-qm", "the template"},
		{"tag", "v1"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func copyTree(from, to string) error {
	return filepath.Walk(from, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, p)
		if err != nil {
			return err
		}
		dest := filepath.Join(to, rel)
		if info.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, info.Mode())
	})
}

// isolate points the cache and the trust store at this test's own directories,
// so nothing touches the machine running it.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("KMP_SCAFFOLD_CACHE", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("KMP_SCAFFOLD_TRUST_FILE", filepath.Join(t.TempDir(), "trusted.json"))
	t.Setenv("KMP_SCAFFOLD_TEMPLATES", filepath.Join(t.TempDir(), "templates"))
	filetmpl.TrustEverything(false)
	filetmpl.SetTrustPrompt(nil)
	filetmpl.SetFetchOptions(filetmpl.FetchOptions{})
	t.Cleanup(func() {
		filetmpl.TrustEverything(false)
		filetmpl.SetTrustPrompt(nil)
	})
}

func TestFetchClonesAndCaches(t *testing.T) {
	isolate(t)
	repo := gitRepo(t)
	filetmpl.TrustEverything(true)

	ref, ok := filetmpl.ParseRef("file://" + repo + "//ktor-service")
	if !ok {
		t.Fatal("the ref did not parse")
	}

	tmpl, err := filetmpl.OpenRemote(ref, filetmpl.FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}

	meta := tmpl.Meta()
	if meta.ID != "ktor-service" {
		t.Errorf("id = %q", meta.ID)
	}
	if meta.Source.Kind != scaffold.SourceRemote {
		t.Errorf("source = %v, want remote", meta.Source.Kind)
	}
	if len(meta.Source.Revision) != 40 {
		t.Errorf("revision = %q, want a full commit", meta.Source.Revision)
	}

	// The second call is served from the cache, which is what makes generating
	// twice cheap - and is checked by removing the repository first.
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	again, err := filetmpl.OpenRemote(ref, filetmpl.FetchOptions{})
	if err != nil {
		t.Fatalf("the second call did not come from the cache: %v", err)
	}
	if again.Meta().Source.Revision != meta.Source.Revision {
		t.Error("the cached copy is a different commit")
	}
}

// A cached template is offered by its own id, and listed as fetched.
func TestCachedTemplatesAreDiscovered(t *testing.T) {
	isolate(t)
	repo := gitRepo(t)
	filetmpl.TrustEverything(true)

	ref, _ := filetmpl.ParseRef("file://" + repo + "//ktor-service")
	if _, err := filetmpl.OpenRemote(ref, filetmpl.FetchOptions{}); err != nil {
		t.Fatal(err)
	}

	cached := filetmpl.CachedTemplates()
	if len(cached) != 1 {
		t.Fatalf("CachedTemplates() = %d entries, want 1", len(cached))
	}
	if cached[0].Template.Meta().ID != "ktor-service" {
		t.Errorf("cached id = %q", cached[0].Template.Meta().ID)
	}

	found := filetmpl.Loader{}.Discover()
	if len(found) != 1 || found[0].Meta().ID != "ktor-service" {
		t.Errorf("Discover() = %+v, want the fetched template", found)
	}

	// And by name, which is what `--template ktor-service` goes through.
	byName, err := filetmpl.Loader{}.Load("ktor-service")
	if err != nil || byName == nil {
		t.Fatalf("Load(ktor-service) = %v, %v", byName, err)
	}

	// Removing it takes both the tree and the record that it was reviewed.
	if err := filetmpl.Forget(cached[0].Ref); err != nil {
		t.Fatal(err)
	}
	if err := filetmpl.ForgetTrust(cached[0].Ref.Slug()); err != nil {
		t.Fatal(err)
	}
	if got := filetmpl.CachedTemplates(); len(got) != 0 {
		t.Errorf("CachedTemplates() = %d entries after removing, want 0", len(got))
	}
	if filetmpl.IsTrusted(cached[0].Ref.Slug(), cached[0].Revision) {
		t.Error("removing a template should forget that it was reviewed")
	}
}

// ---------------------------------------------------------------------------
// Trust
// ---------------------------------------------------------------------------

func TestRemoteTemplateNeedsTrust(t *testing.T) {
	isolate(t)
	repo := gitRepo(t)
	ref, _ := filetmpl.ParseRef("file://" + repo + "//ktor-service")

	// With no prompt installed and no --trust, it is refused rather than
	// silently used.
	if _, err := filetmpl.OpenRemote(ref, filetmpl.FetchOptions{}); err == nil {
		t.Fatal("an unreviewed remote template should not be used")
	} else if !strings.Contains(err.Error(), "--trust") {
		t.Errorf("the error should say how to proceed: %v", err)
	}

	// Saying no is not an error in itself, but it does stop the command.
	filetmpl.SetTrustPrompt(func(filetmpl.Trust) (bool, error) { return false, nil })
	if _, err := filetmpl.OpenRemote(ref, filetmpl.FetchOptions{}); err == nil {
		t.Fatal("declining should stop the command")
	}

	// Saying yes remembers the commit, so the next use asks nothing.
	asked := 0
	var shown filetmpl.Trust
	filetmpl.SetTrustPrompt(func(req filetmpl.Trust) (bool, error) {
		asked++
		shown = req
		return true, nil
	})
	if _, err := filetmpl.OpenRemote(ref, filetmpl.FetchOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := filetmpl.OpenRemote(ref, filetmpl.FetchOptions{}); err != nil {
		t.Fatal(err)
	}
	if asked != 1 {
		t.Errorf("asked %d times, want once - the answer is remembered", asked)
	}

	// What the user is shown is what decides the answer, so it has to be real.
	if shown.ID != "ktor-service" || shown.Files == 0 || len(shown.Revision) != 40 {
		t.Errorf("the prompt was not told enough to decide on: %+v", shown)
	}
	if !slices.Contains(shown.Recipes, "route") || shown.Edits == 0 {
		t.Errorf("the prompt does not mention what it can add: %+v", shown)
	}
}

// A different commit of the same repository is a different set of files, so it
// is asked about again.
func TestTrustIsPerCommit(t *testing.T) {
	isolate(t)
	repo := gitRepo(t)
	ref, _ := filetmpl.ParseRef("file://" + repo + "//ktor-service")

	asked := 0
	filetmpl.SetTrustPrompt(func(filetmpl.Trust) (bool, error) { asked++; return true, nil })

	if _, err := filetmpl.OpenRemote(ref, filetmpl.FetchOptions{}); err != nil {
		t.Fatal(err)
	}

	// Move the template on.
	manifest := filepath.Join(repo, "ktor-service", "template.toml")
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, append(data, []byte("\n# changed\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", "change it"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	if _, err := filetmpl.OpenRemote(ref, filetmpl.FetchOptions{Refresh: true}); err != nil {
		t.Fatal(err)
	}
	if asked != 2 {
		t.Errorf("asked %d times, want twice - a new commit is a new decision", asked)
	}
}

// --offline never reaches for the network, and says so.
func TestOfflineRefusesAnUncachedTemplate(t *testing.T) {
	isolate(t)
	repo := gitRepo(t)
	filetmpl.TrustEverything(true)

	ref, _ := filetmpl.ParseRef("file://" + repo + "//ktor-service")
	if _, err := filetmpl.OpenRemote(ref, filetmpl.FetchOptions{Offline: true}); err == nil {
		t.Fatal("an uncached template should not be fetched offline")
	} else if !strings.Contains(err.Error(), "offline") {
		t.Errorf("the error should say why: %v", err)
	}

	// Once cached, offline works.
	if _, err := filetmpl.OpenRemote(ref, filetmpl.FetchOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := filetmpl.OpenRemote(ref, filetmpl.FetchOptions{Offline: true}); err != nil {
		t.Errorf("a cached template should work offline: %v", err)
	}
}

// A project records the commit it was generated from, and keeps using it: the
// template moving on must not change what `add` does to an existing project.
func TestAProjectStaysOnItsOwnCommit(t *testing.T) {
	isolate(t)
	repo := gitRepo(t)
	filetmpl.TrustEverything(true)

	ref, _ := filetmpl.ParseRef("file://" + repo + "//ktor-service")
	tmpl, err := filetmpl.OpenRemote(ref, filetmpl.FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	first := tmpl.Meta().Source.Revision

	a := tmpl.NewAnswers()
	a.Project = model.Project{Name: "Orders API", Package: "com.example.orders"}
	root := generate(t, tmpl, a)

	manifest, _, err := model.LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Template.Revision != first {
		t.Fatalf("revision = %q, want the commit it was generated from", manifest.Template.Revision)
	}

	// Move the template on: its recipe now writes a differently-named file.
	manifestFile := filepath.Join(repo, "ktor-service", "template.toml")
	data, err := os.ReadFile(manifestFile)
	if err != nil {
		t.Fatal(err)
	}
	moved := strings.ReplaceAll(string(data),
		"{{ .Feature.Pascal }}Routes.kt", "{{ .Feature.Pascal }}Handlers.kt")
	if err := os.WriteFile(manifestFile, []byte(moved), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", "rename"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if _, err := filetmpl.OpenRemote(ref, filetmpl.FetchOptions{Refresh: true}); err != nil {
		t.Fatal(err)
	}

	// The project loads the template it was built with, not the newer one.
	loaded, err := scaffold.LoadFor(manifest.Template)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Meta().Source.Revision; got != first {
		t.Errorf("revision = %q, want the pinned %q", got, first)
	}

	mustApply(t, loaded, root, "route", "reports", nil)
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(
		"src/main/kotlin/com/example/orders/routes/ReportsRoutes.kt"))); err != nil {
		t.Error("the project used a newer template than the one it was generated from")
	}
}

// ---------------------------------------------------------------------------
// A template must not read outside itself
// ---------------------------------------------------------------------------

func TestATemplateCannotReadOutsideItself(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("do not leak me\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	dir := write(t, map[string]string{
		"template.toml": `
schema = 1
[template]
id = "nosy"
[[files]]
from = "files/**"
to = "{{ .RelPath }}"
`,
		"files/ok.txt.tmpl": "fine\n",
	})
	// A symlink pointing out of the template, which a fetched one could carry.
	if err := os.Symlink(secret, filepath.Join(dir, "files", "leak.txt")); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}

	tmpl := open(t, dir)
	a := tmpl.NewAnswers()
	a.Project = model.Project{Name: "Widget"}
	root := generate(t, tmpl, a)

	if _, err := os.Stat(filepath.Join(root, "leak.txt")); err == nil {
		t.Error("a symlink out of the template was followed into the generated project")
	}
	if _, err := os.Stat(filepath.Join(root, "ok.txt")); err != nil {
		t.Error("the ordinary file was not written")
	}
}
