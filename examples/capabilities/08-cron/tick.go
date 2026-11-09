//go:build ignore

package main

type TickIn struct {
	N int `json:"n"`
}

type TickOut struct {
	N int `json:"n"`
}

func Tick(in TickIn) TickOut {
	return TickOut{N: in.N + 1}
}
