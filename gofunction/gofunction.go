// Package gofunction 提供基于 SSA 的 Go 脚本解释执行引擎。
//
// 将 Go 源码编译为 SSA（golang.org/x/tools/go/ssa）后用反射逐指令
// 解释执行，可在不调用 go build 的情况下运行用户脚本，并支持通过
// importer 包将宿主函数注入脚本环境。
//
// 本包是**公共 API 门面**：类型以别名、函数以薄转发暴露
// `gofunction/internal/interp` 的实现，import 路径与历史版本一致。
// 目录结构：
//
//	gofunction/            门面（Run / BuildProgram / RunJSON / Executor…）
//	gofunction/internal/interp/  解释器实现（SSA 编译、指令执行、池化）
//	gofunction/importer/   宿主包注册与 go/types Importer
//	gofunction/packages/   标准库注册（blank-import 生效）
//	gofunction/value/      解释器值抽象
//	gofunction/testdata/   用法样本（由 go test 加载）
//	gofunction/cmd/gofunction/  命令行入口
//
// 基本用法：
//
//	import _ "github.com/linkxzhou/SimpleBase/gofunction/packages" // 注册标准库
//	result, err := gofunction.Run("seq-1", "package main\nfunc add(a, b int) int { return a + b }", "add", 1, 2)
//
// 详细用法、限制与已注册标准库清单见 README.md。
package gofunction

import (
	"io"
	"log/slog"

	"github.com/linkxzhou/SimpleBase/gofunction/internal/interp"
	"golang.org/x/tools/go/ssa"
)

// 类型别名：方法集随实现包自动继承，调用方无需改动。
type (
	// Program 代码编译后生成的结构体
	Program = interp.Program
	// Context 函数执行的上下文环境
	Context = interp.Context
	// Executor Go 代码执行器
	Executor = interp.Executor
	// ExecutorPool 执行器对象池
	ExecutorPool = interp.ExecutorPool
	// ExecutorPoolConfig 执行器池配置
	ExecutorPoolConfig = interp.ExecutorPoolConfig
	// Runtime Go 代码运行时环境
	Runtime = interp.Runtime
	// FuncInfo 描述一个合规的导出 HTTP 函数
	FuncInfo = interp.FuncInfo
)

// Run 编译并执行代码中的指定函数
func Run(seqid, sourceCode string, funcName string, params ...interface{}) (interface{}, error) {
	return interp.Run(seqid, sourceCode, funcName, params...)
}

// BuildProgram 编译代码。packages 为可选的已编译包（供脚本 import）
func BuildProgram(seqid, fname, sourceCode string, packages ...*ssa.Package) (*Program, error) {
	return interp.BuildProgram(seqid, fname, sourceCode, packages...)
}

// ParseFuncList 解析源码并返回函数名列表。
// exportedAll 为 true 时导出所有函数（含方法），否则仅导出包级导出函数。
func ParseFuncList(sourceCode string, exportedAll bool) ([]string, error) {
	return interp.ParseFuncList(sourceCode, exportedAll)
}

// SetLogger 注入解释器内部使用的日志器（默认 slog.Default()）
func SetLogger(l *slog.Logger) { interp.SetLogger(l) }

// SetDebugOutput 设置调试输出目标（SSA 转储等）
func SetDebugOutput(w io.Writer) { interp.SetDebugOutput(w) }

// NewRuntime 创建新的运行时实例
func NewRuntime() *Runtime { return interp.NewRuntime() }

// NewExecutor 创建新的执行器实例；pool 可为 nil（脱离池使用）
func NewExecutor(pool *ExecutorPool) *Executor { return interp.NewExecutor(pool) }

// NewExecutorPool 创建新的执行器池
func NewExecutorPool(config ExecutorPoolConfig) *ExecutorPool { return interp.NewExecutorPool(config) }

// NewDefaultExecutorPool 创建使用默认配置的执行器池
func NewDefaultExecutorPool() *ExecutorPool { return interp.NewDefaultExecutorPool() }

// 池操作错误哨兵
var (
	// ErrPoolClosed 池已关闭
	ErrPoolClosed = interp.ErrPoolClosed
	// ErrPoolExhausted 池已达 MaxActive 上限
	ErrPoolExhausted = interp.ErrPoolExhausted
)

// DefaultExecutorPoolConfig 默认执行器池配置
var DefaultExecutorPoolConfig = interp.DefaultExecutorPoolConfig
