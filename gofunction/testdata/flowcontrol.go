package testdata

import (
	"fmt"
	"math"
	"time"
)

// For 循环
func (__testSet) For() interface{} {
	sum := 0
	for i := 0; i < 10; i++ {
		sum += i
	}
	return fmt.Sprint(sum)
}

// For2 循环
func (__testSet) For2() interface{} {
	sum := 1
	for sum < 1000 {
		sum += sum
	}
	return fmt.Sprint(sum)
}

func sqrt(x float64) string {
	if x < 0 {
		return sqrt(-x) + "i"
	}
	return fmt.Sprint(math.Sqrt(x))
}

// If 条件
func (__testSet) If() interface{} {
	return fmt.Sprint(sqrt(2), sqrt(-4))
}

func pow(x, n, lim float64) float64 {
	if v := math.Pow(x, n); v < lim {
		return v
	}
	return lim
}

// IfWithShortStatement if的简短语句
func (__testSet) IfWithShortStatement() interface{} {
	return fmt.Sprint(
		pow(3, 2, 10),
		pow(3, 3, 20),
	)
}

func pow2(x, n, lim float64) float64 {
	if v := math.Pow(x, n); v < lim {
		return v
	} else {
		fmt.Printf("%g >= %g\n", v, lim)
	}
	return lim
}

// IfElse if 和 else
func (__testSet) IfElse() interface{} {
	return fmt.Sprint(
		pow(3, 2, 10),
		pow(3, 3, 20),
	)
}

var os = "windows"

// Switch switch
func (__testSet) Switch() interface{} {
	switch os {
	case "darwin":
		return "OS X."
	case "linux":
		return "Linux."
	default:
		// freebsd, openbsd,
		// plan9, windows...
		return fmt.Sprintf("%s.\n", os)
	}
}

// SwitchEvaluationOrder switch 的求值顺序
func (__testSet) SwitchEvaluationOrder() interface{} {
	today := time.Now().Weekday()
	switch time.Saturday {
	case today + 0:
		return "Today."
	case today + 1:
		return "Tomorrow."
	case today + 2:
		return "In two days."
	default:
		return "Too far away."
	}
}

// SwitchWithNoCondition 没有条件的 switch
func (__testSet) SwitchWithNoCondition() interface{} {
	now := time.Now()
	switch {
	case now.Hour() < 12:
		return "Good morning!"
	case now.Hour() < 17:
		return "Good afternoon."
	default:
		return "Good evening."
	}
}

var s string

func testDefer() {
	defer func() { s += "world" }()
	s = "hello"
}

// Defer defer
func (__testSet) Defer() interface{} {
	testDefer()
	return s
}
