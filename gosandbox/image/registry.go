// 文件说明：registry.go — gosandbox/image 中 Credentials、Client 等符号的实现
//
// 用法
//
//	type Credentials struct{…}  // Credentials 是 registry 的登录凭据
//	type Client struct{…}  // Client 是 registry 客户端
//	NewClient() *Client  // NewClient 建一个客户端
//	type Resolved struct{…}  // Manifest 是给调用方用的结果：解析后的 manifest 与它的摘要
//	EncodeAuth(host string, cred Credentials) string  // EncodeAuth 生成 X-Registry-Auth 的值（docker 的 base64 JSON 形式）
//	同包调用：parseChallenge() → registry_test.go；isLoopback() → registry_test.go；digestOf() → import.go, pull_test.go
//
// 调用架构
//
//	[上游] gosandbox/vm/box, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/image/registry.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] arch.go, import.go, manifest.go, manifest_test.go, pull.go 等 16 个
//	[关键引用] Credentials ← pull.go, registry_test.go；NewClient ← pull.go, registry_test.go；parseChallenge ← registry_test.go
//
// 主要符号
//
//	var ErrNotFound / ErrUnauthorized / ErrDigestMismatch
//	type Credentials struct{…}
//	type Client struct{…}
//	type tokenEntry struct{…}
//	NewClient() *Client
//	(c *Client) SetCredentials(host string, cred Credentials)
//	type Resolved struct{…}
//	(c *Client) Resolve(ctx context.Context, ref Reference, arch, osName string) (Resolved, error)
//	... 另有 15 个顶层符号

package image

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// 错误分类：调用方据此决定是重试还是报给用户。
var (
	// ErrNotFound 是镜像或 blob 不存在（registry 的 404）。
	ErrNotFound = errors.New("image: not found")
	// ErrUnauthorized 是凭据不足（401 且拿不到 token）。
	ErrUnauthorized = errors.New("image: unauthorized")
	// ErrDigestMismatch 表示下载内容的摘要与声明不一致。
	ErrDigestMismatch = errors.New("image: digest mismatch")
)

// Credentials 是 registry 的登录凭据。
type Credentials struct {
	Username string
	Password string
}

// Client 是 registry 客户端。零值不可用，用 NewClient。
type Client struct {
	http *http.Client
	// creds 按 host 存凭据；空表示匿名。
	mu    sync.Mutex
	creds map[string]Credentials
	// tokens 缓存 host+scope → token，避免每个请求都走一次 token 端点。
	tokens map[string]tokenEntry
}

// tokenEntry 是一个缓存的 bearer token。
type tokenEntry struct {
	token   string
	expires time.Time
}

// NewClient 建一个客户端。超时给足：大层下载本来就很慢。
func NewClient() *Client {
	return &Client{
		http: &http.Client{
			Timeout: 0, // 由 ctx 控制
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				MaxIdleConnsPerHost:   4,
				IdleConnTimeout:       60 * time.Second,
				TLSHandshakeTimeout:   15 * time.Second,
				ResponseHeaderTimeout: 60 * time.Second,
			},
		},
		creds:  map[string]Credentials{},
		tokens: map[string]tokenEntry{},
	}
}

// SetCredentials 为某个 registry 设置凭据。
func (c *Client) SetCredentials(host string, cred Credentials) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.creds[host] = cred
}

// Manifest 是给调用方用的结果：解析后的 manifest 与它的摘要。
type Resolved struct {
	// Manifest 是单平台的 manifest。
	Manifest Manifest
	// Digest 是它的摘要（实际内容算出，不信声明）。
	Digest string
	// IndexDigest 是索引的摘要（如果经过了索引）。
	IndexDigest string
}

// Resolve 解析引用到单平台 manifest。
//
// 引用带摘要时直接取；带 tag 时先取 tag 指向的对象，如果是索引就按
// arch/os 挑一份。摘要一律由内容算出，不用响应头里的值。
func (c *Client) Resolve(ctx context.Context, ref Reference, arch, osName string) (Resolved, error) {
	body, digest, mediaType, err := c.fetchManifest(ctx, ref, ref.manifestRef())
	if err != nil {
		return Resolved{}, err
	}
	m, err := DecodeManifest(body)
	if err != nil {
		return Resolved{}, err
	}
	if m.MediaType == "" && mediaType != "" {
		m.MediaType = mediaType
	}
	res := Resolved{Manifest: m, Digest: digest}
	if !m.IsIndex() {
		return res, nil
	}
	res.IndexDigest = digest
	desc, ok := m.PickPlatform(arch, osName)
	if !ok {
		return Resolved{}, fmt.Errorf("%w: %s has no %s/%s build", ErrUnsupportedPlatform, ref, osName, arch)
	}
	body, digest, _, err = c.fetchManifest(ctx, ref, desc.Digest)
	if err != nil {
		return Resolved{}, err
	}
	sub, err := DecodeManifest(body)
	if err != nil {
		return Resolved{}, err
	}
	if sub.IsIndex() {
		// 索引套索引没有意义，遇到就报错而不是递归。
		return Resolved{}, fmt.Errorf("image: nested index for %s", ref)
	}
	sub.MediaType = desc.MediaType
	if sub.MediaType == "" {
		sub.MediaType = ManifestType
	}
	return Resolved{Manifest: sub, Digest: digest, IndexDigest: res.IndexDigest}, nil
}

