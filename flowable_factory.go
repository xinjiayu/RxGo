// Flowable factory functions for RxGo
// Flowable工厂函数，提供各种创建Flowable的方法
package rxgo

import (
	"context"
	"sync"
	"time"
)

// ============================================================================
// 基础工厂函数
// ============================================================================

// FlowableJust 从给定的值创建Flowable
func FlowableJust(values ...interface{}) Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		var sentIndex int64 = 0
		var mu sync.Mutex

		subscription := NewFlowableSubscription(
			func(n int64) {
				// 请求时发送数据
				go func() {
					mu.Lock()
					defer mu.Unlock()

					for i := int64(0); i < n && sentIndex < int64(len(values)); i++ {
						value := values[sentIndex]
						sentIndex++
						subscriber.OnNext(CreateItem(value))
					}

					if sentIndex >= int64(len(values)) {
						subscriber.OnComplete()
					}
				}()
			},
			func() {
				// 取消操作
			},
		)

		subscriber.OnSubscribe(subscription)
	})
}

// FlowableEmpty 创建一个空的Flowable，立即完成
func FlowableEmpty() Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		subscription := NewFlowableSubscription(
			func(n int64) {
				// 任何请求都立即完成
				go func() {
					subscriber.OnComplete()
				}()
			},
			func() {
				// 取消操作
			},
		)

		subscriber.OnSubscribe(subscription)
	})
}

// FlowableNever 创建一个永不发射任何值的Flowable
func FlowableNever() Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		subscription := NewFlowableSubscription(
			func(n int64) {
				// 什么都不做，永远不发射值
			},
			func() {
				// 取消操作
			},
		)

		subscriber.OnSubscribe(subscription)
	})
}

// FlowableError 创建一个立即发射错误的Flowable
func FlowableError(err error) Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		subscription := NewFlowableSubscription(
			func(n int64) {
				// 任何请求都立即发送错误
				go func() {
					subscriber.OnError(err)
				}()
			},
			func() {
				// 取消操作
			},
		)

		subscriber.OnSubscribe(subscription)
	})
}

// FlowableRange 创建发射指定范围整数的Flowable
func FlowableRange(start int, count int) Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		currentIndex := int64(0)
		var mu sync.Mutex

		subscription := NewFlowableSubscription(
			func(n int64) {
				// 根据请求发送数据
				go func() {
					mu.Lock()
					defer mu.Unlock()

					for i := int64(0); i < n && currentIndex < int64(count); i++ {
						value := start + int(currentIndex)
						currentIndex++
						subscriber.OnNext(CreateItem(value))
					}

					if currentIndex >= int64(count) {
						subscriber.OnComplete()
					}
				}()
			},
			func() {
				// 取消操作
			},
		)

		subscriber.OnSubscribe(subscription)
	})
}

// ============================================================================
// 从数据源创建
// ============================================================================

// FlowableFromSlice 从切片创建Flowable
func FlowableFromSlice(slice []interface{}) Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		currentIndex := int64(0)
		subscription := NewFlowableSubscription(
			func(n int64) {
				// 根据请求发送数据
				go func() {
					for i := int64(0); i < n && currentIndex < int64(len(slice)); i++ {
						value := slice[currentIndex]
						currentIndex++
						subscriber.OnNext(CreateItem(value))
					}

					if currentIndex >= int64(len(slice)) {
						subscriber.OnComplete()
					}
				}()
			},
			func() {
				// 取消操作
			},
		)

		subscriber.OnSubscribe(subscription)
	})
}

// FlowableFromChannel 从Go channel创建Flowable
func FlowableFromChannel(ch <-chan interface{}) Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		ctx, cancel := context.WithCancel(context.Background())
		requested := int64(0)
		buffer := make([]interface{}, 0)
		bufferMutex := make(chan struct{}, 1)
		bufferMutex <- struct{}{} // 初始化mutex

		subscription := NewFlowableSubscription(
			func(n int64) {
				<-bufferMutex
				requested += n
				// 尝试从缓冲区发送数据
				go func() {
					defer func() { bufferMutex <- struct{}{} }()

					for len(buffer) > 0 && requested > 0 {
						value := buffer[0]
						buffer = buffer[1:]
						requested--
						subscriber.OnNext(CreateItem(value))
					}
				}()
			},
			func() {
				cancel()
			},
		)

		subscriber.OnSubscribe(subscription)

		// 启动数据读取协程
		go func() {
			defer cancel()

			for {
				select {
				case <-ctx.Done():
					return
				case value, ok := <-ch:
					if !ok {
						subscriber.OnComplete()
						return
					}

					<-bufferMutex
					if requested > 0 {
						requested--
						bufferMutex <- struct{}{}
						subscriber.OnNext(CreateItem(value))
					} else {
						buffer = append(buffer, value)
						bufferMutex <- struct{}{}
					}
				}
			}
		}()
	})
}

// FlowableFromItemChannel 从Item channel创建Flowable
func FlowableFromItemChannel(ch <-chan Item) Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		ctx, cancel := context.WithCancel(context.Background())
		requested := int64(0)
		buffer := make([]Item, 0)
		bufferMutex := make(chan struct{}, 1)
		bufferMutex <- struct{}{} // 初始化mutex

		subscription := NewFlowableSubscription(
			func(n int64) {
				<-bufferMutex
				requested += n
				// 尝试从缓冲区发送数据
				go func() {
					defer func() { bufferMutex <- struct{}{} }()

					for len(buffer) > 0 && requested > 0 {
						item := buffer[0]
						buffer = buffer[1:]
						requested--
						subscriber.OnNext(item)
					}
				}()
			},
			func() {
				cancel()
			},
		)

		subscriber.OnSubscribe(subscription)

		// 启动数据读取协程
		go func() {
			defer cancel()

			for {
				select {
				case <-ctx.Done():
					return
				case item, ok := <-ch:
					if !ok {
						subscriber.OnComplete()
						return
					}

					<-bufferMutex
					if requested > 0 {
						requested--
						bufferMutex <- struct{}{}
						subscriber.OnNext(item)
					} else {
						buffer = append(buffer, item)
						bufferMutex <- struct{}{}
					}
				}
			}
		}()
	})
}

