package generator

func init() { Register(iosSwiftUIGenerator{}) }

// iosSwiftUIGenerator writes a plain SwiftUI app that links the sharedLogic
// framework and starts Koin at launch.
//
// This is deliberately the *simplest* iOS layout. Adding a richer one (say, a
// feature-per-folder structure mirroring the Android side) means writing a new
// Generator with Kind() == KindIOS and registering it - the wizard picks it up
// with no other changes. See docs/extending.md.
type iosSwiftUIGenerator struct{}

func (iosSwiftUIGenerator) ID() string    { return "swiftui-simple" }
func (iosSwiftUIGenerator) Label() string { return "SwiftUI, single entry point" }
func (iosSwiftUIGenerator) Description() string {
	return "iOSApp.swift + ContentView.swift, Koin started at launch, framework linked by Gradle"
}
func (iosSwiftUIGenerator) Kind() Kind { return KindIOS }

func (iosSwiftUIGenerator) Generate(env *Env) error {
	files := []struct{ tpl, path string }{
		{"ios/project.pbxproj", "iosApp/iosApp.xcodeproj/project.pbxproj"},
		{"ios/contents.xcworkspacedata", "iosApp/iosApp.xcodeproj/project.xcworkspace/contents.xcworkspacedata"},
		{"ios/Config.xcconfig", "iosApp/Configuration/Config.xcconfig"},
		{"ios/iOSApp.swift", "iosApp/iosApp/iOSApp.swift"},
		{"ios/ContentView.swift", "iosApp/iosApp/ContentView.swift"},
		{"ios/Info.plist", "iosApp/iosApp/Info.plist"},
		{"ios/Assets.Contents.json", "iosApp/iosApp/Assets.xcassets/Contents.json"},
		{"ios/AppIcon.Contents.json", "iosApp/iosApp/Assets.xcassets/AppIcon.appiconset/Contents.json"},
		{"ios/AccentColor.Contents.json", "iosApp/iosApp/Assets.xcassets/AccentColor.colorset/Contents.json"},
		{"ios/PreviewAssets.Contents.json", "iosApp/iosApp/Preview Content/Preview Assets.xcassets/Contents.json"},
	}
	for _, f := range files {
		if err := env.Render(f.tpl, f.path); err != nil {
			return err
		}
	}
	return nil
}
