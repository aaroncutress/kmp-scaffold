// Package catalog describes every library, plugin, bundle and version key the
// generator knows about, plus the user-facing groupings ("packs", shared-logic
// utilities, Android extras) that the wizard presents.
//
// Nothing here contains a hard-coded "current" version: each version key
// carries a coordinate to probe for the latest release and a baseline that is
// only used when resolution is skipped or fails. See package resolve.
package catalog

import (
	"sort"
	"strings"

	"github.com/aaroncutress/kmp-scaffold/internal/model"
)

// Repo identifies where an artifact is published.
type Repo string

const (
	// Google is Google's Maven repository (AndroidX, AGP, Play services).
	Google Repo = "google"
	// Central is Maven Central.
	Central Repo = "central"
	// Portal is the Gradle Plugin Portal.
	Portal Repo = "portal"
)

// Channel is how adventurous the user wants version resolution to be.
type Channel string

const (
	// Stable accepts only final releases.
	Stable Channel = "stable"
	// Preview additionally accepts -rc and -beta releases.
	Preview Channel = "preview"
	// Bleeding additionally accepts -alpha and -dev releases.
	Bleeding Channel = "bleeding"
)

// Rank orders channels from least to most adventurous.
func (c Channel) Rank() int {
	switch c {
	case Bleeding:
		return 2
	case Preview:
		return 1
	default:
		return 0
	}
}

// ParseChannel maps a user-supplied string onto a Channel, defaulting to Preview.
func ParseChannel(s string) Channel {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "stable":
		return Stable
	case "bleeding", "alpha", "edge":
		return Bleeding
	default:
		return Preview
	}
}

// Predicate decides whether a catalog item applies to a given project.
type Predicate func(model.Spec) bool

// Always is a Predicate that is true for every project.
func Always(model.Spec) bool { return true }

// Pack returns a Predicate true when the given library pack is selected.
func Pack(id string) Predicate {
	return func(s model.Spec) bool { return s.HasPack(id) }
}

// AnyPack returns a Predicate true when any of the packs is selected.
func AnyPack(ids ...string) Predicate {
	return func(s model.Spec) bool {
		for _, id := range ids {
			if s.HasPack(id) {
				return true
			}
		}
		return false
	}
}

// Util returns a Predicate true when the given shared utility is enabled.
func Util(id string) Predicate {
	return func(s model.Spec) bool { return s.HasSharedUtil(id) }
}

// Extra returns a Predicate true when the given Android extra is enabled.
func Extra(id string) Predicate {
	return func(s model.Spec) bool { return s.HasAndroidExtra(id) }
}

// IOSOnly is true when the project has an iOS target.
func IOSOnly(s model.Spec) bool { return s.IOS }

// AndroidOnly is true when the project has an Android target.
func AndroidOnly(s model.Spec) bool { return s.Android }

// And combines predicates conjunctively.
func And(ps ...Predicate) Predicate {
	return func(s model.Spec) bool {
		for _, p := range ps {
			if !p(s) {
				return false
			}
		}
		return true
	}
}

// ---------------------------------------------------------------------------
// Version keys
// ---------------------------------------------------------------------------

// VersionKey is one entry in the `[versions]` block of libs.versions.toml.
type VersionKey struct {
	// Key is the TOML key, e.g. "androidx-lifecycle".
	Key string
	// Section groups keys under a comment header in the generated file.
	Section string
	// Probe is the coordinate whose maven-metadata.xml is queried to find the
	// newest version. Empty for keys computed by the resolver (agp, kotlin,
	// android-compileSdk, ...) or fixed by the user.
	Probe Coordinate
	// MinChannel forces at least this channel for this key, for libraries that
	// have never had a stable release (navigation3, compose adaptive, ...).
	// The effective channel is max(user channel, MinChannel).
	MinChannel Channel
	// Baseline is used when running offline or when a probe fails. These are
	// known-good values, not "current" values.
	Baseline string
	// Managed marks keys that the resolver computes rather than probes.
	Managed bool
}

// Coordinate is a Maven group:artifact in a specific repository.
type Coordinate struct {
	Group    string
	Artifact string
	Repo     Repo
}

// Empty reports whether the coordinate is unset.
func (c Coordinate) Empty() bool { return c.Group == "" || c.Artifact == "" }

// String renders the coordinate as group:artifact.
func (c Coordinate) String() string { return c.Group + ":" + c.Artifact }

// Version key names used across the catalog and the resolver.
const (
	KeyAGP           = "agp"
	KeyKotlin        = "kotlin"
	KeyKSP           = "ksp"
	KeySkie          = "skie"
	KeyBuildKonfig   = "buildkonfig"
	KeyCompileSDK    = "android-compileSdk"
	KeyMinSDK        = "android-minSdk"
	KeyTargetSDK     = "android-targetSdk"
	KeyBuildTools    = "android-buildTools"
	KeyVersionCode   = "app-versionCode"
	KeyVersionName   = "app-versionName"
	KeyComposeCore   = "androidx-compose-core"
	KeyComposeM3     = "androidx-compose-material3"
	KeyComposeAdapt  = "androidx-compose-adaptive"
	KeyActivity      = "androidx-activity"
	KeyAppCompat     = "androidx-appcompat"
	KeyBrowser       = "androidx-browser"
	KeyCore          = "androidx-core"
	KeyCredentials   = "androidx-credentials"
	KeySplash        = "androidx-core-splashscreen"
	KeyLifecycle     = "androidx-lifecycle"
	KeyMedia3        = "androidx-media3"
	KeyNavigation3   = "androidx-navigation3"
	KeyPaging        = "androidx-paging"
	KeyRoom          = "androidx-room"
	KeySQLite        = "sqlite"
	KeyKoin          = "koin"
	KeyKoinPlugin    = "koin-plugin"
	KeyKable         = "kable"
	KeyKtor          = "ktor"
	KeySerialization = "kotlinx-serialization"
	KeyCoroutines    = "kotlinx-coroutines"
	KeyDateTime      = "kotlinx-datetime"
	KeyLogger        = "logger"
	KeySettings      = "settings"
	KeyObservableVM  = "observableviewmodel"
	KeyCoil          = "coil"
	KeyPlayLocation  = "play-services-location"
	KeyMaps          = "maps-compose"
	KeyIcons         = "composables-icons"
	KeyVico          = "vico"
	KeySupabase      = "supabase"
	KeyEspresso      = "androidx-espresso"
	KeyTestExt       = "androidx-testExt"
	KeyJUnit         = "junit"
	KeyTurbine       = "turbine"
)

