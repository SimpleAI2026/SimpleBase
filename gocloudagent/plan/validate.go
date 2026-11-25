// 文件说明：validate.go — Decision 的归一化、严格校验与确认判定
//
// 校验按固定顺序返回首个错误，错误可用 errors.Is 归到下列哨兵，宿主再映射成 API 错误码：
//   ErrInvalid / ErrEmpty / ErrTooLarge / ErrTooDeep / ErrMemberMissing /
//   ErrMentionMissing / ErrHumanUnavailable / ErrHumanDisabled

package plan

import (
	"errors"
	"fmt"
	"strings"
)

// 校验哨兵。
var (
	ErrInvalid          = errors.New("plan is invalid")
	ErrEmpty            = errors.New("plan has no dispatch")
	ErrTooLarge         = errors.New("plan has too many dispatches")
	ErrTooDeep          = errors.New("plan dependency chain is too deep")
	ErrMemberMissing    = errors.New("plan targets a non-member")
	ErrMentionMissing   = errors.New("plan omits a mentioned assistant")
	ErrHumanUnavailable = errors.New("plan human is unavailable")
	ErrHumanDisabled    = errors.New("plan human tasks are disabled")
)

// humanRefs 是模型可能用来指代「当前人」的写法。
var humanRefs = map[string]bool{
	"@me": true, "me": true, "self": true, "user": true, "current_user": true,
	"currentuser": true, "human": true, "@current": true, "@user": true,
}

// Normalize 清理模型输出：去空白、补默认值、归一 @ 前缀与「人」的写法。不做校验。
func Normalize(d Decision, c Constraints) Decision {
	d.Action = strings.ToLower(strings.TrimSpace(d.Action))
	if d.Action == "" {
		d.Action = ActionCreate
	}
	d.Goal = strings.TrimSpace(d.Goal)
	d.Reason = strings.TrimSpace(d.Reason)
	d.Complexity = strings.ToLower(strings.TrimSpace(d.Complexity))
	if d.Complexity != ComplexityComplex {
		d.Complexity = ComplexitySimple
	}
	for i := range d.Dispatches {
		t := &d.Dispatches[i]
		t.Ref = strings.TrimSpace(t.Ref)
		if t.Ref == "" {
			t.Ref = fmt.Sprintf("d%d", i+1)
		}
		t.Message = strings.TrimSpace(t.Message)
		t.TargetID = strings.TrimSpace(t.TargetID)
		switch strings.ToLower(strings.TrimSpace(t.TargetKind)) {
		case TargetHuman, "person", "user":
			t.TargetKind = TargetHuman
		case TargetAgent, "assistant", "bot", "":
			t.TargetKind = TargetAgent
			if humanRefs[strings.ToLower(t.TargetID)] || (c.HumanID != "" && t.TargetID == c.HumanID) {
				t.TargetKind = TargetHuman
			}
		default:
			t.TargetKind = strings.ToLower(strings.TrimSpace(t.TargetKind))
		}
		if t.TargetKind == TargetHuman {
			if c.HumanID != "" && (humanRefs[strings.ToLower(t.TargetID)] || t.TargetID == "") {
				t.TargetID = c.HumanID
			}
		} else {
			t.TargetID = normalizeMember(t.TargetID, c.Members)
		}
		t.Risk = strings.ToLower(strings.TrimSpace(t.Risk))
		t.DependsOn = cleanRefs(t.DependsOn)
		t.ContextFrom = cleanRefs(t.ContextFrom)
		t.StepID = strings.TrimSpace(t.StepID)
	}
	for i := range d.Steps {
		d.Steps[i].ID = strings.TrimSpace(d.Steps[i].ID)
		d.Steps[i].Title = strings.TrimSpace(d.Steps[i].Title)
		d.Steps[i].Status = strings.ToLower(strings.TrimSpace(d.Steps[i].Status))
		if d.Steps[i].Status == "" {
			d.Steps[i].Status = StepPending
		}
	}
	if d.Progress.Percent < 0 {
		d.Progress.Percent = 0
	}
	if d.Progress.Percent > 100 {
		d.Progress.Percent = 100
	}
	return d
}

func normalizeMember(id string, members []string) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(id, "@"), "#")
	for _, m := range members {
		if m == id || m == trimmed {
			return m
		}
	}
	return trimmed
}

func cleanRefs(refs []string) []string {
	if len(refs) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(refs))
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		r = strings.TrimSpace(r)
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}

