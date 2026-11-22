package agentbridge

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPlanStageOrderAndValidation(t *testing.T) {
	order := PlanStageOrder()
	if len(order) != 6 || order[0] != PlanStagePlanning || order[2] != PlanStageKV || order[5] != PlanStageSite {
		t.Fatalf("order=%v", order)
	}
	valid := []PlanStageSpec{
		{ID: PlanStagePlanning, Goal: "plan"},
		{ID: PlanStageDatabase, Goal: "tables"},
		{ID: PlanStageKV, SkipReason: "no kv"},
		{ID: PlanStageFunctions, Goal: "fn", DependsOn: []PlanStageID{PlanStageDatabase}},
		{ID: PlanStageCron, SkipReason: "no cron"},
		{ID: PlanStageSite, Goal: "site", DependsOn: []PlanStageID{PlanStageDatabase, PlanStageFunctions}},
	}
	if err := ValidatePlanStages(valid); err != nil {
		t.Fatalf("valid plan rejected: %v", err)
	}
	cases := map[string][]PlanStageSpec{
		"missing stage": valid[:5],
		"wrong order": {
			{ID: PlanStageDatabase, Goal: "x"}, {ID: PlanStagePlanning, Goal: "x"},
			{ID: PlanStageKV, SkipReason: "x"}, {ID: PlanStageFunctions, SkipReason: "x"},
			{ID: PlanStageCron, SkipReason: "x"}, {ID: PlanStageSite, SkipReason: "x"},
		},
		"goal and skip": {
			{ID: PlanStagePlanning, Goal: "x"}, {ID: PlanStageDatabase, Goal: "x", SkipReason: "y"},
			{ID: PlanStageKV, SkipReason: "x"}, {ID: PlanStageFunctions, SkipReason: "x"},
			{ID: PlanStageCron, SkipReason: "x"}, {ID: PlanStageSite, SkipReason: "x"},
		},
		"blank goal": {
			{ID: PlanStagePlanning, Goal: "   "}, {ID: PlanStageDatabase, SkipReason: "x"},
			{ID: PlanStageKV, SkipReason: "x"}, {ID: PlanStageFunctions, SkipReason: "x"},
			{ID: PlanStageCron, SkipReason: "x"}, {ID: PlanStageSite, SkipReason: "x"},
		},
		"plan skipped": {
			{ID: PlanStagePlanning, SkipReason: "x"}, {ID: PlanStageDatabase, SkipReason: "x"},
			{ID: PlanStageKV, SkipReason: "x"}, {ID: PlanStageFunctions, SkipReason: "x"},
			{ID: PlanStageCron, SkipReason: "x"}, {ID: PlanStageSite, SkipReason: "x"},
		},
		"forward dependency": {
			{ID: PlanStagePlanning, Goal: "x"}, {ID: PlanStageDatabase, Goal: "x", DependsOn: []PlanStageID{PlanStageSite}},
			{ID: PlanStageKV, SkipReason: "x"}, {ID: PlanStageFunctions, SkipReason: "x"},
			{ID: PlanStageCron, SkipReason: "x"}, {ID: PlanStageSite, SkipReason: "x"},
		},
		"duplicate dependency": {
			{ID: PlanStagePlanning, Goal: "x"}, {ID: PlanStageDatabase, Goal: "x"},
			{ID: PlanStageKV, Goal: "x", DependsOn: []PlanStageID{PlanStageDatabase, PlanStageDatabase}},
			{ID: PlanStageFunctions, SkipReason: "x"}, {ID: PlanStageCron, SkipReason: "x"},
			{ID: PlanStageSite, SkipReason: "x"},
		},
	}
	for name, stages := range cases {
		if err := ValidatePlanStages(stages); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}

func TestPlanStageSkills(t *testing.T) {
	for _, stage := range PlanStageOrder() {
		if _, _, ok := PlanStageSkills(stage); !ok {
			t.Fatalf("stage %q has no skills", stage)
		}
	}
	if _, _, ok := PlanStageSkills("nope"); ok {
		t.Fatal("unknown stage must be rejected")
	}
	skills, readonly, _ := PlanStageSkills(PlanStageFunctions)
	if len(skills) != 2 || len(readonly) != 1 || readonly[0] != "database" {
		t.Fatalf("functions skills=%v readonly=%v", skills, readonly)
	}
	if skills, readonly, _ := PlanStageSkills(PlanStagePlanning); len(skills) != 0 || len(readonly) != 0 {
		t.Fatalf("planning must load no skills: %v %v", skills, readonly)
	}
}

func TestPlanStagePrompt(t *testing.T) {
	p := PlanStagePrompt(PlanStageSite)
	for _, want := range []string{"Plan mode", "Current stage: site", "web/", "never delete"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	if strings.Contains(p, "sb_live_") {
		t.Error("prompt must forbid keys, not embed them")
	}
}

func TestPlanStageDone(t *testing.T) {
	card := func(argv []string, content string, isError bool) string {
		args, _ := json.Marshal(map[string]any{"argv": argv})
		raw, _ := json.Marshal([]map[string]any{{
			"name": "simplebase", "arguments": string(args), "content": content, "is_error": isError,
		}})
		return string(raw)
	}
	cases := []struct {
		stage PlanStageID
		raw   string
		want  bool
	}{
		{PlanStageDatabase, card([]string{"sql", "exec"}, `{"ok":true}`, false), true},
		{PlanStageDatabase, card([]string{"sql", "exec"}, `{"ok":false}`, false), false},
		{PlanStageDatabase, card([]string{"sql", "exec"}, `{"ok":true}`, true), false},
		{PlanStageDatabase, card([]string{"sql", "query"}, `{"ok":true}`, false), false},
		{PlanStageKV, card([]string{"kv", "exec"}, `{"ok":true}`, false), true},
		{PlanStageFunctions, card([]string{"function", "test"}, `{"ok":true}`, false), true},
		{PlanStageCron, card([]string{"cron", "create"}, `{"ok":true}`, false), true},
		{PlanStageSite, card([]string{"site", "publish"}, `{"ok":true}`, false), true},
		{PlanStageSite, card([]string{"site", "status"}, `{"ok":true}`, false), false},
		{PlanStageSite, "[]", false},
		{PlanStageSite, "not json", false},
	}
	for _, tc := range cases {
		if got := PlanStageDone(tc.stage, tc.raw); got != tc.want {
			t.Errorf("%s %s: got %v want %v", tc.stage, tc.raw, got, tc.want)
		}
	}
	if !PlanStageDone(PlanStagePlanning, "[]") {
		t.Error("planning stage is judged by message content, not tools")
	}
}
