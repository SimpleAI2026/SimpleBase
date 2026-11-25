// 文件说明：agent_tag_handler_test.go — internal/api 的单元测试（TestListAgentTagsMergesBuiltinDefaults、TestCreateAgentTagValidation 等）
//
// 用法
//
//	go test ./internal/api -run 'TestListAgentTagsMergesBuiltinDefaults|TestCreateAgentTagValidation|TestPatchAndDeleteAgentTag|TestAgentTagReadonlyAndDeny'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/agent_tag_handler_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	tagEcho(store *systemdb.Store, writable bool) *echo.Echo
//	tagReq(e *echo.Echo, method, path string, body any) *httptest.ResponseRecorder
//	TestListAgentTagsMergesBuiltinDefaults(t *testing.T)
//	TestCreateAgentTagValidation(t *testing.T)
//	TestPatchAndDeleteAgentTag(t *testing.T)
//	TestAgentTagReadonlyAndDeny(t *testing.T)
//	TestAgentTagsUnavailableAgents(t *testing.T)
//	TestAgentThreadsTagFilter(t *testing.T)
//	... 另有 1 个顶层符号

// planv6.7 AGC-NAV：标签 CRUD handler 测试（§7.12）。
// 覆盖：权限（denyReadonly）、只读实例 503、系统项目拒绝、跨项目助手 ID 400、
// 超长提示词 400、预置不可删、列表合并默认值无写副作用、tag_id 分区列表、
// CreateThread 新字段校验。
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func tagEcho(store *systemdb.Store, writable bool) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	h := &cloudAgentHandler{store: store, svc: testAgentRun(store, nil), writable: writable}
	e.GET("/agent-tags", h.ListAgentTags)
	e.POST("/agent-tags", h.CreateAgentTag)
	e.PATCH("/agent-tags/:tagID", h.PatchAgentTag)
	e.DELETE("/agent-tags/:tagID", h.DeleteAgentTag)
	e.GET("/agent-threads", h.ListThreads)
	e.POST("/agent-threads", h.CreateThread)
	e.GET("/agent-threads/:threadID", h.GetThread)
	return e
}

func tagReq(e *echo.Echo, method, path string, body any) *httptest.ResponseRecorder {
	return agentReq(e, method, path, body)
}

