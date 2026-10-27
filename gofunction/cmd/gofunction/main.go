// 文件说明：main.go — Command gofunction 是 gofunction 解释器的命令行入口
//
// 用法
//
//	包内实现细节，由上层包经包级 API 间接触达。
//
// 调用架构
//
//	[上游] （无，本包为入口）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/cmd/gofunction/main.go
//	         │ 依赖
//	         ▼
//	[下游] gofunction, gofunction/importer, gofunction/packages
//
// 主要符号
//
//	const usage
//	main()
//	run(args []string, stdin io.Reader, stdout, stderr io.Writer) int
//	newFlagSet(name string, stderr io.Writer) *flag.FlagSet
//	readSource(path string, stdin io.Reader) (string, error)
//	var data
//	... 另有 6 个顶层符号

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/linkxzhou/SimpleBase/gofunction"
	"github.com/linkxzhou/SimpleBase/gofunction/importer"
	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

const usage = `用法:
  gofunction run   [-f file.go|-] -func Name [-body JSON] [-timeout 10s]
  gofunction check [-f file.go|-]
  gofunction list  [-f file.go|-] [-all]
  gofunction packages

子命令:
  run       编译并执行脚本中的导出函数（HTTP+JSON 约定），结果写 stdout
  check     校验全部导出函数符合 HTTP+JSON 约定，并列出签名
  list      列出函数名（-all 含方法与未导出函数）
  packages  列出已注册的宿主包 import path

-f 缺省或为 - 时从 stdin 读取源码。`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run 返回进程退出码：0 成功，1 执行失败，2 用法错误。
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "run":
		return cmdRun(rest, stdin, stdout, stderr)
	case "check":
		return cmdCheck(rest, stdin, stdout, stderr)
	case "list":
		return cmdList(rest, stdin, stdout, stderr)
	case "packages":
		return cmdPackages(stdout)
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "未知子命令: %s\n\n%s\n", cmd, usage)
		return 2
	}
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func readSource(path string, stdin io.Reader) (string, error) {
	var (
		data []byte
		err  error
	)
	if path == "" || path == "-" {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func cmdRun(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("run", stderr)
	file := fs.String("f", "-", "源码文件，- 表示 stdin")
	funcName := fs.String("func", "", "要执行的导出函数名")
	body := fs.String("body", "", "传给函数入参的 JSON，缺省视为空")
	timeout := fs.Duration("timeout", 10*time.Second, "整体超时（解释器内部上限同为 10s）")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *funcName == "" {
		fmt.Fprintln(stderr, "run: 缺少 -func")
		return 2
	}
	src, err := readSource(*file, stdin)
	if err != nil {
		fmt.Fprintf(stderr, "run: 读取源码失败: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	out, err := gofunction.RunJSON(ctx, "", "main", src, *funcName, []byte(*body))
	if err != nil {
		fmt.Fprintf(stderr, "run: %s\n", describeRunError(err))
		return 1
	}
	fmt.Fprintln(stdout, string(out))
	return 0
}

func describeRunError(err error) string {
	switch {
	case errors.Is(err, gofunction.ErrCompile):
		return "编译失败: " + err.Error()
	case errors.Is(err, gofunction.ErrFunctionNotFound):
		return err.Error()
	case errors.Is(err, gofunction.ErrBind):
		return "参数绑定失败: " + err.Error()
	case errors.Is(err, gofunction.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return "执行超时: " + err.Error()
	default:
		return err.Error()
	}
}

func cmdCheck(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("check", stderr)
	file := fs.String("f", "-", "源码文件，- 表示 stdin")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	src, err := readSource(*file, stdin)
	if err != nil {
		fmt.Fprintf(stderr, "check: 读取源码失败: %v\n", err)
		return 1
	}
	infos, err := gofunction.ValidateHTTPFuncs(src)
	if err != nil {
		fmt.Fprintf(stderr, "check: %v\n", err)
		return 1
	}
	for _, in := range infos {
		fmt.Fprintf(stdout, "%s(%s) %s\n", in.Name, in.ParamType, in.ResultType)
	}
	return 0
}

func cmdList(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("list", stderr)
	file := fs.String("f", "-", "源码文件，- 表示 stdin")
	all := fs.Bool("all", false, "包含方法与未导出函数")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	src, err := readSource(*file, stdin)
	if err != nil {
		fmt.Fprintf(stderr, "list: 读取源码失败: %v\n", err)
		return 1
	}
	names, err := gofunction.ParseFuncList(src, *all)
	if err != nil {
		fmt.Fprintf(stderr, "list: %v\n", err)
		return 1
	}
	for _, n := range names {
		fmt.Fprintln(stdout, n)
	}
	return 0
}

func cmdPackages(stdout io.Writer) int {
	pkgs := importer.GetAllPackages()
	paths := make([]string, 0, len(pkgs))
	for p := range pkgs {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintln(stdout, p)
	}
	return 0
}