// Section names, in the order they appear in the generated catalog.
var sectionOrder = []string{
	"Core Tooling & Build System",
	"Android Configuration",
	"App Versioning",
	"Jetpack Compose",
	"Jetpack Libraries",
	"Dependency Injection",
	"Network & Multiplatform Utilities",
	"External Integrations",
	"Testing",
}

// VersionKeys returns every version key the catalog knows about, in file order.
func VersionKeys() []VersionKey {
	return []VersionKey{
		// Core Tooling & Build System
		{Key: KeyAGP, Section: "Core Tooling & Build System", Managed: true,
			Probe:    Coordinate{"com.android.tools.build", "gradle", Google},
			Baseline: "9.2.1"},
		{Key: KeyKotlin, Section: "Core Tooling & Build System", Managed: true,
			Probe:    Coordinate{"org.jetbrains.kotlin", "kotlin-gradle-plugin", Central},
			Baseline: "2.4.10"},
		{Key: KeyKSP, Section: "Core Tooling & Build System", Managed: true,
			Probe:    Coordinate{"com.google.devtools.ksp", "symbol-processing-gradle-plugin", Central},
			Baseline: "2.3.9"},
		{Key: KeySkie, Section: "Core Tooling & Build System",
			Probe:    Coordinate{"co.touchlab.skie", "co.touchlab.skie.gradle.plugin", Central},
			Baseline: "0.10.14"},
		{Key: KeyBuildKonfig, Section: "Core Tooling & Build System",
			Probe:    Coordinate{"com.codingfeline.buildkonfig", "buildkonfig-gradle-plugin", Central},
			Baseline: "0.22.0"},

		// Android Configuration (all resolver-managed)
		{Key: KeyCompileSDK, Section: "Android Configuration", Managed: true, Baseline: "36"},
		{Key: KeyMinSDK, Section: "Android Configuration", Managed: true, Baseline: "26"},
		{Key: KeyTargetSDK, Section: "Android Configuration", Managed: true, Baseline: "36"},
		{Key: KeyBuildTools, Section: "Android Configuration", Managed: true, Baseline: "36.0.0"},

		// App Versioning
		{Key: KeyVersionCode, Section: "App Versioning", Managed: true, Baseline: "1"},
		{Key: KeyVersionName, Section: "App Versioning", Managed: true, Baseline: "1.0"},

		// Jetpack Compose
		{Key: KeyComposeCore, Section: "Jetpack Compose",
			Probe:    Coordinate{"androidx.compose.ui", "ui", Google},
			Baseline: "1.9.4"},
		{Key: KeyComposeM3, Section: "Jetpack Compose",
			Probe: Coordinate{"androidx.compose.material3", "material3", Google},
			// Material 3 expressive APIs the nav shell uses only ship in alphas.
			MinChannel: Bleeding, Baseline: "1.5.0-alpha25"},
		{Key: KeyComposeAdapt, Section: "Jetpack Compose",
			Probe: Coordinate{"androidx.compose.material3.adaptive", "adaptive", Google},
			// currentWindowAdaptiveInfoV2 + adaptive-navigation3 are preview-only.
			MinChannel: Preview, Baseline: "1.3.0-rc01"},

		// Jetpack Libraries
		{Key: KeyActivity, Section: "Jetpack Libraries",
			Probe: Coordinate{"androidx.activity", "activity-compose", Google}, Baseline: "1.13.0"},
		{Key: KeyAppCompat, Section: "Jetpack Libraries",
			Probe: Coordinate{"androidx.appcompat", "appcompat", Google}, Baseline: "1.7.1"},
		{Key: KeyBrowser, Section: "Jetpack Libraries",
			Probe: Coordinate{"androidx.browser", "browser", Google}, Baseline: "1.10.0"},
		{Key: KeyCore, Section: "Jetpack Libraries",
			Probe: Coordinate{"androidx.core", "core-ktx", Google}, Baseline: "1.19.0"},
		{Key: KeyCredentials, Section: "Jetpack Libraries",
			Probe: Coordinate{"androidx.credentials", "credentials", Google}, Baseline: "1.6.0"},
		{Key: KeySplash, Section: "Jetpack Libraries",
			Probe: Coordinate{"androidx.core", "core-splashscreen", Google}, Baseline: "1.2.0"},
		{Key: KeyLifecycle, Section: "Jetpack Libraries",
			Probe: Coordinate{"androidx.lifecycle", "lifecycle-viewmodel", Google}, Baseline: "2.11.0"},
		{Key: KeyMedia3, Section: "Jetpack Libraries",
			Probe: Coordinate{"androidx.media3", "media3-exoplayer", Google}, Baseline: "1.10.1"},
		{Key: KeyNavigation3, Section: "Jetpack Libraries",
			Probe: Coordinate{"androidx.navigation3", "navigation3-runtime", Google},
			// Navigation 3 has no stable release yet.
			MinChannel: Bleeding, Baseline: "1.2.0-alpha07"},
		{Key: KeyPaging, Section: "Jetpack Libraries",
			Probe: Coordinate{"androidx.paging", "paging-common", Google}, Baseline: "3.5.0"},
		{Key: KeyRoom, Section: "Jetpack Libraries",
			Probe: Coordinate{"androidx.room", "room-runtime", Google}, Baseline: "2.8.4"},
		{Key: KeySQLite, Section: "Jetpack Libraries",
			Probe: Coordinate{"androidx.sqlite", "sqlite-bundled", Google}, Baseline: "2.7.0"},

		// Dependency Injection
		{Key: KeyKoin, Section: "Dependency Injection",
			Probe: Coordinate{"io.insert-koin", "koin-core", Central}, Baseline: "4.2.2"},
		{Key: KeyKoinPlugin, Section: "Dependency Injection",
			Probe:    Coordinate{"io.insert-koin.compiler.plugin", "io.insert-koin.compiler.plugin.gradle.plugin", Portal},
			Baseline: "1.1.0"},

		// Network & Multiplatform Utilities
		{Key: KeyKable, Section: "Network & Multiplatform Utilities",
			Probe: Coordinate{"com.juul.kable", "kable-core", Central}, Baseline: "0.44.3"},
		{Key: KeyKtor, Section: "Network & Multiplatform Utilities",
			Probe: Coordinate{"io.ktor", "ktor-client-core", Central}, Baseline: "3.5.2"},
		{Key: KeySerialization, Section: "Network & Multiplatform Utilities",
			Probe: Coordinate{"org.jetbrains.kotlinx", "kotlinx-serialization-json", Central}, Baseline: "1.11.0"},
		{Key: KeyCoroutines, Section: "Network & Multiplatform Utilities",
			Probe: Coordinate{"org.jetbrains.kotlinx", "kotlinx-coroutines-core", Central}, Baseline: "1.11.0"},
		{Key: KeyDateTime, Section: "Network & Multiplatform Utilities",
			Probe: Coordinate{"org.jetbrains.kotlinx", "kotlinx-datetime", Central}, Baseline: "0.8.0"},
		{Key: KeyLogger, Section: "Network & Multiplatform Utilities",
			Probe: Coordinate{"io.github.shivathapaa", "logger", Central}, Baseline: "2.0.0"},
		{Key: KeySettings, Section: "Network & Multiplatform Utilities",
			Probe: Coordinate{"com.russhwolf", "multiplatform-settings", Central}, Baseline: "1.3.0"},
		{Key: KeyObservableVM, Section: "Network & Multiplatform Utilities",
			Probe:    Coordinate{"com.rickclephas.kmp", "kmp-observableviewmodel-core", Central},
			Baseline: "1.0.6"},
		{Key: KeyCoil, Section: "Network & Multiplatform Utilities",
			Probe: Coordinate{"io.coil-kt.coil3", "coil-compose", Central}, Baseline: "3.5.0"},

		// External Integrations
		{Key: KeyPlayLocation, Section: "External Integrations",
			Probe: Coordinate{"com.google.android.gms", "play-services-location", Google}, Baseline: "21.4.0"},
		{Key: KeyMaps, Section: "External Integrations",
			Probe: Coordinate{"com.google.maps.android", "maps-compose", Central}, Baseline: "8.4.0"},
		{Key: KeyIcons, Section: "External Integrations",
			Probe:    Coordinate{"com.composables", "icons-material-symbols-rounded-cmp", Central},
			Baseline: "2.2.1"},
		{Key: KeyVico, Section: "External Integrations",
			Probe: Coordinate{"com.patrykandpatrick.vico", "compose", Central}, Baseline: "3.2.3"},
		{Key: KeySupabase, Section: "External Integrations",
			Probe: Coordinate{"io.github.jan-tennert.supabase", "bom", Central}, Baseline: "3.7.0"},

		// Testing
		{Key: KeyEspresso, Section: "Testing",
			Probe: Coordinate{"androidx.test.espresso", "espresso-core", Google}, Baseline: "3.7.0"},
		{Key: KeyTestExt, Section: "Testing",
			Probe: Coordinate{"androidx.test.ext", "junit", Google}, Baseline: "1.3.0"},
		{Key: KeyJUnit, Section: "Testing",
			Probe: Coordinate{"junit", "junit", Central}, Baseline: "4.13.2"},
		{Key: KeyTurbine, Section: "Testing",
			Probe: Coordinate{"app.cash.turbine", "turbine", Central}, Baseline: "1.2.1"},
	}
}

