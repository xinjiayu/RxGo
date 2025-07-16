package rxgo

import (
	"reflect"
	"time"
)

// ============================================================================
// Assembly-Time优化系统 - 对标RxJava的assembly-time优化
// 在组装时期进行优化，减少运行时开销
// ============================================================================

// ScalarCallable 标量可调用接口 - 对标RxJava的ScalarCallable
// 表示一个可以立即返回单个值的源
type ScalarCallable interface {
	// Call 获取标量值
	Call() interface{}

	// IsEmpty 检查是否为空值
	IsEmpty() bool
}

// EmptyCallable 空值可调用接口 - 对标RxJava的EmptyCallable
// 表示一个不发射任何值的源
type EmptyCallable interface {
	// IsEmpty 总是返回true
	IsEmpty() bool
}

// CallableWrapper 包装Observable，支持标量优化
type CallableWrapper interface {
	Observable
	ScalarCallable
}

// ============================================================================
// 标量Observable实现 - 优化的Just操作符
// ============================================================================

// scalarObservable 标量Observable实现
type scalarObservable struct {
	value interface{}
}

// NewScalarObservable 创建标量Observable
func NewScalarObservable(value interface{}) Observable {
	return &scalarObservable{value: value}
}

func (s *scalarObservable) Call() interface{} {
	return s.value
}

func (s *scalarObservable) IsEmpty() bool {
	return s.value == nil
}

func (s *scalarObservable) Subscribe(observer Observer) Subscription {
	// 直接发送值和完成信号，无需调度器
	if s.value != nil {
		observer(Item{Value: s.value, Error: nil})
	}
	observer(Item{Value: nil, Error: nil}) // 完成信号

	return &noOpSubscription{}
}

func (s *scalarObservable) SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) Subscription {
	if s.value != nil && onNext != nil {
		onNext(s.value)
	}
	if onComplete != nil {
		onComplete()
	}
	return &noOpSubscription{}
}

// Assembly-time优化：Map操作符
func (s *scalarObservable) Map(transformer Transformer) Observable {
	if s.value == nil {
		return NewEmptyObservable()
	}
	// 在assembly-time直接应用转换
	transformed, err := transformer(s.value)
	if err != nil {
		// 如果转换出错，返回错误Observable
		return Create(func(observer Observer) {
			observer(Item{Value: nil, Error: err})
		})
	}
	return NewScalarObservable(transformed)
}

// Assembly-time优化：Filter操作符
func (s *scalarObservable) Filter(predicate Predicate) Observable {
	if s.value == nil || !predicate(s.value) {
		return NewEmptyObservable()
	}
	return s
}

// Assembly-time优化：FlatMap操作符
func (s *scalarObservable) FlatMap(transformer func(interface{}) Observable) Observable {
	if s.value == nil {
		return NewEmptyObservable()
	}
	return transformer(s.value)
}

// Assembly-time优化：Take操作符
func (s *scalarObservable) Take(count int) Observable {
	if count <= 0 || s.value == nil {
		return NewEmptyObservable()
	}
	return s
}

// Assembly-time优化：Skip操作符
func (s *scalarObservable) Skip(count int) Observable {
	if count > 0 {
		return NewEmptyObservable()
	}
	return s
}

// 委托给标准实现的方法
func (s *scalarObservable) SubscribeOn(scheduler Scheduler) Observable {
	return &scheduledScalarObservable{scalar: s, scheduler: scheduler}
}

func (s *scalarObservable) ObserveOn(scheduler Scheduler) Observable {
	return &scheduledScalarObservable{scalar: s, scheduler: scheduler}
}

