// observe_read_test.go 验证 planv5.0 §慢因复查 四-1 新增观测：
// list_agent_messages 与 metrics_trend 的 query/scan 子段及总耗时打点。
package systemdb

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/observability"
)

func newObservedStore(t *testing.T) (*Store, *bytes.Buffer) {
	t.Helper()
	s := newTestStore(t)
	buf := &bytes.Buffer{}
	s.logger = observability.NewLogger("debug", "json", buf)
	return s, buf
}

// TestObserveListAgentMessagesStages 验证成功路径记录 query/scan 子段与行数。
func TestObserveListAgentMessagesStages(t *testing.T) {
	s, buf := newObservedStore(t)
	ctx := context.Background()
	th, err := s.CreateAgentThread(ctx, AgentThread{
		ID:        "th1",
		ProjectID: "proj1",
		Title:     "t1",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if _, err := s.AppendAgentMessage(ctx, AgentMessage{
		ID:        "m1",
		ThreadID:  th.ID,
		ProjectID: "proj1",
		Role:      "user",
		Content:   "hello",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append message: %v", err)
	}
	out, err := s.ListAgentMessages(ctx, "proj1", th.ID, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 message, got %d", len(out))
	}
	logs := buf.String()
	for _, want := range []string{
		`"operation":"list_agent_messages"`,
		`"part":"query"`,
		`"part":"scan"`,
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("missing %s in observe logs:\n%s", want, logs)
		}
	}
	// 安全约束：不得输出项目 ID/线程 ID/消息内容。
	for _, banned := range []string{"proj1", "th1", "hello"} {
		if strings.Contains(logs, banned) {
			t.Fatalf("observe log leaked %q:\n%s", banned, logs)
		}
	}
}

// TestObserveMetricsTrendStages 验证 MetricsTrend 打点正确且行为不变。
func TestObserveMetricsTrendStages(t *testing.T) {
	s, buf := newObservedStore(t)
	ctx := context.Background()
	s.RecordMetric(MetricSample{
		ID:         "s1",
		ProjectID:  "proj1",
		Name:       "http_requests",
		Value:      5,
		OccurredAt: time.Now().UTC(),
	})
	if err := s.FlushMetrics(ctx); err != nil {
		t.Fatalf("flush metrics: %v", err)
	}
	out, err := s.MetricsTrend(ctx, "proj1", 1)
	if err != nil {
		t.Fatalf("metrics trend: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("want at least 1 trend point")
	}
	logs := buf.String()
	for _, want := range []string{
		`"operation":"metrics_trend"`,
		`"part":"query"`,
		`"part":"scan"`,
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("missing %s in observe logs:\n%s", want, logs)
		}
	}
	if strings.Contains(logs, "proj1") {
		t.Fatalf("observe log leaked project id:\n%s", logs)
	}
}

// TestObserveReadNilStoreSafe 验证观测辅助在 nil/无 logger 时安全。
func TestObserveReadNilStoreSafe(t *testing.T) {
	var s *Store
	s.observeRead("list_agent_messages", 0, 0, nil)
	s.observeReadPart("metrics_trend", "query", 0, nil)
	safe := NewStoreForTest(nil)
	safe.observeRead("list_agent_messages", 0, 0, nil)
	safe.observeReadPart("metrics_trend", "scan", 0, nil)
}
