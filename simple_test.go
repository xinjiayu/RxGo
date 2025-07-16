// Simple tests for RxGo implemented features
// RxGo 已实现功能的简单测试
package rxgo

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// 基础功能测试
// ============================================================================

func TestBasicRxGoFeatures(t *testing.T) {
	fmt.Println("=== 测试RxGo基础功能 ===")

	t.Run("Just和SubscribeWithCallbacks", func(t *testing.T) {
		values := []interface{}{}
		var mu sync.Mutex
		done := make(chan bool, 1)

		obs := Just(1, 2, 3, 4, 5)
		subscription := obs.SubscribeWithCallbacks(
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

		select {
		case <-done:
			mu.Lock()
			expected := []interface{}{1, 2, 3, 4, 5}
			if !reflect.DeepEqual(values, expected) {
				t.Errorf("期望 %v, 得到 %v", expected, values)
			}
			mu.Unlock()
		case <-time.After(time.Second):
			t.Error("测试超时")
		}

		subscription.Unsubscribe()
	})

	t.Run("Map操作符", func(t *testing.T) {
		values := []interface{}{}
		var mu sync.Mutex
		done := make(chan bool, 1)

		obs := Just(1, 2, 3).Map(func(value interface{}) (interface{}, error) {
			return value.(int) * 2, nil
		})

		subscription := obs.SubscribeWithCallbacks(
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

		select {
		case <-done:
			mu.Lock()
			expected := []interface{}{2, 4, 6}
			if !reflect.DeepEqual(values, expected) {
				t.Errorf("Map期望 %v, 得到 %v", expected, values)
			}
			mu.Unlock()
		case <-time.After(time.Second):
			t.Error("Map测试超时")
		}

		subscription.Unsubscribe()
	})

	t.Run("Filter操作符", func(t *testing.T) {
		values := []interface{}{}
		var mu sync.Mutex
		done := make(chan bool, 1)

		obs := Just(1, 2, 3, 4, 5).Filter(func(value interface{}) bool {
			return value.(int)%2 == 0
		})

		subscription := obs.SubscribeWithCallbacks(
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

		select {
		case <-done:
			mu.Lock()
			expected := []interface{}{2, 4}
			if !reflect.DeepEqual(values, expected) {
				t.Errorf("Filter期望 %v, 得到 %v", expected, values)
			}
			mu.Unlock()
		case <-time.After(time.Second):
			t.Error("Filter测试超时")
		}

		subscription.Unsubscribe()
	})

	t.Run("Take和Skip操作符", func(t *testing.T) {
		// 测试Take
		takeValues := []interface{}{}
		var takeMu sync.Mutex
		takeDone := make(chan bool, 1)

		takeObs := Just(1, 2, 3, 4, 5).Take(3)
		takeSubscription := takeObs.SubscribeWithCallbacks(
			func(value interface{}) {
				takeMu.Lock()
				takeValues = append(takeValues, value)
				takeMu.Unlock()
			},
			func(err error) {
				t.Errorf("Take不应该有错误: %v", err)
			},
			func() {
				takeDone <- true
			},
		)

		// 测试Skip
		skipValues := []interface{}{}
		var skipMu sync.Mutex
		skipDone := make(chan bool, 1)

		skipObs := Just(1, 2, 3, 4, 5).Skip(2)
		skipSubscription := skipObs.SubscribeWithCallbacks(
			func(value interface{}) {
				skipMu.Lock()
				skipValues = append(skipValues, value)
				skipMu.Unlock()
			},
			func(err error) {
				t.Errorf("Skip不应该有错误: %v", err)
			},
			func() {
				skipDone <- true
			},
		)

		// 验证Take结果
		select {
		case <-takeDone:
			takeMu.Lock()
			expectedTake := []interface{}{1, 2, 3}
			if !reflect.DeepEqual(takeValues, expectedTake) {
				t.Errorf("Take期望 %v, 得到 %v", expectedTake, takeValues)
			}
			takeMu.Unlock()
		case <-time.After(time.Second):
			t.Error("Take测试超时")
		}

		// 验证Skip结果
		select {
		case <-skipDone:
			skipMu.Lock()
			expectedSkip := []interface{}{3, 4, 5}
			if !reflect.DeepEqual(skipValues, expectedSkip) {
				t.Errorf("Skip期望 %v, 得到 %v", expectedSkip, skipValues)
			}
			skipMu.Unlock()
		case <-time.After(time.Second):
			t.Error("Skip测试超时")
		}

		takeSubscription.Unsubscribe()
		skipSubscription.Unsubscribe()
	})
}

// ============================================================================
// Single 类型测试
// ============================================================================

func TestSingleBasicFeatures(t *testing.T) {
	fmt.Println("=== 测试Single基础功能 ===")

	t.Run("SingleJust创建和订阅", func(t *testing.T) {
		single := SingleJust(42)

		result := make(chan interface{}, 1)
		errChan := make(chan error, 1)

		single.Subscribe(func(value interface{}) {
			result <- value
		}, func(err error) {
			errChan <- err
		})

		select {
		case value := <-result:
			if value != 42 {
				t.Errorf("Single期望42, 得到%v", value)
			}
		case err := <-errChan:
			t.Errorf("Single不应该有错误: %v", err)
		case <-time.After(time.Second):
			t.Error("Single测试超时")
		}
	})

	t.Run("SingleMap操作", func(t *testing.T) {
		single := SingleJust(5).Map(func(value interface{}) (interface{}, error) {
			return value.(int) * 2, nil
		})

		result := make(chan interface{}, 1)
		errChan := make(chan error, 1)

		single.Subscribe(func(value interface{}) {
			result <- value
		}, func(err error) {
			errChan <- err
		})

		select {
		case value := <-result:
			if value != 10 {
				t.Errorf("Single Map期望10, 得到%v", value)
			}
		case err := <-errChan:
			t.Errorf("Single Map不应该有错误: %v", err)
		case <-time.After(time.Second):
			t.Error("Single Map测试超时")
		}
	})

	t.Run("Single阻塞获取", func(t *testing.T) {
		single := SingleJust("测试值")
		value, err := single.BlockingGet()

		if err != nil {
			t.Errorf("Single阻塞获取错误: %v", err)
		}

		if value != "测试值" {
			t.Errorf("Single阻塞获取期望'测试值', 得到'%v'", value)
		}
	})
}

// ============================================================================
// Maybe 类型测试
// ============================================================================

func TestMaybeBasicFeatures(t *testing.T) {
	fmt.Println("=== 测试Maybe基础功能 ===")

	t.Run("MaybeJust有值情况", func(t *testing.T) {
		maybe := MaybeJust(42)

		result := make(chan interface{}, 1)
		errChan := make(chan error, 1)
		completeChan := make(chan bool, 1)

		maybe.Subscribe(func(value interface{}) {
			result <- value
		}, func(err error) {
			errChan <- err
		}, func() {
			completeChan <- true
		})

		select {
		case value := <-result:
			if value != 42 {
				t.Errorf("Maybe期望42, 得到%v", value)
			}
		case err := <-errChan:
			t.Errorf("Maybe不应该有错误: %v", err)
		case <-completeChan:
			t.Error("Maybe有值时不应该直接完成")
		case <-time.After(time.Second):
			t.Error("Maybe测试超时")
		}
	})

	t.Run("MaybeEmpty空值情况", func(t *testing.T) {
		maybe := MaybeEmpty()

		result := make(chan interface{}, 1)
		errChan := make(chan error, 1)
		completeChan := make(chan bool, 1)

		maybe.Subscribe(func(value interface{}) {
			result <- value
		}, func(err error) {
			errChan <- err
		}, func() {
			completeChan <- true
		})

		select {
		case value := <-result:
			t.Errorf("Maybe空值不应该有值: %v", value)
		case err := <-errChan:
			t.Errorf("Maybe空值不应该有错误: %v", err)
		case <-completeChan:
			// 正确，空Maybe应该直接完成
		case <-time.After(time.Second):
			t.Error("Maybe空值测试超时")
		}
	})
}

// ============================================================================
// 错误处理测试
// ============================================================================

func TestErrorHandlingBasics(t *testing.T) {
	fmt.Println("=== 测试基础错误处理 ===")

	t.Run("Create创建和错误处理", func(t *testing.T) {
		hasError := false
		done := make(chan bool, 1)

		obs := Create(func(observer Observer) {
			observer(CreateItem(1))
			observer(CreateItem(2))
			observer(CreateErrorItem(errors.New("测试错误")))
		})

		subscription := obs.SubscribeWithCallbacks(
			func(value interface{}) {
				// 正常接收值
			},
			func(err error) {
				hasError = true
				if err.Error() != "测试错误" {
					t.Errorf("期望错误'测试错误', 得到'%v'", err.Error())
				}
				done <- true
			},
			func() {
				done <- true
			},
		)

		select {
		case <-done:
			if !hasError {
				t.Error("应该收到错误")
			}
		case <-time.After(time.Second):
			t.Error("错误处理测试超时")
		}

		subscription.Unsubscribe()
	})
}

// ============================================================================
// Subject 测试
// ============================================================================

func TestSubjectBasics(t *testing.T) {
	fmt.Println("=== 测试Subject基础功能 ===")

	t.Run("PublishSubject基本功能", func(t *testing.T) {
		subject := NewPublishSubject()

		values1 := []interface{}{}
		values2 := []interface{}{}
		var mu1, mu2 sync.Mutex
		done1 := make(chan bool, 1)
		done2 := make(chan bool, 1)

		// 第一个订阅者
		sub1 := subject.SubscribeWithCallbacks(
			func(value interface{}) {
				mu1.Lock()
				values1 = append(values1, value)
				mu1.Unlock()
			},
			func(err error) {
				t.Errorf("订阅者1不应该有错误: %v", err)
			},
			func() {
				done1 <- true
			},
		)

		// 第二个订阅者
		sub2 := subject.SubscribeWithCallbacks(
			func(value interface{}) {
				mu2.Lock()
				values2 = append(values2, value)
				mu2.Unlock()
			},
			func(err error) {
				t.Errorf("订阅者2不应该有错误: %v", err)
			},
			func() {
				done2 <- true
			},
		)

		// 发射值
		subject.OnNext(1)
		subject.OnNext(2)
		subject.OnNext(3)
		subject.OnComplete()

		// 等待完成
		<-done1
		<-done2

		// 验证结果
		mu1.Lock()
		expected := []interface{}{1, 2, 3}
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
}

// ============================================================================
// 主测试函数
// ============================================================================

func TestImplementedFunctionality(t *testing.T) {
	fmt.Println("========== 开始RxGo 已实现功能测试 ==========")

	// 运行所有基础功能测试
	t.Run("基础功能", TestBasicRxGoFeatures)
	t.Run("Single功能", TestSingleBasicFeatures)
	t.Run("Maybe功能", TestMaybeBasicFeatures)
	t.Run("错误处理", TestErrorHandlingBasics)
	t.Run("Subject功能", TestSubjectBasics)

	fmt.Println("========== RxGo 已实现功能测试完成 ==========")
}
