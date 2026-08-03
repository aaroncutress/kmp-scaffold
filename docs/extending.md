# Extending kmp-scaffold

The tool is built so the things you are most likely to want to change — a new
project layout, a new library, a new shared utility — are additive. This page
covers the four common cases.

## Adding a project layout

This is the main extension point. A layout is a **generator**: something that
knows how to write one part of a project. They live in a registry, so the wizard
lists whatever is registered without being told about it.

Suppose you decide the iOS app should mirror the Android side with a folder per
feature. Today's `swiftui-simple` generator writes a single `ContentView.swift`;
you want a `swiftui-feature` alternative alongside it — not instead of it, so
existing projects still regenerate the way they were built.

### 1. Write the generator

Create `internal/generator/ios_feature.go`:

```go
package generator

func init() { Register(iosFeatureGenerator{}) }

type iosFeatureGenerator struct{}

func (iosFeatureGenerator) ID() string    { return "swiftui-feature" }
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

Create `internal/templates/iosfeature.tmpl` with a `{{define}}` block per file:

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
swiftui-feature` works, and the choice is recorded in `.kmp-scaffold.json` so
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

**2. Add the templates** to `internal/templates/shared.tmpl`.

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
├── model/       the project spec, the manifest, naming helpers
├── catalog/     every library, plugin, pack and version key
├── resolve/     Maven / SDK / Gradle lookups, version comparison, compatibility rules
├── render/      template execution, file writing, collision handling
├── generator/   the generator registry and the concrete generators
├── templates/   *.tmpl, embedded
├── wire/        anchor-based edits to existing files
├── tui/         the wizard: steps, flows, styling
└── cli/         command dispatch and flag parsing
```

The dependency direction is one-way: `cli` → `tui` → `generator` → `render` →
`catalog` → `model`. Nothing imports `cli`, and `model` imports nothing internal.

## Testing a change

```bash
make check     # vet, gofmt, tests
```

`internal/generator/generate_test.go` generates complete projects offline into
temporary directories and asserts on the output — that every expected file
exists, that names agree across `App.kt`, the routes and the catalog, that
optional files are absent when their feature is off, and that `add feature`
wires every place it should. A template change that breaks the wiring fails
there rather than in someone's IDE.