// fetchManifest 取一个 manifest，返回内容、算出的摘要与 Content-Type。
func (c *Client) fetchManifest(ctx context.Context, ref Reference, reference string) ([]byte, string, string, error) {
	u := fmt.Sprintf("https://%s/v2/%s/manifests/%s", ref.APIRegistry(), ref.Repository, reference)
	resp, err := c.do(ctx, ref, u, "", map[string]string{
		// 两类都接受：有些 registry 只给 Docker 的，有些只给 OCI 的。
		"Accept": strings.Join([]string{ManifestType, DockerManifestType, ManifestListType, DockerListType}, ", "),
	})
	if err != nil {
		return nil, "", "", err
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body, 8<<20)
	if err != nil {
		return nil, "", "", err
	}
	digest, err := digestOf(body)
	if err != nil {
		return nil, "", "", err
	}
	// 按摘要请求时必须核对：registry 返回另一个对象说明中间被换了内容。
	// 按 tag 请求时摘要只能自己算，没有可核对的目标。
	if strings.HasPrefix(reference, "sha256:") && reference != digest {
		return nil, "", "", fmt.Errorf("%w: asked for %s, got %s", ErrDigestMismatch, reference, digest)
	}
	if declared := resp.Header.Get("Docker-Content-Digest"); declared != "" && declared != digest {
		// 响应头的摘要与内容不符：同样拒绝，不猜哪个是对的。
		return nil, "", "", fmt.Errorf("%w: header says %s, content is %s", ErrDigestMismatch, declared, digest)
	}
	return body, digest, resp.Header.Get("Content-Type"), nil
}

// GetBlob 流式下载一个 blob 并校验摘要。
//
// 边下边算：写完才发现摘要不对时返回 ErrDigestMismatch，调用方负责删掉
// 写了一半的部分。不把整个 blob 放进内存。
func (c *Client) GetBlob(ctx context.Context, ref Reference, desc Descriptor, w io.Writer) error {
	u := fmt.Sprintf("https://%s/v2/%s/blobs/%s", ref.APIRegistry(), ref.Repository, desc.Digest)
	resp, err := c.do(ctx, ref, u, "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	h := sha256.New()
	// 声明的长度是提示：不匹配时仍以摘要为准（有些 registry 报 0）。
	if _, err := io.Copy(io.MultiWriter(w, h), resp.Body); err != nil {
		return fmt.Errorf("image: download %s: %w", desc.Digest, err)
	}
	got := "sha256:" + hex.EncodeToString(h.Sum(nil))
	if got != desc.Digest {
		return fmt.Errorf("%w: %s got %s", ErrDigestMismatch, desc.Digest, got)
	}
	return nil
}

// GetSmall 取一个小对象（config、manifest）的字节并按摘要校验。
func (c *Client) GetSmall(ctx context.Context, ref Reference, desc Descriptor) ([]byte, error) {
	u := fmt.Sprintf("https://%s/v2/%s/blobs/%s", ref.APIRegistry(), ref.Repository, desc.Digest)
	resp, err := c.do(ctx, ref, u, "", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body, 16<<20)
	if err != nil {
		return nil, err
	}
	got, err := digestOf(body)
	if err != nil {
		return nil, err
	}
	if desc.Digest != "" && got != desc.Digest {
		return nil, fmt.Errorf("%w: %s got %s", ErrDigestMismatch, desc.Digest, got)
	}
	return body, nil
}

// do 发一个 GET，必要时走 token 认证。
//
// 流程与 docker 一致：匿名请求 → 401 + WWW-Authenticate → 换 token → 重试。
// token 按 host + scope 缓存；scope 由 registry 的 challenge 给出。
func (c *Client) do(ctx context.Context, ref Reference, u, _ string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	// 已有缓存 token 就直接带。
	if tok, ok := c.cachedToken(ref); ok {
		req.Header.Set("Authorization", "Bearer "+tok)
	} else if cred, ok := c.credentials(ref.APIRegistry()); ok {
		req.SetBasicAuth(cred.Username, cred.Password)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("image: request %s: %w", u, err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return c.check(resp)
	}

	// 401：按 challenge 换 token 再试一次。
	challenge := resp.Header.Get("WWW-Authenticate")
	resp.Body.Close()
	tok, terr := c.token(ctx, ref, challenge)
	if terr != nil {
		return nil, terr
	}
	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req2.Header.Set(k, v)
	}
	req2.Header.Set("Authorization", "Bearer "+tok)
	resp2, err := c.http.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("image: request %s: %w", u, err)
	}
	return c.check(resp2)
}

