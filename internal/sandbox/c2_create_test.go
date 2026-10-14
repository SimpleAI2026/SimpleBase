package sandbox

// c2_create_test.go：planv6.0 C2 表征测试（零生产修改）。
// 锁定 Manager.Create 的可观察行为，作为未来提取 image→idle 连续
// 归一化/校验段的安全网：
//   1. 幂等命中（未过期）复用旧沙盒（Reused=true），跳过名称/配额/资源校验；
//   2. 幂等 key 过期走原创建路径；读取失败同样走创建路径；
//   3. 同名冲突优先于配额（ErrNameConflict 先触发）；
//   4. 创建失败不记录幂等条目（后续同 key 重新创建）；
//   5. 随机名称仅断言格式（sbx- 前缀 + 非空）；
//   6. Start=false 时保持 pending；非法 image/network/resources/idle 各自拒绝；
//   7. TTL 边界：until 与 now 相等时 Before 为 false → 走创建路径。
// 本文件只补测试；生产代码不动（C 类默认仅分析 + 表征）。

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/config"
)

// c2Store 内存 Store fake。
type c2Store struct {
	byID    map[string]Sandbox
	byName  map[string]string // name -> id
	count   int
	getByID func(id string) (Sandbox, error)
}

func newC2Store() *c2Store {
	return &c2Store{byID: map[string]Sandbox{}, byName: map[string]string{}}
}

func (s *c2Store) CreateSandbox(_ context.Context, sb Sandbox) (Sandbox, error) {
	if _, exists := s.byID[sb.ID]; !exists {
		s.count++
	}
	s.byID[sb.ID] = sb
	s.byName[sb.ProjectID+":"+sb.Name] = sb.ID
	return sb, nil
}

func (s *c2Store) GetSandbox(_ context.Context, projectID, id string) (Sandbox, error) {
	if s.getByID != nil {
		return s.getByID(id)
	}
	sb, ok := s.byID[id]
	if !ok {
		return Sandbox{}, sql.ErrNoRows
	}
	return sb, nil
}

func (s *c2Store) GetSandboxByName(_ context.Context, projectID, name string) (Sandbox, error) {
	if id, ok := s.byName[projectID+":"+name]; ok {
		return s.byID[id], nil
	}
	return Sandbox{}, sql.ErrNoRows
}

func (s *c2Store) GetSandboxByThread(ctx context.Context, projectID, threadID string) (Sandbox, error) {
	return Sandbox{}, sql.ErrNoRows
}
func (s *c2Store) ListSandboxes(_ context.Context, projectID, status, source, cursor string, limit int) ([]Sandbox, string, error) {
	return nil, "", nil
}
func (s *c2Store) CountSandboxes(_ context.Context, projectID string) (int, error) {
	return s.count, nil
}
func (s *c2Store) UpdateSandbox(_ context.Context, sb Sandbox) error { return nil }
func (s *c2Store) SoftDeleteSandbox(_ context.Context, projectID, id, actor string) error {
	return nil
}
func (s *c2Store) ListExpiredSandboxes(_ context.Context, t time.Time, limit int) ([]Sandbox, error) {
	return nil, nil
}
func (s *c2Store) ListStaleRunSandboxes(_ context.Context, t time.Time, limit int) ([]Sandbox, error) {
	return nil, nil
}
func (s *c2Store) ListFailedSandboxRemovals(_ context.Context, limit int) ([]Sandbox, error) {
	return nil, nil
}
func (s *c2Store) PurgeDeletedSandboxes(_ context.Context, t time.Time) (int64, error) {
	return 0, nil
}

func c2Manager(t *testing.T) (*Manager, *c2Store) {
	t.Helper()
	cfg := config.SandboxConfig{
		Enabled: true, Workdir: "/workspace", Image: "default-img",
		CPUs: 1, MemoryMiB: 256, Network: "none",
		IdleTimeout: 10 * time.Minute, MaxDuration: time.Hour,
		MaxPerProject: 2, ExecTimeout: time.Second, ExecTimeoutMax: time.Minute,
		MaxFileBytes: 1024,
	}
	store := newC2Store()
	m := NewManager(cfg, store, NewFakeDriver())
	return m, store
}

// 1. 幂等命中复用旧沙盒，跳过后续校验。
func TestC2_IdemHitReusesSandbox(t *testing.T) {
	m, _ := c2Manager(t)
	ctx := context.Background()
	first, err := m.Create(ctx, "p1", CreateInput{Name: "a", IdempotencyKey: "k1"}, "u")
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Create(ctx, "p1", CreateInput{Name: "different-name", IdempotencyKey: "k1"}, "u")
	if err != nil {
		t.Fatalf("idem hit: %v", err)
	}
	if second.ID != first.ID || !second.Reused {
		t.Fatalf("expected reuse of %s, got %+v", first.ID, second)
	}
}

// 2. 幂等条目过期后同 key 走创建路径（新沙盒、非 Reused）。
func TestC2_IdemExpiryTakesCreatePath(t *testing.T) {
	m, _ := c2Manager(t)
	ctx := context.Background()
	first, err := m.Create(ctx, "p1", CreateInput{Name: "a", IdempotencyKey: "k1"}, "u")
	if err != nil {
		t.Fatal(err)
	}
	// 推进时钟越过 24h TTL。
	base := m.now()
	m.now = func() time.Time { return base.Add(25 * time.Hour) }
	second, err := m.Create(ctx, "p1", CreateInput{Name: "b", IdempotencyKey: "k1"}, "u")
	if err != nil {
		t.Fatalf("expired idem must take create path: %v", err)
	}
	if second.ID == first.ID || second.Reused {
		t.Fatalf("expired idem created new sandbox %+v", second)
	}
}

