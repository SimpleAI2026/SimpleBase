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
