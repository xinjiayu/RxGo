// Combination operators for RxGo
// 组合操作符实现，包含Merge, Concat, Zip, CombineLatest等
package rxgo

import (
	"context"
	"sync"
	"sync/atomic"
)

// ============================================================================
// 组合操作符实现
// ============================================================================

// Merge 合并操作符，将多个Observable合并为一个
func (o *observableImpl) Merge(other Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		completed := int32(0)

		subscription1 := o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				// 完成信号
				if atomic.AddInt32(&completed, 1) == 2 {
					observer(item) // 两个Observable都完成了
				}
				return
			}

			observer(item)
		})

		subscription2 := other.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				// 完成信号
				if atomic.AddInt32(&completed, 1) == 2 {
					observer(item) // 两个Observable都完成了
				}
				return
			}

			observer(item)
		})

		return NewBaseSubscription(NewBaseDisposable(func() {
			subscription1.Unsubscribe()
			subscription2.Unsubscribe()
		}))
	})
}

// Concat 连接操作符，按顺序连接多个Observable
func (o *observableImpl) Concat(other Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		var subscription2 Subscription

		subscription1 := o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				// 第一个Observable完成，开始订阅第二个
				subscription2 = other.Subscribe(observer)
				return
			}

			observer(item)
		})

		return NewBaseSubscription(NewBaseDisposable(func() {
			subscription1.Unsubscribe()
			if subscription2 != nil {
				subscription2.Unsubscribe()
			}
		}))
	})
}

// Zip 压缩操作符，将两个Observable的对应项组合
func (o *observableImpl) Zip(other Observable, zipper func(interface{}, interface{}) interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		var queue1 []interface{}
		var queue2 []interface{}
		mu := &sync.Mutex{}
		completed1 := int32(0)
		completed2 := int32(0)

		processQueues := func() {
			mu.Lock()
			defer mu.Unlock()

			// 处理队列中的配对项
			for len(queue1) > 0 && len(queue2) > 0 {
				item1 := queue1[0]
				item2 := queue2[0]
				queue1 = queue1[1:]
				queue2 = queue2[1:]

				result := zipper(item1, item2)
				observer(CreateItem(result))
			}

			// 检查是否完成
			if atomic.LoadInt32(&completed1) == 1 || atomic.LoadInt32(&completed2) == 1 {
				observer(CreateItem(nil)) // 发送完成信号
			}
		}

		subscription1 := o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				atomic.StoreInt32(&completed1, 1)
				processQueues()
				return
			}

			mu.Lock()
			queue1 = append(queue1, item.Value)
			mu.Unlock()
			processQueues()
		})

		subscription2 := other.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				atomic.StoreInt32(&completed2, 1)
				processQueues()
				return
			}

			mu.Lock()
			queue2 = append(queue2, item.Value)
			mu.Unlock()
			processQueues()
		})

		return NewBaseSubscription(NewBaseDisposable(func() {
			subscription1.Unsubscribe()
			subscription2.Unsubscribe()
		}))
	})
}

// CombineLatest 组合最新值操作符，当任一Observable发射值时，与另一个的最新值组合
func (o *observableImpl) CombineLatest(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		var latest1 interface{}
		var latest2 interface{}
		hasValue1 := int32(0)
		hasValue2 := int32(0)
		mu := &sync.RWMutex{}
		completed1 := int32(0)
		completed2 := int32(0)

		emitCombined := func() {
			mu.RLock()
			defer mu.RUnlock()

			if atomic.LoadInt32(&hasValue1) == 1 && atomic.LoadInt32(&hasValue2) == 1 {
				result := combiner(latest1, latest2)
				observer(CreateItem(result))
			}
		}

		subscription1 := o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				atomic.StoreInt32(&completed1, 1)
				// 检查是否都完成
				if atomic.LoadInt32(&completed2) == 1 {
					observer(CreateItem(nil)) // 发送完成信号
				}
				return
			}

			mu.Lock()
			latest1 = item.Value
			atomic.StoreInt32(&hasValue1, 1)
			mu.Unlock()
			emitCombined()
		})

		subscription2 := other.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				atomic.StoreInt32(&completed2, 1)
				// 检查是否都完成
				if atomic.LoadInt32(&completed1) == 1 {
					observer(CreateItem(nil)) // 发送完成信号
				}
				return
			}

			mu.Lock()
			latest2 = item.Value
			atomic.StoreInt32(&hasValue2, 1)
			mu.Unlock()
			emitCombined()
		})

		return NewBaseSubscription(NewBaseDisposable(func() {
			subscription1.Unsubscribe()
			subscription2.Unsubscribe()
		}))
	})
}

// ============================================================================
// 静态组合操作符
// ============================================================================

// MergeObservables 合并多个Observable
func MergeObservables(observables ...Observable) Observable {
	if len(observables) == 0 {
		return Empty()
	}

	if len(observables) == 1 {
		return observables[0]
	}

	return NewObservable(func(observer Observer) Subscription {
		completed := int32(0)
		totalCount := int32(len(observables))
		subscriptions := make([]Subscription, len(observables))

		for i, obs := range observables {
			subscriptions[i] = obs.Subscribe(func(item Item) {
				if item.IsError() {
					observer(item)
					return
				}

				if item.Value == nil {
					// 完成信号
					if atomic.AddInt32(&completed, 1) == totalCount {
						observer(item) // 所有Observable都完成了
					}
					return
				}

				observer(item)
			})
		}

		return NewBaseSubscription(NewBaseDisposable(func() {
			for _, sub := range subscriptions {
				sub.Unsubscribe()
			}
		}))
	})
}

