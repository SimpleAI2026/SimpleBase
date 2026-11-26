// 文件说明：access_test.go — 集合行级访问规则的解析、判定与 owner 写入测试。
//
// 用法
//
//	go test ./internal/collection -run TestAccessRule
//
// 调用架构
//
//	[上游] 无（同包测试）
//	[本文件] internal/collection/access_test.go
//	[下游] internal/collection/access.go
//
// 主要符号
//
//	TestParseAccessRule / TestAccessRuleCanReadWrite / TestAccessRuleStampAndPreserveOwner

package collection

import (
	"errors"
	"testing"
)

func TestParseAccessRule(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		mode    AccessMode
		field   string
		wantErr bool
	}{
		{name: "empty", raw: "", mode: AccessOff, field: DefaultOwnerField},
		{name: "null", raw: "null", mode: AccessOff, field: DefaultOwnerField},
		{name: "empty object", raw: "{}", mode: AccessOff, field: DefaultOwnerField},
		{name: "owner read", raw: `{"mode":"owner_read"}`, mode: AccessOwnerRead, field: DefaultOwnerField},
		{name: "owner write custom field", raw: `{"mode":"owner_write","owner_field":"user_id"}`, mode: AccessOwnerWrite, field: "user_id"},
		{name: "unknown mode", raw: `{"mode":"owner"}`, wantErr: true},
		{name: "bad field", raw: `{"mode":"owner_read","owner_field":"1bad"}`, wantErr: true},
		{name: "reserved field", raw: `{"mode":"owner_read","owner_field":"id"}`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule, err := ParseAccessRule(tc.raw)
			if tc.wantErr {
				if !errors.Is(err, ErrAccessRuleInvalid) {
					t.Fatalf("want ErrAccessRuleInvalid, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rule.Mode != tc.mode || rule.OwnerField != tc.field {
				t.Fatalf("got %+v, want mode=%s field=%s", rule, tc.mode, tc.field)
			}
		})
	}
}

func TestAccessRuleCanReadWrite(t *testing.T) {
	read := AccessRule{Mode: AccessOwnerRead, OwnerField: "owner_id"}
	write := AccessRule{Mode: AccessOwnerWrite, OwnerField: "owner_id"}
	off := DefaultAccessRule()

	cases := []struct {
		name     string
		rule     AccessRule
		canRead  bool
		canWrite bool
	}{
		{name: "off", rule: off, canRead: true, canWrite: true},
		{name: "owner read own", rule: read, canRead: true, canWrite: true},
		{name: "owner write own", rule: write, canRead: true, canWrite: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.rule.CanRead("user:1", "user:1", false); got != tc.canRead {
				t.Fatalf("CanRead=%v want %v", got, tc.canRead)
			}
			if got := tc.rule.CanWrite("user:1", "user:1", false); got != tc.canWrite {
				t.Fatalf("CanWrite=%v want %v", got, tc.canWrite)
			}
		})
	}

	if read.CanRead("user:2", "user:1", false) {
		t.Fatal("owner_read must not read another owner's row")
	}
	if write.CanWrite("user:2", "user:1", false) {
		t.Fatal("owner_write must not write another owner's row")
	}
	if !write.CanWrite("user:2", "user:1", true) {
		t.Fatal("admin must bypass row rules")
	}
	if write.CanWrite("user:1", "", false) {
		t.Fatal("missing owner must be denied for non-admin")
	}
}

func TestAccessRuleStampAndPreserveOwner(t *testing.T) {
	rule := AccessRule{Mode: AccessOwnerWrite, OwnerField: "owner_id"}

	doc := map[string]any{"name": "a"}
	rule.StampOwner(doc, "user:1", false)
	if doc["owner_id"] != "user:1" {
		t.Fatalf("owner not stamped: %+v", doc)
	}

	forged := map[string]any{"owner_id": "user:2"}
	rule.StampOwner(forged, "user:1", false)
	if forged["owner_id"] != "user:1" {
		t.Fatalf("non-admin must not forge owner: %+v", forged)
	}

	adminDoc := map[string]any{"owner_id": "user:9"}
	rule.StampOwner(adminDoc, "user:1", true)
	if adminDoc["owner_id"] != "user:9" {
		t.Fatalf("admin explicit owner should be kept: %+v", adminDoc)
	}

	updated := map[string]any{"owner_id": "user:2", "name": "b"}
	rule.PreserveOwner(updated, map[string]any{"owner_id": "user:1"}, false)
	if updated["owner_id"] != "user:1" {
		t.Fatalf("owner should be preserved: %+v", updated)
	}

	adminUpdated := map[string]any{"owner_id": "user:2"}
	rule.PreserveOwner(adminUpdated, map[string]any{"owner_id": "user:1"}, true)
	if adminUpdated["owner_id"] != "user:2" {
		t.Fatalf("admin should be able to change owner: %+v", adminUpdated)
	}
}
