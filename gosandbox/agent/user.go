// 文件说明：user.go — 把子进程切到指定 uid/gid
//
// 这一块之前是**空的**：协议里带 User 字段，但 agent 从没读过它，
// 所以「以 uid 999 运行命令」实际是「以 agent 的身份（沙盒里的 0）运行」。
// 对用户来说是静默的权限差异，比报错更危险。
//
// 为什么不能只 setuid：
//
//	沙盒里的 agent 是 userns 内的 root。这个 root 只对 userns 内映射到的
//	uid 有特权。bwrap 默认做单段映射（外部 uid ↔ 沙盒内 0），于是沙盒内
//	只有 uid 0 是「真的」，setuid(999) 会 EINVAL。
//
//	所以：
//	  - 沙盒内已经存在 uid 999 的文件（镜像里 999 拥有 /var/lib/postgres）
//	    → 单段映射下它被映射成别的值，无法 setuid 过去，只能报告不支持；
//	  - 宿主给了多段 subuid（newuidmap 可用）→ 沙盒内 999 是真实可用的，
//	    setuid 成功。
//
// 用法
//
//	attr, err := credentialFor("999:999", mapping)
//
// 调用架构
//
//	[上游] gosandbox/agent（proc.start）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/agent/user.go
//	         │
//	         ▼
//	[下游] os/exec、syscall、仅标准库
//	[同包] proc.go
//
// 主要符号
//
//	SetUserMapping(m *UserMapping)
//	UserMapping{…}
//	credentialFor(spec string) (*syscall.Credential, error)
//	ResolveID(spec string) (uid, gid uint32, err error)

package agent

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
)

// UserMapping 描述沙盒内哪些 uid/gid 是「真的」。
//
// 默认（零值）表示单段映射：沙盒内只有 uid 0 / gid 0 可用。
// 宿主通过 --uidmap 传进来时，agent 用它可以判断某个 uid 能不能切过去。
type UserMapping struct {
	// ExtraUIDs / ExtraGIDs 是除 0 之外、沙盒内可用的 id 区间。
	//
	// 只存起点与长度，够用来判断「这个 id 在不在里面」。
	ExtraUIDs []IDRange
	ExtraGIDs []IDRange
	// MultiSegment 为真表示宿主做了多段映射（bwrap --uidmap 多次）。
	MultiSegment bool
}

// IDRange 是一个 id 区间。
type IDRange struct {
	Start uint32
	Count uint32
}

// contains 报告 id 是否落在区间里。
func (r IDRange) contains(id uint32) bool {
	return id >= r.Start && id < r.Start+r.Count
}

// mapping 是当前生效的映射；零值即单段。
var mapping atomic.Pointer[UserMapping]

// SetUserMapping 设置当前映射（agent 启动时由宿主传入）。
func SetUserMapping(m *UserMapping) {
	if m == nil {
		mapping.Store(&UserMapping{})
		return
	}
	mapping.Store(m)
}

// UserMappingNow 返回当前映射。
func UserMappingNow() *UserMapping {
	if m := mapping.Load(); m != nil {
		return m
	}
	return &UserMapping{}
}

// CanSwitchTo 报告沙盒内是否真的能切到某个 uid。
//
// 单段映射下只有 0 可以；多段映射下看区间。
func (m *UserMapping) CanSwitchTo(uid uint32) bool {
	if uid == 0 {
		return true
	}
	for _, r := range m.ExtraUIDs {
		if r.contains(uid) {
			return true
		}
	}
	return false
}

// CanSwitchToGID 报告是否真的能切到某个 gid。
func (m *UserMapping) CanSwitchToGID(gid uint32) bool {
	if gid == 0 {
		return true
	}
	for _, r := range m.ExtraGIDs {
		if r.contains(gid) {
			return true
		}
	}
	return false
}

// ErrUserNotMapped 表示请求的 uid/gid 在沙盒里无法映射。
//
// 与「切过去失败」区分开：前者是配置问题（宿主没给 subuid），
// 后者是运行时问题（setuid 被内核拒绝）。上层的处理不同：
// 前者应该拒绝启动并告诉用户怎么配，后者可以直接报错。
var ErrUserNotMapped = fmt.Errorf("agent: the requested uid/gid is not mapped inside the sandbox")

// ResolveID 解析 "uid:gid" / "uid" / 空。
//
// 也接受用户名：此时按沙盒内的 /etc/passwd 查。只支持数字与用户名，
// 不做 chown 之类的语义推导——那属于镜像的事。
func ResolveID(spec string) (uid, gid uint32, err error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, 0, nil
	}
	userPart, groupPart, hasGroup := strings.Cut(spec, ":")
	uid, err = lookupID(userPart, true)
	if err != nil {
		return 0, 0, err
	}
	if !hasGroup || groupPart == "" {
		// 未指定 gid：按用户名查它的主组；纯数字时取同名 gid。
		if gid, err = primaryGID(userPart, uid); err != nil {
			return 0, 0, err
		}
		return uid, gid, nil
	}
	gid, err = lookupID(groupPart, false)
	if err != nil {
		return 0, 0, err
	}
	return uid, gid, nil
}

