// 文件说明：user_test.go — gosandbox/agent 的单元测试（TestResolveIDNumeric、TestResolveIDEmpty 等）
//
// 用法
//
//	go test ./gosandbox/agent -run 'TestResolveIDNumeric|TestResolveIDEmpty|TestResolveIDInvalid|TestUserMappingSingleSegment'
//
// 调用架构
//
//	[上游] gosandbox/cmd
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/agent/user_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] agent.go, agent_test.go, fs.go, loopback_linux.go, loopback_other.go 等 16 个
//
// 主要符号
//
//	TestResolveIDNumeric(t *testing.T)
//	TestResolveIDEmpty(t *testing.T)
//	TestResolveIDInvalid(t *testing.T)
//	TestUserMappingSingleSegment(t *testing.T)
//	TestUserMappingMultiSegment(t *testing.T)
//	TestIDRangeBoundaries(t *testing.T)
//	TestCredentialFor(t *testing.T)
//	TestParseUserMapping(t *testing.T)
//	... 另有 5 个顶层符号

package agent

// user_test.go：uid/gid 解析与多段映射判定。
//
// 这一块的错误代价很高：解析错了会把命令跑成别的用户，判定错了会在
// 用户以为降权成功的时候仍以 0 运行。所以两侧都要钉住。

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// 纯数字 uid：gid 默认取同一个值（Docker 的 user:group 语义）。
func TestResolveIDNumeric(t *testing.T) {
	uid, gid, err := ResolveID("999:999")
	if err != nil {
		t.Fatalf("999:999: %v", err)
	}
	if uid != 999 || gid != 999 {
		t.Fatalf("got %d:%d", uid, gid)
	}

	uid, gid, err = ResolveID("1000")
	if err != nil {
		t.Fatalf("1000: %v", err)
	}
	if uid != 1000 || gid != 1000 {
		t.Fatalf("got %d:%d, want 1000:1000", uid, gid)
	}
}

// 空 spec：不切用户。
func TestResolveIDEmpty(t *testing.T) {
	for _, spec := range []string{"", "  "} {
		uid, gid, err := ResolveID(spec)
		if err != nil || uid != 0 || gid != 0 {
			t.Errorf("%q: got %d:%d %v", spec, uid, gid, err)
		}
	}
}

// 非法输入报错。
func TestResolveIDInvalid(t *testing.T) {
	for _, spec := range []string{"abc:def", "999:def", "-1"} {
		if _, _, err := ResolveID(spec); err == nil {
			t.Errorf("%q must fail", spec)
		}
	}
}

// 单段映射：只有 0 可切。
func TestUserMappingSingleSegment(t *testing.T) {
	m := &UserMapping{}
	if !m.CanSwitchTo(0) || !m.CanSwitchToGID(0) {
		t.Fatal("0 must always be switchable")
	}
	for _, uid := range []uint32{1, 999, 1000} {
		if m.CanSwitchTo(uid) {
			t.Errorf("uid %d must not be switchable in single-segment mode", uid)
		}
	}
}

// 多段映射：区间内可切，区间外不可。
func TestUserMappingMultiSegment(t *testing.T) {
	m := &UserMapping{
		ExtraUIDs:    []IDRange{{Start: 1000, Count: 1000}, {Start: 2000, Count: 500}},
		ExtraGIDs:    []IDRange{{Start: 1000, Count: 1000}},
		MultiSegment: true,
	}
	for _, uid := range []uint32{1000, 1500, 1999, 2000, 2499} {
		if !m.CanSwitchTo(uid) {
			t.Errorf("uid %d should be switchable", uid)
		}
	}
	// 区间之间的空隙与两端之外都不可切。
	for _, uid := range []uint32{999, 2500, 3000} {
		if m.CanSwitchTo(uid) {
			t.Errorf("uid %d should NOT be switchable", uid)
		}
	}
	// gid 区间独立。
	if !m.CanSwitchToGID(1500) {
		t.Error("gid 1500 should be switchable")
	}
	if m.CanSwitchToGID(2000) {
		t.Error("gid 2000 is outside the gid range")
	}
}

// 区间边界：start 含、start+count 不含。
func TestIDRangeBoundaries(t *testing.T) {
	r := IDRange{Start: 1000, Count: 10}
	if !r.contains(1000) || !r.contains(1009) {
		t.Fatal("inclusive start and last element must be inside")
	}
	if r.contains(999) || r.contains(1010) {
		t.Fatal("outside values must be rejected")
	}
}

