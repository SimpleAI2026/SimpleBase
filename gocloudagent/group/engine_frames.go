// 文件说明：engine_frames.go — 群聊结构事件的 JSON 帧载荷
//
// 用法：由同包 Engine 与 broker 发布路径调用。
// 调用架构：轮次 / 计划 / 任务 / 人工 / 进度状态 → encoding/json。
// 主要符号：turnJSON、planJSON、planConfirmJSON、taskJSON、humanJSON、progressJSON。

package group

import (
	"encoding/json"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
)

// ---------------------------------------------------------------------------
// 帧载荷
// ---------------------------------------------------------------------------

func turnJSON(status string, _ int) []byte {
	b, _ := json.Marshal(map[string]any{"status": status})
	return b
}

func planJSON(d plan.Decision, confirmRequired bool) []byte {
	b, _ := json.Marshal(map[string]any{
		"decision": d, "confirm_required": confirmRequired,
	})
	return b
}

func planConfirmJSON(status string, edited bool) []byte {
	b, _ := json.Marshal(map[string]any{"state": status, "status": status, "edited": edited})
	return b
}

func taskJSON(t Task, index, total int) []byte {
	b, _ := json.Marshal(map[string]any{
		"status": t.Status, "index": index, "total": total,
		"ref": t.Ref, "target_kind": t.TargetKind, "target_id": t.TargetID,
		"message": agentcore.Truncate(t.Message, maxGroupContentChars), "depends_on": t.DependsOn, "context_from": t.ContextFrom,
		"step_id": t.StepID, "risk": t.Risk, "expected_output": t.ExpectedOutput,
		"result": t.Result, "error_code": t.ErrorCode, "run_id": t.RunID,
		"attempt": t.Attempt, "version": t.Version,
	})
	return b
}

func humanJSON(name, message string, humanIDs ...string) []byte {
	if name == "" {
		name = "当前用户"
	}
	payload := map[string]any{"human_name": name, "message": agentcore.Truncate(message, maxGroupContentChars)}
	if len(humanIDs) > 0 {
		payload["human_id"] = humanIDs[0]
	}
	b, _ := json.Marshal(payload)
	return b
}

func progressJSON(percent int, summary string, steps []plan.Step) []byte {
	b, _ := json.Marshal(map[string]any{
		"percent": percent, "summary": summary, "steps": steps,
	})
	return b
}
