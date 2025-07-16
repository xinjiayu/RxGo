// Observable implementation for RxGo
// 基于Go channels和goroutines的Observable核心实现
package rxgo

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// Observable 核心实现
// ============================================================================

// observableImpl Observable的核心实现
type observableImpl struct {
	source     func(observer Observer) Subscription
	config     *Config
	mu         sync.RWMutex
	disposed   int32
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// NewObservable 创建新的Observable
func NewObservable(source func(observer Observer) Subscription, options ...Option) Observable {
	config := DefaultConfig()
	for _, opt := range options {
		opt.Apply(config)
	}

	ctx, cancel := context.WithCancel(config.Context)

	return &observableImpl{
		source:     source,
		config:     config,
		ctx:        ctx,
		cancelFunc: cancel,
	}
}

// Subscribe 订阅观察者
func (o *observableImpl) Subscribe(observer Observer) Subscription {
	if o.IsDisposed() {
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	}

	// 创建带上下文的观察者包装器
	wrappedObserver := func(item Item) {
		select {
		case <-o.ctx.Done():
			return
		default:
			observer(item)
		}
	}

	subscription := o.source(wrappedObserver)

	// 将subscription添加到composite disposable中
	disposable := NewBaseDisposable(func() {
		subscription.Unsubscribe()
	})

	return NewBaseSubscription(disposable)
}

// SubscribeWithCallbacks 使用回调函数订阅
func (o *observableImpl) SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) Subscription {
	observer := func(item Item) {
		if item.IsError() {
			if onError != nil {
				onError(item.Error)
			}
		} else if item.Value == nil {
			if onComplete != nil {
				onComplete()
			}
		} else {
			if onNext != nil {
				onNext(item.Value)
			}
		}
	}
	return o.Subscribe(observer)
}

// SubscribeOn 指定订阅时使用的调度器
func (o *observableImpl) SubscribeOn(scheduler Scheduler) Observable {
	return NewObservable(func(observer Observer) Subscription {
		disposable := scheduler.Schedule(func() {
			o.Subscribe(observer)
		})
		return NewBaseSubscription(disposable)
	})
}

// ObserveOn 指定观察时使用的调度器
func (o *observableImpl) ObserveOn(scheduler Scheduler) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			scheduler.Schedule(func() {
				observer(item)
			})
		})
	})
}

// IsDisposed 检查是否已释放
func (o *observableImpl) IsDisposed() bool {
	return atomic.LoadInt32(&o.disposed) == 1
}

// Dispose 释放资源
func (o *observableImpl) Dispose() {
	if atomic.CompareAndSwapInt32(&o.disposed, 0, 1) {
		if o.cancelFunc != nil {
			o.cancelFunc()
		}
	}
}

// ============================================================================
// 转换操作符
// ============================================================================

// Map 转换操作符
func (o *observableImpl) Map(transformer Transformer) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				observer(item) // 完成信号
				return
			}

			if result, err := transformer(item.Value); err != nil {
				observer(CreateErrorItem(err))
			} else {
				observer(CreateItem(result))
			}
		})
	})
}

// Filter 过滤操作符
func (o *observableImpl) Filter(predicate Predicate) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() || item.Value == nil {
				observer(item)
				return
			}

			if predicate(item.Value) {
				observer(item)
			}
		})
	})
}

// Take 取前N个元素
func (o *observableImpl) Take(count int) Observable {
	return NewObservable(func(observer Observer) Subscription {
		taken := int32(0)
		return o.Subscribe(func(item Item) {
			if atomic.LoadInt32(&taken) >= int32(count) {
				return
			}

			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				observer(item) // 完成信号
				return
			}

			if atomic.AddInt32(&taken, 1) <= int32(count) {
				observer(item)

				if atomic.LoadInt32(&taken) >= int32(count) {
					observer(CreateItem(nil)) // 发送完成信号
				}
			}
		})
	})
}

// ToChannel 转换为Go channel
func (o *observableImpl) ToChannel() <-chan Item {
	ch := make(chan Item, o.config.BufferSize)

	go func() {
		defer close(ch)

		o.Subscribe(func(item Item) {
			select {
			case ch <- item:
			case <-o.ctx.Done():
				return
			}
		})
	}()

	return ch
}

// Publish 发布为ConnectableObservable
func (o *observableImpl) Publish() ConnectableObservable {
	return NewConnectableObservable(o)
}

// Share 共享Observable
func (o *observableImpl) Share() Observable {
	return o.Publish().RefCount()
}

