package rxgo

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// QueueSubscription融合协议 - 对标RxJava的QueueSubscription
// 实现完整的队列融合机制，支持SYNC、ASYNC、BOUNDARY三种融合模式
// ============================================================================

// 融合模式常量
const (
	// FUSION_NONE 不支持融合
	FUSION_NONE = 0
	// FUSION_SYNC 同步融合 - 操作符链可以同步拉取数据
	FUSION_SYNC = 1
	// FUSION_ASYNC 异步融合 - 操作符链可以异步拉取数据
	FUSION_ASYNC = 2
	// FUSION_ANY 任意融合模式
	FUSION_ANY = FUSION_SYNC | FUSION_ASYNC
	// FUSION_BOUNDARY 边界融合 - 在操作符边界处进行融合
	FUSION_BOUNDARY = 4
)

// QueueSubscription 队列订阅接口 - 对标RxJava的QueueSubscription
type QueueSubscription interface {
	Subscription

	// RequestFusion 请求融合模式，返回实际支持的融合模式
	RequestFusion(mode int) int

	// Poll 从融合队列中拉取下一个元素，如果为空返回空Item
	Poll() (Item, bool)

	// IsEmpty 检查融合队列是否为空
	IsEmpty() bool

	// Clear 清空融合队列
	Clear()

	// Size 返回融合队列大小，-1表示未知
	Size() int

	// Offer 向融合队列添加元素（用于生产者）
	Offer(item Item) bool
}

// ============================================================================
// 高性能融合队列实现 - 对标RxJava的SpscLinkedArrayQueue
// ============================================================================

// FusionQueue 高性能无锁融合队列
type FusionQueue struct {
	buffer     []Item
	mask       int32
	head       int32 // 生产者指针
	tail       int32 // 消费者指针
	capacity   int
	terminated int32        // 终止标志
	error      error        // 错误信息
	mutex      sync.RWMutex // 用于保护error字段
}

// NewFusionQueue 创建新的融合队列
func NewFusionQueue(capacity int) *FusionQueue {
	// 确保容量为2的幂
	if capacity <= 0 {
		capacity = 16
	}
	actualCapacity := 1
	for actualCapacity < capacity {
		actualCapacity <<= 1
	}

	return &FusionQueue{
		buffer:   make([]Item, actualCapacity),
		mask:     int32(actualCapacity - 1),
		capacity: actualCapacity,
	}
}

// Offer 向队列添加元素
func (fq *FusionQueue) Offer(item Item) bool {
	// 检查是否已终止
	if atomic.LoadInt32(&fq.terminated) == 1 {
		return false
	}

	head := atomic.LoadInt32(&fq.head)
	tail := atomic.LoadInt32(&fq.tail)

	// 检查队列是否已满（预留一个位置来区分满和空状态）
	if head-tail >= int32(fq.capacity-1) {
		return false
	}

	// 添加元素
	fq.buffer[head&int32(fq.mask)] = item
	atomic.StoreInt32(&fq.head, head+1)

	// 检查是否为终止信号
	if item.IsError() {
		fq.mutex.Lock()
		fq.error = item.Error
		fq.mutex.Unlock()
		atomic.StoreInt32(&fq.terminated, 1)
	} else if item.Value == nil && item.Error == nil {
		atomic.StoreInt32(&fq.terminated, 1)
	}

	return true
}

// Poll 从队列获取元素
func (fq *FusionQueue) Poll() (Item, bool) {
	tail := atomic.LoadInt32(&fq.tail)
	head := atomic.LoadInt32(&fq.head)

	// 检查队列是否为空
	if tail >= head {
		return Item{}, false
	}

	// 获取元素
	item := fq.buffer[tail&int32(fq.mask)]
	atomic.StoreInt32(&fq.tail, tail+1)
	return item, true
}

// IsEmpty 检查队列是否为空
func (fq *FusionQueue) IsEmpty() bool {
	return atomic.LoadInt32(&fq.tail) >= atomic.LoadInt32(&fq.head)
}

// Size 获取当前队列大小
func (fq *FusionQueue) Size() int {
	head := atomic.LoadInt32(&fq.head)
	tail := atomic.LoadInt32(&fq.tail)
	size := head - tail
	if size < 0 {
		return 0
	}
	return int(size)
}

// Clear 清空队列
func (fq *FusionQueue) Clear() {
	tail := atomic.LoadInt32(&fq.tail)
	head := atomic.LoadInt32(&fq.head)

	// 清除所有引用
	for tail < head {
		fq.buffer[tail&int32(fq.mask)] = Item{}
		tail++
	}

	atomic.StoreInt32(&fq.tail, head)
	atomic.StoreInt32(&fq.terminated, 0)
	fq.mutex.Lock()
	fq.error = nil
	fq.mutex.Unlock()
}

// IsTerminated 检查队列是否已终止
func (fq *FusionQueue) IsTerminated() bool {
	return atomic.LoadInt32(&fq.terminated) == 1
}

// GetError 获取错误信息
func (fq *FusionQueue) GetError() error {
	fq.mutex.RLock()
	defer fq.mutex.RUnlock()
	return fq.error
}

