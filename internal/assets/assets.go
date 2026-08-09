// Package assets embeds the Go templates the built-in project templates render.
//
// Each *.tmpl file contains many {{define "name"}} blocks; the name is the id
// that generators render by. Grouping them this way keeps related output
// together and makes it easy to see a whole module's files at once.
//
// The package is called "assets" rather than "templates" because a *template*
// is now the user-facing concept of a whole generatable project (see
// internal/scaffold). These are the raw files one of those renders.
package assets

import "embed"

//go:embed *.tmpl
var files embed.FS

// FS returns the embedded template filesystem.
func FS() embed.FS { return files }
