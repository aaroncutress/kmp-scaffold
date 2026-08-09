package filetmpl

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
)

func init() { scaffold.SetLoader(Loader{}) }

// Loader resolves template refs that are not compiled in.
type Loader struct{}

// fetchOpts is how the next remote fetch behaves. `new --refresh` and
// `--offline` set it, because the ref itself says nothing about either.
var (
	fetchMu   sync.RWMutex
	fetchOpts FetchOptions
)

// SetFetchOptions controls the next remote fetch.
func SetFetchOptions(opts FetchOptions) {
	fetchMu.Lock()
	defer fetchMu.Unlock()
	fetchOpts = opts
}

func currentFetchOptions() FetchOptions {
	fetchMu.RLock()
	defer fetchMu.RUnlock()
	return fetchOpts
}

// Load returns the template a ref names.
//
// A ref that looks like a path is loaded from there. A bare name is looked for
// in the user's templates folder. Anything else - a git remote - is recognised
// well enough to say what is missing rather than "unknown template". Returning
// (nil, nil) means "not mine", so the caller can report the ref its own way.
func (Loader) Load(ref string) (scaffold.Template, error) {
	switch {
	case isPath(ref):
		dir, err := expandPath(ref)
		if err != nil {
			return nil, err
		}
		return Open(dir, scaffold.Source{Kind: scaffold.SourcePath, Ref: ref})

	case isRemote(ref):
		parsed, ok := ParseRef(ref)
		if !ok {
			return nil, fmt.Errorf("%s is not a template reference this build understands - "+
				"run `kmp-scaffold templates --help` for the forms it takes", ref)
		}
		return OpenRemote(parsed, currentFetchOptions())
	}

	dir := filepath.Join(UserDir(), ref)
	if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err == nil {
		return Open(dir, scaffold.Source{Kind: scaffold.SourceUser, Ref: ref})
	}

	// A bare name may be a remote template already in the cache, offered by
	// whatever id its own manifest gives.
	for _, c := range cachedList() {
		if c.Template.Meta().ID == ref {
			return c.Template, nil
		}
	}
	return nil, nil
}

// OpenRemote fetches a template from a git repository and, the first time a
// given commit is used, asks whether to trust it.
func OpenRemote(ref Ref, opts FetchOptions) (*Template, error) {
	fetched, err := Fetch(ref, opts)
	if err != nil {
		return nil, err
	}

	t, err := Open(fetched.Dir, scaffold.Source{
		Kind:     scaffold.SourceRemote,
		Ref:      ref.Raw,
		Revision: fetched.Revision,
	})
	if err != nil {
		return nil, err
	}
	if err := confirm(ref, fetched, t); err != nil {
		return nil, err
	}
	return t, nil
}

// Discover lists the templates that can be generated from by name: the user's
// own, and any remote one already fetched.
//
// One that will not parse is skipped rather than being an error: a half-written
// template in that folder must not stop `kmp-scaffold new` from working.
func (Loader) Discover() []scaffold.Template {
	out := userTemplates()

	seen := map[string]bool{}
	for _, t := range out {
		seen[t.Meta().ID] = true
	}
	for _, c := range cachedList() {
		if id := c.Template.Meta().ID; !seen[id] {
			seen[id] = true
			out = append(out, c.Template)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Meta().ID < out[j].Meta().ID })
	return out
}

func userTemplates() []scaffold.Template {
	entries, err := os.ReadDir(UserDir())
	if err != nil {
		return nil
	}

	var out []scaffold.Template
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(UserDir(), e.Name())
		if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err != nil {
			continue
		}
		t, err := Open(dir, scaffold.Source{Kind: scaffold.SourceUser, Ref: e.Name()})
		if err != nil {
			continue
		}
		out = append(out, t)
	}
	return out
}

// Cached is a remote template already in the cache.
type Cached struct {
	Template *Template
	Ref      Ref
	Revision string
}

// CachedTemplates lists what has been fetched.
func CachedTemplates() []Cached { return cachedList() }