// ============================================================================
// 融合订阅实现 - 对标RxJava的QueueSubscription
// ============================================================================

// fusionSubscription 融合订阅实现
type fusionSubscription struct {
	queue        *FusionQueue
	fusionMode   int
	unsubscribed int32
	requested    int64
}

// NewFusionSubscription 创建新的融合订阅
func NewFusionSubscription(capacity int, supportedModes int) QueueSubscription {
	return &fusionSubscription{
		queue:      NewFusionQueue(capacity),
		fusionMode: supportedModes,
	}
}

func (fs *fusionSubscription) Unsubscribe() {
	atomic.StoreInt32(&fs.unsubscribed, 1)
	fs.queue.Clear()
}

func (fs *fusionSubscription) IsUnsubscribed() bool {
	return atomic.LoadInt32(&fs.unsubscribed) == 1
}

func (fs *fusionSubscription) RequestFusion(mode int) int {
	// 返回支持的融合模式
	return fs.fusionMode & mode
}

func (fs *fusionSubscription) Poll() (Item, bool) {
	if fs.IsUnsubscribed() {
		return Item{}, false
	}
	return fs.queue.Poll()
}

func (fs *fusionSubscription) IsEmpty() bool {
	return fs.queue.IsEmpty()
}

func (fs *fusionSubscription) Clear() {
	fs.queue.Clear()
}

func (fs *fusionSubscription) Size() int {
	return fs.queue.Size()
}

func (fs *fusionSubscription) Offer(item Item) bool {
	if fs.IsUnsubscribed() {
		return false
	}
	return fs.queue.Offer(item)
}

// ============================================================================
// 同步融合Observable - 对标RxJava的SyncFusionObservable
// ============================================================================

// syncFusionObservable 同步融合Observable
type syncFusionObservable struct {
	source      Observable
	transformer func(Item) (Item, error)
	fusionMode  int
}

// NewSyncFusionObservable 创建同步融合Observable
func NewSyncFusionObservable(source Observable, transformer func(Item) (Item, error)) Observable {
	return &syncFusionObservable{
		source:      source,
		transformer: transformer,
		fusionMode:  FUSION_SYNC,
	}
}

func (sfo *syncFusionObservable) Subscribe(observer Observer) Subscription {
	fusionSub := NewFusionSubscription(128, FUSION_SYNC)

	var sourceSub Subscription

	// 启动生产者goroutine
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fusionSub.Offer(Item{Value: nil, Error: fmt.Errorf("panic: %v", r)})
			}
		}()

		// 订阅源Observable
		sourceSub = sfo.source.Subscribe(func(item Item) {
			if fusionSub.IsUnsubscribed() {
				return
			}

			// 应用转换
			var transformedItem Item
			var err error
			if sfo.transformer != nil {
				transformedItem, err = sfo.transformer(item)
				if err != nil {
					fusionSub.Offer(Item{Value: nil, Error: err})
					return
				}
			} else {
				transformedItem = item
			}

			// 向融合队列添加转换后的项目
			if !fusionSub.Offer(transformedItem) {
				// 队列已满或已终止
				if sourceSub != nil {
					sourceSub.Unsubscribe()
				}
			}
		})

		// 等待源完成
		for sourceSub != nil && !sourceSub.IsUnsubscribed() && !fusionSub.IsUnsubscribed() {
			time.Sleep(1 * time.Millisecond)
		}
	}()

	// 启动消费者goroutine
	go func() {
		for !fusionSub.IsUnsubscribed() {
			if item, ok := fusionSub.Poll(); ok {
				observer(item)
				if item.IsError() || (item.Value == nil && item.Error == nil) {
					break
				}
			} else {
				time.Sleep(1 * time.Millisecond)
			}
		}
	}()

	return fusionSub
}

func (sfo *syncFusionObservable) SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) Subscription {
	return sfo.Subscribe(func(item Item) {
		if item.IsError() {
			if onError != nil {
				onError(item.Error)
			}
		} else if item.Value == nil {
			if onComplete != nil {
				onComplete()
			}
		} else {
			if onNext != nil {
				onNext(item.Value)
			}
		}
	})
}

// ============================================================================
// 异步融合Observable - 对标RxJava的AsyncFusionObservable
// ============================================================================

// asyncFusionObservable 异步融合Observable
type asyncFusionObservable struct {
	source      Observable
	transformer func(Item) (Item, error)
	fusionMode  int
}

// NewAsyncFusionObservable 创建异步融合Observable
func NewAsyncFusionObservable(source Observable, transformer func(Item) (Item, error)) Observable {
	return &asyncFusionObservable{
		source:      source,
		transformer: transformer,
		fusionMode:  FUSION_ASYNC,
	}
}

