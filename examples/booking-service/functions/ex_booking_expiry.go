//go:build ignore

package main

import "time"

type TickInput struct{}
type Window struct { WindowStart string `json:"window_start"`; WindowEnd string `json:"window_end"` }
type Candidate struct { ID string `json:"id"`; ExpiresAt string `json:"expires_at"`; Status string `json:"status"` }
type ComputeInput struct { Cutoff string `json:"cutoff"`; Rows []Candidate `json:"rows"` }
type Result struct { IDs []string `json:"ids"` }
func Tick(_ TickInput) Window {
  end := time.Now().UTC().Truncate(time.Minute)
  return Window{end.Add(-5 * time.Minute).Format("2006-01-02T15:04:05Z07:00"), end.Format("2006-01-02T15:04:05Z07:00")}
}
func Compute(in ComputeInput) Result {
  out := Result{IDs: []string{}}
  for _, row := range in.Rows { if row.Status == "confirmed" && row.ExpiresAt < in.Cutoff { out.IDs = append(out.IDs, row.ID) } }
  return out
}
