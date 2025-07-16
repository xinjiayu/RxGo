// ParallelFlowable implementation for RxGo
// 并行Flowable实现，提供高效的并行数据处理能力，基于goroutines的并发模型
package rxgo

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
)

// ============================================================================
// SingleObserver 单值观察者接口
// ============================================================================

// SingleObserver 单值观察者接口
type SingleObserver interface {
	// OnSuccess 成功时调用
	OnSuccess(value interface{})
	// OnError 错误时调用
	OnError(err error)
}

// ============================================================================
// ParallelFlowable 核心接口定义
// ============================================================================

// ParallelFlowable 并行Flowable接口，支持并行数据处理
type ParallelFlowable interface {
	// ============================================================================
	// 核心属性方法
	// ============================================================================

	// Parallelism 获取并行度
	Parallelism() int

	// ============================================================================
	// 转换操作符
	// ============================================================================

	// Map 并行转换每个数据项
	Map(transformer Transformer) ParallelFlowable

	// Filter 并行过滤数据项
	Filter(predicate Predicate) ParallelFlowable

	// FlatMap 将每个数据项转换为Flowable并合并（并行）
	FlatMap(transformer func(interface{}) Flowable) ParallelFlowable

	// ============================================================================
	// 聚合操作符
	// ============================================================================

	// Reduce 并行缩减操作，每个分区独立计算，最后合并结果
	Reduce(reducer func(accumulator, value interface{}) interface{}) Single

	// ReduceWith 带初始值的并行缩减操作
	ReduceWith(initialValue interface{}, reducer func(accumulator, value interface{}) interface{}) Single

	// ============================================================================
	// 调度器操作符
	// ============================================================================

	// RunOn 指定每个并行分区运行的调度器
	RunOn(scheduler Scheduler) ParallelFlowable

	// ============================================================================
	// 转换和终止操作符
	// ============================================================================

	// Sequential 转换为顺序Flowable
	Sequential() Flowable

	// ToFlowable 转换为Flowable（保持顺序）
	ToFlowable() Flowable

	// SubscribeOn 指定订阅操作运行的调度器
	SubscribeOn(scheduler Scheduler) ParallelFlowable

	// ============================================================================
	// 订阅方法
	// ============================================================================

	// Subscribe 使用多个订阅者并行订阅（订阅者数量应等于并行度）
	Subscribe(subscribers []Subscriber)

	// SubscribeWithCallbacks 使用回调函数订阅
	SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) []FlowableSubscription
}

// ============================================================================
// ParallelFlowable 核心实现
// ============================================================================

// parallelFlowableImpl ParallelFlowable的核心实现
type parallelFlowableImpl struct {
	source      func([]Subscriber)
	parallelism int
	config      *Config
	mu          sync.RWMutex
	disposed    int32
	ctx         context.Context
	cancelFunc  context.CancelFunc
}

// NewParallelFlowable 创建新的ParallelFlowable
func NewParallelFlowable(source func([]Subscriber), parallelism int, options ...Option) ParallelFlowable {
	if parallelism <= 0 {
		parallelism = runtime.NumCPU()
	}

	config := DefaultConfig()
	for _, opt := range options {
		opt.Apply(config)
	}

	ctx, cancel := context.WithCancel(config.Context)

	return &parallelFlowableImpl{
		source:      source,
		parallelism: parallelism,
		config:      config,
		ctx:         ctx,
		cancelFunc:  cancel,
	}
}

// Parallelism 获取并行度
func (pf *parallelFlowableImpl) Parallelism() int {
	return pf.parallelism
}

// Subscribe 使用多个订阅者并行订阅
func (pf *parallelFlowableImpl) Subscribe(subscribers []Subscriber) {
	if pf.IsDisposed() {
		// 如果已释放，立即发送错误给所有订阅者
		for _, subscriber := range subscribers {
			subscriber.OnSubscribe(NewFlowableSubscription(nil, nil))
			subscriber.OnError(ErrDisposed)
		}
		return
	}

	if len(subscribers) != pf.parallelism {
		// 订阅者数量必须等于并行度
		for _, subscriber := range subscribers {
			subscriber.OnSubscribe(NewFlowableSubscription(nil, nil))
			subscriber.OnError(ErrInvalidSubscriberCount)
		}
		return
	}

	// 包装订阅者以支持上下文
	wrappedSubscribers := make([]Subscriber, len(subscribers))
	for i, subscriber := range subscribers {
		wrappedSubscribers[i] = &contextSubscriber{
			delegate: subscriber,
			ctx:      pf.ctx,
		}
	}

	pf.source(wrappedSubscribers)
}

