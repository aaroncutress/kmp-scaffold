package resolve

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
)

// Level classifies a resolution note.
type Level string

const (
	// Info is a neutral observation worth showing in the summary.
	Info Level = "info"
	// Warn means the build will probably work but something was adjusted.
	Warn Level = "warn"
	// Error means the combination is known to be broken.
	Error Level = "error"
)

// Note is a single line of the compatibility report.
type Note struct {
	Level Level
	Text  string
}

// Result is everything the templates need to know about versions.
type Result struct {
	// Versions maps a catalog version key to the chosen version.
	Versions map[string]string
	// Sources records where each version came from, for the summary screen.
	Sources map[string]string
	// Gradle is the wrapper distribution to write.
	Gradle GradleRelease
	// Notes is the compatibility report.
	Notes []Note
	// Offline is true when no network calls were made.
	Offline bool
	// Probed and Failed count the metadata lookups.
	Probed int
	Failed []string
	// Elapsed is how long resolution took.
	Elapsed time.Duration
}

// V returns the resolved version for a key, or "" if absent.
func (r *Result) V(key string) string {
	if r == nil {
		return ""
	}
	return r.Versions[key]
}

// Int returns a resolved version parsed as an integer (for SDK levels).
func (r *Result) Int(key string) int {
	n, _ := strconv.Atoi(r.V(key))
	return n
}

// Has reports whether a version key was resolved.
func (r *Result) Has(key string) bool { _, ok := r.Versions[key]; return ok }

// Note appends a line to the compatibility report. Templates use this to add
// findings of their own once the resolver has answered.
func (r *Result) Note(level Level, format string, args ...any) {
	r.Notes = append(r.Notes, Note{Level: level, Text: fmt.Sprintf(format, args...)})
}

func (r *Result) note(level Level, format string, args ...any) { r.Note(level, format, args...) }

// HasErrors reports whether any note is an error.
func (r *Result) HasErrors() bool {
	for _, n := range r.Notes {
		if n.Level == Error {
			return true
		}
	}
	return false
}

// Progress is called as metadata lookups complete.
type Progress func(done, total int, label string)

// Request describes a resolution run.
//
// It is deliberately not a project spec: a template says which version keys it
// wants and how current they should be, and the resolver answers. That keeps
// the resolver usable by any template rather than only the Kotlin Multiplatform
// one, whose compatibility rules live alongside it instead of inside it.
type Request struct {
	// Keys are catalog version keys to resolve.
	Keys []string
	// Extra are coordinates a template declares itself, for anything the
	// catalog does not already know about.
	Extra []catalog.VersionKey

	Channel  catalog.Channel
	Offline  bool
	Timeout  time.Duration
	Progress Progress
	// Overrides pin specific version keys, bypassing resolution.
	Overrides map[string]string

	// Android turns on the Android SDK passes: compileSdk, targetSdk and
	// build-tools are resolved from Google's SDK index, and minSdk is written
	// from the project setting below.
	Android    bool
	MinSDK     int
	CompileSDK int
	// GradleVer pins the Gradle distribution; empty resolves the current one.
	GradleVer string
}

// Signature is a stable fingerprint of everything that affects the outcome. The
// wizard uses it to decide whether an answer the user changed means the
// versions have to be resolved again.
func (r Request) Signature() string {
	keys := append([]string(nil), r.Keys...)
	sort.Strings(keys)

	overrides := make([]string, 0, len(r.Overrides))
	for k, v := range r.Overrides {
		overrides = append(overrides, k+"="+v)
	}
	sort.Strings(overrides)

	extra := make([]string, 0, len(r.Extra))
	for _, e := range r.Extra {
		extra = append(extra, e.Key+"@"+e.Probe.String())
	}
	sort.Strings(extra)

	return fmt.Sprintf("%s|%v|%d|%d|%s|%s|%s|%s",
		r.Channel, r.Offline, r.MinSDK, r.CompileSDK, r.GradleVer,
		strings.Join(keys, ","), strings.Join(overrides, ","), strings.Join(extra, ","))
}

// definitions indexes every version key this request can resolve: the catalog's
// plus whatever the template declared.
func (r Request) definitions() map[string]catalog.VersionKey {
	defs := map[string]catalog.VersionKey{}
	for _, k := range catalog.VersionKeys() {
		defs[k.Key] = k
	}
	for _, k := range r.Extra {
		defs[k.Key] = k
	}
	return defs
}

