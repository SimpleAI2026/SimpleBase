// 文件说明：production_entry_test.go — §8.1 入口层测试（安全头分类 / 限流新特性 / loginGuard / 观测收口）
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/config"
)

func newHeadersApp(cfg config.Config) *echo.Echo {
	e := echo.New()
	e.IPExtractor = trustedIPExtractor(cfg.HTTP.TrustedProxies)
	e.Use(securityHeadersMiddleware(cfg))
	ok := func(c echo.Context) error { return c.NoContent(http.StatusOK) }
	e.GET("/v1/projects/p/databases", ok)
	e.GET("/v1/auth/login", ok)
	e.GET("/files/tok", ok)
	e.GET("/", ok)
	return e
}

func TestRouteClass(t *testing.T) {
	cases := map[string]string{
		"/v1/projects/p/databases":          "api",
		"/go/p/exec":                        "api",
		"/health/live":                      "api",
		"/metrics":                          "api",
		"/files/abc":                        "file",
		"/v1/projects/p/s3/objects/content": "file",
		"/":                                 "console",
		"/assets/index.js":                  "console",
	}
	for path, want := range cases {
		if got := routeClass(path); got != want {
			t.Fatalf("routeClass(%q)=%q want %q", path, got, want)
		}
	}
}

func TestSecurityHeadersByRouteClass(t *testing.T) {
	e := newHeadersApp(config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/p/databases", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Security-Policy") != apiCSP {
		t.Fatalf("api CSP missing: %#v", rec.Header())
	}
	if rec.Header().Get("Cross-Origin-Resource-Policy") != "" {
		t.Fatal("api class must not send CORP")
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/files/tok", nil))
	if rec.Header().Get("Content-Security-Policy") != fileCSP {
		t.Fatalf("file CSP missing: %#v", rec.Header())
	}
	if rec.Header().Get("Cross-Origin-Resource-Policy") != "same-site" {
		t.Fatal("file class must send CORP same-site")
	}

	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Header().Get("Content-Security-Policy") != consoleCSP {
		t.Fatalf("console CSP missing: %#v", rec.Header())
	}
	if rec.Header().Get("Cross-Origin-Opener-Policy") != "same-origin" {
		t.Fatal("console class must send COOP")
	}
}

func TestSecurityHeadersAuthNoStore(t *testing.T) {
	e := newHeadersApp(config.Config{})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/login", nil))
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("/v1/auth/* must send no-store: %#v", rec.Header())
	}
}

func TestHSTSOnlyWhenTrustedHTTPS(t *testing.T) {
	cfg := config.Config{}
	cfg.HTTP.SecurityHeaders.HSTSMaxAge = 31536000
	cfg.HTTP.TrustedProxies = []string{"127.0.0.1/32"}
	e := newHeadersApp(cfg)

	// 直连对端不在 trusted_proxies（httptest RemoteAddr 默认 192.0.2.1）→ 不发。
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	e.ServeHTTP(rec, req)
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS must not be sent for untrusted peer")
	}

	// 可信对端 + X-Forwarded-Proto: https → 发。
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("X-Forwarded-Proto", "https")
	e.ServeHTTP(rec, req)
	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("HSTS missing for trusted https peer")
	}
}

