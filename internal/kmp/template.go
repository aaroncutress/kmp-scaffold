package kmp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
	"github.com/aaroncutress/kmp-scaffold/internal/generator"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
)

func init() { scaffold.Register(ID, func() scaffold.Template { return Template{} }) }

// Template generates a Kotlin Multiplatform project: a Compose Android app, a
// SwiftUI iOS app and the shared module they both use.
type Template struct{}

// Meta describes this template.
func (Template) Meta() scaffold.Meta {
	return scaffold.Meta{
		ID:    ID,
		Label: "Kotlin Multiplatform app",
		Description: "A Compose Android app, a SwiftUI iOS app and a shared KMP module, " +
			"with Navigation 3, Koin and Ktor already wired together.",
		Source:      scaffold.Source{Kind: scaffold.SourceBuiltin},
		AsksPackage: true,
		// The first two are what an existing Gradle project looks like; the
		// third is one of ours.
		Sentinels: []string{"settings.gradle.kts", "build.gradle.kts", model.ManifestFile},
	}
}

// NewAnswers seeds the recommended answers. The catalog fills in the library
// and utility selections so that a pack and the utility depending on it cannot
// drift apart as the catalog changes.
func (t Template) NewAnswers() *scaffold.Answers {
	spec := model.Defaults()
	spec.Packs = catalog.BasicPacks()
	spec.SharedUtils = catalog.DefaultUtilities()
	spec.AndroidExtras = catalog.DefaultExtras()

	a := scaffold.NewAnswers()
	a.SetState(&spec)
	t.Bind(a)
	return a
}

// Bind copies the spec's structural answers into the bag, so a spec assembled
// from flags answers the same questions the wizard would have.
//
// It does not touch Answers.Project: the project's identity flows the other
// way, from the bag into the spec, because that is the half the tool owns.
func (Template) Bind(a *scaffold.Answers) {
	spec := Spec(a)

	var platforms []string
	if spec.Android {
		platforms = append(platforms, "android")
	}
	if spec.IOS {
		platforms = append(platforms, "ios")
	}
	a.Set(QPlatforms, platforms)
	a.Set(QAndroidLayout, spec.AndroidLayout)
	a.Set(QIOSLayout, spec.IOSLayout)
	a.Set(QRootTabs, spec.RootTabs)
	a.Set(QSharedUtils, spec.SharedUtils)
	a.Set(QAndroidExtras, spec.AndroidExtras)
	a.Set(QPacks, spec.Packs)
	a.Set(QChannel, spec.Channel)
	a.Set(QMinSDK, spec.MinSDK)
}

// Normalise pulls in dependencies between packs, utilities and extras, and
// drops anything whose requirements are missing, explaining each change.
func (t Template) Normalise(a *scaffold.Answers) []string {
	spec := Spec(a)
	notes := catalog.Normalise(spec)
	t.Bind(a)
	return notes
}

// Vars is what a generated project records about how it was built.
func (Template) Vars(a *scaffold.Answers) any { return VarsOf(*Spec(a)) }

// Versions describes the resolution this project needs.
func (Template) Versions(a *scaffold.Answers) resolve.Request { return RequestFor(*Spec(a)) }

// RequestFor turns a spec into a resolution request. It is exported because
// `versions` compares an existing project against the latest releases without
// going anywhere near the wizard.
func RequestFor(spec model.Spec) resolve.Request {
	overrides := map[string]string{}
	if spec.KotlinVer != "" {
		overrides[catalog.KeyKotlin] = spec.KotlinVer
	}
	if spec.AGP != "" {
		overrides[catalog.KeyAGP] = spec.AGP
	}

	return resolve.Request{
		Keys:       RequiredKeys(spec),
		Channel:    catalog.ParseChannel(spec.Channel),
		Offline:    spec.Offline,
		Timeout:    20 * time.Second,
		Overrides:  overrides,
		Android:    spec.Android,
		MinSDK:     spec.MinSDK,
		CompileSDK: spec.CompileSDK,
		GradleVer:  spec.GradleVer,
	}
}

