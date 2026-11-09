package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// 构造一段「user / assistant / tool / assistant」的历史。
func seedTurn(t *testing.T, s *Session, prompt, answer string) {
	t.Helper()
	if _, err := s.AppendMessage("user", prompt); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendMessage("assistant", answer); err != nil {
		t.Fatal(err)
	}
}

// 切点两侧都是完整回合：保留段从 user 开始，摘要段不留下悬空的工具调用。
func TestCutByTurnKeepsWholeTurns(t *testing.T) {
	msgs := []Entry{
		{Role: "user", Content: strings.Repeat("a", 400)},
		{Role: "assistant", Content: strings.Repeat("b", 400)},
		{Role: "user", Content: "second"},
		{Role: "assistant", Content: "answer"},
	}
	cut := cutByTurn(msgs, 4)
	if cut <= 0 || cut >= len(msgs) {
		t.Fatalf("cut = %d, want a valid interior split", cut)
	}
	if msgs[cut].Role != "user" {
		t.Fatalf("kept segment must start a turn, got role %q", msgs[cut].Role)
	}
}

// 保留段不会以孤儿工具结果开头——否则模型会看到没有对应调用的结果。
func TestCutByTurnNeverStartsKeptWithToolResult(t *testing.T) {
	msgs := []Entry{
		{Role: "user", Content: strings.Repeat("a", 400)},
		{Role: "assistant", Content: "thinking"},
		{Role: "assistant", Content: "", ToolCalls: []EntryToolCa{{ID: "c1", Name: "shell"}}},
		{Role: "tool", ToolCallID: "c1", Name: "shell", Content: strings.Repeat("x", 40)},
	}
	cut := cutByTurn(msgs, 1)
	if cut != 0 {
		t.Fatalf("cut = %d: there is no complete turn to keep whole", cut)
	}
}

// 历史装得下 keep 预算时不该压：压了只会丢信息。
func TestCutByTurnReturnsZeroWhenNothingToCompact(t *testing.T) {
	msgs := []Entry{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "yo"}}
	if cut := cutByTurn(msgs, 4096); cut != 0 {
		t.Fatalf("cut = %d, want 0", cut)
	}
	if cut := cutByTurn(nil, 4096); cut != 0 {
		t.Fatalf("empty history cut = %d, want 0", cut)
	}
}

// token 估算：CJK 按字计、其余按 4 字符 1 token，且宁可高估。
func TestEstimateTokens(t *testing.T) {
	if got := estimateTokens(""); got != 0 {
		t.Fatalf("empty = %d", got)
	}
	if got := estimateTokens("你好世界"); got != 4 {
		t.Fatalf("cjk = %d, want 4", got)
	}
	if got := estimateTokens("abcdefgh"); got != 2 {
		t.Fatalf("ascii = %d, want 2", got)
	}
	// 不足 4 个字符也记 1，避免零成本累积出低估。
	if got := estimateTokens("a"); got != 1 {
		t.Fatalf("short = %d, want 1", got)
	}
}

