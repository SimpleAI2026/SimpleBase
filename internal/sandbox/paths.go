package sandbox

import (
	"regexp"
	"strings"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

// 路径规则（落在 workdir 内）在 gosandbox/vm.CleanPath，业务层不再自己实现一份。

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// validName 校验用户可读名（§6）。
func validName(n string) bool { return nameRe.MatchString(n) }

// reservedEnvPrefixes 禁止注入的 env 前缀（§8.2：不把任何凭据送进沙盒）。
var reservedEnvPrefixes = []string{"SIMPLEBASE_", "AWS_"}

func validateEnv(env map[string]string) error {
	return vm.ValidateEnv(env, reservedEnvPrefixes...)
}

// CloudName 是驱动侧的 VM 名：sbx- + 去连字符的 id（local 后端下即工作区目录名）。
func CloudName(id string) string {
	return "sbx-" + strings.ReplaceAll(strutil.Normalize(id), "-", "")
}

// ValidateEnvForE2B 校验 E2B 创建体里的 envVars，规则与命令执行时的 env 相同
// （键格式、数量、长度、保留前缀）。
func ValidateEnvForE2B(env map[string]string) error { return validateEnv(env) }
