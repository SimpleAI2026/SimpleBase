// 文件说明：messages.go — internal/e2bproto 中 Bytes、MarshalJSON 等符号的实现
//
// 用法
//
//	(b Bytes) MarshalJSON() ([]byte, error)
//	(b *Bytes) UnmarshalJSON(data []byte) error
//
// 调用架构
//
//	[上游] internal/api
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/e2bproto/messages.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] envelope.go
//
// 主要符号
//
//	type Bytes []byte
//	(b Bytes) MarshalJSON() ([]byte, error)
//	(b *Bytes) UnmarshalJSON(data []byte) error
//	type PTYSize struct {…}
//	type PTY struct {…}
//	type ProcessConfig struct {…}
//	... 另有 43 个顶层符号

package e2bproto

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// ---- process.Process ----

// Bytes 在 JSON 里是 base64 字符串（protobuf-JSON 规范）。
type Bytes []byte

func (b Bytes) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.StdEncoding.EncodeToString(b))
}

func (b *Bytes) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	// 容忍 URL-safe 与无填充写法。
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if out, err := enc.DecodeString(s); err == nil {
			*b = out
			return nil
		}
	}
	return fmt.Errorf("e2bproto: invalid base64 bytes")
}

// PTYSize 对应 process.PTY.Size。
type PTYSize struct {
	Cols uint32 `json:"cols"`
	Rows uint32 `json:"rows"`
}

// PTY 对应 process.PTY。
type PTY struct {
	Size PTYSize `json:"size"`
}

// ProcessConfig 对应 process.ProcessConfig。
type ProcessConfig struct {
	Cmd  string            `json:"cmd"`
	Args []string          `json:"args,omitempty"`
	Envs map[string]string `json:"envs,omitempty"`
	Cwd  *string           `json:"cwd,omitempty"`
}

// ProcessSelector 对应 process.ProcessSelector（oneof pid | tag）。
type ProcessSelector struct {
	PID *uint32 `json:"pid,omitempty"`
	Tag *string `json:"tag,omitempty"`
}

// StartRequest 对应 process.StartRequest。
type StartRequest struct {
	Process ProcessConfig `json:"process"`
	PTY     *PTY          `json:"pty,omitempty"`
	Tag     *string       `json:"tag,omitempty"`
	Stdin   *bool         `json:"stdin,omitempty"`
}

// ConnectRequest 对应 process.ConnectRequest。
type ConnectRequest struct {
	Process ProcessSelector `json:"process"`
}

// ProcessInput 对应 process.ProcessInput（oneof stdin | pty）。
type ProcessInput struct {
	Stdin Bytes `json:"stdin,omitempty"`
	PTY   Bytes `json:"pty,omitempty"`
}

// SendInputRequest 对应 process.SendInputRequest。
type SendInputRequest struct {
	Process ProcessSelector `json:"process"`
	Input   ProcessInput    `json:"input"`
}

// SendSignalRequest 对应 process.SendSignalRequest；signal 是枚举，JSON 里可能是名字或数字。
type SendSignalRequest struct {
	Process ProcessSelector `json:"process"`
	Signal  Signal          `json:"signal"`
}

// CloseStdinRequest 对应 process.CloseStdinRequest。
type CloseStdinRequest struct {
	Process ProcessSelector `json:"process"`
}

// UpdateRequest 对应 process.UpdateRequest（调整 PTY 大小）。
type UpdateRequest struct {
	Process ProcessSelector `json:"process"`
	PTY     *PTY            `json:"pty,omitempty"`
}

// Signal 对应 process.Signal 枚举。
type Signal int

const (
	SignalUnspecified Signal = 0
	SignalSIGKILL     Signal = 9
	SignalSIGTERM     Signal = 15
)

func (s *Signal) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*s = Signal(n)
		return nil
	}
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return err
	}
	switch name {
	case "SIGNAL_SIGKILL":
		*s = SignalSIGKILL
	case "SIGNAL_SIGTERM":
		*s = SignalSIGTERM
	case "SIGNAL_UNSPECIFIED", "":
		*s = SignalUnspecified
	default:
		return fmt.Errorf("e2bproto: unknown signal %q", name)
	}
	return nil
}

// StartEvent / DataEvent / EndEvent / KeepAlive 对应 process.ProcessEvent 的各分支。
type StartEvent struct {
	PID uint32 `json:"pid"`
}

