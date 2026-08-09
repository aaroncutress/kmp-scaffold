# Project layout

**Template: `kmp-mobile`.** What this template generates, and why it is arranged
this way. A different template writes a different tree; nothing below is a
property of the tool.

```
my-app/
├── settings.gradle.kts          every module, plus the anchor `add feature` appends to
├── build.gradle.kts             plugins declared once, applied per module
├── gradle/libs.versions.toml    the single source of truth for versions
├── buildSrc/                    convention plugins
├── sharedLogic/                 Kotlin Multiplatform: logic, repositories, ViewModels
├── core/navigation/             Route, Navigator, back-stack state, scene strategies
├── core/ui/                     theme, design tokens, shared components, the nav shell
├── feature/<name>/api/          a feature's routes
├── feature/<name>/impl/         a feature's screens
├── androidApp/                  the composition root
├── iosApp/                      the SwiftUI app, and its Features package
└── .run/                        JetBrains run configurations, committed
```

The iOS side has its own layout, mirroring the module split above — see
[iOS architecture](ios-architecture.md).

## The dependency rule

```
androidApp ──► feature/*/impl ──► feature/*/api
     │              │                  │
     │              ▼                  ▼
     └──────► core/ui ─────────► core/navigation
                    │
                    ▼
               sharedLogic
```

The important consequence: **a feature depends on other features' `api` modules,
never their `impl`**. `feature/library/impl` can navigate to
`feature/settings/api`'s `SettingsRoute` without knowing anything about the
settings screen, so the two compile independently and changing one screen does
not rebuild the other.

Only `androidApp` depends on every `impl`, because it is the composition root —
the one place that knows the whole app exists.

## sharedLogic

The Kotlin Multiplatform module. Everything that is not a screen lives here, so
Android and iOS run identical logic rather than two implementations that drift.

```
sharedLogic/src/
├── commonMain/kotlin/<pkg>/
│   ├── core/presentation/BaseViewModel.kt
│   ├── core/network/KtorNetworkEngine.kt
│   ├── core/util/{NetworkObserver,NetworkViewModel,Clock,Platform}.kt
│   ├── core/database/{AppDatabase,DatabaseBuilder}.kt
│   ├── di/SharedModules.kt
│   └── feature/<name>/{domain,data,presentation,di}/
├── androidMain/kotlin/<pkg>/    actual implementations (ConnectivityManager, OkHttp, SharedPreferences)
├── iosMain/kotlin/<pkg>/        actual implementations (NWPathMonitor, Darwin, NSUserDefaults)
└── commonTest/kotlin/<pkg>/
```

Inside a feature package the layers are conventional and worth keeping:

- **`domain`** — interfaces and models. No dependencies on anything concrete.
- **`data`** — the implementations: HTTP calls, DAOs, caches.
- **`presentation`** — the ViewModel and its UI state.
- **`di`** — the Koin module binding them together.

The ViewModel depends on the `domain` interface, which is what lets a test hand
it a fake without a network.

### BaseViewModel

```kotlin
abstract class BaseViewModel<State : Any> : ViewModel() {
    abstract val uiState: StateFlow<State>
    val state: State get() = uiState.value
}
```

It extends KMP-ObservableViewModel's `ViewModel` rather than the AndroidX one,
which is what makes the same class observable from SwiftUI as well as Compose.
Fixing the shape to one `StateFlow` of one immutable state type means every
screen collects exactly one thing.

### The DI graph

`SharedModules.kt` holds a `coreModule` (HTTP client, clock, database) and sums
every feature module into `sharedModules`. `platformModule` is `expect`/`actual`,
so each platform supplies its own HTTP engine and key-value store.

Android starts it in `Application.onCreate()`; iOS calls `KoinHelper.shared.setup()`
from `iOSApp.init()`. Adding a feature adds one line to the `includes(...)` list —
and `kmp-scaffold add feature` adds that line for you.

## core/navigation

Navigation 3's building blocks, wrapped so features do not each invent their own.

**`Route.kt`** defines the destination contract:

```kotlin
enum class Presentation { SHELL, ABOVE_NAV, OVERLAY }

interface Route : NavKey, Parcelable {
    val presentation: Presentation get() = Presentation.SHELL
}

interface RootRoute : Route
```

`Presentation` is what decides which back stack a destination lands on — see
below.

It also defines `routeSerializers`, and this is the part worth understanding:

> Navigation 3 serialises the back stack polymorphically so it survives rotation
> and process death. `Route` is a plain interface spanning many Gradle modules
> rather than one sealed class, so kotlinx.serialization cannot discover its
> subclasses automatically. **Every concrete route needs an explicit
> `subclass(...)` registration**, or the back stack silently fails to save and
> resets to the start destination on the next rotation.

The registration lives in the same file as the route it registers, so adding a
route and registering it are the same edit. `androidApp/AppSerializers.kt` just
sums each module's contribution.

**`NavigationState.kt`** holds one back stack per root tab plus a global stack
above them. Per-tab stacks are what make switching tabs return you to where you
were instead of resetting.

**`Navigator.kt`** is the single navigation entry point, reached through
`LocalNavigator`. It routes by presentation:

