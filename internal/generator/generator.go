// Package generator turns a resolved project spec into files.
//
// Generators are registered rather than hard-coded so that new project layouts
// can be added without touching the wizard or the CLI: register a Generator
// with KindIOS and it shows up as an iOS layout choice automatically. See
// docs/extending.md.
package generator

import (
	"fmt"
	"sort"
	"sync"

	"github.com/aaroncutress/kmp-scaffold/internal/render"
)

// Kind groups generators by the part of the project they own.
type Kind string

const (
	// KindRoot owns the Gradle root: settings, version catalog, buildSrc.
	KindRoot Kind = "root"
	// KindShared owns the sharedLogic KMP module.
	KindShared Kind = "shared"
	// KindAndroid owns androidApp plus its core/feature modules. Generators of
	// this kind are offered as "Android layout" choices in the wizard.
	KindAndroid Kind = "android"
	// KindIOS owns iosApp. Generators of this kind are offered as "iOS layout"
	// choices in the wizard.
	KindIOS Kind = "ios"
)

// Env is everything a generator needs to emit files.
type Env struct {
	Ctx    Ctx
	Engine *render.Engine
	Writer *render.Writer
}

// Render is shorthand for rendering a template to a path with the shared context.
func (e *Env) Render(tplName, path string) error {
	return e.Writer.Render(e.Engine, tplName, path, e.Ctx)
}

// RenderWith renders a template against custom data (e.g. a per-tab context).
func (e *Env) RenderWith(tplName, path string, data any) error {
	return e.Writer.Render(e.Engine, tplName, path, data)
}

// Generator emits one part of a project.
type Generator interface {
	// ID is the stable identifier stored in the project manifest.
	ID() string
	// Label is the short name shown in the wizard.
	Label() string
	// Description is the one-line explanation shown under the label.
	Description() string
	// Kind determines where the generator is offered.
	Kind() Kind
	// Generate writes this generator's files.
	Generate(env *Env) error
}

// FeatureGenerator is implemented by layout generators that can also add a
// feature module to an existing project. A layout that does not implement it
// simply cannot be extended by `kmp-scaffold add feature` (the CLI says so
// rather than failing halfway through).
type FeatureGenerator interface {
	Generator
	// GenerateFeature writes the files for one feature module. env.Ctx.Feature
	// is always set when this is called.
	GenerateFeature(env *Env) error
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Generator{}
)

// Register adds a generator. It panics on duplicate ids, which can only happen
// through a programming error at init time.
func Register(g Generator) {
	registryMu.Lock()
	defer registryMu.Unlock()
	key := string(g.Kind()) + "/" + g.ID()
	if _, dup := registry[key]; dup {
		panic("generator already registered: " + key)
	}
	registry[key] = g
}

// Get returns the generator with the given kind and id.
func Get(kind Kind, id string) (Generator, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	g, ok := registry[string(kind)+"/"+id]
	if !ok {
		return nil, fmt.Errorf("no %s layout named %q (available: %s)", kind, id, availableIDs(kind))
	}
	return g, nil
}

// Layouts lists every generator of a kind, sorted by id, for the wizard.
func Layouts(kind Kind) []Generator {
	registryMu.RLock()
	defer registryMu.RUnlock()
	var out []Generator
	for _, g := range registry {
		if g.Kind() == kind {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// SupportsFeatures reports whether a registered layout can be extended by
// `add feature`, so the wizard can grey out the choice rather than failing
// halfway through.
func SupportsFeatures(kind Kind, id string) bool {
	g, err := Get(kind, id)
	if err != nil {
		return false
	}
	_, ok := g.(FeatureGenerator)
	return ok
}

func availableIDs(kind Kind) string {
	var ids []string
	for _, g := range registry {
		if g.Kind() == kind {
			ids = append(ids, g.ID())
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return "none registered"
	}
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ", "
		}
		out += id
	}
	return out
}
