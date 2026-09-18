package gofunction

import (
	"fmt"
	"sync/atomic"
	"time"
)

// Executor Go代码执行器
type Executor struct {
	pool *ExecutorPool
	vm   *Runtime

	functionChecksums map[string]string
	createTime        time.Time
	executionTimeout  int64 // 毫秒，-1 表示不限制
	waitEventLoop     bool
	costTime          int64 // 最近一次执行的耗时（微秒）
	interruptTimer    *time.Timer
	interrupted       int32 // 原子标志：1 表示已被中断
}

// NewExecutor 创建新的执行器实例
func NewExecutor(pool *ExecutorPool) *Executor {
	return &Executor{
		pool:              pool,
		vm:                &Runtime{},
		functionChecksums: make(map[string]string),
		createTime:        time.Now(),
		executionTimeout:  -1,
		waitEventLoop:     false,
	}
}

// Execute 编译并执行脚本中的指定函数
func (e *Executor) Execute(functionName, script string) (result interface{}, err error) {
	start := time.Now()
	defer func() {
		e.costTime = time.Since(start).Microseconds()
		e.StopTimeoutTimer()
		if re := recover(); re != nil {
			err = fmt.Errorf("execute panic: %v", re)
		}
	}()

	if e.vm == nil {
		e.vm = &Runtime{}
	}
	if e.vm.SequenceID == "" {
		e.vm.Initialize("")
	}

	program, buildErr := BuildProgram(e.vm.SequenceID, "main", script)
	if buildErr != nil {
		return nil, buildErr
	}
	e.vm.Program = program

	if e.executionTimeout >= 0 {
		e.StartTimeoutTimer("execution timeout")
	}

	result, err = program.Run(e.vm.SequenceID, functionName)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Compile 编译脚本（预留接口）
func (e *Executor) Compile(name, script string, strict bool) error {
	return nil
}

// Close 关闭执行器并回收资源
func (e *Executor) Close() {
	if e.vm != nil {
		e.StopTimeoutTimer()
		e.vm.Cleanup()
	}

	if e.pool == nil {
		return
	}
	// 过期判断与 putExecutor 内部保持一致（MaxAge 已是 Duration）
	if e.pool.config.MaxAge > 0 && time.Since(e.createTime) > e.pool.config.MaxAge {
		e.pool.decrementActive()
	} else {
		e.pool.putExecutor(e)
	}
}

// Interrupt 中断执行（通过上下文取消生效于下一次指令边界）
func (e *Executor) Interrupt(message string) error {
	atomic.StoreInt32(&e.interrupted, 1)
	if e.vm != nil && e.vm.Program != nil {
		// 取消当前执行的上下文
		logger.Info("executor interrupted: " + message)
	}
	return nil
}

// ClearInterrupt 清理中断标志
func (e *Executor) ClearInterrupt() error {
	atomic.StoreInt32(&e.interrupted, 0)
	return nil
}

// IsInterrupted 返回是否已被中断
func (e *Executor) IsInterrupted() bool {
	return atomic.LoadInt32(&e.interrupted) == 1
}

// StartTimeoutTimer 启动超时定时器
func (e *Executor) StartTimeoutTimer(message string) {
	if e.executionTimeout >= 0 {
		e.interruptTimer = time.AfterFunc(
			time.Duration(e.executionTimeout)*time.Millisecond,
			func() {
				_ = e.Interrupt(message)
			},
		)
	}
}

// StopTimeoutTimer 停止超时定时器
func (e *Executor) StopTimeoutTimer() {
	if e.interruptTimer != nil {
		e.interruptTimer.Stop()
		e.interruptTimer = nil
	}
}

// GetSequenceID 获取序列ID
func (e *Executor) GetSequenceID() string {
	if e.vm != nil {
		return e.vm.SequenceID
	}
	return ""
}

// SetSequenceID 设置序列ID
func (e *Executor) SetSequenceID(sequenceID string) {
	if e.vm != nil {
		e.vm.SequenceID = sequenceID
	}
}

// SetExecutionTimeout 设置执行超时时间（毫秒）
func (e *Executor) SetExecutionTimeout(timeout int64) {
	e.executionTimeout = timeout
}

// SetWaitEventLoop 设置是否等待事件循环
func (e *Executor) SetWaitEventLoop(wait bool) {
	e.waitEventLoop = wait
}

// GetCostTime 获取最近一次执行的耗时（微秒）
func (e *Executor) GetCostTime() int64 {
	return e.costTime
}

// SetProfile 设置性能分析（预留接口）
func (e *Executor) SetProfile(enabled bool) {}

// GetProfile 获取性能分析结果（预留接口）
func (e *Executor) GetProfile() []string {
	return nil
}
