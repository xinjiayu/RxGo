package main

import (
	"fmt"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xinjiayu/rxgo"
)

// ============================================================================
// 日志分析器
// ============================================================================

// LogAnalytics 日志分析器
type LogAnalytics struct {
	parallelism int
}

// NewLogAnalytics 创建日志分析器
func NewLogAnalytics() *LogAnalytics {
	return &LogAnalytics{
		parallelism: runtime.NumCPU(),
	}
}

// LogEntry 日志条目
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Source    string    `json:"source"`
	UserID    string    `json:"user_id,omitempty"`
	IP        string    `json:"ip,omitempty"`
	Size      int       `json:"size"`
}

// LogSummary 日志汇总
type LogSummary struct {
	Level       string    `json:"level"`
	Count       int       `json:"count"`
	TotalSize   int       `json:"total_size"`
	TimeRange   string    `json:"time_range"`
	ProcessedAt time.Time `json:"processed_at"`
	WorkerID    int       `json:"worker_id"`
}

// RunExample 运行日志分析示例
func (la *LogAnalytics) RunExample() {
	fmt.Printf("📈 启动日志分析器 (并行度: %d)\n", la.parallelism)

	// 1. 生成模拟日志数据
	logs := la.generateLogEntries(100000)
	fmt.Printf("📋 生成了 %d 条日志记录\n", len(logs))

	// 2. 并行分析日志
	start := time.Now()
	summaries := la.analyzeLogsParallel(logs)
	duration := time.Since(start)

	fmt.Printf("⚡ 并行日志分析完成: 生成了 %d 个汇总，耗时 %v\n", len(summaries), duration)

	// 3. 分析结果
	la.analyzeSummaries(summaries)
}

