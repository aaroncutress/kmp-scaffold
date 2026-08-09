package generator

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
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
// derived from the project's deployment target: "18.2" -> "v18".
func (c Ctx) IOSDeploymentMajor() string {
	major, _, _ := strings.Cut(c.Spec.IOSDeployTgt, ".")
	if n, err := strconv.Atoi(major); err == nil && n >= 13 {
		return "v" + strconv.Itoa(n)
	}
	return "v17"
}

// ObservableViewModelSwiftVersion is the Swift-package version matching the
// resolved Gradle artifact.
//
// KMP-ObservableViewModel publishes a Swift package tagged with the same
// version as its Kotlin artifact, so the two sides cannot drift - but the
// Kotlin side also publishes Kotlin-suffixed variants ("1.0.6-kotlin-2.4.20"),
// and only the plain number exists as a Swift tag.
func (c Ctx) ObservableViewModelSwiftVersion() string {
	raw := c.Res.V(catalog.KeyObservableVM)
	if raw == "" {
		return "1.0.6"
	}
	v := resolve.ParseVersion(raw)
	if len(v.Nums) == 0 {
		return raw
	}
	parts := make([]string, 0, 3)
	for i := range 3 {
		if i < len(v.Nums) {
			parts = append(parts, strconv.Itoa(v.Nums[i]))
		} else {
			parts = append(parts, "0")
		}
	}
	return strings.Join(parts, ".")
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
