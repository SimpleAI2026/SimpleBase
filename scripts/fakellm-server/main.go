// fakellm-server.go：fakellm 的独立进程入口（planv4.1 §5.5）。
// 供 smoke.sh 与本地联调启动一个 OpenAI 兼容假上游：
//
//	go run ./scripts/fakellm-server -addr :8787
//
// 剧本通过环境变量 FAKELLM_SCRIPT（JSON 文件路径）可选自定义；
// 默认剧本：简单逐块对话 + 一个 list_databases 工具调用。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/linkxzhou/SimpleBase/internal/testutil/fakellm"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "listen address")
	scriptPath := flag.String("script", "", "optional JSON script file")
	flag.Parse()

	scripts := []fakellm.Script{}
	def := defaultScript()
	if *scriptPath != "" {
		b, err := os.ReadFile(*scriptPath)
		if err != nil {
			log.Fatalf("read script: %v", err)
		}
		if err := json.Unmarshal(b, &def); err != nil {
			log.Fatalf("parse script: %v", err)
		}
	}

	srv := fakellm.NewAt(*addr, scripts, def)
	defer srv.Close()
	fmt.Printf("fakellm listening on http://%s/v1/chat/completions (OpenAI compatible)\n", srv.Listener.Addr().String())
	select {}
}

// defaultScript 返回演示剧本：工具调用一次 + 最终答复。
func defaultScript() fakellm.Script {
	return fakellm.Script{
		Steps: []fakellm.Step{
			{Content: `TOOL_CALL {"name":"list_databases","arguments":{}}`, FinishReason: "tool_calls"},
			{Content: "这是本地假模型的回复：演示结束。"},
			{FinishReason: "stop"},
			{PromptTokens: 10, CompletionTokens: 6},
		},
	}
}
