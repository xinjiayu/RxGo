// Subject comprehensive tests for RxGo
// 全面的Subject测试，验证所有Subject类型的正确行为
package rxgo

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// PublishSubject 详细测试
// ============================================================================

func TestPublishSubjectComprehensive(t *testing.T) {
	t.Run("基本发射和订阅", func(t *testing.T) {
		subject := NewPublishSubject()
		defer subject.Dispose()

		values := []interface{}{}
		var mu sync.Mutex
		done := make(chan bool, 1)

		sub := subject.SubscribeWithCallbacks(
			func(value interface{}) {
				mu.Lock()
				values = append(values, value)
				mu.Unlock()
			},
			func(err error) {
				t.Errorf("不应该有错误: %v", err)
			},
			func() {
				done <- true
			},
		)
		defer sub.Unsubscribe()

		// 发射多个值
		subject.OnNext(1)
		subject.OnNext(2)
		subject.OnNext(3)
		subject.OnComplete()

		<-done

		mu.Lock()
		expected := []interface{}{1, 2, 3}
		if !reflect.DeepEqual(values, expected) {
			t.Errorf("期望 %v, 得到 %v", expected, values)
		}
		mu.Unlock()
	})

	t.Run("多个订阅者同时接收", func(t *testing.T) {
		subject := NewPublishSubject()
		defer subject.Dispose()

		values1 := []interface{}{}
		values2 := []interface{}{}
		var mu1, mu2 sync.Mutex
		done1 := make(chan bool, 1)
		done2 := make(chan bool, 1)

		// 订阅者1
		sub1 := subject.SubscribeWithCallbacks(
			func(value interface{}) {
				mu1.Lock()
				values1 = append(values1, value)
				mu1.Unlock()
			},
			nil,
			func() { done1 <- true },
		)

		// 订阅者2
		sub2 := subject.SubscribeWithCallbacks(
			func(value interface{}) {
				mu2.Lock()
				values2 = append(values2, value)
				mu2.Unlock()
			},
			nil,
			func() { done2 <- true },
		)

		// 发射值
		subject.OnNext("A")
		subject.OnNext("B")
		subject.OnNext("C")
		subject.OnComplete()

		<-done1
		<-done2

		expected := []interface{}{"A", "B", "C"}

		mu1.Lock()
		if !reflect.DeepEqual(values1, expected) {
			t.Errorf("订阅者1期望 %v, 得到 %v", expected, values1)
		}
		mu1.Unlock()

		mu2.Lock()
		if !reflect.DeepEqual(values2, expected) {
			t.Errorf("订阅者2期望 %v, 得到 %v", expected, values2)
		}
		mu2.Unlock()

		sub1.Unsubscribe()
		sub2.Unsubscribe()
	})

	t.Run("订阅后才发射的值不会被错过", func(t *testing.T) {
		subject := NewPublishSubject()
		defer subject.Dispose()

		// 先发射一些值（应该被忽略）
		subject.OnNext("忽略1")
		subject.OnNext("忽略2")

		values := []interface{}{}
		var mu sync.Mutex
		done := make(chan bool, 1)

		// 现在才订阅
		sub := subject.SubscribeWithCallbacks(
			func(value interface{}) {
				mu.Lock()
				values = append(values, value)
				mu.Unlock()
			},
			nil,
			func() { done <- true },
		)
		defer sub.Unsubscribe()

		// 发射新值
		subject.OnNext("接收1")
		subject.OnNext("接收2")
		subject.OnComplete()

		<-done

		mu.Lock()
		expected := []interface{}{"接收1", "接收2"}
		if !reflect.DeepEqual(values, expected) {
			t.Errorf("期望 %v, 得到 %v", expected, values)
		}
		mu.Unlock()
	})
}

// ============================================================================
// BehaviorSubject 详细测试
// ============================================================================

func TestBehaviorSubjectComprehensive(t *testing.T) {
	t.Run("初始值立即发送", func(t *testing.T) {
		subject := NewBehaviorSubject("初始值")
		defer subject.Dispose()

		values := []interface{}{}
		var mu sync.Mutex
		done := make(chan bool, 1)

		sub := subject.SubscribeWithCallbacks(
			func(value interface{}) {
				mu.Lock()
				values = append(values, value)
				mu.Unlock()
			},
			nil,
			func() { done <- true },
		)
		defer sub.Unsubscribe()

		// 发射新值
		subject.OnNext("新值1")
		subject.OnNext("新值2")
		subject.OnComplete()

		<-done

		mu.Lock()
		expected := []interface{}{"初始值", "新值1", "新值2"}
		if !reflect.DeepEqual(values, expected) {
			t.Errorf("期望 %v, 得到 %v", expected, values)
		}
		mu.Unlock()
	})

	t.Run("新订阅者收到当前值", func(t *testing.T) {
		subject := NewBehaviorSubject("初始值")
		defer subject.Dispose()

		// 更新当前值
		subject.OnNext("更新值1")
		subject.OnNext("更新值2")

		values := []interface{}{}
		var mu sync.Mutex

		// 新订阅者应该立即收到最新值
		sub := subject.SubscribeWithCallbacks(
			func(value interface{}) {
				mu.Lock()
				values = append(values, value)
				mu.Unlock()
			},
			nil,
			nil,
		)
		defer sub.Unsubscribe()

		// 给一点时间处理
		time.Sleep(10 * time.Millisecond)

		mu.Lock()
		if len(values) != 1 || values[0] != "更新值2" {
			t.Errorf("期望 [更新值2], 得到 %v", values)
		}
		mu.Unlock()
	})

	t.Run("GetValue获取当前值", func(t *testing.T) {
		subject := NewBehaviorSubject("测试值")
		defer subject.Dispose()

		// 检查初始值
		value, hasValue := subject.GetValue()
		if !hasValue || value != "测试值" {
			t.Errorf("期望 (测试值, true), 得到 (%v, %v)", value, hasValue)
		}

		// 更新值
		subject.OnNext("新测试值")

		// 检查更新后的值
		value, hasValue = subject.GetValue()
		if !hasValue || value != "新测试值" {
			t.Errorf("期望 (新测试值, true), 得到 (%v, %v)", value, hasValue)
		}
	})
}

