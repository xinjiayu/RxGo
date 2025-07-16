// Flowable core implementation for RxGo
// Flowable核心实现，支持背压处理的响应式数据流
package rxgo

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// ============================================================================
// Flowable 核心实现
// ============================================================================

// flowableImpl Flowable的核心实现
type flowableImpl struct {
	source     func(subscriber Subscriber)
	config     *Config
	mu         sync.RWMutex
	disposed   int32
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// NewFlowable 创建新的Flowable
func NewFlowable(source func(subscriber Subscriber), options ...Option) Flowable {
	config := DefaultConfig()
	for _, opt := range options {
		opt.Apply(config)
	}

	ctx, cancel := context.WithCancel(config.Context)

	return &flowableImpl{
		source:     source,
		config:     config,
		ctx:        ctx,
		cancelFunc: cancel,
	}
}

// Subscribe 订阅Subscriber
func (f *flowableImpl) Subscribe(subscriber Subscriber) {
	if f.IsDisposed() {
		// 如果已释放，立即发送错误
		subscriber.OnSubscribe(NewFlowableSubscription(nil, nil))
		subscriber.OnError(errors.New("flowable已释放"))
		return
	}

	// 创建带上下文的Subscriber包装器
	wrappedSubscriber := &contextSubscriber{
		delegate: subscriber,
		ctx:      f.ctx,
	}

	f.source(wrappedSubscriber)
}

// SubscribeWithCallbacks 使用回调函数订阅
func (f *flowableImpl) SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) FlowableSubscription {
	var subscription FlowableSubscription

	subscriber := &callbackSubscriber{
		onNext:     onNext,
		onError:    onError,
		onComplete: onComplete,
	}

	// 包装订阅者以捕获subscription
	wrappedSubscriber := &subscriptionCapturingSubscriber{
		delegate: subscriber,
		onSubscribe: func(s FlowableSubscription) {
			subscription = s
			subscriber.OnSubscribe(s)
		},
	}

	f.Subscribe(wrappedSubscriber)
	// NOTE: 这里可能存在竞态条件，但为了避免死锁暂时接受
	return subscription
}

// SubscribeOn 指定订阅操作运行的调度器
func (f *flowableImpl) SubscribeOn(scheduler Scheduler) Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		disposable := scheduler.Schedule(func() {
			f.Subscribe(subscriber)
		})

		subscription := NewFlowableSubscription(
			func(n int64) {
				// 请求操作在调度器上执行
			},
			func() {
				disposable.Dispose()
			},
		)

		subscriber.OnSubscribe(subscription)
	})
}

// ObserveOn 指定观察操作运行的调度器
func (f *flowableImpl) ObserveOn(scheduler Scheduler) Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		observeOnSubscriber := &observeOnSubscriber{
			downstream: subscriber,
			scheduler:  scheduler,
			buffer:     make(chan Item, f.config.BufferSize),
			done:       make(chan struct{}),
		}

		f.Subscribe(observeOnSubscriber)
	})
}

// IsDisposed 检查是否已释放
func (f *flowableImpl) IsDisposed() bool {
	return atomic.LoadInt32(&f.disposed) == 1
}

// Dispose 释放资源
func (f *flowableImpl) Dispose() {
	if atomic.CompareAndSwapInt32(&f.disposed, 0, 1) {
		if f.cancelFunc != nil {
			f.cancelFunc()
		}
	}
}

// ============================================================================
// 转换操作符实现
// ============================================================================

// Map 转换每个数据项
func (f *flowableImpl) Map(transformer Transformer) Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		mapSubscriber := &mapSubscriber{
			downstream:  subscriber,
			transformer: transformer,
		}
		f.Subscribe(mapSubscriber)
	})
}

// Filter 过滤数据项
func (f *flowableImpl) Filter(predicate Predicate) Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		filterSubscriber := &filterSubscriber{
			downstream: subscriber,
			predicate:  predicate,
		}
		f.Subscribe(filterSubscriber)
	})
}

// Take 取前N个数据项
func (f *flowableImpl) Take(count int64) Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		takeSubscriber := &takeSubscriber{
			downstream: subscriber,
			remaining:  count,
		}
		f.Subscribe(takeSubscriber)
	})
}

// Skip 跳过前N个数据项
func (f *flowableImpl) Skip(count int64) Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		skipSubscriber := &skipSubscriber{
			downstream: subscriber,
			toSkip:     count,
		}
		f.Subscribe(skipSubscriber)
	})
}

// FlatMap 将每个数据项转换为Flowable并合并
func (f *flowableImpl) FlatMap(transformer func(interface{}) Flowable, maxConcurrency int) Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		flatMapSubscriber := &flatMapSubscriber{
			downstream:     subscriber,
			transformer:    transformer,
			maxConcurrency: maxConcurrency,
			active:         make(map[int]FlowableSubscription),
		}
		f.Subscribe(flatMapSubscriber)
	})
}

// ============================================================================
// 背压操作符实现
// ============================================================================

// OnBackpressureBuffer 缓冲背压数据
func (f *flowableImpl) OnBackpressureBuffer() Flowable {
	return f.OnBackpressureBufferWithCapacity(-1) // 无限缓冲
}

// OnBackpressureBufferWithCapacity 带容量限制的缓冲
func (f *flowableImpl) OnBackpressureBufferWithCapacity(capacity int) Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		bufferSubscriber := &backpressureBufferSubscriber{
			downstream: subscriber,
			buffer:     make([]Item, 0),
			capacity:   capacity,
		}
		f.Subscribe(bufferSubscriber)
	})
}

