// Maybe implementation for RxGo
// 可能为空的单值Observable实现，发射0个或1个值
package rxgo

import (
	"context"
	"sync"
	"sync/atomic"
)

// ============================================================================
// Maybe 实现
// ============================================================================

// maybeImpl Maybe的核心实现
type maybeImpl struct {
	source     func(onSuccess func(interface{}), onError OnError, onComplete OnComplete) Subscription
	config     *Config
	mu         sync.RWMutex
	disposed   int32
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// NewMaybe 创建新的Maybe
func NewMaybe(source func(onSuccess func(interface{}), onError OnError, onComplete OnComplete) Subscription, options ...Option) Maybe {
	config := DefaultConfig()
	for _, opt := range options {
		opt.Apply(config)
	}

	ctx, cancel := context.WithCancel(config.Context)

	return &maybeImpl{
		source:     source,
		config:     config,
		ctx:        ctx,
		cancelFunc: cancel,
	}
}

// Subscribe 订阅Maybe观察者
func (m *maybeImpl) Subscribe(onSuccess func(interface{}), onError OnError, onComplete OnComplete) Subscription {
	if m.IsDisposed() {
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	}

	// 创建带上下文的回调包装器
	wrappedOnSuccess := func(value interface{}) {
		select {
		case <-m.ctx.Done():
			return
		default:
			onSuccess(value)
		}
	}

	wrappedOnError := func(err error) {
		select {
		case <-m.ctx.Done():
			return
		default:
			if onError != nil {
				onError(err)
			}
		}
	}

	wrappedOnComplete := func() {
		select {
		case <-m.ctx.Done():
			return
		default:
			if onComplete != nil {
				onComplete()
			}
		}
	}

	subscription := m.source(wrappedOnSuccess, wrappedOnError, wrappedOnComplete)

	// 将subscription添加到composite disposable中
	disposable := NewBaseDisposable(func() {
		subscription.Unsubscribe()
	})

	return NewBaseSubscription(disposable)
}

// Map 转换操作符
func (m *maybeImpl) Map(transformer Transformer) Maybe {
	return NewMaybe(func(onSuccess func(interface{}), onError OnError, onComplete OnComplete) Subscription {
		return m.Subscribe(func(value interface{}) {
			if result, err := transformer(value); err != nil {
				if onError != nil {
					onError(err)
				}
			} else {
				onSuccess(result)
			}
		}, onError, onComplete)
	})
}

// FlatMap 平铺映射
func (m *maybeImpl) FlatMap(transformer func(interface{}) Maybe) Maybe {
	return NewMaybe(func(onSuccess func(interface{}), onError OnError, onComplete OnComplete) Subscription {
		return m.Subscribe(func(value interface{}) {
			innerMaybe := transformer(value)
			innerMaybe.Subscribe(onSuccess, onError, onComplete)
		}, onError, onComplete)
	})
}

// Filter 过滤
func (m *maybeImpl) Filter(predicate Predicate) Maybe {
	return NewMaybe(func(onSuccess func(interface{}), onError OnError, onComplete OnComplete) Subscription {
		return m.Subscribe(func(value interface{}) {
			if predicate(value) {
				onSuccess(value)
			} else {
				onComplete()
			}
		}, onError, onComplete)
	})
}

// DefaultIfEmpty 如果为空则使用默认值
func (m *maybeImpl) DefaultIfEmpty(defaultValue interface{}) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		hasValue := int32(0)
		return m.Subscribe(func(value interface{}) {
			atomic.StoreInt32(&hasValue, 1)
			onSuccess(value)
		}, onError, func() {
			if atomic.LoadInt32(&hasValue) == 0 {
				onSuccess(defaultValue)
			}
		})
	})
}

