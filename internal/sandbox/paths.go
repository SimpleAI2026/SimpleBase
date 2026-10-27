// 文件说明：paths.go — internal/sandbox 中 CloudName、ValidateEnvForE2B 等符号的实现
//
// 用法
//
//	CloudName(id string) string  // CloudName 是驱动侧的 VM 名：sbx- + 去连字符的 id（local 后端下即工作区目录名）
//	ValidateEnvForE2B(env map[string]string) error// ValidateEnvForE2B 校验 E2B 创建体里的 envVars，规则与命令执行时的 env 相同
//	同包调用：cleanPath() → manager.go, proc.go；validateEnv() → manager.go, proc.go；validName() → manager.go
//
// 调用架构
//
//	[上游] internal/api, internal/app, internal/sandbox/sshd
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sandbox/paths.go
//	         │ 依赖
//	         ▼
//	[下游] internal/utils/strutil
//	[同包] errors.go, manager.go, output_hub.go, proc.go, pty.go
//	[关键引用] cleanPath() ← manager.go, proc.go
//
// 主要符号
//
//	CloudName(id string) string
//	ValidateEnvForE2B(env map[string]string) error
//	... 另有 7 个顶层符号

package sandbox

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

// cleanPath 校验 p 落在 workdir 下（§8.2）：
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
var reservedEnvPrefixes = []string{"SIMPLEBASE_", "AWS_"}

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
	return strutil.TruncateValidUTF8(b, max)
}

// CloudName 是驱动侧的 VM 名：sbx- + 去连字符的 id（local 后端下即工作区目录名）。
func CloudName(id string) string {
	return "sbx-" + strings.ReplaceAll(strings.ToLower(strings.TrimSpace(id)), "-", "")
}

// ValidateEnvForE2B 校验 E2B 创建体里的 envVars，规则与命令执行时的 env 相同
// （键格式、数量、长度、保留前缀）。
func ValidateEnvForE2B(env map[string]string) error { return validateEnv(env) }