| Presentation | Lands on | Looks like |
| --- | --- | --- |
| `SHELL` | The current tab's stack | Stays inside the shell, navigation bar visible |
| `ABOVE_NAV` | The global stack | Full screen, covers the navigation bar |
| `OVERLAY` | The global stack, via a scene strategy | Bottom sheet or dialog |

**`NavigationScenes.kt`** implements the sheet and dialog scene strategies. Both
are `OverlayScene`s, which is what keeps the screen underneath composed and
visible through the scrim instead of being torn down.

## core/ui

The theme and the components shared across features.

- **`theme/`** — `Theme.kt` (with dynamic colour on Android 12+), a neutral
  Material 3 palette to replace with your brand, typography, shapes, and
  `Tokens.kt`. Reach for `AppTheme.spacing.md` rather than `16.dp` so spacing
  stays consistent and is changeable in one place.
- **`components/Navigation.kt`** — the adaptive shell. `ResponsiveNavigationShell`
  picks a floating bar, a rail or a permanent drawer from the window size class,
  and holds the `NavDisplay` in a slot that is never torn down when the layout
  changes — which is why navigation transitions survive a rotation or a window
  resize.
- **`components/Snackbar.kt`** — a host mounted as a sibling of the whole
  `NavDisplay`, so a snackbar stays put across navigation instead of vanishing
  with the screen that posted it.
- **`components/ConnectivityBanner.kt`** — driven by the shared network observer,
  so both platforms agree on what "offline" means.

### The floating navigation bar

The selection pill follows your finger while you drag along the bar, with a
haptic tick per item crossed, and only commits the navigation when you let go.
A mis-aimed tap can be corrected without leaving the screen you are on. Dragging
past either end squashes the pill instead of stopping dead.

The same behaviour is used vertically by the rail and the drawer.

## androidApp

The composition root, and the only module that knows the whole app.

**`App.kt`** holds two `NavDisplay`s. The outer one owns the global back stack —
anything pushed above the shell, plus sheets and dialogs. The inner one owns the
current tab's stack. Keeping them separate is what lets a pushed screen cover
the navigation bar while each tab still remembers its own history.

**`AppSerializers.kt`** sums each feature's serializer registrations.

**`<Name>Application.kt`** starts Koin and, if you enabled images, points Coil at
the shared Ktor client so authentication and timeouts are configured once.

## feature/&lt;name&gt;

Each feature is two Gradle modules:

- **`api`** — the routes. Small, dependency-light, and depended on by any feature
  that needs to navigate here.
- **`impl`** — the screens, plus an `entries()` function contributing to the
  app's entry provider. Depended on only by `androidApp`.

The UI lives here; the state does not. A feature's ViewModel and repository live
in `sharedLogic` so iOS can use them too.

## buildSrc

Four convention plugins, so module build scripts stay four lines long:

| Plugin | What it does |
| --- | --- |
| `<name>.android.library` | SDK levels and Java target for every Android library |
| `<name>.android.compose` | The above, plus Compose and the Compose bundle |
| `<name>.android.feature` | The above, plus the shared modules and every feature `api` |
| `<name>.secrets` | Loads `secrets.properties` once and shares it via `rootProject.extra` |

`buildSrc/build.gradle.kts` pins the Android and Kotlin Gradle plugins on the
buildscript classpath, which is what lets the version-less plugin aliases in
`libs.versions.toml` resolve. Those pins are generated from the same resolved
versions as the catalog, so they cannot drift.

## .run

The run configurations, ready in the IDE's run dropdown the first time the
project is opened. Unlike `.idea/`, `.run/` is meant to be committed — it is
shared project configuration, not per-developer state, which is why the
generated `.gitignore` leaves it alone.

| Configuration | Runs | Generated when |
| --- | --- | --- |
| `androidApp` | the Android app on a device or emulator | Android is included |
| `iosApp` | the SwiftUI app on a simulator | an iOS layout is chosen |
| `Generate Build Konfig` | `:sharedLogic:generateBuildKonfig` | the secrets pack is on |
| `Link iOS Framework (Debug)` | `:sharedLogic:linkDebugFrameworkIosSimulatorArm64` | iOS is included |
| `Build iOS XCFramework (Debug)` | `:sharedLogic:assembleSharedLogicDebugXCFramework` | the modular iOS layout |

The last two are the ones worth knowing. **Link iOS Framework** is the fastest
way to see the real Kotlin error when an iOS build fails, because Xcode
truncates the output of the Gradle build phase. **Build iOS XCFramework** is the
modular layout's equivalent: Swift Package Manager consumes the XCFramework, so
assembling it — not linking a single framework — is what makes a new Kotlin
symbol visible to Swift. See
[iOS architecture](ios-architecture.md#the-xcframework).

They are plain files. Add your own — a `Run all tests` Gradle configuration, a
flavour-specific Android run — and they are committed alongside the generated
ones; regenerating never touches a configuration it did not write.

## Anchors

Generated files contain comments like:

```kotlin
// kmp-scaffold:entries
```

`kmp-scaffold add feature` inserts above them. They are ordinary comments —
delete one and the tool tells you which file it could not wire, instead of
silently doing nothing. Moving one is fine; the insertion follows it.