func (afo *asyncFusionObservable) Subscribe(observer Observer) Subscription {
	fusionSub := NewFusionSubscription(256, FUSION_ASYNC)
	var pendingTasks int32    // 待完成的异步任务计数
	var sourceCompleted int32 // 源是否已完成

	// 异步处理模式
	afo.source.Subscribe(func(item Item) {
		if fusionSub.IsUnsubscribed() {
			return
		}

		// 检查是否为完成信号
		if item.Value == nil && item.Error == nil {
			atomic.StoreInt32(&sourceCompleted, 1)
			return
		}

		// 错误信号直接传递
		if item.IsError() {
			fusionSub.Offer(item)
			return
		}

		// 增加待处理任务计数
		atomic.AddInt32(&pendingTasks, 1)

		// 异步处理数据项目
		go func(inputItem Item) {
			defer func() {
				// 减少待处理任务计数
				remaining := atomic.AddInt32(&pendingTasks, -1)

				// 如果源已完成且所有任务已处理完，发送完成信号
				if atomic.LoadInt32(&sourceCompleted) == 1 && remaining == 0 {
					fusionSub.Offer(Item{Value: nil, Error: nil})
				}

				if r := recover(); r != nil {
					fusionSub.Offer(Item{Value: nil, Error: fmt.Errorf("async panic: %v", r)})
				}
			}()

			// 应用转换
			var transformedItem Item
			var err error
			if afo.transformer != nil {
				transformedItem, err = afo.transformer(inputItem)
				if err != nil {
					fusionSub.Offer(Item{Value: nil, Error: err})
					return
				}
			} else {
				transformedItem = inputItem
			}

			// 向融合队列添加转换后的项目
			fusionSub.Offer(transformedItem)
		}(item)
	})

	// 消费者goroutine
	go func() {
		for !fusionSub.IsUnsubscribed() {
			if item, ok := fusionSub.Poll(); ok {
				observer(item)
				if item.IsError() || (item.Value == nil && item.Error == nil) {
					break
				}
			} else {
				time.Sleep(1 * time.Millisecond)
			}
		}
	}()

	return fusionSub
}

func (afo *asyncFusionObservable) SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) Subscription {
	return afo.Subscribe(func(item Item) {
		if item.IsError() {
			if onError != nil {
				onError(item.Error)
			}
		} else if item.Value == nil {
			if onComplete != nil {
				onComplete()
			}
		} else {
			if onNext != nil {
				onNext(item.Value)
			}
		}
	})
}

// ============================================================================
// 边界融合Observable - 对标RxJava的BoundaryFusionObservable
// ============================================================================

// boundaryFusionObservable 边界融合Observable
type boundaryFusionObservable struct {
	source     Observable
	boundary   func() bool // 边界检查函数
	fusionMode int
}

// NewBoundaryFusionObservable 创建边界融合Observable
func NewBoundaryFusionObservable(source Observable, boundary func() bool) Observable {
	return &boundaryFusionObservable{
		source:     source,
		boundary:   boundary,
		fusionMode: FUSION_BOUNDARY,
	}
}

func (bfo *boundaryFusionObservable) Subscribe(observer Observer) Subscription {
	fusionSub := NewFusionSubscription(64, FUSION_BOUNDARY)

	sourceSub := bfo.source.Subscribe(func(item Item) {
		if fusionSub.IsUnsubscribed() {
			return
		}

		// 检查边界条件，这里仅用于优化标记，但所有元素都进入队列保持顺序
		if bfo.boundary != nil {
			bfo.boundary() // 调用边界函数，但不改变处理逻辑
		}

		// 所有元素都通过融合队列以保持顺序
		fusionSub.Offer(item)
	})

	// 处理融合队列中的项目
	go func() {
		for !fusionSub.IsUnsubscribed() {
			if item, ok := fusionSub.Poll(); ok {
				observer(item)
				if item.IsError() || (item.Value == nil && item.Error == nil) {
					break
				}
			} else if sourceSub.IsUnsubscribed() {
				break
			} else {
				time.Sleep(1 * time.Millisecond)
			}
		}
	}()

	return fusionSub
}

func (bfo *boundaryFusionObservable) SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) Subscription {
	return bfo.Subscribe(func(item Item) {
		if item.IsError() {
			if onError != nil {
				onError(item.Error)
			}
		} else if item.Value == nil {
			if onComplete != nil {
				onComplete()
			}
		} else {
			if onNext != nil {
				onNext(item.Value)
			}
		}
	})
}

// ============================================================================
// 为所有融合Observable实现Observable接口的其他方法
// ============================================================================

