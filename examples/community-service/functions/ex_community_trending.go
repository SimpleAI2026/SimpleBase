//go:build ignore

package main

import "time"

type TickInput struct{}
type Window struct { WindowStart string `json:"window_start"`; WindowEnd string `json:"window_end"` }
type Row struct { PostID string `json:"post_id"`; LikeCount int64 `json:"like_count"`; AgeMinutes int64 `json:"age_minutes"` }
type ComputeInput struct { Rows []Row `json:"rows"` }
type Item struct { PostID string `json:"post_id"`; Score int64 `json:"score"` }
type Result struct { Items []Item `json:"items"` }
func Tick(_ TickInput) Window {end:=time.Now().UTC().Truncate(time.Hour);return Window{end.Add(-time.Hour).Format("2006-01-02T15:04:05Z07:00"),end.Format("2006-01-02T15:04:05Z07:00")}}
func Compute(in ComputeInput) Result {
 out:=Result{Items:[]Item{}}
 for _,row:=range in.Rows{score:=row.LikeCount*10-row.AgeMinutes/60;if score<0{score=0};out.Items=append(out.Items,Item{row.PostID,score})}
 for i:=0;i<len(out.Items);i++{for j:=i+1;j<len(out.Items);j++{if out.Items[j].Score>out.Items[i].Score{out.Items[i],out.Items[j]=out.Items[j],out.Items[i]}}}
 if len(out.Items)>10{out.Items=out.Items[:10]};return out
}
