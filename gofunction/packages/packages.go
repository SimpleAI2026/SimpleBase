// Package packages 向 importer 注册脚本可用的宿主包。
//
// 解释器无法直接访问宿主程序的包，需要在此以反射方式注册
// 标准库（或任意宿主包）的函数、常量与变量。注册通过 init()
// 自动完成，使用方只需 blank import 本包：
//
//	import _ "github.com/linkxzhou/SimpleBase/gofunction/packages"
package packages

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
)

func init() {
	mustRegister("fmt", "fmt", fmtObjects()...)
	mustRegister("strings", "strings", stringsObjects()...)
	mustRegister("math", "math", mathObjects()...)
	mustRegister("time", "time", timeObjects()...)
	mustRegister("regexp", "regexp", regexpObjects()...)
	mustRegister("encoding/json", "json", jsonObjects()...)
	mustRegister("encoding/base64", "base64", base64Objects()...)
	mustRegister("bytes", "bytes", bytesPlaceholder()...)
	mustRegister("net/http", "http", httpObjects()...)
	mustRegister("errors", "errors", errorsObjects()...)
	mustRegister("strconv", "strconv", strconvObjects()...)
	mustRegister("github.com/json-iterator/go", "jsoniter", jsoniterObjects()...)
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

// --- fmt ---

func fmtObjects() []*importer.Object {
	return []*importer.Object{
		funcObj("Sprintf", fmt.Sprintf),
		funcObj("Sprint", fmt.Sprint),
		funcObj("Sprintln", fmt.Sprintln),
		funcObj("Printf", fmt.Printf),
		funcObj("Println", fmt.Println),
		funcObj("Print", fmt.Print),
		funcObj("Errorf", fmt.Errorf),
	}
}

// --- strings ---

func stringsObjects() []*importer.Object {
	return []*importer.Object{
		funcObj("ToUpper", strings.ToUpper),
		funcObj("ToLower", strings.ToLower),
		funcObj("Contains", strings.Contains),
		funcObj("HasPrefix", strings.HasPrefix),
		funcObj("HasSuffix", strings.HasSuffix),
		funcObj("Split", strings.Split),
		funcObj("Join", strings.Join),
		funcObj("Replace", strings.Replace),
		funcObj("ReplaceAll", strings.ReplaceAll),
		funcObj("TrimSpace", strings.TrimSpace),
		funcObj("Index", strings.Index),
		funcObj("Repeat", strings.Repeat),
	}
}

// --- math ---

func mathObjects() []*importer.Object {
	return []*importer.Object{
		funcObj("Sqrt", math.Sqrt),
		funcObj("Abs", math.Abs),
		funcObj("Floor", math.Floor),
		funcObj("Ceil", math.Ceil),
		funcObj("Round", math.Round),
		funcObj("Max", math.Max),
		funcObj("Min", math.Min),
		funcObj("Pow", math.Pow),
		funcObj("Mod", math.Mod),
		funcObj("Trunc", math.Trunc),
		funcObj("Log", math.Log),
		funcObj("Log10", math.Log10),
		funcObj("Log2", math.Log2),
		funcObj("Exp", math.Exp),
		funcObj("Sin", math.Sin),
		funcObj("Cos", math.Cos),
		funcObj("Tan", math.Tan),
		constObj("Pi", math.Pi),
		constObj("E", math.E),
		constObj("Ln10", math.Ln10),
		constObj("Ln2", math.Ln2),
		constObj("Sqrt2", math.Sqrt2),
		constObj("MaxInt", math.MaxInt),
		constObj("MinInt", math.MinInt),
	}
}

// --- time ---

func timeObjects() []*importer.Object {
	// time 包主要暴露常用常量与函数；Duration 方法集通过反射自动可用
	return []*importer.Object{
		funcObj("Now", time.Now),
		funcObj("LoadLocation", time.LoadLocation),
		funcObj("Date", time.Date),
		funcObj("Parse", time.Parse),
		funcObj("Since", time.Since),
		funcObj("Unix", time.Unix),
		constObj("Second", time.Second),
		constObj("Millisecond", time.Millisecond),
		constObj("Microsecond", time.Microsecond),
		constObj("Nanosecond", time.Nanosecond),
		constObj("Minute", time.Minute),
		constObj("Hour", time.Hour),
		constObj("Saturday", time.Saturday),
		constObj("Sunday", time.Sunday),
		constObj("Monday", time.Monday),
	}
}

// --- regexp ---

func regexpObjects() []*importer.Object {
	// 注册 Regexp 类型（供 var r *regexp.Regexp 声明解析），
	// Compile/MustCompile 返回 *Regexp，其方法通过反射自动可用
	return []*importer.Object{
		importer.CreateType("Regexp", reflect.TypeOf((*regexp.Regexp)(nil)).Elem(), ""),
		funcObj("MatchString", regexp.MatchString),
		funcObj("QuoteMeta", regexp.QuoteMeta),
		funcObj("Compile", regexp.Compile),
		funcObj("MustCompile", regexp.MustCompile),
	}
}

// --- encoding/json ---

func jsonObjects() []*importer.Object {
	return []*importer.Object{
		funcObj("Marshal", json.Marshal),
		funcObj("Unmarshal", json.Unmarshal),
		funcObj("Valid", json.Valid),
	}
}

// --- encoding/base64 ---

func base64Objects() []*importer.Object {
	return []*importer.Object{
		varObj("StdEncoding", base64.StdEncoding, reflect.TypeOf(base64.StdEncoding)),
		varObj("URLEncoding", base64.URLEncoding, reflect.TypeOf(base64.URLEncoding)),
		varObj("RawStdEncoding", base64.RawStdEncoding, reflect.TypeOf(base64.RawStdEncoding)),
		varObj("RawURLEncoding", base64.RawURLEncoding, reflect.TypeOf(base64.RawURLEncoding)),
	}
}

// --- errors ---

func errorsObjects() []*importer.Object {
	return []*importer.Object{
		funcObj("New", errors.New),
		funcObj("Is", errors.Is),
		funcObj("As", errors.As),
		funcObj("Unwrap", errors.Unwrap),
	}
}

// --- strconv ---

func strconvObjects() []*importer.Object {
	return []*importer.Object{
		funcObj("Itoa", strconv.Itoa),
		funcObj("Atoi", strconv.Atoi),
		funcObj("ParseInt", strconv.ParseInt),
		funcObj("ParseFloat", strconv.ParseFloat),
		funcObj("ParseBool", strconv.ParseBool),
		funcObj("FormatInt", strconv.FormatInt),
		funcObj("FormatFloat", strconv.FormatFloat),
	}
}

// --- bytes ---

func bytesPlaceholder() []*importer.Object {
	return []*importer.Object{
		funcObj("NewReader", bytes.NewReader),
		funcObj("Compare", bytes.Compare),
		funcObj("Equal", bytes.Equal),
		funcObj("Contains", bytes.Contains),
		funcObj("Join", bytes.Join),
	}
}

// --- github.com/json-iterator/go ---

func jsoniterObjects() []*importer.Object {
	return []*importer.Object{
		funcObj("Get", jsoniter.Get),
		funcObj("Marshal", jsoniter.Marshal),
		funcObj("Unmarshal", jsoniter.Unmarshal),
		importer.CreateType("API", reflect.TypeOf((*jsoniter.API)(nil)).Elem(), ""),
	}
}

// --- net/http ---

func httpObjects() []*importer.Object {
	return []*importer.Object{
		funcObj("Get", http.Get),
		funcObj("Post", http.Post),
		funcObj("PostForm", http.PostForm),
		funcObj("Head", http.Head),
		funcObj("NewRequest", http.NewRequest),
		constObj("MethodGet", http.MethodGet),
		constObj("MethodPost", http.MethodPost),
		constObj("MethodPut", http.MethodPut),
		constObj("MethodDelete", http.MethodDelete),
		varObj("DefaultClient", http.DefaultClient, reflect.TypeOf(http.DefaultClient)),
	}
}
