# SimpleBase v3.0 登录与三级权限系统设计

> **状态**：P1–P6 已落地（go test ./... 与 ui npm test / npm run build 全绿）  
> **日期**：2026-09-24  
> **范围**：登录弹窗 / 三级角色（superadminl1 · admin · user）/ 用户管理 / 项目可见性 / 系统表 / Go 中间件 OAuth2 风格登录态  
> **关联**：[`system-database-plan.md`](../planv2.0/system-database-plan.md)、[`ui-principles.md`](../planv2.0/ui-principles.md)、[`proto-http.md`](../planv2.0/proto-http.md)、`internal/AGENTS.md`、`ui/AGENTS.md`

---

## 0. 一句话目标

**为控制台补齐「账号登录 + 三级角色」能力：UI 弹窗登录，后端以 OAuth2 Password Grant + JWT 校验登录态；superadminl1 管用户与全部项目，admin 只读全局，user 只管自己创建的项目。**

---

## 1. 背景与现状问题

### 1.1 现状

| 层 | 现状 | 问题 |
|---|---|---|
| 认证 | 仅 `Authorization: Bearer <api-key>`（`sys_api_keys` HMAC 摘要） | 无「人」的概念；无登录态；前端把 Dev Key 写死在 localStorage |
| 授权 | `Principal{APIKeyID, ProjectIDs, Permissions}` | 权限绑在 Key 上，无法表达角色、用户归属、只读管理员 |
| 项目可见性 | `GET /v1/projects` 按 Key 的 ProjectIDs 过滤；DevMode 种子全可见 | 无法「user 只看自己创建的项目」；无 owner 字段 |
| UI | `SettingsModal → ConnectionPanel` 手填 API Key；401 只弹设置 | 不是登录体验；无法按角色隐藏「用户管理」 |
| 系统表 | `sys_tenants` … `sys_cron_job_runs`（v1–v30） | 无 `sys_users` / 会话表 / 项目归属表 |

### 1.2 产品约束（本计划锁定）

1. **三级角色**：`superadminl1` > `admin` > `user`（字符串枚举，落库即此拼写）。  
2. **默认超管**：首次启动幂等种子 `simplebase2026` / `simplebase2026`，角色 `superadminl1`。  
3. **superadminl1**：侧栏多「用户管理」；可看全部用户；顶部项目切换器可切换**所有项目** + **admin 管理数据库**（系统项目）。  
4. **admin**：由 superadminl1 添加；**能看信息，不能修改**（全局只读）。  
5. **user**：普通用户；**只能看到自己创建的项目**，在自己项目内可正常读写。  
6. **UI**：弹出登录框，风格对齐现有 `SbModal` + Field/Input/Button 体系。  
7. **后端**：Go 增加 middleware，**OAuth2 方式**校验登录态（Password Grant 签发 JWT，Bearer 携带，中间件校验）。既有 API Key 通道**保留**，SDK/程序化调用不受影响。  
8. 遵守 `internal/AGENTS.md` / `ui/AGENTS.md`：不反向 import、系统库禁写路径不变、前端不新增依赖、契约进 `types.ts`。

---

## 2. 角色与权限模型（锁定决策）

### 2.1 决策表

| # | 决策 | 说明 |
|---|---|---|
| D1 | **角色三值枚举** | `superadminl1` / `admin` / `user`；存在 `sys_users.role`，不搞继承链 |
| D2 | **登录态 = OAuth2 Password + JWT** | `POST /v1/auth/login` 换 access_token(JWT,2h) + refresh_token(7d)；中间件校验 `Authorization: Bearer <jwt>` |
| D3 | **双通道认证并存** | JWT（人/控制台）与 API Key（SDK）走同一 `AuthMiddleware`：先按 JWT 形态识别，失败再回落 API Key；最终都注入 `auth.Principal` |
| D4 | **admin 全局只读** | 可浏览全部项目/用户/日志/监控；SQL 仅 `SELECT`；所有写接口 403。不能增删改用户、项目、库、对象 |
| D5 | **user 项目隔离** | `sys_project_owners` 记录归属；`ListProjects` 只返回 `owner_user_id = me`；写操作校验项目归属 |
| D6 | **superadminl1 全集** | 全部项目 + admin 系统项目可见；用户管理 CRUD；业务面与 user 相同可写（系统库仍禁写） |
| D7 | **admin 管理数据库 = 系统项目** | `ReservedSystemProjectID`（`…0099`）；仅 superadminl1 / admin 可切换进入；沿用现有系统库只读保护 |
| D8 | **密码哈希** | `golang.org/x/crypto/argon2id`（已是间接依赖，转 direct）；不存明文、不进日志 |
| D9 | **JWT 自实现 HS256** | 标准库 `crypto/hmac`+`sha256`+`encoding/base64`，不新增 JWT 第三方库 |
| D10 | **refresh token 落库可撤销** | 只存 SHA-256 摘要；登出/禁用用户立即失效 |
| D11 | **登录接口免认证** | `/v1/auth/*` 挂在 `/v1` 之下但跳过 AuthMiddleware；其余 `/v1` 全部要求登录态或 API Key |
| D12 | **默认超管不可删** | `simplebase2026` 禁止删除/禁用；可改密码 |

### 2.2 能力矩阵

