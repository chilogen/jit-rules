package assets

import "embed"

//go:embed guide.md defaults.yaml unit-template.md harnesses/*.yaml packs
var FS embed.FS
