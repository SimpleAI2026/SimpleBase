// kv_handler.go 实现项目级 Key-Value 单端点（key-value-ducklake-plan §3）。
//
// 唯一路由：POST /v1/projects/:projectID/kv
// 请求体两种形态：
//   - {"type":"cmd","argvs":[...]}           Redis 语义命令（读写删扫过期自增）
//   - {"type":"String|Hash|List|Set|ZSet","args":{...}}  按类型写入一份数据
//
// 项目 KV 是 catalog 里一行 kind=kv 的内部记录（建项目时创建）；不存在时
// 尝试补建一次，系统项目直接 404。写命令在只读实例返回 503。
// 成功响应 body 是这条命令的 Redis 回复编码成 JSON，不再包一层 {result:...}。
package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/kv"
)

// KVService 抽象 KV handler 所需的 catalog + registry 能力（项目级）。
type KVService interface {
	// GetKVDatabase 返回项目专属 kind=kv 的 catalog 行（不存在返回 catalog.ErrNotFound）。
	GetKVDatabase(ctx context.Context, tenantID, projectID string) (catalog.Database, error)
	// CreateKVDatabase 补建项目 KV catalog 行（幂等）。
	CreateKVDatabase(ctx context.Context, tenantID, projectID string) (catalog.Database, error)
	// Acquire 获取 KV catalog 的访问租约。
	Acquire(ctx context.Context, db catalog.Database, mode database.AccessMode) (SQLLease, error)
}

// KVHandler 实现项目级 KV 单端点。
type KVHandler struct {
	svc      KVService
	writable bool
}

// NewKVHandler 构造 KV handler。writable=false 时写命令返回 503。
func NewKVHandler(svc KVService, writable bool) *KVHandler {
	return &KVHandler{svc: svc, writable: writable}
}

// kvLease 是一次请求持有的 KV 访问上下文。
type kvLease struct {
	lease SQLLease
	store *kv.Store
}

func (l *kvLease) release() { l.lease.Release() }

// acquireStore 前置流程：项目上下文 → 系统项目 404 → 取/补建 kind=kv 行 →
// 写路径 writable 检查 → Acquire → 构造 kv.Store。
func (h *KVHandler) acquireStore(c echo.Context, mode database.AccessMode) (*kvLease, error) {
	if _, ok := PrincipalFromContext(c.Request().Context()); !ok {
		return nil, auth.ErrMissingCredentials
	}
	project, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return nil, errors.New("project context missing")
	}
	if catalog.IsSystemProject(project.ID) {
		return nil, NewAPIError(http.StatusNotFound, "kv_not_found", "project has no key-value store", RequestIDFromContext(c.Request().Context()))
	}
	ctx := c.Request().Context()
	db, err := h.svc.GetKVDatabase(ctx, project.TenantID, project.ID)
	if err != nil {
		if !errors.Is(err, catalog.ErrNotFound) {
			return nil, err
		}
		// 项目没有 KV catalog：尝试补建一次（历史项目/建项目时失败）。
		if !h.writable {
			return nil, NewAPIError(http.StatusNotFound, "kv_not_found", "project has no key-value store", RequestIDFromContext(ctx))
		}
		db, err = h.svc.CreateKVDatabase(ctx, project.TenantID, project.ID)
		if err != nil {
			return nil, err
		}
	}
	if mode == database.ReadWrite && !h.writable {
		return nil, database.ErrWriterUnavailable
	}
	lease, err := h.svc.Acquire(ctx, db, mode)
	if err != nil {
		return nil, err
	}
	return &kvLease{
		lease: lease,
		store: kv.New(lease.Raw(), kv.WithOnWrite(lease.NotifyWrite)),
	}, nil
}

// Execute: POST /v1/projects/:projectID/kv
func (h *KVHandler) Execute(c echo.Context) error {
	var req kvRequest
	if err := c.Bind(&req); err != nil {
		return WriteError(c, kvInvalidArgument("malformed JSON body", c))
	}
	switch req.Type {
	case "cmd":
		if req.Args != nil {
			return WriteError(c, kvInvalidArgument("cmd must not carry args", c))
		}
		if len(req.Argvs) == 0 {
			return WriteError(c, kvInvalidArgument("argvs must be a non-empty array", c))
		}
		return h.execCmd(c, req.Argvs)
	case "String", "Hash", "List", "Set", "ZSet":
		if len(req.Argvs) > 0 {
			return WriteError(c, kvInvalidArgument("data type must not carry argvs", c))
		}
		if req.Args == nil {
			return WriteError(c, kvInvalidArgument("args required", c))
		}
		return h.execTyped(c, req.Type, req.Args)
	case "":
		return WriteError(c, kvInvalidArgument("type is required", c))
	default:
		return WriteError(c, kvInvalidArgument("unknown type "+req.Type, c))
	}
}

