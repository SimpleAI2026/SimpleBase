// 文件说明：embed.go — go:embed 内嵌 SimpleBase 技能文档

package skills

import "embed"

// Simplebase contains the skill index and all built-in sub-skills.
//
//go:embed simplebase
var Simplebase embed.FS
