// 文件说明：runthread.go — 线程占用与运行取消

package cloudagent

import (
	"context"

	"go.uber.org/zap"
)

// ClaimThread 在持久化用户消息之前锁定会话，避免并发交错。
func (r *Runtime) ClaimThread(threadID, runID string) error {
	if r == nil {
		return &RunError{Code: "llm_not_configured", Message: "模型服务未配置"}
	}
	return r.claimThread(threadID, runID)
}

func (r *Runtime) claimThread(threadID, runID string) error {
	if threadID == "" || runID == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.threads == nil {
		r.threads = map[string]string{}
	}
	if active := r.threads[threadID]; active != "" && active != runID {
		r.trace().Warn("gocloudagent: thread busy",
			zap.String("thread", threadID),
			zap.String("run_id", runID),
			zap.String("active_run", active),
		)
		return ErrThreadBusy
	}
	r.threads[threadID] = runID
	return nil
}

// ReleaseThread 释放失败/取消/完成的会话运行锁。
func (r *Runtime) ReleaseThread(threadID, runID string) {
	if r != nil {
		r.releaseThread(threadID, runID)
	}
}

func (r *Runtime) releaseThread(threadID, runID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.threads[threadID] == runID {
		delete(r.threads, threadID)
	}
}

// IsRunActive 报告 run 是否仍在本进程内执行（持有会话锁）。
// 会话锁覆盖「落库前 Claim → StartRun → 收尾 Release」全过程，交互与定时任务两条路径都会登记。
func (r *Runtime) IsRunActive(runID string) bool {
	if r == nil || runID == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range r.threads {
		if id == runID {
			return true
		}
	}
	_, ok := r.cancels[runID]
	return ok
}

// CancelRun cancels an in-flight run.
func (r *Runtime) CancelRun(runID string) bool {
	if r == nil || runID == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.cancels[runID]
	if !ok {
		return false
	}
	c()
	return true
}

func (r *Runtime) storeCancel(id string, cancel context.CancelFunc) {
	if r == nil || id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancels == nil {
		r.cancels = map[string]context.CancelFunc{}
	}
	r.cancels[id] = cancel
}

func (r *Runtime) clearCancel(id string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cancels, id)
}
