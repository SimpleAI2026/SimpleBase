// cases.go 实现 plan §2.4.1 用例矩阵的三个执行组（用户要求收敛为）：
//   - HTTP：链路下限与管理面（SELECT 1 点查、库列表、健康检查）
//   - DB  ：SQL 增删改查 + Data 文档增删改查（A/B 组合并）
//   - KV  ：项目 KV 增删改查（C 组）
//
// 每条用例自带契约断言（状态码/行数/错误码），失败即计入错误样本。
package perfbench

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// benchError 携带稳定错误码的压测断言错误。
type benchError struct {
	CodeVal string
	Message string
}

func (e *benchError) Error() string   { return e.CodeVal + ": " + e.Message }
func (e *benchError) ErrCode() string { return e.CodeVal }

func fail(code, msg string, args ...any) error {
	return &benchError{CodeVal: code, Message: fmt.Sprintf(msg, args...)}
}

// httpCases 返回 HTTP 组用例：链路固定开销 + 管理面。
func httpCases() []benchCase {
	return []benchCase{
		{
			ID:       "HTTP-SELECT1",
			Desc:     "链路下限：query SELECT 1",
			Endpoint: "sql.query",
			Setup: func(env *benchEnv) error {
				return env.setupUserDB("bench-http")
			},
			Step: func(env *benchEnv, i int) error {
				code, body, err := env.Do("POST", env.sqlPath("query"), map[string]any{"sql": "SELECT 1 AS v"})
				if err != nil {
					return fail("transport", "%v", err)
				}
				if code != http.StatusOK {
					return fail("http_"+strconv.Itoa(code), "%s", body)
				}
				var resp struct {
					RowCount int `json:"row_count"`
				}
				if json.Unmarshal(body, &resp) != nil || resp.RowCount != 1 {
					return fail("contract", "row_count=%d body=%s", resp.RowCount, body)
				}
				return nil
			},
		},
		{
			ID:       "HTTP-LISTDB",
			Desc:     "管理面：GET /databases 列表（含缓存行数填充）",
			Endpoint: "databases.list",
			Setup: func(env *benchEnv) error {
				return env.setupUserDB("bench-http-list")
			},
			Step: func(env *benchEnv, i int) error {
				code, body, err := env.Do("GET", "/v1/projects/"+env.ProjectID+"/databases?limit=20", nil)
				if err != nil {
					return fail("transport", "%v", err)
				}
				if code != http.StatusOK {
					return fail("http_"+strconv.Itoa(code), "%s", body)
				}
				var resp struct {
					Databases []json.RawMessage `json:"databases"`
				}
				if json.Unmarshal(body, &resp) != nil {
					return fail("contract", "bad json: %s", body)
				}
				return nil
			},
		},
		{
			ID:       "HTTP-HEALTH",
			Desc:     "无认证健康检查（框架开销下限）",
			Endpoint: "health.ready",
			Setup:    func(env *benchEnv) error { return nil },
			Step: func(env *benchEnv, i int) error {
				req, err := httpNewRequest("GET", env.Server.URL+"/health/ready")
				if err != nil {
					return fail("transport", "%v", err)
				}
				resp, err := env.Client.Do(req)
				if err != nil {
					return fail("transport", "%v", err)
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					return fail("http_"+strconv.Itoa(resp.StatusCode), "health not ok")
				}
				return nil
			},
		},
	}
}

