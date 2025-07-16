// Factory functions for RxGo
// 工厂函数，提供符合Go习惯的API设计
package rxgo

import (
	"context"
	"sync"
	"time"
)

// ============================================================================
// 基础工厂函数
// ============================================================================

// Just 从给定的值创建Observable
func Just(values ...interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		go func() {
			for _, value := range values {
				observer(CreateItem(value))
			}
			observer(CreateItem(nil)) // 完成信号
		}()

		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// Empty 创建一个空的Observable，立即完成
func Empty() Observable {
	return NewObservable(func(observer Observer) Subscription {
		go func() {
			observer(CreateItem(nil)) // 立即发送完成信号
		}()

		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// Never 创建一个永不发射任何值的Observable
func Never() Observable {
	return NewObservable(func(observer Observer) Subscription {
		// 什么都不做，永远不发射值
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// Error 创建一个立即发射错误的Observable
func Error(err error) Observable {
	return NewObservable(func(observer Observer) Subscription {
		go func() {
			observer(CreateErrorItem(err))
		}()

		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// Range 创建发射指定范围整数的Observable
func Range(start, count int) Observable {
	return NewObservable(func(observer Observer) Subscription {
		go func() {
			for i := 0; i < count; i++ {
				observer(CreateItem(start + i))
			}
			observer(CreateItem(nil)) // 完成信号
		}()

		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// ============================================================================
// 从数据源创建
// ============================================================================

// FromSlice 从切片创建Observable
func FromSlice(slice []interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		go func() {
			for _, item := range slice {
				observer(CreateItem(item))
			}
			observer(CreateItem(nil)) // 完成信号
		}()

		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// FromChannel 从Go channel创建Observable
func FromChannel(ch <-chan interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			defer cancel()

			for {
				select {
				case <-ctx.Done():
					return
				case value, ok := <-ch:
					if !ok {
						observer(CreateItem(nil)) // 完成信号
						return
					}
					observer(CreateItem(value))
				}
			}
		}()

		return NewBaseSubscription(NewBaseDisposable(cancel))
	})
}

// FromItemChannel 从Item channel创建Observable
func FromItemChannel(ch <-chan Item) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			defer cancel()

			for {
				select {
				case <-ctx.Done():
					return
				case item, ok := <-ch:
					if !ok {
						observer(CreateItem(nil)) // 完成信号
						return
					}
					observer(item)
				}
			}
		}()

		return NewBaseSubscription(NewBaseDisposable(cancel))
	})
}

// FromIterable 从可迭代函数创建Observable
func FromIterable(iterable func() <-chan interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ch := iterable()
		return FromChannel(ch).Subscribe(observer)
	})
}

// ============================================================================
// 时间相关工厂函数
// ============================================================================

// Interval 创建定期发射递增整数的Observable
func Interval(duration time.Duration) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			defer cancel()

			ticker := time.NewTicker(duration)
			defer ticker.Stop()

			counter := 0
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					observer(CreateItem(counter))
					counter++
				}
			}
		}()

		return NewBaseSubscription(NewBaseDisposable(cancel))
	})
}

// Timer 创建在指定延迟后发射单个值的Observable
func Timer(duration time.Duration) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			defer cancel()

			timer := time.NewTimer(duration)
			defer timer.Stop()

			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				observer(CreateItem(0))   // 发射0
				observer(CreateItem(nil)) // 完成信号
			}
		}()

		return NewBaseSubscription(NewBaseDisposable(cancel))
	})
}

// ============================================================================
// 创建操作符
// ============================================================================

// Create 从自定义发射器创建Observable
func Create(emitter func(Observer)) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())

		// 创建带上下文检查的观察者包装器
		wrappedObserver := func(item Item) {
			select {
			case <-ctx.Done():
				return
			default:
				observer(item)
			}
		}

		// 在goroutine中执行emitter，但不在完成时取消context
		go func() {
			emitter(wrappedObserver)
		}()

		return NewBaseSubscription(NewBaseDisposable(cancel))
	})
}

// Defer 延迟创建Observable，直到有观察者订阅
func Defer(factory func() Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		// 每次订阅时都创建新的Observable
		observable := factory()
		return observable.Subscribe(observer)
	})
}

// ============================================================================
// 组合操作符
// ============================================================================

// Merge 合并多个Observable的发射
func Merge(observables ...Observable) Observable {
	if len(observables) == 0 {
		return Empty()
	}

	if len(observables) == 1 {
		return observables[0]
	}

	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		compositeDisposable := NewCompositeDisposable()

		var wg sync.WaitGroup
		completed := make(chan struct{})

		// 订阅所有Observable
		for _, obs := range observables {
			wg.Add(1)

			subscription := obs.Subscribe(func(item Item) {
				select {
				case <-ctx.Done():
					return
				default:
					if item.IsError() {
						observer(item)
						cancel()
						return
					}

					if item.Value == nil {
						// 某个Observable完成
						wg.Done()
						return
					}

					observer(item)
				}
			})

			compositeDisposable.Add(NewBaseDisposable(func() {
				subscription.Unsubscribe()
			}))
		}

		// 等待所有Observable完成
		go func() {
			wg.Wait()
			close(completed)
		}()

		// 监听完成信号
		go func() {
			select {
			case <-ctx.Done():
				return
			case <-completed:
				observer(CreateItem(nil)) // 发送完成信号
			}
		}()

		return NewBaseSubscription(NewBaseDisposable(func() {
			cancel()
			compositeDisposable.Dispose()
		}))
	})
}

