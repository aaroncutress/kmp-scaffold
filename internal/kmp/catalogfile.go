package kmp

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AppendToCatalog inserts version and library lines into the right sections of
// gradle/libs.versions.toml.
//
// The file belongs to the project, and by the time anything appends to it the
// user has probably reordered or commented parts of it - so this finds the
// section headers rather than regenerating, and falls back to appending a
// section that is not there rather than failing.
func AppendToCatalog(root string, versionLines, libraryLines []string) error {
	file := filepath.Join(root, "gradle", "libs.versions.toml")
	raw, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("reading the version catalog: %w", err)
	}
	content := string(raw)

	insert := func(content, section string, lines []string) string {
		if len(lines) == 0 {
			return content
		}
		idx := strings.Index(content, "\n["+section+"]\n")
		if idx < 0 {
			return content + "\n[" + section + "]\n" + strings.Join(lines, "\n") + "\n"
		}
		// Find the start of the next section header after this one.
		rest := content[idx+len(section)+4:]
		next := strings.Index(rest, "\n[")
		insertAt := len(content)
		if next >= 0 {
			insertAt = idx + len(section) + 4 + next
		}
		block := "\n# Added by kmp-scaffold\n" + strings.Join(lines, "\n") + "\n"
		return content[:insertAt] + block + content[insertAt:]
	}

	content = insert(content, "libraries", libraryLines)
	content = insert(content, "versions", versionLines)

	return os.WriteFile(file, []byte(content), 0o644)
}

// readCatalog returns every key already declared in the version catalog, in
// either the [versions] or [libraries] section.
//
// It is a deliberately loose parse: the question is only "is this name already
// spoken for", and answering it wrongly would either duplicate an entry or drop
// one. Anything that looks like `name = ` counts, including a commented-out
// line, because a commented entry is one the user turned off on purpose.
func readCatalog(root string) (map[string]bool, error) {
	f, err := os.Open(filepath.Join(root, "gradle", "libs.versions.toml"))
	if err != nil {
		return nil, fmt.Errorf("reading the version catalog: %w", err)
	}
	defer f.Close()

	keys := map[string]bool{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		name, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if name = strings.TrimSpace(name); name != "" && !strings.ContainsAny(name, " \t[]") {
			keys[name] = true
		}
	}
	return keys, scanner.Err()
}
