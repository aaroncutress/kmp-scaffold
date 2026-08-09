package vcs_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaroncutress/kmp-scaffold/internal/vcs"
)

// project is a directory with one file in it, standing in for a generated
// project.
func project(t *testing.T) string {
	t.Helper()
	if !vcs.Available() {
		t.Skip("git is not on PATH")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// identity gives this test's git an author, without touching the machine's own
// configuration.
func identity(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	config := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(config, []byte(
		"[user]\n\tname = Test\n\temail = test@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(home, "nonexistent"))
}

// noIdentity is a machine where git has never been configured.
func noIdentity(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "nonexistent"))
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(home, "nonexistent-system"))
	t.Setenv("GIT_AUTHOR_NAME", "")
	t.Setenv("GIT_AUTHOR_EMAIL", "")
	t.Setenv("GIT_COMMITTER_NAME", "")
	t.Setenv("GIT_COMMITTER_EMAIL", "")
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestInitCommits(t *testing.T) {
	identity(t)
	dir := project(t)

	result, err := vcs.Init(dir, "Initial commit")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Initialised || !result.Staged || !result.Committed {
		t.Fatalf("result = %+v, want all three steps", result)
	}
	if result.Note != "" {
		t.Errorf("unexpected note: %q", result.Note)
	}
	if got := result.Summary(); !strings.Contains(got, "first commit") {
		t.Errorf("summary = %q", got)
	}

	if got := git(t, dir, "log", "--oneline"); !strings.Contains(got, "Initial commit") {
		t.Errorf("git log = %q, want the commit", got)
	}
	// The generated file is in that commit, not left untracked.
	if got := git(t, dir, "show", "--name-only", "--format="); !strings.Contains(got, "README.md") {
		t.Errorf("the commit does not contain the generated file: %q", got)
	}
	// The branch name comes from the tool, not from whatever this machine
	// happens to default to.
	if got := strings.TrimSpace(git(t, dir, "rev-parse", "--abbrev-ref", "HEAD")); got != "main" {
		t.Errorf("branch = %q, want main", got)
	}
}

// Generating into a subdirectory of an existing project is ordinary. Nesting a
// repository inside another one is almost never what was meant.
func TestInitLeavesAnExistingRepositoryAlone(t *testing.T) {
	identity(t)
	outer := project(t)
	git(t, outer, "init", "-q", "-b", "main")

	inner := filepath.Join(outer, "packages", "thing")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}

	if !vcs.InsideRepo(inner) {
		t.Fatal("InsideRepo should see the outer repository")
	}

	result, err := vcs.Init(inner, "Initial commit")
	if err != nil {
		t.Fatal(err)
	}
	if result.Initialised {
		t.Error("a repository was nested inside another one")
	}
	if !strings.Contains(result.Note, "already inside") {
		t.Errorf("note = %q, want it to explain why nothing happened", result.Note)
	}
	if _, err := os.Stat(filepath.Join(inner, ".git")); err == nil {
		t.Error("a .git directory was created inside an existing repository")
	}
}

// A machine where git has never been configured still gets a repository, with
// its files staged and a note saying what to do.
func TestInitStopsAtStagingWithoutAnIdentity(t *testing.T) {
	noIdentity(t)
	dir := project(t)

	result, err := vcs.Init(dir, "Initial commit")
	if err != nil {
		t.Fatalf("a missing identity should not be an error: %v", err)
	}
	if !result.Initialised || !result.Staged {
		t.Fatalf("result = %+v, want the repository created and the files staged", result)
	}
	if result.Committed {
		t.Error("committed without an identity")
	}
	if !strings.Contains(result.Note, "user.name") {
		t.Errorf("note = %q, want it to name what is missing", result.Note)
	}
	if !strings.Contains(result.Note, "git commit") {
		t.Errorf("note = %q, want it to say how to finish", result.Note)
	}

	// Staged means staged: the file is in the index, ready to commit.
	if got := git(t, dir, "diff", "--cached", "--name-only"); !strings.Contains(got, "README.md") {
		t.Errorf("staged files = %q, want README.md", got)
	}
}

// InsideRepo is asked about a directory that does not exist yet, because the
// wizard decides whether to offer before anything is generated.
func TestInsideRepoHandlesAMissingDirectory(t *testing.T) {
	identity(t)
	outer := project(t)
	git(t, outer, "init", "-q", "-b", "main")

	if !vcs.InsideRepo(filepath.Join(outer, "not", "created", "yet")) {
		t.Error("a path under a repository is inside it, whether or not it exists")
	}

	loose := t.TempDir()
	if vcs.InsideRepo(filepath.Join(loose, "not-created-yet")) {
		t.Error("a path under no repository is not inside one")
	}
}

// Windows has no executable bit, so `gradlew` is written 0644 there and git
// records it that way - and a clone on macOS or Linux gets a gradlew that will
// not run. Git tracks the bit itself, so it can be set in the index even where
// the file cannot carry it.
//
// The pass is a no-op everywhere else, because the mode is already right.
func TestInitRecordsTheExecutableBit(t *testing.T) {
	identity(t)
	dir := project(t)

	// A script and a plain file, written exactly as the generator writes them.
	if err := os.WriteFile(filepath.Join(dir, "gradlew"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := vcs.Init(dir, "Initial commit")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Committed {
		t.Fatalf("result = %+v, want a commit", result)
	}

	// git ls-files -s prints the mode first: 100755 for executable.
	modes := map[string]string{}
	for _, line := range strings.Split(git(t, dir, "ls-files", "-s"), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 4 {
			modes[fields[3]] = fields[0]
		}
	}
	if got := modes["gradlew"]; got != "100755" {
		t.Errorf("gradlew is recorded as %s, want 100755 - it will not run when cloned", got)
	}
	if got := modes["notes.txt"]; got != "100644" {
		t.Errorf("notes.txt is recorded as %s, want 100644", got)
	}
}

// Notes accumulate. More than one thing can go unsaid in a single run, and the
// last one used to overwrite the rest.
func TestNotesAccumulate(t *testing.T) {
	noIdentity(t)
	dir := project(t)

	result, err := vcs.Init(dir, "Initial commit")
	if err != nil {
		t.Fatal(err)
	}
	// Only the identity note is expected here, but it must be present and whole
	// rather than truncated by a later writer.
	if !strings.Contains(result.Note, "user.name") {
		t.Errorf("note = %q, want the identity explanation", result.Note)
	}
}
