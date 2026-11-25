// 文件说明：fence.go — Plan 栅栏：群消息的唯一入口（planv6.8 §13.3）
//
// 决策顺序（确定性，先规则后模型）：
//  1. 规则短路：单成员群未 @人 → 原文直发；无模型 → fallback 或被 @ 目标各收原文；
//  2. Planner 强制以 plan 工具输出 Decision（校验失败一次修复重试，仍失败走
//     「被 @ 目标各收原文」或 fallback 回退）；
//  3. 确认判定：ApplyConfirmPolicy（模型建议 ∪ 宿主规则，只能加严）。
//
// @ 是提示与硬约束，不再绕过栅栏直接分派（取代 BuildMentionDispatchPlan 的
// 「同一段原文发给每个人」，修复 CB-BE-12）。

package group

import (
	"context"
	"fmt"
	"strings"

	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
)

// ErrRoutingFailed 对齐旧错误码（宿主 API 映射复用）。
var ErrRoutingFailed = fmt.Errorf("routing_failed")

// fenceResult 是栅栏的一次决策产物。
type fenceResult struct {
	decision plan.Decision
	// rule 表示决策来自规则短路（不调模型）。
	rule bool
}

// fence 决定本轮分工。
type fence struct {
	planner *plan.Planner // nil → 只走规则
}

// decide 运行栅栏；失败时按 §13.3-3 回退（保持可用，不静默选第一个成员）。
func (f *fence) decide(ctx context.Context, in TurnInput, g Group, cands []plan.Candidate) (fenceResult, error) {
	// 输入硬校验（fail-closed；宿主已做，这里双保险，哨兵可经 plan.ErrorCode 映射）。
	for _, m := range in.Mentions {
		if !memberOf(g, m) {
			return fenceResult{}, fmt.Errorf("%w: %s is not a member of this group", plan.ErrMemberMissing, m)
		}
	}
	if in.MentionedHuman {
		if g.HumanDisabled {
			return fenceResult{}, fmt.Errorf("%w: group disables human tasks", plan.ErrHumanDisabled)
		}
		if g.HumanID == "" {
			return fenceResult{}, fmt.Errorf("%w: no human assignee for this turn", plan.ErrHumanUnavailable)
		}
	}

	// 规则短路：单成员且未 @人 → 原文直发（决策仍落库，前端展示一致）。
	if len(g.MemberIDs) == 1 && len(in.Mentions) == 0 && !in.MentionedHuman {
		return fenceResult{decision: ruleDecision(in, g, []string{g.MemberIDs[0]}, nil), rule: true}, nil
	}

	// 模型决策（Planner 强制 plan 工具输出）。
	if f.planner != nil {
		req := plan.Request{
			Goal:       "",
			UserText:   in.UserText,
			Candidates: cands,
			History:    in.History,
			Constraints: plan.Constraints{
				Members:        g.MemberIDs,
				HumanID:        g.HumanID,
				HumanDisabled:  g.HumanDisabled,
				Mentioned:      in.Mentions,
				MentionedHuman: in.MentionedHuman,
				Limits:         plan.DefaultLimits(),
			},
			RoutingPrompt: in.RoutingPrompt,
			HumanName:     g.HumanName,
		}
		d, err := f.planner.Decide(ctx, req)
		if err == nil {
			d.Source = plan.SourceModel
			return fenceResult{decision: d}, nil
		}
		// 回退 1：被 @ 的目标各收原文（显式点名时）。
		if len(in.Mentions) > 0 || in.MentionedHuman {
			return fenceResult{decision: ruleDecision(in, g, in.Mentions, humanTargets(in, g)), rule: true}, nil
		}
		// 回退 2：群配置的兜底助手。
		if g.Policy.FallbackAgent != "" && memberOf(g, g.Policy.FallbackAgent) {
			return fenceResult{decision: ruleDecision(in, g, []string{g.Policy.FallbackAgent}, nil), rule: true}, nil
		}
		// 回退 3：无兜底但有当前人 → 请人澄清（不静默选成员）。
		if g.HumanID != "" && !g.HumanDisabled {
			return fenceResult{decision: ruleDecision(in, g, nil, []string{g.HumanID}), rule: true}, nil
		}
		return fenceResult{}, fmt.Errorf("%w: %v", ErrRoutingFailed, err)
	}

	// 无模型：显式 @ 直接分派；否则 fallback；再否则 routing_failed（保持现有语义）。
	if len(in.Mentions) > 0 || in.MentionedHuman {
		return fenceResult{decision: ruleDecision(in, g, in.Mentions, humanTargets(in, g)), rule: true}, nil
	}
	if g.Policy.FallbackAgent != "" && memberOf(g, g.Policy.FallbackAgent) {
		return fenceResult{decision: ruleDecision(in, g, []string{g.Policy.FallbackAgent}, nil), rule: true}, nil
	}
	return fenceResult{}, fmt.Errorf("%w: no model and no fallback agent", ErrRoutingFailed)
}

// humanTargets 返回规则短路时人类任务的目标（仅显式 @ 人且人可用）。
func humanTargets(in TurnInput, g Group) []string {
	if in.MentionedHuman && g.HumanID != "" && !g.HumanDisabled {
		return []string{g.HumanID}
	}
	return nil
}

// ruleDecision 构造规则短路的决策：被 @ 的目标（或指定列表）各收原文。
// 每个目标收到的是同一段自包含原文，但**独立成任务**——与 CB-BE-12 的修复语义
// 一致：模型决策时每人才有独立 message；规则短路只是可用性回退。
func ruleDecision(in TurnInput, g Group, agentIDs []string, humanIDs []string) plan.Decision {
	d := plan.Decision{
		Action:     plan.ActionCreate,
		Goal:       clipGoal(in.UserText),
		Complexity: plan.ComplexitySimple,
		Source:     plan.SourceRule,
	}
	i := 1
	for _, id := range agentIDs {
		if !memberOf(g, id) {
			continue
		}
		d.Dispatches = append(d.Dispatches, plan.Dispatch{
			Ref:        fmt.Sprintf("d%d", i),
			TargetKind: plan.TargetAgent,
			TargetID:   id,
			Message:    strings.TrimSpace(in.UserText),
		})
		i++
	}
	for _, id := range humanIDs {
		if g.HumanID == "" || g.HumanDisabled || id != g.HumanID {
			continue
		}
		d.Dispatches = append(d.Dispatches, plan.Dispatch{
			Ref:        fmt.Sprintf("d%d", i),
			TargetKind: plan.TargetHuman,
			TargetID:   id,
			Message:    strings.TrimSpace(in.UserText),
		})
		i++
	}
	return d
}

func memberOf(g Group, id string) bool {
	for _, m := range g.MemberIDs {
		if m == id {
			return true
		}
	}
	return false
}

// clipGoal 把用户文本压成单句目标（展示用）。
func clipGoal(text string) string {
	text = strings.TrimSpace(text)
	r := []rune(text)
	if len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return text
}
