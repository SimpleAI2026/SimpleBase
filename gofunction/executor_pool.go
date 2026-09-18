package gofunction

import (
	"container/list"
	"errors"
	"sync"
	"time"
)

// 池操作错误
var (
	ErrPoolClosed    = errors.New("executor pool closed")
	ErrPoolExhausted = errors.New("executor pool exhausted")
)

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

// CodeEntry 代码条目
type CodeEntry struct {
	Name string
	Body string
}

// ExecutorPool 执行器对象池
type ExecutorPool struct {
	config ExecutorPoolConfig

	extraRegistry []CodeEntry
	registryPool  *sync.Pool
	vmPool        *sync.Pool

	mutex       sync.Mutex
	activeCount int
	idleList    list.List
	serviceName string
	closed      bool
}

// NewExecutorPool 创建新的执行器池
func NewExecutorPool(config ExecutorPoolConfig) *ExecutorPool {
	return &ExecutorPool{
		config:        config,
		extraRegistry: make([]CodeEntry, 0),
		registryPool:  &sync.Pool{},
		vmPool:        &sync.Pool{},
		activeCount:   0,
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
		executor.vm.Cleanup()
	}
}

// reset 重置执行器状态
func (e *Executor) reset() {
	e.functionChecksums = make(map[string]string)
	e.costTime = 0
	e.StopTimeoutTimer()
	if e.vm != nil {
		e.vm.Reset()
	}
}