// VersionKeyByName looks up a key definition.
func VersionKeyByName(name string) (VersionKey, bool) {
	for _, k := range VersionKeys() {
		if k.Key == name {
			return k, true
		}
	}
	return VersionKey{}, false
}

// ---------------------------------------------------------------------------
// Library entries
// ---------------------------------------------------------------------------

// Library is one entry in the `[libraries]` block.
type Library struct {
	Alias   string    // "androidx-lifecycle-viewmodel"
	Module  string    // "androidx.lifecycle:lifecycle-viewmodel"
	Version string    // version key, or "" when a BOM supplies it
	Section string    // comment header in the generated file
	When    Predicate // whether this project includes it
}

// Accessor converts a catalog alias into its type-safe Gradle accessor:
// "androidx-compose-ui-tooling" -> "libs.androidx.compose.ui.tooling".
func Accessor(alias string) string {
	return "libs." + strings.ReplaceAll(alias, "-", ".")
}

// Libraries returns every library entry, in file order.
func Libraries() []Library {
	core := Always
	return []Library{
		// Build System (consumed by buildSrc, not by modules)
		{"android-gradlePlugin", "com.android.tools.build:gradle", KeyAGP, "Build System", core},
		{"kotlin-gradlePlugin", "org.jetbrains.kotlin:kotlin-gradle-plugin", KeyKotlin, "Build System", core},
		{"compose-compiler-gradlePlugin", "org.jetbrains.kotlin:compose-compiler-gradle-plugin", KeyKotlin, "Build System", AndroidOnly},

		// AndroidX & Jetpack
		{"androidx-activity-compose", "androidx.activity:activity-compose", KeyActivity, "AndroidX & Jetpack", AndroidOnly},
		{"androidx-appcompat", "androidx.appcompat:appcompat", KeyAppCompat, "AndroidX & Jetpack", Pack("appcompat")},
		{"androidx-browser", "androidx.browser:browser", KeyBrowser, "AndroidX & Jetpack", Pack("browser")},
		{"androidx-core-ktx", "androidx.core:core-ktx", KeyCore, "AndroidX & Jetpack", AndroidOnly},
		{"androidx-core-splashscreen", "androidx.core:core-splashscreen", KeySplash, "AndroidX & Jetpack", And(AndroidOnly, Extra("splash-screen"))},
		{"androidx-credentials", "androidx.credentials:credentials", KeyCredentials, "AndroidX & Jetpack", Pack("passkeys")},
		{"androidx-credentials-play-services-auth", "androidx.credentials:credentials-play-services-auth", KeyCredentials, "AndroidX & Jetpack", Pack("passkeys")},
		{"androidx-lifecycle-runtime-compose", "androidx.lifecycle:lifecycle-runtime-compose", KeyLifecycle, "AndroidX & Jetpack", AndroidOnly},
		{"androidx-lifecycle-viewmodel", "androidx.lifecycle:lifecycle-viewmodel", KeyLifecycle, "AndroidX & Jetpack", core},
		{"androidx-lifecycle-viewmodel-compose", "androidx.lifecycle:lifecycle-viewmodel-compose", KeyLifecycle, "AndroidX & Jetpack", AndroidOnly},
		{"androidx-lifecycle-viewmodel-navigation3", "androidx.lifecycle:lifecycle-viewmodel-navigation3", KeyLifecycle, "AndroidX & Jetpack", AndroidOnly},

		// Media playback
		{"androidx-media3-exoplayer", "androidx.media3:media3-exoplayer", KeyMedia3, "Media playback (Media3 / ExoPlayer)", Pack("media3")},
		{"androidx-media3-ui-compose", "androidx.media3:media3-ui-compose", KeyMedia3, "Media playback (Media3 / ExoPlayer)", Pack("media3")},
		{"androidx-media3-ui-compose-material3", "androidx.media3:media3-ui-compose-material3", KeyMedia3, "Media playback (Media3 / ExoPlayer)", Pack("media3")},

		// Paging
		{"androidx-paging-common", "androidx.paging:paging-common", KeyPaging, "Paging", Pack("paging")},
		{"androidx-paging-compose", "androidx.paging:paging-compose", KeyPaging, "Paging", And(AndroidOnly, Pack("paging"))},

		// Compose
		{"androidx-compose-foundation", "androidx.compose.foundation:foundation", KeyComposeCore, "Compose", AndroidOnly},
		{"androidx-compose-material3", "androidx.compose.material3:material3", KeyComposeM3, "Compose", AndroidOnly},
		{"androidx-compose-runtime", "androidx.compose.runtime:runtime", KeyComposeCore, "Compose", AndroidOnly},
		{"androidx-compose-runtime-saveable", "androidx.compose.runtime:runtime-saveable", KeyComposeCore, "Compose", AndroidOnly},
		{"androidx-compose-ui", "androidx.compose.ui:ui", KeyComposeCore, "Compose", AndroidOnly},
		{"androidx-compose-ui-tooling", "androidx.compose.ui:ui-tooling", KeyComposeCore, "Compose", AndroidOnly},
		{"androidx-compose-ui-tooling-preview", "androidx.compose.ui:ui-tooling-preview", KeyComposeCore, "Compose", AndroidOnly},

		// Adaptive UI
		{"androidx-compose-material3-adaptive", "androidx.compose.material3.adaptive:adaptive", KeyComposeAdapt, "Adaptive UI", AndroidOnly},
		{"androidx-compose-material3-adaptive-layout", "androidx.compose.material3.adaptive:adaptive-layout", KeyComposeAdapt, "Adaptive UI", AndroidOnly},
		{"androidx-compose-material3-adaptive-navigation3", "androidx.compose.material3.adaptive:adaptive-navigation3", KeyComposeAdapt, "Adaptive UI", AndroidOnly},

		// Navigation 3
		{"androidx-navigation3-runtime", "androidx.navigation3:navigation3-runtime", KeyNavigation3, "Navigation 3", AndroidOnly},
		{"androidx-navigation3-ui", "androidx.navigation3:navigation3-ui", KeyNavigation3, "Navigation 3", AndroidOnly},

		// Persistence
		{"androidx-room-compiler", "androidx.room:room-compiler", KeyRoom, "Persistence (Room)", Pack("database")},
		{"androidx-room-runtime", "androidx.room:room-runtime", KeyRoom, "Persistence (Room)", Pack("database")},
		{"androidx-room-paging", "androidx.room:room-paging", KeyRoom, "Persistence (Room)", And(Pack("database"), Pack("paging"))},
		{"androidx-sqlite-bundled", "androidx.sqlite:sqlite-bundled", KeySQLite, "Persistence (Room)", Pack("database")},

		// Dependency Injection
		{"koin-android", "io.insert-koin:koin-android", KeyKoin, "Dependency Injection (Koin)", AndroidOnly},
		{"koin-androidx-compose", "io.insert-koin:koin-androidx-compose", KeyKoin, "Dependency Injection (Koin)", AndroidOnly},
		{"koin-core", "io.insert-koin:koin-core", KeyKoin, "Dependency Injection (Koin)", core},
		{"koin-core-viewmodel", "io.insert-koin:koin-core-viewmodel", KeyKoin, "Dependency Injection (Koin)", core},
		{"koin-test", "io.insert-koin:koin-test", KeyKoin, "Dependency Injection (Koin)", core},

		// Networking & Serialization
		{"ktor-client-android", "io.ktor:ktor-client-android", KeyKtor, "Networking & Serialization", func(model.Spec) bool { return false }},
		{"ktor-client-okhttp", "io.ktor:ktor-client-okhttp", KeyKtor, "Networking & Serialization", AndroidOnly},
		{"ktor-client-content-negotiation", "io.ktor:ktor-client-content-negotiation", KeyKtor, "Networking & Serialization", core},
		{"ktor-client-core", "io.ktor:ktor-client-core", KeyKtor, "Networking & Serialization", core},
		{"ktor-client-darwin", "io.ktor:ktor-client-darwin", KeyKtor, "Networking & Serialization", IOSOnly},
		{"ktor-serialization-kotlinx-json", "io.ktor:ktor-serialization-kotlinx-json", KeyKtor, "Networking & Serialization", core},
		{"kotlinx-datetime", "org.jetbrains.kotlinx:kotlinx-datetime", KeyDateTime, "Networking & Serialization", core},
		{"kotlinx-serialization-json", "org.jetbrains.kotlinx:kotlinx-serialization-json", KeySerialization, "Networking & Serialization", core},
		{"kotlinx-coroutines-play-services", "org.jetbrains.kotlinx:kotlinx-coroutines-play-services", KeyCoroutines, "Networking & Serialization", Pack("location")},

		// External Libraries
		{"kable", "com.juul.kable:kable-core", KeyKable, "External Libraries", Pack("ble")},
		{"logger", "io.github.shivathapaa:logger", KeyLogger, "External Libraries", Pack("logger")},
		{"settings", "com.russhwolf:multiplatform-settings", KeySettings, "External Libraries", Util("settings")},
		{"observableviewmodel-core", "com.rickclephas.kmp:kmp-observableviewmodel-core", KeyObservableVM, "External Libraries", core},
		{"coil-compose", "io.coil-kt.coil3:coil-compose", KeyCoil, "External Libraries", Pack("images")},
		{"coil-network-ktor", "io.coil-kt.coil3:coil-network-ktor3", KeyCoil, "External Libraries", Pack("images")},
		{"play-services-location", "com.google.android.gms:play-services-location", KeyPlayLocation, "External Libraries", Pack("location")},

		// UI Components & Icons
		{"composables-material-symbols-rounded", "com.composables:icons-material-symbols-rounded-cmp", KeyIcons, "UI Components & Icons", AndroidOnly},
		{"composables-material-symbols-rounded-filled", "com.composables:icons-material-symbols-rounded-filled-cmp", KeyIcons, "UI Components & Icons", AndroidOnly},
		{"maps-compose", "com.google.maps.android:maps-compose", KeyMaps, "UI Components & Icons", Pack("maps")},
		{"vico-compose", "com.patrykandpatrick.vico:compose", KeyVico, "UI Components & Icons", Pack("charts")},
		{"vico-compose-m3", "com.patrykandpatrick.vico:compose-m3", KeyVico, "UI Components & Icons", Pack("charts")},

		// Supabase (BOM-managed: no version on the individual artifacts)
		{"supabase-bom", "io.github.jan-tennert.supabase:bom", KeySupabase, "Supabase", Pack("supabase")},
		{"supabase-auth", "io.github.jan-tennert.supabase:auth-kt", "", "Supabase", Pack("supabase")},
		{"supabase-functions", "io.github.jan-tennert.supabase:functions-kt", "", "Supabase", Pack("supabase")},
		{"supabase-postgrest", "io.github.jan-tennert.supabase:postgrest-kt", "", "Supabase", Pack("supabase")},
		{"supabase-storage", "io.github.jan-tennert.supabase:storage-kt", "", "Supabase", Pack("supabase")},

		// Testing
		{"androidx-espresso-core", "androidx.test.espresso:espresso-core", KeyEspresso, "Testing", AndroidOnly},
		{"androidx-testExt-junit", "androidx.test.ext:junit", KeyTestExt, "Testing", AndroidOnly},
		{"junit", "junit:junit", KeyJUnit, "Testing", AndroidOnly},
		{"kotlin-test", "org.jetbrains.kotlin:kotlin-test", KeyKotlin, "Testing", core},
		{"kotlin-testJunit", "org.jetbrains.kotlin:kotlin-test-junit", KeyKotlin, "Testing", AndroidOnly},
		{"kotlinx-coroutines-test", "org.jetbrains.kotlinx:kotlinx-coroutines-test", KeyCoroutines, "Testing", core},
		{"turbine", "app.cash.turbine:turbine", KeyTurbine, "Testing", core},
		{"ktor-client-mock", "io.ktor:ktor-client-mock", KeyKtor, "Testing", core},
	}
}

