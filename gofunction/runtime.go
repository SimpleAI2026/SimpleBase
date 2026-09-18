package gofunction

import (
	"fmt"
	"sync"
)

// Runtime Go代码运行时环境
type Runtime struct {
	Program *Program
	Async   RuntimeAsync

	// 用户私有数据
	userData      map[string]interface{}
	userDataMutex sync.RWMutex

	// 系统成员
	SequenceID string
}

// RuntimeAsync 异步运行时
type RuntimeAsync struct {
	waitGroup sync.WaitGroup
	isAsync   bool
}

// RuntimeTaskInfo 运行时任务信息
type RuntimeTaskInfo struct {
	Result interface{}
	Error  error
}

// NewRuntime 创建新的运行时实例
func NewRuntime() *Runtime {
	return &Runtime{
		userData: make(map[string]interface{}),
	}
}

// Initialize 初始化运行时。sequenceID 由调用方提供（如请求 ID）；
// 传入空字符串时自动生成。
func (r *Runtime) Initialize(sequenceID string) {
	r.SequenceID = sequenceID
	if r.SequenceID == "" {
		r.SequenceID = newSequenceID()
	}
	r.userData = make(map[string]interface{})
}

// GetUserData 获取用户数据
func (r *Runtime) GetUserData(key string) (interface{}, bool) {
	r.userDataMutex.RLock()
	defer r.userDataMutex.RUnlock()

	if r.userData == nil {
		return nil, false
	}
	v, exists := r.userData[key]
	return v, exists
}

// SetUserData 设置用户数据
func (r *Runtime) SetUserData(key string, value interface{}) {
	r.userDataMutex.Lock()
	defer r.userDataMutex.Unlock()

	if r.userData == nil {
		r.userData = make(map[string]interface{})
	}
	r.userData[key] = value
}

// ConsoleLog 控制台日志输出（写入当前 Program 的执行输出缓冲区）
func (r *Runtime) ConsoleLog(level int, tag string, data interface{}) {
	_ = level
	logger.Info(fmt.Sprintf("%v %v", tag, data))
}

// Reset 重置运行时状态
func (r *Runtime) Reset() {
	r.userDataMutex.Lock()
	defer r.userDataMutex.Unlock()

	r.userData = make(map[string]interface{})
	r.Program = nil
}

// Cleanup 清理运行时资源
func (r *Runtime) Cleanup() {
	// 预留：后续接入主工程可观测性时扩展
}
