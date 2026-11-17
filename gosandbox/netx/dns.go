// 文件说明：dns.go — 白名单模式下的最小 DNS 代理
//
// 状态：已实现并有单测，但当前没有任何生产代码启动它（agent 不起 DNS，
// netx.StartOptions 也不带白名单），egress-allowlist 档因此不会按域名限制出站。
//
// 沙盒内的进程把 DNS 指向这个代理（由 agent 在 netns 里监听 127.0.0.1:53）。
// 代理只解析白名单里的域名，把上游的应答原样返回；不在白名单的查询直接
// 回 NXDOMAIN。
//
// 为什么在沙盒内而不是宿主：只有沙盒自己知道它要解析什么；
// 放到宿主就得为每个沙盒维护一份「谁在解析什么」的状态，还要
// 在控制通道上多开一条流量路径。
//
// 协议处理刻意保守：
//   - 只支持单问题查询（QDCOUNT=1），这是所有实际客户端的做法；
//   - 只透传 A / AAAA / CNAME / HTTPS 之外的记录也不拦——拦截按
//     「域名是否白名单」做，不按记录类型做；
//   - 应答里的 IP 不再二次校验（白名单语义是「这个域名可信」），
//     但私网地址仍会被 CheckAllowed 拦住——这里只做域名层。
//
// 用法
//
//	srv, err := netx.NewDNSProxy(netx.DNSPolicy{Allow: []string{"pypi.org", "*.npmjs.org"}})
//	srv.ListenAndServe(ctx, "127.0.0.1:53")
//
// 调用架构
//
//	[上游] gosandbox/agent（启动时按 sandbox.network 决定是否起）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/netx/dns.go
//	         │
//	         ▼
//	[下游] 仅标准库
//	[同包] netx.go
//
// 主要符号
//
//	type DNSPolicy struct {…}
//	type DNSProxy struct {…}
//	NewDNSProxy(p DNSPolicy) (*DNSProxy, error)
//	(p *DNSProxy) ListenAndServe(ctx context.Context, addr string) error
//	(p *DNSProxy) Resolve(ctx context.Context, name string) ([]netip.Addr, error)

package netx

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// DNSPolicy 是 DNS 代理的策略。
type DNSPolicy struct {
	// Allow 是白名单（规则同 Policy.Allow）。
	Allow []string
	// Upstream 是上游 DNS 地址；空取 1.1.1.1:53 与 8.8.8.8:53（依次尝试）。
	Upstream []string
	// MaxTTL 是应答缓存上限；0 取 60 秒。
	//
	// 缓存不是为了省上游查询，而是为了让白名单里的域名在 TTL 内
	// 解析结果稳定：调用方会把它写进 nft set，结果跳动会让连接时通时不通。
	MaxTTL time.Duration
}

// 默认上游。
var defaultUpstream = []string{"1.1.1.1:53", "8.8.8.8:53"}

// DNSProxy 是最小白名单 DNS 代理。
type DNSProxy struct {
	policy DNSPolicy

	mu    sync.Mutex
	cache map[string]dnsCacheEntry
}

// dnsCacheEntry 是一条缓存。
type dnsCacheEntry struct {
	addrs  []netip.Addr
	expire time.Time
}

// NewDNSProxy 建一个代理。
func NewDNSProxy(p DNSPolicy) (*DNSProxy, error) {
	if len(p.Allow) == 0 {
		return nil, errors.New("netx: DNS allowlist is empty; every lookup would fail")
	}
	if len(p.Upstream) == 0 {
		p.Upstream = defaultUpstream
	}
	if p.MaxTTL <= 0 {
		p.MaxTTL = 60 * time.Second
	}
	return &DNSProxy{policy: p, cache: map[string]dnsCacheEntry{}}, nil
}

