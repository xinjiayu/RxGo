// Subject implementations for RxGo
// 实现Subject系统，包括PublishSubject、BehaviorSubject、ReplaySubject
package rxgo

import (
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// PublishSubject - 发布主题
// ============================================================================

// PublishSubject 发布主题，只向当前订阅者发送新的值
type PublishSubject struct {
	mu          sync.RWMutex
	observers   []Observer
	completed   int32
	errored     int32
	error       error
	disposed    int32
	disposables *CompositeDisposable
}

// 实现Observer函数类型，使PublishSubject可以作为Observer使用
func (ps *PublishSubject) Call(item Item) {
	if item.IsError() {
		ps.OnError(item.Error)
	} else if item.Value == nil {
		ps.OnComplete()
	} else {
		ps.OnNext(item.Value)
	}
}

// AsObserver 返回Observer函数
func (ps *PublishSubject) AsObserver() Observer {
	return ps.Call
}

// NewPublishSubject 创建新的发布主题
func NewPublishSubject() *PublishSubject {
	return &PublishSubject{
		observers:   make([]Observer, 0),
		disposables: NewCompositeDisposable(),
	}
}

// Subscribe 订阅观察者
func (ps *PublishSubject) Subscribe(observer Observer) Subscription {
	if ps.IsDisposed() {
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()

	// 如果已经完成或出错，立即通知观察者
	if atomic.LoadInt32(&ps.completed) == 1 {
		go observer(CreateItem(nil)) // 发送完成信号
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	}

	if atomic.LoadInt32(&ps.errored) == 1 {
		go observer(CreateErrorItem(ps.error))
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	}

	// 添加观察者
	ps.observers = append(ps.observers, observer)
	index := len(ps.observers) - 1

	// 创建取消订阅的disposable
	disposable := NewBaseDisposable(func() {
		ps.removeObserver(index)
	})

	return NewBaseSubscription(disposable)
}

// SubscribeWithCallbacks 使用回调函数订阅
func (ps *PublishSubject) SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) Subscription {
	observer := func(item Item) {
		if item.IsError() {
			if onError != nil {
				onError(item.Error)
			}
		} else if item.Value == nil && atomic.LoadInt32(&ps.completed) == 1 {
			if onComplete != nil {
				onComplete()
			}
		} else {
			if onNext != nil {
				onNext(item.Value)
			}
		}
	}
	return ps.Subscribe(observer)
}

// OnNext 发送下一个值
func (ps *PublishSubject) OnNext(value interface{}) {
	if ps.IsDisposed() || atomic.LoadInt32(&ps.completed) == 1 || atomic.LoadInt32(&ps.errored) == 1 {
		return
	}

	ps.mu.RLock()
	observers := make([]Observer, len(ps.observers))
	copy(observers, ps.observers)
	ps.mu.RUnlock()

	item := CreateItem(value)
	// 同步调用观察者以保证顺序和完整性
	for _, observer := range observers {
		observer(item)
	}
}

// OnError 发送错误
func (ps *PublishSubject) OnError(err error) {
	if ps.IsDisposed() || atomic.LoadInt32(&ps.completed) == 1 || atomic.LoadInt32(&ps.errored) == 1 {
		return
	}

	if atomic.CompareAndSwapInt32(&ps.errored, 0, 1) {
		ps.error = err

		ps.mu.RLock()
		observers := make([]Observer, len(ps.observers))
		copy(observers, ps.observers)
		ps.mu.RUnlock()

		item := CreateErrorItem(err)
		// 同步调用观察者以保证错误立即传播
		for _, observer := range observers {
			observer(item)
		}
	}
}

// OnComplete 发送完成信号
func (ps *PublishSubject) OnComplete() {
	if ps.IsDisposed() || atomic.LoadInt32(&ps.completed) == 1 || atomic.LoadInt32(&ps.errored) == 1 {
		return
	}

	if atomic.CompareAndSwapInt32(&ps.completed, 0, 1) {
		ps.mu.RLock()
		observers := make([]Observer, len(ps.observers))
		copy(observers, ps.observers)
		ps.mu.RUnlock()

		item := CreateItem(nil) // 使用nil表示完成
		// 同步调用观察者以保证完成信号的即时性
		for _, observer := range observers {
			observer(item)
		}
	}
}

// HasObservers 检查是否有观察者
func (ps *PublishSubject) HasObservers() bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.observers) > 0
}

