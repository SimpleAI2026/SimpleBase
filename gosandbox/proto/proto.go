// Package proto 是宿主与沙盒内 agent 之间的线协议。
//
// 帧格式：len u32 LE | kind u16 LE | payload，len 计 kind + payload。
// 大块数据（文件写入）跟在帧之后以裸字节发送，不复制进帧：
//
//	FsWrite{Path, Offset, Len} + 紧随其后的 Len 字节
//
// 这样 1 MiB 的输出块、几 MB 的文件写入都不会在两端各多复制一次。
package proto

// 协议常量。改动其中任何一个都要同时改两侧并升 Version。
const (
	// Version 是线协议版本；Hello 里带上，两侧不一致直接拒绝。
	//
	// v2：Ack / FsWriteAck / Error 带上 Seq（宿主按 Seq 关联请求，缺了它们
	// 所有文件操作都会等到超时）；RelayClose 带上 Flags（区分「连不上」与
	// 「对端正常关闭」）。
	Version uint16 = 2
	// MaxFrame 是单帧上限（1 MiB）。超过视为协议错误，避免对端无限申请内存。
	MaxFrame = 1 << 20
	// Header 是 payload 之前的字节数：len(4) + kind(2)。
	Header = 6
	// MaxOutputChunk 是单条 Output 的负载上限，保证帧远小于 MaxFrame。
	MaxOutputChunk = 64 << 10
	// InputWindow 是宿主可超前发送给某个进程的 stdin 字节数。沙盒内进程不读时
	// 由 agent 缓冲到这个量为止，宿主随后阻塞调用方，而不是无限占用内存。
	InputWindow = 4 << 20
	// MaxPathLen / MaxArgLen 是字段长度上限，防止构造的帧撑爆内存。
	MaxPathLen = 4096
	MaxArgLen  = 64 << 10
	// MaxArgs / MaxEnv 是单个请求里 argv 与 env 的条目上限。
	MaxArgs = 4096
	MaxEnv  = 4096
)

// Kind 是帧类型。取值一旦发布就不要复用。
type Kind uint16

const (
	// 会话：连接建立后的第一帧是 Hello；任一侧可发 Ping，对端回 Pong。
	KindHello Kind = iota + 1
	KindPing
	KindPong
	KindShutdown
	// KindError 是 agent → 宿主的失败说明；KindAck 是成功且无其它内容可回。
	KindError
	KindAck

	// 进程：Start → Started → Output* → Exited。
	// Signal / Resize / CloseStdin 无回复；Input 无回复，靠 InputConsumed 流控。
	KindStart
	KindStarted
	KindOutput
	KindExited
	KindInput
	KindInputConsumed
	KindCloseStdin
	KindResize
	KindSignal

	// 文件：请求均带 ID 与路径，回复带同一 ID。
	KindFsStat
	KindFsStatReply
	KindFsList
	KindFsListReply
	KindFsRead
	KindFsReadReply
	KindFsWrite
	KindFsRemove
	KindFsMkdir
	KindFsRename
	// KindFsWriteAck 是 FsWrite 的回复（成功或错误单独走 KindError）。
	KindFsWriteAck

	// 目录监听：Watch 注册，事件以 WatchEvent 推送，Unwatch 取消。
	KindWatch
	KindUnwatch
	KindWatchEvent

	// 观测。
	KindStatsQuery
	KindStats
	KindTopQuery
	KindTop
	KindListenQuery
	KindListening

	// 入站连接中继：Relay{ConnID, Port} → agent 连 127.0.0.1:Port，
	// 之后该 ConnID 的 Data / Close 双向流动。
	//
	// agent 连上后先回一个**没有数据**的 RelayData（宿主据此让 DialPort 立刻
	// 返回，不必等服务端先开口），连不上则回带 RelayConnectFailed 的 RelayClose。
	KindRelay
	KindRelayData
	// KindRelayHalfClose 表示「我这个方向不再发数据，但仍能收」。
	// 隧道模式靠它区分「请求发完了」与「整条连接断」：
	// HTTP/1.1 的半关闭、以及 TLS 的 close_notify 都依赖这个语义。
	KindRelayHalfClose
	KindRelayClose
)

// RelayClose 帧的 Flags 取值。
const (
	// RelayConnectFailed 表示 agent 连不上沙盒内的端口（没人 listen）。
	// 宿主据此把 DialPort 判为失败，而不是「连上了但立刻 EOF」。
	RelayConnectFailed uint8 = 1
)

// Stream 区分 stdout 与 stderr；PTY 模式下只有 Stdout。
type Stream uint8

const (
	StreamStdout Stream = 1
	StreamStderr Stream = 2
)

// String 便于日志。
func (s Stream) String() string {
	switch s {
	case StreamStdout:
		return "stdout"
	case StreamStderr:
		return "stderr"
	}
	return "unknown"
}

// Signal 是 Signal 帧里的信号编号（与 syscall 数值一致）。
// 用 int32 而不是 syscall.Signal，避免非 POSIX 平台的编译问题。
type Signal int32

// Rlimit 是一条资源上限。Name 取 "nofile" / "nproc" / "core" / "fsize" / "as" / "stack"。
type Rlimit struct {
	Name string
	Cur  uint64
	Max  uint64
}

// ExitReason 说明进程为什么结束。
type ExitReason uint8

const (
	// ExitCode 是正常退出，Code 有效。
	ExitCode ExitReason = iota + 1
	// ExitSignaled 是被信号终止，Signal 有效。
	ExitSignaled
	// ExitOOM 是沙盒内 OOM 被杀（cgroup 的 memory.events 记到它）。
	ExitOOM
)
