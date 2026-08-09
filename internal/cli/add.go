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
	// Which recipe is being applied decides what flags there are, so it has to
	// be picked out before any of them are parsed.
	what, rest := splitRecipeName(args)

	// The library catalog is a command of the tool's, not a recipe, so it is
	// resolved after them: a template that names a recipe "library" gets its
	// own, and every other project gets this one.
	recipe, template, root, manifest, err := findRecipe(peekFlag(args, "dir", "."), what)
	switch {
	case err != nil && what == "":
		return err
	case err != nil:
		if what == "library" || what == "lib" || what == "dependency" {
			return runAddLibrary(ctx, rest)
		}
		return err
	}

	return runRecipe(ctx, recipeRun{
		recipe:   recipe,
		template: template,
		manifest: manifest,
		root:     root,
		args:     rest,
	})
}

// splitRecipeName pulls the first non-flag argument out of the list. It is not
// necessarily first: `kmp-scaffold add --dir ../app route` is a fair thing to
// type.
func splitRecipeName(args []string) (string, []string) {
	for i, arg := range args {
		if arg == "--" {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		// A flag's value is not the recipe name.
		if i > 0 && takesAValue(args[i-1]) {
			continue
		}
		return arg, append(append([]string(nil), args[:i]...), args[i+1:]...)
	}
	return "", args
}

// takesAValue reports whether a flag is one of `add`'s value-taking ones, so
// the argument after it is not mistaken for the recipe name. The list is short
// because it only has to cover the flags every recipe shares.
func takesAValue(arg string) bool {
	switch strings.TrimLeft(arg, "-") {
	case "dir", "targets", "presentation", "channel":
		return true
	default:
		return false
	}
}

// findRecipe loads the project and resolves what to add against the recipes its
// template offers.
func findRecipe(dir, what string) (scaffold.Recipe, scaffold.Template, string, *model.Manifest, error) {
	fail := func(err error) (scaffold.Recipe, scaffold.Template, string, *model.Manifest, error) {
		return scaffold.Recipe{}, nil, "", nil, err
	}

	manifest, root, err := model.LoadManifest(dir)
	if err != nil {
		return fail(err)
	}
	template, err := scaffold.LoadFor(manifest.Template)
	if err != nil {
		return fail(err)
	}

	recipes := template.Recipes()
	if len(recipes) == 0 {
		return fail(fmt.Errorf(
			"the %q template has nothing to add - it generates a project in one go",
			template.Meta().ID))
	}
	if what == "" {
		return fail(fmt.Errorf("what would you like to add?\n\n%s", recipeList(recipes)))
	}
	r, ok := scaffold.FindRecipe(template, what)
	if !ok {
		return fail(fmt.Errorf("the %q template cannot add %q.\n\n%s",
			template.Meta().ID, what, recipeList(recipes)))
	}
	return r, template, root, manifest, nil
}

func recipeList(recipes []scaffold.Recipe) string {
	// Width from the longest name rather than a fixed one, so a recipe called
	// something long does not push its own label out of the column.
	width := 0
	for _, r := range recipes {
		width = max(width, len(r.Name))
	}
	const prefix = "  kmp-scaffold add "
	indent := strings.Repeat(" ", len(prefix)+width+1)

	var b strings.Builder
	b.WriteString(sBold.Render("This project accepts") + "\n")
	for _, r := range recipes {
		b.WriteString(fmt.Sprintf("%s%-*s %s\n", prefix, width, r.Name, sMuted.Render(r.Label)))
		if r.Description != "" {
			b.WriteString(indent + sMuted.Render(r.Description) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

type recipeRun struct {
	recipe   scaffold.Recipe
	template scaffold.Template
	manifest *model.Manifest
	root     string
	args     []string
}

func runRecipe(ctx context.Context, run recipeRun) error {
	r := run.recipe
	noun := r.NounOr()

	fs := flag.NewFlagSet("add "+r.Name, flag.ContinueOnError)
	fs.Usage = func() {
		usage := "Usage: kmp-scaffold add %s [name] [flags]\n\n"
		if r.Singleton {
			usage = "Usage: kmp-scaffold add %s [flags]\n\n"
		}
		fmt.Fprintf(os.Stderr, usage, r.Name)
		if r.Description != "" {
			fmt.Fprintf(os.Stderr, "%s\n\n", r.Description)
		}
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
	}

	var (
		yes     = fs.Bool("yes", false, "Skip the wizard")
		dryRun  = fs.Bool("dry-run", false, "Report what would change without changing it")
		force   = fs.Bool("force", false, "Overwrite files that already exist")
		dir     = fs.String("dir", ".", "Project directory")
		verbose = fs.Bool("verbose", false, "List every file written")

		// The Kotlin Multiplatform feature recipe has two shortcuts worth
		// keeping. They are registered only for it, so another template's
		// recipe does not advertise flags it has never heard of.
		targetList  *string
		presentFlag *string
	)
	if run.template.Meta().ID == kmp.ID && r.Name == "feature" {
		targetList = fs.String("targets", "",
			"Comma-separated: android, ios, shared (default: every one this project supports)")
		presentFlag = fs.String("presentation", "",
			"above-nav, overlay, dialog or shell (a root tab)")
	}

	flags, operands := permute(fs, run.args)
	if err := fs.Parse(flags); err != nil {
		return ErrUsage
	}

	_ = dir // already read by the pre-scan that found this recipe
	manifest, root := run.manifest, run.root

	answers := scaffold.NewAnswers()
	answers.Project = manifest.Project
	positional := firstOperand(operands)
	if r.Singleton {
		// There is one of it, so there is nothing to call it. Taking a name and
		// ignoring it would leave the user thinking they had named something.
		if positional != "" {
			return fmt.Errorf("`add %s` takes no name - there is only one %s in a project", r.Name, noun)
		}
		if r.Applied(manifest) && !*force {
			return fmt.Errorf("this project already has %s - `--force` re-applies it, "+
				"overwriting the files it owns", noun)
		}
	} else if positional != "" {
		answers.Set(scaffold.NameAnswer, model.Kebab(positional))
	}

	if targetList != nil {
		targets := kmp.DefaultTargets(manifest)
		if *targetList != "" {
			targets = splitList(*targetList)
			if err := kmp.ValidateTargets(targets); err != nil {
				return err
			}
		}
		answers.Set(kmp.QFeatureTargets, targets)
		if *presentFlag != "" {
			answers.Set(kmp.QFeaturePresentation, *presentFlag)
		}
	}

	switch {
	case r.Singleton && (*yes || !interactive()):
		// Nothing to validate: a singleton has no name, and whether it has
		// already been applied was settled above.
	case *yes || !interactive():
		name := answers.Str(scaffold.NameAnswer)
		if name == "" {
			return fmt.Errorf("give the %s a name, e.g. `kmp-scaffold add %s billing`", noun, r.Name)
		}
		if err := model.ValidateFeatureName(name); err != nil {
			return fmt.Errorf("%s name: %w", noun, err)
		}
		if manifest.FindFeatureOf(r.Name, name) != nil {
			return fmt.Errorf("this project already has a %s called %q", noun, name)
		}
	default:
		answered, err := tui.RecipeFlow(r, manifest, answers).Run(ctx)
		if err != nil {
			if errors.Is(err, tui.ErrCancelled) {
				return errCancelled
			}
			return err
		}
		answers = answered
	}

	name := answers.Str(scaffold.NameAnswer)
	if r.Singleton {
		// The manifest keys a recipe's record by name, and a singleton's name is
		// the recipe itself - so a second `add tests` finds the first one.
		name = r.Name
	}
	writer := render.NewWriter(root, *dryRun, *force)
	// A recipe writes into a project someone has been working in, so a file it
	// wants to write and cannot is worth showing rather than only counting.
	writer.Sidecars = true
	report, err := r.Apply(ctx, scaffold.RecipeRequest{
		Recipe:   r.Name,
		Manifest: manifest,
		Root:     root,
		Name:     name,
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
		fmt.Println(sOK.Render("✔ ") + sBold.Render(name) + sMuted.Render(" added"))
		printMigrationNotice(manifest)
	}
	fmt.Println()
	printWriteSummary(writer, *verbose)
	printWireSummary(report.Wire)

	for _, n := range report.Notes {
		fmt.Println("  " + sMuted.Render("· "+n))
	}
	for _, w := range report.Warnings {
		fmt.Println("  " + sWarn.Render("! "+w))
	}

	// Only when something Gradle reads actually changed. Editor configuration
	// is written into a Kotlin project too, and telling someone to sync for it
	// trains them to ignore the line.
	if !*dryRun && run.template.Meta().ID == kmp.ID && touchedGradle(writer, report) {
		fmt.Println()
		fmt.Println(sMuted.Render("Sync Gradle to pick up the changes."))
	}
	return nil
}

// touchedGradle reports whether anything Gradle reads was written or edited.
func touchedGradle(w *render.Writer, report *scaffold.Report) bool {
	isGradle := func(p string) bool {
		return strings.HasSuffix(p, ".gradle.kts") || strings.HasSuffix(p, ".versions.toml")
	}
	for _, a := range w.Actions() {
		if a.Status != render.Skipped && a.Status != render.Unchanged && isGradle(a.Path) {
			return true
		}
	}
	for _, r := range report.Wire {
		if r.Inserted > 0 && isGradle(r.Path) {
			return true
		}
	}
	// A catalog edit is not one of the writer's, so the recipe reports it.
	return len(report.Notes) > 0
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

	// An anchor that is not there is the ordinary case for a project generated
	// before it existed, which is exactly the project someone retrofits into.
	// Saying what to paste and where is the difference between a warning and a
	// thing the reader can act on.
	for _, r := range results {
		if !r.Missing || len(r.Lines) == 0 {
			continue
		}
		fmt.Println()
		fmt.Printf("%s %s\n", sWarn.Render("!"), sBold.Render(r.Path))
		fmt.Println(sMuted.Render(fmt.Sprintf(
			"  no `%s` anchor here, so add this by hand:", r.Anchor)))
		// The blank lines a block carries are for spacing it from whatever it
		// was inserted next to, and there is nothing next to it here.
		lines := r.Lines
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		for _, line := range lines {
			fmt.Println("    " + line)
		}
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
		if err := kmp.AppendToCatalog(root, versionLines, libraryLines); err != nil {
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
