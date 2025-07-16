// ParallelFlowable tests for RxGo
// ParallelFlowable测试，验证并行处理功能和性能
package rxgo

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// 基础功能测试
// ============================================================================

func TestParallelFlowableBasics(t *testing.T) {
	t.Run("ParallelJust", func(t *testing.T) {
		values := []interface{}{1, 2, 3, 4, 5}
		parallelFlowable := ParallelJust(values...)

		if parallelFlowable.Parallelism() <= 0 {
			t.Errorf("并行度应该大于0，实际为: %d", parallelFlowable.Parallelism())
		}

		var received []interface{}
		var mu sync.Mutex
		completed := int32(0)

		subscriptions := parallelFlowable.SubscribeWithCallbacks(
			func(value interface{}) {
				mu.Lock()
				received = append(received, value)
				mu.Unlock()
			},
			func(err error) {
				t.Errorf("不应该收到错误: %v", err)
			},
			func() {
				atomic.AddInt32(&completed, 1)
			},
		)

		// 请求所有数据
		for _, subscription := range subscriptions {
			subscription.Request(100)
		}

		// 等待完成
		time.Sleep(100 * time.Millisecond)

		mu.Lock()
		receivedCount := len(received)
		mu.Unlock()

		if receivedCount != len(values) {
			t.Errorf("期望接收 %d 个值，实际接收 %d 个", len(values), receivedCount)
		}
	})

	t.Run("ParallelRange", func(t *testing.T) {
		start := 0
		count := 100
		parallelism := 4

		parallelFlowable := ParallelRange(start, count, parallelism)

		if parallelFlowable.Parallelism() != parallelism {
			t.Errorf("期望并行度 %d，实际为 %d", parallelism, parallelFlowable.Parallelism())
		}

		var received []interface{}
		var mu sync.Mutex
		completed := int32(0)

		subscriptions := parallelFlowable.SubscribeWithCallbacks(
			func(value interface{}) {
				mu.Lock()
				received = append(received, value)
				mu.Unlock()
			},
			func(err error) {
				t.Errorf("不应该收到错误: %v", err)
			},
			func() {
				atomic.AddInt32(&completed, 1)
			},
		)

		// 请求所有数据
		for _, subscription := range subscriptions {
			subscription.Request(100)
		}

		// 等待完成
		time.Sleep(200 * time.Millisecond)

		mu.Lock()
		receivedCount := len(received)
		mu.Unlock()

		if receivedCount != count {
			t.Errorf("期望接收 %d 个值，实际接收 %d 个", count, receivedCount)
		}
	})

	t.Run("ParallelFromFlowable", func(t *testing.T) {
		flowable := FlowableRange(1, 10)
		parallelism := 3

		parallelFlowable := ParallelFromFlowable(flowable, parallelism)

		if parallelFlowable.Parallelism() != parallelism {
			t.Errorf("期望并行度 %d，实际为 %d", parallelism, parallelFlowable.Parallelism())
		}

		var received []interface{}
		var mu sync.Mutex

		subscriptions := parallelFlowable.SubscribeWithCallbacks(
			func(value interface{}) {
				mu.Lock()
				received = append(received, value)
				mu.Unlock()
			},
			func(err error) {
				t.Errorf("不应该收到错误: %v", err)
			},
			func() {
				// 完成
			},
		)

		// 请求所有数据
		for _, subscription := range subscriptions {
			subscription.Request(100)
		}

		// 等待完成
		time.Sleep(100 * time.Millisecond)

		mu.Lock()
		receivedCount := len(received)
		mu.Unlock()

		if receivedCount != 10 {
			t.Errorf("期望接收 10 个值，实际接收 %d 个", receivedCount)
		}
	})
}

// ============================================================================
// 操作符测试
// ============================================================================

