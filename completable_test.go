// Completable tests for RxGo
// Completable全面测试套件，验证所有核心功能
package rxgo

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// 基础功能测试
// ============================================================================

func TestCompletableBasics(t *testing.T) {
	t.Run("CompletableComplete", func(t *testing.T) {
		completed := int32(0)
		errored := int32(0)

		completable := CompletableComplete()
		subscription := completable.Subscribe(func() {
			atomic.StoreInt32(&completed, 1)
		}, func(err error) {
			atomic.StoreInt32(&errored, 1)
		})

		// 等待完成
		time.Sleep(50 * time.Millisecond)

		if atomic.LoadInt32(&completed) != 1 {
			t.Error("CompletableComplete应该立即完成")
		}

		if atomic.LoadInt32(&errored) != 0 {
			t.Error("CompletableComplete不应该发生错误")
		}

		subscription.Unsubscribe()
	})

	t.Run("CompletableError", func(t *testing.T) {
		completed := int32(0)
		errored := int32(0)
		var receivedError error

		expectedError := errors.New("测试错误")
		completable := CompletableError(expectedError)
		subscription := completable.Subscribe(func() {
			atomic.StoreInt32(&completed, 1)
		}, func(err error) {
			atomic.StoreInt32(&errored, 1)
			receivedError = err
		})

		// 等待错误
		time.Sleep(50 * time.Millisecond)

		if atomic.LoadInt32(&completed) != 0 {
			t.Error("CompletableError不应该完成")
		}

		if atomic.LoadInt32(&errored) != 1 {
			t.Error("CompletableError应该发生错误")
		}

		if receivedError != expectedError {
			t.Errorf("期望错误 %v，实际错误 %v", expectedError, receivedError)
		}

		subscription.Unsubscribe()
	})

	t.Run("CompletableCreate", func(t *testing.T) {
		completed := int32(0)
		errored := int32(0)

		completable := CompletableCreate(func(onComplete OnComplete, onError OnError) {
			// 模拟异步操作
			go func() {
				time.Sleep(10 * time.Millisecond)
				onComplete()
			}()
		})

		subscription := completable.Subscribe(func() {
			atomic.StoreInt32(&completed, 1)
		}, func(err error) {
			atomic.StoreInt32(&errored, 1)
		})

		// 等待完成
		time.Sleep(100 * time.Millisecond)

		if atomic.LoadInt32(&completed) != 1 {
			t.Error("CompletableCreate应该完成")
		}

		if atomic.LoadInt32(&errored) != 0 {
			t.Error("CompletableCreate不应该发生错误")
		}

		subscription.Unsubscribe()
	})
}

// ============================================================================
// 操作符测试
// ============================================================================

func TestCompletableOperators(t *testing.T) {
	t.Run("AndThen", func(t *testing.T) {
		completed1 := int32(0)
		completed2 := int32(0)
		var order []int
		var mu sync.Mutex

		first := CompletableCreate(func(onComplete OnComplete, onError OnError) {
			go func() {
				time.Sleep(10 * time.Millisecond)
				atomic.StoreInt32(&completed1, 1)
				mu.Lock()
				order = append(order, 1)
				mu.Unlock()
				onComplete()
			}()
		})

		second := CompletableCreate(func(onComplete OnComplete, onError OnError) {
			go func() {
				atomic.StoreInt32(&completed2, 1)
				mu.Lock()
				order = append(order, 2)
				mu.Unlock()
				onComplete()
			}()
		})

		finalCompleted := int32(0)
		completable := first.AndThen(second)
		subscription := completable.Subscribe(func() {
			atomic.StoreInt32(&finalCompleted, 1)
		}, func(err error) {
			t.Errorf("AndThen不应该发生错误: %v", err)
		})

		// 等待完成
		time.Sleep(100 * time.Millisecond)

		if atomic.LoadInt32(&completed1) != 1 {
			t.Error("第一个Completable应该完成")
		}

		if atomic.LoadInt32(&completed2) != 1 {
			t.Error("第二个Completable应该完成")
		}

		if atomic.LoadInt32(&finalCompleted) != 1 {
			t.Error("AndThen应该完成")
		}

		mu.Lock()
		if len(order) != 2 || order[0] != 1 || order[1] != 2 {
			t.Errorf("执行顺序错误: %v", order)
		}
		mu.Unlock()

		subscription.Unsubscribe()
	})

	t.Run("Merge", func(t *testing.T) {
		completed1 := int32(0)
		completed2 := int32(0)

		first := CompletableCreate(func(onComplete OnComplete, onError OnError) {
			go func() {
				time.Sleep(20 * time.Millisecond)
				atomic.StoreInt32(&completed1, 1)
				onComplete()
			}()
		})

		second := CompletableCreate(func(onComplete OnComplete, onError OnError) {
			go func() {
				time.Sleep(10 * time.Millisecond)
				atomic.StoreInt32(&completed2, 1)
				onComplete()
			}()
		})

		finalCompleted := int32(0)
		completable := first.Merge(second)
		subscription := completable.Subscribe(func() {
			atomic.StoreInt32(&finalCompleted, 1)
		}, func(err error) {
			t.Errorf("Merge不应该发生错误: %v", err)
		})

		// 等待完成
		time.Sleep(100 * time.Millisecond)

		if atomic.LoadInt32(&completed1) != 1 {
			t.Error("第一个Completable应该完成")
		}

		if atomic.LoadInt32(&completed2) != 1 {
			t.Error("第二个Completable应该完成")
		}

		if atomic.LoadInt32(&finalCompleted) != 1 {
			t.Error("Merge应该完成")
		}

		subscription.Unsubscribe()
	})
}

