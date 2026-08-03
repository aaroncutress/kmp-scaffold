package generator

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/render"
	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
	"github.com/aaroncutress/kmp-scaffold/internal/templates"
	"github.com/aaroncutress/kmp-scaffold/internal/wire"
)

// Engine builds the shared template engine.
func Engine() (*render.Engine, error) {
	return render.NewEngine(templates.FS())
}

// Report summarises what a generation run did.
type Report struct {
	Writer   *render.Writer
	Wire     []wire.Result
	Warnings []string
	Manifest model.Manifest
}

// NewProject generates a whole project.
func NewProject(ctx context.Context, spec model.Spec, res *resolve.Result, version string, w *render.Writer) (*Report, error) {
	engine, err := Engine()
	if err != nil {
		return nil, err
	}

	env := &Env{Ctx: NewCtx(spec, res, version), Engine: engine, Writer: w}
	report := &Report{Writer: w}

	// Root and shared modules always exist.
	if err := runGenerator(KindRoot, "root", env); err != nil {
		return nil, err
	}
	if err := runGenerator(KindShared, "shared", env); err != nil {
		return nil, err
	}

	if spec.Android {
		if err := runGenerator(KindAndroid, spec.AndroidLayout, env); err != nil {
			return nil, err
		}
	}
	if spec.IOS && spec.IOSLayout != "none" {
		if err := runGenerator(KindIOS, spec.IOSLayout, env); err != nil {
			return nil, err
		}
	}

	// The Gradle wrapper: scripts and jar are fetched rather than generated, so
	// they are exactly what `gradle wrapper` would have produced.
	if !spec.Offline && !w.DryRun {
		client := resolve.NewClient(30 * time.Second)
		wrapCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		files, err := resolve.FetchWrapper(wrapCtx, client, res.Gradle.Version)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf(
				"could not download the Gradle wrapper (%v) - run `gradle wrapper --gradle-version %s` "+
					"in the project, or just open it in Android Studio, which offers to do it for you",
				err, res.Gradle.Version))
		} else {
			if err := w.WriteBytes("gradlew", files.Gradlew, 0o755); err != nil {
				return nil, err
			}
			if err := w.WriteBytes("gradlew.bat", files.GradlewBat, 0o644); err != nil {
				return nil, err
			}
			if err := w.WriteBytes("gradle/wrapper/gradle-wrapper.jar", files.JAR, 0o644); err != nil {
				return nil, err
			}
		}
	}

	// Record the root tabs as features, so `add feature` sees a complete picture.
	var features []model.Feature
	for _, tab := range env.Ctx.Tabs {
		features = append(features, model.Feature{
			Name:         tab.Name,
			Android:      spec.Android,
			Shared:       false,
			Presentation: "shell",
			RootTab:      spec.AndroidLayout == "nav3-shell",
		})
	}
	manifest := spec.ToManifest(version, features)
	report.Manifest = manifest

	if !w.DryRun {
		if err := manifest.Save(w.Root); err != nil {
			return nil, fmt.Errorf("writing the project manifest: %w", err)
		}
	}

	return report, nil
}

func runGenerator(kind Kind, id string, env *Env) error {
	g, err := Get(kind, id)
	if err != nil {
		return err
	}
	if err := g.Generate(env); err != nil {
		return fmt.Errorf("%s generator: %w", g.ID(), err)
	}
	return nil
}

// FeatureRequest describes a feature to add to an existing project.
type FeatureRequest struct {
	Name         string
	Android      bool
	Shared       bool
	Presentation string
	RootTab      bool
}

