//go:build ignore

package main

import "time"

type TickInput struct{}
type Window struct { WindowStart string `json:"window_start"`; WindowEnd string `json:"window_end"` }
type Row struct { PlayerID string `json:"player_id"`; Score int64 `json:"score"` }
type ComputeInput struct { Rows []Row `json:"rows"` }
type Result struct { Ranks []Row `json:"ranks"` }
func Tick(_ TickInput) Window {end:=time.Now().UTC().Truncate(time.Hour);return Window{end.Add(-time.Hour).Format("2006-01-02T15:04:05Z07:00"),end.Format("2006-01-02T15:04:05Z07:00")}}
func Compute(in ComputeInput) Result {
  ranks:=append([]Row{},in.Rows...)
  for i:=0;i<len(ranks);i++{for j:=i+1;j<len(ranks);j++{if ranks[j].Score>ranks[i].Score || (ranks[j].Score==ranks[i].Score&&ranks[j].PlayerID<ranks[i].PlayerID){ranks[i],ranks[j]=ranks[j],ranks[i]}}}
  if len(ranks)>10{ranks=ranks[:10]};return Result{ranks}
}
