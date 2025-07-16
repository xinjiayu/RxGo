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

// NetworkCrawler 网络爬虫
type NetworkCrawler struct {
	maxConcurrency int
	timeout        time.Duration
}

// NewNetworkCrawler 创建网络爬虫
func NewNetworkCrawler() *NetworkCrawler {
	return &NetworkCrawler{
		maxConcurrency: runtime.NumCPU() * 2, // 网络IO可以设置更高的并发度
		timeout:        5 * time.Second,
	}
}

// WebPage 网页信息
type WebPage struct {
	URL      string   `json:"url"`
	Title    string   `json:"title"`
	Size     int      `json:"size"`
	LoadTime int64    `json:"load_time_ms"`
	Status   int      `json:"status"`
	Links    []string `json:"links"`
	Depth    int      `json:"depth"`
}

// CrawlResult 爬取结果
type CrawlResult struct {
	URL         string    `json:"url"`
	Success     bool      `json:"success"`
	StatusCode  int       `json:"status_code"`
	ContentSize int       `json:"content_size"`
	ResponseMS  int64     `json:"response_ms"`
	WorkerID    int       `json:"worker_id"`
	Timestamp   time.Time `json:"timestamp"`
	Error       string    `json:"error,omitempty"`
}

// RunExample 运行网络爬虫示例
func (nc *NetworkCrawler) RunExample() {
	fmt.Printf("🕷️ 启动网络爬虫 (最大并发: %d)\n", nc.maxConcurrency)

	// 1. 生成模拟URL列表
	urls := nc.generateURLs(200)
	fmt.Printf("🌐 生成了 %d 个待爬取URL\n", len(urls))

	// 2. 并行爬取
	start := time.Now()
	results := nc.crawlURLsParallel(urls)
	duration := time.Since(start)

	fmt.Printf("⚡ 并行爬取完成: 处理了 %d 个URL，耗时 %v\n", len(results), duration)

	// 3. 串行爬取对比
	// 只取部分URL进行串行对比，避免耗时过长
	testURLs := urls[:50]
	start = time.Now()
	_ = nc.crawlURLsSerial(testURLs) // 串行结果仅用于性能对比
	serialDuration := time.Since(start)

	// 按比例计算预期的串行总时间
	estimatedSerialTime := time.Duration(float64(serialDuration) * float64(len(urls)) / float64(len(testURLs)))

	fmt.Printf("🐌 串行爬取样本完成: %d个URL耗时 %v\n", len(testURLs), serialDuration)
	fmt.Printf("📊 预估串行总时间: %v\n", estimatedSerialTime)
	fmt.Printf("🚀 性能提升: %.2fx\n", float64(estimatedSerialTime)/float64(duration))

	// 4. 分析爬取结果
	nc.analyzeCrawlResults(results)
}

// generateURLs 生成模拟URL列表
func (nc *NetworkCrawler) generateURLs(count int) []string {
	domains := []string{
		"example.com", "test.org", "demo.net", "sample.io", "mock.dev",
		"api.service", "web.app", "data.site", "news.portal", "blog.platform",
	}

	paths := []string{
		"/", "/home", "/about", "/contact", "/products", "/services",
		"/blog", "/news", "/api/v1/users", "/api/v1/data", "/search",
		"/category/tech", "/category/business", "/archives", "/help", "/docs",
	}

	urls := make([]string, count)
	for i := 0; i < count; i++ {
		domain := domains[rand.Intn(len(domains))]
		path := paths[rand.Intn(len(paths))]
		urls[i] = fmt.Sprintf("https://%s%s?id=%d", domain, path, i+1)
	}

	return urls
}

// crawlURLsParallel 并行爬取URL
func (nc *NetworkCrawler) crawlURLsParallel(urls []string) []CrawlResult {
	var results []CrawlResult
	var mu sync.Mutex
	var wg sync.WaitGroup
	var processedCount int64

	// 转换为interface{}切片
	urlInterfaces := make([]interface{}, len(urls))
	for i, url := range urls {
		urlInterfaces[i] = url
	}

	// 创建并行Flowable
	parallelFlowable := rxgo.ParallelFromSlice(urlInterfaces, nc.maxConcurrency).
		Map(func(v interface{}) (interface{}, error) {
			url := v.(string)
			workerID := int(atomic.AddInt64(&processedCount, 1)) % nc.maxConcurrency

			// 爬取单个URL
			result := nc.crawlSingleURL(url, workerID)
			return result, nil
		}).
		Filter(func(v interface{}) bool {
			// 可以在这里过滤掉某些结果，比如只保留成功的请求
			return true // 保留所有结果用于分析
		})

	// 收集结果
	subscriptions := parallelFlowable.SubscribeWithCallbacks(
		func(value interface{}) {
			result := value.(CrawlResult)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		},
		func(err error) {
			fmt.Printf("❌ 爬取错误: %v\n", err)
		},
		func() {
			wg.Done()
		},
	)

	wg.Add(len(subscriptions))

	// 请求所有数据
	for _, subscription := range subscriptions {
		subscription.Request(int64(len(urls)))
	}

	wg.Wait()
	return results
}

