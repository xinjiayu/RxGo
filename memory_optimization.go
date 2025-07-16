// RxGo 高级内存优化系统
// 实现对象池化、性能监控和内存管理优化
package rxgo

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// 对象池化系统
// ============================================================================

// ItemPool Item对象池
type ItemPool struct {
	pool sync.Pool
	gets int64
	puts int64
}

// NewItemPool 创建Item对象池
func NewItemPool() *ItemPool {
	return &ItemPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &Item{}
			},
		},
	}
}

// Get 从池中获取Item
func (p *ItemPool) Get() *Item {
	atomic.AddInt64(&p.gets, 1)
	return p.pool.Get().(*Item)
}

// Put 将Item放回池中
func (p *ItemPool) Put(item *Item) {
	if item != nil {
		// 清空Item内容
		item.Value = nil
		item.Error = nil
		atomic.AddInt64(&p.puts, 1)
		p.pool.Put(item)
	}
}

// Stats 获取池统计信息
func (p *ItemPool) Stats() (gets, puts int64) {
	return atomic.LoadInt64(&p.gets), atomic.LoadInt64(&p.puts)
}

// QueuePool 队列对象池
type QueuePool struct {
	pools []*sync.Pool // 不同容量的队列池
	sizes []int        // 对应的容量大小
}

// NewQueuePool 创建队列池
func NewQueuePool() *QueuePool {
	sizes := []int{16, 32, 64, 128, 256, 512, 1024, 2048}
	pools := make([]*sync.Pool, len(sizes))

	for i, size := range sizes {
		capacity := size
		pools[i] = &sync.Pool{
			New: func() interface{} {
				return make([]interface{}, 0, capacity)
			},
		}
	}

	return &QueuePool{
		pools: pools,
		sizes: sizes,
	}
}

// GetQueue 获取指定容量的队列
func (qp *QueuePool) GetQueue(capacity int) []interface{} {
	// 找到最适合的池
	for i, size := range qp.sizes {
		if capacity <= size {
			queue := qp.pools[i].Get().([]interface{})
			// 清空队列内容但保持容量
			return queue[:0]
		}
	}

	// 如果需要的容量超过最大池容量，直接创建
	return make([]interface{}, 0, capacity)
}

// PutQueue 将队列放回池中
func (qp *QueuePool) PutQueue(queue []interface{}) {
	if queue == nil {
		return
	}

	capacity := cap(queue)

	// 找到对应的池
	for i, size := range qp.sizes {
		if capacity <= size {
			// 清空队列内容
			for j := range queue {
				queue[j] = nil
			}
			queue = queue[:0]
			qp.pools[i].Put(queue)
			return
		}
	}

	// 超大容量的队列不放回池中，让GC处理
}

// ============================================================================
// 全局对象池
// ============================================================================

var (
	// GlobalItemPool 全局Item对象池
	GlobalItemPool = NewItemPool()

	// GlobalQueuePool 全局队列池
	GlobalQueuePool = NewQueuePool()
)

// ============================================================================
// 性能监控和统计
// ============================================================================

// PerformanceStats 性能统计信息
type PerformanceStats struct {
	// 操作统计
	ObserverCalls        int64 // Observer调用次数
	SubscriptionCreated  int64 // 订阅创建次数
	SubscriptionCanceled int64 // 订阅取消次数

	// 内存统计
	ItemsAllocated int64 // 分配的Item数量
	ItemsPooled    int64 // 池化的Item数量
	QueuePoolHits  int64 // 队列池命中次数
	QueuePoolMiss  int64 // 队列池未命中次数

	// 优化统计
	FastPathUsed    int64 // Fast-Path使用次数
	ScalarOptimized int64 // 标量优化次数
	EmptyOptimized  int64 // 空Observable优化次数
	ErrorOptimized  int64 // 错误Observable优化次数

	// 时间统计
	TotalProcessingTime int64 // 总处理时间(纳秒)
	AverageLatency      int64 // 平均延迟(纳秒)

	// 并发统计
	MaxConcurrentObs int64 // 最大并发Observable数
	CurrentActiveObs int64 // 当前活跃Observable数
	GoroutineCount   int64 // 当前goroutine数量
}

// globalPerformanceStats 全局性能统计
var globalPerformanceStats = &PerformanceStats{}