// ---------------------------------------------------------------------------
// Bundles
// ---------------------------------------------------------------------------

// Bundle is one entry in the `[bundles]` block.
type Bundle struct {
	Name string
	Libs []string
	When Predicate
}

// Bundles returns every bundle definition, in file order.
func Bundles() []Bundle {
	return []Bundle{
		{"compose", []string{
			"androidx-compose-runtime",
			"androidx-compose-ui",
			"androidx-compose-foundation",
			"androidx-compose-material3",
			"androidx-compose-ui-tooling-preview",
		}, AndroidOnly},
		{"navigation", []string{
			"androidx-navigation3-runtime",
			"androidx-navigation3-ui",
			"androidx-lifecycle-viewmodel-navigation3",
		}, AndroidOnly},
		{"adaptive", []string{
			"androidx-compose-material3-adaptive",
			"androidx-compose-material3-adaptive-layout",
			"androidx-compose-material3-adaptive-navigation3",
		}, AndroidOnly},
		{"koin", []string{
			"koin-androidx-compose",
			"observableviewmodel-core",
		}, AndroidOnly},
		{"ui-extras", []string{
			"composables-material-symbols-rounded",
			"composables-material-symbols-rounded-filled",
		}, AndroidOnly},
		{"media3", []string{
			"androidx-media3-exoplayer",
			"androidx-media3-ui-compose",
			"androidx-media3-ui-compose-material3",
		}, Pack("media3")},
		{"utils", []string{"kotlinx-datetime"}, AndroidOnly},
	}
}