// ObserverCount 获取观察者数量
func (ps *PublishSubject) ObserverCount() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.observers)
}

// IsDisposed 检查是否已释放
func (ps *PublishSubject) IsDisposed() bool {
	return atomic.LoadInt32(&ps.disposed) == 1
}

// Dispose 释放资源
func (ps *PublishSubject) Dispose() {
	if atomic.CompareAndSwapInt32(&ps.disposed, 0, 1) {
		ps.disposables.Dispose()
		ps.mu.Lock()
		ps.observers = nil
		ps.mu.Unlock()
	}
}

// removeObserver 移除观察者
func (ps *PublishSubject) removeObserver(index int) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if index < len(ps.observers) {
		ps.observers = append(ps.observers[:index], ps.observers[index+1:]...)
	}
}

// 实现Observable接口的其他方法（这里只实现核心方法，其他方法会委托给基础实现）
func (ps *PublishSubject) SubscribeOn(scheduler Scheduler) Observable {
	return &observableWrapper{subject: ps, scheduler: scheduler}
}

func (ps *PublishSubject) ObserveOn(scheduler Scheduler) Observable {
	return &observableWrapper{subject: ps, scheduler: scheduler}
}

// 实现Observable接口的所有方法（委托给observableWrapper）
func (ps *PublishSubject) Map(transformer Transformer) Observable {
	return (&observableWrapper{subject: ps}).Map(transformer)
}

func (ps *PublishSubject) Filter(predicate Predicate) Observable {
	return (&observableWrapper{subject: ps}).Filter(predicate)
}

func (ps *PublishSubject) Take(count int) Observable {
	return (&observableWrapper{subject: ps}).Take(count)
}

func (ps *PublishSubject) FlatMap(transformer func(interface{}) Observable) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) Skip(count int) Observable     { return &observableWrapper{subject: ps} }
func (ps *PublishSubject) TakeLast(count int) Observable { return &observableWrapper{subject: ps} }
func (ps *PublishSubject) TakeUntil(other Observable) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) TakeWhile(predicate Predicate) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) SkipLast(count int) Observable { return &observableWrapper{subject: ps} }
func (ps *PublishSubject) SkipUntil(other Observable) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) SkipWhile(predicate Predicate) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) Distinct() Observable               { return &observableWrapper{subject: ps} }
func (ps *PublishSubject) DistinctUntilChanged() Observable   { return &observableWrapper{subject: ps} }
func (ps *PublishSubject) Merge(other Observable) Observable  { return &observableWrapper{subject: ps} }
func (ps *PublishSubject) Concat(other Observable) Observable { return &observableWrapper{subject: ps} }
func (ps *PublishSubject) Zip(other Observable, zipper func(interface{}, interface{}) interface{}) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) CombineLatest(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) Reduce(reducer Reducer) Single     { return nil }
func (ps *PublishSubject) Scan(reducer Reducer) Observable   { return &observableWrapper{subject: ps} }
func (ps *PublishSubject) Count() Single                     { return nil }
func (ps *PublishSubject) First() Single                     { return nil }
func (ps *PublishSubject) Last() Single                      { return nil }
func (ps *PublishSubject) Min() Single                       { return nil }
func (ps *PublishSubject) Max() Single                       { return nil }
func (ps *PublishSubject) Sum() Single                       { return nil }
func (ps *PublishSubject) Average() Single                   { return nil }
func (ps *PublishSubject) All(predicate Predicate) Single    { return nil }
func (ps *PublishSubject) Any(predicate Predicate) Single    { return nil }
func (ps *PublishSubject) Contains(value interface{}) Single { return nil }
func (ps *PublishSubject) ElementAt(index int) Single        { return nil }
func (ps *PublishSubject) Buffer(count int) Observable       { return &observableWrapper{subject: ps} }
func (ps *PublishSubject) BufferWithTime(timespan time.Duration) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) Window(count int) Observable { return &observableWrapper{subject: ps} }
func (ps *PublishSubject) WindowWithTime(timespan time.Duration) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) Debounce(duration time.Duration) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) Throttle(duration time.Duration) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) Delay(duration time.Duration) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) Timeout(duration time.Duration) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) Catch(handler func(error) Observable) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) Retry(count int) Observable { return &observableWrapper{subject: ps} }
func (ps *PublishSubject) RetryWhen(handler func(Observable) Observable) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) DoOnNext(action OnNext) Observable { return &observableWrapper{subject: ps} }
func (ps *PublishSubject) DoOnError(action OnError) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) DoOnComplete(action OnComplete) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) ToSlice() Single                     { return nil }
func (ps *PublishSubject) ToChannel() <-chan Item              { return nil }
func (ps *PublishSubject) Publish() ConnectableObservable      { return nil }
func (ps *PublishSubject) Share() Observable                   { return &observableWrapper{subject: ps} }
func (ps *PublishSubject) BlockingSubscribe(observer Observer) {}
func (ps *PublishSubject) BlockingFirst() (interface{}, error) { return nil, nil }
func (ps *PublishSubject) BlockingLast() (interface{}, error)  { return nil, nil }

