// ConnectableObservable implementation for RxGo
// 实现ConnectableObservable，独立类型，支持多播
package rxgo

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// ConnectableObservable 实现
// ============================================================================

// connectableObservableImpl ConnectableObservable的核心实现
type connectableObservableImpl struct {
	source     Observable
	subject    *PublishSubject
	connected  int32
	connection Disposable
	mu         sync.RWMutex
	disposed   int32
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// NewConnectableObservable 创建新的ConnectableObservable
func NewConnectableObservable(source Observable) ConnectableObservable {
	ctx, cancel := context.WithCancel(context.Background())

	return &connectableObservableImpl{
		source:     source,
		subject:    NewPublishSubject(),
		ctx:        ctx,
		cancelFunc: cancel,
	}
}

// ============================================================================
// Observable 接口实现
// ============================================================================

// Subscribe 订阅观察者
func (co *connectableObservableImpl) Subscribe(observer Observer) Subscription {
	if co.IsDisposed() {
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	}

	return co.subject.Subscribe(observer)
}

// SubscribeWithCallbacks 使用回调函数订阅
func (co *connectableObservableImpl) SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) Subscription {
	return co.subject.SubscribeWithCallbacks(onNext, onError, onComplete)
}

// SubscribeOn 指定订阅时使用的调度器
func (co *connectableObservableImpl) SubscribeOn(scheduler Scheduler) Observable {
	return co.subject.SubscribeOn(scheduler)
}

// ObserveOn 指定观察时使用的调度器
func (co *connectableObservableImpl) ObserveOn(scheduler Scheduler) Observable {
	return co.subject.ObserveOn(scheduler)
}

// ============================================================================
// ConnectableObservable 接口实现
// ============================================================================

// Connect 开始发射数据给订阅者
func (co *connectableObservableImpl) Connect() Disposable {
	return co.ConnectWithContext(co.ctx)
}

// ConnectWithContext 带上下文的连接
func (co *connectableObservableImpl) ConnectWithContext(ctx context.Context) Disposable {
	if co.IsDisposed() {
		return NewBaseDisposable(func() {})
	}

	co.mu.Lock()
	defer co.mu.Unlock()

	// 如果已经连接，返回现有连接
	if atomic.LoadInt32(&co.connected) == 1 {
		return co.connection
	}

	// 设置连接状态
	atomic.StoreInt32(&co.connected, 1)

	// 创建连接的上下文
	connCtx, connCancel := context.WithCancel(ctx)

	// 订阅源Observable并将数据转发给subject
	sourceSubscription := co.source.Subscribe(func(item Item) {
		select {
		case <-connCtx.Done():
			return
		default:
			// 转发给subject
			if item.IsError() {
				co.subject.OnError(item.Error)
			} else if item.Value == nil {
				co.subject.OnComplete()
			} else {
				co.subject.OnNext(item.Value)
			}
		}
	})

	// 创建连接的disposable
	co.connection = NewBaseDisposable(func() {
		connCancel()
		sourceSubscription.Unsubscribe()
		atomic.StoreInt32(&co.connected, 0)
	})

	return co.connection
}

// IsConnected 检查是否已连接
func (co *connectableObservableImpl) IsConnected() bool {
	return atomic.LoadInt32(&co.connected) == 1
}

// RefCount 返回一个自动连接/断开的Observable
func (co *connectableObservableImpl) RefCount() Observable {
	return NewObservable(func(observer Observer) Subscription {
		var connection Disposable
		var connectionMu sync.Mutex

		// 检查是否需要连接
		connectionMu.Lock()
		if !co.IsConnected() {
			connection = co.Connect()
		}
		connectionMu.Unlock()

		// 订阅subject
		subscription := co.subject.Subscribe(observer)

		// 创建取消订阅的disposable
		return NewBaseSubscription(NewBaseDisposable(func() {
			subscription.Unsubscribe()

			// 如果没有其他订阅者，断开连接
			connectionMu.Lock()
			if !co.subject.HasObservers() && connection != nil {
				connection.Dispose()
			}
			connectionMu.Unlock()
		}))
	})
}

// AutoConnect 当有指定数量的订阅者时自动连接
func (co *connectableObservableImpl) AutoConnect(subscriberCount int) Observable {
	return NewObservable(func(observer Observer) Subscription {
		// 订阅subject
		subscription := co.subject.Subscribe(observer)

		// 检查是否达到自动连接的订阅者数量
		if co.subject.ObserverCount() >= subscriberCount && !co.IsConnected() {
			go co.Connect()
		}

		return subscription
	})
}

// IsDisposed 检查是否已释放
func (co *connectableObservableImpl) IsDisposed() bool {
	return atomic.LoadInt32(&co.disposed) == 1
}

// Dispose 释放资源
func (co *connectableObservableImpl) Dispose() {
	if atomic.CompareAndSwapInt32(&co.disposed, 0, 1) {
		co.mu.Lock()
		defer co.mu.Unlock()

		if co.connection != nil {
			co.connection.Dispose()
		}

		co.subject.Dispose()

		if co.cancelFunc != nil {
			co.cancelFunc()
		}
	}
}