// RequiredKeys returns the version keys a project actually needs, so that the
// generated catalog has no unused entries and no unnecessary lookups are made.
func RequiredKeys(spec model.Spec) []string {
	need := map[string]bool{}
	for _, lib := range catalog.Libraries() {
		if lib.Version != "" && lib.When(spec) {
			need[lib.Version] = true
		}
	}
	for _, p := range catalog.Plugins() {
		if p.Version != "" && p.When(spec) {
			need[p.Version] = true
		}
	}
	// Always-present managed keys.
	need[catalog.KeyAGP] = true
	need[catalog.KeyKotlin] = true
	need[catalog.KeyVersionCode] = true
	need[catalog.KeyVersionName] = true
	if spec.Android {
		need[catalog.KeyCompileSDK] = true
		need[catalog.KeyMinSDK] = true
		need[catalog.KeyTargetSDK] = true
		need[catalog.KeyBuildTools] = true
	}
	if spec.IOS && spec.HasPack("skie") {
		need[catalog.KeySkie] = true
	}

	var out []string
	for _, k := range catalog.VersionKeys() {
		if need[k.Key] {
			out = append(out, k.Key)
		}
	}
	return out
}

// Check flags version combinations worth telling the user about. These are
// judgements about this project rather than about the versions themselves,
// which is why they live with the template and not with the resolver.
func (Template) Check(a *scaffold.Answers, res *resolve.Result) {
	if res == nil {
		return
	}
	spec := Spec(a)
	kotlin := resolve.ParseVersion(res.V(catalog.KeyKotlin))

	if spec.IOS && spec.HasPack("skie") && res.Has(catalog.KeySkie) {
		res.Note(resolve.Info,
			"SKIE %s is pinned against Kotlin %s. SKIE usually trails new Kotlin releases by a few days - "+
				"if the iOS framework fails to link, drop Kotlin one patch or remove the SKIE plugin.",
			res.V(catalog.KeySkie), kotlin.Raw)
	}
	if kotlin.Channel() != catalog.Stable {
		res.Note(resolve.Warn,
			"Kotlin %s is a pre-release. Third-party compiler plugins may not support it yet.", kotlin.Raw)
	}
	if spec.Android {
		compose := resolve.ParseVersion(res.V(catalog.KeyComposeM3))
		if compose.Channel() == catalog.Bleeding {
			res.Note(resolve.Info,
				"Material 3 %s is an alpha. The generated navigation shell uses expressive APIs that only exist on that track.",
				compose.Raw)
		}
		nav3 := resolve.ParseVersion(res.V(catalog.KeyNavigation3))
		if nav3.Raw != "" && nav3.Channel() != catalog.Stable {
			res.Note(resolve.Info, "Navigation 3 %s is pre-release - its API still moves between builds.", nav3.Raw)
		}
		if spec.MinSDK > 0 && spec.MinSDK < 24 {
			res.Note(resolve.Warn,
				"minSdk %d is below 24; several AndroidX libraries in this catalog require 24+.", spec.MinSDK)
		}
	}
}

// Headlines is the short list of versions worth showing on screen; the full set
// goes into the generated version catalog.
func (Template) Headlines(a *scaffold.Answers, res *resolve.Result) []scaffold.Headline {
	spec := Spec(a)

	var rows []scaffold.Headline
	add := func(label, key string) {
		if v := res.V(key); v != "" {
			rows = append(rows, scaffold.Headline{Label: label, Value: v, Note: res.Sources[key]})
		}
	}

	rows = append(rows, scaffold.Headline{Label: "Gradle", Value: res.Gradle.Version, Note: "current release"})
	add("Android Gradle Plugin", catalog.KeyAGP)
	add("Kotlin", catalog.KeyKotlin)
	add("KSP", catalog.KeyKSP)
	if spec.Android {
		add("compileSdk / targetSdk", catalog.KeyCompileSDK)
		add("minSdk", catalog.KeyMinSDK)
		add("Compose UI", catalog.KeyComposeCore)
		add("Material 3", catalog.KeyComposeM3)
		add("Navigation 3", catalog.KeyNavigation3)
	}
	add("Koin", catalog.KeyKoin)
	add("Ktor", catalog.KeyKtor)
	if spec.HasPack("database") {
		add("Room", catalog.KeyRoom)
	}
	if spec.IOS && spec.HasPack("skie") {
		add("SKIE", catalog.KeySkie)
	}
	return rows
}