// ListenAndServe 在 addr 上服务 DNS（通常是 127.0.0.1:53）。
//
// ctx 取消时关闭监听。UDP 与 TCP 都监听：glibc 在应答被截断时会
// 重试 TCP，只开 UDP 会让大应答（例如带一堆 AAAA 的域名）解析失败。
func (p *DNSProxy) ListenAndServe(ctx context.Context, addr string) error {
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return err
	}
	ln, lerr := net.Listen("tcp", addr)
	if lerr != nil {
		// TCP 起不来不致命：UDP 能覆盖绝大多数查询。
		ln = nil
	}
	go func() {
		<-ctx.Done()
		_ = pc.Close()
		if ln != nil {
			_ = ln.Close()
		}
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.serveUDP(pc)
	}()
	if ln != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.serveTCP(ln)
		}()
	}
	wg.Wait()
	return nil
}

// serveUDP 处理 UDP 查询。
func (p *DNSProxy) serveUDP(pc net.PacketConn) {
	buf := make([]byte, 4096)
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		q := make([]byte, n)
		copy(q, buf[:n])
		go func() {
			resp := p.handle(q)
			if len(resp) > 0 {
				_, _ = pc.WriteTo(resp, from)
			}
		}()
	}
}

// serveTCP 处理 TCP 查询（带 2 字节长度前缀）。
func (p *DNSProxy) serveTCP(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(conn net.Conn) {
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
			for {
				var hdr [2]byte
				if _, rerr := io.ReadFull(conn, hdr[:]); rerr != nil {
					return
				}
				size := int(binary.BigEndian.Uint16(hdr[:]))
				if size <= 0 || size > 65535 {
					return
				}
				msg := make([]byte, size)
				if _, rerr := io.ReadFull(conn, msg); rerr != nil {
					return
				}
				resp := p.handle(msg)
				if len(resp) == 0 {
					return
				}
				out := make([]byte, 2+len(resp))
				binary.BigEndian.PutUint16(out, uint16(len(resp)))
				copy(out[2:], resp)
				if _, werr := conn.Write(out); werr != nil {
					return
				}
			}
		}(c)
	}
}

// handle 处理一条查询：白名单内转发上游，否则回 NXDOMAIN。
func (p *DNSProxy) handle(query []byte) []byte {
	name, _, ok := parseQuestion(query)
	if !ok {
		return nil // 畸形包：不回，让客户端自己超时
	}
	if !HostAllowed(name, Policy{Allow: p.policy.Allow}) {
		return nxdomain(query)
	}
	if resp := p.fromCache(query, name); resp != nil {
		return resp
	}
	resp, addrs := p.forward(query)
	if len(resp) > 0 && len(addrs) > 0 {
		p.store(name, addrs)
	}
	return resp
}

// fromCache 用缓存拼一个应答；没有缓存返回 nil。
func (p *DNSProxy) fromCache(query []byte, name string) []byte {
	p.mu.Lock()
	e, ok := p.cache[name]
	p.mu.Unlock()
	if !ok || time.Now().After(e.expire) {
		return nil
	}
	return buildAnswer(query, e.addrs)
}

// store 存一条缓存。
func (p *DNSProxy) store(name string, addrs []netip.Addr) {
	p.mu.Lock()
	p.cache[name] = dnsCacheEntry{addrs: addrs, expire: time.Now().Add(p.policy.MaxTTL)}
	p.mu.Unlock()
}

// forward 把查询转给上游，返回原始应答与解析出的地址。
//
// 上游依次尝试：第一个失败就换下一个。全部失败返回空——
// 调用方回 SERVFAIL 比回 NXDOMAIN 更诚实（后者会让客户端以为域名不存在）。
func (p *DNSProxy) forward(query []byte) ([]byte, []netip.Addr) {
	for _, up := range p.policy.Upstream {
		conn, err := net.DialTimeout("udp", up, 3*time.Second)
		if err != nil {
			continue
		}
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		if _, werr := conn.Write(query); werr != nil {
			conn.Close()
			continue
		}
		buf := make([]byte, 4096)
		n, rerr := conn.Read(buf)
		conn.Close()
		if rerr != nil || n < 12 {
			continue
		}
		resp := make([]byte, n)
		copy(resp, buf[:n])
		return resp, answerAddrs(resp)
	}
	return nil, nil
}

