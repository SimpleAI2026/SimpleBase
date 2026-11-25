// 文件说明：tool.go — plan / explore / reflect 的 eino 工具实现
//
// Tools 绑定一个 Store；三个工具都只记录与反思，不执行任何副作用操作。
//   - plan：有 Constraints 时按群约束严格校验（群聊 Planner / 助手 replan）；
//           没有时只做结构校验（单助手：goal + steps + progress，dispatches 可空）。
//   - explore：追加 explore.jsonl，返回累计摘要。
//   - reflect：调用模型做一次无工具反思，输出结构化结果并把 facts 写入 facts.md；
//              未配置模型时退化为基于已有记录的确定性检查清单。
// 宿主从已落库的工具卡片恢复当前计划用 ReplayCall。

package plan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// Tools 是规划工具组。
type Tools struct {
	Store Store
	// Constraints 返回群约束；nil 或 ok=false 表示只做结构校验。
	Constraints func() (Constraints, bool)
	// OnDecision 在 plan 成功记录后回调（宿主据此广播 plan / progress 帧）；ctx 是本次工具调用的上下文。
	OnDecision func(ctx context.Context, d Decision)
	// LLM / Scope / Model 供 reflect 使用；LLM 为 nil 时 reflect 走确定性清单。
	LLM   agentcore.ChatClient
	Scope string
	Model string
	// Goal 返回当前用户目标（reflect 对齐检查用）；nil 时取已存决策的 goal。
	Goal func() string
}

// All 返回 plan / explore / reflect 三个工具。
func (t *Tools) All() []tool.BaseTool {
	return []tool.BaseTool{&planTool{t}, &exploreTool{t}, &reflectTool{t}}
}

func (t *Tools) store() Store {
	if t.Store == nil {
		t.Store = &MemoryStore{}
	}
	return t.Store
}

func toolFailure(code, msg string) string { return agentcore.ErrorEnvelope(code, msg) }

func encode(v any) string {
	s, err := agentcore.MarshalToolJSON(v)
	if err != nil {
		return toolFailure("encode_failed", err.Error())
	}
	return s
}

// ---- plan ----

type planTool struct{ t *Tools }

func (p *planTool) Info(context.Context) (*schema.ToolInfo, error) { return PlanToolInfo(), nil }

func (p *planTool) InvokableRun(ctx context.Context, args string, _ ...tool.Option) (string, error) {
	d, err := ParseDecision(args)
	if err != nil {
		return toolFailure("bad_arguments", err.Error()), nil
	}
	c, strict := Constraints{}, false
	if p.t.Constraints != nil {
		c, strict = p.t.Constraints()
	}
	d = Normalize(d, c)
	updating := d.Action == ActionUpdate || d.Action == ActionComplete
	var prev Decision
	if updating {
		var hasPrev bool
		prev, hasPrev, err = p.t.store().LoadDecision(ctx)
		if err != nil {
			return toolFailure("store_failed", err.Error()), nil
		}
		if !hasPrev {
			return toolFailure("no_plan", "call plan with action=create first"), nil
		}
		// §13.4：update 只改进度类字段；带 dispatches 必须走 replan。
		if len(d.Dispatches) > 0 {
			return toolFailure("invalid_plan", d.Action+" must not change dispatches; use action=replan"), nil
		}
	}
	if strict {
		err = Validate(d, c)
	} else {
		err = validateStructure(d)
	}
	if err != nil {
		return toolFailure(errorCode(err), err.Error()), nil
	}
	if updating {
		d = mergeUpdate(prev, d)
	}
	d.Source = SourceModel
	if err := p.t.store().SaveDecision(ctx, d); err != nil {
		return toolFailure("store_failed", err.Error()), nil
	}
	if line := progressLine(d); line != "" {
		if err := p.t.store().AppendProgress(ctx, line); err != nil {
			return toolFailure("store_failed", err.Error()), nil
		}
	}
	if len(d.Findings) > 0 {
		if err := p.t.store().AppendFindings(ctx, d.Findings); err != nil {
			return toolFailure("store_failed", err.Error()), nil
		}
	}
	if p.t.OnDecision != nil {
		p.t.OnDecision(ctx, d)
	}
	return encode(map[string]any{
		"ok": true, "action": d.Action, "goal": d.Goal, "dispatches": len(d.Dispatches),
		"steps": len(d.Steps), "progress": d.Progress, "confirm_required": d.Confirm.Required,
	}), nil
}