// keys is every key to resolve: those asked for, plus the template's own.
func (r Request) keys() []string {
	seen := map[string]bool{}
	var out []string
	for _, k := range r.Keys {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for _, k := range r.Extra {
		if !seen[k.Key] {
			seen[k.Key] = true
			out = append(out, k.Key)
		}
	}
	return out
}

// Run resolves every version key a request asks for.
func Run(ctx context.Context, req Request) *Result {
	start := time.Now()
	res := &Result{
		Versions: map[string]string{},
		Sources:  map[string]string{},
		Offline:  req.Offline,
	}

	keys := req.keys()
	defs := req.definitions()

	// Seed everything from baselines so a failed probe always leaves a usable
	// value behind.
	for _, key := range keys {
		def := defs[key]
		res.Versions[key] = def.Baseline
		res.Sources[key] = "baseline"
	}

	if req.Offline {
		res.Gradle = GradleRelease{Version: baselineGradle}
		res.note(Warn, "Offline mode: using the baseline version set from %s, not the latest releases.", baselineDate)
		applyFixed(res, req)
		applyAndroidBaseline(res, req)
		res.Elapsed = time.Since(start)
		return res
	}

	client := NewClient(req.Timeout)
	candidates := map[string][]string{}

	// Count the extra non-Maven lookups so progress is honest.
	extra := 1 // Gradle
	if req.Android {
		extra++ // Android SDK index
	}
	total := len(keys) + extra
	var done int
	var progMu sync.Mutex
	tick := func(label string) {
		progMu.Lock()
		done++
		d := done
		progMu.Unlock()
		if req.Progress != nil {
			req.Progress(d, total, label)
		}
	}

	// Probe Maven metadata concurrently.
	var mu sync.Mutex
	sem := make(chan struct{}, max(4, runtime.NumCPU()))
	var wg sync.WaitGroup
	for _, key := range keys {
		def := defs[key]
		if def.Probe.Empty() {
			tick(key)
			continue
		}
		if _, pinned := req.Overrides[key]; pinned {
			tick(key)
			continue
		}
		wg.Add(1)
		go func(key string, def catalog.VersionKey) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			versions, err := client.Versions(ctx, def.Probe)
			mu.Lock()
			res.Probed++
			if err != nil {
				res.Failed = append(res.Failed, fmt.Sprintf("%s (%s)", key, def.Probe))
			} else {
				candidates[key] = versions
			}
			mu.Unlock()
			tick(def.Probe.String())
		}(key, def)
	}
	wg.Wait()

	// Choose a version per key from its candidate list.
	for _, key := range keys {
		def := defs[key]
		list, ok := candidates[key]
		if !ok {
			continue
		}
		want := EffectiveChannel(req.Channel, def.MinChannel)
		picked, relaxed := Pick(list, want)
		if picked == "" {
			continue
		}
		res.Versions[key] = picked
		res.Sources[key] = "latest " + string(want)
		if relaxed {
			res.note(Info, "%s has no %s release; using %s.", key, want, picked)
		}
	}

	// Gradle distribution.
	if req.GradleVer != "" {
		res.Gradle = client.GradleForVersion(ctx, req.GradleVer)
		res.Sources["gradle"] = "pinned"
	} else if rel, err := client.CurrentGradle(ctx); err == nil {
		res.Gradle = rel
	} else {
		res.Gradle = GradleRelease{Version: baselineGradle}
		res.Failed = append(res.Failed, "gradle release feed")
		res.note(Warn, "Could not reach the Gradle release feed; falling back to Gradle %s.", baselineGradle)
	}
	tick("gradle")

	// Compatibility passes. Their order matters: the Android SDK pass can be
	// capped by the AGP version chosen above it, and applyFixed writes the
	// user's overrides last so they always win.
	applyKotlinKSP(res, candidates, req)
	applyAGPGradle(res, candidates, client, ctx, req)
	if req.Android {
		applyAndroidSDK(res, req, client, ctx)
		tick("android sdk")
	}
	applyFixed(res, req)

	if len(res.Failed) > 0 {
		sort.Strings(res.Failed)
		res.note(Warn, "%d lookup(s) failed and fell back to baseline versions: %s.",
			len(res.Failed), strings.Join(res.Failed, ", "))
	}

	res.Elapsed = time.Since(start)
	return res
}