// generateLogEntries 生成日志条目
func (la *LogAnalytics) generateLogEntries(count int) []LogEntry {
	levels := []string{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"}
	sources := []string{"web-server", "api-gateway", "database", "cache", "worker", "scheduler"}
	messages := []string{
		"请求处理完成", "数据库连接建立", "缓存命中", "用户登录成功", "文件上传完成",
		"API调用超时", "内存使用过高", "磁盘空间不足", "网络连接异常", "服务启动",
	}

	logs := make([]LogEntry, count)
	baseTime := time.Now().Add(-24 * time.Hour)

	for i := 0; i < count; i++ {
		logs[i] = LogEntry{
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			Level:     levels[rand.Intn(len(levels))],
			Message:   messages[rand.Intn(len(messages))],
			Source:    sources[rand.Intn(len(sources))],
			UserID:    fmt.Sprintf("user_%d", rand.Intn(1000)),
			IP:        fmt.Sprintf("192.168.%d.%d", rand.Intn(255), rand.Intn(255)),
			Size:      rand.Intn(1000) + 100,
		}
	}

	return logs
}

// analyzeLogsParallel 并行分析日志
func (la *LogAnalytics) analyzeLogsParallel(logs []LogEntry) []LogSummary {
	var summaries []LogSummary
	var mu sync.Mutex
	var wg sync.WaitGroup
	var processedBatches int64

	// 将日志分批处理
	batchSize := 1000
	batches := la.createLogBatches(logs, batchSize)

	// 转换为interface{}切片
	batchInterfaces := make([]interface{}, len(batches))
	for i, batch := range batches {
		batchInterfaces[i] = batch
	}

	// 创建并行Flowable
	parallelFlowable := rxgo.ParallelFromSlice(batchInterfaces, la.parallelism).
		Map(func(v interface{}) (interface{}, error) {
			batch := v.([]LogEntry)
			workerID := int(atomic.AddInt64(&processedBatches, 1)) % la.parallelism

			// 分析单批日志
			batchSummaries := la.analyzeSingleBatch(batch, workerID)
			return batchSummaries, nil
		}).
		Filter(func(v interface{}) bool {
			summaries := v.([]LogSummary)
			return len(summaries) > 0
		})

	// 收集结果
	subscriptions := parallelFlowable.SubscribeWithCallbacks(
		func(value interface{}) {
			batchSummaries := value.([]LogSummary)
			mu.Lock()
			summaries = append(summaries, batchSummaries...)
			mu.Unlock()
		},
		func(err error) {
			fmt.Printf("❌ 日志分析错误: %v\n", err)
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
	return summaries
}

// createLogBatches 创建日志批次
func (la *LogAnalytics) createLogBatches(logs []LogEntry, batchSize int) [][]LogEntry {
	var batches [][]LogEntry
	for i := 0; i < len(logs); i += batchSize {
		end := i + batchSize
		if end > len(logs) {
			end = len(logs)
		}
		batches = append(batches, logs[i:end])
	}
	return batches
}

// analyzeSingleBatch 分析单批日志
func (la *LogAnalytics) analyzeSingleBatch(logs []LogEntry, workerID int) []LogSummary {
	// 按级别统计
	levelStats := make(map[string]struct {
		count     int
		totalSize int
		minTime   time.Time
		maxTime   time.Time
	})

	for _, log := range logs {
		stats := levelStats[log.Level]
		stats.count++
		stats.totalSize += log.Size

		if stats.minTime.IsZero() || log.Timestamp.Before(stats.minTime) {
			stats.minTime = log.Timestamp
		}
		if stats.maxTime.IsZero() || log.Timestamp.After(stats.maxTime) {
			stats.maxTime = log.Timestamp
		}

		levelStats[log.Level] = stats
	}

	// 转换为汇总
	var summaries []LogSummary
	for level, stats := range levelStats {
		timeRange := fmt.Sprintf("%s ~ %s",
			stats.minTime.Format("15:04:05"),
			stats.maxTime.Format("15:04:05"))

		summaries = append(summaries, LogSummary{
			Level:       level,
			Count:       stats.count,
			TotalSize:   stats.totalSize,
			TimeRange:   timeRange,
			ProcessedAt: time.Now(),
			WorkerID:    workerID,
		})
	}

	return summaries
}

// analyzeSummaries 分析汇总结果
func (la *LogAnalytics) analyzeSummaries(summaries []LogSummary) {
	fmt.Println("\n📈 日志分析结果:")

	// 按级别聚合
	levelTotals := make(map[string]struct {
		count     int
		totalSize int
	})

	for _, summary := range summaries {
		totals := levelTotals[summary.Level]
		totals.count += summary.Count
		totals.totalSize += summary.TotalSize
		levelTotals[summary.Level] = totals
	}

	// 输出结果
	fmt.Printf("📊 按日志级别统计:\n")
	totalLogs := 0
	for level, totals := range levelTotals {
		fmt.Printf("   %s: %d条日志, %.2fKB\n", level, totals.count, float64(totals.totalSize)/1024)
		totalLogs += totals.count
	}
	fmt.Printf("   总计: %d条日志\n", totalLogs)
}

// ============================================================================
// 流处理器
// ============================================================================

// StreamProcessor 流处理器
type StreamProcessor struct {
	parallelism int
}

// NewStreamProcessor 创建流处理器
func NewStreamProcessor() *StreamProcessor {
	return &StreamProcessor{
		parallelism: runtime.NumCPU(),
	}
}

// RunExample 运行流处理示例
func (sp *StreamProcessor) RunExample() {
	fmt.Printf("🌊 启动流处理器 (并行度: %d)\n", sp.parallelism)

	// 模拟实时数据流处理
	fmt.Println("📡 模拟实时数据流处理...")

	// 创建数据流
	dataStream := sp.createDataStream(1000)

	// 并行处理流数据
	start := time.Now()
	results := sp.processStreamParallel(dataStream)
	duration := time.Since(start)

	fmt.Printf("⚡ 流处理完成: 处理了 %d 个数据包，耗时 %v\n", len(results), duration)
	fmt.Printf("📈 处理速度: %.2f 包/秒\n", float64(len(results))/duration.Seconds())
}

// createDataStream 创建数据流
func (sp *StreamProcessor) createDataStream(count int) []interface{} {
	stream := make([]interface{}, count)
	for i := 0; i < count; i++ {
		stream[i] = map[string]interface{}{
			"id":        i + 1,
			"timestamp": time.Now(),
			"value":     rand.Float64() * 100,
			"category":  fmt.Sprintf("cat_%d", rand.Intn(5)),
		}
	}
	return stream
}

// processStreamParallel 并行处理流数据
func (sp *StreamProcessor) processStreamParallel(stream []interface{}) []interface{} {
	var results []interface{}
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 创建并行Flowable
	parallelFlowable := rxgo.ParallelFromSlice(stream, sp.parallelism).
		Map(func(v interface{}) (interface{}, error) {
			data := v.(map[string]interface{})

			// 模拟流数据处理
			processed := map[string]interface{}{
				"id":              data["id"],
				"original_value":  data["value"],
				"processed_value": data["value"].(float64) * 2,
				"category":        data["category"],
				"processed_at":    time.Now(),
			}

			// 模拟处理延迟
			time.Sleep(time.Microsecond * 50)

			return processed, nil
		})

	// 收集结果
	subscriptions := parallelFlowable.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			results = append(results, value)
			mu.Unlock()
		},
		func(err error) {
			fmt.Printf("❌ 流处理错误: %v\n", err)
		},
		func() {
			wg.Done()
		},
	)

	wg.Add(len(subscriptions))

	// 请求所有数据
	for _, subscription := range subscriptions {
		subscription.Request(int64(len(stream)))
	}

	wg.Wait()
	return results
}

// ============================================================================
// MapReduce处理器
// ============================================================================

// MapReduceProcessor MapReduce处理器
type MapReduceProcessor struct {
	parallelism int
}

// NewMapReduceProcessor 创建MapReduce处理器
func NewMapReduceProcessor() *MapReduceProcessor {
	return &MapReduceProcessor{
		parallelism: runtime.NumCPU(),
	}
}

// RunExample 运行MapReduce示例
func (mrp *MapReduceProcessor) RunExample() {
	fmt.Printf("🗺️ 启动MapReduce处理器 (并行度: %d)\n", mrp.parallelism)

	// 示例：词频统计
	fmt.Println("📄 执行大规模文本词频统计...")

	// 生成模拟文档
	documents := mrp.generateDocuments(1000)
	fmt.Printf("📚 生成了 %d 个文档\n", len(documents))

	// 执行MapReduce
	start := time.Now()
	wordCounts := mrp.wordCountMapReduce(documents)
	duration := time.Since(start)

	fmt.Printf("⚡ MapReduce完成: 统计了 %d 个唯一词汇，耗时 %v\n", len(wordCounts), duration)

	// 输出前10个最频繁的词
	mrp.showTopWords(wordCounts, 10)
}

// generateDocuments 生成模拟文档
func (mrp *MapReduceProcessor) generateDocuments(count int) []string {
	words := []string{
		"golang", "programming", "concurrent", "parallel", "reactive", "stream",
		"data", "processing", "algorithm", "performance", "optimization", "scalable",
		"distributed", "system", "architecture", "microservice", "cloud", "computing",
	}

	documents := make([]string, count)
	for i := 0; i < count; i++ {
		// 每个文档包含随机数量的随机词汇
		docLength := rand.Intn(50) + 10
		var docWords []string
		for j := 0; j < docLength; j++ {
			docWords = append(docWords, words[rand.Intn(len(words))])
		}
		documents[i] = strings.Join(docWords, " ")
	}

	return documents
}

// wordCountMapReduce 执行词频统计MapReduce
func (mrp *MapReduceProcessor) wordCountMapReduce(documents []string) map[string]int {
	// 转换为interface{}切片
	docInterfaces := make([]interface{}, len(documents))
	for i, doc := range documents {
		docInterfaces[i] = doc
	}

	// Map阶段：并行处理每个文档，输出词汇计数
	var allWordCounts []map[string]int
	var mu sync.Mutex
	var wg sync.WaitGroup

	parallelFlowable := rxgo.ParallelFromSlice(docInterfaces, mrp.parallelism).
		Map(func(v interface{}) (interface{}, error) {
			document := v.(string)

			// Map阶段：统计单个文档的词频
			wordCount := make(map[string]int)
			words := strings.Fields(document)
			for _, word := range words {
				word = strings.ToLower(strings.Trim(word, ".,!?;"))
				if len(word) > 0 {
					wordCount[word]++
				}
			}

			return wordCount, nil
		})

	// 收集Map结果
	subscriptions := parallelFlowable.SubscribeWithCallbacks(
		func(value interface{}) {
			wordCount := value.(map[string]int)
			mu.Lock()
			allWordCounts = append(allWordCounts, wordCount)
			mu.Unlock()
		},
		func(err error) {
			fmt.Printf("❌ Map阶段错误: %v\n", err)
		},
		func() {
			wg.Done()
		},
	)

	wg.Add(len(subscriptions))

	// 请求所有数据
	for _, subscription := range subscriptions {
		subscription.Request(int64(len(documents)))
	}

	wg.Wait()

	// Reduce阶段：合并所有词频统计
	finalWordCount := make(map[string]int)
	for _, wordCount := range allWordCounts {
		for word, count := range wordCount {
			finalWordCount[word] += count
		}
	}

	return finalWordCount
}

// showTopWords 显示最频繁的词汇
func (mrp *MapReduceProcessor) showTopWords(wordCounts map[string]int, topN int) {
	// 转换为切片并排序
	type wordFreq struct {
		word  string
		count int
	}

	var frequencies []wordFreq
	for word, count := range wordCounts {
		frequencies = append(frequencies, wordFreq{word, count})
	}

	// 简单的冒泡排序（演示用）
	for i := 0; i < len(frequencies)-1; i++ {
		for j := 0; j < len(frequencies)-i-1; j++ {
			if frequencies[j].count < frequencies[j+1].count {
				frequencies[j], frequencies[j+1] = frequencies[j+1], frequencies[j]
			}
		}
	}

	fmt.Printf("📊 前 %d 个最频繁的词汇:\n", topN)
	for i := 0; i < topN && i < len(frequencies); i++ {
		freq := frequencies[i]
		fmt.Printf("   %d. %s: %d次\n", i+1, freq.word, freq.count)
	}
}
