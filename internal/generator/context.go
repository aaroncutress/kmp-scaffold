package generator

import (
	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
)

// Ctx is the data every template is rendered against.
type Ctx struct {
	Spec model.Spec
	Res  *resolve.Result
	Cat  CatalogView
	Tabs []Tab
	// Feature is set only while rendering a feature module.
	Feature *FeatureCtx
	// Gen is the generator version, stamped into generated headers.
	Gen string
}

// Tab is one root destination in the navigation shell.
type Tab struct {
	Label  string // "Home"
	Name   string // "home"
	Pascal string // "Home"
	Camel  string // "home"
	Pkg    string // "home"
	Icon   string // Material Symbols icon name, for Android
	Symbol string // SF Symbol name, for iOS
	First  bool
}

// FeatureCtx describes the feature module currently being generated.
type FeatureCtx struct {
	Name         string // kebab-case: "firmware-update"
	Pascal       string // "FirmwareUpdate"
	Camel        string // "firmwareUpdate"
	Pkg          string // "firmwareupdate"
	Presentation string // shell | above-nav | overlay | dialog
	RootTab      bool
	Android      bool
	Shared       bool
	IOS          bool
	// ProjectAccessor is the type-safe Gradle accessor segment for the module,
	// which camel-cases kebab names: feature/firmware-update -> firmwareUpdate.
	ProjectAccessor string
	// Symbol is the SF Symbol used when this feature is an iOS tab.
	Symbol string
}

// PresentationConst maps the presentation choice onto the Kotlin enum entry.
func (f FeatureCtx) PresentationConst() string {
	switch f.Presentation {
	case "shell":
		return "SHELL"
	case "overlay", "dialog":
		return "OVERLAY"
	default:
		return "ABOVE_NAV"
	}
}

// IsSheet reports whether the feature presents as a bottom sheet.
func (f FeatureCtx) IsSheet() bool { return f.Presentation == "overlay" }

// IsDialog reports whether the feature presents as a dialog.
func (f FeatureCtx) IsDialog() bool { return f.Presentation == "dialog" }

// ---------------------------------------------------------------------------
// Catalog view
// ---------------------------------------------------------------------------

// CatalogView is the filtered, ordered catalog data used to emit
// libs.versions.toml.
type CatalogView struct {
	VersionSections []VersionSection
	LibrarySections []LibrarySection
	Bundles         []catalog.Bundle
	Plugins         []catalog.Plugin
}

// VersionSection groups version keys under a comment header.
type VersionSection struct {
	Name string
	Rows []VersionRow
}

// VersionRow is one `key = "value"` line.
type VersionRow struct {
	Key   string
	Value string
}

// LibrarySection groups library aliases under a comment header.
type LibrarySection struct {
	Name string
	Libs []catalog.Library
}

// BuildCatalogView filters the catalog down to what this project needs.
func BuildCatalogView(spec model.Spec, res *resolve.Result) CatalogView {
	var view CatalogView

	// Libraries first, so we know which version keys are actually referenced.
	usedKeys := map[string]bool{}
	libsBySection := map[string][]catalog.Library{}
	var libSectionOrder []string
	for _, lib := range catalog.Libraries() {
		if !lib.When(spec) {
			continue
		}
		if lib.Version != "" {
			usedKeys[lib.Version] = true
		}
		if _, seen := libsBySection[lib.Section]; !seen {
			libSectionOrder = append(libSectionOrder, lib.Section)
		}
		libsBySection[lib.Section] = append(libsBySection[lib.Section], lib)
	}
	for _, name := range libSectionOrder {
		view.LibrarySections = append(view.LibrarySections, LibrarySection{
			Name: name, Libs: libsBySection[name],
		})
	}

	// Plugins.
	for _, p := range catalog.Plugins() {
		if !p.When(spec) {
			continue
		}
		if p.Version != "" {
			usedKeys[p.Version] = true
		}
		view.Plugins = append(view.Plugins, p)
	}

	// Bundles, with members restricted to libraries that made the cut. A
	// bundle whose members all dropped out is omitted entirely.
	included := map[string]bool{}
	for _, sec := range view.LibrarySections {
		for _, lib := range sec.Libs {
			included[lib.Alias] = true
		}
	}
	for _, b := range catalog.Bundles() {
		if !b.When(spec) {
			continue
		}
		var members []string
		for _, alias := range b.Libs {
			if included[alias] {
				members = append(members, alias)
			}
		}
		if len(members) == 0 {
			continue
		}
		view.Bundles = append(view.Bundles, catalog.Bundle{Name: b.Name, Libs: members, When: b.When})
	}

	// Version keys: those referenced above, plus the resolver-managed ones the
	// build scripts read directly (SDK levels, app version).
	always := []string{
		catalog.KeyAGP, catalog.KeyKotlin,
		catalog.KeyVersionCode, catalog.KeyVersionName,
	}
	if spec.Android {
		always = append(always,
			catalog.KeyCompileSDK, catalog.KeyMinSDK,
			catalog.KeyTargetSDK, catalog.KeyBuildTools)
	}
	for _, k := range always {
		usedKeys[k] = true
	}

	bySection := map[string][]VersionRow{}
	for _, def := range catalog.VersionKeys() {
		if !usedKeys[def.Key] {
			continue
		}
		value := res.V(def.Key)
		if value == "" {
			value = def.Baseline
		}
		bySection[def.Section] = append(bySection[def.Section], VersionRow{def.Key, value})
	}
	for _, name := range catalog.SectionOrder() {
		if rows := bySection[name]; len(rows) > 0 {
			view.VersionSections = append(view.VersionSections, VersionSection{name, rows})
		}
	}

	return view
}

