// Scheduler implementations for RxGo
// 实现调度器系统，支持不同的执行策略
package rxgo

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// 立即调度器 - Immediate Scheduler
// ============================================================================

// immediateScheduler 立即在当前goroutine中执行任务
type immediateScheduler struct{}

// NewImmediateScheduler 创建立即调度器
func NewImmediateScheduler() Scheduler {
	return &immediateScheduler{}
}

// Schedule 立即执行任务
func (s *immediateScheduler) Schedule(action func()) Disposable {
	action()
	return NewBaseDisposable(func() {})
}

// ScheduleWithDelay 延迟执行任务
func (s *immediateScheduler) ScheduleWithDelay(action func(), delay time.Duration) Disposable {
	timer := time.NewTimer(delay)

	go func() {
		<-timer.C
		action()
	}()

	return NewBaseDisposable(func() {
		timer.Stop()
	})
}

// ScheduleWithContext 带上下文执行任务
func (s *immediateScheduler) ScheduleWithContext(ctx context.Context, action func()) Disposable {
	select {
	case <-ctx.Done():
		return NewBaseDisposable(func() {})
	default:
		action()
		return NewBaseDisposable(func() {})
	}
}

// ============================================================================
// 当前线程调度器 - Current Thread Scheduler
// ============================================================================

// currentThreadScheduler 在当前线程中按顺序执行任务
type currentThreadScheduler struct {
	queue      []func()
	mu         sync.Mutex
	processing bool
}

// NewCurrentThreadScheduler 创建当前线程调度器
func NewCurrentThreadScheduler() Scheduler {
	return &currentThreadScheduler{
		queue: make([]func(), 0),
	}
}

// Schedule 在当前线程中调度任务
func (s *currentThreadScheduler) Schedule(action func()) Disposable {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.queue = append(s.queue, action)

	if !s.processing {
		s.processing = true
		go s.processQueue()
	}

	return NewBaseDisposable(func() {
		// TODO: 实现任务取消
	})
}

// ScheduleWithDelay 延迟调度任务
func (s *currentThreadScheduler) ScheduleWithDelay(action func(), delay time.Duration) Disposable {
	timer := time.NewTimer(delay)

	go func() {
		<-timer.C
		s.Schedule(action)
	}()

	return NewBaseDisposable(func() {
		timer.Stop()
	})
}

// ScheduleWithContext 带上下文调度任务
func (s *currentThreadScheduler) ScheduleWithContext(ctx context.Context, action func()) Disposable {
	wrappedAction := func() {
		select {
		case <-ctx.Done():
			return
		default:
			action()
		}
	}

	return s.Schedule(wrappedAction)
}

// processQueue 处理队列中的任务
func (s *currentThreadScheduler) processQueue() {
	for {
		s.mu.Lock()
		if len(s.queue) == 0 {
			s.processing = false
			s.mu.Unlock()
			return
		}

		action := s.queue[0]
		s.queue = s.queue[1:]
		s.mu.Unlock()

		action()
	}
}

// ============================================================================
// 新线程调度器 - New Thread Scheduler
// ============================================================================

// newThreadScheduler 为每个任务创建新的goroutine
type newThreadScheduler struct{}

// NewNewThreadScheduler 创建新线程调度器
func NewNewThreadScheduler() Scheduler {
	return &newThreadScheduler{}
}

// Schedule 在新goroutine中执行任务
func (s *newThreadScheduler) Schedule(action func()) Disposable {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		select {
		case <-ctx.Done():
			return
		default:
			action()
		}
	}()

	return NewBaseDisposable(cancel)
}

// ScheduleWithDelay 延迟在新goroutine中执行任务
func (s *newThreadScheduler) ScheduleWithDelay(action func(), delay time.Duration) Disposable {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			action()
		}
	}()

	return NewBaseDisposable(cancel)
}

// ScheduleWithContext 带上下文在新goroutine中执行任务
func (s *newThreadScheduler) ScheduleWithContext(ctx context.Context, action func()) Disposable {
	childCtx, cancel := context.WithCancel(ctx)

	go func() {
		select {
		case <-childCtx.Done():
			return
		default:
			action()
		}
	}()

	return NewBaseDisposable(cancel)
}

// ============================================================================
// 线程池调度器 - Thread Pool Scheduler
// ============================================================================

// threadPoolScheduler 使用固定大小的goroutine池执行任务
type threadPoolScheduler struct {
	workers   int
	taskQueue chan func()
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	disposed  int32
}

// NewThreadPoolScheduler 创建线程池调度器
func NewThreadPoolScheduler(workers int) Scheduler {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	ctx, cancel := context.WithCancel(context.Background())

	scheduler := &threadPoolScheduler{
		workers:   workers,
		taskQueue: make(chan func(), workers*2), // 缓冲区大小为worker数量的2倍
		ctx:       ctx,
		cancel:    cancel,
	}

	// 启动worker goroutines
	for i := 0; i < workers; i++ {
		scheduler.wg.Add(1)
		go scheduler.worker()
	}

	return scheduler
}