| 能力 | superadminl1 | admin | user | API Key（SDK） |
|---|---|---|---|---|
| 登录控制台 | ✓ | ✓ | ✓ | — |
| 侧栏「用户管理」 | ✓ | ✗（入口隐藏） | ✗ | — |
| 查看用户列表/详情 | ✓ | ✓ 只读 | ✗ 403 | ✗ |
| 创建/改角色/重置密码/禁用/删除用户 | ✓ | ✗ 403 | ✗ 403 | ✗ |
| 项目列表 | **全部** + admin 系统项目 | **全部** + admin 系统项目 | **仅自己创建** | Key 绑定项目 |
| 创建项目 | ✓（owner=自己或指定） | ✗ 403 | ✓（owner=自己） | 需 `project:admin` |
| 删除/改项目 | ✓ | ✗ 403 | ✓（限自己项目） | 需 `project:admin` |
| 数据库 CRUD | ✓（非系统库） | ✗ 403（可看） | ✓（限自己项目） | Key 权限位 |
| SQL 查询（SELECT） | ✓ | ✓ | ✓（限自己项目） | `database:read` |
| SQL 写 / 文档写 / S3 写 / 云函数写 / 定时任务写 | ✓（限可见项目，系统库拒绝不变） | ✗ 403 | ✓（限自己项目） | `database:write` |
| admin 系统库 | 只读 | 只读 | 不可见 | 不可见 |
| 用户管理 API | ✓ | 列表/详情只读 | ✗ | ✗ |

> 权限位沿用 `database:read|write|admin`、`llm:invoke`、`project:admin`；新增 `user:admin`（仅 superadminl1）。角色 → 权限位在构造 `Principal` 时展开，handler 仍走 `auth.Require`，**不在 handler 里 switch role**。

### 2.3 角色 → Principal 映射

```text
superadminl1 → Permissions{database:read, database:write, database:admin,
                           llm:invoke, project:admin, user:admin}
               ProjectIDs = 全部项目 ∪ {admin-system}（运行时由 catalog 解析）

admin        → Permissions{database:read, llm:invoke}          // 只读
               ProjectIDs = 全部项目 ∪ {admin-system}

user         → Permissions{database:read, database:write, database:admin,
                           llm:invoke, project:admin}
               ProjectIDs = { p | sys_project_owners.user_id = me }

API Key      → 沿用 sys_api_keys.permissions；UserID=""；Role=""
```

`Principal` 扩展字段（向后兼容，零值表示 API Key 通道）：

```go
type Principal struct {
    APIKeyID    string
    UserID      string   // 新增；API Key 为空
    Username    string   // 新增
    Role        Role     // 新增；"" = API Key
    TenantID    string
    ProjectIDs  map[string]struct{}
    Permissions map[Permission]struct{}
}
```

---

## 3. 系统表设计

> 仍走 `internal/systemdb/migrate.go` 只前进迁移；DuckLake 无 PK，唯一性应用层 + 查询判重。版本号接 v30 之后。

### 3.1 `sys_users`（v31）

```sql
CREATE TABLE IF NOT EXISTS sys_users (
    id                  VARCHAR NOT NULL,          -- UUID
    username            VARCHAR NOT NULL,          -- 登录名，大小写不敏感比较，存原样
    password_hash       VARCHAR NOT NULL,          -- argon2id 编码串（含 salt/params）
    role                VARCHAR NOT NULL,          -- superadminl1 | admin | user
    display_name        VARCHAR NOT NULL DEFAULT '',
    email               VARCHAR NOT NULL DEFAULT '',
    status              VARCHAR NOT NULL,          -- active | disabled
    must_change_password BIGINT NOT NULL DEFAULT 0, -- 1=下次登录强制改密
    created_by          VARCHAR NOT NULL DEFAULT '', -- 创建者 user_id；种子超管为 ''
    created_at          TIMESTAMP NOT NULL,
    updated_at          TIMESTAMP NOT NULL,
    last_login_at       TIMESTAMP,
    disabled_at         TIMESTAMP
)
```

唯一性：`username` 全局唯一（应用层 `SELECT` 判重 + 返回 `ErrAlreadyExists`）。

### 3.2 `sys_user_sessions`（v32）— OAuth2 Refresh Token 会话

```sql
CREATE TABLE IF NOT EXISTS sys_user_sessions (
    id                  VARCHAR NOT NULL,          -- session UUID（= refresh token 的 jti）
    user_id             VARCHAR NOT NULL,
    refresh_token_hash  VARCHAR NOT NULL,          -- SHA-256(refresh_token)，绝不存原文
    access_jti          VARCHAR NOT NULL DEFAULT '',-- 当前关联 access JWT jti（便于审计）
    user_agent          VARCHAR NOT NULL DEFAULT '',
    ip                  VARCHAR NOT NULL DEFAULT '',
    expires_at          TIMESTAMP NOT NULL,        -- refresh 过期时间
    created_at          TIMESTAMP NOT NULL,
    revoked_at          TIMESTAMP                  -- 登出/禁用/重置密码时写入
)
```

### 3.3 `sys_project_owners`（v33）— 项目归属

```sql
CREATE TABLE IF NOT EXISTS sys_project_owners (
    project_id          VARCHAR NOT NULL,
    user_id             VARCHAR NOT NULL,
    created_at          TIMESTAMP NOT NULL
)
```

- 历史/种子项目（DevMode `DevProjectID`、admin 系统项目）回填 owner = 超管。  
- 一项目一 owner（应用层保证；若未来要协作再扩 `role` 列，本期不做）。

### 3.4 表关系

```text
sys_users 1 ──── n sys_user_sessions
    │
    │ 1
    │
    n
sys_project_owners n ──── 1 (project_id → sys_projects.id)
```

### 3.5 种子数据（`systemdb.Seed` 扩展）

| 对象 | 值 | 时机 |
|---|---|---|
| 超管用户 | username=`simplebase2026` password=`simplebase2026` role=`superadminl1` status=`active` | 每次启动幂等：用户不存在才插入 |
| 项目归属 | admin 系统项目、DevMode 项目 owner → 超管 | 同上，缺则补 |
| 密码 | 首次种子明文仅用于生成 argon2id 哈希，**立即丢弃**；日志只打 `seeded user simplebase2026` | — |

