//go:build ignore

package main

import "time"

type WindowInput struct{}
type TimeWindow struct {
	Start string `json:"start"`
	End string `json:"end"`
}
type Point struct { Value float64 `json:"value"` }
type AggregateInput struct { Points []Point `json:"points"` }
type AggregateResult struct { Avg float64 `json:"avg"`; Max float64 `json:"max"`; Count int `json:"count"` }

func Window(_ WindowInput) TimeWindow {
	end := time.Now().UTC().Truncate(5 * time.Minute)
	return TimeWindow{Start: end.Add(-5 * time.Minute).Format("2006-01-02T15:04:05Z07:00"), End: end.Format("2006-01-02T15:04:05Z07:00")}
}
func Aggregate(in AggregateInput) AggregateResult {
	var out AggregateResult
	for _, point := range in.Points {
		out.Avg += point.Value
		if out.Count == 0 || point.Value > out.Max { out.Max = point.Value }
		out.Count++
	}
	if out.Count > 0 { out.Avg /= float64(out.Count) }
	return out
}
