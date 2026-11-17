// 文件说明：ports_host.go — 宿主注入的日志 / 命令策略 / 设置端口

package cloudagent

import (
	"context"

	"go.uber.org/zap"
)

// Logger 是本模块需要的日志出口（元数据层与内容层共用）。
//
// 宿主 observability.Logger 的方法签名与之兼容（Field 是 zap.Field 别名），
// 可直接传入；装配侧不要把 nil *zapLogger 装进非 nil 接口，否则 panic。
type Logger interface {
	Debug(msg string, fields ...zap.Field)
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
}

// CommandDecision 是一条 simplebase 命令的安全分类结果。
type CommandDecision struct {
	// ReadOnly 表示命令不改变任何资源（事实来源是宿主的 sbcli 只读表）。
	ReadOnly bool
	// Destructive 表示执行前必须取得界面确认。
	Destructive bool
}

// CommandPolicy 是宿主命令语义的唯一入口。
//
// Classify 与 RedactArgv 的规则全部来自宿主的 sbcli / sqlguard：模块内不得
// 复制第二份只读允许表或 SQL 解析器。策略缺失或返回错误时，调用方必须拒绝
// 执行（fail-closed），不能退回「确认后执行」。
type CommandPolicy interface {
	Classify(argv []string, stdin string) (CommandDecision, error)
	// RedactArgv 返回可展示的 argv 副本：敏感值替换为 ***，SQL 只留首关键字。
	RedactArgv(argv []string) []string
}

// SettingsAccess 读取项目级 LLM 默认设置（不含密钥）。
type SettingsAccess interface {
	LLMSettings(ctx context.Context, projectID string) (LLMSettings, error)
}