// cachedList walks the cache: <host>/<owner>/<repo>/refs.json names the commit
// each ref resolved to, and the tree sits beside it under that commit.
func cachedList() []Cached {
	root := CacheDir()
	if root == "" {
		return nil
	}

	var out []Cached
	seen := map[string]bool{}

	// The layout is exactly three levels deep, so the walk stops there rather
	// than descending into every cached tree.
	hosts, _ := os.ReadDir(root)
	for _, host := range hosts {
		if !host.IsDir() {
			continue
		}
		owners, _ := os.ReadDir(filepath.Join(root, host.Name()))
		for _, owner := range owners {
			if !owner.IsDir() {
				continue
			}
			repos, _ := os.ReadDir(filepath.Join(root, host.Name(), owner.Name()))
			for _, repo := range repos {
				if !repo.IsDir() {
					continue
				}
				repoDir := filepath.Join(root, host.Name(), owner.Name(), repo.Name())
				for refName, entry := range readIndex(repoDir).Refs {
					key := repoDir + "@" + entry.Revision + "//" + entry.Subdir
					if seen[key] {
						continue
					}
					seen[key] = true

					ref := Ref{
						Host: host.Name(), Owner: owner.Name(), Repo: repo.Name(),
						Subdir: entry.Subdir,
						URL:    fmt.Sprintf("https://%s/%s/%s.git", host.Name(), owner.Name(), repo.Name()),
					}
					if rev, _, _ := strings.Cut(refName, "//"); rev != "HEAD" {
						ref.Rev = rev
					}
					ref.Raw = entry.Ref
					if ref.Raw == "" {
						// Written before the ref was recorded; the best that can
						// be done is to rebuild one.
						ref.Raw = refText(ref)
					}

					dir := filepath.Join(repoDir, entry.Revision)
					if entry.Subdir != "" {
						dir = filepath.Join(dir, filepath.FromSlash(entry.Subdir))
					}
					t, err := Open(dir, scaffold.Source{
						Kind: scaffold.SourceRemote, Ref: ref.Raw, Revision: entry.Revision,
					})
					if err != nil {
						continue
					}
					out = append(out, Cached{Template: t, Ref: ref, Revision: entry.Revision})
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref.Raw < out[j].Ref.Raw })
	return out
}

// refText rebuilds a ref from a cache entry that did not record one. It is a
// best effort: the cache's directory names are sanitised, so a path with a
// drive letter or a colon in it cannot come back exactly as it went in.
func refText(ref Ref) string {
	var prefix string
	switch ref.Host {
	case "github.com":
		prefix = "github:" + ref.Owner + "/" + ref.Repo
	case "gitlab.com":
		prefix = "gitlab:" + ref.Owner + "/" + ref.Repo
	case "file":
		prefix = "file:///" + strings.ReplaceAll(ref.Owner, "_", "/") + "/" + ref.Repo
	default:
		prefix = "https://" + ref.Host + "/" + ref.Owner + "/" + ref.Repo + ".git"
	}
	if ref.Subdir != "" {
		if strings.Contains(prefix, "://") {
			prefix += "//" + ref.Subdir
		} else {
			prefix += "/" + ref.Subdir
		}
	}
	if ref.Rev != "" {
		prefix += "@" + ref.Rev
	}
	return prefix
}

// Broken lists the directories in the user's templates folder that look like
// templates but will not load, with the reason. `kmp-scaffold templates` shows
// these, so a mistake there is visible rather than silently absent.
func Broken() map[string]error {
	entries, err := os.ReadDir(UserDir())
	if err != nil {
		return nil
	}
	out := map[string]error{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(UserDir(), e.Name())
		if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err != nil {
			continue
		}
		if _, err := Open(dir, scaffold.Source{Kind: scaffold.SourceUser, Ref: e.Name()}); err != nil {
			out[e.Name()] = err
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// UserDir is where a user's own templates live: one directory per template,
// each with a template.toml.
func UserDir() string {
	if dir := os.Getenv("KMP_SCAFFOLD_TEMPLATES"); dir != "" {
		return dir
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "kmp-scaffold", "templates")
	}
	if dir, ok := windowsDir("AppData", "templates"); ok {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "kmp-scaffold", "templates")
}

// windowsDir is a path under one of Windows' own directories, and false
// everywhere else.
//
// The XDG variables above are honoured first wherever they are set, because
// someone who has set them means it. This is the fallback, so that a Windows
// user's files land where Windows puts files rather than in a dot-directory
// borrowed from another platform's conventions.
//
// AppData is roaming configuration - it follows the user between machines.
// LocalAppData is machine-local state, which is where a cache belongs.
func windowsDir(env string, parts ...string) (string, bool) {
	if runtime.GOOS != "windows" {
		return "", false
	}
	base := os.Getenv(env)
	if base == "" {
		return "", false
	}
	return filepath.Join(append([]string{base, "kmp-scaffold"}, parts...)...), true
}

// isPath reports whether a ref names a directory rather than an id.
func isPath(ref string) bool {
	switch {
	case strings.HasPrefix(ref, "."), strings.HasPrefix(ref, "~"):
		return true
	case filepath.IsAbs(ref):
		return true
	case strings.ContainsAny(ref, `/\`) && !isRemote(ref):
		return true
	default:
		return false
	}
}

func isRemote(ref string) bool {
	return strings.Contains(ref, "://") ||
		strings.HasPrefix(ref, "github:") ||
		strings.HasPrefix(ref, "gitlab:") ||
		strings.HasPrefix(ref, "git@")
}

func expandPath(ref string) (string, error) {
	if strings.HasPrefix(ref, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		ref = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(ref, "~"), "/"))
	}
	abs, err := filepath.Abs(ref)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("no template at %s", ref)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is a file; a template is a directory containing %s", ref, ManifestFile)
	}
	return abs, nil
}