// ============================================================================
// ReplaySubject 详细测试
// ============================================================================

func TestReplaySubjectComprehensive(t *testing.T) {
	t.Run("缓存指定数量的值", func(t *testing.T) {
		subject := NewReplaySubject(3) // 缓存3个值
		defer subject.Dispose()

		// 发射多个值
		subject.OnNext(1)
		subject.OnNext(2)
		subject.OnNext(3)
		subject.OnNext(4)
		subject.OnNext(5)

		values := []interface{}{}
		var mu sync.Mutex
		done := make(chan bool, 1)

		// 新订阅者应该收到最后3个值
		sub := subject.SubscribeWithCallbacks(
			func(value interface{}) {
				mu.Lock()
				values = append(values, value)
				mu.Unlock()
			},
			nil,
			func() { done <- true },
		)
		defer sub.Unsubscribe()

		// 继续发射新值
		subject.OnNext(6)
		subject.OnComplete()

		<-done

		mu.Lock()
		expected := []interface{}{3, 4, 5, 6} // 缓存的最后3个值(3,4,5) + 新值(6)
		if !reflect.DeepEqual(values, expected) {
			t.Errorf("期望 %v, 得到 %v", expected, values)
		}
		mu.Unlock()
	})

	t.Run("GetBufferedValues获取缓存值", func(t *testing.T) {
		subject := NewReplaySubject(2)
		defer subject.Dispose()

		// 发射值
		subject.OnNext("A")
		subject.OnNext("B")
		subject.OnNext("C")

		buffered := subject.GetBufferedValues()
		expected := []interface{}{"B", "C"} // 最后2个值

		if !reflect.DeepEqual(buffered, expected) {
			t.Errorf("期望 %v, 得到 %v", expected, buffered)
		}
	})
}

// ============================================================================
// AsyncSubject 详细测试
// ============================================================================

func TestAsyncSubjectComprehensive(t *testing.T) {
	t.Run("只在完成时发送最后一个值", func(t *testing.T) {
		subject := NewAsyncSubject()
		defer subject.Dispose()

		values := []interface{}{}
		var mu sync.Mutex
		done := make(chan bool, 1)

		sub := subject.SubscribeWithCallbacks(
			func(value interface{}) {
				mu.Lock()
				values = append(values, value)
				mu.Unlock()
			},
			nil,
			func() { done <- true },
		)
		defer sub.Unsubscribe()

		// 发射多个值（但不会立即收到）
		subject.OnNext(1)
		subject.OnNext(2)
		subject.OnNext(3)

		// 给一点时间确保没有值被发射
		time.Sleep(10 * time.Millisecond)

		mu.Lock()
		if len(values) != 0 {
			t.Errorf("在完成前不应该收到任何值，但收到 %v", values)
		}
		mu.Unlock()

		// 完成Subject
		subject.OnComplete()

		<-done

		mu.Lock()
		expected := []interface{}{3} // 只有最后一个值
		if !reflect.DeepEqual(values, expected) {
			t.Errorf("期望 %v, 得到 %v", expected, values)
		}
		mu.Unlock()
	})

	t.Run("没有值时只发送完成信号", func(t *testing.T) {
		subject := NewAsyncSubject()
		defer subject.Dispose()

		values := []interface{}{}
		var mu sync.Mutex
		done := make(chan bool, 1)

		sub := subject.SubscribeWithCallbacks(
			func(value interface{}) {
				mu.Lock()
				values = append(values, value)
				mu.Unlock()
			},
			nil,
			func() { done <- true },
		)
		defer sub.Unsubscribe()

		// 直接完成，没有发射任何值
		subject.OnComplete()

		<-done

		mu.Lock()
		if len(values) != 0 {
			t.Errorf("期望空值列表, 得到 %v", values)
		}
		mu.Unlock()
	})
}

// ============================================================================
// 错误处理测试
// ============================================================================

func TestSubjectErrorHandling(t *testing.T) {
	t.Run("PublishSubject错误传播", func(t *testing.T) {
		subject := NewPublishSubject()
		defer subject.Dispose()

		var receivedError error
		done := make(chan bool, 1)

		sub := subject.SubscribeWithCallbacks(
			func(value interface{}) {
				t.Error("不应该收到值")
			},
			func(err error) {
				receivedError = err
				done <- true
			},
			func() {
				t.Error("不应该完成")
			},
		)
		defer sub.Unsubscribe()

		testError := &SubjectError{"测试错误"}
		subject.OnError(testError)

		<-done

		if receivedError != testError {
			t.Errorf("期望错误 %v, 得到 %v", testError, receivedError)
		}
	})
}

// ============================================================================
// 辅助类型
// ============================================================================

// SubjectError 测试用的错误类型
type SubjectError struct {
	Message string
}

func (e *SubjectError) Error() string {
	return e.Message
}
