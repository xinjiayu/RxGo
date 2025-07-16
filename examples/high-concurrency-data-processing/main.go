package main

import (
	"fmt"
	"strings"
)

func main() {
	fmt.Println("🚀 RxGo 高并发数据处理示例")
	fmt.Println(strings.Repeat("=", 50))

	// 运行各种高并发数据处理示例
	runDataProcessingPipeline()
	runBatchProcessing()
	runNetworkCrawler()
	runLogAnalytics()
	runStreamProcessing()
	runMapReduceExample()

	fmt.Println("\n✅ 所有示例运行完成!")
}

// 数据处理管道示例
func runDataProcessingPipeline() {
	fmt.Println("\n📊 1. 数据处理管道示例")
	fmt.Println(strings.Repeat("-", 30))

	pipeline := NewDataProcessingPipeline()
	pipeline.RunExample()
}

// 批处理示例
func runBatchProcessing() {
	fmt.Println("\n📦 2. 批处理示例")
	fmt.Println(strings.Repeat("-", 30))

	batcher := NewBatchProcessor()
	batcher.RunExample()
}

// 网络爬虫示例
func runNetworkCrawler() {
	fmt.Println("\n🕷️ 3. 网络爬虫示例")
	fmt.Println(strings.Repeat("-", 30))

	crawler := NewNetworkCrawler()
	crawler.RunExample()
}

// 日志分析示例
func runLogAnalytics() {
	fmt.Println("\n📈 4. 日志分析示例")
	fmt.Println(strings.Repeat("-", 30))

	analytics := NewLogAnalytics()
	analytics.RunExample()
}

// 流处理示例
func runStreamProcessing() {
	fmt.Println("\n🌊 5. 流处理示例")
	fmt.Println(strings.Repeat("-", 30))

	stream := NewStreamProcessor()
	stream.RunExample()
}

// MapReduce示例
func runMapReduceExample() {
	fmt.Println("\n🗺️ 6. MapReduce示例")
	fmt.Println(strings.Repeat("-", 30))

	mapReduce := NewMapReduceProcessor()
	mapReduce.RunExample()
}