> 与 `DevRawAPIKey` 同理：默认口令仅限首启引导；文档要求部署后立刻改密（`must_change_password` 可配置为 1，默认 0 以满足「开箱即用」）。

---

## 4. 后端详细设计

### 4.1 总体架构图

```text
                    ┌──────────────────────────────────────────────┐
                    │                 浏览器控制台 UI               │
                    │  LoginModal ─► stores/auth ─► http(Bearer JWT)│
                    └───────────────────┬──────────────────────────┘
                                        │ HTTPS
                                        ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                          SimpleBase Server (Echo)                         │
│                                                                           │
│  middleware 链（自上而下）：                                               │
│  requestID → Recover → AccessLog → BodyLimit → [路由级 Auth] → Project    │
│                                                                           │
│  ┌──────────────────────── AuthMiddleware（新，双通道）─────────────────┐  │
│  │  1) ExtractBearerToken                                             │  │
│  │  2) 若形如 JWT(header.payload.sig) → SessionAuthenticator          │  │
│  │       · 校验 HS256 签名 / exp / iss                                │  │
│  │       · 载入 sys_users（status=active）                             │  │
│  │       · 展开 Role → Permissions + ProjectIDs（owner 查询）          │  │
│  │  3) 否则 → 现有 APIKeyAuthenticator（sys_api_keys）                 │  │
│  │  4) inject Principal → context                                     │  │
│  └───────────────────────────────────┬─────────────────────────────────┘  │
│                                      ▼                                    │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   │
│  │ /v1/auth/*   │  │ /v1/users/*  │  │ /v1/projects │  │ /v1/... 业务 │   │
│  │ login/refresh│  │ Require      │  │ 按 Role 过滤 │  │ Require+归属 │   │
│  │ logout/me    │  │ user:admin   │  │ +owners      │  │              │   │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘   │
│         │                 │                 │                 │           │
│         ▼                 ▼                 ▼                 ▼           │
│  ┌────────────────────────────────────────────────────────────────────┐   │
│  │ internal/auth                                                      │   │
│  │  Service(APIKey) · SessionService(JWT/refresh) · UserService       │   │
│  │  Role · Principal · Middleware                                     │   │
│  └───────────────────────────────┬────────────────────────────────────┘   │
│                                  │                                        │
│  ┌───────────────────────────────▼────────────────────────────────────┐   │
│  │ systemdb.Store（常驻 *sql.DB）                                      │   │
│  │  sys_users · sys_user_sessions · sys_project_owners · sys_* …       │   │
│  └────────────────────────────────────────────────────────────────────┘   │
└───────────────────────────────────────────────────────────────────────────┘
```

### 4.2 包结构（遵守依赖方向：`api → auth`，`auth` 不 import `api`）

```text
internal/auth/
  principal.go          # Role + Principal 扩展 + HasPermission/CanAccessProject
  role.go               # Role 枚举、rolePermissions()、ValidRole()
  password.go           # argon2id Hash/Verify（golang.org/x/crypto）
  jwt.go                # HS256 签发/校验（标准库）
  session.go            # SessionService：Login / Refresh / Logout / Me
  session_repository.go # sys_user_sessions 读写
  user.go               # User 模型 + UserService（CRUD，RBAC 断言）
  user_repository.go    # sys_users / sys_project_owners 读写
  service.go            # 既有 API Key Service（保留）
  middleware.go         # APIKeyMiddleware / Require（保留）+ AuthMiddleware（新双通道）
  api_key_repository.go # 保留

internal/api/
  auth_handler.go       # POST login/refresh/logout，GET me，PUT password
  users_handler.go      # GET/POST/PATCH/DELETE /v1/users
  projects_handler.go   # ListProjects 按 Principal.Role 收敛可见集
  router.go             # 挂载 /v1/auth/*、/v1/users/*；AuthMiddleware 接入
```

### 4.3 OAuth2 协议细节（Password Grant 子集）

#### 4.3.1 `POST /v1/auth/login`（免认证）

```jsonc
// request
{ "username": "simplebase2026", "password": "simplebase2026" }

// 200
{
  "token_type": "Bearer",
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_in": 7200,
  "refresh_token": "rt_<64hex>",
  "user": {
    "id": "…", "username": "simplebase2026", "role": "superadminl1",
    "display_name": "", "must_change_password": false
  }
}

// 401 { "error": { "code": "invalid_credentials", ... } }   // 用户名或密码错误（不区分）
// 403 { "error": { "code": "user_disabled", ... } }
// 423 { "error": { "code": "must_change_password", ... } }  // 登录成功但只回 user + 短 ticket，强制改密后发正式 token
```

登录成功副作用：`last_login_at = now`；创建 `sys_user_sessions` 行。

#### 4.3.2 `POST /v1/auth/refresh`（免认证，携带 refresh_token）

```jsonc
// request  { "refresh_token": "rt_..." }
// 200 同 login 的 token 三元组（轮转：旧 refresh 作废，发新 refresh）
// 401 invalid_refresh_token / session_revoked
```

**Refresh 轮转**：每次 refresh 生成新 `rt`，旧 hash 行 `revoked_at=now` 并写新行；检测到已撤销的 rt 被重放 → 吊销该用户全部会话（防泄漏）。

#### 4.3.3 `POST /v1/auth/logout`（需登录态）

```jsonc
// request { "refresh_token": "rt_..." }   // 可选；缺省吊销当前 access_jti 关联会话
// 204
```

#### 4.3.4 `GET /v1/auth/me`（需登录态）

