package resolve

import (
	"context"
	"fmt"
)

// WrapperFiles are the three files that make `./gradlew` work. They are
// fetched from the Gradle source tree at the matching tag rather than
// generated, so they are byte-for-byte what `gradle wrapper` would produce.
type WrapperFiles struct {
	Gradlew    []byte
	GradlewBat []byte
	JAR        []byte
}

// Complete reports whether every wrapper file was fetched.
func (w WrapperFiles) Complete() bool {
	return len(w.Gradlew) > 0 && len(w.GradlewBat) > 0 && len(w.JAR) > 0
}

const wrapperSourceBase = "https://raw.githubusercontent.com/gradle/gradle/v%s/%s"

// FetchWrapper downloads the wrapper scripts and jar for a Gradle version.
// A partial or failed download is not fatal: the caller falls back to writing
// only gradle-wrapper.properties and telling the user to run
// `gradle wrapper` (or to let the IDE do it).
func FetchWrapper(ctx context.Context, client *Client, gradleVersion string) (WrapperFiles, error) {
	var out WrapperFiles
	targets := []struct {
		path string
		dst  *[]byte
	}{
		{"gradlew", &out.Gradlew},
		{"gradlew.bat", &out.GradlewBat},
		{"gradle/wrapper/gradle-wrapper.jar", &out.JAR},
	}
	var firstErr error
	for _, t := range targets {
		url := fmt.Sprintf(wrapperSourceBase, gradleVersion, t.path)
		body, err := client.get(ctx, url)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		*t.dst = body
	}
	if !out.Complete() {
		if firstErr == nil {
			firstErr = fmt.Errorf("wrapper files for Gradle %s were incomplete", gradleVersion)
		}
		return out, firstErr
	}
	return out, nil
}
