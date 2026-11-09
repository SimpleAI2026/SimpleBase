// 文件说明：command.go — internal/agentbridge 中 CommandPolicy、NewCommandPolicy 等符号的实现
//
// 用法
//
//	NewCommandPolicy() CommandPolicy
//	DestructiveArgv(argv []string, stdin string) bool  // DestructiveArgv 判断是否破坏性命令（planv6.0 D3）
//	ArgvFlag(argv []string, name string) (string, bool)  // ArgvFlag 读取 --name value / --name=value
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/command.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent, godatabase/sqlguard, internal/sbcli, internal/utils/mask
//	[同包] cli.go, cron_agent.go, delegation.go, errors.go, principal.go 等 6 个
//
// 主要符号
//
//	type CommandPolicy struct {…}
//	NewCommandPolicy() CommandPolicy
//	DestructiveArgv(argv []string, stdin string) bool
//	ArgvFlag(argv []string, name string) (string, bool)
//	SQLDestructive(stmt string) bool
//	RedactArgv(argv []string) []string
//	... 另有 3 个顶层符号

package agentbridge

import (
	"fmt"
	"strings"

	"github.com/linkxzhou/SimpleBase/gocloudagent"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlguard"
	"github.com/linkxzhou/SimpleBase/internal/sbcli"
	"github.com/linkxzhou/SimpleBase/internal/utils/mask"
)

// CommandPolicy 是宿主命令语义的实现：只读判定来自 sbcli，SQL 语义来自
// godatabase/sqlguard，展示脱敏沿用 planv6.0 §3.5 的规则。
//
// gocloudagent 侧只持有接口，不复制规则；策略缺失或出错时调用方必须拒绝执行。
type CommandPolicy struct{}

// NewCommandPolicy 返回默认策略。无状态，可重复构造。
func NewCommandPolicy() CommandPolicy { return CommandPolicy{} }

// Classify 给出一条 simplebase 命令的安全分类。
func (CommandPolicy) Classify(argv []string, stdin string) (gocloudagent.CommandDecision, error) {
	if len(argv) == 0 {
		return gocloudagent.CommandDecision{}, fmt.Errorf("empty argv")
	}
	return gocloudagent.CommandDecision{
		ReadOnly:    sbcli.CommandReadOnlyWithArgv(argv),
		Destructive: DestructiveArgv(argv, stdin),
	}, nil
}

// RedactArgv 返回确认事件里可展示的 argv 副本。
func (CommandPolicy) RedactArgv(argv []string) []string { return RedactArgv(argv) }

// DestructiveArgv 判断是否破坏性命令（planv6.0 D3）。
// stdin 是工具参数内联的内容；宿主机文件不可读，分析对象只有 argv 与 stdin。
func DestructiveArgv(argv []string, stdin string) bool {
	if len(argv) < 2 {
		return false
	}
	switch argv[0] + " " + argv[1] {
	case "database delete", "document delete", "object delete", "function delete",
		"cron delete", "cron trigger", "sandbox delete", "user delete", "user disable",
		"apikey revoke", "settings set":
		return true
	case "log retention":
		return len(argv) >= 3 && argv[2] == "set"
	case "kv exec":
		cmd, ok := ArgvFlag(argv, "command")
		return ok && strings.EqualFold(cmd, "DEL")
	case "sql exec":
		stmt, ok := ArgvFlag(argv, "statement")
		if !ok {
			return true
		}
		if stmt == "-" {
			stmt = stdin
		}
		return SQLDestructive(stmt)
	case "sql batch":
		// planv6.0 D3/--statement 内联优先；--file 只能是 "-"（stdin）。
		if stmt, ok := ArgvFlag(argv, "statement"); ok {
			return SQLDestructive(stmt)
		}
		if path, ok := ArgvFlag(argv, "file"); ok && path == "-" {
			return SQLDestructive(stdin)
		}
		return true
	case "database create":
		path, ok := ArgvFlag(argv, "init-sql")
		if !ok {
			return false
		}
		if path == "-" {
			return SQLDestructive(stdin)
		}
		return SQLDestructive(path)
	default:
		return false
	}
}

