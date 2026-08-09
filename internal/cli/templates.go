package cli

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
)

func runTemplates(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("templates", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `Usage: kmp-scaffold templates [name]

Lists the templates `+"`kmp-scaffold new --template`"+` accepts. With a name, shows
what that template asks and what it generates.
`)
	}
	if err := fs.Parse(args); err != nil {
		return ErrUsage
	}

	if name := firstOperand(fs.Args()); name != "" {
		return showTemplate(name)
	}

	fmt.Println(sBold.Render("Templates"))
	for _, t := range scaffold.Builtins() {
		m := t.Meta()
		marker := "  "
		if m.ID == scaffold.DefaultTemplate {
			marker = sOK.Render("* ")
		}
		fmt.Printf("%s%-14s %s\n", marker, m.ID, m.Label)
		fmt.Printf("  %-14s %s\n", "", sMuted.Render(m.Description))
	}
	fmt.Println()
	fmt.Println(sMuted.Render("* the default. kmp-scaffold new --template <name> picks another."))
	return nil
}

func showTemplate(name string) error {
	t, err := scaffold.Load(name)
	if err != nil {
		return err
	}
	m := t.Meta()

	fmt.Println(sBold.Render(m.Label) + sMuted.Render("  ("+m.ID+")"))
	fmt.Println(m.Description)
	fmt.Println()
	fmt.Println(sMuted.Render("Source: " + m.Source.String()))

	fmt.Println()
	fmt.Println(sBold.Render("Questions"))
	fmt.Printf("  %-16s %s\n", "name", sMuted.Render("What is your project called?"))
	fmt.Printf("  %-16s %s\n", "directory", sMuted.Render("Where should it go?"))
	if m.AsksPackage {
		fmt.Printf("  %-16s %s\n", "package", sMuted.Render("Package name?"))
	}
	for _, q := range t.Questions() {
		fmt.Printf("  %-16s %s\n", q.ID, sMuted.Render(q.Prompt))
	}

	if ft, ok := t.(scaffold.FeatureTemplate); ok {
		fmt.Println()
		fmt.Printf("%s %s\n", sBold.Render("Can add:"), ft.FeatureNoun()+"s")
	}
	return nil
}