// applyKotlinKSP keeps Kotlin and KSP on versions that actually work together.
//
// KSP has used two version schemes. The classic one embeds the Kotlin version
// ("2.2.20-2.0.4"), so a Kotlin release without a matching KSP build cannot be
// used. KSP2 versions its plugin independently ("2.3.10"). Both are handled:
// under the classic scheme Kotlin is stepped back until a matching KSP exists.
func applyKotlinKSP(res *Result, candidates map[string][]string, req Request) {
	kotlinList := candidates[catalog.KeyKotlin]
	kspList := candidates[catalog.KeyKSP]
	if len(kotlinList) == 0 {
		return
	}

	kotlinDef, _ := catalog.VersionKeyByName(catalog.KeyKotlin)
	want := EffectiveChannel(req.Channel, kotlinDef.MinChannel)

	if _, pinned := req.Overrides[catalog.KeyKotlin]; pinned {
		return
	}
	if len(kspList) == 0 {
		return
	}
	if _, pinned := req.Overrides[catalog.KeyKSP]; pinned {
		return
	}

	latestKSP, _ := Pick(kspList, want)
	if !usesLegacyKSPScheme(latestKSP) {
		// Independent versioning: nothing to reconcile.
		res.Versions[catalog.KeyKSP] = latestKSP
		res.Sources[catalog.KeyKSP] = "latest " + string(want)
		return
	}

	// Classic scheme: walk Kotlin candidates from newest down until one has a
	// matching KSP release.
	sorted := append([]string(nil), kotlinList...)
	sort.Slice(sorted, func(i, j int) bool {
		return ParseVersion(sorted[j]).Less(ParseVersion(sorted[i]))
	})
	chosenKotlin := res.Versions[catalog.KeyKotlin]
	for _, k := range sorted {
		if ParseVersion(k).Channel().Rank() > want.Rank() {
			continue
		}
		if ksp := HighestWithPrefix(kspList, k+"-", want); ksp != "" {
			if k != chosenKotlin {
				res.note(Warn,
					"Kotlin %s has no KSP release yet; pinned Kotlin %s, which KSP %s supports.",
					chosenKotlin, k, ksp)
			}
			res.Versions[catalog.KeyKotlin] = k
			res.Versions[catalog.KeyKSP] = ksp
			res.Sources[catalog.KeyKotlin] = "matched to KSP"
			res.Sources[catalog.KeyKSP] = "matched to Kotlin"
			return
		}
	}
	res.note(Error, "No KSP release matches any recent Kotlin version; check %s.",
		"https://github.com/google/ksp/releases")
}

// usesLegacyKSPScheme reports whether a KSP version embeds a Kotlin version,
// i.e. it has two dotted number groups separated by a dash ("2.2.20-2.0.4").
func usesLegacyKSPScheme(v string) bool {
	i := strings.Index(v, "-")
	if i < 0 {
		return false
	}
	tail := v[i+1:]
	// A KSP1-style tail is itself a dotted version like "2.0.4"; a prerelease
	// qualifier like "alpha01" or "RC2" is not.
	return strings.Count(tail, ".") >= 1 && tail[0] >= '0' && tail[0] <= '9'
}

// agpMinGradle maps an AGP major version to the Gradle version it requires.
// AGP is strict about this: too old a Gradle fails the build outright.
var agpMinGradle = map[int]string{
	7: "7.5",
	8: "8.13",
	9: "9.0",
}

// agpMaxCompileSDK caps compileSdk per AGP major. AGP refuses to build against
// a platform it does not know about.
var agpMaxCompileSDK = map[int]int{
	7: 34,
	8: 36,
	9: 37,
}

func applyAGPGradle(res *Result, candidates map[string][]string, client *Client, ctx context.Context, req Request) {
	agp := ParseVersion(res.V(catalog.KeyAGP))
	if agp.Raw == "" {
		return
	}
	minGradle, known := agpMinGradle[agp.Major()]
	if !known {
		return
	}
	have := ParseVersion(res.Gradle.Version)
	if have.AtLeast(ParseVersion(minGradle)) {
		return
	}
	// Only reachable when the user pinned an older Gradle.
	res.note(Error, "Android Gradle Plugin %s needs Gradle %s or newer, but Gradle %s is pinned.",
		agp.Raw, minGradle, res.Gradle.Version)
}

