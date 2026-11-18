package hashx

import "testing"

const emptySum = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func TestSum(t *testing.T) {
	if got := Sum(nil); got != emptySum {
		t.Fatalf("Sum(nil) = %s", got)
	}
	if got := SumString(""); got != emptySum {
		t.Fatalf("SumString(\"\") = %s", got)
	}
	if Sum([]byte("a")) != SumString("a") {
		t.Fatal("Sum and SumString disagree")
	}
}
