// 文件说明：packages.go — 向 importer 注册脚本可用的宿主包
//
// 用法
//
//	同包调用：funcObj() → std_encoding.go, std_fmt.go 等；typeObj() → std_encoding.go, std_http.go 等；constObj() → std_http.go, std_math.go 等
//
// 调用架构
//
//	[上游] gofunction/bench, gofunction/cmd/gofunction, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/packages/packages.go
//	         │ 依赖
//	         ▼
//	[下游] gofunction/importer
//	[同包] std_encoding.go, std_fmt.go, std_http.go, std_math.go, std_misc.go 等 7 个
//	[关键引用] funcObj() ← std_encoding.go, std_fmt.go, std_http.go
//
// 主要符号
//
//	init()
//	mustRegister(path, name string, objects ...*importer.Object)
//	funcObj(name string, fn interface{}) *importer.Object
//	constObj(name string, value interface{}) *importer.Object
//	varObj(name string, addr interface{}, typ reflect.Type) *importer.Object
//	typeObj(name string, typ reflect.Type) *importer.Object

package packages

import (
	"fmt"
	"reflect"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
)

func init() {
	mustRegister("fmt", "fmt", stdFmt()...)
	mustRegister("strings", "strings", stdStrings()...)
	mustRegister("math", "math", stdMath()...)
	mustRegister("time", "time", stdTime()...)
	mustRegister("regexp", "regexp", stdRegexp()...)
	mustRegister("encoding/json", "json", stdJSON()...)
	mustRegister("encoding/base64", "base64", stdBase64()...)
	mustRegister("encoding/binary", "binary", stdBinary()...)
	mustRegister("bytes", "bytes", stdBytes()...)
	mustRegister("net/http", "http", stdHTTP()...)
	mustRegister("errors", "errors", stdErrors()...)
	mustRegister("strconv", "strconv", stdStrconv()...)
	mustRegister("sync/atomic", "atomic", stdAtomic()...)
	mustRegister("io", "io", stdIO()...)
	mustRegister("io/ioutil", "ioutil", stdIoutil()...)
	mustRegister("github.com/json-iterator/go", "jsoniter", stdJSONIter()...)
}

// mustRegister 注册包，失败时 panic（注册发生在启动期，属编程错误）
func mustRegister(path, name string, objects ...*importer.Object) {
	if err := importer.RegisterPackage(path, name, objects...); err != nil {
		panic(fmt.Sprintf("gofunction/packages: register %s: %v", path, err))
	}
}

// funcObj 注册函数
func funcObj(name string, fn interface{}) *importer.Object {
	return importer.CreateFunction(name, fn, "")
}

// constObj 注册常量
func constObj(name string, value interface{}) *importer.Object {
	return importer.CreateConstant(name, value, "")
}

// varObj 注册变量
func varObj(name string, addr interface{}, typ reflect.Type) *importer.Object {
	return importer.CreateVariable(name, addr, typ, "")
}

// typeObj 注册类型
func typeObj(name string, typ reflect.Type) *importer.Object {
	return importer.CreateType(name, typ, "")
}