// ReplayCall 把一次 plan 调用的参数叠加到既有决策上（与工具实现同一套规则）：
// create/replan 整份替换，update/complete 只合并进度类字段，非法参数跳过。
// 宿主用它从已落库的工具卡片恢复「当前计划」，让后续 run 可以 update 而不必重建。
// 返回 ok=false 表示这些调用里没有可用的 create/replan。
func ReplayCall(prev Decision, hasPrev bool, arguments string) (Decision, bool) {
	d, err := ParseDecision(arguments)
	if err != nil {
		return prev, hasPrev
	}
	d = Normalize(d, Constraints{})
	switch d.Action {
	case ActionCreate, ActionReplan:
		if err := validateStructure(d); err != nil {
			return prev, hasPrev
		}
		d.Source = SourceModel
		return d, true
	case ActionUpdate, ActionComplete:
		if !hasPrev || len(d.Dispatches) > 0 {
			return prev, hasPrev
		}
		if err := validateSteps(d); err != nil {
			return prev, hasPrev
		}
		return mergeUpdate(prev, d), true
	default:
		return prev, hasPrev
	}
}

// mergeUpdate 把 update/complete 叠加到上一份决策：分工不变，只更新进度类字段。
func mergeUpdate(prev, upd Decision) Decision {
	out := prev
	out.Action = upd.Action
	if len(upd.Steps) > 0 {
		byID := make(map[string]int, len(out.Steps))
		for i, s := range out.Steps {
			byID[s.ID] = i
		}
		for _, s := range upd.Steps {
			if i, ok := byID[s.ID]; ok {
				if s.Title != "" {
					out.Steps[i].Title = s.Title
				}
				out.Steps[i].Status = s.Status
				if s.Evidence != "" {
					out.Steps[i].Evidence = s.Evidence
				}
				continue
			}
			out.Steps = append(out.Steps, s)
		}
	}
	if upd.Progress != (Progress{}) {
		out.Progress = upd.Progress
	}
	if upd.Action == ActionComplete {
		out.Progress.Percent = 100
		for i := range out.Steps {
			if out.Steps[i].Status == StepPending || out.Steps[i].Status == StepInProgress {
				out.Steps[i].Status = StepDone
			}
		}
	}
	out.Findings = appendUnique(append([]string(nil), out.Findings...), upd.Findings)
	if upd.Unknowns != nil {
		out.Unknowns = upd.Unknowns
	}
	return out
}

// validateStructure 是无群约束时的校验：create/replan 需要 goal；dispatch 字段完整；步骤合法。
func validateStructure(d Decision) error {
	switch d.Action {
	case ActionCreate, ActionReplan, ActionUpdate, ActionComplete:
	default:
		return fmt.Errorf("%w: unknown action %q", ErrInvalid, d.Action)
	}
	if (d.Action == ActionCreate || d.Action == ActionReplan) && d.Goal == "" {
		return fmt.Errorf("%w: goal is required for %s", ErrInvalid, d.Action)
	}
	for _, t := range d.Dispatches {
		if t.Message == "" || t.TargetID == "" {
			return fmt.Errorf("%w: dispatch %q needs target_id and message", ErrInvalid, t.Ref)
		}
	}
	return validateSteps(d)
}

// ErrorCode 把校验错误映射为稳定的错误码（宿主 API 与工具信封共用）。
func ErrorCode(err error) string { return errorCode(err) }

func errorCode(err error) string {
	switch {
	case errors.Is(err, ErrHumanDisabled):
		return "human_policy_disabled"
	case errors.Is(err, ErrHumanUnavailable):
		return "human_assignee_required"
	case errors.Is(err, ErrMemberMissing):
		return "agent_not_in_tag"
	case errors.Is(err, ErrMentionMissing):
		return "mention_not_planned"
	case errors.Is(err, ErrTooLarge), errors.Is(err, ErrTooDeep):
		return "invalid_dispatch_graph"
	case errors.Is(err, ErrEmpty):
		return "routing_failed"
	default:
		return "invalid_plan"
	}
}

