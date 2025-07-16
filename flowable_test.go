// Flowable tests for RxGo
// Flowable测试，验证核心功能和背压处理
package rxgo

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// 基础功能测试
// ============================================================================

func TestFlowableJust(t *testing.T) {
	// 测试FlowableJust基础功能
	values := []interface{}{1, 2, 3, 4, 5}
	flowable := FlowableJust(values...)

	received := make([]interface{}, 0)
	completed := false
	var mu sync.Mutex

	subscription := flowable.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			received = append(received, value)
			mu.Unlock()
		},
		func(err error) {
			t.Errorf("不应该有错误: %v", err)
		},
		func() {
			mu.Lock()
			completed = true
			mu.Unlock()
		},
	)

	// 请求所有数据
	subscription.Request(int64(len(values)))

	// 等待一段时间确保数据处理完成
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if len(received) != len(values) {
		t.Errorf("期望接收到 %d 个值，实际接收到 %d 个", len(values), len(received))
	}

	for i, expected := range values {
		if i < len(received) && received[i] != expected {
			t.Errorf("索引 %d: 期望 %v，实际 %v", i, expected, received[i])
		}
	}

	if !completed {
		t.Error("期望流已完成")
	}
	mu.Unlock()
}

func TestFlowableRange(t *testing.T) {
	// 测试FlowableRange
	start := 10
	count := 5
	flowable := FlowableRange(start, count)

	received := make([]interface{}, 0)
	completed := false
	var mu sync.Mutex

	subscription := flowable.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			received = append(received, value)
			mu.Unlock()
		},
		func(err error) {
			t.Errorf("不应该有错误: %v", err)
		},
		func() {
			mu.Lock()
			completed = true
			mu.Unlock()
		},
	)

	// 请求所有数据
	subscription.Request(int64(count))

	// 等待数据处理完成
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if len(received) != count {
		t.Errorf("期望接收到 %d 个值，实际接收到 %d 个", count, len(received))
	}

	for i := 0; i < count; i++ {
		expected := start + i
		if i < len(received) && received[i] != expected {
			t.Errorf("索引 %d: 期望 %v，实际 %v", i, expected, received[i])
		}
	}

	if !completed {
		t.Error("期望流已完成")
	}
	mu.Unlock()
}

func TestFlowableEmpty(t *testing.T) {
	// 测试FlowableEmpty
	flowable := FlowableEmpty()

	received := make([]interface{}, 0)
	completed := false
	var mu sync.Mutex

	subscription := flowable.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			received = append(received, value)
			mu.Unlock()
		},
		func(err error) {
			t.Errorf("不应该有错误: %v", err)
		},
		func() {
			mu.Lock()
			completed = true
			mu.Unlock()
		},
	)

	// 请求数据
	subscription.Request(1)

	// 等待处理完成
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if len(received) != 0 {
		t.Errorf("期望接收到 0 个值，实际接收到 %d 个", len(received))
	}

	if !completed {
		t.Error("期望流已完成")
	}
	mu.Unlock()
}

func TestFlowableError(t *testing.T) {
	// 测试FlowableError
	expectedError := errors.New("测试错误")
	flowable := FlowableError(expectedError)

	received := make([]interface{}, 0)
	var receivedError error
	completed := false
	var mu sync.Mutex

	subscription := flowable.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			received = append(received, value)
			mu.Unlock()
		},
		func(err error) {
			mu.Lock()
			receivedError = err
			mu.Unlock()
		},
		func() {
			mu.Lock()
			completed = true
			mu.Unlock()
		},
	)

	// 请求数据
	subscription.Request(1)

	// 等待处理完成
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if len(received) != 0 {
		t.Errorf("期望接收到 0 个值，实际接收到 %d 个", len(received))
	}

	if receivedError == nil {
		t.Error("期望接收到错误")
	} else if receivedError.Error() != expectedError.Error() {
		t.Errorf("期望错误 '%v'，实际错误 '%v'", expectedError, receivedError)
	}

	if completed {
		t.Error("不期望流已完成")
	}
	mu.Unlock()
}

