// 文件说明：ptyx.go — 伪终端（PTY）的打开与窗口大小设置，供 gosandbox 的宿主侧（host）与沙盒内 agent 共用
//
// 用法
//
//	master, slave, err := ptyx.Open()              // 打开一对伪终端；不设置窗口大小
//	err := ptyx.SetWinsize(master, rows, cols)     // 设置窗口大小；对 master 或 slave 调用效果相同
//
// 调用架构
//
//	[上游] gosandbox/host（start_posix.go 的 attachStdio / Resize）、gosandbox/agent（proc.go 的 startWith / resize）
//	         │
//	         ▼
//	[本包] ptyx.Open       → ptyx_linux.go / ptyx_darwin.go / ptyx_other.go
//	       ptyx.SetWinsize → ptyx_unix.go / ptyx_other.go
//	[下游] 仅标准库 + golang.org/x/sys/unix；不 import gosandbox 的其它包
//
// 窗口大小的策略留给调用方，本包不替它们决定：
//   - host 总是设置，失败就关闭两端并报 unavailable；
//   - agent 只在 rows、cols 都大于 0 时设置，失败忽略。
//
// 本包不是对外 API。打开 /dev/ptmx 时不显式传 O_CLOEXEC：os.OpenFile 在 unix 上总会追加它。

package ptyx

import "errors"

// ErrUnsupported 表示当前平台既不是 linux 也不是 darwin，伪终端不可用。
var ErrUnsupported = errors.New("ptyx: pseudo terminals are only available on linux and darwin")