// Schedule 在线程池中执行任务
func (s *threadPoolScheduler) Schedule(action func()) Disposable {
	if atomic.LoadInt32(&s.disposed) == 1 {
		return NewBaseDisposable(func() {})
	}

	select {
	case s.taskQueue <- action:
		return NewBaseDisposable(func() {
			// TODO: 实现任务取消
		})
	case <-s.ctx.Done():
		return NewBaseDisposable(func() {})
	}
}

// ScheduleWithDelay 延迟在线程池中执行任务
func (s *threadPoolScheduler) ScheduleWithDelay(action func(), delay time.Duration) Disposable {
	timer := time.NewTimer(delay)

	go func() {
		defer timer.Stop()

		select {
		case <-s.ctx.Done():
			return
		case <-timer.C:
			s.Schedule(action)
		}
	}()

	return NewBaseDisposable(func() {
		timer.Stop()
	})
}

// ScheduleWithContext 带上下文在线程池中执行任务
func (s *threadPoolScheduler) ScheduleWithContext(ctx context.Context, action func()) Disposable {
	wrappedAction := func() {
		select {
		case <-ctx.Done():
			return
		default:
			action()
		}
	}

	return s.Schedule(wrappedAction)
}

// worker 工作goroutine
func (s *threadPoolScheduler) worker() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case task := <-s.taskQueue:
			if task != nil {
				task()
			}
		}
	}
}

// Dispose 释放线程池资源
func (s *threadPoolScheduler) Dispose() {
	if atomic.CompareAndSwapInt32(&s.disposed, 0, 1) {
		s.cancel()
		close(s.taskQueue)
		s.wg.Wait()
	}
}

// ============================================================================
// 测试调度器 - Test Scheduler
// ============================================================================

// testScheduler 用于测试的调度器，可以手动控制时间
type testScheduler struct {
	clock      int64
	queue      []scheduledAction
	mu         sync.Mutex
	isDisposed bool
}

// scheduledAction 调度的动作
type scheduledAction struct {
	time   int64
	action func()
}

// NewTestScheduler 创建测试调度器
func NewTestScheduler() *testScheduler {
	return &testScheduler{
		queue: make([]scheduledAction, 0),
	}
}

// Schedule 调度任务
func (s *testScheduler) Schedule(action func()) Disposable {
	return s.ScheduleAt(s.clock, action)
}

// ScheduleWithDelay 延迟调度任务
func (s *testScheduler) ScheduleWithDelay(action func(), delay time.Duration) Disposable {
	return s.ScheduleAt(s.clock+delay.Nanoseconds(), action)
}

// ScheduleWithContext 带上下文调度任务
func (s *testScheduler) ScheduleWithContext(ctx context.Context, action func()) Disposable {
	wrappedAction := func() {
		select {
		case <-ctx.Done():
			return
		default:
			action()
		}
	}

	return s.Schedule(wrappedAction)
}

// ScheduleAt 在指定时间调度任务
func (s *testScheduler) ScheduleAt(time int64, action func()) Disposable {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isDisposed {
		return NewBaseDisposable(func() {})
	}

	// 插入到正确的位置以保持时间顺序
	newAction := scheduledAction{time: time, action: action}

	inserted := false
	for i, existing := range s.queue {
		if time <= existing.time {
			s.queue = append(s.queue[:i], append([]scheduledAction{newAction}, s.queue[i:]...)...)
			inserted = true
			break
		}
	}

	if !inserted {
		s.queue = append(s.queue, newAction)
	}

	return NewBaseDisposable(func() {
		s.removeAction(newAction)
	})
}

// AdvanceTimeBy 推进时间
func (s *testScheduler) AdvanceTimeBy(duration time.Duration) {
	s.AdvanceTimeTo(s.clock + duration.Nanoseconds())
}

// AdvanceTimeTo 推进时间到指定时刻
func (s *testScheduler) AdvanceTimeTo(time int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isDisposed {
		return
	}

	s.clock = time

	// 执行所有应该在这个时间或之前执行的任务
	for len(s.queue) > 0 && s.queue[0].time <= time {
		action := s.queue[0]
		s.queue = s.queue[1:]

		// 解锁以允许action执行时调度新任务
		s.mu.Unlock()
		action.action()
		s.mu.Lock()
	}
}

// GetClock 获取当前时钟
func (s *testScheduler) GetClock() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clock
}

// removeAction 移除动作
func (s *testScheduler) removeAction(actionToRemove scheduledAction) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, action := range s.queue {
		if &action == &actionToRemove {
			s.queue = append(s.queue[:i], s.queue[i+1:]...)
			break
		}
	}
}

// Dispose 释放测试调度器
func (s *testScheduler) Dispose() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.isDisposed = true
	s.queue = nil
}

// ============================================================================
// 默认调度器
// ============================================================================

