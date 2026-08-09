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
	"runtime"
	"strings"

	"github.com/aaroncutress/kmp-scaffold/internal/render"
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
	// More than one thing can go unsaid, so add to it with addNote rather than
	// assigning - the last writer would otherwise silently win.
	Note string
}

// addNote records something the caller should be told, keeping whatever was
// already there.
func (r *Result) addNote(s string) {
	if s == "" {
		return
	}
	if r.Note != "" {
		r.Note += "\n  "
	}
	r.Note += s
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
		result.addNote("the repository was created, but the files could not be staged")
		return result, nil
	}
	result.Staged = true
	result.addNote(markExecutable(dir))

	// A commit needs an identity, and a machine that has never configured one
	// is common enough to be worth handling rather than reporting as a failure.
	if !hasIdentity(dir) {
		result.addNote("the files are staged but not committed: git has no user.name or user.email yet.\n" +
			"  Set them, then run `git commit -m \"" + message + "\"`")
		return result, nil
	}
	if out, err := run(dir, "commit", "-m", message); err != nil {
		result.addNote("the files are staged but the commit failed: " + firstLine(out))
		return result, nil
	}
	result.Committed = true
	return result, nil
}

// markExecutable records the executable bit for scripts, on the one platform
// that cannot set it.
//
// Windows has no executable bit, so `gradlew` and the shell scripts are written
// 0644 whatever mode was asked for - and git then records them that way. Clone
// that repository on macOS or Linux and `./gradlew` will not run, which is a
// confusing thing to inherit from the machine a project happened to be
// generated on.
//
// Git tracks the bit itself, independently of the filesystem, so it can be set
// in the index even where the file cannot carry it. Everywhere else this is a
// no-op: the mode is already right and git has already recorded it.
//
// A failure here is worth mentioning and not worth stopping for - the files are
// staged either way, and `git update-index --chmod=+x` is one command to run by
// hand.
func markExecutable(dir string) string {
	if runtime.GOOS != "windows" {
		return ""
	}

	staged, err := run(dir, "diff", "--cached", "--name-only")
	if err != nil {
		return ""
	}
	var scripts []string
	for _, path := range strings.Split(staged, "\n") {
		if path = strings.TrimSpace(path); path == "" {
			continue
		}
		// The same rule the writer used when it chose the mode.
		if render.ExecutableMode(path) == 0o755 {
			scripts = append(scripts, path)
		}
	}
	if len(scripts) == 0 {
		return ""
	}

	args := append([]string{"update-index", "--chmod=+x"}, scripts...)
	if _, err := run(dir, args...); err != nil {
		return "could not mark " + strings.Join(scripts, ", ") + " executable in git.\n" +
			"  Run `git update-index --chmod=+x " + strings.Join(scripts, " ") + "`, " +
			"or they will not be runnable when this is cloned on macOS or Linux"
	}
	return ""
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
	cmd := exec.Command("git", append(gitConfig(), args...)...)
	cmd.Dir = dir
	// A credential or editor prompt would hang a run that has no terminal.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// gitConfig is the configuration every git invocation here runs with.
//
// core.longpaths lets Git for Windows use the APIs that are not capped at 260
// characters. A generated project nests a Kotlin package under several source
// directories, so a project in a deep enough place can exceed that before git
// has added anything of its own. The setting does not exist off Windows.
func gitConfig() []string {
	if runtime.GOOS != "windows" {
		return nil
	}
	return []string{"-c", "core.longpaths=true"}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
