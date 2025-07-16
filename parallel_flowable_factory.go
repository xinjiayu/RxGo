// ParallelFlowable factory functions for RxGo
// ParallelFlowable工厂函数实现，提供各种创建并行Flowable的方法
package rxgo

import (
	"context"
	"runtime"
	"sync"
	"time"
)

// ============================================================================
// 从Flowable创建ParallelFlowable
// ============================================================================

// ParallelFromFlowable 从Flowable创建ParallelFlowable
// parallelism: 并行度，默认为CPU核心数
func ParallelFromFlowable(flowable Flowable, parallelism int) ParallelFlowable {
	if parallelism <= 0 {
		parallelism = runtime.NumCPU()
	}

	return NewParallelFlowable(func(subscribers []Subscriber) {
		// 创建分发器，将数据分发到不同的分区
		dispatcher := &flowableDispatcher{
			subscribers: subscribers,
			parallelism: parallelism,
			index:       0,
		}

		flowable.Subscribe(dispatcher)
	}, parallelism)
}

// flowableDispatcher 将Flowable的数据分发到多个并行分区
type flowableDispatcher struct {
	BaseSubscriber
	subscribers []Subscriber
	parallelism int
	index       int64
	mu          sync.Mutex
}

func (fd *flowableDispatcher) OnSubscribe(subscription FlowableSubscription) {
	fd.BaseSubscriber.OnSubscribe(subscription)

	// 为所有订阅者创建subscription包装器
	for _, subscriber := range fd.subscribers {
		wrappedSubscription := &sharedSubscription{
			delegate: subscription,
		}
		subscriber.OnSubscribe(wrappedSubscription)
	}
}

func (fd *flowableDispatcher) OnNext(item Item) {
	if item.IsError() {
		// 错误发送给所有分区
		for _, subscriber := range fd.subscribers {
			subscriber.OnError(item.Error)
		}
		return
	}

	if item.Value == nil {
		// 完成信号发送给所有分区
		for _, subscriber := range fd.subscribers {
			subscriber.OnComplete()
		}
		return
	}

	// 轮询分发数据到各个分区
	fd.mu.Lock()
	targetIndex := fd.index % int64(fd.parallelism)
	fd.index++
	fd.mu.Unlock()

	fd.subscribers[targetIndex].OnNext(item)
}

func (fd *flowableDispatcher) OnError(err error) {
	// 错误发送给所有分区
	for _, subscriber := range fd.subscribers {
		subscriber.OnError(err)
	}
}

func (fd *flowableDispatcher) OnComplete() {
	// 完成信号发送给所有分区
	for _, subscriber := range fd.subscribers {
		subscriber.OnComplete()
	}
}

// ============================================================================
// 基础工厂函数
// ============================================================================

// ParallelJust 从给定的值创建ParallelFlowable
func ParallelJust(values ...interface{}) ParallelFlowable {
	parallelism := runtime.NumCPU()
	if len(values) < parallelism {
		parallelism = len(values)
	}

	return NewParallelFlowable(func(subscribers []Subscriber) {
		// 将值分发到各个分区
		valuesPerPartition := len(values) / len(subscribers)
		remainder := len(values) % len(subscribers)

		for i, subscriber := range subscribers {
			i := i // 捕获循环变量
			subscriber := subscriber

			// 为每个分区创建subscription
			subscription := NewFlowableSubscription(
				func(n int64) {
					// 计算这个分区的值范围
					start := i * valuesPerPartition
					end := start + valuesPerPartition
					if i < remainder {
						start += i
						end += i + 1
					} else {
						start += remainder
						end += remainder
					}

					// 发送这个分区的值
					go func() {
						for j := start; j < end && j < len(values); j++ {
							subscriber.OnNext(CreateItem(values[j]))
						}
						subscriber.OnComplete()
					}()
				},
				func() {
					// 取消逻辑
				},
			)

			subscriber.OnSubscribe(subscription)
		}
	}, parallelism)
}

// ParallelRange 创建发射数值范围的ParallelFlowable
func ParallelRange(start, count int, parallelism int) ParallelFlowable {
	if parallelism <= 0 {
		parallelism = runtime.NumCPU()
	}

	return NewParallelFlowable(func(subscribers []Subscriber) {
		// 将范围分割到各个分区
		itemsPerPartition := count / len(subscribers)
		remainder := count % len(subscribers)

		for i, subscriber := range subscribers {
			i := i // 捕获循环变量
			subscriber := subscriber

			// 计算这个分区的范围
			partitionStart := start + i*itemsPerPartition
			partitionCount := itemsPerPartition
			if i < remainder {
				partitionStart += i
				partitionCount++
			} else {
				partitionStart += remainder
			}

			// 为每个分区创建subscription
			subscription := NewFlowableSubscription(
				func(n int64) {
					go func() {
						for j := 0; j < partitionCount; j++ {
							value := partitionStart + j
							subscriber.OnNext(CreateItem(value))
						}
						subscriber.OnComplete()
					}()
				},
				func() {
					// Cancel function
				},
			)

			subscriber.OnSubscribe(subscription)
		}
	}, parallelism)
}

