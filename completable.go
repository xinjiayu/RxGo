// Completable implementation for RxGo
// 无返回值异步操作的响应式编程实现，专注于状态通知
package rxgo

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// Completable 核心实现
// ============================================================================

// completableImpl Completable的核心实现
type completableImpl struct {
	source     func(onComplete OnComplete, onError OnError) Subscription
	config     *Config
	mu         sync.RWMutex
	disposed   int32
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// NewCompletable 创建新的Completable
func NewCompletable(source func(onComplete OnComplete, onError OnError) Subscription, options ...Option) Completable {
	config := DefaultConfig()
	for _, opt := range options {
		opt.Apply(config)
	}

	ctx, cancel := context.WithCancel(config.Context)

	return &completableImpl{
		source:     source,
		config:     config,
		ctx:        ctx,
		cancelFunc: cancel,
	}
}

// ============================================================================
// 核心订阅方法
// ============================================================================

// Subscribe 订阅完成和错误回调
func (c *completableImpl) Subscribe(onComplete OnComplete, onError OnError) Subscription {
	if c.IsDisposed() {
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	}

	// 创建带上下文的回调包装器
	wrappedOnComplete := func() {
		select {
		case <-c.ctx.Done():
			return
		default:
			if onComplete != nil {
				onComplete()
			}
		}
	}

	wrappedOnError := func(err error) {
		select {
		case <-c.ctx.Done():
			return
		default:
			if onError != nil {
				onError(err)
			}
		}
	}

	subscription := c.source(wrappedOnComplete, wrappedOnError)

	// 将subscription添加到composite disposable中
	disposable := NewBaseDisposable(func() {
		subscription.Unsubscribe()
	})

	return NewBaseSubscription(disposable)
}

// SubscribeWithCallbacks 订阅（为了兼容性）
func (c *completableImpl) SubscribeWithCallbacks(onComplete OnComplete, onError OnError) Subscription {
	return c.Subscribe(onComplete, onError)
}

// SubscribeOn 指定订阅时使用的调度器
func (c *completableImpl) SubscribeOn(scheduler Scheduler) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		disposable := scheduler.Schedule(func() {
			c.Subscribe(onComplete, onError)
		})
		return NewBaseSubscription(disposable)
	})
}

// ObserveOn 指定观察时使用的调度器
func (c *completableImpl) ObserveOn(scheduler Scheduler) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		return c.Subscribe(func() {
			scheduler.Schedule(func() {
				if onComplete != nil {
					onComplete()
				}
			})
		}, func(err error) {
			scheduler.Schedule(func() {
				if onError != nil {
					onError(err)
				}
			})
		})
	})
}

// ============================================================================
// 组合操作符
// ============================================================================

// AndThen 完成后执行另一个Completable
func (c *completableImpl) AndThen(next Completable) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		subscription1 := c.Subscribe(func() {
			// 第一个Completable完成后，订阅第二个
			next.Subscribe(onComplete, onError)
		}, onError)

		return subscription1
	})
}

// AndThenObservable 完成后执行Observable
func (c *completableImpl) AndThenObservable(next Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		subscription1 := c.Subscribe(func() {
			// Completable完成后，订阅Observable
			next.Subscribe(observer)
		}, func(err error) {
			observer(CreateErrorItem(err))
		})

		return subscription1
	})
}

// AndThenSingle 完成后执行Single
func (c *completableImpl) AndThenSingle(next Single) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		subscription1 := c.Subscribe(func() {
			// Completable完成后，订阅Single
			next.Subscribe(onSuccess, onError)
		}, onError)

		return subscription1
	})
}

// AndThenMaybe 完成后执行Maybe
func (c *completableImpl) AndThenMaybe(next Maybe) Maybe {
	return NewMaybe(func(onSuccess func(interface{}), onError OnError, onComplete OnComplete) Subscription {
		subscription1 := c.Subscribe(func() {
			// Completable完成后，订阅Maybe
			next.Subscribe(onSuccess, onError, onComplete)
		}, onError)

		return subscription1
	})
}

