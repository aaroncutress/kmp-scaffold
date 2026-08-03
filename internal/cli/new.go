package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
	"github.com/aaroncutress/kmp-scaffold/internal/generator"
	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/render"
	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
	"github.com/aaroncutress/kmp-scaffold/internal/tui"
)

var errCancelled = errors.New("cancelled")

func runNew(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `Usage: kmp-scaffold new [directory] [flags]

Creates a Kotlin Multiplatform project. With no flags it runs an interactive
wizard; --yes generates straight away using the defaults plus any flags given.

Flags:
`)
		fs.PrintDefaults()
	}

	var (
		name       = fs.String("name", "", "Project name (rootProject.name)")
		pkg        = fs.String("package", "", "Package name, e.g. com.example.app")
		appID      = fs.String("application-id", "", "Android applicationId (defaults to --package)")
		noAndroid  = fs.Bool("no-android", false, "Do not generate an Android app")
		noIOS      = fs.Bool("no-ios", false, "Do not generate an iOS app")
		androidLay = fs.String("android-layout", "", "Android layout id (nav3-shell, nav3-single)")
		iosLay     = fs.String("ios-layout", "", "iOS layout id (swiftui-simple, none)")
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

	spec := model.Defaults()
	spec.Packs = catalog.BasicPacks()
	spec.SharedUtils = catalog.DefaultUtilities()
	spec.AndroidExtras = catalog.DefaultExtras()

	if positional := firstOperand(operands); positional != "" {
		spec.Dir = positional
		if *name == "" {
			spec.Name = model.Pascal(filepath.Base(filepath.Clean(positional)))
		}
	}

	// Flags override defaults, and are also the seed values the wizard starts from.
	if *name != "" {
		spec.Name = *name
	}
	if *pkg != "" {
		spec.Package = *pkg
		spec.ApplicationID = *pkg
	}
	if *appID != "" {
		spec.ApplicationID = *appID
	}
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

	var result *resolve.Result

	if *yes || !interactive() {
		if spec.Name == "" {
			return fmt.Errorf("--name is required when not running interactively")
		}
		if spec.Package == "" {
			spec.Package = "com.example." + model.LowerAlnum(spec.Name)
			spec.ApplicationID = spec.Package
		}
		if spec.Dir == "" {
			spec.Dir = model.Kebab(spec.Name)
		}
		if err := model.ValidateProjectName(spec.Name); err != nil {
			return fmt.Errorf("--name: %w", err)
		}
		if err := model.ValidatePackage(spec.Package); err != nil {
			return fmt.Errorf("--package: %w", err)
		}
		catalog.Normalise(&spec)

		fmt.Println(sBold.Render("Resolving versions..."))
		result = resolve.Run(ctx, spec, resolveOptions(spec, *kotlinVer, *agpVer))
	} else {
		wizard, holder := tui.NewFlow(ctx, spec)
		answered, err := wizard.Run(ctx)
		if err != nil {
			if errors.Is(err, tui.ErrCancelled) {
				return errCancelled
			}
			return err
		}
		spec = answered
		result = holder.Result
		if result == nil {
			result = resolve.Run(ctx, spec, resolveOptions(spec, *kotlinVer, *agpVer))
		}
	}

	notes := catalog.Normalise(&spec)

	// The wizard resolved against pre-normalisation answers; if normalisation
	// changed the selection, resolve again so the catalog matches the project.
	if len(notes) > 0 && !spec.Offline {
		result = resolve.Run(ctx, spec, resolveOptions(spec, *kotlinVer, *agpVer))
	}

	root, err := filepath.Abs(spec.Dir)
	if err != nil {
		return err
	}
	if err := ensureUsableDir(root, *force); err != nil {
		return err
	}

	writer := render.NewWriter(root, *dryRun, *force)
	report, err := generator.NewProject(ctx, spec, result, Version, writer)
	if err != nil {
		return err
	}

	fmt.Println()
	if *dryRun {
		fmt.Println(sBold.Render("Dry run - nothing was written."))
	} else {
		fmt.Println(sOK.Render("✔ ") + sBold.Render(spec.Name) + sMuted.Render(" created in "+root))
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
		printNextSteps(spec, root)
	}
	return nil
}

func resolveOptions(spec model.Spec, kotlinVer, agpVer string) resolve.Options {
	overrides := map[string]string{}
	if kotlinVer != "" {
		overrides[catalog.KeyKotlin] = kotlinVer
	}
	if agpVer != "" {
		overrides[catalog.KeyAGP] = agpVer
	}
	return resolve.Options{
		Channel:   catalog.ParseChannel(spec.Channel),
		Offline:   spec.Offline,
		Timeout:   20 * time.Second,
		Overrides: overrides,
	}
}

// ensureUsableDir refuses to generate into a directory that already looks like
// a project, unless --force was given.
func ensureUsableDir(root string, force bool) error {
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
	for _, e := range entries {
		switch e.Name() {
		case "settings.gradle.kts", "build.gradle.kts", model.ManifestFile:
			return fmt.Errorf(
				"%s already contains a Gradle project (%s) - generate somewhere else, or pass --force",
				root, e.Name())
		}
	}
	return nil
}

func printNextSteps(spec model.Spec, root string) {
	rel, err := filepath.Rel(mustGetwd(), root)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = root
	}

	fmt.Println()
	fmt.Println(sBold.Render("Next steps"))
	if rel != "." {
		fmt.Printf("  cd %s\n", rel)
	}
	if spec.HasPack("secrets") {
		fmt.Println("  cp secrets.properties.template secrets.properties   " +
			sMuted.Render("# then fill it in"))
	}
	if spec.Android {
		fmt.Println("  ./gradlew :androidApp:assembleDebug")
	}
	if spec.IOS && spec.IOSLayout != "none" {
		fmt.Println("  open iosApp/iosApp.xcodeproj                        " +
			sMuted.Render("# on a Mac"))
	}
	fmt.Println()
	fmt.Println(sMuted.Render("  kmp-scaffold add feature <name>   to add a feature module"))
	fmt.Println(sMuted.Render("  kmp-scaffold versions            to check for newer releases"))
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