// FlatMap 将每个源项转换为Observable，然后将所有Observable合并为一个
func (o *observableImpl) FlatMap(transformer func(interface{}) Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		subscription := NewBaseSubscription(NewBaseDisposable(cancel))

		go func() {
			defer cancel()

			// 用于跟踪所有内部订阅
			var innerSubscriptions []Subscription
			var mu sync.Mutex
			completedCount := 0
			sourceCompleted := false

			// 源Observable的观察者
			sourceObserver := func(item Item) {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if item.IsError() {
					observer(item)
					return
				}

				if item.Value == nil {
					// 源Observable完成
					mu.Lock()
					sourceCompleted = true
					if completedCount == len(innerSubscriptions) {
						// 所有内部Observable都已完成
						observer(CreateItem(nil))
					}
					mu.Unlock()
					return
				}

				// 转换项为Observable
				innerObservable := transformer(item.Value)

				// 订阅内部Observable
				innerSub := innerObservable.Subscribe(func(innerItem Item) {
					select {
					case <-ctx.Done():
						return
					default:
					}

					if innerItem.IsError() {
						observer(innerItem)
						return
					}

					if innerItem.Value == nil {
						// 内部Observable完成
						mu.Lock()
						completedCount++
						if sourceCompleted && completedCount == len(innerSubscriptions) {
							observer(CreateItem(nil))
						}
						mu.Unlock()
						return
					}

					// 转发值
					observer(innerItem)
				})

				mu.Lock()
				innerSubscriptions = append(innerSubscriptions, innerSub)
				mu.Unlock()
			}

			// 订阅源Observable
			sourceSub := o.Subscribe(sourceObserver)

			// 等待取消
			<-ctx.Done()

			// 取消所有订阅
			sourceSub.Unsubscribe()
			mu.Lock()
			for _, sub := range innerSubscriptions {
				sub.Unsubscribe()
			}
			mu.Unlock()
		}()

		return subscription
	})
}

// Skip 跳过源Observable的前N个项
func (o *observableImpl) Skip(count int) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		subscription := NewBaseSubscription(NewBaseDisposable(cancel))

		go func() {
			defer cancel()

			skipped := 0
			var mu sync.Mutex

			sourceObserver := func(item Item) {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if item.IsError() {
					observer(item)
					return
				}

				if item.Value == nil {
					// 源Observable完成
					observer(CreateItem(nil))
					return
				}

				mu.Lock()
				if skipped < count {
					skipped++
					mu.Unlock()
					// 还需要跳过，不发出这个项
					return
				}
				mu.Unlock()

				// 已经跳过了足够的项，发出这个项
				observer(item)
			}

			// 订阅源Observable
			sourceSub := o.Subscribe(sourceObserver)

			// 等待取消
			<-ctx.Done()

			// 取消源订阅
			sourceSub.Unsubscribe()
		}()

		return subscription
	})
}

// TakeLast 只取源Observable的最后N个项
func (o *observableImpl) TakeLast(count int) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		subscription := NewBaseSubscription(NewBaseDisposable(cancel))

		go func() {
			defer cancel()

			buffer := make([]interface{}, 0, count)
			var mu sync.Mutex

			sourceObserver := func(item Item) {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if item.IsError() {
					observer(item)
					return
				}

				if item.Value == nil {
					// 源Observable完成，发出缓冲区中的项
					mu.Lock()
					for _, value := range buffer {
						observer(CreateItem(value))
					}
					mu.Unlock()
					observer(CreateItem(nil))
					return
				}

				mu.Lock()
				if len(buffer) < count {
					buffer = append(buffer, item.Value)
				} else {
					// 缓冲区已满，移除第一个元素，添加新元素
					buffer = append(buffer[1:], item.Value)
				}
				mu.Unlock()
			}

			// 订阅源Observable
			sourceSub := o.Subscribe(sourceObserver)

			// 等待取消
			<-ctx.Done()

			// 取消源订阅
			sourceSub.Unsubscribe()
		}()

		return subscription
	})
}

