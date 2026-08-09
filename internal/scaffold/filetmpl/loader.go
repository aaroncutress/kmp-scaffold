package filetmpl

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
)

func init() { scaffold.SetLoader(Loader{}) }

// Loader resolves template refs that are not compiled in.
type Loader struct{}

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
		return nil, fmt.Errorf(
			"%s is a remote template, which this build cannot fetch yet - "+
				"clone it and pass the directory instead", ref)
	}

	dir := filepath.Join(UserDir(), ref)
	if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err == nil {
		return Open(dir, scaffold.Source{Kind: scaffold.SourceUser, Ref: ref})
	}
	return nil, nil
}

// Discover lists the templates in the user's templates folder.
//
// One that will not parse is skipped rather than being an error: a half-written
// template in that folder must not stop `kmp-scaffold new` from working.
func (Loader) Discover() []scaffold.Template {
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
	sort.Slice(out, func(i, j int) bool { return out[i].Meta().ID < out[j].Meta().ID })
	return out
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
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("AppData"); appData != "" {
			return filepath.Join(appData, "kmp-scaffold", "templates")
		}
	}
	return filepath.Join(home, ".config", "kmp-scaffold", "templates")
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