// OnBackpressureDrop 丢弃背压数据
func (f *flowableImpl) OnBackpressureDrop() Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		dropSubscriber := &backpressureDropSubscriber{
			downstream: subscriber,
		}
		f.Subscribe(dropSubscriber)
	})
}

// OnBackpressureLatest 保留最新数据
func (f *flowableImpl) OnBackpressureLatest() Flowable {
	return NewFlowable(func(subscriber Subscriber) {
		latestSubscriber := &backpressureLatestSubscriber{
			downstream: subscriber,
		}
		f.Subscribe(latestSubscriber)
	})
}

// ============================================================================
// 转换和终止操作符
// ============================================================================

// ToObservable 转换为Observable（忽略背压）
func (f *flowableImpl) ToObservable() Observable {
	return NewObservable(func(observer Observer) Subscription {
		subscription := f.SubscribeWithCallbacks(
			func(value interface{}) {
				observer(CreateItem(value))
			},
			func(err error) {
				observer(CreateErrorItem(err))
			},
			func() {
				observer(CreateItem(nil)) // 完成信号
			},
		)

		// 自动请求无限数据（Observable不支持背压）
		subscription.Request(int64(^uint64(0) >> 1)) // Long.MAX_VALUE

		return NewBaseSubscription(NewBaseDisposable(func() {
			subscription.Cancel()
		}))
	})
}

// ToSlice 转换为切片
func (f *flowableImpl) ToSlice() Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		var items []interface{}
		var mu sync.Mutex

		subscription := f.SubscribeWithCallbacks(
			func(value interface{}) {
				mu.Lock()
				items = append(items, value)
				mu.Unlock()
			},
			onError,
			func() {
				mu.Lock()
				result := make([]interface{}, len(items))
				copy(result, items)
				mu.Unlock()
				onSuccess(result)
			},
		)

		return NewBaseSubscription(NewBaseDisposable(func() {
			subscription.Cancel()
		}))
	})
}

// BlockingFirst 阻塞获取第一个数据项
func (f *flowableImpl) BlockingFirst() (interface{}, error) {
	done := make(chan struct{})
	var result interface{}
	var err error

	subscription := f.SubscribeWithCallbacks(
		func(value interface{}) {
			result = value
			close(done)
			// 取消订阅，我们只需要第一个值
		},
		func(e error) {
			err = e
			close(done)
		},
		func() {
			err = errors.New("flowable为空，没有数据项")
			close(done)
		},
	)

	// 立即请求第一个元素
	subscription.Request(1)

	<-done
	subscription.Cancel()

	return result, err
}

// ============================================================================
// 辅助结构体
// ============================================================================

// contextSubscriber 带上下文的订阅者包装器
type contextSubscriber struct {
	delegate Subscriber
	ctx      context.Context
}

func (cs *contextSubscriber) OnSubscribe(subscription FlowableSubscription) {
	// 包装subscription以支持上下文取消
	wrappedSubscription := &contextSubscription{
		delegate: subscription,
		ctx:      cs.ctx,
	}
	cs.delegate.OnSubscribe(wrappedSubscription)
}

func (cs *contextSubscriber) OnNext(item Item) {
	select {
	case <-cs.ctx.Done():
		return
	default:
		cs.delegate.OnNext(item)
	}
}

func (cs *contextSubscriber) OnError(err error) {
	select {
	case <-cs.ctx.Done():
		return
	default:
		cs.delegate.OnError(err)
	}
}

func (cs *contextSubscriber) OnComplete() {
	select {
	case <-cs.ctx.Done():
		return
	default:
		cs.delegate.OnComplete()
	}
}

// contextSubscription 带上下文的订阅包装器
type contextSubscription struct {
	delegate FlowableSubscription
	ctx      context.Context
}

func (cs *contextSubscription) Request(n int64) {
	select {
	case <-cs.ctx.Done():
		return
	default:
		cs.delegate.Request(n)
	}
}

func (cs *contextSubscription) Cancel() {
	cs.delegate.Cancel()
}

func (cs *contextSubscription) IsCancelled() bool {
	return cs.delegate.IsCancelled()
}

// callbackSubscriber 回调订阅者
type callbackSubscriber struct {
	BaseSubscriber
	onNext     OnNext
	onError    OnError
	onComplete OnComplete
}

func (cs *callbackSubscriber) OnNext(item Item) {
	if item.IsError() {
		if cs.onError != nil {
			cs.onError(item.Error)
		}
	} else if item.Value == nil {
		if cs.onComplete != nil {
			cs.onComplete()
		}
	} else {
		if cs.onNext != nil {
			cs.onNext(item.Value)
		}
	}
}

func (cs *callbackSubscriber) OnError(err error) {
	if cs.onError != nil {
		cs.onError(err)
	}
}

func (cs *callbackSubscriber) OnComplete() {
	if cs.onComplete != nil {
		cs.onComplete()
	}
}

// subscriptionCapturingSubscriber 捕获订阅的订阅者
type subscriptionCapturingSubscriber struct {
	delegate    Subscriber
	onSubscribe func(FlowableSubscription)
}

func (scs *subscriptionCapturingSubscriber) OnSubscribe(subscription FlowableSubscription) {
	if scs.onSubscribe != nil {
		scs.onSubscribe(subscription)
	} else {
		scs.delegate.OnSubscribe(subscription)
	}
}

func (scs *subscriptionCapturingSubscriber) OnNext(item Item) {
	scs.delegate.OnNext(item)
}

func (scs *subscriptionCapturingSubscriber) OnError(err error) {
	scs.delegate.OnError(err)
}

func (scs *subscriptionCapturingSubscriber) OnComplete() {
	scs.delegate.OnComplete()
}
