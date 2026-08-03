package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
	"github.com/aaroncutress/kmp-scaffold/internal/generator"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/render"
	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
	"github.com/aaroncutress/kmp-scaffold/internal/tui"
	"github.com/aaroncutress/kmp-scaffold/internal/wire"
)

func runAdd(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return ErrUsage
	}
	switch args[0] {
	case "feature":
		return runAddFeature(ctx, args[1:])
	case "library", "lib", "dependency":
		return runAddLibrary(ctx, args[1:])
	default:
		return fmt.Errorf("unknown thing to add: %q (try `feature` or `library`)", args[0])
	}
}

func runAddFeature(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("add feature", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `Usage: kmp-scaffold add feature [name] [flags]

Creates a feature module and wires it into the project: settings.gradle.kts,
the app's dependencies, the route serializers, the entry provider and - for a
shared feature - the Koin graph.

Flags:
`)
		fs.PrintDefaults()
	}

	var (
		androidOnly = fs.Bool("android-only", false, "Only generate the Android module")
		sharedOnly  = fs.Bool("shared-only", false, "Only generate the shared logic")
		present     = fs.String("presentation", "", "above-nav, overlay, dialog or shell (a root tab)")
		yes         = fs.Bool("yes", false, "Skip the wizard")
		dryRun      = fs.Bool("dry-run", false, "Report what would change without changing it")
		force       = fs.Bool("force", false, "Overwrite files that already exist")
		dir         = fs.String("dir", ".", "Project directory")
		verbose     = fs.Bool("verbose", false, "List every file written")
	)
	flags, operands := permute(fs, args)
	if err := fs.Parse(flags); err != nil {
		return ErrUsage
	}

	manifest, root, err := model.LoadManifest(*dir)
	if err != nil {
		return err
	}

	draft := tui.FeatureDraft{
		Android:      manifest.Android && !*sharedOnly,
		Shared:       !*androidOnly,
		Presentation: *present,
	}
	if draft.Presentation == "" {
		draft.Presentation = "above-nav"
	}
	draft.RootTab = draft.Presentation == "shell"
	if positional := firstOperand(operands); positional != "" {
		draft.Name = model.Kebab(positional)
	}

	spec := model.Spec{
		Name:          manifest.Name,
		Package:       manifest.Package,
		Android:       manifest.Android,
		IOS:           manifest.IOS,
		AndroidLayout: manifest.AndroidLayout,
		IOSLayout:     manifest.IOSLayout,
		SharedUtils:   manifest.SharedUtils,
		AndroidExtras: manifest.AndroidExtras,
		Packs:         manifest.Packs,
		RootTabs:      manifest.RootTabs,
	}

	if *yes || !interactive() {
		if draft.Name == "" {
			return fmt.Errorf("give the feature a name, e.g. `kmp-scaffold add feature billing`")
		}
		if err := model.ValidateFeatureName(draft.Name); err != nil {
			return fmt.Errorf("feature name: %w", err)
		}
		if manifest.FindFeature(draft.Name) != nil {
			return fmt.Errorf("this project already has a feature called %q", draft.Name)
		}
	} else {
		wizard, collected := tui.FeatureFlow(spec, *manifest, draft)
		if _, err := wizard.Run(ctx); err != nil {
			if errors.Is(err, tui.ErrCancelled) {
				return errCancelled
			}
			return err
		}
		draft = *collected
	}

	if !draft.Android && !draft.Shared {
		return fmt.Errorf("nothing to generate - pick the Android module, the shared logic, or both")
	}

	writer := render.NewWriter(root, *dryRun, *force)
	report, err := generator.AddFeature(manifest, root, generator.FeatureRequest{
		Name:         draft.Name,
		Android:      draft.Android,
		Shared:       draft.Shared,
		Presentation: draft.Presentation,
		RootTab:      draft.RootTab,
	}, Version, writer, *dryRun)
	if err != nil {
		return err
	}

	fmt.Println()
	if *dryRun {
		fmt.Println(sBold.Render("Dry run - nothing was written."))
	} else {
		fmt.Println(sOK.Render("✔ ") + sBold.Render(draft.Name) + sMuted.Render(" added"))
	}
	fmt.Println()
	printWriteSummary(writer, *verbose)
	printWireSummary(report.Wire)

	for _, w := range report.Warnings {
		fmt.Println("  " + sWarn.Render("! "+w))
	}

	if !*dryRun {
		fmt.Println()
		fmt.Println(sMuted.Render("Sync Gradle to pick up the new modules."))
	}
	return nil
}

func printWireSummary(results []wire.Result) {
	touched := 0
	for _, r := range results {
		if r.Inserted > 0 {
			fmt.Printf("  %s %s %s\n", sWarn.Render("~"), r.Path,
				sMuted.Render(fmt.Sprintf("(%d line(s) wired in)", r.Inserted)))
			touched++
		}
	}
	if touched == 0 && len(results) > 0 {
		fmt.Println(sMuted.Render("  every wiring point was already in place"))
	}
}