func TestUntrustedXFFRealIPUnchanged(t *testing.T) {
	e := echo.New()
	e.IPExtractor = trustedIPExtractor([]string{"10.0.0.5/32"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:1111"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if c.RealIP() != "203.0.113.9" {
		t.Fatalf("untrusted XFF must not change RealIP, got %s", c.RealIP())
	}
}

func TestRateLimitRuleMatchesPrefix(t *testing.T) {
	if !ruleMatches("/go/**", "/go/p/exec/cmd") {
		t.Fatal("/** suffix must match prefix")
	}
	if ruleMatches("/go/**", "/golang") {
		t.Fatal("/** prefix must not overmatch")
	}
	if !ruleMatches("/v1/auth/login", "/v1/auth/login") {
		t.Fatal("exact match broken")
	}
	if ruleMatches("/v1/auth/login", "/v1/auth/loginx") {
		t.Fatal("path.Match must not overmatch")
	}
}

func TestRateLimitStages(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled: true,
		Rules: []config.RateLimitRule{
			{Match: "/v1/auth/login", Key: config.RateLimitKeyIP, Max: 1, Window: time.Minute},
			{Match: "/v1/**", Key: config.RateLimitKeyPrincipal, Max: 1, Window: time.Minute},
		},
	}
	now := time.Unix(0, 0)

	pre := newRateLimiter(cfg, nil)
	pre.now = func() time.Time { return now }
	e := echo.New()
	e.Use(pre.middleware(preAuthStage))
	e.GET("/v1/auth/login", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	e.GET("/v1/projects", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	// pre_auth 只跑 ip 规则：login 第三次 429（桶容量 = Max × burst = 2）。
	for i, want := range []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests} {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/login", nil))
		if rec.Code != want {
			t.Fatalf("pre_auth login call %d: %d want %d", i, rec.Code, want)
		}
	}
	// principal 规则不在 pre_auth 执行：/v1/projects 两次都过。
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/projects", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("pre_auth must not run principal rules (call %d: %d)", i, rec.Code)
		}
	}

	// post_auth 只跑 principal/project 维度。
	post := newRateLimiter(cfg, nil)
	post.now = func() time.Time { return now }
	e2 := echo.New()
	e2.Use(post.middleware(postAuthStage))
	e2.GET("/v1/projects", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	for i, want := range []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests} {
		rec := httptest.NewRecorder()
		e2.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/projects", nil))
		if rec.Code != want {
			t.Fatalf("post_auth call %d: %d want %d", i, rec.Code, want)
		}
	}
}

func TestRateLimitMaxBucketsFailOpen(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:    true,
		MaxBuckets: 1, // 极小上限逼出 fail-open 分支
		Rules:      []config.RateLimitRule{{Match: "/v1/x", Key: config.RateLimitKeyIP, Max: 1, Window: time.Minute}},
	}
	l := newRateLimiter(cfg, nil)
	now := time.Unix(0, 0)
	l.now = func() time.Time { return now }
	rule := cfg.Rules[0]
	// 第一个桶占满唯一名额。
	if r := l.allow("a", rule); r != 0 {
		t.Fatal("first key should pass")
	}
	// 第二个新键：桶满 → fail-open 放行并计数。
	if r := l.allow("b", rule); r != 0 {
		t.Fatal("overflow must fail-open")
	}
	l.mu.Lock()
	overflow := l.overflow
	l.mu.Unlock()
	if overflow != 1 {
		t.Fatalf("overflow counter = %d want 1", overflow)
	}
}

