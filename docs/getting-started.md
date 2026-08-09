# Getting started

This page walks through creating your first project. It assumes you have
`kmp-scaffold` on your `PATH` — see the [README](../README.md#install) if not.

The wizard's screens depend on the template you generate from. The walkthrough
below uses [`kmp-mobile`](kmp-mobile/README.md), the default; the first few
screens and the last two are the same whatever you pick.

## Running the wizard

```bash
kmp-scaffold new
```

That is the whole command. Everything else is a question.

Throughout the wizard:

| Key | What it does |
| --- | --- |
| <kbd>↑</kbd> <kbd>↓</kbd> | Move between options |
| <kbd>space</kbd> | Tick or untick an option on a checklist |
| <kbd>a</kbd> / <kbd>n</kbd> | Tick all / untick all |
| <kbd>enter</kbd> | Confirm this screen and move on |
| <kbd>esc</kbd> | Go **back** to the previous screen — your answers are kept |
| <kbd>ctrl+c</kbd> | Quit. Nothing is written unless you reach the end. |

Going back and changing an answer is always safe. Nothing touches the disk until
the final confirmation screen.

## The screens

### 1–2. Name and directory

The project name becomes `rootProject.name`, the Android app label, and the
prefix for generated Kotlin types — `Tunesic` gives you `TunesicTheme` and
`TunesicApplication`.

### 3. Git repository

Whether to run `git init` and make an initial commit once everything is written,
so the first thing you change afterwards shows up as a diff.

It is only asked when it could do something: the question is skipped when the
target is already inside a working tree — generating into a subdirectory of an
existing project is ordinary, and nesting a repository inside another one is
almost never meant — and when `git` is not on your `PATH`.

Nothing here can fail the run. Without a configured `user.name` and
`user.email`, the repository is created and the files staged, and the summary
says what to set and what to run to finish.

### 4. Tests

Whether to generate test source sets, the dependencies they need, and one
worked example per platform: a shared test in `commonTest` that runs on every
target, a JVM unit test for the Android app, and a Swift test target.

Saying no leaves nothing behind — no test directories, no `testImplementation`
lines, and no testing entries in `libs.versions.toml`. Saying yes is the
default, and `./gradlew :sharedLogic:allTests` has something to run immediately.

Saying no is not permanent either: `kmp-scaffold add tests` puts all of it into
an existing project later, without touching anything you have written. See
[adding them later](kmp-mobile/project-layout.md#adding-them-later).

### 5. CI workflow

Whether to write `.github/workflows/ci.yml`: a build on every pull request and
on the default branch, with a macOS job building the iOS side when the project
has one. It runs the tests too, if you asked for them.

`kmp-scaffold add ci` adds it to a project that went without.

### 6. Package

Reverse-DNS (`io.kontour.tunesic`), doubling as the Android `applicationId`. It
cannot contain a Kotlin keyword; the wizard tells you if it does.

### 7. Platforms

Tick Android, iOS, or both. `sharedLogic` is always generated — it is the point
of the exercise.

### 8. Android navigation layout

Two options today, both built on Navigation 3:

- **Navigation 3 + adaptive shell** — root tabs in a floating bar on a phone,
  which becomes a navigation rail on a tablet and a permanent drawer on a
  desktop-width window. Each tab keeps its own back stack.
- **Navigation 3, single stack** — one back stack, no tabs. For apps that
  navigate from a single entry point.

These come from a registry, so a new layout added later appears here
automatically — see [extending kmp-scaffold](extending.md#adding-a-project-layout).

### 9. Root tabs

Only asked for the shell layout. Comma-separated, in order:

```
Home, Library, Settings
```

Each becomes a real feature module (`feature/home/api` and `feature/home/impl`),
a `RootRoute`, and an entry in the navigation bar with a sensible Material
Symbols icon picked from its name. Six is the practical maximum on a phone, and
the wizard will say so.

### 10. iOS layout

- **SwiftUI, one target per feature** (default) — a local Swift package with a
  target per feature, a shared `CoreNavigation` module for route payloads, and a
  `TabView` giving each tab its own `NavigationPath`. This is the counterpart to
  the Android `api`/`impl` split, and the one `kmp-scaffold add feature` can
  extend. See [iOS architecture](kmp-mobile/ios-architecture.md).
- **SwiftUI, single entry point** — one `ContentView.swift`. Fine for a small
  app or a spike.
- **No iOS project files** — still builds the framework; add the Xcode project
  yourself later.

The modular layout needs the shared framework as an XCFramework, so there is one
extra step before the first Xcode build (`./iosApp/build-framework.sh`). The
generator tells you, and so does `iosApp/README.md`.

### 11. Shared utilities

The checklist that matters most. These are the pieces of `sharedLogic` that
every one of your projects tends to grow anyway:

| Utility | What it gives you |
| --- | --- |
| Koin DI graph | `SharedModules.kt` with a `coreModule` and an `expect`/`actual` `platformModule` |
| BaseViewModel | A shared ViewModel base exposing one typed `uiState` StateFlow, observable from Compose *and* SwiftUI |
| Network observer | `ConnectivityManager` on Android, `NWPathMonitor` on iOS, behind one `Flow<Boolean>`, plus a `NetworkViewModel` |
| Ktor client factory | One configured `HttpClient`, engine injected per platform |
| App settings registry | Type-safe `AppSetting` tokens over multiplatform-settings — adding a setting is one line |
| Clock abstraction | `expect`/`actual` `SystemClock` so time-dependent logic is testable |
| Room database builder | `expect`/`actual` database builders and a starter `AppDatabase` |
| iOS Koin helper | `KoinHelper` so Swift can start and query the graph |

All are on by default. Turning one off simply means those files are not written.

Some depend on others (the network observer needs `BaseViewModel`); the wizard
pulls those in for you and says so on the summary screen.

### 12. Android extras

Wiring in `androidApp` and `core/ui` that most apps end up writing by hand:
bottom-sheet and dialog scene strategies, the splash screen, a global snackbar
host that survives navigation, an offline banner, and edge-to-edge handling.

### 13–14. Libraries

First pick a starting point — **Basic** (images, local database, secrets,
logging), **Minimal** (core only), or **Everything** — then fine-tune the exact
list on the next screen.

The core is always present and is not offered as a choice: Compose,
Navigation 3, Koin, Ktor, coroutines, serialisation, datetime, lifecycle and the
test stack.

See [Libraries and versions](kmp-mobile/libraries.md) for what each pack pulls
in.

### 15. Version channel

How current you want to be:

- **Latest, including release candidates** (default) — finals plus `-rc` and
  `-beta`.
- **Stable only** — finals. The safest choice if you are shipping soon.
- **Bleeding edge** — alphas too.

Libraries that have never had a stable release always use their newest track
regardless, because there is nothing else available.

### 16. Minimum Android SDK

`compileSdk` and `targetSdk` are resolved for you from Google's SDK index,
capped at what your Android Gradle Plugin version supports. Only `minSdk` is
your call.

### 17. Minimum iOS version

The counterpart to `minSdk`: the oldest iOS a device can be on and still run
this. iOS 18 by default, with iOS 26 and iOS 17 offered.

This is not only a build setting. It decides the `platforms:` line in the Swift
package, and Swift Package Manager only knows a platform its declared
`swift-tools-version` is new enough for — so the two move together.

### 18. Swift language mode

Swift 5 by default, Swift 6 on request. Swift 6 checks concurrency at compile
time, and the Kotlin framework's exported classes carry no `Sendable`
annotations — so anything crossing an actor boundary will need annotating by
hand. The generated Swift is written against mode 5.

Only asked when the project has an iOS side.

### 19. Java version

What the JVM targets compile to: Java 17 by default, with 21 and 11 offered.
This is `jvmTarget` and `sourceCompatibility`, not the JVM Gradle itself runs
on — the generated README says how to pin that.

### 20. Resolution

The tool now contacts Maven Central, Google Maven, Google's SDK index and the
Gradle release feed, and shows you what it found:

```
Resolved versions

  ✔ Gradle                 9.6.1            current release
  ✔ Android Gradle Plugin  9.3.1            latest preview
  ✔ Kotlin                 2.4.10           matched to KSP
  ✔ KSP                    2.3.10           matched to Kotlin
  ✔ compileSdk / targetSdk 37               Android SDK index
  ✔ Compose UI             1.12.0-rc01      latest preview
  ...

  · Navigation 3 1.2.0-alpha07 is pre-release - its API still moves between builds.

  42 artifacts checked in 1.6s
```

Anything the resolver had to adjust is explained here, not buried in a log.

### 21. Review

A final checklist of every decision, plus the toolchain versions. <kbd>enter</kbd>
generates; <kbd>esc</kbd> goes back to change something.

## After generating

If a repository was set up, everything generated is already in its first commit
— so whatever you change next is a diff against a known starting point, and
`kmp-scaffold add feature` shows up as one too.

```bash
cd my-app
cp secrets.properties.template secrets.properties   # if you enabled secrets
./gradlew :androidApp:assembleDebug
```

For iOS on the modular layout:

```bash
./iosApp/build-framework.sh    # once, before the first Xcode build
open iosApp/iosApp.xcodeproj
```

Swift Package Manager resolves the framework's path before Xcode runs any build
phase, so it has to exist first. After that the app's "Build Kotlin Framework"
phase keeps it current on every build.

On the single-entry-point layout there is no bootstrap step: just open
`iosApp/iosApp.xcodeproj` and run.

Opening the project in Android Studio or IntelliJ, the run dropdown is already
populated — the app targets plus the Gradle tasks you reach for most, generated
into `.run/`. See [project layout](kmp-mobile/project-layout.md#run).

## Skipping the wizard

Every answer has a flag, so the same project can be generated from a script:

```bash
kmp-scaffold new my-app \
  --name "My App" \
  --package com.example.myapp \
  --tabs Home,Library,Settings \
  --libraries images,database,secrets \
  --channel stable \
  --min-sdk 26 \
  --ios-deployment-target 18.0 \
  --jvm-target 17 \
  --yes
```

`kmp-scaffold new --help` lists them all. The wizard is also skipped
automatically when stdout is not a terminal, so piping output works without
`--yes`.

The git repository is set up in this mode too, unless the target is already
inside one. `--no-git` turns it off; `--git` forces it on.

Tests and a CI workflow are generated in this mode too. `--no-tests` and
`--no-ci` turn them off. Unlike the flags above, these apply to any template
that says it can generate them, not only to `kmp-mobile` — and a template that
cannot says so rather than accepting the flag and ignoring it.

Add `--dry-run` to see exactly what would be written first.

## Templates

`new` generates from a **template**, and the Kotlin Multiplatform project this
page describes is the default one, `kmp-mobile`. To see what is available:

```bash
kmp-scaffold templates              # the list
kmp-scaffold templates kmp-mobile   # what one asks, and what it can add later
kmp-scaffold new my-app --template kmp-mobile
```

When there is more than one, the wizard asks which to use before anything else,
because the answer decides what the rest of the questions are.

`--template` also takes a directory, so a template you are working on needs no
installation:

```bash
kmp-scaffold new my-api --template ./examples/templates/ktor-service
```

Which template built a project is recorded in its `.kmp-scaffold.json`, so
`kmp-scaffold add feature` extends it the same way it was made. Writing your own
is covered in [templates](templates/writing.md).
