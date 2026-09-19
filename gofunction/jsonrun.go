// jsonrun.go 提供 HTTP+JSON 调用云函数所需的公共 API：
// ValidateHTTPFuncs（保存时校验）与 RunJSON（invoke 时执行）。
//
// 约定（见 plan/planv2.0/ui-gofunction-plan.md §3.1/§6）：
//   - 每个包级导出函数必须恰好 1 个入参、1 个返回值
//   - 入参/返回值类型必须能通过 typeChange（未导出字段会 panic，须保存时拦截）
//   - 未导出函数不可调用（与列表不可见一致）
package gofunction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"reflect"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// ErrFunctionNotFound 表示目标函数不存在或未导出（对外统一语义）。
var ErrFunctionNotFound = errors.New("function not found")

// ErrTimeout 哨兵：解释器超时被 recover 包装后 errors.Is 会失配，
// RunJSON 识别后用本错误包裹返回，供 handler 映射 504。
var ErrTimeout = errors.New("gofunction: execution timed out")

// ErrBind 表示请求 body 无法解到函数形参，供 handler 映射 400。
var ErrBind = errors.New("gofunction: cannot bind request body")

// ErrCompile 表示 invoke 时 BuildProgram 失败（保存后环境变化），
// 供 handler 映射 500 gofunction_compile_error（ui-gofunction-plan §7.2）。
var ErrCompile = errors.New("gofunction: compile failed")

// FuncInfo 描述一个合规的导出 HTTP 函数。
type FuncInfo struct {
	Name       string // Hello
	ParamType  string // main.Request / map[string]interface{} / string
	ResultType string
}

// ValidateHTTPFuncs 编译源码并校验全部包级导出函数符合 HTTP 约定：
// Params==1 且 Results==1，且参数/返回类型可通过 typeChange 预演
// （拦截未导出 struct 字段、解释器不支持的类型），把问题提前到保存时。
// 错误信息列出每个不合规函数的名字与原因，可原样透传给前端。
func ValidateHTTPFuncs(source string) ([]FuncInfo, error) {
	program, err := BuildProgram(newSequenceID(), "main", source)
	if err != nil {
		return nil, err
	}
	var infos []FuncInfo
	var problems []string
	for name, member := range program.mainPkg.Members {
		fn, ok := member.(*ssa.Function)
		if !ok || fn.Pkg != program.mainPkg || fn.Object() == nil {
			continue
		}
		// 跳过方法（通过 wrapper/成员函数进入 Members 的情况）
		if sig, ok := fn.Object().Type().(*types.Signature); ok && sig.Recv() != nil {
			continue
		}
		if !ast.IsExported(name) {
			continue
		}
		if name == "init" {
			continue
		}
		sig := fn.Signature
		if sig.Params().Len() != 1 {
			problems = append(problems, fmt.Sprintf("%s: 期望 1 个入参，实际 %d", name, sig.Params().Len()))
			continue
		}
		if sig.Results().Len() != 1 {
			problems = append(problems, fmt.Sprintf("%s: 期望 1 个返回值，实际 %d", name, sig.Results().Len()))
			continue
		}
		rt, err := safeTypeChange(sig.Params().At(0).Type())
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: 参数类型不被支持（%v）", name, err))
			continue
		}
		if _, err := safeTypeChange(sig.Results().At(0).Type()); err != nil {
			problems = append(problems, fmt.Sprintf("%s: 返回值类型不被支持（%v）", name, err))
			continue
		}
		infos = append(infos, FuncInfo{
			Name:       name,
			ParamType:  sig.Params().At(0).Type().String(),
			ResultType: sig.Results().At(0).Type().String(),
		})
		_ = rt
	}
	if len(infos) == 0 {
		if len(problems) > 0 {
			return nil, errors.New("至少导出一个合规的大写函数；问题：\n" + strings.Join(problems, "\n"))
		}
		return nil, errors.New("至少导出一个大写函数（func Name(req T) R）")
	}
	if len(problems) > 0 {
		return nil, errors.New("以下导出函数不符合约定：\n" + strings.Join(problems, "\n"))
	}
	// 按名字排序保证 exports_json 稳定
	for i := 1; i < len(infos); i++ {
		for j := i; j > 0 && infos[j-1].Name > infos[j].Name; j-- {
			infos[j-1], infos[j] = infos[j], infos[j-1]
		}
	}
	return infos, nil
}

