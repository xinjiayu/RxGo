// Single implementation for RxGo
// 单值Observable的实现，发射单个值或错误
package rxgo

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// Single 实现
// ============================================================================

// singleImpl Single的核心实现
type singleImpl struct {
	source     func(onSuccess func(interface{}), onError OnError) Subscription
	config     *Config
	mu         sync.RWMutex
	disposed   int32
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// NewSingle 创建新的Single
func NewSingle(source func(onSuccess func(interface{}), onError OnError) Subscription, options ...Option) Single {
	config := DefaultConfig()
	for _, opt := range options {
		opt.Apply(config)
	}

	ctx, cancel := context.WithCancel(config.Context)

	return &singleImpl{
		source:     source,
		config:     config,
		ctx:        ctx,
		cancelFunc: cancel,
	}
}

// Subscribe 订阅单值观察者
func (s *singleImpl) Subscribe(onSuccess func(interface{}), onError OnError) Subscription {
	if s.IsDisposed() {
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	}

	// 创建带上下文的回调包装器
	wrappedOnSuccess := func(value interface{}) {
		select {
		case <-s.ctx.Done():
			return
		default:
			onSuccess(value)
		}
	}

	wrappedOnError := func(err error) {
		select {
		case <-s.ctx.Done():
			return
		default:
			if onError != nil {
				onError(err)
			}
		}
	}

	subscription := s.source(wrappedOnSuccess, wrappedOnError)

	// 将subscription添加到composite disposable中
	disposable := NewBaseDisposable(func() {
		subscription.Unsubscribe()
	})

	return NewBaseSubscription(disposable)
}

// Map 转换操作符
func (s *singleImpl) Map(transformer Transformer) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		return s.Subscribe(func(value interface{}) {
			if result, err := transformer(value); err != nil {
				if onError != nil {
					onError(err)
				}
			} else {
				onSuccess(result)
			}
		}, onError)
	})
}

// FlatMap 平铺映射
func (s *singleImpl) FlatMap(transformer func(interface{}) Single) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		return s.Subscribe(func(value interface{}) {
			innerSingle := transformer(value)
			innerSingle.Subscribe(onSuccess, onError)
		}, onError)
	})
}

// Filter 过滤（可能返回空）
func (s *singleImpl) Filter(predicate Predicate) Maybe {
	return NewMaybe(func(onSuccess func(interface{}), onError OnError, onComplete OnComplete) Subscription {
		return s.Subscribe(func(value interface{}) {
			if predicate(value) {
				onSuccess(value)
			} else {
				onComplete()
			}
		}, onError)
	})
}

// Catch 错误处理
func (s *singleImpl) Catch(handler func(error) Single) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		return s.Subscribe(onSuccess, func(err error) {
			recoverySingle := handler(err)
			recoverySingle.Subscribe(onSuccess, onError)
		})
	})
}

// Retry 重试
func (s *singleImpl) Retry(count int) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		attempts := int32(0)
		var subscription Subscription

		var attempt func()
		attempt = func() {
			subscription = s.Subscribe(onSuccess, func(err error) {
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

// ToObservable 转换为Observable
func (s *singleImpl) ToObservable() Observable {
	return NewObservable(func(observer Observer) Subscription {
		return s.Subscribe(func(value interface{}) {
			observer(CreateItem(value))
			observer(CreateItem(nil)) // 发送完成信号
		}, func(err error) {
			observer(CreateErrorItem(err))
		})
	})
}

// BlockingGet 阻塞获取值
func (s *singleImpl) BlockingGet() (interface{}, error) {
	ch := make(chan interface{}, 1)
	errCh := make(chan error, 1)

	subscription := s.Subscribe(func(value interface{}) {
		ch <- value
	}, func(err error) {
		errCh <- err
	})

	defer subscription.Unsubscribe()

	select {
	case value := <-ch:
		return value, nil
	case err := <-errCh:
		return nil, err
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

// IsDisposed 检查是否已释放
func (s *singleImpl) IsDisposed() bool {
	return atomic.LoadInt32(&s.disposed) == 1
}

// Dispose 释放资源
func (s *singleImpl) Dispose() {
	if atomic.CompareAndSwapInt32(&s.disposed, 0, 1) {
		if s.cancelFunc != nil {
			s.cancelFunc()
		}
	}
}

// ============================================================================
// Single 工厂函数
// ============================================================================

// SingleJust 创建发射单个值的Single
func SingleJust(value interface{}) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		go func() {
			onSuccess(value)
		}()
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// SingleError 创建发射错误的Single
func SingleError(err error) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		go func() {
			if onError != nil {
				onError(err)
			}
		}()
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// SingleCreate 使用自定义函数创建Single
func SingleCreate(emitter func(onSuccess func(interface{}), onError OnError)) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		go func() {
			emitter(onSuccess, onError)
		}()
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// SingleFromObservable 从Observable创建Single（取第一个值）
func SingleFromObservable(observable Observable) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		hasValue := int32(0)
		return observable.Subscribe(func(item Item) {
			if item.IsError() {
				if onError != nil {
					onError(item.Error)
				}
				return
			}

			if item.Value == nil {
				// 完成信号
				if atomic.LoadInt32(&hasValue) == 0 {
					// 没有值，发送错误
					if onError != nil {
						onError(NewNoSuchElementError("Single expected exactly one item but got none"))
					}
				}
				return
			}

			// 有值
			if atomic.CompareAndSwapInt32(&hasValue, 0, 1) {
				onSuccess(item.Value)
			}
		})
	})
}

// SingleTimer 创建延迟发射的Single
func SingleTimer(delay time.Duration) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		timer := time.NewTimer(delay)
		go func() {
			<-timer.C
			onSuccess(0) // 发射0作为值
		}()

		return NewBaseSubscription(NewBaseDisposable(func() {
			timer.Stop()
		}))
	})
}

// ============================================================================
// 错误类型
// ============================================================================

// NoSuchElementError 没有元素错误
type NoSuchElementError struct {
	message string
}

func (e *NoSuchElementError) Error() string {
	return e.message
}

// NewNoSuchElementError 创建没有元素错误
func NewNoSuchElementError(message string) *NoSuchElementError {
	return &NoSuchElementError{message: message}
}
