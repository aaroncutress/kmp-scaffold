# Extending kmp-scaffold

The tool is built so the things you are most likely to want to change are
additive. This page covers the five common cases, from the largest to the
smallest.

## Adding a template

A **template** owns a whole generatable project: the questions it asks, the
files it writes, and what `add feature` does to a project it made. The Kotlin
Multiplatform project this tool started life generating is one template,
`kmp-mobile`, and it is the default.

Templates live behind one interface, `scaffold.Template`
(`internal/scaffold/template.go`), and register themselves from `init()`:

```go
package ktor

func init() { scaffold.Register("ktor-service", func() scaffold.Template { return Template{} }) }

type Template struct{}

func (Template) Meta() scaffold.Meta {
    return scaffold.Meta{
        ID: "ktor-service", Label: "Ktor service",
        Description: "A Ktor server with layered routes, Koin DI and a Dockerfile.",
        Source:      scaffold.Source{Kind: scaffold.SourceBuiltin},
        AsksPackage: true,
        Sentinels:   []string{"build.gradle.kts"},
    }
}
```

The tool asks the universal questions itself — name, directory, and (when
`AsksPackage` is set) the package — and the template supplies the rest as data:

```go
func (Template) Questions() []scaffold.Question {
    return []scaffold.Question{{
        ID:     "database",
        Kind:   scaffold.KindSelect,
        Prompt: "Which database?",
        Options: []scaffold.Option{
            {ID: "postgres", Label: "PostgreSQL", Desc: "Exposed and HikariCP."},
            {ID: "none", Label: "No database"},
        },
        Default: "postgres",
    }}
}
```

`Question` has a declarative half and a Go half. The declarative fields —
`Default`, `Options`, `AllowEmpty` — are enough for most questions. The hooks
below them (`DefaultFor`, `OptionsFor`, `SkipFor`, `Validate`, `Apply`) win
where both are given, and are how `kmp-mobile` computes its layout options from
the generator registry: a newly registered layout appears in the wizard without
the question list being touched.

The rest of the interface is what happens with the answers:

| Method | What it is for |
| --- | --- |
| `NewAnswers` | The seeded defaults the wizard starts from. |
| `Normalise` | Repair cross-question dependencies, returning a note per change. |
| `Versions` | Which version keys to resolve. Return an empty request to skip resolution entirely. |
| `Check` | Add compatibility findings once versions are known. |
| `Headlines` / `Summary` | What the resolve and review screens show. |
| `Generate` | Write the project. |
| `Vars` | What the project records about itself, for a later `add`. |
| `NextSteps` | What to print once it exists. |

`Recipes()` is what `kmp-scaffold add` drives — the named things this template
can add to a project it generated:

```go
func (Template) Recipes() []scaffold.Recipe {
    return []scaffold.Recipe{{
        Name: "route", Noun: "route", Label: "Route module",
        Questions: func(m *model.Manifest) []scaffold.Question { … },
        Summary:   func(m *model.Manifest, a *scaffold.Answers) []scaffold.Section { … },
        Apply:     addRoute,
    }}
}
```

The tool asks for the name (and rejects a duplicate), the recipe asks the rest,
and `Apply` writes the files and applies `wire.Edit`s to the ones that already
exist. Return nil and `add` says the template generates a project in one go,
rather than half-generating something.

`internal/kmp` is the worked example. It is a thin adapter: the generation lives
in `internal/generator`, the libraries in `internal/catalog` and the answers in
`model.Spec`, and the package's job is to present all of that as one template.

### Or write it as files instead