// ============================================================================
// 背压测试
// ============================================================================

func TestBackpressureRequest(t *testing.T) {
	// 测试背压请求机制
	values := []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	flowable := FlowableJust(values...)

	received := make([]interface{}, 0)
	var mu sync.Mutex

	subscription := flowable.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			received = append(received, value)
			mu.Unlock()
		},
		func(err error) {
			t.Errorf("不应该有错误: %v", err)
		},
		func() {
			// 完成
		},
	)

	// 先请求前3个
	subscription.Request(3)
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(received) != 3 {
		t.Errorf("第一批: 期望接收到 3 个值，实际接收到 %d 个", len(received))
	}
	mu.Unlock()

	// 再请求5个
	subscription.Request(5)
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(received) != 8 {
		t.Errorf("第二批: 期望接收到 8 个值，实际接收到 %d 个", len(received))
	}
	mu.Unlock()

	// 请求剩余的
	subscription.Request(10)
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(received) != len(values) {
		t.Errorf("最终: 期望接收到 %d 个值，实际接收到 %d 个", len(values), len(received))
	}
	mu.Unlock()
}

// ============================================================================
// 操作符测试
// ============================================================================

func TestFlowableMap(t *testing.T) {
	// 测试Map操作符
	flowable := FlowableRange(1, 5).Map(func(value interface{}) (interface{}, error) {
		return value.(int) * 2, nil
	})

	received := make([]interface{}, 0)
	completed := false
	var mu sync.Mutex

	subscription := flowable.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			received = append(received, value)
			mu.Unlock()
		},
		func(err error) {
			t.Errorf("不应该有错误: %v", err)
		},
		func() {
			mu.Lock()
			completed = true
			mu.Unlock()
		},
	)

	// 请求所有数据
	subscription.Request(5)

	// 等待处理完成
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	expectedValues := []int{2, 4, 6, 8, 10}
	if len(received) != len(expectedValues) {
		t.Errorf("期望接收到 %d 个值，实际接收到 %d 个", len(expectedValues), len(received))
	}

	for i, expected := range expectedValues {
		if i < len(received) && received[i] != expected {
			t.Errorf("索引 %d: 期望 %v，实际 %v", i, expected, received[i])
		}
	}

	if !completed {
		t.Error("期望流已完成")
	}
	mu.Unlock()
}

func TestFlowableFilter(t *testing.T) {
	// 测试Filter操作符
	flowable := FlowableRange(1, 10).Filter(func(value interface{}) bool {
		return value.(int)%2 == 0 // 只保留偶数
	})

	received := make([]interface{}, 0)
	completed := false
	var mu sync.Mutex

	subscription := flowable.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			received = append(received, value)
			mu.Unlock()
		},
		func(err error) {
			t.Errorf("不应该有错误: %v", err)
		},
		func() {
			mu.Lock()
			completed = true
			mu.Unlock()
		},
	)

	// 请求足够的数据
	subscription.Request(10)

	// 等待处理完成
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	expectedValues := []int{2, 4, 6, 8, 10}
	if len(received) != len(expectedValues) {
		t.Errorf("期望接收到 %d 个值，实际接收到 %d 个", len(expectedValues), len(received))
	}

	for i, expected := range expectedValues {
		if i < len(received) && received[i] != expected {
			t.Errorf("索引 %d: 期望 %v，实际 %v", i, expected, received[i])
		}
	}

	if !completed {
		t.Error("期望流已完成")
	}
	mu.Unlock()
}

