// 文件说明：message.go — gosandbox/proto 中 FileKind、Entry 等符号的实现
//
// 用法
//
//	type FileKind uint8  // FileKind 是目录项的类型
//	type Entry struct{…}  // Entry 是一个目录项
//	type Stats struct{…}  // Stats 是沙盒的用量快照
//	type Msg struct{…}  // Msg 是一帧的内容
//
// 调用架构
//
//	[上游] gosandbox/agent, gosandbox/box, gosandbox/vm/box
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/proto/message.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] codec.go, codec_contract_test.go, codec_test.go, frame.go, proto.go
//	[关键引用] Entry ← codec.go, codec_test.go；Stats ← codec_test.go；Msg ← codec.go, codec_contract_test.go
//
// 主要符号
//
//	type FileKind uint8
//	const FileRegular / FileDir / FileSymlink / FileOther
//	type Entry struct{…}
//	type Stats struct{…}
//	type Msg struct{…}

package proto

// FileKind 是目录项的类型。
type FileKind uint8

const (
	FileRegular FileKind = iota + 1
	FileDir
	FileSymlink
	FileOther
)

// Entry 是一个目录项。
type Entry struct {
	Name   string
	Kind   FileKind
	Size   int64
	Mode   uint32
	ModNS  int64
	Target string // 符号链接的目标；其它类型为空
}

// Stats 是沙盒的用量快照。
type Stats struct {
	MemoryBytes    int64
	MemoryPeak     int64
	CPUUsec        int64
	Pids           int64
	OOMKills       int64
	DiskReadBytes  int64
	DiskWriteBytes int64
	// DiskUsedBytes 是沙盒可写部分实际占用的字节数（upper + ws）。
	//
	// 由宿主侧驱动计算并填进 Stats：agent 在沙盒里看到的是 overlay 的
	// 合并视图，分不出「镜像本来就有」与「用户写入」，算不准。
	// 这与 DiskWriteBytes（io 带宽）不是一回事。
	DiskUsedBytes int64
}

// Msg 是一帧的内容。只有 Kind 对应的字段有效；其余为零值。
//
// 每帧只编码它用到的字段（见 Encode 的分支），所以 Output 这种高频帧
// 不会被无关字段撑大。
type Msg struct {
	Kind Kind

	// 进程 id 与文件请求 id 共用；文件请求用 Seq 关联回复。
	ID  string
	Seq uint32

	// Hello
	Version uint16
	Agent   string
	BootNS  int64

	// Ping / Pong
	Nonce uint64

	// Error
	Err string

	// Start
	Argv    []string
	Env     []string
	Cwd     string
	User    string // "uid:gid"；空表示继承 agent
	TTY     bool
	Rows    uint16
	Cols    uint16
	Stdin   bool
	Rlimits []Rlimit

	// Started
	PID int32

	// Output / Input / RelayData
	Stream Stream
	Data   []byte

	// Exited
	Reason ExitReason
	Code   int32
	Sig    int32

	// InputConsumed
	Bytes uint32

	// Signal
	Signal Signal

	// Resize
	// Rows / Cols 复用 Start 的字段

	// 文件
	Path    string
	To      string
	Offset  uint64
	Mode    uint32
	Size    int64
	ModNS   int64
	FileKnd FileKind
	Link    string
	Entries []Entry

	// 观测
	Stats Stats
	Lines []string
	Ports []uint16

	// Relay
	ConnID uint32
	Port   uint16

	// Watch
	Recursive bool

	// Flags 是位标记：半关闭方向等。
	Flags uint8
}
