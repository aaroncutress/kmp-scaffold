package resolve

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
)

// Repository base URLs.
const (
	googleMaven   = "https://dl.google.com/dl/android/maven2"
	centralMaven  = "https://repo1.maven.org/maven2"
	portalMaven   = "https://plugins.gradle.org/m2"
	gradleAPI     = "https://services.gradle.org/versions/current"
	androidSDKXML = "https://dl.google.com/android/repository/repository2-3.xml"
)

// Client fetches version metadata over HTTP, memoising results per run.
type Client struct {
	HTTP    *http.Client
	mu      sync.Mutex
	cache   map[string][]string
	rawMu   sync.Mutex
	rawData map[string][]byte
}

// NewClient builds a Client with sensible timeouts. The default transport is
// used, so HTTPS_PROXY and the system CA bundle are honoured.
func NewClient(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &Client{
		HTTP:    &http.Client{Timeout: timeout},
		cache:   map[string][]string{},
		rawData: map[string][]byte{},
	}
}

func repoBase(r catalog.Repo) string {
	switch r {
	case catalog.Google:
		return googleMaven
	case catalog.Portal:
		return portalMaven
	default:
		return centralMaven
	}
}

// MetadataURL builds the maven-metadata.xml URL for a coordinate.
func MetadataURL(c catalog.Coordinate) string {
	return fmt.Sprintf("%s/%s/%s/maven-metadata.xml",
		repoBase(c.Repo), strings.ReplaceAll(c.Group, ".", "/"), c.Artifact)
}

type mavenMetadata struct {
	Versioning struct {
		Latest   string   `xml:"latest"`
		Release  string   `xml:"release"`
		Versions []string `xml:"versions>version"`
	} `xml:"versioning"`
}

// Versions returns every published version of a coordinate, newest last.
func (c *Client) Versions(ctx context.Context, coord catalog.Coordinate) ([]string, error) {
	url := MetadataURL(coord)

	c.mu.Lock()
	if v, ok := c.cache[url]; ok {
		c.mu.Unlock()
		return v, nil
	}
	c.mu.Unlock()

	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	var md mavenMetadata
	if err := xml.Unmarshal(body, &md); err != nil {
		return nil, fmt.Errorf("parsing metadata for %s: %w", coord, err)
	}
	versions := md.Versioning.Versions
	if len(versions) == 0 {
		if md.Versioning.Release != "" {
			versions = []string{md.Versioning.Release}
		} else if md.Versioning.Latest != "" {
			versions = []string{md.Versioning.Latest}
		}
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no versions listed for %s", coord)
	}

	c.mu.Lock()
	c.cache[url] = versions
	c.mu.Unlock()
	return versions, nil
}

func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	c.rawMu.Lock()
	if b, ok := c.rawData[url]; ok {
		c.rawMu.Unlock()
		return b, nil
	}
	c.rawMu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "kmp-scaffold")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}

	c.rawMu.Lock()
	c.rawData[url] = body
	c.rawMu.Unlock()
	return body, nil
}

// GradleRelease describes the current Gradle distribution.
type GradleRelease struct {
	Version         string `json:"version"`
	DownloadURL     string `json:"downloadUrl"`
	Checksum        string `json:"checksum"`
	WrapperChecksum string `json:"wrapperChecksum"`
}

// DistributionURL returns the -bin.zip URL for the release.
func (g GradleRelease) DistributionURL() string {
	if g.DownloadURL != "" {
		return g.DownloadURL
	}
	return fmt.Sprintf("https://services.gradle.org/distributions/gradle-%s-bin.zip", g.Version)
}

// CurrentGradle fetches the current stable Gradle release.
func (c *Client) CurrentGradle(ctx context.Context) (GradleRelease, error) {
	body, err := c.get(ctx, gradleAPI)
	if err != nil {
		return GradleRelease{}, err
	}
	var rel GradleRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return GradleRelease{}, fmt.Errorf("parsing Gradle release feed: %w", err)
	}
	if rel.Version == "" {
		return GradleRelease{}, fmt.Errorf("Gradle release feed had no version")
	}
	return rel, nil
}

// GradleForVersion builds a GradleRelease for an explicitly requested version,
// fetching the distribution checksum so the wrapper can verify it.
func (c *Client) GradleForVersion(ctx context.Context, version string) GradleRelease {
	rel := GradleRelease{Version: version}
	url := fmt.Sprintf("https://services.gradle.org/distributions/gradle-%s-bin.zip.sha256", version)
	if body, err := c.get(ctx, url); err == nil {
		rel.Checksum = strings.TrimSpace(string(body))
	}
	return rel
}

var (
	platformRe   = regexp.MustCompile(`path="platforms;android-(\d+)"`)
	buildToolsRe = regexp.MustCompile(`path="build-tools;([0-9]+(?:\.[0-9]+)*)"`)
)

// AndroidSDK is the newest platform and build-tools discovered from Google's
// SDK repository index.
type AndroidSDK struct {
	MaxPlatformAPI int
	BuildTools     []string
}

// LatestAndroidSDK reads Google's SDK repository index. It is a large XML
// document, so only the two path patterns we care about are extracted.
func (c *Client) LatestAndroidSDK(ctx context.Context) (AndroidSDK, error) {
	body, err := c.get(ctx, androidSDKXML)
	if err != nil {
		return AndroidSDK{}, err
	}
	var out AndroidSDK
	for _, m := range platformRe.FindAllStringSubmatch(string(body), -1) {
		if n, err := strconv.Atoi(m[1]); err == nil && n > out.MaxPlatformAPI {
			out.MaxPlatformAPI = n
		}
	}
	seen := map[string]bool{}
	for _, m := range buildToolsRe.FindAllStringSubmatch(string(body), -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out.BuildTools = append(out.BuildTools, m[1])
		}
	}
	if out.MaxPlatformAPI == 0 && len(out.BuildTools) == 0 {
		return AndroidSDK{}, fmt.Errorf("no platforms or build-tools found in the SDK index")
	}
	return out, nil
}