// TakeUntil 取源Observable的项直到另一个Observable发出值
func (o *observableImpl) TakeUntil(other Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		subscription := NewBaseSubscription(NewBaseDisposable(cancel))

		go func() {
			defer cancel()

			var stopped int32

			// 订阅触发Observable
			otherSub := other.Subscribe(func(item Item) {
				if atomic.CompareAndSwapInt32(&stopped, 0, 1) {
					// 其他Observable发出了值，停止主Observable
					observer(CreateItem(nil))
				}
			})

			sourceObserver := func(item Item) {
				if atomic.LoadInt32(&stopped) == 1 {
					return
				}

				select {
				case <-ctx.Done():
					return
				default:
				}

				if item.IsError() {
					observer(item)
					return
				}

				if item.Value == nil {
					// 源Observable完成
					observer(CreateItem(nil))
					return
				}

				// 发出项
				observer(item)
			}

			// 订阅源Observable
			sourceSub := o.Subscribe(sourceObserver)

			// 等待取消
			<-ctx.Done()

			// 取消所有订阅
			sourceSub.Unsubscribe()
			otherSub.Unsubscribe()
		}()

		return subscription
	})
}

// TakeWhile 取源Observable的项直到谓词返回false
func (o *observableImpl) TakeWhile(predicate Predicate) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		subscription := NewBaseSubscription(NewBaseDisposable(cancel))

		go func() {
			defer cancel()

			sourceObserver := func(item Item) {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if item.IsError() {
					observer(item)
					return
				}

				if item.Value == nil {
					// 源Observable完成
					observer(CreateItem(nil))
					return
				}

				// 检查谓词
				if predicate(item.Value) {
					// 谓词为true，发出项
					observer(item)
				} else {
					// 谓词为false，停止并完成
					observer(CreateItem(nil))
				}
			}

			// 订阅源Observable
			sourceSub := o.Subscribe(sourceObserver)

			// 等待取消
			<-ctx.Done()

			// 取消源订阅
			sourceSub.Unsubscribe()
		}()

		return subscription
	})
}

// SkipLast 跳过源Observable的最后N个项
func (o *observableImpl) SkipLast(count int) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		subscription := NewBaseSubscription(NewBaseDisposable(cancel))

		go func() {
			defer cancel()

			buffer := make([]interface{}, 0, count)
			var mu sync.Mutex

			sourceObserver := func(item Item) {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if item.IsError() {
					observer(item)
					return
				}

				if item.Value == nil {
					// 源Observable完成，不发出缓冲区中的项
					observer(CreateItem(nil))
					return
				}

				mu.Lock()
				if len(buffer) < count {
					buffer = append(buffer, item.Value)
				} else {
					// 缓冲区已满，发出第一个元素，添加新元素
					observer(CreateItem(buffer[0]))
					buffer = append(buffer[1:], item.Value)
				}
				mu.Unlock()
			}

			// 订阅源Observable
			sourceSub := o.Subscribe(sourceObserver)

			// 等待取消
			<-ctx.Done()

			// 取消源订阅
			sourceSub.Unsubscribe()
		}()

		return subscription
	})
}

// SkipUntil 跳过源Observable的项直到另一个Observable发出值
func (o *observableImpl) SkipUntil(other Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		subscription := NewBaseSubscription(NewBaseDisposable(cancel))

		go func() {
			defer cancel()

			var started int32

			// 订阅触发Observable
			otherSub := other.Subscribe(func(item Item) {
				atomic.StoreInt32(&started, 1)
			})

			sourceObserver := func(item Item) {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if item.IsError() {
					observer(item)
					return
				}

				if item.Value == nil {
					// 源Observable完成
					observer(CreateItem(nil))
					return
				}

				// 只有在其他Observable发出值后才开始发出项
				if atomic.LoadInt32(&started) == 1 {
					observer(item)
				}
			}

			// 订阅源Observable
			sourceSub := o.Subscribe(sourceObserver)

			// 等待取消
			<-ctx.Done()

			// 取消所有订阅
			sourceSub.Unsubscribe()
			otherSub.Unsubscribe()
		}()

		return subscription
	})
}

// SkipWhile 跳过源Observable的项直到谓词返回false
func (o *observableImpl) SkipWhile(predicate Predicate) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		subscription := NewBaseSubscription(NewBaseDisposable(cancel))

		go func() {
			defer cancel()

			var started int32

			sourceObserver := func(item Item) {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if item.IsError() {
					observer(item)
					return
				}

				if item.Value == nil {
					// 源Observable完成
					observer(CreateItem(nil))
					return
				}

				// 如果还没有开始，检查谓词
				if atomic.LoadInt32(&started) == 0 {
					if predicate(item.Value) {
						// 谓词为true，继续跳过
						return
					} else {
						// 谓词为false，开始发出项
						atomic.StoreInt32(&started, 1)
					}
				}

				// 发出项
				observer(item)
			}

			// 订阅源Observable
			sourceSub := o.Subscribe(sourceObserver)

			// 等待取消
			<-ctx.Done()

			// 取消源订阅
			sourceSub.Unsubscribe()
		}()

		return subscription
	})
}

