package systemdb

import (
	"context"
	"testing"
)

// planv6.0 v2 §3.8 / R7：v48 增加 writable 列，必须验证 CloudAgent 的读写往返，
// 因为 scanCloudAgent 按位置扫描，列顺序错位会静默读错字段。
func TestCloudAgentWritableRoundtrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	const pid = "writable-roundtrip"

	created, err := s.CreateCloudAgent(ctx, CloudAgent{
		ProjectID: pid, Name: "writer", Module: "database", Writable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.Writable {
		t.Fatal("created agent must keep Writable=true")
	}

	got, err := s.GetCloudAgent(ctx, pid, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Writable {
		t.Fatal("GetCloudAgent lost Writable")
	}
	if got.TeamEnabled {
		t.Fatal("TeamEnabled must stay false (column order check)")
	}
	if got.Name != "writer" || got.Module != "database" {
		t.Fatalf("field mismatch after adding a column: %+v", got)
	}

	// 默认 false：只读是新建时的默认值。
	readonly, err := s.CreateCloudAgent(ctx, CloudAgent{ProjectID: pid, Name: "reader", Module: "logs"})
	if err != nil {
		t.Fatal(err)
	}
	if readonly.Writable {
		t.Fatal("new agent must default to readonly")
	}

	// 更新为可写，并验证其它字段未被新列挤错。
	got.Writable = true
	got.TeamEnabled = true
	updated, err := s.UpdateCloudAgent(ctx, got)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Writable || !updated.TeamEnabled {
		t.Fatalf("update lost flags: %+v", updated)
	}
	reloaded, err := s.GetCloudAgent(ctx, pid, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Writable || !reloaded.TeamEnabled || reloaded.Name != "writer" {
		t.Fatalf("reload mismatch: %+v", reloaded)
	}

	// 列表路径同样要走 scanCloudAgent。
	list, err := s.ListCloudAgents(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range list {
		if a.ID == created.ID {
			found = true
			if !a.Writable {
				t.Fatal("list lost Writable")
			}
		}
	}
	if !found {
		t.Fatal("created agent missing from list")
	}
}

// 存量数据（迁移前插入的行）读出来应是只读，靠 DEFAULT 0 兼容。
func TestCloudAgentWritableDefaultsOffForLegacyRows(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	const pid = "writable-legacy"
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_cloud_agents(id, project_id, name, module, description, system_prompt, tool_ids, model_override, team_enabled, created_at, updated_at, archived_at, builtin_key)
		 VALUES('legacy', ?, 'legacy', 'general', '', '', '[]', '', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, '')`, pid); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetCloudAgent(ctx, pid, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if got.Writable {
		t.Fatal("legacy row must read as readonly")
	}
}