```jsonc
// 200
{
  "id": "…", "username": "…", "role": "user",
  "display_name": "…", "email": "…",
  "must_change_password": false,
  "projects": [ { "id": "…", "name": "…", "owner": true } ]  // 按角色收敛后的可见项目
}
```

#### 4.3.5 `PUT /v1/auth/password`（需登录态）

```jsonc
// request { "old_password": "...", "new_password": "..." }
// 204；成功后吊销该用户其他全部会话（保留当前 access 直至过期，或直接轮转——实现取「全部吊销并要求重新登录」更安全）
```

#### 4.3.6 JWT 载荷（HS256）

```jsonc
// header  { "alg": "HS256", "typ": "JWT" }
// payload
{
  "iss": "simplebase",
  "sub": "<user_id>",
  "username": "simplebase2026",
  "role": "superadminl1",
  "sid": "<session_id>",          // 关联 sys_user_sessions.id
  "jti": "<access_token_id>",
  "iat": 1760000000,
  "exp": 1760007200               // 2h
}
```

签名密钥：`config.Auth.APIKeyHashSecret`（复用现有盐，**不**在 YAML 明文落盘；生产走 env）。可选后续拆 `auth.jwt_secret`，本期不拆以减配置面。

### 4.4 用户管理 API（`/v1/users`）

| 方法 | 路径 | 权限 | 语义 |
|---|---|---|---|
| GET | `/v1/users` | `user:admin` 或 admin 只读 | 列表（分页 `limit/cursor`）；admin 调用时服务端降级为只读序列化 |
| POST | `/v1/users` | `user:admin` | 创建 `admin` 或 `user`（**禁止**创建第二个 superadminl1） |
| GET | `/v1/users/:id` | 登录态 + （super/admin 或本人） | 详情（永不含 password_hash） |
| PATCH | `/v1/users/:id` | `user:admin` | 改 display_name/email/role/status；重置密码；禁用/启用 |
| DELETE | `/v1/users/:id` | `user:admin` | 软删：`status=disabled` + 吊销全部会话；超管与自己不可删 |

```jsonc
// GET /v1/users 200
{
  "users": [
    {
      "id": "…", "username": "simplebase2026", "role": "superadminl1",
      "display_name": "Super", "email": "", "status": "active",
      "created_by": "", "created_at": "…", "last_login_at": "…",
      "project_count": 3
    }
  ],
  "next_cursor": ""
}

// POST /v1/users
{ "username": "alice", "password": "…", "role": "admin", "display_name": "Alice" }

// PATCH /v1/users/:id
{ "role": "admin", "status": "disabled", "password": "new-secret" }   // 字段均可选
```

**硬规则**：

1. 仅 superadminl1 可写；admin 读到写接口 → `403 permission_denied`。  
2. `role` 只允许 `admin|user`（创建）；PATCH 可降级 superadminl1 之外用户的角色，**不可**把任何人升为 superadminl1。  
3. 不能禁用/删除/改自己的 role；不能动 `simplebase2026`。  
4. 重置密码/禁用 → `UPDATE sys_user_sessions SET revoked_at=now WHERE user_id=? AND revoked_at IS NULL`。

### 4.5 项目可见性与归属

```text
ListProjects(principal):
  switch principal.Role:
    case superadminl1, admin:
        return 全部非删除项目 + admin 系统项目（kind=system，标记 managed=true）
    case user:
        return JOIN sys_project_owners ON user_id = principal.UserID
                （不含 admin 系统项目）
    default:  # API Key
        return 沿用现逻辑（Key 绑定项目）

CreateProject(principal, name):
  role=admin            → 403
  role=superadminl1     → 创建 + owner=指定 user_id（默认自己）
  role=user             → 创建 + owner=自己
  API Key               → 沿用 ProjectAdmin 校验，owner 留空或 = APIKeyID（实现取后者，便于追溯）

DeleteProject / UpdateProject:
  role=admin            → 403
  role=user             → 必须 owner=自己
  role=superadminl1     → 全部可
```

**写路径归属校验**（CreateDatabase / SQL write / S3 write / GoFunction / CronJob 等）：在现有 `Principal.CanAccessProject` 之上，对 Role=`user` **额外**校验 `sys_project_owners`；superadminl1/admin 直接放行（admin 的写已由 `Require(database:write)` 挡掉）。

### 4.6 中间件伪代码

```go
// AuthMiddleware：/v1 挂载；/v1/auth/login|refresh 除外
func AuthMiddleware(users *UserService, sessions *SessionService, keys *Service,
                    inject func(ctx, Principal) context.Context) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            raw, err := ExtractBearerToken(c.Request().Header.Get(echo.HeaderAuthorization))
            if err != nil { return 401 }

            if looksLikeJWT(raw) {
                claims, err := sessions.VerifyAccess(raw)      // 签名+exp+iss
                if err != nil { return 401 invalid_or_expired_token }
                p, err := users.PrincipalFromClaims(c.Request().Context(), claims)
                //  ├─ load user（status 必须 active）
                //  ├─ 展开 Permissions by Role
                //  └─ ProjectIDs：user→owners；super/admin→catalog.AllProjects+admin
                if err != nil { return 401/403 }
                c.SetRequest(...WithContext(inject(ctx, p)))
                return next(c)
            }

            p, err := keys.Authenticate(ctx, raw)              // 既有 API Key
            if err != nil { return 401 }
            c.SetRequest(...WithContext(inject(ctx, p)))
            return next(c)
        }
    }
}

// Require 保持不变：只查 Permissions 位。
// 归属校验仍由 handler/service 做（与 projectContextMiddlewareEcho 一致）。
```

`looksLikeJWT`：`strings.Count(raw, ".") == 2` 且三段均为 base64url。