func TestFlowableTake(t *testing.T) {
	// 测试Take操作符
	flowable := FlowableRange(1, 10).Take(3)

	received := make([]interface{}, 0)
	completed := false
	var mu sync.Mutex

	subscription := flowable.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			received = append(received, value)
			mu.Unlock()
		},
		func(err error) {
			t.Errorf("不应该有错误: %v", err)
		},
		func() {
			mu.Lock()
			completed = true
			mu.Unlock()
		},
	)

	// 请求足够的数据
	subscription.Request(10)

	// 等待处理完成
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	expectedValues := []int{1, 2, 3}
	if len(received) != len(expectedValues) {
		t.Errorf("期望接收到 %d 个值，实际接收到 %d 个", len(expectedValues), len(received))
	}

	for i, expected := range expectedValues {
		if i < len(received) && received[i] != expected {
			t.Errorf("索引 %d: 期望 %v，实际 %v", i, expected, received[i])
		}
	}

	if !completed {
		t.Error("期望流已完成")
	}
	mu.Unlock()
}

// ============================================================================
// 背压操作符测试
// ============================================================================

func TestOnBackpressureBuffer(t *testing.T) {
	// 测试OnBackpressureBuffer操作符
	values := []interface{}{1, 2, 3, 4, 5}
	flowable := FlowableJust(values...).OnBackpressureBuffer()

	received := make([]interface{}, 0)
	completed := false
	var mu sync.Mutex

	subscription := flowable.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			received = append(received, value)
			mu.Unlock()
		},
		func(err error) {
			t.Errorf("不应该有错误: %v", err)
		},
		func() {
			mu.Lock()
			completed = true
			mu.Unlock()
		},
	)

	// 先不请求数据，让数据被缓冲
	time.Sleep(50 * time.Millisecond)

	// 然后请求所有数据
	subscription.Request(int64(len(values)))

	// 等待处理完成
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if len(received) != len(values) {
		t.Errorf("期望接收到 %d 个值，实际接收到 %d 个", len(values), len(received))
	}

	if !completed {
		t.Error("期望流已完成")
	}
	mu.Unlock()
}

func TestOnBackpressureDrop(t *testing.T) {
	// 测试OnBackpressureDrop操作符
	values := []interface{}{1, 2, 3, 4, 5}
	flowable := FlowableJust(values...).OnBackpressureDrop()

	received := make([]interface{}, 0)
	var mu sync.Mutex

	subscription := flowable.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			received = append(received, value)
			mu.Unlock()
		},
		func(err error) {
			t.Errorf("不应该有错误: %v", err)
		},
		func() {
			// 完成
		},
	)

	// 只请求2个数据项
	subscription.Request(2)

	// 等待处理完成
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	// 由于丢弃策略，应该只接收到请求的数量
	if len(received) > 2 {
		t.Errorf("期望接收到最多 2 个值，实际接收到 %d 个", len(received))
	}
	mu.Unlock()
}

// ============================================================================
// 转换测试
// ============================================================================

func TestFlowableToObservable(t *testing.T) {
	// 测试Flowable转Observable
	flowable := FlowableRange(1, 5)
	observable := flowable.ToObservable()

	received := make([]interface{}, 0)
	completed := false
	var mu sync.Mutex

	subscription := observable.Subscribe(func(item Item) {
		mu.Lock()
		defer mu.Unlock()

		if item.IsError() {
			t.Errorf("不应该有错误: %v", item.Error)
		} else if item.Value == nil {
			completed = true
		} else {
			received = append(received, item.Value)
		}
	})

	// 等待处理完成
	time.Sleep(200 * time.Millisecond)

	subscription.Unsubscribe()

	mu.Lock()
	expectedValues := []int{1, 2, 3, 4, 5}
	if len(received) != len(expectedValues) {
		t.Errorf("期望接收到 %d 个值，实际接收到 %d 个", len(expectedValues), len(received))
	}

	if !completed {
		t.Error("期望流已完成")
	}
	mu.Unlock()
}

func TestFlowableBlockingFirst(t *testing.T) {
	// 测试BlockingFirst
	flowable := FlowableRange(10, 5)

	result, err := flowable.BlockingFirst()
	if err != nil {
		t.Errorf("不应该有错误: %v", err)
	}

	expected := 10
	if result != expected {
		t.Errorf("期望第一个值为 %v，实际为 %v", expected, result)
	}
}
