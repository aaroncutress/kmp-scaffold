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
		{"root/README.md", "README.md"},
		{"buildSrc/build.gradle.kts", "buildSrc/build.gradle.kts"},
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

	for _, f := range files {
		if err := env.Render(f.tpl, f.path); err != nil {
			return err
		}
	}
	return nil
}