// Validate 严格校验一份（已 Normalize 的）决策；返回首个错误。
func Validate(d Decision, c Constraints) error {
	switch d.Action {
	case ActionCreate, ActionReplan, ActionUpdate, ActionComplete:
	default:
		return fmt.Errorf("%w: unknown action %q", ErrInvalid, d.Action)
	}
	if d.Action == ActionUpdate || d.Action == ActionComplete {
		if len(d.Dispatches) > 0 {
			return fmt.Errorf("%w: %s must not change dispatches; use replan", ErrInvalid, d.Action)
		}
		return validateSteps(d)
	}
	if d.Goal == "" {
		return fmt.Errorf("%w: goal is required for %s", ErrInvalid, d.Action)
	}
	if len(d.Dispatches) == 0 {
		return fmt.Errorf("%w: at least one dispatch is required", ErrEmpty)
	}
	limits := c.Limits.normalized()
	if len(d.Dispatches) > limits.MaxDispatches {
		return fmt.Errorf("%w: %d dispatches exceed %d", ErrTooLarge, len(d.Dispatches), limits.MaxDispatches)
	}
	members := make(map[string]bool, len(c.Members))
	for _, m := range c.Members {
		members[m] = true
	}
	refIdx := make(map[string]int, len(d.Dispatches))
	hasHuman := false
	covered := make(map[string]bool, len(d.Dispatches))
	for i, t := range d.Dispatches {
		if _, dup := refIdx[t.Ref]; dup {
			return fmt.Errorf("%w: duplicate ref %q", ErrInvalid, t.Ref)
		}
		refIdx[t.Ref] = i
		if t.Message == "" {
			return fmt.Errorf("%w: dispatch %q has empty message", ErrInvalid, t.Ref)
		}
		if len([]rune(t.Message)) > limits.MaxMessage {
			return fmt.Errorf("%w: dispatch %q message is too long", ErrInvalid, t.Ref)
		}
		switch t.Risk {
		case "", RiskRead, RiskWrite, RiskDestructive:
		default:
			return fmt.Errorf("%w: dispatch %q has unknown risk %q", ErrInvalid, t.Ref, t.Risk)
		}
		switch t.TargetKind {
		case TargetAgent:
			if !members[t.TargetID] {
				return fmt.Errorf("%w: dispatch %q targets %q", ErrMemberMissing, t.Ref, t.TargetID)
			}
			covered[t.TargetID] = true
		case TargetHuman:
			if c.HumanDisabled {
				return fmt.Errorf("%w: dispatch %q", ErrHumanDisabled, t.Ref)
			}
			if c.HumanID == "" || t.TargetID != c.HumanID {
				return fmt.Errorf("%w: dispatch %q", ErrHumanUnavailable, t.Ref)
			}
			hasHuman = true
		default:
			return fmt.Errorf("%w: dispatch %q has unknown target kind %q", ErrInvalid, t.Ref, t.TargetKind)
		}
	}
	for _, m := range c.Mentioned {
		if !covered[m] {
			return fmt.Errorf("%w: %q", ErrMentionMissing, m)
		}
	}
	if c.MentionedHuman && !hasHuman {
		if c.HumanDisabled {
			return ErrHumanDisabled
		}
		if c.HumanID == "" {
			return ErrHumanUnavailable
		}
		return fmt.Errorf("%w: mentioned human is not assigned", ErrMentionMissing)
	}
	// 依赖与上下文只能引用更早的 ref（天然无环），按序递推深度。
	depths := make([]int, len(d.Dispatches))
	for i, t := range d.Dispatches {
		depth := 1
		for _, list := range [][]string{t.DependsOn, t.ContextFrom} {
			for _, dep := range list {
				j, ok := refIdx[dep]
				if !ok {
					return fmt.Errorf("%w: %q depends on unknown %q", ErrInvalid, t.Ref, dep)
				}
				if j >= i {
					return fmt.Errorf("%w: %q depends on later %q", ErrInvalid, t.Ref, dep)
				}
				if depths[j]+1 > depth {
					depth = depths[j] + 1
				}
			}
		}
		depths[i] = depth
		if depth > limits.MaxDepth {
			return fmt.Errorf("%w: depth %d exceeds %d", ErrTooDeep, depth, limits.MaxDepth)
		}
	}
	if err := validateSteps(d); err != nil {
		return err
	}
	if d.Complexity == ComplexityComplex {
		// §13.4：complex 计划必须带 steps（simple 忽略 steps）。
		if len(d.Steps) == 0 {
			return fmt.Errorf("%w: complex plan requires steps", ErrInvalid)
		}
		steps := make(map[string]bool, len(d.Steps))
		for _, s := range d.Steps {
			steps[s.ID] = true
		}
		for _, t := range d.Dispatches {
			if t.StepID == "" {
				return fmt.Errorf("%w: dispatch %q requires step_id in a complex plan", ErrInvalid, t.Ref)
			}
			if !steps[t.StepID] {
				return fmt.Errorf("%w: dispatch %q references unknown step %q", ErrInvalid, t.Ref, t.StepID)
			}
		}
	}
	return nil
}