// ---------------------------------------------------------------------------
// Plugins
// ---------------------------------------------------------------------------

// Plugin is one entry in the `[plugins]` block.
type Plugin struct {
	Alias string
	ID    string
	// Version is the version key, or "" when the version comes from the
	// buildSrc classpath (Kotlin/AGP plugins pinned there).
	Version string
	When    Predicate
}

// Plugins returns every plugin entry, in file order.
func Plugins() []Plugin {
	return []Plugin{
		{"androidApplication", "com.android.application", "", AndroidOnly},
		{"androidLibrary", "com.android.library", "", AndroidOnly},
		{"androidMultiplatformLibrary", "com.android.kotlin.multiplatform.library", "", Always},
		{"androidx-room", "androidx.room", KeyRoom, Pack("database")},
		{"composeCompiler", "org.jetbrains.kotlin.plugin.compose", "", AndroidOnly},
		{"koin-compiler", "io.insert-koin.compiler.plugin", KeyKoinPlugin, Pack("koin-compiler")},
		{"kotlinMultiplatform", "org.jetbrains.kotlin.multiplatform", "", Always},
		{"kotlin-parcelize", "org.jetbrains.kotlin.plugin.parcelize", "", AndroidOnly},
		{"kotlin-serialization", "org.jetbrains.kotlin.plugin.serialization", KeyKotlin, Always},
		{"ksp", "com.google.devtools.ksp", KeyKSP, AnyPack("database", "koin-compiler")},
		{"skie", "co.touchlab.skie", KeySkie, And(IOSOnly, Pack("skie"))},
		{"buildkonfig", "com.codingfeline.buildkonfig", KeyBuildKonfig, Pack("secrets")},
	}
}

