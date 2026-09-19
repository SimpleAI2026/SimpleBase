// Example: echo + gofunction 云函数 HTTP 服务性能示例
//
// 本示例演示 SimpleBase 的典型生产形态：echo 提供 HTTP 接入层，
// 云函数脚本（运行在 gofunction 解释器中）承载业务逻辑，
// 并与「同等逻辑的原生 Go 实现」做性能对照。
//
// 三组有性能代表性的负载：
//
//  1. Compute  递归 fib(22)      —— 纯解释执行开销（CPU 密集）
//  2. JSON     订单记录编解码     —— 结构体/切片/map/字符串（真实业务形态）
//  3. Text     日志行解析聚合     —— 字符串切分/拼接/map 聚合（IO 服务常见）
//
// 启动：
//
//	go run ./examples/gofunction
//
// 基准（服务起来后另开终端）：
//
//	go test -bench=. -benchmem ./examples/gofunction      # 进程内直接调用对照
//	hey -z 10s -c 8 http://127.0.0.1:8091/bench/compute   # HTTP 层压测（未安装 hey 可用 ab）
//
// 三条路径对照：
//
//	/native/*   原生 Go 编写的同逻辑 handler（性能上限基准）
//	/bench/*    脚本函数经 RunJSON 执行（生产调用路径）
//	/inproc/*   脚本函数进程内直调（剥离 HTTP 开销，测解释器净耗时）
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gofunction"
	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

// scriptSource 云函数脚本：三个导出函数遵循 HTTP 约定（1 入参 1 返回值）。
// 与 native handler 逻辑严格一致，保证对照公平。
const scriptSource = `package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Compute 递归 fib
func Compute(n int) int {
	return fib(n)
}
func fib(n int) int {
	if n < 2 {
		return n
	}
	return fib(n-1) + fib(n-2)
}

// Order 输入结构
type Order struct {
	ID     string  ` + "`json:\"id\"`" + `
	UserID string  ` + "`json:\"user_id\"`" + `
	Amount float64 ` + "`json:\"amount\"`" + `
	Items  []struct {
		SKU string  ` + "`json:\"sku\"`" + `
		Qty int     ` + "`json:\"qty\"`" + `
		Sub float64 ` + "`json:\"sub\"`" + `
	} ` + "`json:\"items\"`" + `
}

// JSON 处理一笔订单：重算小计/校验/格式化摘要。
// 输入为订单 JSON 文本（云函数典型的 string 入参形态），
// 逻辑与 native/nativeJSON 严格一致。
func JSON(order string) string {
	var o Order
	json.Unmarshal([]byte(order), &o)
	total := 0.0
	lines := 0
	for _, it := range o.Items {
		if it.Sub != float64(it.Qty)*unitPrice(it.SKU) {
			continue // 小计不符的行丢弃
		}
		total += it.Sub
		lines++
	}
	if total != o.Amount {
		return fmt.Sprintf("ERR %s total=%.2f want=%.2f", o.ID, total, o.Amount)
	}
	return fmt.Sprintf("OK %s lines=%d total=%.2f", o.ID, lines, total)
}

func unitPrice(sku string) float64 {
	h := 0
	for _, c := range sku {
		h = h*31 + int(c)
	}
	return float64(h%1000) / 10.0
}

// Text 解析一行访问日志并聚合：格式 "GET /path 200 1.5ms"
func Text(logs string) string {
	counts := make(map[string]int)
	totalLat := 0.0
	n := 0
	for _, line := range strings.Split(logs, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 {
			continue
		}
		lat := 0.0
		fmt.Sscanf(fields[3], "%fms", &lat)
		counts[fields[1]]++
		totalLat += lat
		n++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	avg := 0.0
	if n > 0 {
		avg = totalLat / float64(n)
	}
	return fmt.Sprintf("paths=%d reqs=%d avg=%.3fms", len(keys), n, avg)
}
`

// ---------- 原生 Go 对照实现（与脚本逻辑严格一致） ----------

