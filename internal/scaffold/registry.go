package scaffold

import (
	"fmt"
	"sort"
	"strings"
	"sync"
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

// Load returns the template named by a ref.
//
// Only built-in ids are understood for now; paths, the user templates folder
// and remote refs are recognised well enough to give a useful error rather than
// "unknown template".
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

	if looksLikeExternalRef(ref) {
		return nil, fmt.Errorf(
			"%s looks like an external template, which this build cannot load yet - "+
				"run `kmp-scaffold templates` for the ones it has", ref)
	}
	return nil, fmt.Errorf("unknown template %q - available: %s", ref, strings.Join(IDs(), ", "))
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