### 4.7 路由挂载（router.go 增量）

```go
// 免认证
e.POST("/v1/auth/login",  ah.Login)
e.POST("/v1/auth/refresh", ah.Refresh)

// 需认证（AuthMiddleware）
v1 := e.Group("/v1", authMW)
v1.POST("/v1/auth/logout", ah.Logout)     // 实际挂 v1.POST("/auth/logout")
v1.GET("/auth/me", ah.Me)
v1.PUT("/auth/password", ah.ChangePassword)

v1.GET("/users", uh.List)                 // super/admin
v1.POST("/users", uh.Create, require(auth.UserAdmin))
v1.GET("/users/:id", uh.Get)
v1.PATCH("/users/:id", uh.Patch, require(auth.UserAdmin))
v1.DELETE("/users/:id", uh.Delete, require(auth.UserAdmin))

// 既有业务路由全部改挂 authMW（原 APIKeyMiddleware 并入双通道）
```

**兼容**：`/health/*`、`/metrics` 不变；`/go/:projectID` 同样走 `authMW`（JWT 或 API Key 均可）。

### 4.8 安全细节

| 项 | 策略 |
|---|---|
| 密码 | argon2id（time=1, memory=64MB, threads=2, keyLen=32, salt=16B）；`password_hash` 为 `$argon2id$v=19$m=…` 编码串 |
| 口令比对 | `argon2id.Verify` + 常量时间；登录失败统一 `invalid_credentials`（防用户枚举） |
| 登录限速 | 同 IP + 用户名 10 次/分钟（内存 token bucket；超限 429），写 `sys_log_events` |
| JWT | exp=2h；iss=simplebase；密钥 = APIKeyHashSecret；拒绝 `alg=none` |
| refresh | 32 字节随机 `rt_`+hex；只存 SHA-256；7d；轮转 + 重放检测 |
| 会话吊销 | 登出 / 改密 / 禁用 / 删除用户 → `revoked_at`；access JWT 在 2h 窗口内仍有效 **但** `PrincipalFromClaims` 每次查 `sys_users.status`，disabled 立即 401 |
| 日志/审计 | 绝不打 password / token 原文；审计只记 user_id、username、kind、request_id |
| 系统库 | 本计划**不**开任何对系统表的用户 SQL 写路径；用户管理走专用 handler + `systemdb.Store` 内部直连 |

---

## 5. UI 详细设计

> 遵循 `ui/AGENTS.md`：`SbModal` / `Field` / `Input` / `Button` / `TablePager` / `vue-sonner`；不新增依赖；契约进 `services/types.ts`。

### 5.1 登录弹窗（LoginModal）

未登录或 401 时**弹出**（覆盖层，不可绕过），风格对齐 `SettingsModal`（`SbModal` + `maxWidth=420`）。

```text
┌──────────────────────────────────────────────────────────┐
│  登录 SimpleBase                                          │
│  使用管理员分配的账号登录控制台                             │
│                                                          │
│  ┌────────────────────────────────────────────────────┐  │
│  │ 用户名                                             │  │
│  │ ┌────────────────────────────────────────────────┐ │  │
│  │ │ simplebase2026                                 │ │  │
│  │ └────────────────────────────────────────────────┘ │  │
│  │ 输入登录用户名                                      │  │
│  └────────────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────────────┐  │
│  │ 密码                                               │  │
│  │ ┌────────────────────────────────────────────────┐ │  │
│  │ │ ••••••••••••                            [显示] │ │  │
│  │ └────────────────────────────────────────────────┘ │  │
│  │ 输入登录密码                                        │  │
│  └────────────────────────────────────────────────────┘  │
│                                                          │
│  ⚠ 用户名或密码错误                    ← Alert(可选)     │
│                                                          │
│                          ┌──────────┐ ┌──────────────┐   │
│                          │  取消    │ │  ●  登录     │   │
│                          └──────────┘ └──────────────┘   │
│                                    默认按钮 / Enter 提交  │
└──────────────────────────────────────────────────────────┘
     backdrop blur · 居中 · Esc 不关闭（必须登录）· 点遮罩不关
```

**强制改密态**（`must_change_password` 或登录返回 423）：

```text
┌──────────────────────────────────────────────────────────┐
│  修改密码                                                │
│  首次登录请设置新密码后再进入控制台                         │
│                                                          │
│  当前密码   [••••••••                                    ]│
│  新密码     [••••••••••••••                              ]│
│  确认新密码 [••••••••••••••                              ]│
│  ⚠ 两次输入不一致 / 新密码至少 8 位                       │
│                          ┌──────────┐ ┌──────────────┐   │
│                          │  退出    │ │  确认修改    │   │
│                          └──────────┘ └──────────────┘   │
└──────────────────────────────────────────────────────────┘
```

**组件**：`ui/src/components/modal/LoginModal.vue`（业务组件，不进 `components/ui/`）。

### 5.2 控制台主布局（三角色差异）

#### 5.2.1 superadminl1 — 多「用户管理」，顶部可切全部项目 + admin 库

