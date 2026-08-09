package resolve

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
)

// Swift Package Manager has no metadata endpoint. A package is a git
// repository and its published versions are its tags, so resolving one means
// asking the remote what tags it has - which `git ls-remote` does without
// cloning anything.
//
// The tag list then goes through exactly the same Pick/Channel machinery Maven
// versions do, so a `1.0.6-beta.1` is treated as a prerelease here for the same
// reason it is there.

// PackageURL is the git URL a Swift coordinate names.
func PackageURL(c catalog.Coordinate) string {
	return strings.TrimSuffix(c.Group, "/") + "/" + c.Artifact
}

// swiftVersions lists a Swift package's versions, newest last.
func (c *Client) swiftVersions(ctx context.Context, coord catalog.Coordinate) ([]string, error) {
	url := PackageURL(coord)

	c.mu.Lock()
	if v, ok := c.cache[url]; ok {
		c.mu.Unlock()
		return v, nil
	}
	c.mu.Unlock()

	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("resolving %s needs git on your PATH", coord)
	}

	// --refs drops the ^{} peeled entries an annotated tag adds; --tags keeps
	// branches out of it.
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--tags", "--refs", url)
	// Without this a private or mistyped URL waits forever for a password.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing tags for %s: %w", coord, err)
	}

	versions := ParseTags(string(out))
	if len(versions) == 0 {
		return nil, fmt.Errorf("no version tags on %s", coord)
	}

	c.mu.Lock()
	c.cache[url] = versions
	c.mu.Unlock()
	return versions, nil
}

// ParseTags turns `git ls-remote --tags` output into a version list, oldest
// first - the order maven-metadata.xml uses, so the two are interchangeable.
//
// A leading "v" is stripped, because whether a project writes "v1.2.0" or
// "1.2.0" is a habit rather than a difference. Anything that does not start
// with a digit after that is not a version: release names, "latest", and the
// documentation tags some projects push.
func ParseTags(lsRemote string) []string {
	var out []string
	seen := map[string]bool{}

	for line := range strings.SplitSeq(lsRemote, "\n") {
		// Each line is "<sha>\trefs/tags/<name>".
		_, ref, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok {
			continue
		}
		name, ok := strings.CutPrefix(strings.TrimSpace(ref), "refs/tags/")
		if !ok {
			continue
		}
		name = strings.TrimSuffix(name, "^{}")

		version := strings.TrimPrefix(name, "v")
		if version == "" || version[0] < '0' || version[0] > '9' {
			continue
		}
		if !seen[version] {
			seen[version] = true
			out = append(out, version)
		}
	}

	// Tags come back in whatever order the remote lists them, which is
	// lexicographic - so "1.10.0" sorts before "1.9.0". Ordering them properly
	// here means the rest of the resolver cannot tell a tag list from a Maven
	// one.
	sort.SliceStable(out, func(i, j int) bool {
		return ParseVersion(out[i]).Less(ParseVersion(out[j]))
	})
	return out
}
