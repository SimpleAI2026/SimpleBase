// user.go 定义控制台用户模型与 UserService（CRUD + 登录校验 + Principal 展开）。
package auth

import (
	"context"
	"errors"
	"strings"
	"time"
)

// 用户领域错误（api 层映射到 4xx）。
var (
	ErrUserNotFound     = errors.New("auth: user not found")
	ErrUsernameTaken    = errors.New("auth: username already exists")
	ErrUserDisabled     = errors.New("auth: user disabled")
	ErrUserProtected    = errors.New("auth: user is protected")
	ErrInvalidRole      = errors.New("auth: invalid role")
	ErrWeakPassword     = errors.New("auth: password too weak")
	ErrMustChangePasswd = errors.New("auth: must change password")
)

// MinPasswordLength 是口令最小长度。
const MinPasswordLength = 8

// UserStatus 账号状态。
const (
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
)

// User 是控制台账号（不含口令哈希的对外视图由 handler 裁剪）。
type User struct {
	ID                 string
	Username           string
	PasswordHash       string // 仅仓储/服务内部使用，绝不进 API/日志
	Role               Role
	DisplayName        string
	Email              string
	Status             string
	MustChangePassword bool
	CreatedBy          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastLoginAt        *time.Time
	DisabledAt         *time.Time
}

// CreateUserInput 创建账号输入。
type CreateUserInput struct {
	Username    string
	Password    string
	Role        Role
	DisplayName string
	Email       string
	CreatedBy   string
}

// UpdateUserInput 改账号输入；指针字段为 nil 表示不改。
type UpdateUserInput struct {
	Role               *Role
	DisplayName        *string
	Email              *string
	Status             *string
	Password           *string
	MustChangePassword *bool
}

// UserRepository 抽象 sys_users / sys_project_owners 持久化。
type UserRepository interface {
	Create(ctx context.Context, u User) error
	GetByID(ctx context.Context, id string) (User, error)
	GetByUsername(ctx context.Context, username string) (User, error)
	List(ctx context.Context, limit int, cursor string) ([]User, string, error)
	Update(ctx context.Context, u User) error
	CountByUsername(ctx context.Context, username string) (int, error)

	SetProjectOwner(ctx context.Context, projectID, userID string, at time.Time) error
	GetProjectOwner(ctx context.Context, projectID string) (string, error)
	ListProjectIDsByUser(ctx context.Context, userID string) ([]string, error)
	ListAllProjectIDs(ctx context.Context) ([]string, error)
}

// UserService 提供账号管理与登录校验。
type UserService struct {
	repo   UserRepository
	tenant string // 默认租户（ReservedTenantID）
	now    func() time.Time
}

// NewUserService 构造 UserService。
func NewUserService(repo UserRepository, tenantID string) *UserService {
	return &UserService{repo: repo, tenant: tenantID, now: time.Now}
}

// WithClock 注入时钟（测试用）。
func (s *UserService) WithClock(now func() time.Time) *UserService {
	s.now = now
	return s
}

func normalizeUsername(u string) string { return strings.ToLower(strings.TrimSpace(u)) }

// ValidatePassword 检查口令强度。
func ValidatePassword(p string) error {
	if len(p) < MinPasswordLength {
		return ErrWeakPassword
	}
	return nil
}

// Create 创建账号。role 仅允许 admin|user（禁止建第二个 superadminl1）。
func (s *UserService) Create(ctx context.Context, in CreateUserInput) (User, error) {
	uname := normalizeUsername(in.Username)
	if uname == "" {
		return User{}, ErrInvalidCredentials
	}
	if !in.Role.Valid() || in.Role.IsSuper() {
		return User{}, ErrInvalidRole
	}
	if err := ValidatePassword(in.Password); err != nil {
		return User{}, err
	}
	n, err := s.repo.CountByUsername(ctx, uname)
	if err != nil {
		return User{}, err
	}
	if n > 0 {
		return User{}, ErrUsernameTaken
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return User{}, err
	}
	now := s.now().UTC()
	u := User{
		ID:           newID(),
		Username:     uname,
		PasswordHash: hash,
		Role:         in.Role,
		DisplayName:  strings.TrimSpace(in.DisplayName),
		Email:        strings.TrimSpace(in.Email),
		Status:       UserStatusActive,
		CreatedBy:    in.CreatedBy,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.Create(ctx, u); err != nil {
		return User{}, err
	}
	return u, nil
}

// GetByID / GetByUsername / List 透传仓储。
func (s *UserService) GetByID(ctx context.Context, id string) (User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return User{}, err
	}
	return u, nil
}

func (s *UserService) GetByUsername(ctx context.Context, username string) (User, error) {
	return s.repo.GetByUsername(ctx, normalizeUsername(username))
}

func (s *UserService) List(ctx context.Context, limit int, cursor string) ([]User, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.List(ctx, limit, cursor)
}