func (s *scalarObservable) TakeLast(count int) Observable { return s.Take(count) }
func (s *scalarObservable) TakeUntil(other Observable) Observable {
	return Just(s.value).TakeUntil(other)
}
func (s *scalarObservable) TakeWhile(predicate Predicate) Observable { return s.Filter(predicate) }
func (s *scalarObservable) SkipLast(count int) Observable            { return s.Skip(count) }
func (s *scalarObservable) SkipUntil(other Observable) Observable {
	return Just(s.value).SkipUntil(other)
}
func (s *scalarObservable) SkipWhile(predicate Predicate) Observable {
	if s.value != nil && predicate(s.value) {
		return NewEmptyObservable()
	}
	return s
}
func (s *scalarObservable) Distinct() Observable             { return s }
func (s *scalarObservable) DistinctUntilChanged() Observable { return s }
func (s *scalarObservable) Buffer(count int) Observable {
	if s.value == nil {
		return NewEmptyObservable()
	}
	return Just([]interface{}{s.value})
}
func (s *scalarObservable) BufferWithTime(timespan time.Duration) Observable { return s.Buffer(1) }
func (s *scalarObservable) Window(count int) Observable                      { return Just(s) }
func (s *scalarObservable) WindowWithTime(timespan time.Duration) Observable { return s.Window(1) }
func (s *scalarObservable) Merge(other Observable) Observable                { return Just(s.value).Merge(other) }
func (s *scalarObservable) Concat(other Observable) Observable               { return Just(s.value).Concat(other) }
func (s *scalarObservable) Zip(other Observable, zipper func(interface{}, interface{}) interface{}) Observable {
	return Just(s.value).Zip(other, zipper)
}
func (s *scalarObservable) CombineLatest(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return Just(s.value).CombineLatest(other, combiner)
}
func (s *scalarObservable) Reduce(reducer Reducer) Single   { return SingleJust(s.value) }
func (s *scalarObservable) Scan(reducer Reducer) Observable { return s }
func (s *scalarObservable) Count() Single {
	if s.value == nil {
		return SingleJust(0)
	}
	return SingleJust(1)
}
func (s *scalarObservable) First() Single   { return SingleJust(s.value) }
func (s *scalarObservable) Last() Single    { return SingleJust(s.value) }
func (s *scalarObservable) Min() Single     { return SingleJust(s.value) }
func (s *scalarObservable) Max() Single     { return SingleJust(s.value) }
func (s *scalarObservable) Sum() Single     { return SingleJust(s.value) }
func (s *scalarObservable) Average() Single { return SingleJust(s.value) }
func (s *scalarObservable) All(predicate Predicate) Single {
	result := s.value == nil || predicate(s.value)
	return SingleJust(result)
}
func (s *scalarObservable) Any(predicate Predicate) Single {
	result := s.value != nil && predicate(s.value)
	return SingleJust(result)
}
func (s *scalarObservable) Contains(value interface{}) Single {
	result := s.value == value
	return SingleJust(result)
}
func (s *scalarObservable) ElementAt(index int) Single {
	if index == 0 && s.value != nil {
		return SingleJust(s.value)
	}
	return SingleJust(nil)
}
func (s *scalarObservable) Debounce(duration time.Duration) Observable {
	return Just(s.value).Debounce(duration)
}
func (s *scalarObservable) Throttle(duration time.Duration) Observable {
	return Just(s.value).Throttle(duration)
}
func (s *scalarObservable) Delay(duration time.Duration) Observable {
	return Just(s.value).Delay(duration)
}
func (s *scalarObservable) Timeout(duration time.Duration) Observable {
	return Just(s.value).Timeout(duration)
}
func (s *scalarObservable) Catch(handler func(error) Observable) Observable          { return s }
func (s *scalarObservable) Retry(count int) Observable                               { return s }
func (s *scalarObservable) RetryWhen(handler func(Observable) Observable) Observable { return s }
func (s *scalarObservable) DoOnNext(action OnNext) Observable                        { return Just(s.value).DoOnNext(action) }
func (s *scalarObservable) DoOnError(action OnError) Observable {
	return Just(s.value).DoOnError(action)
}
func (s *scalarObservable) DoOnComplete(action OnComplete) Observable {
	return Just(s.value).DoOnComplete(action)
}
func (s *scalarObservable) ToSlice() Single {
	if s.value == nil {
		return SingleJust([]interface{}{})
	}
	return SingleJust([]interface{}{s.value})
}
func (s *scalarObservable) ToChannel() <-chan Item {
	ch := make(chan Item, 2)
	if s.value != nil {
		ch <- Item{Value: s.value, Error: nil}
	}
	ch <- Item{Value: nil, Error: nil}
	close(ch)
	return ch
}
func (s *scalarObservable) Publish() ConnectableObservable      { return Just(s.value).Publish() }
func (s *scalarObservable) Share() Observable                   { return Just(s.value).Share() }
func (s *scalarObservable) BlockingSubscribe(observer Observer) { s.Subscribe(observer) }
func (s *scalarObservable) BlockingFirst() (interface{}, error) { return s.value, nil }
func (s *scalarObservable) BlockingLast() (interface{}, error)  { return s.value, nil }
func (s *scalarObservable) StartWith(values ...interface{}) Observable {
	return Just(s.value).StartWith(values...)
}
func (s *scalarObservable) StartWithIterable(iterable func() <-chan interface{}) Observable {
	return Just(s.value).StartWithIterable(iterable)
}
func (s *scalarObservable) Timestamp() Observable    { return Just(s.value).Timestamp() }
func (s *scalarObservable) TimeInterval() Observable { return Just(s.value).TimeInterval() }
func (s *scalarObservable) Cast(targetType reflect.Type) Observable {
	return Just(s.value).Cast(targetType)
}
func (s *scalarObservable) OfType(targetType reflect.Type) Observable {
	return Just(s.value).OfType(targetType)
}
func (s *scalarObservable) WithLatestFrom(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return Just(s.value).WithLatestFrom(other, combiner)
}
func (s *scalarObservable) DoFinally(action func()) Observable {
	return Just(s.value).DoFinally(action)
}
func (s *scalarObservable) DoAfterTerminate(action func()) Observable {
	return Just(s.value).DoAfterTerminate(action)
}
func (s *scalarObservable) Cache() Observable                         { return s }
func (s *scalarObservable) CacheWithCapacity(capacity int) Observable { return s }
func (s *scalarObservable) Amb(other Observable) Observable           { return Just(s.value).Amb(other) }
func (s *scalarObservable) Join(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, interface{}) interface{}) Observable {
	return Just(s.value).Join(other, leftWindowSelector, rightWindowSelector, resultSelector)
}
func (s *scalarObservable) GroupJoin(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, Observable) interface{}) Observable {
	return Just(s.value).GroupJoin(other, leftWindowSelector, rightWindowSelector, resultSelector)
}
func (s *scalarObservable) SwitchOnNext() Observable { return Just(s.value).SwitchOnNext() }

