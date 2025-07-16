// RxGo 工作窃取并行算法实现
// 对标RxJava的ParallelFlowable性能优化，实现高效的负载均衡
package rxgo

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// 工作窃取调度器
// ============================================================================

// WorkStealingScheduler 工作窃取调度器
type WorkStealingScheduler struct {
	workers    []*WorkStealingWorker
	workerNum  int
	roundRobin int64
	stopped    int32
	mu         sync.RWMutex
}

// NewWorkStealingScheduler 创建工作窃取调度器
func NewWorkStealingScheduler(workerNum int) *WorkStealingScheduler {
	if workerNum <= 0 {
		workerNum = runtime.NumCPU()
	}

	scheduler := &WorkStealingScheduler{
		workers:   make([]*WorkStealingWorker, workerNum),
		workerNum: workerNum,
	}

	// 创建worker
	for i := 0; i < workerNum; i++ {
		scheduler.workers[i] = NewWorkStealingWorker(i, scheduler)
	}

	return scheduler
}

// Start 启动调度器
func (ws *WorkStealingScheduler) Start() {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if atomic.LoadInt32(&ws.stopped) == 0 {
		for _, worker := range ws.workers {
			go worker.Run()
		}
	}
}

// Stop 停止调度器
func (ws *WorkStealingScheduler) Stop() {
	atomic.StoreInt32(&ws.stopped, 1)

	ws.mu.RLock()
	defer ws.mu.RUnlock()

	for _, worker := range ws.workers {
		worker.Stop()
	}
}

// Submit 提交任务
func (ws *WorkStealingScheduler) Submit(task func()) {
	if atomic.LoadInt32(&ws.stopped) != 0 {
		return
	}

	// 轮询分配任务
	workerIndex := atomic.AddInt64(&ws.roundRobin, 1) % int64(ws.workerNum)
	worker := ws.workers[workerIndex]
	worker.Submit(task)
}

// SubmitToWorker 提交任务到指定worker
func (ws *WorkStealingScheduler) SubmitToWorker(workerID int, task func()) {
	if workerID < 0 || workerID >= ws.workerNum {
		ws.Submit(task) // 回退到轮询分配
		return
	}

	worker := ws.workers[workerID]
	worker.Submit(task)
}

// GetWorkerCount 获取worker数量
func (ws *WorkStealingScheduler) GetWorkerCount() int {
	return ws.workerNum
}

// GetAllWorkers 获取所有worker (用于工作窃取)
func (ws *WorkStealingScheduler) GetAllWorkers() []*WorkStealingWorker {
	return ws.workers
}

// ============================================================================
// 工作窃取Worker
// ============================================================================

// WorkStealingWorker 工作窃取Worker
type WorkStealingWorker struct {
	id            int
	scheduler     *WorkStealingScheduler
	localQueue    chan func()
	stopped       int32
	idleTime      int64 // 空闲时间（纳秒）
	executedTasks int64 // 执行的任务数
	stolenTasks   int64 // 窃取的任务数
	lostTasks     int64 // 被窃取的任务数
	mu            sync.RWMutex
}

// NewWorkStealingWorker 创建工作窃取Worker
func NewWorkStealingWorker(id int, scheduler *WorkStealingScheduler) *WorkStealingWorker {
	return &WorkStealingWorker{
		id:         id,
		scheduler:  scheduler,
		localQueue: make(chan func(), 256), // 缓冲队列
	}
}

// Run 运行Worker
func (w *WorkStealingWorker) Run() {
	for atomic.LoadInt32(&w.stopped) == 0 {
		select {
		case task := <-w.localQueue:
			// 执行本地任务
			if task != nil {
				w.executeTask(task)
				atomic.AddInt64(&w.executedTasks, 1)
			}
		default:
			// 本地队列为空，尝试工作窃取
			if w.attemptWorkStealing() {
				continue // 窃取成功，继续下一轮
			}

			// 没有任务可执行，短暂休眠
			w.idleSleep()
		}
	}
}

// Stop 停止Worker
func (w *WorkStealingWorker) Stop() {
	atomic.StoreInt32(&w.stopped, 1)
	close(w.localQueue)
}

// Submit 提交任务到本地队列
func (w *WorkStealingWorker) Submit(task func()) {
	if atomic.LoadInt32(&w.stopped) != 0 {
		return
	}

	select {
	case w.localQueue <- task:
		// 任务提交成功
	default:
		// 队列已满，直接执行
		w.executeTask(task)
		atomic.AddInt64(&w.executedTasks, 1)
	}
}

// executeTask 执行任务
func (w *WorkStealingWorker) executeTask(task func()) {
	defer func() {
		if r := recover(); r != nil {
			// 处理panic，记录日志但不让worker崩溃
		}
	}()

	task()
}

