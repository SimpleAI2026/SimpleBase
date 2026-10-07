package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/ducklake"
)

// —— fake KVService ——

// fakeKVService 实现项目级 KVService：按 projectID 返回固定的 catalog 行，
// lease 由测试注入（真实 DuckLake 或手工假实现）。
type fakeKVService struct {
	db      catalog.Database // GetKVDatabase 返回的行；ID 空 → NotFound
	lease   SQLLease
	created bool // CreateKVDatabase 被调用过
}

func (f *fakeKVService) GetKVDatabase(ctx context.Context, tenantID, projectID string) (catalog.Database, error) {
	if f.db.ID == "" {
		return catalog.Database{}, catalog.ErrNotFound
	}
	return f.db, nil
}

func (f *fakeKVService) CreateKVDatabase(ctx context.Context, tenantID, projectID string) (catalog.Database, error) {
	f.created = true
	return f.db, nil
}

func (f *fakeKVService) Acquire(ctx context.Context, db catalog.Database, mode database.AccessMode) (SQLLease, error) {
	if f.lease == nil {
		return nil, database.ErrWriterUnavailable
	}
	return f.lease, nil
}

// setupKVTestRouter 注册项目级 KV 单端点的测试 echo（不含 auth 中间件，
// 由 mux 注入 principal/project——与 data handler 测试同模式）。
func setupKVTestRouter(t *testing.T, svc KVService, writable bool) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HideBanner = true
	h := NewKVHandler(svc, writable)
	e.Use(kvTestPrincipalMiddleware)
	e.POST("/v1/projects/:projectID/kv", h.Execute)
	return e
}

func kvTestPrincipalMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		p := auth.Principal{
			APIKeyID:   "key-test",
			TenantID:   "tenant-1",
			ProjectIDs: map[string]struct{}{"proj-1": {}},
			Permissions: map[auth.Permission]struct{}{
				auth.DatabaseRead:  {},
				auth.DatabaseWrite: {},
			},
		}
		ctx := WithPrincipal(c.Request().Context(), p)
		ctx = WithProject(ctx, ProjectContext{ID: "proj-1", TenantID: "tenant-1"})
		c.SetRequest(c.Request().WithContext(ctx))
		return next(c)
	}
}

