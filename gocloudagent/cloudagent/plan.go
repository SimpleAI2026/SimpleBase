package cloudagent

import (
	"fmt"
	"strings"
)

// PlanStageSpec describes one unit of planned work, not a tool authorization.
type PlanStageSpec struct {
	ID         string   `json:"id"`
	Goal       string   `json:"goal,omitempty"`
	SkipReason string   `json:"skip_reason,omitempty"`
	DependsOn  []string `json:"depends_on,omitempty"`
}

// ValidatePlanStages validates a provider-defined, topologically ordered plan.
// Dependencies must refer to distinct earlier stages.
func ValidatePlanStages(stages []PlanStageSpec) error {
	if len(stages) == 0 {
		return fmt.Errorf("plan requires at least one stage")
	}
	seen := make(map[string]struct{}, len(stages))
	for i, stage := range stages {
		stage.ID = strings.TrimSpace(stage.ID)
		if stage.ID == "" {
			return fmt.Errorf("stage %d has an empty id", i)
		}
		if _, ok := seen[stage.ID]; ok {
			return fmt.Errorf("stage %q is duplicated", stage.ID)
		}
		hasGoal := strings.TrimSpace(stage.Goal) != ""
		hasSkipReason := strings.TrimSpace(stage.SkipReason) != ""
		if hasGoal == hasSkipReason {
			return fmt.Errorf("invalid goal or skip reason for stage %q", stage.ID)
		}
		dependencies := make(map[string]struct{}, len(stage.DependsOn))
		for _, dependency := range stage.DependsOn {
			if _, ok := seen[dependency]; !ok {
				return fmt.Errorf("stage %q depends on non-preceding stage %q", stage.ID, dependency)
			}
			if _, ok := dependencies[dependency]; ok {
				return fmt.Errorf("stage %q repeats dependency %q", stage.ID, dependency)
			}
			dependencies[dependency] = struct{}{}
		}
		seen[stage.ID] = struct{}{}
	}
	return nil
}