func TestParallelFlowableOperators(t *testing.T) {
	t.Run("Map", func(t *testing.T) {
		parallelFlowable := ParallelRange(1, 10, 2).Map(func(v interface{}) (interface{}, error) {
			return v.(int) * 2, nil
		})

		var received []interface{}
		var mu sync.Mutex

		subscriptions := parallelFlowable.SubscribeWithCallbacks(
			func(value interface{}) {
				mu.Lock()
				received = append(received, value)
				mu.Unlock()
			},
			func(err error) {
				t.Errorf("不应该收到错误: %v", err)
			},
			func() {
				// 完成
			},
		)

		// 请求所有数据
		for _, subscription := range subscriptions {
			subscription.Request(100)
		}

		// 等待完成
		time.Sleep(100 * time.Millisecond)

		mu.Lock()
		receivedCount := len(received)
		// 验证所有值都被翻倍
		for _, v := range received {
			if v.(int)%2 != 0 {
				t.Errorf("值应该是偶数（翻倍后），实际为: %v", v)
			}
		}
		mu.Unlock()

		if receivedCount != 10 {
			t.Errorf("期望接收 10 个值，实际接收 %d 个", receivedCount)
		}
	})

	t.Run("Filter", func(t *testing.T) {
		parallelFlowable := ParallelRange(1, 10, 2).Filter(func(v interface{}) bool {
			return v.(int)%2 == 0 // 只保留偶数
		})

		var received []interface{}
		var mu sync.Mutex

		subscriptions := parallelFlowable.SubscribeWithCallbacks(
			func(value interface{}) {
				mu.Lock()
				received = append(received, value)
				mu.Unlock()
			},
			func(err error) {
				t.Errorf("不应该收到错误: %v", err)
			},
			func() {
				// 完成
			},
		)

		// 请求所有数据
		for _, subscription := range subscriptions {
			subscription.Request(100)
		}

		// 等待完成
		time.Sleep(100 * time.Millisecond)

		mu.Lock()
		// 验证所有值都是偶数
		for _, v := range received {
			if v.(int)%2 != 0 {
				t.Errorf("值应该是偶数，实际为: %v", v)
			}
		}
		expectedCount := 5 // 1-10中有5个偶数
		receivedCount := len(received)
		mu.Unlock()

		if receivedCount != expectedCount {
			t.Errorf("期望接收 %d 个值，实际接收 %d 个", expectedCount, receivedCount)
		}
	})

	t.Run("Reduce", func(t *testing.T) {
		parallelFlowable := ParallelRange(1, 10, 2)

		single := parallelFlowable.Reduce(func(acc, curr interface{}) interface{} {
			return acc.(int) + curr.(int)
		})

		result, err := single.BlockingGet()
		if err != nil {
			t.Errorf("不应该收到错误: %v", err)
		}

		expectedSum := 55 // 1+2+3+...+10 = 55
		if result.(int) != expectedSum {
			t.Errorf("期望和为 %d，实际为 %d", expectedSum, result.(int))
		}
	})

	t.Run("ReduceWith", func(t *testing.T) {
		parallelFlowable := ParallelRange(1, 5, 2)

		single := parallelFlowable.ReduceWith(0, func(acc, curr interface{}) interface{} {
			return acc.(int) + curr.(int)
		})

		result, err := single.BlockingGet()
		if err != nil {
			t.Errorf("不应该收到错误: %v", err)
		}

		expectedSum := 15 // 1+2+3+4+5 = 15
		if result.(int) != expectedSum {
			t.Errorf("期望和为 %d，实际为 %d", expectedSum, result.(int))
		}
	})
}

// ============================================================================
// 转换测试
// ============================================================================

