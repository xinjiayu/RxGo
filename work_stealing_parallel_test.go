package rxgo

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// 工作窃取调度器测试
// ============================================================================

func TestWorkStealingScheduler_Basic(t *testing.T) {
	scheduler := NewWorkStealingScheduler(4)

	// 检查worker数量
	if scheduler.GetWorkerCount() != 4 {
		t.Errorf("期望worker数量为4，实际为%d", scheduler.GetWorkerCount())
	}

	// 启动调度器
	scheduler.Start()

	// 提交一些任务
	executed := make([]bool, 10)
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		taskID := i
		scheduler.Submit(func() {
			executed[taskID] = true
			wg.Done()
		})
	}

	// 等待所有任务完成
	wg.Wait()

	// 验证所有任务都被执行
	for i, exec := range executed {
		if !exec {
			t.Errorf("任务%d未被执行", i)
		}
	}

	// 停止调度器
	scheduler.Stop()
}

func TestWorkStealingScheduler_SubmitToWorker(t *testing.T) {
	scheduler := NewWorkStealingScheduler(2)
	scheduler.Start()
	defer scheduler.Stop()

	// 测试提交到指定worker
	executed := false
	var wg sync.WaitGroup

	wg.Add(1)
	scheduler.SubmitToWorker(0, func() {
		executed = true
		wg.Done()
	})

	wg.Wait()

	if !executed {
		t.Error("任务未被执行")
	}

	// 测试无效worker ID的回退
	executed = false
	wg.Add(1)
	scheduler.SubmitToWorker(999, func() {
		executed = true
		wg.Done()
	})

	wg.Wait()

	if !executed {
		t.Error("回退任务未被执行")
	}
}

// ============================================================================
// 工作窃取Worker测试
// ============================================================================

func TestWorkStealingWorker_Basic(t *testing.T) {
	scheduler := NewWorkStealingScheduler(1)
	worker := scheduler.GetAllWorkers()[0]

	// 启动worker
	go worker.Run()

	// 提交任务
	executed := false
	var wg sync.WaitGroup

	wg.Add(1)
	worker.Submit(func() {
		executed = true
		wg.Done()
	})

	wg.Wait()

	if !executed {
		t.Error("任务未被执行")
	}

	// 检查统计信息
	stats := worker.GetStats()
	if stats.ExecutedTasks == 0 {
		t.Error("应该有执行的任务统计")
	}

	// 停止worker
	worker.Stop()
}

func TestWorkStealingWorker_WorkStealing(t *testing.T) {
	scheduler := NewWorkStealingScheduler(3)
	scheduler.Start()
	defer scheduler.Stop()

	// 给第一个worker分配大量任务
	workers := scheduler.GetAllWorkers()
	firstWorker := workers[0]

	taskCount := 20
	executed := make([]bool, taskCount)
	var wg sync.WaitGroup

	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		taskID := i
		firstWorker.Submit(func() {
			time.Sleep(time.Millisecond) // 模拟工作
			executed[taskID] = true
			wg.Done()
		})
	}

	// 等待一段时间让工作窃取发生
	time.Sleep(100 * time.Millisecond)

	wg.Wait()

	// 验证所有任务都被执行
	for i, exec := range executed {
		if !exec {
			t.Errorf("任务%d未被执行", i)
		}
	}

	// 检查是否发生了工作窃取
	totalStolenTasks := int64(0)
	for _, worker := range workers {
		stats := worker.GetStats()
		totalStolenTasks += stats.StolenTasks
	}

	if totalStolenTasks == 0 {
		t.Log("警告：未检测到工作窃取行为")
	}
}

// ============================================================================
// 增强的ParallelFlowable测试
// ============================================================================

