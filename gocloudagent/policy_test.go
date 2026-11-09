package gocloudagent

import (
	"fmt"
	"strings"
)

// testPolicy 是模块测试用的命令策略替身。
//
// 真实规则在宿主 internal/agentbridge（sbcli 只读表 + sqlguard 切分），
// 那边的对账测试用真实现；这里只保留足够的分类语义，避免模块测试反向
// 依赖根模块。故意不复刻完整的只读表。
type testPolicy struct{}

func (testPolicy) Classify(argv []string, stdin string) (CommandDecision, error) {
	if len(argv) == 0 {
		return CommandDecision{}, fmt.Errorf("empty argv")
	}
	return CommandDecision{
		ReadOnly:    argv[1] == "list" || argv[1] == "query" || argv[1] == "get" || argv[1] == "stats",
		Destructive: isTestDestructive(argv, stdin),
	}, nil
}

func (testPolicy) RedactArgv(argv []string) []string {
	out := append([]string(nil), argv...)
	for i, a := range out {
		if strings.HasPrefix(a, "--password") || strings.HasPrefix(a, "--api_key") {
			out[i] = strings.SplitN(a, "=", 2)[0] + "=***"
		}
	}
	return out
}

func isTestDestructive(argv []string, stdin string) bool {
	if len(argv) < 2 {
		return false
	}
	switch argv[0] + " " + argv[1] {
	case "database delete", "object delete", "function delete", "cron delete":
		return true
	case "sql exec":
		stmt := ""
		for i := 0; i+1 < len(argv); i++ {
			if argv[i] == "--statement" {
				stmt = argv[i+1]
			}
		}
		if stmt == "" {
			return true
		}
		upper := strings.ToUpper(strings.TrimSpace(stmt))
		for _, kw := range []string{"DELETE", "DROP", "TRUNCATE", "ALTER"} {
			if strings.HasPrefix(upper, kw) {
				return true
			}
		}
		return false
	default:
		_ = stdin
		return false
	}
}