// dbCases 返回 DB 组用例：SQL CRUD + Data 文档 CRUD（plan §2.4.1 A/B 组）。
func dbCases() []benchCase {
	return []benchCase{
		{
			ID:       "DB-INSERT",
			Desc:     "SQL 增：execute INSERT 单行",
			Endpoint: "sql.execute",
			Setup: func(env *benchEnv) error {
				if err := env.setupUserDB("bench-db-ins"); err != nil {
					return err
				}
				// 预置 UPDATE/DELETE 目标行池。
				for i := 0; i < 200; i++ {
					if err := benchInsert(env, i); err != nil {
						return err
					}
				}
				return nil
			},
			Step: func(env *benchEnv, i int) error {
				return benchInsert(env, 100000+i)
			},
		},
		{
			ID:       "DB-SELECT",
			Desc:     "SQL 查：query 主键点查命中 1 行（播种 1k 行）",
			Endpoint: "sql.query",
			Setup: func(env *benchEnv) error {
				if err := env.setupUserDB("bench-db-sel"); err != nil {
					return err
				}
				// T3：播种目标行，保证点查命中扫描与序列化路径。
				for i := 0; i < 1000; i++ {
					if err := benchInsert(env, i); err != nil {
						return err
					}
				}
				return nil
			},
			Step: func(env *benchEnv, i int) error {
				code, body, err := env.Do("POST", env.sqlPath("query"), map[string]any{
					"sql":  "SELECT id, payload, seq FROM bench_rows WHERE seq = ?",
					"args": []any{float64(i % 1000)},
				})
				if err != nil {
					return fail("transport", "%v", err)
				}
				if code != http.StatusOK {
					return fail("http_"+strconv.Itoa(code), "%s", body)
				}
				var resp struct {
					RowCount int `json:"row_count"`
				}
				if json.Unmarshal(body, &resp) != nil {
					return fail("contract", "bad json: %s", body)
				}
				if resp.RowCount != 1 {
					return fail("contract", "row_count=%d (seeded point lookup must hit 1)", resp.RowCount)
				}
				return nil
			},
		},
		{
			ID:       "DB-UPDATE",
			Desc:     "SQL 改：execute UPDATE 单行",
			Endpoint: "sql.execute",
			Setup: func(env *benchEnv) error {
				if err := env.setupUserDB("bench-db-upd"); err != nil {
					return err
				}
				for i := 0; i < 200; i++ {
					if err := benchInsert(env, i); err != nil {
						return err
					}
				}
				return nil
			},
			Step: func(env *benchEnv, i int) error {
				code, body, err := env.Do("POST", env.sqlPath("execute"), map[string]any{
					"sql":  "UPDATE bench_rows SET payload = ? WHERE seq = ?",
					"args": []any{"updated-" + strconv.Itoa(i), float64(i % 200)},
				})
				if err != nil {
					return fail("transport", "%v", err)
				}
				if code != http.StatusOK {
					return fail("http_"+strconv.Itoa(code), "%s", body)
				}
				var resp struct {
					RowsAffected int64 `json:"rows_affected"`
				}
				if json.Unmarshal(body, &resp) != nil {
					return fail("contract", "bad json: %s", body)
				}
				if resp.RowsAffected != 1 {
					return fail("contract", "rows_affected=%d", resp.RowsAffected)
				}
				return nil
			},
		},
		{
			ID:       "DB-DELETE",
			Desc:     "SQL 删：execute DELETE 单行",
			Endpoint: "sql.execute",
			Setup: func(env *benchEnv) error {
				if err := env.setupUserDB("bench-db-del"); err != nil {
					return err
				}
				// 每轮删除后重新播种（setup 只跑一次，Step 里删一行补一行维持数据量）。
				for i := 0; i < 300; i++ {
					if err := benchInsert(env, i); err != nil {
						return err
					}
				}
				return nil
			},
			Step: func(env *benchEnv, i int) error {
				seq := i % 300
				code, body, err := env.Do("POST", env.sqlPath("execute"), map[string]any{
					"sql":  "DELETE FROM bench_rows WHERE seq = ?",
					"args": []any{float64(seq)},
				})
				if err != nil {
					return fail("transport", "%v", err)
				}
				if code != http.StatusOK {
					return fail("http_"+strconv.Itoa(code), "%s", body)
				}
				// 维持行池：删除后立刻补回同 seq 新行。
				return benchInsert(env, seq)
			},
		},
		{
			ID:       "DB-BATCH",
			Desc:     "SQL 批：batch 10 条 INSERT（事务）",
			Endpoint: "sql.batch",
			Setup: func(env *benchEnv) error {
				return env.setupUserDB("bench-db-batch")
			},
			Step: func(env *benchEnv, i int) error {
				stmts := make([]map[string]any, 10)
				for j := range stmts {
					stmts[j] = map[string]any{
						"sql":  "INSERT INTO bench_rows (id, payload, seq) VALUES (?, ?, ?)",
						"args": []any{benchUUID(), "batch-payload", float64(200000 + i*10 + j)},
					}
				}
				code, body, err := env.Do("POST", env.sqlPath("batch"), map[string]any{
					"statements":    stmts,
					"transactional": true,
				})
				if err != nil {
					return fail("transport", "%v", err)
				}
				if code != http.StatusOK {
					return fail("http_"+strconv.Itoa(code), "%s", body)
				}
				return nil
			},
		},
		{
			ID:       "DB-DOC-CREATE",
			Desc:     "Data 增：POST documents（幂等 DDL + INSERT 两次 Execute）",
			Endpoint: "data.create",
			Setup: func(env *benchEnv) error {
				return env.setupUserDB("bench-db-doc")
			},
			Step: func(env *benchEnv, i int) error {
				code, body, err := env.Do("POST", env.dataPath("collections", "docs", "documents"), map[string]any{
					"field": "doc-" + strconv.Itoa(i),
					"n":     float64(i),
				})
				if err != nil {
					return fail("transport", "%v", err)
				}
				if code != http.StatusCreated {
					return fail("http_"+strconv.Itoa(code), "%s", body)
				}
				return nil
			},
		},
		{
			ID:       "DB-DOC-LIST",
			Desc:     "Data 查：GET documents（默认 1000 行上限）",
			Endpoint: "data.list",
			Setup: func(env *benchEnv) error {
				if err := env.setupUserDB("bench-db-doc-l"); err != nil {
					return err
				}
				for i := 0; i < 100; i++ {
					code, body, err := env.Do("POST", env.dataPath("collections", "docs", "documents"), map[string]any{"n": float64(i)})
					if err != nil {
						return err
					}
					if code != http.StatusCreated {
						return fmt.Errorf("perfbench: seed doc: http %d: %s", code, body)
					}
				}
				return nil
			},
			Step: func(env *benchEnv, i int) error {
				code, body, err := env.Do("GET", env.dataPath("collections", "docs"), nil)
				if err != nil {
					return fail("transport", "%v", err)
				}
				if code != http.StatusOK {
					return fail("http_"+strconv.Itoa(code), "%s", body)
				}
				var resp struct {
					Rows []json.RawMessage `json:"rows"`
				}
				if json.Unmarshal(body, &resp) != nil {
					return fail("contract", "bad json: %s", body)
				}
				return nil
			},
		},
		{
			ID:       "DB-DOC-UPDATE-DELETE",
			Desc:     "Data 改+删：LIST+PUT+DELETE+POST 4 请求复合样本",
			Endpoint: "data.update_delete",
			ReqCount: 4,
			Setup: func(env *benchEnv) error {
				if err := env.setupUserDB("bench-db-doc-ud"); err != nil {
					return err
				}
				for i := 0; i < 100; i++ {
					code, body, err := env.Do("POST", env.dataPath("collections", "docs", "documents"), map[string]any{"n": float64(i)})
					if err != nil {
						return err
					}
					if code != http.StatusCreated {
						return fmt.Errorf("perfbench: seed doc: http %d: %s", code, body)
					}
				}
				return nil
			},
			Step: func(env *benchEnv, i int) error {
				// 先列出拿一个文档 id。
				code, body, err := env.Do("GET", env.dataPath("collections", "docs"), nil)
				if err != nil {
					return fail("transport", "%v", err)
				}
				if code != http.StatusOK {
					return fail("http_"+strconv.Itoa(code), "%s", body)
				}
				var listResp struct {
					Rows []struct {
						ID string `json:"id"`
					} `json:"rows"`
				}
				if json.Unmarshal(body, &listResp) != nil || len(listResp.Rows) == 0 {
					return fail("contract", "no docs: %s", body)
				}
				// 按 worker 取模错开目标行，降低同行竞争。
				id := listResp.Rows[i%len(listResp.Rows)].ID

				// PUT 更新。
				code, body, err = env.Do("PUT", env.dataPath("collections", "docs", "documents", id), map[string]any{"updated": true})
				if err != nil {
					return fail("transport", "%v", err)
				}
				if code == http.StatusNotFound {
					return nil // 并发竞态：目标已被其他 worker 删走，不计失败。
				}
				if code != http.StatusOK {
					return fail("http_"+strconv.Itoa(code), "%s", body)
				}
				// DELETE 删除。
				code, body, err = env.Do("DELETE", env.dataPath("collections", "docs", "documents", id), nil)
				if err != nil {
					return fail("transport", "%v", err)
				}
				if code == http.StatusNotFound {
					return nil
				}
				if code != http.StatusNoContent {
					return fail("http_"+strconv.Itoa(code), "%s", body)
				}
				// 补一行维持数据量。
				code, body, err = env.Do("POST", env.dataPath("collections", "docs", "documents"), map[string]any{"n": float64(i)})
				if err != nil {
					return fail("transport", "%v", err)
				}
				if code != http.StatusCreated {
					return fail("http_"+strconv.Itoa(code), "%s", body)
				}
				return nil
			},
		},
	}
}

