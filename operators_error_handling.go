// Error handling operators for RxGo
// 错误处理操作符实现，包含Catch, Retry, RetryWhen等
package rxgo

import (
	"context"
	"sync/atomic"
	"time"
)

// ============================================================================
// 错误处理操作符实现
// ============================================================================

// Catch 错误捕获操作符，当发生错误时切换到另一个Observable
func (o *observableImpl) Catch(handler func(error) Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				// 发生错误，切换到恢复Observable
				recoveryObservable := handler(item.Error)
				recoveryObservable.Subscribe(observer)
				return
			}

			// 正常值或完成信号
			observer(item)
		})
	})
}

// Retry 重试操作符，发生错误时重试指定次数
func (o *observableImpl) Retry(count int) Observable {
	return NewObservable(func(observer Observer) Subscription {
		attempts := int32(0)
		ctx, cancel := context.WithCancel(context.Background())

		var subscribe func()
		subscribe = func() {
			currentAttempt := atomic.AddInt32(&attempts, 1)

			subscription := o.Subscribe(func(item Item) {
				if item.IsError() {
					if int(currentAttempt) < count {
						// 重试
						go func() {
							select {
							case <-ctx.Done():
								return
							default:
								subscribe()
							}
						}()
					} else {
						// 达到最大重试次数，发送错误
						observer(item)
						cancel()
					}
					return
				}

				// 正常值或完成信号
				observer(item)
				if item.Value == nil {
					// 完成信号
					cancel()
				}
			})

			// 如果上下文被取消，取消订阅
			go func() {
				<-ctx.Done()
				subscription.Unsubscribe()
			}()
		}

		go subscribe()

		return NewBaseSubscription(NewBaseDisposable(func() {
			cancel()
		}))
	})
}

// RetryWhen 条件重试操作符，根据错误Observable决定是否重试
func (o *observableImpl) RetryWhen(handler func(Observable) Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		errorSubject := NewPublishSubject()

		// 创建重试控制Observable
		retryObservable := handler(errorSubject)

		var subscribe func()
		subscribe = func() {
			subscription := o.Subscribe(func(item Item) {
				if item.IsError() {
					// 发送错误到错误Subject
					errorSubject.OnNext(item.Error)
					return
				}

				// 正常值或完成信号
				observer(item)
				if item.Value == nil {
					// 完成信号
					cancel()
				}
			})

			// 如果上下文被取消，取消订阅
			go func() {
				<-ctx.Done()
				subscription.Unsubscribe()
			}()
		}

		// 订阅重试控制Observable
		retryObservable.Subscribe(func(item Item) {
			if item.IsError() {
				// 重试控制Observable发生错误，停止重试
				observer(item)
				cancel()
				return
			}

			if item.Value == nil {
				// 重试控制Observable完成，停止重试
				observer(CreateErrorItem(NewRetryExhaustedError("Retry exhausted")))
				cancel()
				return
			}

			// 重试控制Observable发射值，进行重试
			go func() {
				select {
				case <-ctx.Done():
					return
				default:
					subscribe()
				}
			}()
		})

		go subscribe()

		return NewBaseSubscription(NewBaseDisposable(func() {
			cancel()
			errorSubject.OnComplete()
		}))
	})
}

// OnErrorReturn 错误返回操作符，发生错误时返回指定值
func (o *observableImpl) OnErrorReturn(value interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				// 发生错误，返回指定值
				observer(CreateItem(value))
				observer(CreateItem(nil)) // 发送完成信号
				return
			}

			// 正常值或完成信号
			observer(item)
		})
	})
}

// OnErrorResumeNext 错误恢复操作符，发生错误时切换到另一个Observable
func (o *observableImpl) OnErrorResumeNext(next Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				// 发生错误，切换到下一个Observable
				next.Subscribe(observer)
				return
			}

			// 正常值或完成信号
			observer(item)
		})
	})
}

// Finally 最终操作符，无论正常完成还是错误都会执行的操作
func (o *observableImpl) Finally(action func()) Observable {
	return NewObservable(func(observer Observer) Subscription {
		subscription := o.Subscribe(func(item Item) {
			observer(item)

			// 如果是错误或完成信号，执行finally动作
			if item.IsError() || item.Value == nil {
				action()
			}
		})

		return NewBaseSubscription(NewBaseDisposable(func() {
			subscription.Unsubscribe()
			action() // 取消订阅时也执行finally动作
		}))
	})
}

// Timeout 超时操作符，如果在指定时间内没有发射值则发射超时错误
func (o *observableImpl) TimeoutWithFallback(duration time.Duration, fallback Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		var lastActivity int64
		atomic.StoreInt64(&lastActivity, time.Now().UnixNano())
		hasCompleted := int32(0)

		// 启动超时检查
		go func() {
			select {
			case <-time.After(duration):
				if atomic.LoadInt32(&hasCompleted) == 0 {
					// 超时，切换到fallback Observable
					fallback.Subscribe(observer)
					cancel()
				}
			case <-ctx.Done():
				return
			}
		}()

		subscription := o.Subscribe(func(item Item) {
			// 更新活动时间
			atomic.StoreInt64(&lastActivity, time.Now().UnixNano())

			if item.IsError() || item.Value == nil {
				atomic.StoreInt32(&hasCompleted, 1)
				observer(item)
				cancel()
				return
			}

			observer(item)
		})

		return NewBaseSubscription(NewBaseDisposable(func() {
			cancel()
			subscription.Unsubscribe()
		}))
	})
}

// ============================================================================
// 错误类型定义
// ============================================================================

// RetryExhaustedError 重试耗尽错误
type RetryExhaustedError struct {
	message string
}

func (e *RetryExhaustedError) Error() string {
	return e.message
}

// NewRetryExhaustedError 创建重试耗尽错误
func NewRetryExhaustedError(message string) *RetryExhaustedError {
	return &RetryExhaustedError{message: message}
}

// CompositeError 组合错误，用于包含多个错误
type CompositeError struct {
	errors []error
}

func (e *CompositeError) Error() string {
	if len(e.errors) == 0 {
		return "composite error with no errors"
	}

	result := "composite error: "
	for i, err := range e.errors {
		if i > 0 {
			result += ", "
		}
		result += err.Error()
	}
	return result
}

// Errors 获取所有错误
func (e *CompositeError) Errors() []error {
	return e.errors
}

// NewCompositeError 创建组合错误
func NewCompositeError(errors []error) *CompositeError {
	return &CompositeError{errors: errors}
}