func kvReq(t *testing.T, e *echo.Echo, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func kvExec(t *testing.T, e *echo.Echo, argvs string) *httptest.ResponseRecorder {
	t.Helper()
	return kvReq(t, e, http.MethodPost, "/v1/projects/proj-1/kv",
		`{"type":"cmd","argvs":`+argvs+`}`)
}

func kvKVDB() catalog.Database {
	return catalog.Database{ID: "kv-1", Name: catalog.KVDatabaseName, ProjectID: "proj-1",
		Kind: catalog.DatabaseKindKV, Status: catalog.DatabaseReady}
}

// —— 请求形态校验（不触碰引擎）——

func TestKVRequestShape(t *testing.T) {
	e := setupKVTestRouter(t, &fakeKVService{db: kvKVDB()}, true)

	// 空 type
	rec := kvReq(t, e, http.MethodPost, "/v1/projects/proj-1/kv", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty type = %d, want 400; %s", rec.Code, rec.Body.String())
	}
	// 未知 type
	rec = kvReq(t, e, http.MethodPost, "/v1/projects/proj-1/kv", `{"type":"Bogus","args":{"key":"k"}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bogus type = %d, want 400", rec.Code)
	}
	// cmd 缺 argvs
	rec = kvReq(t, e, http.MethodPost, "/v1/projects/proj-1/kv", `{"type":"cmd"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cmd no argvs = %d, want 400", rec.Code)
	}
	// 数据类型缺 args
	rec = kvReq(t, e, http.MethodPost, "/v1/projects/proj-1/kv", `{"type":"String"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("String no args = %d, want 400", rec.Code)
	}
	// 数据类型缺 key
	rec = kvReq(t, e, http.MethodPost, "/v1/projects/proj-1/kv", `{"type":"String","args":{"value":"v"}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("String no key = %d, want 400", rec.Code)
	}
	// 未知命令
	rec = kvExec(t, e, `["BOGUSCMD","k"]`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bogus cmd = %d, want 400; %s", rec.Code, rec.Body.String())
	}
	var body APIErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "kv_unknown_command" {
		t.Errorf("code = %q, want kv_unknown_command", body.Error.Code)
	}
	// 参数个数错误
	rec = kvExec(t, e, `["SET","k"]`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("SET arity = %d, want 400", rec.Code)
	}
}

func TestKVReadOnlyPrincipal(t *testing.T) {
	_, svc := kvRealEnv(t)
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p := auth.Principal{TenantID: "tenant-1", ProjectIDs: map[string]struct{}{"proj-1": {}},
				Permissions: map[auth.Permission]struct{}{auth.DatabaseRead: {}}}
			ctx := WithPrincipal(c.Request().Context(), p)
			ctx = WithProject(ctx, ProjectContext{ID: "proj-1", TenantID: "tenant-1"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.POST("/v1/projects/:projectID/kv", NewKVHandler(svc, true).Execute,
		auth.Require(auth.DatabaseRead, PrincipalFromContext))
	cases := []struct {
		name, body string
		want       int
	}{
		{"GET", `{"type":"cmd","argvs":["GET","missing"]}`, 200},
		{"HGETALL", `{"type":"cmd","argvs":["HGETALL","missing"]}`, 200},
		{"SCAN", `{"type":"cmd","argvs":["SCAN","0"]}`, 200},
		{"ZRANGE", `{"type":"cmd","argvs":["ZRANGE","missing","0","-1"]}`, 200},
		{"SET", `{"type":"cmd","argvs":["SET","k","v"]}`, 403},
		{"HSET", `{"type":"cmd","argvs":["HSET","k","f","v"]}`, 403},
		{"DEL", `{"type":"cmd","argvs":["DEL","k"]}`, 403},
		{"typed String", `{"type":"String","args":{"key":"k","value":"v"}}`, 403},
		{"unknown", `{"type":"cmd","argvs":["BOGUS"]}`, 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := kvReq(t, e, http.MethodPost, "/v1/projects/proj-1/kv", tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestKVWritableFalseRejectsWrites(t *testing.T) {
	e := setupKVTestRouter(t, &fakeKVService{db: kvKVDB()}, false)
	rec := kvExec(t, e, `["SET","k","v"]`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readonly SET = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
	var body APIErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "writer_unavailable" {
		t.Errorf("code = %q, want writer_unavailable", body.Error.Code)
	}
}

func TestKVSystemProjectRejected(t *testing.T) {
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := WithPrincipal(c.Request().Context(), auth.Principal{TenantID: "tenant-1",
				Permissions: map[auth.Permission]struct{}{auth.DatabaseRead: {}, auth.DatabaseWrite: {}}})
			ctx = WithProject(ctx, ProjectContext{ID: catalog.ReservedSystemProjectID, TenantID: "tenant-1"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.POST("/v1/projects/:projectID/kv", NewKVHandler(&fakeKVService{db: kvKVDB()}, true).Execute)
	for _, body := range []string{`{"type":"cmd","argvs":["GET","k"]}`, `{"type":"cmd","argvs":["SET","k","v"]}`} {
		rec := kvReq(t, e, http.MethodPost, "/v1/projects/sb-admin/kv", body)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "kv_not_found") {
			t.Fatalf("system KV status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
}

func TestKVDatabaseNotFound(t *testing.T) {
	svc := &fakeKVService{db: catalog.Database{}}
	e := setupKVTestRouter(t, svc, false) // readonly：不补建 → 404
	rec := kvExec(t, e, `["GET","k"]`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("no kv row = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
	// writable：补建一次
	e2 := setupKVTestRouter(t, svc, true)
	rec = kvExec(t, e2, `["SET","k","v"]`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("recreate but no lease = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
	if !svc.created {
		t.Error("CreateKVDatabase not called on writable instance")
	}
}

// —— 真实引擎端到端用例（fake lease 的 Raw 指向真 DuckLake）——

func kvRealEnv(t *testing.T) (*echo.Echo, *fakeKVService) {
	t.Helper()
	f := &ducklake.Factory{
		CacheDir: t.TempDir(),
		Options:  ducklake.DefaultOptions(),
		Syncer:   ducklake.NewLocalSyncer(),
	}
	raw, err := f.Open(context.Background(), catalog.Database{
		ID: uuid.NewString(), Name: "kv-e2e", Status: catalog.DatabaseReady,
	}, database.ReadWrite)
	if err != nil {
		t.Fatalf("open ducklake: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	svc := &fakeKVService{
		db:    kvKVDB(),
		lease: &fakeSQLLease{raw: raw},
	}
	return setupKVTestRouter(t, svc, true), svc
}

func TestKVCmdStringRoundTrip(t *testing.T) {
	e, _ := kvRealEnv(t)

	// SET → "OK"
	rec := kvExec(t, e, `["SET","greet","hello"]`)
	if rec.Code != http.StatusOK {
		t.Fatalf("set = %d; %s", rec.Code, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != `"OK"` {
		t.Errorf("set resp = %s, want \"OK\"", rec.Body.String())
	}
	// GET → "hello"
	rec = kvExec(t, e, `["GET","greet"]`)
	if rec.Code != http.StatusOK {
		t.Fatalf("get = %d; %s", rec.Code, rec.Body.String())
	}
	var got any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Errorf("get = %v, want hello", got)
	}
	// SETNX 已存在 → 0
	rec = kvExec(t, e, `["SETNX","greet","other"]`)
	if rec.Code != http.StatusOK {
		t.Fatalf("setnx = %d; %s", rec.Code, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != "0" {
		t.Errorf("setnx resp = %s, want 0", rec.Body.String())
	}
	// INCR
	rec = kvExec(t, e, `["INCRBY","count","5"]`)
	if rec.Code != http.StatusOK {
		t.Fatalf("incrby = %d; %s", rec.Code, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != "5" {
		t.Errorf("incrby resp = %s, want 5", rec.Body.String())
	}
	// MGET
	rec = kvExec(t, e, `["MGET","greet","count","missing"]`)
	if rec.Code != http.StatusOK {
		t.Fatalf("mget = %d; %s", rec.Code, rec.Body.String())
	}
	var vals []any
	if err := json.Unmarshal(rec.Body.Bytes(), &vals); err != nil {
		t.Fatal(err)
	}
	if len(vals) != 3 || vals[0] != "hello" || vals[2] != nil {
		t.Errorf("mget = %v", vals)
	}
}

func TestKVCmdKeyTTL(t *testing.T) {
	e, _ := kvRealEnv(t)

	kvExec(t, e, `["SET","tmp","v","EX","100"]`)
	// TTL ∈ (0,100]
	rec := kvExec(t, e, `["TTL","tmp"]`)
	var ttl int64
	if err := json.Unmarshal(rec.Body.Bytes(), &ttl); err != nil {
		t.Fatal(err)
	}
	if ttl <= 0 || ttl > 100 {
		t.Errorf("ttl = %d, want (0,100]", ttl)
	}
	// PERSIST → 1；TTL → -1
	if rec := kvExec(t, e, `["PERSIST","tmp"]`); strings.TrimSpace(rec.Body.String()) != "1" {
		t.Errorf("persist = %s, want 1", rec.Body.String())
	}
	if rec := kvExec(t, e, `["TTL","tmp"]`); strings.TrimSpace(rec.Body.String()) != "-1" {
		t.Errorf("ttl after persist = %s, want -1", rec.Body.String())
	}
	// 不存在 → -2
	if rec := kvExec(t, e, `["TTL","nope"]`); strings.TrimSpace(rec.Body.String()) != "-2" {
		t.Errorf("ttl missing = %s, want -2", rec.Body.String())
	}
	// TYPE
	if rec := kvExec(t, e, `["TYPE","tmp"]`); strings.TrimSpace(rec.Body.String()) != `"string"` {
		t.Errorf("type = %s, want \"string\"", rec.Body.String())
	}
	// DEL → 1；EXISTS → 0
	if rec := kvExec(t, e, `["DEL","tmp"]`); strings.TrimSpace(rec.Body.String()) != "1" {
		t.Errorf("del = %s, want 1", rec.Body.String())
	}
}

func TestKVCmdScan(t *testing.T) {
	e, _ := kvRealEnv(t)

	kvExec(t, e, `["SET","a:1","v1"]`)
	kvExec(t, e, `["SET","a:2","v2"]`)
	kvExec(t, e, `["HSET","h:1","f","v"]`)

	// DBSIZE
	if rec := kvExec(t, e, `["DBSIZE"]`); strings.TrimSpace(rec.Body.String()) != "3" {
		t.Errorf("dbsize = %s, want 3", rec.Body.String())
	}
	// SCAN pattern + TYPE 过滤
	rec := kvExec(t, e, `["SCAN","0","MATCH","a:*"]`)
	var reply []any
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	if len(reply) != 2 {
		t.Fatalf("scan reply shape = %v", reply)
	}
	cursor, _ := reply[0].(string)
	keys, _ := reply[1].([]any)
	if cursor != "0" {
		t.Errorf("scan cursor = %q, want 0", cursor)
	}
	if len(keys) != 2 {
		t.Errorf("scan keys = %v, want 2", keys)
	}
	rec = kvExec(t, e, `["SCAN","0","TYPE","hash"]`)
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	if keys = reply[1].([]any); len(keys) != 1 || keys[0] != "h:1" {
		t.Errorf("scan by type = %v, want [h:1]", keys)
	}
	// 无效 COUNT
	rec = kvExec(t, e, `["SCAN","0","COUNT","9999"]`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("scan count=9999 = %d, want 400", rec.Code)
	}
}

func TestKVCmdHash(t *testing.T) {
	e, _ := kvRealEnv(t)

	// HSET 返回新增字段数
	if rec := kvExec(t, e, `["HSET","h","f1","v1","f2","v2"]`); strings.TrimSpace(rec.Body.String()) != "2" {
		t.Errorf("hset = %s, want 2", rec.Body.String())
	}
	// HGET
	if rec := kvExec(t, e, `["HGET","h","f1"]`); strings.TrimSpace(rec.Body.String()) != `"v1"` {
		t.Errorf("hget = %s, want \"v1\"", rec.Body.String())
	}
	// HMGET
	rec := kvExec(t, e, `["HMGET","h","f1","f2","nope"]`)
	var vals []any
	if err := json.Unmarshal(rec.Body.Bytes(), &vals); err != nil {
		t.Fatal(err)
	}
	if len(vals) != 3 || vals[0] != "v1" || vals[2] != nil {
		t.Errorf("hmget = %v", vals)
	}
	// HGETALL 扁平数组
	rec = kvExec(t, e, `["HGETALL","h"]`)
	var flat []any
	if err := json.Unmarshal(rec.Body.Bytes(), &flat); err != nil {
		t.Fatal(err)
	}
	if len(flat) != 4 {
		t.Fatalf("hgetall = %v, want 4 items", flat)
	}
	// HINCRBY
	if rec := kvExec(t, e, `["HINCRBY","h","cnt","3"]`); strings.TrimSpace(rec.Body.String()) != "3" {
		t.Errorf("hincrby = %s, want 3", rec.Body.String())
	}
	// HDEL → 1；HLEN → 2
	kvExec(t, e, `["HDEL","h","f1"]`)
	if rec := kvExec(t, e, `["HLEN","h"]`); strings.TrimSpace(rec.Body.String()) != "2" {
		t.Errorf("hlen = %s, want 2", rec.Body.String())
	}
}

func TestKVCmdList(t *testing.T) {
	e, _ := kvRealEnv(t)

	// RPUSH 返回长度
	if rec := kvExec(t, e, `["RPUSH","q","a","b"]`); strings.TrimSpace(rec.Body.String()) != "2" {
		t.Errorf("rpush = %s, want 2", rec.Body.String())
	}
	kvExec(t, e, `["LPUSH","q","head"]`)
	// LRANGE 全量
	rec := kvExec(t, e, `["LRANGE","q","0","-1"]`)
	var vals []any
	if err := json.Unmarshal(rec.Body.Bytes(), &vals); err != nil {
		t.Fatal(err)
	}
	if len(vals) != 3 || vals[0] != "head" || vals[2] != "b" {
		t.Errorf("lrange = %v", vals)
	}
	// LLEN
	if rec := kvExec(t, e, `["LLEN","q"]`); strings.TrimSpace(rec.Body.String()) != "3" {
		t.Errorf("llen = %s, want 3", rec.Body.String())
	}
	// LSET + LRANGE 区间
	kvExec(t, e, `["LSET","q","1","mid"]`)
	rec = kvExec(t, e, `["LRANGE","q","1","2"]`)
	if err := json.Unmarshal(rec.Body.Bytes(), &vals); err != nil {
		t.Fatal(err)
	}
	if len(vals) != 2 || vals[0] != "mid" {
		t.Errorf("lrange after lset = %v", vals)
	}
	// RPOP
	if rec := kvExec(t, e, `["RPOP","q"]`); strings.TrimSpace(rec.Body.String()) != `"b"` {
		t.Errorf("rpop = %s, want \"b\"", rec.Body.String())
	}
	// LTRIM
	if rec := kvExec(t, e, `["LTRIM","q","0","0"]`); strings.TrimSpace(rec.Body.String()) != `"OK"` {
		t.Errorf("ltrim = %s, want OK", rec.Body.String())
	}
}

func TestKVCmdSetOps(t *testing.T) {
	e, _ := kvRealEnv(t)

	kvExec(t, e, `["SADD","s1","a","b"]`)
	kvExec(t, e, `["SADD","s2","b","c"]`)

	if rec := kvExec(t, e, `["SISMEMBER","s1","a"]`); strings.TrimSpace(rec.Body.String()) != "1" {
		t.Errorf("sismember = %s, want 1", rec.Body.String())
	}
	// SINTER
	rec := kvExec(t, e, `["SINTER","s1","s2"]`)
	var vals []any
	if err := json.Unmarshal(rec.Body.Bytes(), &vals); err != nil {
		t.Fatal(err)
	}
	if len(vals) != 1 || vals[0] != "b" {
		t.Errorf("sinter = %v, want [b]", vals)
	}
	// SUNION
	rec = kvExec(t, e, `["SUNION","s1","s2"]`)
	if err := json.Unmarshal(rec.Body.Bytes(), &vals); err != nil {
		t.Fatal(err)
	}
	if len(vals) != 3 {
		t.Errorf("sunion = %v, want 3 elems", vals)
	}
	// SUNIONSTORE
	if rec := kvExec(t, e, `["SUNIONSTORE","u","s1","s2"]`); strings.TrimSpace(rec.Body.String()) != "3" {
		t.Errorf("sunionstore = %s, want 3", rec.Body.String())
	}
	// SCARD
	if rec := kvExec(t, e, `["SCARD","u"]`); strings.TrimSpace(rec.Body.String()) != "3" {
		t.Errorf("scard = %s, want 3", rec.Body.String())
	}
	// SREM → 1
	if rec := kvExec(t, e, `["SREM","u","a"]`); strings.TrimSpace(rec.Body.String()) != "1" {
		t.Errorf("srem = %s, want 1", rec.Body.String())
	}
	// SMEMBERS
	rec = kvExec(t, e, `["SMEMBERS","u"]`)
	if err := json.Unmarshal(rec.Body.Bytes(), &vals); err != nil {
		t.Fatal(err)
	}
	if len(vals) != 2 {
		t.Errorf("smembers = %v, want 2", vals)
	}
}

func TestKVCmdZSet(t *testing.T) {
	e, _ := kvRealEnv(t)

	// ZADD 返回新增数
	if rec := kvExec(t, e, `["ZADD","z","1.5","a","3","b"]`); strings.TrimSpace(rec.Body.String()) != "2" {
		t.Errorf("zadd = %s, want 2", rec.Body.String())
	}
	// ZSCORE
	if rec := kvExec(t, e, `["ZSCORE","z","a"]`); strings.TrimSpace(rec.Body.String()) != "1.5" {
		t.Errorf("zscore = %s, want 1.5", rec.Body.String())
	}
	// ZRANK
	if rec := kvExec(t, e, `["ZRANK","z","b"]`); strings.TrimSpace(rec.Body.String()) != "1" {
		t.Errorf("zrank = %s, want 1", rec.Body.String())
	}
	// ZRANGE WITHSCORES
	rec := kvExec(t, e, `["ZRANGE","z","0","-1","WITHSCORES"]`)
	var vals []any
	if err := json.Unmarshal(rec.Body.Bytes(), &vals); err != nil {
		t.Fatal(err)
	}
	if len(vals) != 4 || vals[0] != "a" || vals[3] != float64(3) {
		t.Errorf("zrange = %v", vals)
	}
	// ZRANGEBYSCORE
	rec = kvExec(t, e, `["ZRANGEBYSCORE","z","2","+inf"]`)
	if err := json.Unmarshal(rec.Body.Bytes(), &vals); err != nil {
		t.Fatal(err)
	}
	if len(vals) != 1 || vals[0] != "b" {
		t.Errorf("zrangebyscore = %v, want [b]", vals)
	}
	// ZINCRBY
	if rec := kvExec(t, e, `["ZINCRBY","z","0.5","a"]`); strings.TrimSpace(rec.Body.String()) != "2" {
		t.Errorf("zincrby = %s, want 2", rec.Body.String())
	}
	// ZCARD / ZCOUNT
	if rec := kvExec(t, e, `["ZCARD","z"]`); strings.TrimSpace(rec.Body.String()) != "2" {
		t.Errorf("zcard = %s, want 2", rec.Body.String())
	}
	if rec := kvExec(t, e, `["ZCOUNT","z","2.5","+inf"]`); strings.TrimSpace(rec.Body.String()) != "1" {
		t.Errorf("zcount = %s, want 1", rec.Body.String())
	}
	// ZREM
	if rec := kvExec(t, e, `["ZREM","z","b"]`); strings.TrimSpace(rec.Body.String()) != "1" {
		t.Errorf("zrem = %s, want 1", rec.Body.String())
	}
}

func TestKVTypedWrite(t *testing.T) {
	e, _ := kvRealEnv(t)

	// String 类型写入 → "OK"
	rec := kvReq(t, e, http.MethodPost, "/v1/projects/proj-1/kv",
		`{"type":"String","args":{"key":"s1","value":"hello"}}`)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `"OK"` {
		t.Fatalf("typed string = %d %s", rec.Code, rec.Body.String())
	}
	// 读回验证
	if rec := kvExec(t, e, `["GET","s1"]`); strings.TrimSpace(rec.Body.String()) != `"hello"` {
		t.Errorf("get typed = %s, want hello", rec.Body.String())
	}
	// Hash 类型 → 新增字段数
	rec = kvReq(t, e, http.MethodPost, "/v1/projects/proj-1/kv",
		`{"type":"Hash","args":{"key":"h1","fields":{"f":"v"}}}`)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "1" {
		t.Fatalf("typed hash = %d %s", rec.Code, rec.Body.String())
	}
	// List 类型 → 推入后长度
	rec = kvReq(t, e, http.MethodPost, "/v1/projects/proj-1/kv",
		`{"type":"List","args":{"key":"l1","elems":["a","b"],"side":"front"}}`)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "2" {
		t.Fatalf("typed list = %d %s", rec.Code, rec.Body.String())
	}
	// Set 类型 → 新增数
	rec = kvReq(t, e, http.MethodPost, "/v1/projects/proj-1/kv",
		`{"type":"Set","args":{"key":"set1","elems":["x","y"]}}`)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "2" {
		t.Fatalf("typed set = %d %s", rec.Code, rec.Body.String())
	}
	// ZSet 类型 → 新增数
	rec = kvReq(t, e, http.MethodPost, "/v1/projects/proj-1/kv",
		`{"type":"ZSet","args":{"key":"z1","items":[{"elem":"a","score":1},{"elem":"b","score":2}]}}`)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "2" {
		t.Fatalf("typed zset = %d %s", rec.Code, rec.Body.String())
	}
	// 带 ttl_ms：立即过期后 GET → null
	rec = kvReq(t, e, http.MethodPost, "/v1/projects/proj-1/kv",
		`{"type":"String","args":{"key":"gone","value":"v","ttl_ms":0}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("typed string ttl=0 = %d %s", rec.Code, rec.Body.String())
	}
	if rec := kvExec(t, e, `["GET","gone"]`); strings.TrimSpace(rec.Body.String()) != "null" {
		t.Errorf("expired get = %s, want null", rec.Body.String())
	}
}

func TestKVReadOnEmptyStore(t *testing.T) {
	e, _ := kvRealEnv(t)

	// 未建 schema 时读命令直接返回空回复，不触发建表
	if rec := kvExec(t, e, `["GET","any"]`); strings.TrimSpace(rec.Body.String()) != "null" {
		t.Errorf("get empty = %s, want null", rec.Body.String())
	}
	if rec := kvExec(t, e, `["DBSIZE"]`); strings.TrimSpace(rec.Body.String()) != "0" {
		t.Errorf("dbsize empty = %s, want 0", rec.Body.String())
	}
	if rec := kvExec(t, e, `["SCAN","0"]`); strings.TrimSpace(rec.Body.String()) == "" {
		t.Error("scan empty should return cursor reply")
	}
	// 但写命令会建表并成功
	if rec := kvExec(t, e, `["SET","k","v"]`); rec.Code != http.StatusOK {
		t.Fatalf("first write = %d %s", rec.Code, rec.Body.String())
	}
	if rec := kvExec(t, e, `["DBSIZE"]`); strings.TrimSpace(rec.Body.String()) != "1" {
		t.Errorf("dbsize after write = %s, want 1", rec.Body.String())
	}
}
