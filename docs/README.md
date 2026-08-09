# Documentation

`kmp-scaffold` is a project generator. It runs a wizard, resolves the current
version of everything it is about to write, and generates a project from a
**template**. Afterwards, `kmp-scaffold add` extends that project using the same
template's recipes.

That split runs through these pages too: some describe the tool, and some
describe one particular template.

## The tool

Read these whatever you generate.

| Page | What it covers |
| --- | --- |
| [Getting started](getting-started.md) | the wizard screen by screen, and the flags that skip it |
| [Using a template](templates/using.md) | picking one, fetching one from git, reviewing and trusting it |
| [Writing a template](templates/writing.md) | the `template.toml` reference, recipes, and the format's limits |
| [Extending kmp-scaffold](extending.md) | adding a built-in template, a project layout or an anchor, in Go |
| [Troubleshooting](troubleshooting.md) | when generating, the wizard, or a remote template misbehaves |

## The default template

[**kmp-mobile**](kmp-mobile/README.md) generates a Kotlin Multiplatform project:
an Android app, an iOS app, and shared Kotlin logic underneath both. It is what
you get if you do not ask for another template, and it has its own pages,
because everything in them is a property of that template rather than of the
tool.

| Page | What it covers |
| --- | --- |
| [Overview](kmp-mobile/README.md) | what it generates, what is optional, how to build it |
| [Project layout](kmp-mobile/project-layout.md) | every module and the dependency rule between them |
| [iOS architecture](kmp-mobile/ios-architecture.md) | the local Swift package, navigation, the XCFramework |
| [Adding features](kmp-mobile/features.md) | what `add feature` writes and wires |
| [Libraries and versions](kmp-mobile/libraries.md) | library packs, version resolution, staying current |

## If you are looking for

- **"how do I generate a project?"** — [getting started](getting-started.md).
- **"what did it just generate?"** —
  [kmp-mobile](kmp-mobile/README.md), assuming you took the default.
- **"how do I add a screen?"** — [adding features](kmp-mobile/features.md).
- **"how do I generate something that is not a KMP app?"** —
  [using a template](templates/using.md), then
  [writing one](templates/writing.md).
- **"why did it pick that version?"** —
  [how resolution works](kmp-mobile/libraries.md#how-resolution-works).
- **"how do I turn off the tests / the CI workflow?"** —
  [getting started](getting-started.md#4-tests), or `--no-tests` / `--no-ci`.
- **"I said no and now I want them"** — `kmp-scaffold add tests` / `add ci`; see
  [adding them later](kmp-mobile/project-layout.md#adding-them-later).
- **"which Swift / Java / iOS version does it target?"** —
  [what you target](kmp-mobile/libraries.md#what-you-target), and
  [the Swift toolchain](kmp-mobile/ios-architecture.md#the-swift-toolchain).
- **"it will not build"** — [troubleshooting](troubleshooting.md).
