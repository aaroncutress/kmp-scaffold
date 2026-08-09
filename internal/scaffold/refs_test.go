package scaffold

import (
	"runtime"
	"strings"
	"testing"
)

// A template given as an absolute Windows path used to be mistaken for a git
// remote, because the check was for a leading "/" - which recognises an
// absolute path on Unix and misses one on Windows. The ref then had a commit
// appended, and nothing could open it.
func TestRefsAreClassifiedTheSameOnEveryPlatform(t *testing.T) {
	for _, c := range []struct {
		ref            string
		local, remote  bool
		windowsPathery bool
	}{
		{ref: "kmp-mobile"},
		{ref: "ktor-service"},

		{ref: "./templates/mine", local: true},
		{ref: "../mine", local: true},
		{ref: "~/templates/mine", local: true},
		{ref: "/srv/templates/mine", local: true},
		{ref: `C:\templates\mine`, local: true, windowsPathery: true},
		{ref: "C:/templates/mine", local: true, windowsPathery: true},

		{ref: "github:owner/repo", remote: true},
		{ref: "gitlab:owner/repo", remote: true},
		{ref: "https://example.com/o/r.git", remote: true},
		{ref: "git@example.com:o/r.git", remote: true},
	} {
		// filepath.IsAbs only knows a drive letter on Windows, so those two
		// cases can only be asserted there.
		if c.windowsPathery && runtime.GOOS != "windows" {
			continue
		}

		if got := localRef(c.ref); got != c.local {
			t.Errorf("localRef(%q) = %v, want %v", c.ref, got, c.local)
		}
		if got := remoteRef(c.ref); got != c.remote {
			t.Errorf("remoteRef(%q) = %v, want %v", c.ref, got, c.remote)
		}
		if got := looksLikeExternalRef(c.ref); got != (c.local || c.remote) {
			t.Errorf("looksLikeExternalRef(%q) = %v, want %v", c.ref, got, c.local || c.remote)
		}
	}
}

// Pinning appends the commit a remote resolved to. A local path has no commit
// to pin, and a bare name is not a location at all - appending to either
// produces a ref that names nothing.
func TestOnlyRemotesArePinned(t *testing.T) {
	for _, c := range []struct {
		source string
		want   string
	}{
		{"github:owner/repo", "github:owner/repo@abc123"},
		{"https://example.com/o/r.git", "https://example.com/o/r.git@abc123"},
		{"github:owner/repo@v2", "github:owner/repo@abc123"},

		{"kmp-mobile", ""},
		{"./templates/mine", ""},
		{"/srv/templates/mine", ""},
		{"~/mine", ""},
	} {
		got, ok := pinned(c.source, "abc123")
		if c.want == "" {
			if ok {
				t.Errorf("pinned(%q) = %q, want it left alone", c.source, got)
			}
			continue
		}
		if !ok || got != c.want {
			t.Errorf("pinned(%q) = %q/%v, want %q", c.source, got, ok, c.want)
		}
	}

	// The case the old code got wrong. On Unix a backslash is a legal filename
	// character, so this is only a path on Windows - but it must never come
	// back with a commit appended, because it is not a repository either way.
	if got, ok := pinned(`C:\templates\mine`, "abc123"); ok && strings.Contains(got, "@abc123") {
		t.Errorf("pinned a Windows path: %q", got)
	}
}
