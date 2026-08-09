package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
	"github.com/aaroncutress/kmp-scaffold/internal/kmp"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/render"
	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
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

Creates a feature and wires it into the project: settings.gradle.kts, the app's
dependencies, the route serializers, the entry provider, the Koin graph and -
on the modular iOS layout - the Features package and the app coordinator.

Flags:
`)
		fs.PrintDefaults()
	}

	var (
		targets_ = fs.String("targets", "",
			"Comma-separated: android, ios, shared (default: every one this project supports)")
		present = fs.String("presentation", "", "above-nav, overlay, dialog or shell (a root tab)")
		yes     = fs.Bool("yes", false, "Skip the wizard")
		dryRun  = fs.Bool("dry-run", false, "Report what would change without changing it")
		force   = fs.Bool("force", false, "Overwrite files that already exist")
		dir     = fs.String("dir", ".", "Project directory")
		verbose = fs.Bool("verbose", false, "List every file written")
	)
	flags, operands := permute(fs, args)
	if err := fs.Parse(flags); err != nil {
		return ErrUsage
	}

	manifest, root, err := model.LoadManifest(*dir)
	if err != nil {
		return err
	}

	base, err := scaffold.LoadFor(manifest.Template)
	if err != nil {
		return err
	}
	template, ok := base.(scaffold.FeatureTemplate)
	if !ok {
		return fmt.Errorf(
			"the %q template has nothing to add - it generates a project in one go",
			base.Meta().ID)
	}
	noun := template.FeatureNoun()

	answers := scaffold.NewAnswers()
	if positional := firstOperand(operands); positional != "" {
		answers.Project.Name = model.Kebab(positional)
	}

	// By default a feature covers every side of the project it can.
	targets := kmp.DefaultTargets(manifest)
	if *targets_ != "" {
		targets = splitList(*targets_)
		if err := kmp.ValidateTargets(targets); err != nil {
			return err
		}
	}
	answers.Set(kmp.QFeatureTargets, targets)
	if *present != "" {
		answers.Set(kmp.QFeaturePresentation, *present)
	}

	if *yes || !interactive() {
		if answers.Project.Name == "" {
			return fmt.Errorf("give the %s a name, e.g. `kmp-scaffold add %s billing`", noun, noun)
		}
		if err := model.ValidateFeatureName(answers.Project.Name); err != nil {
			return fmt.Errorf("%s name: %w", noun, err)
		}
		if manifest.FindFeature(answers.Project.Name) != nil {
			return fmt.Errorf("this project already has a %s called %q", noun, answers.Project.Name)
		}
	} else {
		wizard := tui.FeatureFlow(template, manifest, answers)
		answered, err := wizard.Run(ctx)
		if err != nil {
			if errors.Is(err, tui.ErrCancelled) {
				return errCancelled
			}
			return err
		}
		answers = answered
	}

	writer := render.NewWriter(root, *dryRun, *force)
	report, err := template.AddFeature(ctx, scaffold.FeatureRequest{
		Manifest: manifest,
		Root:     root,
		Name:     answers.Project.Name,
		Answers:  answers,
		Writer:   writer,
		Version:  Version,
		DryRun:   *dryRun,
	})
	if err != nil {
		return err
	}

	fmt.Println()
	if *dryRun {
		fmt.Println(sBold.Render("Dry run - nothing was written."))
	} else {
		fmt.Println(sOK.Render("✔ ") + sBold.Render(answers.Project.Name) + sMuted.Render(" added"))
		printMigrationNotice(manifest)
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

// printMigrationNotice explains a manifest that has just been upgraded in
// place, because the new file cannot be read by an older kmp-scaffold.
func printMigrationNotice(m *model.Manifest) {
	if !m.Migrated {
		return
	}
	fmt.Println("  " + sMuted.Render(fmt.Sprintf(
		"· %s was upgraded to schema %d; earlier versions of kmp-scaffold will not read it.",
		model.ManifestFile, model.ManifestSchema)))
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

	// The library catalog is the Kotlin Multiplatform template's, so this only
	// applies to a project generated from it.
	if id := manifest.Template.ID; id != "" && id != kmp.ID {
		return fmt.Errorf(
			"`add library` works on %s projects; this one was generated by the %q template",
			kmp.ID, id)
	}

	// Build a spec with only the new packs added, so the diff is exactly what
	// these packs contribute.
	spec, _, err := kmp.SpecFrom(manifest)
	if err != nil {
		return err
	}
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
	req := kmp.RequestFor(spec)
	req.Channel = catalog.ParseChannel(*channel)
	req.Offline = false
	result := resolve.Run(ctx, req)

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
		if err := manifest.SetVars(kmp.VarsOf(spec)); err != nil {
			return err
		}
		if err := manifest.Save(root); err != nil {
			return err
		}
		printMigrationNotice(manifest)
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
