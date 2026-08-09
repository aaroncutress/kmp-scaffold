# iOS architecture

**Template: `kmp-mobile`.** The iOS side of what this template generates.

Its default iOS layout (`swiftui-features`) is the counterpart to the Android
`api`/`impl` split: one local Swift package, one target per feature, and a
shared `CoreNavigation` target that every feature depends on and none of them
depends on each other through.

If you want something smaller, `--ios-layout swiftui-simple` generates a single
`ContentView.swift` instead. Everything below is about the modular layout.

## The shape

```
iosApp/
├── iosApp.xcodeproj
├── README.md                       how to build it, and in what order
├── build-framework.sh              Gradle → Frameworks/SharedLogic.xcframework
├── Frameworks/                     generated, gitignored
├── Configuration/
│   └── Config.xcconfig             bundle id, product name, TEAM_ID
├── iosApp/
│   ├── Info.plist
│   ├── Assets.xcassets/
│   └── App/
│       ├── iOSApp.swift            @main, starts Koin
│       └── AppCoordinator.swift    TabView + one NavigationPath per tab
└── Packages/Features/
    ├── Package.swift               every target and product
    └── Sources/
        ├── CoreNavigation/         route payloads + the Koin bridge
        │   ├── HomeRoute.swift
        │   ├── SettingsRoute.swift
        │   └── Resolve.swift
        ├── Home/
        │   ├── HomeScreen.swift
        │   └── HomeDestination.swift
        └── Settings/
            ├── SettingsScreen.swift
            └── SettingsDestination.swift
```

## The dependency rule

```
                    ┌────────────────────────────┐
                    │   iosApp (app target)      │
                    │   AppCoordinator: TabView  │
                    │   + one path per tab       │
                    └─────────────┬──────────────┘
                                  │
          ┌───────────────────────┼───────────────────────┐
          ▼                       ▼                       ▼
    ┌───────────┐           ┌───────────┐           ┌───────────┐
    │   Home    │           │ Settings  │           │  Billing  │
    │ (tab root)│           │ (tab root)│           │  (pushed) │
    └─────┬─────┘           └─────┬─────┘           └─────┬─────┘
          │                       │                       │
          └───────────────────────┼───────────────────────┘
                                  │  (routes only)
                                  ▼
                     ┌────────────────────────┐
                     │     CoreNavigation     │
                     │  HomeRoute             │
                     │  SettingsRoute         │
                     │  BillingRoute          │
                     └────────────────────────┘
```

A feature never imports another feature. `Settings` can send you to `Billing`
without knowing `Billing` exists, because the route payload lives in
`CoreNavigation` and the coordinator is what turns it into a screen.

This is the same rule as the Android side, where `feature/settings/impl` depends
on `feature/billing/api` rather than `feature/billing/impl`.

### Why it matters

- **No circular imports.** Two features that can each open the other would
  deadlock a build graph. Routing through a third module they both depend on
  makes that impossible by construction.
- **Granular rebuilds.** Editing one screen recompiles one target, not the app.
- **Each feature is previewable and testable alone.** Every target is also a
  product, so a test bundle or another package can import just the one.

## Navigating between features

Take a callback rather than an import. The generated `*Destination.swift` shows
the pattern in its doc comment:

```swift
// Sources/Contacts/ContactsScreen.swift
struct ContactsScreen: View {
    let onOpenChat: (ChatRoute) -> Void      // ChatRoute is from CoreNavigation

    var body: some View {
        List(contacts, id: \.self) { contact in
            Button(contact) { onOpenChat(ChatRoute(roomId: contact.id)) }
        }
    }
}
```

```swift
// Sources/Contacts/ContactsDestination.swift
public extension View {
    @ViewBuilder
    func contactsDestination(
        for route: ContactsRoute,
        onOpenChat: @escaping (ChatRoute) -> Void
    ) -> some View {
        ContactsScreen(onOpenChat: onOpenChat)
    }
}
```

Then in `AppCoordinator.swift`, append to the active tab's path:

```swift
.contactsDestination(for: ContactsRoute()) { chatRoute in
    contactsPath.append(chatRoute)
}
```

`Chat` opens *inside the Contacts tab*, so backing out lands where the user
started rather than in some other tab.

## Multi-stack navigation

`AppCoordinator` holds one `NavigationPath` per tab:

```swift
@State private var homePath = NavigationPath()
@State private var settingsPath = NavigationPath()
```

That is what preserves each tab's history: go three screens deep in Home, switch
to Settings, come back — Home is still three screens deep. A single shared path
would flatten all of it into one history and lose your place.

