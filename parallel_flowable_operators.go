// ParallelFlowable operators implementation for RxGo
// ParallelFlowable操作符实现，包含所有并行操作符的订阅者和逻辑
package rxgo

import (
	"sync"
	"sync/atomic"
)

// ============================================================================
// Map操作符订阅者
// ============================================================================

// parallelMapSubscriber Map操作符的并行订阅者实现
type parallelMapSubscriber struct {
	BaseSubscriber
	downstream  Subscriber
	transformer Transformer
}

func (pms *parallelMapSubscriber) OnSubscribe(subscription FlowableSubscription) {
	pms.BaseSubscriber.OnSubscribe(subscription)
	pms.downstream.OnSubscribe(subscription)
}

func (pms *parallelMapSubscriber) OnNext(item Item) {
	if item.IsError() {
		pms.downstream.OnError(item.Error)
		return
	}

	if item.Value == nil {
		pms.downstream.OnComplete()
		return
	}

	// 转换数据项
	result, err := pms.transformer(item.Value)
	if err != nil {
		pms.downstream.OnError(err)
		return
	}

	pms.downstream.OnNext(CreateItem(result))
}

func (pms *parallelMapSubscriber) OnError(err error) {
	pms.downstream.OnError(err)
}

func (pms *parallelMapSubscriber) OnComplete() {
	pms.downstream.OnComplete()
}

// ============================================================================
// Filter操作符订阅者
// ============================================================================

// parallelFilterSubscriber Filter操作符的并行订阅者实现
type parallelFilterSubscriber struct {
	BaseSubscriber
	downstream Subscriber
	predicate  Predicate
}

func (pfs *parallelFilterSubscriber) OnSubscribe(subscription FlowableSubscription) {
	pfs.BaseSubscriber.OnSubscribe(subscription)
	pfs.downstream.OnSubscribe(subscription)
}

func (pfs *parallelFilterSubscriber) OnNext(item Item) {
	if item.IsError() {
		pfs.downstream.OnError(item.Error)
		return
	}

	if item.Value == nil {
		pfs.downstream.OnComplete()
		return
	}

	// 应用过滤条件
	if pfs.predicate(item.Value) {
		pfs.downstream.OnNext(item)
	}
}

func (pfs *parallelFilterSubscriber) OnError(err error) {
	pfs.downstream.OnError(err)
}

func (pfs *parallelFilterSubscriber) OnComplete() {
	pfs.downstream.OnComplete()
}

// ============================================================================
// FlatMap操作符订阅者
// ============================================================================

// parallelFlatMapSubscriber FlatMap操作符的并行订阅者实现
type parallelFlatMapSubscriber struct {
	BaseSubscriber
	downstream  Subscriber
	transformer func(interface{}) Flowable
	active      map[int]FlowableSubscription
	nextIndex   int32
	completed   bool
	mu          sync.RWMutex
}

func (pfms *parallelFlatMapSubscriber) OnSubscribe(subscription FlowableSubscription) {
	pfms.BaseSubscriber.OnSubscribe(subscription)
	pfms.downstream.OnSubscribe(subscription)
}

func (pfms *parallelFlatMapSubscriber) OnNext(item Item) {
	if item.IsError() {
		pfms.downstream.OnError(item.Error)
		return
	}

	if item.Value == nil {
		pfms.mu.Lock()
		pfms.completed = true
		if len(pfms.active) == 0 {
			pfms.downstream.OnComplete()
		}
		pfms.mu.Unlock()
		return
	}

	// 转换为Flowable
	innerFlowable := pfms.transformer(item.Value)
	index := int(atomic.AddInt32(&pfms.nextIndex, 1))

	// 订阅内部Flowable
	innerSubscription := innerFlowable.SubscribeWithCallbacks(
		func(value interface{}) {
			pfms.downstream.OnNext(CreateItem(value))
		},
		func(err error) {
			pfms.downstream.OnError(err)
		},
		func() {
			pfms.mu.Lock()
			delete(pfms.active, index)
			allComplete := pfms.completed && len(pfms.active) == 0
			pfms.mu.Unlock()

			if allComplete {
				pfms.downstream.OnComplete()
			}
		},
	)

	pfms.mu.Lock()
	pfms.active[index] = innerSubscription
	pfms.mu.Unlock()

	// 请求数据
	innerSubscription.Request(1)
}

func (pfms *parallelFlatMapSubscriber) OnError(err error) {
	pfms.downstream.OnError(err)
}

