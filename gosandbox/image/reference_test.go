package image

// reference_test.go：引用解析与归一化。
//
//   1. 没写主机补 docker.io；Docker Hub 单段名字补 library/；
//   2. 没写 tag 也没写 digest 补 latest；
//   3. 端口形式的主机（localhost:5000）不能被当成 tag；
//   4. 大写、越界字符、坏摘要一律拒绝。

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"alpine", "docker.io/library/alpine:latest"},
		{"alpine:3.19", "docker.io/library/alpine:3.19"},
		{"library/alpine", "docker.io/library/alpine:latest"},
		{"user/repo", "docker.io/user/repo:latest"},
		{"user/repo:v1", "docker.io/user/repo:v1"},
		{"ghcr.io/o/r", "ghcr.io/o/r:latest"},
		{"ghcr.io/o/r:1.2.3", "ghcr.io/o/r:1.2.3"},
		{"localhost:5000/x", "localhost:5000/x:latest"},
		{"localhost:5000/x:tag", "localhost:5000/x:tag"},
		{"registry.local:443/a/b/c:v", "registry.local:443/a/b/c:v"},
		{"alpine@sha256:" + hex64("a"), "docker.io/library/alpine@sha256:" + hex64("a")},
		{"ghcr.io/o/r:tag@sha256:" + hex64("b"), "ghcr.io/o/r:tag@sha256:" + hex64("b")},
		{"quay.io/a/b_c-d.e:v1", "quay.io/a/b_c-d.e:v1"},
	}
	for _, tc := range cases {
		got, err := Parse(tc.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", tc.in, err)
			continue
		}
		if got.String() != tc.want {
			t.Errorf("Parse(%q) = %q, want %q", tc.in, got.String(), tc.want)
		}
	}
}

func TestParseRejects(t *testing.T) {
	cases := []string{
		"",
		"Alpine",                   // 大写
		"alpine:",                  // 空 tag
		"alpine@sha256:abc",        // 摘要长度不对
		"alpine@md5:" + hex64("a"), // 非 sha256
		"a//b",                     // 空段
		"-bad/repo",                // 段以分隔符开头
		"repo/",                    // 空尾段
		"ghcr.io//x",               // 空段
	}
	for _, in := range cases {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) must fail", in)
		}
	}
}

// 端口形式的主机不能被当成 tag：localhost:5000/x 的主机是 localhost:5000。
func TestParseRegistryWithPort(t *testing.T) {
	ref, err := Parse("localhost:5000/app")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Registry != "localhost:5000" || ref.Repository != "app" || ref.Tag != "latest" {
		t.Fatalf("ref = %+v", ref)
	}
}

// Repo 不带 tag；APIRegistry 把 docker.io 换成 registry-1.docker.io。
func TestReferenceHelpers(t *testing.T) {
	ref, err := Parse("alpine:3.19")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Repo() != "docker.io/library/alpine" {
		t.Fatalf("repo = %q", ref.Repo())
	}
	if ref.APIRegistry() != "registry-1.docker.io" {
		t.Fatalf("api = %q", ref.APIRegistry())
	}
	gh, _ := Parse("ghcr.io/o/r")
	if gh.APIRegistry() != "ghcr.io" {
		t.Fatalf("ghcr api = %q", gh.APIRegistry())
	}
	// 按摘要引用时 manifestRef 用摘要。
	dig, _ := Parse("alpine@sha256:" + hex64("c"))
	if dig.manifestRef() != dig.Digest {
		t.Fatalf("manifestRef = %q", dig.manifestRef())
	}
}

// hex64 造一个 64 位十六进制串。
func hex64(seed string) string {
	out := make([]byte, 64)
	for i := range out {
		out[i] = "0123456789abcdef"[int(seed[0]+byte(i))%16]
	}
	return string(out)
}
