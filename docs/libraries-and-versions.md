# Libraries and versions

How the tool decides which versions to write, and how to keep a project current
afterwards.

## The core

Always present, never offered as a choice, because the generated code uses it:

Compose (UI, foundation, Material 3, adaptive), Navigation 3, Koin,
KMP-ObservableViewModel, Ktor (core, content negotiation, JSON), kotlinx
serialisation / coroutines / datetime, AndroidX lifecycle and activity,
Material Symbols icons, and the test stack (kotlin-test, Turbine, Koin test,
coroutines test, Ktor MockEngine).

## Library packs

Everything else is a **pack** — a named group of artifacts that are only useful
together.

### On by default (the "basic" set)

| Pack | What it adds |
| --- | --- |
| `images` | Coil 3 plus its Ktor fetcher, wired to the shared `HttpClient` |
| `database` | Room KMP, bundled SQLite, KSP processors, an exported schema directory |
| `secrets` | BuildKonfig — `secrets.properties` becomes typed fields and manifest placeholders |
| `logger` | A multiplatform `Log` facade usable from `commonMain` |
| `skie` | SKIE, which turns sealed classes, flows and suspend functions into idiomatic Swift (iOS only) |

### Available on request

| Pack | What it adds |
| --- | --- |
| `paging` | Paging 3, plus Room's paging support if the database pack is on |
| `media3` | ExoPlayer and its Compose UI artifacts |
| `maps` | Google Maps Compose, plus the manifest API-key placeholder |
| `location` | Play services location and the coroutines interop artifact |
| `charts` | Vico, with Material 3 chart theming |
| `ble` | Kable, multiplatform Bluetooth LE |
| `passkeys` | AndroidX Credential Manager and the Play services back-port |
| `supabase` | BOM-managed auth, postgrest, storage and functions clients |
| `koin-compiler` | Compile-time verification of the Koin graph |
| `browser` | Custom Tabs |
| `appcompat` | Only needed when interoperating with View-based screens |

Packs are also what keeps the version catalog honest: nothing that no module
depends on is written into `libs.versions.toml`.

## Adding a pack later

```bash
kmp-scaffold add library paging
```

This resolves the newest compatible versions, adds the `[versions]` and
`[libraries]` entries to `gradle/libs.versions.toml`, and prints the dependency
lines to paste into the modules that need them:

```
Add to the modules that need them
  implementation(libs.androidx.paging.common)
  implementation(libs.androidx.paging.compose)

Shared code goes in sharedLogic's commonMain block; Compose-only libraries go in
androidApp or core/ui.
```

The last step is deliberately yours: only you know whether a library belongs in
`commonMain`, `androidMain` or an Android-only module, and getting that wrong is
a confusing compile error rather than an obvious one.

`kmp-scaffold add library --help` lists every pack. Add `--dry-run` to preview.

## How resolution works

Nothing in this repository pins a "current" version. At generation time the tool
makes four kinds of request:

1. **`maven-metadata.xml`** from Maven Central, Google Maven or the Gradle Plugin
   Portal, for every artifact it is about to write — in parallel.
2. **Google's Android SDK index** (`repository2-3.xml`) for the newest platform
   and build-tools.
3. **The Gradle release feed** for the current distribution and its SHA-256, so
   `gradle-wrapper.properties` gets a verified checksum.
4. **The Gradle source tree** for `gradlew`, `gradlew.bat` and
   `gradle-wrapper.jar`, so the wrapper is byte-for-byte what `gradle wrapper`
   would have produced.

### Release channels

Each candidate version is classified from its qualifier:

| Channel | Accepts |
| --- | --- |
| `stable` | Final releases only |
| `preview` (default) | Finals, plus `-rc` and `-beta` |
| `bleeding` | The above, plus `-alpha`, `-dev`, `-SNAPSHOT` |

Suffixes that are variants rather than prereleases — `0.8.0-0.6.x-compat`,
`1.0.6-kotlin-2.4.20` — count as releases, but the plain artifact wins when both
exist.

Some libraries carry a **minimum channel** because they have never had a stable
release: Navigation 3 and adaptive Material are alpha-only, so asking for
`stable` still gets you their newest alpha, and the tool tells you it relaxed
the rule.

### Compatibility rules

Picking the newest of everything independently produces combinations that do not
build. These rules are applied afterwards:

**Kotlin ↔ KSP.** KSP has used two version schemes. The classic one embeds the
Kotlin version (`2.2.20-2.0.4`), so a Kotlin release with no matching KSP build
cannot be used at all — the resolver steps Kotlin back until it finds one, and
says so:

```
! Kotlin 2.3.0 has no KSP release yet; pinned Kotlin 2.2.21, which KSP 2.2.21-2.0.5 supports.
```

KSP2 versions independently (`2.3.10`), in which case both simply take their
newest. Both schemes are handled, so this keeps working as KSP changes.

**AGP ↔ Gradle.** Each AGP major has a minimum Gradle version. The tool always
uses the current Gradle release, so this only bites when you pin an older one
with `--gradle` — and then it is reported as an error, not a surprise at build
time.

**AGP ↔ compileSdk.** AGP refuses to build against a platform it does not know
about. The newest platform from the SDK index is capped at what your AGP major
supports:

```
· Android SDK 38 is available, but AGP 9.3.1 supports at most compileSdk 37 - using 37.
```

**Compose compiler ↔ Kotlin.** The Compose compiler plugin ships with Kotlin, so
it is not versioned separately; `buildSrc` pins it to the same version.

**SKIE ↔ Kotlin.** SKIE usually trails new Kotlin releases by a few days. It is
resolved independently and flagged, since the failure mode (the iOS framework
does not link) is otherwise cryptic.

## Pinning versions

Any resolved version can be overridden:

```bash
kmp-scaffold new my-app --kotlin 2.4.10 --agp 9.2.1 --gradle 9.6.1 --compile-sdk 36
```

Pinned versions bypass resolution but still go through the compatibility checks,
so you are told if the combination is known not to work.

## Working offline

```bash
kmp-scaffold new my-app --offline --yes
```

This uses a baseline version set — a snapshot of a combination known to build
together, not an attempt to track current — and says so:

```
! Offline mode: using the baseline version set from August 2026, not the latest releases.
```

The baseline is also the per-key fallback when an individual lookup fails, so a
flaky network degrades one version rather than the whole run. Anything that fell
back is listed.

## Keeping a project current

```bash
cd my-app
kmp-scaffold versions
```

Compares the project's `libs.versions.toml` against the newest releases:

```
Checking Tunesic against the latest releases...

  androidx-lifecycle    2.11.0   →  2.12.0
  koin                  4.2.2    →  4.3.0
  ktor                  3.5.2    →  3.6.0

3 of 41 entries have newer releases. Edit gradle/libs.versions.toml to take them.
```

It never edits anything: bumping a version is a decision, and often a
one-line change you want in its own commit.

`--channel stable` checks against stable releases only, whatever the project was
generated with. `--all` also lists what is already current. Run it outside a
project and it shows what a fresh project would use today.

## Where the catalog lives

Everything above — coordinates, packs, minimum channels, baselines — is one Go
file: `internal/catalog/catalog.go`. Adding a library means adding a row to a
table. See [Extending](extending.md#adding-a-library-pack).