// 压缩写入 compaction 条目，且历史投影只保留切点之后的消息；原文仍在文件里。
func TestForceCompactWritesSummaryAndHonoursCut(t *testing.T) {
	s := newTestSession(t)
	// 足够长的早期历史，保证能切出完整回合。
	for i := 0; i < 6; i++ {
		seedTurn(t, s, strings.Repeat("u", 200), strings.Repeat("a", 200))
	}
	seedTurn(t, s, "latest question", "latest answer")
	before := len(s.Messages())

	client := &scriptedClient{replies: []agentcore.ChatResponse{{Content: "早期对话摘要"}}}
	r := &SessionRunner{Session: s, Client: client, CompactKeepTokens: 8}
	ok, err := r.ForceCompact(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ForceCompact should compact a long history")
	}

	c, found := s.LatestCompaction()
	if !found || c.Summary != "早期对话摘要" {
		t.Fatalf("compaction = %+v found=%v", c, found)
	}
	msgs := s.Messages()
	if len(msgs) >= before {
		t.Fatalf("context should shrink: before=%d after=%d", before, len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "latest question" {
		t.Fatalf("kept segment = %+v", msgs)
	}
	// 原文不删（§13.5）：文件行数必须多于投影出的消息数。
	if len(s.Lines) <= len(msgs)+1 {
		t.Fatalf("originals must be preserved: lines=%d projected=%d", len(s.Lines), len(msgs))
	}
}

// 摘要在写入前脱敏：模型把密钥写进摘要也不能落进上下文。
func TestForceCompactRedactsSummary(t *testing.T) {
	s := newTestSession(t)
	for i := 0; i < 6; i++ {
		seedTurn(t, s, strings.Repeat("u", 200), strings.Repeat("a", 200))
	}
	seedTurn(t, s, "q", "a")
	secret := "sk-abcdefghijklmnopqrstuvwxyz"
	client := &scriptedClient{replies: []agentcore.ChatResponse{{Content: "key was " + secret}}}
	r := &SessionRunner{Session: s, Client: client, CompactKeepTokens: 8}
	if _, err := r.ForceCompact(context.Background()); err != nil {
		t.Fatal(err)
	}
	c, _ := s.LatestCompaction()
	if strings.Contains(c.Summary, secret) {
		t.Fatalf("summary leaked the secret: %q", c.Summary)
	}
	if !strings.Contains(c.Summary, "[REDACTED_API_KEY]") {
		t.Fatalf("summary should carry the redaction mark: %q", c.Summary)
	}
}

// 摘要失败必须保留原路径（§13.5 硬约束 3）：不写 compaction、不改历史、不回错误码。
func TestCompactFailureKeepsOriginalHistory(t *testing.T) {
	s := newTestSession(t)
	for i := 0; i < 6; i++ {
		seedTurn(t, s, strings.Repeat("u", 200), strings.Repeat("a", 200))
	}
	seedTurn(t, s, "q", "a")
	before := s.Messages()

	r := &SessionRunner{Session: s, Client: erroringClient{}, CompactKeepTokens: 8}
	if _, err := r.ForceCompact(context.Background()); err == nil {
		t.Fatal("want the upstream error to surface to the caller")
	}
	if _, found := s.LatestCompaction(); found {
		t.Fatal("a failed summary must not write a compaction entry")
	}
	if len(s.Messages()) != len(before) {
		t.Fatalf("history changed on failure: %d -> %d", len(before), len(s.Messages()))
	}
}

// 模型拿摘要指令去调工具时不算摘要成功，不能写入空摘要。
func TestCompactRejectsToolCallInsteadOfSummary(t *testing.T) {
	s := newTestSession(t)
	for i := 0; i < 6; i++ {
		seedTurn(t, s, strings.Repeat("u", 200), strings.Repeat("a", 200))
	}
	seedTurn(t, s, "q", "a")
	client := &scriptedClient{replies: []agentcore.ChatResponse{
		{ToolCalls: []agentcore.ChatToolCall{{ID: "c1", Name: "shell", Arguments: "{}"}}},
	}}
	r := &SessionRunner{Session: s, Client: client, CompactKeepTokens: 8}
	ok, err := r.ForceCompact(context.Background())
	if err == nil || ok {
		t.Fatalf("ok=%v err=%v, want a refusal with an error", ok, err)
	}
	if _, found := s.LatestCompaction(); found {
		t.Fatal("must not write a compaction entry from a tool call")
	}
}

// 阈值未到时不压缩：Compact 是「按需」，ForceCompact 才忽略阈值。
func TestCompactRespectsTrigger(t *testing.T) {
	s := newTestSession(t)
	seedTurn(t, s, "short", "reply")
	client := &scriptedClient{}
	r := &SessionRunner{Session: s, Client: client}
	ok, err := r.Compact(context.Background())
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want a no-op on a short history", ok, err)
	}
	if client.callCount() != 0 {
		t.Fatal("no summary request should be sent below the trigger")
	}
}

// 阈值可由 CompactTriggerTokens 覆盖（也让「消息条数兜底」可测）。
func TestShouldCompactHonoursOverrides(t *testing.T) {
	msgs := []Entry{{Role: "user", Content: "hi"}}
	if (&SessionRunner{}).shouldCompact(msgs) {
		t.Fatal("a one-message history must not trigger")
	}
	if !(&SessionRunner{CompactTriggerTokens: 1}).shouldCompact(msgs) {
		t.Fatal("token override should lower the trigger")
	}
}

// Run 里压缩失败不改变本轮结果（不新增可见错误码、不改 Reason）。
func TestRunSurvivesCompactionFailure(t *testing.T) {
	s := newTestSession(t)
	for i := 0; i < 6; i++ {
		seedTurn(t, s, strings.Repeat("u", 200), strings.Repeat("a", 200))
	}
	// 第一次回复给一个工具调用，第二次给最终答案；摘要请求会先失败。
	client := &scriptedClient{replies: []agentcore.ChatResponse{{Content: "final"}}}
	r := &SessionRunner{Session: s, Client: client, CompactKeepTokens: 8, CompactTriggerTokens: 1}
	res, err := r.Run(context.Background(), "hello", nil)
	if err != nil {
		t.Fatalf("compaction must never fail the run: %v", err)
	}
	if res.Reason != "stop" {
		t.Fatalf("reason = %q, want stop", res.Reason)
	}
}

// 摘要请求本身必须带「压成摘要」的指令，而不是把历史原样再问一遍。
func TestSummarizeSendsInstruction(t *testing.T) {
	s := newTestSession(t)
	seedTurn(t, s, "早期问题", "早期回答")
	client := &scriptedClient{replies: []agentcore.ChatResponse{{Content: "摘要"}}}
	r := &SessionRunner{Session: s, Client: client}
	out, err := r.summarize(context.Background(), s.Messages())
	if err != nil {
		t.Fatal(err)
	}
	if out != "摘要" {
		t.Fatalf("summary = %q", out)
	}
	if client.callCount() != 1 {
		t.Fatalf("calls = %d, want 1", client.callCount())
	}
	req := client.requests[0]
	found := false
	for _, m := range req.Messages {
		if strings.Contains(m.Content, "Summarize the conversation") {
			found = true
		}
	}
	if !found {
		t.Fatalf("summary request must carry the instruction: %+v", req.Messages)
	}
}

// 压缩条目落在链末端（它就是新的叶子）时，投影仍要保留切点之后的消息。
// 回归保护：早先的 Messages() 实现是「遇 compaction 就清空 out」，一旦压缩条目
// 本身是最后一条，投影会整个变成空——正是压缩刚做完就要用历史的那个时刻。
func TestCompactionAsLeafKeepsPostCutMessages(t *testing.T) {
	s := newTestSession(t)
	_, _ = s.AppendMessage("user", "old")
	_, _ = s.AppendMessage("assistant", "old-answer")
	pivot := s.Lines[len(s.Lines)-1].ID
	_, _ = s.AppendMessage("user", "kept")
	kept := s.Lines[len(s.Lines)-1].ID

	if err := s.Append(Entry{Type: EntryCompaction, Summary: "摘要", FirstKept: pivot}); err != nil {
		t.Fatal(err)
	}
	// 压缩条目成为叶子（没有后续消息）。
	if _, err := s.AppendMessage("user", "after"); err != nil {
		t.Fatal(err)
	}

	msgs := s.Messages()
	roles := make([]string, 0, len(msgs))
	for _, m := range msgs {
		roles = append(roles, m.Role+":"+m.Content)
	}
	if len(msgs) != 2 || msgs[0].Content != "kept" {
		t.Fatalf("messages = %v, want the kept turn onwards", roles)
	}
	if msgs[0].ID != kept {
		t.Fatalf("first kept message = %q, want %q", msgs[0].ID, kept)
	}
}

// REPL 的 /compact：手动压缩忽略阈值，并在历史不足时明确回话而不是静默失败。
func TestREPLCompactCommand(t *testing.T) {
	env := testEnv(t)
	s, err := NewSession(env, Config{Endpoint: "http://e", ProjectID: "p"}, "/tmp/w")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		seedTurn(t, s, strings.Repeat("u", 200), strings.Repeat("a", 200))
	}
	seedTurn(t, s, "latest", "answer")

	var out, errb strings.Builder
	opts := Options{
		Env: env, Stdout: &out, Stderr: &errb,
		Stdin: strings.NewReader("/compact\n/exit\n"),
	}
	client := &scriptedClient{replies: []agentcore.ChatResponse{{Content: "摘要内容"}}}
	runtime := &SessionRunner{Session: s, Client: client, CompactKeepTokens: 8}

	if code := runREPL(opts, runtime, s); code != exitOK {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(out.String(), "已压缩上下文") {
		t.Fatalf("stdout = %q, want the compaction notice", out.String())
	}
	if _, found := s.LatestCompaction(); !found {
		t.Fatal("/compact must write a compaction entry")
	}
}

// /help 要列出 /compact，否则用户无从发现这个能力。
func TestREPLHelpMentionsCompact(t *testing.T) {
	help := replHelp(nil)
	if !strings.Contains(help, "/compact") {
		t.Fatalf("help = %q", help)
	}
}

var _ = errors.Is