```text
┌────────────┬──────────────────────────────────────────────────────────────────┐
│ ▣ Simple   │  用户管理          [使用文档] [▼ 项目: 全部…  ] [⚙] [↻] [👤▾]   │
│   Base     │                                                                  │
│────────────│  ┌ PageContainer: 用户管理 / 管理系统内全部账号与角色 ─────────┐ │
│            │  │                                                          │ │
│  监控大盘  │  │  工具栏 [+ 新建用户]  [角色筛选▾]  [状态筛选▾]  [搜索…]   │ │
│  数据库管理│  │                                                          │ │
│  S3 对象…  │  │  ┌──────┬────────┬──────────┬──────┬────────┬───────────┐ │ │
│  云函数    │  │  │用户名 │角色    │状态      │项目数│最后登录│操作       │ │ │
│  定时任务  │  │  ├──────┼────────┼──────────┼──────┼────────┼───────────┤ │ │
│  云 Agent │  │  │simple│超管    │● 启用    │    3 │ 09-24  │编辑 重置  │ │ │
│  日志管理  │  │  │base26│admin   │● 启用    │    5 │ 09-23  │编辑 重置  │ │ │
│ ──────────│  │  │alice │user    │○ 禁用    │    1 │ 09-20  │编辑 重置  │ │ │
│ ★用户管理 │  │  └──────┴────────┴──────────┴──────┴────────┴───────────┘ │ │
│            │  │  共 3 条                        ‹ 1 ›                     │ │
└────────────┴──┴──────────────────────────────────────────────────────────┴─┘
  侧栏 h-11 菜单           右上角顺序：文档 → 项目切换(w-56) → 设置 → 刷新 → 账号
  「用户管理」仅 super     项目切换器内含「admin 管理数据库」分组
```

**项目切换器（superadminl1 / admin）**：

```text
        [▼ 📁 项目: 全部…                    ]
        ┌──────────────────────────────────────┐
        │ 搜索项目名称或 ID                     │
        │ ─────────────────────────────────── │
        │ 全部项目                              │
        │  📁 商城后台                          │
        │     0000…0002                        │
        │  📁 数据分析                          │
        │     1111…1111                        │
        │ ─────────────────────────────────── │
        │ admin 管理数据库                      │
        │  🛡 system（只读）                    │
        │     0000…0099                        │
        │ ─────────────────────────────────── │
        │ [+ 新建项目]                          │
        └──────────────────────────────────────┘
```

#### 5.2.2 admin — 无「用户管理」，全局只读

```text
┌────────────┬──────────────────────────────────────────────────────────────────┐
│ ▣ Simple   │  日志管理          [使用文档] [▼ 项目: 全部…  ] [⚙] [↻] [👤▾]   │
│   Base     │                                                                  │
│────────────│   （侧栏无「用户管理」；所有页面写操作按钮隐藏/禁用）            │
│  监控大盘  │   （项目切换器仍可见全部项目 + admin 管理数据库）                │
│  数据库管理│   （数据库页仅「查看数据 / SQL 查询」；无新建/删除/执行）          │
│  S3 对象…  │                                                                  │
│  云函数    │   顶部 [👤▾] 菜单：                                              │
│  定时任务  │   ┌────────────────┐                                            │
│  云 Agent │   │ alice（admin） │                                            │
│  日志管理  │   │ 修改密码       │                                            │
│            │   │ 退出登录       │                                            │
└────────────┴───┴────────────────┴────────────────────────────────────────────┘
```

#### 5.2.3 user — 仅自己的项目

```text
┌────────────┬──────────────────────────────────────────────────────────────────┐
│ ▣ Simple   │  数据库管理        [使用文档] [▼ 项目: 我的商城 ] [⚙] [↻] [👤▾] │
│   Base     │                                                                  │
│────────────│                                                                  │
│  监控大盘  │   项目切换器：                                                   │
│  数据库管理│   ┌──────────────────────────────────────┐                       │
│  S3 对象…  │   │ 搜索项目                            │                       │
│  云函数    │   │ 我的项目                            │                       │
│  定时任务  │   │  📁 我的商城                        │                       │
│  云 Agent │   │  📁 实验室                          │                       │
│  日志管理  │   │ [+ 新建项目]                        │                       │
│            │   └──────────────────────────────────────┘                       │
│ （无用户   │   ✗ 无「admin 管理数据库」分组                                   │
│   管理）   │   ✗ 看不到他人项目                                               │
└────────────┴──────────────────────────────────────────────────────────────────┘
```

#### 5.2.4 侧栏菜单结构（NavMenu 增量）

```text
routeOrder = [
  'dashboard', 'databases', 's3', 'gofunctions',
  'cron-jobs', 'agents', 'logs',
  'users',        // ★ 仅 superadminl1；meta.requiresRole='superadminl1'
]
iconMap.users = UsersIcon
```

```text
┌──────────────┐
│  监控大盘    │
│  数据库管理  │
│  S3 对象存储 │
│  云函数      │
│  定时任务    │
│  云 Agent    │
│  日志管理    │
│ ──────────── │
│  用户管理 ★  │  ← superadminl1 专属（router meta + auth store 双保险）
└──────────────┘
```

### 5.3 用户管理页 ASCII（Users.vue）

```text
┌────────────────────────────────────────────────────────────────────────────┐
│ 用户管理 / 管理系统内全部账号与角色                                         │
│                                                                            │
│ [+ 新建用户]   角色: [全部 ▾]   状态: [全部 ▾]   [🔍 搜索用户名…        ]  │
│                                                                            │
│ ┌─────────┬──────────┬─────────┬──────┬────────────┬─────────────────────┐ │
│ │ 用户名  │ 角色     │ 状态    │项目数│ 最后登录   │ 操作                │ │
│ ├─────────┼──────────┼─────────┼──────┼────────────┼─────────────────────┤ │
│ │simple…  │ 超管     │ ● 启用  │    3 │ 2026-09-24 │ [查看] 重置 禁用 ⋯ │ │
│ │bob      │ 管理员   │ ● 启用  │    5 │ 2026-09-23 │ [查看] [编辑][禁用] │ │
│ │alice    │ 普通用户 │ ○ 禁用  │    1 │ 2026-09-20 │ [查看] [编辑][启用] │ │
│ └─────────┴──────────┴─────────┴──────┴────────────┴─────────────────────┘ │
│ 共 3 条                                                    ‹ 1/1 ›  [10条/页]│
└────────────────────────────────────────────────────────────────────────────┘

新建用户（SbModal）：                 编辑（SbModal，admin 只读打开）：
┌────────────────────────────┐        ┌────────────────────────────────┐
│ 新建用户                   │        │ 用户详情                       │
│ 用户名   [              ]  │        │ 用户名  bob          （只读）  │
│ 显示名   [              ]  │        │ 角色    [管理员 ▾]             │
│ 角色     [普通用户 ▾]      │        │ 状态    [启用 ▾]               │
│ 初始密码 [••••••••      ]  │        │ 项目数  5                      │
│ （≥8 位）                  │        │ 最后登录 2026-09-23 10:02      │
│        [取消] [创建]       │        │ 重置密码 [••••••••          ]  │
└────────────────────────────┘        │     [取消] [保存]  ← admin 隐藏│
                                      └────────────────────────────────┘
```

