// 文件说明：keyring.go — API Key 盐的版本化密钥环（production-readiness-plan §8.2.4）
//
// 用法
//
//	ParseKeyring(spec string) (Keyring, error)          // "v2:<new>,v1:<old>" → 验签环
//	(k Keyring) Hash(rawKey string) string              // 主版本（最新）计算摘要
//	(k Keyring) Hashes(rawKey string) []string          // 全版本摘要（认证时逐一比对）
//
// 轮换流程：改 SIMPLEBASE_AUTH_APIKEY_SECRETS=v2:<new>,v1:<old> 重启 →
// 新签发的 Key 用 v2 摘要存储；认证时对所有版本摘要查表 → 下个窗口去掉 v1。
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrKeyringFormat 是 keyring 解析失败（配置错误，启动期暴露）。
var ErrKeyringFormat = errors.New("auth: invalid keyring spec")

// Keyring 是有序（新→旧）的版本化盐集合。
type Keyring struct {
	// versions 严格按「新→旧」排序；versions[0] 是主版本（签发用）。
	versions []keyringEntry
}

type keyringEntry struct {
	version string
	secret  string
}

// Primary 返回主版本（最新）的版本号与盐。
func (k Keyring) Primary() (version, secret string) {
	if len(k.versions) == 0 {
		return "", ""
	}
	return k.versions[0].version, k.versions[0].secret
}

// Len 返回环内版本数（测试用）。
func (k Keyring) Len() int { return len(k.versions) }

// ParseKeyring 解析 "v2:<secret>,v1:<secret>" 形式的密钥环。
// 规则（§8.2.4）：
//   - 逗号分隔，每项 "版本号:盐"，版本号形如 vN（N 为正整数）；
//   - 必须至少一项；重复版本号报错；
//   - 版本号数字最大者为主版本（签发用），其余按数字降序为验签顺序；
//   - spec 为空返回零值 Keyring（调用方退化到单盐模式）。
func ParseKeyring(spec string) (Keyring, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return Keyring{}, nil
	}
	seen := map[string]bool{}
	entries := make([]keyringEntry, 0, 4)
	for _, item := range strings.Split(spec, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		idx := strings.Index(item, ":")
		if idx <= 0 {
			return Keyring{}, fmt.Errorf("%w: %q (want vN:secret)", ErrKeyringFormat, item)
		}
		version := strings.TrimSpace(item[:idx])
		secret := strings.TrimSpace(item[idx+1:])
		if !isVersionLabel(version) {
			return Keyring{}, fmt.Errorf("%w: bad version %q", ErrKeyringFormat, version)
		}
		if secret == "" {
			return Keyring{}, fmt.Errorf("%w: empty secret for %s", ErrKeyringFormat, version)
		}
		if seen[version] {
			return Keyring{}, fmt.Errorf("%w: duplicate version %s", ErrKeyringFormat, version)
		}
		seen[version] = true
		entries = append(entries, keyringEntry{version: version, secret: secret})
	}
	if len(entries) == 0 {
		return Keyring{}, fmt.Errorf("%w: empty spec", ErrKeyringFormat)
	}
	// 版本号数字降序（新→旧）。
	sort.SliceStable(entries, func(i, j int) bool {
		return versionNumber(entries[i].version) > versionNumber(entries[j].version)
	})
	return Keyring{versions: entries}, nil
}

func isVersionLabel(v string) bool {
	if len(v) < 2 || v[0] != 'v' {
		return false
	}
	for _, r := range v[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func versionNumber(v string) int {
	n := 0
	for _, r := range v[1:] {
		n = n*10 + int(r-'0')
		if n > 1<<30 {
			return n
		}
	}
	return n
}

// Hash 用主版本计算摘要（签发路径）。
func (k Keyring) Hash(rawKey string) string {
	_, secret := k.Primary()
	return hmacHex(secret, rawKey)
}

// Hashes 返回全版本摘要（验签路径：认证时逐一比对/查表）。
func (k Keyring) Hashes(rawKey string) []string {
	out := make([]string, len(k.versions))
	for i, e := range k.versions {
		out[i] = hmacHex(e.secret, rawKey)
	}
	return out
}

func hmacHex(secret, rawKey string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(rawKey))
	return hex.EncodeToString(mac.Sum(nil))
}
