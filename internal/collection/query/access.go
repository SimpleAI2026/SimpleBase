// 文件说明：access.go — 为行级访问规则生成 owner 过滤条件。
//
// 用法
//
//	cond := query.OwnerCondition(dialect, "owner_id") // json_extract_string(data,'$.owner_id') = ?
//
// 调用架构
//
//	[上游] internal/api/datahttp 的集合列表 / 导出 / 聚合
//	         │ 调用本文件
//	         ▼
//	[本文件] internal/collection/query/access.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase.Dialect
//
// 主要符号
//
//	OwnerCondition

package query

import "github.com/linkxzhou/SimpleBase/godatabase"

// OwnerCondition 返回 owner 字段等于参数的 SQL 条件，参数由调用方追加。
// ownerField 必须已通过 collection.FieldNamePattern 校验，避免把用户输入拼进 JSON 路径。
func OwnerCondition(dialect godatabase.Dialect, ownerField string) string {
	return dialect.JSONExtractText("$."+ownerField) + " = ?"
}
