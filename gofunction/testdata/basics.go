// 文件说明：basics.go — gofunction/testdata 中 __testSet.Imports、__testSet.Functions 等符号的实现
//
// 用法
//
//	(__testSet) Imports() interface{}  // Imports 导入
//	(__testSet) Functions() interface{}  // Functions 函数
//	(__testSet) Functions2() interface{}  // Functions2 函数
//	(__testSet) MultipleResult() interface{}  // MultipleResult 多值返回
//	(__testSet) NamedResults() interface{}  // NamedResults 命名返回值
//	(__testSet) Variables() interface{}  // Variables 变量
//
// 调用架构
//
//	[上游] 仅测试引用：gofunction/internal/interp
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/testdata/basics.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] basic.go, controlflow.go, flowcontrol.go, funclist.go, globals.go 等 13 个
//
// 主要符号
//
//	(__testSet) Imports() interface{}
//	add(x int, y int) int
//	(__testSet) Functions() interface{}
//	add2(x, y int) int
//	(__testSet) Functions2() interface{}
//	swap(x, y string) (string, string)
//	(__testSet) MultipleResult() interface{}
//	split(sum int) (x, y int)
//	... 另有 14 个顶层符号

package testdata

import (
	"fmt"
	"math"
)

// Imports 导入
func (__testSet) Imports() interface{} {
	return math.Sqrt(7)
}

func add(x int, y int) int {
	return x + y
}

// Functions 函数
func (__testSet) Functions() interface{} {
	return add(42, 13)
}

func add2(x, y int) int {
	return x + y
}

// Functions2 函数
func (__testSet) Functions2() interface{} {
	return add2(42, 13)
}

func swap(x, y string) (string, string) {
	return y, x
}

// MultipleResult 多值返回
func (__testSet) MultipleResult() interface{} {
	a, b := swap("hello", "world")
	return fmt.Sprint(a, b)
}

func split(sum int) (x, y int) {
	x = sum * 4 / 9
	y = sum - x
	return
}

// NamedResults 命名返回值
func (__testSet) NamedResults() interface{} {
	return fmt.Sprint(split(17))
}

var c, python, java bool

// Variables 变量
func (__testSet) Variables() interface{} {
	var i int
	return fmt.Sprint(i, c, python, java)
}

var i, j int = 1, 2

// VariablesWithInit 变量的初始化
func (__testSet) VariablesWithInit() interface{} {
	var c, python, java = true, false, "no!"
	return fmt.Sprint(i, c, python, java)
}

// ShortVariableDeclaration 短变量声明
func (__testSet) ShortVariableDeclaration() interface{} {
	var i, j int = 1, 2
	k := 3
	c, python, java := true, false, "no!"
	return fmt.Sprint(i, j, k, c, python, java)
}

// Zero 零值
func (__testSet) Zero() interface{} {
	var i int
	var f float64
	var b bool
	var s string
	return fmt.Sprintf("%v %v %v %q\n", i, f, b, s)
}

// TypeConversions 类型转换
func (__testSet) TypeConversions() interface{} {
	var x, y int = 3, 4
	var f float64 = math.Sqrt(float64(x*x + y*y))
	var z uint = uint(f)
	return fmt.Sprint(x, y, z)
}

const Pi = 3.14

// Constants 常量
func (__testSet) Constants() interface{} {
	const World = "世界"
	const Truth = true
	return fmt.Sprint(Pi, World, Truth)
}

const (
	Big   = 1 << 100
	Small = Big >> 99
)

func needInt(x int) int { return x*10 + 1 }
func needFloat(x float64) float64 {
	return x * 0.1
}

// NumericConstants 数值常量
func (__testSet) NumericConstants() interface{} {
	return fmt.Sprint(needInt(Small), needFloat(Small), needFloat(Big))
}