// ConcatObservables 连接多个Observable
func ConcatObservables(observables ...Observable) Observable {
	if len(observables) == 0 {
		return Empty()
	}

	if len(observables) == 1 {
		return observables[0]
	}

	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		currentIndex := int32(0)
		var currentSubscription Subscription

		var subscribeNext func()
		subscribeNext = func() {
			index := atomic.LoadInt32(&currentIndex)
			if int(index) >= len(observables) {
				observer(CreateItem(nil)) // 所有Observable都完成了
				cancel()
				return
			}

			currentSubscription = observables[index].Subscribe(func(item Item) {
				if item.IsError() {
					observer(item)
					cancel()
					return
				}

				if item.Value == nil {
					// 当前Observable完成，订阅下一个
					atomic.AddInt32(&currentIndex, 1)
					go func() {
						select {
						case <-ctx.Done():
							return
						default:
							subscribeNext()
						}
					}()
					return
				}

				observer(item)
			})
		}

		go subscribeNext()

		return NewBaseSubscription(NewBaseDisposable(func() {
			cancel()
			if currentSubscription != nil {
				currentSubscription.Unsubscribe()
			}
		}))
	})
}

// ============================================================================
// Phase 3 扩展 - Amb操作符系列
// ============================================================================

// Amb 竞速操作符，只订阅第一个发射数据的Observable，忽略其他的
func (o *observableImpl) Amb(other Observable) Observable {
	return AmbArray([]Observable{o, other})
}

// AmbArray 竞速操作符数组版本，只订阅第一个发射数据的Observable数组中的源
func AmbArray(observables []Observable) Observable {
	if len(observables) == 0 {
		return Empty()
	}
	if len(observables) == 1 {
		return observables[0]
	}

	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		var winner atomic.Int32 // 记录获胜者的索引，-1表示尚未确定
		winner.Store(-1)

		subscriptions := make([]Subscription, len(observables))

		// 为每个Observable创建订阅
		for i, obs := range observables {
			index := int32(i)
			subscriptions[i] = obs.Subscribe(func(item Item) {
				// 检查是否已经有获胜者
				if winner.Load() == -1 {
					// 尝试设置当前Observable为获胜者
					if winner.CompareAndSwap(-1, index) {
						// 成功设置为获胜者，取消其他订阅
						for j, sub := range subscriptions {
							if int32(j) != index && sub != nil {
								sub.Unsubscribe()
							}
						}
					}
				}

				// 只有获胜者才能发射数据
				if winner.Load() == index {
					select {
					case <-ctx.Done():
						return
					default:
						observer(item)
					}
				}
			})
		}

		return NewBaseSubscription(NewBaseDisposable(func() {
			cancel()
			for _, sub := range subscriptions {
				if sub != nil {
					sub.Unsubscribe()
				}
			}
		}))
	})
}

// MergeArray 合并多个Observable数组为一个，并行发射所有数据
func MergeArray(observables []Observable) Observable {
	if len(observables) == 0 {
		return Empty()
	}
	if len(observables) == 1 {
		return observables[0]
	}

	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		var activeCount int32 = int32(len(observables))
		subscriptions := make([]Subscription, len(observables))

		for i, obs := range observables {
			subscriptions[i] = obs.Subscribe(func(item Item) {
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
						// Observable完成
						if atomic.AddInt32(&activeCount, -1) == 0 {
							// 所有Observable都完成了
							observer(CreateItem(nil))
						}
						return
					}

					observer(item)
				}
			})
		}

		return NewBaseSubscription(NewBaseDisposable(func() {
			cancel()
			for _, sub := range subscriptions {
				if sub != nil {
					sub.Unsubscribe()
				}
			}
		}))
	})
}

// ConcatArray 顺序连接多个Observable数组，按顺序发射数据
func ConcatArray(observables []Observable) Observable {
	if len(observables) == 0 {
		return Empty()
	}
	if len(observables) == 1 {
		return observables[0]
	}

	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		var currentIndex int32 = 0
		var currentSubscription Subscription

		var subscribeNext func()
		subscribeNext = func() {
			index := atomic.LoadInt32(&currentIndex)
			if index >= int32(len(observables)) {
				observer(CreateItem(nil)) // 所有Observable都完成了
				cancel()
				return
			}

			currentSubscription = observables[index].Subscribe(func(item Item) {
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
						atomic.AddInt32(&currentIndex, 1)
						go func() {
							select {
							case <-ctx.Done():
								return
							default:
								subscribeNext()
							}
						}()
						return
					}

					observer(item)
				}
			})
		}

		go subscribeNext()

		return NewBaseSubscription(NewBaseDisposable(func() {
			cancel()
			if currentSubscription != nil {
				currentSubscription.Unsubscribe()
			}
		}))
	})
}
