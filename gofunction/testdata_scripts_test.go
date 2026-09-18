package gofunction

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

func testdataScript(t *testing.T, name string) string {
	t.Helper()
	_, filename, _, _ := runtime.Caller(0)
	src, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(src)
}

func TestTestdataScripts(t *testing.T) {
	tests := []struct {
		name string
		file string
		run  func(t *testing.T, src string)
	}{
		{
			name: "hello",
			file: "hello.go",
			run: func(t *testing.T, src string) {
				got, err := Run("hello-seq", src, "add", 40, 2)
				if err != nil {
					t.Fatal(err)
				}
				if got != 42 {
					t.Fatalf("add(40, 2) = %#v, want 42", got)
				}
			},
		},
		{
			name: "stdlib",
			file: "stdlib.go",
			run: func(t *testing.T, src string) {
				got, err := Run("stdlib-seq", src, "demo", "gofunction")
				if err != nil {
					t.Fatal(err)
				}
				want := `{"msg":"hello GOFUNCTION"}`
				if got != want {
					t.Fatalf("demo = %#v, want %#v", got, want)
				}
			},
		},
		{
			name: "globals",
			file: "globals.go",
			run: func(t *testing.T, src string) {
				prog, err := BuildProgram("globals-seq", "main", src)
				if err != nil {
					t.Fatal(err)
				}
				before, err := prog.GetGlobalValue("N")
				if err != nil {
					t.Fatal(err)
				}
				if before != 1 {
					t.Fatalf("N after init = %#v, want 1", before)
				}
				if err := prog.SetGlobalValue("N", 9); err != nil {
					t.Fatal(err)
				}
				after, err := prog.GetGlobalValue("N")
				if err != nil {
					t.Fatal(err)
				}
				if after != 9 {
					t.Fatalf("N after SetGlobalValue = %#v, want 9", after)
				}
				got, err := prog.Run("globals-seq", "current")
				if err != nil {
					t.Fatal(err)
				}
				if got != 9 {
					t.Fatalf("current() = %#v, want 9", got)
				}
			},
		},
		{
			name: "funclist",
			file: "funclist.go",
			run: func(t *testing.T, src string) {
				exported, err := ParseFuncList(src, false)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(exported, []string{"Exported"}) {
					t.Fatalf("exportedOnly = %v, want [Exported]", exported)
				}
				all, err := ParseFuncList(src, true)
				if err != nil {
					t.Fatal(err)
				}
				want := map[string]bool{"Exported": true, "hidden": true, "Method": true}
				got := map[string]bool{}
				for _, name := range all {
					got[name] = true
				}
				for name := range want {
					if !got[name] {
						t.Fatalf("exportedAll = %v, missing %q", all, name)
					}
				}
			},
		},
		{
			name: "hostfn",
			file: "hostfn.go",
			run: func(t *testing.T, src string) {
				err := importer.RegisterPackage("example.com/host", "host",
					importer.CreateFunction("Add", func(a, b int) int { return a + b }, "a+b"),
					importer.CreateConstant("Label", "host-add", "demo label"),
				)
				if err != nil && !strings.Contains(err.Error(), "already registered") {
					t.Fatal(err)
				}
				got, err := Run("hostfn-seq", src, "demo")
				if err != nil {
					t.Fatal(err)
				}
				if got != 42 {
					t.Fatalf("host.Add(20, 22) = %#v, want 42", got)
				}
			},
		},
		{
			name: "pool",
			file: "pool.go",
			run: func(t *testing.T, src string) {
				pool := NewDefaultExecutorPool()
				pool.SetServiceName("gofunction-testdata")
				defer pool.Close()

				ex, err := pool.GetExecutor()
				if err != nil {
					t.Fatal(err)
				}
				ex.SetSequenceID("pool-1")
				ex.SetExecutionTimeout(-1)
				got, err := ex.Execute("answer", src)
				if err != nil {
					t.Fatal(err)
				}
				if got != 42 {
					t.Fatalf("first Execute = %#v, want 42", got)
				}
				if ex.GetCostTime() < 0 {
					t.Fatal("cost time should be recorded")
				}
				if ex.GetSequenceID() != "pool-1" {
					t.Fatalf("seq = %q, want pool-1", ex.GetSequenceID())
				}
				ex.Close()

				active, idle := pool.GetStats()
				if pool.GetServiceName() != "gofunction-testdata" {
					t.Fatalf("service = %q", pool.GetServiceName())
				}
				if idle < 1 {
					t.Fatalf("after Close: active=%d idle=%d, want idle >= 1", active, idle)
				}

				ex2, err := pool.GetExecutor()
				if err != nil {
					t.Fatal(err)
				}
				got2, err := ex2.Execute("answer", src)
				if err != nil {
					t.Fatal(err)
				}
				if got2 != 42 {
					t.Fatalf("reused Execute = %#v, want 42", got2)
				}
				ex2.Close()
			},
		},
		{
			name: "controlflow",
			file: "controlflow.go",
			run: func(t *testing.T, src string) {
				got, err := Run("cf-seq", src, "classify", 10)
				if err != nil {
					t.Fatal(err)
				}
				want := "medium/even:55/ok"
				if got != want {
					t.Fatalf("classify(10) = %#v, want %#v", got, want)
				}
			},
		},
		{
			name: "http",
			file: "http.go",
			run: func(t *testing.T, src string) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/ping" {
						http.NotFound(w, r)
						return
					}
					_, _ = w.Write([]byte("pong"))
				}))
				defer srv.Close()

				got, err := Run("http-seq", src, "fetch", srv.URL+"/ping")
				if err != nil {
					t.Fatal(err)
				}
				if got != "pong" {
					t.Fatalf("fetch = %#v, want %q", got, "pong")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := testdataScript(t, tt.file)
			tt.run(t, src)
		})
	}
}