func (pfms *parallelFlatMapSubscriber) OnComplete() {
	pfms.mu.Lock()
	pfms.completed = true
	if len(pfms.active) == 0 {
		pfms.downstream.OnComplete()
	}
	pfms.mu.Unlock()
}

// ============================================================================
// Reduce操作符订阅者
// ============================================================================

// parallelReduceSubscriber Reduce操作符的并行订阅者实现
type parallelReduceSubscriber struct {
	BaseSubscriber
	index       int
	reducer     func(accumulator, value interface{}) interface{}
	accumulator interface{}
	hasValue    bool
	onResult    func(int, interface{})
	onError     func(int, error)
	mu          sync.Mutex
}

func (prs *parallelReduceSubscriber) OnSubscribe(subscription FlowableSubscription) {
	prs.BaseSubscriber.OnSubscribe(subscription)
	// 自动请求所有数据
	subscription.Request(9223372036854775807)
}

func (prs *parallelReduceSubscriber) OnNext(item Item) {
	if item.IsError() {
		prs.onError(prs.index, item.Error)
		return
	}

	if item.Value == nil {
		prs.OnComplete()
		return
	}

	prs.mu.Lock()
	defer prs.mu.Unlock()

	if !prs.hasValue {
		prs.accumulator = item.Value
		prs.hasValue = true
	} else {
		prs.accumulator = prs.reducer(prs.accumulator, item.Value)
	}
}

func (prs *parallelReduceSubscriber) OnError(err error) {
	prs.onError(prs.index, err)
}

func (prs *parallelReduceSubscriber) OnComplete() {
	prs.mu.Lock()
	result := prs.accumulator
	hasValue := prs.hasValue
	prs.mu.Unlock()

	if hasValue {
		prs.onResult(prs.index, result)
	} else {
		prs.onResult(prs.index, nil)
	}
}

// ============================================================================
// ReduceWith操作符订阅者
// ============================================================================

// parallelReduceWithSubscriber ReduceWith操作符的并行订阅者实现
type parallelReduceWithSubscriber struct {
	BaseSubscriber
	index        int
	initialValue interface{}
	reducer      func(accumulator, value interface{}) interface{}
	accumulator  interface{}
	onResult     func(int, interface{})
	onError      func(int, error)
	mu           sync.Mutex
}

func (prws *parallelReduceWithSubscriber) OnSubscribe(subscription FlowableSubscription) {
	prws.BaseSubscriber.OnSubscribe(subscription)
	prws.accumulator = prws.initialValue
	// 自动请求所有数据
	subscription.Request(9223372036854775807)
}

func (prws *parallelReduceWithSubscriber) OnNext(item Item) {
	if item.IsError() {
		prws.onError(prws.index, item.Error)
		return
	}

	if item.Value == nil {
		prws.OnComplete()
		return
	}

	prws.mu.Lock()
	defer prws.mu.Unlock()

	prws.accumulator = prws.reducer(prws.accumulator, item.Value)
}

func (prws *parallelReduceWithSubscriber) OnError(err error) {
	prws.onError(prws.index, err)
}

func (prws *parallelReduceWithSubscriber) OnComplete() {
	prws.mu.Lock()
	result := prws.accumulator
	prws.mu.Unlock()

	prws.onResult(prws.index, result)
}

// ============================================================================
// RunOn操作符订阅者
// ============================================================================

// parallelRunOnSubscriber RunOn操作符的并行订阅者实现
type parallelRunOnSubscriber struct {
	BaseSubscriber
	downstream Subscriber
	scheduler  Scheduler
	buffer     chan Item
	done       chan struct{}
	started    int32
}

func (pros *parallelRunOnSubscriber) OnSubscribe(subscription FlowableSubscription) {
	pros.BaseSubscriber.OnSubscribe(subscription)

	// 启动调度器工作
	if atomic.CompareAndSwapInt32(&pros.started, 0, 1) {
		go pros.scheduleWork()
	}

	pros.downstream.OnSubscribe(subscription)
}

func (pros *parallelRunOnSubscriber) scheduleWork() {
	pros.scheduler.Schedule(func() {
		for {
			select {
			case item, ok := <-pros.buffer:
				if !ok {
					return
				}
				if item.IsError() {
					pros.downstream.OnError(item.Error)
				} else if item.Value == nil {
					pros.downstream.OnComplete()
				} else {
					pros.downstream.OnNext(item)
				}
			case <-pros.done:
				return
			}
		}
	})
}