// ============================================================================
// 委托给subject的Observable方法
// ============================================================================

func (co *connectableObservableImpl) Map(transformer Transformer) Observable {
	return co.subject.Map(transformer)
}

func (co *connectableObservableImpl) Filter(predicate Predicate) Observable {
	return co.subject.Filter(predicate)
}

func (co *connectableObservableImpl) Take(count int) Observable {
	return co.subject.Take(count)
}

func (co *connectableObservableImpl) FlatMap(transformer func(interface{}) Observable) Observable {
	return co.subject.FlatMap(transformer)
}

func (co *connectableObservableImpl) Skip(count int) Observable {
	return co.subject.Skip(count)
}

func (co *connectableObservableImpl) TakeLast(count int) Observable {
	return co.subject.TakeLast(count)
}

func (co *connectableObservableImpl) TakeUntil(other Observable) Observable {
	return co.subject.TakeUntil(other)
}

func (co *connectableObservableImpl) TakeWhile(predicate Predicate) Observable {
	return co.subject.TakeWhile(predicate)
}

func (co *connectableObservableImpl) SkipLast(count int) Observable {
	return co.subject.SkipLast(count)
}

func (co *connectableObservableImpl) SkipUntil(other Observable) Observable {
	return co.subject.SkipUntil(other)
}

func (co *connectableObservableImpl) SkipWhile(predicate Predicate) Observable {
	return co.subject.SkipWhile(predicate)
}

func (co *connectableObservableImpl) Distinct() Observable {
	return co.subject.Distinct()
}

func (co *connectableObservableImpl) DistinctUntilChanged() Observable {
	return co.subject.DistinctUntilChanged()
}

func (co *connectableObservableImpl) Merge(other Observable) Observable {
	return co.subject.Merge(other)
}

func (co *connectableObservableImpl) Concat(other Observable) Observable {
	return co.subject.Concat(other)
}

func (co *connectableObservableImpl) Zip(other Observable, zipper func(interface{}, interface{}) interface{}) Observable {
	return co.subject.Zip(other, zipper)
}

func (co *connectableObservableImpl) CombineLatest(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return co.subject.CombineLatest(other, combiner)
}

func (co *connectableObservableImpl) Reduce(reducer Reducer) Single {
	return co.subject.Reduce(reducer)
}

func (co *connectableObservableImpl) Scan(reducer Reducer) Observable {
	return co.subject.Scan(reducer)
}

func (co *connectableObservableImpl) Count() Single {
	return co.subject.Count()
}

func (co *connectableObservableImpl) First() Single {
	return co.subject.First()
}

func (co *connectableObservableImpl) Last() Single {
	return co.subject.Last()
}

func (co *connectableObservableImpl) Min() Single {
	return co.subject.Min()
}

func (co *connectableObservableImpl) Max() Single {
	return co.subject.Max()
}

func (co *connectableObservableImpl) Sum() Single {
	return co.subject.Sum()
}

func (co *connectableObservableImpl) Average() Single {
	return co.subject.Average()
}

func (co *connectableObservableImpl) All(predicate Predicate) Single {
	return co.subject.All(predicate)
}

func (co *connectableObservableImpl) Any(predicate Predicate) Single {
	return co.subject.Any(predicate)
}

func (co *connectableObservableImpl) Contains(value interface{}) Single {
	return co.subject.Contains(value)
}

func (co *connectableObservableImpl) ElementAt(index int) Single {
	return co.subject.ElementAt(index)
}

func (co *connectableObservableImpl) Buffer(count int) Observable {
	return co.subject.Buffer(count)
}

func (co *connectableObservableImpl) BufferWithTime(timespan time.Duration) Observable {
	return co.subject.BufferWithTime(timespan)
}

func (co *connectableObservableImpl) Window(count int) Observable {
	return co.subject.Window(count)
}

func (co *connectableObservableImpl) WindowWithTime(timespan time.Duration) Observable {
	return co.subject.WindowWithTime(timespan)
}

func (co *connectableObservableImpl) Debounce(duration time.Duration) Observable {
	return co.subject.Debounce(duration)
}

func (co *connectableObservableImpl) Throttle(duration time.Duration) Observable {
	return co.subject.Throttle(duration)
}

func (co *connectableObservableImpl) Delay(duration time.Duration) Observable {
	return co.subject.Delay(duration)
}

func (co *connectableObservableImpl) Timeout(duration time.Duration) Observable {
	return co.subject.Timeout(duration)
}

func (co *connectableObservableImpl) Catch(handler func(error) Observable) Observable {
	return co.subject.Catch(handler)
}

func (co *connectableObservableImpl) Retry(count int) Observable {
	return co.subject.Retry(count)
}

func (co *connectableObservableImpl) RetryWhen(handler func(Observable) Observable) Observable {
	return co.subject.RetryWhen(handler)
}

func (co *connectableObservableImpl) DoOnNext(action OnNext) Observable {
	return co.subject.DoOnNext(action)
}

