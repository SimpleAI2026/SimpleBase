package agentbridge

import (
	"context"
	"fmt"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// RunContext is SimpleBase's provider-owned request context.
type RunContext struct {
	ProjectID      string
	Principal      auth.Principal
	Skills         []string
	ReadOnlySkills []string
	PlanStage      PlanStageID

	RunID    string
	ThreadID string
	Headless bool
	Writable bool
	Emit     func(cloudagent.Event)
}

func hostContext(value any) (RunContext, error) {
	switch rc := value.(type) {
	case RunContext:
		return rc, nil
	case *RunContext:
		if rc != nil {
			return *rc, nil
		}
	}
	return RunContext{}, fmt.Errorf("simplebase agent run context missing")
}

func withRunContext(ctx context.Context, rc RunContext) context.Context {
	return cloudagent.WithRunContext(ctx, cloudagent.RunContext{
		RunID: rc.RunID, ThreadID: rc.ThreadID, Value: rc, Emit: rc.Emit,
	})
}

func requireRun(ctx context.Context) (RunContext, error) {
	generic, ok := cloudagent.RunContextFrom(ctx)
	if !ok {
		return RunContext{}, fmt.Errorf("agent run context missing")
	}
	rc, err := hostContext(generic.Value)
	if err != nil || rc.ProjectID == "" {
		return RunContext{}, fmt.Errorf("agent run context missing")
	}
	rc.RunID = generic.RunID
	rc.ThreadID = generic.ThreadID
	rc.Emit = generic.Emit
	return rc, nil
}

func principalKey(p auth.Principal) string {
	if p.UserID != "" {
		return p.UserID
	}
	if p.APIKeyID != "" {
		return "apikey:" + p.APIKeyID
	}
	return ""
}
