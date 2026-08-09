package filetmpl

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Ref is a template that lives in a git repository.
type Ref struct {
	// Host, Owner and Repo identify the repository.
	Host  string
	Owner string
	Repo  string
	// Subdir is the template's directory inside it, if it is not the root.
	Subdir string
	// Rev is the branch, tag or commit asked for. Empty means the default branch.
	Rev string
	// URL is what git is given.
	URL string
	// Raw is what the user typed, and what a generated project records.
	Raw string
}

// Slug is the repository's cache and trust key.
//
// It becomes a directory, so each part is reduced to something every filesystem
// will accept. A Windows drive letter is the case that forces this - "C:" is a
// perfectly good part of a file:// path and an illegal directory name - but a
// repository path can hold surprises on any platform.
func (r Ref) Slug() string {
	return path.Join(safeComponent(r.Host), safeComponent(r.Owner), safeComponent(r.Repo))
}

// safeComponent replaces the characters Windows forbids in a path component,
// and trims the trailing dots and spaces it also refuses.
func safeComponent(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, s)
	s = strings.TrimRight(s, ". ")
	if s == "" {
		return "_"
	}
	return s
}

// String is the ref in its canonical form.
func (r Ref) String() string { return r.Raw }

var (
	shorthandRe = regexp.MustCompile(`^(github|gitlab):([^/]+)/([^/@]+)(/[^@]+)?(@(.+))?$`)
	scpRe       = regexp.MustCompile(`^([^@]+)@([^:]+):([^/]+)/(.+?)(\.git)?(@(.+))?$`)
	shaRe       = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
)

// ParseRef recognises the forms `--template` accepts for a remote template.
//
//	github:owner/repo
//	github:owner/repo/templates/service@v2
//	gitlab:owner/repo@main
//	https://git.example.com/team/templates.git//service@v2
//	git@github.com:owner/repo.git@v2
//
// The `//` in a URL separates the repository from a directory inside it, the
// same convention Terraform and go-getter use, because a single `/` is
// ambiguous with the repository path itself.
func ParseRef(s string) (Ref, bool) {
	s = strings.TrimSpace(s)

	if m := shorthandRe.FindStringSubmatch(s); m != nil {
		host := "github.com"
		if m[1] == "gitlab" {
			host = "gitlab.com"
		}
		return Ref{
			Host:   host,
			Owner:  m[2],
			Repo:   strings.TrimSuffix(m[3], ".git"),
			Subdir: strings.Trim(m[4], "/"),
			Rev:    m[6],
			URL:    fmt.Sprintf("https://%s/%s/%s.git", host, m[2], strings.TrimSuffix(m[3], ".git")),
			Raw:    s,
		}, true
	}

	if strings.Contains(s, "://") {
		return parseURLRef(s)
	}
	if m := scpRe.FindStringSubmatch(s); m != nil && strings.Contains(s, ":") {
		return Ref{
			Host:  m[2],
			Owner: m[3],
			Repo:  strings.TrimSuffix(m[4], ".git"),
			Rev:   m[7],
			URL:   fmt.Sprintf("%s@%s:%s/%s.git", m[1], m[2], m[3], strings.TrimSuffix(m[4], ".git")),
			Raw:   s,
		}, true
	}
	return Ref{}, false
}

func parseURLRef(s string) (Ref, bool) {
	raw := s

	if strings.HasPrefix(s, "file://") {
		return parseFileRef(raw)
	}

	rev := ""
	// The revision is separated by the last @, which cannot appear in a path.
	if i := strings.LastIndex(s, "@"); i > strings.Index(s, "://") {
		rev, s = s[i+1:], s[:i]
	}

	subdir := ""
	if i := strings.Index(s[strings.Index(s, "://")+3:], "//"); i >= 0 {
		at := strings.Index(s, "://") + 3 + i
		subdir, s = strings.Trim(s[at+2:], "/"), s[:at]
	}

	rest := s[strings.Index(s, "://")+3:]
	host, pathPart, ok := strings.Cut(rest, "/")
	if !ok || host == "" || pathPart == "" {
		return Ref{}, false
	}
	pathPart = strings.TrimSuffix(strings.Trim(pathPart, "/"), ".git")

	owner, repo := path.Split(pathPart)
	if repo == "" {
		return Ref{}, false
	}
	return Ref{
		Host:   host,
		Owner:  strings.Trim(owner, "/"),
		Repo:   repo,
		Subdir: subdir,
		Rev:    rev,
		URL:    s,
		Raw:    raw,
	}, true
}