// attemptWorkStealing 尝试工作窃取
func (w *WorkStealingWorker) attemptWorkStealing() bool {
	workers := w.scheduler.GetAllWorkers()
	workerCount := len(workers)

	// 随机选择一个worker开始窃取
	startIndex := (w.id + 1) % workerCount

	for i := 0; i < workerCount-1; i++ {
		targetIndex := (startIndex + i) % workerCount
		targetWorker := workers[targetIndex]

		// 不从自己窃取
		if targetWorker.id == w.id {
			continue
		}

		// 尝试从目标worker窃取任务
		stolenTask := targetWorker.stealTask()
		if stolenTask != nil {
			w.executeTask(stolenTask)
			atomic.AddInt64(&w.executedTasks, 1)
			atomic.AddInt64(&w.stolenTasks, 1)
			return true
		}
	}

	return false
}

// stealTask 被其他worker窃取任务
func (w *WorkStealingWorker) stealTask() func() {
	select {
	case task := <-w.localQueue:
		if task != nil {
			atomic.AddInt64(&w.lostTasks, 1)
		}
		return task
	default:
		return nil // 队列为空
	}
}

// idleSleep 空闲时休眠
func (w *WorkStealingWorker) idleSleep() {
	startTime := time.Now()

	// 使用指数退避策略
	sleepTime := time.Microsecond * 10 // 初始10微秒
	time.Sleep(sleepTime)

	// 记录空闲时间
	idleDuration := time.Since(startTime)
	atomic.AddInt64(&w.idleTime, idleDuration.Nanoseconds())
}

// GetStats 获取Worker统计信息
func (w *WorkStealingWorker) GetStats() WorkerStats {
	return WorkerStats{
		ID:            w.id,
		ExecutedTasks: atomic.LoadInt64(&w.executedTasks),
		StolenTasks:   atomic.LoadInt64(&w.stolenTasks),
		LostTasks:     atomic.LoadInt64(&w.lostTasks),
		IdleTimeNs:    atomic.LoadInt64(&w.idleTime),
		QueueSize:     len(w.localQueue),
	}
}

// WorkerStats Worker统计信息
type WorkerStats struct {
	ID            int
	ExecutedTasks int64
	StolenTasks   int64
	LostTasks     int64
	IdleTimeNs    int64
	QueueSize     int
}

// ============================================================================
// 工作窃取ParallelFlowable增强
// ============================================================================

// EnhancedParallelFlowable 增强的并行Flowable
type EnhancedParallelFlowable struct {
	source    ParallelFlowable
	scheduler *WorkStealingScheduler
}

// NewEnhancedParallelFlowable 创建增强的并行Flowable
func NewEnhancedParallelFlowable(source ParallelFlowable, parallelism int) *EnhancedParallelFlowable {
	scheduler := NewWorkStealingScheduler(parallelism)
	scheduler.Start()

	return &EnhancedParallelFlowable{
		source:    source,
		scheduler: scheduler,
	}
}

// Subscribe 订阅并行流
func (epf *EnhancedParallelFlowable) Subscribe(subscribers []Subscriber) {
	parallelism := epf.scheduler.GetWorkerCount()

	// 创建工作窃取订阅者包装器
	enhancedSubscribers := make([]Subscriber, parallelism)
	for i := 0; i < parallelism; i++ {
		workerID := i
		originalSubscriber := subscribers[i%len(subscribers)]

		enhancedSubscribers[i] = &workStealingSubscriber{
			workerID:   workerID,
			scheduler:  epf.scheduler,
			downstream: originalSubscriber,
		}
	}

	// 订阅源
	epf.source.Subscribe(enhancedSubscribers)
}

// Stop 停止工作窃取调度器
func (epf *EnhancedParallelFlowable) Stop() {
	epf.scheduler.Stop()
}

// GetStats 获取工作窃取统计信息
func (epf *EnhancedParallelFlowable) GetStats() []WorkerStats {
	workers := epf.scheduler.GetAllWorkers()
	stats := make([]WorkerStats, len(workers))

	for i, worker := range workers {
		stats[i] = worker.GetStats()
	}

	return stats
}

// workStealingSubscriber 工作窃取订阅者包装器
type workStealingSubscriber struct {
	workerID   int
	scheduler  *WorkStealingScheduler
	downstream Subscriber
}

// OnNext 处理下一个值
func (wss *workStealingSubscriber) OnNext(item Item) {
	wss.scheduler.SubmitToWorker(wss.workerID, func() {
		wss.downstream.OnNext(item)
	})
}

// OnError 处理错误
func (wss *workStealingSubscriber) OnError(err error) {
	wss.scheduler.SubmitToWorker(wss.workerID, func() {
		wss.downstream.OnError(err)
	})
}

// OnComplete 处理完成
func (wss *workStealingSubscriber) OnComplete() {
	wss.scheduler.SubmitToWorker(wss.workerID, func() {
		wss.downstream.OnComplete()
	})
}

