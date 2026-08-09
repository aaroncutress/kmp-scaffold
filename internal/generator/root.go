package generator

import "path"

func init() { Register(rootGenerator{}) }

// rootGenerator writes the Gradle root: settings, version catalog, wrapper
// properties, gradle.properties and the buildSrc convention plugins.
type rootGenerator struct{}

func (rootGenerator) ID() string    { return "root" }
func (rootGenerator) Label() string { return "Gradle root" }
func (rootGenerator) Description() string {
	return "settings.gradle.kts, the version catalog and the buildSrc convention plugins"
}
func (rootGenerator) Kind() Kind { return KindRoot }

func (rootGenerator) Generate(env *Env) error {
	spec := env.Ctx.Spec
	ns := spec.Namespace()

	files := []struct{ tpl, path string }{
		{"root/settings.gradle.kts", "settings.gradle.kts"},
		{"root/build.gradle.kts", "build.gradle.kts"},
		{"root/gradle.properties", "gradle.properties"},
		{"root/libs.versions.toml", "gradle/libs.versions.toml"},
		{"root/gradle-wrapper.properties", "gradle/wrapper/gradle-wrapper.properties"},
		{"root/gitignore", ".gitignore"},
		{"root/editorconfig", ".editorconfig"},
		{"root/gitattributes", ".gitattributes"},
		{"root/README.md", "README.md"},
		{"buildSrc/build.gradle.kts", "buildSrc/build.gradle.kts"},
	}

	if spec.CI {
		files = append(files, struct{ tpl, path string }{
			"ci/workflow.yml", ".github/workflows/ci.yml",
		})
	}

	if spec.HasPack("secrets") {
		files = append(files,
			struct{ tpl, path string }{"root/secrets.properties.template", "secrets.properties.template"},
			struct{ tpl, path string }{"buildSrc/secrets.gradle.kts",
				path.Join("buildSrc/src/main/kotlin", ns+".secrets.gradle.kts")},
		)
	}

	if spec.Android {
		files = append(files,
			struct{ tpl, path string }{"buildSrc/android.library.gradle.kts",
				path.Join("buildSrc/src/main/kotlin", ns+".android.library.gradle.kts")},
			struct{ tpl, path string }{"buildSrc/android.compose.gradle.kts",
				path.Join("buildSrc/src/main/kotlin", ns+".android.compose.gradle.kts")},
			struct{ tpl, path string }{"buildSrc/android.feature.gradle.kts",
				path.Join("buildSrc/src/main/kotlin", ns+".android.feature.gradle.kts")},
		)
	}

	// .run/ is the shared JetBrains run-configuration directory, so these land
	// in the IDE's run dropdown the first time the project is opened. Unlike
	// .idea/ it is meant to be committed, and the generated .gitignore leaves
	// it alone.
	for _, r := range runConfigurations(env.Ctx) {
		files = append(files, struct{ tpl, path string }{r, path.Join(".run", path.Base(r))})
	}

	for _, f := range files {
		if err := env.Render(f.tpl, f.path); err != nil {
			return err
		}
	}
	return nil
}

// runConfigurations names the template blocks for the .run configurations this
// project can use. Each block's base name is also its file name, so a new
// configuration is one entry here plus one {{define}} in run.tmpl.
// HasRunConfigurations reports whether a .run/ directory was written, so the
// README only describes one when it exists. It asks the same function that
// writes them rather than restating the conditions.
func (c Ctx) HasRunConfigurations() bool { return len(runConfigurations(c)) > 0 }

func runConfigurations(c Ctx) []string {
	spec := c.Spec
	var tpls []string

	if spec.Android {
		tpls = append(tpls, "run/androidApp.run.xml")
	}
	if spec.HasPack("secrets") {
		tpls = append(tpls, "run/Generate Build Konfig.run.xml")
	}
	if spec.IOS {
		if spec.IOSLayout != "" && spec.IOSLayout != "none" {
			tpls = append(tpls, "run/iosApp.run.xml")
		}
		tpls = append(tpls, "run/Link iOS Framework (Debug).run.xml")
		// The modular layout consumes an XCFramework through SPM, so
		// assembling it - not linking a single framework - is the task you
		// reach for after editing sharedLogic.
		if c.UsesIOSFeatures() {
			tpls = append(tpls, "run/Build iOS XCFramework (Debug).run.xml")
		}
	}
	return tpls
}
