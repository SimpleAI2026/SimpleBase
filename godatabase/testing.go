// 文件说明：testing.go — godatabase 中 RecordingLogger、LogEntry 等符号的实现
//
// 用法
//
//	(l *RecordingLogger) Debug(msg string, f ...zap.Field)
//	(l *RecordingLogger) Info(msg string, f ...zap.Field)
//
// 调用架构
//
//	[上游] godatabase/cache, godatabase/ducklake, godatabase/filedb 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/testing.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] dialect.go, query.go, runtime.go, serialize.go, snapshot.go 等 6 个
//
// 主要符号
//
//	type RecordingLogger struct {…}
//	type LogEntry struct {…}
//	(l *RecordingLogger) Debug(msg string, f ...zap.Field)
//	(l *RecordingLogger) Info(msg string, f ...zap.Field)
//	(l *RecordingLogger) Warn(msg string, f ...zap.Field)
//	(l *RecordingLogger) Error(msg string, f ...zap.Field)
//	... 另有 18 个顶层符号

package godatabase

import (
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// 本文件提供库自身与各子包测试共用的记录型 Logger / Observer。
// 它们是导出的普通类型（非 _test.go），子包的测试才能引用；不含任何业务依赖，开销可忽略。

// RecordingLogger 把日志条目记入内存，便于断言。并发安全。
type RecordingLogger struct {
	mu      sync.Mutex
	entries []LogEntry
}

// LogEntry 是 RecordingLogger 捕获的一条日志。
type LogEntry struct {
	Level  string
	Msg    string
	Fields []zap.Field
}

func (l *RecordingLogger) add(level, msg string, f []zap.Field) {
	l.mu.Lock()
	l.entries = append(l.entries, LogEntry{Level: level, Msg: msg, Fields: f})
	l.mu.Unlock()
}

func (l *RecordingLogger) Debug(msg string, f ...zap.Field) { l.add("debug", msg, f) }
func (l *RecordingLogger) Info(msg string, f ...zap.Field)  { l.add("info", msg, f) }
func (l *RecordingLogger) Warn(msg string, f ...zap.Field)  { l.add("warn", msg, f) }
func (l *RecordingLogger) Error(msg string, f ...zap.Field) { l.add("error", msg, f) }

// Entries 返回已记录日志的拷贝。
func (l *RecordingLogger) Entries() []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]LogEntry(nil), l.entries...)
}

// Count 返回指定级别与消息前缀匹配的条数；level 为空表示不限级别。
func (l *RecordingLogger) Count(level, msgPrefix string) int {
	n := 0
	for _, e := range l.Entries() {
		if (level == "" || e.Level == level) && len(e.Msg) >= len(msgPrefix) && e.Msg[:len(msgPrefix)] == msgPrefix {
			n++
		}
	}
	return n
}

// RecordingObserver 记录 Observer 的全部调用。并发安全。
type RecordingObserver struct {
	Handles      atomic.Int64
	Bytes        atomic.Int64
	Evictions    atomic.Int64
	SyncOK       atomic.Int64
	SyncFailed   atomic.Int64
	LocalAhead   atomic.Int64
	Conflicts    atomic.Int64
	LastSyncDur  atomic.Int64
	mu           sync.Mutex
	lag          map[string]int64
	catalogBytes map[string]int64
	conflictByDB map[string]int64
	lastOutcome  string
}

func (o *RecordingObserver) OpenHandleDelta(d int) { o.Handles.Add(int64(d)) }
func (o *RecordingObserver) CacheBytes(b int64)    { o.Bytes.Store(b) }
func (o *RecordingObserver) CacheEvicted()         { o.Evictions.Add(1) }
func (o *RecordingObserver) CatalogLocalAhead()    { o.LocalAhead.Add(1) }

func (o *RecordingObserver) CatalogSync(outcome string, d time.Duration, failed bool) {
	if failed {
		o.SyncFailed.Add(1)
	} else {
		o.SyncOK.Add(1)
	}
	o.LastSyncDur.Store(int64(d))
	o.mu.Lock()
	o.lastOutcome = outcome
	o.mu.Unlock()
}

func (o *RecordingObserver) CatalogSyncLag(dbID string, lag int64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.lag == nil {
		o.lag = map[string]int64{}
	}
	o.lag[dbID] = lag
}

func (o *RecordingObserver) CatalogSyncConflict(dbID string) {
	o.Conflicts.Add(1)
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.conflictByDB == nil {
		o.conflictByDB = map[string]int64{}
	}
	o.conflictByDB[dbID]++
}

func (o *RecordingObserver) CatalogSize(dbID string, b int64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.catalogBytes == nil {
		o.catalogBytes = map[string]int64{}
	}
	o.catalogBytes[dbID] = b
}

// Lag 返回最近一次上报的某库同步滞后。
func (o *RecordingObserver) Lag(dbID string) (int64, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	v, ok := o.lag[dbID]
	return v, ok
}

// CatalogBytes 返回最近一次上报的某库 catalog 体积。
func (o *RecordingObserver) CatalogBytes(dbID string) (int64, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	v, ok := o.catalogBytes[dbID]
	return v, ok
}

// LastOutcome 返回最近一次 CatalogSync 的 outcome。
func (o *RecordingObserver) LastOutcome() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.lastOutcome
}

var (
	_ Logger   = (*RecordingLogger)(nil)
	_ Observer = (*RecordingObserver)(nil)
	_ Observer = NopObserver{}
)
