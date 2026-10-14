package auth

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	_ "github.com/uglyer/go-sqlite3"
)

// SYNC-01 测试仓储：只实现 UserRepository 所需方法，记录 Update 调用。
type sync01UserRepo struct {
	mu     sync.Mutex
	users  map[string]User
	update int
}

func (r *sync01UserRepo) Create(_ context.Context, u User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users[u.ID] = u
	return nil
}
func (r *sync01UserRepo) GetByID(_ context.Context, id string) (User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return User{}, sql.ErrNoRows
	}
	return u, nil
}
func (r *sync01UserRepo) GetByUsername(_ context.Context, uname string) (User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.users {
		if u.Username == uname {
			return u, nil
		}
	}
	return User{}, sql.ErrNoRows
}
func (r *sync01UserRepo) List(_ context.Context, _ int, _ string) ([]User, string, error) {
	return nil, "", nil
}
func (r *sync01UserRepo) Update(_ context.Context, u User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.update++
	r.users[u.ID] = u
	return nil
}
func (r *sync01UserRepo) CountByUsername(_ context.Context, uname string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, u := range r.users {
		if u.Username == uname {
			n++
		}
	}
	return n, nil
}
func (r *sync01UserRepo) SetProjectOwner(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}
func (r *sync01UserRepo) GetProjectOwner(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (r *sync01UserRepo) ListProjectIDsByUser(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (r *sync01UserRepo) ListAllProjectIDs(_ context.Context) ([]string, error) {
	return nil, nil
}

// SYNC-01：改密/禁用（Update/ChangePassword/Create）写成功后必须触发
// OnWrite 回调——auth 仓储绕过 systemdb 的 notifyWrite，若回调缺失，
// “改密是唯一一次写入”的场景在崩溃冷恢复后旧凭据仍然有效。
func TestUserService_WriteNotifiesSync(t *testing.T) {
	repo := &sync01UserRepo{users: map[string]User{}}
	svc := NewUserService(repo, "t")
	var mu sync.Mutex
	calls := 0
	svc.OnWrite(func(ctx context.Context) {
		mu.Lock()
		calls++
		mu.Unlock()
		if ctx == nil {
			t.Error("OnWrite callback received nil ctx")
		}
	})

	ctx := context.Background()
	// Create 触发一次。
	u, err := svc.Create(ctx, CreateUserInput{
		Username: "alice", Password: "strong-pass-123", Role: RoleUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if calls != 1 {
		t.Fatalf("after Create: onWrite calls=%d want 1", calls)
	}
	mu.Unlock()

	// 改密触发一次。
	if err := svc.ChangePassword(ctx, u.ID, "strong-pass-123", "another-pass-456"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if calls != 2 {
		t.Fatalf("after ChangePassword: onWrite calls=%d want 2", calls)
	}
	mu.Unlock()

	// Update（禁用用户）触发一次。
	st := UserStatusDisabled
	if _, err := svc.Update(ctx, u.ID, UpdateUserInput{Status: &st}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if calls != 3 {
		t.Fatalf("after Update: onWrite calls=%d want 3", calls)
	}
	mu.Unlock()

	// 失败路径不触发：弱口令改密在写库前被拒。
	if err := svc.ChangePassword(ctx, u.ID, "another-pass-456", "short"); err == nil {
		t.Fatal("weak password must be rejected")
	}
	mu.Lock()
	if calls != 3 {
		t.Fatalf("failed write must not notify: calls=%d want 3", calls)
	}
	mu.Unlock()
}