func nativeFib(n int) int {
	if n < 2 {
		return n
	}
	return nativeFib(n-1) + nativeFib(n-2)
}

type nativeOrder struct {
	ID     string  `json:"id"`
	UserID string  `json:"user_id"`
	Amount float64 `json:"amount"`
	Items  []struct {
		SKU string  `json:"sku"`
		Qty int     `json:"qty"`
		Sub float64 `json:"sub"`
	} `json:"items"`
}

func nativeUnitPrice(sku string) float64 {
	h := 0
	for _, c := range sku {
		h = h*31 + int(c)
	}
	return float64(h%1000) / 10.0
}

func nativeJSON(order nativeOrder) string {
	total := 0.0
	lines := 0
	for _, it := range order.Items {
		if it.Sub != float64(it.Qty)*nativeUnitPrice(it.SKU) {
			continue
		}
		total += it.Sub
		lines++
	}
	if total != order.Amount {
		return fmt.Sprintf("ERR %s total=%.2f want=%.2f", order.ID, total, order.Amount)
	}
	return fmt.Sprintf("OK %s lines=%d total=%.2f", order.ID, lines, total)
}

func nativeText(logs string) string {
	counts := make(map[string]int)
	totalLat := 0.0
	n := 0
	for _, line := range strings.Split(logs, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 {
			continue
		}
		lat := 0.0
		fmt.Sscanf(fields[3], "%fms", &lat)
		counts[fields[1]]++
		totalLat += lat
		n++
	}
	avg := 0.0
	if n > 0 {
		avg = totalLat / float64(n)
	}
	return fmt.Sprintf("paths=%d reqs=%d avg=%.3fms", len(counts), n, avg)
}

// ---------- 基准数据 ----------

// buildOrderJSON 由 Go 侧生成订单 payload，
// 小计严格按脚本 unitPrice 同款算法生成，保证校验路径可命中。
func buildOrderJSON() string {
	type item struct {
		SKU string  `json:"sku"`
		Qty int     `json:"qty"`
		Sub float64 `json:"sub"`
	}
	items := make([]item, 0, 8)
	total := 0.0
	for i := 0; i < 8; i++ {
		sku := fmt.Sprintf("SKU-%04d", i*7)
		qty := 1 + i%3
		sub := float64(qty) * nativeUnitPrice(sku)
		items = append(items, item{SKU: sku, Qty: qty, Sub: sub})
		total += sub
	}
	type orderT struct {
		ID     string  `json:"id"`
		UserID string  `json:"user_id"`
		Amount float64 `json:"amount"`
		Items  []item  `json:"items"`
	}
	b, _ := json.Marshal(orderT{ID: "o-10086", UserID: "u-42", Amount: total, Items: items})
	return string(b)
}

func buildLogText() string {
	// 50 行访问日志
	var b strings.Builder
	for i := 0; i < 50; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "GET /api/v1/items/%d 200 %.2fms", i%20, 0.5+float64(i%13)*0.3)
	}
	return b.String()
}

var (
	orderJSON = buildOrderJSON()
	logText   = buildLogText()
)