// ============================================================================
// 错误处理测试
// ============================================================================

func TestCompletableErrorHandling(t *testing.T) {
	t.Run("Catch", func(t *testing.T) {
		completed := int32(0)
		errored := int32(0)

		originalError := errors.New("原始错误")
		failing := CompletableError(originalError)

		recovery := CompletableComplete()

		completable := failing.Catch(func(err error) Completable {
			if err != originalError {
				t.Errorf("期望错误 %v，实际错误 %v", originalError, err)
			}
			return recovery
		})

		subscription := completable.Subscribe(func() {
			atomic.StoreInt32(&completed, 1)
		}, func(err error) {
			atomic.StoreInt32(&errored, 1)
		})

		// 等待完成
		time.Sleep(100 * time.Millisecond)

		if atomic.LoadInt32(&completed) != 1 {
			t.Error("Catch应该恢复并完成")
		}

		if atomic.LoadInt32(&errored) != 0 {
			t.Error("Catch不应该传播错误")
		}

		subscription.Unsubscribe()
	})

	t.Run("Retry", func(t *testing.T) {
		attempts := int32(0)
		completed := int32(0)

		failing := CompletableCreate(func(onComplete OnComplete, onError OnError) {
			go func() {
				count := atomic.AddInt32(&attempts, 1)
				if count < 3 {
					// 前两次失败
					onError(errors.New("重试测试错误"))
				} else {
					// 第三次成功
					onComplete()
				}
			}()
		})

		completable := failing.Retry(3)
		subscription := completable.Subscribe(func() {
			atomic.StoreInt32(&completed, 1)
		}, func(err error) {
			t.Errorf("Retry不应该发生错误: %v", err)
		})

		// 等待重试完成
		time.Sleep(200 * time.Millisecond)

		if atomic.LoadInt32(&attempts) != 3 {
			t.Errorf("期望3次尝试，实际 %d 次", atomic.LoadInt32(&attempts))
		}

		if atomic.LoadInt32(&completed) != 1 {
			t.Error("Retry应该最终完成")
		}

		subscription.Unsubscribe()
	})
}

// ============================================================================
// 时间操作符测试
// ============================================================================

