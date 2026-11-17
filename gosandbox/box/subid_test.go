//go:build linux

package box

// subid_test.go：多段 uid 映射的计划生成。
//
// 这部分错了会让沙盒里的服务用户「看起来存在但 setuid 失败」，
// 或者反过来把系统 uid 段也映射进去。所以计划本身要钉住。

import (
	"strings"
	"testing"
)

// 单段模式：不产生任何映射参数。
func TestSubIDMapSingleSegment(t *testing.T) {
	m := SubIDMap{Reason: "no subuid"}
	if m.MultiSegment() {
		t.Fatal("must not be multi-segment")
	}
	if args := m.IDMapArgs(); len(args) != 0 {
		t.Fatalf("args = %v", args)
	}
	uids, gids := m.MappedRanges()
	if len(uids) != 0 || len(gids) != 0 {
		t.Fatalf("uids=%v gids=%v", uids, gids)
	}
	if spec := subIDSpec(m); spec != "" {
		t.Fatalf("spec = %q", spec)
	}
}

// 多段模式：第一段是 0→宿主 uid，之后从 1000 起铺开。
func TestSubIDMapMultiSegment(t *testing.T) {
	m := SubIDMap{
		UID: 1000, GID: 1000,
		UIDs:           []SubID{{Start: 100000, Count: 65536}},
		GIDs:           []SubID{{Start: 100000, Count: 65536}},
		SandboxUIDBase: 1000, SandboxGIDBase: 1000,
	}
	if !m.MultiSegment() {
		t.Fatal("must be multi-segment")
	}
	args := m.IDMapArgs()
	if len(args) == 0 {
		t.Fatal("no args")
	}
	// 参数是键值成对出现。
	if len(args)%2 != 0 {
		t.Fatalf("args must be key/value pairs: %v", args)
	}
	joined := strings.Join(args, " ")
	// 沙盒内 0 → 宿主 1000。
	if !strings.Contains(joined, "--uidmap 0:1000:1") {
		t.Errorf("missing root mapping: %s", joined)
	}
	// 沙盒内 1000 起的一段 → 宿主 100000 起。
	if !strings.Contains(joined, "--uidmap 1000:100000:") {
		t.Errorf("missing service-user range: %s", joined)
	}
	if !strings.Contains(joined, "--gidmap 0:1000:1") {
		t.Errorf("missing gid root mapping: %s", joined)
	}
}

// 映射总量截到 maxMappedIDs。
func TestSubIDMapCapsTotal(t *testing.T) {
	m := SubIDMap{
		UID: 1000, GID: 1000,
		UIDs:           []SubID{{Start: 100000, Count: 1 << 20}}, // 一百万，远超上限
		GIDs:           []SubID{{Start: 100000, Count: 1 << 20}},
		SandboxUIDBase: 1000, SandboxGIDBase: 1000,
	}
	uids, _ := m.MappedRanges()
	var total uint32
	for _, r := range uids {
		total += r.Count
	}
	if total > maxMappedIDs {
		t.Fatalf("mapped %d ids, want <= %d", total, maxMappedIDs)
	}
	if total == 0 {
		t.Fatal("should map something")
	}
}

// 区间不重叠、连续。
func TestSubIDMapRangesAreContiguous(t *testing.T) {
	m := SubIDMap{
		UID: 1000, GID: 1000,
		UIDs:           []SubID{{Start: 100000, Count: 500}, {Start: 200000, Count: 500}},
		GIDs:           []SubID{{Start: 100000, Count: 500}},
		SandboxUIDBase: 1000, SandboxGIDBase: 1000,
	}
	uids, _ := m.MappedRanges()
	if len(uids) != 2 {
		t.Fatalf("uids = %v", uids)
	}
	if uids[0].Start != 1000 || uids[0].Count != 500 {
		t.Fatalf("first = %+v", uids[0])
	}
	if uids[1].Start != 1500 {
		t.Fatalf("second must start right after the first: %+v", uids[1])
	}
	// 沙盒内不该出现 0（那是 agent 自己的 root，单独映射）。
	for _, r := range uids {
		if r.Start == 0 {
			t.Fatal("mapped range must not start at 0")
		}
	}
}

// 启动参数规格：只有多段模式才生成。
func TestSubIDSpec(t *testing.T) {
	m := SubIDMap{
		UID: 1000, GID: 1000,
		UIDs:           []SubID{{Start: 100000, Count: 1000}},
		GIDs:           []SubID{{Start: 100000, Count: 1000}},
		SandboxUIDBase: 1000, SandboxGIDBase: 1000,
	}
	spec := subIDSpec(m)
	if spec == "" {
		t.Fatal("no spec")
	}
	if !strings.Contains(spec, "uid=1000-1999") {
		t.Errorf("spec = %q", spec)
	}
	if !strings.Contains(spec, "gid=1000-1999") {
		t.Errorf("spec = %q", spec)
	}
	// 格式必须与 agent 的 parseUserMapping 一致：
	// `uid=lo-hi,lo-hi;gid=lo-hi`（agent 侧有对应的解析测试）。
	if !strings.HasPrefix(spec, "uid=") {
		t.Errorf("spec must start with uid=: %q", spec)
	}
	if !strings.Contains(spec, "uid=1000-1999;gid=") {
		t.Errorf("spec separators look wrong: %q", spec)
	}
}