// ============================================================================
// 空Observable实现 - 优化的Empty操作符
// ============================================================================

// emptyObservable 空Observable实现
type emptyObservable struct{}

// NewEmptyObservable 创建空Observable
func NewEmptyObservable() Observable {
	return &emptyObservable{}
}

func (e *emptyObservable) IsEmpty() bool {
	return true
}

func (e *emptyObservable) Subscribe(observer Observer) Subscription {
	// 直接发送完成信号
	observer(Item{Value: nil, Error: nil})
	return &noOpSubscription{}
}

func (e *emptyObservable) SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) Subscription {
	if onComplete != nil {
		onComplete()
	}
	return &noOpSubscription{}
}

// Assembly-time优化：所有转换操作符都返回空Observable
func (e *emptyObservable) Map(transformer Transformer) Observable                      { return e }
func (e *emptyObservable) Filter(predicate Predicate) Observable                       { return e }
func (e *emptyObservable) FlatMap(transformer func(interface{}) Observable) Observable { return e }
func (e *emptyObservable) Take(count int) Observable                                   { return e }
func (e *emptyObservable) TakeLast(count int) Observable                               { return e }
func (e *emptyObservable) TakeUntil(other Observable) Observable                       { return e }
func (e *emptyObservable) TakeWhile(predicate Predicate) Observable                    { return e }
func (e *emptyObservable) Skip(count int) Observable                                   { return e }
func (e *emptyObservable) SkipLast(count int) Observable                               { return e }
func (e *emptyObservable) SkipUntil(other Observable) Observable                       { return e }
func (e *emptyObservable) SkipWhile(predicate Predicate) Observable                    { return e }
func (e *emptyObservable) Distinct() Observable                                        { return e }
func (e *emptyObservable) DistinctUntilChanged() Observable                            { return e }
func (e *emptyObservable) Buffer(count int) Observable                                 { return e }
func (e *emptyObservable) BufferWithTime(timespan time.Duration) Observable            { return e }
func (e *emptyObservable) Window(count int) Observable                                 { return e }
func (e *emptyObservable) WindowWithTime(timespan time.Duration) Observable            { return e }

