// 文件说明：testutil_test.go — internal/agentbridge 的测试辅助代码
//
// 用法
//
//	同包调用：testPolicy → failclosed_test.go, module_skills_test.go；onceStream → failclosed_test.go；errDB → tools_build_errors_test.go
//
// 调用架构
//
//	[上游] internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/testutil_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] access_db.go, access_llm.go, access_object.go, access_test.go, bridge_test.go 等 33 个
//	[关键引用] testPolicy ← failclosed_test.go, module_skills_test.go；onceStream ← failclosed_test.go；errDB ← tools_build_errors_test.go
//
// 主要符号
//
//	type testPolicy struct{…}
//	(testPolicy) Classify(argv []string, stdin string) (cloudagent.ToolDecision, error)
//	(testPolicy) RedactArgv(argv []string) []string
//	isTestDestructive(argv []string, stdin string) bool
//	type onceStream struct{…}
//	(s *onceStream) Next() (string, bool, error)
//	(*onceStream) Close() error
//	type errDB struct{…}
//	... 另有 10 个顶层符号

package agentbridge

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

type testPolicy struct{}

func (testPolicy) Classify(argv []string, stdin string) (cloudagent.ToolDecision, error) {
	if len(argv) < 2 {
		return cloudagent.ToolDecision{}, fmt.Errorf("invalid argv")
	}
	return cloudagent.ToolDecision{
		ReadOnly:    argv[1] == "list" || argv[1] == "query" || argv[1] == "get" || argv[1] == "stats",
		Destructive: isTestDestructive(argv, stdin),
	}, nil
}

func (testPolicy) RedactArgv(argv []string) []string {
	out := append([]string(nil), argv...)
	for i, arg := range out {
		if strings.HasPrefix(arg, "--password") || strings.HasPrefix(arg, "--api_key") {
			out[i] = strings.SplitN(arg, "=", 2)[0] + "=***"
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
		statement := ""
		for i := 0; i+1 < len(argv); i++ {
			if argv[i] == "--statement" {
				statement = argv[i+1]
			}
		}
		if statement == "" {
			return true
		}
		upper := strings.ToUpper(strings.TrimSpace(statement))
		for _, keyword := range []string{"DELETE", "DROP", "TRUNCATE", "ALTER"} {
			if strings.HasPrefix(upper, keyword) {
				return true
			}
		}
	}
	return false
}

type onceStream struct {
	content string
	done    bool
}

func (s *onceStream) Next() (string, bool, error) {
	if s.done {
		return "", true, io.EOF
	}
	s.done = true
	return s.content, true, nil
}
func (*onceStream) Close() error { return nil }

type errDB struct {
	dbs []DatabaseInfo
	err error
}

func (f *errDB) ListDatabases(context.Context, auth.Principal, string) ([]DatabaseInfo, error) {
	return f.dbs, f.err
}
func (f *errDB) ListCollections(context.Context, auth.Principal, string, string) ([]string, error) {
	if f.err != nil {
		return nil, fmt.Errorf("coll: %w", f.err)
	}
	return []string{"users"}, nil
}
func (f *errDB) ReadOnlyQuery(context.Context, auth.Principal, string, string, string, int) (SQLResult, error) {
	if f.err != nil {
		return SQLResult{}, fmt.Errorf("sql: %w", f.err)
	}
	return SQLResult{Columns: []string{"n"}, Rows: [][]any{{int64(1)}}, RowCount: 1}, nil
}

type errObj struct {
	list []ObjectInfo
	head ObjectInfo
	err  error
}

func (f *errObj) ListObjects(context.Context, string, string, int) ([]ObjectInfo, error) {
	return f.list, f.err
}
func (f *errObj) HeadObject(context.Context, string, string) (ObjectInfo, error) {
	if f.err != nil {
		return ObjectInfo{}, fmt.Errorf("head: %w", f.err)
	}
	return f.head, nil
}

type errLogs struct {
	events []LogEvent
	stats  []LogLevelCount
	err    error
}

func (f *errLogs) SearchLogs(context.Context, string, string, string, int) ([]LogEvent, error) {
	if f.err != nil {
		return nil, fmt.Errorf("search: %w", f.err)
	}
	return f.events, nil
}
func (f *errLogs) LevelStats(context.Context, string) ([]LogLevelCount, error) {
	if f.err != nil {
		return nil, fmt.Errorf("stats: %w", f.err)
	}
	return f.stats, nil
}

func withRunContext(ctx context.Context, rc RunContext) context.Context {
	return cloudagent.WithRunContext(ctx, cloudagent.RunContext{
		RunID: rc.RunID, ThreadID: rc.ThreadID, Value: rc, Emit: rc.Emit,
	})
}
