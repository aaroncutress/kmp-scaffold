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
  [docs/kmp-mobile/libraries.md](docs/kmp-mobile/libraries.md)).
- **Tests and CI, if you want them** — test source sets with one worked example
  per platform, and a GitHub Actions workflow building both sides on every pull
  request. `--no-tests` and `--no-ci` leave out the files *and* the dependencies
  that would have gone unused.
- **A git repository, `.editorconfig` and `.gitattributes`** — so the first
  thing you change shows up as a diff, and so a checkout on Windows and one on
  macOS produce the same working tree.

## Install

Download a build for your platform from
[the latest release](https://github.com/aaroncutress/kmp-scaffold/releases/latest),
or with Go 1.26 or newer:

```bash
go install github.com/aaroncutress/kmp-scaffold@latest
```

Or build from a clone:

```bash
git clone https://github.com/aaroncutress/kmp-scaffold
cd kmp-scaffold
task install          # or: go build -o kmp-scaffold .
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
| `kmp-scaffold new [dir]` | Create a project from a template. Interactive unless `--yes`. |
| `kmp-scaffold add <what> [name]` | Apply one of the template's recipes and wire it in — `add feature` on a Kotlin Multiplatform project. `add` alone lists them. |
| `kmp-scaffold add library <pack>…` | Resolve and add a library pack to the version catalog. |
| `kmp-scaffold templates [name]` | List the templates `new` can generate from, or show what one asks. |
| `kmp-scaffold templates add <ref>` | Fetch a template from a git repository, after showing you what it does. |
| `kmp-scaffold versions` | Report which dependencies have newer releases. |
| `kmp-scaffold version` | Print the tool's version. |

Every command that writes files accepts `--dry-run` (report without writing) and
`--force` (overwrite files that already exist). Run any command with `--help` for
its full flag list.

## Documentation

[**docs/**](docs/README.md) is the index. It is split the way the tool is: pages
about `kmp-scaffold` itself, and pages about `kmp-mobile`, the template it
generates from by default.

**The tool**

- **[Getting started](docs/getting-started.md)** — the wizard, screen by screen.
- **[Using a template](docs/templates/using.md)** — generating from something
  other than the default, and fetching one from a git repository.
- **[Writing a template](docs/templates/writing.md)** — the `template.toml`
  reference and recipes, without touching Go.
- **[Extending kmp-scaffold](docs/extending.md)** — adding a built-in template,
  a project layout, or an anchor, in Go.
- **[Troubleshooting](docs/troubleshooting.md)** — when generating, the wizard,
  or a remote template misbehaves.

**The default template**

- **[kmp-mobile](docs/kmp-mobile/README.md)** — what it generates, what is
  optional, and how to build it.
- **[Project layout](docs/kmp-mobile/project-layout.md)** — what each generated
  module is for, and why the navigation is split the way it is.
- **[iOS architecture](docs/kmp-mobile/ios-architecture.md)** — the modular
  Swift package layout, how it maps onto Android, and the XCFramework it needs.
- **[Adding features](docs/kmp-mobile/features.md)** — feature modules, root
  tabs, sheets and dialogs, and what gets wired where.
- **[Libraries and versions](docs/kmp-mobile/libraries.md)** — library packs,
  release channels, and keeping a project current.

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

Tasks are in [`Taskfile.yml`](Taskfile.yml), run with
[Task](https://taskfile.dev). `task` on its own lists them.

```bash
task check     # vet, gofmt and the tests - what CI runs
task build     # ./kmp-scaffold
task run -- new my-app --yes
task dist      # cross-compile release archives into dist/
```

Nothing in there needs Task, though: every task is a `go` command you can run
by hand. CI installs Task and runs `task check`, so a green build means those
exact commands passed.

CI runs on pull requests and on pushes to `main` — a work-in-progress branch
runs nothing, so `task check` locally is the fast loop. Tagging `v*` builds,
verifies and publishes a release.

The generator has no hidden state: `go test ./internal/kmp` builds complete
projects offline into temporary directories and asserts on the output, so a
change that breaks the wiring fails the tests. It also fingerprints three whole
generated projects against `internal/kmp/testdata/golden-*.txt`, so a refactor
meant to change nothing shows up as a diff rather than as a surprise. When
output is *meant* to change, read the diff, satisfy yourself that every line of
it was intended, then re-run with `-update`:

```bash
task golden
```
