// 文件说明：schema.go — plan / explore / reflect 工具的 JSON Schema（planv6.8 §13.4）
//
// schema 用手写 JSON 固定下来（字段描述就是给模型的说明书），再转成 eino ToolInfo；
// 原生 function calling 与 text 协议回退（agentcore.toolProtocolPrompt）共用同一份。

package plan

import (
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// 工具名。
const (
	ToolPlan    = "plan"
	ToolExplore = "explore"
	ToolReflect = "reflect"
)

const planDescription = "Record or update the structured plan BEFORE doing the work; it DOES NOT execute anything. " +
	"For multi-step requests set a one-sentence goal and ordered steps, then keep step status and progress up to date " +
	"as you work (planning-with-files style: the plan is persisted to plan.md/progress.md). In a group also decide " +
	"which assistants to @, the exact self-contained message each one receives, dependencies between them, and whether " +
	"a human must confirm the plan first; the host validates that against group membership and policy. " +
	"Call again with action=update to record progress, action=replan after a failure or new information, " +
	"action=complete when the goal is met."

const planSchema = `{
  "type": "object",
  "properties": {
    "action": {"type": "string", "enum": ["create", "update", "replan", "complete"], "default": "create",
      "description": "create on first decision; update to record progress only; replan after failure or new info; complete when the goal is met."},
    "goal": {"type": "string", "description": "Single sentence: what the user ultimately wants. Required for create/replan."},
    "complexity": {"type": "string", "enum": ["simple", "complex"],
      "description": "simple = one-shot dispatch, no step tracking; complex = maintain steps/progress and persist plan files."},
    "reason": {"type": "string", "description": "Short rationale for this split (shown to the user)."},
    "dispatches": {
      "type": "array",
      "description": "Who to @ and what each one receives. Order is execution order unless depends_on says otherwise.",
      "items": {
        "type": "object",
        "properties": {
          "ref": {"type": "string", "description": "Local id like d1/d2 used by depends_on."},
          "target_kind": {"type": "string", "enum": ["agent", "human"]},
          "target_id": {"type": "string", "description": "Candidate assistant id, or the given human id."},
          "message": {"type": "string", "description": "The exact message sent to this target. Must be self-contained; do not include instructions meant for other targets."},
          "expected_output": {"type": "string", "description": "What this target should hand back (used for progress and downstream context)."},
          "depends_on": {"type": "array", "items": {"type": "string"}, "description": "Refs that must complete first; only earlier refs."},
          "context_from": {"type": "array", "items": {"type": "string"}, "description": "Refs whose results are forwarded into this message as context."},
          "risk": {"type": "string", "enum": ["read", "write", "destructive"], "description": "Highest expected side effect; destructive forces plan confirmation."},
          "step_id": {"type": "string", "description": "For complex plans: which step this dispatch advances."}
        },
        "required": ["ref", "target_kind", "target_id", "message"]
      }
    },
    "confirm": {
      "type": "object",
      "description": "Whether a human must approve this plan before dispatch.",
      "properties": {
        "required": {"type": "boolean"},
        "reason": {"type": "string", "description": "Why confirmation is needed (shown in the dialog)."},
        "question": {"type": "string", "description": "Optional concrete question for the user, e.g. which environment to target."},
        "options": {"type": "array", "items": {"type": "string"}, "description": "Optional quick choices rendered as buttons."},
        "editable": {"type": "boolean", "default": true, "description": "Whether the user may edit messages/remove dispatches before approving."}
      },
      "required": ["required"]
    },
    "steps": {
      "type": "array",
      "description": "Complex plans only: ordered milestones toward the goal.",
      "items": {
        "type": "object",
        "properties": {
          "id": {"type": "string"},
          "title": {"type": "string"},
          "status": {"type": "string", "enum": ["pending", "in_progress", "done", "blocked", "skipped"]},
          "evidence": {"type": "string", "description": "What proves it is done (cite task/result)."}
        },
        "required": ["id", "title", "status"]
      }
    },
    "progress": {
      "type": "object",
      "properties": {
        "percent": {"type": "integer", "minimum": 0, "maximum": 100},
        "summary": {"type": "string", "description": "One-line status shown in the group header."},
        "next": {"type": "string", "description": "Next concrete action."}
      }
    },
    "need_summary": {"type": "boolean", "description": "Whether a final summary message should be produced after all dispatches finish."},
    "findings": {"type": "array", "items": {"type": "string"}, "description": "Facts learned so far (persisted to findings.md for complex plans)."},
    "unknowns": {"type": "array", "items": {"type": "string"}, "description": "Open questions; a non-empty list usually means asking the human."}
  },
  "required": ["action"]
}`

const exploreDescription = "Record a structured exploration note before editing any code: goal, sub-questions, files/symbols you have read " +
	"(or plan to read next), confirmed findings and remaining unknowns. It DOES NOT read files for you — use `read` / " +
	"`glob` / `grep` to gather evidence first, then call `explore` to consolidate. Appended to the session's " +
	"explore.jsonl; safe to call multiple times as understanding grows."

const exploreSchema = `{
  "type": "object",
  "properties": {
    "goal": {"type": "string", "description": "What user problem this exploration is trying to understand. Short, single sentence."},
    "questions": {"type": "array", "items": {"type": "string"}, "description": "Concrete sub-questions the exploration should answer."},
    "reads": {
      "type": "array",
      "description": "Files/symbols the model has read or plans to read next.",
      "items": {
        "type": "object",
        "properties": {
          "path": {"type": "string", "description": "File path (absolute or relative to cwd)."},
          "symbol": {"type": "string", "description": "Optional: class/function/section anchor inside the file."},
          "why": {"type": "string", "description": "Why this file/symbol matters for the goal."}
        },
        "required": ["path"]
      }
    },
    "findings": {"type": "array", "items": {"type": "string"}, "description": "Key facts already confirmed by reading. Cite path[:line] when possible."},
    "unknowns": {"type": "array", "items": {"type": "string"}, "description": "Open questions still needing evidence before planning."}
  },
  "required": ["goal"]
}`

const reflectDescription = "Reflect on the current execution flow when the task is complex, risky, stuck, or near a decision point. " +
	"It checks whether the work still matches the user goal, performs adversarial review of assumptions and failure modes, " +
	"suggests next actions, and extracts durable memory facts into facts.md when appropriate. " +
	"Use this tool by model decision; it is not triggered automatically."

const reflectSchema = `{
  "type": "object",
  "properties": {
    "reason": {"type": "string", "default": "",
      "description": "Why reflection is needed now, e.g. complex task, repeated failure, before final answer, or memory consolidation."}
  }
}`

// toolInfo 把手写 schema 转成 eino ToolInfo；schema 是编译期常量，解析失败即程序错误。
func toolInfo(name, desc, raw string) *schema.ToolInfo {
	var js jsonschema.Schema
	if err := json.Unmarshal([]byte(raw), &js); err != nil {
		panic(fmt.Sprintf("plan: invalid %s schema: %v", name, err))
	}
	return &schema.ToolInfo{Name: name, Desc: desc, ParamsOneOf: schema.NewParamsOneOfByJSONSchema(&js)}
}

// PlanToolInfo / ExploreToolInfo / ReflectToolInfo 返回工具描述（Planner 与 text 协议共用）。
func PlanToolInfo() *schema.ToolInfo { return toolInfo(ToolPlan, planDescription, planSchema) }
func ExploreToolInfo() *schema.ToolInfo {
	return toolInfo(ToolExplore, exploreDescription, exploreSchema)
}
func ReflectToolInfo() *schema.ToolInfo {
	return toolInfo(ToolReflect, reflectDescription, reflectSchema)
}

// PlanSchemaJSON 返回 plan 工具参数的原始 JSON Schema（原生 function calling 用）。
func PlanSchemaJSON() json.RawMessage { return json.RawMessage(planSchema) }

// PlanToolSpec 返回 plan 工具的 ToolSpec（Planner 原生 function calling 用）。
func PlanToolSpec() agentcore.ToolSpec {
	return agentcore.ToolSpec{Name: ToolPlan, Description: planDescription, Parameters: PlanSchemaJSON()}
}

// ParseDecision 解析 plan 工具参数（宽容：容忍代码块包裹与首尾杂文）。
func ParseDecision(raw string) (Decision, error) {
	body := extractJSONObject(raw)
	if body == "" {
		return Decision{}, fmt.Errorf("%w: empty arguments", ErrInvalid)
	}
	var d Decision
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		return Decision{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return d, nil
}

func extractJSONObject(s string) string {
	start := -1
	for i := 0; i < len(s); i++ {
		if s[i] == '{' {
			start = i
			break
		}
	}
	end := -1
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '}' {
			end = i
			break
		}
	}
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}
