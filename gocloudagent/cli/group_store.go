// 文件说明：group_store.go — 本地群的轮次 / 任务 / 事件持久化（实现 gocloudagent/group.Store，planv6.8 §13.8）
//
// 用法
//
//	openGroupStore(path) (*fileGroupStore, error)   // 打开（不存在则初始化）groups/<name>/state.json
//
// 状态整份落在 groups/<name>/state.json（临时文件 + rename 写入，0600）：
//   - turns：轮次（含决策、确认状态、再规划次数）+ 任务 + 事件（seq 单调，单轮最多保留最近 1000 条）；
//   - claims：会话占用（threadID → turnID），进程重启后清空（本进程没有活动轮次）。
// 单进程单写：所有方法在同一把锁下读改写；迁移以 from 为前置条件（乐观锁语义与宿主实现一致），
// 非法迁移返回 group.ErrIllegalTransition，不静默覆盖终态。计划文件（plan.md 等）另由 plan.FileStore 落盘。

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/gocloudagent/group"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
)

// groupStoreMaxEvents 是单轮保留的事件上限（超出丢最旧；终端不需要完整回放）。
const groupStoreMaxEvents = 1000

type storedTurn struct {
	Turn group.Turn `json:"turn"`
	// ErrorCode 是最近一次失败 / 取消的原因码（展示用；group.Turn 不承载它）。
	ErrorCode string        `json:"error_code,omitempty"`
	Tasks     []group.Task  `json:"tasks"`
	Events    []group.Event `json:"events"`
}

type groupState struct {
	Turns  map[string]*storedTurn `json:"turns"`
	Claims map[string]string      `json:"claims"`
}

// fileGroupStore 是 group.Store 的本地文件实现。
type fileGroupStore struct {
	path string
	mu   sync.Mutex
	st   groupState
}

var _ group.Store = (*fileGroupStore)(nil)

// openGroupStore 读取状态文件；不存在时初始化空状态并落盘。
func openGroupStore(path string) (*fileGroupStore, error) {
	s := &fileGroupStore{path: path, st: groupState{Turns: map[string]*storedTurn{}, Claims: map[string]string{}}}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, s.saveLocked()
	case err != nil:
		return nil, err
	}
	if err := json.Unmarshal(raw, &s.st); err != nil {
		return nil, err
	}
	if s.st.Turns == nil {
		s.st.Turns = map[string]*storedTurn{}
	}
	// 进程重启：内存里的执行循环已不存在，占用位随之失效；未终态轮次由 Engine.Converge 收敛。
	s.st.Claims = map[string]string{}
	return s, nil
}