// SubscribeWithCallbacks 使用回调函数订阅
func (pf *parallelFlowableImpl) SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) []FlowableSubscription {
	subscriptions := make([]FlowableSubscription, pf.parallelism)
	subscribers := make([]Subscriber, pf.parallelism)

	var subscriptionWg sync.WaitGroup
	subscriptionWg.Add(pf.parallelism)

	// 为每个并行分区创建订阅者
	for i := 0; i < pf.parallelism; i++ {
		i := i // 捕获循环变量
		baseSubscriber := &callbackSubscriber{
			onNext:     onNext,
			onError:    onError,
			onComplete: onComplete,
		}

		// 包装订阅者以捕获subscription
		subscribers[i] = &subscriptionCapturingSubscriber{
			delegate: baseSubscriber,
			onSubscribe: func(s FlowableSubscription) {
				subscriptions[i] = s
				baseSubscriber.OnSubscribe(s)
				subscriptionWg.Done()
			},
		}
	}

	pf.Subscribe(subscribers)

	// 等待所有订阅完成
	subscriptionWg.Wait()
	return subscriptions
}

// IsDisposed 检查是否已释放
func (pf *parallelFlowableImpl) IsDisposed() bool {
	return atomic.LoadInt32(&pf.disposed) == 1
}

// Dispose 释放资源
func (pf *parallelFlowableImpl) Dispose() {
	if atomic.CompareAndSwapInt32(&pf.disposed, 0, 1) {
		if pf.cancelFunc != nil {
			pf.cancelFunc()
		}
	}
}

// ============================================================================
// 转换操作符实现
// ============================================================================

// Map 并行转换每个数据项
func (pf *parallelFlowableImpl) Map(transformer Transformer) ParallelFlowable {
	return NewParallelFlowable(func(subscribers []Subscriber) {
		// 为每个分区创建map订阅者
		mapSubscribers := make([]Subscriber, len(subscribers))
		for i, subscriber := range subscribers {
			mapSubscribers[i] = &parallelMapSubscriber{
				downstream:  subscriber,
				transformer: transformer,
			}
		}
		pf.Subscribe(mapSubscribers)
	}, pf.parallelism)
}

// Filter 并行过滤数据项
func (pf *parallelFlowableImpl) Filter(predicate Predicate) ParallelFlowable {
	return NewParallelFlowable(func(subscribers []Subscriber) {
		// 为每个分区创建filter订阅者
		filterSubscribers := make([]Subscriber, len(subscribers))
		for i, subscriber := range subscribers {
			filterSubscribers[i] = &parallelFilterSubscriber{
				downstream: subscriber,
				predicate:  predicate,
			}
		}
		pf.Subscribe(filterSubscribers)
	}, pf.parallelism)
}

// FlatMap 将每个数据项转换为Flowable并合并（并行）
func (pf *parallelFlowableImpl) FlatMap(transformer func(interface{}) Flowable) ParallelFlowable {
	return NewParallelFlowable(func(subscribers []Subscriber) {
		// 为每个分区创建flatMap订阅者
		flatMapSubscribers := make([]Subscriber, len(subscribers))
		for i, subscriber := range subscribers {
			flatMapSubscribers[i] = &parallelFlatMapSubscriber{
				downstream:  subscriber,
				transformer: transformer,
				active:      make(map[int]FlowableSubscription),
			}
		}
		pf.Subscribe(flatMapSubscribers)
	}, pf.parallelism)
}

// ============================================================================
// 聚合操作符实现
// ============================================================================

