package limit

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		n, def, max, want int
	}{
		{50, 20, 100, 50},
		{0, 20, 100, 20},
		{-1, 20, 100, 20},
		{500, 20, 100, 20},  // 超上限回落默认值
		{100, 20, 100, 100}, // 恰好等于上限保留
		{5, 20, 0, 5},       // max<=0 不设上限
		{0, 20, 0, 20},
	}
	for _, c := range cases {
		if got := Normalize(c.n, c.def, c.max); got != c.want {
			t.Errorf("Normalize(%d, %d, %d) = %d, want %d", c.n, c.def, c.max, got, c.want)
		}
	}
}

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