// Phase 3 新增方法
func (ps *PublishSubject) StartWith(values ...interface{}) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) StartWithIterable(iterable func() <-chan interface{}) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) Timestamp() Observable    { return &observableWrapper{subject: ps} }
func (ps *PublishSubject) TimeInterval() Observable { return &observableWrapper{subject: ps} }
func (ps *PublishSubject) Cast(targetType reflect.Type) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) OfType(targetType reflect.Type) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) WithLatestFrom(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) DoFinally(action func()) Observable { return &observableWrapper{subject: ps} }
func (ps *PublishSubject) DoAfterTerminate(action func()) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) Cache() Observable { return &observableWrapper{subject: ps} }
func (ps *PublishSubject) CacheWithCapacity(capacity int) Observable {
	return &observableWrapper{subject: ps}
}
func (ps *PublishSubject) Amb(other Observable) Observable {
	return &observableWrapper{subject: ps}
}

// 高级组合操作符
func (ps *PublishSubject) Join(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, interface{}) interface{}) Observable {
	return &observableWrapper{subject: ps}
}

func (ps *PublishSubject) GroupJoin(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, Observable) interface{}) Observable {
	return &observableWrapper{subject: ps}
}

func (ps *PublishSubject) SwitchOnNext() Observable {
	return &observableWrapper{subject: ps}
}

// ============================================================================
// BehaviorSubject - 行为主题
// ============================================================================

// BehaviorSubject 行为主题，保存最后一个值，新订阅者会立即收到最后的值
type BehaviorSubject struct {
	*PublishSubject
	hasValue     int32
	currentValue interface{}
}

// NewBehaviorSubject 创建新的行为主题
func NewBehaviorSubject(initialValue interface{}) *BehaviorSubject {
	return &BehaviorSubject{
		PublishSubject: NewPublishSubject(),
		hasValue:       1,
		currentValue:   initialValue,
	}
}

// SubscribeWithCallbacks 使用回调函数订阅，立即发送当前值
func (bs *BehaviorSubject) SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) Subscription {
	if bs.IsDisposed() {
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	}

	// 如果有当前值，立即发送
	if atomic.LoadInt32(&bs.hasValue) == 1 && onNext != nil {
		onNext(bs.currentValue)
	}

	return bs.PublishSubject.SubscribeWithCallbacks(onNext, onError, onComplete)
}

// Subscribe 订阅观察者，立即发送当前值
func (bs *BehaviorSubject) Subscribe(observer Observer) Subscription {
	if bs.IsDisposed() {
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	}

	// 如果有当前值，立即发送（同步发送以避免竞态条件）
	if atomic.LoadInt32(&bs.hasValue) == 1 {
		observer(CreateItem(bs.currentValue))
	}

	return bs.PublishSubject.Subscribe(observer)
}

// OnNext 发送下一个值并更新当前值
func (bs *BehaviorSubject) OnNext(value interface{}) {
	if bs.IsDisposed() || atomic.LoadInt32(&bs.completed) == 1 || atomic.LoadInt32(&bs.errored) == 1 {
		return
	}

	// 更新当前值
	bs.currentValue = value
	atomic.StoreInt32(&bs.hasValue, 1)

	// 调用父类方法发送给所有观察者
	bs.PublishSubject.OnNext(value)
}