Destinations that are not tab roots are registered once, on a modifier every
stack applies:

```swift
private struct AppDestinations: ViewModifier {
    @Binding var path: NavigationPath

    func body(content: Content) -> some View {
        content
            .navigationDestination(for: BillingRoute.self) { route in
                EmptyView().billingDestination(for: route)
            }
            // kmp-scaffold:ios-destinations
    }
}
```

Registering on every stack rather than one is deliberate: a route pushed from
Settings has to resolve on the Settings stack.

## Using shared ViewModels

A feature paired with shared logic (`kmp-scaffold add feature x`, keeping the
`shared` target) gets a screen driven by the same Kotlin ViewModel the Android
screen collects from:

```swift
struct BillingScreen: View {
    @StateViewModel private var viewModel: BillingViewModel

    init() {
        _viewModel = StateViewModel(wrappedValue: resolveShared(BillingViewModel.self))
    }

    var body: some View {
        Text(viewModel.uiState.message)
    }
}
```

Two things make that work:

- **SKIE + KMP-ObservableViewModel** turn the Kotlin `StateFlow<UiState>` into
  something SwiftUI observes directly. `@StateViewModel` re-renders the view when
  the shared state changes, with no bridging code of your own.
- **`resolveShared`** (in `CoreNavigation/Resolve.swift`) asks the shared Koin
  graph for the instance. It has to: the generated ViewModels take an `internal`
  Kotlin constructor, so they are not exported to Swift and cannot be built on
  this side. Getting them from Koin also means their dependencies can be swapped
  in Kotlin without touching any Swift.

## The XCFramework

The Features package declares the shared module as a binary target:

```swift
.binaryTarget(name: "SharedLogic", path: "../../Frameworks/SharedLogic.xcframework")
```

This is the one place the modular layout costs you something. The usual KMP
setup (`embedAndSignAppleFrameworkForXcode`) produces a framework inside the app
target's build directory, where Swift package targets cannot see it — so a
feature target could not `import SharedLogic` at all. An XCFramework at a fixed
path can be consumed by SPM, which is what makes the whole layout possible.

To keep it current, `sharedLogic/build.gradle.kts` registers an `XCFramework`,
and `iosApp/build-framework.sh` assembles and copies it:

```bash
./iosApp/build-framework.sh
```

**Run that once before the first Xcode build.** SPM resolves the binary target's
path before Xcode runs any build phase, so the framework has to be on disk
already. After that the app's "Build Kotlin Framework" phase re-runs the script
on every build.

If you change the shared module's *API* while Xcode is open and Swift cannot see
the new symbols, run the script and then **File → Packages → Reset Package
Caches**.

`Frameworks/` is gitignored — it is a build product.

## How it maps onto Android

| Android | iOS |
| --- | --- |
| `feature/x/api` — routes | `CoreNavigation/XRoute.swift` |
| `feature/x/impl` — screens | `Sources/X/` target |
| `Route.presentation` | which anchor the destination is registered at |
| `RootRoute` | a `case` in `AppTab` with its own `NavigationPath` |
| `NavigationState`'s per-root back stacks | one `NavigationPath` per tab |
| `entryProvider { xEntries() }` | `.navigationDestination(for: XRoute.self)` |
| `sharedLogic` ViewModel | the same class, via `resolveShared` |

`kmp-scaffold add feature <name>` writes both sides at once, so they cannot
drift apart.

## Adding a feature

```bash
kmp-scaffold add feature billing
```

By default that covers Android, iOS and the shared logic. Narrow it with
`--targets`:

```bash
kmp-scaffold add feature billing --targets ios,shared
```

On the iOS side it creates the route, the screen and the destination wrapper,
then wires them into `Package.swift` (product, target, and the `AppFeatures`
umbrella) and `AppCoordinator.swift` (import, plus either a tab or a
destination registration).

## Why the app links one product

`Package.swift` declares a product per target *and* an umbrella `AppFeatures`
product covering all of them. The Xcode project links only `AppFeatures` and
`SharedLogic`.

That is on purpose: Xcode rewrites `project.pbxproj` whenever it saves, so any
anchor comment placed there would be silently discarded, and `add feature` could
not maintain a per-feature link list. Linking one umbrella product means adding
a feature never touches the Xcode project at all. Targets stay separate, so the
build cache is just as granular either way.

The generated `project.pbxproj` already contains the wiring Xcode would have
written had you added the package through its own UI — an
`XCLocalSwiftPackageReference` for `Packages/Features`, an
`XCSwiftPackageProductDependency` per linked product, and the build files that
carry them into the frameworks phase. There is nothing to do in Xcode before the
project builds.
