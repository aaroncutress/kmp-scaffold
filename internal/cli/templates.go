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
	switch firstOperand(args) {
	case "add", "fetch", "install":
		return runTemplatesAdd(args[1:])
	case "remove", "rm", "forget":
		return runTemplatesRemove(args[1:])
	}

	fs := flag.NewFlagSet("templates", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `Usage: kmp-scaffold templates [name]
       kmp-scaffold templates add <ref> [--refresh] [--trust]
       kmp-scaffold templates remove <ref>

Lists the templates `+"`kmp-scaffold new --template`"+` accepts: the built-in
ones, any in your templates folder, and any fetched from a git repository. With
a name or a path, shows what that template asks and what it writes.

A <ref> is any of:

  github:owner/repo                      the default branch
  github:owner/repo@v2                   a tag, branch or commit
  github:owner/repo/templates/service    a directory inside the repository
  gitlab:owner/repo@main
  https://git.example.com/t/tmpl.git//service@v2
  git@github.com:owner/repo.git@v2

The first time a given commit is used you are shown what it would do and asked
whether to trust it. Say yes and that commit is remembered; a later one asks
again.
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

	if cached := filetmpl.CachedTemplates(); len(cached) > 0 {
		fmt.Println()
		fmt.Println(sBold.Render("Fetched"))
		for _, c := range cached {
			m := c.Template.Meta()
			trusted := sWarn.Render("not yet reviewed")
			if filetmpl.IsTrusted(c.Ref.Slug(), c.Revision) {
				trusted = sMuted.Render("reviewed")
			}
			fmt.Printf("  %-14s %s  %s\n", m.ID, c.Ref.Raw, trusted)
			fmt.Printf("  %-14s %s\n", "", sMuted.Render("at "+short(c.Revision)))
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
	fmt.Println(sMuted.Render("  --template ./some/directory generates from one on disk, and"))
	fmt.Println(sMuted.Render("  --template github:owner/repo from a git repository."))
	fmt.Println(sMuted.Render("  Your own templates live in " + filetmpl.UserDir()))
	return nil
}

func short(revision string) string {
	if len(revision) > 12 {
		return revision[:12]
	}
	return revision
}

func runTemplatesAdd(args []string) error {
	fs := flag.NewFlagSet("templates add", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `Usage: kmp-scaffold templates add <ref> [flags]

Fetches a template from a git repository, checks that it parses, and caches it
so `+"`--template <its id>`"+` works from anywhere.

Flags:
`)
		fs.PrintDefaults()
	}
	var (
		refresh = fs.Bool("refresh", false, "Re-fetch even if it is already cached")
		trust   = fs.Bool("trust", false, "Do not ask before using it")
	)
	flags, operands := permute(fs, args)
	if err := fs.Parse(flags); err != nil {
		return ErrUsage
	}

	raw := firstOperand(operands)
	if raw == "" {
		return ErrUsage
	}
	ref, ok := filetmpl.ParseRef(raw)
	if !ok {
		return fmt.Errorf("%q is not a template reference - run `kmp-scaffold templates --help` "+
			"for the forms it takes", raw)
	}

	filetmpl.TrustEverything(*trust)
	fmt.Println(sBold.Render("Fetching " + ref.Raw + "..."))

	t, err := filetmpl.OpenRemote(ref, filetmpl.FetchOptions{Refresh: *refresh})
	if err != nil {
		return err
	}

	m := t.Meta()
	fmt.Println()
	fmt.Println(sOK.Render("✔ ") + sBold.Render(m.Label) + sMuted.Render("  ("+m.ID+")"))
	fmt.Println("  " + sMuted.Render("at "+short(m.Source.Revision)))
	fmt.Println()
	fmt.Printf("  kmp-scaffold new my-project --template %s\n", m.ID)
	return nil
}

func runTemplatesRemove(args []string) error {
	fs := flag.NewFlagSet("templates remove", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `Usage: kmp-scaffold templates remove <ref>

Drops a fetched template from the cache, and forgets that it was reviewed. The
next use fetches it again and asks again.
`)
	}
	if err := fs.Parse(args); err != nil {
		return ErrUsage
	}

	raw := firstOperand(fs.Args())
	if raw == "" {
		return ErrUsage
	}

	ref, ok := filetmpl.ParseRef(raw)
	if !ok {
		// A bare id is friendlier to type than the ref it was fetched with.
		for _, c := range filetmpl.CachedTemplates() {
			if c.Template.Meta().ID == raw {
				ref, ok = c.Ref, true
				break
			}
		}
	}
	if !ok {
		return fmt.Errorf("%q is not a fetched template - `kmp-scaffold templates` lists them", raw)
	}

	if err := filetmpl.Forget(ref); err != nil {
		return err
	}
	if err := filetmpl.ForgetTrust(ref.Slug()); err != nil {
		return err
	}
	fmt.Println(sOK.Render("✔ ") + ref.Raw + sMuted.Render(" removed from the cache"))
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
