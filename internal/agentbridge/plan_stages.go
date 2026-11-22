package agentbridge

import (
	"fmt"
	"strings"
)

// PlanStageID identifies a stage independently of its display language.
type PlanStageID string

const (
	PlanStagePlanning  PlanStageID = "plan"
	PlanStageDatabase  PlanStageID = "database"
	PlanStageKV        PlanStageID = "kv"
	PlanStageFunctions PlanStageID = "functions"
	PlanStageCron      PlanStageID = "cron"
	PlanStageSite      PlanStageID = "site"
)

// PlanStageSpec describes a proposed stage, not a tool authorization.
type PlanStageSpec struct {
	ID         PlanStageID   `json:"id"`
	Goal       string        `json:"goal,omitempty"`
	SkipReason string        `json:"skip_reason,omitempty"`
	DependsOn  []PlanStageID `json:"depends_on,omitempty"`
}

func PlanStageOrder() []PlanStageID {
	return []PlanStageID{PlanStagePlanning, PlanStageDatabase, PlanStageKV, PlanStageFunctions, PlanStageCron, PlanStageSite}
}

// ValidatePlanStages enforces the fixed stage order and only permits dependencies
// on earlier stages. Omitted work must be represented by a skip reason.
func ValidatePlanStages(stages []PlanStageSpec) error {
	order := PlanStageOrder()
	if len(stages) != len(order) {
		return fmt.Errorf("plan requires %d stages", len(order))
	}
	seen := make(map[PlanStageID]struct{}, len(order))
	for i, stage := range stages {
		if stage.ID != order[i] {
			return fmt.Errorf("stage %d must be %q", i, order[i])
		}
		hasGoal := strings.TrimSpace(stage.Goal) != ""
		hasSkipReason := strings.TrimSpace(stage.SkipReason) != ""
		if hasGoal == hasSkipReason || (i == 0 && hasSkipReason) {
			return fmt.Errorf("invalid goal or skip reason for stage %q", stage.ID)
		}
		dependencies := make(map[PlanStageID]struct{}, len(stage.DependsOn))
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
