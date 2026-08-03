package generator

import "path"

func init() { Register(sharedGenerator{}) }

// sharedGenerator writes the sharedLogic KMP module: the build script plus the
// utilities the user opted into.
type sharedGenerator struct{}

func (sharedGenerator) ID() string    { return "shared" }
func (sharedGenerator) Label() string { return "sharedLogic module" }
func (sharedGenerator) Description() string {
	return "the Kotlin Multiplatform module shared by Android and iOS"
}
func (sharedGenerator) Kind() Kind { return KindShared }

func (sharedGenerator) Generate(env *Env) error {
	spec := env.Ctx.Spec
	pkg := spec.PackagePath()

	common := func(p string) string {
		return path.Join("sharedLogic/src/commonMain/kotlin", pkg, p)
	}
	androidSrc := func(p string) string {
		return path.Join("sharedLogic/src/androidMain/kotlin", pkg, p)
	}
	iosSrc := func(p string) string {
		return path.Join("sharedLogic/src/iosMain/kotlin", pkg, p)
	}
	test := func(p string) string {
		return path.Join("sharedLogic/src/commonTest/kotlin", pkg, p)
	}

	type file struct {
		tpl, path string
		when      bool
	}

	files := []file{
		{"shared/build.gradle.kts", "sharedLogic/build.gradle.kts", true},

		// Platform introspection - the one thing every KMP module starts with.
		{"shared/Platform.kt", common("core/util/Platform.kt"), true},
		{"shared/Platform.android.kt", androidSrc("core/util/Platform.android.kt"), spec.Android},
		{"shared/Platform.apple.kt", iosSrc("core/util/Platform.apple.kt"), spec.IOS},

		// BaseViewModel
		{"shared/BaseViewModel.kt", common("core/presentation/BaseViewModel.kt"),
			spec.HasSharedUtil("base-viewmodel")},

		// Network observer
		{"shared/NetworkObserver.kt", common("core/util/NetworkObserver.kt"),
			spec.HasSharedUtil("network-observer")},
		{"shared/NetworkObserver.android.kt", androidSrc("core/util/NetworkObserver.android.kt"),
			spec.HasSharedUtil("network-observer") && spec.Android},
		{"shared/NetworkObserver.apple.kt", iosSrc("core/util/NetworkObserver.apple.kt"),
			spec.HasSharedUtil("network-observer") && spec.IOS},
		{"shared/NetworkViewModel.kt", common("core/util/NetworkViewModel.kt"),
			spec.HasSharedUtil("network-observer")},

		// Clock
		{"shared/Clock.kt", common("core/util/Clock.kt"), spec.HasSharedUtil("clock")},
		{"shared/SystemClock.android.kt", androidSrc("core/util/SystemClock.android.kt"),
			spec.HasSharedUtil("clock") && spec.Android},
		{"shared/SystemClock.apple.kt", iosSrc("core/util/SystemClock.apple.kt"),
			spec.HasSharedUtil("clock") && spec.IOS},

		// Ktor
		{"shared/KtorNetworkEngine.kt", common("core/network/KtorNetworkEngine.kt"),
			spec.HasSharedUtil("ktor-engine")},

		// Settings
		{"shared/AppSetting.kt", common("feature/settings/domain/AppSetting.kt"),
			spec.HasSharedUtil("settings")},
		{"shared/AppSettingsRepository.kt", common("feature/settings/domain/AppSettingsRepository.kt"),
			spec.HasSharedUtil("settings")},
		{"shared/AppSettingsRepositoryImpl.kt", common("feature/settings/data/AppSettingsRepositoryImpl.kt"),
			spec.HasSharedUtil("settings")},
		{"shared/SettingsModule.kt", common("feature/settings/di/SettingsModule.kt"),
			spec.HasSharedUtil("settings")},
		{"shared/SettingsViewModel.kt", common("feature/settings/presentation/SettingsViewModel.kt"),
			spec.HasSharedUtil("settings")},
		{"shared/SettingsViewModelTest.kt", test("feature/settings/presentation/SettingsViewModelTest.kt"),
			spec.HasSharedUtil("settings")},

		// Database
		{"shared/AppDatabase.kt", common("core/database/AppDatabase.kt"),
			spec.HasSharedUtil("database")},
		{"shared/DatabaseBuilder.kt", common("core/database/DatabaseBuilder.kt"),
			spec.HasSharedUtil("database")},
		{"shared/DatabaseBuilder.android.kt", androidSrc("core/database/DatabaseBuilder.android.kt"),
			spec.HasSharedUtil("database") && spec.Android},
		{"shared/DatabaseBuilder.apple.kt", iosSrc("core/database/DatabaseBuilder.apple.kt"),
			spec.HasSharedUtil("database") && spec.IOS},

		// Paging bridge
		{"shared/SwiftPagingPresenter.kt", common("core/util/SwiftPagingPresenter.kt"),
			spec.HasSharedUtil("paging-presenter")},

		// DI graph
		{"shared/SharedModules.kt", common("di/SharedModules.kt"), spec.HasSharedUtil("koin-di")},
		{"shared/SharedModules.android.kt", androidSrc("di/SharedModules.android.kt"),
			spec.HasSharedUtil("koin-di") && spec.Android},
		{"shared/SharedModules.ios.kt", iosSrc("di/SharedModules.ios.kt"),
			spec.HasSharedUtil("koin-di") && spec.IOS},
		{"shared/KoinHelper.kt", iosSrc("KoinHelper.kt"), spec.HasSharedUtil("ios-koin-helper")},
	}

	for _, f := range files {
		if !f.when {
			continue
		}
		if err := env.Render(f.tpl, f.path); err != nil {
			return err
		}
	}

	if spec.HasPack("database") {
		// Room writes exported schemas here; keep the directory in the repo.
		if err := env.Writer.WriteBytes("sharedLogic/schemas/.gitkeep", []byte("\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}