### 5.4 前端状态与请求流

```text
App 启动
  │
  ├─ auth.bootstrap()
  │    ├─ 有 access_token（内存/localStorage sb_access_token）
  │    │    └─ GET /v1/auth/me
  │    │         ├─ 200 → user + role 写入 store；放行路由
  │    │         └─ 401 → 尝试 POST /v1/auth/refresh
  │    │              ├─ 200 → 换新 token，重试 me
  │    │              └─ 401 → 清 token → 打开 LoginModal
  │    └─ 无 token → 打开 LoginModal
  │
  ├─ http 请求拦截器
  │    Authorization: Bearer <access_token>
  │    401 → auth.markUnauthorized() → 自动 refresh 一次 → 再 401 则 LoginModal
  │
  └─ LoginModal 提交
       POST /v1/auth/login
         ├─ 200 → 存 token；关弹窗；loadProjects()
         ├─ 423 must_change_password → 强制改密视图
         └─ 401 → Alert「用户名或密码错误」
```

**store 扩展**（`stores/auth.ts`）：

```ts
state: {
  accessToken, refreshToken, user: {id,username,role,displayName,mustChangePassword},
  loginOpen, loginReason, lastUnauthorizedAt
}
getters: {
  isSuper  // role === 'superadminl1'
  isAdmin  // role === 'admin'      // 注意：与 project.isAdmin 命名空间分开
  isUser   // role === 'user'
  canManageUsers // isSuper
}
actions: {
  bootstrap, login, logout, refresh, changePassword, markUnauthorized
}
```

> `stores/project.ts` 的 `isAdmin`（是否 admin **项目**）保留不动；角色判断统一走 `auth.isSuper/isAdmin/isUser`，避免语义混淆。

### 5.5 组件/文件清单（UI）

| 文件 | 动作 | 说明 |
|---|---|---|
| `components/modal/LoginModal.vue` | 新增 | 登录 + 强制改密 |
| `pages/Users.vue` | 新增 | 用户管理列表 |
| `components/modal/UserFormModal.vue` | 新增 | 新建/编辑用户 |
| `components/UserMenu.vue` | 新增 | 右上角头像/用户名下拉（改密、退出） |
| `layouts/DefaultLayout.vue` | 改 | 挂 LoginModal + UserMenu；未登录只渲染弹窗 |
| `components/NavMenu.vue` | 改 | `users` 菜单项 + `UsersIcon` |
| `components/GlobalProjectSwitcher.vue` | 改 | 按角色分组「全部项目 / admin 管理数据库 / 我的项目」 |
| `router/index.ts` | 改 | `/users` 路由 + `meta.requiresRole` |
| `stores/auth.ts` | 改 | 角色/token/登录流 |
| `stores/project.ts` | 改 | `loadProjects` 后按角色过滤（服务端已滤，前端只展示） |
| `services/types.ts` | 改 | `LoginRequest/TokenPair/UserItem/UserRole...` |
| `services/http.ts` | 改 | Bearer 改用 access_token；401 自动 refresh |
| `services/http-api.ts` + `mock.js` | 改 | `auth.*` / `users.*` 两侧同步 |

---

## 6. 端到端时序图

```text
浏览器                UI                     Echo                     auth                 systemdb
  │                   │                       │                        │                     │
  │ 打开控制台         │                       │                        │                     │
  │──────────────────►│ bootstrap             │                        │                     │
  │                   │── GET /auth/me ──────►│ AuthMiddleware         │                     │
  │                   │                       │── VerifyAccess ───────►│                     │
  │                   │                       │                        │── SELECT sys_users ►│
  │                   │◄── 401 ───────────────│                        │                     │
  │                   │ 打开 LoginModal       │                        │                     │
  │◄──────────────────│                       │                        │                     │
  │ 输入账号密码       │                       │                        │                     │
  │──────────────────►│ POST /auth/login ───►│（免认证）              │                     │
  │                   │                       │── VerifyPassword ─────►│── SELECT users ────►│
  │                   │                       │── IssueJWT+RT ────────►│── INSERT sessions ─►│
  │                   │◄── 200 token+user ────│                        │                     │
  │                   │ 存 token · 关弹窗     │                        │                     │
  │                   │── GET /projects ────►│ AuthMiddleware(JWT)    │── owners 过滤 ─────►│
  │                   │◄── 项目列表（按角色）─│                        │                     │
  │◄──────────────────│ 顶部切换器渲染        │                        │                     │
  │                   │                       │                        │                     │
  │（super）点用户管理 │                       │                        │                     │
  │──────────────────►│ GET /v1/users ──────►│ Require(user:admin)    │── SELECT users ────►│
  │                   │◄── 全量用户列表 ──────│                        │                     │
  │（admin）同请求     │                       │                        │                     │
  │                   │ GET /v1/users ──────►│ Role=admin 只读放行    │── SELECT users ────►│
  │                   │◄── 列表（无写按钮）───│                        │                     │
  │（user）同请求      │                       │                        │                     │
  │                   │ GET /v1/users ──────►│ Require(user:admin) ✗  │                     │
  │                   │◄── 403 ──────────────│                        │                     │
```

