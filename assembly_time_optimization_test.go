package rxgo

import (
	"fmt"
	"testing"
	"time"
)

// ============================================================================
// Assembly-Time优化测试
// ============================================================================

func TestScalarObservableOptimization(t *testing.T) {
	t.Run("标量值Observable基础功能", func(t *testing.T) {
		value := 42
		obs := NewScalarObservable(value)

		// 验证ScalarCallable接口
		if scalar, ok := obs.(ScalarCallable); ok {
			if scalar.Call() != value {
				t.Errorf("期望标量值 %v，但得到 %v", value, scalar.Call())
			}
			if scalar.IsEmpty() {
				t.Error("期望非空标量，但得到空值")
			}
		} else {
			t.Error("标量Observable应该实现ScalarCallable接口")
		}

		// 验证订阅功能
		received := make(chan interface{}, 1)
		completed := make(chan bool, 1)

		obs.SubscribeWithCallbacks(
			func(item interface{}) {
				received <- item
			},
			func(err error) {
				t.Errorf("不应该收到错误: %v", err)
			},
			func() {
				completed <- true
			},
		)

		// 验证接收到的值
		select {
		case item := <-received:
			if item != value {
				t.Errorf("期望接收到 %v，但得到 %v", value, item)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("超时：未接收到值")
		}

		// 验证完成信号
		select {
		case <-completed:
			// 正常完成
		case <-time.After(100 * time.Millisecond):
			t.Error("超时：未接收到完成信号")
		}
	})

	t.Run("Map操作符assembly-time优化", func(t *testing.T) {
		value := 10
		obs := NewScalarObservable(value)

		// 应用Map转换
		mapped := obs.Map(func(item interface{}) (interface{}, error) {
			return item.(int) * 2, nil
		})

		// 验证优化后的类型
		if scalar, ok := mapped.(ScalarCallable); ok {
			result := scalar.Call()
			if result != 20 {
				t.Errorf("期望Map结果为 20，但得到 %v", result)
			}
		} else {
			t.Error("Map操作符应该返回优化的标量Observable")
		}

		// 验证订阅功能
		received := make(chan interface{}, 1)
		mapped.SubscribeWithCallbacks(
			func(item interface{}) {
				received <- item
			},
			nil,
			nil,
		)

		select {
		case item := <-received:
			if item != 20 {
				t.Errorf("期望接收到 20，但得到 %v", item)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("超时：未接收到值")
		}
	})

	t.Run("Filter操作符assembly-time优化", func(t *testing.T) {
		value := 10
		obs := NewScalarObservable(value)

		// 过滤通过的情况
		filtered := obs.Filter(func(item interface{}) bool {
			return item.(int) > 5
		})

		if scalar, ok := filtered.(ScalarCallable); ok {
			if scalar.Call() != value {
				t.Errorf("期望过滤后的值为 %v，但得到 %v", value, scalar.Call())
			}
		} else {
			t.Error("Filter操作符（通过）应该返回标量Observable")
		}

		// 过滤不通过的情况
		filteredOut := obs.Filter(func(item interface{}) bool {
			return item.(int) < 5
		})

		if empty, ok := filteredOut.(EmptyCallable); ok {
			if !empty.IsEmpty() {
				t.Error("期望Filter操作符（不通过）返回空Observable")
			}
		} else {
			t.Error("Filter操作符（不通过）应该返回空Observable")
		}
	})

	t.Run("Take操作符assembly-time优化", func(t *testing.T) {
		value := 42
		obs := NewScalarObservable(value)

		// Take(1) - 应该返回原始标量
		taken := obs.Take(1)
		if scalar, ok := taken.(ScalarCallable); ok {
			if scalar.Call() != value {
				t.Errorf("期望Take(1)结果为 %v，但得到 %v", value, scalar.Call())
			}
		} else {
			t.Error("Take(1)应该返回标量Observable")
		}

		// Take(0) - 应该返回空Observable
		empty := obs.Take(0)
		if emptyObs, ok := empty.(EmptyCallable); ok {
			if !emptyObs.IsEmpty() {
				t.Error("期望Take(0)返回空Observable")
			}
		} else {
			t.Error("Take(0)应该返回空Observable")
		}
	})

	t.Run("Skip操作符assembly-time优化", func(t *testing.T) {
		value := 42
		obs := NewScalarObservable(value)

		// Skip(0) - 应该返回原始标量
		notSkipped := obs.Skip(0)
		if scalar, ok := notSkipped.(ScalarCallable); ok {
			if scalar.Call() != value {
				t.Errorf("期望Skip(0)结果为 %v，但得到 %v", value, scalar.Call())
			}
		} else {
			t.Error("Skip(0)应该返回标量Observable")
		}

		// Skip(1) - 应该返回空Observable
		skipped := obs.Skip(1)
		if emptyObs, ok := skipped.(EmptyCallable); ok {
			if !emptyObs.IsEmpty() {
				t.Error("期望Skip(1)返回空Observable")
			}
		} else {
			t.Error("Skip(1)应该返回空Observable")
		}
	})
}

func TestEmptyObservableOptimization(t *testing.T) {
	t.Run("空Observable基础功能", func(t *testing.T) {
		obs := NewEmptyObservable()

		// 验证EmptyCallable接口
		if empty, ok := obs.(EmptyCallable); ok {
			if !empty.IsEmpty() {
				t.Error("期望空Observable返回true，但得到false")
			}
		} else {
			t.Error("空Observable应该实现EmptyCallable接口")
		}

		// 验证订阅功能
		received := make(chan interface{}, 1)
		completed := make(chan bool, 1)

		obs.SubscribeWithCallbacks(
			func(item interface{}) {
				received <- item
			},
			func(err error) {
				t.Errorf("不应该收到错误: %v", err)
			},
			func() {
				completed <- true
			},
		)

		// 验证不应该接收到值
		select {
		case item := <-received:
			t.Errorf("空Observable不应该发射值，但收到 %v", item)
		case <-time.After(50 * time.Millisecond):
			// 正常，不应该接收到值
		}

		// 验证完成信号
		select {
		case <-completed:
			// 正常完成
		case <-time.After(100 * time.Millisecond):
			t.Error("超时：未接收到完成信号")
		}
	})

	t.Run("空Observable操作符优化", func(t *testing.T) {
		obs := NewEmptyObservable()

		// 所有转换操作符都应该返回空Observable
		operations := []Observable{
			obs.Map(func(item interface{}) (interface{}, error) { return item, nil }),
			obs.Filter(func(item interface{}) bool { return true }),
			obs.Take(10),
			obs.Skip(0),
			obs.Buffer(5),
		}

		for i, op := range operations {
			if empty, ok := op.(EmptyCallable); ok {
				if !empty.IsEmpty() {
					t.Errorf("操作符 %d 应该返回空Observable", i)
				}
			} else {
				t.Errorf("操作符 %d 应该返回空Observable", i)
			}
		}
	})

	t.Run("空Observable组合操作符优化", func(t *testing.T) {
		empty := NewEmptyObservable()
		value := NewScalarObservable(42)

		// Merge操作符优化
		merged := empty.Merge(value)
		if scalar, ok := merged.(ScalarCallable); ok {
			if scalar.Call() != 42 {
				t.Errorf("期望Merge结果为 42，但得到 %v", scalar.Call())
			}
		} else {
			// 标准实现也可以接受
		}

		// Concat操作符优化
		concatenated := empty.Concat(value)
		if scalar, ok := concatenated.(ScalarCallable); ok {
			if scalar.Call() != 42 {
				t.Errorf("期望Concat结果为 42，但得到 %v", scalar.Call())
			}
		} else {
			// 标准实现也可以接受
		}

		// Amb操作符优化
		amb := empty.Amb(value)
		if scalar, ok := amb.(ScalarCallable); ok {
			if scalar.Call() != 42 {
				t.Errorf("期望Amb结果为 42，但得到 %v", scalar.Call())
			}
		} else {
			// 标准实现也可以接受
		}
	})
}

func TestAssemblyTimeOptimizationFactory(t *testing.T) {
	t.Run("OptimizedJust工厂函数", func(t *testing.T) {
		value := "hello"
		obs := OptimizedJust(value)

		if scalar, ok := obs.(ScalarCallable); ok {
			if scalar.Call() != value {
				t.Errorf("期望标量值 %v，但得到 %v", value, scalar.Call())
			}
		} else {
			t.Error("OptimizedJust应该返回标量Observable")
		}
	})

	t.Run("OptimizedEmpty工厂函数", func(t *testing.T) {
		obs := OptimizedEmpty()

		if empty, ok := obs.(EmptyCallable); ok {
			if !empty.IsEmpty() {
				t.Error("OptimizedEmpty应该返回空Observable")
			}
		} else {
			t.Error("OptimizedEmpty应该返回空Observable")
		}
	})

	t.Run("AssemblyTimeOptimization检测函数", func(t *testing.T) {
		// 测试标量Observable检测
		scalar := NewScalarObservable(42)
		if scalarCallable, ok := IsScalarCallable(scalar); ok {
			if scalarCallable.Call() != 42 {
				t.Error("标量检测失败")
			}
		} else {
			t.Error("应该检测到标量Observable")
		}

		// 测试空Observable检测
		empty := NewEmptyObservable()
		if emptyCallable, ok := IsEmptyCallable(empty); ok {
			if !emptyCallable.IsEmpty() {
				t.Error("空Observable检测失败")
			}
		} else {
			t.Error("应该检测到空Observable")
		}

		// 测试优化应用
		optimized := ApplyAssemblyTimeOptimization(scalar)
		if optimizedScalar, ok := optimized.(ScalarCallable); ok {
			if optimizedScalar.Call() != 42 {
				t.Error("优化应用失败")
			}
		} else {
			t.Error("优化应用应该返回标量Observable")
		}
	})
}

func TestMapTransformationErrors(t *testing.T) {
	t.Run("Map转换错误处理", func(t *testing.T) {
		obs := NewScalarObservable(42)

		// 应用会产生错误的Map转换
		mapped := obs.Map(func(item interface{}) (interface{}, error) {
			return nil, fmt.Errorf("转换错误")
		})

		// 验证错误处理
		errorReceived := make(chan error, 1)
		mapped.SubscribeWithCallbacks(
			func(item interface{}) {
				t.Error("不应该接收到值")
			},
			func(err error) {
				errorReceived <- err
			},
			func() {
				t.Error("不应该完成")
			},
		)

		// 验证接收到错误
		select {
		case err := <-errorReceived:
			if err.Error() != "转换错误" {
				t.Errorf("期望错误消息 '转换错误'，但得到 '%v'", err.Error())
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("超时：未接收到错误")
		}
	})
}

// ============================================================================
// 性能基准测试
// ============================================================================

func BenchmarkScalarObservableVsJust(b *testing.B) {
	value := 42

	b.Run("ScalarObservable", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			obs := NewScalarObservable(value)
			obs.Subscribe(func(item Item) {
				_ = item.Value
			})
		}
	})

	b.Run("StandardJust", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			obs := Just(value)
			obs.Subscribe(func(item Item) {
				_ = item.Value
			})
		}
	})
}

func BenchmarkEmptyObservableVsEmpty(b *testing.B) {
	b.Run("EmptyObservable", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			obs := NewEmptyObservable()
			obs.Subscribe(func(item Item) {
				_ = item.Value
			})
		}
	})

	b.Run("StandardEmpty", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			obs := Empty()
			obs.Subscribe(func(item Item) {
				_ = item.Value
			})
		}
	})
}

func BenchmarkAssemblyTimeMapOptimization(b *testing.B) {
	value := 10
	transformer := func(item interface{}) (interface{}, error) {
		return item.(int) * 2, nil
	}

	b.Run("OptimizedMap", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			obs := NewScalarObservable(value)
			mapped := obs.Map(transformer)
			mapped.Subscribe(func(item Item) {
				_ = item.Value
			})
		}
	})

	b.Run("StandardMap", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			obs := Just(value)
			mapped := obs.Map(transformer)
			mapped.Subscribe(func(item Item) {
				_ = item.Value
			})
		}
	})
}