// AddFeature generates a feature module in an existing project and wires it in.
func AddFeature(manifest *model.Manifest, root string, req FeatureRequest, version string, w *render.Writer, dryRun bool) (*Report, error) {
	engine, err := Engine()
	if err != nil {
		return nil, err
	}

	// Reconstruct enough of a Spec for the templates from the manifest, so the
	// feature matches how the project was originally generated.
	spec := model.Spec{
		Name:          manifest.Name,
		Package:       manifest.Package,
		ApplicationID: manifest.ApplicationID,
		Android:       manifest.Android,
		IOS:           manifest.IOS,
		AndroidLayout: manifest.AndroidLayout,
		IOSLayout:     manifest.IOSLayout,
		SharedUtils:   manifest.SharedUtils,
		AndroidExtras: manifest.AndroidExtras,
		Packs:         manifest.Packs,
		RootTabs:      manifest.RootTabs,
		JVMTarget:     "11",
	}

	feature := &FeatureCtx{
		Name:            model.Kebab(req.Name),
		Pascal:          model.Pascal(req.Name),
		Camel:           model.Camel(req.Name),
		Pkg:             model.PackageSegment(req.Name),
		Presentation:    req.Presentation,
		RootTab:         req.RootTab,
		Android:         req.Android,
		Shared:          req.Shared,
		ProjectAccessor: model.Camel(req.Name),
	}

	ctx := Ctx{
		Spec:    spec,
		Res:     &resolve.Result{Versions: map[string]string{}},
		Cat:     CatalogView{},
		Tabs:    BuildTabs(manifest.RootTabs),
		Feature: feature,
		Gen:     version,
	}
	env := &Env{Ctx: ctx, Engine: engine, Writer: w}
	report := &Report{Writer: w}

	if req.Android {
		if manifest.AndroidLayout == "" || manifest.AndroidLayout == "none" {
			return nil, fmt.Errorf("this project has no Android app, so an Android feature cannot be added")
		}
		g, err := Get(KindAndroid, manifest.AndroidLayout)
		if err != nil {
			return nil, err
		}
		fg, ok := g.(FeatureGenerator)
		if !ok {
			return nil, fmt.Errorf(
				"the %q Android layout does not support adding feature modules", manifest.AndroidLayout)
		}
		if err := fg.GenerateFeature(env); err != nil {
			return nil, err
		}
	}

	if req.Shared {
		if err := GenerateSharedFeature(env); err != nil {
			return nil, err
		}
	}

	// Wire the new module into the files that already exist.
	applier := wire.NewApplier(root, dryRun)
	for _, edit := range featureEdits(manifest, spec, feature, req) {
		if err := applier.Apply(edit); err != nil {
			return nil, err
		}
	}
	report.Wire = applier.Results()

	for _, missing := range applier.MissingAnchors() {
		report.Warnings = append(report.Warnings, fmt.Sprintf(
			"could not find a kmp-scaffold anchor comment in %s - wire the new module in by hand", missing))
	}

	// Record the feature, replacing any earlier entry with the same name.
	updated := false
	for i := range manifest.Features {
		if manifest.Features[i].Name == feature.Name {
			manifest.Features[i] = model.Feature{
				Name: feature.Name, Android: req.Android, Shared: req.Shared,
				Presentation: req.Presentation, RootTab: req.RootTab,
			}
			updated = true
		}
	}
	if !updated {
		manifest.Features = append(manifest.Features, model.Feature{
			Name: feature.Name, Android: req.Android, Shared: req.Shared,
			Presentation: req.Presentation, RootTab: req.RootTab,
		})
	}
	if req.RootTab && !model.Has(manifest.RootTabs, feature.Pascal) {
		manifest.RootTabs = append(manifest.RootTabs, feature.Pascal)
	}
	report.Manifest = *manifest

	if !dryRun {
		if err := manifest.Save(root); err != nil {
			return nil, fmt.Errorf("updating the project manifest: %w", err)
		}
	}

	return report, nil
}