// GetPerformanceStats 获取性能统计信息
func GetPerformanceStats() PerformanceStats {
	stats := PerformanceStats{
		ObserverCalls:        atomic.LoadInt64(&globalPerformanceStats.ObserverCalls),
		SubscriptionCreated:  atomic.LoadInt64(&globalPerformanceStats.SubscriptionCreated),
		SubscriptionCanceled: atomic.LoadInt64(&globalPerformanceStats.SubscriptionCanceled),
		ItemsAllocated:       atomic.LoadInt64(&globalPerformanceStats.ItemsAllocated),
		ItemsPooled:          atomic.LoadInt64(&globalPerformanceStats.ItemsPooled),
		QueuePoolHits:        atomic.LoadInt64(&globalPerformanceStats.QueuePoolHits),
		QueuePoolMiss:        atomic.LoadInt64(&globalPerformanceStats.QueuePoolMiss),
		FastPathUsed:         atomic.LoadInt64(&globalPerformanceStats.FastPathUsed),
		ScalarOptimized:      atomic.LoadInt64(&globalPerformanceStats.ScalarOptimized),
		EmptyOptimized:       atomic.LoadInt64(&globalPerformanceStats.EmptyOptimized),
		ErrorOptimized:       atomic.LoadInt64(&globalPerformanceStats.ErrorOptimized),
		TotalProcessingTime:  atomic.LoadInt64(&globalPerformanceStats.TotalProcessingTime),
		AverageLatency:       atomic.LoadInt64(&globalPerformanceStats.AverageLatency),
		MaxConcurrentObs:     atomic.LoadInt64(&globalPerformanceStats.MaxConcurrentObs),
		CurrentActiveObs:     atomic.LoadInt64(&globalPerformanceStats.CurrentActiveObs),
		GoroutineCount:       int64(runtime.NumGoroutine()),
	}

	// 计算平均延迟
	if stats.ObserverCalls > 0 {
		stats.AverageLatency = stats.TotalProcessingTime / stats.ObserverCalls
	}

	return stats
}

// ResetPerformanceStats 重置性能统计
func ResetPerformanceStats() {
	atomic.StoreInt64(&globalPerformanceStats.ObserverCalls, 0)
	atomic.StoreInt64(&globalPerformanceStats.SubscriptionCreated, 0)
	atomic.StoreInt64(&globalPerformanceStats.SubscriptionCanceled, 0)
	atomic.StoreInt64(&globalPerformanceStats.ItemsAllocated, 0)
	atomic.StoreInt64(&globalPerformanceStats.ItemsPooled, 0)
	atomic.StoreInt64(&globalPerformanceStats.QueuePoolHits, 0)
	atomic.StoreInt64(&globalPerformanceStats.QueuePoolMiss, 0)
	atomic.StoreInt64(&globalPerformanceStats.FastPathUsed, 0)
	atomic.StoreInt64(&globalPerformanceStats.ScalarOptimized, 0)
	atomic.StoreInt64(&globalPerformanceStats.EmptyOptimized, 0)
	atomic.StoreInt64(&globalPerformanceStats.ErrorOptimized, 0)
	atomic.StoreInt64(&globalPerformanceStats.TotalProcessingTime, 0)
	atomic.StoreInt64(&globalPerformanceStats.AverageLatency, 0)
	atomic.StoreInt64(&globalPerformanceStats.MaxConcurrentObs, 0)
	atomic.StoreInt64(&globalPerformanceStats.CurrentActiveObs, 0)
}

// ============================================================================
// 性能监控辅助函数
// ============================================================================

// IncrementObserverCalls 增加Observer调用计数
func IncrementObserverCalls() {
	atomic.AddInt64(&globalPerformanceStats.ObserverCalls, 1)
}

// IncrementSubscriptionCreated 增加订阅创建计数
func IncrementSubscriptionCreated() {
	atomic.AddInt64(&globalPerformanceStats.SubscriptionCreated, 1)

	// 更新并发统计
	current := atomic.AddInt64(&globalPerformanceStats.CurrentActiveObs, 1)
	for {
		max := atomic.LoadInt64(&globalPerformanceStats.MaxConcurrentObs)
		if current <= max || atomic.CompareAndSwapInt64(&globalPerformanceStats.MaxConcurrentObs, max, current) {
			break
		}
	}
}

// IncrementSubscriptionCanceled 增加订阅取消计数
func IncrementSubscriptionCanceled() {
	atomic.AddInt64(&globalPerformanceStats.SubscriptionCanceled, 1)
	atomic.AddInt64(&globalPerformanceStats.CurrentActiveObs, -1)
}

// IncrementItemsAllocated 增加Item分配计数
func IncrementItemsAllocated() {
	atomic.AddInt64(&globalPerformanceStats.ItemsAllocated, 1)
}

// IncrementItemsPooled 增加Item池化计数
func IncrementItemsPooled() {
	atomic.AddInt64(&globalPerformanceStats.ItemsPooled, 1)
}

// IncrementFastPathUsed 增加Fast-Path使用计数
func IncrementFastPathUsed() {
	atomic.AddInt64(&globalPerformanceStats.FastPathUsed, 1)
}

// IncrementScalarOptimized 增加标量优化计数
func IncrementScalarOptimized() {
	atomic.AddInt64(&globalPerformanceStats.ScalarOptimized, 1)
}