// 2b. 幂等命中但 GetSandbox 失败（store 读错误）→ 走原创建路径。
func TestC2_IdemHitStoreErrorTakesCreatePath(t *testing.T) {
	m, store := c2Manager(t)
	ctx := context.Background()
	first, err := m.Create(ctx, "p1", CreateInput{Name: "a", IdempotencyKey: "k1"}, "u")
	if err != nil {
		t.Fatal(err)
	}
	// 幂等命中后让 GetSandbox 报错：必须降级为重新创建而非返回错误。
	store.getByID = func(string) (Sandbox, error) { return Sandbox{}, errors.New("store down") }
	second, err := m.Create(ctx, "p1", CreateInput{Name: "b", IdempotencyKey: "k1"}, "u")
	if err != nil {
		t.Fatalf("store read failure on idem hit must fall through to create: %v", err)
	}
	if second.ID == first.ID || second.Reused {
		t.Fatalf("expected fresh sandbox on store error, got %+v", second)
	}
}

// 3. 同名冲突优先于配额（配额满时同名先报 ErrNameConflict）。
func TestC2_NameConflictBeatsQuota(t *testing.T) {
	m, _ := c2Manager(t)
	ctx := context.Background()
	if _, err := m.Create(ctx, "p1", CreateInput{Name: "a"}, "u"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, "p1", CreateInput{Name: "b"}, "u"); err != nil {
		t.Fatal(err)
	}
	// 配额已满（MaxPerProject=2）。重名必须报 ErrNameConflict 而非 ErrLimitExceeded。
	_, err := m.Create(ctx, "p1", CreateInput{Name: "a"}, "u")
	if !errors.Is(err, ErrNameConflict) {
		t.Fatalf("name conflict must precede quota, got %v", err)
	}
	// 新名字才报配额。
	_, err = m.Create(ctx, "p1", CreateInput{Name: "c"}, "u")
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("quota: %v", err)
	}
}

// 4. 创建失败不记录幂等条目：同 key 重新创建成功。
func TestC2_FailedCreateDoesNotRecordIdem(t *testing.T) {
	m, store := c2Manager(t)
	ctx := context.Background()
	// 让第一次创建在资源校验失败（image 不允许）。
	_, err := m.Create(ctx, "p1", CreateInput{Name: "a", Image: "bad-img", IdempotencyKey: "k1"}, "u")
	if err == nil || !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("bad image: %v", err)
	}
	// 同 key 再来一次（合法 image）：必须创建成功且非 Reused。
	got, err := m.Create(ctx, "p1", CreateInput{Name: "a", IdempotencyKey: "k1"}, "u")
	if err != nil {
		t.Fatalf("same key after failure must create: %v", err)
	}
	if got.Reused {
		t.Fatalf("failed create must not record idem entry")
	}
	if store.count != 1 {
		t.Fatalf("store created=%d want 1", store.count)
	}
}

// 5. 随机名称格式：sbx- 前缀 + hex 后缀；仅断言格式不固定值。
func TestC2_RandomNameFormat(t *testing.T) {
	m, _ := c2Manager(t)
	ctx := context.Background()
	sb, err := m.Create(ctx, "p1", CreateInput{}, "u")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sb.Name, "sbx-") || len(sb.Name) != len("sbx-")+6 {
		t.Fatalf("random name format: %q", sb.Name)
	}
}

// 6. Start=false 保持 pending；非法参数各自拒绝。
func TestC2_PendingAndValidationRejections(t *testing.T) {
	m, _ := c2Manager(t)
	ctx := context.Background()
	sb, err := m.Create(ctx, "p1", CreateInput{Name: "a"}, "u")
	if err != nil {
		t.Fatal(err)
	}
	if sb.Status != "pending" {
		t.Fatalf("Start=false must stay pending, got %s", sb.Status)
	}

	for name, in := range map[string]CreateInput{
		"bad image":    {Name: "x1", Image: "not-allowed"},
		"bad network":  {Name: "x2", Network: "bridge"},
		"bad cpus":     {Name: "x3", CPUs: 99},
		"bad memory":   {Name: "x4", MemoryMiB: 8},
		"bad idle":     {Name: "x5", IdleTimeoutS: 99999},
		"bad idem len": {Name: "x6", IdempotencyKey: strings.Repeat("k", 129)},
	} {
		if _, err := m.Create(ctx, "p1", in, "u"); !errors.Is(err, ErrInvalidSpec) {
			t.Fatalf("%s: want ErrInvalidSpec, got %v", name, err)
		}
	}
}

// 7. TTL 边界：until == now 时 Before 为 false → 不复用，走创建路径。
func TestC2_TTLBoundaryAtExactlyNow(t *testing.T) {
	m, _ := c2Manager(t)
	ctx := context.Background()
	if _, err := m.Create(ctx, "p1", CreateInput{Name: "a", IdempotencyKey: "k1"}, "u"); err != nil {
		t.Fatal(err)
	}
	// 固定时钟到 TTL 边界：until == now → Before(now)=false → 创建路径。
	frozen := m.now().Add(24 * time.Hour)
	m.now = func() time.Time { return frozen }
	sb, err := m.Create(ctx, "p1", CreateInput{Name: "b", IdempotencyKey: "k1"}, "u")
	if err != nil {
		t.Fatalf("TTL boundary must take create path: %v", err)
	}
	if sb.Reused {
		t.Fatalf("until==now must not reuse (Before is strict), got %+v", sb)
	}
}