// check 把非 200 的状态码翻成分类错误。
func (c *Client) check(resp *http.Response) (*http.Response, error) {
	switch {
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent:
		return resp, nil
	case resp.StatusCode == http.StatusNotFound:
		resp.Body.Close()
		return nil, fmt.Errorf("%w: %s", ErrNotFound, resp.Request.URL)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		resp.Body.Close()
		return nil, fmt.Errorf("%w: %s", ErrUnauthorized, resp.Request.URL)
	default:
		body, _ := readLimited(resp.Body, 4<<10)
		resp.Body.Close()
		return nil, fmt.Errorf("image: %s: %s: %s", resp.Request.URL, resp.Status, strings.TrimSpace(string(body)))
	}
}

// token 按 WWW-Authenticate 的 realm/service/scope 换一个 bearer token。
//
// 先看缓存：同一 registry 的同一仓库在一次拉取里会反复要 token（manifest、
// 每层 blob 各一次），缓存命中能省掉大部分往返。
func (c *Client) token(ctx context.Context, ref Reference, challenge string) (string, error) {
	if tok, ok := c.cachedToken(ref); ok {
		return tok, nil
	}
	params := parseChallenge(challenge)
	realm := params["realm"]
	if realm == "" {
		return "", fmt.Errorf("%w: no realm in challenge", ErrUnauthorized)
	}
	// 凭据只能发给 HTTPS 或 loopback：明文 HTTP 上传密码等于泄露。
	u, err := url.Parse(realm)
	if err != nil {
		return "", fmt.Errorf("%w: bad realm %q", ErrUnauthorized, realm)
	}
	if u.Scheme != "https" && !isLoopback(u.Hostname()) {
		return "", fmt.Errorf("%w: refusing credentials over %s", ErrUnauthorized, u.Scheme)
	}

	q := u.Query()
	if s := params["service"]; s != "" {
		q.Set("service", s)
	}
	scope := params["scope"]
	if scope == "" {
		scope = "repository:" + ref.Repository + ":pull"
	}
	// 按 scope 精简：只请求 pull。
	if i := strings.LastIndex(scope, ":"); i > 0 {
		scope = scope[:i+1] + "pull"
	}
	q.Set("scope", scope)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	if cred, ok := c.credentials(ref.APIRegistry()); ok {
		req.SetBasicAuth(cred.Username, cred.Password)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("image: token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := readLimited(resp.Body, 4<<10)
		return "", fmt.Errorf("%w: token %s: %s", ErrUnauthorized, resp.Status, strings.TrimSpace(string(body)))
	}
	var tok struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tok); err != nil {
		return "", fmt.Errorf("image: token response: %w", err)
	}
	t := tok.Token
	if t == "" {
		t = tok.AccessToken
	}
	if t == "" {
		return "", fmt.Errorf("%w: empty token", ErrUnauthorized)
	}
	// 留 60 秒余量，避免刚好在过期瞬间用上。
	ttl := time.Duration(tok.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	if ttl > time.Minute {
		ttl -= time.Minute
	}
	c.mu.Lock()
	c.tokens[tokenKey(ref)+"|"+scope] = tokenEntry{token: t, expires: time.Now().Add(ttl)}
	c.mu.Unlock()
	return t, nil
}

// tokenKey 是缓存键的前缀：同一个 registry 的不同仓库 token 不通用。
func tokenKey(ref Reference) string { return ref.APIRegistry() + "/" + ref.Repository }

// cachedToken 返回未过期的缓存 token。
func (c *Client) cachedToken(ref Reference) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := tokenKey(ref) + "|"
	for k, v := range c.tokens {
		if strings.HasPrefix(k, prefix) && time.Now().Before(v.expires) {
			return v.token, true
		}
	}
	return "", false
}

// credentials 返回某个 registry 的凭据。
func (c *Client) credentials(host string) (Credentials, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cred, ok := c.creds[host]
	if !ok || (cred.Username == "" && cred.Password == "") {
		return Credentials{}, false
	}
	return cred, true
}

// parseChallenge 解析 WWW-Authenticate: Bearer realm="…",service="…",scope="…"。
func parseChallenge(s string) map[string]string {
	out := map[string]string{}
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, ' '); i > 0 {
		s = s[i+1:]
	}
	for _, part := range splitParams(s) {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"`)
	}
	return out
}

// splitParams 按逗号切分，但不切引号里的逗号（scope 里可能有）。
func splitParams(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case r == ',' && !inQuote:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// isLoopback 判断主机名是否为本机。
func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	return strings.HasPrefix(host, "127.") || host == "::1" || host == "[::1]"
}

// digestOf 算内容的 sha256 摘要。
func digestOf(b []byte) (string, error) {
	h := sha256.New()
	if _, err := h.Write(b); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// readLimited 读至多 limit 字节；超出视为错误（响应体不该这么大）。
func readLimited(r io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("image: response larger than %d bytes", limit)
	}
	return body, nil
}

// EncodeAuth 生成 X-Registry-Auth 的值（docker 的 base64 JSON 形式）。
func EncodeAuth(host string, cred Credentials) string {
	payload, _ := json.Marshal(map[string]string{
		"username": cred.Username, "password": cred.Password, "serveraddress": host,
	})
	return base64.URLEncoding.EncodeToString(payload)
}
