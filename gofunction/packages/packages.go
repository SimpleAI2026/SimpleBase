// Package packages 向 importer 注册脚本可用的宿主包。
//
// 解释器无法直接访问宿主程序的包，需要在此以反射方式注册
// 标准库（或任意宿主包）的函数、常量与变量。注册通过 init()
// 自动完成，使用方只需 blank import 本包：
//
//	import _ "github.com/linkxzhou/SimpleBase/gofunction/packages"
//
// 各标准库的注册清单按 std_<pkg>.go 拆分，按测试用例需求逐步补齐。
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
