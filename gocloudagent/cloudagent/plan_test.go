package cloudagent

import "testing"

func TestValidatePlanStages(t *testing.T) {
	valid := []PlanStageSpec{
		{ID: "inspect", Goal: "inspect the workspace"},
		{ID: "unused", SkipReason: "not needed"},
		{ID: "build", Goal: "build the result", DependsOn: []string{"inspect"}},
	}
	if err := ValidatePlanStages(valid); err != nil {
		t.Fatal(err)
	}
	cases := [][]PlanStageSpec{
		nil,
		{{ID: "", Goal: "x"}},
		{{ID: "a", Goal: "x"}, {ID: "a", Goal: "y"}},
		{{ID: "a"}},
		{{ID: "a", Goal: "x", SkipReason: "y"}},
		{{ID: "a", Goal: "x", DependsOn: []string{"missing"}}},
		{{ID: "a", Goal: "x"}, {ID: "b", Goal: "y", DependsOn: []string{"a", "a"}}},
	}
	for _, stages := range cases {
		if err := ValidatePlanStages(stages); err == nil {
			t.Fatalf("expected invalid plan: %+v", stages)
		}
	}
}
