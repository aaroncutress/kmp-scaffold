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
	var b strings.Builder
	b.WriteString(sBold.Render("This project accepts") + "\n")
	for _, r := range recipes {
		b.WriteString(fmt.Sprintf("  kmp-scaffold add %-10s %s\n", r.Name, sMuted.Render(r.Label)))
		if r.Description != "" {
			b.WriteString("  " + strings.Repeat(" ", 27) + sMuted.Render(r.Description) + "\n")
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
		fmt.Fprintf(os.Stderr, "Usage: kmp-scaffold add %s [name] [flags]\n\n", r.Name)
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
	if positional := firstOperand(operands); positional != "" {
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

	if *yes || !interactive() {
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
	} else {
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
	writer := render.NewWriter(root, *dryRun, *force)
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

	for _, w := range report.Warnings {
		fmt.Println("  " + sWarn.Render("! "+w))
	}

	if !*dryRun && run.template.Meta().ID == kmp.ID {
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