// OnSubscribe 处理订阅
func (wss *workStealingSubscriber) OnSubscribe(subscription FlowableSubscription) {
	wss.downstream.OnSubscribe(subscription)
}

// ============================================================================
// 全局工作窃取调度器
// ============================================================================

var (
	// GlobalWorkStealingScheduler 全局工作窃取调度器
	GlobalWorkStealingScheduler *WorkStealingScheduler

	// 初始化全局调度器
	initGlobalSchedulerOnce sync.Once
)

// GetGlobalWorkStealingScheduler 获取全局工作窃取调度器
func GetGlobalWorkStealingScheduler() *WorkStealingScheduler {
	initGlobalSchedulerOnce.Do(func() {
		GlobalWorkStealingScheduler = NewWorkStealingScheduler(runtime.NumCPU())
		GlobalWorkStealingScheduler.Start()
	})
	return GlobalWorkStealingScheduler
}

// WithWorkStealing 为ParallelFlowable启用工作窃取
func WithWorkStealing(source ParallelFlowable) *EnhancedParallelFlowable {
	scheduler := GetGlobalWorkStealingScheduler()
	return &EnhancedParallelFlowable{
		source:    source,
		scheduler: scheduler,
	}
}

// ============================================================================
// 工作窃取统计和监控
// ============================================================================

// WorkStealingStats 工作窃取统计信息
type WorkStealingStats struct {
	TotalTasks       int64   // 总任务数
	CompletedTasks   int64   // 完成任务数
	StolenTasks      int64   // 窃取任务数
	TotalIdleTime    int64   // 总空闲时间（纳秒）
	LoadBalanceRatio float64 // 负载均衡比例
	StealSuccessRate float64 // 窃取成功率
	AverageQueueSize float64 // 平均队列大小
}

// GetWorkStealingStats 获取工作窃取统计信息
func GetWorkStealingStats() WorkStealingStats {
	scheduler := GetGlobalWorkStealingScheduler()
	workerStats := make([]WorkerStats, scheduler.GetWorkerCount())

	for i, worker := range scheduler.GetAllWorkers() {
		workerStats[i] = worker.GetStats()
	}

	// 计算统计信息
	var totalTasks, completedTasks, stolenTasks, totalIdleTime int64
	var totalQueueSize int

	for _, stats := range workerStats {
		totalTasks += stats.ExecutedTasks + stats.StolenTasks
		completedTasks += stats.ExecutedTasks
		stolenTasks += stats.StolenTasks
		totalIdleTime += stats.IdleTimeNs
		totalQueueSize += stats.QueueSize
	}

	workerCount := len(workerStats)

	// 计算负载均衡比例
	var loadBalanceRatio float64 = 1.0
	if workerCount > 0 && completedTasks > 0 {
		avgTasks := float64(completedTasks) / float64(workerCount)
		var variance float64
		for _, stats := range workerStats {
			diff := float64(stats.ExecutedTasks) - avgTasks
			variance += diff * diff
		}
		stdDev := variance / float64(workerCount)
		if avgTasks > 0 {
			loadBalanceRatio = 1.0 - (stdDev / (avgTasks * avgTasks))
		}
	}

	// 计算窃取成功率
	var stealSuccessRate float64
	if totalTasks > 0 {
		stealSuccessRate = float64(stolenTasks) / float64(totalTasks)
	}

	// 计算平均队列大小
	var averageQueueSize float64
	if workerCount > 0 {
		averageQueueSize = float64(totalQueueSize) / float64(workerCount)
	}

	return WorkStealingStats{
		TotalTasks:       totalTasks,
		CompletedTasks:   completedTasks,
		StolenTasks:      stolenTasks,
		TotalIdleTime:    totalIdleTime,
		LoadBalanceRatio: loadBalanceRatio,
		StealSuccessRate: stealSuccessRate,
		AverageQueueSize: averageQueueSize,
	}
}

// ============================================================================
// 实用工具函数
// ============================================================================

// SubmitGlobalTask 提交任务到全局工作窃取调度器
func SubmitGlobalTask(task func()) {
	scheduler := GetGlobalWorkStealingScheduler()
	scheduler.Submit(task)
}

// GetGlobalWorkStealingStats 获取全局工作窃取统计
func GetGlobalWorkStealingStats() []WorkerStats {
	scheduler := GetGlobalWorkStealingScheduler()
	workers := scheduler.GetAllWorkers()
	stats := make([]WorkerStats, len(workers))

	for i, worker := range workers {
		stats[i] = worker.GetStats()
	}

	return stats
}

// ResetGlobalWorkStealingStats 重置全局工作窃取统计
func ResetGlobalWorkStealingStats() {
	// 由于统计信息在worker中，需要重新创建scheduler来重置
	if GlobalWorkStealingScheduler != nil {
		GlobalWorkStealingScheduler.Stop()
	}

	// 强制重新初始化
	initGlobalSchedulerOnce = sync.Once{}
	GlobalWorkStealingScheduler = nil
}
