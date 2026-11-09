package testdata

import (
	"fmt"
	"math"
	"strings"
)

// Pointers 指针
func (__testSet) Pointers() interface{} {
	i, j := 42, 2701
	p := &i      // 指向 i
	*p = 21      // 通过指针设置 i 的值
	p = &j       // 指向 j
	*p = *p / 37 // 通过指针对 j 进行除法运算
	return fmt.Sprint(i, j, *p)
}

type Vertex struct {
	X float64
	Y float64
}

// Structs 结构体
func (__testSet) Structs() interface{} {
	return fmt.Sprint(Vertex{1, 2})
}

// StructsFields 结构体字段
func (__testSet) StructsFields() interface{} {
	v := Vertex{1, 2}
	v.X = 4
	return fmt.Sprint(v.X)
}

// StructsPointers 结构体指针
func (__testSet) StructsPointers() interface{} {
	v := Vertex{1, 2}
	p := &v
	p.X = 1e9
	return fmt.Sprint(v)
}

// StructLiterals 结构体文法
func (__testSet) StructLiterals() interface{} {
	var (
		v1 = Vertex{1, 2}  // 创建一个 Vertex 类型的结构体
		v2 = Vertex{X: 1}  // Y:0 被隐式地赋予
		v3 = Vertex{}      // X:0 Y:0
		p  = &Vertex{1, 2} // 创建一个 *Vertex 类型的结构体（指针）
	)
	return fmt.Sprint(v1, p, v2, v3)
}

// Array 数组
func (__testSet) Array() interface{} {
	var a [2]string
	a[0] = "Hello"
	a[1] = "World"
	primes := [6]int{2, 3, 5, 7, 11, 13}
	return fmt.Sprint(a[0], a[1], a, primes)
}

// Slice 切片
func (__testSet) Slice() interface{} {
	primes := [6]int{2, 3, 5, 7, 11, 13}
	var s []int = primes[1:4]
	return fmt.Sprint(s)
}

// SlicePointers 切片指针
func (__testSet) SlicePointers() interface{} {
	names := [4]string{
		"John",
		"Paul",
		"George",
		"Ringo",
	}

	a := names[0:2]
	b := names[1:3]

	b[0] = "XXX"
	return fmt.Sprint(names, a, b)
}

// SliceLiterals 切片文法
func (__testSet) SliceLiterals() interface{} {
	q := []int{2, 3, 5, 7, 11, 13}

	r := []bool{true, false, true, true, false, true}

	s := []struct {
		I int
		B bool
	}{
		{2, true},
		{3, false},
		{5, true},
		{7, true},
		{11, false},
		{13, true},
	}
	return fmt.Sprint(q, r, s)
}

// SliceAddr 切片地址
func (__testSet) SliceAddr() interface{} {
	c := 1
	d := 2
	a := []*int{&c, &d}
	a[0] = a[1]
	return a
}

// SliceBounds 切片边界
func (__testSet) SliceBounds() interface{} {
	s := []int{2, 3, 5, 7, 11, 13}

	s1 := s[1:4]
	s2 := s[:2]
	s3 := s[1:]
	return fmt.Sprint(s1, s2, s3)
}

// NilSlices nil 切片
func (__testSet) NilSlices() interface{} {
	var s []int
	return fmt.Sprint(s, len(s), cap(s), s)
	// TODO return fmt.Sprint(s, len(s), cap(s), s == nil)
}

// MakingSlices 创建切片
func (__testSet) MakingSlices() interface{} {
	a := make([]int, 5)
	b := make([]int, 0, 5)
	c := b[:2]
	d := c[2:5]
	return fmt.Sprint(a, b, c, d)
}

// SlicesOfSlices 切片的切片
func (__testSet) SlicesOfSlices() interface{} {
	// 创建一个井字板（经典游戏）
	board := [][]string{
		[]string{"_", "_", "_"},
		[]string{"_", "_", "_"},
		[]string{"_", "_", "_"},
	}

	// 两个玩家轮流打上 X 和 O
	board[0][0] = "X"
	board[2][2] = "O"
	board[1][2] = "X"
	board[1][0] = "O"
	board[0][2] = "X"
	output := ""
	for i := 0; i < len(board); i++ {
		output += fmt.Sprintf("%s\n", strings.Join(board[i], " "))
	}
	return output
}

// Append 向切片追加元素
func (__testSet) Append() interface{} {
	var s []int
	// 添加一个空切片
	s = append(s, 0)
	// 这个切片会按需增长
	s = append(s, 1)
	// 可以一次性添加多个元素
	s = append(s, 2, 3, 4)
	return fmt.Sprint(s)
}

// Range 切片遍历
func (__testSet) Range() interface{} {
	var pow = []int{1, 2, 4, 8, 16, 32, 64, 128}
	output := ""
	for i, v := range pow {
		output += fmt.Sprintf("2**%d = %d\n", i, v)
	}
	return output
}

// Range2 切片遍历2
func (__testSet) Range2() interface{} {
	pow := make([]int, 10)
	for i := range pow {
		pow[i] = 1 << uint(i) // == 2**i
	}
	output := ""
	for _, value := range pow {
		output += fmt.Sprintf("%d\n", value)
	}
	return output
}

// Maps 映射
func (__testSet) Maps() interface{} {
	var m map[string]Vertex
	m = make(map[string]Vertex)
	m["Bell Labs"] = Vertex{
		40, -74,
	}
	return fmt.Sprint(m["Bell Labs"])
}

// MapsLiterals 映射的文法
func (__testSet) MapsLiterals() interface{} {
	var m = map[string]Vertex{
		"Bell Labs": Vertex{
			40, -74,
		},
		"Google": Vertex{
			37, -122,
		},
	}
	return fmt.Sprint(m["Google"])
}

// MapsLiterals2 映射的文法
func (__testSet) MapsLiterals2() interface{} {
	var m = map[string]Vertex{
		"Bell Labs": {40, -74},
		"Google":    {37, -122},
	}
	return fmt.Sprint(m["Bell Labs"])
}

// MutatingMaps 修改映射
func (__testSet) MutatingMaps() interface{} {
	result := []interface{}{}
	m := make(map[string]int)
	m["Answer"] = 42
	result = append(result, m["Answer"])
	m["Answer"] = 48
	result = append(result, m["Answer"])
	delete(m, "Answer")
	result = append(result, m["Answer"])
	v, ok := m["Answer"]
	result = append(result, v, ok)
	return result
}
func compute(fn func(float64, float64) float64) float64 {
	return fn(3, 4)
}

// FunctionValues 函数值
func (__testSet) FunctionValues() interface{} {
	hypot := func(x, y float64) float64 {
		return math.Sqrt(x*x + y*y)
	}
	return fmt.Sprint(hypot(5, 12), compute(hypot), compute(math.Pow))
}
func adder() func(int) int {
	sum := 0
	return func(x int) int {
		sum += x
		return sum
	}
}

// FunctionClosures 函数的闭包
func (__testSet) FunctionClosures() interface{} {
	result := []interface{}{}
	pos, neg := adder(), adder()
	for i := 0; i < 10; i++ {
		result = append(result, pos(i), neg(-2*i))
	}
	return result
}