// RunJSON 编译并执行 funcName，body 解到唯一形参，返回值 JSON 编码。
// body 为空视为 {}。函数不存在或未导出返回 ErrFunctionNotFound。
// 解释器超时返回 ErrTimeout（%w 包裹 context.DeadlineExceeded 语义）。
func RunJSON(ctx context.Context, seqid, fname, source, funcName string, body []byte) ([]byte, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	program, err := BuildProgram(seqid, fname, source)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCompile, err)
	}
	mainFn := program.mainPkg.Func(funcName)
	if mainFn == nil || !ast.IsExported(funcName) {
		return nil, fmt.Errorf("%w: %s", ErrFunctionNotFound, funcName)
	}
	sig := mainFn.Signature
	if sig.Params().Len() != 1 || sig.Results().Len() != 1 {
		return nil, fmt.Errorf("function %s must have exactly 1 param and 1 result", funcName)
	}

	rt, err := safeTypeChange(sig.Params().At(0).Type())
	if err != nil {
		return nil, fmt.Errorf("function %s: unsupported param type: %w", funcName, err)
	}
	elemType := rt
	if rt.Kind() == reflect.Ptr {
		elemType = rt.Elem()
	}
	ptr := reflect.New(elemType)
	if len(strings.TrimSpace(string(body))) == 0 {
		if elemType.Kind() != reflect.Struct && elemType.Kind() != reflect.Map && elemType.Kind() != reflect.Slice {
			return nil, fmt.Errorf("%w: empty body requires struct/map/slice param, got %s", ErrBind, elemType)
		}
	} else if err := json.Unmarshal(body, ptr.Interface()); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBind, err)
	}

	var arg interface{}
	if rt.Kind() == reflect.Ptr {
		arg = ptr.Interface()
	} else {
		arg = ptr.Elem().Interface()
	}
	result, runErr := program.Run(seqid, funcName, arg)
	// 调用方取消优先（§6 要点 8）：解释器返回后若 ctx 已取消且非超时，返回调用方取消
	if ctx != nil {
		if cerr := ctx.Err(); cerr != nil && !errors.Is(cerr, context.DeadlineExceeded) {
			return nil, cerr
		}
	}
	if runErr != nil {
		if isDeadlineExceeded(runErr) {
			return nil, fmt.Errorf("%w: %v", ErrTimeout, runErr)
		}
		return nil, runErr
	}
	out, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("cannot encode result: %w", err)
	}
	return out, nil
}

// safeTypeChange 带 recover 的 typeChange：未导出字段等会 panic，转为 error。
func safeTypeChange(typ types.Type) (rt reflect.Type, err error) {
	defer func() {
		if re := recover(); re != nil {
			err = fmt.Errorf("%v", re)
			rt = nil
		}
	}()
	rt = typeChangeCached(typ)
	// StructOf 生成的类型若含未导出字段，后续 Set 会受限，但 Unmarshal 可用；
	// 这里只要求类型本身可构造。
	return rt, nil
}

// isDeadlineExceeded 识别解释器 recover 包装后的超时错误。
// runFrame 超时以 panic(ctx.Err()) 抛出，最终包成 "err: context deadline exceeded"
// 或 "recover: ... context deadline exceeded"，errors.Is 无法匹配。
func isDeadlineExceeded(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return strings.Contains(err.Error(), context.DeadlineExceeded.Error())
}