// ArgvFlag 读取 --name value / --name=value。
func ArgvFlag(argv []string, name string) (string, bool) {
	flag := "--" + name
	for i := 0; i < len(argv); i++ {
		if argv[i] == flag && i+1 < len(argv) {
			return argv[i+1], true
		}
		if strings.HasPrefix(argv[i], flag+"=") {
			return strings.TrimPrefix(argv[i], flag+"="), true
		}
	}
	return "", false
}

// SQLDestructive 按 sqlguard 切分后逐句判定（planv6.0 §3.5：复用认识引号与
// 注释的切分，不在别处再写一套）。
func SQLDestructive(stmt string) bool {
	parts, err := sqlguard.SplitStatements(stmt)
	if err != nil || len(parts) == 0 {
		return true
	}
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if err := sqlguard.Validate(p, sqlguard.ReadOnly); err == nil {
			continue
		}
		switch sqlguard.FirstKeyword(p) {
		case "INSERT", "UPDATE", "CREATE", "REPLACE":
			continue
		case "ALTER":
			if strings.Contains(strings.ToUpper(p), "DROP") {
				return true
			}
			continue
		case "DROP", "DELETE", "TRUNCATE":
			return true
		default:
			return true
		}
	}
	return false
}

// RedactArgv 生成确认事件里的脱敏 argv（planv6.0 §3.5）：
//   - --statement 只保留首关键字 + 前 200 字符（防确认框泄漏大段 SQL/数据）；
//   - kv exec 的 --arg 除第一个（key）以外全部替换 ***；
//   - 匹配敏感词的 flag 值替换 ***。
//
// 同时支持 "--k v" 与 "--k=v" 两种形式。
func RedactArgv(argv []string) []string {
	out := make([]string, len(argv))
	copy(out, argv)
	kvArgSeen := 0
	for i := 0; i < len(out); i++ {
		name, inlineValue, isFlag := splitFlag(out[i])
		if !isFlag {
			continue
		}
		// 取值：内联（--k=v）或下一个非 flag 元素（--k v）。
		valueIdx := -1
		value := ""
		if inlineValue != nil {
			value = *inlineValue
		} else if i+1 < len(out) && !strings.HasPrefix(out[i+1], "--") {
			valueIdx = i + 1
			value = out[valueIdx]
		}
		if value == "" && inlineValue == nil {
			continue
		}
		switch {
		case name == "statement":
			if inlineValue != nil {
				out[i] = "--statement " + RedactStatement(*inlineValue)
			} else {
				out[valueIdx] = RedactStatement(value)
			}
		case name == "arg" && isKVExec(argv):
			kvArgSeen++
			// 第一个 --arg 是 key，保留；其余替换。
			if kvArgSeen > 1 {
				if inlineValue != nil {
					out[i] = "--arg=***"
				} else {
					out[valueIdx] = "***"
				}
			}
		case mask.IsSensitiveKey(name):
			if inlineValue != nil {
				out[i] = "--" + name + "=***"
			} else if valueIdx >= 0 {
				out[valueIdx] = "***"
			}
		}
		if valueIdx >= 0 {
			i = valueIdx
		}
	}
	return out
}

// splitFlag 拆 "--name value" / "--name=value"；非 flag 返回 isFlag=false。
func splitFlag(s string) (name string, value *string, isFlag bool) {
	if !strings.HasPrefix(s, "--") {
		return "", nil, false
	}
	rest := s[2:]
	if eq := strings.Index(rest, "="); eq >= 0 {
		v := rest[eq+1:]
		return rest[:eq], &v, true
	}
	return rest, nil, true
}

func isKVExec(argv []string) bool {
	return len(argv) >= 2 && argv[0] == "kv" && argv[1] == "exec"
}

// RedactStatement 保留首关键字与前 200 字符（截断时不重复关键字）。
func RedactStatement(stmt string) string {
	stmt = strings.TrimSpace(stmt)
	if len(stmt) <= 200 {
		return stmt
	}
	head := stmt[:200] + "..."
	if kw := sqlguard.FirstKeyword(stmt); kw != "" && strings.HasPrefix(stmt, kw+" ") {
		return kw + " " + head[len(kw)+1:]
	}
	return head
}
