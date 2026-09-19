package packages

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"sync/atomic"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
)

// stdHTTP net/http 包注册清单
func stdHTTP() []*importer.Object {
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
		varObj("DefaultClient", &http.DefaultClient, reflect.TypeOf(http.DefaultClient)),
		typeObj("Client", reflect.TypeOf((*http.Client)(nil)).Elem()),
		typeObj("Request", reflect.TypeOf((*http.Request)(nil)).Elem()),
		typeObj("Response", reflect.TypeOf((*http.Response)(nil)).Elem()),
		typeObj("Header", reflect.TypeOf(http.Header{})),
	}
}

// stdBytes bytes 包注册清单
func stdBytes() []*importer.Object {
	return []*importer.Object{
		funcObj("NewReader", bytes.NewReader),
		funcObj("Compare", bytes.Compare),
		funcObj("Equal", bytes.Equal),
		funcObj("Contains", bytes.Contains),
		funcObj("Join", bytes.Join),
	}
}

// stdAtomic sync/atomic 包注册清单
func stdAtomic() []*importer.Object {
	return []*importer.Object{
		funcObj("AddInt32", atomic.AddInt32),
		funcObj("AddInt64", atomic.AddInt64),
		funcObj("AddUint32", atomic.AddUint32),
		funcObj("AddUint64", atomic.AddUint64),
		funcObj("LoadInt32", atomic.LoadInt32),
		funcObj("LoadInt64", atomic.LoadInt64),
		funcObj("LoadUint32", atomic.LoadUint32),
		funcObj("LoadUint64", atomic.LoadUint64),
		funcObj("StoreInt32", atomic.StoreInt32),
		funcObj("StoreInt64", atomic.StoreInt64),
		funcObj("StoreUint32", atomic.StoreUint32),
		funcObj("StoreUint64", atomic.StoreUint64),
		funcObj("CompareAndSwapInt32", atomic.CompareAndSwapInt32),
		funcObj("CompareAndSwapInt64", atomic.CompareAndSwapInt64),
	}
}

// stdIO io 包注册清单
func stdIO() []*importer.Object {
	return []*importer.Object{
		funcObj("ReadAll", io.ReadAll),
		funcObj("Copy", io.Copy),
		funcObj("NopCloser", io.NopCloser),
		varObj("EOF", &io.EOF, reflect.TypeOf((*error)(nil)).Elem()),
		typeObj("Reader", reflect.TypeOf((*io.Reader)(nil)).Elem()),
		typeObj("Writer", reflect.TypeOf((*io.Writer)(nil)).Elem()),
		typeObj("Closer", reflect.TypeOf((*io.Closer)(nil)).Elem()),
		typeObj("ReadCloser", reflect.TypeOf((*io.ReadCloser)(nil)).Elem()),
	}
}

// stdIoutil io/ioutil 包注册清单（已弃用，仅为兼容旧脚本保留）
func stdIoutil() []*importer.Object {
	return []*importer.Object{
		funcObj("ReadAll", ioutil.ReadAll),
		funcObj("ReadFile", ioutil.ReadFile),
		funcObj("NopCloser", ioutil.NopCloser),
		funcObj("WriteFile", ioutil.WriteFile),
	}
}