// GetValue 获取当前值
func (bs *BehaviorSubject) GetValue() (interface{}, bool) {
	if atomic.LoadInt32(&bs.hasValue) == 1 {
		return bs.currentValue, true
	}
	return nil, false
}

// ============================================================================
// ReplaySubject - 重放主题
// ============================================================================

// ReplaySubject 重放主题，缓存指定数量的值，新订阅者会收到所有缓存的值
type ReplaySubject struct {
	*PublishSubject
	bufferSize int
	buffer     []interface{}
	bufferMu   sync.RWMutex
}

// NewReplaySubject 创建新的重放主题
func NewReplaySubject(bufferSize int) *ReplaySubject {
	if bufferSize <= 0 {
		bufferSize = 100 // 默认缓存100个值
	}

	return &ReplaySubject{
		PublishSubject: NewPublishSubject(),
		bufferSize:     bufferSize,
		buffer:         make([]interface{}, 0, bufferSize),
	}
}

// Subscribe 订阅观察者，立即发送所有缓存的值
func (rs *ReplaySubject) Subscribe(observer Observer) Subscription {
	if rs.IsDisposed() {
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	}

	// 发送所有缓存的值
	rs.bufferMu.RLock()
	bufferedValues := make([]interface{}, len(rs.buffer))
	copy(bufferedValues, rs.buffer)
	rs.bufferMu.RUnlock()

	// 同步发送缓存的值以保证顺序
	for _, value := range bufferedValues {
		observer(CreateItem(value))
	}

	return rs.PublishSubject.Subscribe(observer)
}

// SubscribeWithCallbacks 使用回调函数订阅，立即发送所有缓存的值
func (rs *ReplaySubject) SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) Subscription {
	if rs.IsDisposed() {
		return NewBaseSubscription(NewBaseDisposable(func() {}))
	}

	// 发送所有缓存的值
	rs.bufferMu.RLock()
	bufferedValues := make([]interface{}, len(rs.buffer))
	copy(bufferedValues, rs.buffer)
	rs.bufferMu.RUnlock()

	// 同步发送缓存的值
	if onNext != nil {
		for _, value := range bufferedValues {
			onNext(value)
		}
	}

	// 然后订阅后续的值
	return rs.PublishSubject.SubscribeWithCallbacks(onNext, onError, onComplete)
}

// OnNext 发送下一个值并添加到缓存
func (rs *ReplaySubject) OnNext(value interface{}) {
	if rs.IsDisposed() || atomic.LoadInt32(&rs.completed) == 1 || atomic.LoadInt32(&rs.errored) == 1 {
		return
	}

	// 添加到缓存
	rs.bufferMu.Lock()
	if len(rs.buffer) >= rs.bufferSize {
		// 移除最老的值
		rs.buffer = rs.buffer[1:]
	}
	rs.buffer = append(rs.buffer, value)
	rs.bufferMu.Unlock()

	// 调用父类方法发送给所有观察者
	rs.PublishSubject.OnNext(value)
}

// GetBufferedValues 获取所有缓存的值
func (rs *ReplaySubject) GetBufferedValues() []interface{} {
	rs.bufferMu.RLock()
	defer rs.bufferMu.RUnlock()

	result := make([]interface{}, len(rs.buffer))
	copy(result, rs.buffer)
	return result
}

// ============================================================================
// AsyncSubject - 异步主题
// ============================================================================

// AsyncSubject 异步主题，只在完成时发送最后一个值
type AsyncSubject struct {
	*PublishSubject
	lastValue interface{}
	hasValue  int32
}

// NewAsyncSubject 创建新的异步主题
func NewAsyncSubject() *AsyncSubject {
	return &AsyncSubject{
		PublishSubject: NewPublishSubject(),
	}
}

// OnNext 记录最后一个值但不立即发送
func (as *AsyncSubject) OnNext(value interface{}) {
	if as.IsDisposed() || atomic.LoadInt32(&as.completed) == 1 || atomic.LoadInt32(&as.errored) == 1 {
		return
	}

	as.lastValue = value
	atomic.StoreInt32(&as.hasValue, 1)
}

