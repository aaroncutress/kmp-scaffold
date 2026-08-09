package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
	"github.com/aaroncutress/kmp-scaffold/internal/generator"
	"github.com/aaroncutress/kmp-scaffold/internal/kmp"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/render"
	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold/filetmpl"
	"github.com/aaroncutress/kmp-scaffold/internal/tui"
)

var errCancelled = errors.New("cancelled")

func runNew(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `Usage: kmp-scaffold new [directory] [flags]

Creates a project from a template. With no flags it runs an interactive
wizard; --yes generates straight away using the defaults plus any flags given.

Flags:
`)
		fs.PrintDefaults()
	}

	var (
		templateRef = fs.String("template", "",
			"Template to generate from (default: "+scaffold.DefaultTemplate+")")
		name       = fs.String("name", "", "Project name")
		pkg        = fs.String("package", "", "Package name, e.g. com.example.app")
		appID      = fs.String("application-id", "", "Android applicationId (defaults to --package)")
		noAndroid  = fs.Bool("no-android", false, "Do not generate an Android app")
		noIOS      = fs.Bool("no-ios", false, "Do not generate an iOS app")
		androidLay = fs.String("android-layout", "",
			"Android layout id ("+strings.Join(kmp.LayoutIDs(generator.KindAndroid), ", ")+")")
		iosLay = fs.String("ios-layout", "",
			"iOS layout id ("+strings.Join(kmp.LayoutIDs(generator.KindIOS), ", ")+")")
		tabs       = fs.String("tabs", "", "Comma-separated root tabs, e.g. Home,Settings")
		packs      = fs.String("libraries", "", "Comma-separated library packs (default: the basic set)")
		utils      = fs.String("utilities", "", "Comma-separated shared utilities (default: all)")
		extras     = fs.String("android-extras", "", "Comma-separated Android extras (default: all)")
		channel    = fs.String("channel", "", "Version channel: stable, preview or bleeding")
		minSDK     = fs.Int("min-sdk", 0, "Android minSdk")
		compileSDK = fs.Int("compile-sdk", 0, "Pin compileSdk instead of resolving it")
		gradleVer  = fs.String("gradle", "", "Pin the Gradle version")
		kotlinVer  = fs.String("kotlin", "", "Pin the Kotlin version")
		agpVer     = fs.String("agp", "", "Pin the Android Gradle Plugin version")
		offline    = fs.Bool("offline", false, "Skip version resolution and use the built-in baseline")
		refresh    = fs.Bool("refresh", false, "Re-fetch a remote template instead of using the cached copy")
		trust      = fs.Bool("trust", false, "Use a remote template without being asked to review it")
		yes        = fs.Bool("yes", false, "Skip the wizard and accept the defaults")
		dryRun     = fs.Bool("dry-run", false, "Report what would be written without writing it")
		force      = fs.Bool("force", false, "Overwrite files that already exist")
		verbose    = fs.Bool("verbose", false, "List every file written")
	)
	flags, operands := permute(fs, args)
	if err := fs.Parse(flags); err != nil {
		return ErrUsage
	}

	// Which flags were actually given, so that `--libraries ""` clears the
	// list rather than being indistinguishable from not passing it at all.
	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })

	// A remote template is fetched as it is loaded, so how that fetch behaves
	// has to be settled first.
	filetmpl.SetFetchOptions(filetmpl.FetchOptions{Refresh: *refresh, Offline: *offline})
	filetmpl.TrustEverything(*trust)

	// The template decides what the remaining questions are, so it has to be
	// chosen before the wizard can be built.
	choices := scaffold.All()
	if *templateRef == "" && !*yes && interactive() && len(choices) > 1 {
		picked, err := tui.PickTemplate(choices).Run(ctx)
		if err != nil {
			if errors.Is(err, tui.ErrCancelled) {
				return errCancelled
			}
			return err
		}
		*templateRef = picked.Str(tui.TemplateQuestion)
	}

	template, err := scaffold.Load(*templateRef)
	if err != nil {
		return err
	}
	answers := template.NewAnswers()
	answers.Offline = *offline

	// The structural flags belong to the Kotlin Multiplatform template. Another
	// template answers its questions through the wizard or its own defaults;
	// silently ignoring flags it has never heard of would be worse than saying
	// so, so they are rejected.
	kmpTemplate, isKMP := template.(kmp.Template)
	kmpFlags := []string{"no-android", "no-ios", "android-layout", "ios-layout", "tabs",
		"libraries", "utilities", "android-extras", "min-sdk", "compile-sdk", "gradle", "kotlin", "agp"}
	if !isKMP {
		for _, f := range kmpFlags {
			if given[f] {
				return fmt.Errorf("--%s only applies to the %s template", f, kmp.ID)
			}
		}
	}

	if positional := firstOperand(operands); positional != "" {
		answers.Project.Dir = positional
		if *name == "" {
			answers.Project.Name = model.Pascal(filepath.Base(filepath.Clean(positional)))
		}
	}
	if *name != "" {
		answers.Project.Name = *name
	}
	if *pkg != "" {
		answers.Project.Package = *pkg
		answers.Project.ApplicationID = *pkg
	}
	if *appID != "" {
		answers.Project.ApplicationID = *appID
	}

	if isKMP {
		spec := kmp.Spec(answers)
		if *noAndroid {
			spec.Android = false
		}
		if *noIOS {
			spec.IOS = false
		}
		if *androidLay != "" {
			spec.AndroidLayout = *androidLay
		}
		if *iosLay != "" {
			spec.IOSLayout = *iosLay
		}
		if *tabs != "" {
			spec.RootTabs = splitList(*tabs)
		}
		if given["libraries"] {
			spec.Packs = splitList(*packs)
		}
		if given["utilities"] {
			spec.SharedUtils = splitList(*utils)
		}
		if given["android-extras"] {
			spec.AndroidExtras = splitList(*extras)
		}
		if *channel != "" {
			spec.Channel = *channel
		}
		if *minSDK > 0 {
			spec.MinSDK = *minSDK
		}
		if *compileSDK > 0 {
			spec.CompileSDK = *compileSDK
		}
		spec.GradleVer = *gradleVer
		spec.KotlinVer = *kotlinVer
		spec.AGP = *agpVer
		spec.Offline = *offline
		kmpTemplate.Bind(answers)
	}

	var result *resolve.Result

	if *yes || !interactive() {
		if answers.Project.Name == "" {
			return fmt.Errorf("--name is required when not running interactively")
		}
		if err := model.ValidateProjectName(answers.Project.Name); err != nil {
			return fmt.Errorf("--name: %w", err)
		}
		if template.Meta().AsksPackage {
			if answers.Project.Package == "" {
				answers.Project.Package = "com.example." + model.LowerAlnum(answers.Project.Name)
				answers.Project.ApplicationID = answers.Project.Package
			}
			if err := model.ValidatePackage(answers.Project.Package); err != nil {
				return fmt.Errorf("--package: %w", err)
			}
		}
		if answers.Project.Dir == "" {
			answers.Project.Dir = model.Kebab(answers.Project.Name)
		}
		template.Normalise(answers)

		if req := template.Versions(answers); !req.Empty() {
			fmt.Println(sBold.Render("Resolving versions..."))
			result = resolve.Run(ctx, req)
			template.Check(answers, result)
		}
	} else {
		wizard, holder := tui.NewFlow(ctx, template, answers)
		answered, err := wizard.Run(ctx)
		if err != nil {
			if errors.Is(err, tui.ErrCancelled) {
				return errCancelled
			}
			return err
		}
		answers = answered
		result = holder.Result
	}

	notes := template.Normalise(answers)

	// The wizard resolved against pre-normalisation answers; if normalisation
	// changed the selection, resolve again so the catalog matches the project.
	// Offline resolution cannot change its mind, so it is only worth repeating
	// when there was no result at all - which is also the cancelled-early case.
	req := template.Versions(answers)
	if !req.Empty() && (result == nil || (len(notes) > 0 && !req.Offline)) {
		result = resolve.Run(ctx, req)
		template.Check(answers, result)
	}

	root, err := filepath.Abs(answers.Project.Dir)
	if err != nil {
		return err
	}
	if err := ensureUsableDir(root, template.Meta().Sentinels, *force); err != nil {
		return err
	}

	writer := render.NewWriter(root, *dryRun, *force)
	report, err := template.Generate(ctx, scaffold.GenRequest{
		Answers: answers,
		Result:  result,
		Writer:  writer,
		Version: Version,
	})
	if err != nil {
		return err
	}

	fmt.Println()
	if *dryRun {
		fmt.Println(sBold.Render("Dry run - nothing was written."))
	} else {
		fmt.Println(sOK.Render("✔ ") + sBold.Render(answers.Project.Name) + sMuted.Render(" created in "+root))
	}
	fmt.Println()
	printWriteSummary(writer, *verbose)

	for _, n := range notes {
		fmt.Println("  " + sMuted.Render("· "+n))
	}
	for _, wrn := range report.Warnings {
		fmt.Println("  " + sWarn.Render("! "+wrn))
	}
	printResolveNotes(result)

	if !*dryRun {
		printNextSteps(template, template.NextSteps(answers), root)
	}
	return nil
}