// syncFusionObservable的其他方法实现
func (sfo *syncFusionObservable) SubscribeOn(scheduler Scheduler) Observable {
	return sfo.source.SubscribeOn(scheduler)
}
func (sfo *syncFusionObservable) ObserveOn(scheduler Scheduler) Observable {
	return sfo.source.ObserveOn(scheduler)
}
func (sfo *syncFusionObservable) Map(transformer Transformer) Observable {
	return sfo.source.Map(transformer)
}
func (sfo *syncFusionObservable) FlatMap(transformer func(interface{}) Observable) Observable {
	return sfo.source.FlatMap(transformer)
}
func (sfo *syncFusionObservable) Filter(predicate Predicate) Observable {
	return sfo.source.Filter(predicate)
}
func (sfo *syncFusionObservable) Take(count int) Observable     { return sfo.source.Take(count) }
func (sfo *syncFusionObservable) TakeLast(count int) Observable { return sfo.source.TakeLast(count) }
func (sfo *syncFusionObservable) TakeUntil(other Observable) Observable {
	return sfo.source.TakeUntil(other)
}
func (sfo *syncFusionObservable) TakeWhile(predicate Predicate) Observable {
	return sfo.source.TakeWhile(predicate)
}
func (sfo *syncFusionObservable) Skip(count int) Observable     { return sfo.source.Skip(count) }
func (sfo *syncFusionObservable) SkipLast(count int) Observable { return sfo.source.SkipLast(count) }
func (sfo *syncFusionObservable) SkipUntil(other Observable) Observable {
	return sfo.source.SkipUntil(other)
}
func (sfo *syncFusionObservable) SkipWhile(predicate Predicate) Observable {
	return sfo.source.SkipWhile(predicate)
}
func (sfo *syncFusionObservable) Distinct() Observable { return sfo.source.Distinct() }
func (sfo *syncFusionObservable) DistinctUntilChanged() Observable {
	return sfo.source.DistinctUntilChanged()
}
func (sfo *syncFusionObservable) Buffer(count int) Observable { return sfo.source.Buffer(count) }
func (sfo *syncFusionObservable) BufferWithTime(timespan time.Duration) Observable {
	return sfo.source.BufferWithTime(timespan)
}
func (sfo *syncFusionObservable) Window(count int) Observable { return sfo.source.Window(count) }
func (sfo *syncFusionObservable) WindowWithTime(timespan time.Duration) Observable {
	return sfo.source.WindowWithTime(timespan)
}
func (sfo *syncFusionObservable) Merge(other Observable) Observable  { return sfo.source.Merge(other) }
func (sfo *syncFusionObservable) Concat(other Observable) Observable { return sfo.source.Concat(other) }
func (sfo *syncFusionObservable) Zip(other Observable, zipper func(interface{}, interface{}) interface{}) Observable {
	return sfo.source.Zip(other, zipper)
}
func (sfo *syncFusionObservable) CombineLatest(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return sfo.source.CombineLatest(other, combiner)
}
func (sfo *syncFusionObservable) Reduce(reducer Reducer) Single   { return sfo.source.Reduce(reducer) }
func (sfo *syncFusionObservable) Scan(reducer Reducer) Observable { return sfo.source.Scan(reducer) }
func (sfo *syncFusionObservable) Count() Single                   { return sfo.source.Count() }
func (sfo *syncFusionObservable) First() Single                   { return sfo.source.First() }
func (sfo *syncFusionObservable) Last() Single                    { return sfo.source.Last() }
func (sfo *syncFusionObservable) Min() Single                     { return sfo.source.Min() }
func (sfo *syncFusionObservable) Max() Single                     { return sfo.source.Max() }
func (sfo *syncFusionObservable) Sum() Single                     { return sfo.source.Sum() }
func (sfo *syncFusionObservable) Average() Single                 { return sfo.source.Average() }
func (sfo *syncFusionObservable) All(predicate Predicate) Single  { return sfo.source.All(predicate) }
func (sfo *syncFusionObservable) Any(predicate Predicate) Single  { return sfo.source.Any(predicate) }
func (sfo *syncFusionObservable) Contains(value interface{}) Single {
	return sfo.source.Contains(value)
}
func (sfo *syncFusionObservable) ElementAt(index int) Single { return sfo.source.ElementAt(index) }
func (sfo *syncFusionObservable) Debounce(duration time.Duration) Observable {
	return sfo.source.Debounce(duration)
}
func (sfo *syncFusionObservable) Throttle(duration time.Duration) Observable {
	return sfo.source.Throttle(duration)
}
func (sfo *syncFusionObservable) Delay(duration time.Duration) Observable {
	return sfo.source.Delay(duration)
}
func (sfo *syncFusionObservable) Timeout(duration time.Duration) Observable {
	return sfo.source.Timeout(duration)
}
func (sfo *syncFusionObservable) Catch(handler func(error) Observable) Observable {
	return sfo.source.Catch(handler)
}
func (sfo *syncFusionObservable) Retry(count int) Observable { return sfo.source.Retry(count) }
func (sfo *syncFusionObservable) RetryWhen(handler func(Observable) Observable) Observable {
	return sfo.source.RetryWhen(handler)
}
func (sfo *syncFusionObservable) DoOnNext(action OnNext) Observable {
	return sfo.source.DoOnNext(action)
}
func (sfo *syncFusionObservable) DoOnError(action OnError) Observable {
	return sfo.source.DoOnError(action)
}
func (sfo *syncFusionObservable) DoOnComplete(action OnComplete) Observable {
	return sfo.source.DoOnComplete(action)
}
func (sfo *syncFusionObservable) ToSlice() Single                { return sfo.source.ToSlice() }
func (sfo *syncFusionObservable) ToChannel() <-chan Item         { return sfo.source.ToChannel() }
func (sfo *syncFusionObservable) Publish() ConnectableObservable { return sfo.source.Publish() }
func (sfo *syncFusionObservable) Share() Observable              { return sfo.source.Share() }
func (sfo *syncFusionObservable) BlockingSubscribe(observer Observer) {
	sfo.source.BlockingSubscribe(observer)
}
func (sfo *syncFusionObservable) BlockingFirst() (interface{}, error) {
	return sfo.source.BlockingFirst()
}
func (sfo *syncFusionObservable) BlockingLast() (interface{}, error) {
	return sfo.source.BlockingLast()
}
func (sfo *syncFusionObservable) StartWith(values ...interface{}) Observable {
	return sfo.source.StartWith(values...)
}
func (sfo *syncFusionObservable) StartWithIterable(iterable func() <-chan interface{}) Observable {
	return sfo.source.StartWithIterable(iterable)
}
func (sfo *syncFusionObservable) Timestamp() Observable    { return sfo.source.Timestamp() }
func (sfo *syncFusionObservable) TimeInterval() Observable { return sfo.source.TimeInterval() }
func (sfo *syncFusionObservable) Cast(targetType reflect.Type) Observable {
	return sfo.source.Cast(targetType)
}
func (sfo *syncFusionObservable) OfType(targetType reflect.Type) Observable {
	return sfo.source.OfType(targetType)
}
func (sfo *syncFusionObservable) WithLatestFrom(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return sfo.source.WithLatestFrom(other, combiner)
}
func (sfo *syncFusionObservable) DoFinally(action func()) Observable {
	return sfo.source.DoFinally(action)
}
func (sfo *syncFusionObservable) DoAfterTerminate(action func()) Observable {
	return sfo.source.DoAfterTerminate(action)
}
func (sfo *syncFusionObservable) Cache() Observable { return sfo.source.Cache() }
func (sfo *syncFusionObservable) CacheWithCapacity(capacity int) Observable {
	return sfo.source.CacheWithCapacity(capacity)
}
func (sfo *syncFusionObservable) Amb(other Observable) Observable { return sfo.source.Amb(other) }

