package rxgo

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// QueueSubscription融合协议测试
// ============================================================================

func TestFusionQueue(t *testing.T) {
	t.Run("基本队列功能", func(t *testing.T) {
		queue := NewFusionQueue(8)

		// 测试空队列
		if !queue.IsEmpty() {
			t.Error("新创建的队列应该为空")
		}

		if queue.Size() != 0 {
			t.Errorf("空队列大小应该为0，但得到 %d", queue.Size())
		}

		// 测试添加元素
		item1 := Item{Value: 1, Error: nil}
		if !queue.Offer(item1) {
			t.Error("应该能够添加元素到队列")
		}

		if queue.IsEmpty() {
			t.Error("添加元素后队列不应该为空")
		}

		if queue.Size() != 1 {
			t.Errorf("队列大小应该为1，但得到 %d", queue.Size())
		}

		// 测试获取元素
		if item, ok := queue.Poll(); !ok || item.Value != 1 {
			t.Errorf("应该能够获取元素，期望1，得到 %v", item.Value)
		}

		if !queue.IsEmpty() {
			t.Error("取出元素后队列应该为空")
		}
	})

	t.Run("队列容量和满队列测试", func(t *testing.T) {
		queue := NewFusionQueue(4) // 实际容量会是4，可用容量是3（预留1个位置）

		// 调试信息
		t.Logf("初始队列状态 - 大小: %d, 空: %v", queue.Size(), queue.IsEmpty())

		// 填满队列（容量4，可用3）
		for i := 0; i < 3; i++ {
			success := queue.Offer(Item{Value: i, Error: nil})
			t.Logf("添加元素 %d: %v, 队列大小: %d", i, success, queue.Size())
			if !success {
				t.Errorf("应该能够添加第 %d 个元素", i)
			}
		}

		// 尝试添加超出容量的元素
		extraSuccess := queue.Offer(Item{Value: 999, Error: nil})
		t.Logf("尝试添加额外元素: %v, 队列大小: %d", extraSuccess, queue.Size())
		if extraSuccess {
			t.Error("满队列不应该能够继续添加元素")
		}

		// 验证元素顺序
		for i := 0; i < 3; i++ {
			item, ok := queue.Poll()
			t.Logf("取出元素 %d: ok=%v, value=%v", i, ok, item.Value)
			if !ok || item.Value != i {
				t.Errorf("元素顺序错误，期望 %d，得到 %v", i, item.Value)
			}
		}
	})

	t.Run("错误处理和终止", func(t *testing.T) {
		queue := NewFusionQueue(8)

		// 添加错误项目
		errorItem := Item{Value: nil, Error: fmt.Errorf("测试错误")}
		if !queue.Offer(errorItem) {
			t.Error("应该能够添加错误项目")
		}

		if !queue.IsTerminated() {
			t.Error("添加错误后队列应该标记为终止")
		}

		if queue.GetError() == nil {
			t.Error("应该能够获取错误信息")
		}

		// 尝试向终止队列添加元素
		if queue.Offer(Item{Value: 1, Error: nil}) {
			t.Error("终止队列不应该接受新元素")
		}
	})

	t.Run("完成信号处理", func(t *testing.T) {
		queue := NewFusionQueue(8)

		// 添加完成信号
		completeItem := Item{Value: nil, Error: nil}
		if !queue.Offer(completeItem) {
			t.Error("应该能够添加完成信号")
		}

		if !queue.IsTerminated() {
			t.Error("添加完成信号后队列应该标记为终止")
		}
	})
}