func runAddLibrary(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("add library", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `Usage: kmp-scaffold add library <pack>... [flags]

Resolves the newest version of a library pack and adds its entries to
gradle/libs.versions.toml, then prints the dependency lines to paste into the
modules that need them.

Packs:
`)
		for _, p := range catalog.Packs() {
			fmt.Fprintf(os.Stderr, "  %-14s %s\n", p.ID, p.Description)
		}
		fmt.Fprintln(os.Stderr, "\nFlags:")
		fs.PrintDefaults()
	}

	var (
		dir     = fs.String("dir", ".", "Project directory")
		channel = fs.String("channel", "preview", "Version channel: stable, preview or bleeding")
		dryRun  = fs.Bool("dry-run", false, "Print the catalog entries without writing them")
	)
	flags, ids := permute(fs, args)
	if err := fs.Parse(flags); err != nil {
		return ErrUsage
	}
	if len(ids) == 0 {
		return ErrUsage
	}

	manifest, root, err := model.LoadManifest(*dir)
	if err != nil {
		return err
	}

	// Build a spec with only the new packs added, so the diff is exactly what
	// these packs contribute.
	spec := specFromManifest(manifest)
	before := map[string]bool{}
	for _, lib := range catalog.Libraries() {
		if lib.When(spec) {
			before[lib.Alias] = true
		}
	}

	for _, id := range ids {
		if _, ok := catalog.PackByID(id); !ok {
			return fmt.Errorf("unknown library pack %q - run `kmp-scaffold add library --help` for the list", id)
		}
		if !model.Has(spec.Packs, id) {
			spec.Packs = append(spec.Packs, id)
		}
	}
	catalog.Normalise(&spec)

	fmt.Println(sBold.Render("Resolving versions..."))
	result := resolve.Run(ctx, spec, resolve.Options{
		Channel: catalog.ParseChannel(*channel),
		Offline: false,
	})

	var newLibs []catalog.Library
	usedKeys := map[string]bool{}
	for _, lib := range catalog.Libraries() {
		if lib.When(spec) && !before[lib.Alias] {
			newLibs = append(newLibs, lib)
			if lib.Version != "" {
				usedKeys[lib.Version] = true
			}
		}
	}
	if len(newLibs) == 0 {
		fmt.Println(sMuted.Render("Nothing to add - those packs are already in the catalog."))
		return nil
	}

	var versionLines, libraryLines []string
	for _, def := range catalog.VersionKeys() {
		if usedKeys[def.Key] {
			versionLines = append(versionLines, fmt.Sprintf("%s = %q", def.Key, result.V(def.Key)))
		}
	}
	for _, lib := range newLibs {
		if lib.Version != "" {
			libraryLines = append(libraryLines,
				fmt.Sprintf("%s = { module = %q, version.ref = %q }", lib.Alias, lib.Module, lib.Version))
		} else {
			libraryLines = append(libraryLines,
				fmt.Sprintf("%s = { module = %q }", lib.Alias, lib.Module))
		}
	}

	fmt.Println()
	fmt.Println(sBold.Render("gradle/libs.versions.toml"))
	for _, l := range versionLines {
		fmt.Println("  " + sOK.Render("+ ") + l)
	}
	for _, l := range libraryLines {
		fmt.Println("  " + sOK.Render("+ ") + l)
	}

	if !*dryRun {
		if err := appendToCatalog(root, versionLines, libraryLines); err != nil {
			return err
		}
		if err := manifest.Save(root); err != nil {
			return err
		}
		manifest.Packs = spec.Packs
		if err := manifest.Save(root); err != nil {
			return err
		}
	}

	fmt.Println()
	fmt.Println(sBold.Render("Add to the modules that need them"))
	for _, lib := range newLibs {
		fmt.Printf("  implementation(%s)\n", catalog.Accessor(lib.Alias))
	}
	fmt.Println()
	fmt.Println(sMuted.Render(
		"Shared code goes in sharedLogic's commonMain block; Compose-only libraries go in androidApp or core/ui."))

	printResolveNotes(result)
	if *dryRun {
		fmt.Println()
		fmt.Println(sBold.Render("Dry run - the catalog was not modified."))
	}
	return nil
}

func specFromManifest(m *model.Manifest) model.Spec {
	return model.Spec{
		Name:          m.Name,
		Package:       m.Package,
		ApplicationID: m.ApplicationID,
		Android:       m.Android,
		IOS:           m.IOS,
		AndroidLayout: m.AndroidLayout,
		IOSLayout:     m.IOSLayout,
		SharedUtils:   m.SharedUtils,
		AndroidExtras: m.AndroidExtras,
		Packs:         append([]string(nil), m.Packs...),
		RootTabs:      m.RootTabs,
		JVMTarget:     "11",
	}
}

// appendToCatalog inserts new entries at the end of the [versions] and
// [libraries] blocks of libs.versions.toml.
func appendToCatalog(root string, versionLines, libraryLines []string) error {
	path := root + "/gradle/libs.versions.toml"
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading the version catalog: %w", err)
	}
	content := string(raw)

	insert := func(content, section string, lines []string) string {
		if len(lines) == 0 {
			return content
		}
		idx := strings.Index(content, "\n["+section+"]\n")
		if idx < 0 {
			return content + "\n[" + section + "]\n" + strings.Join(lines, "\n") + "\n"
		}
		// Find the start of the next section header after this one.
		rest := content[idx+len(section)+4:]
		next := strings.Index(rest, "\n[")
		insertAt := len(content)
		if next >= 0 {
			insertAt = idx + len(section) + 4 + next
		}
		block := "\n# Added by kmp-scaffold add library\n" + strings.Join(lines, "\n") + "\n"
		return content[:insertAt] + block + content[insertAt:]
	}

	content = insert(content, "libraries", libraryLines)
	content = insert(content, "versions", versionLines)

	return os.WriteFile(path, []byte(content), 0o644)
}
