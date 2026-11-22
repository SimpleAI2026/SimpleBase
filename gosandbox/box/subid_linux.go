//go:build linux

package box

import (
	"bufio"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// SubID 是一段 id 区间。
type SubID struct {
	Start uint32
	Count uint32
}

// SubIDMap 是多段映射所需的宿主信息。
type SubIDMap struct {
	// UID / GID 是宿主侧的真实 id。
	UID uint32
	GID uint32
	// UIDs / GIDs 是 /etc/subuid、/etc/subgid 给本用户的配额。
	UIDs []SubID
	GIDs []SubID
	// SandboxUIDBase / SandboxGIDBase 是沙盒内映射区间的起点。
	SandboxUIDBase uint32
	SandboxGIDBase uint32
	// Newuidmap / Newgidmap 是可执行文件路径。
	Newuidmap string
	Newgidmap string
	// Reason 是「为什么退到单段映射」；多段可用时为空。
	Reason string
}

// MultiSegment 报告能否做多段映射。
func (m SubIDMap) MultiSegment() bool { return m.Reason == "" }

// newuidmapCandidates 是 newuidmap 的常见安装位置。
var newuidmapCandidates = []string{"/usr/bin/newuidmap", "/usr/sbin/newuidmap", "/bin/newuidmap"}

// subidOnce 缓存探测结果：subuid 在进程生命周期里不会变。
var (
	subidOnce sync.Once
	subidMap  SubIDMap
)

// ProbeSubIDs 探测多段映射能力（结果缓存）。
func ProbeSubIDs() SubIDMap {
	subidOnce.Do(func() { subidMap = probeSubIDs() })
	return subidMap
}

// probeSubIDs 做一次真实探测。
func probeSubIDs() SubIDMap {
	m := SubIDMap{SandboxUIDBase: 1000, SandboxGIDBase: 1000}
	u, err := user.Current()
	if err != nil {
		m.Reason = "cannot determine the current user: " + err.Error()
		return m
	}
	uid, uerr := strconv.ParseUint(u.Uid, 10, 32)
	gid, gerr := strconv.ParseUint(u.Gid, 10, 32)
	if uerr != nil || gerr != nil {
		m.Reason = "current user has a non-numeric uid/gid"
		return m
	}
	// id 0 是宿主 root：那种情况下多段映射没有意义（本来就是全权）。
	if uid == 0 {
		m.Reason = "running as root; multi-segment mapping is only meaningful for rootless users"
		return m
	}
	m.UID, m.GID = uint32(uid), uint32(gid)

	m.Newuidmap = firstExisting(newuidmapCandidates)
	if m.Newuidmap != "" {
		m.Newgidmap = firstExisting([]string{
			filepath.Join(filepath.Dir(m.Newuidmap), "newgidmap"),
			"/usr/bin/newgidmap", "/usr/sbin/newgidmap", "/bin/newgidmap",
		})
	}
	if m.Newuidmap == "" || m.Newgidmap == "" {
		m.Reason = "newuidmap/newgidmap not found (install the uidmap package)"
		return m
	}
	m.UIDs = readSubID("/etc/subuid", u.Username, u.Uid)
	m.GIDs = readSubID("/etc/subgid", u.Username, u.Uid)
	if len(m.UIDs) == 0 {
		m.Reason = "/etc/subuid has no range for " + u.Username +
			" (run: usermod --add-subuids 100000-165535 --add-subgids 100000-165535 " + u.Username + ")"
		return m
	}
	if len(m.GIDs) == 0 {
		m.Reason = "/etc/subgid has no range for " + u.Username +
			" (run: usermod --add-subgids 100000-165535 " + u.Username + ")"
		return m
	}
	return m
}

// firstExisting 返回第一个存在的文件路径。
func firstExisting(paths []string) string {
	for _, p := range paths {
		if p == "" {
			continue
		}
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

// readSubID 从 /etc/subuid 或 /etc/subgid 读一个用户的区间。
//
// 匹配 name 或 uid：两种写法都存在（useradd 写名字，部分工具写数字）。
func readSubID(path, name, uid string) []SubID {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []SubID
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// 格式：name:start:count
		fields := strings.Split(line, ":")
		if len(fields) < 3 {
			continue
		}
		if fields[0] != name && fields[0] != uid {
			continue
		}
		start, serr := strconv.ParseUint(fields[1], 10, 32)
		count, cerr := strconv.ParseUint(fields[2], 10, 32)
		if serr != nil || cerr != nil || count == 0 {
			continue
		}
		out = append(out, SubID{Start: uint32(start), Count: uint32(count)})
	}
	return out
}

// maxMappedIDs 是沙盒内可映射的 id 总数上限。
//
// 不把整段 subuid（常见 65536）全映射进去：映射表在内核里是红黑树，
// 六万多条会让每次 uid 查找变慢，bwrap 启动也多几十毫秒。
// 4096 足够覆盖「镜像里几十个服务用户」，且远小于常见配额。
const maxMappedIDs = 4096

// plan 是最终的映射计划：每段是 (沙盒内起点, 宿主起点, 长度)。
type plan struct {
	sandbox uint32
	host    uint32
	count   uint32
}

// buildPlan 把 subuid 配额折成映射计划。
//
// 第一段固定是沙盒内 0 → 宿主 uid（长度 1）：沙盒里的 root 必须可用，
// agent 自己就是它。之后从 SandboxUIDBase 开始顺序铺开，总长度截到
// maxMappedIDs。
//
// 不按宿主 id 做任何过滤：映射是「区间到区间」的搬运，宿主区间的具体
// 数值不影响内层可用性；只有内层区间必须避开 0 与系统用户段。
func buildPlan(hostID, sandboxBase uint32, ranges []SubID) []plan {
	out := []plan{{sandbox: 0, host: hostID, count: 1}}
	nextSandbox := sandboxBase
	left := uint32(maxMappedIDs)
	for _, r := range ranges {
		if left == 0 {
			break
		}
		n := r.Count
		if n > left {
			n = left
		}
		out = append(out, plan{sandbox: nextSandbox, host: r.Start, count: n})
		nextSandbox += n
		left -= n
	}
	return out
}

// MappedRanges 返回沙盒内实际可用的 uid/gid 区间（不含 0）。
//
// agent 用它判断「能不能切到某个 uid」：区间外的 id 在内层根本不存在。
func (m SubIDMap) MappedRanges() (uids, gids []SubID) {
	if !m.MultiSegment() {
		return nil, nil
	}
	for _, p := range buildPlan(m.UID, m.SandboxUIDBase, m.UIDs) {
		if p.sandbox == 0 {
			continue
		}
		uids = append(uids, SubID{Start: p.sandbox, Count: p.count})
	}
	for _, p := range buildPlan(m.GID, m.SandboxGIDBase, m.GIDs) {
		if p.sandbox == 0 {
			continue
		}
		gids = append(gids, SubID{Start: p.sandbox, Count: p.count})
	}
	return uids, gids
}

// IDMapArgs 返回完整的 bwrap id 映射参数；单段模式返回 nil。
func (m SubIDMap) IDMapArgs() []string {
	if !m.MultiSegment() {
		return nil
	}
	var out []string
	for _, p := range buildPlan(m.UID, m.SandboxUIDBase, m.UIDs) {
		out = append(out, "--uidmap", mapArg(p))
	}
	for _, p := range buildPlan(m.GID, m.SandboxGIDBase, m.GIDs) {
		out = append(out, "--gidmap", mapArg(p))
	}
	return out
}

// mapArg 拼一个 `sandbox:host:count` 参数。
func mapArg(p plan) string {
	return strconv.FormatUint(uint64(p.sandbox), 10) + ":" +
		strconv.FormatUint(uint64(p.host), 10) + ":" +
		strconv.FormatUint(uint64(p.count), 10)
}

// subIDSpec 把映射区间编成一个紧凑的启动参数。
//
// 格式：`uid=1000-1999,2000-2999;gid=1000-1999`。用区间而不是列出每个 id：
// 4096 个数字会让 argv 超出合理长度，而 agent 只需要判断「在不在区间里」。
func subIDSpec(m SubIDMap) string {
	if !m.MultiSegment() {
		return ""
	}
	uids, gids := m.MappedRanges()
	if len(uids) == 0 && len(gids) == 0 {
		return ""
	}
	return "uid=" + rangeSpec(uids) + ";gid=" + rangeSpec(gids)
}

// rangeSpec 拼一段区间列表。
func rangeSpec(rs []SubID) string {
	var sb strings.Builder
	for i, r := range rs {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatUint(uint64(r.Start), 10))
		sb.WriteByte('-')
		sb.WriteString(strconv.FormatUint(uint64(r.Start+r.Count-1), 10))
	}
	return sb.String()
}