// Catch 错误处理
func (m *maybeImpl) Catch(handler func(error) Maybe) Maybe {
	return NewMaybe(func(onSuccess func(interface{}), onError OnError, onComplete OnComplete) Subscription {
		return m.Subscribe(onSuccess, func(err error) {
			recoveryMaybe := handler(err)
			recoveryMaybe.Subscribe(onSuccess, onError, onComplete)
		}, onComplete)
	})
}

// ToObservable 转换为Observable
func (m *maybeImpl) ToObservable() Observable {
	return NewObservable(func(observer Observer) Subscription {
		return m.Subscribe(func(value interface{}) {
			observer(CreateItem(value))
			observer(CreateItem(nil)) // 发送完成信号
		}, func(err error) {
			observer(CreateErrorItem(err))
		}, func() {
			observer(CreateItem(nil)) // 发送完成信号
		})
	})
}

// ToSingle 转换为Single
func (m *maybeImpl) ToSingle() Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		hasValue := int32(0)
		return m.Subscribe(func(value interface{}) {
			atomic.StoreInt32(&hasValue, 1)
			onSuccess(value)
		}, onError, func() {
			if atomic.LoadInt32(&hasValue) == 0 {
				if onError != nil {
					onError(NewNoSuchElementError("Maybe expected exactly one item but got none"))
				}
			}
		})
	})
}

// BlockingGet 阻塞获取值
func (m *maybeImpl) BlockingGet() (interface{}, error) {
	ch := make(chan interface{}, 1)
	errCh := make(chan error, 1)
	completeCh := make(chan struct{}, 1)

	subscription := m.Subscribe(func(value interface{}) {
		ch <- value
	}, func(err error) {
		errCh <- err
	}, func() {
		completeCh <- struct{}{}
	})

	defer subscription.Unsubscribe()

	select {
	case value := <-ch:
		return value, nil
	case err := <-errCh:
		return nil, err
	case <-completeCh:
		return nil, NewNoSuchElementError("Maybe completed without emitting any value")
	case <-m.ctx.Done():
		return nil, m.ctx.Err()
	}
}

// IsDisposed 检查是否已释放
func (m *maybeImpl) IsDisposed() bool {
	return atomic.LoadInt32(&m.disposed) == 1
}

// Dispose 释放资源
func (m *maybeImpl) Dispose() {
	if atomic.CompareAndSwapInt32(&m.disposed, 0, 1) {
		if m.cancelFunc != nil {
			m.cancelFunc()
		}
	}
}

// ============================================================================
// Maybe 工厂函数
// ============================================================================

// MaybeJust 创建发射单个值的Maybe
func MaybeJust(value interface{}) Maybe {
	return NewMaybe(func(onSuccess func(interface{}), onError OnError, onComplete OnComplete) Subscription {
		go func() {
			onSuccess(value)
		}()
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// MaybeEmpty 创建空的Maybe
func MaybeEmpty() Maybe {
	return NewMaybe(func(onSuccess func(interface{}), onError OnError, onComplete OnComplete) Subscription {
		go func() {
			onComplete()
		}()
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// MaybeError 创建发射错误的Maybe
func MaybeError(err error) Maybe {
	return NewMaybe(func(onSuccess func(interface{}), onError OnError, onComplete OnComplete) Subscription {
		go func() {
			if onError != nil {
				onError(err)
			}
		}()
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// MaybeCreate 使用自定义函数创建Maybe
func MaybeCreate(emitter func(onSuccess func(interface{}), onError OnError, onComplete OnComplete)) Maybe {
	return NewMaybe(func(onSuccess func(interface{}), onError OnError, onComplete OnComplete) Subscription {
		go func() {
			emitter(onSuccess, onError, onComplete)
		}()
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// MaybeFromObservable 从Observable创建Maybe（取第一个值）
func MaybeFromObservable(observable Observable) Maybe {
	return NewMaybe(func(onSuccess func(interface{}), onError OnError, onComplete OnComplete) Subscription {
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
					// 没有值，发送完成信号
					onComplete()
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