func TestListAgentTagsMergesBuiltinDefaults(t *testing.T) {
	store := openAgentStore(t)
	e := tagEcho(store, true)

	// 先播种助手（ListAgents 路径会 ensureAgents，这里直接 Seed）
	if err := store.SeedDefaultCloudAgents(context.Background(), "testprj1"); err != nil {
		t.Fatal(err)
	}
	rec := tagReq(e, http.MethodGet, "/agent-tags", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Tags []agentTagDTO `json:"tags"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Tags) != 4 {
		t.Fatalf("expected builtin general+alerts+stats+scheduled, got %d", len(listed.Tags))
	}
	general, alerts, stats := listed.Tags[0], listed.Tags[1], listed.Tags[2]
	agents, err := store.ListCloudAgents(context.Background(), "testprj1")
	if err != nil {
		t.Fatal(err)
	}
	if general.ID != systemdb.AgentTagGeneral || general.BuiltinKey != systemdb.AgentTagGeneral || len(general.AgentIDs) != len(agents) || general.FallbackAgentID == "" {
		t.Fatalf("general default: %+v", general)
	}
	if alerts.ID != systemdb.AgentTagAlerts || alerts.BuiltinKey != systemdb.AgentTagAlerts || alerts.Version != 0 {
		t.Fatalf("alerts default: %+v", alerts)
	}
	if stats.ID != systemdb.AgentTagStats || stats.Version != 0 {
		t.Fatalf("stats default: %+v", stats)
	}
	if len(alerts.AgentIDs) == 0 || len(stats.AgentIDs) == 0 {
		t.Fatalf("defaults should resolve agents: %+v", listed.Tags)
	}
	if len(alerts.Prompts) != 2 || len(stats.Prompts) != 2 {
		t.Fatalf("default prompts: %+v", listed.Tags)
	}
	created, err := store.CreateCloudAgent(context.Background(), systemdb.CloudAgent{ProjectID: "testprj1", Name: "新助手", Module: "general"})
	if err != nil {
		t.Fatal(err)
	}
	rec = tagReq(e, http.MethodGet, "/agent-tags", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	if len(listed.Tags[0].AgentIDs) != len(agents)+1 || !slices.Contains(listed.Tags[0].AgentIDs, created.ID) {
		t.Fatalf("general must include new agents: %+v", listed.Tags[0])
	}
	// 只读列表无写副作用：不产生覆盖行
	rows, err := store.ListAgentTagRows(context.Background(), "testprj1")
	if err != nil || len(rows) != 0 {
		t.Fatalf("read list must not write: %v %d", err, len(rows))
	}
}

func TestCreateAgentTagValidation(t *testing.T) {
	store := openAgentStore(t)
	if err := store.SeedDefaultCloudAgents(context.Background(), "testprj1"); err != nil {
		t.Fatal(err)
	}
	agents, err := store.ListCloudAgents(context.Background(), "testprj1")
	if err != nil || len(agents) == 0 {
		t.Fatalf("agents: %v", err)
	}
	first := agents[0].ID
	e := tagEcho(store, true)

	// 正常创建
	rec := tagReq(e, http.MethodPost, "/agent-tags", map[string]any{
		"name": "发布检查", "agent_ids": []string{first},
		"prompts": []map[string]any{{"title": "检查", "content": "检查发布"}},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created agentTagDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Version != 1 || len(created.Prompts) != 1 || created.Prompts[0].ID == "" {
		t.Fatalf("created dto: %+v", created)
	}

	// 保留名拒绝
	for _, reserved := range []string{"通用", "告警", "统计", "定时", "general", "alerts", "stats", "scheduled"} {
		rec = tagReq(e, http.MethodPost, "/agent-tags", map[string]any{"name": reserved})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("reserved %q: %d", reserved, rec.Code)
		}
	}
	// 空白/超长名
	rec = tagReq(e, http.MethodPost, "/agent-tags", map[string]any{"name": "   "})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("blank name: %d", rec.Code)
	}
	rec = tagReq(e, http.MethodPost, "/agent-tags", map[string]any{"name": strings.Repeat("长", 33)})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("long name: %d", rec.Code)
	}
	// 跨项目助手 ID
	rec = tagReq(e, http.MethodPost, "/agent-tags", map[string]any{"name": "外助手", "agent_ids": []string{"cross-project-agent"}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cross agent: %d", rec.Code)
	}
	// 超长提示词正文（>4000 字符）
	rec = tagReq(e, http.MethodPost, "/agent-tags", map[string]any{
		"name": "长提示词", "prompts": []map[string]any{{"title": "t", "content": strings.Repeat("x", 4001)}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("long prompt: %d", rec.Code)
	}
	// 提示词数量上限 20
	prompts := make([]map[string]any, 21)
	for i := range prompts {
		prompts[i] = map[string]any{"title": "t", "content": "c"}
	}
	rec = tagReq(e, http.MethodPost, "/agent-tags", map[string]any{"name": "多提示词", "prompts": prompts})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("prompt count: %d", rec.Code)
	}
	// 重名（同项目）
	rec = tagReq(e, http.MethodPost, "/agent-tags", map[string]any{"name": "发布检查"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate: %d", rec.Code)
	}
}

func TestPatchAndDeleteAgentTag(t *testing.T) {
	store := openAgentStore(t)
	if err := store.SeedDefaultCloudAgents(context.Background(), "testprj1"); err != nil {
		t.Fatal(err)
	}
	e := tagEcho(store, true)

	rec := tagReq(e, http.MethodPost, "/agent-tags", map[string]any{"name": "统计复盘", "agent_ids": []string{}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var tag agentTagDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &tag)

	// 版本不符 → 409 agent_tag_conflict
	rec = tagReq(e, http.MethodPatch, "/agent-tags/"+tag.ID, map[string]any{
		"name": "改名", "expected_version": 99,
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale patch: %d %s", rec.Code, rec.Body.String())
	}
	// 正常更新
	rec = tagReq(e, http.MethodPatch, "/agent-tags/"+tag.ID, map[string]any{
		"name": "改名", "expected_version": 1,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	var patched agentTagDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &patched)
	if patched.Version != 2 || patched.Name != "改名" {
		t.Fatalf("patched: %+v", patched)
	}

	// 预置标签：首存（expected_version=0）→ 插入覆盖行
	rec = tagReq(e, http.MethodPatch, "/agent-tags/"+systemdb.AgentTagStats, map[string]any{
		"agent_ids": []string{}, "prompts": []map[string]any{{"title": "T", "content": "C"}}, "expected_version": 0,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("builtin first save: %d %s", rec.Code, rec.Body.String())
	}
	// 预置标签改名为不同名 → 400
	rec = tagReq(e, http.MethodPatch, "/agent-tags/"+systemdb.AgentTagStats, map[string]any{
		"name": "改名统计", "expected_version": 1,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("builtin rename: %d", rec.Code)
	}
	// 预置标签版本推进
	rec = tagReq(e, http.MethodPatch, "/agent-tags/"+systemdb.AgentTagStats, map[string]any{
		"expected_version": 1,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("builtin bump: %d %s", rec.Code, rec.Body.String())
	}

	// #通用 与其他预置标签一样可保存群配置。
	rec = tagReq(e, http.MethodPatch, "/agent-tags/"+systemdb.AgentTagGeneral, map[string]any{"agent_ids": []string{}, "expected_version": 0})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch general: %d %s", rec.Code, rec.Body.String())
	}
	var general agentTagDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &general)
	if general.Version != 1 || len(general.AgentIDs) != 0 {
		t.Fatalf("general override: %+v", general)
	}
	rec = tagReq(e, http.MethodGet, "/agent-tags", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list general override: %d", rec.Code)
	}
	var generalList struct {
		Tags []agentTagDTO `json:"tags"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &generalList)
	if len(generalList.Tags) == 0 || generalList.Tags[0].Version != 1 || len(generalList.Tags[0].AgentIDs) != 0 {
		t.Fatalf("general override must persist: %+v", generalList.Tags)
	}
	// 预置删除 → 400 agent_tag_builtin
	rec = tagReq(e, http.MethodDelete, "/agent-tags/"+systemdb.AgentTagAlerts, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("delete builtin: %d", rec.Code)
	}
	// 自定义删除 → 204
	rec = tagReq(e, http.MethodDelete, "/agent-tags/"+tag.ID, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	// 再删 → 404
	rec = tagReq(e, http.MethodDelete, "/agent-tags/"+tag.ID, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("redelete: %d", rec.Code)
	}
	// 不存在的预置之外的标签 → 404
	rec = tagReq(e, http.MethodPatch, "/agent-tags/missing", map[string]any{"expected_version": 1})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("patch missing: %d", rec.Code)
	}
}

