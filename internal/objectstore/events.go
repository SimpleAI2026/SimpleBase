// 文件说明：events.go — 文件上下文的领域事件（发布语言）：对象上传 / 删除
//
// 用法
//
//	ObjectEvent{ProjectID, Key, Action}  // 文件写路径在存储写入成功后发布
//
// 调用架构
//
//	[上游] internal/api（对象上传 / 删除路径发布）, internal/app（登记订阅方：trigger）
//	         │ 使用本包导出类型
//	         ▼
//	[本文件] internal/objectstore/events.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] filestore.go, keys.go 等
//
// 主要符号
//
//	type ObjectEvent struct {…}
//
// 事件类型属于发布方（internal-ddd-hexagonal-implementation-plan §4、决策 D5）：进程内同步分发、
// 不持久化；Key 是用户可见的相对 key，不是物理对象键。

package objectstore

// ObjectEvent 是用户文件发生变更后由写路径发布的领域事件。Action 取值：upload / delete。
type ObjectEvent struct {
	ProjectID string
	Key       string
	Action    string
}
