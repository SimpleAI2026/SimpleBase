package gofunction

import (
	"testing"

	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

func TestSlowEvalFallback(t *testing.T) {
	skipFastEval = true
	t.Cleanup(func() { skipFastEval = false })

	src := `package main
func All() int {
	var u uint = 8
	var f float64 = 2.5
	s := "a" + "b"
	i := 10 + 2 - 3*2/2%3
	i = i & 7 | 1 ^ 2
	i = i &^ 1
	i = (1 << 2) + (8 >> 1)
	_ = u + 1 - 1*1/1%2&3 | 1 ^ 1
	_ = u << 1 >> 1
	_ = f + 1 - 1*1/1
	_ = s < "c" && s <= "ab" && s > "a" && s >= "ab"
	_ = 1 < 2 && 2 <= 2 && 3 > 1 && 3 >= 3 && 1 == 1 && 1 != 0
	_ = u < 9 && u <= 8 && u > 1 && u >= 8
	_ = f < 3 && f <= 2.5 && f > 1 && f >= 2.5
	_ = -i + ^i
	_ = -u + ^u
	_ = -f
	_ = !false
	if s == "ab" && s != "x" {
		i++
	}
	return i + len(s)
}
`
	v, err := Run("slow", src, "All")
	if err != nil {
		t.Fatal(err)
	}
	if v == nil {
		t.Fatal("nil")
	}
}
