package main

import (
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xinjiayu/rxgo"
)

// BatchProcessor 批处理器
type BatchProcessor struct {
	batchSize   int
	parallelism int
}

// NewBatchProcessor 创建批处理器
func NewBatchProcessor() *BatchProcessor {
	return &BatchProcessor{
		batchSize:   1000,
		parallelism: runtime.NumCPU(),
	}
}

// BatchItem 批处理项
type BatchItem struct {
	ID       int     `json:"id"`
	Data     string  `json:"data"`
	Size     int     `json:"size"`
	Priority int     `json:"priority"`
	Value    float64 `json:"value"`
}

// BatchResult 批处理结果
type BatchResult struct {
	BatchID      int         `json:"batch_id"`
	ProcessedAt  time.Time   `json:"processed_at"`
	ItemCount    int         `json:"item_count"`
	TotalSize    int         `json:"total_size"`
	ProcessingMS int64       `json:"processing_ms"`
	WorkerID     int         `json:"worker_id"`
	Items        []BatchItem `json:"items"`
}

// RunExample 运行批处理示例
func (bp *BatchProcessor) RunExample() {
	fmt.Printf("📦 启动批处理器 (批大小: %d, 并行度: %d)\n", bp.batchSize, bp.parallelism)

	// 1. 生成大量数据
	totalItems := 50000
	items := bp.generateBatchItems(totalItems)
	fmt.Printf("📋 生成了 %d 个待处理项\n", len(items))

	// 2. 将数据分批
	batches := bp.createBatches(items)
	fmt.Printf("📦 创建了 %d 个批次\n", len(batches))

	// 3. 并行处理批次
	start := time.Now()
	results := bp.processBatchesParallel(batches)
	duration := time.Since(start)

	fmt.Printf("⚡ 并行批处理完成: 处理了 %d 个批次，耗时 %v\n", len(results), duration)

	// 4. 串行处理对比
	start = time.Now()
	serialResults := bp.processBatchesSerial(batches)
	serialDuration := time.Since(start)

	fmt.Printf("🐌 串行批处理完成: 处理了 %d 个批次，耗时 %v\n", len(serialResults), serialDuration)
	fmt.Printf("🚀 性能提升: %.2fx\n", float64(serialDuration)/float64(duration))

	// 5. 分析批处理结果
	bp.analyzeBatchResults(results)
}

// generateBatchItems 生成批处理项
func (bp *BatchProcessor) generateBatchItems(count int) []BatchItem {
	items := make([]BatchItem, count)
	dataTemplates := []string{"用户数据", "订单信息", "产品详情", "日志记录", "系统事件"}

	for i := 0; i < count; i++ {
		template := dataTemplates[rand.Intn(len(dataTemplates))]
		items[i] = BatchItem{
			ID:       i + 1,
			Data:     fmt.Sprintf("%s_%d", template, i),
			Size:     rand.Intn(1000) + 100,
			Priority: rand.Intn(5) + 1,
			Value:    rand.Float64() * 1000,
		}
	}

	return items
}

// createBatches 创建批次
func (bp *BatchProcessor) createBatches(items []BatchItem) [][]BatchItem {
	var batches [][]BatchItem

	for i := 0; i < len(items); i += bp.batchSize {
		end := i + bp.batchSize
		if end > len(items) {
			end = len(items)
		}
		batches = append(batches, items[i:end])
	}

	return batches
}