// execTyped 处理 type=String/Hash/List/Set/ZSet 的按类型写入。
func (h *KVHandler) execTyped(c echo.Context, typ string, args *kvTypedArgs) error {
	if args.Key == "" {
		return WriteError(c, kvInvalidArgument("key is required", c))
	}
	if args.TTLms != nil && *args.TTLms < 0 {
		return WriteError(c, kvInvalidArgument("ttl_ms must be >= 0", c))
	}
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, auth.ErrMissingCredentials)
	}
	if !principal.HasPermission(auth.DatabaseWrite) {
		return WriteError(c, auth.ErrForbidden)
	}
	mode := database.ReadWrite
	l, err := h.acquireStore(c, mode)
	if err != nil {
		return WriteError(c, err)
	}
	defer l.release()
	ctx := c.Request().Context()

	var result any
	err = l.store.Update(ctx, func(tx *kv.Tx) error {
		var derr error
		switch typ {
		case "String":
			result, derr = h.typedString(ctx, tx, args)
		case "Hash":
			result, derr = h.typedHash(ctx, tx, args)
		case "List":
			result, derr = h.typedList(ctx, tx, args)
		case "Set":
			result, derr = h.typedSet(ctx, tx, args)
		default: // ZSet
			result, derr = h.typedZSet(ctx, tx, args)
		}
		if derr != nil {
			return derr
		}
		// 可选 ttl_ms：数据写成功后再 PEXPIRE；0 表示立即过期删除。
		if args.TTLms != nil && *args.TTLms >= 0 {
			if _, derr := tx.Key().Expire(ctx, args.Key, *args.TTLms); derr != nil {
				if errors.Is(derr, kv.ErrNotFound) && *args.TTLms == 0 {
					return nil // NX/XX 未写入或空结构自动删 key：无 key 可过期
				}
				return derr
			}
		}
		return nil
	})
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, result)
}

func (h *KVHandler) typedString(ctx context.Context, tx *kv.Tx, args *kvTypedArgs) (any, error) {
	if args.Value == nil {
		return nil, kvInvalidArgumentRaw("value is required")
	}
	res, err := tx.Str().SetWith(ctx, args.Key, []byte(*args.Value), kv.SetOptions{
		NX: args.NX, XX: args.XX, KeepTTL: args.KeepTTL,
	})
	if err != nil {
		return nil, err
	}
	if !res.Set {
		return nil, nil // 条件不满足：响应 null（与 Redis SET NX 一致）
	}
	return "OK", nil
}

func (h *KVHandler) typedHash(ctx context.Context, tx *kv.Tx, args *kvTypedArgs) (any, error) {
	if len(args.Fields) == 0 {
		return nil, kvInvalidArgumentRaw("fields must be non-empty")
	}
	if len(args.Fields) > 1000 {
		return nil, kvInvalidArgumentRaw("fields must be 1..1000")
	}
	fields := make(map[string][]byte, len(args.Fields))
	for f, v := range args.Fields {
		fields[f] = []byte(v)
	}
	added, err := tx.Hash().Set(ctx, args.Key, fields)
	if err != nil {
		return nil, err
	}
	return added, nil
}

func (h *KVHandler) typedList(ctx context.Context, tx *kv.Tx, args *kvTypedArgs) (any, error) {
	if len(args.Elems) == 0 {
		return nil, kvInvalidArgumentRaw("elems must be non-empty")
	}
	if len(args.Elems) > 1000 {
		return nil, kvInvalidArgumentRaw("elems must be 1..1000")
	}
	side := args.Side
	if side == "" {
		side = "back"
	}
	if side != "back" && side != "front" {
		return nil, kvInvalidArgumentRaw("side must be back|front")
	}
	elems := make([][]byte, len(args.Elems))
	for i, e := range args.Elems {
		elems[i] = []byte(e)
	}
	if side == "front" {
		return tx.List().PushFront(ctx, args.Key, elems...)
	}
	return tx.List().PushBack(ctx, args.Key, elems...)
}

func (h *KVHandler) typedSet(ctx context.Context, tx *kv.Tx, args *kvTypedArgs) (any, error) {
	if len(args.Elems) == 0 {
		return nil, kvInvalidArgumentRaw("elems must be non-empty")
	}
	if len(args.Elems) > 1000 {
		return nil, kvInvalidArgumentRaw("elems must be 1..1000")
	}
	elems := make([][]byte, len(args.Elems))
	for i, e := range args.Elems {
		elems[i] = []byte(e)
	}
	return tx.Set().Add(ctx, args.Key, elems...)
}

