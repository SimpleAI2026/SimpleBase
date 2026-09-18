package gofunction

import (
	"go/ast"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
	"github.com/linkxzhou/SimpleBase/gofunction/testdata"

	"golang.org/x/tools/go/ssa"
)

const seqid = "gotest"

// testCase 执行测试用例，会通过解释器和原生go分别执行指定名称的函数并比较
// 两者的返回值是否一致，funcName用于指定需要执行的函数名，funcName为空时执行所有函数
func testCase(t *testing.T, funcName string) {
	if funcName != "" {
		debugging = true // 测试单个函数时，打开debug开关，输出执行详情
	}
	_, filename, _, _ := runtime.Caller(0)
	testdataDir := filepath.Join(filepath.Dir(filename), "testdata")
	dir, err := os.ReadDir(testdataDir)
	if err != nil {
		t.Error(err)
		return
	}

	testSet := reflect.ValueOf(testdata.TestSet)
	for _, file := range dir {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".go") || file.Name() == "main.go" {
			continue
		}

		source, err := os.ReadFile(filepath.Join(testdataDir, file.Name()))
		if err != nil {
			t.Error(err)
			return
		}
		// Usage samples in testdata/ are package main scripts (often //go:build ignore)
		// loaded by TestTestdataScripts, not dual-executed against testdata.TestSet.
		if !strings.Contains(string(source), `func (__testSet)`) {
			continue
		}
		src := strings.Replace(string(source), `func (__testSet)`, `func `, -1)
		program, err := BuildProgram(seqid, "testSet", src)
		if err != nil {
			t.Errorf("build %s: %v", file.Name(), err)
			continue
		}
		for name, member := range program.mainPkg.Members {
			if _, ok := member.(*ssa.Function); ok {
				if !ast.IsExported(name) || (len(funcName) > 0 && name != funcName) {
					continue
				}
				if testing.Short() && (name == "NilInterface") {
					t.Logf("skip %s in short mode (network)", name)
					continue
				}
				t.Log("test", name)
				result, err := program.Run("", name)
				if err != nil {
					t.Errorf("run func %s: %v", name, err)
					continue
				}
				method := testSet.MethodByName(name)
				if !method.IsValid() {
					t.Errorf("native testdata method %s not found", name)
					continue
				}
				expected := method.Call(nil)[0].Interface()
				if !reflect.DeepEqual(result, expected) {
					t.Errorf("func %s expected %#v got %#v", name, expected, result)
				}
			}
		}
	}
}

func TestAll(t *testing.T) {
	testCase(t, "")
}

// TestImportGofun 测试将编译成的库导入到其他脚本程序中
func TestImportGofun(t *testing.T) {
	sources := `
package test
	
import "pkg1"
import "pkg2"

var A = "1"
func test() string {
	return A + pkg1.F() + pkg2.S
}
	`

	pkg1 := `
package pkg1
func F() string {
	return "hello"
}
`

	pkg2 := `
package pkg2
const S = "world"
func F() string {
	return "world"
}
`
	p1, err := BuildProgram(seqid, "pkg1", pkg1)
	if err != nil {
		t.Error(err)
		return
	}
	p2, err := BuildProgram(seqid, "pkg2", pkg2)
	if err != nil {
		t.Error(err)
		return
	}

	p, err := BuildProgram(seqid, "main", sources, p1.mainPkg, p2.mainPkg)
	if err != nil {
		t.Error(err)
		return
	}

	out, err := p.Run("", "test")
	if err != nil {
		t.Error(err)
		return
	}
	expected := "1helloworld"
	if !reflect.DeepEqual(out, expected) {
		t.Errorf("Expected %#v got %#v.", expected, out)
	}
}

// BenchmarkFib 递归计算斐波那契数列，测试解释器的执行性能
func BenchmarkFib(b *testing.B) {
	b.StopTimer()
	b.ReportAllocs()
	code := `
package test

func fib(i int) int {
	if i < 2 {
		return i
	}
	return fib(i - 1) + fib(i - 2)
}

func test(i int) int {
	return fib(i)
}
`
	interpreter, err := BuildProgram(seqid, "test", code)
	if err != nil {
		b.Error(err)
		return
	}

	var ret interface{}
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ret, err = interpreter.Run("", "test", 25)
	}
	b.Log(ret, err)
}

// TestGetGlobalValue 测试获取全局变量的值
func TestGetGlobalValue(t *testing.T) {
	sources := `
package main

var exports = map[string]interface{}{
	"test": 1,
}
	`
	interpreter, err := BuildProgram(seqid, "test", sources)
	if err != nil {
		t.Error(err)
		return
	}
	exports, err := interpreter.GetGlobalValue("exports")
	if err != nil {
		t.Error(err)
		return
	}
	if exports == nil {
		t.Error("exports should not be nil")
		return
	}
	m, ok := exports.(map[string]interface{})
	if !ok {
		t.Errorf("expected map[string]interface{}, got %T", exports)
		return
	}
	if m["test"] != 1 {
		t.Errorf("expected exports[\"test\"] == 1, got %v", m["test"])
	}
}

// TestRunFunction 测试函数执行
func TestRunFunction(t *testing.T) {
	sources := `
package main

func testFunction(req map[string]interface{}) string {
	var i int = 0
	for ; i < 10; i++ {
		println("testFunction ==== ", i)
	}
	println("testFunction ==== ")
	println("req: ", req)
	return "hello world "
}

func testFunction1() string {
	var i int = 0
	for ; i < 10; i++ {
		println("testFunction ==== ", i)
	}
	println("testFunction ==== ")
	return "hello world "
}


var Exports = map[string]interface{}{
	"testFunction": testFunction,
	"testFunction1": testFunction1,
}

var test = testFunction(nil)
var test1 = testFunction1()
	`
	interpreter, err := BuildProgram(seqid, "test", sources)
	if err != nil {
		t.Error(err)
		return
	}
	req := map[string]interface{}{
		"test": 1,
	}
	result, _, err := interpreter.RunWithContext("", "testFunction", req)
	if err != nil {
		t.Error(err)
		return
	}
	if result != "hello world " {
		t.Errorf("expected \"hello world \", got %#v", result)
	}
}
