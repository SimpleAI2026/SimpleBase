package sandbox

// c2_create_test.go：Manager.Create 的可观察行为（E2B 兼容 API 之后的语义）。
//   1. 同名冲突优先于配额（ErrNameConflict 先触发）；
//   2. 随机名称仅断言格式（sbx- 前缀 + 非空）；
//   3. Start=false 时保持 pending；Start=true 立即 running；
//   4. 非法 image / 保留 metadata 键 / 负数 timeout 各自拒绝；
//   5. timeout：0 取默认；超过 max_duration 时截断；
//   6. 规格由实例配置统一决定，网络恒为 none。

import (
	"context"
	"database/sql"
	"errors"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm/fake"
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

func (s *c2Store) GetSandboxByID(_ context.Context, id string) (Sandbox, error) {
	if sb, ok := s.byID[id]; ok {
		return sb, nil
	}
	return Sandbox{}, sql.ErrNoRows
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
	n := 0
	for _, sb := range s.byID {
		if sb.ProjectID == projectID {
			n++
		}
	}
	return n, nil
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
	m := NewManager(cfg, store, fake.New())
	return m, store
}

func TestC2_NameConflictBeforeLimit(t *testing.T) {
	m, _ := c2Manager(t)
	ctx := context.Background()
	for _, n := range []string{"a", "b"} {
		if _, err := m.Create(ctx, "p1", CreateInput{Name: n}, "u"); err != nil {
			t.Fatal(err)
		}
	}
	// 已满（上限 2）且名称冲突：先报冲突。
	if _, err := m.Create(ctx, "p1", CreateInput{Name: "a"}, "u"); !errors.Is(err, ErrNameConflict) {
		t.Fatalf("want ErrNameConflict, got %v", err)
	}
	if _, err := m.Create(ctx, "p1", CreateInput{Name: "c"}, "u"); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("want ErrLimitExceeded, got %v", err)
	}
}

func TestC2_RandomNameAndDefaults(t *testing.T) {
	m, _ := c2Manager(t)
	sb, err := m.Create(context.Background(), "p1", CreateInput{}, "u")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sb.Name, "sbx-") || len(sb.Name) <= 4 {
		t.Fatalf("random name = %q", sb.Name)
	}
	if sb.Status != "pending" || sb.Image != "default-img" || sb.CPUs != 1 || sb.MemoryMiB != 256 || sb.Network != "none" {
		t.Fatalf("defaults = %+v", sb)
	}
	if sb.IdleTimeoutS != 600 || sb.MaxDurationS != 3600 {
		t.Fatalf("timeouts = idle %d max %d", sb.IdleTimeoutS, sb.MaxDurationS)
	}
	if sb.Source != "api" || sb.ID == "" || sb.CloudName != CloudName(sb.ID) {
		t.Fatalf("identity = %+v", sb)
	}
}

func TestC2_StartTrueRuns(t *testing.T) {
	m, _ := c2Manager(t)
	sb, err := m.Create(context.Background(), "p1", CreateInput{Name: "a", Start: true}, "u")
	if err != nil {
		t.Fatal(err)
	}
	if sb.Status != "running" || sb.ExpiresAt == nil {
		t.Fatalf("start=true must run: %+v", sb)
	}
}

func TestC2_TimeoutClampedToMaxDuration(t *testing.T) {
	m, _ := c2Manager(t)
	ctx := context.Background()
	cases := map[string]struct {
		in   int64
		want int64
	}{"zero uses default": {0, 600}, "explicit": {120, 120}, "clamped": {99999, 3600}}
	i := 0
	for name, tc := range cases {
		i++
		sb, err := m.Create(ctx, "p"+strings.Repeat("x", i), CreateInput{Name: "t", TimeoutS: tc.in}, "u")
		if err != nil || sb.IdleTimeoutS != tc.want {
			t.Fatalf("%s: idle = %d err=%v, want %d", name, sb.IdleTimeoutS, err, tc.want)
		}
	}
	if _, err := m.Create(ctx, "pneg", CreateInput{Name: "t", TimeoutS: -1}, "u"); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("negative timeout: %v", err)
	}
}

func TestC2_InvalidInputsRejected(t *testing.T) {
	m, _ := c2Manager(t)
	ctx := context.Background()
	bad := map[string]CreateInput{
		"bad image":        {Name: "x1", Image: "bad-img"},
		"bad name":         {Name: "Bad_Name"},
		"reserved prefix":  {Name: "x2", Metadata: map[string]string{"simplebase.source": "agent"}},
		"reserved name":    {Name: "x3", Metadata: map[string]string{"name": "other"}},
		"empty key":        {Name: "x4", Metadata: map[string]string{"": "v"}},
		"control char val": {Name: "x5", Metadata: map[string]string{"k": "a\nb"}},
		"long value":       {Name: "x6", Metadata: map[string]string{"k": strings.Repeat("v", 1025)}},
	}
	for name, in := range bad {
		if _, err := m.Create(ctx, "pbad", in, "u"); !errors.Is(err, ErrInvalidSpec) {
			t.Errorf("%s: want ErrInvalidSpec, got %v", name, err)
		}
	}
	many := map[string]string{}
	for i := 0; i < 33; i++ {
		many[strings.Repeat("k", i+1)] = "v"
	}
	if _, err := m.Create(ctx, "pbad", CreateInput{Name: "x7", Metadata: many}, "u"); !errors.Is(err, ErrInvalidSpec) {
		t.Errorf("too many metadata entries: %v", err)
	}
	ok := map[string]string{"team": "a", "env": "prod"}
	sb, err := m.Create(ctx, "pbad", CreateInput{Name: "x8", Metadata: ok}, "u")
	if err != nil || sb.Metadata["team"] != "a" {
		t.Fatalf("valid metadata: %+v %v", sb, err)
	}
}

// 创建失败不留下半成品：非法输入不占用配额。
func TestC2_FailedCreateDoesNotConsumeQuota(t *testing.T) {
	m, store := c2Manager(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_, _ = m.Create(ctx, "p1", CreateInput{Name: "ok", Image: "bad"}, "u")
	}
	if store.count != 0 {
		t.Fatalf("failed creates consumed quota: %d", store.count)
	}
}