// Concat 顺序连接多个Completable
func (c *completableImpl) Concat(other Completable) Completable {
	return c.AndThen(other)
}

// Merge 合并多个Completable
func (c *completableImpl) Merge(other Completable) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		_, cancel := context.WithCancel(context.Background())
		completed := int32(0)
		errored := int32(0)

		complete := func() {
			if atomic.AddInt32(&completed, 1) == 2 && atomic.LoadInt32(&errored) == 0 {
				cancel()
				if onComplete != nil {
					onComplete()
				}
			}
		}

		errorHandler := func(err error) {
			if atomic.CompareAndSwapInt32(&errored, 0, 1) {
				cancel()
				if onError != nil {
					onError(err)
				}
			}
		}

		subscription1 := c.Subscribe(complete, errorHandler)
		subscription2 := other.Subscribe(complete, errorHandler)

		return NewBaseSubscription(NewBaseDisposable(func() {
			cancel()
			subscription1.Unsubscribe()
			subscription2.Unsubscribe()
		}))
	})
}

// ============================================================================
// 错误处理
// ============================================================================

// Catch 捕获错误并返回新的Completable
func (c *completableImpl) Catch(handler func(error) Completable) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		return c.Subscribe(onComplete, func(err error) {
			// 发生错误时，执行错误处理器返回的Completable
			resumeCompletable := handler(err)
			resumeCompletable.Subscribe(onComplete, onError)
		})
	})
}

// Retry 重试指定次数
func (c *completableImpl) Retry(count int) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		attempts := int32(0)
		var subscription Subscription

		var attempt func()
		attempt = func() {
			subscription = c.Subscribe(onComplete, func(err error) {
				currentAttempt := atomic.AddInt32(&attempts, 1)
				if int(currentAttempt) < count {
					// 重试
					go attempt()
				} else {
					// 达到最大重试次数，发送错误
					if onError != nil {
						onError(err)
					}
				}
			})
		}

		go attempt()

		return NewBaseSubscription(NewBaseDisposable(func() {
			if subscription != nil {
				subscription.Unsubscribe()
			}
		}))
	})
}

// RetryWhen 条件重试
func (c *completableImpl) RetryWhen(handler func(error) bool) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		var subscription Subscription

		var attempt func()
		attempt = func() {
			subscription = c.Subscribe(onComplete, func(err error) {
				if handler(err) {
					// 条件满足，重试
					go attempt()
				} else {
					// 条件不满足，发送错误
					if onError != nil {
						onError(err)
					}
				}
			})
		}

		go attempt()

		return NewBaseSubscription(NewBaseDisposable(func() {
			if subscription != nil {
				subscription.Unsubscribe()
			}
		}))
	})
}

// OnErrorResumeNext 错误时恢复执行另一个Completable
func (c *completableImpl) OnErrorResumeNext(resumeWith Completable) Completable {
	return c.Catch(func(error) Completable {
		return resumeWith
	})
}

// OnErrorComplete 错误时作为完成处理
func (c *completableImpl) OnErrorComplete() Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		return c.Subscribe(onComplete, func(err error) {
			// 将错误转换为完成
			if onComplete != nil {
				onComplete()
			}
		})
	})
}

// ============================================================================
// 时间操作符
// ============================================================================

// Delay 延迟执行
func (c *completableImpl) Delay(duration time.Duration) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		return c.Subscribe(func() {
			timer := time.NewTimer(duration)
			go func() {
				<-timer.C
				if onComplete != nil {
					onComplete()
				}
			}()
		}, onError)
	})
}

// Timeout 超时处理
func (c *completableImpl) Timeout(duration time.Duration) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		ctx, cancel := context.WithTimeout(context.Background(), duration)
		completed := int32(0)

		subscription := c.Subscribe(func() {
			if atomic.CompareAndSwapInt32(&completed, 0, 1) {
				cancel()
				if onComplete != nil {
					onComplete()
				}
			}
		}, func(err error) {
			if atomic.CompareAndSwapInt32(&completed, 0, 1) {
				cancel()
				if onError != nil {
					onError(err)
				}
			}
		})

		go func() {
			<-ctx.Done()
			if ctx.Err() == context.DeadlineExceeded {
				if atomic.CompareAndSwapInt32(&completed, 0, 1) {
					if onError != nil {
						onError(NewTimeoutError("Completable timed out"))
					}
				}
			}
		}()

		return NewBaseSubscription(NewBaseDisposable(func() {
			cancel()
			subscription.Unsubscribe()
		}))
	})
}

