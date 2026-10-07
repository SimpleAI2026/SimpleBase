// Package lease 实现 per-database 写租约（multi-instance-consistency-plan §4.6）。
//
// 只依赖对象存储的 create-if-absent（PutIfAbsent）一个条件原语；
// 判活采用「epoch 观察式」：本地单调时钟测等待窗口，前后两次观察服务端
// 最大 epoch 是否变化，不做任何跨时钟减法（不受时钟漂移影响）。
//
// epoch 单调递增本身就是 fencing：CatalogSyncer 的快照对象 key 含
// writer_epoch，被抢占的旧 writer 写出的对象不覆盖任何东西、且一眼可辨。
package lease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// Config 控制租约行为（对应 config.InstanceLeaseConfig）。
type Config struct {
	Enabled       bool          // 仅 writable=true 且配置了 S3 时生效
	TTL           time.Duration // 持租判活窗口；<=0 用默认 30s
	RenewInterval time.Duration // 续约间隔；<=0 用 ttl/3
	Grace         time.Duration // 观察式判活的额外等待；<=0 用默认 10s
}

func (c Config) normalized() Config {
	if c.TTL <= 0 {
		c.TTL = 30 * time.Second
	}
	if c.RenewInterval <= 0 {
		c.RenewInterval = c.TTL / 3
	}
	if c.Grace <= 0 {
		c.Grace = 10 * time.Second
	}
	return c
}

// Payload 是 lease/{epoch}.json 的载荷。
type Payload struct {
	OwnerID    string `json:"owner_id"` // instance.id + pid + bootID（判「自己是持租者」用）
	InstanceID string `json:"instance_id"`
	Hostname   string `json:"hostname"`
	PID        int    `json:"pid"`
	BootID     string `json:"boot_id,omitempty"` // 进程启动 UUID：区分重启/容器共享 PID 的实例（§3.4）
	AcquiredAt string `json:"acquired_at"`
	RenewedAt  string `json:"renewed_at"`
	TTLMS      int64  `json:"ttl_ms"`
	Epoch      int64  `json:"epoch"`
}

// ErrLeaseHeld 表示租约被其他实例持有。错误信息含当前 owner。
var ErrLeaseHeld = errors.New("lease: held by another instance")

// ErrAcquiring 表示租约正处于观察式判活的等待窗口（同进程并发首写
// 已由 Gate 串行化；此错误只出现在等待者身上，供 API 层返回 503+Retry-After）。
var ErrAcquiring = errors.New("lease: acquisition in progress")

// IsHeld 判定错误是否为「他人持租」。
func IsHeld(err error) bool {
	return errors.Is(err, ErrLeaseHeld)
}

// Observer 抽象对租约对象的探测（便于测试注入 fake 时钟与 store）。
type storeProxy struct {
	store   objectstore.BlobStore
	keys    objectstore.KeyBuilder
	tenant  string
	db      string
	ownerID string
}

func (p *storeProxy) get(ctx context.Context, epoch int64) (*Payload, error) {
	key, err := p.keys.DuckLakeLeaseKey(p.tenant, p.db, epoch)
	if err != nil {
		return nil, err
	}
	data, _, err := p.store.GetBytes(ctx, key)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var pl Payload
	if err := json.Unmarshal(data, &pl); err != nil {
		return nil, fmt.Errorf("lease: parse epoch %d: %w", epoch, err)
	}
	return &pl, nil
}

func (p *storeProxy) putIfAbsent(ctx context.Context, epoch int64, pl Payload) error {
	key, err := p.keys.DuckLakeLeaseKey(p.tenant, p.db, epoch)
	if err != nil {
		return err
	}
	pl.Epoch = epoch
	body, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	if _, err := p.store.PutIfAbsent(ctx, key, body, "application/json"); err != nil {
		if errors.Is(err, objectstore.ErrPreconditionFailed) {
			return objectstore.ErrPreconditionFailed
		}
		return err
	}
	return nil
}

// Manager 管理一批 per-database 租约并驱动续约循环。
type Manager struct {
	store    objectstore.BlobStore
	keys     objectstore.KeyBuilder
	cfg      Config
	hostname string
	pid      int
	bootID   string // 进程启动 UUID（§3.4：instance.id+pid 无法区分重启/共享 PID 的实例）
	logger   Logger // 可选日志；nil 时静默

	mu       sync.Mutex
	leases   map[string]*Lease
	stopCh   chan struct{}
	stopOnce sync.Once
}