// kvCases 返回 KV 组用例：项目 KV 单端点增删改查（plan §2.4.1 C 组）。
func kvCases() []benchCase {
	return []benchCase{
		{
			ID:       "KV-SET",
			Desc:     "KV 增/改：cmd SET（写事务 + NotifyWrite）",
			Endpoint: "kv.set",
			Setup:    func(env *benchEnv) error { return nil }, // dev-shop 种子已含项目 KV
			Step: func(env *benchEnv, i int) error {
				code, body, err := env.Do("POST", env.kvPath(), map[string]any{
					"type": "cmd", "argvs": []string{"SET", "bench:str:" + strconv.Itoa(i%64), "value-" + strconv.Itoa(i)},
				})
				if err != nil {
					return fail("transport", "%v", err)
				}
				if code != http.StatusOK {
					return fail("http_"+strconv.Itoa(code), "%s", body)
				}
				var out string
				if json.Unmarshal(body, &out) != nil || out != "OK" {
					return fail("contract", "want OK got %s", body)
				}
				return nil
			},
		},
		{
			ID:       "KV-GET",
			Desc:     "KV 查：cmd GET 命中",
			Endpoint: "kv.get",
			Setup: func(env *benchEnv) error {
				code, body, err := env.Do("POST", env.kvPath(), map[string]any{
					"type": "cmd", "argvs": []string{"SET", "bench:get", "hit"},
				})
				if err != nil {
					return err
				}
				if code != http.StatusOK {
					return fmt.Errorf("perfbench: kv seed: http %d: %s", code, body)
				}
				return nil
			},
			Step: func(env *benchEnv, i int) error {
				code, body, err := env.Do("POST", env.kvPath(), map[string]any{
					"type": "cmd", "argvs": []string{"GET", "bench:get"},
				})
				if err != nil {
					return fail("transport", "%v", err)
				}
				if code != http.StatusOK {
					return fail("http_"+strconv.Itoa(code), "%s", body)
				}
				var out string
				if json.Unmarshal(body, &out) != nil || out != "hit" {
					return fail("contract", "want hit got %s", body)
				}
				return nil
			},
		},
		{
			ID:       "KV-HSETALL",
			Desc:     "KV 查：cmd HSET + HGETALL（100 field）",
			Endpoint: "kv.hgetall",
			Setup: func(env *benchEnv) error {
				fields := map[string]string{}
				for i := 0; i < 100; i++ {
					fields["f"+strconv.Itoa(i)] = "v" + strconv.Itoa(i)
				}
				code, body, err := env.Do("POST", env.kvPath(), map[string]any{
					"type": "cmd", "argvs": []string{"DEL", "bench:hash"},
				})
				if err != nil {
					return err
				}
				if code != http.StatusOK {
					return fmt.Errorf("perfbench: kv del seed: http %d: %s", code, body)
				}
				code, body, err = env.Do("POST", env.kvPath(), map[string]any{
					"type": "cmd", "argvs": append([]string{"HSET", "bench:hash"}, hashArgvs(fields)...),
				})
				if err != nil {
					return err
				}
				if code != http.StatusOK {
					return fmt.Errorf("perfbench: kv hset seed: http %d: %s", code, body)
				}
				return nil
			},
			Step: func(env *benchEnv, i int) error {
				code, body, err := env.Do("POST", env.kvPath(), map[string]any{
					"type": "cmd", "argvs": []string{"HGETALL", "bench:hash"},
				})
				if err != nil {
					return fail("transport", "%v", err)
				}
				if code != http.StatusOK {
					return fail("http_"+strconv.Itoa(code), "%s", body)
				}
				// HGETALL 返回扁平数组 [field1, value1, field2, value2, ...]。
				var out []string
				if json.Unmarshal(body, &out) != nil || len(out) != 200 {
					return fail("contract", "want 200 flat entries (100 fields) got %d: %s", len(out), body)
				}
				return nil
			},
		},
		{
			ID:       "KV-INCR",
			Desc:     "KV 增：cmd INCR（读-改-写事务串行化）",
			Endpoint: "kv.incr",
			Setup:    func(env *benchEnv) error { return nil },
			Step: func(env *benchEnv, i int) error {
				code, body, err := env.Do("POST", env.kvPath(), map[string]any{
					"type": "cmd", "argvs": []string{"INCR", "bench:counter"},
				})
				if err != nil {
					return fail("transport", "%v", err)
				}
				if code != http.StatusOK {
					return fail("http_"+strconv.Itoa(code), "%s", body)
				}
				var out json.Number
				if json.Unmarshal(body, &out) != nil {
					return fail("contract", "want number got %s", body)
				}
				return nil
			},
		},
		{
			ID:       "KV-DEL",
			Desc:     "KV 删：cmd SET 后 DEL（2 请求复合样本）",
			Endpoint: "kv.del",
			ReqCount: 2,
			Setup:    func(env *benchEnv) error { return nil },
			Step: func(env *benchEnv, i int) error {
				key := "bench:del:" + strconv.Itoa(i%64)
				code, body, err := env.Do("POST", env.kvPath(), map[string]any{
					"type": "cmd", "argvs": []string{"SET", key, "tmp"},
				})
				if err != nil {
					return fail("transport", "%v", err)
				}
				if code != http.StatusOK {
					return fail("http_"+strconv.Itoa(code), "%s", body)
				}
				code, body, err = env.Do("POST", env.kvPath(), map[string]any{
					"type": "cmd", "argvs": []string{"DEL", key},
				})
				if err != nil {
					return fail("transport", "%v", err)
				}
				if code != http.StatusOK {
					return fail("http_"+strconv.Itoa(code), "%s", body)
				}
				var n json.Number
				if json.Unmarshal(body, &n) != nil {
					return fail("contract", "want count got %s", body)
				}
				return nil
			},
		},
	}
}
