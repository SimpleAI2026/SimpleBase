// 文件说明：bus.go — 进程内告警信号总线（production-readiness-plan §8.3.5）
//
// 用法
//
//	bus := alerts.NewBus(logger)
//	bus.Emit(alerts.Signal{Kind: "backup.failed", Severity: alerts.SeverityCritical})
//	cancel := bus.Subscribe(func(s alerts.Signal) { ... })
//
// 不引入 Alertmanager / PagerDuty 等外部组件（§6 已排除）：
// audit / metrics / 控制台 WS 三个订阅方在本进程内闭环。
package alerts

import (
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/internal/observability"
)

// Severity 取值。
const (
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// Signal 是一条告警信号。
type Signal struct {
	Kind     string // "auth.bruteforce" | "backup.failed" | "disk.critical" | "quota.exceeded" | "ratelimit.flood"
	Project  string
	Detail   string
	Severity string // warning | critical
	At       time.Time
}

// Normalize 填充缺省字段（severity 默认 warning；At 默认 now）。
func (s Signal) Normalize(now time.Time) Signal {
	if strings.TrimSpace(s.Severity) != SeverityCritical {
		s.Severity = SeverityWarning
	}
	if s.At.IsZero() {
		s.At = now
	}
	return s
}

// subscriberId 自增句柄。
type subscriber struct {
	id uint64
	fn func(Signal)
}

// Bus 是进程内告警总线：Emit 非阻塞（内部 ring buffer 256），订阅方同步回调。
type Bus struct {
	mu        sync.Mutex
	ring      [256]Signal
	head      int // 下一个写入位置
	count     int // 环内条数（≤256）
	subs      []subscriber
	nextSubID uint64
	dropped   int64 // Emit 在锁竞争下不丢弃；计数器仅为防御性观测
	logger    observability.Logger
}

// NewBus 构造总线；logger 可为 nil。
func NewBus(logger observability.Logger) *Bus {
	return &Bus{logger: logger}
}

// Emit 发布一条信号：写 ring buffer（最近 256 条）并同步通知订阅方。
// 回调内不允许再调用 Emit（会死锁——用 TryEmit 或异步包装）。
func (b *Bus) Emit(sig Signal) {
	sig = sig.Normalize(time.Now())
	b.mu.Lock()
	b.ring[b.head] = sig
	b.head = (b.head + 1) % len(b.ring)
	if b.count < len(b.ring) {
		b.count++
	}
	subs := make([]subscriber, len(b.subs))
	copy(subs, b.subs)
	b.mu.Unlock()
	for _, s := range subs {
		func() {
			defer func() {
				if r := recover(); r != nil && b.logger != nil {
					b.logger.Error("alert subscriber panic", zap.Any("recover", r))
				}
			}()
			s.fn(sig)
		}()
	}
}

// Subscribe 注册订阅方，返回取消函数。
func (b *Bus) Subscribe(fn func(Signal)) (cancel func()) {
	if fn == nil {
		return func() {}
	}
	b.mu.Lock()
	b.nextSubID++
	id := b.nextSubID
	b.subs = append(b.subs, subscriber{id: id, fn: fn})
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		for i, s := range b.subs {
			if s.id == id {
				b.subs = append(b.subs[:i], b.subs[i+1:]...)
				break
			}
		}
		b.mu.Unlock()
	}
}

// Recent 返回最近 n 条信号（新→旧；n 上限 256）。
func (b *Bus) Recent(n int) []Signal {
	if n <= 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if n > b.count {
		n = b.count
	}
	out := make([]Signal, 0, n)
	// head-1 是最新一条，向前回溯。
	idx := (b.head - 1 - (0) + len(b.ring)) % len(b.ring)
	for i := 0; i < n; i++ {
		out = append(out, b.ring[idx])
		idx = (idx - 1 + len(b.ring)) % len(b.ring)
	}
	return out
}