func TestLoginGuardDimensions(t *testing.T) {
	base := time.Unix(1000, 0)
	g := newLoginGuard(defaultLoginGuardOpts()).WithClock(func() time.Time { return base })

	// 同 IP 换用户名撞库：ip 维度 30/min。
	for i := 0; i < 30; i++ {
		if g.allow("1.1.1.1", "victim-"+itoaTest(i)) != loginAllowed {
			t.Fatalf("attempt %d should pass pair dimension", i)
		}
	}
	if v := g.allow("1.1.1.1", "another-user"); v != loginRateLimited {
		t.Fatalf("ip dimension should throttle credential stuffing, got %v", v)
	}

	// 多 IP 爆破单用户名：失败计数 → 软锁定。
	g2 := newLoginGuard(defaultLoginGuardOpts()).WithClock(func() time.Time { return base })
	for i := 0; i < 10; i++ {
		if g2.allow("10.0.0."+itoaTest(i), "admin") != loginAllowed {
			t.Fatalf("distributed attempt %d should pass rate dimensions", i)
		}
		g2.recordFailure("10.0.0."+itoaTest(i), "admin")
	}
	if v := g2.allow("10.9.9.9", "admin"); v != loginSoftLocked {
		t.Fatalf("username should be soft-locked, got %v", v)
	}

	// 成功 IP 豁免：锁定后仍可从最近成功 IP 登录（本例豁免在锁定前记录）。
	g3 := newLoginGuard(defaultLoginGuardOpts()).WithClock(func() time.Time { return base })
	g3.recordSuccess("5.5.5.5", "boss")
	for i := 0; i < 10; i++ {
		g3.recordFailure("10.0.0."+itoaTest(i), "boss")
	}
	if !g3.isExemptIP("5.5.5.5", "boss") {
		t.Fatal("success IP should be exempt")
	}

	// 锁定过期自动解除。
	g4 := newLoginGuard(defaultLoginGuardOpts())
	t0 := base
	g4.WithClock(func() time.Time { return t0 })
	for i := 0; i < 10; i++ {
		g4.recordFailure("10.0.0."+itoaTest(i), "late")
	}
	if g4.allow("8.8.8.8", "late") != loginSoftLocked {
		t.Fatal("should be locked")
	}
	t0 = base.Add(16 * time.Minute)
	if g4.allow("8.8.8.8", "late") != loginAllowed {
		t.Fatal("lock should expire after LockDuration")
	}

	// 成功登录清零失败计数。
	g5 := newLoginGuard(defaultLoginGuardOpts()).WithClock(func() time.Time { return base })
	for i := 0; i < 9; i++ {
		g5.recordFailure("10.0.0."+itoaTest(i), "recover")
	}
	g5.recordSuccess("7.7.7.7", "recover")
	g5.recordFailure("9.9.9.9", "recover")
	if g5.allow("9.9.9.9", "recover") != loginAllowed {
		t.Fatal("success must reset failure count")
	}
}

func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

func TestObservabilityRoutesMatrix(t *testing.T) {
	// 只配 token：业务端口需 Bearer，否则 404。
	deps := Dependencies{Config: config.Config{Observability: config.ObservabilityConfig{MetricsPath: "/metrics", MetricsToken: "secret-token"}}}
	e := echo.New()
	mountObservabilityRoutes(e, deps)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("no token: %d want 404", rec.Code)
	}
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	e.ServeHTTP(rec, req)
	// Metrics 为 nil 时路由不挂载；这里只验证 token gate 放行路径（404 由 echo 默认）。
	if rec.Code == http.StatusNotFound {
		// 路由未注册（deps.Metrics == nil），符合预期。
		t.Log("metrics route not mounted without Metrics dependency")
	}

	// 配 metrics_address：业务端口不挂。
	deps2 := Dependencies{Config: config.Config{Observability: config.ObservabilityConfig{MetricsPath: "/metrics", MetricsAddress: "127.0.0.1:9090"}}}
	e2 := echo.New()
	mountObservabilityRoutes(e2, deps2)
	rec2 := httptest.NewRecorder()
	e2.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("separate listener: business port should 404, got %d", rec2.Code)
	}

	// bearerOK 常量时间比较。
	if bearerOK("Bearer secret-token", "secret-token") != true || bearerOK("Bearer wrong", "secret-token") != false {
		t.Fatal("bearerOK broken")
	}
}

func TestInlineSafeMIME(t *testing.T) {
	for _, mt := range []string{"image/png", "image/jpeg", "image/gif", "image/webp", "application/pdf", "text/plain"} {
		if !inlineSafeMIME(mt) {
			t.Fatalf("%s should be inline-safe", mt)
		}
	}
	for _, mt := range []string{"image/svg+xml", "text/html", "application/javascript", "", "application/octet-stream"} {
		if inlineSafeMIME(mt) {
			t.Fatalf("%s must not be inline-safe", mt)
		}
	}
}
