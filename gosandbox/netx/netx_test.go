package netx

// netx_test.go：出网策略与白名单的判定逻辑。
//
// 这些用例是「安全边界」的回归测试：每一条都对应一个真实的 SSRF 落点。
// 改这块代码时先看这里，别让「顺手放行」把某个网段漏出去。

import (
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"
)

// 默认策略下，这些地址必须被拒。
func TestBlockedAddresses(t *testing.T) {
	p := Policy{Mode: ModeEgress}
	for _, s := range []string{
		"169.254.169.254", // 云元数据
		"169.254.1.1",     // 链路本地
		"100.64.0.1",      // CGNAT
		"10.1.2.3",        // 私网
		"172.16.0.1",
		"172.31.255.254",
		"192.168.1.1",
		"127.0.0.1", // 宿主回环
		"0.0.0.0",
		"fc00::1", // IPv6 ULA
		"fe80::1", // IPv6 链路本地
	} {
		ip := netip.MustParseAddr(s)
		if err := CheckAllowed(ip, p); err == nil {
			t.Errorf("%s must be blocked", s)
		} else if !errors.Is(err, ErrBlockedAddress) {
			t.Errorf("%s: err = %v", s, err)
		}
	}
}

// 公网地址放行。
func TestAllowedPublic(t *testing.T) {
	p := Policy{Mode: ModeEgress}
	for _, s := range []string{"1.1.1.1", "8.8.8.8", "93.184.216.34", "2606:4700::1111"} {
		ip := netip.MustParseAddr(s)
		if err := CheckAllowed(ip, p); err != nil {
			t.Errorf("%s should be allowed: %v", s, err)
		}
	}
}

// none 档全拒：包括公网。
func TestModeNoneBlocksEverything(t *testing.T) {
	p := Policy{Mode: ModeNone}
	if err := CheckAllowed(netip.MustParseAddr("1.1.1.1"), p); err == nil {
		t.Fatal("mode=none must block public addresses too")
	}
}

// AllowPrivate 打开后私网可通，但元数据地址仍要单独开关。
func TestAllowPrivateButNotMetadata(t *testing.T) {
	p := Policy{Mode: ModeEgress, AllowPrivate: true}
	if err := CheckAllowed(netip.MustParseAddr("10.1.2.3"), p); err != nil {
		t.Fatalf("private should be allowed: %v", err)
	}
	if err := CheckAllowed(netip.MustParseAddr("169.254.169.254"), p); err == nil {
		t.Fatal("metadata must stay blocked even with AllowPrivate")
	}
	// 单独打开 AllowMetadata 才通。
	p.AllowMetadata = true
	if err := CheckAllowed(netip.MustParseAddr("169.254.169.254"), p); err != nil {
		t.Fatalf("metadata should be allowed: %v", err)
	}
}

// 白名单：精确与通配。
func TestHostAllowed(t *testing.T) {
	p := Policy{Mode: ModeAllowlist, Allow: []string{"pypi.org", "*.npmjs.org", "Example.COM."}}
	cases := map[string]bool{
		"pypi.org":           true,
		"files.pypi.org":     false, // 精确规则不覆盖子域
		"registry.npmjs.org": true,
		"a.b.npmjs.org":      true,
		"npmjs.org":          false, // 通配不覆盖父域
		"example.com":        true,  // 大小写与末尾点归一
		"EXAMPLECOM":         false,
		"evil-pypi.org":      false, // 后缀匹配必须带点
		"pypi.org.evil.test": false,
	}
	for host, want := range cases {
		if got := HostAllowed(host, p); got != want {
			t.Errorf("HostAllowed(%q) = %v, want %v", host, got, want)
		}
	}
}