// featureEdits lists every anchor insertion a new feature needs.
func featureEdits(manifest *model.Manifest, spec model.Spec, f *FeatureCtx, req FeatureRequest) []wire.Edit {
	var edits []wire.Edit
	pkg := manifest.Package

	// A root tab's entries belong to the inner (per-tab) NavDisplay; everything
	// else is pushed above the shell by the outer one.
	entriesAnchor := wire.AnchorEntries
	if req.RootTab {
		entriesAnchor = wire.AnchorRootEntries
	}

	if req.Android {
		edits = append(edits,
			wire.Edit{
				Path:   "settings.gradle.kts",
				Anchor: wire.AnchorFeatures,
				Lines: []string{
					fmt.Sprintf(`include(":feature:%s:api")`, f.Name),
					fmt.Sprintf(`include(":feature:%s:impl")`, f.Name),
				},
			},
			wire.Edit{
				Path:   "androidApp/build.gradle.kts",
				Anchor: wire.AnchorFeatureDeps,
				Lines: []string{
					fmt.Sprintf("implementation(projects.feature.%s.api)", f.ProjectAccessor),
					fmt.Sprintf("implementation(projects.feature.%s.impl)", f.ProjectAccessor),
				},
			},
			wire.Edit{
				Path: path.Join("buildSrc/src/main/kotlin",
					model.LowerAlnum(manifest.Name)+".android.feature.gradle.kts"),
				Anchor: wire.AnchorFeatureAPIs,
				Lines: []string{
					fmt.Sprintf(`add("implementation", project(":feature:%s:api"))`, f.Name),
				},
			},
			wire.Edit{
				Path:    path.Join("androidApp/src/main/kotlin", spec.PackagePath(), "AppSerializers.kt"),
				Anchor:  wire.AnchorSerializers,
				Lines:   []string{fmt.Sprintf("%sNavSerializers,", f.Camel)},
				Imports: []string{fmt.Sprintf("import %s.feature.%s.api.%sNavSerializers", pkg, f.Pkg, f.Camel)},
			},
			wire.Edit{
				Path:    path.Join("androidApp/src/main/kotlin", spec.PackagePath(), "App.kt"),
				Anchor:  entriesAnchor,
				Lines:   []string{fmt.Sprintf("%sEntries()", f.Camel)},
				Imports: []string{fmt.Sprintf("import %s.feature.%s.impl.%sEntries", pkg, f.Pkg, f.Camel)},
			},
		)
	}

	// A root tab additionally joins the shell: the roots set the back stacks are
	// keyed by, the navigation item list, and core/ui's dependencies (which is
	// where that list lives).
	if req.Android && req.RootTab && manifest.AndroidLayout == "nav3-shell" {
		icon := DefaultTabIcons[f.Name]
		if icon == "" {
			icon = "Widgets"
		}
		edits = append(edits,
			wire.Edit{
				Path:    path.Join("androidApp/src/main/kotlin", spec.PackagePath(), "App.kt"),
				Anchor:  wire.AnchorRoots,
				Lines:   []string{fmt.Sprintf("%sRoute,", f.Pascal)},
				Imports: []string{fmt.Sprintf("import %s.feature.%s.api.%sRoute", pkg, f.Pkg, f.Pascal)},
			},
			wire.Edit{
				Path:   "core/ui/build.gradle.kts",
				Anchor: wire.AnchorFeatureAPIs,
				Lines:  []string{fmt.Sprintf("implementation(projects.feature.%s.api)", f.ProjectAccessor)},
			},
			wire.Edit{
				Path: path.Join("core/ui/src/main/kotlin", spec.PackagePath(),
					"core/ui/components/Navigation.kt"),
				Anchor: wire.AnchorNavItems,
				Lines: []string{fmt.Sprintf(
					`AppNavigationItem(%sRoute, "%s", MaterialSymbols.Rounded.%s),`,
					f.Pascal, model.Pascal(req.Name), icon)},
				Imports: []string{
					fmt.Sprintf("import %s.feature.%s.api.%sRoute", pkg, f.Pkg, f.Pascal),
					fmt.Sprintf("import com.composables.icons.materialsymbols.rounded.%s", icon),
				},
			},
		)
	}

	if req.Shared {
		edits = append(edits, wire.Edit{
			Path:    path.Join("sharedLogic/src/commonMain/kotlin", spec.PackagePath(), "di/SharedModules.kt"),
			Anchor:  wire.AnchorSharedModules,
			Lines:   []string{fmt.Sprintf("%sModule,", f.Camel)},
			Imports: []string{fmt.Sprintf("import %s.feature.%s.di.%sModule", pkg, f.Pkg, f.Camel)},
		})
	}

	return edits
}
