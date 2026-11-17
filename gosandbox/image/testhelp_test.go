package image

// testhelp_test.go：测试用的小工具。
//
// newTestClient 返回一个把 https 换成 http 的客户端：registry 客户端固定拼
// https://<host>/…，而假 registry 是明文 HTTP。生产代码不做这个替换——
// 真实 registry 必须走 TLS。readFileIn 读 lower 里的文件。

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestClient 造一个指向 srvURL 的客户端，把 https 请求改写成 http。
func newTestClient(srvURL string) *Client {
	c := NewClient()
	base, err := url.Parse(srvURL)
	if err != nil {
		return c
	}
	// 用一个自定义 RoundTripper 把 scheme 与 host 都改掉。
	c.http = &http.Client{Transport: &rewriteTransport{base: base}}
	return c
}

// rewriteTransport 把请求改写到测试服务器。
type rewriteTransport struct{ base *url.URL }

// RoundTrip 实现 http.RoundTripper。
func (rt *rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.URL.Scheme = rt.base.Scheme
	clone.URL.Host = rt.base.Host
	clone.Host = rt.base.Host
	return http.DefaultTransport.RoundTrip(clone)
}

// readFileIn 读 lower 里的一个文件。
func readFileIn(t *testing.T, root, name string) (string, error) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, strings.TrimPrefix(name, "/")))
	return string(b), err
}