// ParseMode：空值取 none（最安全的一档）。
func TestParseMode(t *testing.T) {
	for in, want := range map[string]Mode{
		"": ModeNone, "none": ModeNone, "NONE": ModeNone,
		"egress": ModeEgress, "egress-allowlist": ModeAllowlist, "allowlist": ModeAllowlist,
	} {
		got, err := ParseMode(in)
		if err != nil {
			t.Errorf("ParseMode(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseMode(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := ParseMode("bridge"); err == nil {
		t.Fatal("unknown mode must error")
	}
}

// BlockedCIDRs 返回副本：改它不影响下一次调用。
func TestBlockedCIDRsIsCopy(t *testing.T) {
	a := BlockedCIDRs()
	if len(a) == 0 {
		t.Fatal("no cidrs")
	}
	a[0] = netip.MustParsePrefix("192.0.2.0/24")
	b := BlockedCIDRs()
	if b[0].String() == "192.0.2.0/24" {
		t.Fatal("BlockedCIDRs must return a copy")
	}
}

// 私网判定覆盖到边界地址。
func TestBlockedBoundaries(t *testing.T) {
	p := Policy{Mode: ModeEgress}
	blocked := []string{"10.0.0.0", "10.255.255.255", "172.16.0.0", "172.31.255.255",
		"192.168.0.0", "192.168.255.255", "169.254.0.0", "169.254.255.255",
		"100.64.0.0", "100.127.255.255", "127.0.0.0", "127.255.255.255"}
	for _, s := range blocked {
		if err := CheckAllowed(netip.MustParseAddr(s), p); err == nil {
			t.Errorf("boundary %s must be blocked", s)
		}
	}
	// 边界外一个地址应当放行。
	allowed := []string{"11.0.0.1", "172.15.255.255", "172.32.0.1", "192.167.255.255",
		"169.253.255.255", "169.255.0.1", "100.63.255.255", "100.128.0.1"}
	for _, s := range allowed {
		if err := CheckAllowed(netip.MustParseAddr(s), p); err != nil {
			t.Errorf("%s should be allowed: %v", s, err)
		}
	}
}

// 非单播地址被拒。
func TestNonUnicastRejected(t *testing.T) {
	p := Policy{Mode: ModeEgress, AllowPrivate: true}
	for _, s := range []string{"224.0.0.1", "ff02::1"} {
		if err := CheckAllowed(netip.MustParseAddr(s), p); err == nil {
			t.Errorf("%s must be rejected", s)
		}
	}
}

// 白名单为空时建不出代理：否则每个查询都会失败，而调用方不知道原因。
func TestNewDNSProxyNeedsAllowlist(t *testing.T) {
	if _, err := NewDNSProxy(DNSPolicy{}); err == nil {
		t.Fatal("empty allowlist must be rejected")
	}
	if _, err := NewDNSProxy(DNSPolicy{Allow: []string{"pypi.org"}}); err != nil {
		t.Fatalf("valid policy: %v", err)
	}
}

// NXDOMAIN 改写：保留 id 与问题段，rcode=3。
func TestNXDomainRewrite(t *testing.T) {
	q := buildQuery("blocked.example", 1)
	if q == nil {
		t.Fatal("cannot build query")
	}
	resp := nxdomain(q)
	if len(resp) != len(q) {
		t.Fatalf("len = %d, want %d", len(resp), len(q))
	}
	if resp[0] != q[0] || resp[1] != q[1] {
		t.Fatal("id must be preserved")
	}
	// rcode = 低 4 位。
	if rcode := resp[3] & 0x0F; rcode != 3 {
		t.Fatalf("rcode = %d, want 3", rcode)
	}
	if resp[2]&0x80 == 0 {
		t.Fatal("QR must be set")
	}
}

// buildAnswer：把缓存地址写进应答。
func TestBuildAnswer(t *testing.T) {
	q := buildQuery("pypi.org", 1)
	if q == nil {
		t.Fatal("cannot build query")
	}
	addrs := []netip.Addr{netip.MustParseAddr("151.101.1.1"), netip.MustParseAddr("2a04:4e42::223")}
	resp := buildAnswer(q, addrs)
	if resp == nil {
		t.Fatal("no answer")
	}
	got := answerAddrs(resp)
	if len(got) != 2 {
		t.Fatalf("addrs = %v", got)
	}
	if got[0] != addrs[0] || got[1] != addrs[1] {
		t.Fatalf("addrs = %v, want %v", got, addrs)
	}
}

// 畸形查询：不回包，也不 panic。
func TestMalformedQuery(t *testing.T) {
	p, err := NewDNSProxy(DNSPolicy{Allow: []string{"pypi.org"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range [][]byte{
		nil,
		{1, 2, 3},
		make([]byte, 12),                     // QDCOUNT=0
		append(make([]byte, 12), 0xC0, 0x0C), // 问题段用压缩指针
	} {
		if resp := p.handle(in); len(resp) > 0 {
			t.Errorf("input %v: expected no reply, got %d bytes", in, len(resp))
		}
	}
}

// nowMillis 返回当前毫秒时间戳（测试里的耗时断言用）。
func nowMillis() int64 { return time.Now().UnixMilli() }

// 白名单外：立刻 NXDOMAIN，不打上游。
func TestBlockedHostGetsNXDOMAIN(t *testing.T) {
	p, err := NewDNSProxy(DNSPolicy{Allow: []string{"pypi.org"}, Upstream: []string{"127.0.0.1:1"}})
	if err != nil {
		t.Fatal(err)
	}
	// 上游指向一个必然连不上的端口：如果实现漏了白名单检查，
	// 就会走到 forward 并超时（5 秒），这个用例也会因此变慢。
	start := nowMillis()
	resp := p.handle(buildQuery("evil.example", 1))
	if resp == nil {
		t.Fatal("expected NXDOMAIN")
	}
	if rcode := resp[3] & 0x0F; rcode != 3 {
		t.Fatalf("rcode = %d, want 3", rcode)
	}
	if el := nowMillis() - start; el > 1000 {
		t.Fatalf("blocked host was forwarded upstream (%d ms)", el)
	}
}

// Resolve 拒绝白名单外的名字。
func TestResolveRejectsNonAllowlisted(t *testing.T) {
	p, err := NewDNSProxy(DNSPolicy{Allow: []string{"pypi.org"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Resolve(t.Context(), "evil.example"); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("err = %v", err)
	}
}

// 查询构造：长标签与空标签被拒。
func TestBuildQueryValidation(t *testing.T) {
	if q := buildQuery("", 1); q != nil {
		t.Fatal("empty name must be rejected")
	}
	if q := buildQuery(strings.Repeat("a", 64)+".com", 1); q != nil {
		t.Fatal("label > 63 bytes must be rejected")
	}
	if q := buildQuery("a..b", 1); q != nil {
		t.Fatal("empty label must be rejected")
	}
	if q := buildQuery("ok.example", 1); len(q) == 0 {
		t.Fatal("valid name must build")
	}
}