func progressLine(d Decision) string {
	parts := []string{d.Action}
	if d.Progress.Percent > 0 || d.Progress.Summary != "" {
		parts = append(parts, fmt.Sprintf("%d%%", d.Progress.Percent))
	}
	if d.Progress.Summary != "" {
		parts = append(parts, d.Progress.Summary)
	} else if d.Goal != "" && (d.Action == ActionCreate || d.Action == ActionReplan) {
		parts = append(parts, d.Goal)
	}
	if d.Progress.Next != "" {
		parts = append(parts, "next: "+d.Progress.Next)
	}
	return strings.Join(parts, " · ")
}

// ---- explore ----

type exploreTool struct{ t *Tools }

func (e *exploreTool) Info(context.Context) (*schema.ToolInfo, error) { return ExploreToolInfo(), nil }

func (e *exploreTool) InvokableRun(ctx context.Context, args string, _ ...tool.Option) (string, error) {
	var note ExploreNote
	if err := json.Unmarshal([]byte(extractJSONObject(args)), &note); err != nil {
		return toolFailure("bad_arguments", err.Error()), nil
	}
	note.Goal = strings.TrimSpace(note.Goal)
	if note.Goal == "" {
		return toolFailure("bad_arguments", "goal is required"), nil
	}
	for _, r := range note.Reads {
		if strings.TrimSpace(r.Path) == "" {
			return toolFailure("bad_arguments", "every read needs a path"), nil
		}
	}
	note.At = time.Now().UTC()
	if err := e.t.store().AppendExplore(ctx, note); err != nil {
		return toolFailure("store_failed", err.Error()), nil
	}
	if len(note.Findings) > 0 {
		if err := e.t.store().AppendFindings(ctx, note.Findings); err != nil {
			return toolFailure("store_failed", err.Error()), nil
		}
	}
	all, err := e.t.store().Explores(ctx)
	if err != nil {
		return toolFailure("store_failed", err.Error()), nil
	}
	reads := map[string]bool{}
	var findings, unknowns []string
	for _, n := range all {
		for _, r := range n.Reads {
			reads[r.Path] = true
		}
		findings = appendUnique(findings, n.Findings)
		unknowns = n.Unknowns
	}
	return encode(map[string]any{
		"ok": true, "notes": len(all), "files": len(reads),
		"findings": findings, "open_unknowns": unknowns,
	}), nil
}

// ---- reflect ----

type reflectTool struct{ t *Tools }

func (r *reflectTool) Info(context.Context) (*schema.ToolInfo, error) { return ReflectToolInfo(), nil }

// Reflection 是 reflect 的结构化输出。
type Reflection struct {
	Aligned       bool     `json:"aligned"`
	Assessment    string   `json:"assessment,omitempty"`
	Risks         []string `json:"risks,omitempty"`
	Counterpoints []string `json:"counterpoints,omitempty"`
	NextActions   []string `json:"next_actions,omitempty"`
	Facts         []string `json:"facts,omitempty"`
}

const reflectSystem = "You are a rigorous reviewer of an agent's work in progress. " +
	"Given the user goal, the current plan and recorded findings, answer ONLY with one JSON object: " +
	`{"aligned":bool,"assessment":"one sentence","risks":["..."],"counterpoints":["adversarial challenge to an assumption"],` +
	`"next_actions":["concrete next step"],"facts":["durable fact worth remembering across sessions"]}. ` +
	"Be concrete and brief; at most 5 items per list; facts must be verified statements, not plans."