---

## 7. API 契约汇总

| 方法 | 路径 | 认证 | 角色 | 说明 |
|---|---|---|---|---|
| POST | `/v1/auth/login` | 无 | — | Password Grant |
| POST | `/v1/auth/refresh` | 无（持 rt） | — | 轮转 refresh |
| POST | `/v1/auth/logout` | JWT | any | 吊销会话 |
| GET | `/v1/auth/me` | JWT | any | 当前用户+可见项目 |
| PUT | `/v1/auth/password` | JWT | any | 改自己的密码 |
| GET | `/v1/users` | JWT | super / admin* | 列表（*admin 只读） |
| POST | `/v1/users` | JWT | super | 创建 admin/user |
| GET | `/v1/users/:id` | JWT | super / admin / 本人 | 详情 |
| PATCH | `/v1/users/:id` | JWT | super | 改角色/状态/重置密码 |
| DELETE | `/v1/users/:id` | JWT | super | 软删（disable） |
| GET | `/v1/projects` | JWT / API Key | any | **按角色收敛可见集** |
| POST | `/v1/projects` | JWT / API Key | super / user | admin → 403 |
| 其余 `/v1/...` | 既有 | JWT / API Key | Require + 归属 | 写路径对 admin 403 |

错误码新增：`invalid_credentials` · `user_disabled` · `must_change_password` · `invalid_refresh_token` · `session_revoked` · `role_forbidden` · `user_protected`。

---

## 8. 实施阶段

| Phase | 内容 | 交付 |
|---|---|---|
| **P1 后端身份** | 迁移 v31–v33；password/jwt/session/user 仓储与服务；种子超管 | `go test ./internal/auth/... ./internal/systemdb/...` |
| **P2 中间件 + auth API** | 双通道 `AuthMiddleware`；`/v1/auth/*`；Principal 扩展 | 登录/me/refresh/logout 可用 curl 验收 |
| **P3 用户 API + 项目归属** | `/v1/users/*`；ListProjects/CreateProject 归属过滤；写路径 user 归属校验 | RBAC 用例全绿 |
| **P4 UI 登录** | LoginModal + auth store + http 拦截器 + 路由守卫 | 未登录弹窗；登录进控制台；401 自动恢复 |
| **P5 UI 用户管理 + 角色差异** | Users 页 + NavMenu + UserMenu + 项目切换器分组 | 三角色界面差异验收 |
| **P6 加固与文档** | 登录限速、审计补全、README/ops 说明、mock 对齐 | 全量 `go test` + `npm run build` + `npm test` |

---

## 9. 验收标准

| # | 场景 | 期望 |
|---|---|---|
| A1 | 首启空库 | 自动种子 `simplebase2026`/`simplebase2026`，登录成功 |
| A2 | 登录弹窗 | 未登录访问任意控制台路由 → 弹 LoginModal；风格与 SbModal 一致 |
| A3 | super 侧栏 | 出现「用户管理」；可增删改用户；可切换全部项目 + admin 管理数据库 |
| A4 | admin 权限 | 可登录；侧栏无「用户管理」直达；**能看**用户/项目/数据；任意写接口 403；SQL 仅查询 |
| A5 | user 隔离 | 项目切换器仅自己创建的项目；GET /v1/users → 403；访问他人项目 ID → 403 |
| A6 | user 建项目 | 创建后 owner=自己；删除项目仅本人/超管 |
| A7 | 登出/禁用 | 登出后 refresh 失效；禁用用户后其 access 下一次请求 401 |
| A8 | API Key 兼容 | 旧 `Bearer sb_live_...` 调 `/v1` 业务接口行为不回归 |
| A9 | 系统库保护 | 任何角色经 SQL/Data 写接口碰系统库 → 仍 `ErrSystemProtected` |
| A10 | 安全 | 响应/日志/审计无 password、token 原文；连续错误密码触发 429 |

---

## 10. 风险与边界

| 风险 | 缓解 |
|---|---|
| JWT 与 API Key 同 Header 误判 | `looksLikeJWT` 三段 base64url 形态判断；API Key 前缀 `sb_` 优先走 Key |
| admin「只读」漏挡 | 权限位层直接不给 `*:write/admin`；`Require` 统一拦截；补测试矩阵 |
| user 越权历史项目 | 迁移后回填 owners=超管；user 看不到即安全 |
| DuckLake 无唯一约束 | 用户名/项目 owner 查询判重 + `ErrAlreadyExists`；并发种子幂等 |
| access JWT 吊销延迟 | 每请求查 `sys_users.status`；敏感操作（用户管理写）额外查 session `revoked_at` |
| 默认口令上生产 | README 醒目提示；可选 `must_change_password` 配置开关 |

---

## 11. 配置增量（可选，YAML）

```yaml
auth:
  api_key_hash_secret: ""          # 既有；JWT 签名复用
  session:
    access_ttl: 2h                 # 默认 2h
    refresh_ttl: 168h              # 默认 7d
    login_rate_limit: 10/m         # 默认每用户/IP 10 次/分
  bootstrap:
    username: "simplebase2026"     # 允许覆盖默认超管名
    # 密码不进 YAML；仅常量默认 simplebase2026，种子后可改
```

> 密钥仍禁止写 YAML（`dev_mode: false` 时沿用现有拒绝策略）。