// OnComplete 发送最后一个值然后完成
func (as *AsyncSubject) OnComplete() {
	if as.IsDisposed() || atomic.LoadInt32(&as.completed) == 1 || atomic.LoadInt32(&as.errored) == 1 {
		return
	}

	if atomic.CompareAndSwapInt32(&as.completed, 0, 1) {
		as.mu.RLock()
		observers := make([]Observer, len(as.observers))
		copy(observers, as.observers)
		as.mu.RUnlock()

		// 如果有最后的值，先发送它
		if atomic.LoadInt32(&as.hasValue) == 1 {
			item := CreateItem(as.lastValue)
			// 同步发送最后的值以保证顺序
			for _, observer := range observers {
				observer(item)
			}
		}

		// 然后发送完成信号
		item := CreateItem(nil)
		// 同步发送完成信号
		for _, observer := range observers {
			observer(item)
		}
	}
}

// ============================================================================
// observableWrapper - Observable包装器
// ============================================================================

// observableWrapper 用于包装Subject以实现完整的Observable接口
type observableWrapper struct {
	subject   Subject
	scheduler Scheduler
}

// 实现Observable接口的方法（这里只实现几个核心方法作为示例）
func (ow *observableWrapper) Subscribe(observer Observer) Subscription {
	if ow.scheduler != nil {
		wrappedObserver := func(item Item) {
			ow.scheduler.Schedule(func() {
				observer(item)
			})
		}
		return ow.subject.Subscribe(wrappedObserver)
	}
	return ow.subject.Subscribe(observer)
}

func (ow *observableWrapper) SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) Subscription {
	return ow.subject.SubscribeWithCallbacks(onNext, onError, onComplete)
}

func (ow *observableWrapper) SubscribeOn(scheduler Scheduler) Observable {
	return &observableWrapper{subject: ow.subject, scheduler: scheduler}
}

func (ow *observableWrapper) ObserveOn(scheduler Scheduler) Observable {
	return &observableWrapper{subject: ow.subject, scheduler: scheduler}
}

// Map 转换操作符的基础实现
func (ow *observableWrapper) Map(transformer Transformer) Observable {
	return Create(func(observer Observer) {
		ow.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				observer(item) // 完成信号
				return
			}

			if result, err := transformer(item.Value); err != nil {
				observer(CreateErrorItem(err))
			} else {
				observer(CreateItem(result))
			}
		})
	})
}

// 其他Observable方法的占位符实现
func (ow *observableWrapper) Filter(predicate Predicate) Observable {
	return Create(func(observer Observer) {
		ow.Subscribe(func(item Item) {
			if item.IsError() || item.Value == nil {
				observer(item)
				return
			}

			if predicate(item.Value) {
				observer(item)
			}
		})
	})
}

func (ow *observableWrapper) Take(count int) Observable {
	return Create(func(observer Observer) {
		taken := 0
		ow.Subscribe(func(item Item) {
			if taken >= count {
				return
			}

			if item.IsError() || item.Value == nil {
				observer(item)
				return
			}

			taken++
			observer(item)

			if taken >= count {
				observer(CreateItem(nil)) // 发送完成信号
			}
		})
	})
}

