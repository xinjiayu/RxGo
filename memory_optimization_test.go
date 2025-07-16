package rxgo

import (
	"testing"
	"time"
)

// ============================================================================
// ItemPool 测试
// ============================================================================

func TestItemPool_BasicOperations(t *testing.T) {
	pool := NewItemPool()

	// 测试获取Item
	item1 := pool.Get()
	if item1 == nil {
		t.Error("获取的Item不应该为nil")
	}

	// 设置数据
	item1.Value = "test"
	item1.Error = nil

	// 测试归还Item
	pool.Put(item1)

	// 再次获取Item，应该是池化的对象
	item2 := pool.Get()
	if item2 == nil {
		t.Error("获取的Item不应该为nil")
	}

	// 检查是否被正确清理
	if item2.Value != nil {
		t.Error("池化的Item应该被清理")
	}
	if item2.Error != nil {
		t.Error("池化的Item应该被清理")
	}
}

func TestItemPool_Stats(t *testing.T) {
	pool := NewItemPool()

	// 初始统计应该为0
	gets, puts := pool.Stats()
	if gets != 0 || puts != 0 {
		t.Errorf("初始统计应该为0，实际为 gets=%d, puts=%d", gets, puts)
	}

	// 获取几个Item
	item1 := pool.Get()
	item2 := pool.Get()
	item3 := pool.Get()

	gets, puts = pool.Stats()
	if gets != 3 || puts != 0 {
		t.Errorf("获取3个Item后统计错误，实际为 gets=%d, puts=%d", gets, puts)
	}

	// 归还Item
	pool.Put(item1)
	pool.Put(item2)

	gets, puts = pool.Stats()
	if gets != 3 || puts != 2 {
		t.Errorf("归还2个Item后统计错误，实际为 gets=%d, puts=%d", gets, puts)
	}

	// 清理
	pool.Put(item3)
}

// ============================================================================
// QueuePool 测试
// ============================================================================

func TestQueuePool_BasicOperations(t *testing.T) {
	pool := NewQueuePool()

	// 测试获取不同容量的队列
	queue16 := pool.GetQueue(10) // 应该分配16容量
	queue32 := pool.GetQueue(20) // 应该分配32容量
	queue64 := pool.GetQueue(50) // 应该分配64容量

	// 检查容量
	if cap(queue16) < 10 {
		t.Errorf("队列容量不足: 期望>=10, 实际=%d", cap(queue16))
	}
	if cap(queue32) < 20 {
		t.Errorf("队列容量不足: 期望>=20, 实际=%d", cap(queue32))
	}
	if cap(queue64) < 50 {
		t.Errorf("队列容量不足: 期望>=50, 实际=%d", cap(queue64))
	}

	// 添加一些数据
	queue16 = append(queue16, 1, 2, 3)
	queue32 = append(queue32, "a", "b", "c")

	// 归还队列
	pool.PutQueue(queue16)
	pool.PutQueue(queue32)
	pool.PutQueue(queue64)

	// 再次获取，应该是清理过的
	newQueue := pool.GetQueue(10)
	if len(newQueue) != 0 {
		t.Errorf("重用的队列应该为空，实际长度=%d", len(newQueue))
	}
}

func TestQueuePool_LargeCapacity(t *testing.T) {
	pool := NewQueuePool()

	// 测试超大容量的队列
	largeQueue := pool.GetQueue(5000)
	if cap(largeQueue) < 5000 {
		t.Errorf("大容量队列容量不足: 期望>=5000, 实际=%d", cap(largeQueue))
	}

	// 归还大容量队列（应该被丢弃而不是池化）
	pool.PutQueue(largeQueue)
}

// ============================================================================
// 全局对象池测试
// ============================================================================

func TestGlobalPools(t *testing.T) {
	// 测试全局Item池
	item := GetPooledItem()
	if item == nil {
		t.Error("全局池获取的Item不应该为nil")
	}

	item.Value = "test"
	ReturnPooledItem(item)

	// 测试全局队列池
	queue := GetPooledQueue(50)
	if cap(queue) < 50 {
		t.Errorf("全局队列池容量不足: 期望>=50, 实际=%d", cap(queue))
	}

	ReturnPooledQueue(queue)

	// 检查统计
	gets, _ := GetGlobalPoolStats()
	if gets == 0 {
		t.Error("全局池统计应该有获取记录")
	}
}

// ============================================================================
// 性能统计测试
// ============================================================================

func TestPerformanceStats_Basic(t *testing.T) {
	// 重置统计
	ResetPerformanceStats()

	// 检查初始状态
	stats := GetPerformanceStats()
	if stats.ObserverCalls != 0 {
		t.Error("初始Observer调用次数应该为0")
	}
	if stats.SubscriptionCreated != 0 {
		t.Error("初始订阅创建次数应该为0")
	}

	// 增加一些统计
	IncrementObserverCalls()
	IncrementSubscriptionCreated()
	IncrementItemsAllocated()
	IncrementItemsPooled()
	IncrementFastPathUsed()

	// 检查更新后的统计
	stats = GetPerformanceStats()
	if stats.ObserverCalls != 1 {
		t.Errorf("期望Observer调用次数为1，实际为%d", stats.ObserverCalls)
	}
	if stats.SubscriptionCreated != 1 {
		t.Errorf("期望订阅创建次数为1，实际为%d", stats.SubscriptionCreated)
	}
	if stats.ItemsAllocated != 1 {
		t.Errorf("期望Item分配次数为1，实际为%d", stats.ItemsAllocated)
	}
	if stats.ItemsPooled != 1 {
		t.Errorf("期望Item池化次数为1，实际为%d", stats.ItemsPooled)
	}
	if stats.FastPathUsed != 1 {
		t.Errorf("期望Fast-Path使用次数为1，实际为%d", stats.FastPathUsed)
	}
}

