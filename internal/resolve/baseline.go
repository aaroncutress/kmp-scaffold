package resolve

// The baseline is the version set used when running with --offline, or for any
// individual lookup that fails. It is deliberately a snapshot of a combination
// that was known to build together, not an attempt to track "current" - the
// whole point of the resolver is that current versions come from the network.
//
// Per-library baselines live next to their version keys in package catalog.
const (
	// baselineGradle is the Gradle distribution used when the release feed is
	// unreachable.
	baselineGradle = "9.6.1"

	// baselineCompileSDK is the Android platform assumed when the SDK index is
	// unreachable and nothing was pinned.
	baselineCompileSDK = "36"

	// baselineDate describes when the baseline set was captured, so the
	// warning shown in offline mode is meaningful.
	baselineDate = "August 2026"
)