func TestEnhancedParallelFlowable_Basic(t *testing.T) {
	// 创建一个简单的ParallelFlowable源（模拟）
	mockSource := &mockParallelFlowable{
		items: []int{1, 2, 3, 4, 5},
	}

	enhanced := NewEnhancedParallelFlowable(mockSource, 2)
	defer enhanced.Stop()

	// 创建订阅者
	received := make([]int, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup

	subscriber := &mockSubscriber{
		onNext: func(item Item) {
			if !item.IsError() && item.Value != nil {
				mu.Lock()
				received = append(received, item.Value.(int))
				mu.Unlock()
				wg.Done()
			}
		},
	}

	wg.Add(len(mockSource.items))
	enhanced.Subscribe([]Subscriber{subscriber})

	wg.Wait()

	// 验证结果
	if len(received) != len(mockSource.items) {
		t.Errorf("期望接收%d个值，实际接收%d个", len(mockSource.items), len(received))
	}

	// 检查统计信息
	stats := enhanced.GetStats()
	if len(stats) != 2 {
		t.Errorf("期望2个worker统计，实际%d个", len(stats))
	}
}

// ============================================================================
// 全局工作窃取调度器测试
// ============================================================================

func TestGlobalWorkStealingScheduler(t *testing.T) {
	// 获取全局调度器
	scheduler1 := GetGlobalWorkStealingScheduler()
	scheduler2 := GetGlobalWorkStealingScheduler()

	// 应该是同一个实例
	if scheduler1 != scheduler2 {
		t.Error("全局调度器应该是单例")
	}

	// 测试提交全局任务
	executed := false
	var wg sync.WaitGroup

	wg.Add(1)
	SubmitGlobalTask(func() {
		executed = true
		wg.Done()
	})

	wg.Wait()

	if !executed {
		t.Error("全局任务未被执行")
	}

	// 检查全局统计
	stats := GetGlobalWorkStealingStats()
	if len(stats) == 0 {
		t.Error("应该有worker统计信息")
	}
}

// ============================================================================
// 工作窃取统计测试
// ============================================================================

func TestWorkStealingStats(t *testing.T) {
	// 重置全局统计以确保干净的状态
	ResetGlobalWorkStealingStats()

	// 使用全局调度器进行测试
	taskCount := 10
	var wg sync.WaitGroup

	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		SubmitGlobalTask(func() {
			time.Sleep(time.Millisecond)
			wg.Done()
		})
	}

	wg.Wait()

	// 等待一段时间确保任务处理完成
	time.Sleep(10 * time.Millisecond)

	// 获取统计信息
	stats := GetWorkStealingStats()

	if stats.CompletedTasks == 0 {
		t.Error("应该有完成的任务")
	}

	if stats.TotalTasks == 0 {
		t.Error("应该有总任务数")
	}

	// 检查负载均衡比例
	if stats.LoadBalanceRatio < 0 || stats.LoadBalanceRatio > 1 {
		t.Errorf("负载均衡比例应该在0-1之间，实际为%f", stats.LoadBalanceRatio)
	}
}

// ============================================================================
// 性能基准测试
// ============================================================================

func BenchmarkWorkStealingScheduler_Submit(b *testing.B) {
	scheduler := NewWorkStealingScheduler(runtime.NumCPU())
	scheduler.Start()
	defer scheduler.Stop()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var wg sync.WaitGroup
			wg.Add(1)
			scheduler.Submit(func() {
				wg.Done()
			})
			wg.Wait()
		}
	})
}

func BenchmarkGlobalWorkStealingScheduler(b *testing.B) {
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var wg sync.WaitGroup
			wg.Add(1)
			SubmitGlobalTask(func() {
				wg.Done()
			})
			wg.Wait()
		}
	})
}

// ============================================================================
// Mock实现用于测试
// ============================================================================

// mockParallelFlowable 模拟的ParallelFlowable
type mockParallelFlowable struct {
	items []int
}

func (m *mockParallelFlowable) Subscribe(subscribers []Subscriber) {
	// 简单地将所有items发送给第一个订阅者
	if len(subscribers) > 0 {
		subscriber := subscribers[0]
		go func() {
			for _, item := range m.items {
				subscriber.OnNext(Item{Value: item})
			}
			subscriber.OnComplete()
		}()
	}
}

