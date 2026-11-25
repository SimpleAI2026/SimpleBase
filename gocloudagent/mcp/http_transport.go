// 文件说明：http_transport.go — MCP HTTP 客户端的通用 RoundTripper 装饰
//
// 只提供与平台无关的两件事：注入静态 Header、限制响应体大小。网络出口策略
// （私网封锁、重定向、TLS、超时）、凭据来源、URL 校验仍由宿主构造 base 时负责。

package mcp

import (
	"errors"
	"io"
	"net/http"
)

// HeaderTransport 在每个请求上注入静态 Header，并可选地限制响应体大小。
type HeaderTransport struct {
	// Base 是实际发送请求的传输层；nil 用 http.DefaultTransport。
	Base http.RoundTripper
	// Headers 逐项以 Set 语义覆盖请求头（同名多值时后者生效）。
	Headers http.Header
	// MaxResponseBytes >0 时限制响应体读取量，超限后 Read 返回错误；<=0 不限制。
	MaxResponseBytes int
}

// ErrResponseTooLarge 表示响应体超过了 MaxResponseBytes。
var ErrResponseTooLarge = errors.New("mcp: response body exceeds size limit")

// RoundTrip 实现 http.RoundTripper。不修改传入的请求：需要注入时克隆一份。
func (t *HeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(t.Headers) > 0 {
		clone := req.Clone(req.Context())
		for k, vs := range t.Headers {
			for _, v := range vs {
				clone.Header.Set(k, v)
			}
		}
		req = clone
	}
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	res, err := base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	// SDK 会把整个结果解码进内存；上限只保护调用方，不会截断后假装成功。
	if t.MaxResponseBytes > 0 && res.Body != nil {
		res.Body = &limitedBody{r: res.Body, remain: t.MaxResponseBytes}
	}
	return res, nil
}

// limitedBody 在读取量用尽后返回 ErrResponseTooLarge（不探测 EOF），阻止无限响应体。
type limitedBody struct {
	r      io.ReadCloser
	remain int
}

func (l *limitedBody) Read(p []byte) (int, error) {
	if l.remain <= 0 {
		return 0, ErrResponseTooLarge
	}
	if len(p) > l.remain {
		p = p[:l.remain]
	}
	n, err := l.r.Read(p)
	l.remain -= n
	return n, err
}

func (l *limitedBody) Close() error { return l.r.Close() }
