// Package templates embeds the project templates.
//
// Each *.tmpl file contains many {{define "name"}} blocks; the name is the
// template id that generators render by. Grouping them this way keeps related
// output together and makes it easy to see a whole module's files at once.
package templates

import "embed"

//go:embed *.tmpl
var files embed.FS

// FS returns the embedded template filesystem.
func FS() embed.FS { return files }
