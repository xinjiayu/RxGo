package main

import (
	"runtime"
	"sync"
	"testing"

	"github.com/xinjiayu/rxgo"
)

// ============================================================================
// 数据处理管道基准测试
// ============================================================================

func BenchmarkDataPipelineParallel(b *testing.B) {
	pipeline := NewDataProcessingPipeline()
	testData := pipeline.generateTestData(1000) // 使用较小的数据集以减少测试时间

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = pipeline.processDataParallel(testData)
	}
}

func BenchmarkDataPipelineSerial(b *testing.B) {
	pipeline := NewDataProcessingPipeline()
	testData := pipeline.generateTestData(1000)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = pipeline.processDataSerial(testData)
	}
}

// ============================================================================
// 批处理基准测试
// ============================================================================

func BenchmarkBatchProcessingParallel(b *testing.B) {
	processor := NewBatchProcessor()
	items := processor.generateBatchItems(5000)
	batches := processor.createBatches(items)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = processor.processBatchesParallel(batches)
	}
}

func BenchmarkBatchProcessingSerial(b *testing.B) {
	processor := NewBatchProcessor()
	items := processor.generateBatchItems(5000)
	batches := processor.createBatches(items)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = processor.processBatchesSerial(batches)
	}
}

// ============================================================================
// 网络爬虫基准测试
// ============================================================================

func BenchmarkNetworkCrawlerParallel(b *testing.B) {
	crawler := NewNetworkCrawler()
	urls := crawler.generateURLs(100) // 减少URL数量以加快测试

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = crawler.crawlURLsParallel(urls)
	}
}

func BenchmarkNetworkCrawlerSerial(b *testing.B) {
	crawler := NewNetworkCrawler()
	urls := crawler.generateURLs(50) // 串行测试使用更少的URL

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = crawler.crawlURLsSerial(urls)
	}
}

// ============================================================================
// 流处理基准测试
// ============================================================================

func BenchmarkStreamProcessingParallel(b *testing.B) {
	processor := NewStreamProcessor()
	stream := processor.createDataStream(1000)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = processor.processStreamParallel(stream)
	}
}

// ============================================================================
// MapReduce基准测试
// ============================================================================

func BenchmarkMapReduceWordCount(b *testing.B) {
	processor := NewMapReduceProcessor()
	documents := processor.generateDocuments(500)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = processor.wordCountMapReduce(documents)
	}
}

// ============================================================================
// ParallelFlowable vs 传统goroutine基准测试
// ============================================================================

func BenchmarkParallelFlowableMap(b *testing.B) {
	data := make([]interface{}, 1000)
	for i := range data {
		data[i] = i
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var results []interface{}
		var mu sync.Mutex
		var wg sync.WaitGroup

		parallelFlowable := rxgo.ParallelFromSlice(data, runtime.NumCPU()).
			Map(func(v interface{}) (interface{}, error) {
				return v.(int) * 2, nil
			})

		subscriptions := parallelFlowable.SubscribeWithCallbacks(
			func(value interface{}) {
				mu.Lock()
				results = append(results, value)
				mu.Unlock()
			},
			func(err error) {},
			func() {
				wg.Done()
			},
		)

		wg.Add(len(subscriptions))
		for _, sub := range subscriptions {
			sub.Request(int64(len(data)))
		}
		wg.Wait()
	}
}

func BenchmarkTraditionalGoroutineMap(b *testing.B) {
	data := make([]interface{}, 1000)
	for i := range data {
		data[i] = i
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var results []interface{}
		var mu sync.Mutex
		var wg sync.WaitGroup

		numWorkers := runtime.NumCPU()
		chunkSize := len(data) / numWorkers

		for w := 0; w < numWorkers; w++ {
			wg.Add(1)
			go func(start int) {
				defer wg.Done()
				end := start + chunkSize
				if end > len(data) {
					end = len(data)
				}

				var localResults []interface{}
				for j := start; j < end; j++ {
					localResults = append(localResults, data[j].(int)*2)
				}

				mu.Lock()
				results = append(results, localResults...)
				mu.Unlock()
			}(w * chunkSize)
		}

		wg.Wait()
	}
}

// ============================================================================
// 内存分配基准测试
// ============================================================================

func BenchmarkMemoryAllocation(b *testing.B) {
	b.Run("ParallelFlowable", func(b *testing.B) {
		data := make([]interface{}, 100)
		for i := range data {
			data[i] = i
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			parallelFlowable := rxgo.ParallelRange(1, 100, 4).
				Map(func(v interface{}) (interface{}, error) {
					return v.(int) * v.(int), nil
				})

			subscriptions := parallelFlowable.SubscribeWithCallbacks(
				func(value interface{}) {},
				func(err error) {},
				func() { wg.Done() },
			)

			wg.Add(len(subscriptions))
			for _, sub := range subscriptions {
				sub.Request(100)
			}
			wg.Wait()
		}
	})

	b.Run("TraditionalGoroutines", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			ch := make(chan int, 100)

			// 生产者
			go func() {
				for j := 1; j <= 100; j++ {
					ch <- j
				}
				close(ch)
			}()

			// 消费者
			for w := 0; w < 4; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for val := range ch {
						_ = val * val
					}
				}()
			}

			wg.Wait()
		}
	})
}

// ============================================================================
// 背压处理基准测试
// ============================================================================

func BenchmarkBackpressureHandling(b *testing.B) {
	b.Run("WithBackpressure", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			flowable := rxgo.FlowableRange(1, 100). // 减少数据量
								Map(func(v interface{}) (interface{}, error) {
					return v.(int) * 2, nil // 移除睡眠以加快测试
				})

			wg.Add(1)
			subscription := flowable.SubscribeWithCallbacks(
				func(value interface{}) {},
				func(err error) {},
				func() {
					wg.Done()
				},
			)

			subscription.Request(100) // 请求所有数据
			wg.Wait()
		}
	})

	b.Run("WithoutBackpressure", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			ch := make(chan int, 100)

			// 快速生产者
			go func() {
				for j := 1; j <= 100; j++ {
					ch <- j
				}
				close(ch)
			}()

			// 消费者
			wg.Add(1)
			go func() {
				defer wg.Done()
				for val := range ch {
					_ = val * 2
				}
			}()

			wg.Wait()
		}
	})
}

// ============================================================================
// 错误处理基准测试
// ============================================================================

func BenchmarkErrorHandling(b *testing.B) {
	b.Run("RxGoErrorHandling", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			parallelFlowable := rxgo.ParallelRange(1, 100, 4).
				Map(func(v interface{}) (interface{}, error) {
					if v.(int)%10 == 0 {
						return nil, &TestError{Message: "test error"}
					}
					return v.(int) * 2, nil
				})

			subscriptions := parallelFlowable.SubscribeWithCallbacks(
				func(value interface{}) {},
				func(err error) {},
				func() { wg.Done() },
			)

			wg.Add(len(subscriptions))
			for _, sub := range subscriptions {
				sub.Request(100)
			}
			wg.Wait()
		}
	})
}

// TestError 测试错误类型
type TestError struct {
	Message string
}

func (e *TestError) Error() string {
	return e.Message
}