// parseFileRef handles a repository on this machine. It is a real git
// transport, and the one that makes a template on a shared drive - or in a
// test - work exactly like one from a git host.
//
// The whole path becomes the cache key, because two directories can perfectly
// well end in the same name.
func parseFileRef(raw string) (Ref, bool) {
	// Windows paths arrive with backslashes and a drive letter, whether written
	// as file://C:\src\tmpl or as the proper file:///C:/src/tmpl. Both are
	// worth accepting; both normalise to the same thing.
	//
	// The replacement is unconditional rather than filepath.ToSlash, which does
	// nothing off Windows - and this has to understand a Windows path typed on
	// any machine, not just on one.
	body := strings.ReplaceAll(strings.TrimPrefix(raw, "file://"), `\`, "/")

	rev := ""
	if i := strings.LastIndex(body, "@"); i > 0 {
		rev, body = body[i+1:], body[:i]
	}
	// The path itself starts with slashes, so a // separating a subdirectory is
	// looked for past them rather than from the front.
	lead := len(body) - len(strings.TrimLeft(body, "/"))
	subdir := ""
	if i := strings.Index(body[lead:], "//"); i >= 0 {
		at := lead + i
		subdir, body = strings.Trim(body[at+2:], "/"), body[:at]
	}

	body = strings.TrimSuffix(strings.TrimRight(body, "/"), ".git")
	clean := strings.TrimLeft(body, "/")
	if clean == "" {
		return Ref{}, false
	}

	dir, repo := path.Split(clean)
	if repo == "" {
		return Ref{}, false
	}
	return Ref{
		Host:   "file",
		Owner:  strings.ReplaceAll(strings.Trim(dir, "/"), "/", "_"),
		Repo:   repo,
		Subdir: subdir,
		Rev:    rev,
		// Three slashes is the form git wants for a local repository, on either
		// platform: file:///srv/tmpl, file:///C:/src/tmpl.
		URL: "file:///" + clean,
		Raw: raw,
	}, true
}

// ---------------------------------------------------------------------------
// The cache
// ---------------------------------------------------------------------------

// CacheDir is where fetched templates are kept.
func CacheDir() string {
	if dir := os.Getenv("KMP_SCAFFOLD_CACHE"); dir != "" {
		return dir
	}
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, "kmp-scaffold", "templates")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "kmp-scaffold", "templates")
}

// branchTTL is how long a branch ref is reused before being checked again. A
// tag or a commit is immutable, so it is never re-fetched.
const branchTTL = 24 * time.Hour

// refIndex records which commit each ref resolved to, and when.
type refIndex struct {
	Refs map[string]refEntry `json:"refs"`
}

type refEntry struct {
	Revision string `json:"revision"`
	// Subdir is the template's directory inside the repository, so a listing
	// can find it again without being told.
	Subdir string `json:"subdir,omitempty"`
	// Ref is what the user typed. A listing shows it back verbatim rather than
	// rebuilding one from the cache's own directory names, which are sanitised
	// and so cannot be turned back into the original.
	Ref     string    `json:"ref,omitempty"`
	Fetched time.Time `json:"fetched"`
}

// Fetched describes a template tree on disk, fetched or reused.
type Fetched struct {
	// Dir is the template's directory, including any subdirectory of the repo.
	Dir string
	// Revision is the commit the ref resolved to.
	Revision string
	// FromCache is true when nothing was downloaded.
	FromCache bool
}

// FetchOptions controls a fetch.
type FetchOptions struct {
	// Refresh re-fetches even when the cache would have served.
	Refresh bool
	// Offline refuses to touch the network; a cache miss is an error.
	Offline bool
}

// Fetch returns the template tree for a ref, cloning it if it is not cached.
//
// The payload is keyed by the resolved commit rather than by the ref, so the
// revision a project records means something years later, and two runs
// fetching the same branch at the same time cannot tread on each other.
func Fetch(ref Ref, opts FetchOptions) (Fetched, error) {
	root := filepath.Join(CacheDir(), filepath.FromSlash(ref.Slug()))

	// An immutable ref that is already on disk needs no network at all.
	if !opts.Refresh {
		if rev, ok := cachedRevision(root, ref); ok {
			dir := filepath.Join(root, rev)
			if _, err := os.Stat(dir); err == nil {
				return Fetched{Dir: templateDir(dir, ref), Revision: rev, FromCache: true}, nil
			}
		}
	}

	if opts.Offline {
		return Fetched{}, fmt.Errorf(
			"%s is not in the cache and --offline was given; run it once without --offline", ref.Raw)
	}
	if _, err := exec.LookPath("git"); err != nil {
		return Fetched{}, fmt.Errorf(
			"fetching %s needs git on your PATH; install it, or clone the template and pass the directory",
			ref.Raw)
	}

	tmp, err := os.MkdirTemp(filepath.Dir(root), ".fetch-*")
	if err != nil {
		if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
			return Fetched{}, err
		}
		if tmp, err = os.MkdirTemp(filepath.Dir(root), ".fetch-*"); err != nil {
			return Fetched{}, err
		}
	}
	defer os.RemoveAll(tmp)

	clone := filepath.Join(tmp, "repo")
	args := []string{"clone", "--depth", "1", "--quiet"}
	if ref.Rev != "" && !shaRe.MatchString(ref.Rev) {
		args = append(args, "--branch", ref.Rev)
	}
	args = append(args, ref.URL, clone)

	if out, err := runGit("", args...); err != nil {
		return Fetched{}, fmt.Errorf("cloning %s: %w%s", ref.URL, err, gitDetail(out))
	}

	// A commit cannot be cloned shallowly by name, so it is fetched after.
	if ref.Rev != "" && shaRe.MatchString(ref.Rev) {
		if out, err := runGit(clone, "fetch", "--depth", "1", "origin", ref.Rev); err != nil {
			return Fetched{}, fmt.Errorf("fetching %s from %s: %w%s", ref.Rev, ref.URL, err, gitDetail(out))
		}
		if out, err := runGit(clone, "checkout", "--quiet", ref.Rev); err != nil {
			return Fetched{}, fmt.Errorf("checking out %s: %w%s", ref.Rev, err, gitDetail(out))
		}
	}

	revision, err := runGit(clone, "rev-parse", "HEAD")
	if err != nil {
		return Fetched{}, fmt.Errorf("reading the commit of %s: %w", ref.URL, err)
	}
	revision = strings.TrimSpace(revision)

	// The history is not wanted - only the tree - and leaving it behind would
	// make the cache several times larger than it needs to be.
	if err := os.RemoveAll(filepath.Join(clone, ".git")); err != nil {
		return Fetched{}, err
	}

	dest := filepath.Join(root, revision)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return Fetched{}, err
	}
	if _, err := os.Stat(dest); err != nil {
		// Rename is atomic, so a run that is killed here leaves no half-tree.
		if err := os.Rename(clone, dest); err != nil {
			return Fetched{}, fmt.Errorf("caching %s: %w", ref.Raw, err)
		}
	}
	recordRevision(root, ref, revision)

	return Fetched{Dir: templateDir(dest, ref), Revision: revision}, nil
}

// templateDir applies the ref's subdirectory to a fetched tree.
func templateDir(dir string, ref Ref) string {
	if ref.Subdir == "" {
		return dir
	}
	return filepath.Join(dir, filepath.FromSlash(ref.Subdir))
}

// cachedRevision reports the commit a ref last resolved to, when that answer is
// still good. A commit or a tag never changes; a branch is re-checked daily.
func cachedRevision(root string, ref Ref) (string, bool) {
	if shaRe.MatchString(ref.Rev) {
		return ref.Rev, true
	}

	index := readIndex(root)
	entry, ok := index.Refs[ref.cacheKey()]
	if !ok {
		return "", false
	}
	if ref.Rev == "" && time.Since(entry.Fetched) > branchTTL {
		// The default branch moves, so a stale answer is only a starting point.
		return "", false
	}
	if ref.Rev != "" && looksLikeBranch(ref.Rev) && time.Since(entry.Fetched) > branchTTL {
		return "", false
	}
	return entry.Revision, true
}

// cacheKey names one entry of a repository's ref index. The subdirectory is
// part of it: one repository can hold several templates, each fetched at its
// own ref.
func (r Ref) cacheKey() string {
	key := r.Rev
	if key == "" {
		key = "HEAD"
	}
	if r.Subdir != "" {
		key += "//" + r.Subdir
	}
	return key
}

// looksLikeBranch guesses whether a ref moves. A tag conventionally starts with
// a digit or a "v", which is the best signal available without asking the
// remote - and guessing wrong only costs one extra fetch a day.
func looksLikeBranch(rev string) bool {
	if rev == "" {
		return true
	}
	switch {
	case strings.HasPrefix(rev, "v") && len(rev) > 1 && rev[1] >= '0' && rev[1] <= '9':
		return false
	case rev[0] >= '0' && rev[0] <= '9':
		return false
	default:
		return true
	}
}

func readIndex(root string) refIndex {
	index := refIndex{Refs: map[string]refEntry{}}
	data, err := os.ReadFile(filepath.Join(root, "refs.json"))
	if err != nil {
		return index
	}
	if err := json.Unmarshal(data, &index); err != nil || index.Refs == nil {
		return refIndex{Refs: map[string]refEntry{}}
	}
	return index
}

func recordRevision(root string, ref Ref, revision string) {
	index := readIndex(root)
	index.Refs[ref.cacheKey()] = refEntry{
		Revision: revision, Subdir: ref.Subdir, Ref: ref.Raw, Fetched: time.Now(),
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return
	}
	// A failure here only costs a re-fetch next time, so it is not worth
	// failing the command over.
	_ = os.WriteFile(filepath.Join(root, "refs.json"), append(data, '\n'), 0o644)
}

// Forget removes a repository's cached trees.
func Forget(ref Ref) error {
	root := filepath.Join(CacheDir(), filepath.FromSlash(ref.Slug()))
	if _, err := os.Stat(root); err != nil {
		return fmt.Errorf("%s is not cached", ref.Raw)
	}
	return os.RemoveAll(root)
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	// A prompt would hang a non-interactive run forever; failing is better.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "SSH_ASKPASS=")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// gitDetail appends git's own message, which usually says exactly what went
// wrong ("Repository not found", "could not read Username").
func gitDetail(out string) string {
	out = strings.TrimSpace(out)
	if out == "" {
		return ""
	}
	lines := strings.Split(out, "\n")
	return "\n  " + strings.Join(lines, "\n  ")
}