// ensureUsableDir refuses to generate into a directory that already looks like
// a project, unless --force was given. What "looks like a project" means is the
// template's call: a Gradle build and a Node one leave different traces.
func ensureUsableDir(root string, sentinels []string, force bool) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if force {
		return nil
	}
	if len(sentinels) == 0 {
		sentinels = []string{model.ManifestFile}
	}
	for _, e := range entries {
		if model.Has(sentinels, e.Name()) {
			return fmt.Errorf(
				"%s already contains a project (%s) - generate somewhere else, or pass --force",
				root, e.Name())
		}
	}
	return nil
}

func printNextSteps(t scaffold.Template, steps []scaffold.NextStep, root string) {
	rel, err := filepath.Rel(mustGetwd(), root)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = root
	}

	fmt.Println()
	fmt.Println(sBold.Render("Next steps"))
	if rel != "." {
		fmt.Printf("  cd %s\n", rel)
	}

	width := 0
	for _, s := range steps {
		if s.Note != "" && len(s.Command) > width {
			width = len(s.Command)
		}
	}
	for _, s := range steps {
		if s.Note == "" {
			fmt.Printf("  %s\n", s.Command)
			continue
		}
		fmt.Printf("  %-*s   %s\n", width, s.Command, sMuted.Render("# "+s.Note))
	}

	fmt.Println()
	// Only offer `add` for a template that can actually extend what it made.
	for _, r := range t.Recipes() {
		fmt.Println(sMuted.Render(fmt.Sprintf(
			"  kmp-scaffold add %s <name>%s to add a %s", r.Name,
			strings.Repeat(" ", max(1, 15-len(r.Name))), r.NounOr())))
	}
	fmt.Println(sMuted.Render("  kmp-scaffold versions            to check for newer releases"))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// defaultSpec is the answer set `versions` uses outside a project: what a fresh
// project would be generated with today.
func defaultSpec(channel string) model.Spec {
	spec := model.Defaults()
	spec.Packs = catalog.BasicPacks()
	spec.SharedUtils = catalog.DefaultUtilities()
	spec.AndroidExtras = catalog.DefaultExtras()
	if channel != "" {
		spec.Channel = channel
	}
	return spec
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
