# Templates

`kmp-scaffold new` generates from a **template**. A template owns a whole
project: the questions it asks, the versions it resolves, and the files it
writes.

The Kotlin Multiplatform project the rest of this documentation describes is one
template, `kmp-mobile`, and it is the default. This page is about using others,
and writing your own.

## Using a template

```bash
kmp-scaffold templates                    # what is available
kmp-scaffold templates ktor-service       # what one asks, and what it writes
kmp-scaffold new my-api --template ktor-service
```

`--template` takes any of:

| Ref | Means |
| --- | --- |
| `kmp-mobile` | a built-in template |
| `ktor-service` | a template in your templates folder |
| `./templates/my-thing` | a directory, relative or absolute |

When more than one template is available, the wizard asks which to use before
anything else — the answer decides what the rest of the questions are.

Whichever built the project is recorded in its `.kmp-scaffold.json`, so a later
`kmp-scaffold add` extends it the way it was made.

## Your templates folder

Every directory here containing a `template.toml` is a template, offered by
name:

```
~/.config/kmp-scaffold/templates/
└── ktor-service/
    ├── template.toml
    └── files/
```

The location follows `XDG_CONFIG_HOME` if it is set, and
`KMP_SCAFFOLD_TEMPLATES` overrides it entirely. `kmp-scaffold templates` prints
the path it is using, and lists anything in there that will not load, with the
reason — so a mistake shows up rather than the template silently vanishing.

Built-in ids win, so a template in your folder cannot shadow `kmp-mobile` by
accident.

## Writing a template

