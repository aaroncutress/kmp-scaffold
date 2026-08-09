package filetmpl_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/render"
	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
	"github.com/aaroncutress/kmp-scaffold/internal/scaffold/filetmpl"
)

// exampleDir is the template that ships with the repository. Testing against it
// rather than a fixture means the example cannot rot: if it stops working, this
// fails.
const exampleDir = "../../../examples/templates/ktor-service"

func open(t *testing.T, dir string) *filetmpl.Template {
	t.Helper()
	tmpl, err := filetmpl.Open(dir, scaffold.Source{Kind: scaffold.SourcePath, Ref: dir})
	if err != nil {
		t.Fatalf("opening %s: %v", dir, err)
	}
	return tmpl
}

// write puts a template on disk from a map of path to content, so each test
// says exactly what it is about.
func write(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func generate(t *testing.T, tmpl scaffold.Template, a *scaffold.Answers) string {
	t.Helper()
	root := t.TempDir()
	a.Project.Dir = root
	a.Offline = true

	var res *resolve.Result
	if req := tmpl.Versions(a); !req.Empty() {
		res = resolve.Run(context.Background(), req)
	}

	writer := render.NewWriter(root, false, false)
	if _, err := tmpl.Generate(context.Background(), scaffold.GenRequest{
		Answers: a, Result: res, Writer: writer, Version: "test",
	}); err != nil {
		t.Fatalf("generating: %v", err)
	}
	return root
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return string(data)
}

// ---------------------------------------------------------------------------
// The example template, end to end
// ---------------------------------------------------------------------------

func TestExampleTemplateGenerates(t *testing.T) {
	tmpl := open(t, exampleDir)

	if got := tmpl.Meta().ID; got != "ktor-service" {
		t.Errorf("id = %q", got)
	}
	if !tmpl.Meta().AsksPackage {
		t.Error("the example asks for a package")
	}

	a := tmpl.NewAnswers()
	a.Project = model.Project{Name: "Orders API", Package: "com.example.orders"}
	root := generate(t, tmpl, a)

	// A glob writes the whole subtree, keeping its shape; for_each writes one
	// file per item; `when` decides the rest.
	for _, rel := range []string{
		"build.gradle.kts",
		"settings.gradle.kts",
		".gitignore",
		"src/main/resources/application.yaml",
		"src/main/kotlin/com/example/orders/Application.kt",
		"src/main/kotlin/com/example/orders/Modules.kt",
		"src/main/kotlin/com/example/orders/routes/HealthRoutes.kt",
		"src/main/kotlin/com/example/orders/routes/UsersRoutes.kt",
		"src/main/kotlin/com/example/orders/service/HealthService.kt",
		"src/main/kotlin/com/example/orders/service/UsersService.kt",
		"Dockerfile",
		model.ManifestFile,
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}

	// The answers reach the content, not just the paths.
	if got := read(t, root, "settings.gradle.kts"); !strings.Contains(got, `rootProject.name = "orders-api"`) {
		t.Errorf("settings.gradle.kts = %q", got)
	}
	if got := read(t, root, "src/main/resources/application.yaml"); !strings.Contains(got, "port: 8080") {
		t.Errorf("the port answer did not reach application.yaml:\n%s", got)
	}

	app := read(t, root, "src/main/kotlin/com/example/orders/Application.kt")
	for _, want := range []string{"healthRoutes()", "usersRoutes()", "install(CallLogging)"} {
		if !strings.Contains(app, want) {
			t.Errorf("Application.kt is missing %q:\n%s", want, app)
		}
	}
	// CORS was not picked, so its plugin must not be installed.
	if strings.Contains(app, "install(CORS)") {
		t.Error("an unpicked extra was generated anyway")
	}
}

// The manifest records which template built the project, so a later command
// loads the same one.
func TestGeneratedProjectRecordsItsTemplate(t *testing.T) {
	tmpl := open(t, exampleDir)
	a := tmpl.NewAnswers()
	a.Project = model.Project{Name: "Orders API", Package: "com.example.orders"}
	root := generate(t, tmpl, a)

	manifest, _, err := model.LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Template.ID != "ktor-service" {
		t.Errorf("template id = %q", manifest.Template.ID)
	}
	if manifest.Template.Source == "builtin" {
		t.Error("a path template should record where it came from")
	}

	// A file-based template's vars are its answers, verbatim.
	var vars map[string]any
	if err := manifest.DecodeVars(&vars); err != nil {
		t.Fatal(err)
	}
	if vars["port"] != "8080" {
		t.Errorf("vars = %v, want the port answer", vars)
	}
}

// `when` on a file, and the answers it reads, decide whether it is written.
func TestWhenDecidesWhetherAFileIsWritten(t *testing.T) {
	tmpl := open(t, exampleDir)
	a := tmpl.NewAnswers()
	a.Project = model.Project{Name: "Orders API", Package: "com.example.orders"}
	a.Set("docker", false)
	a.Set("extras", []string{"cors"})
	root := generate(t, tmpl, a)

	if _, err := os.Stat(filepath.Join(root, "Dockerfile")); err == nil {
		t.Error("the Dockerfile was written even though docker was off")
	}
	app := read(t, root, "src/main/kotlin/com/example/orders/Application.kt")
	if !strings.Contains(app, "install(CORS)") {
		t.Error("the cors extra did not reach Application.kt")
	}
	if strings.Contains(app, "install(CallLogging)") {
		t.Error("logging was not picked but was generated anyway")
	}
}

// ---------------------------------------------------------------------------
// The manifest format
// ---------------------------------------------------------------------------

const minimal = `
schema = 1
[template]
id = "tiny"
[[files]]
from = "files/hello.txt.tmpl"
to = "hello.txt"
`

func TestMinimalTemplate(t *testing.T) {
	dir := write(t, map[string]string{
		"template.toml":        minimal,
		"files/hello.txt.tmpl": "Hello, {{ .Project.Name }}.\n",
	})
	tmpl := open(t, dir)

	if tmpl.Meta().Label != "tiny" {
		t.Errorf("a template with no name falls back to its id, got %q", tmpl.Meta().Label)
	}
	if !tmpl.Versions(tmpl.NewAnswers()).Empty() {
		t.Error("a template that declares no versions should skip resolution")
	}

	a := tmpl.NewAnswers()
	a.Project = model.Project{Name: "Widget"}
	root := generate(t, tmpl, a)
	if got := read(t, root, "hello.txt"); got != "Hello, Widget.\n" {
		t.Errorf("hello.txt = %q", got)
	}
}

// A mistyped key is a mistake, and saying so beats silently doing nothing.
func TestUnknownKeysAreRejected(t *testing.T) {
	dir := write(t, map[string]string{
		"template.toml":        minimal + "\n[[questions]]\nid = \"x\"\nprompt = \"?\"\nwhne = \"typo\"\n",
		"files/hello.txt.tmpl": "hi\n",
	})
	_, err := filetmpl.Open(dir, scaffold.Source{})
	if err == nil {
		t.Fatal("a mistyped key should be an error")
	}
	if !strings.Contains(err.Error(), "whne") {
		t.Errorf("the error should name the key: %v", err)
	}
}

func TestManifestValidation(t *testing.T) {
	for _, tc := range []struct {
		name, manifest, want string
	}{
		{"no schema", "[template]\nid = \"x\"\n[[files]]\nfrom = \"files/hello.txt.tmpl\"\nto = \"a\"\n", "schema"},
		{"no id", "schema = 1\n[template]\nname = \"x\"\n[[files]]\nfrom = \"files/hello.txt.tmpl\"\nto = \"a\"\n", "no id"},
		{"bad id", "schema = 1\n[template]\nid = \"Not Valid\"\n[[files]]\nfrom = \"files/hello.txt.tmpl\"\nto = \"a\"\n", "template id"},
		{"no files", "schema = 1\n[template]\nid = \"x\"\n", "would generate nothing"},
		{"missing source", "schema = 1\n[template]\nid = \"x\"\n[[files]]\nfrom = \"files/nope.tmpl\"\nto = \"a\"\n", "does not exist"},
		{"escaping source", "schema = 1\n[template]\nid = \"x\"\n[[files]]\nfrom = \"../secrets\"\nto = \"a\"\n", "outside the template"},
		{"unknown kind", "schema = 1\n[template]\nid = \"x\"\n[[questions]]\nid = \"q\"\nprompt = \"?\"\nkind = \"slider\"\n[[files]]\nfrom = \"files/hello.txt.tmpl\"\nto = \"a\"\n", "unknown kind"},
		{"select with no options", "schema = 1\n[template]\nid = \"x\"\n[[questions]]\nid = \"q\"\nprompt = \"?\"\nkind = \"select\"\n[[files]]\nfrom = \"files/hello.txt.tmpl\"\nto = \"a\"\n", "no options"},
		{"duplicate ids", "schema = 1\n[template]\nid = \"x\"\n[[questions]]\nid = \"q\"\nprompt = \"?\"\n[[questions]]\nid = \"q\"\nprompt = \"?\"\n[[files]]\nfrom = \"files/hello.txt.tmpl\"\nto = \"a\"\n", "share the id"},
		{"bad mode", "schema = 1\n[template]\nid = \"x\"\n[[files]]\nfrom = \"files/hello.txt.tmpl\"\nto = \"a\"\nmode = \"rwx\"\n", "octal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := write(t, map[string]string{
				"template.toml":        tc.manifest,
				"files/hello.txt.tmpl": "hi\n",
			})
			_, err := filetmpl.Open(dir, scaffold.Source{})
			if err == nil {
				t.Fatalf("expected an error mentioning %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// An output path cannot escape the project directory, however it is templated.
func TestOutputPathsCannotEscape(t *testing.T) {
	dir := write(t, map[string]string{
		"template.toml": `
schema = 1
[template]
id = "escape"
[[files]]
from = "files/hello.txt.tmpl"
to = "../../{{ .Project.Name }}"
`,
		"files/hello.txt.tmpl": "hi\n",
	})
	tmpl := open(t, dir)

	a := tmpl.NewAnswers()
	a.Project = model.Project{Name: "Widget", Dir: t.TempDir()}
	writer := render.NewWriter(a.Project.Dir, false, false)
	_, err := tmpl.Generate(context.Background(), scaffold.GenRequest{
		Answers: a, Writer: writer, Version: "test",
	})
	if err == nil {
		t.Fatal("a path escaping the project root should be refused")
	}
	if !strings.Contains(err.Error(), "escapes") {
		t.Errorf("error = %v, want it to say the path escapes", err)
	}
}

// ---------------------------------------------------------------------------
// Rendering rules
// ---------------------------------------------------------------------------

func TestGlobsKeepTheirShapeAndHonourDotPrefixes(t *testing.T) {
	dir := write(t, map[string]string{
		"template.toml": `
schema = 1
[template]
id = "globs"
[[files]]
from = "files/tree/**"
to = "out/{{ .RelPath }}"
`,
		"files/tree/a.txt.tmpl":         "{{ .Project.Name }}\n",
		"files/tree/nested/b.txt.tmpl":  "nested\n",
		"files/tree/dot-gitignore.tmpl": "build/\n",
		"files/tree/plain.md":           "not a template\n",
	})

	a := open(t, dir).NewAnswers()
	a.Project = model.Project{Name: "Widget"}
	root := generate(t, open(t, dir), a)

	for _, rel := range []string{"out/a.txt", "out/nested/b.txt", "out/.gitignore", "out/plain.md"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %s: %v", rel, err)
		}
	}
	if got := read(t, root, "out/a.txt"); got != "Widget\n" {
		t.Errorf("out/a.txt = %q", got)
	}
}

// A destination ending in a slash keeps the source's own name.
func TestDirectoryDestinationKeepsTheSourceName(t *testing.T) {
	dir := write(t, map[string]string{
		"template.toml": `
schema = 1
[template]
id = "dir-dest"
uses_package = true
[[files]]
from = "files/Main.kt.tmpl"
to = "src/{{ packagePath .Project.Package }}/"
`,
		"files/Main.kt.tmpl": "package {{ .Project.Package }}\n",
	})
	tmpl := open(t, dir)
	a := tmpl.NewAnswers()
	a.Project = model.Project{Name: "Widget", Package: "com.example.widget"}
	root := generate(t, tmpl, a)

	if got := read(t, root, "src/com/example/widget/Main.kt"); !strings.Contains(got, "com.example.widget") {
		t.Errorf("Main.kt = %q", got)
	}
}

func TestCopyLeavesContentAlone(t *testing.T) {
	const raw = "{{ this is not a template }}\n\n\n\nand blank runs are kept\n"
	dir := write(t, map[string]string{
		"template.toml": `
schema = 1
[template]
id = "copier"
[[files]]
from = "files/raw.txt"
to = "raw.txt"
copy = true
`,
		"files/raw.txt": raw,
	})
	tmpl := open(t, dir)
	a := tmpl.NewAnswers()
	a.Project = model.Project{Name: "Widget"}
	root := generate(t, tmpl, a)

	if got := read(t, root, "raw.txt"); got != raw {
		t.Errorf("raw.txt = %q, want it byte-for-byte", got)
	}
}

func TestModeIsHonoured(t *testing.T) {
	dir := write(t, map[string]string{
		"template.toml": `
schema = 1
[template]
id = "modes"
[[files]]
from = "files/run.sh.tmpl"
to = "run.sh"
mode = "0755"
[[files]]
from = "files/notes.txt.tmpl"
to = "notes.txt"
`,
		"files/run.sh.tmpl":    "#!/bin/sh\necho {{ .Project.Name }}\n",
		"files/notes.txt.tmpl": "notes\n",
	})
	tmpl := open(t, dir)
	a := tmpl.NewAnswers()
	a.Project = model.Project{Name: "Widget"}
	root := generate(t, tmpl, a)

	script, err := os.Stat(filepath.Join(root, "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if script.Mode()&0o111 == 0 {
		t.Errorf("run.sh mode = %v, want it executable", script.Mode())
	}
	notes, err := os.Stat(filepath.Join(root, "notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if notes.Mode()&0o111 != 0 {
		t.Errorf("notes.txt mode = %v, want it not executable", notes.Mode())
	}
}

// ---------------------------------------------------------------------------
// Questions
// ---------------------------------------------------------------------------

func TestQuestionsCarryTheirRules(t *testing.T) {
	tmpl := open(t, exampleDir)
	byID := map[string]scaffold.Question{}
	for _, q := range tmpl.Questions() {
		byID[q.ID] = q
	}

	port, ok := byID["port"]
	if !ok {
		t.Fatal("the port question is missing")
	}
	if err := port.Validate("not-a-port", tmpl.NewAnswers()); err == nil {
		t.Error("the pattern was not applied")
	}
	if err := port.Validate("8080", tmpl.NewAnswers()); err != nil {
		t.Errorf("8080 should be valid: %v", err)
	}

	routes := byID["routes"]
	if err := routes.Validate([]string{}, tmpl.NewAnswers()); err == nil {
		t.Error("min was not applied")
	}
	if err := routes.Validate([]string{"Bad Name"}, tmpl.NewAnswers()); err == nil {
		t.Error("the per-item pattern was not applied")
	}
}

func TestWhenSkipsAQuestion(t *testing.T) {
	dir := write(t, map[string]string{
		"template.toml": `
schema = 1
[template]
id = "conditional"
[[questions]]
id = "database"
kind = "select"
prompt = "Which database?"
default = "none"
  [[questions.options]]
  id = "postgres"
  [[questions.options]]
  id = "none"
[[questions]]
id = "migrations"
kind = "confirm"
prompt = "Generate migrations?"
when = '{{ ne .Vars.database "none" }}'
[[files]]
from = "files/hello.txt.tmpl"
to = "hello.txt"
`,
		"files/hello.txt.tmpl": "hi\n",
	})
	tmpl := open(t, dir)

	var migrations scaffold.Question
	for _, q := range tmpl.Questions() {
		if q.ID == "migrations" {
			migrations = q
		}
	}

	a := tmpl.NewAnswers()
	if !migrations.Skip(a) {
		t.Error("with no database, the migrations question should be skipped")
	}
	a.Set("database", "postgres")
	if migrations.Skip(a) {
		t.Error("with a database, the migrations question should be asked")
	}
}

func TestDisabledWhenGreysAnOptionOut(t *testing.T) {
	dir := write(t, map[string]string{
		"template.toml": `
schema = 1
[template]
id = "greying"
[[questions]]
id = "kind"
kind = "select"
prompt = "Which?"
  [[questions.options]]
  id = "simple"
  [[questions.options]]
  id = "advanced"
  disabled_when = "{{ not .Vars.expert }}"
  disabled_note = "turn on expert mode first"
[[files]]
from = "files/hello.txt.tmpl"
to = "hello.txt"
`,
		"files/hello.txt.tmpl": "hi\n",
	})
	tmpl := open(t, dir)
	q := tmpl.Questions()[0]

	a := tmpl.NewAnswers()
	opts := q.OptionList(a)
	if !opts[1].Disabled || opts[1].DisabledNote == "" {
		t.Errorf("advanced should be greyed out with a reason: %+v", opts[1])
	}

	a.Set("expert", true)
	if q.OptionList(a)[1].Disabled {
		t.Error("advanced should be selectable once expert mode is on")
	}
}

// ---------------------------------------------------------------------------
// Discovery
// ---------------------------------------------------------------------------

func TestUserFolderDiscovery(t *testing.T) {
	folder := t.TempDir()
	t.Setenv("KMP_SCAFFOLD_TEMPLATES", folder)

	// A working template, and one that will not parse.
	good := filepath.Join(folder, "tiny")
	if err := os.MkdirAll(filepath.Join(good, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "template.toml"), []byte(minimal), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "files/hello.txt.tmpl"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bad := filepath.Join(folder, "broken")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "template.toml"), []byte("schema = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	found := filetmpl.Loader{}.Discover()
	if len(found) != 1 || found[0].Meta().ID != "tiny" {
		t.Fatalf("discovered %d template(s), want just the working one", len(found))
	}
	if found[0].Meta().Source.Kind != scaffold.SourceUser {
		t.Errorf("source = %v, want user", found[0].Meta().Source.Kind)
	}

	// A bare name resolves against the folder.
	loaded, err := filetmpl.Loader{}.Load("tiny")
	if err != nil || loaded == nil {
		t.Fatalf("Load(tiny) = %v, %v", loaded, err)
	}

	// The broken one is reported rather than silently missing.
	if broken := filetmpl.Broken(); len(broken) != 1 || broken["broken"] == nil {
		t.Errorf("Broken() = %v, want the unparseable template named", broken)
	}
}

func TestLoaderIgnoresNamesItDoesNotOwn(t *testing.T) {
	t.Setenv("KMP_SCAFFOLD_TEMPLATES", t.TempDir())

	// A bare name that is not in the folder is not this loader's business, so
	// the caller can report it against the full list of templates.
	tmpl, err := filetmpl.Loader{}.Load("kmp-mobile")
	if tmpl != nil || err != nil {
		t.Errorf("Load(kmp-mobile) = %v, %v, want nil, nil", tmpl, err)
	}

	// A remote ref is recognised, so the message says what is missing.
	if _, err := (filetmpl.Loader{}).Load("github:acme/tmpl@v1"); err == nil {
		t.Error("a remote ref should be an error until they are supported")
	}

	// A path that is not there says so.
	if _, err := (filetmpl.Loader{}).Load("./does-not-exist"); err == nil {
		t.Error("a missing directory should be an error")
	}
}

// A template on disk is loadable through the registry, which is what
// `--template ./dir` goes through.
func TestRegistryLoadsAPathTemplate(t *testing.T) {
	abs, err := filepath.Abs(exampleDir)
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := scaffold.Load(abs)
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.Meta().ID != "ktor-service" {
		t.Errorf("id = %q", tmpl.Meta().ID)
	}
	if tmpl.Meta().Source.Kind != scaffold.SourcePath {
		t.Errorf("source = %v, want path", tmpl.Meta().Source.Kind)
	}
}