func TestParallelFlowableConversions(t *testing.T) {
	t.Run("Sequential", func(t *testing.T) {
		// 使用ParallelRange创建并行Flowable
		parallel := ParallelRange(1, 10, 4)

		// 转换回顺序Flowable
		sequential := parallel.Sequential()

		// 收集结果
		results := make([]interface{}, 0)
		var mu sync.Mutex
		var wg sync.WaitGroup
		wg.Add(1)

		sequential.SubscribeWithCallbacks(
			func(value interface{}) {
				mu.Lock()
				results = append(results, value)
				mu.Unlock()
			},
			func(err error) {
				t.Errorf("Sequential转换出现错误: %v", err)
				wg.Done()
			},
			func() {
				wg.Done()
			},
		)

		wg.Wait()

		// 验证结果
		if len(results) != 10 {
			t.Errorf("期望10个结果，实际得到%d个", len(results))
		}

		// 验证包含所有数字（不验证顺序，因为并行处理可能改变顺序）
		resultMap := make(map[int]bool)
		for _, v := range results {
			if num, ok := v.(int); ok {
				resultMap[num] = true
			}
		}

		for i := 1; i <= 10; i++ {
			if !resultMap[i] {
				t.Errorf("结果中缺少数字%d", i)
			}
		}
	})

	t.Run("ToFlowable", func(t *testing.T) {
		// 创建并行Flowable并应用转换
		parallel := ParallelRange(1, 5, 2).Map(func(i interface{}) (interface{}, error) {
			return i.(int) * 2, nil
		})

		// 转换为Flowable
		flowable := parallel.ToFlowable()

		// 收集结果
		results := make([]interface{}, 0)
		var mu sync.Mutex
		var wg sync.WaitGroup
		wg.Add(1)

		flowable.SubscribeWithCallbacks(
			func(value interface{}) {
				mu.Lock()
				results = append(results, value)
				mu.Unlock()
			},
			func(err error) {
				t.Errorf("ToFlowable转换出现错误: %v", err)
				wg.Done()
			},
			func() {
				wg.Done()
			},
		)

		wg.Wait()

		// 验证结果
		if len(results) != 5 {
			t.Errorf("期望5个结果，实际得到%d个", len(results))
		}

		// 验证转换后的值（应该都是原值的2倍）
		resultMap := make(map[int]bool)
		for _, v := range results {
			if num, ok := v.(int); ok {
				resultMap[num] = true
			}
		}

		expectedValues := []int{2, 4, 6, 8, 10} // 1*2, 2*2, 3*2, 4*2, 5*2
		for _, expected := range expectedValues {
			if !resultMap[expected] {
				t.Errorf("结果中缺少期望值%d", expected)
			}
		}
	})
}

// ============================================================================
// 并行性能测试
// ============================================================================

func TestParallelFlowablePerformance(t *testing.T) {
	t.Run("并行处理性能对比", func(t *testing.T) {
		const dataSize = 1000
		parallelism := runtime.NumCPU()

		// 并行处理
		start := time.Now()
		parallelFlowable := ParallelRange(1, dataSize, parallelism).Map(func(v interface{}) (interface{}, error) {
			// 模拟一些计算工作
			time.Sleep(1 * time.Millisecond)
			return v.(int) * v.(int), nil
		})

		var parallelResults []interface{}
		var mu sync.Mutex
		done := make(chan struct{})
		completedCount := int32(0)

		subscriptions := parallelFlowable.SubscribeWithCallbacks(
			func(value interface{}) {
				mu.Lock()
				parallelResults = append(parallelResults, value)
				mu.Unlock()
			},
			func(err error) {
				t.Errorf("不应该收到错误: %v", err)
			},
			func() {
				if atomic.AddInt32(&completedCount, 1) == int32(parallelism) {
					close(done)
				}
			},
		)

		// 请求所有数据
		for _, subscription := range subscriptions {
			subscription.Request(int64(dataSize))
		}

		<-done
		parallelDuration := time.Since(start)

		mu.Lock()
		parallelCount := len(parallelResults)
		mu.Unlock()

		if parallelCount != dataSize {
			t.Errorf("并行处理期望接收 %d 个值，实际接收 %d 个", dataSize, parallelCount)
		}

		// 顺序处理（作为对比）
		start = time.Now()
		flowable := FlowableRange(1, dataSize).Map(func(v interface{}) (interface{}, error) {
			time.Sleep(1 * time.Millisecond)
			return v.(int) * v.(int), nil
		})

		var sequentialResults []interface{}
		done2 := make(chan struct{})

		subscription := flowable.SubscribeWithCallbacks(
			func(value interface{}) {
				sequentialResults = append(sequentialResults, value)
			},
			func(err error) {
				t.Errorf("不应该收到错误: %v", err)
			},
			func() {
				close(done2)
			},
		)

		subscription.Request(int64(dataSize))
		<-done2
		sequentialDuration := time.Since(start)

		if len(sequentialResults) != dataSize {
			t.Errorf("顺序处理期望接收 %d 个值，实际接收 %d 个", dataSize, len(sequentialResults))
		}

		t.Logf("并行处理耗时: %v", parallelDuration)
		t.Logf("顺序处理耗时: %v", sequentialDuration)
		t.Logf("性能提升: %.2fx", float64(sequentialDuration)/float64(parallelDuration))

		// 并行处理应该更快（在有足够数据和CPU核心的情况下）
		if parallelDuration >= sequentialDuration {
			t.Logf("警告: 并行处理没有显示出性能优势，这可能是由于数据量太小或系统限制")
		}
	})
}