func (r *reflectTool) InvokableRun(ctx context.Context, args string, _ ...tool.Option) (string, error) {
	var in struct {
		Reason string `json:"reason"`
	}
	if body := extractJSONObject(args); body != "" {
		_ = json.Unmarshal([]byte(body), &in)
	}
	st := r.t.store()
	d, hasPlan, err := st.LoadDecision(ctx)
	if err != nil {
		return toolFailure("store_failed", err.Error()), nil
	}
	goal := d.Goal
	if r.t.Goal != nil {
		if g := strings.TrimSpace(r.t.Goal()); g != "" {
			goal = g
		}
	}
	explores, err := st.Explores(ctx)
	if err != nil {
		return toolFailure("store_failed", err.Error()), nil
	}
	var findings, unknowns []string
	findings = appendUnique(findings, d.Findings)
	for _, n := range explores {
		findings = appendUnique(findings, n.Findings)
		unknowns = appendUnique(unknowns, n.Unknowns)
	}
	unknowns = appendUnique(unknowns, d.Unknowns)

	var out Reflection
	if r.t.LLM != nil {
		var err error
		out, err = r.askModel(ctx, in.Reason, goal, d, hasPlan, findings, unknowns)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			out = checklist(goal, d, hasPlan, unknowns)
			out.Assessment = "model reflection failed; deterministic checklist used: " + agentcore.Truncate(err.Error(), 200)
		}
	} else {
		out = checklist(goal, d, hasPlan, unknowns)
	}
	if len(out.Facts) > 0 {
		if err := st.AppendFacts(ctx, out.Facts); err != nil {
			return toolFailure("store_failed", err.Error()), nil
		}
	}
	if err := st.AppendProgress(ctx, "reflect · "+strings.TrimSpace(in.Reason+" "+out.Assessment)); err != nil {
		return toolFailure("store_failed", err.Error()), nil
	}
	return encode(map[string]any{"ok": true, "reflection": out}), nil
}

func (r *reflectTool) askModel(ctx context.Context, reason, goal string, d Decision, hasPlan bool, findings, unknowns []string) (Reflection, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Reason for reflection: %s\nUser goal: %s\n", strings.TrimSpace(reason), goal)
	if hasPlan {
		b.WriteString("\nCurrent plan:\n" + RenderMarkdown(d))
	}
	if len(findings) > 0 {
		b.WriteString("\nFindings:\n- " + strings.Join(findings, "\n- ") + "\n")
	}
	if len(unknowns) > 0 {
		b.WriteString("\nUnknowns:\n- " + strings.Join(unknowns, "\n- ") + "\n")
	}
	maxTokens := 600
	temp := 0.2
	resp, err := r.t.LLM.Chat(ctx, r.t.Scope, agentcore.ChatRequest{
		Model: r.t.Model, MaxTokens: &maxTokens, Temperature: &temp,
		Messages: []agentcore.ChatMessage{
			{Role: "system", Content: reflectSystem},
			{Role: "user", Content: agentcore.Truncate(b.String(), 8000)},
		},
	})
	if err != nil {
		return Reflection{}, err
	}
	var out Reflection
	body := extractJSONObject(resp.Content)
	if body == "" || json.Unmarshal([]byte(body), &out) != nil {
		return Reflection{}, fmt.Errorf("reflection is not valid JSON")
	}
	out.Risks = capList(out.Risks, 5)
	out.Counterpoints = capList(out.Counterpoints, 5)
	out.NextActions = capList(out.NextActions, 5)
	out.Facts = capList(out.Facts, 5)
	return out, nil
}

func capList(l []string, n int) []string {
	if len(l) > n {
		return l[:n]
	}
	return l
}

// checklist 是无模型时的确定性反思：检查目标、计划完整度、阻塞与未知项。
func checklist(goal string, d Decision, hasPlan bool, unknowns []string) Reflection {
	out := Reflection{Aligned: goal != "" && hasPlan}
	switch {
	case goal == "":
		out.Assessment = "no explicit goal recorded"
		out.NextActions = append(out.NextActions, "call plan with action=create and a one-sentence goal")
	case !hasPlan:
		out.Assessment = "goal known but no plan recorded"
		out.NextActions = append(out.NextActions, "record a plan before acting")
	default:
		out.Assessment = fmt.Sprintf("plan %q at %d%%", goal, d.Progress.Percent)
	}
	for _, s := range d.Steps {
		if s.Status == StepBlocked {
			out.Risks = append(out.Risks, "step "+s.ID+" is blocked: "+s.Title)
		}
	}
	for _, t := range d.Dispatches {
		if t.Risk == RiskDestructive {
			out.Risks = append(out.Risks, "dispatch "+t.Ref+" is destructive; confirm before running")
		}
	}
	if len(unknowns) > 0 {
		out.Counterpoints = append(out.Counterpoints, "open unknowns remain: "+strings.Join(capList(unknowns, 3), "; "))
		out.NextActions = append(out.NextActions, "resolve unknowns with read/grep or ask the user")
	}
	return out
}
