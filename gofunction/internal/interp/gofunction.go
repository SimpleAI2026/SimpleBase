// 文件说明：gofunction.go — 基于 SSA 的 Go 脚本解释执行引擎
//
// 用法
//
//	Run(seqid, sourceCode string, funcName string, params ...interface{}) (interface{} ...  // Run 编译并执行代码中的指定函数
//	ParseFuncList(sourceCode string, exportedAll bool) ([]string, error)  // ParseFuncList 解析源码并返回函数名列表
//	同包调用：Run() → compile.go, layout.go 等；newSequenceID() → jsonrun.go, runtime.go；SetLogger() → frame.go
//
// 调用架构
//
//	[上游] gofunction
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/internal/interp/gofunction.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] call.go, compile.go, eval.go, executor.go, frame.go 等 13 个
//	[关键引用] Run() ← compile.go, layout.go, program.go
//
// 主要符号
//
//	Run(seqid, sourceCode string, funcName string, params ...interface{}) (interface{} ...
//	ParseFuncList(sourceCode string, exportedAll bool) ([]string, error)
//	SetLogger(l *slog.Logger)
//	SetDebugOutput(w io.Writer)
//	... 另有 1 个顶层符号

package interp

import (
	"crypto/rand"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log/slog"
)

// Run 编译并执行代码中的指定函数
func Run(seqid, sourceCode string, funcName string, params ...interface{}) (interface{}, error) {
	program, err := BuildProgram(seqid, "main", sourceCode)
	if err != nil {
		return nil, err
	}
	return program.Run(seqid, funcName, params...)
}

// ParseFuncList 解析源码并返回函数名列表。
// exportedAll 为 true 时导出所有函数（含方法），否则仅导出包级导出函数。
func ParseFuncList(sourceCode string, exportedAll bool) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", sourceCode, parser.AllErrors)
	if err != nil {
		return nil, err
	}
	var flist []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		// fn.Recv != nil 表示 struct 成员方法
		if fn.Recv != nil && !exportedAll {
			continue
		}
		if exportedAll {
			flist = append(flist, fn.Name.Name)
			continue
		}
		if fn.Name.IsExported() && fn.Name.Name != "init" {
			flist = append(flist, fn.Name.Name)
		}
	}
	return flist, nil
}

// SetLogger 注入解释器内部使用的日志器（默认 slog.Default()）
func SetLogger(l *slog.Logger) {
	if l != nil {
		logger = l
	}
}

// SetDebugOutput 设置调试输出目标（SSA 转储等）
func SetDebugOutput(w io.Writer) {
	if w != nil {
		stderrWriter = w
	}
}

// newSequenceID 生成随机序列 ID
func newSequenceID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "seq-unknown"
	}
	return "seq-" + hex.EncodeToString(buf)
}