func (e *emptyObservable) SubscribeOn(scheduler Scheduler) Observable { return e }
func (e *emptyObservable) ObserveOn(scheduler Scheduler) Observable   { return e }
func (e *emptyObservable) Merge(other Observable) Observable          { return other }
func (e *emptyObservable) Concat(other Observable) Observable         { return other }
func (e *emptyObservable) Zip(other Observable, zipper func(interface{}, interface{}) interface{}) Observable {
	return e
}
func (e *emptyObservable) CombineLatest(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return e
}
func (e *emptyObservable) Reduce(reducer Reducer) Single                            { return SingleJust(nil) }
func (e *emptyObservable) Scan(reducer Reducer) Observable                          { return e }
func (e *emptyObservable) Count() Single                                            { return SingleJust(0) }
func (e *emptyObservable) First() Single                                            { return SingleJust(nil) }
func (e *emptyObservable) Last() Single                                             { return SingleJust(nil) }
func (e *emptyObservable) Min() Single                                              { return SingleJust(nil) }
func (e *emptyObservable) Max() Single                                              { return SingleJust(nil) }
func (e *emptyObservable) Sum() Single                                              { return SingleJust(0) }
func (e *emptyObservable) Average() Single                                          { return SingleJust(0) }
func (e *emptyObservable) All(predicate Predicate) Single                           { return SingleJust(true) }
func (e *emptyObservable) Any(predicate Predicate) Single                           { return SingleJust(false) }
func (e *emptyObservable) Contains(value interface{}) Single                        { return SingleJust(false) }
func (e *emptyObservable) ElementAt(index int) Single                               { return SingleJust(nil) }
func (e *emptyObservable) Debounce(duration time.Duration) Observable               { return e }
func (e *emptyObservable) Throttle(duration time.Duration) Observable               { return e }
func (e *emptyObservable) Delay(duration time.Duration) Observable                  { return e }
func (e *emptyObservable) Timeout(duration time.Duration) Observable                { return e }
func (e *emptyObservable) Catch(handler func(error) Observable) Observable          { return e }
func (e *emptyObservable) Retry(count int) Observable                               { return e }
func (e *emptyObservable) RetryWhen(handler func(Observable) Observable) Observable { return e }
func (e *emptyObservable) DoOnNext(action OnNext) Observable                        { return e }
func (e *emptyObservable) DoOnError(action OnError) Observable                      { return e }
func (e *emptyObservable) DoOnComplete(action OnComplete) Observable {
	return Empty().DoOnComplete(action)
}
func (e *emptyObservable) ToSlice() Single { return SingleJust([]interface{}{}) }
func (e *emptyObservable) ToChannel() <-chan Item {
	ch := make(chan Item, 1)
	ch <- Item{Value: nil, Error: nil}
	close(ch)
	return ch
}
func (e *emptyObservable) Publish() ConnectableObservable             { return Empty().Publish() }
func (e *emptyObservable) Share() Observable                          { return e }
func (e *emptyObservable) BlockingSubscribe(observer Observer)        { e.Subscribe(observer) }
func (e *emptyObservable) BlockingFirst() (interface{}, error)        { return nil, nil }
func (e *emptyObservable) BlockingLast() (interface{}, error)         { return nil, nil }
func (e *emptyObservable) StartWith(values ...interface{}) Observable { return FromSlice(values) }
func (e *emptyObservable) StartWithIterable(iterable func() <-chan interface{}) Observable {
	return FromChannel(iterable())
}
func (e *emptyObservable) Timestamp() Observable                     { return e }
func (e *emptyObservable) TimeInterval() Observable                  { return e }
func (e *emptyObservable) Cast(targetType reflect.Type) Observable   { return e }
func (e *emptyObservable) OfType(targetType reflect.Type) Observable { return e }
func (e *emptyObservable) WithLatestFrom(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return e
}
func (e *emptyObservable) DoFinally(action func()) Observable { return Empty().DoFinally(action) }
func (e *emptyObservable) DoAfterTerminate(action func()) Observable {
	return Empty().DoAfterTerminate(action)
}
func (e *emptyObservable) Cache() Observable                         { return e }
func (e *emptyObservable) CacheWithCapacity(capacity int) Observable { return e }
func (e *emptyObservable) Amb(other Observable) Observable           { return other }