// 占位符实现其他方法
func (ow *observableWrapper) FlatMap(transformer func(interface{}) Observable) Observable { return ow }
func (ow *observableWrapper) Skip(count int) Observable                                   { return ow }
func (ow *observableWrapper) TakeLast(count int) Observable                               { return ow }
func (ow *observableWrapper) TakeUntil(other Observable) Observable                       { return ow }
func (ow *observableWrapper) TakeWhile(predicate Predicate) Observable                    { return ow }
func (ow *observableWrapper) SkipLast(count int) Observable                               { return ow }
func (ow *observableWrapper) SkipUntil(other Observable) Observable                       { return ow }
func (ow *observableWrapper) SkipWhile(predicate Predicate) Observable                    { return ow }
func (ow *observableWrapper) Distinct() Observable                                        { return ow }
func (ow *observableWrapper) DistinctUntilChanged() Observable                            { return ow }
func (ow *observableWrapper) Merge(other Observable) Observable                           { return ow }
func (ow *observableWrapper) Concat(other Observable) Observable                          { return ow }
func (ow *observableWrapper) Zip(other Observable, zipper func(interface{}, interface{}) interface{}) Observable {
	return ow
}
func (ow *observableWrapper) CombineLatest(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return ow
}
func (ow *observableWrapper) Reduce(reducer Reducer) Single                            { return nil }
func (ow *observableWrapper) Scan(reducer Reducer) Observable                          { return ow }
func (ow *observableWrapper) Count() Single                                            { return nil }
func (ow *observableWrapper) First() Single                                            { return nil }
func (ow *observableWrapper) Last() Single                                             { return nil }
func (ow *observableWrapper) Min() Single                                              { return nil }
func (ow *observableWrapper) Max() Single                                              { return nil }
func (ow *observableWrapper) Sum() Single                                              { return nil }
func (ow *observableWrapper) Average() Single                                          { return nil }
func (ow *observableWrapper) All(predicate Predicate) Single                           { return nil }
func (ow *observableWrapper) Any(predicate Predicate) Single                           { return nil }
func (ow *observableWrapper) Contains(value interface{}) Single                        { return nil }
func (ow *observableWrapper) ElementAt(index int) Single                               { return nil }
func (ow *observableWrapper) Buffer(count int) Observable                              { return ow }
func (ow *observableWrapper) BufferWithTime(timespan time.Duration) Observable         { return ow }
func (ow *observableWrapper) Window(count int) Observable                              { return ow }
func (ow *observableWrapper) WindowWithTime(timespan time.Duration) Observable         { return ow }
func (ow *observableWrapper) Debounce(duration time.Duration) Observable               { return ow }
func (ow *observableWrapper) Throttle(duration time.Duration) Observable               { return ow }
func (ow *observableWrapper) Delay(duration time.Duration) Observable                  { return ow }
func (ow *observableWrapper) Timeout(duration time.Duration) Observable                { return ow }
func (ow *observableWrapper) Catch(handler func(error) Observable) Observable          { return ow }
func (ow *observableWrapper) Retry(count int) Observable                               { return ow }
func (ow *observableWrapper) RetryWhen(handler func(Observable) Observable) Observable { return ow }
func (ow *observableWrapper) DoOnNext(action OnNext) Observable                        { return ow }
func (ow *observableWrapper) DoOnError(action OnError) Observable                      { return ow }
func (ow *observableWrapper) DoOnComplete(action OnComplete) Observable                { return ow }
func (ow *observableWrapper) ToSlice() Single                                          { return nil }
func (ow *observableWrapper) ToChannel() <-chan Item                                   { return nil }
func (ow *observableWrapper) Publish() ConnectableObservable                           { return nil }
func (ow *observableWrapper) Share() Observable                                        { return ow }
func (ow *observableWrapper) BlockingSubscribe(observer Observer)                      {}
func (ow *observableWrapper) BlockingFirst() (interface{}, error)                      { return nil, nil }
func (ow *observableWrapper) BlockingLast() (interface{}, error)                       { return nil, nil }

// Phase 3 新增方法的占位符实现
func (ow *observableWrapper) StartWith(values ...interface{}) Observable { return ow }
func (ow *observableWrapper) StartWithIterable(iterable func() <-chan interface{}) Observable {
	return ow
}
func (ow *observableWrapper) Timestamp() Observable                     { return ow }
func (ow *observableWrapper) TimeInterval() Observable                  { return ow }
func (ow *observableWrapper) Cast(targetType reflect.Type) Observable   { return ow }
func (ow *observableWrapper) OfType(targetType reflect.Type) Observable { return ow }
func (ow *observableWrapper) WithLatestFrom(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return ow
}
func (ow *observableWrapper) DoFinally(action func()) Observable        { return ow }
func (ow *observableWrapper) DoAfterTerminate(action func()) Observable { return ow }
func (ow *observableWrapper) Cache() Observable                         { return ow }
func (ow *observableWrapper) CacheWithCapacity(capacity int) Observable { return ow }
func (ow *observableWrapper) Amb(other Observable) Observable           { return ow }

// 高级组合操作符
func (ow *observableWrapper) Join(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, interface{}) interface{}) Observable {
	return ow
}
func (ow *observableWrapper) GroupJoin(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, Observable) interface{}) Observable {
	return ow
}
func (ow *observableWrapper) SwitchOnNext() Observable { return ow }

// ============================================================================
// 工具函数
// ============================================================================