// 高级组合操作符
func (sfo *syncFusionObservable) Join(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, interface{}) interface{}) Observable {
	return sfo.source.Join(other, leftWindowSelector, rightWindowSelector, resultSelector)
}
func (sfo *syncFusionObservable) GroupJoin(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, Observable) interface{}) Observable {
	return sfo.source.GroupJoin(other, leftWindowSelector, rightWindowSelector, resultSelector)
}
func (sfo *syncFusionObservable) SwitchOnNext() Observable { return sfo.source.SwitchOnNext() }

// asyncFusionObservable和boundaryFusionObservable的其他方法实现类似，委托给source
func (afo *asyncFusionObservable) SubscribeOn(scheduler Scheduler) Observable {
	return afo.source.SubscribeOn(scheduler)
}
func (afo *asyncFusionObservable) ObserveOn(scheduler Scheduler) Observable {
	return afo.source.ObserveOn(scheduler)
}
func (afo *asyncFusionObservable) Map(transformer Transformer) Observable {
	return afo.source.Map(transformer)
}
func (afo *asyncFusionObservable) FlatMap(transformer func(interface{}) Observable) Observable {
	return afo.source.FlatMap(transformer)
}
func (afo *asyncFusionObservable) Filter(predicate Predicate) Observable {
	return afo.source.Filter(predicate)
}
func (afo *asyncFusionObservable) Take(count int) Observable     { return afo.source.Take(count) }
func (afo *asyncFusionObservable) TakeLast(count int) Observable { return afo.source.TakeLast(count) }
func (afo *asyncFusionObservable) TakeUntil(other Observable) Observable {
	return afo.source.TakeUntil(other)
}
func (afo *asyncFusionObservable) TakeWhile(predicate Predicate) Observable {
	return afo.source.TakeWhile(predicate)
}
func (afo *asyncFusionObservable) Skip(count int) Observable     { return afo.source.Skip(count) }
func (afo *asyncFusionObservable) SkipLast(count int) Observable { return afo.source.SkipLast(count) }
func (afo *asyncFusionObservable) SkipUntil(other Observable) Observable {
	return afo.source.SkipUntil(other)
}
func (afo *asyncFusionObservable) SkipWhile(predicate Predicate) Observable {
	return afo.source.SkipWhile(predicate)
}
func (afo *asyncFusionObservable) Distinct() Observable { return afo.source.Distinct() }
func (afo *asyncFusionObservable) DistinctUntilChanged() Observable {
	return afo.source.DistinctUntilChanged()
}
func (afo *asyncFusionObservable) Buffer(count int) Observable { return afo.source.Buffer(count) }
func (afo *asyncFusionObservable) BufferWithTime(timespan time.Duration) Observable {
	return afo.source.BufferWithTime(timespan)
}
func (afo *asyncFusionObservable) Window(count int) Observable { return afo.source.Window(count) }
func (afo *asyncFusionObservable) WindowWithTime(timespan time.Duration) Observable {
	return afo.source.WindowWithTime(timespan)
}
func (afo *asyncFusionObservable) Merge(other Observable) Observable { return afo.source.Merge(other) }
func (afo *asyncFusionObservable) Concat(other Observable) Observable {
	return afo.source.Concat(other)
}
func (afo *asyncFusionObservable) Zip(other Observable, zipper func(interface{}, interface{}) interface{}) Observable {
	return afo.source.Zip(other, zipper)
}
func (afo *asyncFusionObservable) CombineLatest(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return afo.source.CombineLatest(other, combiner)
}
func (afo *asyncFusionObservable) Reduce(reducer Reducer) Single   { return afo.source.Reduce(reducer) }
func (afo *asyncFusionObservable) Scan(reducer Reducer) Observable { return afo.source.Scan(reducer) }
func (afo *asyncFusionObservable) Count() Single                   { return afo.source.Count() }
func (afo *asyncFusionObservable) First() Single                   { return afo.source.First() }
func (afo *asyncFusionObservable) Last() Single                    { return afo.source.Last() }
func (afo *asyncFusionObservable) Min() Single                     { return afo.source.Min() }
func (afo *asyncFusionObservable) Max() Single                     { return afo.source.Max() }
func (afo *asyncFusionObservable) Sum() Single                     { return afo.source.Sum() }
func (afo *asyncFusionObservable) Average() Single                 { return afo.source.Average() }
func (afo *asyncFusionObservable) All(predicate Predicate) Single  { return afo.source.All(predicate) }
func (afo *asyncFusionObservable) Any(predicate Predicate) Single  { return afo.source.Any(predicate) }
func (afo *asyncFusionObservable) Contains(value interface{}) Single {
	return afo.source.Contains(value)
}
func (afo *asyncFusionObservable) ElementAt(index int) Single { return afo.source.ElementAt(index) }
func (afo *asyncFusionObservable) Debounce(duration time.Duration) Observable {
	return afo.source.Debounce(duration)
}
func (afo *asyncFusionObservable) Throttle(duration time.Duration) Observable {
	return afo.source.Throttle(duration)
}
func (afo *asyncFusionObservable) Delay(duration time.Duration) Observable {
	return afo.source.Delay(duration)
}
func (afo *asyncFusionObservable) Timeout(duration time.Duration) Observable {
	return afo.source.Timeout(duration)
}
func (afo *asyncFusionObservable) Catch(handler func(error) Observable) Observable {
	return afo.source.Catch(handler)
}
func (afo *asyncFusionObservable) Retry(count int) Observable { return afo.source.Retry(count) }
func (afo *asyncFusionObservable) RetryWhen(handler func(Observable) Observable) Observable {
	return afo.source.RetryWhen(handler)
}
func (afo *asyncFusionObservable) DoOnNext(action OnNext) Observable {
	return afo.source.DoOnNext(action)
}
func (afo *asyncFusionObservable) DoOnError(action OnError) Observable {
	return afo.source.DoOnError(action)
}
func (afo *asyncFusionObservable) DoOnComplete(action OnComplete) Observable {
	return afo.source.DoOnComplete(action)
}
func (afo *asyncFusionObservable) ToSlice() Single                { return afo.source.ToSlice() }
func (afo *asyncFusionObservable) ToChannel() <-chan Item         { return afo.source.ToChannel() }
func (afo *asyncFusionObservable) Publish() ConnectableObservable { return afo.source.Publish() }
func (afo *asyncFusionObservable) Share() Observable              { return afo.source.Share() }
func (afo *asyncFusionObservable) BlockingSubscribe(observer Observer) {
	afo.source.BlockingSubscribe(observer)
}
func (afo *asyncFusionObservable) BlockingFirst() (interface{}, error) {
	return afo.source.BlockingFirst()
}
func (afo *asyncFusionObservable) BlockingLast() (interface{}, error) {
	return afo.source.BlockingLast()
}
func (afo *asyncFusionObservable) StartWith(values ...interface{}) Observable {
	return afo.source.StartWith(values...)
}
func (afo *asyncFusionObservable) StartWithIterable(iterable func() <-chan interface{}) Observable {
	return afo.source.StartWithIterable(iterable)
}
func (afo *asyncFusionObservable) Timestamp() Observable    { return afo.source.Timestamp() }
func (afo *asyncFusionObservable) TimeInterval() Observable { return afo.source.TimeInterval() }
func (afo *asyncFusionObservable) Cast(targetType reflect.Type) Observable {
	return afo.source.Cast(targetType)
}
func (afo *asyncFusionObservable) OfType(targetType reflect.Type) Observable {
	return afo.source.OfType(targetType)
}
func (afo *asyncFusionObservable) WithLatestFrom(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return afo.source.WithLatestFrom(other, combiner)
}
func (afo *asyncFusionObservable) DoFinally(action func()) Observable {
	return afo.source.DoFinally(action)
}
func (afo *asyncFusionObservable) DoAfterTerminate(action func()) Observable {
	return afo.source.DoAfterTerminate(action)
}
func (afo *asyncFusionObservable) Cache() Observable { return afo.source.Cache() }
func (afo *asyncFusionObservable) CacheWithCapacity(capacity int) Observable {
	return afo.source.CacheWithCapacity(capacity)
}
func (afo *asyncFusionObservable) Amb(other Observable) Observable { return afo.source.Amb(other) }

