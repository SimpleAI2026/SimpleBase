package sandbox

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

// cleanPath 校验 p 落在 workdir 下（§8.2，沿用 v3 语义）：
// 绝对路径、Clean 后前缀匹配；allowRoot=false 时拒绝 workdir 本身。
func cleanPath(workdir, p string, allowRoot bool) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("%w: path is required", ErrInvalidPath)
	}
	if strings.ContainsRune(p, 0) {
		return "", fmt.Errorf("%w: path contains NUL", ErrInvalidPath)
	}
	if !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("%w: path must be absolute", ErrInvalidPath)
	}
	clean := path.Clean(p)
	if clean == workdir {
		if allowRoot {
			return clean, nil
		}
		return "", fmt.Errorf("%w: path must be a file under %s", ErrInvalidPath, workdir)
	}
	if !strings.HasPrefix(clean, workdir+"/") {
		return "", fmt.Errorf("%w: path must be under %s", ErrInvalidPath, workdir)
	}
	return clean, nil
}

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// validName 校验用户可读名（§6）。
func validName(n string) bool { return nameRe.MatchString(n) }

var envKeyRe = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// reservedEnvPrefixes 禁止注入的 env 前缀（§8.2：不把任何凭据送进沙盒）。
var reservedEnvPrefixes = []string{"SIMPLEBASE_", "MSB_", "AWS_"}

func validateEnv(env map[string]string) error {
	if len(env) > 32 {
		return fmt.Errorf("%w: env has more than 32 entries", ErrInvalidSpec)
	}
	for k, v := range env {
		if len(k) > 128 || !envKeyRe.MatchString(k) {
			return fmt.Errorf("%w: invalid env key %q", ErrInvalidSpec, k)
		}
		for _, pfx := range reservedEnvPrefixes {
			if strings.HasPrefix(k, pfx) {
				return fmt.Errorf("%w: env key %q uses a reserved prefix", ErrInvalidSpec, k)
			}
		}
		if len(v) > 4096 {
			return fmt.Errorf("%w: env value for %q exceeds 4KiB", ErrInvalidSpec, k)
		}
	}
	return nil
}

// truncateUTF8 把 b 转成合法 UTF-8 并截到 max 字节以内（不截断半个字符）。
func truncateUTF8(b []byte, max int) (string, bool) {
	s := strings.ToValidUTF8(string(b), "\uFFFD")
	if max <= 0 || len(s) <= max {
		return s, false
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

// CloudName 是 Cloud 侧 VM 名（§6）：sbx- + 去连字符的 id。
func CloudName(id string) string {
	return "sbx-" + strings.ReplaceAll(strings.ToLower(strings.TrimSpace(id)), "-", "")
}

// ThreadCloudName 是 v3 时期 agent thread 的 VM 名（sb- 前缀），用于接回旧 VM（§9.2）。
func ThreadCloudName(threadID string) string {
	return "sb-" + strings.ReplaceAll(strings.ToLower(strings.TrimSpace(threadID)), "-", "")
}