// Distinct 过滤掉重复的项，只发出唯一的项
func (o *observableImpl) Distinct() Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		subscription := NewBaseSubscription(NewBaseDisposable(cancel))

		go func() {
			defer cancel()

			// 使用map来跟踪已经见过的值
			seen := make(map[interface{}]bool)
			var mu sync.Mutex

			sourceObserver := func(item Item) {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if item.IsError() {
					observer(item)
					return
				}

				if item.Value == nil {
					// 源Observable完成
					observer(CreateItem(nil))
					return
				}

				mu.Lock()
				if !seen[item.Value] {
					seen[item.Value] = true
					mu.Unlock()
					// 第一次见到这个值，发出它
					observer(item)
				} else {
					mu.Unlock()
					// 已经见过这个值，跳过
				}
			}

			// 订阅源Observable
			sourceSub := o.Subscribe(sourceObserver)

			// 等待取消
			<-ctx.Done()

			// 取消源订阅
			sourceSub.Unsubscribe()
		}()

		return subscription
	})
}

// DistinctUntilChanged 过滤掉连续重复的项
func (o *observableImpl) DistinctUntilChanged() Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		subscription := NewBaseSubscription(NewBaseDisposable(cancel))

		go func() {
			defer cancel()

			var lastValue interface{}
			var hasLastValue bool
			var mu sync.Mutex

			sourceObserver := func(item Item) {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if item.IsError() {
					observer(item)
					return
				}

				if item.Value == nil {
					// 源Observable完成
					observer(CreateItem(nil))
					return
				}

				mu.Lock()
				if !hasLastValue || lastValue != item.Value {
					lastValue = item.Value
					hasLastValue = true
					mu.Unlock()
					// 值发生了变化，发出它
					observer(item)
				} else {
					mu.Unlock()
					// 值没有变化，跳过
				}
			}

			// 订阅源Observable
			sourceSub := o.Subscribe(sourceObserver)

			// 等待取消
			<-ctx.Done()

			// 取消源订阅
			sourceSub.Unsubscribe()
		}()

		return subscription
	})
}

// Buffer 将源Observable的项收集到缓冲区中，每当缓冲区满时发出缓冲区
func (o *observableImpl) Buffer(count int) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		subscription := NewBaseSubscription(NewBaseDisposable(cancel))

		go func() {
			defer cancel()

			buffer := make([]interface{}, 0, count)
			var mu sync.Mutex

			sourceObserver := func(item Item) {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if item.IsError() {
					observer(item)
					return
				}

				if item.Value == nil {
					// 源Observable完成，发出剩余的缓冲区
					mu.Lock()
					if len(buffer) > 0 {
						observer(CreateItem(buffer))
					}
					mu.Unlock()
					observer(CreateItem(nil))
					return
				}

				mu.Lock()
				buffer = append(buffer, item.Value)
				if len(buffer) == count {
					// 缓冲区已满，发出缓冲区
					observer(CreateItem(buffer))
					buffer = make([]interface{}, 0, count)
				}
				mu.Unlock()
			}

			// 订阅源Observable
			sourceSub := o.Subscribe(sourceObserver)

			// 等待取消
			<-ctx.Done()

			// 取消源订阅
			sourceSub.Unsubscribe()
		}()

		return subscription
	})
}

// BufferWithTime 将源Observable的项收集到缓冲区中，每当指定时间间隔过去时发出缓冲区
func (o *observableImpl) BufferWithTime(timespan time.Duration) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		subscription := NewBaseSubscription(NewBaseDisposable(cancel))

		go func() {
			defer cancel()

			buffer := make([]interface{}, 0)
			var mu sync.Mutex

			// 启动定时器
			ticker := time.NewTicker(timespan)
			defer ticker.Stop()

			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						mu.Lock()
						if len(buffer) > 0 {
							observer(CreateItem(buffer))
							buffer = make([]interface{}, 0)
						}
						mu.Unlock()
					}
				}
			}()

			sourceObserver := func(item Item) {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if item.IsError() {
					observer(item)
					return
				}

				if item.Value == nil {
					// 源Observable完成，发出剩余的缓冲区
					mu.Lock()
					if len(buffer) > 0 {
						observer(CreateItem(buffer))
					}
					mu.Unlock()
					observer(CreateItem(nil))
					return
				}

				mu.Lock()
				buffer = append(buffer, item.Value)
				mu.Unlock()
			}

			// 订阅源Observable
			sourceSub := o.Subscribe(sourceObserver)

			// 等待取消
			<-ctx.Done()

			// 取消源订阅
			sourceSub.Unsubscribe()
		}()

		return subscription
	})
}