func TestPerformanceStats_Concurrency(t *testing.T) {
	ResetPerformanceStats()

	// 测试并发统计
	IncrementSubscriptionCreated()
	IncrementSubscriptionCreated()
	IncrementSubscriptionCreated()

	stats := GetPerformanceStats()
	if stats.MaxConcurrentObs < 3 {
		t.Errorf("期望最大并发数>=3，实际为%d", stats.MaxConcurrentObs)
	}
	if stats.CurrentActiveObs != 3 {
		t.Errorf("期望当前活跃数为3，实际为%d", stats.CurrentActiveObs)
	}

	// 取消一些订阅
	IncrementSubscriptionCanceled()
	IncrementSubscriptionCanceled()

	stats = GetPerformanceStats()
	if stats.CurrentActiveObs != 1 {
		t.Errorf("期望当前活跃数为1，实际为%d", stats.CurrentActiveObs)
	}
}

func TestPerformanceStats_ProcessingTime(t *testing.T) {
	ResetPerformanceStats()

	// 添加处理时间
	duration1 := 100 * time.Millisecond
	duration2 := 200 * time.Millisecond

	AddProcessingTime(duration1)
	AddProcessingTime(duration2)
	IncrementObserverCalls()
	IncrementObserverCalls()

	stats := GetPerformanceStats()

	expectedTotal := (duration1 + duration2).Nanoseconds()
	if stats.TotalProcessingTime != expectedTotal {
		t.Errorf("期望总处理时间为%d纳秒，实际为%d", expectedTotal, stats.TotalProcessingTime)
	}

	expectedAverage := expectedTotal / 2
	if stats.AverageLatency != expectedAverage {
		t.Errorf("期望平均延迟为%d纳秒，实际为%d", expectedAverage, stats.AverageLatency)
	}
}

// ============================================================================
// 统计摘要测试
// ============================================================================

func TestStatsSummary(t *testing.T) {
	ResetPerformanceStats()

	// 模拟一些活动
	IncrementItemsAllocated()
	IncrementItemsAllocated()
	IncrementItemsPooled()

	IncrementSubscriptionCreated()
	IncrementSubscriptionCreated()
	IncrementFastPathUsed()
	IncrementScalarOptimized()

	summary := GetStatsSummary()

	// 检查内存效率 (1/2 = 50%)
	if summary.MemoryEfficiency != 50.0 {
		t.Errorf("期望内存效率为50.0%%，实际为%.1f%%", summary.MemoryEfficiency)
	}

	// 检查优化比例 (2/2 = 100%)
	if summary.OptimizationRatio != 100.0 {
		t.Errorf("期望优化比例为100.0%%，实际为%.1f%%", summary.OptimizationRatio)
	}
}

// ============================================================================
// 内存管理测试
// ============================================================================

func TestMemoryManagement(t *testing.T) {
	// 测试内存使用获取
	_, total1, _, gc1 := GetMemoryUsage()

	// 创建一些对象
	items := make([]*Item, 1000)
	for i := range items {
		items[i] = GetPooledItem()
		items[i].Value = i
	}

	// 强制垃圾回收
	ForceGC()

	_, total2, _, gc2 := GetMemoryUsage()

	// 基本检查
	if total2 <= total1 {
		t.Error("总分配内存应该增加")
	}
	if gc2 < gc1 {
		t.Error("GC次数应该增加")
	}

	// 清理
	for _, item := range items {
		ReturnPooledItem(item)
	}

	// 检查内存统计打印
	summary := PrintMemoryStats()
	if summary.MemoryEfficiency < 0 {
		t.Error("内存效率不应该为负数")
	}

	// 检查配置
	if !IsMemoryOptimizationEnabled() {
		t.Error("内存优化应该是启用的")
	}
}

// ============================================================================
// 性能基准测试
// ============================================================================

func BenchmarkItemPool_GetPut(b *testing.B) {
	pool := NewItemPool()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		item := pool.Get()
		item.Value = i
		pool.Put(item)
	}
}

func BenchmarkGlobalItemPool_GetPut(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		item := GetPooledItem()
		item.Value = i
		ReturnPooledItem(item)
	}
}

func BenchmarkQueuePool_GetPut(b *testing.B) {
	pool := NewQueuePool()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		queue := pool.GetQueue(64)
		queue = append(queue, i)
		pool.PutQueue(queue)
	}
}

func BenchmarkPerformanceStats_Increment(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IncrementObserverCalls()
	}
}

func BenchmarkPerformanceStats_Get(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetPerformanceStats()
	}
}

// ============================================================================
// 并发安全测试
// ============================================================================

func TestItemPool_ConcurrentAccess(t *testing.T) {
	pool := NewItemPool()

	// 并发获取和归还Item
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				item := pool.Get()
				item.Value = j
				pool.Put(item)
			}
			done <- struct{}{}
		}()
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		<-done
	}

	gets, puts := pool.Stats()
	if gets != 1000 {
		t.Errorf("期望获取1000次，实际%d次", gets)
	}
	if puts != 1000 {
		t.Errorf("期望归还1000次，实际%d次", puts)
	}
}

func TestPerformanceStats_ConcurrentIncrement(t *testing.T) {
	ResetPerformanceStats()

	// 并发增加统计
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				IncrementObserverCalls()
				IncrementItemsAllocated()
			}
			done <- struct{}{}
		}()
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		<-done
	}

	stats := GetPerformanceStats()
	if stats.ObserverCalls != 1000 {
		t.Errorf("期望Observer调用1000次，实际%d次", stats.ObserverCalls)
	}
	if stats.ItemsAllocated != 1000 {
		t.Errorf("期望Item分配1000次，实际%d次", stats.ItemsAllocated)
	}
}
