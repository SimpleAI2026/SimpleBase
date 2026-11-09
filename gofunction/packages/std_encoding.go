// 文件说明：std_encoding.go — gofunction/packages 的内部实现（stdJSON、stdBase64 等）
//
// 用法
//
//	同包调用：stdJSON() → packages.go；stdBase64() → packages.go；stdBinary() → packages.go
//
// 调用架构
//
//	[上游] gofunction/bench, gofunction/cmd/gofunction, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/packages/std_encoding.go
//	         │ 依赖
//	         ▼
//	[下游] gofunction/importer
//	[同包] packages.go, std_fmt.go, std_http.go, std_math.go, std_misc.go 等 7 个
//	[关键引用] stdJSON() ← packages.go
//
// 主要符号
//
//	stdJSON() []*importer.Object
//	stdBase64() []*importer.Object
//	stdBinary() []*importer.Object
//	stdJSONIter() []*importer.Object

package packages

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"reflect"

	jsoniter "github.com/json-iterator/go"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
)

// stdJSON encoding/json 包注册清单
func stdJSON() []*importer.Object {
	return []*importer.Object{
		funcObj("Marshal", json.Marshal),
		funcObj("Unmarshal", json.Unmarshal),
		funcObj("Valid", json.Valid),
	}
}

// stdBase64 encoding/base64 包注册清单
func stdBase64() []*importer.Object {
	return []*importer.Object{
		typeObj("Encoding", reflect.TypeOf((*base64.Encoding)(nil)).Elem()),
		varObj("StdEncoding", &base64.StdEncoding, reflect.TypeOf(base64.StdEncoding)),
		varObj("URLEncoding", &base64.URLEncoding, reflect.TypeOf(base64.URLEncoding)),
		varObj("RawStdEncoding", &base64.RawStdEncoding, reflect.TypeOf(base64.RawStdEncoding)),
		varObj("RawURLEncoding", &base64.RawURLEncoding, reflect.TypeOf(base64.RawURLEncoding)),
	}
}

// stdBinary encoding/binary 包注册清单
func stdBinary() []*importer.Object {
	return []*importer.Object{
		funcObj("Read", binary.Read),
		funcObj("Write", binary.Write),
		funcObj("Size", binary.Size),
		varObj("BigEndian", &binary.BigEndian, reflect.TypeOf(binary.BigEndian)),
		varObj("LittleEndian", &binary.LittleEndian, reflect.TypeOf(binary.LittleEndian)),
	}
}

// stdJSONIter github.com/json-iterator/go 注册清单
func stdJSONIter() []*importer.Object {
	return []*importer.Object{
		funcObj("Get", jsoniter.Get),
		funcObj("Marshal", jsoniter.Marshal),
		funcObj("Unmarshal", jsoniter.Unmarshal),
		typeObj("API", reflect.TypeOf((*jsoniter.API)(nil)).Elem()),
		typeObj("Any", reflect.TypeOf((*jsoniter.Any)(nil)).Elem()),
	}
}