// Logger 是租约包的最小日志接口（避免依赖具体实现）。
type Logger interface {
	Printf(format string, args ...any)
}

// NewManager 构造租约管理器。
func NewManager(store objectstore.BlobStore, keys objectstore.KeyBuilder, cfg Config) *Manager {
	host, _ := os.Hostname()
	return &Manager{
		store:    store,
		keys:     keys,
		cfg:      cfg.normalized(),
		hostname: host,
		pid:      os.Getpid(),
		bootID:   uuid.NewString(),
		leases:   map[string]*Lease{},
		stopCh:   make(chan struct{}),
	}
}

// SetLogger 注入日志实现（nil 表示静默）。
func (m *Manager) SetLogger(l Logger) {
	if m != nil {
		m.logger = l
	}
}

func (m *Manager) logf(format string, args ...any) {
	if m != nil && m.logger != nil {
		m.logger.Printf(format, args...)
	}
}

// Lease 是单个库的持租状态。
type Lease struct {
	DatabaseID string
	TenantID   string
	Epoch      int64

	mu     sync.Mutex
	held   bool
	cancel context.CancelFunc
	onLost func(dbID string)
}

// Acquire 为 (tenantID, dbID) 获取写租约。
//
// 算法（§4.6）：
//  1. 从 epoch 1 向前探测到最大 epoch E（首个 404 停止）；
//  2. E 不存在 → PutIfAbsent(lease/0001)；
//  3. E 存在且 owner 是自己 → PutIfAbsent(lease/{E+1}) 续租接管；
//  4. E 存在且 owner 是他人 → 观察式判活：
//     记 obs0=E，sleep(ttl+grace)（本地单调时钟），再探测 obs1；
//     obs1==obs0 → 持租者停止续约 → 抢占 E+1；obs1>obs0 → ErrLeaseHeld。
//
// 成功后启动续约 goroutine（每 RenewInterval 写 lease/{epoch+1}），
// 409/412 → 被抢占 → 调用 onLost(dbID)。
func (m *Manager) Acquire(ctx context.Context, tenantID, dbID, instanceID string, onLost func(dbID string)) (*Lease, error) {
	if m == nil || !m.cfg.Enabled {
		return nil, errors.New("lease: manager disabled")
	}
	p := &storeProxy{
		store:   m.store,
		keys:    m.keys,
		tenant:  tenantID,
		db:      dbID,
		ownerID: fmt.Sprintf("%s/%d/%s", instanceID, m.pid, m.bootID),
	}
	pl := Payload{
		OwnerID:    p.ownerID,
		InstanceID: instanceID,
		Hostname:   m.hostname,
		PID:        m.pid,
		BootID:     m.bootID,
		TTLMS:      m.cfg.TTL.Milliseconds(),
	}

	maxEpoch, cur, err := m.probeMax(ctx, p)
	if err != nil {
		return nil, err
	}

	next := maxEpoch + 1
	if maxEpoch > 0 && cur != nil && cur.OwnerID != p.ownerID {
		// 同 PID 重启（ownerID 前缀一致但 bootID 不同）：明确提示而非含糊的「他人持租」。
		if cur.InstanceID == instanceID && cur.PID == m.pid && cur.BootID != m.bootID {
			m.logf("lease: previous holder has same instance/pid (crash or restart of this process), waiting for TTL=%s grace=%s before takeover", m.cfg.TTL, m.cfg.Grace)
		}
		// 他人持有：观察式判活。
		obs0 := maxEpoch
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(m.cfg.TTL + m.cfg.Grace):
		}
		obs1, cur1, err := m.probeMax(ctx, p)
		if err != nil {
			return nil, err
		}
		if obs1 > obs0 {
			return nil, fmt.Errorf("%w: owner=%s instance=%s host=%s pid=%d epoch=%d",
				ErrLeaseHeld, cur1.OwnerID, cur1.InstanceID, cur1.Hostname, cur1.PID, obs1)
		}
		// 持租者停止续约：抢占。
		next = obs0 + 1
	}

	pl.AcquiredAt = time.Now().UTC().Format(time.RFC3339Nano)
	pl.RenewedAt = pl.AcquiredAt
	if err := p.putIfAbsent(ctx, next, pl); err != nil {
		if errors.Is(err, objectstore.ErrPreconditionFailed) {
			return nil, fmt.Errorf("%w: epoch=%d concurrently taken", ErrLeaseHeld, next)
		}
		return nil, err
	}

	l := &Lease{DatabaseID: dbID, TenantID: tenantID, Epoch: next, held: true, onLost: onLost}
	m.mu.Lock()
	m.leases[dbID] = l
	m.mu.Unlock()

	ctx2, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	go m.renewLoop(ctx2, p, l, pl)
	return l, nil
}