func TestQueueSubscription(t *testing.T) {
	t.Run("融合模式请求", func(t *testing.T) {
		queueSub := NewFusionSubscription(16, FUSION_SYNC|FUSION_ASYNC)

		// 测试SYNC融合
		syncMode := queueSub.RequestFusion(FUSION_SYNC)
		if syncMode != FUSION_SYNC {
			t.Errorf("期望SYNC融合模式，但得到 %d", syncMode)
		}

		// 测试ASYNC融合
		asyncMode := queueSub.RequestFusion(FUSION_ASYNC)
		if asyncMode != FUSION_ASYNC {
			t.Errorf("期望ASYNC融合模式，但得到 %d", asyncMode)
		}

		// 测试不支持的模式
		boundaryMode := queueSub.RequestFusion(FUSION_BOUNDARY)
		if boundaryMode != FUSION_NONE {
			t.Errorf("期望不支持BOUNDARY融合，但得到 %d", boundaryMode)
		}

		// 测试ANY模式
		anyMode := queueSub.RequestFusion(FUSION_ANY)
		if anyMode != (FUSION_SYNC | FUSION_ASYNC) {
			t.Errorf("期望支持ANY融合，但得到 %d", anyMode)
		}
	})

	t.Run("队列操作", func(t *testing.T) {
		queueSub := NewFusionSubscription(8, FUSION_SYNC)

		// 测试Offer和Poll
		item := Item{Value: "test", Error: nil}
		if !queueSub.Offer(item) {
			t.Error("应该能够offer项目")
		}

		if queueSub.IsEmpty() {
			t.Error("offer后队列不应该为空")
		}

		if queueSub.Size() != 1 {
			t.Errorf("队列大小应该为1，但得到 %d", queueSub.Size())
		}

		if polledItem, ok := queueSub.Poll(); !ok || polledItem.Value != "test" {
			t.Errorf("Poll应该返回正确的项目，但得到 %v", polledItem.Value)
		}

		if !queueSub.IsEmpty() {
			t.Error("poll后队列应该为空")
		}
	})

	t.Run("订阅取消", func(t *testing.T) {
		queueSub := NewFusionSubscription(8, FUSION_SYNC)

		if queueSub.IsUnsubscribed() {
			t.Error("新订阅不应该被取消")
		}

		queueSub.Unsubscribe()

		if !queueSub.IsUnsubscribed() {
			t.Error("取消后订阅应该被标记为已取消")
		}

		// 取消后应该无法offer
		if queueSub.Offer(Item{Value: 1, Error: nil}) {
			t.Error("取消订阅后不应该能够offer项目")
		}

		// 取消后应该无法poll
		if _, ok := queueSub.Poll(); ok {
			t.Error("取消订阅后不应该能够poll项目")
		}
	})
}

func TestSyncFusionObservable(t *testing.T) {
	t.Run("同步融合基本功能", func(t *testing.T) {
		source := FromSlice([]interface{}{1, 2, 3, 4, 5})

		// 创建同步融合Observable
		fusionObs := NewSyncFusionObservable(source, func(item Item) (Item, error) {
			if item.Value != nil {
				value := item.Value.(int) * 2
				return Item{Value: value, Error: nil}, nil
			}
			return item, nil
		})

		received := make([]int, 0, 5)
		completed := make(chan bool, 1)

		fusionObs.SubscribeWithCallbacks(
			func(item interface{}) {
				received = append(received, item.(int))
			},
			func(err error) {
				t.Errorf("不应该收到错误: %v", err)
			},
			func() {
				completed <- true
			},
		)

		// 等待完成
		select {
		case <-completed:
			// 正常完成
		case <-time.After(1 * time.Second):
			t.Error("超时：未完成")
		}

		// 验证结果
		expected := []int{2, 4, 6, 8, 10}
		if len(received) != len(expected) {
			t.Errorf("期望接收到 %d 个元素，但得到 %d 个", len(expected), len(received))
		}

		for i, val := range received {
			if val != expected[i] {
				t.Errorf("位置 %d：期望 %d，但得到 %d", i, expected[i], val)
			}
		}
	})

	t.Run("同步融合错误处理", func(t *testing.T) {
		source := FromSlice([]interface{}{1, 2, 3})

		fusionObs := NewSyncFusionObservable(source, func(item Item) (Item, error) {
			if item.Value != nil && item.Value.(int) == 2 {
				return Item{}, fmt.Errorf("测试错误")
			}
			return item, nil
		})

		errorReceived := make(chan error, 1)

		fusionObs.SubscribeWithCallbacks(
			func(item interface{}) {
				// 不应该接收到值
			},
			func(err error) {
				errorReceived <- err
			},
			func() {
				t.Error("不应该完成")
			},
		)

		// 等待错误
		select {
		case err := <-errorReceived:
			if err.Error() != "测试错误" {
				t.Errorf("期望错误消息 '测试错误'，但得到 '%v'", err.Error())
			}
		case <-time.After(1 * time.Second):
			t.Error("超时：未收到错误")
		}
	})
}