// processBatchesParallel 并行处理批次
func (bp *BatchProcessor) processBatchesParallel(batches [][]BatchItem) []BatchResult {
	var results []BatchResult
	var mu sync.Mutex
	var wg sync.WaitGroup
	var processedBatches int64

	// 转换为interface{}切片
	batchInterfaces := make([]interface{}, len(batches))
	for i, batch := range batches {
		batchInterfaces[i] = batch
	}

	// 创建并行Flowable
	parallelFlowable := rxgo.ParallelFromSlice(batchInterfaces, bp.parallelism).
		Map(func(v interface{}) (interface{}, error) {
			batch := v.([]BatchItem)
			batchID := int(atomic.AddInt64(&processedBatches, 1))
			workerID := (batchID - 1) % bp.parallelism

			processingStart := time.Now()

			// 模拟批处理逻辑
			processedBatch := bp.processSingleBatch(batchID, batch, workerID)

			processingDuration := time.Since(processingStart)
			processedBatch.ProcessingMS = processingDuration.Milliseconds()

			return processedBatch, nil
		}).
		Filter(func(v interface{}) bool {
			// 可以在这里添加批处理结果的过滤逻辑
			result := v.(BatchResult)
			return result.ItemCount > 0
		})

	// 收集结果
	subscriptions := parallelFlowable.SubscribeWithCallbacks(
		func(value interface{}) {
			result := value.(BatchResult)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		},
		func(err error) {
			fmt.Printf("❌ 批处理错误: %v\n", err)
		},
		func() {
			wg.Done()
		},
	)

	wg.Add(len(subscriptions))

	// 请求所有数据
	for _, subscription := range subscriptions {
		subscription.Request(int64(len(batches)))
	}

	wg.Wait()
	return results
}

// processBatchesSerial 串行处理批次
func (bp *BatchProcessor) processBatchesSerial(batches [][]BatchItem) []BatchResult {
	var results []BatchResult

	for i, batch := range batches {
		processingStart := time.Now()
		result := bp.processSingleBatch(i+1, batch, 0)
		result.ProcessingMS = time.Since(processingStart).Milliseconds()
		results = append(results, result)
	}

	return results
}

// processSingleBatch 处理单个批次
func (bp *BatchProcessor) processSingleBatch(batchID int, items []BatchItem, workerID int) BatchResult {
	// 模拟批处理操作
	var totalSize int
	processedItems := make([]BatchItem, len(items))

	for i, item := range items {
		// 模拟数据处理
		processedItem := item
		processedItem.Value = processedItem.Value * 1.1 // 简单的值转换

		processedItems[i] = processedItem
		totalSize += item.Size

		// 模拟处理延迟
		time.Sleep(time.Microsecond * 10)
	}

	return BatchResult{
		BatchID:     batchID,
		ProcessedAt: time.Now(),
		ItemCount:   len(processedItems),
		TotalSize:   totalSize,
		WorkerID:    workerID,
		Items:       processedItems,
	}
}

// analyzeBatchResults 分析批处理结果
func (bp *BatchProcessor) analyzeBatchResults(results []BatchResult) {
	if len(results) == 0 {
		fmt.Println("❌ 没有批处理结果可分析")
		return
	}

	fmt.Println("\n📈 批处理结果分析:")

	// 统计信息
	var totalItems, totalSize int
	var totalProcessingTime int64
	workerStats := make(map[int]struct {
		batches     int
		items       int
		totalTimeMS int64
	})

	for _, result := range results {
		totalItems += result.ItemCount
		totalSize += result.TotalSize
		totalProcessingTime += result.ProcessingMS

		stats := workerStats[result.WorkerID]
		stats.batches++
		stats.items += result.ItemCount
		stats.totalTimeMS += result.ProcessingMS
		workerStats[result.WorkerID] = stats
	}

	// 输出总体统计
	fmt.Printf("📊 总体统计:\n")
	fmt.Printf("   总批次数: %d\n", len(results))
	fmt.Printf("   总项目数: %d\n", totalItems)
	fmt.Printf("   总数据大小: %d bytes\n", totalSize)
	fmt.Printf("   总处理时间: %d ms\n", totalProcessingTime)
	fmt.Printf("   平均批次处理时间: %.2f ms\n", float64(totalProcessingTime)/float64(len(results)))

	// 输出工作者统计
	fmt.Printf("🔧 工作者统计:\n")
	for workerID, stats := range workerStats {
		avgTimePerBatch := float64(stats.totalTimeMS) / float64(stats.batches)
		fmt.Printf("   Worker %d: %d批次, %d项目, 平均%.2fms/批次\n",
			workerID, stats.batches, stats.items, avgTimePerBatch)
	}

	// 计算吞吐量
	if totalProcessingTime > 0 {
		throughputPerSecond := float64(totalItems) / (float64(totalProcessingTime) / 1000.0)
		fmt.Printf("📈 处理吞吐量: %.2f 项目/秒\n", throughputPerSecond)
	}
}