// 高级组合操作符
func (afo *asyncFusionObservable) Join(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, interface{}) interface{}) Observable {
	return afo.source.Join(other, leftWindowSelector, rightWindowSelector, resultSelector)
}
func (afo *asyncFusionObservable) GroupJoin(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, Observable) interface{}) Observable {
	return afo.source.GroupJoin(other, leftWindowSelector, rightWindowSelector, resultSelector)
}
func (afo *asyncFusionObservable) SwitchOnNext() Observable { return afo.source.SwitchOnNext() }

// boundaryFusionObservable的其他方法实现
func (bfo *boundaryFusionObservable) SubscribeOn(scheduler Scheduler) Observable {
	return bfo.source.SubscribeOn(scheduler)
}
func (bfo *boundaryFusionObservable) ObserveOn(scheduler Scheduler) Observable {
	return bfo.source.ObserveOn(scheduler)
}
func (bfo *boundaryFusionObservable) Map(transformer Transformer) Observable {
	return bfo.source.Map(transformer)
}
func (bfo *boundaryFusionObservable) FlatMap(transformer func(interface{}) Observable) Observable {
	return bfo.source.FlatMap(transformer)
}
func (bfo *boundaryFusionObservable) Filter(predicate Predicate) Observable {
	return bfo.source.Filter(predicate)
}
func (bfo *boundaryFusionObservable) Take(count int) Observable { return bfo.source.Take(count) }
func (bfo *boundaryFusionObservable) TakeLast(count int) Observable {
	return bfo.source.TakeLast(count)
}
func (bfo *boundaryFusionObservable) TakeUntil(other Observable) Observable {
	return bfo.source.TakeUntil(other)
}
func (bfo *boundaryFusionObservable) TakeWhile(predicate Predicate) Observable {
	return bfo.source.TakeWhile(predicate)
}
func (bfo *boundaryFusionObservable) Skip(count int) Observable { return bfo.source.Skip(count) }
func (bfo *boundaryFusionObservable) SkipLast(count int) Observable {
	return bfo.source.SkipLast(count)
}
func (bfo *boundaryFusionObservable) SkipUntil(other Observable) Observable {
	return bfo.source.SkipUntil(other)
}
func (bfo *boundaryFusionObservable) SkipWhile(predicate Predicate) Observable {
	return bfo.source.SkipWhile(predicate)
}
func (bfo *boundaryFusionObservable) Distinct() Observable { return bfo.source.Distinct() }
func (bfo *boundaryFusionObservable) DistinctUntilChanged() Observable {
	return bfo.source.DistinctUntilChanged()
}
func (bfo *boundaryFusionObservable) Buffer(count int) Observable { return bfo.source.Buffer(count) }
func (bfo *boundaryFusionObservable) BufferWithTime(timespan time.Duration) Observable {
	return bfo.source.BufferWithTime(timespan)
}
func (bfo *boundaryFusionObservable) Window(count int) Observable { return bfo.source.Window(count) }
func (bfo *boundaryFusionObservable) WindowWithTime(timespan time.Duration) Observable {
	return bfo.source.WindowWithTime(timespan)
}
func (bfo *boundaryFusionObservable) Merge(other Observable) Observable {
	return bfo.source.Merge(other)
}
func (bfo *boundaryFusionObservable) Concat(other Observable) Observable {
	return bfo.source.Concat(other)
}
func (bfo *boundaryFusionObservable) Zip(other Observable, zipper func(interface{}, interface{}) interface{}) Observable {
	return bfo.source.Zip(other, zipper)
}
func (bfo *boundaryFusionObservable) CombineLatest(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return bfo.source.CombineLatest(other, combiner)
}
func (bfo *boundaryFusionObservable) Reduce(reducer Reducer) Single {
	return bfo.source.Reduce(reducer)
}
func (bfo *boundaryFusionObservable) Scan(reducer Reducer) Observable {
	return bfo.source.Scan(reducer)
}
func (bfo *boundaryFusionObservable) Count() Single   { return bfo.source.Count() }
func (bfo *boundaryFusionObservable) First() Single   { return bfo.source.First() }
func (bfo *boundaryFusionObservable) Last() Single    { return bfo.source.Last() }
func (bfo *boundaryFusionObservable) Min() Single     { return bfo.source.Min() }
func (bfo *boundaryFusionObservable) Max() Single     { return bfo.source.Max() }
func (bfo *boundaryFusionObservable) Sum() Single     { return bfo.source.Sum() }
func (bfo *boundaryFusionObservable) Average() Single { return bfo.source.Average() }
func (bfo *boundaryFusionObservable) All(predicate Predicate) Single {
	return bfo.source.All(predicate)
}
func (bfo *boundaryFusionObservable) Any(predicate Predicate) Single {
	return bfo.source.Any(predicate)
}
func (bfo *boundaryFusionObservable) Contains(value interface{}) Single {
	return bfo.source.Contains(value)
}
func (bfo *boundaryFusionObservable) ElementAt(index int) Single { return bfo.source.ElementAt(index) }
func (bfo *boundaryFusionObservable) Debounce(duration time.Duration) Observable {
	return bfo.source.Debounce(duration)
}
func (bfo *boundaryFusionObservable) Throttle(duration time.Duration) Observable {
	return bfo.source.Throttle(duration)
}
func (bfo *boundaryFusionObservable) Delay(duration time.Duration) Observable {
	return bfo.source.Delay(duration)
}
func (bfo *boundaryFusionObservable) Timeout(duration time.Duration) Observable {
	return bfo.source.Timeout(duration)
}
func (bfo *boundaryFusionObservable) Catch(handler func(error) Observable) Observable {
	return bfo.source.Catch(handler)
}
func (bfo *boundaryFusionObservable) Retry(count int) Observable { return bfo.source.Retry(count) }
func (bfo *boundaryFusionObservable) RetryWhen(handler func(Observable) Observable) Observable {
	return bfo.source.RetryWhen(handler)
}
func (bfo *boundaryFusionObservable) DoOnNext(action OnNext) Observable {
	return bfo.source.DoOnNext(action)
}
func (bfo *boundaryFusionObservable) DoOnError(action OnError) Observable {
	return bfo.source.DoOnError(action)
}
func (bfo *boundaryFusionObservable) DoOnComplete(action OnComplete) Observable {
	return bfo.source.DoOnComplete(action)
}
func (bfo *boundaryFusionObservable) ToSlice() Single                { return bfo.source.ToSlice() }
func (bfo *boundaryFusionObservable) ToChannel() <-chan Item         { return bfo.source.ToChannel() }
func (bfo *boundaryFusionObservable) Publish() ConnectableObservable { return bfo.source.Publish() }
func (bfo *boundaryFusionObservable) Share() Observable              { return bfo.source.Share() }
func (bfo *boundaryFusionObservable) BlockingSubscribe(observer Observer) {
	bfo.source.BlockingSubscribe(observer)
}
func (bfo *boundaryFusionObservable) BlockingFirst() (interface{}, error) {
	return bfo.source.BlockingFirst()
}
func (bfo *boundaryFusionObservable) BlockingLast() (interface{}, error) {
	return bfo.source.BlockingLast()
}
func (bfo *boundaryFusionObservable) StartWith(values ...interface{}) Observable {
	return bfo.source.StartWith(values...)
}
func (bfo *boundaryFusionObservable) StartWithIterable(iterable func() <-chan interface{}) Observable {
	return bfo.source.StartWithIterable(iterable)
}
func (bfo *boundaryFusionObservable) Timestamp() Observable    { return bfo.source.Timestamp() }
func (bfo *boundaryFusionObservable) TimeInterval() Observable { return bfo.source.TimeInterval() }
func (bfo *boundaryFusionObservable) Cast(targetType reflect.Type) Observable {
	return bfo.source.Cast(targetType)
}
func (bfo *boundaryFusionObservable) OfType(targetType reflect.Type) Observable {
	return bfo.source.OfType(targetType)
}
func (bfo *boundaryFusionObservable) WithLatestFrom(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return bfo.source.WithLatestFrom(other, combiner)
}
func (bfo *boundaryFusionObservable) DoFinally(action func()) Observable {
	return bfo.source.DoFinally(action)
}
func (bfo *boundaryFusionObservable) DoAfterTerminate(action func()) Observable {
	return bfo.source.DoAfterTerminate(action)
}
func (bfo *boundaryFusionObservable) Cache() Observable { return bfo.source.Cache() }
func (bfo *boundaryFusionObservable) CacheWithCapacity(capacity int) Observable {
	return bfo.source.CacheWithCapacity(capacity)
}
func (bfo *boundaryFusionObservable) Amb(other Observable) Observable { return bfo.source.Amb(other) }