var (
	// DefaultScheduler 默认调度器
	DefaultScheduler Scheduler = NewNewThreadScheduler()

	// ImmediateScheduler 立即调度器实例
	ImmediateScheduler Scheduler = NewImmediateScheduler()

	// CurrentThreadScheduler 当前线程调度器实例
	CurrentThreadScheduler Scheduler = NewCurrentThreadScheduler()

	// NewThreadScheduler 新线程调度器实例
	NewThreadScheduler Scheduler = NewNewThreadScheduler()

	// ThreadPoolScheduler 线程池调度器实例
	ThreadPoolScheduler Scheduler = NewThreadPoolScheduler(runtime.NumCPU())
)

// ============================================================================
// 调度器工厂函数
// ============================================================================

// SchedulerFactory 调度器工厂函数类型
type SchedulerFactory func() Scheduler

// WithScheduler 创建使用指定调度器的选项
func WithScheduler(scheduler Scheduler) Option {
	return &schedulerOption{scheduler: scheduler}
}

// schedulerOption 调度器选项
type schedulerOption struct {
	scheduler Scheduler
}

// Apply 应用调度器选项
func (o *schedulerOption) Apply(config *Config) {
	config.Scheduler = o.scheduler
}

// ============================================================================
// 调度器辅助函数
// ============================================================================

// ScheduleRecurring 递归调度任务
func ScheduleRecurring(scheduler Scheduler, action func(), period time.Duration) Disposable {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		ticker := time.NewTicker(period)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				scheduler.Schedule(action)
			}
		}
	}()

	return NewBaseDisposable(cancel)
}

// ScheduleOnce 一次性调度任务
func ScheduleOnce(scheduler Scheduler, action func(), delay time.Duration) Disposable {
	return scheduler.ScheduleWithDelay(action, delay)
}

// ScheduleWithTimeout 带超时的调度任务
func ScheduleWithTimeout(scheduler Scheduler, action func(), timeout time.Duration) Disposable {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	disposable := scheduler.ScheduleWithContext(ctx, action)

	// 创建组合disposable
	return NewBaseDisposable(func() {
		cancel()
		disposable.Dispose()
	})
}

// ============================================================================
// 调度器性能监控
// ============================================================================

// SchedulerMetrics 调度器性能指标
type SchedulerMetrics struct {
	TasksScheduled int64
	TasksCompleted int64
	TasksFailed    int64
	AverageLatency time.Duration
}

// monitoredScheduler 带监控的调度器包装器
type monitoredScheduler struct {
	scheduler Scheduler
	metrics   *SchedulerMetrics
	mu        sync.RWMutex
}

// NewMonitoredScheduler 创建带监控的调度器
func NewMonitoredScheduler(scheduler Scheduler) Scheduler {
	return &monitoredScheduler{
		scheduler: scheduler,
		metrics:   &SchedulerMetrics{},
	}
}

// Schedule 调度任务并记录指标
func (s *monitoredScheduler) Schedule(action func()) Disposable {
	atomic.AddInt64(&s.metrics.TasksScheduled, 1)

	startTime := time.Now()
	wrappedAction := func() {
		defer func() {
			latency := time.Since(startTime)
			s.updateAverageLatency(latency)

			if r := recover(); r != nil {
				atomic.AddInt64(&s.metrics.TasksFailed, 1)
				panic(r)
			} else {
				atomic.AddInt64(&s.metrics.TasksCompleted, 1)
			}
		}()

		action()
	}

	return s.scheduler.Schedule(wrappedAction)
}

// ScheduleWithDelay 延迟调度任务并记录指标
func (s *monitoredScheduler) ScheduleWithDelay(action func(), delay time.Duration) Disposable {
	atomic.AddInt64(&s.metrics.TasksScheduled, 1)

	wrappedAction := func() {
		defer func() {
			if r := recover(); r != nil {
				atomic.AddInt64(&s.metrics.TasksFailed, 1)
				panic(r)
			} else {
				atomic.AddInt64(&s.metrics.TasksCompleted, 1)
			}
		}()

		action()
	}

	return s.scheduler.ScheduleWithDelay(wrappedAction, delay)
}

// ScheduleWithContext 带上下文调度任务并记录指标
func (s *monitoredScheduler) ScheduleWithContext(ctx context.Context, action func()) Disposable {
	return s.Schedule(func() {
		select {
		case <-ctx.Done():
			return
		default:
			action()
		}
	})
}

// GetMetrics 获取调度器指标
func (s *monitoredScheduler) GetMetrics() SchedulerMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return *s.metrics
}

// updateAverageLatency 更新平均延迟
func (s *monitoredScheduler) updateAverageLatency(latency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 简单的移动平均计算
	if s.metrics.AverageLatency == 0 {
		s.metrics.AverageLatency = latency
	} else {
		s.metrics.AverageLatency = (s.metrics.AverageLatency + latency) / 2
	}
}
