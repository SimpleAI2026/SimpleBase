// 文件说明：env_test.go — gosandbox/vm 的单元测试（TestValidateEnv 等）
//
// 用法
//
//	go test ./gosandbox/vm -run 'TestValidateEnv'
//
// 调用架构
//
//	[上游] gosandbox/vm/box, gosandbox/vm/fake, gosandbox/vm/local, internal/api 等 7 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/env_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] driver.go, env.go, errors.go, output.go, output_test.go 等 9 个
//
// 主要符号
//
//	TestValidateEnv(t *testing.T)

package vm

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestValidateEnv(t *testing.T) {
	valid := map[string]string{"PATH": "/bin", "_EMPTY": "", "A1": "value"}
	if err := ValidateEnv(valid); err != nil {
		t.Fatal(err)
	}
	for name, env := range map[string]map[string]string{
		"lowercase key": {"lower": "value"},
		"empty key":     {"": "value"},
		"reserved":      {"PLATFORM_TOKEN": "secret"},
		"long value":    {"KEY": strings.Repeat("x", 4097)},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateEnv(env, "PLATFORM_"); !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
	tooMany := make(map[string]string, 33)
	for i := range 33 {
		tooMany[fmt.Sprintf("K%d", i)] = ""
	}
	if err := ValidateEnv(tooMany); !errors.Is(err, ErrInvalid) {
		t.Fatalf("too many entries: %v", err)
	}
}
