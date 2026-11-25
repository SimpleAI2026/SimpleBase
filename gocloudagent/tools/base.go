// 文件说明：base.go — 工具基类与共享辅助（planv6.8 §13.6）
//
// Go 里不用继承：
//   - Exec 是统一执行包装：参数解码 → fn → 结果编码（失败信封 / JSON 脱敏 / 截断）；
//   - Workspace 充当「Root mixin」：路径安全与确认策略由它提供，工具通过闭包持有；
//   - ArgString / ArgStrings 是宽松取参辅助，供手写 schema 的工具（plan/explore 等）复用。
//
// 这里不做注册中心：工具仍是普通的 eino tool.BaseTool，由 Toolset 按名字组装。

package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// OutputLimit 是 Exec 包装后单次工具结果的字符上限。
const OutputLimit = 64 << 10

// Fail 是工具的业务失败：Exec 把它编码成统一失败信封，而不是中止整轮。
type Fail struct {
	Code    string
	Message string
}

func (f *Fail) Error() string { return f.Code + ": " + f.Message }

// Failf 构造一个业务失败。
func Failf(code, format string, args ...any) *Fail {
	return &Fail{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Exec 用 fn 构造一个可调用工具。
//
// fn 的返回值约定：
//   - (string, nil)：原样作为结果（工具自行决定格式），超长截断；
//   - (其他值, nil)：MarshalToolJSON（含凭据脱敏）；
//   - (_, *Fail)：编码为 {"ok":false,"error":{...}}，message 经脱敏；
//   - (_, 其他 error)：context 取消原样上抛，其余转 internal 失败信封。
func Exec[T any](name, desc string, fn func(ctx context.Context, in T) (any, error)) (tool.BaseTool, error) {
	return utils.InferTool(name, desc, func(ctx context.Context, in T) (string, error) {
		v, err := fn(ctx, in)
		return encodeResult(ctx, v, err)
	})
}

// encodeResult 把 (结果, 错误) 统一编码成工具返回字符串。
func encodeResult(ctx context.Context, v any, err error) (string, error) {
	if err != nil {
		var f *Fail
		if errors.As(err, &f) {
			return ErrEnvelope(f.Code, agentcore.RedactSecrets(f.Message)), nil
		}
		if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			return "", err
		}
		return ErrEnvelope("internal", agentcore.RedactSecrets(truncateText(err.Error(), 500))), nil
	}
	switch out := v.(type) {
	case string:
		return truncateText(out, OutputLimit), nil
	case nil:
		return `{"ok":true}`, nil
	default:
		s, mErr := agentcore.MarshalToolJSON(out)
		if mErr != nil {
			return ErrEnvelope("encode_failed", mErr.Error()), nil
		}
		return truncateText(s, OutputLimit), nil
	}
}

// Args 是宽松参数表（手写 JSON Schema 的工具用）。
type Args map[string]any

// DecodeArgs 解析工具参数；空串视为空对象。
func DecodeArgs(raw string) (Args, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Args{}, nil
	}
	var a Args
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		return nil, Failf("bad_arguments", "arguments must be a JSON object: %v", err)
	}
	if a == nil {
		a = Args{}
	}
	return a, nil
}

// ArgString 取字符串参数：数字/布尔转成文本，缺失返回 def。
func ArgString(a Args, key, def string) string {
	v, ok := a[key]
	if !ok || v == nil {
		return def
	}
	switch x := v.(type) {
	case string:
		return x
	case float64, bool, json.Number:
		return fmt.Sprint(x)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

// ArgStrings 取字符串列表：接受数组或单个字符串（模型常把单元素写成字符串）。
func ArgStrings(a Args, key string) []string {
	v, ok := a[key]
	if !ok || v == nil {
		return nil
	}
	switch x := v.(type) {
	case string:
		if s := strings.TrimSpace(x); s != "" {
			return []string{s}
		}
		return nil
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if item == nil {
				continue
			}
			s := strings.TrimSpace(ArgString(Args{"v": item}, "v", ""))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// truncateText 按字节截断并加标记（保持既有本机工具的截断口径）。
func truncateText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	// 不切断 UTF-8 多字节字符。
	for cut > 0 && cut < len(s) && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut] + "\n…(truncated)"
}