// bindJSON 显式按 JSON 解析请求体（echo Bind 会按 Content-Type 猜测，
// curl/压测工具默认 octet-stream 时走 form 解析导致字段全零）
func bindJSON(c echo.Context, v interface{}) error {
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

// ---------- 服务 ----------

func main() {
	if _, err := gofunction.ValidateHTTPFuncs(scriptSource); err != nil {
		fmt.Fprintln(os.Stderr, "script invalid:", err)
		os.Exit(1)
	}

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	var nativeReqs, scriptReqs, inprocReqs int64

	// 原生对照
	e.GET("/native/compute", func(c echo.Context) error {
		atomic.AddInt64(&nativeReqs, 1)
		return c.String(http.StatusOK, strconv.Itoa(nativeFib(22)))
	})
	e.POST("/native/json", func(c echo.Context) error {
		atomic.AddInt64(&nativeReqs, 1)
		var o nativeOrder
		if err := bindJSON(c, &o); err != nil {
			return err
		}
		return c.String(http.StatusOK, nativeJSON(o))
	})
	e.POST("/native/text", func(c echo.Context) error {
		atomic.AddInt64(&nativeReqs, 1)
		body, _ := io.ReadAll(c.Request().Body)
		return c.String(http.StatusOK, nativeText(string(body)))
	})
	// 脚本经 RunJSON（生产调用路径：每请求一次 JSON 绑定 + 解释执行）
	runJSON := func(c echo.Context, funcName string, body []byte) ([]byte, error) {
		atomic.AddInt64(&scriptReqs, 1)
		return gofunction.RunJSON(c.Request().Context(), "bench", "main", scriptSource, funcName, body)
	}
	e.GET("/bench/compute", func(c echo.Context) error {
		res, err := runJSON(c, "Compute", []byte(strconv.Itoa(22)))
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.Blob(http.StatusOK, echo.MIMEApplicationJSON, res)
	})
	e.POST("/bench/json", func(c echo.Context) error {
		body, _ := io.ReadAll(c.Request().Body)
		// RunJSON 的 body 会做 json.Unmarshal 到 string 形参，
		// 因此这里传 JSON 编码的字符串（与生产调用约定一致）
		enc, _ := json.Marshal(string(body))
		res, err := runJSON(c, "JSON", enc)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.Blob(http.StatusOK, echo.MIMEApplicationJSON, res)
	})
	e.POST("/bench/text", func(c echo.Context) error {
		body, _ := io.ReadAll(c.Request().Body)
		// RunJSON body 走 json.Unmarshal 到 string 形参，需 JSON 编码
		enc, _ := json.Marshal(string(body))
		res, err := runJSON(c, "Text", enc)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.Blob(http.StatusOK, echo.MIMEApplicationJSON, res)
	})

	// 进程内直调（剥离 HTTP 与 JSON 绑定，测解释器净耗时）。
	// JSON/Text 负载在此路径按原始类型直传（不走 RunJSON 的 JSON 编码），
	// 与 native 路径构成「同输入同输出」的公平对照。
	program, err := gofunction.BuildProgram("bench", "main", scriptSource)
	if err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.Exit(1)
	}
	runInproc := func(c echo.Context, funcName string, arg interface{}) error {
		atomic.AddInt64(&inprocReqs, 1)
		res, err := program.Run("", funcName, arg)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.String(http.StatusOK, fmt.Sprint(res))
	}
	e.GET("/inproc/compute", func(c echo.Context) error {
		return runInproc(c, "Compute", 22)
	})
	e.POST("/inproc/json", func(c echo.Context) error {
		body, _ := io.ReadAll(c.Request().Body)
		// 脚本 JSON 函数入参为 string（订单 JSON 文本）
		return runInproc(c, "JSON", string(body))
	})
	e.POST("/inproc/text", func(c echo.Context) error {
		body, _ := io.ReadAll(c.Request().Body)
		return runInproc(c, "Text", string(body))
	})

	// 压测负载输出：保证 payload 与脚本校验逻辑一致（供 wrk/hey 引用）
	e.GET("/payload/json", func(c echo.Context) error {
		return c.String(http.StatusOK, orderJSON)
	})
	e.GET("/payload/text", func(c echo.Context) error {
		return c.String(http.StatusOK, logText)
	})

	// 统计输出
	go func() {
		for {
			time.Sleep(10 * time.Second)
			fmt.Printf("[stats] native=%d bench=%d inproc=%d\n",
				atomic.LoadInt64(&nativeReqs),
				atomic.LoadInt64(&scriptReqs),
				atomic.LoadInt64(&inprocReqs))
		}
	}()

	addr := "127.0.0.1:8091"
	fmt.Println("echo + gofunction bench server on http://" + addr)
	fmt.Println("  native:  /native/compute /native/json /native/text   (原生 Go 上限)")
	fmt.Println("  bench:   /bench/compute  /bench/json  /bench/text    (RunJSON 生产路径)")
	fmt.Println("  inproc:  /inproc/compute                            (进程内直调)")
	if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	}
}