// ---------------------------------------------------------------------------
// User-facing groupings
// ---------------------------------------------------------------------------

// Tier controls how a pack is presented and defaulted in the wizard.
type Tier int

const (
	// TierCore packs are always installed and are not offered as choices.
	TierCore Tier = iota
	// TierBasic packs are pre-ticked in the "basic" preset.
	TierBasic
	// TierExtra packs are off by default and must be opted into.
	TierExtra
)

// PackDef is a user-selectable group of libraries.
type PackDef struct {
	ID          string
	Label       string
	Description string
	Tier        Tier
	// RequiresIOS/RequiresAndroid hide the pack when the target is absent.
	RequiresIOS     bool
	RequiresAndroid bool
	// Implies lists packs that are switched on with this one.
	Implies []string
}

// Packs returns every selectable library pack, in wizard order.
func Packs() []PackDef {
	return []PackDef{
		{ID: "images", Label: "Image loading (Coil 3)",
			Description: "Coil + its Ktor network fetcher, wired to the shared HttpClient.",
			Tier:        TierBasic},
		{ID: "database", Label: "Local database (Room KMP)",
			Description: "Room runtime, bundled SQLite, KSP processors and a schema directory.",
			Tier:        TierBasic},
		{ID: "secrets", Label: "Secrets (BuildKonfig)",
			Description: "secrets.properties -> BuildKonfig fields + Android manifest placeholders.",
			Tier:        TierBasic},
		{ID: "logger", Label: "Multiplatform logging",
			Description: "Shared Log facade usable from commonMain.",
			Tier:        TierBasic},
		{ID: "skie", Label: "SKIE (nicer Swift API)",
			Description: "Maps sealed classes, flows and suspend functions into idiomatic Swift.",
			Tier:        TierBasic, RequiresIOS: true},
		{ID: "paging", Label: "Paging 3",
			Description: "androidx.paging common + compose artifacts, and Room paging support.",
			Tier:        TierExtra},
		{ID: "media3", Label: "Media playback (Media3)",
			Description: "ExoPlayer and its Compose UI artifacts.",
			Tier:        TierExtra},
		{ID: "maps", Label: "Google Maps Compose",
			Description: "maps-compose plus the manifest API-key placeholder.",
			Tier:        TierExtra, RequiresAndroid: true},
		{ID: "location", Label: "Location services",
			Description: "play-services-location and the coroutines interop artifact.",
			Tier:        TierExtra, RequiresAndroid: true},
		{ID: "charts", Label: "Charts (Vico)",
			Description: "Vico Compose + Material 3 chart theming.",
			Tier:        TierExtra, RequiresAndroid: true},
		{ID: "ble", Label: "Bluetooth LE (Kable)",
			Description: "Multiplatform BLE client.",
			Tier:        TierExtra},
		{ID: "passkeys", Label: "Passkeys (Credential Manager)",
			Description: "androidx.credentials plus the Play services back-port.",
			Tier:        TierExtra, RequiresAndroid: true},
		{ID: "supabase", Label: "Supabase",
			Description: "BOM-managed auth, postgrest, storage and functions clients.",
			Tier:        TierExtra},
		{ID: "koin-compiler", Label: "Koin compiler plugin",
			Description: "Compile-time verification of the Koin graph.",
			Tier:        TierExtra},
		{ID: "browser", Label: "Custom Tabs (androidx.browser)",
			Description: "In-app browser tabs.",
			Tier:        TierExtra, RequiresAndroid: true},
		{ID: "appcompat", Label: "AppCompat",
			Description: "Only needed when interoperating with View-based screens.",
			Tier:        TierExtra, RequiresAndroid: true},
	}
}

