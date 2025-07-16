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

// DataProcessingPipeline 数据处理管道
type DataProcessingPipeline struct {
	parallelism int
}

// NewDataProcessingPipeline 创建数据处理管道
func NewDataProcessingPipeline() *DataProcessingPipeline {
	return &DataProcessingPipeline{
		parallelism: runtime.NumCPU(),
	}
}

// DataRecord 数据记录结构
type DataRecord struct {
	ID        int       `json:"id"`
	Value     float64   `json:"value"`
	Category  string    `json:"category"`
	Timestamp time.Time `json:"timestamp"`
}

// ProcessedData 处理后的数据
type ProcessedData struct {
	ID              int     `json:"id"`
	OriginalValue   float64 `json:"original_value"`
	ProcessedValue  float64 `json:"processed_value"`
	Category        string  `json:"category"`
	ProcessingTime  int64   `json:"processing_time_ns"`
	ProcessorWorker int     `json:"processor_worker"`
}

// RunExample 运行数据处理管道示例
func (dp *DataProcessingPipeline) RunExample() {
	fmt.Printf("🔧 启动数据处理管道 (并行度: %d)\n", dp.parallelism)

	// 1. 生成大量测试数据
	testData := dp.generateTestData(10000)
	fmt.Printf("📋 生成了 %d 条测试数据\n", len(testData))

	// 2. 使用ParallelFlowable进行并行处理
	start := time.Now()
	results := dp.processDataParallel(testData)
	duration := time.Since(start)

	fmt.Printf("⚡ 并行处理完成: 处理了 %d 条记录，耗时 %v\n", len(results), duration)
	fmt.Printf("📊 处理速度: %.2f 记录/秒\n", float64(len(results))/duration.Seconds())

	// 3. 性能对比：串行处理
	start = time.Now()
	serialResults := dp.processDataSerial(testData)
	serialDuration := time.Since(start)

	fmt.Printf("🐌 串行处理完成: 处理了 %d 条记录，耗时 %v\n", len(serialResults), serialDuration)
	fmt.Printf("🚀 性能提升: %.2fx\n", float64(serialDuration)/float64(duration))

	// 4. 分析处理结果
	dp.analyzeResults(results)
}

// generateTestData 生成测试数据
func (dp *DataProcessingPipeline) generateTestData(count int) []interface{} {
	categories := []string{"A", "B", "C", "D", "E"}
	data := make([]interface{}, count)

	for i := 0; i < count; i++ {
		data[i] = DataRecord{
			ID:        i + 1,
			Value:     rand.Float64() * 1000,
			Category:  categories[rand.Intn(len(categories))],
			Timestamp: time.Now().Add(time.Duration(i) * time.Millisecond),
		}
	}

	return data
}

// processDataParallel 并行处理数据
func (dp *DataProcessingPipeline) processDataParallel(data []interface{}) []ProcessedData {
	var results []ProcessedData
	var mu sync.Mutex
	var processedCount int64
	var wg sync.WaitGroup

	// 创建并行Flowable
	parallelFlowable := rxgo.ParallelFromSlice(data, dp.parallelism).
		Map(func(v interface{}) (interface{}, error) {
			// 模拟复杂的数据处理操作
			record := v.(DataRecord)
			processingStart := time.Now()

			// 模拟CPU密集型计算
			processedValue := dp.complexCalculation(record.Value)

			// 模拟一些处理延迟
			time.Sleep(time.Microsecond * 100)

			processingTime := time.Since(processingStart).Nanoseconds()

			processed := ProcessedData{
				ID:              record.ID,
				OriginalValue:   record.Value,
				ProcessedValue:  processedValue,
				Category:        record.Category,
				ProcessingTime:  processingTime,
				ProcessorWorker: int(atomic.AddInt64(&processedCount, 1)) % dp.parallelism,
			}

			return processed, nil
		}).
		Filter(func(v interface{}) bool {
			// 过滤掉一些无效数据
			processed := v.(ProcessedData)
			return processed.ProcessedValue > 0
		})

	// 收集所有分区的结果
	subscriptions := parallelFlowable.SubscribeWithCallbacks(
		func(value interface{}) {
			processed := value.(ProcessedData)
			mu.Lock()
			results = append(results, processed)
			mu.Unlock()
		},
		func(err error) {
			fmt.Printf("❌ 处理错误: %v\n", err)
		},
		func() {
			wg.Done()
		},
	)

	wg.Add(len(subscriptions))

	// 请求所有数据
	for _, subscription := range subscriptions {
		subscription.Request(int64(len(data)))
	}

	wg.Wait()
	return results
}

// processDataSerial 串行处理数据（用于性能对比）
func (dp *DataProcessingPipeline) processDataSerial(data []interface{}) []ProcessedData {
	var results []ProcessedData

	for _, item := range data {
		record := item.(DataRecord)
		processingStart := time.Now()

		// 相同的计算逻辑
		processedValue := dp.complexCalculation(record.Value)

		// 相同的处理延迟
		time.Sleep(time.Microsecond * 100)

		processingTime := time.Since(processingStart).Nanoseconds()

		if processedValue > 0 { // 相同的过滤条件
			processed := ProcessedData{
				ID:              record.ID,
				OriginalValue:   record.Value,
				ProcessedValue:  processedValue,
				Category:        record.Category,
				ProcessingTime:  processingTime,
				ProcessorWorker: 0, // 串行处理只有一个worker
			}
			results = append(results, processed)
		}
	}

	return results
}

// complexCalculation 模拟复杂计算
func (dp *DataProcessingPipeline) complexCalculation(value float64) float64 {
	// 模拟一些数学运算
	result := value
	for i := 0; i < 100; i++ {
		result = result*0.99 + float64(i)*0.01
	}
	return result
}

// analyzeResults 分析处理结果
func (dp *DataProcessingPipeline) analyzeResults(results []ProcessedData) {
	if len(results) == 0 {
		fmt.Println("❌ 没有处理结果可分析")
		return
	}

	fmt.Println("\n📈 处理结果分析:")

	// 按类别统计
	categoryStats := make(map[string]int)
	workerStats := make(map[int]int)
	var totalProcessingTime int64

	for _, result := range results {
		categoryStats[result.Category]++
		workerStats[result.ProcessorWorker]++
		totalProcessingTime += result.ProcessingTime
	}

	// 输出统计信息
	fmt.Println("📊 按类别统计:")
	for category, count := range categoryStats {
		fmt.Printf("   类别 %s: %d 条记录\n", category, count)
	}

	fmt.Println("🔧 按处理器统计:")
	for worker, count := range workerStats {
		fmt.Printf("   Worker %d: %d 条记录\n", worker, count)
	}

	avgProcessingTime := float64(totalProcessingTime) / float64(len(results)) / 1000000 // 转换为毫秒
	fmt.Printf("⏱️  平均处理时间: %.2f ms/记录\n", avgProcessingTime)
}
