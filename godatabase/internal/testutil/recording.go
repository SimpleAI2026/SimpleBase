// Package testutil contains shared test doubles for godatabase packages.
package testutil

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"go.uber.org/zap"
)

// RecordingLogger is a concurrency-safe in-memory Logger.
type RecordingLogger struct {
	mu      sync.Mutex
	entries []LogEntry
}

// LogEntry is one captured log entry.
type LogEntry struct {
	Level  string
	Msg    string
	Fields []zap.Field
}

func (l *RecordingLogger) add(level, msg string, fields []zap.Field) {
	l.mu.Lock()
	l.entries = append(l.entries, LogEntry{Level: level, Msg: msg, Fields: fields})
	l.mu.Unlock()
}

func (l *RecordingLogger) Debug(msg string, fields ...zap.Field) { l.add("debug", msg, fields) }
func (l *RecordingLogger) Info(msg string, fields ...zap.Field)  { l.add("info", msg, fields) }
func (l *RecordingLogger) Warn(msg string, fields ...zap.Field)  { l.add("warn", msg, fields) }
func (l *RecordingLogger) Error(msg string, fields ...zap.Field) { l.add("error", msg, fields) }

// Entries returns a copy of the captured entries.
func (l *RecordingLogger) Entries() []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]LogEntry(nil), l.entries...)
}

// Count returns the number of entries matching level and message prefix.
// An empty level matches all levels.
func (l *RecordingLogger) Count(level, msgPrefix string) int {
	n := 0
	for _, entry := range l.Entries() {
		if (level == "" || entry.Level == level) && strings.HasPrefix(entry.Msg, msgPrefix) {
			n++
		}
	}
	return n
}

// RecordingObserver is a concurrency-safe in-memory Observer.
type RecordingObserver struct {
	Handles    atomic.Int64
	Bytes      atomic.Int64
	Evictions  atomic.Int64
	SyncOK     atomic.Int64
	SyncFailed atomic.Int64
	LocalAhead atomic.Int64
	Conflicts  atomic.Int64

	mu           sync.Mutex
	lag          map[string]int64
	catalogBytes map[string]int64
	lastOutcome  string
}

func (o *RecordingObserver) OpenHandleDelta(delta int) { o.Handles.Add(int64(delta)) }
func (o *RecordingObserver) CacheBytes(bytes int64)    { o.Bytes.Store(bytes) }
func (o *RecordingObserver) CacheEvicted()             { o.Evictions.Add(1) }
func (o *RecordingObserver) CatalogLocalAhead()        { o.LocalAhead.Add(1) }

func (o *RecordingObserver) CatalogSync(outcome string, _ time.Duration, failed bool) {
	if failed {
		o.SyncFailed.Add(1)
	} else {
		o.SyncOK.Add(1)
	}
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

func (o *RecordingObserver) CatalogSyncConflict(string) { o.Conflicts.Add(1) }

func (o *RecordingObserver) CatalogSize(dbID string, bytes int64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.catalogBytes == nil {
		o.catalogBytes = map[string]int64{}
	}
	o.catalogBytes[dbID] = bytes
}

// Lag returns the most recently reported lag for dbID.
func (o *RecordingObserver) Lag(dbID string) (int64, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	value, ok := o.lag[dbID]
	return value, ok
}

// CatalogBytes returns the most recently reported catalog size for dbID.
func (o *RecordingObserver) CatalogBytes(dbID string) (int64, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	value, ok := o.catalogBytes[dbID]
	return value, ok
}

// LastOutcome returns the outcome passed to the most recent CatalogSync call.
func (o *RecordingObserver) LastOutcome() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.lastOutcome
}

var (
	_ godatabase.Logger   = (*RecordingLogger)(nil)
	_ godatabase.Observer = (*RecordingObserver)(nil)
)
