// Package gofunction 提供基于 SSA 的 Go 脚本解释执行引擎。
//
// 将 Go 源码编译为 SSA（golang.org/x/tools/go/ssa）后用反射逐指令
// 解释执行，可在不调用 go build 的情况下运行用户脚本，并支持通过
// importer 包将宿主函数注入脚本环境。
//
// 基本用法：
//
//	import _ "github.com/linkxzhou/SimpleBase/gofunction/packages" // 注册标准库
//	result, err := gofunction.Run("seq-1", "package main\nfunc add(a, b int) int { return a + b }", "add", 1, 2)
//
// 详细用法、限制与已注册标准库清单见 README.md；用法样本见 testdata/，由 go test 加载执行。
package gofunction

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
