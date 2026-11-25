// 文件说明：model_internal_test.go — gocloudagent/agentcore 的单元测试（TestResetUnsupportedNativeToolsConcurrent、TestMergeToolsPrefersOptions 等）
//
// 用法
//
//	go test ./gocloudagent/agentcore -run 'TestResetUnsupportedNativeToolsConcurrent|TestMergeToolsPrefersOptions|TestSingleMessageStreamYieldsOnceThenEOF'
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/cloudagent, gocloudagent/group, gocloudagent/plan 等 6 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/agentcore/model_internal_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] emit_token_test.go, messages.go, model.go, model_length_test.go, model_stream.go 等 13 个
//
// 主要符号
//
//	TestResetUnsupportedNativeToolsConcurrent(t *testing.T)
//	TestMergeToolsPrefersOptions(t *testing.T)
//	TestSingleMessageStreamYieldsOnceThenEOF(t *testing.T)

package agentcore

import (
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// TestResetUnsupportedNativeToolsConcurrent 锁定重置的并发契约：
// 逐键删除与并发 Store/Load 安全交错（-race 门禁），重置期间读到旧值可接受。
func TestResetUnsupportedNativeToolsConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); unsupportedNativeTools.Store("race|model", true) }()
		go func() { defer wg.Done(); unsupportedNativeTools.Load("race|model") }()
		go func() { defer wg.Done(); ResetUnsupportedNativeTools() }()
	}
	wg.Wait()
	ResetUnsupportedNativeTools()
	if _, ok := unsupportedNativeTools.Load("race|model"); ok {
		t.Fatal("cache not empty after reset")
	}
}

func TestMergeToolsPrefersOptions(t *testing.T) {
	bound := []*schema.ToolInfo{{Name: "b"}}
	if mergeTools(bound, nil)[0].Name != "b" {
		t.Fatal("merge bound")
	}
	if mergeTools(bound, []*schema.ToolInfo{{Name: "o"}})[0].Name != "o" {
		t.Fatal("merge opt")
	}
}

func TestSingleMessageStreamYieldsOnceThenEOF(t *testing.T) {
	sr := singleMessageStream(schema.AssistantMessage("one", nil), nil)
	got, err := sr.Recv()
	if err != nil || got.Content != "one" {
		t.Fatalf("single stream %v %v", got, err)
	}
	if _, err = sr.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("eof=%v", err)
	}
}