// Update 修改账号。禁止把任何人升为 superadminl1；禁止改/禁用保护账号。
func (s *UserService) Update(ctx context.Context, id string, in UpdateUserInput) (User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return User{}, err
	}
	if u.Username == BootstrapUsername {
		// 保护账号：允许改密码/显示名，禁止改角色/状态。
		if in.Role != nil && *in.Role != u.Role {
			return User{}, ErrUserProtected
		}
		if in.Status != nil && *in.Status != u.Status {
			return User{}, ErrUserProtected
		}
	}
	if in.Role != nil {
		if !in.Role.Valid() || in.Role.IsSuper() {
			return User{}, ErrInvalidRole
		}
		u.Role = *in.Role
	}
	if in.DisplayName != nil {
		u.DisplayName = strings.TrimSpace(*in.DisplayName)
	}
	if in.Email != nil {
		u.Email = strings.TrimSpace(*in.Email)
	}
	if in.Status != nil {
		switch *in.Status {
		case UserStatusActive, UserStatusDisabled:
			u.Status = *in.Status
			if u.Status == UserStatusDisabled {
				t := s.now().UTC()
				u.DisabledAt = &t
			} else {
				u.DisabledAt = nil
			}
		default:
			return User{}, ErrInvalidRole
		}
	}
	if in.MustChangePassword != nil {
		u.MustChangePassword = *in.MustChangePassword
	}
	if in.Password != nil && *in.Password != "" {
		if err := ValidatePassword(*in.Password); err != nil {
			return User{}, err
		}
		hash, err := HashPassword(*in.Password)
		if err != nil {
			return User{}, err
		}
		u.PasswordHash = hash
		u.MustChangePassword = true
	}
	u.UpdatedAt = s.now().UTC()
	if err := s.repo.Update(ctx, u); err != nil {
		return User{}, err
	}
	return u, nil
}

// Disable 软删：status=disabled。禁止禁用超管与自己（由 handler 传 actor 校验）。
func (s *UserService) Disable(ctx context.Context, id string) (User, error) {
	st := UserStatusDisabled
	return s.Update(ctx, id, UpdateUserInput{Status: &st})
}

// ChangePassword 修改自己的口令；需验旧密。
func (s *UserService) ChangePassword(ctx context.Context, id, oldPassword, newPassword string) error {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if !VerifyPassword(oldPassword, u.PasswordHash) {
		return ErrInvalidCredentials
	}
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	u.PasswordHash = hash
	u.MustChangePassword = false
	u.UpdatedAt = s.now().UTC()
	return s.repo.Update(ctx, u)
}

// VerifyCredentials 校验用户名口令，返回用户。错误统一语义防枚举。
func (s *UserService) VerifyCredentials(ctx context.Context, username, password string) (User, error) {
	u, err := s.repo.GetByUsername(ctx, normalizeUsername(username))
	if err != nil {
		return User{}, ErrInvalidCredentials
	}
	if !VerifyPassword(password, u.PasswordHash) {
		return User{}, ErrInvalidCredentials
	}
	if u.Status != UserStatusActive {
		return User{}, ErrUserDisabled
	}
	return u, nil
}

// MarkLogin 写入 last_login_at。
func (s *UserService) MarkLogin(ctx context.Context, id string) error {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	u.LastLoginAt = &now
	u.UpdatedAt = now
	return s.repo.Update(ctx, u)
}

// AssignProjectOwner 记录项目归属（user 项目隔离）。
func (s *UserService) AssignProjectOwner(ctx context.Context, projectID, userID string) error {
	if projectID == "" || userID == "" {
		return errors.New("auth: project and user required")
	}
	return s.repo.SetProjectOwner(ctx, projectID, userID, s.now().UTC())
}

// ProjectIDsFor 根据角色展开 Principal.ProjectIDs。
// super/admin：全部项目 + admin 系统项目；user：仅自己拥有的项目（不含 admin）。
func (s *UserService) ProjectIDsFor(ctx context.Context, u User) (map[string]struct{}, error) {
	switch u.Role {
	case RoleSuperAdmin, RoleAdmin:
		ids, err := s.repo.ListAllProjectIDs(ctx)
		if err != nil {
			return nil, err
		}
		out := make(map[string]struct{}, len(ids)+1)
		for _, id := range ids {
			out[id] = struct{}{}
		}
		out[AdminProjectID] = struct{}{}
		return out, nil
	case RoleUser:
		ids, err := s.repo.ListProjectIDsByUser(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		out := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			if id == AdminProjectID {
				continue
			}
			out[id] = struct{}{}
		}
		return out, nil
	default:
		return map[string]struct{}{}, nil
	}
}

// AdminProjectID 与 catalog.ReservedSystemProjectID 保持一致（避免 auth→catalog 依赖）。
const AdminProjectID = "sb-admin"

// BootstrapUsername / BootstrapPassword 是首启种子超管（planv3.0 §3.5）。
const (
	BootstrapUsername = "simplebase2026"
	BootstrapPassword = "simplebase2026"
)

// PrincipalFromUser 把登录用户展开为 Principal（权限位 + 项目集）。
func (s *UserService) PrincipalFromUser(ctx context.Context, u User, sessionID string) (Principal, error) {
	pids, err := s.ProjectIDsFor(ctx, u)
	if err != nil {
		return Principal{}, err
	}
	return Principal{
		UserID:      u.ID,
		Username:    u.Username,
		Role:        u.Role,
		SessionID:   sessionID,
		TenantID:    s.tenant,
		ProjectIDs:  pids,
		Permissions: PermissionsForRole(u.Role),
	}, nil
}

// EnsureBootstrapUser 幂等种子超管；不存在则创建，存在则不动。
// 超管只能由此处创建（Create API 显式拒绝 superadminl1）。
func (s *UserService) EnsureBootstrapUser(ctx context.Context) (User, bool, error) {
	existing, err := s.repo.GetByUsername(ctx, BootstrapUsername)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, ErrUserNotFound) {
		return User{}, false, err
	}
	hash, err := HashPassword(BootstrapPassword)
	if err != nil {
		return User{}, false, err
	}
	now := s.now().UTC()
	u := User{
		ID:           newID(),
		Username:     BootstrapUsername,
		PasswordHash: hash,
		Role:         RoleSuperAdmin,
		Status:       UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.Create(ctx, u); err != nil {
		if errors.Is(err, ErrUsernameTaken) {
			existing, err2 := s.repo.GetByUsername(ctx, BootstrapUsername)
			return existing, false, err2
		}
		return User{}, false, err
	}
	return u, true, nil
}
