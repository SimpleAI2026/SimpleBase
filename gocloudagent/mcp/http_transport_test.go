// 文件说明：http_transport_test.go — gocloudagent/mcp 的单元测试（TestHeaderTransportClonesRequestAndOverrides、TestHeaderTransportNoHeadersPassesRequestThrough 等）
//
// 用法
//
//	go test ./gocloudagent/mcp -run 'TestHeaderTransportClonesRequestAndOverrides|TestHeaderTransportNoHeadersPassesRequestThrough|TestHeaderTransportPropagatesBaseError|TestHeaderTransportResponseLimitBoundary'
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/cloudagent, gocloudagent/tools, internal/agentbridge 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/mcp/http_transport_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] adapter.go, adapter_test.go, client.go, http_transport.go, limits.go 等 10 个
//
// 主要符号
//
//	type roundTripFunc func(*http.Request) (*http.Response, error)
//	(f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error)
//	type countingBody struct{…}
//	(b *countingBody) Close() error
//	respWith(body io.ReadCloser) *http.Response
//	TestHeaderTransportClonesRequestAndOverrides(t *testing.T)
//	TestHeaderTransportNoHeadersPassesRequestThrough(t *testing.T)
//	TestHeaderTransportPropagatesBaseError(t *testing.T)
//	... 另有 3 个顶层符号

package mcp

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type countingBody struct {
	io.Reader
	closed int
}

func (b *countingBody) Close() error { b.closed++; return nil }

func respWith(body io.ReadCloser) *http.Response {
	return &http.Response{StatusCode: 200, Body: body, Header: http.Header{}}
}

// 注入头时克隆请求：原请求不被修改，同名头以后值覆盖。
func TestHeaderTransportClonesRequestAndOverrides(t *testing.T) {
	var seen *http.Request
	tr := &HeaderTransport{
		Base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			seen = r
			return respWith(io.NopCloser(strings.NewReader(""))), nil
		}),
		Headers: http.Header{"Authorization": {"first", "second"}, "X-Extra": {"1"}},
	}
	req, _ := http.NewRequest("GET", "http://example.test", nil)
	req.Header.Set("Authorization", "orig")
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if seen == req {
		t.Fatal("must clone the request when injecting headers")
	}
	if got := seen.Header.Get("Authorization"); got != "second" || seen.Header.Get("X-Extra") != "1" {
		t.Fatalf("headers=%v", seen.Header)
	}
	if req.Header.Get("Authorization") != "orig" || req.Header.Get("X-Extra") != "" {
		t.Fatalf("original request modified: %v", req.Header)
	}
}

// 没有要注入的头时直接透传同一个请求。
func TestHeaderTransportNoHeadersPassesRequestThrough(t *testing.T) {
	var seen *http.Request
	tr := &HeaderTransport{Base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		seen = r
		return respWith(io.NopCloser(strings.NewReader(""))), nil
	})}
	req, _ := http.NewRequest("GET", "http://example.test", nil)
	if _, err := tr.RoundTrip(req); err != nil || seen != req {
		t.Fatalf("err=%v same=%v", err, seen == req)
	}
}

func TestHeaderTransportPropagatesBaseError(t *testing.T) {
	boom := errors.New("boom")
	tr := &HeaderTransport{Base: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, boom })}
	req, _ := http.NewRequest("GET", "http://example.test", nil)
	if _, err := tr.RoundTrip(req); !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
}

// 响应体上限：恰好等于上限可读完；再读一次得到“超限”错误而不是 EOF；Close 透传。
func TestHeaderTransportResponseLimitBoundary(t *testing.T) {
	body := &countingBody{Reader: strings.NewReader("abcdef")}
	tr := &HeaderTransport{
		Base:             roundTripFunc(func(*http.Request) (*http.Response, error) { return respWith(body), nil }),
		MaxResponseBytes: 6,
	}
	req, _ := http.NewRequest("GET", "http://example.test", nil)
	res, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	n, err := res.Body.Read(buf)
	if n != 6 || err != nil {
		t.Fatalf("first read n=%d err=%v", n, err)
	}
	if _, err := res.Body.Read(buf); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("second read err=%v, want ErrResponseTooLarge", err)
	}
	if err := res.Body.Close(); err != nil || body.closed != 1 {
		t.Fatalf("close err=%v closed=%d", err, body.closed)
	}
}

// 上限小于响应：只读到上限，然后报错（不静默截断成功）。
func TestHeaderTransportResponseOverLimit(t *testing.T) {
	tr := &HeaderTransport{
		Base: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return respWith(io.NopCloser(strings.NewReader("0123456789"))), nil
		}),
		MaxResponseBytes: 4,
	}
	req, _ := http.NewRequest("GET", "http://example.test", nil)
	res, _ := tr.RoundTrip(req)
	got, err := io.ReadAll(res.Body)
	if string(got) != "0123" || !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

// MaxResponseBytes<=0 不限制，也不包装 Body；nil Body 不 panic。
func TestHeaderTransportUnlimitedAndNilBody(t *testing.T) {
	orig := io.NopCloser(strings.NewReader("hello"))
	tr := &HeaderTransport{Base: roundTripFunc(func(*http.Request) (*http.Response, error) { return respWith(orig), nil })}
	req, _ := http.NewRequest("GET", "http://example.test", nil)
	res, _ := tr.RoundTrip(req)
	if res.Body != orig {
		t.Fatal("unlimited transport must not wrap the body")
	}
	tr2 := &HeaderTransport{
		Base:             roundTripFunc(func(*http.Request) (*http.Response, error) { return &http.Response{StatusCode: 204}, nil }),
		MaxResponseBytes: 10,
	}
	if res, err := tr2.RoundTrip(req); err != nil || res.Body != nil {
		t.Fatalf("nil body: res=%+v err=%v", res, err)
	}
}