// 高级组合操作符
func (e *emptyObservable) Join(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, interface{}) interface{}) Observable {
	return e
}
func (e *emptyObservable) GroupJoin(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, Observable) interface{}) Observable {
	return e
}
func (e *emptyObservable) SwitchOnNext() Observable { return e }

// ============================================================================
// 调度器感知的标量Observable
// ============================================================================

// scheduledScalarObservable 带调度器的标量Observable
type scheduledScalarObservable struct {
	scalar    *scalarObservable
	scheduler Scheduler
}

func (s *scheduledScalarObservable) Subscribe(observer Observer) Subscription {
	s.scheduler.Schedule(func() {
		s.scalar.Subscribe(observer)
	})
	return &noOpSubscription{}
}

func (s *scheduledScalarObservable) SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) Subscription {
	s.scheduler.Schedule(func() {
		s.scalar.SubscribeWithCallbacks(onNext, onError, onComplete)
	})
	return &noOpSubscription{}
}

// 委托给标量Observable的所有方法
func (s *scheduledScalarObservable) SubscribeOn(scheduler Scheduler) Observable {
	return &scheduledScalarObservable{scalar: s.scalar, scheduler: scheduler}
}
func (s *scheduledScalarObservable) ObserveOn(scheduler Scheduler) Observable {
	return &scheduledScalarObservable{scalar: s.scalar, scheduler: scheduler}
}
func (s *scheduledScalarObservable) Map(transformer Transformer) Observable {
	return s.scalar.Map(transformer)
}
func (s *scheduledScalarObservable) FlatMap(transformer func(interface{}) Observable) Observable {
	return s.scalar.FlatMap(transformer)
}
func (s *scheduledScalarObservable) Filter(predicate Predicate) Observable {
	return s.scalar.Filter(predicate)
}
func (s *scheduledScalarObservable) Take(count int) Observable     { return s.scalar.Take(count) }
func (s *scheduledScalarObservable) TakeLast(count int) Observable { return s.scalar.TakeLast(count) }
func (s *scheduledScalarObservable) TakeUntil(other Observable) Observable {
	return s.scalar.TakeUntil(other)
}
func (s *scheduledScalarObservable) TakeWhile(predicate Predicate) Observable {
	return s.scalar.TakeWhile(predicate)
}
func (s *scheduledScalarObservable) Skip(count int) Observable     { return s.scalar.Skip(count) }
func (s *scheduledScalarObservable) SkipLast(count int) Observable { return s.scalar.SkipLast(count) }
func (s *scheduledScalarObservable) SkipUntil(other Observable) Observable {
	return s.scalar.SkipUntil(other)
}
func (s *scheduledScalarObservable) SkipWhile(predicate Predicate) Observable {
	return s.scalar.SkipWhile(predicate)
}
func (s *scheduledScalarObservable) Distinct() Observable { return s.scalar.Distinct() }
func (s *scheduledScalarObservable) DistinctUntilChanged() Observable {
	return s.scalar.DistinctUntilChanged()
}
func (s *scheduledScalarObservable) Buffer(count int) Observable { return s.scalar.Buffer(count) }
func (s *scheduledScalarObservable) BufferWithTime(timespan time.Duration) Observable {
	return s.scalar.BufferWithTime(timespan)
}
func (s *scheduledScalarObservable) Window(count int) Observable { return s.scalar.Window(count) }
func (s *scheduledScalarObservable) WindowWithTime(timespan time.Duration) Observable {
	return s.scalar.WindowWithTime(timespan)
}
func (s *scheduledScalarObservable) Merge(other Observable) Observable { return s.scalar.Merge(other) }
func (s *scheduledScalarObservable) Concat(other Observable) Observable {
	return s.scalar.Concat(other)
}
func (s *scheduledScalarObservable) Zip(other Observable, zipper func(interface{}, interface{}) interface{}) Observable {
	return s.scalar.Zip(other, zipper)
}
func (s *scheduledScalarObservable) CombineLatest(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return s.scalar.CombineLatest(other, combiner)
}
func (s *scheduledScalarObservable) Reduce(reducer Reducer) Single   { return s.scalar.Reduce(reducer) }
func (s *scheduledScalarObservable) Scan(reducer Reducer) Observable { return s.scalar.Scan(reducer) }
func (s *scheduledScalarObservable) Count() Single                   { return s.scalar.Count() }
func (s *scheduledScalarObservable) First() Single                   { return s.scalar.First() }
func (s *scheduledScalarObservable) Last() Single                    { return s.scalar.Last() }
func (s *scheduledScalarObservable) Min() Single                     { return s.scalar.Min() }
func (s *scheduledScalarObservable) Max() Single                     { return s.scalar.Max() }
func (s *scheduledScalarObservable) Sum() Single                     { return s.scalar.Sum() }
func (s *scheduledScalarObservable) Average() Single                 { return s.scalar.Average() }
func (s *scheduledScalarObservable) All(predicate Predicate) Single  { return s.scalar.All(predicate) }
func (s *scheduledScalarObservable) Any(predicate Predicate) Single  { return s.scalar.Any(predicate) }
func (s *scheduledScalarObservable) Contains(value interface{}) Single {
	return s.scalar.Contains(value)
}
func (s *scheduledScalarObservable) ElementAt(index int) Single { return s.scalar.ElementAt(index) }
func (s *scheduledScalarObservable) Debounce(duration time.Duration) Observable {
	return s.scalar.Debounce(duration)
}
func (s *scheduledScalarObservable) Throttle(duration time.Duration) Observable {
	return s.scalar.Throttle(duration)
}
func (s *scheduledScalarObservable) Delay(duration time.Duration) Observable {
	return s.scalar.Delay(duration)
}
func (s *scheduledScalarObservable) Timeout(duration time.Duration) Observable {
	return s.scalar.Timeout(duration)
}
func (s *scheduledScalarObservable) Catch(handler func(error) Observable) Observable {
	return s.scalar.Catch(handler)
}
func (s *scheduledScalarObservable) Retry(count int) Observable { return s.scalar.Retry(count) }
func (s *scheduledScalarObservable) RetryWhen(handler func(Observable) Observable) Observable {
	return s.scalar.RetryWhen(handler)
}
func (s *scheduledScalarObservable) DoOnNext(action OnNext) Observable {
	return s.scalar.DoOnNext(action)
}
func (s *scheduledScalarObservable) DoOnError(action OnError) Observable {
	return s.scalar.DoOnError(action)
}
func (s *scheduledScalarObservable) DoOnComplete(action OnComplete) Observable {
	return s.scalar.DoOnComplete(action)
}
func (s *scheduledScalarObservable) ToSlice() Single                { return s.scalar.ToSlice() }
func (s *scheduledScalarObservable) ToChannel() <-chan Item         { return s.scalar.ToChannel() }
func (s *scheduledScalarObservable) Publish() ConnectableObservable { return s.scalar.Publish() }
func (s *scheduledScalarObservable) Share() Observable              { return s.scalar.Share() }
func (s *scheduledScalarObservable) BlockingSubscribe(observer Observer) {
	s.scalar.BlockingSubscribe(observer)
}
func (s *scheduledScalarObservable) BlockingFirst() (interface{}, error) {
	return s.scalar.BlockingFirst()
}
func (s *scheduledScalarObservable) BlockingLast() (interface{}, error) {
	return s.scalar.BlockingLast()
}
func (s *scheduledScalarObservable) StartWith(values ...interface{}) Observable {
	return s.scalar.StartWith(values...)
}
func (s *scheduledScalarObservable) StartWithIterable(iterable func() <-chan interface{}) Observable {
	return s.scalar.StartWithIterable(iterable)
}
func (s *scheduledScalarObservable) Timestamp() Observable    { return s.scalar.Timestamp() }
func (s *scheduledScalarObservable) TimeInterval() Observable { return s.scalar.TimeInterval() }
func (s *scheduledScalarObservable) Cast(targetType reflect.Type) Observable {
	return s.scalar.Cast(targetType)
}
func (s *scheduledScalarObservable) OfType(targetType reflect.Type) Observable {
	return s.scalar.OfType(targetType)
}
func (s *scheduledScalarObservable) WithLatestFrom(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return s.scalar.WithLatestFrom(other, combiner)
}
func (s *scheduledScalarObservable) DoFinally(action func()) Observable {
	return s.scalar.DoFinally(action)
}
func (s *scheduledScalarObservable) DoAfterTerminate(action func()) Observable {
	return s.scalar.DoAfterTerminate(action)
}
func (s *scheduledScalarObservable) Cache() Observable { return s.scalar.Cache() }
func (s *scheduledScalarObservable) CacheWithCapacity(capacity int) Observable {
	return s.scalar.CacheWithCapacity(capacity)
}
func (s *scheduledScalarObservable) Amb(other Observable) Observable { return s.scalar.Amb(other) }

