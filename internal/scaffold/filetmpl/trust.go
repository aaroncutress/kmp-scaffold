package filetmpl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Trust describes a remote template the user is being asked to accept.
//
// The point of showing all of it is that the answer is about *this* version of
// the template: the same repository at a different commit is a different set of
// files, and gets asked about again.
type Trust struct {
	// Ref is what the user typed.
	Ref string
	// URL is the repository it came from.
	URL string
	// Revision is the commit it resolved to.
	Revision string
	// ID and Label come from the template's own manifest.
	ID    string
	Label string
	// Files is how many files it would write, and Edits how many existing files
	// its recipes would insert into.
	Files int
	Edits int
	// Recipes are what it can add to a project later.
	Recipes []string
}

// TrustPrompt is asked whether to use a remote template. Returning false means
// the user said no, which is not an error - the command simply stops.
type TrustPrompt func(Trust) (bool, error)

var (
	trustMu     sync.RWMutex
	trustPrompt TrustPrompt
	// trustAll skips the prompt, for a non-interactive run that passed --trust.
	trustAll bool
)

// SetTrustPrompt installs how the user is asked. Without one, a remote template
// that has not been trusted before is refused rather than silently used.
func SetTrustPrompt(fn TrustPrompt) {
	trustMu.Lock()
	defer trustMu.Unlock()
	trustPrompt = fn
}

// TrustEverything accepts remote templates without asking.
//
// This is what --trust does, and it is deliberately separate from --yes: "do
// not ask me the wizard's questions" and "run code from the internet without
// looking" are different decisions, and conflating them would let a script
// acquire the second by asking for the first.
func TrustEverything(on bool) {
	trustMu.Lock()
	defer trustMu.Unlock()
	trustAll = on
}

// TrustFile is where accepted commits are recorded.
func TrustFile() string {
	if path := os.Getenv("KMP_SCAFFOLD_TRUST_FILE"); path != "" {
		return path
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "kmp-scaffold", "trusted.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "kmp-scaffold", "trusted.json")
}

// trustStore maps a repository to the commits accepted from it.
type trustStore struct {
	Trusted map[string][]string `json:"trusted"`
}

func readTrust() trustStore {
	store := trustStore{Trusted: map[string][]string{}}
	data, err := os.ReadFile(TrustFile())
	if err != nil {
		return store
	}
	if err := json.Unmarshal(data, &store); err != nil || store.Trusted == nil {
		return trustStore{Trusted: map[string][]string{}}
	}
	return store
}

// IsTrusted reports whether this exact commit of this repository was accepted
// before.
func IsTrusted(slug, revision string) bool {
	if revision == "" {
		return false
	}
	for _, rev := range readTrust().Trusted[slug] {
		if rev == revision {
			return true
		}
	}
	return false
}

// RecordTrust remembers an accepted commit.
func RecordTrust(slug, revision string) error {
	if revision == "" {
		return nil
	}
	store := readTrust()
	for _, rev := range store.Trusted[slug] {
		if rev == revision {
			return nil
		}
	}
	store.Trusted[slug] = append(store.Trusted[slug], revision)

	path := TrustFile()
	if path == "" {
		return fmt.Errorf("could not work out where to record trusted templates")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// ForgetTrust drops every accepted commit for a repository.
func ForgetTrust(slug string) error {
	store := readTrust()
	if _, ok := store.Trusted[slug]; !ok {
		return nil
	}
	delete(store.Trusted, slug)

	path := TrustFile()
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// confirm asks about a fetched remote template, unless it has been accepted
// before or --trust was given.
func confirm(ref Ref, fetched Fetched, t *Template) error {
	trustMu.RLock()
	all, prompt := trustAll, trustPrompt
	trustMu.RUnlock()

	if IsTrusted(ref.Slug(), fetched.Revision) {
		return nil
	}
	if all {
		// --trust is a decision, not a bypass: remembering it is what makes the
		// next use of this same commit need no flag at all.
		return RecordTrust(ref.Slug(), fetched.Revision)
	}
	if prompt == nil {
		return fmt.Errorf(
			"%s has not been used before. Run it once in a terminal to review it, or pass --trust",
			ref.Raw)
	}

	meta := t.Meta()
	req := Trust{
		Ref:      ref.Raw,
		URL:      ref.URL,
		Revision: fetched.Revision,
		ID:       meta.ID,
		Label:    meta.Label,
		Files:    len(t.manifest.File),
	}
	for _, name := range t.manifest.RecipeNames() {
		req.Recipes = append(req.Recipes, name)
		req.Edits += len(t.manifest.Recipes[name].Edit)
	}

	ok, err := prompt(req)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("not using %s", ref.Raw)
	}
	return RecordTrust(ref.Slug(), fetched.Revision)
}
