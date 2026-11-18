package vm

import "testing"

func TestClampPTYDim(t *testing.T) {
	if ClampPTYDim(0, 80) != 80 {
		t.Fatal("zero must fall back to the default")
	}
	if ClampPTYDim(50, 80) != 50 {
		t.Fatal("a value within the limit must be kept")
	}
	if ClampPTYDim(PTYDimLimit, 80) != PTYDimLimit {
		t.Fatal("the limit itself is valid")
	}
	if ClampPTYDim(PTYDimLimit+1, 80) != PTYDimLimit {
		t.Fatal("values above the limit must be clamped")
	}
}