// PackByID looks up a pack definition.
func PackByID(id string) (PackDef, bool) {
	for _, p := range Packs() {
		if p.ID == id {
			return p, true
		}
	}
	return PackDef{}, false
}

// BasicPacks returns the ids pre-selected by the "basic" preset.
func BasicPacks() []string {
	var out []string
	for _, p := range Packs() {
		if p.Tier == TierBasic {
			out = append(out, p.ID)
		}
	}
	return out
}

// UtilityDef is a shared-logic utility the generator can emit.
type UtilityDef struct {
	ID          string
	Label       string
	Description string
	Default     bool
	// Requires lists other utility ids this one needs.
	Requires []string
	// RequiresPack lists library packs this one needs.
	RequiresPack []string
}

// SharedUtilities returns every shared-logic utility, in wizard order.
func SharedUtilities() []UtilityDef {
	return []UtilityDef{
		{ID: "koin-di", Label: "Koin DI graph",
			Description: "SharedModules.kt with a coreModule plus expect/actual platformModule.",
			Default:     true},
		{ID: "base-viewmodel", Label: "BaseViewModel",
			Description: "Shared ViewModel base exposing a typed uiState StateFlow to both platforms.",
			Default:     true},
		{ID: "network-observer", Label: "Network observer",
			Description: "expect/actual connectivity Flow (ConnectivityManager / NWPathMonitor) + NetworkViewModel.",
			Default:     true, Requires: []string{"base-viewmodel"}},
		{ID: "ktor-engine", Label: "Ktor client factory",
			Description: "Shared HttpClient builder with JSON content negotiation.",
			Default:     true},
		{ID: "settings", Label: "App settings registry",
			Description: "Type-safe AppSetting tokens backed by multiplatform-settings, plus a ViewModel.",
			Default:     true, Requires: []string{"base-viewmodel"}},
		{ID: "clock", Label: "Clock abstraction",
			Description: "expect/actual SystemClock so time can be faked in tests.",
			Default:     true},
		{ID: "database", Label: "Room database builder",
			Description: "expect/actual RoomDatabase builders and a starter database class.",
			Default:     true, RequiresPack: []string{"database"}},
		{ID: "paging-presenter", Label: "Swift paging presenter",
			Description: "Bridges Paging 3 flows into something SwiftUI can consume.",
			Default:     false, RequiresPack: []string{"paging"}},
		{ID: "ios-koin-helper", Label: "iOS Koin helper",
			Description: "KoinHelper object so Swift can start Koin and resolve dependencies.",
			Default:     true, Requires: []string{"koin-di"}},
	}
}

// DefaultUtilities returns the utility ids enabled by default.
func DefaultUtilities() []string {
	var out []string
	for _, u := range SharedUtilities() {
		if u.Default {
			out = append(out, u.ID)
		}
	}
	return out
}

// UtilityByID looks up a utility definition.
func UtilityByID(id string) (UtilityDef, bool) {
	for _, u := range SharedUtilities() {
		if u.ID == id {
			return u, true
		}
	}
	return UtilityDef{}, false
}

// ExtraDef is an Android app extra the generator can emit.
type ExtraDef struct {
	ID           string
	Label        string
	Description  string
	Default      bool
	Requires     []string // other extras
	RequiresUtil []string
}

// AndroidExtras returns every Android extra, in wizard order.
func AndroidExtras() []ExtraDef {
	return []ExtraDef{
		{ID: "nav3-scenes", Label: "Bottom sheet & dialog scenes",
			Description: "SceneStrategy implementations so routes can present as sheets or dialogs.",
			Default:     true},
		{ID: "splash-screen", Label: "Splash screen",
			Description: "androidx.core.splashscreen wiring and the matching theme.",
			Default:     true},
		{ID: "snackbar-host", Label: "Global snackbar host",
			Description: "A single SnackbarHost that survives navigation, with sheet-aware padding.",
			Default:     true},
		{ID: "connectivity-banner", Label: "Connectivity banner",
			Description: "Offline banner driven by the shared network observer.",
			Default:     true, RequiresUtil: []string{"network-observer"}},
		{ID: "edge-to-edge", Label: "Edge-to-edge",
			Description: "enableEdgeToEdge plus safe-drawing inset handling in the shell.",
			Default:     true},
	}
}