func TestCompletableTimeOperators(t *testing.T) {
	t.Run("Delay", func(t *testing.T) {
		completed := int32(0)
		start := time.Now()

		completable := CompletableComplete().Delay(50 * time.Millisecond)
		subscription := completable.Subscribe(func() {
			atomic.StoreInt32(&completed, 1)
		}, func(err error) {
			t.Errorf("Delay不应该发生错误: %v", err)
		})

		// 等待延迟完成
		time.Sleep(100 * time.Millisecond)

		elapsed := time.Since(start)
		if elapsed < 50*time.Millisecond {
			t.Error("Delay时间不足")
		}

		if atomic.LoadInt32(&completed) != 1 {
			t.Error("Delay应该完成")
		}

		subscription.Unsubscribe()
	})

	t.Run("Timeout", func(t *testing.T) {
		errored := int32(0)
		var receivedError error

		// 创建一个需要很长时间的Completable
		slow := CompletableCreate(func(onComplete OnComplete, onError OnError) {
			go func() {
				time.Sleep(200 * time.Millisecond)
				onComplete()
			}()
		})

		completable := slow.Timeout(50 * time.Millisecond)
		subscription := completable.Subscribe(func() {
			t.Error("Timeout不应该完成")
		}, func(err error) {
			atomic.StoreInt32(&errored, 1)
			receivedError = err
		})

		// 等待超时
		time.Sleep(100 * time.Millisecond)

		if atomic.LoadInt32(&errored) != 1 {
			t.Error("Timeout应该发生错误")
		}

		if receivedError == nil {
			t.Error("应该收到超时错误")
		}

		subscription.Unsubscribe()
	})
}

// ============================================================================
// 转换测试
// ============================================================================

func TestCompletableConversions(t *testing.T) {
	t.Run("ToObservable", func(t *testing.T) {
		completed := int32(0)
		itemCount := int32(0)

		completable := CompletableComplete()
		observable := completable.ToObservable()

		subscription := observable.Subscribe(func(item Item) {
			if item.IsError() {
				t.Errorf("ToObservable不应该发生错误: %v", item.Error)
				return
			}

			if item.Value == nil {
				atomic.StoreInt32(&completed, 1)
			} else {
				atomic.AddInt32(&itemCount, 1)
			}
		})

		// 等待完成
		time.Sleep(50 * time.Millisecond)

		if atomic.LoadInt32(&completed) != 1 {
			t.Error("ToObservable应该发送完成信号")
		}

		if atomic.LoadInt32(&itemCount) != 0 {
			t.Error("ToObservable不应该发送值")
		}

		subscription.Unsubscribe()
	})

	t.Run("ToSingle", func(t *testing.T) {
		received := int32(0)
		var receivedValue interface{}

		completable := CompletableComplete()
		single := completable.ToSingle("默认值")

		subscription := single.Subscribe(func(value interface{}) {
			atomic.StoreInt32(&received, 1)
			receivedValue = value
		}, func(err error) {
			t.Errorf("ToSingle不应该发生错误: %v", err)
		})

		// 等待完成
		time.Sleep(50 * time.Millisecond)

		if atomic.LoadInt32(&received) != 1 {
			t.Error("ToSingle应该发送值")
		}

		if receivedValue != "默认值" {
			t.Errorf("期望值 '默认值'，实际值 '%v'", receivedValue)
		}

		subscription.Unsubscribe()
	})
}

// ============================================================================
// 阻塞操作测试
// ============================================================================

func TestCompletableBlocking(t *testing.T) {
	t.Run("BlockingAwait", func(t *testing.T) {
		completable := CompletableTimer(50 * time.Millisecond)

		start := time.Now()
		err := completable.BlockingAwait()
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("BlockingAwait不应该发生错误: %v", err)
		}

		if elapsed < 50*time.Millisecond {
			t.Error("BlockingAwait时间不足")
		}
	})

	t.Run("BlockingAwaitWithTimeout", func(t *testing.T) {
		// 测试正常完成
		fast := CompletableTimer(10 * time.Millisecond)
		err := fast.BlockingAwaitWithTimeout(50 * time.Millisecond)

		if err != nil {
			t.Errorf("快速完成不应该超时: %v", err)
		}

		// 测试超时
		slow := CompletableTimer(100 * time.Millisecond)
		err = slow.BlockingAwaitWithTimeout(20 * time.Millisecond)

		if err == nil {
			t.Error("慢速完成应该超时")
		}
	})
}

// ============================================================================
// 工厂函数测试
// ============================================================================