// Reduce 并行缩减操作
func (pf *parallelFlowableImpl) Reduce(reducer func(accumulator, value interface{}) interface{}) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		results := make([]interface{}, pf.parallelism)
		completed := make([]bool, pf.parallelism)
		errors := make([]error, pf.parallelism)
		var mu sync.Mutex

		// 检查是否所有分区都完成
		checkCompletion := func() {
			mu.Lock()
			defer mu.Unlock()

			// 检查错误
			for _, err := range errors {
				if err != nil {
					onError(err)
					return
				}
			}

			// 检查是否全部完成
			allCompleted := true
			for _, comp := range completed {
				if !comp {
					allCompleted = false
					break
				}
			}

			if allCompleted {
				// 合并所有分区的结果
				var finalResult interface{}
				hasResult := false

				for _, result := range results {
					if result != nil {
						if !hasResult {
							finalResult = result
							hasResult = true
						} else {
							finalResult = reducer(finalResult, result)
						}
					}
				}

				if hasResult {
					onSuccess(finalResult)
				} else {
					onError(ErrEmpty)
				}
			}
		}

		subscribers := make([]Subscriber, pf.parallelism)
		for i := 0; i < pf.parallelism; i++ {
			i := i // 捕获循环变量
			subscribers[i] = &parallelReduceSubscriber{
				index:   i,
				reducer: reducer,
				onResult: func(idx int, result interface{}) {
					mu.Lock()
					results[idx] = result
					completed[idx] = true
					mu.Unlock()
					checkCompletion()
				},
				onError: func(idx int, err error) {
					mu.Lock()
					errors[idx] = err
					mu.Unlock()
					checkCompletion()
				},
			}
		}

		pf.Subscribe(subscribers)
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// ReduceWith 带初始值的并行缩减操作
func (pf *parallelFlowableImpl) ReduceWith(initialValue interface{}, reducer func(accumulator, value interface{}) interface{}) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		results := make([]interface{}, pf.parallelism)
		completed := make([]bool, pf.parallelism)
		errors := make([]error, pf.parallelism)
		var mu sync.Mutex

		// 初始化所有分区的初始值
		for i := range results {
			results[i] = initialValue
		}

		checkCompletion := func() {
			mu.Lock()
			defer mu.Unlock()

			// 检查错误
			for _, err := range errors {
				if err != nil {
					onError(err)
					return
				}
			}

			// 检查是否全部完成
			allCompleted := true
			for _, comp := range completed {
				if !comp {
					allCompleted = false
					break
				}
			}

			if allCompleted {
				// 合并所有分区的结果
				finalResult := initialValue
				for _, result := range results {
					finalResult = reducer(finalResult, result)
				}
				onSuccess(finalResult)
			}
		}

		subscribers := make([]Subscriber, pf.parallelism)
		for i := 0; i < pf.parallelism; i++ {
			i := i // 捕获循环变量
			subscribers[i] = &parallelReduceWithSubscriber{
				index:        i,
				initialValue: initialValue,
				reducer:      reducer,
				onResult: func(idx int, result interface{}) {
					mu.Lock()
					results[idx] = result
					completed[idx] = true
					mu.Unlock()
					checkCompletion()
				},
				onError: func(idx int, err error) {
					mu.Lock()
					errors[idx] = err
					mu.Unlock()
					checkCompletion()
				},
			}
		}

		pf.Subscribe(subscribers)
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}

// ============================================================================
// 调度器操作符实现
// ============================================================================

// RunOn 指定每个并行分区运行的调度器
func (pf *parallelFlowableImpl) RunOn(scheduler Scheduler) ParallelFlowable {
	return NewParallelFlowable(func(subscribers []Subscriber) {
		// 为每个分区创建调度订阅者
		scheduledSubscribers := make([]Subscriber, len(subscribers))
		for i, subscriber := range subscribers {
			scheduledSubscribers[i] = &parallelRunOnSubscriber{
				downstream: subscriber,
				scheduler:  scheduler,
				buffer:     make(chan Item, pf.config.BufferSize),
				done:       make(chan struct{}),
			}
		}
		pf.Subscribe(scheduledSubscribers)
	}, pf.parallelism)
}

// SubscribeOn 指定订阅操作运行的调度器
func (pf *parallelFlowableImpl) SubscribeOn(scheduler Scheduler) ParallelFlowable {
	return NewParallelFlowable(func(subscribers []Subscriber) {
		// 在指定调度器上执行订阅
		disposable := scheduler.Schedule(func() {
			pf.Subscribe(subscribers)
		})

		// 包装订阅者以支持取消
		for i, subscriber := range subscribers {
			subscribers[i] = &subscribeOnSubscriber{
				delegate:   subscriber,
				disposable: disposable,
			}
		}
	}, pf.parallelism)
}

// ============================================================================
// 转换操作符实现
// ============================================================================

// Sequential 转换为顺序Flowable
func (pf *parallelFlowableImpl) Sequential() Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		// 创建一个合并所有分区的管道
		mergeSubscriber := &parallelSequentialSubscriber{
			downstream:  subscriber,
			parallelism: pf.parallelism,
			completed:   make([]bool, pf.parallelism),
			hasError:    false,
		}

		subscribers := make([]Subscriber, pf.parallelism)
		for i := 0; i < pf.parallelism; i++ {
			i := i // 捕获循环变量
			subscribers[i] = &sequentialPartitionSubscriber{
				partitionIndex: i,
				parent:         mergeSubscriber,
			}
		}

		pf.Subscribe(subscribers)
	})
}

// ToFlowable 转换为Flowable（保持顺序）
func (pf *parallelFlowableImpl) ToFlowable() Flowable {
	return pf.Sequential()
}

// ============================================================================
// 错误定义
// ============================================================================

var (
	// ErrDisposed ParallelFlowable已释放错误
	ErrDisposed = errors.New("ParallelFlowable已释放")
	// ErrInvalidSubscriberCount 无效的订阅者数量错误
	ErrInvalidSubscriberCount = errors.New("订阅者数量必须等于并行度")
	// ErrEmpty 空结果错误
	ErrEmpty = errors.New("没有可缩减的元素")
)