func applyAndroidSDK(res *Result, req Request, client *Client, ctx context.Context) {
	agp := ParseVersion(res.V(catalog.KeyAGP))
	cap, capped := agpMaxCompileSDK[agp.Major()]

	compile := 0
	buildTools := ""

	if sdk, err := client.LatestAndroidSDK(ctx); err == nil {
		compile = sdk.MaxPlatformAPI
		// Build-tools are published slightly ahead of the platform package in
		// the stable index, and their major version tracks the API level, so
		// take whichever is higher.
		best := ""
		for _, bt := range sdk.BuildTools {
			v := ParseVersion(bt)
			if v.Major() > compile {
				compile = v.Major()
			}
			if best == "" || ParseVersion(best).Less(v) {
				best = bt
			}
		}
		buildTools = best
		res.Sources[catalog.KeyCompileSDK] = "Android SDK index"
	} else {
		res.Failed = append(res.Failed, "android sdk index")
		compile, _ = strconv.Atoi(res.V(catalog.KeyCompileSDK))
	}

	if capped && compile > cap {
		res.note(Info, "Android SDK %d is available, but AGP %s supports at most compileSdk %d - using %d.",
			compile, agp.Raw, cap, cap)
		compile = cap
	}
	if req.CompileSDK > 0 {
		if req.CompileSDK != compile {
			res.note(Info, "compileSdk pinned to %d by request (resolver suggested %d).", req.CompileSDK, compile)
		}
		compile = req.CompileSDK
		res.Sources[catalog.KeyCompileSDK] = "pinned"
	}
	if compile <= 0 {
		compile, _ = strconv.Atoi(baselineCompileSDK)
	}

	// Pick the newest build-tools whose major matches compileSdk.
	chosenBT := ""
	if buildTools != "" {
		if ParseVersion(buildTools).Major() == compile {
			chosenBT = buildTools
		}
	}
	if chosenBT == "" {
		chosenBT = fmt.Sprintf("%d.0.0", compile)
	}

	res.Versions[catalog.KeyCompileSDK] = strconv.Itoa(compile)
	res.Versions[catalog.KeyTargetSDK] = strconv.Itoa(compile)
	res.Versions[catalog.KeyBuildTools] = chosenBT
	res.Sources[catalog.KeyTargetSDK] = res.Sources[catalog.KeyCompileSDK]
	res.Sources[catalog.KeyBuildTools] = res.Sources[catalog.KeyCompileSDK]
}

func applyAndroidBaseline(res *Result, req Request) {
	if !req.Android {
		return
	}
	compile := req.CompileSDK
	if compile <= 0 {
		compile, _ = strconv.Atoi(baselineCompileSDK)
	}
	res.Versions[catalog.KeyCompileSDK] = strconv.Itoa(compile)
	res.Versions[catalog.KeyTargetSDK] = strconv.Itoa(compile)
	res.Versions[catalog.KeyBuildTools] = fmt.Sprintf("%d.0.0", compile)
}

// applyFixed writes the values that never come from a repository, and applies
// explicit user overrides last so they always win.
func applyFixed(res *Result, req Request) {
	defs := req.definitions()

	if req.Android {
		minSDK := req.MinSDK
		if minSDK <= 0 {
			minSDK = 26
		}
		res.Versions[catalog.KeyMinSDK] = strconv.Itoa(minSDK)
		res.Sources[catalog.KeyMinSDK] = "project setting"
	}
	res.Versions[catalog.KeyVersionCode] = "1"
	res.Versions[catalog.KeyVersionName] = "1.0"
	res.Sources[catalog.KeyVersionCode] = "project setting"
	res.Sources[catalog.KeyVersionName] = "project setting"

	for key, value := range req.Overrides {
		if value == "" {
			continue
		}
		if _, known := defs[key]; !known {
			res.note(Warn, "Ignoring override for unknown version key %q.", key)
			continue
		}
		res.Versions[key] = value
		res.Sources[key] = "pinned"
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