// Summary is the review screen: every decision, in one place.
func (Template) Summary(a *scaffold.Answers, res *resolve.Result) []scaffold.Section {
	spec := Spec(a)

	rows := []scaffold.Row{
		{Label: "Project", Value: spec.Name},
		{Label: "Directory", Value: spec.Dir},
		{Label: "Package", Value: spec.Package},
	}

	targets := []string{"sharedLogic"}
	if spec.Android {
		targets = append(targets, "androidApp")
	}
	if spec.IOS && spec.IOSLayout != "none" {
		targets = append(targets, "iosApp")
	}
	rows = append(rows, scaffold.Row{Label: "Modules", Value: strings.Join(targets, ", ")})

	if spec.Android {
		rows = append(rows, scaffold.Row{Label: "Android layout", Value: spec.AndroidLayout})
		if spec.AndroidLayout == "nav3-shell" {
			rows = append(rows, scaffold.Row{Label: "Root tabs", Value: strings.Join(spec.RootTabs, ", ")})
		}
	}
	if spec.IOS {
		rows = append(rows, scaffold.Row{Label: "iOS layout", Value: spec.IOSLayout})
	}

	sections := []scaffold.Section{
		{Rows: rows},
		{Title: "Shared utilities", Items: labelsFor(spec.SharedUtils, func(id string) (string, bool) {
			def, ok := catalog.UtilityByID(id)
			return def.Label, ok
		})},
	}

	if spec.Android {
		sections = append(sections, scaffold.Section{
			Title: "Android extras",
			Items: labelsFor(spec.AndroidExtras, func(id string) (string, bool) {
				def, ok := catalog.ExtraByID(id)
				return def.Label, ok
			}),
		})
	}

	sections = append(sections, scaffold.Section{
		Title: "Libraries",
		Items: append(
			[]string{"Core (Compose, Navigation 3, Koin, Ktor, coroutines, serialisation)"},
			labelsFor(spec.Packs, func(id string) (string, bool) {
				def, ok := catalog.PackByID(id)
				return def.Label, ok
			})...),
	})

	if res != nil {
		toolchain := scaffold.Section{
			Title: "Toolchain",
			Items: []string{fmt.Sprintf("Gradle %s · AGP %s · Kotlin %s",
				res.Gradle.Version, res.V(catalog.KeyAGP), res.V(catalog.KeyKotlin))},
		}
		if res.HasErrors() {
			toolchain.Note = "The resolver flagged an incompatibility above - " +
				"go back and check before generating."
		}
		sections = append(sections, toolchain)
	}

	return sections
}

func labelsFor(ids []string, lookup func(string) (string, bool)) []string {
	var out []string
	for _, id := range ids {
		if label, ok := lookup(id); ok {
			out = append(out, label)
		}
	}
	return out
}

// Generate writes the project.
func (t Template) Generate(ctx context.Context, req scaffold.GenRequest) (*scaffold.Report, error) {
	spec := Spec(req.Answers)

	report, err := generator.NewProject(ctx, *spec, req.Result, req.Version, req.Writer)
	if err != nil {
		return nil, err
	}

	features, err := featureRecords("feature", report.Features)
	if err != nil {
		return nil, err
	}
	manifest, err := model.NewManifest(req.Version, t.Meta().Ref(), req.Answers.Project,
		t.Vars(req.Answers), features)
	if err != nil {
		return nil, err
	}
	if !req.Writer.DryRun {
		if err := manifest.Save(req.Writer.Root); err != nil {
			return nil, fmt.Errorf("writing the project manifest: %w", err)
		}
	}

	return &scaffold.Report{
		Writer:   req.Writer,
		Warnings: report.Warnings,
		Manifest: manifest,
	}, nil
}

// NextSteps is what to do once the project exists.
func (Template) NextSteps(a *scaffold.Answers) []scaffold.NextStep {
	spec := Spec(a)

	var steps []scaffold.NextStep
	if spec.HasPack("secrets") {
		steps = append(steps, scaffold.NextStep{
			Command: "cp secrets.properties.template secrets.properties",
			Note:    "then fill it in",
		})
	}
	if spec.Android {
		steps = append(steps, scaffold.NextStep{Command: "./gradlew :androidApp:assembleDebug"})
	}
	switch {
	case spec.IOS && spec.IOSLayout == generator.IOSFeaturesLayout:
		steps = append(steps,
			scaffold.NextStep{Command: "./iosApp/build-framework.sh", Note: "on a Mac, before opening Xcode"},
			scaffold.NextStep{Command: "open iosApp/iosApp.xcodeproj"})
	case spec.IOS && spec.IOSLayout != "none":
		steps = append(steps, scaffold.NextStep{Command: "open iosApp/iosApp.xcodeproj", Note: "on a Mac"})
	}
	return steps
}
