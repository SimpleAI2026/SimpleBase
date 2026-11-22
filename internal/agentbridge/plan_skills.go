package agentbridge

import (
	"encoding/json"
	"strings"
)

// PlanStageSkills 返回某阶段加载的技能与其中只读的子集。
// 技能只由服务端按阶段决定，客户端不能传技能 id。
func PlanStageSkills(stage PlanStageID) (skills, readonly []string, ok bool) {
	switch stage {
	case PlanStagePlanning:
		return nil, nil, true
	case PlanStageDatabase:
		return []string{"database"}, nil, true
	case PlanStageKV:
		return []string{"kv"}, nil, true
	case PlanStageFunctions:
		return []string{"gofunction", "database"}, []string{"database"}, true
	case PlanStageCron:
		return []string{"cron", "gofunction"}, []string{"gofunction"}, true
	case PlanStageSite:
		return []string{"s3", "site"}, nil, true
	default:
		return nil, nil, false
	}
}

const planModulePrompt = `Plan mode: you are building a small application for the user in this project, one stage per turn. The current stage and the skills loaded this turn are listed in the project env.
Rules:
- Do only the current stage. Never run commands that belong to another stage, never trigger a cron job, and never delete any resource.
- Migrations are append-only: use CREATE TABLE IF NOT EXISTS and never DROP or rewrite existing tables.
- A cloud function is a single file with package main; run function test and read its result before you claim success.
- The front end is one web/index.html: inline CSS and JS, hash routing, no build step, no CDN, at most 200KiB, and no key or token of any kind. Upload it with object upload and put the content in stdin. Only the web/ prefix may be published, never the object root.
- The site surface is read-only and has no data API. To show data, write the result to web/data/*.json and read it with a relative fetch.
- If site status reports publishable=false, skip publishing and say why.
- If a command fails, report the error code and stop that step; never invent results.
- End the final message with exactly one line: <!-- sb:delivery {"site_url":"","functions":[{"name":"","url":"/go/<project_id>/<function>/<export>"}],"tables":[],"collections":[],"cron":[{"name":"","expr":""}],"next":[]} --> filled in only with facts you confirmed through tools.`

// PlanStagePrompt 返回某阶段追加的模块层提示词。
func PlanStagePrompt(stage PlanStageID) string {
	if stage == PlanStagePlanning {
		return `Read-only planning. Return only a JSON array with exactly six objects in this order: plan, database, kv, functions, cron, site. Each object has id and exactly one of goal or skip_reason. The plan object must have a goal. Optional depends_on lists earlier stage IDs required for the work. Skip stages irrelevant to the request. Do not execute tools or include Markdown.`
	}
	return planModulePrompt + "\nCurrent stage: " + string(stage) + "."
}

// planToolCard 是工具卡片里判定阶段完成所需的最小字段。
type planToolCard struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error"`
}

// PlanStageDone 从本轮工具卡片判定阶段是否完成。
//
// 只看真实的、成功的工具调用，不依赖模型自报。plan 阶段没有工具要求，
// 由调用方按「消息非空」判定；site 阶段要求公开 web/ 前缀成功。
func PlanStageDone(stage PlanStageID, toolCallsJSON string) bool {
	if stage == PlanStagePlanning {
		return true
	}
	var cards []planToolCard
	if json.Unmarshal([]byte(toolCallsJSON), &cards) != nil {
		return false
	}
	for _, card := range cards {
		if card.Name != "simplebase" || card.IsError || toolResultFailed(card.Content) {
			continue
		}
		argv := cardArgv(card.Arguments)
		if len(argv) < 2 {
			continue
		}
		if planArgvMatches(stage, argv) {
			return true
		}
	}
	return false
}

func cardArgv(arguments string) []string {
	var payload struct {
		Argv []string `json:"argv"`
	}
	if json.Unmarshal([]byte(arguments), &payload) != nil {
		return nil
	}
	return payload.Argv
}

// toolResultFailed 识别 CLI 结果信封里的失败（工具本身 status 仍可能是 success）。
func toolResultFailed(content string) bool {
	if content == "" {
		return false
	}
	var v struct {
		OK      *bool `json:"ok"`
		IsError *bool `json:"is_error"`
	}
	if json.Unmarshal([]byte(content), &v) != nil {
		return false
	}
	return (v.OK != nil && !*v.OK) || (v.IsError != nil && *v.IsError)
}

func planArgvMatches(stage PlanStageID, argv []string) bool {
	head := strings.ToLower(argv[0])
	sub := strings.ToLower(argv[1])
	switch stage {
	case PlanStageDatabase:
		return (head == "sql" && (sub == "exec" || sub == "batch")) ||
			(head == "collection" && sub == "create") ||
			(head == "schema" && sub == "create-table")
	case PlanStageKV:
		return head == "kv" && sub == "exec"
	case PlanStageFunctions:
		return head == "function" && (sub == "test" || sub == "create" || sub == "update")
	case PlanStageCron:
		return head == "cron" && (sub == "create" || sub == "update")
	case PlanStageSite:
		return head == "site" && sub == "publish"
	default:
		return false
	}
}
