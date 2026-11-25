// 文件说明：validate_test.go — Validate 首错表驱动、Normalize、确认判定与人工编辑（planv6.8 §13.11.1-4）

package plan

import (
	"errors"
	"strings"
	"testing"
)

func baseConstraints() Constraints {
	return Constraints{
		Members: []string{"a1", "a2", "a3"},
		HumanID: "u1",
		Limits:  Limits{MaxDispatches: 4, MaxDepth: 3, MaxMessage: 50},
	}
}

func validDecision() Decision {
	return Decision{
		Action: ActionCreate,
		Goal:   "answer the question",
		Dispatches: []Dispatch{
			{Ref: "d1", TargetKind: TargetAgent, TargetID: "a1", Message: "查资料并给出结论"},
		},
	}
}

// 首错表驱动：每条输入只命中一个哨兵，且错误文本可定位。
func TestValidateFirstErrorTable(t *testing.T) {
	c := baseConstraints()
	over := func(n int) string { return strings.Repeat("x", n) }

	cases := []struct {
		name string
		mut  func(*Decision)
		want error
	}{
		{"unknown action", func(d *Decision) { d.Action = "explode" }, ErrInvalid},
		{"create without goal", func(d *Decision) { d.Goal = "" }, ErrInvalid},
		{"create without dispatches", func(d *Decision) { d.Dispatches = nil }, ErrEmpty},
		{"too many dispatches", func(d *Decision) {
			for i := 0; i < 5; i++ {
				d.Dispatches = append(d.Dispatches, Dispatch{Ref: "e", TargetKind: TargetAgent, TargetID: "a2", Message: "m"})
			}
		}, ErrTooLarge},
		{"duplicate ref", func(d *Decision) {
			d.Dispatches = append(d.Dispatches, Dispatch{Ref: "d1", TargetKind: TargetAgent, TargetID: "a2", Message: "m"})
		}, ErrInvalid},
		{"empty message", func(d *Decision) { d.Dispatches[0].Message = "" }, ErrInvalid},
		{"message too long", func(d *Decision) { d.Dispatches[0].Message = over(51) }, ErrInvalid},
		{"unknown risk", func(d *Decision) { d.Dispatches[0].Risk = "nuclear" }, ErrInvalid},
		{"non-member target", func(d *Decision) { d.Dispatches[0].TargetID = "zz" }, ErrMemberMissing},
		{"human mismatch id", func(d *Decision) { d.Dispatches[0].TargetKind = TargetHuman; d.Dispatches[0].TargetID = "u2" }, ErrHumanUnavailable},
		{"human disabled", func(d *Decision) {
			c.HumanDisabled = true
			d.Dispatches[0].TargetKind = TargetHuman
			d.Dispatches[0].TargetID = "u1"
		}, ErrHumanDisabled},
		{"depends on unknown", func(d *Decision) { d.Dispatches[0].DependsOn = []string{"nope"} }, ErrInvalid},
		{"depends on later", func(d *Decision) {
			d.Dispatches = append(d.Dispatches, Dispatch{Ref: "d2", TargetKind: TargetAgent, TargetID: "a2", Message: "m"})
			d.Dispatches[0].DependsOn = []string{"d2"}
		}, ErrInvalid},
		{"depth exceeded", func(d *Decision) {
			d.Dispatches = []Dispatch{
				{Ref: "d1", TargetKind: TargetAgent, TargetID: "a1", Message: "m"},
				{Ref: "d2", TargetKind: TargetAgent, TargetID: "a2", Message: "m", DependsOn: []string{"d1"}},
				{Ref: "d3", TargetKind: TargetAgent, TargetID: "a3", Message: "m", DependsOn: []string{"d2"}},
				{Ref: "d4", TargetKind: TargetAgent, TargetID: "a1", Message: "m", DependsOn: []string{"d3"}},
			}
		}, ErrTooDeep},
		{"complex without steps for step ref", func(d *Decision) {
			d.Complexity = ComplexityComplex
			d.Dispatches[0].StepID = "s1"
		}, ErrInvalid},
		{"update with dispatches", func(d *Decision) { d.Action = ActionUpdate }, ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := baseConstraints()
			if tc.name == "human disabled" {
				c.HumanDisabled = true
			}
			d := validDecision()
			tc.mut(&d)
			err := Validate(d, c)
			if err == nil {
				t.Fatal("want error")
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want sentinel %v", err, tc.want)
			}
		})
	}
}