// lookupID 把 "999" 或 "postgres" 变成数字 id。
func lookupID(s string, isUser bool) (uint32, error) {
	if s == "" {
		return 0, nil
	}
	if n, err := strconv.ParseUint(s, 10, 32); err == nil {
		return uint32(n), nil
	}
	// 名字：查 /etc/passwd 或 /etc/group。
	if isUser {
		return lookupPasswd(s)
	}
	return lookupGroup(s)
}

// lookupPasswd 从 /etc/passwd 查 uid 与主 gid。
func lookupPasswd(name string) (uint32, error) {
	body, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return 0, fmt.Errorf("agent: cannot resolve user %q: %w", name, err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		f := strings.Split(line, ":")
		if len(f) < 4 || f[0] != name {
			continue
		}
		n, perr := strconv.ParseUint(f[2], 10, 32)
		if perr != nil {
			return 0, fmt.Errorf("agent: bad uid for %q", name)
		}
		return uint32(n), nil
	}
	return 0, fmt.Errorf("agent: no such user %q in the sandbox", name)
}

// lookupGroup 从 /etc/group 查 gid。
func lookupGroup(name string) (uint32, error) {
	body, err := os.ReadFile("/etc/group")
	if err != nil {
		return 0, fmt.Errorf("agent: cannot resolve group %q: %w", name, err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		f := strings.Split(line, ":")
		if len(f) < 3 || f[0] != name {
			continue
		}
		n, perr := strconv.ParseUint(f[2], 10, 32)
		if perr != nil {
			return 0, fmt.Errorf("agent: bad gid for %q", name)
		}
		return uint32(n), nil
	}
	return 0, fmt.Errorf("agent: no such group %q in the sandbox", name)
}

// primaryGID 取一个用户的主组。
//
// 纯数字 uid 没有「主组」概念：取与 uid 相同的 gid（Docker 的 user:group
// 默认行为，也是最不容易出错的选择）。
func primaryGID(userPart string, uid uint32) (uint32, error) {
	if _, err := strconv.ParseUint(userPart, 10, 32); err == nil {
		return uid, nil
	}
	body, rerr := os.ReadFile("/etc/passwd")
	if rerr != nil {
		return 0, fmt.Errorf("agent: cannot resolve primary group for %q: %w", userPart, rerr)
	}
	for _, line := range strings.Split(string(body), "\n") {
		f := strings.Split(line, ":")
		if len(f) < 4 || f[0] != userPart {
			continue
		}
		n, perr := strconv.ParseUint(f[3], 10, 32)
		if perr != nil {
			return 0, fmt.Errorf("agent: bad gid for %q", userPart)
		}
		return uint32(n), nil
	}
	return uid, nil
}

// credentialFor 把 "uid:gid" 解析成 syscall.Credential。
//
// 空 spec 返回 nil：表示不切用户（继承 agent）。
// 请求了切用户但沙盒内没有对应映射时返回 ErrUserNotMapped——
// 这比「静默按 0 跑」安全得多。
func credentialFor(spec string) (*syscall.Credential, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, nil
	}
	uid, gid, err := ResolveID(spec)
	if err != nil {
		return nil, err
	}
	m := UserMappingNow()
	if !m.CanSwitchTo(uid) || !m.CanSwitchToGID(gid) {
		return nil, fmt.Errorf("%w: uid=%d gid=%d (the sandbox was created with a single uid mapping; "+
			"the host needs /etc/subuid and newuidmap for multi-segment mapping)", ErrUserNotMapped, uid, gid)
	}
	// NoSetGroups：沙盒内没有权限调 setgroups，指定它会 EPERM。
	return &syscall.Credential{Uid: uid, Gid: gid, NoSetGroups: true}, nil
}

// parseUserMapping 解析宿主传来的映射规格：`uid=1000-1999,2000-2999;gid=...`
//
// 解析失败不影响 agent 启动：退回单段映射（只有 0 可用），
// 这样「映射参数写错了」不会让沙盒完全起不来，只是多用户镜像受限。
func parseUserMapping(spec string) (*UserMapping, error) {
	m := &UserMapping{}
	for _, part := range strings.Split(spec, ";") {
		if part == "" {
			continue
		}
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("agent: bad --uidmap segment %q", part)
		}
		ranges, err := parseRanges(val)
		if err != nil {
			return nil, err
		}
		switch key {
		case "uid":
			m.ExtraUIDs = ranges
		case "gid":
			m.ExtraGIDs = ranges
		default:
			return nil, fmt.Errorf("agent: unknown --uidmap key %q", key)
		}
	}
	m.MultiSegment = len(m.ExtraUIDs) > 0
	return m, nil
}

// parseRanges 解析 `1000-1999,2000-2999`。
func parseRanges(s string) ([]IDRange, error) {
	var out []IDRange
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		lo, hi, ok := strings.Cut(item, "-")
		if !ok {
			return nil, fmt.Errorf("agent: bad range %q", item)
		}
		start, serr := strconv.ParseUint(lo, 10, 32)
		end, eerr := strconv.ParseUint(hi, 10, 32)
		if serr != nil || eerr != nil || end < start {
			return nil, fmt.Errorf("agent: bad range %q", item)
		}
		out = append(out, IDRange{Start: uint32(start), Count: uint32(end-start) + 1})
	}
	return out, nil
}
