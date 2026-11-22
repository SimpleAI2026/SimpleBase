package vm

import (
	"fmt"
	"regexp"
	"strings"
)

var envKeyPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// ValidateEnv 在环境变量进入沙盒前校验键、数量、长度与保留前缀。
// 保留前缀由调用方指定，模块不内置任何平台前缀。
func ValidateEnv(env map[string]string, reservedPrefixes ...string) error {
	if len(env) > 32 {
		return fmt.Errorf("%w: env has more than 32 entries", ErrInvalid)
	}
	for key, value := range env {
		if len(key) > 128 || !envKeyPattern.MatchString(key) {
			return fmt.Errorf("%w: invalid env key %q", ErrInvalid, key)
		}
		for _, prefix := range reservedPrefixes {
			if prefix != "" && strings.HasPrefix(key, prefix) {
				return fmt.Errorf("%w: env key %q uses a reserved prefix", ErrInvalid, key)
			}
		}
		if len(value) > 4096 {
			return fmt.Errorf("%w: env value for %q exceeds 4KiB", ErrInvalid, key)
		}
	}
	return nil
}
