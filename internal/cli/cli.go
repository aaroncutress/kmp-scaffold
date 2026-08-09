// Package cli wires the commands together.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	// filetmpl installs the loader for templates that are directories rather
	// than Go code. It is imported for that effect alone: internal/scaffold
	// cannot depend on the packages that implement templates, so the one place
	// that knows about both wires them together.
	_ "github.com/aaroncutress/kmp-scaffold/internal/scaffold/filetmpl"
)

// Version is the generator version, stamped into generated files. Overridden at
// build time with -ldflags "-X .../internal/cli.Version=vX.Y.Z".
var Version = "dev"

// ErrUsage signals that usage should be printed alongside the error.
var ErrUsage = errors.New("usage")

const usage = `kmp-scaffold - create and extend Kotlin Multiplatform projects

Usage:
  kmp-scaffold new [directory]        Create a project (interactive by default)
  kmp-scaffold add <what> [name]      Apply a template recipe and wire it in
  kmp-scaffold add library <pack>...  Add a library pack to the version catalog
  kmp-scaffold templates [name]       List the templates new can generate from
  kmp-scaffold versions               Show which dependencies have newer releases
  kmp-scaffold version                Print the kmp-scaffold version

Run any command with --help for its options.

Every command that writes files supports --dry-run, which reports what would
change without touching anything.`

// Run dispatches a command line. It returns the process exit code.
func Run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Println(usage)
		return 0
	}

	var err error
	switch args[0] {
	case "new", "create", "init":
		err = runNew(ctx, args[1:])
	case "add":
		err = runAdd(ctx, args[1:])
	case "templates", "template":
		err = runTemplates(ctx, args[1:])
	case "versions", "outdated":
		err = runVersions(ctx, args[1:])
	case "version", "--version", "-v":
		fmt.Printf("kmp-scaffold %s\n", Version)
		return 0
	case "help", "--help", "-h":
		fmt.Println(usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s\n", args[0], usage)
		return 2
	}

	switch {
	case err == nil:
		return 0
	case errors.Is(err, errCancelled):
		fmt.Println(dim("Cancelled - nothing was written."))
		return 130
	case errors.Is(err, ErrUsage):
		fmt.Fprintln(os.Stderr, usage)
		return 2
	default:
		fmt.Fprintln(os.Stderr, errorLine(err.Error()))
		return 1
	}
}

// permute separates flags from positional arguments so they can appear in any
// order. Go's flag package stops at the first non-flag argument, which would
// make `new my-app --yes` silently ignore --yes.
//
// Whether a flag consumes the next argument is asked of the FlagSet itself, so
// this stays correct as flags are added.
func permute(fs *flag.FlagSet, args []string) (flags, operands []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			operands = append(operands, args[i+1:]...)
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			operands = append(operands, arg)
			continue
		}

		flags = append(flags, arg)
		name, hasValue := strings.CutPrefix(arg, "-")
		name = strings.TrimPrefix(name, "-")
		if name, _, hasValue = strings.Cut(name, "="); hasValue {
			continue
		}

		f := fs.Lookup(name)
		if f == nil {
			continue // let flag.Parse report the unknown flag
		}
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			continue // bool flags take no separate value
		}
		if i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return flags, operands
}

// peekFlag reads one flag's value before a FlagSet exists.
//
// `add` has to load the project in order to know which recipe is being applied,
// and only then can it register that recipe's flags - but the project is where
// --dir points. This is the one flag that has to be read out of order.
func peekFlag(args []string, name, fallback string) string {
	for i, arg := range args {
		switch {
		case arg == "--"+name || arg == "-"+name:
			if i+1 < len(args) {
				return args[i+1]
			}
		case strings.HasPrefix(arg, "--"+name+"="):
			return strings.TrimPrefix(arg, "--"+name+"=")
		case strings.HasPrefix(arg, "-"+name+"="):
			return strings.TrimPrefix(arg, "-"+name+"=")
		}
	}
	return fallback
}

// firstOperand returns the first positional argument, or "".
func firstOperand(operands []string) string {
	if len(operands) == 0 {
		return ""
	}
	return operands[0]
}

// splitList parses a comma-separated flag value.
func splitList(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}