// @ 硬约束：被点名的成员必须出现，否则 ErrMentionMissing。
func TestValidateMentionHardConstraint(t *testing.T) {
	c := baseConstraints()
	c.Mentioned = []string{"a1", "a2"}
	d := validDecision()
	err := Validate(d, c)
	if !errors.Is(err, ErrMentionMissing) || !strings.Contains(err.Error(), "a2") {
		t.Fatalf("err = %v, want ErrMentionMissing mentioning a2", err)
	}
	d.Dispatches = append(d.Dispatches, Dispatch{Ref: "d2", TargetKind: TargetAgent, TargetID: "a2", Message: "m"})
	if err := Validate(d, c); err != nil {
		t.Fatalf("covered mentions must pass: %v", err)
	}
}

// @ 了人但没有 human dispatch：ErrMentionMissing。
func TestValidateMentionedHuman(t *testing.T) {
	c := baseConstraints()
	c.MentionedHuman = true
	d := validDecision()
	err := Validate(d, c)
	if !errors.Is(err, ErrMentionMissing) {
		t.Fatalf("err = %v, want ErrMentionMissing", err)
	}
	d.Dispatches = append(d.Dispatches, Dispatch{Ref: "d2", TargetKind: TargetHuman, TargetID: "u1", Message: "请确认"})
	if err := Validate(d, c); err != nil {
		t.Fatalf("human dispatch satisfies mention: %v", err)
	}
}

// update/complete 只允许进度字段。
func TestValidateUpdateOnlyProgress(t *testing.T) {
	c := baseConstraints()
	d := Decision{Action: ActionUpdate, Progress: Progress{Percent: 40}}
	if err := Validate(d, c); err != nil {
		t.Fatalf("pure progress update: %v", err)
	}
	d = Decision{Action: ActionComplete, Progress: Progress{Percent: 100}}
	if err := Validate(d, c); err != nil {
		t.Fatalf("complete: %v", err)
	}
}

func TestValidateComplexDispatchRequiresStep(t *testing.T) {
	d := validDecision()
	d.Complexity = ComplexityComplex
	d.Steps = []Step{{ID: "s1", Title: "step", Status: StepPending}}
	if err := Validate(d, baseConstraints()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("complex dispatch without step_id error = %v, want ErrInvalid", err)
	}
	d.Dispatches[0].StepID = "s1"
	if err := Validate(d, baseConstraints()); err != nil {
		t.Fatalf("valid complex dispatch: %v", err)
	}
}

// Normalize：@ 前缀、人类别名、默认 ref、complexity、越界 percent。
func TestNormalize(t *testing.T) {
	c := baseConstraints()
	d := Decision{
		Action:     "  Create ",
		Goal:       " g ",
		Complexity: "COMPLEX",
		Dispatches: []Dispatch{
			{TargetKind: "assistant", TargetID: "@a1", Message: " m "},
			{TargetKind: "agent", TargetID: "me", Message: "请确认"},
			{TargetKind: "agent", TargetID: "@not-a-member", Message: "x"},
		},
		Progress: Progress{Percent: 250},
	}
	out := Normalize(d, c)
	if out.Action != ActionCreate || out.Goal != "g" || out.Complexity != ComplexityComplex {
		t.Fatalf("normalized basics: %+v", out)
	}
	if out.Dispatches[0].Ref != "d1" || out.Dispatches[0].TargetID != "a1" {
		t.Fatalf("ref default and @ prefix: %+v", out.Dispatches[0])
	}
	if out.Dispatches[1].TargetKind != TargetHuman || out.Dispatches[1].TargetID != "u1" {
		t.Fatalf("human alias: %+v", out.Dispatches[1])
	}
	// 非成员 id 只去 @ 前缀，不虚构成员（由 Validate 报 ErrMemberMissing）。
	if out.Dispatches[2].TargetID != "not-a-member" {
		t.Fatalf("non-member preserved: %+v", out.Dispatches[2])
	}
	if out.Progress.Percent != 100 {
		t.Fatalf("percent clamp = %d", out.Progress.Percent)
	}
}