func (h *KVHandler) typedZSet(ctx context.Context, tx *kv.Tx, args *kvTypedArgs) (any, error) {
	if len(args.Items) == 0 {
		return nil, kvInvalidArgumentRaw("items must be non-empty")
	}
	if len(args.Items) > 1000 {
		return nil, kvInvalidArgumentRaw("items must be 1..1000")
	}
	items := make([]kv.ZSetItem, len(args.Items))
	for i, it := range args.Items {
		items[i] = kv.ZSetItem{Elem: []byte(it.Elem), Score: it.Score}
	}
	return tx.ZSet().Add(ctx, args.Key, items...)
}

// execCmd 处理 type=cmd：解析 argvs → 查命令表 → 读/写分类 → 执行仓库方法。
func (h *KVHandler) execCmd(c echo.Context, argvs []string) error {
	name := strings.ToUpper(strings.TrimSpace(argvs[0]))
	spec, ok := kvCommandTable[name]
	if !ok {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "kv_unknown_command",
			"unknown command "+name, RequestIDFromContext(c.Request().Context())))
	}
	run, err := spec.parse(c, argvs[1:])
	if err != nil {
		return WriteError(c, err)
	}
	mode := database.ReadOnly
	if spec.writable {
		principal, ok := PrincipalFromContext(c.Request().Context())
		if !ok {
			return WriteError(c, auth.ErrMissingCredentials)
		}
		if !principal.HasPermission(auth.DatabaseWrite) {
			return WriteError(c, auth.ErrForbidden)
		}
		mode = database.ReadWrite
	}
	l, err := h.acquireStore(c, mode)
	if err != nil {
		return WriteError(c, err)
	}
	defer l.release()
	ctx := c.Request().Context()

	// 读命令：无 schema 视为空库（不建表）；写命令由 Update 惰性建表。
	if mode == database.ReadOnly {
		has, herr := l.store.HasSchema(ctx)
		if herr != nil {
			return WriteError(c, herr)
		}
		if !has {
			result, rerr := spec.empty(ctx)
			if rerr != nil {
				return WriteError(c, rerr)
			}
			return c.JSON(http.StatusOK, result)
		}
		var result any
		if verr := l.store.View(ctx, func(tx *kv.Tx) error {
			var derr error
			result, derr = run(tx)
			return derr
		}); verr != nil {
			return WriteError(c, verr)
		}
		return c.JSON(http.StatusOK, result)
	}

	// §7.2 M1：KV 自管事务原先绕过计时；exec 计入 db_exec、commit 计入 db_commit。
	var result any
	timer := StageTimerFrom(ctx)
	if timer != nil {
		l.store.ObserveCost(func(exec, commit time.Duration) {
			timer.Observe(StageDBExec, exec)
			timer.Observe(StageDBCommit, commit)
		})
	}
	execScope := timer.StageScope(StageDBExec)
	uerr := l.store.Update(ctx, func(tx *kv.Tx) error {
		var derr error
		result, derr = run(tx)
		return derr
	})
	execScope.Done()
	if uerr != nil {
		return WriteError(c, uerr)
	}
	return c.JSON(http.StatusOK, result)
}

// —— 请求类型 ——

type kvRequest struct {
	Type string `json:"type"`
	// Argvs 仅 type=cmd：argvs[0] 是命令名，其余为参数。
	Argvs []string `json:"argvs"`
	// Args 仅数据类型：该类型的一份数据。
	Args *kvTypedArgs `json:"args"`
}

type kvTypedArgs struct {
	Key     string            `json:"key"`
	Value   *string           `json:"value"`
	Fields  map[string]string `json:"fields"`
	Elems   []string          `json:"elems"`
	Side    string            `json:"side"`
	Items   []kvZItem         `json:"items"`
	TTLms   *int64            `json:"ttl_ms"`
	NX      bool              `json:"nx"`
	XX      bool              `json:"xx"`
	KeepTTL bool              `json:"keep_ttl"`
}

type kvZItem struct {
	Elem  string  `json:"elem"`
	Score float64 `json:"score"`
}

// —— 错误辅助 ——

func kvInvalidArgumentRaw(msg string) error {
	return &kvInvalidArgError{msg: msg}
}

func kvInvalidArgument(msg string, c echo.Context) error {
	return NewAPIError(http.StatusBadRequest, "kv_invalid_argument", msg, RequestIDFromContext(c.Request().Context()))
}

// kvInvalidArgError 在事务内使用（拿不到 echo.Context），WriteError 统一映射。
type kvInvalidArgError struct{ msg string }

func (e *kvInvalidArgError) Error() string { return "kv_invalid_argument: " + e.msg }