// probeMax 用指数探测 + 二分定位当前租约 epoch，避免长时运行后线性 GET。
// 依赖租约对象从 1 连续且不可删除的协议；外部删除历史对象不受支持。
func (m *Manager) probeMax(ctx context.Context, p *storeProxy) (int64, *Payload, error) {
	last, err := p.get(ctx, 1)
	if err != nil || last == nil {
		return 0, nil, err
	}
	lo, hi := int64(1), int64(2)
	for hi <= 1_000_000 {
		pl, err := p.get(ctx, hi)
		if err != nil {
			return 0, nil, err
		}
		if pl == nil {
			break
		}
		lo, last = hi, pl
		hi *= 2
	}
	if hi > 1_000_000 {
		hi = 1_000_001
	}
	for hi-lo > 1 {
		mid := lo + (hi-lo)/2
		pl, err := p.get(ctx, mid)
		if err != nil {
			return 0, nil, err
		}
		if pl == nil {
			hi = mid
		} else {
			lo, last = mid, pl
		}
	}
	return lo, last, nil
}

// renewLoop 每 RenewInterval 用 PutIfAbsent 写 lease/{epoch+1}；
// 冲突 → 被抢占 → 标失租并回调 onLost。
func (m *Manager) renewLoop(ctx context.Context, p *storeProxy, l *Lease, pl Payload) {
	ticker := time.NewTicker(m.cfg.RenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
		}
		l.mu.Lock()
		next := l.Epoch + 1
		pl2 := pl
		pl2.RenewedAt = time.Now().UTC().Format(time.RFC3339Nano)
		err := p.putIfAbsent(ctx, next, pl2)
		if err == nil {
			l.Epoch = next
			l.mu.Unlock()
			continue
		}
		// §3.4：任何续写失败（含网络错误）都无法再从远端权威水位证明
		// 自己持租——fail closed：标失租并回调 onLost，而不是只在
		// precondition 冲突时才处理（网络错误后旧 handle 继续写无保护）。
		l.held = false
		l.mu.Unlock()
		if l.onLost != nil {
			l.onLost(l.DatabaseID)
		}
		return
	}
}

// Held 返回该库当前是否持租。
func (l *Lease) Held() bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.held
}

// Stop 释放（停止续约）。不删除对象——留给接管者按观察式判活回收。
func (l *Lease) Stop() {
	if l == nil {
		return
	}
	l.mu.Lock()
	if l.cancel != nil {
		l.cancel()
	}
	l.held = false
	l.mu.Unlock()
}

// ValidFor 返回指定库当前是否持租（维护任务门禁用）。
func (m *Manager) ValidFor(dbID string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	l := m.leases[dbID]
	return l != nil && l.Held()
}

// Has 返回指定库是否存在租约记录（无论是否仍持租；失租后仍为 true）。
func (m *Manager) Has(dbID string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.leases[dbID]
	return ok
}

// EpochFor 返回指定库当前租约 epoch；无记录返回 0。
func (m *Manager) EpochFor(dbID string) int64 {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	l := m.leases[dbID]
	m.mu.Unlock()
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.Epoch
}

// StopAll 停止全部续约（进程关闭时）。
func (m *Manager) StopAll() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() { close(m.stopCh) })
	m.mu.Lock()
	leases := make([]*Lease, 0, len(m.leases))
	for _, l := range m.leases {
		leases = append(leases, l)
	}
	m.mu.Unlock()
	for _, l := range leases {
		l.Stop()
	}
}
