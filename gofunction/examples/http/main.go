// http 在宿主侧用 httptest 起本地服务，脚本通过已注册的 net/http.Get 访问。
// 不访问公网。仓库测试 TestHttpRequest/TestHttpsRequest 在 testing.Short() 下会跳过外网用例。
package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/linkxzhou/SimpleBase/gofunction"
	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

func main() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ping" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("pong"))
	}))
	defer srv.Close()

	src := `package main

import (
	"io"
	"net/http"
)

func fetch(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		return err.Error()
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err.Error()
	}
	return string(b)
}
`
	result, err := gofunction.Run("http-seq", src, "fetch", srv.URL+"/ping")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("script http.Get => %q\n", result)
}
