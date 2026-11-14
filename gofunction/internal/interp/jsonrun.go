package interp

import (
	"cmp"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"reflect"
	"slices"
	"strings"
	"sync"

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
	slices.SortFunc(infos, func(a, b FuncInfo) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return infos, nil
}

// programCache RunJSON 编译缓存（B2）：key = sha256(source+"\x00"+fname)。
// 值为 compileOnly 产物（不可变：SSA/布局/块缓存/importer）。
// 每次 RunJSON 命中后 fork + replayInit，得到与全量 BuildProgram
// 语义一致的独立实例（globals 每 fork 一份，init 每次重放），
// 并发请求互不共享可变状态。
// 淘汰：简化 LRU——map + 双向链表，容量默认 128（SetProgramCacheLimit 可调）。
// 不缓存错误；源码变化即新 key，天然失效。
type programCache struct {
	mu           sync.Mutex
	m            map[[32]byte]*list.Element
	lru          *list.List // front = 最近使用；Element.Value = *cacheEntry
	limit        int
	hits, misses int64
}

type cacheEntry struct {
	key  [32]byte
	prog *Program
}

var runJSONCache = newProgramCache(128)

// SetProgramCacheLimit 调整 RunJSON 编译缓存容量（<=0 关闭缓存）。
// 导出为门面 gofunction.SetProgramCacheLimit 供宿主按内存预算调节。
func SetProgramCacheLimit(n int) { runJSONCache.setLimit(n) }

func newProgramCache(limit int) *programCache {
	return &programCache{
		m:     make(map[[32]byte]*list.Element),
		lru:   list.New(),
		limit: limit,
	}
}

func (c *programCache) setLimit(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.limit = n
	for c.limit <= 0 && c.lru.Len() > 0 {
		c.removeOldestLocked()
	}
	for c.limit > 0 && c.lru.Len() > c.limit {
		c.removeOldestLocked()
	}
}

func (c *programCache) removeOldestLocked() {
	oldest := c.lru.Back()
	if oldest == nil {
		return
	}
	c.lru.Remove(oldest)
	delete(c.m, oldest.Value.(*cacheEntry).key)
}

func (c *programCache) get(key [32]byte) *Program {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.m[key]; ok {
		c.lru.MoveToFront(el)
		c.hits++
		return el.Value.(*cacheEntry).prog
	}
	c.misses++
	return nil
}

func (c *programCache) put(key [32]byte, prog *Program) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.limit <= 0 {
		return
	}
	if el, ok := c.m[key]; ok {
		c.lru.MoveToFront(el)
		return
	}
	el := c.lru.PushFront(&cacheEntry{key: key, prog: prog})
	c.m[key] = el
	for c.lru.Len() > c.limit {
		c.removeOldestLocked()
	}
}

// cachedProgram 取（或编译并缓存）源码的编译产物。
// 缓存关闭（limit<=0）或首次未命中时全量编译；命中路径 fork 出
// 每请求实例，等价于 BuildProgram 的产物。
func cachedProgram(seqid, fname, source string) (*Program, error) {
	key := sha256Sum(fname + "\x00" + source)
	if prog := runJSONCache.get(key); prog != nil {
		clone := prog.fork()
		clone.replayInit(seqid)
		return clone, nil
	}
	prog, err := compileOnly(seqid, fname, source)
	if err != nil {
		return nil, err
	}
	runJSONCache.put(key, prog)
	clone := prog.fork()
	clone.replayInit(seqid)
	return clone, nil
}

// sha256Sum 编码缓存键（独立小函数便于测试断言键一致性）。
func sha256Sum(s string) [32]byte {
	return sha256.Sum256([]byte(s))
}

// RunJSON 编译并执行 funcName，body 解到唯一形参，返回值 JSON 编码。
// body 为空视为 {}。函数不存在或未导出返回 ErrFunctionNotFound。
// 解释器超时返回 ErrTimeout（%w 包裹 context.DeadlineExceeded 语义）。
// B2：编译产物按 (fname, source) LRU 缓存（默认 128，SetProgramCacheLimit
// 可调/关闭）；命中时 fork + 重放 init，语义与全量编译一致。
func RunJSON(ctx context.Context, seqid, fname, source, funcName string, body []byte) ([]byte, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	program, err := cachedProgram(seqid, fname, source)
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
	// GF-RUN-01/02：父 ctx 贯穿执行——取消时脚本在块边界中断，
	// goroutine 等待纳入同一截止时间；签名不变。
	result, runErr := program.RunInParentContext(ctx, seqid, funcName, arg)
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