func TestCompletableFactory(t *testing.T) {
	t.Run("CompletableFromAction", func(t *testing.T) {
		executed := int32(0)
		completed := int32(0)

		action := func() error {
			atomic.StoreInt32(&executed, 1)
			return nil
		}

		completable := CompletableFromAction(action)
		subscription := completable.Subscribe(func() {
			atomic.StoreInt32(&completed, 1)
		}, func(err error) {
			t.Errorf("FromAction不应该发生错误: %v", err)
		})

		// 等待完成
		time.Sleep(50 * time.Millisecond)

		if atomic.LoadInt32(&executed) != 1 {
			t.Error("Action应该被执行")
		}

		if atomic.LoadInt32(&completed) != 1 {
			t.Error("FromAction应该完成")
		}

		subscription.Unsubscribe()
	})

	t.Run("CompletableMerge", func(t *testing.T) {
		completed1 := int32(0)
		completed2 := int32(0)
		completed3 := int32(0)
		finalCompleted := int32(0)

		c1 := CompletableCreate(func(onComplete OnComplete, onError OnError) {
			go func() {
				time.Sleep(10 * time.Millisecond)
				atomic.StoreInt32(&completed1, 1)
				onComplete()
			}()
		})

		c2 := CompletableCreate(func(onComplete OnComplete, onError OnError) {
			go func() {
				time.Sleep(20 * time.Millisecond)
				atomic.StoreInt32(&completed2, 1)
				onComplete()
			}()
		})

		c3 := CompletableCreate(func(onComplete OnComplete, onError OnError) {
			go func() {
				time.Sleep(15 * time.Millisecond)
				atomic.StoreInt32(&completed3, 1)
				onComplete()
			}()
		})

		merged := CompletableMerge(c1, c2, c3)
		subscription := merged.Subscribe(func() {
			atomic.StoreInt32(&finalCompleted, 1)
		}, func(err error) {
			t.Errorf("Merge不应该发生错误: %v", err)
		})

		// 等待所有完成
		time.Sleep(100 * time.Millisecond)

		if atomic.LoadInt32(&completed1) != 1 {
			t.Error("第一个Completable应该完成")
		}

		if atomic.LoadInt32(&completed2) != 1 {
			t.Error("第二个Completable应该完成")
		}

		if atomic.LoadInt32(&completed3) != 1 {
			t.Error("第三个Completable应该完成")
		}

		if atomic.LoadInt32(&finalCompleted) != 1 {
			t.Error("Merge应该完成")
		}

		subscription.Unsubscribe()
	})
}

// ============================================================================
// 副作用操作符测试
// ============================================================================

func TestCompletableSideEffects(t *testing.T) {
	t.Run("DoOnComplete", func(t *testing.T) {
		sideEffectExecuted := int32(0)
		completed := int32(0)

		completable := CompletableComplete().DoOnComplete(func() {
			atomic.StoreInt32(&sideEffectExecuted, 1)
		})

		subscription := completable.Subscribe(func() {
			atomic.StoreInt32(&completed, 1)
		}, func(err error) {
			t.Errorf("DoOnComplete不应该发生错误: %v", err)
		})

		// 等待完成
		time.Sleep(50 * time.Millisecond)

		if atomic.LoadInt32(&sideEffectExecuted) != 1 {
			t.Error("DoOnComplete副作用应该执行")
		}

		if atomic.LoadInt32(&completed) != 1 {
			t.Error("DoOnComplete应该完成")
		}

		subscription.Unsubscribe()
	})

	t.Run("DoOnError", func(t *testing.T) {
		sideEffectExecuted := int32(0)
		errored := int32(0)

		expectedError := errors.New("测试错误")
		completable := CompletableError(expectedError).DoOnError(func(err error) {
			if err == expectedError {
				atomic.StoreInt32(&sideEffectExecuted, 1)
			}
		})

		subscription := completable.Subscribe(func() {
			t.Error("DoOnError不应该完成")
		}, func(err error) {
			atomic.StoreInt32(&errored, 1)
		})

		// 等待错误
		time.Sleep(50 * time.Millisecond)

		if atomic.LoadInt32(&sideEffectExecuted) != 1 {
			t.Error("DoOnError副作用应该执行")
		}

		if atomic.LoadInt32(&errored) != 1 {
			t.Error("DoOnError应该传播错误")
		}

		subscription.Unsubscribe()
	})
}
