// 文件说明：main.go — gofunction/testdata 的内部实现（__testSet 等）
//
// 用法
//
//	同包调用：__testSet → basic.go, basics.go
//
// 调用架构
//
//	[上游] 仅测试引用：gofunction/internal/interp
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/testdata/main.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] basic.go, basics.go, controlflow.go, flowcontrol.go, funclist.go 等 13 个
//	[关键引用] __testSet ← basic.go, basics.go
//
// 主要符号
//
//	type __testSet struct{…}
//	var TestSet

package testdata

type __testSet struct{}

// TestSet 将测试用例全部定位为TestSet结构体的方法，可以通过反射的方式来遍历调用所有的测试用例
var TestSet = __testSet{}