// 高级组合操作符
func (bfo *boundaryFusionObservable) Join(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, interface{}) interface{}) Observable {
	return bfo.source.Join(other, leftWindowSelector, rightWindowSelector, resultSelector)
}
func (bfo *boundaryFusionObservable) GroupJoin(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, Observable) interface{}) Observable {
	return bfo.source.GroupJoin(other, leftWindowSelector, rightWindowSelector, resultSelector)
}
func (bfo *boundaryFusionObservable) SwitchOnNext() Observable { return bfo.source.SwitchOnNext() }

// ============================================================================
// 工厂函数和工具函数
// ============================================================================

// FusionSync 创建同步融合Observable
func FusionSync(source Observable, transformer func(Item) (Item, error)) Observable {
	return NewSyncFusionObservable(source, transformer)
}

// FusionAsync 创建异步融合Observable
func FusionAsync(source Observable, transformer func(Item) (Item, error)) Observable {
	return NewAsyncFusionObservable(source, transformer)
}

// FusionBoundary 创建边界融合Observable
func FusionBoundary(source Observable, boundary func() bool) Observable {
	return NewBoundaryFusionObservable(source, boundary)
}

// IsQueueSubscription 检查Subscription是否支持队列融合
func IsQueueSubscription(sub Subscription) (QueueSubscription, bool) {
	queueSub, ok := sub.(QueueSubscription)
	return queueSub, ok
}
