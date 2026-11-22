//go:build linux

package agent

import (
	"encoding/binary"
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// blockedPrefixes 是出网必须拦掉的地址段：
//   - 云元数据 169.254.0.0/16（含 169.254.169.254）
//   - 私网 10/8、172.16/12、192.168/16 与 CGNAT 100.64/10
//   - 回环 127/8（pasta 已用 --no-map-gw，这里再叠一层）
//   - IPv6 的 ULA fc00::/7、链路本地 fe80::/10、回环 ::1/128
//
// 不提供「允许私网」开关：需要沙盒访问宿主私网服务的用法必须先立项，
// 不能靠放开这份清单实现。
var blockedPrefixes = []string{
	"169.254.0.0/16",
	"100.64.0.0/10",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"127.0.0.0/8",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
}

// blockPrivateNetworks 给 blockedPrefixes 里的每一段加一条 blackhole 路由。
//
// 重复执行是安全的：已存在的路由按 EEXIST 处理为成功。地址族不可用
// （IPv6 未开启）时跳过该族，不算失败——那些地址本来就不可达。
func blockPrivateNetworks() error {
	for _, c := range blockedPrefixes {
		_, ipnet, err := net.ParseCIDR(c)
		if err != nil {
			return fmt.Errorf("agent: bad blocked prefix %q: %w", c, err)
		}
		if err := addBlackholeRoute(ipnet); err != nil {
			return err
		}
	}
	return nil
}

// addBlackholeRoute 用 netlink 往主路由表插入一条 blackhole 路由。
func addBlackholeRoute(ipnet *net.IPNet) error {
	ip := ipnet.IP
	family := uint8(unix.AF_INET)
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	} else if v6 := ip.To16(); v6 != nil {
		ip = v6
		family = unix.AF_INET6
	} else {
		return fmt.Errorf("agent: bad blocked address in %s", ipnet.String())
	}
	ones, _ := ipnet.Mask.Size()

	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		return fmt.Errorf("agent: netlink socket for %s: %w", ipnet.String(), err)
	}
	defer unix.Close(fd)

	// struct rtmsg：12 字节，字段顺序固定（见 rtnetlink(7)）。
	body := make([]byte, 12)
	body[0] = family                 // rtm_family
	body[1] = uint8(ones)            // rtm_dst_len
	body[4] = unix.RT_TABLE_MAIN     // rtm_table
	body[5] = unix.RTPROT_BOOT       // rtm_protocol
	body[6] = unix.RT_SCOPE_UNIVERSE // rtm_scope
	body[7] = unix.RTN_BLACKHOLE     // rtm_type

	// RTA_DST：4 字节属性头 + 地址。
	attr := make([]byte, 4+len(ip))
	binary.NativeEndian.PutUint16(attr[0:2], uint16(4+len(ip)))
	binary.NativeEndian.PutUint16(attr[2:4], unix.RTA_DST)
	copy(attr[4:], ip)
	body = append(body, attr...)
	if pad := len(body) % unix.NLMSG_ALIGNTO; pad != 0 {
		body = append(body, make([]byte, unix.NLMSG_ALIGNTO-pad)...)
	}

	// struct nlmsghdr：16 字节头 + body。
	req := make([]byte, 16+len(body))
	binary.NativeEndian.PutUint32(req[0:4], uint32(len(req)))
	binary.NativeEndian.PutUint16(req[4:6], unix.RTM_NEWROUTE)
	binary.NativeEndian.PutUint16(req[6:8],
		unix.NLM_F_REQUEST|unix.NLM_F_CREATE|unix.NLM_F_EXCL|unix.NLM_F_ACK)
	copy(req[16:], body)

	if err := unix.Sendto(fd, req, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return fmt.Errorf("agent: add blackhole %s: %w", ipnet.String(), err)
	}
	return readRouteAck(fd, ipnet)
}

// dropNetAdmin 丢掉 CAP_NET_ADMIN：拉起 lo 与加黑洞路由只在启动时做一次，
// 之后不需要这份能力，而它留在沙盒里等于允许用户命令改回网络配置。
//
// 只清 effective / permitted / inheritable，不动 bounding set：摘 bounding
// 需要 CAP_SETPCAP，而 bwrap 只加回了 CAP_NET_ADMIN。清空之后再 exec 也不会
// 自动填回来（没有 file capability 的情况下），所以效果足够。
func dropNetAdmin() error {
	hdr := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var data [2]unix.CapUserData
	if err := unix.Capset(&hdr, &data[0]); err != nil {
		return fmt.Errorf("agent: clear capabilities: %w", err)
	}
	return nil
}

// readRouteAck 读回 NLMSG_ERROR，把内核 errno 翻成 Go error。
func readRouteAck(fd int, ipnet *net.IPNet) error {
	buf := make([]byte, 4096)
	n, _, err := unix.Recvfrom(fd, buf, 0)
	if err != nil {
		return fmt.Errorf("agent: add blackhole %s: %w", ipnet.String(), err)
	}
	if n < 20 {
		return fmt.Errorf("agent: add blackhole %s: short netlink reply (%d bytes)", ipnet.String(), n)
	}
	// NLMSG_ERROR 的 payload 前 4 字节是 int32 errno（0 表示 ACK）。
	code := int32(binary.NativeEndian.Uint32(buf[16:20]))
	switch {
	case code == 0, code == -int32(unix.EEXIST):
		return nil
	case code == -int32(unix.EAFNOSUPPORT), code == -int32(unix.ESRCH), code == -int32(unix.ENETUNREACH):
		// 该地址族在这个 netns 里不可用：那些地址本来也到不了。
		return nil
	default:
		return fmt.Errorf("agent: add blackhole %s: %w", ipnet.String(), unix.Errno(-code))
	}
}
