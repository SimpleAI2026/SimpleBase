package limit

import "testing"

func TestClamp(t *testing.T) {
	cases := []struct {
		n, def, max, want int
	}{
		{50, 50, 100, 50},
		{0, 50, 100, 50},
		{-5, 50, 100, 50},
		{500, 50, 100, 100}, // 超上限截到上限，而非回落默认值
		{100, 50, 100, 100},
		{7, 1000, 0, 7}, // max<=0 不设上限
	}
	for _, c := range cases {
		if got := Clamp(c.n, c.def, c.max); got != c.want {
			t.Errorf("Clamp(%d, %d, %d) = %d, want %d", c.n, c.def, c.max, got, c.want)
		}
	}
}