// 实现ParallelFlowable接口的其他方法（简单模拟）
func (m *mockParallelFlowable) Parallelism() int                             { return 1 }
func (m *mockParallelFlowable) Map(transformer Transformer) ParallelFlowable { return m }
func (m *mockParallelFlowable) Filter(predicate Predicate) ParallelFlowable  { return m }
func (m *mockParallelFlowable) FlatMap(transformer func(interface{}) Flowable) ParallelFlowable {
	return m
}
func (m *mockParallelFlowable) Reduce(reducer func(accumulator, value interface{}) interface{}) Single {
	return nil
}
func (m *mockParallelFlowable) ReduceWith(initialValue interface{}, reducer func(accumulator, value interface{}) interface{}) Single {
	return nil
}
func (m *mockParallelFlowable) RunOn(scheduler Scheduler) ParallelFlowable       { return m }
func (m *mockParallelFlowable) Sequential() Flowable                             { return nil }
func (m *mockParallelFlowable) ToFlowable() Flowable                             { return nil }
func (m *mockParallelFlowable) SubscribeOn(scheduler Scheduler) ParallelFlowable { return m }
func (m *mockParallelFlowable) SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) []FlowableSubscription {
	// 简单模拟实现
	return nil
}

// mockSubscriber 模拟的订阅者
type mockSubscriber struct {
	onNext     func(Item)
	onError    func(error)
	onComplete func()
}

func (m *mockSubscriber) OnNext(item Item) {
	if m.onNext != nil {
		m.onNext(item)
	}
}

func (m *mockSubscriber) OnError(err error) {
	if m.onError != nil {
		m.onError(err)
	}
}

func (m *mockSubscriber) OnComplete() {
	if m.onComplete != nil {
		m.onComplete()
	}
}

func (m *mockSubscriber) OnSubscribe(subscription FlowableSubscription) {
	// 模拟实现
}

// ============================================================================
// 并发安全测试
// ============================================================================

func TestWorkStealingScheduler_ConcurrentSubmit(t *testing.T) {
	scheduler := NewWorkStealingScheduler(4)
	scheduler.Start()
	defer scheduler.Stop()

	const numGoroutines = 10
	const tasksPerGoroutine = 100

	executed := make([]int64, numGoroutines)
	var wg sync.WaitGroup

	// 并发提交任务
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < tasksPerGoroutine; j++ {
				scheduler.Submit(func() {
					atomic.AddInt64(&executed[goroutineID], 1)
				})
			}
		}(i)
	}

	wg.Wait()

	// 等待所有任务完成
	time.Sleep(100 * time.Millisecond)

	// 验证所有任务都被执行
	totalExecuted := int64(0)
	for i, count := range executed {
		totalExecuted += count
		if count != tasksPerGoroutine {
			t.Errorf("Goroutine %d 执行了%d个任务，期望%d个", i, count, tasksPerGoroutine)
		}
	}

	expectedTotal := int64(numGoroutines * tasksPerGoroutine)
	if totalExecuted != expectedTotal {
		t.Errorf("总执行任务数%d，期望%d", totalExecuted, expectedTotal)
	}
}

func TestWorkStealingStats_Reset(t *testing.T) {
	// 提交一些任务
	var wg sync.WaitGroup
	wg.Add(5)
	for i := 0; i < 5; i++ {
		SubmitGlobalTask(func() {
			wg.Done()
		})
	}
	wg.Wait()

	// 获取统计信息
	stats := GetWorkStealingStats()
	if stats.CompletedTasks == 0 {
		t.Error("重置前应该有完成的任务")
	}

	// 重置统计
	ResetGlobalWorkStealingStats()

	// 重新获取统计信息
	stats = GetWorkStealingStats()
	if stats.CompletedTasks != 0 {
		t.Error("重置后完成任务数应该为0")
	}
}