// Resolve 解析一个名字（供调用方把结果写进 nft set）。
func (p *DNSProxy) Resolve(ctx context.Context, name string) ([]netip.Addr, error) {
	if !HostAllowed(name, Policy{Allow: p.policy.Allow}) {
		return nil, fmt.Errorf("%w: %s", ErrNotAllowed, name)
	}
	p.mu.Lock()
	e, ok := p.cache[name]
	p.mu.Unlock()
	if ok && time.Now().Before(e.expire) {
		return e.addrs, nil
	}
	q := buildQuery(name, 1) // A
	if q == nil {
		return nil, fmt.Errorf("netx: cannot build a query for %q", name)
	}
	type result struct {
		addrs []netip.Addr
	}
	ch := make(chan result, 1)
	go func() {
		_, addrs := p.forward(q)
		ch <- result{addrs}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if len(r.addrs) == 0 {
			return nil, fmt.Errorf("netx: no address for %s", name)
		}
		p.store(name, r.addrs)
		return r.addrs, nil
	}
}

// parseQuestion 解析查询里的第一个问题。
func parseQuestion(msg []byte) (name string, qtype uint16, ok bool) {
	if len(msg) < 12 {
		return "", 0, false
	}
	qd := binary.BigEndian.Uint16(msg[4:6])
	if qd != 1 {
		return "", 0, false
	}
	off := 12
	var sb strings.Builder
	for {
		if off >= len(msg) {
			return "", 0, false
		}
		l := int(msg[off])
		off++
		if l == 0 {
			break
		}
		// 压缩指针：问题段不该出现，出现就拒绝（防止解析循环）。
		if l&0xC0 != 0 {
			return "", 0, false
		}
		if off+l > len(msg) {
			return "", 0, false
		}
		if sb.Len() > 0 {
			sb.WriteByte('.')
		}
		sb.Write(msg[off : off+l])
		off += l
	}
	if off+4 > len(msg) {
		return "", 0, false
	}
	return sb.String(), binary.BigEndian.Uint16(msg[off : off+2]), true
}

// answerAddrs 从应答里提取 A / AAAA 地址。
func answerAddrs(msg []byte) []netip.Addr {
	if len(msg) < 12 {
		return nil
	}
	an := binary.BigEndian.Uint16(msg[6:8])
	if an == 0 {
		return nil
	}
	// 跳过问题段。
	off := 12
	for {
		if off >= len(msg) {
			return nil
		}
		l := int(msg[off])
		off++
		if l == 0 {
			break
		}
		if l&0xC0 != 0 {
			off++ // 压缩指针占 2 字节，已读 1 字节
			break
		}
		off += l
	}
	off += 4 // QTYPE + QCLASS
	var out []netip.Addr
	for i := 0; i < int(an); i++ {
		off = skipName(msg, off)
		if off < 0 || off+10 > len(msg) {
			return out
		}
		typ := binary.BigEndian.Uint16(msg[off : off+2])
		rdlen := int(binary.BigEndian.Uint16(msg[off+8 : off+10]))
		off += 10
		if off+rdlen > len(msg) {
			return out
		}
		switch typ {
		case 1: // A
			if rdlen == 4 {
				var b [4]byte
				copy(b[:], msg[off:off+4])
				out = append(out, netip.AddrFrom4(b))
			}
		case 28: // AAAA
			if rdlen == 16 {
				var b [16]byte
				copy(b[:], msg[off:off+16])
				out = append(out, netip.AddrFrom16(b))
			}
		}
		off += rdlen
	}
	return out
}

