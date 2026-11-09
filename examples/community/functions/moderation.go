//go:build ignore

package main

import "strings"

type CheckInput struct { Title string `json:"title"`; Body string `json:"body"` }
type CheckResult struct { Allowed bool `json:"allowed"` }
type ScoreItem struct { Likes int64 `json:"likes"` }
type ScoreInput struct { Items []ScoreItem `json:"items"` }
type ScoreResult struct { Scores []int64 `json:"scores"` }

func Check(in CheckInput) CheckResult { return CheckResult{Allowed: len(in.Title) > 0 && len(in.Body) > 0 && !strings.Contains(in.Title+in.Body, "spam")} }
func Score(in ScoreInput) ScoreResult {
	out := ScoreResult{Scores: []int64{}}
	for _, item := range in.Items { out.Scores = append(out.Scores, item.Likes * 10) }
	return out
}