// ============================================================================
// 时间相关工厂函数
// ============================================================================

// FlowableInterval 创建定期发射递增整数的Flowable
func FlowableInterval(duration time.Duration) Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		ctx, cancel := context.WithCancel(context.Background())
		currentValue := int64(0)
		requested := int64(0)
		requestChan := make(chan int64, 1)

		subscription := NewFlowableSubscription(
			func(n int64) {
				select {
				case requestChan <- n:
				default:
					// 如果channel满了，直接添加到requested
					requested += n
				}
			},
			func() {
				cancel()
			},
		)

		subscriber.OnSubscribe(subscription)

		// 启动定时器协程
		go func() {
			defer cancel()
			ticker := time.NewTicker(duration)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// 检查是否有请求
					if requested > 0 {
						requested--
						subscriber.OnNext(CreateItem(currentValue))
						currentValue++
					}
				case n := <-requestChan:
					requested += n
				}
			}
		}()
	})
}

// FlowableTimer 创建在指定延迟后发射单个值的Flowable
func FlowableTimer(duration time.Duration) Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		ctx, cancel := context.WithCancel(context.Background())
		emitted := false

		subscription := NewFlowableSubscription(
			func(n int64) {
				if n > 0 && !emitted {
					go func() {
						timer := time.NewTimer(duration)
						defer timer.Stop()

						select {
						case <-ctx.Done():
							return
						case <-timer.C:
							emitted = true
							subscriber.OnNext(CreateItem(0)) // 发射0值
							subscriber.OnComplete()
						}
					}()
				}
			},
			func() {
				cancel()
			},
		)

		subscriber.OnSubscribe(subscription)
	})
}

// ============================================================================
// 创建操作符
// ============================================================================

// FlowableCreate 使用自定义发射器创建Flowable
func FlowableCreate(emitter func(FlowableEmitter), strategy BackpressureStrategy) Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		ctx, cancel := context.WithCancel(context.Background())

		flowableEmitter := &flowableEmitterImpl{
			subscriber: subscriber,
			ctx:        ctx,
			strategy:   strategy,
			requested:  0,
			buffer:     make([]Item, 0),
		}

		subscription := NewFlowableSubscription(
			func(n int64) {
				flowableEmitter.addRequest(n)
			},
			func() {
				cancel()
			},
		)

		subscriber.OnSubscribe(subscription)

		// 在新的协程中执行emitter
		go func() {
			defer cancel()
			emitter(flowableEmitter)
		}()
	})
}

// FlowableEmitter Flowable发射器接口
type FlowableEmitter interface {
	// OnNext 发射下一个值
	OnNext(value interface{})
	// OnError 发射错误
	OnError(err error)
	// OnComplete 发射完成信号
	OnComplete()
	// IsCancelled 检查是否已取消
	IsCancelled() bool
	// GetRequested 获取当前请求数量
	GetRequested() int64
}

// flowableEmitterImpl FlowableEmitter的实现
type flowableEmitterImpl struct {
	subscriber Subscriber
	ctx        context.Context
	strategy   BackpressureStrategy
	requested  int64
	buffer     []Item
	completed  bool
	mu         sync.Mutex
}

func (fe *flowableEmitterImpl) OnNext(value interface{}) {
	fe.mu.Lock()
	defer fe.mu.Unlock()

	if fe.completed || fe.IsCancelled() {
		return
	}

	item := CreateItem(value)

	if fe.requested > 0 {
		fe.requested--
		fe.subscriber.OnNext(item)
	} else {
		// 处理背压
		fe.handleBackpressure(item)
	}
}

func (fe *flowableEmitterImpl) OnError(err error) {
	fe.mu.Lock()
	defer fe.mu.Unlock()

	if fe.completed || fe.IsCancelled() {
		return
	}

	fe.completed = true
	fe.subscriber.OnError(err)
}

func (fe *flowableEmitterImpl) OnComplete() {
	fe.mu.Lock()
	defer fe.mu.Unlock()

	if fe.completed || fe.IsCancelled() {
		return
	}

	fe.completed = true
	fe.subscriber.OnComplete()
}

func (fe *flowableEmitterImpl) IsCancelled() bool {
	select {
	case <-fe.ctx.Done():
		return true
	default:
		return false
	}
}

func (fe *flowableEmitterImpl) GetRequested() int64 {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	return fe.requested
}

func (fe *flowableEmitterImpl) addRequest(n int64) {
	fe.mu.Lock()
	defer fe.mu.Unlock()

	fe.requested += n

	// 处理缓冲区中的数据
	for len(fe.buffer) > 0 && fe.requested > 0 {
		item := fe.buffer[0]
		fe.buffer = fe.buffer[1:]
		fe.requested--
		fe.subscriber.OnNext(item)
	}
}

func (fe *flowableEmitterImpl) handleBackpressure(item Item) {
	switch fe.strategy {
	case BufferStrategy:
		fe.buffer = append(fe.buffer, item)
	case DropStrategy:
		// 丢弃项目，什么都不做
	case BlockStrategy:
		// 简化实现：转为缓冲策略
		fe.buffer = append(fe.buffer, item)
	}
}