A template that does not need any of the Go-only power — options from a
registry, cross-question dependency rules, a hook into the resolver — can be a
directory with a `template.toml` and a tree of Go templates, with no compiler
involved. That is usually the right answer for a second template. See
[templates](templates.md#writing-a-template); the implementation is
`internal/scaffold/filetmpl`, and `examples/templates/ktor-service` is the
worked example, exercised end to end by the tests.

### The answer bag

Answers are a `scaffold.Answers`: `Project` for the universal ones, `Values` for
one entry per question, and `State` for a template's own typed view. The KMP
template keeps a `*model.Spec` in `State`, so its question hooks work with the
real thing rather than reading everything back out of a map.

Read `Values` through the accessors (`Str`, `Strs`, `Bool`, `Int`) rather than
indexing it. Anything that has been through the project manifest comes back from
`encoding/json` as `[]any` and `float64`, and the accessors handle both.

## Adding a project layout

This is the main extension point. A layout is a **generator**: something that
knows how to write one part of a project. They live in a registry, so the wizard
lists whatever is registered without being told about it.

`swiftui-features` was added this way: `swiftui-simple` writes a single
`ContentView.swift`, and the modular layout was registered *alongside* it rather
than replacing it, so projects generated against the old one still regenerate
the way they were built. `internal/generator/ios_features.go` is the worked
example — the sketch below is the same shape.

### 1. Write the generator

Create `internal/generator/ios_feature.go`:

```go
package generator

func init() { Register(iosFeatureGenerator{}) }

type iosFeatureGenerator struct{}

func (iosFeatureGenerator) ID() string    { return "swiftui-custom" }
func (iosFeatureGenerator) Label() string { return "SwiftUI, one folder per feature" }
func (iosFeatureGenerator) Description() string {
    return "A Views/<Feature>/ folder per feature, mirroring the Android module layout"
}
func (iosFeatureGenerator) Kind() Kind { return KindIOS }

func (iosFeatureGenerator) Generate(env *Env) error {
    files := []struct{ tpl, path string }{
        {"ios/project.pbxproj", "iosApp/iosApp.xcodeproj/project.pbxproj"},
        {"ios/Config.xcconfig", "iosApp/Configuration/Config.xcconfig"},
        {"iosfeature/iOSApp.swift", "iosApp/iosApp/iOSApp.swift"},
        {"iosfeature/AppRouter.swift", "iosApp/iosApp/AppRouter.swift"},
    }
    for _, f := range files {
        if err := env.Render(f.tpl, f.path); err != nil {
            return err
        }
    }

    // One folder per root destination, so the structure matches Android's.
    for _, tab := range env.Ctx.Tabs {
        path := fmt.Sprintf("iosApp/iosApp/Views/%s/%sView.swift", tab.Pascal, tab.Pascal)
        if err := env.RenderWith("iosfeature/FeatureView.swift", path, tab); err != nil {
            return err
        }
    }
    return nil
}
```

`Kind()` decides where it is offered: `KindIOS` puts it on the iOS layout screen,
`KindAndroid` on the Android one.

### 2. Add the templates

Create `internal/assets/iosfeature.tmpl` with a `{{define}}` block per file:

```
{{define "iosfeature/FeatureView.swift"}}
import SwiftUI
import SharedLogic

struct {{ .Pascal }}View: View {
    var body: some View {
        Text("{{ .Label }}")
    }
}
{{end}}
```

Templates are embedded with `//go:embed *.tmpl`, so a new file in that directory
is picked up with no registration. Blocks rendered with `env.Render` receive the
full `Ctx` (`.Spec`, `.Res`, `.Tabs`, `.Feature`); `env.RenderWith` passes
whatever you give it, as above.

### 3. That is it

The wizard's iOS layout screen now lists your generator, `--ios-layout
swiftui-custom` works, and the choice is recorded in `.kmp-scaffold.json` so
later `add` runs use the same layout.

### Supporting `add feature`

To let `kmp-scaffold add feature` extend your layout, also implement
`GenerateFeature`:

```go
func (iosFeatureGenerator) GenerateFeature(env *Env) error {
    f := env.Ctx.Feature
    path := fmt.Sprintf("iosApp/iosApp/Views/%s/%sView.swift", f.Pascal, f.Pascal)
    return env.Render("iosfeature/FeatureView.swift", path)
}
```

A layout without it still works for `new`; `add feature` just says the layout
does not support being extended, rather than half-generating something.

## Adding a library pack

Everything about a library lives in `internal/catalog/catalog.go`.

**1. A version key**, in `VersionKeys()`, with the coordinate to probe:

```go
{Key: "sqldelight", Section: "Network & Multiplatform Utilities",
    Probe:    Coordinate{"app.cash.sqldelight", "runtime", Central},
    Baseline: "2.0.2"},
```

`Baseline` is the offline fallback, not a pin. Add `MinChannel: Bleeding` if the
library has never had a stable release.

**2. The artifacts**, in `Libraries()`, gated on a pack:

```go
{"sqldelight-runtime", "app.cash.sqldelight:runtime", "sqldelight",
    "External Libraries", Pack("sqldelight")},
{"sqldelight-coroutines", "app.cash.sqldelight:coroutines-extensions", "sqldelight",
    "External Libraries", Pack("sqldelight")},
```

The last field is a predicate. `Pack(id)`, `Util(id)`, `Extra(id)`, `AndroidOnly`,
`IOSOnly`, `Always`, `And(...)` and `AnyPack(...)` compose to describe when the
artifact applies.

**3. The pack itself**, in `Packs()`:

```go
{ID: "sqldelight", Label: "SQLDelight",
    Description: "Typed SQL, generated from your schema.",
    Tier:        TierExtra},
```

`TierBasic` puts it in the default set; `TierExtra` makes it opt-in. Add
`RequiresIOS` or `RequiresAndroid` if it only makes sense on one platform.

**4. A plugin**, if it needs one, in `Plugins()`:

```go
{"sqldelight", "app.cash.sqldelight", "sqldelight", Pack("sqldelight")},
```

Then reference it from the module templates that need the dependency:

```
{{- if .Spec.HasPack "sqldelight" }}
    implementation(libs.sqldelight.runtime)
{{- end }}
```

`go test ./internal/catalog` checks that every library and bundle references a
version key that exists and that every key has a baseline, so a typo fails
immediately.

## Adding a shared utility

A utility is a group of files in `sharedLogic` the wizard can toggle.

**1. Declare it**, in `SharedUtilities()`:

```go
{ID: "analytics", Label: "Analytics facade",
    Description: "A shared Analytics interface with a no-op default implementation.",
    Default:     true, Requires: []string{"koin-di"}},
```

`Requires` names other utilities; `RequiresPack` names library packs. Both are
enforced by `Normalise`, which pulls in dependencies and drops anything whose
requirements are missing, explaining each change.

**2. Add the templates** to `internal/assets/shared.tmpl`.

**3. List the files** in `internal/generator/shared.go`:

```go
{"shared/Analytics.kt", common("core/analytics/Analytics.kt"),
    spec.HasSharedUtil("analytics")},
```

The third field is the condition, so a utility that is switched off writes
nothing.

## Adding an anchor

If a new generated file needs to be extended by `add feature`, give it an anchor.

**1. Name it** in `internal/wire/wire.go`:

```go
// AnchorAnalytics is in sharedLogic's AnalyticsRegistry.kt.
AnchorAnalytics = "kmp-scaffold:analytics"
```

**2. Emit it** from the template, on its own line, indented to match what will be
inserted:

```kotlin
val registered = listOf(
    homeAnalytics,
    // kmp-scaffold:analytics
)
```

**3. Insert into it** from `featureEdits` in `internal/generator/project.go`:

```go
edits = append(edits, wire.Edit{
    Path:    "sharedLogic/.../AnalyticsRegistry.kt",
    Anchor:  wire.AnchorAnalytics,
    Lines:   []string{fmt.Sprintf("%sAnalytics,", f.Camel)},
    Imports: []string{fmt.Sprintf("import %s.feature.%s.analytics", pkg, f.Pkg)},
})
```

Insertion matches the anchor's indentation, skips lines already present, and
sorts new imports into the existing block.

`TestGeneratedProjectHasEveryAnchor` asserts every anchor exists in a freshly
generated project — add yours to that list so a template edit cannot quietly
remove it.

## Project structure

```
internal/
├── model/       the KMP spec, the project manifest, naming helpers
├── catalog/     every library, plugin, pack and version key
├── resolve/     Maven / SDK / Gradle lookups, version comparison, compatibility rules
├── render/      template execution, file writing, collision handling
├── wire/        anchor-based edits to existing files
├── scaffold/    what a template is: Template, Question, Answers, the registry
│   └── filetmpl/    templates written as a directory rather than as Go
├── kmp/         the kmp-mobile template
├── generator/   the layout registry and the concrete generators it drives
├── assets/      *.tmpl, embedded
├── tui/         the wizard: steps, flows, styling
└── cli/         command dispatch and flag parsing
```

The dependency direction is one-way: `cli` → `tui` → `scaffold` → `resolve` →
`catalog` → `model`, with `kmp` sitting on top of `scaffold` and `generator`.
Nothing imports `cli`, `model` imports nothing internal, and — the rule that
keeps the interface honest — **`scaffold` must not import the packages that
implement templates**. They register themselves from their own `init()`.

## Testing a change

```bash
task check     # vet, gofmt, tests
```

`internal/kmp/generate_test.go` generates complete projects offline into
temporary directories and asserts on the output — that every expected file
exists, that names agree across `App.kt`, the routes and the catalog, that
optional files are absent when their feature is off, and that `add feature`
wires every place it should. A change that breaks the wiring fails there rather
than in someone's IDE.

`internal/kmp/golden_test.go` is the other half: it fingerprints every file of
three whole generated projects against `testdata/golden-*.txt`. That is the
safety net for a refactor meant to change nothing — a stray whitespace change in
a template, or a reordered resolver pass, shows up as a named diff. When the
output is *meant* to change, read the diff, satisfy yourself that every line of
it was intended, then re-run with `-update`:

```bash
task golden
```
