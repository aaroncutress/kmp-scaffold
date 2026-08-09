# Using a template

`kmp-scaffold new` generates from a **template**. A template owns a whole
project: the questions it asks, the versions it resolves, and the files it
writes.

`kmp-mobile` — the Kotlin Multiplatform project documented under
[kmp-mobile/](../kmp-mobile/README.md) — is one template, and the default. This
page is about picking, fetching and trusting others. To write one, see
[writing a template](writing.md).

## Picking one

```bash
kmp-scaffold templates                    # what is available
kmp-scaffold templates ktor-service       # what one asks, and what it writes
kmp-scaffold new my-api --template ktor-service
```

`--template` takes any of:

| Ref | Means |
| --- | --- |
| `kmp-mobile` | a built-in template |
| `ktor-service` | one in your templates folder, or one already fetched |
| `./templates/my-thing` | a directory, relative or absolute |
| `github:owner/repo` | a git repository — see [below](#templates-from-a-git-repository) |

When more than one template is available, the wizard asks which to use before
anything else — the answer decides what the rest of the questions are.

Whichever built the project is recorded in its `.kmp-scaffold.json`, so a later
`kmp-scaffold add` extends it the way it was made — and for a remote template,
at the exact commit it was built from, not at whatever the branch points at
today.

## Templates from a git repository

```bash
kmp-scaffold templates add github:acme/templates          # fetch and review it
kmp-scaffold new my-api --template ktor-service           # then use it by name
```

or in one step, `--template github:acme/templates`. The forms a ref takes:

| Ref | Means |
| --- | --- |
| `github:acme/templates` | the default branch |
| `github:acme/templates@v2` | a tag, branch or commit |
| `github:acme/templates/services/ktor` | a directory inside the repository |
| `gitlab:acme/templates@main` | GitLab |
| `https://git.example.com/t/tmpl.git//service@v2` | any git remote; `//` separates the directory |
| `git@github.com:acme/templates.git@v2` | over SSH |
| `file:///srv/templates` | a repository on this machine (`file://C:\src\templates` on Windows) |

Fetching shells out to `git`, so it uses your existing credentials and says so
plainly if `git` is not installed.

### Reviewing what you fetch

A template writes files into your project, so the first time a given **commit**
is used you are shown what it would do and asked:

```
A template from the internet

  Template   Ktor service  (ktor-service)
  From       https://github.com/acme/templates.git
  Commit     d722a1a14af0b4dbc96121b12b4ef4a27e66f31b
  Writes     6 file(s), and inserts into existing ones in 2 place(s)
  Can add    route
  Runs       nothing - templates cannot execute commands

Use this template? [y/N]
```

Say yes and that commit is recorded in `~/.config/kmp-scaffold/trusted.json`.
The answer is about the commit, not the repository: the same template at a later
commit is a different set of files, and is asked about again.

`--trust` accepts without asking, for CI. It is deliberately **not** implied by
`--yes` — "do not ask me the wizard's questions" and "run files from the
internet without looking" are different decisions, and a script should not
acquire the second by asking for the first.

The strongest part of this is what a template *cannot* do: there is no `run =`,
no shell hook, no post-generate step. A template writes files and inserts at
anchors. That is why the prompt can honestly say "Runs nothing".

### The cache

Fetched templates go under `${XDG_CACHE_HOME:-~/.cache}/kmp-scaffold/templates/`,
keyed by the commit they resolved to. A tag or a commit is never re-fetched; a
branch is re-checked once a day, or immediately with `--refresh`. `--offline`
never touches the network, and says so if what you asked for is not cached.

```bash
kmp-scaffold templates                       # built-in, yours, and fetched
kmp-scaffold templates add <ref> --refresh   # take the latest commit
kmp-scaffold templates remove <ref>          # drop it, and forget it was reviewed
```

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

## Writing your own

A template is a directory with a `template.toml` and a tree of Go templates —
no compiler and no Go. See [writing a template](writing.md) for the reference,
and [`examples/templates/ktor-service`](../../examples/templates/ktor-service)
for a complete worked one.