// saveLocked 整份写回；调用方持有 s.mu。
func (s *fileGroupStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(&s.st)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".state.*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

// writes 执行一次读改写并落盘；write 返回 error 时状态不落盘但内存已改，调用方按错误处理。
func (s *fileGroupStore) update(write func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := write(); err != nil {
		return err
	}
	return s.saveLocked()
}

func (s *fileGroupStore) turnLocked(turnID string) *storedTurn {
	return s.st.Turns[turnID]
}

func (s *fileGroupStore) CreateTurn(_ context.Context, t group.Turn) (group.Turn, bool, error) {
	var reused bool
	err := s.update(func() error {
		if t.ID == "" {
			// 引擎不指定 ID：与宿主实现一致，由存储分配。
			t.ID = uuid.NewString()
		}
		if t.IdempotencyKey != "" {
			for _, st := range s.st.Turns {
				if st.Turn.ThreadID == t.ThreadID && st.Turn.IdempotencyKey == t.IdempotencyKey {
					t = st.Turn
					reused = true
					return nil
				}
			}
		}
		if _, dup := s.st.Turns[t.ID]; dup {
			t = s.st.Turns[t.ID].Turn
			reused = true
			return nil
		}
		if t.Status == "" {
			t.Status = group.TurnPlanning
		}
		if t.Version == 0 {
			t.Version = 1
		}
		now := time.Now().UTC()
		t.CreatedAt, t.UpdatedAt = now, now
		s.st.Turns[t.ID] = &storedTurn{Turn: t}
		return nil
	})
	return t, reused, err
}

func (s *fileGroupStore) TransitionTurn(_ context.Context, turnID, from, to, errorCode string) error {
	return s.update(func() error {
		st := s.turnLocked(turnID)
		if st == nil || st.Turn.Status != from {
			return group.ErrIllegalTransition
		}
		st.Turn.Status = to
		st.Turn.Version++
		st.Turn.UpdatedAt = time.Now().UTC()
		if errorCode == "" {
			st.ErrorCode = ""
		} else {
			st.ErrorCode = errorCode
		}
		return nil
	})
}

func (s *fileGroupStore) SaveDecision(_ context.Context, turnID string, d plan.Decision, confirmState string) error {
	return s.update(func() error {
		st := s.turnLocked(turnID)
		if st == nil {
			return group.ErrIllegalTransition
		}
		st.Turn.Decision = d
		st.Turn.ConfirmState = confirmState
		st.Turn.UpdatedAt = time.Now().UTC()
		return nil
	})
}

func (s *fileGroupStore) SaveReplans(_ context.Context, turnID string, replans int) error {
	return s.update(func() error {
		st := s.turnLocked(turnID)
		if st == nil {
			return group.ErrIllegalTransition
		}
		st.Turn.Replans = replans
		return nil
	})
}

func (s *fileGroupStore) LoadTurn(_ context.Context, turnID string) (group.Turn, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.turnLocked(turnID)
	if st == nil {
		return group.Turn{}, false, nil
	}
	return st.Turn, true, nil
}

func (s *fileGroupStore) CreateTasks(_ context.Context, turnID string, tasks []group.Task) error {
	return s.update(func() error {
		st := s.turnLocked(turnID)
		if st == nil {
			return group.ErrIllegalTransition
		}
		out := make([]group.Task, 0, len(tasks))
		for _, t := range tasks {
			if t.TurnID == "" {
				t.TurnID = turnID
			}
			out = append(out, t)
		}
		st.Tasks = out
		return nil
	})
}

func (s *fileGroupStore) TransitionTask(_ context.Context, taskID, from, to, result, errorCode string) error {
	return s.update(func() error {
		for _, st := range s.st.Turns {
			for i := range st.Tasks {
				if st.Tasks[i].ID != taskID {
					continue
				}
				if st.Tasks[i].Status != from {
					return group.ErrIllegalTransition
				}
				st.Tasks[i].Status = to
				st.Tasks[i].Version++
				st.Tasks[i].ErrorCode = errorCode
				if result != "" {
					st.Tasks[i].Result = result
				}
				return nil
			}
		}
		return group.ErrIllegalTransition
	})
}

func (s *fileGroupStore) SetTaskRunID(_ context.Context, taskID, runID string) error {
	return s.update(func() error {
		for _, st := range s.st.Turns {
			for i := range st.Tasks {
				if st.Tasks[i].ID == taskID {
					st.Tasks[i].RunID = runID
					return nil
				}
			}
		}
		return group.ErrIllegalTransition
	})
}

func (s *fileGroupStore) LoadTasks(_ context.Context, turnID string) ([]group.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.turnLocked(turnID)
	if st == nil {
		return nil, nil
	}
	return append([]group.Task(nil), st.Tasks...), nil
}

func (s *fileGroupStore) AppendEvent(_ context.Context, turnID string, e group.Event) (group.Event, error) {
	err := s.update(func() error {
		st := s.turnLocked(turnID)
		if st == nil {
			return group.ErrIllegalTransition
		}
		e.TurnID = turnID
		e.Seq = len(st.Events) + 1
		st.Events = append(st.Events, e)
		if len(st.Events) > groupStoreMaxEvents {
			st.Events = append([]group.Event(nil), st.Events[len(st.Events)-groupStoreMaxEvents:]...)
			for i := range st.Events {
				st.Events[i].Seq = i + 1
			}
			e = st.Events[len(st.Events)-1]
		}
		return nil
	})
	return e, err
}

func (s *fileGroupStore) ListEvents(_ context.Context, turnID string, after int) ([]group.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.turnLocked(turnID)
	if st == nil {
		return nil, nil
	}
	out := make([]group.Event, 0, len(st.Events))
	for _, e := range st.Events {
		if e.Seq > after {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *fileGroupStore) Claim(_ context.Context, threadID, turnID string) error {
	return s.update(func() error {
		if other, busy := s.st.Claims[threadID]; busy && other != turnID {
			return group.ErrTurnBusy
		}
		s.st.Claims[threadID] = turnID
		return nil
	})
}

func (s *fileGroupStore) Release(_ context.Context, threadID, turnID string) error {
	return s.update(func() error {
		if s.st.Claims[threadID] == turnID {
			delete(s.st.Claims, threadID)
		}
		return nil
	})
}

func (s *fileGroupStore) ListUnfinishedTurns(_ context.Context) ([]group.Turn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []group.Turn
	for _, st := range s.st.Turns {
		if !group.TerminalTurn(st.Turn.Status) {
			out = append(out, st.Turn)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// ListResumableTurns 返回可由用户显式恢复的轮次（非终态 + interrupted），时间升序。
// 与 ListUnfinishedTurns（只给 Converge 用）分开：interrupted 是终态，但 /group resume 应当认它。
func (s *fileGroupStore) ListResumableTurns(_ context.Context) ([]group.Turn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []group.Turn
	for _, st := range s.st.Turns {
		if !group.TerminalTurn(st.Turn.Status) || st.Turn.Status == group.TurnInterrupted {
			out = append(out, st.Turn)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
