package scaffold

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/aaroncutress/kmp-scaffold/internal/model"
)

var (
	mu       sync.RWMutex
	builtins = map[string]func() Template{}
	order    []string
)

// Register adds a built-in template. Templates call this from init(), so a new
// one is a new file rather than an edit to a list.
//
// The constructor is called per use rather than the template being stored,
// because a template carries the answers it was loaded with.
func Register(id string, make func() Template) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := builtins[id]; exists {
		panic("scaffold: template " + id + " registered twice")
	}
	builtins[id] = make
	order = append(order, id)
	sort.Strings(order)
}

// DefaultTemplate is used when none is named.
const DefaultTemplate = "kmp-mobile"

// Builtins returns every compiled-in template, in a stable order with the
// default first, because that is the order the wizard offers them in.
func Builtins() []Template {
	mu.RLock()
	defer mu.RUnlock()

	var out []Template
	if make, ok := builtins[DefaultTemplate]; ok {
		out = append(out, make())
	}
	for _, id := range order {
		if id == DefaultTemplate {
			continue
		}
		out = append(out, builtins[id]())
	}
	return out
}

// ExternalLoader loads templates that are not compiled in: a directory, one in
// the user's templates folder, or - later - a git remote.
//
// It is installed rather than imported because the implementation needs this
// package, and this package must not depend on the things that implement
// templates.
type ExternalLoader interface {
	// Load returns the template a ref names, or an error saying why not.
	Load(ref string) (Template, error)
	// Discover lists the templates found without being asked for by name.
	Discover() []Template
}

var external ExternalLoader

// SetLoader installs the loader for external template refs.
func SetLoader(l ExternalLoader) {
	mu.Lock()
	defer mu.Unlock()
	external = l
}

func loader() ExternalLoader {
	mu.RLock()
	defer mu.RUnlock()
	return external
}

// Load returns the template named by a ref: a built-in id, a directory, or a
// template in the user's templates folder.
//
// Built-ins win, so a template in the user folder cannot shadow one of them by
// accident.
func Load(ref string) (Template, error) {
	if ref == "" {
		ref = DefaultTemplate
	}

	mu.RLock()
	make, ok := builtins[ref]
	mu.RUnlock()
	if ok {
		return make(), nil
	}

	if l := loader(); l != nil {
		t, err := l.Load(ref)
		if err != nil {
			return nil, err
		}
		if t != nil {
			return t, nil
		}
	} else if looksLikeExternalRef(ref) {
		return nil, fmt.Errorf(
			"%s looks like an external template, which this build cannot load - "+
				"run `kmp-scaffold templates` for the ones it has", ref)
	}

	return nil, fmt.Errorf("unknown template %q - available: %s",
		ref, strings.Join(available(), ", "))
}

// LoadFor returns the template a generated project was made from.
//
// The recorded source is tried first, so a project made from a directory or a
// remote loads that same template rather than a different one that happens to
// share its id. Falling back to the id is what lets a template that has since
// been installed properly still be found.
func LoadFor(ref model.TemplateRef) (Template, error) {
	if ref.ID == "" {
		return nil, fmt.Errorf("this project does not record which template generated it")
	}
	if source := ref.Source; source != "" && source != string(SourceBuiltin) {
		// A remote is loaded at the commit the project was generated from, not
		// at whatever the branch points at today. Adding to a project two years
		// later has to use the template that built it.
		if ref.Revision != "" {
			if at, ok := pinned(source, ref.Revision); ok {
				if t, err := Load(at); err == nil {
					return t, nil
				}
			}
		}
		if t, err := Load(source); err == nil {
			return t, nil
		}
	}
	return Load(ref.ID)
}

// pinned rewrites a ref to name an exact commit, replacing whatever revision it
// carried. It reports false for a ref with no revision to replace - a path, or
// a bare name - where the commit means nothing.
func pinned(source, revision string) (string, bool) {
	if !strings.Contains(source, ":") && !strings.Contains(source, "/") {
		return "", false
	}
	if strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") ||
		strings.HasPrefix(source, "~") {
		return "", false
	}
	// The revision is whatever follows the last @, which cannot appear in a
	// repository path.
	if i := strings.LastIndex(source, "@"); i > strings.Index(source, "//") {
		source = source[:i]
	}
	return source + "@" + revision, true
}

// All lists every template that can be generated from without being named by
// path: the built-ins, plus whatever the loader discovered.
func All() []Template {
	out := Builtins()
	if l := loader(); l != nil {
		seen := map[string]bool{}
		for _, t := range out {
			seen[t.Meta().ID] = true
		}
		for _, t := range l.Discover() {
			if id := t.Meta().ID; !seen[id] {
				seen[id] = true
				out = append(out, t)
			}
		}
	}
	return out
}

func available() []string {
	var out []string
	for _, t := range All() {
		out = append(out, t.Meta().ID)
	}
	sort.Strings(out)
	return out
}

// IDs lists the ids Load accepts.
func IDs() []string {
	mu.RLock()
	defer mu.RUnlock()
	return append([]string(nil), order...)
}

// looksLikeExternalRef reports whether a ref names something outside the binary:
// a directory, or a git remote.
func looksLikeExternalRef(ref string) bool {
	switch {
	case strings.HasPrefix(ref, "."), strings.HasPrefix(ref, "/"), strings.HasPrefix(ref, "~"):
		return true
	case strings.Contains(ref, "://"), strings.HasPrefix(ref, "github:"), strings.HasPrefix(ref, "gitlab:"):
		return true
	default:
		return false
	}
}
