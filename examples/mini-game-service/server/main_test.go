package main

import "testing"

func TestValidScore(t *testing.T) {
	for _, c := range []struct {
		score int64
		want  bool
	}{{0, true}, {1000, true}, {-1, false}, {1001, false}} {
		if validScore(c.score) != c.want {
			t.Fatalf("score %d: want %v", c.score, c.want)
		}
	}
}
