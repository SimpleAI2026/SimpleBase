package gofunction

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// 池操作错误
var (
	ErrPoolClosed    = errors.New("executor pool closed")
	ErrPoolExhausted = errors.New("executor pool exhausted")
)

// Executor Go代码执行器
type Executor struct {
	pool *ExecutorPool
	vm   *Runtime
	// mu 保护 vm 的跨 goroutine 访问（超时定时器会并发调用 Interrupt）
	mu sync.RWMutex

	functionChecksums map[string]string
	createTime        time.Time
	executionTimeout  int64 // 毫秒，-1 表示不限制
	waitEventLoop     bool
	costTime          int64 // 最近一次执行的耗时（微秒）
	interruptTimer    *time.Timer
	interrupted       int32  // 原子标志：1 表示已被中断
	cancelFunc        func() // 当前执行上下文的取消函数（超时/Interrupt 真正生效，P2-4）
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

	e.mu.Lock()
	if e.vm == nil {
		e.vm = &Runtime{}
	}
	if e.vm.SequenceID == "" {
		e.vm.Initialize("")
	}
	seqid := e.vm.SequenceID
	e.mu.Unlock()

	program, buildErr := BuildProgram(seqid, "main", script)
	if buildErr != nil {
		return nil, buildErr
	}
	e.mu.Lock()
	e.vm.Program = program
	e.mu.Unlock()

	// 先创建执行上下文并注册取消函数，超时定时器经 Interrupt
	// 取消该 context 真正中断执行（P2-4）
	ctx := newCallContext()
	e.mu.Lock()
	e.cancelFunc = ctx.cancelFunc
	e.mu.Unlock()

	if e.executionTimeout >= 0 {
		e.StartTimeoutTimer("execution timeout")
	}

	result, err = program.runWithContext(seqid, functionName, ctx)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		err = fmt.Errorf("execution interrupted: %w", err)
	}
	return result, err
}

// Compile 编译脚本（预留接口）
func (e *Executor) Compile(name, script string, strict bool) error {
	return nil
}

// Close 关闭执行器并回收资源
func (e *Executor) Close() {
	e.mu.Lock()
	e.cancelFunc = nil
	if e.vm != nil {
		e.StopTimeoutTimer()
		e.vm.Cleanup()
	}
	e.mu.Unlock()

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

// Interrupt 中断执行：取消当前执行的上下文（P2-4 后真正生效，
// 于块边界被主循环检测到并 panic 终止）。
// 可能由超时定时器 goroutine 并发调用，访问共享字段需持有 mu。
func (e *Executor) Interrupt(message string) error {
	atomic.StoreInt32(&e.interrupted, 1)
	e.mu.RLock()
	hasProgram := e.vm != nil && e.vm.Program != nil
	cancel := e.cancelFunc
	e.mu.RUnlock()
	if hasProgram && cancel != nil {
		logger.Info("executor interrupted: " + message)
		cancel()
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
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.vm != nil {
		return e.vm.SequenceID
	}
	return ""
}

// SetSequenceID 设置序列ID
func (e *Executor) SetSequenceID(sequenceID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.vm == nil {
		e.vm = &Runtime{}
	}
	e.vm.SequenceID = sequenceID
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

// ExecutorPoolConfig 执行器池配置
type ExecutorPoolConfig struct {
	MaxIdle   int           // 最大空闲数
	MaxActive int           // 最大活跃数
	MaxAge    time.Duration // 最大存活时间
}

// DefaultExecutorPoolConfig 默认执行器池配置
var DefaultExecutorPoolConfig = ExecutorPoolConfig{
	MaxIdle:   10,
	MaxActive: 100,
	MaxAge:    30 * time.Minute,
}

// ExecutorPool 执行器对象池
type ExecutorPool struct {
	config ExecutorPoolConfig

	mutex       sync.Mutex
	activeCount int
	idleList    list.List
	serviceName string
	closed      bool
}

// NewExecutorPool 创建新的执行器池
func NewExecutorPool(config ExecutorPoolConfig) *ExecutorPool {
	return &ExecutorPool{
		config:      config,
		activeCount: 0,
	}
}

// NewDefaultExecutorPool 创建使用默认配置的执行器池
func NewDefaultExecutorPool() *ExecutorPool {
	return NewExecutorPool(DefaultExecutorPoolConfig)
}

// GetExecutor 从池中获取执行器
func (p *ExecutorPool) GetExecutor() (*Executor, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		return nil, ErrPoolClosed
	}

	// 尝试从空闲列表获取
	if element := p.idleList.Front(); element != nil {
		p.idleList.Remove(element)
		return element.Value.(*Executor), nil
	}

	// 检查是否可以创建新的执行器
	if p.activeCount < p.config.MaxActive {
		p.activeCount++
		return p.createNewExecutor(), nil
	}

	// 池已满
	return nil, ErrPoolExhausted
}

// putExecutor 将执行器放回池中（调用方不得持有 p.mutex）
func (p *ExecutorPool) putExecutor(executor *Executor) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// 池已关闭或执行器过期：直接销毁
	if p.closed ||
		(p.config.MaxAge > 0 && time.Since(executor.createTime) > p.config.MaxAge) ||
		p.idleList.Len() >= p.config.MaxIdle {
		p.activeCount--
		return
	}

	// 重置执行器状态
	executor.reset()

	// 放入空闲列表
	p.idleList.PushBack(executor)
}

// createNewExecutor 创建新的执行器
func (p *ExecutorPool) createNewExecutor() *Executor {
	return NewExecutor(p)
}

// decrementActive 减少活跃计数
func (p *ExecutorPool) decrementActive() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.activeCount--
}

// SetServiceName 设置服务名称
func (p *ExecutorPool) SetServiceName(name string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.serviceName = name
}

// GetServiceName 获取服务名称
func (p *ExecutorPool) GetServiceName() string {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.serviceName
}

// GetStats 获取池统计信息
func (p *ExecutorPool) GetStats() (active, idle int) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.activeCount, p.idleList.Len()
}

// Close 关闭执行器池。
// 注意：必须先在持锁状态下摘下所有空闲执行器，再在锁外逐个清理，
// 避免 Executor.Close 回调 putExecutor/decrementActive 时重复加锁死锁。
func (p *ExecutorPool) Close() {
	p.mutex.Lock()
	if p.closed {
		p.mutex.Unlock()
		return
	}
	p.closed = true

	// 摘下全部空闲执行器
	idle := make([]*Executor, 0, p.idleList.Len())
	for element := p.idleList.Front(); element != nil; element = element.Next() {
		idle = append(idle, element.Value.(*Executor))
	}
	p.idleList.Init()
	p.activeCount = 0
	p.mutex.Unlock()

	// 锁外清理执行器内部资源
	for _, executor := range idle {
		executor.mu.Lock()
		executor.vm.Cleanup()
		executor.mu.Unlock()
	}
}

// reset 重置执行器状态
func (e *Executor) reset() {
	e.functionChecksums = make(map[string]string)
	e.costTime = 0
	e.StopTimeoutTimer()
	e.mu.Lock()
	e.cancelFunc = nil
	if e.vm != nil {
		e.vm.Reset()
	}
	e.mu.Unlock()
}
