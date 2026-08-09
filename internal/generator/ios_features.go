package generator

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/wire"
)

func init() { Register(iosFeaturesGenerator{}) }

// IOSFeaturesLayout is the id of the modular iOS layout. The shared module's
// build script needs to know about it (it has to produce an XCFramework rather
// than only an embedded framework), which is the one place the layout leaks
// outside this file.
const IOSFeaturesLayout = "swiftui-features"

// iosFeaturesGenerator writes the "single Features package" iOS architecture: a
// local Swift package with one target per feature, a shared CoreNavigation
// target holding route payloads, and an app target that orchestrates them in a
// multi-stack TabView.
//
// Features depend only on CoreNavigation, never on each other, so a screen can
// navigate to another feature's route without importing that feature - which is
// what keeps the dependency graph acyclic and the build cache granular. It is
// the same api/impl split the Android side uses, expressed in SPM terms.
type iosFeaturesGenerator struct{}

func (iosFeaturesGenerator) ID() string    { return IOSFeaturesLayout }
func (iosFeaturesGenerator) Label() string { return "SwiftUI, one target per feature" }
func (iosFeaturesGenerator) Description() string {
	return "A local Swift package with a target per feature and a shared CoreNavigation module, in a multi-stack TabView"
}
func (iosFeaturesGenerator) Kind() Kind { return KindIOS }

// iosPaths keeps the layout's directory decisions in one place.
const (
	iosRoot        = "iosApp"
	iosPackageRoot = "iosApp/Packages/Features"
	iosSources     = "iosApp/Packages/Features/Sources"
	iosAppSources  = "iosApp/iosApp"
)

func (g iosFeaturesGenerator) Generate(env *Env) error {
	files := []struct{ tpl, path string }{
		{"iosfeatures/project.pbxproj", path.Join(iosRoot, "iosApp.xcodeproj/project.pbxproj")},
		{"ios/contents.xcworkspacedata", path.Join(iosRoot, "iosApp.xcodeproj/project.xcworkspace/contents.xcworkspacedata")},
		{"ios/Config.xcconfig", path.Join(iosRoot, "Configuration/Config.xcconfig")},
		{"iosfeatures/build-framework.sh", path.Join(iosRoot, "build-framework.sh")},
		{"iosfeatures/README.md", path.Join(iosRoot, "README.md")},
		{"iosfeatures/Package.swift", path.Join(iosPackageRoot, "Package.swift")},
		{"iosfeatures/Resolve.swift", path.Join(iosSources, "CoreNavigation/Resolve.swift")},
		{"iosfeatures/iOSApp.swift", path.Join(iosAppSources, "App/iOSApp.swift")},
		{"iosfeatures/AppCoordinator.swift", path.Join(iosAppSources, "App/AppCoordinator.swift")},
		{"ios/Info.plist", path.Join(iosAppSources, "Info.plist")},
		{"ios/Assets.Contents.json", path.Join(iosAppSources, "Assets.xcassets/Contents.json")},
		{"ios/AppIcon.Contents.json", path.Join(iosAppSources, "Assets.xcassets/AppIcon.appiconset/Contents.json")},
		{"ios/AccentColor.Contents.json", path.Join(iosAppSources, "Assets.xcassets/AccentColor.colorset/Contents.json")},
		{"ios/PreviewAssets.Contents.json", path.Join(iosAppSources, "Preview Content/Preview Assets.xcassets/Contents.json")},
	}
	if env.Ctx.Spec.Tests {
		// SwiftPM finds a test target's sources under Tests/<target name>.
		files = append(files, struct{ tpl, path string }{
			"iosfeatures/RouteTests.swift",
			path.Join(iosPackageRoot, "Tests/FeatureTests/RouteTests.swift"),
		})
	}
	for _, f := range files {
		if err := env.Render(f.tpl, f.path); err != nil {
			return err
		}
	}

	// One target per root tab, matching the Android feature modules.
	for _, tab := range env.Ctx.Tabs {
		sub := *env
		sub.Ctx.Feature = &FeatureCtx{
			Name:         tab.Name,
			Pascal:       tab.Pascal,
			Camel:        tab.Camel,
			Pkg:          tab.Pkg,
			Presentation: "shell",
			RootTab:      true,
			IOS:          true,
			Symbol:       tab.Symbol,
			// Root tabs generated with the project have no shared ViewModel yet;
			// `add feature` is what pairs a target with one.
			Shared: false,
		}
		if err := g.GenerateFeature(&sub); err != nil {
			return fmt.Errorf("generating the %s iOS target: %w", tab.Label, err)
		}
	}
	return nil
}

