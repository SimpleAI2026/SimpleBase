package hashx

import "testing"

const emptySum = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func TestSum(t *testing.T) {
	if got := Sum(nil); got != emptySum {
		t.Fatalf("Sum(nil) = %s", got)
	}
	if got := Sum([]byte("")); got != emptySum {
		t.Fatalf("Sum(\"\") = %s", got)
	}
}

func TestShort(t *testing.T) {
	if got := Short(nil, 8); got != emptySum[:16] {
		t.Fatalf("Short(nil, 8) = %s", got)
	}
	if Short([]byte("x"), 0) != "" || Short([]byte("x"), -1) != "" {
		t.Fatal("n<=0 should be empty")
	}
	if got := Short([]byte("x"), 100); got != Sum([]byte("x")) {
		t.Fatalf("n>32 should clamp to full digest, got %s", got)
	}
}