func (co *connectableObservableImpl) DoOnError(action OnError) Observable {
	return co.subject.DoOnError(action)
}

func (co *connectableObservableImpl) DoOnComplete(action OnComplete) Observable {
	return co.subject.DoOnComplete(action)
}

func (co *connectableObservableImpl) ToSlice() Single {
	return co.subject.ToSlice()
}

func (co *connectableObservableImpl) ToChannel() <-chan Item {
	return co.subject.ToChannel()
}

func (co *connectableObservableImpl) Publish() ConnectableObservable {
	return co.subject.Publish()
}

func (co *connectableObservableImpl) Share() Observable {
	return co.subject.Share()
}

func (co *connectableObservableImpl) BlockingSubscribe(observer Observer) {
	co.subject.BlockingSubscribe(observer)
}

func (co *connectableObservableImpl) BlockingFirst() (interface{}, error) {
	return co.subject.BlockingFirst()
}

func (co *connectableObservableImpl) BlockingLast() (interface{}, error) {
	return co.subject.BlockingLast()
}

// Phase 3 新增方法
func (co *connectableObservableImpl) StartWith(values ...interface{}) Observable {
	return co.subject.StartWith(values...)
}

func (co *connectableObservableImpl) StartWithIterable(iterable func() <-chan interface{}) Observable {
	return co.subject.StartWithIterable(iterable)
}

func (co *connectableObservableImpl) Timestamp() Observable {
	return co.subject.Timestamp()
}

func (co *connectableObservableImpl) TimeInterval() Observable {
	return co.subject.TimeInterval()
}

func (co *connectableObservableImpl) Cast(targetType reflect.Type) Observable {
	return co.subject.Cast(targetType)
}

func (co *connectableObservableImpl) OfType(targetType reflect.Type) Observable {
	return co.subject.OfType(targetType)
}

func (co *connectableObservableImpl) WithLatestFrom(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return co.subject.WithLatestFrom(other, combiner)
}

func (co *connectableObservableImpl) DoFinally(action func()) Observable {
	return co.subject.DoFinally(action)
}

func (co *connectableObservableImpl) DoAfterTerminate(action func()) Observable {
	return co.subject.DoAfterTerminate(action)
}

func (co *connectableObservableImpl) Cache() Observable {
	return co.subject.Cache()
}

func (co *connectableObservableImpl) CacheWithCapacity(capacity int) Observable {
	return co.subject.CacheWithCapacity(capacity)
}

func (co *connectableObservableImpl) Amb(other Observable) Observable {
	return co.subject.Amb(other)
}

// 高级组合操作符
func (co *connectableObservableImpl) Join(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, interface{}) interface{}) Observable {
	return co.subject.Join(other, leftWindowSelector, rightWindowSelector, resultSelector)
}

func (co *connectableObservableImpl) GroupJoin(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, Observable) interface{}) Observable {
	return co.subject.GroupJoin(other, leftWindowSelector, rightWindowSelector, resultSelector)
}

func (co *connectableObservableImpl) SwitchOnNext() Observable {
	return co.subject.SwitchOnNext()
}

// ============================================================================
// 工厂函数
// ============================================================================

// PublishObservable 将Observable转换为ConnectableObservable
func PublishObservable(source Observable) ConnectableObservable {
	return NewConnectableObservable(source)
}

// ShareObservable 将Observable转换为共享的Observable
func ShareObservable(source Observable) Observable {
	return NewConnectableObservable(source).RefCount()
}

// ============================================================================
// 多播策略
// ============================================================================

// MulticastStrategy 多播策略
type MulticastStrategy int

const (
	// PublishStrategy 发布策略，使用PublishSubject
	PublishStrategy MulticastStrategy = iota
	// BehaviorStrategy 行为策略，使用BehaviorSubject
	BehaviorStrategy
	// ReplayStrategy 重放策略，使用ReplaySubject
	ReplayStrategy
	// AsyncStrategy 异步策略，使用AsyncSubject
	AsyncStrategy
)

// MulticastObservable 使用指定策略创建多播Observable
func MulticastObservable(source Observable, strategy MulticastStrategy, options ...interface{}) ConnectableObservable {
	var subject Subject

	switch strategy {
	case BehaviorStrategy:
		var initialValue interface{}
		if len(options) > 0 {
			initialValue = options[0]
		}
		subject = NewBehaviorSubject(initialValue)

	case ReplayStrategy:
		bufferSize := 100 // 默认缓冲区大小
		if len(options) > 0 {
			if size, ok := options[0].(int); ok {
				bufferSize = size
			}
		}
		subject = NewReplaySubject(bufferSize)

	case AsyncStrategy:
		subject = NewAsyncSubject()

	default: // PublishStrategy
		subject = NewPublishSubject()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &connectableObservableImpl{
		source:     source,
		subject:    subject.(*PublishSubject), // 需要类型断言，这里简化处理
		ctx:        ctx,
		cancelFunc: cancel,
	}
}
