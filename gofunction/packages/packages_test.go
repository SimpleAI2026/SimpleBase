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