func validateSteps(d Decision) error {
	seen := make(map[string]bool, len(d.Steps))
	for i, s := range d.Steps {
		titleRequired := d.Action != ActionUpdate && d.Action != ActionComplete
		if s.ID == "" || titleRequired && s.Title == "" {
			return fmt.Errorf("%w: step %d needs id and title", ErrInvalid, i+1)
		}
		if seen[s.ID] {
			return fmt.Errorf("%w: duplicate step %q", ErrInvalid, s.ID)
		}
		seen[s.ID] = true
		switch s.Status {
		case StepPending, StepInProgress, StepDone, StepBlocked, StepSkipped:
		default:
			return fmt.Errorf("%w: step %q has invalid status %q", ErrInvalid, s.ID, s.Status)
		}
	}
	return nil
}

// Depth 返回依赖图深度（1 表示无依赖）；未知引用忽略。
func Depth(d Decision) int {
	idx := make(map[string]int, len(d.Dispatches))
	depths := make([]int, len(d.Dispatches))
	max := 0
	for i, t := range d.Dispatches {
		idx[t.Ref] = i
		depth := 1
		for _, list := range [][]string{t.DependsOn, t.ContextFrom} {
			for _, dep := range list {
				if j, ok := idx[dep]; ok && j < i && depths[j]+1 > depth {
					depth = depths[j] + 1
				}
			}
		}
		depths[i] = depth
		if depth > max {
			max = depth
		}
	}
	return max
}

// ApplyConfirmPolicy 计算最终是否需要确认：模型建议 ∪ 宿主规则；destructive 永远强制。
// 返回更新后的 Confirm（Reason 追加触发原因）。
func ApplyConfirmPolicy(d Decision, p ConfirmPolicy) Confirm {
	out := d.Confirm
	mode := strings.ToLower(strings.TrimSpace(p.Mode))
	if mode == ConfirmNever {
		out.Required = false
		out.Reason = ""
	}
	var reasons []string
	if mode == ConfirmAlways {
		reasons = append(reasons, "群策略要求每次确认")
	}
	minN := p.MinDispatches
	if minN == 0 {
		minN = 3
	}
	if minN > 0 && len(d.Dispatches) >= minN {
		reasons = append(reasons, fmt.Sprintf("本轮包含 %d 个任务", len(d.Dispatches)))
	}
	minD := p.MinDepth
	if minD == 0 {
		minD = 2
	}
	if minD > 0 && Depth(d) >= minD {
		reasons = append(reasons, "任务之间存在依赖")
	}
	for _, t := range d.Dispatches {
		if t.Risk == RiskDestructive {
			reasons = append(reasons, "包含破坏性操作")
			break
		}
	}
	if len(reasons) > 0 {
		out.Required = true
		joined := strings.Join(reasons, "；")
		if out.Reason == "" {
			out.Reason = joined
		} else if !strings.Contains(out.Reason, joined) {
			out.Reason += "（" + joined + "）"
		}
	}
	return out
}

// Edit 是人工确认时对某个 dispatch 的修改。
type Edit struct {
	Ref     string  `json:"ref"`
	Message *string `json:"message,omitempty"`
	Remove  bool    `json:"remove,omitempty"`
}

// ApplyEdits 应用人工编辑：只允许改 message 与删除；删除后清理悬空依赖。结果需重新 Validate。
func ApplyEdits(d Decision, edits []Edit) (Decision, error) {
	if len(edits) == 0 {
		return d, nil
	}
	if !d.Confirm.CanEdit() {
		return d, fmt.Errorf("%w: plan is not editable", ErrInvalid)
	}
	byRef := make(map[string]Edit, len(edits))
	for _, e := range edits {
		byRef[strings.TrimSpace(e.Ref)] = e
	}
	removed := map[string]bool{}
	out := make([]Dispatch, 0, len(d.Dispatches))
	for _, t := range d.Dispatches {
		e, ok := byRef[t.Ref]
		if ok && e.Remove {
			removed[t.Ref] = true
			continue
		}
		if ok && e.Message != nil {
			t.Message = strings.TrimSpace(*e.Message)
		}
		out = append(out, t)
	}
	for ref := range byRef {
		found := false
		for _, t := range d.Dispatches {
			if t.Ref == ref {
				found = true
				break
			}
		}
		if !found {
			return d, fmt.Errorf("%w: edit references unknown ref %q", ErrInvalid, ref)
		}
	}
	for i := range out {
		out[i].DependsOn = dropRefs(out[i].DependsOn, removed)
		out[i].ContextFrom = dropRefs(out[i].ContextFrom, removed)
	}
	d.Dispatches = out
	d.Source = SourceEdited
	return d, nil
}

func dropRefs(refs []string, removed map[string]bool) []string {
	if len(refs) == 0 || len(removed) == 0 {
		return refs
	}
	out := refs[:0:0]
	for _, r := range refs {
		if !removed[r] {
			out = append(out, r)
		}
	}
	return out
}