// skipName 跳过一个（可能压缩的）名字，返回之后的偏移；越界返回 -1。
func skipName(msg []byte, off int) int {
	for {
		if off >= len(msg) {
			return -1
		}
		l := int(msg[off])
		if l == 0 {
			return off + 1
		}
		if l&0xC0 == 0xC0 {
			// 压缩指针 2 字节，之后就是下一条记录。
			if off+2 > len(msg) {
				return -1
			}
			return off + 2
		}
		off += 1 + l
	}
}

// buildQuery 组一个 A/AAAA 查询。
func buildQuery(name string, qtype uint16) []byte {
	name = strings.TrimSuffix(name, ".")
	if name == "" {
		return nil
	}
	var buf []byte
	var hdr [12]byte
	binary.BigEndian.PutUint16(hdr[0:2], 0x1234) // 任意 id
	binary.BigEndian.PutUint16(hdr[2:4], 0x0100) // RD
	binary.BigEndian.PutUint16(hdr[4:6], 1)      // QDCOUNT
	buf = append(buf, hdr[:]...)
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 || len(label) > 63 {
			return nil
		}
		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}
	buf = append(buf, 0, byte(qtype>>8), byte(qtype), 0, 1)
	return buf
}

// nxdomain 把查询改写成 NXDOMAIN 应答。
//
// 保留问题段与 id，这样客户端能把应答对上请求；rcode=3。
// RA 置位（我们确实提供了递归服务，只是拒绝回答）。
func nxdomain(query []byte) []byte {
	if len(query) < 12 {
		return nil
	}
	resp := make([]byte, len(query))
	copy(resp, query)
	// QR=1, RD 保留, RA=1, rcode=3
	flags := binary.BigEndian.Uint16(resp[2:4])
	flags |= 0x8000 // QR
	flags |= 0x0080 // RA
	flags = (flags &^ 0x000F) | 3
	binary.BigEndian.PutUint16(resp[2:4], flags)
	binary.BigEndian.PutUint16(resp[6:8], 0) // ANCOUNT
	binary.BigEndian.PutUint16(resp[8:10], 0)
	binary.BigEndian.PutUint16(resp[10:12], 0)
	return resp
}

// buildAnswer 用缓存地址拼一个应答。
func buildAnswer(query []byte, addrs []netip.Addr) []byte {
	if len(query) < 12 {
		return nil
	}
	resp := make([]byte, 12, 12+byte(64))
	copy(resp, query[:12])
	flags := binary.BigEndian.Uint16(resp[2:4])
	flags |= 0x8000           // QR
	flags |= 0x0080           // RA
	flags = (flags &^ 0x000F) // rcode=0
	binary.BigEndian.PutUint16(resp[2:4], flags)
	// 只保留问题段（跳过任何 EDNS 附加记录，简化实现）。
	qEnd := skipName(query, 12)
	if qEnd < 0 || qEnd+4 > len(query) {
		return nil
	}
	resp = append(resp, query[12:qEnd+4]...)
	binary.BigEndian.PutUint16(resp[4:6], 1) // QDCOUNT
	binary.BigEndian.PutUint16(resp[6:8], uint16(len(addrs)))
	binary.BigEndian.PutUint16(resp[8:10], 0)
	binary.BigEndian.PutUint16(resp[10:12], 0)
	// 应答记录用压缩指针指向问题里的名字。
	for _, a := range addrs {
		resp = append(resp, 0xC0, 0x0C) // 指向偏移 12
		if a.Is4() {
			resp = append(resp, 0, 1, 0, 1) // TYPE=A CLASS=IN
		} else {
			resp = append(resp, 0, 28, 0, 1) // TYPE=AAAA
		}
		// TTL 固定 30 秒：缓存是我们自己的，不该让客户端的缓存超过它。
		resp = append(resp, 0, 0, 0, 30)
		b := a.AsSlice()
		resp = append(resp, byte(len(b)>>8), byte(len(b)))
		resp = append(resp, b...)
	}
	return resp
}
