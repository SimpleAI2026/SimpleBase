// 文件说明：commands_snapshot_test.go — internal/sbcli 的单元测试（TestCommandSnapshotBeforeMerge 等）
//
// 用法
//
//	go test ./internal/sbcli -run 'TestCommandSnapshotBeforeMerge'
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/agentbridge
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sbcli/commands_snapshot_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] cli_test.go, collection_transfer.go, collection_transfer_test.go, commands.go, commands_admin.go 等 27 个
//
// 主要符号
//
//	TestCommandSnapshotBeforeMerge(t *testing.T)

package sbcli

import (
	"slices"
	"strings"
	"testing"
)

// 锁定旧实现下的 readOnlyCommands 键集合（按键分发的命令清单）。
// 合并前后必须相等，确保整改没有误删或误改判定结果。
func TestCommandSnapshotBeforeMerge(t *testing.T) {
	oldReadOnlyKeys := []string{
		"apikey list",
		"audit list",
		"backup list",
		"collection list",
		"collection schema-get",
		"cron get",
		"cron list",
		"cron runs",
		"database get",
		"database list",
		"dev plan",
		"dev status",
		"dev validate",
		"document export",
		"document get",
		"document list",
		"function get",
		"function list",
		"function test",
		"log search",
		"migrate plan",
		"migrate status",
		"object list",
		"object presign",
		"project list",
		"quota get",
		"sandbox capabilities",
		"sandbox get",
		"sandbox list",
		"schema rows",
		"schema show",
		"settings get",
		"site status",
		"sql query",
		"user get",
		"user list",
	}
	slices.Sort(oldReadOnlyKeys)

	var got []string
	for key, spec := range commandSpecs {
		ro := spec.readOnly
		if ro {
			got = append(got, key)
		}
	}
	slices.Sort(got)

	if len(got) != len(oldReadOnlyKeys) {
		t.Fatalf("readOnlyCommands count=%d, want %d", len(got), len(oldReadOnlyKeys))
	}
	for i := range got {
		if got[i] != oldReadOnlyKeys[i] {
			t.Errorf("readOnlyCommands[%d]=%q, want %q", i, got[i], oldReadOnlyKeys[i])
		}
	}

	// 锁定旧实现下 DispatchKnown 返回 true 的完整键集。
	// 合并后 commandSpecs 的覆盖范围必须相等。
	allKnown := []string{
		"apikey create",
		"apikey list",
		"apikey revoke",
		"audit list",
		"backup create",
		"backup delete",
		"backup list",
		"backup restore",
		"collection create",
		"collection list",
		"collection schema-get",
		"collection schema-put",
		"collection truncate",
		"cron create",
		"cron delete",
		"cron get",
		"cron list",
		"cron runs",
		"cron trigger",
		"cron update",
		"database create",
		"database delete",
		"database get",
		"database list",
		"dev apply",
		"dev plan",
		"dev status",
		"dev validate",
		"document batch",
		"document delete",
		"document export",
		"document get",
		"document import",
		"document insert",
		"document list",
		"document patch",
		"document update",
		"function activate",
		"function create",
		"function delete",
		"function get",
		"function invoke",
		"function list",
		"function test",
		"function update",
		"function version",
		"function versions",
		"kv exec",
		"log retention",
		"log search",
		"migrate plan",
		"migrate status",
		"object delete",
		"object list",
		"object presign",
		"object upload",
		"project create",
		"project list",
		"quota get",
		"sandbox capabilities",
		"sandbox connect",
		"sandbox create",
		"sandbox delete",
		"sandbox get",
		"sandbox list",
		"sandbox pause",
		"schema add-column",
		"schema create-table",
		"schema rows",
		"schema show",
		"settings get",
		"settings set",
		"site publish",
		"site status",
		"sql batch",
		"sql exec",
		"sql query",
		"user create",
		"user delete",
		"user disable",
		"user get",
		"user list",
		"user update",
	}
	slices.Sort(allKnown)
	var actual []string
	for _, k := range allKnown {
		resource, action, ok := strings.Cut(k, " ")
		if ok && DispatchKnown(resource, action) {
			actual = append(actual, k)
		}
	}
	slices.Sort(actual)
	if len(actual) != len(allKnown) {
		t.Fatalf("DispatchKnown count=%d, want %d", len(actual), len(allKnown))
	}
	for i := range actual {
		if actual[i] != allKnown[i] {
			t.Errorf("DispatchKnown[%d]=%q, want %q", i, actual[i], allKnown[i])
		}
	}
}
