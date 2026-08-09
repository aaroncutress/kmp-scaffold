# kmp-mobile

The default template: a Kotlin Multiplatform project with an Android app, an
iOS app, and shared Kotlin logic underneath both. It is what
`kmp-scaffold new my-app` generates when you do not ask for anything else.

Everything in this folder describes **this template**. The tool's own
documentation — the wizard, other templates, writing your own — is
[one level up](../README.md).

## What it generates

```
my-app/
├── settings.gradle.kts          every module, plus the anchor `add feature` appends to
├── gradle/libs.versions.toml    one source of truth for versions
├── buildSrc/                    convention plugins, so module scripts stay short
├── sharedLogic/                 Kotlin Multiplatform: models, repositories, ViewModels
├── core/navigation/             routes, the back stack, scene strategies
├── core/ui/                     theme, shared composables
├── feature/<name>/              one Gradle module per feature, api/impl split
├── androidApp/                  the Android application module
├── iosApp/                      the Xcode project and its local Swift package
└── .run/                        JetBrains run configurations, ready in the IDE
```

Both apps are thin: they wire navigation and host feature modules. The logic
they share lives once, in `sharedLogic`, and is consumed as an ordinary Gradle
dependency on Android and as an XCFramework on iOS.

The shape is the point. Features never depend on each other — they depend on a
navigation core that knows only about routes — so adding one is additive and the
build stays parallel and cacheable. Android's `api`/`impl` module split and
iOS's one-target-per-feature Swift package are the same rule expressed twice.

## Where to read next

| Page | What it covers |
| --- | --- |
| [Project layout](project-layout.md) | every module, what belongs in it, and the dependency rule |
| [iOS architecture](ios-architecture.md) | the local Swift package, multi-stack navigation, the XCFramework |
| [Adding features](features.md) | what `add feature` writes and wires, on each side |
| [Libraries and versions](libraries.md) | library packs, version resolution, keeping a project current |

## What is optional

Most of it. The wizard asks, and `new` takes the same answers as flags:

- **Either platform alone.** `--no-ios` or `--no-android` drops a whole side.
- **The iOS layout.** `--ios-layout swiftui-simple` generates one
  `ContentView.swift` instead of the modular Swift package.
- **Library packs.** A pack is a named group of artifacts — images, database,
  secrets, maps, media, and a dozen more. Nothing unselected reaches
  `libs.versions.toml`.
- **Shared utilities.** Small pieces of `sharedLogic` — a Koin graph, a result
  type, an analytics facade — each switchable.

Versions are never hard-coded: each one is resolved against its repository when
you generate, with a known-good baseline used offline. See
[libraries and versions](libraries.md#how-resolution-works).

## Building it

```bash
cd my-app
cp secrets.properties.template secrets.properties   # if the secrets pack is on
./gradlew :androidApp:assembleDebug
./iosApp/build-framework.sh                         # on a Mac, before opening Xcode
open iosApp/iosApp.xcodeproj
```

The XCFramework has to exist before Xcode's first build, because Swift Package
Manager resolves it as a binary target before any build phase runs. After that,
the app's "Build Kotlin Framework" phase keeps it current. If a build fails, see
[troubleshooting](../troubleshooting.md#the-kmp-mobile-project).