// Window 将源Observable的项分组到窗口中，每当窗口满时发出窗口Observable
func (o *observableImpl) Window(count int) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		subscription := NewBaseSubscription(NewBaseDisposable(cancel))

		go func() {
			defer cancel()

			var currentSubject *PublishSubject
			itemCount := 0
			var mu sync.Mutex

			sourceObserver := func(item Item) {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if item.IsError() {
					mu.Lock()
					if currentSubject != nil {
						currentSubject.OnError(item.Error)
					}
					mu.Unlock()
					observer(item)
					return
				}

				if item.Value == nil {
					// 源Observable完成
					mu.Lock()
					if currentSubject != nil {
						currentSubject.OnComplete()
					}
					mu.Unlock()
					observer(CreateItem(nil))
					return
				}

				mu.Lock()
				if currentSubject == nil || itemCount == 0 {
					// 创建新的窗口
					currentSubject = NewPublishSubject()
					observer(CreateItem(currentSubject))
					itemCount = count
				}

				currentSubject.OnNext(item.Value)
				itemCount--

				if itemCount == 0 {
					// 窗口已满，完成当前窗口
					currentSubject.OnComplete()
					currentSubject = nil
				}
				mu.Unlock()
			}

			// 订阅源Observable
			sourceSub := o.Subscribe(sourceObserver)

			// 等待取消
			<-ctx.Done()

			// 取消源订阅
			sourceSub.Unsubscribe()
		}()

		return subscription
	})
}

// WindowWithTime 将源Observable的项分组到时间窗口中
func (o *observableImpl) WindowWithTime(timespan time.Duration) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		subscription := NewBaseSubscription(NewBaseDisposable(cancel))

		go func() {
			defer cancel()

			var currentSubject *PublishSubject
			var mu sync.Mutex

			// 启动定时器
			ticker := time.NewTicker(timespan)
			defer ticker.Stop()

			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						mu.Lock()
						if currentSubject != nil {
							currentSubject.OnComplete()
						}
						currentSubject = NewPublishSubject()
						observer(CreateItem(currentSubject))
						mu.Unlock()
					}
				}
			}()

			sourceObserver := func(item Item) {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if item.IsError() {
					mu.Lock()
					if currentSubject != nil {
						currentSubject.OnError(item.Error)
					}
					mu.Unlock()
					observer(item)
					return
				}

				if item.Value == nil {
					// 源Observable完成
					mu.Lock()
					if currentSubject != nil {
						currentSubject.OnComplete()
					}
					mu.Unlock()
					observer(CreateItem(nil))
					return
				}

				mu.Lock()
				if currentSubject == nil {
					// 创建第一个窗口
					currentSubject = NewPublishSubject()
					observer(CreateItem(currentSubject))
				}

				currentSubject.OnNext(item.Value)
				mu.Unlock()
			}

			// 订阅源Observable
			sourceSub := o.Subscribe(sourceObserver)

			// 等待取消
			<-ctx.Done()

			// 取消源订阅
			sourceSub.Unsubscribe()
		}()

		return subscription
	})
}

// Reduce 对源Observable的所有项应用累积函数，并发出最终结果
func (o *observableImpl) Reduce(reducer Reducer) Single {
	// 简化实现，直接返回nil，稍后会实现完整的Single
	return nil
}

// Scan 对源Observable的每个项应用累积函数，并发出每个中间结果
func (o *observableImpl) Scan(reducer Reducer) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		subscription := NewBaseSubscription(NewBaseDisposable(cancel))

		go func() {
			defer cancel()

			var accumulator interface{}
			isFirst := true

			sourceObserver := func(item Item) {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if item.IsError() {
					observer(item)
					return
				}

				if item.Value == nil {
					// 源Observable完成
					observer(CreateItem(nil))
					return
				}

				if isFirst {
					// 第一个值直接作为累积器
					accumulator = item.Value
					isFirst = false
				} else {
					// 应用reducer函数
					accumulator = reducer(accumulator, item.Value)
				}

				// 发出累积结果
				observer(CreateItem(accumulator))
			}

			// 订阅源Observable
			sourceSub := o.Subscribe(sourceObserver)

			// 等待取消
			<-ctx.Done()

			// 取消源订阅
			sourceSub.Unsubscribe()
		}()

		return subscription
	})
}