// Concat 按顺序连接多个Observable的发射
func Concat(observables ...Observable) Observable {
	if len(observables) == 0 {
		return Empty()
	}

	if len(observables) == 1 {
		return observables[0]
	}

	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		compositeDisposable := NewCompositeDisposable()

		var subscribeNext func(index int)
		subscribeNext = func(index int) {
			if index >= len(observables) {
				observer(CreateItem(nil)) // 所有Observable都完成
				return
			}

			subscription := observables[index].Subscribe(func(item Item) {
				select {
				case <-ctx.Done():
					return
				default:
					if item.IsError() {
						observer(item)
						cancel()
						return
					}

					if item.Value == nil {
						// 当前Observable完成，订阅下一个
						go subscribeNext(index + 1)
						return
					}

					observer(item)
				}
			})

			compositeDisposable.Add(NewBaseDisposable(func() {
				subscription.Unsubscribe()
			}))
		}

		go subscribeNext(0)

		return NewBaseSubscription(NewBaseDisposable(func() {
			cancel()
			compositeDisposable.Dispose()
		}))
	})
}

// Zip 将多个Observable的发射组合成元组
func Zip(observables []Observable, zipper func(...interface{}) interface{}) Observable {
	if len(observables) == 0 {
		return Empty()
	}

	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		compositeDisposable := NewCompositeDisposable()

		// 为每个Observable创建缓冲区
		buffers := make([][]interface{}, len(observables))
		completed := make([]bool, len(observables))
		var mu sync.Mutex

		checkAndEmit := func() {
			mu.Lock()
			defer mu.Unlock()

			// 检查是否所有缓冲区都有数据
			minLength := -1
			for i, buffer := range buffers {
				if completed[i] && len(buffer) == 0 {
					// 某个Observable已完成且缓冲区为空
					observer(CreateItem(nil)) // 发送完成信号
					return
				}

				if len(buffer) > 0 {
					if minLength == -1 || len(buffer) < minLength {
						minLength = len(buffer)
					}
				} else {
					return // 还有空缓冲区，不能发射
				}
			}

			if minLength > 0 {
				// 从每个缓冲区取出第一个元素
				values := make([]interface{}, len(observables))
				for i := range buffers {
					values[i] = buffers[i][0]
					buffers[i] = buffers[i][1:] // 移除已使用的元素
				}

				// 使用zipper函数组合值
				result := zipper(values...)
				observer(CreateItem(result))
			}
		}

		// 订阅所有Observable
		for i, obs := range observables {
			i := i // 捕获循环变量
			subscription := obs.Subscribe(func(item Item) {
				select {
				case <-ctx.Done():
					return
				default:
					if item.IsError() {
						observer(item)
						cancel()
						return
					}

					mu.Lock()
					if item.Value == nil {
						completed[i] = true
					} else {
						buffers[i] = append(buffers[i], item.Value)
					}
					mu.Unlock()

					checkAndEmit()
				}
			})

			compositeDisposable.Add(NewBaseDisposable(func() {
				subscription.Unsubscribe()
			}))
		}

		return NewBaseSubscription(NewBaseDisposable(func() {
			cancel()
			compositeDisposable.Dispose()
		}))
	})
}

// CombineLatest 组合多个Observable的最新值
func CombineLatest(observables []Observable, combiner func(...interface{}) interface{}) Observable {
	if len(observables) == 0 {
		return Empty()
	}

	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		compositeDisposable := NewCompositeDisposable()

		// 存储每个Observable的最新值
		latestValues := make([]interface{}, len(observables))
		hasValue := make([]bool, len(observables))
		completed := make([]bool, len(observables))
		var mu sync.Mutex

		checkAndEmit := func() {
			mu.Lock()
			defer mu.Unlock()

			// 检查是否所有Observable都有值
			allHaveValue := true
			for i := range hasValue {
				if !hasValue[i] {
					allHaveValue = false
					break
				}
			}

			if allHaveValue {
				// 所有Observable都有值，可以发射
				values := make([]interface{}, len(latestValues))
				copy(values, latestValues)

				result := combiner(values...)
				observer(CreateItem(result))
			}

			// 检查是否所有Observable都已完成
			allCompleted := true
			for i := range completed {
				if !completed[i] {
					allCompleted = false
					break
				}
			}

			if allCompleted {
				observer(CreateItem(nil)) // 发送完成信号
			}
		}

		// 订阅所有Observable
		for i, obs := range observables {
			i := i // 捕获循环变量
			subscription := obs.Subscribe(func(item Item) {
				select {
				case <-ctx.Done():
					return
				default:
					if item.IsError() {
						observer(item)
						cancel()
						return
					}

					mu.Lock()
					if item.Value == nil {
						completed[i] = true
					} else {
						latestValues[i] = item.Value
						hasValue[i] = true
					}
					mu.Unlock()

					checkAndEmit()
				}
			})

			compositeDisposable.Add(NewBaseDisposable(func() {
				subscription.Unsubscribe()
			}))
		}

		return NewBaseSubscription(NewBaseDisposable(func() {
			cancel()
			compositeDisposable.Dispose()
		}))
	})
}

// ============================================================================
// 便利函数
// ============================================================================

// Of 是Just的别名，用于创建Observable
func Of(values ...interface{}) Observable {
	return Just(values...)
}

// FromValues 是Just的别名，用于创建Observable
func FromValues(values ...interface{}) Observable {
	return Just(values...)
}

// Start 在指定调度器上执行函数并发射结果
func Start(fn func() interface{}, scheduler Scheduler) Observable {
	return NewObservable(func(observer Observer) Subscription {
		disposable := scheduler.Schedule(func() {
			result := fn()
			observer(CreateItem(result))
			observer(CreateItem(nil)) // 完成信号
		})

		return NewBaseSubscription(disposable)
	})
}