// 确认判定：模型建议 ∪ 宿主规则；never 不放宽 destructive；always 永远要求。
func TestApplyConfirmPolicy(t *testing.T) {
	destructive := validDecision()
	destructive.Dispatches[0].Risk = RiskDestructive

	// 模型建议。
	d := validDecision()
	d.Confirm = Confirm{Required: true, Reason: "多步破坏性操作"}
	if out := ApplyConfirmPolicy(d, ConfirmPolicy{}); !out.Required {
		t.Fatal("model-requested confirm must stay required")
	}
	// destructive 强制。
	d = validDecision()
	d.Confirm = Confirm{}
	d.Dispatches[0].Risk = RiskDestructive
	if out := ApplyConfirmPolicy(d, ConfirmPolicy{}); !out.Required || !strings.Contains(out.Reason, "破坏性") {
		t.Fatalf("destructive forces confirm: %+v", out)
	}
	// never 不能放宽 destructive。
	d = destructive
	d.Confirm = Confirm{}
	if out := ApplyConfirmPolicy(d, ConfirmPolicy{Mode: ConfirmNever}); !out.Required {
		t.Fatal("never must not relax destructive")
	}
	// never 忽略模型建议（非 destructive）。
	d = validDecision()
	d.Confirm = Confirm{Required: true, Reason: "model"}
	if out := ApplyConfirmPolicy(d, ConfirmPolicy{Mode: ConfirmNever}); out.Required {
		t.Fatal("never ignores model suggestion")
	}
	// never 不关闭宿主任务数与依赖深度强制规则。
	d.Dispatches = append(d.Dispatches,
		Dispatch{Ref: "d2", TargetKind: TargetAgent, TargetID: "a2", Message: "m"},
		Dispatch{Ref: "d3", TargetKind: TargetAgent, TargetID: "a3", Message: "m"})
	if out := ApplyConfirmPolicy(d, ConfirmPolicy{Mode: ConfirmNever}); !out.Required {
		t.Fatal("never must keep dispatch-count confirmation")
	}
	d = validDecision()
	d.Dispatches = append(d.Dispatches, Dispatch{Ref: "d2", TargetKind: TargetAgent, TargetID: "a2", Message: "m", DependsOn: []string{"d1"}})
	if out := ApplyConfirmPolicy(d, ConfirmPolicy{Mode: ConfirmNever}); !out.Required {
		t.Fatal("never must keep dependency-depth confirmation")
	}
	// 任务数阈值（默认 3）。
	d = validDecision()
	d.Confirm = Confirm{}
	d.Dispatches = append(d.Dispatches,
		Dispatch{Ref: "d2", TargetKind: TargetAgent, TargetID: "a2", Message: "m"},
		Dispatch{Ref: "d3", TargetKind: TargetAgent, TargetID: "a3", Message: "m"})
	if out := ApplyConfirmPolicy(d, ConfirmPolicy{}); !out.Required || !strings.Contains(out.Reason, "3 个任务") {
		t.Fatalf("min dispatches: %+v", out)
	}
	// 依赖深度阈值（默认 2）。
	d = validDecision()
	d.Confirm = Confirm{}
	d.Dispatches = append(d.Dispatches, Dispatch{Ref: "d2", TargetKind: TargetAgent, TargetID: "a2", Message: "m", DependsOn: []string{"d1"}})
	if out := ApplyConfirmPolicy(d, ConfirmPolicy{}); !out.Required || !strings.Contains(out.Reason, "依赖") {
		t.Fatalf("min depth: %+v", out)
	}
	// always。
	d = validDecision()
	d.Confirm = Confirm{}
	if out := ApplyConfirmPolicy(d, ConfirmPolicy{Mode: ConfirmAlways}); !out.Required {
		t.Fatal("always forces confirm")
	}
}

// 人工编辑：改 message / 删除 dispatch / 清理悬空依赖；未知 ref 报错；不可编辑拒绝。
func TestApplyEdits(t *testing.T) {
	d := Decision{
		Action: ActionCreate, Goal: "g",
		Dispatches: []Dispatch{
			{Ref: "d1", TargetKind: TargetAgent, TargetID: "a1", Message: "one"},
			{Ref: "d2", TargetKind: TargetAgent, TargetID: "a2", Message: "two", DependsOn: []string{"d1"}, ContextFrom: []string{"d1"}},
		},
	}
	msg := "edited"
	out, err := ApplyEdits(d, []Edit{{Ref: "d1", Message: &msg}})
	if err != nil || out.Dispatches[0].Message != "edited" || out.Source != SourceEdited {
		t.Fatalf("edit message: %v %+v", err, out)
	}

	out, err = ApplyEdits(d, []Edit{{Ref: "d1", Remove: true}})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(out.Dispatches) != 1 || out.Dispatches[0].Ref != "d2" {
		t.Fatalf("remaining dispatch: %+v", out.Dispatches)
	}
	if len(out.Dispatches[0].DependsOn) != 0 || len(out.Dispatches[0].ContextFrom) != 0 {
		t.Fatalf("dangling refs must be dropped: %+v", out.Dispatches[0])
	}

	if _, err := ApplyEdits(d, []Edit{{Ref: "zz"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown ref: %v", err)
	}

	no := false
	d.Confirm = Confirm{Editable: &no}
	if _, err := ApplyEdits(d, []Edit{{Ref: "d1", Remove: true}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("not editable: %v", err)
	}
}

// Depth：线性深度与未知引用忽略。
func TestDepth(t *testing.T) {
	d := Decision{Dispatches: []Dispatch{
		{Ref: "d1"}, {Ref: "d2", DependsOn: []string{"d1"}}, {Ref: "d3", DependsOn: []string{"d2", "ghost"}},
	}}
	if got := Depth(d); got != 3 {
		t.Fatalf("depth = %d, want 3", got)
	}
}