A template is a directory with a `template.toml` and a tree of
[Go templates](https://pkg.go.dev/text/template). No compiler, no Go.

There is a complete worked example in
[`examples/templates/ktor-service`](../examples/templates/ktor-service) — a Ktor
server with routes, DI and a Dockerfile. Generate from it to see it work:

```bash
kmp-scaffold new my-api --template ./examples/templates/ktor-service
```

### The shape

```
my-template/
├── template.toml                  the manifest
├── files/                         what gets written
│   ├── build.gradle.kts.tmpl
│   ├── dot-gitignore.tmpl         →  .gitignore
│   └── app/Application.kt.tmpl
└── partials/                      optional shared {{define}} blocks
    └── header.tmpl
```

Two naming conventions, both there so a template's own files stay ordinary
files:

- **`.tmpl` is stripped** from the output name.
- **A `dot-` prefix becomes a leading dot**, so a template can carry a
  `.gitignore` without git honouring it inside the template itself.

### The manifest

```toml
schema = 1

[template]
id           = "ktor-service"        # required; lowercase, dashes allowed
name         = "Ktor service"
description  = "A Ktor server with layered routes and Koin DI."
version      = "1.0.0"
uses_package = true                  # ask the universal "package name?" question
sentinels    = ["build.gradle.kts"]  # files meaning "already a project here"
```

#### Questions

Asked in order, after the universal name, directory and package questions.

```toml
[[questions]]
id      = "port"
kind    = "text"                     # text | list | select | multiselect | confirm
prompt  = "Which port should the server listen on?"
hint    = "Overridden at runtime by $PORT."
default = "8080"
pattern = '^[0-9]{2,5}$'             # regexp every answer must match
invalid = "enter a port number, e.g. 8080"

[[questions]]
id      = "routes"
kind    = "list"                     # one comma-separated line, answered as a list
default = ["health", "users"]
prompt  = "Which top-level routes?"
min     = 1                          # bounds for list and multiselect
max     = 8
pattern = '^[a-z][a-z0-9-]*$'        # applied to each item

[[questions]]
id          = "extras"
kind        = "multiselect"
prompt      = "Which extras?"
allow_empty = true
default     = ["logging"]

  [[questions.options]]
  id    = "logging"
  label = "Request logging"
  desc  = "The CallLogging plugin, at INFO."

  [[questions.options]]
  id            = "migrations"
  label         = "Flyway migrations"
  disabled_when = '{{ eq .Vars.database "none" }}'
  disabled_note = "needs a database"

[[questions]]
id      = "docker"
kind    = "confirm"
prompt  = "Generate a Dockerfile?"
default = true
when    = '{{ ne .Vars.database "none" }}'   # skipped when this is falsey
```

`id` is what the answer is stored under, and how everything else refers to it:
`{{ .Vars.port }}`.

#### Versions

Declare what to resolve and the generated files are never pinned to whatever was
current when the template was written. Leave the block out entirely and the
resolve screen is skipped.

```toml
[versions]
keys         = ["kotlin", "ktor", "koin"]   # keys kmp-scaffold already knows
channel_from = "channel"                    # a question holding stable/preview/bleeding
channel      = "preview"                    # used when no question supplies one
android      = false                        # resolve compileSdk / build-tools too
min_sdk_from = "minSdk"

  [[versions.probe]]                        # anything not already in the catalog
  key      = "logback"
  group    = "ch.qos.logback"
  artifact = "logback-classic"
  repo     = "central"                      # central | google | portal
  baseline = "1.5.16"                       # used offline, or if the lookup fails
  min_channel = "stable"
```

The catalog's key names are in
[libraries and versions](libraries-and-versions.md).

#### Files

Rendered in order. Every `to` is itself a Go template.

```toml
[[files]]
from = "files/build.gradle.kts.tmpl"
to   = "build.gradle.kts"

[[files]]
from = "files/root/**"                       # the whole subtree
to   = "{{ .RelPath }}"                      # .RelPath is the path under files/root

[[files]]
from = "files/app/Application.kt.tmpl"
to   = "src/main/kotlin/{{ packagePath .Project.Package }}/Application.kt"

[[files]]
from     = "files/routes/Routes.kt.tmpl"
to       = "src/main/kotlin/{{ packagePath .Project.Package }}/{{ pascal .Item }}Routes.kt"
for_each = ".Vars.routes"                    # once per item, with .Item bound

[[files]]
from = "files/Dockerfile.tmpl"
to   = "Dockerfile"
when = "{{ .Vars.docker }}"

[[files]]
from = "files/assets/**"
to   = "src/main/resources/{{ .RelPath }}"
copy = true                                  # verbatim; not parsed as a template

[[files]]
from      = "files/gradlew.tmpl"
to        = "gradlew"
mode      = "0755"
normalise = false                            # keep blank runs and trailing space
```

A `to` ending in `/` keeps the source's own name under it. A file whose content
renders to nothing is not written, so a whole file can be made conditional with
`{{ if }}` alone.

Output paths are cleaned and must stay inside the project directory — a `to`
that escapes it is refused, however it was templated.

#### The review screen and next steps

Without a `[[summary]]` block the review is built from the questions, which is
usually enough. To say it differently:

```toml
[[summary]]
title = "Service"
  [[summary.rows]]
  label = "Port"
  value = "{{ .Vars.port }}"

[[summary]]
title = "Routes"
items = "{{ range .Vars.routes }}{{ . }}\n{{ end }}"   # one per line
when  = "{{ .Vars.routes }}"

[[next_steps]]
command = "./gradlew run"
note    = "listens on {{ .Vars.port }}"

[[next_steps]]
command = "docker build -t {{ kebab .Project.Name }} ."
when    = "{{ .Vars.docker }}"
```

### What a template sees

Every Go template — file contents, `to` paths, `when` conditions, summary values
— is rendered against the same context:

| Field | What it is |
| --- | --- |
| `.Project.Name` | the project name, as typed |
| `.Project.Package` | the package, when `uses_package` is set |
| `.Project.Namespace` | the name, lowercased and stripped to letters and digits |
| `.Project.TypePrefix` | the name in PascalCase |
| `.Project.Kebab` | the name in kebab-case |
| `.Vars.<id>` | one entry per question |
| `.Versions.<key>` | a resolved version |
| `.Versions.Of "kebab-key"` | the same, for a key a dot cannot address |
| `.Gradle.Version` | the resolved Gradle distribution |
| `.Item` `.Index` `.First` `.Last` | inside a `for_each` |
| `.RelPath` | inside a glob: the path under its base |
| `.Gen` | the kmp-scaffold version |

Plus these functions:

| Function | Example |
| --- | --- |
| `pascal` `camel` `kebab` `lowerAlnum` | `{{ pascal .Item }}` |
| `packagePath` | `{{ packagePath .Project.Package }}` → `com/example/app` |
| `has` | `{{ if has .Vars.extras "cors" }}` |
| `join` `split` `first` | `{{ join ", " .Vars.routes }}` |
| `upper` `lower` `trim` `replace` `quote` | `{{ upper .Project.Name }}` |
| `indent` `nindent` | `{{ nindent 4 .Vars.body }}` |
| `default` | `{{ default "8080" .Vars.port }}` |

A condition is true when it renders to anything other than empty, `false`, `0`,
`no`, `off` or an empty list.

### A mistake is an error

Unknown keys in `template.toml` are rejected by name, as are a question with no
prompt, a select with no options, two questions sharing an id, a `from` that
does not exist, and a mode that is not octal. A `when` that quietly did nothing
would be far harder to find than a message naming the line.

## What a file-based template cannot do

Worth stating plainly, because it is where the format stops and Go begins:

- **Compute a question's options from a registry.** They have to be listed.
  `disabled_when` covers greying one out based on earlier answers, which is most
  of the need.
- **Express dependency rules beyond `when` and `disabled_when`.** No fixpoint,
  no "turning this on pulls that in, and say so in the summary".
- **Hook into the resolver's compatibility rules.** It gets the resolver's
  *output* — Kotlin against KSP, AGP against `compileSdk` and the rest are not
  extensible from a manifest.
- **Add to the library catalog**, so `kmp-scaffold add library` does not apply
  to a project generated from one.
- **Fetch anything.** No Gradle wrapper jar, no downloads. This is deliberate: a
  general fetch primitive is the hole the format otherwise does not have.
- **Run commands.** There is no `run =`, and there will not be one without a
  separate opt-in. A template writes files; that is all it can do.

If you need any of those, write a Go template — see
[extending kmp-scaffold](extending.md#adding-a-template). `kmp-mobile` is the
worked example, and it is a Go template for exactly these reasons.