// IncrementEmptyOptimized 增加空Observable优化计数
func IncrementEmptyOptimized() {
	atomic.AddInt64(&globalPerformanceStats.EmptyOptimized, 1)
}

// IncrementErrorOptimized 增加错误Observable优化计数
func IncrementErrorOptimized() {
	atomic.AddInt64(&globalPerformanceStats.ErrorOptimized, 1)
}

// AddProcessingTime 添加处理时间
func AddProcessingTime(duration time.Duration) {
	atomic.AddInt64(&globalPerformanceStats.TotalProcessingTime, duration.Nanoseconds())
}

// ============================================================================
// 统计摘要和报告
// ============================================================================

// StatsSummary 统计摘要
type StatsSummary struct {
	MemoryEfficiency    float64 // 内存效率 (池化比例)
	ConcurrencyLevel    float64 // 并发水平
	OptimizationRatio   float64 // 优化比例
	AverageLatencyMs    float64 // 平均延迟(毫秒)
	ThroughputPerSecond float64 // 每秒吞吐量
}

// GetStatsSummary 获取统计摘要
func GetStatsSummary() StatsSummary {
	stats := GetPerformanceStats()

	summary := StatsSummary{}

	// 计算内存效率
	if stats.ItemsAllocated > 0 {
		summary.MemoryEfficiency = float64(stats.ItemsPooled) / float64(stats.ItemsAllocated) * 100
	}

	// 计算并发水平
	summary.ConcurrencyLevel = float64(stats.MaxConcurrentObs)

	// 计算优化比例
	totalOptimizations := stats.FastPathUsed + stats.ScalarOptimized + stats.EmptyOptimized + stats.ErrorOptimized
	if stats.SubscriptionCreated > 0 {
		summary.OptimizationRatio = float64(totalOptimizations) / float64(stats.SubscriptionCreated) * 100
	}

	// 计算平均延迟(毫秒)
	if stats.ObserverCalls > 0 {
		summary.AverageLatencyMs = float64(stats.TotalProcessingTime) / float64(stats.ObserverCalls) / 1e6
	}

	// 计算吞吐量(假设1秒窗口)
	summary.ThroughputPerSecond = float64(stats.ObserverCalls)

	return summary
}

// ============================================================================
// 内存管理工具函数
// ============================================================================

// ForceGC 强制垃圾回收
func ForceGC() {
	runtime.GC()
	runtime.GC() // 二次GC确保清理完整
}

// GetMemoryUsage 获取内存使用情况
func GetMemoryUsage() (alloc, totalAlloc, sys uint64, numGC uint32) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc, m.TotalAlloc, m.Sys, m.NumGC
}

// PrintMemoryStats 打印内存统计信息
func PrintMemoryStats() StatsSummary {
	summary := GetStatsSummary()

	// 这里可以添加日志输出逻辑
	// 由于没有log包，暂时返回摘要
	return summary
}

// ============================================================================
// 实用工具函数
// ============================================================================

// GetPooledItem 获取池化的Item对象
func GetPooledItem() *Item {
	IncrementItemsAllocated()
	item := GlobalItemPool.Get()
	IncrementItemsPooled()
	return item
}

// ReturnPooledItem 归还池化的Item对象
func ReturnPooledItem(item *Item) {
	GlobalItemPool.Put(item)
}

// GetPooledQueue 获取池化的队列
func GetPooledQueue(capacity int) []interface{} {
	return GlobalQueuePool.GetQueue(capacity)
}

// ReturnPooledQueue 归还池化的队列
func ReturnPooledQueue(queue []interface{}) {
	GlobalQueuePool.PutQueue(queue)
}

// GetGlobalPoolStats 获取全局对象池统计
func GetGlobalPoolStats() (itemGets, itemPuts int64) {
	return GlobalItemPool.Stats()
}

// ============================================================================
// 内存优化配置
// ============================================================================

// MemoryOptimizationConfig 内存优化配置
type MemoryOptimizationConfig struct {
	EnableItemPooling  bool
	EnableQueuePooling bool
	EnableStats        bool
	MaxPoolSize        int
}

// DefaultMemoryOptimizationConfig 默认内存优化配置
var DefaultMemoryOptimizationConfig = MemoryOptimizationConfig{
	EnableItemPooling:  true,
	EnableQueuePooling: true,
	EnableStats:        true,
	MaxPoolSize:        1000,
}

// ApplyMemoryOptimization 应用内存优化配置
func ApplyMemoryOptimization(config MemoryOptimizationConfig) {
	// 可以在这里根据配置调整池的行为
	// 由于Go的sync.Pool没有直接的大小控制，这里主要是示意
}

// IsMemoryOptimizationEnabled 检查是否启用了内存优化
func IsMemoryOptimizationEnabled() bool {
	return DefaultMemoryOptimizationConfig.EnableItemPooling ||
		DefaultMemoryOptimizationConfig.EnableQueuePooling
}
