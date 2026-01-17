// Package skills holds the agent skills that ship with indexit.
package skills

import (
	"embed"
	"io/fs"
)

//go:generate go run ../internal/skill/gen indexit/references/cli.md

//go:embed indexit
var files embed.FS

// Indexit returns the indexit skill with SKILL.md at its root.
func Indexit() fs.FS {
	sub, err := fs.Sub(files, "indexit")
	if err != nil {
		panic(err)
	}
	return sub
}