// ============================================================================
// 错误处理测试
// ============================================================================

func TestParallelFlowableErrorHandling(t *testing.T) {
	t.Run("订阅者数量错误", func(t *testing.T) {
		parallelFlowable := ParallelRange(1, 10, 2)

		// 创建错误数量的订阅者
		wrongSubscribers := make([]Subscriber, 3) // 应该是2个
		for i := range wrongSubscribers {
			wrongSubscribers[i] = &callbackSubscriber{
				onNext: func(value interface{}) {
					t.Errorf("不应该收到值: %v", value)
				},
				onError: func(err error) {
					if err != ErrInvalidSubscriberCount {
						t.Errorf("期望错误 %v，实际错误 %v", ErrInvalidSubscriberCount, err)
					}
				},
				onComplete: func() {
					t.Errorf("不应该收到完成信号")
				},
			}
		}

		parallelFlowable.Subscribe(wrongSubscribers)
	})

	t.Run("Map操作符错误", func(t *testing.T) {
		parallelFlowable := ParallelJust(1, 2, 3).Map(func(v interface{}) (interface{}, error) {
			if v.(int) == 2 {
				return nil, fmt.Errorf("测试错误")
			}
			return v.(int) * 2, nil
		})

		errorReceived := int32(0)

		subscriptions := parallelFlowable.SubscribeWithCallbacks(
			func(value interface{}) {
				// 正常值
			},
			func(err error) {
				atomic.AddInt32(&errorReceived, 1)
			},
			func() {
				// 完成
			},
		)

		// 请求所有数据
		for _, subscription := range subscriptions {
			subscription.Request(100)
		}

		// 等待完成
		time.Sleep(100 * time.Millisecond)

		if atomic.LoadInt32(&errorReceived) == 0 {
			t.Errorf("期望收到至少一个错误")
		}
	})
}

// ============================================================================
// 基准测试
// ============================================================================

func BenchmarkParallelFlowableMap(b *testing.B) {
	parallelism := runtime.NumCPU()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parallelFlowable := ParallelRange(1, 1000, parallelism).Map(func(v interface{}) (interface{}, error) {
			return v.(int) * v.(int), nil
		})

		done := make(chan struct{})
		completedCount := int32(0)

		subscriptions := parallelFlowable.SubscribeWithCallbacks(
			func(value interface{}) {
				// 处理值
			},
			func(err error) {
				b.Errorf("不应该收到错误: %v", err)
			},
			func() {
				if atomic.AddInt32(&completedCount, 1) == int32(parallelism) {
					close(done)
				}
			},
		)

		// 请求所有数据
		for _, subscription := range subscriptions {
			subscription.Request(1000)
		}

		<-done
	}
}

func BenchmarkParallelFlowableReduce(b *testing.B) {
	parallelism := runtime.NumCPU()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parallelFlowable := ParallelRange(1, 1000, parallelism)

		single := parallelFlowable.Reduce(func(acc, curr interface{}) interface{} {
			return acc.(int) + curr.(int)
		})

		_, err := single.BlockingGet()
		if err != nil {
			b.Errorf("不应该收到错误: %v", err)
		}
	}
}
