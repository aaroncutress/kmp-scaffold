# Adding features

**Template: `kmp-mobile`.** `add` applies a template's own recipes to a project
it generated, so what `add feature` writes is this template's business. Another
template offers whatever recipes it declares — `kmp-scaffold add` with no
arguments lists them.

This template offers four: `feature`, and the three that turn on something the
project was generated without —
[`tests`, `ci` and `editorconfig`](project-layout.md#adding-them-later). Those
take no name, because there is one of each in a project.

```bash
cd my-app
kmp-scaffold add feature
```

The wizard asks three things: the name, which sides of the project it covers
(Android, iOS, shared logic), and how the screen should appear. Then it shows
you every file it will create and every file it will edit, before touching
anything.

Run it from anywhere inside the project — it finds the root by walking up to
`.kmp-scaffold.json`.

## Naming

Lowercase kebab-case: `billing`, `firmware-update`, `drive-logger`.

The name is transformed consistently everywhere it appears, and the differences
matter:

| Where | `firmware-update` becomes |
| --- | --- |
| Gradle path | `:feature:firmware-update:api` |
| Gradle type-safe accessor | `projects.feature.firmwareUpdate.api` |
| Kotlin package | `com.example.app.feature.firmwareupdate.api` |
| Types | `FirmwareUpdateRoute`, `FirmwareUpdateScreen` |
| Functions | `firmwareUpdateEntries()`, `firmwareUpdateNavSerializers` |

The camel-cased Gradle accessor for a kebab-cased module is the one that trips
people up when adding a module by hand.

## What gets created

**Android module** (`--targets android` for just this):

```
feature/<name>/api/build.gradle.kts
feature/<name>/api/src/main/kotlin/…/api/<Name>Route.kt
feature/<name>/impl/build.gradle.kts
feature/<name>/impl/src/main/kotlin/…/impl/<Name>Navigation.kt
feature/<name>/impl/src/main/kotlin/…/impl/<Name>Screen.kt
```

**iOS target** (modular iOS layout only):

```
iosApp/Packages/Features/Sources/CoreNavigation/<Name>Route.swift
iosApp/Packages/Features/Sources/<Name>/<Name>Screen.swift
iosApp/Packages/Features/Sources/<Name>/<Name>Destination.swift
```

**Shared logic:**

```
sharedLogic/…/feature/<name>/domain/<Name>Repository.kt
sharedLogic/…/feature/<name>/data/<Name>RepositoryImpl.kt
sharedLogic/…/feature/<name>/presentation/<Name>ViewModel.kt
sharedLogic/…/feature/<name>/di/<Name>Module.kt
```

By default you get all three, so the two platforms cannot drift apart. Narrow it
with `--targets`:

```bash
kmp-scaffold add feature billing --targets android,shared
kmp-scaffold add feature billing --targets ios
```

If a feature reuses another feature's ViewModel, drop `shared`.

### It does not write tests

Test files are yours. `add feature` creates the source sets' worth of production
code and leaves the tests to you, deliberately — a generated test asserting that
generated code does what it was generated to do proves nothing, and deleting it
is one more chore.

Where to put them, if the project was generated with tests:

- Logic in the shared ViewModel or repository → `sharedLogic/src/commonTest/`,
  which both platforms run. `SettingsViewModelTest` is the worked example.
- Android-only behaviour → `androidApp/src/test/`.
- Swift routes and views → `iosApp/Packages/Features/Tests/FeatureTests/`. That
  target depends on `CoreNavigation` alone, so add the feature target to its
  dependency list in `Package.swift` if you want to reach into it.

## What gets wired

This is the part that is tedious to do by hand and easy to half-finish:

| File | What is added |
| --- | --- |
| `settings.gradle.kts` | `include(":feature:<name>:api")` and `:impl` |
| `androidApp/build.gradle.kts` | `implementation(projects.feature.<name>.api)` and `.impl` |
| `buildSrc/…android.feature.gradle.kts` | The new `api` module, so other features can navigate to it |
| `androidApp/…/AppSerializers.kt` | `<name>NavSerializers`, plus its import |
| `androidApp/…/App.kt` | `<name>Entries()`, plus its import |
| `sharedLogic/…/di/SharedModules.kt` | `<name>Module` in the `includes(...)` list |
| `iosApp/Packages/Features/Package.swift` | the product, the target, and the `AppFeatures` umbrella |
| `iosApp/iosApp/App/AppCoordinator.swift` | the import, plus a tab or a destination registration |

The serializer registration is the one worth knowing about. Without it the
screen still works, but the back stack silently fails to save — navigate to the
screen, rotate the device, and you are back at the start destination. See
[Project layout](project-layout.md#corenavigation) for why.

Insertions are idempotent: nothing is duplicated if a line is already there.

The Swift test target is deliberately absent from that list. It depends on
`CoreNavigation` and nothing else, so adding a feature never changes it.

## How the screen appears

The wizard's third question sets the route's `Presentation`, which decides which
back stack it lands on.

### Full screen (`--presentation above-nav`)

The default. Pushed on top of the shell, covering the navigation bar.

```kotlin
@Serializable @Parcelize
data object BillingRoute : Route {
    @IgnoredOnParcel
    override val presentation = Presentation.ABOVE_NAV
}
```

### Bottom sheet (`--presentation overlay`)

A modal sheet over the current screen. The generated `Navigation.kt` tags the
entry so the sheet scene strategy picks it up:

```kotlin
entry<BillingRoute>(
    metadata = BottomSheetSceneStrategy.bottomSheet(
        enabledValues = BottomSheetSceneStrategy.FullHeightOnly
    )
) {
    BillingScreen()
}
```

Content inside a sheet can dismiss itself smoothly through `LocalSheetDismissal`,
which animates the sheet down before popping the stack rather than snapping it
away.

### Dialog (`--presentation dialog`)

The same idea, using the dialog scene strategy.

### Root tab (`--presentation shell`)

Only available with the adaptive shell layout. On iOS it becomes a new tab in
`AppCoordinator`'s `TabView`, with its own `AppTab` case and `NavigationPath`.

On Android, as well as the usual wiring, a root tab is added to:

- the `roots` set in `App.kt`, which is what gives it its own back stack;
- `NavigationItemsList` in `core/ui/…/Navigation.kt`, with a Material Symbols
  icon guessed from the name;
- `core/ui/build.gradle.kts`, since the item list references the route.

Its `entries()` call goes into the per-tab entry provider rather than the global
one — a tab is inside the shell, not on top of it. The iOS side makes the same
distinction: a tab gets a `NavigationStack`, anything else gets registered on
the `AppDestinations` modifier that every stack applies.

Root tabs are also the thing to be sparing with: more than five or six will not
fit a phone-width bar.

## Navigating to it

From any screen:

```kotlin
val navigator = LocalNavigator.current
navigator.navigate(BillingRoute)
```

`impl` modules can reach every other feature's `api` module through the
`android.feature` convention plugin, so no build-file change is needed to
navigate somewhere new.

To replace the current screen rather than pushing on top of it:

```kotlin
navigator.navigate(BillingRoute, replace = true)
```

## Filling in the generated code

The generated screens are placeholders with the wiring done. When the feature
includes shared logic, both the Compose screen and the SwiftUI screen already
collect the same ViewModel — see [iOS architecture](ios-architecture.md#using-shared-viewmodels).

The generated repository returns a canned value and points at where the real
call goes:

```kotlin
override suspend fun load(): Result<String> = runCatching {
    // TODO: replace with a real call, e.g.
    // client.get("$BASE_URL/billing").body<BillingResponse>()
    "Billing is wired up"
}
```

The ViewModel already has the loading/error shape most screens need, and the
screen already collects it with `collectAsStateWithLifecycle`.

## Non-interactive

```bash
kmp-scaffold add feature billing --yes
kmp-scaffold add feature billing --yes --presentation overlay
kmp-scaffold add feature billing --yes --targets android,shared
kmp-scaffold add feature billing --dry-run     # see the plan first
```

## If a wiring point is missing

If you have reorganised a file and removed an anchor comment, the tool says so
rather than failing:

```
! could not find a kmp-scaffold anchor comment in androidApp/src/main/kotlin/…/App.kt
  - wire the new module in by hand
```

Everything else is still generated and wired. Either paste the anchor comment
back where you want future insertions to go, or add the one line by hand.
