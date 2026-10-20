// Package skills embeds the repository's built-in SimpleBase agent skills.
package skills

import "embed"

// Simplebase contains the skill index and all built-in sub-skills.
//
//go:embed simplebase
var Simplebase embed.FS
