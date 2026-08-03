# Troubleshooting

## Generation

### "already contains a Gradle project"

```
error: /path/to/dir already contains a Gradle project (settings.gradle.kts)
       - generate somewhere else, or pass --force
```

The safety check. Pick an empty directory, or pass `--force` if you really do
mean to regenerate over the top — `--force` overwrites files, so commit first.

### Some files were skipped

Without `--force`, files that already exist are left alone and listed:

```
Some files already existed and were left untouched:
  settings.gradle.kts
  androidApp/build.gradle.kts
Re-run with --force to overwrite them.
```

This is what you want when regenerating into a project you have edited. Files
whose content is already identical are reported as unchanged, not skipped.

### "could not download the Gradle wrapper"

The `gradlew` scripts and `gradle-wrapper.jar` are fetched from the Gradle
source tree. If that fails, everything else is still generated, only
`gradle-wrapper.properties` is missing its companions. Fix it with either:

```bash
gradle wrapper --gradle-version 9.6.1     # if you have Gradle installed
```

or just open the project in Android Studio, which offers to create the wrapper.

### Lookups failed and fell back to baselines

```
! 2 lookup(s) failed and fell back to baseline versions: koin (io.insert-koin:koin-core), ktor (io.ktor:ktor-client-core).
```

A network hiccup, or a proxy blocking Maven Central. Those two versions came
from the built-in baseline rather than the latest release; the rest are current.
Re-run, or run `kmp-scaffold versions` afterwards to catch up.

If you are behind a corporate proxy, the tool uses Go's default HTTP transport,
so `HTTPS_PROXY` and the system certificate store are honoured.

## Building the generated project

### `Unresolved reference: libs`

Gradle has not synced. In Android Studio, **File → Sync Project with Gradle
Files**. From the terminal, any `./gradlew` command does it.

### A newly added feature module is not found

Two possibilities:

1. Gradle has not synced since `add feature` edited `settings.gradle.kts`.
2. The type-safe accessor is wrong for a kebab-case name. Gradle camel-cases
   them: `:feature:firmware-update:api` is `projects.feature.firmwareUpdate.api`,
   not `firmware-update`. `add feature` gets this right; adding a module by hand
   is where it goes wrong.

### The back stack resets when I rotate the device

The route is not registered for serialisation. Every concrete `Route` needs an
explicit `subclass(...)` call:

```kotlin
val billingNavSerializers: SerializersModule = routeSerializers {
    subclass(BillingRoute::class)
    subclass(BillingDetailRoute::class)   // ← every route in this file
}
```

and that module must be summed into `appNavSerializersModule` in
`AppSerializers.kt`. `add feature` does both for the first route; adding a
*second* route to an existing file is the case to watch — add it to the same
`routeSerializers` block, which is why the registration lives next to the route
definitions.

### `Duplicate class` or `Could not resolve` after adding a library

The library is probably declared in two source sets — for example in both
`commonMain` and `androidMain`. A multiplatform artifact belongs in `commonMain`
only; the platform variant is selected automatically.

### The iOS build cannot find the shared framework

The Xcode project runs `./gradlew :sharedLogic:embedAndSignAppleFrameworkForXcode`
in a build phase. If it fails:

- Run `./gradlew :sharedLogic:linkDebugFrameworkIosSimulatorArm64` from the
  terminal to see the real error, which Xcode tends to truncate.
- Check `iosApp/Configuration/Config.xcconfig` has your `TEAM_ID` if you are
  building for a device. The simulator does not need it.
- Confirm you are on an ARM Mac. The generated project targets `iosArm64` and
  `iosSimulatorArm64`; add `iosX64()` to `sharedLogic/build.gradle.kts` for an
  Intel simulator.

### The iOS framework will not link after a Kotlin upgrade

Usually SKIE. It trails new Kotlin releases by a few days, and the tool warns
when it pins a SKIE version against a newer Kotlin:

```
· SKIE 0.10.14 is pinned against Kotlin 2.4.20-Beta2. SKIE usually trails new
  Kotlin releases by a few days - if the iOS framework fails to link, drop
  Kotlin one patch or remove the SKIE plugin.
```

Either drop Kotlin back one release in `libs.versions.toml`, or remove the SKIE
plugin from `sharedLogic/build.gradle.kts` — Swift interop still works, it is
just less idiomatic.

### A Kotlin function is not visible from Swift

Kotlin top-level functions are exported as `<FileName>Kt`. `currentPlatform()` in
`Platform.kt` arrives as `PlatformKt.currentPlatform()`. A Kotlin `object` is
`.shared` — `KoinHelper.shared.setup()`.

Names starting with `init` are renamed by the Objective-C exporter, which is why
the Koin entry point is `setup()` rather than `initKoin()`.

### `Compose Compiler` version mismatch

The Compose compiler plugin ships with Kotlin, so it has no version of its own.
It resolves through `buildSrc/build.gradle.kts`, which pins the Kotlin Gradle
plugin. If you bump `kotlin` in `libs.versions.toml`, bump it in
`buildSrc/build.gradle.kts` too — they are the same artifact, and the generated
comment above them says so.

### Material 3 APIs are unresolved

The navigation shell uses expressive Material 3 APIs that only exist on the
alpha track. If you generated with `--channel stable` you may have an older
Material 3 than the shell expects. Either move `androidx-compose-material3` in
`libs.versions.toml` onto the alpha track, or replace the shell's
`FloatingNavBar` with a standard `NavigationBar`.

## The wizard

### It did not appear

The wizard is skipped when stdin or stdout is not a terminal — inside CI, when
piping output, or under some IDE terminals. Pass `--yes` with flags to generate
non-interactively, or run in a real terminal.

### Going back lost my answer

It should not: <kbd>esc</kbd> keeps everything. Changing an answer that affects
version resolution (packs, channel, minSdk) does re-run the resolution when you
reach that screen again, which takes a second or two.

### I want to see what it would do first

```bash
kmp-scaffold new my-app --yes --dry-run
kmp-scaffold add feature billing --dry-run
```

Both list every file that would be created and every file that would be edited,
and write nothing.