func TestAsyncFusionObservable(t *testing.T) {
	t.Run("异步融合基本功能", func(t *testing.T) {
		source := FromSlice([]interface{}{1, 2, 3})

		// 创建异步融合Observable
		fusionObs := NewAsyncFusionObservable(source, func(item Item) (Item, error) {
			if item.Value != nil {
				// 模拟异步处理延迟
				time.Sleep(10 * time.Millisecond)
				value := item.Value.(int) * 3
				return Item{Value: value, Error: nil}, nil
			}
			return item, nil
		})

		received := make([]int, 0, 3)
		var receivedMutex sync.Mutex
		completed := make(chan bool, 1)

		fusionObs.SubscribeWithCallbacks(
			func(item interface{}) {
				receivedMutex.Lock()
				received = append(received, item.(int))
				receivedMutex.Unlock()
			},
			func(err error) {
				t.Errorf("不应该收到错误: %v", err)
			},
			func() {
				completed <- true
			},
		)

		// 等待完成（给足够时间进行异步处理）
		select {
		case <-completed:
			// 正常完成
		case <-time.After(2 * time.Second):
			t.Error("超时：未完成")
		}

		// 验证结果（异步处理可能改变顺序）
		receivedMutex.Lock()
		if len(received) != 3 {
			t.Errorf("期望接收到 3 个元素，但得到 %d 个", len(received))
		}
		receivedMutex.Unlock()

		// 验证所有值都被正确转换（3, 6, 9）
		expectedSum := 3 + 6 + 9
		actualSum := 0
		for _, val := range received {
			actualSum += val
		}
		if actualSum != expectedSum {
			t.Errorf("期望总和 %d，但得到 %d", expectedSum, actualSum)
		}
	})
}

func TestBoundaryFusionObservable(t *testing.T) {
	t.Run("边界融合基本功能", func(t *testing.T) {
		source := FromSlice([]interface{}{1, 2, 3, 4, 5})

		boundaryCount := int32(0)

		// 创建边界融合Observable
		fusionObs := NewBoundaryFusionObservable(source, func() bool {
			// 每隔一个元素触发边界融合
			count := atomic.AddInt32(&boundaryCount, 1)
			return count%2 == 0
		})

		received := make([]int, 0, 5)
		completed := make(chan bool, 1)

		fusionObs.SubscribeWithCallbacks(
			func(item interface{}) {
				received = append(received, item.(int))
			},
			func(err error) {
				t.Errorf("不应该收到错误: %v", err)
			},
			func() {
				completed <- true
			},
		)

		// 等待完成
		select {
		case <-completed:
			// 正常完成
		case <-time.After(1 * time.Second):
			t.Error("超时：未完成")
		}

		// 验证接收到所有元素
		expected := []int{1, 2, 3, 4, 5}
		if len(received) != len(expected) {
			t.Errorf("期望接收到 %d 个元素，但得到 %d 个", len(expected), len(received))
		}

		for i, val := range received {
			if val != expected[i] {
				t.Errorf("位置 %d：期望 %d，但得到 %d", i, expected[i], val)
			}
		}
	})
}