// ============================================================================
// 副作用操作符
// ============================================================================

// DoOnComplete 完成时执行副作用
func (c *completableImpl) DoOnComplete(action OnComplete) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		return c.Subscribe(func() {
			if action != nil {
				action()
			}
			if onComplete != nil {
				onComplete()
			}
		}, onError)
	})
}

// DoOnError 错误时执行副作用
func (c *completableImpl) DoOnError(action OnError) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		return c.Subscribe(onComplete, func(err error) {
			if action != nil {
				action(err)
			}
			if onError != nil {
				onError(err)
			}
		})
	})
}

// DoOnSubscribe 订阅时执行副作用
func (c *completableImpl) DoOnSubscribe(action func()) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		if action != nil {
			action()
		}
		return c.Subscribe(onComplete, onError)
	})
}

// DoOnDispose 释放时执行副作用
func (c *completableImpl) DoOnDispose(action func()) Completable {
	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		subscription := c.Subscribe(onComplete, onError)

		return NewBaseSubscription(NewBaseDisposable(func() {
			if action != nil {
				action()
			}
			subscription.Unsubscribe()
		}))
	})
}

// ============================================================================
// 转换操作符
// ============================================================================

// ToObservable 转换为Observable（发射完成信号）
func (c *completableImpl) ToObservable() Observable {
	return NewObservable(func(observer Observer) Subscription {
		return c.Subscribe(func() {
			observer(CreateItem(nil)) // 发送完成信号
		}, func(err error) {
			observer(CreateErrorItem(err))
		})
	})
}

// ToSingle 转换为Single（提供默认值）
func (c *completableImpl) ToSingle(defaultValue interface{}) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		return c.Subscribe(func() {
			onSuccess(defaultValue)
		}, onError)
	})
}

// ToMaybe 转换为Maybe（空完成）
func (c *completableImpl) ToMaybe() Maybe {
	return NewMaybe(func(onSuccess func(interface{}), onError OnError, onComplete OnComplete) Subscription {
		return c.Subscribe(func() {
			if onComplete != nil {
				onComplete()
			}
		}, onError)
	})
}

// ============================================================================
// 阻塞操作
// ============================================================================

// BlockingAwait 阻塞等待完成
func (c *completableImpl) BlockingAwait() error {
	completeCh := make(chan struct{}, 1)
	errCh := make(chan error, 1)

	subscription := c.Subscribe(func() {
		completeCh <- struct{}{}
	}, func(err error) {
		errCh <- err
	})

	defer subscription.Unsubscribe()

	select {
	case <-completeCh:
		return nil
	case err := <-errCh:
		return err
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

// BlockingAwaitWithTimeout 阻塞等待完成（带超时）
func (c *completableImpl) BlockingAwaitWithTimeout(timeout time.Duration) error {
	completeCh := make(chan struct{}, 1)
	errCh := make(chan error, 1)

	subscription := c.Subscribe(func() {
		completeCh <- struct{}{}
	}, func(err error) {
		errCh <- err
	})

	defer subscription.Unsubscribe()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-completeCh:
		return nil
	case err := <-errCh:
		return err
	case <-timer.C:
		return NewTimeoutError("Completable await timed out")
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

// ============================================================================
// 生命周期管理
// ============================================================================

// IsDisposed 检查是否已释放
func (c *completableImpl) IsDisposed() bool {
	return atomic.LoadInt32(&c.disposed) == 1
}

// Dispose 释放资源
func (c *completableImpl) Dispose() {
	if atomic.CompareAndSwapInt32(&c.disposed, 0, 1) {
		if c.cancelFunc != nil {
			c.cancelFunc()
		}
	}
}
