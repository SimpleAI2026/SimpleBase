package gofunction

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

const helloSrc = `package main

type Request struct {
	Name string ` + "`json:\"name\"`" + `
}

type Response struct {
	Message string ` + "`json:\"message\"`" + `
}

func Hello(req Request) Response {
	return Response{Message: "hello, " + req.Name}
}

func Ping(req Request) Response {
	return Response{Message: "pong"}
}
`

// TestValidateHTTPFuncs_OK 合规源码通过并返回全部导出函数
func TestValidateHTTPFuncs_OK(t *testing.T) {
	infos, err := ValidateHTTPFuncs(helloSrc)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 || infos[0].Name != "Hello" || infos[1].Name != "Ping" {
		t.Fatalf("want [Hello Ping], got %v", infos)
	}
}

// TestValidateHTTPFuncs_MultiParam 多参导出函数报错且指明函数名
func TestValidateHTTPFuncs_MultiParam(t *testing.T) {
	src := `package main
func Bad(a, b int) int { return a + b }
`
	_, err := ValidateHTTPFuncs(src)
	if err == nil {
		t.Fatal("want error for multi-param export")
	}
	if !strings.Contains(err.Error(), "Bad") {
		t.Fatalf("error should mention function name, got: %v", err)
	}
}

// TestValidateHTTPFuncs_TwoResults (R, error) 签名报错
func TestValidateHTTPFuncs_TwoResults(t *testing.T) {
	src := `package main
func Bad() (int, error) { return 0, nil }
`
	_, err := ValidateHTTPFuncs(src)
	if err == nil {
		t.Fatal("want error for (R, error) signature")
	}
	if !strings.Contains(err.Error(), "Bad") {
		t.Fatalf("error should mention function name, got: %v", err)
	}
}

// TestValidateHTTPFuncs_NoExport 仅未导出函数报错
func TestValidateHTTPFuncs_NoExport(t *testing.T) {
	src := `package main
func helper(x int) int { return x }
`
	_, err := ValidateHTTPFuncs(src)
	if err == nil {
		t.Fatal("want error when no exported functions")
	}
}

// TestValidateHTTPFuncs_CompileError 非法 Go 报编译错误
func TestValidateHTTPFuncs_CompileError(t *testing.T) {
	src := `package main
func Broken( {
`
	_, err := ValidateHTTPFuncs(src)
	if err == nil {
		t.Fatal("want compile error")
	}
}

// TestValidateHTTPFuncs_UnregisteredPkg 未注册标准库符号报编译错误
func TestValidateHTTPFuncs_UnregisteredPkg(t *testing.T) {
	src := `package main
import "os"
func F(x string) string { return os.Getenv(x) }
`
	_, err := ValidateHTTPFuncs(src)
	if err == nil {
		t.Fatal("want error for unregistered package")
	}
}

// TestValidateHTTPFuncs_PartialBad 一个合规 + 一个不合规：整体拒绝并指明不合规者
func TestValidateHTTPFuncs_PartialBad(t *testing.T) {
	src := helloSrc + "\nfunc Bad(a, b int) int { return a + b }\n"
	_, err := ValidateHTTPFuncs(src)
	if err == nil {
		t.Fatal("want error when one export is invalid")
	}
	if !strings.Contains(err.Error(), "Bad") {
		t.Fatalf("error should mention Bad, got: %v", err)
	}
}

// TestRunJSON_StructRoundTrip struct + json tag 往返
func TestRunJSON_StructRoundTrip(t *testing.T) {
	out, err := RunJSON(context.Background(), "t", "hello", helloSrc, "Hello", []byte(`{"name":"a"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"message":"hello, a"}` {
		t.Fatalf("got %s", out)
	}
}

// TestRunJSON_MapRoundTrip map[string]interface{} 入参
func TestRunJSON_MapRoundTrip(t *testing.T) {
	src := `package main
import "encoding/json"
func F(m map[string]interface{}) string {
	b, _ := json.Marshal(m)
	return string(b)
}
`
	out, err := RunJSON(context.Background(), "t", "m", src, "F", []byte(`{"k":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `"{\"k\":1}"` {
		t.Fatalf("got %s", out)
	}
}

// TestRunJSON_BasicTypeParam 基本类型入参：body 为 JSON 字面量
func TestRunJSON_BasicTypeParam(t *testing.T) {
	src := `package main
func Upper(s string) string {
	return s + "!"
}
`
	out, err := RunJSON(context.Background(), "t", "b", src, "Upper", []byte(`"abc"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `"abc!"` {
		t.Fatalf("got %s", out)
	}
}

// TestRunJSON_EmptyBody 空 body 视为 {}
func TestRunJSON_EmptyBody(t *testing.T) {
	out, err := RunJSON(context.Background(), "t", "hello", helloSrc, "Ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"message":"pong"}` {
		t.Fatalf("got %s", out)
	}
}

// TestRunJSON_Unexported 未导出函数不可调用
func TestRunJSON_Unexported(t *testing.T) {
	src := `package main
func helper(x int) int { return x }
`
	_, err := RunJSON(context.Background(), "t", "u", src, "helper", []byte(`1`))
	if !errors.Is(err, ErrFunctionNotFound) {
		t.Fatalf("want ErrFunctionNotFound, got %v", err)
	}
}

// TestRunJSON_MissingFunction 函数不存在
func TestRunJSON_MissingFunction(t *testing.T) {
	_, err := RunJSON(context.Background(), "t", "hello", helloSrc, "Nope", []byte(`{}`))
	if !errors.Is(err, ErrFunctionNotFound) {
		t.Fatalf("want ErrFunctionNotFound, got %v", err)
	}
}

// TestRunJSON_BadBody JSON 无法解到入参
func TestRunJSON_BadBody(t *testing.T) {
	_, err := RunJSON(context.Background(), "t", "hello", helloSrc, "Hello", []byte(`not json`))
	if err == nil {
		t.Fatal("want bind error")
	}
	if !strings.Contains(err.Error(), "cannot bind") {
		t.Fatalf("got %v", err)
	}
}

// TestRunJSON_Timeout 死循环脚本触发超时并可识别
func TestRunJSON_Timeout(t *testing.T) {
	src := `package main
func Loop(x int) int {
	for {
	}
	return 0
}
`
	start := time.Now()
	_, err := RunJSON(context.Background(), "t", "loop", src, "Loop", []byte(`1`))
	if err == nil {
		t.Fatal("want timeout error")
	}
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("want ErrTimeout, got %v", err)
	}
	if time.Since(start) > 30*time.Second {
		t.Fatalf("timeout took too long: %v", time.Since(start))
	}
}

// TestRunJSON_CallerCanceled 调用方 ctx 已取消时直接返回
func TestRunJSON_CallerCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RunJSON(ctx, "t", "hello", helloSrc, "Hello", []byte(`{"name":"a"}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

// TestRunJSON_CompileErrorSentinel invoke 时编译失败返回 ErrCompile（§7.2 映射 500 compile_error）
func TestRunJSON_CompileErrorSentinel(t *testing.T) {
	src := `package main
func Bad(x int) int { return undefined_symbol }
`
	_, err := RunJSON(context.Background(), "t", "bad", src, "Bad", []byte(`1`))
	if err == nil {
		t.Fatal("want compile error")
	}
	if !errors.Is(err, ErrCompile) {
		t.Fatalf("want ErrCompile, got %v", err)
	}
}
