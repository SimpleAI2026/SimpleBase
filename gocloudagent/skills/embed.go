// 文件说明：embed.go — embeds the repository's built-in SimpleBase agent skills
//
// 用法
//
//	包内实现细节，由上层包经包级 API 间接触达。
//
// 调用架构
//
//	[上游] gocloudagent
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/skills/embed.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//
// 主要符号
//
//	var Simplebase
//	Package skills embeds the repository's built-in SimpleBase agent skills.

package skills

import "embed"

// Simplebase contains the skill index and all built-in sub-skills.
//
//go:embed simplebase
var Simplebase embed.FS
