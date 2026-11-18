// 文件说明：watch.go — envd 的目录监听消息（WatchDir / CreateWatcher /
// GetWatcherEvents / RemoveWatcher）
//
// E2B 的 envd 用「watcher」把「订阅」与「取事件」分成两步：CreateWatcher
// 建一个带 id 的订阅，GetWatcherEvents 按 id 拉事件。这与 inotify 的
// 「一个 fd 就是一路订阅」不冲突：服务端把两者对应起来即可。
//
// WatchDir 是 CreateWatcher + 立刻返回第一批事件的便捷形式，CLI 常用它。
//
// 调用架构
//
//	[上游] internal/api（e2b_envd_handler）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/e2bproto/watch.go
//	         │
//	         ▼
//	[下游] 仅标准库
//	[同包] messages.go, envelope.go
//
// 主要符号
//
//	type WatchRequest struct {…}
//	type WatchCreateResponse struct {…}
//	type WatchEventsRequest struct {…}
//	type WatchEventsResponse struct {…}
//	type WatchRemoveRequest struct {…}
//	type WatchRemoveResponse struct {…}

package e2bproto

// WatchRequest 是 WatchDir 与 CreateWatcher 的请求体。
type WatchRequest struct {
	// Path 是要监听的目录（沙盒内绝对路径）。
	Path string `json:"path"`
	// Recursive 为真时连子目录一起看。
	Recursive bool `json:"recursive,omitempty"`
}

// WatchCreateResponse 是 CreateWatcher 的回执。
type WatchCreateResponse struct {
	// WatcherID 是后续拉事件用的句柄。
	WatcherID string `json:"watcherID"`
}

// WatchEventsRequest 按 watcher 拉事件。
type WatchEventsRequest struct {
	WatcherID string `json:"watcherID"`
	// Limit 是最多返回多少条；0 = 由服务端决定。
	Limit uint32 `json:"limit,omitempty"`
}

// WatchEvent 是一次文件变化。字段名与 envd 的 FileEvent 一致。
type WatchEvent struct {
	// Path 是变化的路径（沙盒内视角）。
	Path string `json:"path"`
	// Type 是 "create" / "write" / "remove" / "rename" / "chmod"。
	Type string `json:"type"`
	// IsDir 为真表示变化的对象是目录。
	IsDir bool `json:"isDir,omitempty"`
}

// WatchEventsResponse 是一批事件。
type WatchEventsResponse struct {
	Events []WatchEvent `json:"events"`
}

// WatchRemoveRequest 取消一个 watcher。
type WatchRemoveRequest struct {
	WatcherID string `json:"watcherID"`
}

// WatchRemoveResponse 是空响应。
type WatchRemoveResponse struct{}
