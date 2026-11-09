//go:build ignore

package main

// 假密钥，仅用于校验 fixture。
var leaked = "sk-aaaaaaaaaaaaaaaaaaaaaaaa"

var leakedSB = "sb_live_00000000000000000000000000000000"

func Leak(in string) string { return leaked + leakedSB + in }