// 高级组合操作符
func (s *scheduledScalarObservable) Join(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, interface{}) interface{}) Observable {
	return s.scalar.Join(other, leftWindowSelector, rightWindowSelector, resultSelector)
}
func (s *scheduledScalarObservable) GroupJoin(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, Observable) interface{}) Observable {
	return s.scalar.GroupJoin(other, leftWindowSelector, rightWindowSelector, resultSelector)
}
func (s *scheduledScalarObservable) SwitchOnNext() Observable { return s.scalar.SwitchOnNext() }

// ============================================================================
// 无操作订阅实现
// ============================================================================

type noOpSubscription struct{}

func (n *noOpSubscription) Unsubscribe()         {}
func (n *noOpSubscription) IsUnsubscribed() bool { return false }

// ============================================================================
// Assembly-time优化工厂函数
// ============================================================================

// OptimizedJust 创建优化的Just操作符，使用assembly-time优化
func OptimizedJust(value interface{}) Observable {
	return NewScalarObservable(value)
}

// OptimizedEmpty 创建优化的Empty操作符，使用assembly-time优化
func OptimizedEmpty() Observable {
	return NewEmptyObservable()
}

// IsScalarCallable 检查Observable是否支持标量调用优化
func IsScalarCallable(obs Observable) (ScalarCallable, bool) {
	scalar, ok := obs.(ScalarCallable)
	return scalar, ok
}

// IsEmptyCallable 检查Observable是否支持空值调用优化
func IsEmptyCallable(obs Observable) (EmptyCallable, bool) {
	empty, ok := obs.(EmptyCallable)
	return empty, ok
}

// ApplyAssemblyTimeOptimization 应用assembly-time优化
// 检测操作符链中的优化机会并应用相应的优化
func ApplyAssemblyTimeOptimization(obs Observable) Observable {
	// 检查是否为标量Observable
	if scalar, ok := IsScalarCallable(obs); ok {
		if scalar.IsEmpty() {
			return NewEmptyObservable()
		}
		return NewScalarObservable(scalar.Call())
	}

	// 检查是否为空Observable
	if empty, ok := IsEmptyCallable(obs); ok && empty.IsEmpty() {
		return NewEmptyObservable()
	}

	return obs
}