type DataEvent struct {
	Stdout Bytes `json:"stdout,omitempty"`
	Stderr Bytes `json:"stderr,omitempty"`
	PTY    Bytes `json:"pty,omitempty"`
}

type EndEvent struct {
	ExitCode int32   `json:"exitCode"`
	Exited   bool    `json:"exited"`
	Status   string  `json:"status"`
	Error    *string `json:"error,omitempty"`
}

type KeepAlive struct{}

// ProcessEvent 对应 process.ProcessEvent（oneof event）。同一时刻只设置一个分支。
type ProcessEvent struct {
	Start     *StartEvent `json:"start,omitempty"`
	Data      *DataEvent  `json:"data,omitempty"`
	End       *EndEvent   `json:"end,omitempty"`
	KeepAlive *KeepAlive  `json:"keepalive,omitempty"`
}

// StartResponse / ConnectResponse 都只含一个 event。
type StartResponse struct {
	Event ProcessEvent `json:"event"`
}

type ConnectResponse = StartResponse

// ProcessInfo 对应 process.ProcessInfo。
type ProcessInfo struct {
	Config ProcessConfig `json:"config"`
	PID    uint32        `json:"pid"`
	Tag    *string       `json:"tag,omitempty"`
}

// ListResponse 对应 process.ListResponse。
type ListResponse struct {
	Processes []ProcessInfo `json:"processes"`
}

// ---- filesystem.Filesystem ----

// FileType 对应 filesystem.FileType 枚举；JSON 用名字。
type FileType string

const (
	FileTypeUnspecified FileType = "FILE_TYPE_UNSPECIFIED"
	FileTypeFile        FileType = "FILE_TYPE_FILE"
	FileTypeDirectory   FileType = "FILE_TYPE_DIRECTORY"
	FileTypeSymlink     FileType = "FILE_TYPE_SYMLINK"
)

// Int64 在 JSON 里是字符串（protobuf-JSON 规范：int64 不能用数字，避免 JS 精度丢失）。
type Int64 int64

func (n Int64) MarshalJSON() ([]byte, error) { return json.Marshal(strconv.FormatInt(int64(n), 10)) }

func (n *Int64) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		*n = Int64(v)
		return nil
	}
	var v int64
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*n = Int64(v)
	return nil
}

// EntryInfo 对应 filesystem.EntryInfo。
type EntryInfo struct {
	Name          string   `json:"name"`
	Type          FileType `json:"type"`
	Path          string   `json:"path"`
	Size          Int64    `json:"size"`
	Mode          uint32   `json:"mode"`
	Permissions   string   `json:"permissions"`
	Owner         string   `json:"owner"`
	Group         string   `json:"group"`
	ModifiedTime  string   `json:"modifiedTime,omitempty"` // RFC3339（protobuf Timestamp 的 JSON 形式）
	SymlinkTarget *string  `json:"symlinkTarget,omitempty"`
	IsSymlink     bool     `json:"isSymlink,omitempty"`
}

// FormatTimestamp 把时间格式化为 protobuf Timestamp 的 JSON 形式。
func FormatTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// PermString 把权限位格式化为 "-rw-r--r--" 风格的字符串（envd 的 permissions 字段）。
func PermString(mode uint32, dir bool) string {
	const rwx = "rwxrwxrwx"
	b := make([]byte, 10)
	if dir {
		b[0] = 'd'
	} else {
		b[0] = '-'
	}
	for i := 0; i < 9; i++ {
		if mode&(1<<uint(8-i)) != 0 {
			b[i+1] = rwx[i]
		} else {
			b[i+1] = '-'
		}
	}
	return string(b)
}

type StatRequest struct {
	Path string `json:"path"`
}
type StatResponse struct {
	Entry EntryInfo `json:"entry"`
}
type MakeDirRequest struct {
	Path string `json:"path"`
}
type MakeDirResponse struct {
	Entry EntryInfo `json:"entry"`
}
type MoveRequest struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}
type MoveResponse struct {
	Entry EntryInfo `json:"entry"`
}
type ListDirRequest struct {
	Path  string `json:"path"`
	Depth uint32 `json:"depth,omitempty"`
}
type ListDirResponse struct {
	Entries []EntryInfo `json:"entries"`
}
type RemoveRequest struct {
	Path string `json:"path"`
}
type RemoveResponse struct{}