// DefaultExtras returns the Android extra ids enabled by default.
func DefaultExtras() []string {
	var out []string
	for _, e := range AndroidExtras() {
		if e.Default {
			out = append(out, e.ID)
		}
	}
	return out
}

// ExtraByID looks up an extra definition.
func ExtraByID(id string) (ExtraDef, bool) {
	for _, e := range AndroidExtras() {
		if e.ID == id {
			return e, true
		}
	}
	return ExtraDef{}, false
}

// ---------------------------------------------------------------------------
// Selection resolution
// ---------------------------------------------------------------------------

// Normalise expands implied selections, drops choices that do not apply to the
// selected targets, and returns human-readable notes about what it changed.
func Normalise(s *model.Spec) []string {
	var notes []string

	// Packs: drop target-incompatible ones, expand implications.
	seen := map[string]bool{}
	var packs []string
	add := func(id string) {
		if seen[id] {
			return
		}
		def, ok := PackByID(id)
		if !ok {
			notes = append(notes, "ignored unknown library pack "+id)
			return
		}
		if def.RequiresIOS && !s.IOS {
			notes = append(notes, def.Label+" needs an iOS target - not included")
			return
		}
		if def.RequiresAndroid && !s.Android {
			notes = append(notes, def.Label+" needs an Android target - not included")
			return
		}
		seen[id] = true
		packs = append(packs, id)
	}
	for _, id := range s.Packs {
		add(id)
	}
	for _, id := range s.Packs {
		if def, ok := PackByID(id); ok {
			for _, imp := range def.Implies {
				if !seen[imp] {
					add(imp)
					notes = append(notes, imp+" enabled because it is required by "+def.Label)
				}
			}
		}
	}
	// Keep pack order stable and matching the wizard ordering.
	packs = sortByCatalogOrder(packs, packIDs())
	s.Packs = packs

	// Utilities: satisfy requirements, drop ones whose pack is missing.
	utilSeen := map[string]bool{}
	for _, id := range s.SharedUtils {
		utilSeen[id] = true
	}
	changed := true
	for changed {
		changed = false
		for id := range utilSeen {
			def, ok := UtilityByID(id)
			if !ok {
				continue
			}
			for _, req := range def.Requires {
				if !utilSeen[req] {
					utilSeen[req] = true
					changed = true
					if rd, ok := UtilityByID(req); ok {
						notes = append(notes, rd.Label+" enabled because "+def.Label+" depends on it")
					}
				}
			}
		}
	}
	var utils []string
	for _, def := range SharedUtilities() {
		if !utilSeen[def.ID] {
			continue
		}
		missing := ""
		for _, p := range def.RequiresPack {
			if !model.Has(s.Packs, p) {
				missing = p
				break
			}
		}
		if missing != "" {
			if pd, ok := PackByID(missing); ok {
				notes = append(notes, def.Label+" skipped - it needs the "+pd.Label+" pack")
			}
			continue
		}
		if def.ID == "ios-koin-helper" && !s.IOS {
			continue
		}
		utils = append(utils, def.ID)
	}
	s.SharedUtils = utils

	// Android extras: only meaningful with an Android target.
	// The single-stack layout has exactly one root; the shell can have many.
	if s.Android && s.AndroidLayout == "nav3-single" {
		s.RootTabs = []string{"Home"}
	}
	if s.Android && len(s.RootTabs) == 0 {
		s.RootTabs = []string{"Home"}
		notes = append(notes, "no root destinations given - added Home")
	}

	if !s.Android {
		if len(s.AndroidExtras) > 0 {
			notes = append(notes, "Android extras skipped - no Android target selected")
		}
		s.AndroidExtras = nil
		s.RootTabs = nil
		s.AndroidLayout = "none"
	} else {
		extraSeen := map[string]bool{}
		for _, id := range s.AndroidExtras {
			extraSeen[id] = true
		}
		var extras []string
		for _, def := range AndroidExtras() {
			if !extraSeen[def.ID] {
				continue
			}
			missing := ""
			for _, u := range def.RequiresUtil {
				if !model.Has(s.SharedUtils, u) {
					missing = u
					break
				}
			}
			if missing != "" {
				if ud, ok := UtilityByID(missing); ok {
					notes = append(notes, def.Label+" skipped - it needs the "+ud.Label+" utility")
				}
				continue
			}
			extras = append(extras, def.ID)
		}
		s.AndroidExtras = extras
	}

	if !s.IOS {
		s.IOSLayout = "none"
	}

	return notes
}

func packIDs() []string {
	var ids []string
	for _, p := range Packs() {
		ids = append(ids, p.ID)
	}
	return ids
}

func sortByCatalogOrder(selected, order []string) []string {
	pos := map[string]int{}
	for i, id := range order {
		pos[id] = i
	}
	out := append([]string(nil), selected...)
	sort.SliceStable(out, func(i, j int) bool {
		pi, oki := pos[out[i]]
		pj, okj := pos[out[j]]
		if !oki || !okj {
			return out[i] < out[j]
		}
		return pi < pj
	})
	return out
}

// SectionOrder exposes the canonical ordering of version-key sections.
func SectionOrder() []string { return append([]string(nil), sectionOrder...) }