func TestAgentTagReadonlyAndDeny(t *testing.T) {
	store := openAgentStore(t)
	// 只读实例 → 503
	ro := false
	eRO := tagEcho(store, ro)
	rec := tagReq(eRO, http.MethodPost, "/agent-tags", map[string]any{"name": "x"})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readonly instance: %d", rec.Code)
	}
	// denyReadonly 走 h.writable；writable 为 true 时正常通过。
	w := true
	eW := tagEcho(store, w)
	rec = tagReq(eW, http.MethodPost, "/agent-tags", map[string]any{"name": "可写"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("writable create: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAgentTagsUnavailableAgents(t *testing.T) {
	store := openAgentStore(t)
	if err := store.SeedDefaultCloudAgents(context.Background(), "testprj1"); err != nil {
		t.Fatal(err)
	}
	e := tagEcho(store, true)
	// 引用一个不存在的助手 → 400（创建时校验归属）。
	// 不可用助手出现在编辑场景：先创建含有效助手的标签，归档助手后列表应标记不可用。
	agents, _ := store.ListCloudAgents(context.Background(), "testprj1")
	if len(agents) < 1 {
		t.Fatal("no agents")
	}
	rec := tagReq(e, http.MethodPost, "/agent-tags", map[string]any{"name": "含助手", "agent_ids": []string{agents[0].ID}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	if err := store.ArchiveCloudAgent(context.Background(), "testprj1", agents[0].ID); err != nil {
		t.Fatal(err)
	}
	rec = tagReq(e, http.MethodGet, "/agent-tags", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var listed struct {
		Tags []agentTagDTO `json:"tags"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	found := false
	for _, tag := range listed.Tags {
		if tag.Name == "含助手" {
			found = true
			if len(tag.UnavailableAgentIDs) != 1 || tag.UnavailableAgentIDs[0] != agents[0].ID {
				t.Fatalf("unavailable: %+v", tag)
			}
		}
	}
	if !found {
		t.Fatal("tag missing in list")
	}
}

func TestAgentThreadsTagFilter(t *testing.T) {
	store := openAgentStore(t)
	e := tagEcho(store, true)

	// 建标签
	rec := tagReq(e, http.MethodPost, "/agent-tags", map[string]any{"name": "发布"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tag: %d %s", rec.Code, rec.Body.String())
	}
	var tag agentTagDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &tag)

	// 建标签会话（合法）
	rec = tagReq(e, http.MethodPost, "/agent-threads", map[string]any{
		"tag_id": tag.ID, "dispatch_mode": "collaborative",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tag thread: %d %s", rec.Code, rec.Body.String())
	}
	var th struct {
		ID           string `json:"id"`
		TagID        string `json:"tag_id"`
		DispatchMode string `json:"dispatch_mode"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &th)
	if th.TagID != tag.ID || th.DispatchMode != "collaborative" {
		t.Fatalf("thread dto: %+v", th)
	}

	rec = tagReq(e, http.MethodPost, "/agent-threads", map[string]any{"tag_id": systemdb.AgentTagGeneral, "dispatch_mode": "collaborative"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create general thread: %d %s", rec.Code, rec.Body.String())
	}
	var generalTh struct {
		ID           string `json:"id"`
		DispatchMode string `json:"dispatch_mode"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &generalTh)
	if generalTh.DispatchMode != "collaborative" {
		t.Fatalf("general thread dto: %+v", generalTh)
	}

	// 非法组合
	rec = tagReq(e, http.MethodPost, "/agent-threads", map[string]any{"tag_id": tag.ID})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("tag without auto: %d", rec.Code)
	}
	for _, mode := range []string{"manual", "auto"} {
		// 一轮一助手的 auto 调度已下线（planv6.8 §13.15 CB-BE-13）：群会话只有 collaborative。
		rec = tagReq(e, http.MethodPost, "/agent-threads", map[string]any{"dispatch_mode": mode})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("bad mode %q: %d", mode, rec.Code)
		}
	}
	rec = tagReq(e, http.MethodPost, "/agent-threads", map[string]any{"source": "nope"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad source: %d", rec.Code)
	}
	agents, _ := store.ListCloudAgents(context.Background(), "testprj1")
	agentID := ""
	for _, a := range agents {
		if a.ID != "" {
			agentID = a.ID
			break
		}
	}
	if agentID != "" {
		rec = tagReq(e, http.MethodPost, "/agent-threads", map[string]any{"dispatch_mode": "collaborative", "agent_id": agentID})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("collaborative+agent: %d", rec.Code)
		}
	}
	rec = tagReq(e, http.MethodPost, "/agent-threads", map[string]any{"tag_id": "missing-tag", "dispatch_mode": "collaborative"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing tag: %d", rec.Code)
	}

	rec = tagReq(e, http.MethodGet, "/agent-threads?tag_id=general", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list general: %d %s", rec.Code, rec.Body.String())
	}
	var generalList struct {
		Threads []struct {
			ID string `json:"id"`
		} `json:"threads"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &generalList)
	if len(generalList.Threads) != 1 || generalList.Threads[0].ID != generalTh.ID {
		t.Fatalf("general threads: %+v", generalList.Threads)
	}

	// tag_id=<标签> 过滤
	rec = tagReq(e, http.MethodGet, "/agent-threads?tag_id="+tag.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list tag: %d", rec.Code)
	}
	var tagList struct {
		Threads []struct {
			ID    string `json:"id"`
			TagID string `json:"tag_id"`
		} `json:"threads"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tagList)
	if len(tagList.Threads) != 1 || tagList.Threads[0].ID != th.ID || tagList.Threads[0].TagID != tag.ID {
		t.Fatalf("tag threads: %+v", tagList.Threads)
	}

	// 归档标签后：标签分区 404
	rec = tagReq(e, http.MethodDelete, "/agent-tags/"+tag.ID, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("archive tag: %d", rec.Code)
	}
	rec = tagReq(e, http.MethodGet, "/agent-threads?tag_id="+tag.ID, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("archived tag filter: %d", rec.Code)
	}
	rec = tagReq(e, http.MethodGet, "/agent-threads?tag_id=general", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &generalList)
	if len(generalList.Threads) != 1 || generalList.Threads[0].ID != generalTh.ID {
		t.Fatalf("general threads after custom tag archive: %+v", generalList.Threads)
	}

	// 不带 tag_id 保持原语义：返回全部
	rec = tagReq(e, http.MethodGet, "/agent-threads", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("plain list: %d", rec.Code)
	}
	var all struct {
		Threads []json.RawMessage `json:"threads"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &all)
	if len(all.Threads) < 2 {
		t.Fatalf("plain list should include all: %d", len(all.Threads))
	}

	// GetThread 返回 tag_id/dispatch_mode
	rec = tagReq(e, http.MethodGet, "/agent-threads/"+th.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get thread: %d", rec.Code)
	}
	var got struct {
		TagID string `json:"tag_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	// v2 §9.2：群归档保留历史会话的归属（不再清空 tag_id），但该会话不再接受新发送。
	if got.TagID != tag.ID {
		t.Fatalf("tag should be kept after archive: %+v", got)
	}
}

// 系统项目拒绝写操作（writeGate 中间件在真实路由里做；这里测 handler 层
// normalizeTagBody 的助手归属校验使用项目上下文，跨项目助手 ID 拒绝已覆盖）。
func TestAgentTagBuiltinDefaultsResolve(t *testing.T) {
	store := openAgentStore(t)
	if err := store.SeedDefaultCloudAgents(context.Background(), "testprj1"); err != nil {
		t.Fatal(err)
	}
	e := tagEcho(store, true)
	rec := tagReq(e, http.MethodGet, "/agent-tags", nil)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code)
	}
	var listed struct {
		Tags []agentTagDTO `json:"tags"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	if len(listed.Tags) != 4 {
		t.Fatalf("tags: %d", len(listed.Tags))
	}
	// builtin 默认 agent_ids 应解析为本项目真实助手 ID（非 builtin_key）
	agents, _ := store.ListCloudAgents(context.Background(), "testprj1")
	byKey := map[string]string{}
	for _, a := range agents {
		if a.BuiltinKey != "" {
			byKey[a.BuiltinKey] = a.ID
		}
	}
	alerts := listed.Tags[1]
	for _, id := range alerts.AgentIDs {
		if id != byKey["logs"] && id != byKey["general"] {
			t.Fatalf("alerts agent not resolved: %s", id)
		}
	}
	// 确认无碰撞
	_ = catalog.ReservedTenantID
}
