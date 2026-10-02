//go:build ignore

package main

import "time"

type TickInput struct{}

type Window struct {
	WindowStart string `json:"window_start"`
	WindowEnd   string `json:"window_end"`
}
type Row struct {
	CreatedAt string `json:"created_at"`
}
type ComputeInput struct {
	Rows []Row `json:"rows"`
}
type Summary struct {
	Count int64 `json:"count"`
}

func Tick(_ TickInput) Window {
	end := time.Now().UTC().Truncate(time.Hour)
	return Window{WindowStart: end.Add(-time.Hour).Format("2006-01-02T15:04:05Z07:00"), WindowEnd: end.Format("2006-01-02T15:04:05Z07:00")}
}

func Compute(in ComputeInput) Summary {
	return Summary{Count: int64(len(in.Rows))}
}