// DefaultTabIcons maps common tab names onto Material Symbols icons that the
// composables icon pack provides. Anything unrecognised falls back to a
// generic icon, which the user can change in one place.
var DefaultTabIcons = map[string]string{
	"home":          "Home",
	"settings":      "Settings",
	"profile":       "Person",
	"search":        "Search",
	"library":       "Library_music",
	"collection":    "Layers",
	"store":         "Shopping_cart",
	"shop":          "Shopping_bag",
	"more":          "More_horiz",
	"feed":          "Dynamic_feed",
	"explore":       "Explore",
	"dashboard":     "Dashboard",
	"notifications": "Notifications",
	"messages":      "Chat",
	"favourites":    "Favorite",
	"favorites":     "Favorite",
	"map":           "Map",
	"calendar":      "Calendar_month",
	"account":       "Account_circle",
	"stats":         "Bar_chart",
	"activity":      "Timeline",
}

// DefaultSFSymbols maps common tab names onto SF Symbols, the iOS equivalent of
// DefaultTabIcons. Anything unrecognised falls back to a generic symbol.
var DefaultSFSymbols = map[string]string{
	"home":          "house",
	"settings":      "gearshape",
	"profile":       "person.crop.circle",
	"search":        "magnifyingglass",
	"library":       "books.vertical",
	"collection":    "square.stack",
	"store":         "cart",
	"shop":          "bag",
	"more":          "ellipsis",
	"feed":          "list.bullet.rectangle",
	"explore":       "safari",
	"dashboard":     "square.grid.2x2",
	"notifications": "bell",
	"messages":      "bubble.left.and.bubble.right",
	"favourites":    "heart",
	"favorites":     "heart",
	"map":           "map",
	"calendar":      "calendar",
	"account":       "person.crop.circle",
	"stats":         "chart.bar",
	"activity":      "waveform.path.ecg",
}

// TabIcon returns the Material Symbols name for a tab.
func TabIcon(name string) string {
	if icon, ok := DefaultTabIcons[name]; ok {
		return icon
	}
	return "Widgets"
}

// TabSymbol returns the SF Symbol name for a tab.
func TabSymbol(name string) string {
	if symbol, ok := DefaultSFSymbols[name]; ok {
		return symbol
	}
	return "square.grid.2x2"
}

// BuildTabs turns the user's tab labels into template-ready descriptors.
func BuildTabs(labels []string) []Tab {
	var out []Tab
	for i, label := range labels {
		name := model.Kebab(label)
		out = append(out, Tab{
			Label:  label,
			Name:   name,
			Pascal: model.Pascal(label),
			Camel:  model.Camel(label),
			Pkg:    model.PackageSegment(label),
			Icon:   TabIcon(name),
			Symbol: TabSymbol(name),
			First:  i == 0,
		})
	}
	return out
}

// NewCtx assembles the render context for a whole-project generation.
func NewCtx(spec model.Spec, res *resolve.Result, version string) Ctx {
	return Ctx{
		Spec: spec,
		Res:  res,
		Cat:  BuildCatalogView(spec, res),
		Tabs: BuildTabs(spec.RootTabs),
		Gen:  version,
	}
}
