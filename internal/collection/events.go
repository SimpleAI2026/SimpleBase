// 文件说明：events.go — 数据服务上下文的领域事件（发布语言）：集合文档变更
//
// 用法
//
//	RecordEvent{ProjectID, Collection, RecordID, Action, Changed, Data}  // 写入路径在提交 / 索引之后发布
//
// 调用架构
//
//	[上游] internal/api（集合写路径发布）, internal/app（登记订阅方：realtime、trigger）
//	         │ 使用本包导出类型
//	         ▼
//	[本文件] internal/collection/events.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] model.go, fieldtypes.go, repository.go
//
// 主要符号
//
//	type RecordEvent struct {…}
//
// 事件类型属于发布方（internal-ddd-hexagonal-implementation-plan §4、决策 D5）：进程内同步分发、
// 不持久化、不承诺 exactly-once；消费方（自动化 trigger、实时 realtime）由 app 接线，
// 不反向依赖 HTTP 包。

package collection

// RecordEvent 是集合文档发生变更后由写入路径发布的领域事件。
// Action 取值：create / update / delete（批量导入成功的记录逐条按 create 发布）。
type RecordEvent struct {
	ProjectID  string
	Collection string
	RecordID   string
	Action     string
	// Changed 是 update / patch 实际变化的字段名。
	Changed []string
	// Data 是变更后的文档；是否携带由发布路径决定（批量导入可能省略）。
	Data map[string]any
}
