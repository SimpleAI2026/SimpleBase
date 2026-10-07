package goexample

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

type Window struct {
	Start string `json:"window_start"`
	End   string `json:"window_end"`
}
type Run struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	ResponseJSON string `json:"response_json"`
}
type Job struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type JobList struct {
	Jobs []Job `json:"jobs"`
}
type RunList struct {
	Runs []Run `json:"runs"`
}

func (r *Runtime) Poll(ctx context.Context, jobName string, apply func(context.Context, Window, Run) error) error {
	var jobs JobList
	if err := r.Request(ctx, http.MethodGet, r.ProjectPath("/cron-jobs"), nil, &jobs); err != nil {
		return err
	}
	for _, job := range jobs.Jobs {
		if job.Name == jobName {
			var response RunList
			if err := r.Request(ctx, http.MethodGet, r.ProjectPath("/cron-jobs/"+job.ID+"/runs?limit=100"), nil, &response); err != nil {
				return err
			}
			if len(response.Runs) >= 100 {
				return errors.New("100 run visibility limit reached; stop and reconcile manually")
			}
			for i := len(response.Runs) - 1; i >= 0; i-- {
				run := response.Runs[i]
				if run.Status != "completed" {
					continue
				}
				var window Window
				if err := json.Unmarshal([]byte(run.ResponseJSON), &window); err != nil {
					return fmt.Errorf("invalid run response: %w", err)
				}
				start, err := time.Parse(time.RFC3339, window.Start)
				if err != nil {
					return err
				}
				end, err := time.Parse(time.RFC3339, window.End)
				if err != nil {
					return err
				}
				if !end.After(start) || end.Sub(start) > 24*time.Hour || end.After(time.Now().Add(time.Minute)) {
					return errors.New("invalid window")
				}
				if err := apply(ctx, window, run); err != nil {
					return err
				}
			}
			return nil
		}
	}
	return nil
}
func (r *Runtime) StartWorker(ctx context.Context, jobName string, apply func(context.Context, Window, Run) error) {
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := r.Poll(ctx, jobName, apply); err != nil {
					fmt.Fprintf(os.Stderr, "%s worker stopped: %v\n", jobName, err)
					return
				}
			}
		}
	}()
}
