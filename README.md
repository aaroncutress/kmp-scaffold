# kmp-scaffold

A command-line tool that creates Kotlin Multiplatform projects — Android app,
iOS app and a shared `sharedLogic` module — wired together the way a real
project ends up being wired, with dependency versions resolved live rather than
frozen into a template.

It walks you through the choices step by step, like `npx sv` does for SvelteKit.

```
$ kmp-scaffold new

kmp-scaffold · new project
Step 8 of 14

Which shared utilities do you want?

These live in sharedLogic and are used by both platforms. All are on by default.

  ◉ Koin DI graph
    SharedModules.kt with a coreModule plus expect/actual platformModule.
› ◉ BaseViewModel
    Shared ViewModel base exposing a typed uiState StateFlow to both platforms.
  ◉ Network observer
    expect/actual connectivity Flow (ConnectivityManager / NWPathMonitor) + NetworkViewModel.
  ◉ Ktor client factory
    Shared HttpClient builder with JSON content negotiation.
  ...

↑/↓ move · space toggle · a all · n none · enter confirm · esc back
```

## What you get

A project with the module layout, navigation architecture and shared utilities
already in place:

- **`sharedLogic`** — a Kotlin Multiplatform module holding business logic,
  repositories and ViewModels, with the utilities most apps need: a Koin graph,
  a `BaseViewModel`, an `expect`/`actual` network observer, a typed settings
  registry, a Ktor client factory, a testable clock, and optionally a Room
  database.
- **`androidApp` + `core/` + `feature/`** — Navigation 3 with a serialisable
  back stack, an adaptive shell (floating bar → rail → drawer as the window
  grows), bottom-sheet and dialog scene strategies, a Material 3 theme with
  design tokens, and one `api`/`impl` module pair per feature.
- **`iosApp`** — a SwiftUI app in one of two shapes: a modular layout with a
  local Swift package holding one target per feature and a shared
  `CoreNavigation` module (the default, mirroring the Android module split), or
  a single entry point for smaller apps. Both link the framework Gradle builds
  and start Koin at launch.
- **`gradle/libs.versions.toml`** — every version in one place, resolved against
  Maven Central, Google Maven and the Gradle release feed at the moment you
  generate, with compatibility rules applied (see
  [docs/libraries-and-versions.md](docs/libraries-and-versions.md)).

## Install

Requires **Go 1.26** or newer.

```bash
go install github.com/aaroncutress/kmp-scaffold@latest
```

Or build from a clone:

```bash
git clone https://github.com/aaroncutress/kmp-scaffold
cd kmp-scaffold
make install          # or: go build -o kmp-scaffold .
```

## Quick start

```bash
kmp-scaffold new                    # the wizard asks everything
cd my-app
./gradlew :androidApp:assembleDebug
```

Non-interactive, for scripts and CI:

```bash
kmp-scaffold new my-app \
  --name "My App" \
  --package com.example.myapp \
  --tabs Home,Library,Settings \
  --yes
```

## Commands

| Command | What it does |
| --- | --- |
| `kmp-scaffold new [dir]` | Create a project. Interactive unless `--yes`. |
| `kmp-scaffold add feature [name]` | Add a feature module and wire it into every place that needs to know about it. |
| `kmp-scaffold add library <pack>…` | Resolve and add a library pack to the version catalog. |
| `kmp-scaffold versions` | Report which dependencies have newer releases. |
| `kmp-scaffold version` | Print the tool's version. |

Every command that writes files accepts `--dry-run` (report without writing) and
`--force` (overwrite files that already exist). Run any command with `--help` for
its full flag list.

## Documentation

- **[Getting started](docs/getting-started.md)** — the wizard, screen by screen.
- **[Project layout](docs/project-layout.md)** — what each generated module is
  for, and why the navigation is split the way it is.
- **[Adding features](docs/adding-features.md)** — feature modules, root tabs,
  sheets and dialogs, and what gets wired where.
- **[Libraries and versions](docs/libraries-and-versions.md)** — library packs,
  how version resolution works, release channels, and keeping a project current.
- **[iOS architecture](docs/ios-architecture.md)** — the modular Swift package
  layout, how it maps onto the Android side, and the XCFramework it needs.
- **[Extending kmp-scaffold](docs/extending.md)** — adding a new project layout,
  a library pack, or a shared utility.
- **[Troubleshooting](docs/troubleshooting.md)** — what to do when a build fails.

## How it decides versions

Nothing is pinned in this repository except a fallback baseline used offline.
At generation time the tool:

1. Reads `maven-metadata.xml` for every artifact it is about to write.
2. Reads Google's Android SDK index for the newest platform and build-tools.
3. Reads the Gradle release feed for the current distribution and its checksum.
4. Applies compatibility rules — Kotlin against KSP, AGP against Gradle, AGP
   against `compileSdk` — and reports anything it had to adjust.

You pick how adventurous to be (stable only, release candidates, or alphas), and
libraries with no stable release ever — Navigation 3, adaptive Material — always
resolve on their newest track, because there is nothing else to resolve to.

## Development

```bash
make test      # go test ./...
make check     # vet, gofmt and tests
make build     # ./kmp-scaffold
```

The generator has no hidden state: `go test ./internal/generator` builds
complete projects offline into temporary directories and asserts on the output,
so a template change that breaks the wiring fails the tests.