// crawlURLsSerial 串行爬取URL
func (nc *NetworkCrawler) crawlURLsSerial(urls []string) []CrawlResult {
	var results []CrawlResult

	for _, url := range urls {
		result := nc.crawlSingleURL(url, 0)
		results = append(results, result)
	}

	return results
}

// crawlSingleURL 爬取单个URL
func (nc *NetworkCrawler) crawlSingleURL(url string, workerID int) CrawlResult {
	start := time.Now()

	// 模拟HTTP请求
	_, statusCode, contentSize, err := nc.simulateHTTPRequest(url)

	result := CrawlResult{
		URL:         url,
		Success:     err == nil,
		StatusCode:  statusCode,
		ContentSize: contentSize,
		ResponseMS:  time.Since(start).Milliseconds(),
		WorkerID:    workerID,
		Timestamp:   time.Now(),
	}

	if err != nil {
		result.Error = err.Error()
	}

	return result
}

// simulateHTTPRequest 模拟HTTP请求
func (nc *NetworkCrawler) simulateHTTPRequest(url string) (time.Duration, int, int, error) {
	// 模拟不同的响应时间
	responseTime := time.Duration(rand.Intn(200)+50) * time.Millisecond
	time.Sleep(responseTime)

	// 模拟不同的状态码
	statusCodes := []int{200, 200, 200, 200, 200, 404, 500, 503} // 大部分成功
	statusCode := statusCodes[rand.Intn(len(statusCodes))]

	// 模拟不同的内容大小
	contentSize := rand.Intn(100000) + 1000

	// 模拟错误情况
	if statusCode >= 400 {
		return responseTime, statusCode, 0, fmt.Errorf("HTTP %d error", statusCode)
	}

	return responseTime, statusCode, contentSize, nil
}

// analyzeCrawlResults 分析爬取结果
func (nc *NetworkCrawler) analyzeCrawlResults(results []CrawlResult) {
	if len(results) == 0 {
		fmt.Println("❌ 没有爬取结果可分析")
		return
	}

	fmt.Println("\n📈 爬取结果分析:")

	// 统计信息
	var successCount, errorCount int
	var totalResponseTime int64
	var totalContentSize int
	statusStats := make(map[int]int)
	workerStats := make(map[int]struct {
		requests    int
		successes   int
		totalTimeMS int64
		totalSizeKB int
	})

	for _, result := range results {
		if result.Success {
			successCount++
			totalContentSize += result.ContentSize
		} else {
			errorCount++
		}

		totalResponseTime += result.ResponseMS
		statusStats[result.StatusCode]++

		stats := workerStats[result.WorkerID]
		stats.requests++
		if result.Success {
			stats.successes++
			stats.totalSizeKB += result.ContentSize / 1024
		}
		stats.totalTimeMS += result.ResponseMS
		workerStats[result.WorkerID] = stats
	}

	// 输出总体统计
	fmt.Printf("📊 总体统计:\n")
	fmt.Printf("   总请求数: %d\n", len(results))
	fmt.Printf("   成功请求: %d (%.1f%%)\n", successCount, float64(successCount)*100/float64(len(results)))
	fmt.Printf("   失败请求: %d (%.1f%%)\n", errorCount, float64(errorCount)*100/float64(len(results)))
	fmt.Printf("   总内容大小: %.2f MB\n", float64(totalContentSize)/(1024*1024))
	fmt.Printf("   平均响应时间: %.2f ms\n", float64(totalResponseTime)/float64(len(results)))

	// 状态码分布
	fmt.Printf("📋 状态码分布:\n")
	for status, count := range statusStats {
		percentage := float64(count) * 100 / float64(len(results))
		fmt.Printf("   %d: %d次 (%.1f%%)\n", status, count, percentage)
	}

	// 工作者统计
	fmt.Printf("🔧 工作者统计:\n")
	for workerID, stats := range workerStats {
		avgTimePerRequest := float64(stats.totalTimeMS) / float64(stats.requests)
		successRate := float64(stats.successes) * 100 / float64(stats.requests)
		fmt.Printf("   Worker %d: %d请求, 成功率%.1f%%, 平均%.2fms/请求, 总计%.2fMB\n",
			workerID, stats.requests, successRate, avgTimePerRequest, float64(stats.totalSizeKB)/1024)
	}

	// 计算吞吐量
	if totalResponseTime > 0 {
		totalTimeSeconds := float64(totalResponseTime) / 1000.0
		throughputPerSecond := float64(len(results)) / totalTimeSeconds * float64(nc.maxConcurrency)
		fmt.Printf("📈 处理吞吐量: %.2f 请求/秒\n", throughputPerSecond)
	}
}
