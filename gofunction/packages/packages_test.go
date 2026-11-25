// 文件说明：packages_test.go — gofunction/packages 的单元测试（TestStdlibPackagesRegistered、TestFmtSprintRegistered 等）
//
// 用法
//
//	go test ./gofunction/packages -run 'TestStdlibPackagesRegistered|TestFmtSprintRegistered|TestMustRegisterDuplicatePanics|TestAtomicAddInt32Registered'
//
// 调用架构
//
//	[上游] gofunction/bench, gofunction/cmd, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/packages/packages_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] packages.go, std_encoding.go, std_fmt.go, std_http.go, std_math.go 等 8 个
//
// 主要符号
//
//	TestStdlibPackagesRegistered(t *testing.T)
//	TestFmtSprintRegistered(t *testing.T)
//	TestMustRegisterDuplicatePanics(t *testing.T)
//	TestAtomicAddInt32Registered(t *testing.T)

package packages

import (
	"testing"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
)

func TestStdlibPackagesRegistered(t *testing.T) {
	required := []string{
		"fmt", "strings", "math", "time", "regexp",
		"encoding/json", "encoding/base64", "bytes",
		"net/http", "errors", "strconv", "sync/atomic",
		"io", "io/ioutil", "encoding/binary",
		"github.com/json-iterator/go",
	}
	pkgs := importer.GetAllPackages()
	for _, path := range required {
		pkg, ok := pkgs[path]
		if !ok {
			t.Errorf("package %s not registered", path)
			continue
		}
		if len(pkg.Objects) == 0 {
			t.Errorf("package %s has no objects", path)
		}
	}
}

func TestFmtSprintRegistered(t *testing.T) {
	pkg, ok := importer.GetAllPackages()["fmt"]
	if !ok {
		t.Fatal("fmt not registered")
	}
	found := false
	for _, obj := range pkg.Objects {
		if obj.Name == "Sprintf" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("fmt.Sprintf not registered")
	}
}

func TestMustRegisterDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate register")
		}
	}()
	mustRegister("fmt", "fmt")
}

func TestAtomicAddInt32Registered(t *testing.T) {
	pkg, ok := importer.GetAllPackages()["sync/atomic"]
	if !ok {
		t.Fatal("sync/atomic not registered")
	}
	found := false
	for _, obj := range pkg.Objects {
		if obj.Name == "AddInt32" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("atomic.AddInt32 not registered")
	}
}