// GenerateFeature writes one feature target plus its route in CoreNavigation.
func (iosFeaturesGenerator) GenerateFeature(env *Env) error {
	f := env.Ctx.Feature
	if f == nil {
		return fmt.Errorf("GenerateFeature called without a feature context")
	}

	files := []struct{ tpl, path string }{
		{"iosfeatures/Route.swift", path.Join(iosSources, "CoreNavigation", f.Pascal+"Route.swift")},
		{"iosfeatures/Screen.swift", path.Join(iosSources, f.Pascal, f.Pascal+"Screen.swift")},
		{"iosfeatures/Destination.swift", path.Join(iosSources, f.Pascal, f.Pascal+"Destination.swift")},
	}
	for _, file := range files {
		if err := env.Render(file.tpl, file.path); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Template data
// ---------------------------------------------------------------------------

// IOSDeploymentMajor is the major iOS version for Package.swift's `platforms`,
// derived from the project's deployment target: "18.0" -> "v18".
//
// SwiftPM's platform cases are gated on the manifest's swift-tools-version -
// .v18 needs 6.0 and .v26 needs 6.2 - so the version declared at the top of the
// generated Package.swift is the floor for anything this returns.
func (c Ctx) IOSDeploymentMajor() string {
	major, _, _ := strings.Cut(c.Spec.IOSDeployTgt, ".")
	if n, err := strconv.Atoi(major); err == nil && n >= 13 {
		return "v" + strconv.Itoa(n)
	}
	// No deployment target to read, which means a spec that was rebuilt from a
	// manifest written before it was recorded. What that project was generated
	// with is the default, not some third value.
	fallback, _, _ := strings.Cut(model.Defaults().IOSDeployTgt, ".")
	return "v" + fallback
}

// ObservableViewModelSwiftVersion is the version Package.swift pins.
//
// KMP-ObservableViewModel's Swift and Kotlin sides are published together and
// have to match: the Swift package reads the Kotlin runtime's internals, so a
// mismatched pair compiles and then misbehaves. The resolver already pairs the
// two keys, resolving the Swift one from the repository's git tags and taking
// the Kotlin version whenever it exists as a tag - so by the time this is
// asked, the answer is either that shared version or the closest the Swift
// side has, with a note explaining the difference.
func (c Ctx) ObservableViewModelSwiftVersion() string {
	for _, key := range []string{catalog.KeyObservableVMSwift, catalog.KeyObservableVM} {
		if raw := c.Res.V(key); raw != "" {
			return raw
		}
	}
	// Nothing resolved - offline, say - so the catalog's own floor stands in,
	// which is the same version libs.versions.toml will have got.
	if key, ok := catalog.VersionKeyByName(catalog.KeyObservableVMSwift); ok {
		return key.Baseline
	}
	return "1.0.6"
}

// IOSCoordinatorImports is the app coordinator's import list, sorted, so that
// the imports `add feature` inserts later slot into the same ordering.
func (c Ctx) IOSCoordinatorImports() []string {
	modules := []string{"CoreNavigation", "SwiftUI"}
	for _, tab := range c.Tabs {
		modules = append(modules, tab.Pascal)
	}
	sort.Strings(modules)
	return modules
}

// UsesIOSFeatures reports whether the project uses the modular iOS layout.
func (c Ctx) UsesIOSFeatures() bool { return c.Spec.IOSLayout == IOSFeaturesLayout }

// iosFeatureEdits lists the anchor insertions a new iOS feature target needs.
//
// Nothing here touches the Xcode project: the app target links one umbrella
// product ("AppFeatures"), so a new target only ever changes Package.swift and
// the coordinator. That matters because Xcode rewrites project.pbxproj whenever
// it saves, which would discard any anchor comments living there.
func iosFeatureEdits(f *FeatureCtx, req FeatureRequest) []wire.Edit {
	edits := []wire.Edit{
		{
			Path:   path.Join(iosPackageRoot, "Package.swift"),
			Anchor: wire.AnchorIOSProducts,
			Lines: []string{
				fmt.Sprintf(`.library(name: %q, targets: [%q]),`, f.Pascal, f.Pascal),
			},
		},
		{
			Path:   path.Join(iosPackageRoot, "Package.swift"),
			Anchor: wire.AnchorIOSAppFeatures,
			Lines:  []string{fmt.Sprintf("%q,", f.Pascal)},
		},
		{
			Path:   path.Join(iosPackageRoot, "Package.swift"),
			Anchor: wire.AnchorIOSTargets,
			Lines: []string{
				fmt.Sprintf(`.target(name: %q, dependencies: featureDependencies),`, f.Pascal),
			},
		},
	}

	coordinator := path.Join(iosAppSources, "App/AppCoordinator.swift")

	if req.RootTab {
		symbol := TabSymbol(f.Name)
		edits = append(edits,
			wire.Edit{
				Path:    coordinator,
				Anchor:  wire.AnchorIOSTabCases,
				Lines:   []string{"case " + f.Camel},
				Imports: []string{"import " + f.Pascal},
			},
			wire.Edit{
				Path:   coordinator,
				Anchor: wire.AnchorIOSTabPaths,
				Lines:  []string{fmt.Sprintf("@State private var %sPath = NavigationPath()", f.Camel)},
			},
			wire.Edit{
				Path:   coordinator,
				Anchor: wire.AnchorIOSTabs,
				Lines: []string{
					fmt.Sprintf("NavigationStack(path: $%sPath) {", f.Camel),
					"    EmptyView()",
					fmt.Sprintf("        .%sDestination(for: %sRoute())", f.Camel, f.Pascal),
					fmt.Sprintf("        .modifier(AppDestinations(path: $%sPath))", f.Camel),
					"}",
					fmt.Sprintf(".tabItem { Label(%q, systemImage: %q) }", model.Pascal(req.Name), symbol),
					fmt.Sprintf(".tag(AppTab.%s)", f.Camel),
					"",
				},
			},
		)
		return edits
	}

	// A pushed screen is registered once, on the modifier every tab applies, so
	// it opens inside whichever tab the user is already in.
	edits = append(edits, wire.Edit{
		Path:   coordinator,
		Anchor: wire.AnchorIOSDestinations,
		Lines: []string{
			fmt.Sprintf(".navigationDestination(for: %sRoute.self) { route in", f.Pascal),
			fmt.Sprintf("    EmptyView().%sDestination(for: route)", f.Camel),
			"}",
		},
		Imports: []string{"import " + f.Pascal},
	})
	return edits
}