func TestFusionModeConstants(t *testing.T) {
	t.Run("融合模式常量值", func(t *testing.T) {
		if FUSION_NONE != 0 {
			t.Errorf("FUSION_NONE应该为0，但得到 %d", FUSION_NONE)
		}

		if FUSION_SYNC != 1 {
			t.Errorf("FUSION_SYNC应该为1，但得到 %d", FUSION_SYNC)
		}

		if FUSION_ASYNC != 2 {
			t.Errorf("FUSION_ASYNC应该为2，但得到 %d", FUSION_ASYNC)
		}

		if FUSION_ANY != (FUSION_SYNC | FUSION_ASYNC) {
			t.Errorf("FUSION_ANY应该为%d，但得到 %d", FUSION_SYNC|FUSION_ASYNC, FUSION_ANY)
		}

		if FUSION_BOUNDARY != 4 {
			t.Errorf("FUSION_BOUNDARY应该为4，但得到 %d", FUSION_BOUNDARY)
		}
	})
}

func TestFusionUtilityFunctions(t *testing.T) {
	t.Run("IsQueueSubscription检测", func(t *testing.T) {
		// 测试QueueSubscription检测
		queueSub := NewFusionSubscription(8, FUSION_SYNC)
		if detectedSub, ok := IsQueueSubscription(queueSub); !ok {
			t.Error("应该检测到QueueSubscription")
		} else if detectedSub.RequestFusion(FUSION_SYNC) != FUSION_SYNC {
			t.Error("检测到的QueueSubscription应该支持SYNC融合")
		}

		// 测试普通Subscription
		normalSub := &noOpSubscription{}
		if _, ok := IsQueueSubscription(normalSub); ok {
			t.Error("不应该将普通Subscription检测为QueueSubscription")
		}
	})

	t.Run("工厂函数", func(t *testing.T) {
		source := Just(42)

		// 测试FusionSync
		syncFusion := FusionSync(source, nil)
		if syncFusion == nil {
			t.Error("FusionSync应该返回非nil的Observable")
		}

		// 测试FusionAsync
		asyncFusion := FusionAsync(source, nil)
		if asyncFusion == nil {
			t.Error("FusionAsync应该返回非nil的Observable")
		}

		// 测试FusionBoundary
		boundaryFusion := FusionBoundary(source, func() bool { return true })
		if boundaryFusion == nil {
			t.Error("FusionBoundary应该返回非nil的Observable")
		}
	})
}

// ============================================================================
// 性能基准测试
// ============================================================================

func BenchmarkFusionQueue(b *testing.B) {
	queue := NewFusionQueue(256)

	b.Run("Offer", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if !queue.Offer(Item{Value: i, Error: nil}) {
				// 队列满时清空
				queue.Clear()
				queue.Offer(Item{Value: i, Error: nil})
			}
		}
	})

	b.Run("Poll", func(b *testing.B) {
		// 预填充队列
		for i := 0; i < 100; i++ {
			queue.Offer(Item{Value: i, Error: nil})
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, ok := queue.Poll(); !ok {
				// 队列空时重新填充
				for j := 0; j < 100; j++ {
					queue.Offer(Item{Value: j, Error: nil})
				}
			}
		}
	})
}

func BenchmarkSyncFusionVsStandard(b *testing.B) {
	source := Range(1, 1000)
	transformer := func(item Item) (Item, error) {
		if item.Value != nil {
			return Item{Value: item.Value.(int) * 2, Error: nil}, nil
		}
		return item, nil
	}

	b.Run("SyncFusion", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fusionObs := NewSyncFusionObservable(source, transformer)
			fusionObs.Subscribe(func(item Item) {
				_ = item.Value
			})
		}
	})

	b.Run("StandardMap", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mappedObs := source.Map(func(value interface{}) (interface{}, error) {
				return value.(int) * 2, nil
			})
			mappedObs.Subscribe(func(item Item) {
				_ = item.Value
			})
		}
	})
}
