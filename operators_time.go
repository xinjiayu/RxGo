// Time-based operators for RxGo
// 时间操作符实现，包含Debounce, Throttle, Delay, Timeout等
package rxgo

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// 时间操作符实现
// ============================================================================

// Debounce 防抖操作符，只有在指定时间内没有新值时才发射最后一个值
func (o *observableImpl) Debounce(duration time.Duration) Observable {
	return NewObservable(func(observer Observer) Subscription {
		var lastValue interface{}
		var lastTime int64
		var timer *time.Timer
		mu := &sync.Mutex{}
		hasValue := int32(0)

		return o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				// 完成信号
				mu.Lock()
				if timer != nil {
					timer.Stop()
				}
				// 发射最后一个值（如果有）
				if atomic.LoadInt32(&hasValue) == 1 {
					observer(CreateItem(lastValue))
				}
				mu.Unlock()
				observer(item)
				return
			}

			mu.Lock()
			lastValue = item.Value
			atomic.StoreInt32(&hasValue, 1)
			currentTime := time.Now().UnixNano()
			atomic.StoreInt64(&lastTime, currentTime)

			// 取消之前的定时器
			if timer != nil {
				timer.Stop()
			}

			// 设置新的定时器
			timer = time.AfterFunc(duration, func() {
				mu.Lock()
				defer mu.Unlock()
				// 检查是否是最后一个值
				if atomic.LoadInt64(&lastTime) == currentTime {
					observer(CreateItem(lastValue))
				}
			})
			mu.Unlock()
		})
	})
}

// Throttle 节流操作符，在指定时间间隔内只发射第一个值
func (o *observableImpl) Throttle(duration time.Duration) Observable {
	return NewObservable(func(observer Observer) Subscription {
		var lastEmitTime int64
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				// 完成信号
				observer(item)
				return
			}

			now := time.Now().UnixNano()
			lastTime := atomic.LoadInt64(&lastEmitTime)

			// 检查是否可以发射
			if lastTime == 0 || now-lastTime >= duration.Nanoseconds() {
				if atomic.CompareAndSwapInt64(&lastEmitTime, lastTime, now) {
					observer(item)
				}
			}
		})
	})
}

// Delay 延迟操作符，将所有发射延迟指定时间
func (o *observableImpl) Delay(duration time.Duration) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())

		subscription := o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				// 延迟发射完成信号
				go func() {
					select {
					case <-time.After(duration):
						observer(item)
					case <-ctx.Done():
						return
					}
				}()
				return
			}

			// 延迟发射值
			go func() {
				select {
				case <-time.After(duration):
					observer(item)
				case <-ctx.Done():
					return
				}
			}()
		})

		return NewBaseSubscription(NewBaseDisposable(func() {
			cancel()
			subscription.Unsubscribe()
		}))
	})
}

// Timeout 超时操作符，如果在指定时间内没有发射值则发射超时错误
func (o *observableImpl) Timeout(duration time.Duration) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		var lastActivity int64
		atomic.StoreInt64(&lastActivity, time.Now().UnixNano())

		// 启动超时检查
		go func() {
			ticker := time.NewTicker(duration / 10) // 检查频率为超时时间的1/10
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					now := time.Now().UnixNano()
					last := atomic.LoadInt64(&lastActivity)
					if now-last >= duration.Nanoseconds() {
						observer(CreateErrorItem(NewTimeoutError("Observable timed out")))
						cancel()
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()

		subscription := o.Subscribe(func(item Item) {
			// 更新活动时间
			atomic.StoreInt64(&lastActivity, time.Now().UnixNano())

			if item.IsError() {
				observer(item)
				cancel()
				return
			}

			if item.Value == nil {
				// 完成信号
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

// Sample 采样操作符，定期发射最新的值
func (o *observableImpl) Sample(duration time.Duration) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())
		var latestValue interface{}
		hasValue := int32(0)
		mu := &sync.RWMutex{}

		// 启动采样定时器
		go func() {
			ticker := time.NewTicker(duration)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					mu.RLock()
					if atomic.LoadInt32(&hasValue) == 1 {
						observer(CreateItem(latestValue))
					}
					mu.RUnlock()
				case <-ctx.Done():
					return
				}
			}
		}()

		subscription := o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				cancel()
				return
			}

			if item.Value == nil {
				// 完成信号
				observer(item)
				cancel()
				return
			}

			// 更新最新值
			mu.Lock()
			latestValue = item.Value
			atomic.StoreInt32(&hasValue, 1)
			mu.Unlock()
		})

		return NewBaseSubscription(NewBaseDisposable(func() {
			cancel()
			subscription.Unsubscribe()
		}))
	})
}

// ============================================================================
// 时间相关数据结构
// ============================================================================

// TimestampedItem 带时间戳的项
type TimestampedItem struct {
	Value     interface{}
	Timestamp time.Time
}

// TimeIntervalItem 带时间间隔的项
type TimeIntervalItem struct {
	Value    interface{}
	Interval time.Duration
}

// TimeoutError 超时错误
type TimeoutError struct {
	message string
}

func (e *TimeoutError) Error() string {
	return e.message
}

// NewTimeoutError 创建超时错误
func NewTimeoutError(message string) *TimeoutError {
	return &TimeoutError{message: message}
}
