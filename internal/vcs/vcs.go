// Package vcs sets up version control in a freshly generated project.
//
// It is the last thing a generation run does, and the least important: the
// project exists either way. Nothing here fails a command - anything that did
// not happen comes back as a note for the caller to print.
package vcs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Available reports whether git can be run at all.
func Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// InsideRepo reports whether dir already sits inside a working tree.
//
// Generating into a subdirectory of an existing project is a perfectly ordinary
// thing to do, and nesting a repository inside another one is almost never what
// was meant - so this is what decides whether to offer at all.
func InsideRepo(dir string) bool {
	if !Available() {
		return false
	}
	// The directory may not exist yet, so walk up to the first one that does.
	for {
		if _, err := os.Stat(dir); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}

	out, err := run(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// Result records what Init managed to do.
type Result struct {
	// Initialised is true when a repository was created.
	Initialised bool
	// Staged is true when the generated files were added to the index.
	Staged bool
	// Committed is true when the first commit was made.
	Committed bool
	// Note explains anything that did not happen, in a form worth printing.
	Note string
}

// Summary is one line describing what happened, or "" when nothing did.
func (r Result) Summary() string {
	switch {
	case r.Committed:
		return "initialised a git repository and made the first commit"
	case r.Staged:
		return "initialised a git repository and staged the files"
	case r.Initialised:
		return "initialised a git repository"
	default:
		return ""
	}
}

// Init creates a repository in dir, stages everything and commits it.
//
// Each step is allowed to be the last one. A machine with no git, or no
// configured identity, still ends up with a generated project - and a note
// saying exactly what to do to finish the job.
func Init(dir, message string) (Result, error) {
	if !Available() {
		return Result{Note: "git is not on your PATH, so no repository was created"}, nil
	}
	if InsideRepo(dir) {
		return Result{Note: "this directory is already inside a git repository"}, nil
	}

	// -b main rather than whatever init.defaultBranch happens to be, so the
	// branch name does not depend on the machine. Older git does not know the
	// flag, in which case the default is used and nothing is lost.
	if _, err := run(dir, "init", "-b", "main"); err != nil {
		if _, err := run(dir, "init"); err != nil {
			return Result{}, fmt.Errorf("git init: %w", err)
		}
	}
	result := Result{Initialised: true}

	if _, err := run(dir, "add", "-A"); err != nil {
		result.Note = "the repository was created, but the files could not be staged"
		return result, nil
	}
	result.Staged = true

	// A commit needs an identity, and a machine that has never configured one
	// is common enough to be worth handling rather than reporting as a failure.
	if !hasIdentity(dir) {
		result.Note = "the files are staged but not committed: git has no user.name or user.email yet.\n" +
			"  Set them, then run `git commit -m \"" + message + "\"`"
		return result, nil
	}
	if out, err := run(dir, "commit", "-m", message); err != nil {
		result.Note = "the files are staged but the commit failed: " + firstLine(out)
		return result, nil
	}
	result.Committed = true
	return result, nil
}

// hasIdentity reports whether git knows who is committing.
func hasIdentity(dir string) bool {
	for _, key := range []string{"user.name", "user.email"} {
		out, err := run(dir, "config", "--get", key)
		if err != nil || strings.TrimSpace(out) == "" {
			return false
		}
	}
	return true
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	// A credential or editor prompt would hang a run that has no terminal.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
