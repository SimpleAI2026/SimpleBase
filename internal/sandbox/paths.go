package sandbox

import (
	"fmt"
	"regexp"
	"strings"
)

// 路径规则（落在 workdir 内）在 gosandbox/vm.CleanPath，业务层不再自己实现一份。

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

// CloudName 是驱动侧的 VM 名：sbx- + 去连字符的 id（local 后端下即工作区目录名）。
func CloudName(id string) string {
	return "sbx-" + strings.ReplaceAll(strings.ToLower(strings.TrimSpace(id)), "-", "")
}

// ValidateEnvForE2B 校验 E2B 创建体里的 envVars，规则与命令执行时的 env 相同
// （键格式、数量、长度、保留前缀）。
func ValidateEnvForE2B(env map[string]string) error { return validateEnv(env) }