// credentialFor：空 spec 返回 nil；无法映射的 uid 返回 ErrUserNotMapped。
func TestCredentialFor(t *testing.T) {
	SetUserMapping(nil)
	c, err := credentialFor("")
	if err != nil || c != nil {
		t.Fatalf("empty spec: %v %v", c, err)
	}

	// 单段映射下请求 999：必须报 ErrUserNotMapped，而不是静默跑成 0。
	_, err = credentialFor("999:999")
	if !errors.Is(err, ErrUserNotMapped) {
		t.Fatalf("err = %v, want ErrUserNotMapped", err)
	}
	// 报错信息里要能看出「怎么办」。
	if err != nil && !contains(err.Error(), "subuid") {
		t.Errorf("error should mention subuid: %v", err)
	}

	// 显式 0:0 在单段映射下可用。
	c, err = credentialFor("0:0")
	if err != nil || c == nil || c.Uid != 0 || c.Gid != 0 {
		t.Fatalf("0:0: %v %v", c, err)
	}

	// 多段映射下 999 可用。
	SetUserMapping(&UserMapping{
		ExtraUIDs:    []IDRange{{Start: 1000, Count: 2000}},
		ExtraGIDs:    []IDRange{{Start: 1000, Count: 2000}},
		MultiSegment: true,
	})
	c, err = credentialFor("1500:1500")
	if err != nil {
		t.Fatalf("1500:1500: %v", err)
	}
	if c.Uid != 1500 || c.Gid != 1500 {
		t.Fatalf("got %d:%d", c.Uid, c.Gid)
	}
	// NoSetGroups 必须为真：沙盒内没有 setgroups 权限，指定它会 EPERM。
	if !c.NoSetGroups {
		t.Error("NoSetGroups must be set")
	}
	SetUserMapping(nil)
}

// 解析宿主传来的映射规格。
func TestParseUserMapping(t *testing.T) {
	m, err := parseUserMapping("uid=1000-1999,2000-2999;gid=1000-1999")
	if err != nil {
		t.Fatal(err)
	}
	if !m.MultiSegment {
		t.Fatal("must be multi-segment")
	}
	if len(m.ExtraUIDs) != 2 || len(m.ExtraGIDs) != 1 {
		t.Fatalf("uids=%v gids=%v", m.ExtraUIDs, m.ExtraGIDs)
	}
	if m.ExtraUIDs[0].Start != 1000 || m.ExtraUIDs[0].Count != 1000 {
		t.Fatalf("range = %+v", m.ExtraUIDs[0])
	}
	if m.ExtraUIDs[1].Start != 2000 || m.ExtraUIDs[1].Count != 1000 {
		t.Fatalf("range = %+v", m.ExtraUIDs[1])
	}
	if !m.CanSwitchTo(2500) {
		t.Error("2500 should be switchable")
	}
}

// 畸形映射规格报错。
func TestParseUserMappingInvalid(t *testing.T) {
	for _, spec := range []string{"uid=1000", "uid=a-b", "uid=2000-1000", "other=1-2", "noequals"} {
		if _, err := parseUserMapping(spec); err == nil {
			t.Errorf("%q must fail", spec)
		}
	}
}

// contains 是 strings.Contains 的短名（避免为一个断言引入 strings 到这个文件）。
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

const passwdFixture = `root:x:0:0:root:/root:/bin/sh
postgres:x:1001:1001::/var/lib/postgresql:/bin/sh
short:x
broken:x:notanumber:1001::/bin/sh
nogid:x:1002:notanumber::/bin/sh
`

func TestColonField(t *testing.T) {
	cases := []struct {
		want      string
		name      string
		col       int
		minFields int
		id        uint32
		st        colonLookup
	}{
		{"root 的 uid", "root", 2, 4, 0, colOK},
		{"普通用户的 uid", "postgres", 2, 4, 1001, colOK},
		{"第 3 列是主组 gid", "postgres", 3, 4, 1001, colOK},
		{"字段数不足的行按不存在处理", "short", 2, 4, 0, colMissing},
		{"uid 列不是数字", "broken", 2, 4, 0, colBad},
		{"主组列不是数字", "nogid", 3, 4, 0, colBad},
		{"没有这个用户", "nobody-here", 2, 4, 0, colMissing},
	}
	for _, tc := range cases {
		id, st := colonField(passwdFixture, tc.name, tc.minFields, tc.col)
		if id != tc.id || st != tc.st {
			t.Errorf("%s: colonField = (%d, %d), want (%d, %d)", tc.want, id, st, tc.id, tc.st)
		}
	}
}

func TestLookupGroupReadsEtcGroup(t *testing.T) {
	// 依赖运行环境的 /etc/group：大多数镜像与开发机都有 root:0；没有 root 组的环境跳过。
	body, err := os.ReadFile("/etc/group")
	if err != nil {
		t.Skip("/etc/group not readable")
	}
	if _, st := colonField(string(body), "root", 3, 2); st == colMissing {
		t.Skip("no root group in /etc/group")
	}
	gid, err := lookupGroup("root")
	if err != nil || gid != 0 {
		t.Fatalf("lookupGroup(root) = %d, %v; want 0, nil", gid, err)
	}
	if _, err := lookupGroup("no-such-group-" + "planv6.8"); err == nil || !strings.Contains(err.Error(), "no such group") {
		t.Fatalf("missing group: err = %v, want no-such-group error", err)
	}
}
