package cloudagent

import (
	"context"
	"fmt"
	"io"
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

type scriptedChat struct {
	content       string
	err           error
	streamErr     error
	streamNextErr error
	streamChunks  []string
	last          ChatRequest
}

func (s *scriptedChat) Chat(_ context.Context, _ string, req ChatRequest) (ChatResponse, error) {
	s.last = req
	if s.err != nil {
		return ChatResponse{}, s.err
	}
	return ChatResponse{Content: s.content}, nil
}

func (s *scriptedChat) Stream(_ context.Context, _ string, req ChatRequest) (TokenStream, error) {
	s.last = req
	if s.streamErr != nil {
		return nil, s.streamErr
	}
	return &seqStream{chunks: s.streamChunks, err: s.streamNextErr}, nil
}

type seqStream struct {
	chunks []string
	i      int
	err    error
}

func (s *seqStream) Next() (string, bool, error) {
	if s.i >= len(s.chunks) {
		if s.err != nil {
			return "", false, s.err
		}
		return "", true, io.EOF
	}
	c := s.chunks[s.i]
	s.i++
	finish := s.i >= len(s.chunks)
	return c, finish, nil
}

func (s *seqStream) Close() error { return nil }

type fakeSettings struct {
	st  LLMSettings
	err error
}

func (f *fakeSettings) LLMSettings(context.Context, string) (LLMSettings, error) {
	return f.st, f.err
}

type errDB struct {
	dbs []DatabaseInfo
	err error
}

func (f *errDB) ListDatabases(context.Context, Principal, string) ([]DatabaseInfo, error) {
	return f.dbs, f.err
}

func (f *errDB) ListCollections(context.Context, Principal, string, string) ([]string, error) {
	return nil, f.err
}

func (f *errDB) ReadOnlyQuery(context.Context, Principal, string, string, string, int) (SQLResult, error) {
	return SQLResult{}, f.err
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
	return f.head, f.err
}

type errLogs struct {
	events []LogEvent
	stats  []LogLevelCount
	err    error
}

func (f *errLogs) SearchLogs(context.Context, string, string, string, int) ([]LogEvent, error) {
	return f.events, f.err
}

func (f *errLogs) LevelStats(context.Context, string) ([]LogLevelCount, error) {
	return f.stats, f.err
}
