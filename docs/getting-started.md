# Getting started

This page walks through creating your first project. It assumes you have
`kmp-scaffold` on your `PATH` — see the [README](../README.md#install) if not.

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

### 1–3. Name, directory, package

The project name becomes `rootProject.name`, the Android app label, and the
prefix for generated Kotlin types — `Tunesic` gives you `TunesicTheme` and
`TunesicApplication`.

The package is reverse-DNS (`io.kontour.tunesic`) and doubles as the Android
`applicationId`. It cannot contain a Kotlin keyword; the wizard tells you if it
does.

### 4. Platforms

Tick Android, iOS, or both. `sharedLogic` is always generated — it is the point
of the exercise.

### 5. Android navigation layout

Two options today, both built on Navigation 3:

- **Navigation 3 + adaptive shell** — root tabs in a floating bar on a phone,
  which becomes a navigation rail on a tablet and a permanent drawer on a
  desktop-width window. Each tab keeps its own back stack.
- **Navigation 3, single stack** — one back stack, no tabs. For apps that
  navigate from a single entry point.

These come from a registry, so a new layout added later appears here
automatically — see [Extending](extending.md).

### 6. Root tabs

Only asked for the shell layout. Comma-separated, in order:

```
Home, Library, Settings
```

Each becomes a real feature module (`feature/home/api` and `feature/home/impl`),
a `RootRoute`, and an entry in the navigation bar with a sensible Material
Symbols icon picked from its name. Six is the practical maximum on a phone, and
the wizard will say so.

### 7. iOS layout

- **SwiftUI, one target per feature** (default) — a local Swift package with a
  target per feature, a shared `CoreNavigation` module for route payloads, and a
  `TabView` giving each tab its own `NavigationPath`. This is the counterpart to
  the Android `api`/`impl` split, and the one `kmp-scaffold add feature` can
  extend. See [iOS architecture](ios-architecture.md).
- **SwiftUI, single entry point** — one `ContentView.swift`. Fine for a small
  app or a spike.
- **No iOS project files** — still builds the framework; add the Xcode project
  yourself later.

The modular layout needs the shared framework as an XCFramework, so there is one
extra step before the first Xcode build (`./iosApp/build-framework.sh`). The
generator tells you, and so does `iosApp/README.md`.

### 8. Shared utilities

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

### 9. Android extras

Wiring in `androidApp` and `core/ui` that most apps end up writing by hand:
bottom-sheet and dialog scene strategies, the splash screen, a global snackbar
host that survives navigation, an offline banner, and edge-to-edge handling.

### 10–11. Libraries

First pick a starting point — **Basic** (images, local database, secrets,
logging), **Minimal** (core only), or **Everything** — then fine-tune the exact
list on the next screen.

The core is always present and is not offered as a choice: Compose,
Navigation 3, Koin, Ktor, coroutines, serialisation, datetime, lifecycle and the
test stack.

See [Libraries and versions](libraries-and-versions.md) for what each pack pulls
in.

### 12. Version channel

How current you want to be:

- **Latest, including release candidates** (default) — finals plus `-rc` and
  `-beta`.
- **Stable only** — finals. The safest choice if you are shipping soon.
- **Bleeding edge** — alphas too.

Libraries that have never had a stable release always use their newest track
regardless, because there is nothing else available.

### 13. Minimum Android SDK

`compileSdk` and `targetSdk` are resolved for you from Google's SDK index,
capped at what your Android Gradle Plugin version supports. Only `minSdk` is
your call.

### 14. Resolution

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

### 15. Review

A final checklist of every decision, plus the toolchain versions. <kbd>enter</kbd>
generates; <kbd>esc</kbd> goes back to change something.

## After generating

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
into `.run/`. See [project layout](project-layout.md#run).

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
  --yes
```

`kmp-scaffold new --help` lists them all. The wizard is also skipped
automatically when stdout is not a terminal, so piping output works without
`--yes`.

Add `--dry-run` to see exactly what would be written first.