func (pros *parallelRunOnSubscriber) OnNext(item Item) {
	select {
	case pros.buffer <- item:
	case <-pros.done:
	}
}

func (pros *parallelRunOnSubscriber) OnError(err error) {
	select {
	case pros.buffer <- CreateErrorItem(err):
	case <-pros.done:
	}
}

func (pros *parallelRunOnSubscriber) OnComplete() {
	select {
	case pros.buffer <- CreateItem(nil):
	case <-pros.done:
	}
	close(pros.buffer)
}

// ============================================================================
// SubscribeOn操作符订阅者
// ============================================================================

// subscribeOnSubscriber SubscribeOn操作符的订阅者实现
type subscribeOnSubscriber struct {
	delegate   Subscriber
	disposable Disposable
}

func (sos *subscribeOnSubscriber) OnSubscribe(subscription FlowableSubscription) {
	// 包装subscription以支持disposable
	wrappedSubscription := &disposableSubscription{
		delegate:   subscription,
		disposable: sos.disposable,
	}
	sos.delegate.OnSubscribe(wrappedSubscription)
}

func (sos *subscribeOnSubscriber) OnNext(item Item) {
	sos.delegate.OnNext(item)
}

func (sos *subscribeOnSubscriber) OnError(err error) {
	sos.delegate.OnError(err)
}

func (sos *subscribeOnSubscriber) OnComplete() {
	sos.delegate.OnComplete()
}

// ============================================================================
// Sequential转换订阅者
// ============================================================================

// parallelSequentialSubscriber Sequential转换的主订阅者
type parallelSequentialSubscriber struct {
	downstream  Subscriber
	parallelism int
	completed   []bool
	hasError    bool
	mu          sync.Mutex
}

func (pss *parallelSequentialSubscriber) onPartitionNext(item Item) {
	pss.mu.Lock()
	defer pss.mu.Unlock()

	if pss.hasError {
		return
	}

	pss.downstream.OnNext(item)
}

func (pss *parallelSequentialSubscriber) onPartitionError(err error) {
	pss.mu.Lock()
	defer pss.mu.Unlock()

	if !pss.hasError {
		pss.hasError = true
		pss.downstream.OnError(err)
	}
}

func (pss *parallelSequentialSubscriber) onPartitionComplete(partitionIndex int) {
	pss.mu.Lock()
	defer pss.mu.Unlock()

	if pss.hasError {
		return
	}

	pss.completed[partitionIndex] = true

	// 检查是否所有分区都完成
	allCompleted := true
	for _, comp := range pss.completed {
		if !comp {
			allCompleted = false
			break
		}
	}

	if allCompleted {
		pss.downstream.OnComplete()
	}
}

// sequentialPartitionSubscriber Sequential转换的分区订阅者
type sequentialPartitionSubscriber struct {
	BaseSubscriber
	partitionIndex int
	parent         *parallelSequentialSubscriber
}

func (sps *sequentialPartitionSubscriber) OnSubscribe(subscription FlowableSubscription) {
	sps.BaseSubscriber.OnSubscribe(subscription)
	// 自动请求所有数据
	subscription.Request(9223372036854775807)
}

func (sps *sequentialPartitionSubscriber) OnNext(item Item) {
	if item.IsError() {
		sps.parent.onPartitionError(item.Error)
		return
	}

	if item.Value == nil {
		sps.parent.onPartitionComplete(sps.partitionIndex)
		return
	}

	sps.parent.onPartitionNext(item)
}

func (sps *sequentialPartitionSubscriber) OnError(err error) {
	sps.parent.onPartitionError(err)
}

func (sps *sequentialPartitionSubscriber) OnComplete() {
	sps.parent.onPartitionComplete(sps.partitionIndex)
}

// ============================================================================
// 辅助结构体
// ============================================================================

// disposableSubscription 支持Disposable的订阅包装器
type disposableSubscription struct {
	delegate   FlowableSubscription
	disposable Disposable
}

func (ds *disposableSubscription) Request(n int64) {
	ds.delegate.Request(n)
}

func (ds *disposableSubscription) Cancel() {
	ds.delegate.Cancel()
	if ds.disposable != nil {
		ds.disposable.Dispose()
	}
}

func (ds *disposableSubscription) IsCancelled() bool {
	return ds.delegate.IsCancelled()
}