// ParallelEmpty 创建空的ParallelFlowable
func ParallelEmpty(parallelism int) ParallelFlowable {
	if parallelism <= 0 {
		parallelism = runtime.NumCPU()
	}

	return NewParallelFlowable(func(subscribers []Subscriber) {
		for _, subscriber := range subscribers {
			subscription := NewFlowableSubscription(
				func(n int64) {
					go func() {
						subscriber.OnComplete()
					}()
				},
				func() {
					// 取消逻辑
				},
			)
			subscriber.OnSubscribe(subscription)
		}
	}, parallelism)
}

// ParallelError 创建发射错误的ParallelFlowable
func ParallelError(err error, parallelism int) ParallelFlowable {
	if parallelism <= 0 {
		parallelism = runtime.NumCPU()
	}

	return NewParallelFlowable(func(subscribers []Subscriber) {
		for _, subscriber := range subscribers {
			subscription := NewFlowableSubscription(
				func(n int64) {
					go func() {
						subscriber.OnError(err)
					}()
				},
				func() {
					// 取消逻辑
				},
			)
			subscriber.OnSubscribe(subscription)
		}
	}, parallelism)
}

// ============================================================================
// 高级工厂函数
// ============================================================================

// ParallelFromSlice 从切片创建ParallelFlowable
func ParallelFromSlice(slice []interface{}, parallelism int) ParallelFlowable {
	if parallelism <= 0 {
		parallelism = runtime.NumCPU()
	}

	return NewParallelFlowable(func(subscribers []Subscriber) {
		// 将切片分割到各个分区
		itemsPerPartition := len(slice) / len(subscribers)
		remainder := len(slice) % len(subscribers)

		for i, subscriber := range subscribers {
			i := i // 捕获循环变量
			subscriber := subscriber

			// 计算这个分区的范围
			start := i * itemsPerPartition
			end := start + itemsPerPartition
			if i < remainder {
				start += i
				end += i + 1
			} else {
				start += remainder
				end += remainder
			}

			// 为每个分区创建subscription
			subscription := NewFlowableSubscription(
				func(n int64) {
					go func() {
						for j := start; j < end && j < len(slice); j++ {
							subscriber.OnNext(CreateItem(slice[j]))
						}
						subscriber.OnComplete()
					}()
				},
				func() {
					// 取消逻辑
				},
			)

			subscriber.OnSubscribe(subscription)
		}
	}, parallelism)
}

// ParallelFromChannel 从多个channel创建ParallelFlowable
func ParallelFromChannel(channels []<-chan interface{}) ParallelFlowable {
	parallelism := len(channels)

	return NewParallelFlowable(func(subscribers []Subscriber) {
		if len(subscribers) != len(channels) {
			// 错误处理：订阅者数量必须等于channel数量
			for _, subscriber := range subscribers {
				subscriber.OnSubscribe(NewFlowableSubscription(nil, nil))
				subscriber.OnError(ErrInvalidSubscriberCount)
			}
			return
		}

		// 每个channel对应一个分区
		for i, ch := range channels {
			i := i // 捕获循环变量
			ch := ch
			subscriber := subscribers[i]

			// 为每个分区创建subscription
			ctx, cancel := context.WithCancel(context.Background())
			subscription := NewFlowableSubscription(
				func(n int64) {
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
								subscriber.OnNext(CreateItem(value))
							}
						}
					}()
				},
				func() {
					cancel()
				},
			)

			subscriber.OnSubscribe(subscription)
		}
	}, parallelism)
}

// ============================================================================
// 时间相关工厂函数
// ============================================================================

// ParallelInterval 创建定期发射的ParallelFlowable
func ParallelInterval(duration time.Duration, parallelism int) ParallelFlowable {
	if parallelism <= 0 {
		parallelism = runtime.NumCPU()
	}

	return NewParallelFlowable(func(subscribers []Subscriber) {
		// 为每个分区创建独立的interval
		for i, subscriber := range subscribers {
			i := i // 捕获循环变量
			subscriber := subscriber

			ctx, cancel := context.WithCancel(context.Background())
			subscription := NewFlowableSubscription(
				func(n int64) {
					go func() {
						defer cancel()
						ticker := time.NewTicker(duration)
						defer ticker.Stop()

						counter := int64(i) // 每个分区从不同的起始值开始

						for {
							select {
							case <-ctx.Done():
								return
							case <-ticker.C:
								subscriber.OnNext(CreateItem(counter))
								counter += int64(parallelism) // 每次增加parallelism以避免重复
							}
						}
					}()
				},
				func() {
					cancel()
				},
			)

			subscriber.OnSubscribe(subscription)
		}
	}, parallelism)
}

// ============================================================================
// 辅助结构体
// ============================================================================

// sharedSubscription 共享的subscription实现
type sharedSubscription struct {
	delegate FlowableSubscription
	mu       sync.Mutex
}

func (ss *sharedSubscription) Request(n int64) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.delegate.Request(n)
}

func (ss *sharedSubscription) Cancel() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.delegate.Cancel()
}

func (ss *sharedSubscription) IsCancelled() bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.delegate.IsCancelled()
}
