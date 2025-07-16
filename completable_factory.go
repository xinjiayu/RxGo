// Completable factory functions for RxGo
// Completable工厂函数，提供各种创建Completable的便捷方法
package rxgo

import (
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// 基础工厂函数
// ============================================================================

// CompletableComplete 创建立即完成的Completable
func CompletableComplete() Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		go func() {
			if onComplete != nil {
				onComplete()
			}
		}()
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// CompletableError 创建立即发射错误的Completable
func CompletableError(err error) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		go func() {
			if onError != nil {
				onError(err)
			}
		}()
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// CompletableCreate 使用自定义发射器创建Completable
func CompletableCreate(emitter func(onComplete OnComplete, onError OnError)) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		go func() {
			emitter(onComplete, onError)
		}()
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// CompletableNever 创建永不完成的Completable
func CompletableNever() Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		// 什么都不做，永远不完成
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// ============================================================================
// 从异步操作创建
// ============================================================================

// CompletableFromAction 从无返回值的异步操作创建Completable
func CompletableFromAction(action func() error) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		go func() {
			if err := action(); err != nil {
				if onError != nil {
					onError(err)
				}
			} else {
				if onComplete != nil {
					onComplete()
				}
			}
		}()
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// CompletableFromFunc 从返回error的函数创建Completable
func CompletableFromFunc(fn func() error) Completable {
	return CompletableFromAction(fn)
}

// CompletableFromRunnable 从Runnable函数创建Completable（无错误返回）
func CompletableFromRunnable(runnable func()) Completable {
	return CompletableFromAction(func() error {
		runnable()
		return nil
	})
}

// ============================================================================
// 从其他类型转换
// ============================================================================

// CompletableFromObservable 从Observable创建Completable（忽略所有值，只关注完成/错误）
func CompletableFromObservable(observable Observable) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		return observable.Subscribe(func(item Item) {
			if item.IsError() {
				if onError != nil {
					onError(item.Error)
				}
				return
			}

			if item.Value == nil {
				// 完成信号
				if onComplete != nil {
					onComplete()
				}
			}
			// 忽略所有正常值
		})
	})
}

// CompletableFromSingle 从Single创建Completable（忽略值）
func CompletableFromSingle(single Single) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		return single.Subscribe(func(value interface{}) {
			// 忽略值，只触发完成
			if onComplete != nil {
				onComplete()
			}
		}, onError)
	})
}

// CompletableFromMaybe 从Maybe创建Completable（忽略值）
func CompletableFromMaybe(maybe Maybe) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		return maybe.Subscribe(func(value interface{}) {
			// 忽略值，只触发完成
			if onComplete != nil {
				onComplete()
			}
		}, onError, func() {
			// Maybe完成但无值
			if onComplete != nil {
				onComplete()
			}
		})
	})
}

// ============================================================================
// 时间相关工厂函数
// ============================================================================

// CompletableTimer 创建延迟指定时间后完成的Completable
func CompletableTimer(delay time.Duration) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		timer := time.NewTimer(delay)
		go func() {
			<-timer.C
			if onComplete != nil {
				onComplete()
			}
		}()

		return NewBaseSubscription(NewBaseDisposable(func() {
			timer.Stop()
		}))
	})
}

// CompletableInterval 创建定期完成的Completable（每次间隔后完成一次）
func CompletableInterval(interval time.Duration) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		ticker := time.NewTicker(interval)

		go func() {
			defer ticker.Stop()
			for range ticker.C {
				if onComplete != nil {
					onComplete()
				}
				// 注意：这个会不断完成，实际使用中可能需要限制次数
			}
		}()

		return NewBaseSubscription(NewBaseDisposable(func() {
			ticker.Stop()
		}))
	})
}

// ============================================================================
// 组合工厂函数
// ============================================================================

// CompletableMerge 合并多个Completable，当所有都完成时才完成
func CompletableMerge(completables ...Completable) Completable {
	if len(completables) == 0 {
		return CompletableComplete()
	}

	if len(completables) == 1 {
		return completables[0]
	}

	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		var wg sync.WaitGroup
		compositeDisposable := NewCompositeDisposable()

		wg.Add(len(completables))

		for _, completable := range completables {
			subscription := completable.Subscribe(func() {
				wg.Done()
			}, func(err error) {
				if onError != nil {
					onError(err)
				}
			})

			compositeDisposable.Add(NewBaseDisposable(func() {
				subscription.Unsubscribe()
			}))
		}

		go func() {
			wg.Wait()
			if onComplete != nil {
				onComplete()
			}
		}()

		return NewBaseSubscription(NewBaseDisposable(func() {
			compositeDisposable.Dispose()
		}))
	})
}

// CompletableConcat 按顺序连接多个Completable
func CompletableConcat(completables ...Completable) Completable {
	if len(completables) == 0 {
		return CompletableComplete()
	}

	if len(completables) == 1 {
		return completables[0]
	}

	result := completables[0]
	for i := 1; i < len(completables); i++ {
		result = result.AndThen(completables[i])
	}

	return result
}

// CompletableAmb 返回第一个完成的Completable的结果
func CompletableAmb(completables ...Completable) Completable {
	if len(completables) == 0 {
		return CompletableNever()
	}

	if len(completables) == 1 {
		return completables[0]
	}

	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		compositeDisposable := NewCompositeDisposable()
		firstCompleted := int32(0)

		for _, completable := range completables {
			subscription := completable.Subscribe(func() {
				// 第一个完成的获胜
				if atomic.CompareAndSwapInt32(&firstCompleted, 0, 1) {
					if onComplete != nil {
						onComplete()
					}
					compositeDisposable.Dispose()
				}
			}, func(err error) {
				// 第一个错误的获胜
				if atomic.CompareAndSwapInt32(&firstCompleted, 0, 1) {
					if onError != nil {
						onError(err)
					}
					compositeDisposable.Dispose()
				}
			})

			compositeDisposable.Add(NewBaseDisposable(func() {
				subscription.Unsubscribe()
			}))
		}

		return NewBaseSubscription(NewBaseDisposable(func() {
			compositeDisposable.Dispose()
		}))
	})
}

// ============================================================================
// 条件工厂函数
// ============================================================================

// CompletableDefer 延迟创建Completable，每次订阅时调用工厂函数
func CompletableDefer(factory func() Completable) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		// 每次订阅时都创建新的Completable
		completable := factory()
		return completable.Subscribe(onComplete, onError)
	})
}

// CompletableUsing 使用资源创建Completable，自动管理资源生命周期
func CompletableUsing(
	resourceFactory func() interface{},
	completableFactory func(interface{}) Completable,
	disposer func(interface{}),
) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		resource := resourceFactory()

		completable := completableFactory(resource)
		subscription := completable.Subscribe(onComplete, onError)

		return NewBaseSubscription(NewBaseDisposable(func() {
			subscription.Unsubscribe()
			if disposer != nil {
				disposer(resource)
			}
		}))
	})
}

// ============================================================================
// 工具函数
// ============================================================================

// CompletableFromChannel 从channel的关闭事件创建Completable
func CompletableFromChannel(ch <-chan struct{}) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		go func() {
			<-ch // 等待channel关闭
			if onComplete != nil {
				onComplete()
			}
		}()
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// CompletableFromCallback 从回调函数创建Completable
func CompletableFromCallback(subscribe func(onComplete func(), onError func(error))) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		go func() {
			subscribe(
				func() {
					if onComplete != nil {
						onComplete()
					}
				},
				func(err error) {
					if onError != nil {
						onError(err)
					}
				},
			)
		}()
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}
