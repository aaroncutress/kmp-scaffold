// Command dist cross-compiles the release archives.
//
// This is a Go program rather than a shell block in the Taskfile because the
// shell block needed mkdir, rm, cp, ls, zip and sha256sum, none of which exist
// on Windows - go-task runs commands through an embedded interpreter that
// provides shell *syntax*, not coreutils. A release could therefore only ever
// be cut from Linux or macOS, and `task clean` failed on Windows for the same
// reason.
//
// Everything here is the standard library, so the only tools any task needs are
// go and git.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const distDir = "dist"

func main() {
	var (
		clean     = flag.Bool("clean", false, "Remove dist/ and the built binary, then stop")
		binary    = flag.String("binary", "kmp-scaffold", "Base name of the binary, without any .exe")
		version   = flag.String("version", "dev", "Version to stamp in and to name the archives with")
		ldflags   = flag.String("ldflags", "", "Passed to go build")
		platforms = flag.String("platforms", "", "Space-separated os/arch pairs")
	)
	flag.Parse()

	if err := run(*clean, *binary, *version, *ldflags, *platforms); err != nil {
		fmt.Fprintln(os.Stderr, "dist:", err)
		os.Exit(1)
	}
}

func run(clean bool, binary, version, ldflags, platforms string) error {
	if clean {
		return cleanUp(binary)
	}
	if err := os.RemoveAll(distDir); err != nil {
		return err
	}
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		return err
	}

	targets := strings.Fields(platforms)
	if len(targets) == 0 {
		return fmt.Errorf("no platforms given")
	}

	fmt.Printf("Building %s...\n", version)
	var archives []string
	for _, target := range targets {
		goos, goarch, ok := strings.Cut(target, "/")
		if !ok {
			return fmt.Errorf("%q is not an os/arch pair", target)
		}
		archive, err := build(binary, version, ldflags, goos, goarch)
		if err != nil {
			return fmt.Errorf("%s: %w", target, err)
		}
		archives = append(archives, archive)
		fmt.Println("  " + filepath.Base(archive))
	}

	return writeChecksums(archives)
}

// build compiles one target and packs it, returning the archive path.
func build(binary, version, ldflags, goos, goarch string) (string, error) {
	name := fmt.Sprintf("%s_%s_%s_%s", binary, version, goos, goarch)

	exe := binary
	if goos == "windows" {
		exe += ".exe"
	}
	out := filepath.Join(distDir, name, exe)

	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", ldflags, "-o", out, ".")
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}

	// Whatever else ships alongside it, if it is there.
	for _, extra := range []string{"README.md", "LICENSE"} {
		if err := copyIfPresent(extra, filepath.Join(distDir, name, extra)); err != nil {
			return "", err
		}
	}

	dir := filepath.Join(distDir, name)
	archive := filepath.Join(distDir, name+".tar.gz")
	pack := writeTarGz
	if goos == "windows" {
		archive = filepath.Join(distDir, name+".zip")
		pack = writeZip
	}
	if err := pack(dir, name, archive); err != nil {
		return "", err
	}
	// The staging directory has served its purpose; only the archive ships.
	return archive, os.RemoveAll(dir)
}

// executable reports whether a packed file should be runnable once unpacked.
//
// The archive carries the mode, not the filesystem the archive was built on -
// which is what lets a release cross-compiled on Windows still unpack with an
// executable binary on Linux. The old shell pipeline could not do this at all.
func executable(name string) bool {
	return !strings.HasSuffix(name, ".md") && !strings.EqualFold(name, "LICENSE")
}

// writeTarGz packs dir into out, with every member under name/.
func writeTarGz(dir, name, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	return walk(dir, func(rel string, data []byte, info fs.FileInfo) error {
		mode := int64(0o644)
		if executable(rel) {
			mode = 0o755
		}
		// The archive root is the release directory, so unpacking produces one
		// directory rather than scattering files into the current one.
		if err := tw.WriteHeader(&tar.Header{
			Name:    name + "/" + rel,
			Mode:    mode,
			Size:    int64(len(data)),
			ModTime: info.ModTime(),
		}); err != nil {
			return err
		}
		_, err := tw.Write(data)
		return err
	})
}

// writeZip is writeTarGz for the platforms that expect a zip.
func writeZip(dir, name, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	return walk(dir, func(rel string, data []byte, info fs.FileInfo) error {
		header := &zip.FileHeader{Name: name + "/" + rel, Method: zip.Deflate}
		// Zip cannot represent a time before 1980, and a zero one renders as
		// "1980-00-00" in every listing tool.
		header.Modified = info.ModTime()
		if executable(rel) {
			header.SetMode(0o755)
		} else {
			header.SetMode(0o644)
		}
		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
}

// walk visits every file under dir in a stable order, with its path relative to
// dir and always slash-separated - an archive member name is not an OS path.
func walk(dir string, fn func(rel string, data []byte, info fs.FileInfo) error) error {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		files = append(files, p)
		return nil
	})
	if err != nil {
		return err
	}
	// Sorted, so two runs of the same commit produce the same archive.
	sort.Strings(files)

	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := os.Stat(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if err := fn(filepath.ToSlash(rel), data, info); err != nil {
			return err
		}
	}
	return nil
}

// writeChecksums hashes the archives, and only the archives - a checksum file
// that includes itself is the classic way to get this wrong.
func writeChecksums(archives []string) error {
	sort.Strings(archives)

	var b strings.Builder
	for _, path := range archives {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		// The format sha256sum -c expects: hash, two spaces, name.
		fmt.Fprintf(&b, "%s  %s\n", hex.EncodeToString(sum[:]), filepath.Base(path))
	}
	return os.WriteFile(filepath.Join(distDir, "checksums.txt"), []byte(b.String()), 0o644)
}

func cleanUp(binary string) error {
	if err := os.RemoveAll(distDir); err != nil {
		return err
	}
	name := binary
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func copyIfPresent(from, to string) error {
	data, err := os.ReadFile(from)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.WriteFile(to, data, 0o644)
}
