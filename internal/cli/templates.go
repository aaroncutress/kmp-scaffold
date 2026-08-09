package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold/filetmpl"
)

func runTemplates(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("templates", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `Usage: kmp-scaffold templates [name]

Lists the templates `+"`kmp-scaffold new --template`"+` accepts: the built-in
ones and any in your templates folder. With a name or a path, shows what that
template asks.
`)
	}
	if err := fs.Parse(args); err != nil {
		return ErrUsage
	}

	if name := firstOperand(fs.Args()); name != "" {
		return showTemplate(name)
	}

	fmt.Println(sBold.Render("Templates"))
	for _, t := range scaffold.All() {
		m := t.Meta()
		marker := "  "
		if m.ID == scaffold.DefaultTemplate {
			marker = sOK.Render("* ")
		}
		origin := ""
		if m.Source.Kind != scaffold.SourceBuiltin {
			origin = sMuted.Render("  (" + string(m.Source.Kind) + ")")
		}
		fmt.Printf("%s%-14s %s%s\n", marker, m.ID, m.Label, origin)
		if m.Description != "" {
			fmt.Printf("  %-14s %s\n", "", sMuted.Render(m.Description))
		}
	}

	// A template in the user folder that will not parse is skipped when
	// generating, which would otherwise look like it had simply vanished.
	if broken := filetmpl.Broken(); len(broken) > 0 {
		names := make([]string, 0, len(broken))
		for name := range broken {
			names = append(names, name)
		}
		sort.Strings(names)
		fmt.Println()
		fmt.Println(sBold.Render("Not loadable"))
		for _, name := range names {
			fmt.Printf("  %-14s %s\n", name, sWarn.Render(broken[name].Error()))
		}
	}

	fmt.Println()
	fmt.Println(sMuted.Render("* the default. kmp-scaffold new --template <name> picks another;"))
	fmt.Println(sMuted.Render("  --template ./some/directory generates from a template on disk."))
	fmt.Println(sMuted.Render("  Your own templates live in " + filetmpl.UserDir()))
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
	if m.Version != "" {
		fmt.Println(sMuted.Render("Version: " + m.Version))
	}
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

	if d, ok := t.(scaffold.Describable); ok {
		if outputs := d.Outputs(); len(outputs) > 0 {
			fmt.Println()
			fmt.Println(sBold.Render("Writes"))
			for _, o := range outputs {
				fmt.Println("  " + o)
			}
		}
	}

	if recipes := t.Recipes(); len(recipes) > 0 {
		fmt.Println()
		fmt.Println(sBold.Render("Can add"))
		for _, r := range recipes {
			fmt.Printf("  %-16s %s\n", r.Name, r.Label)
			if r.Description != "" {
				fmt.Printf("  %-16s %s\n", "", sMuted.Render(r.Description))
			}
		}
	}
	return nil
}
