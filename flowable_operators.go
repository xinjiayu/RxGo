// Flowable operators implementation for RxGo
// Flowable操作符的具体实现，包含背压处理的订阅者
package rxgo

import (
	"sync"
	"sync/atomic"
)

// ============================================================================
// Map操作符订阅者
// ============================================================================

// mapSubscriber Map操作符的订阅者实现
type mapSubscriber struct {
	BaseSubscriber
	downstream  Subscriber
	transformer Transformer
}

func (ms *mapSubscriber) OnSubscribe(subscription FlowableSubscription) {
	ms.BaseSubscriber.OnSubscribe(subscription)
	ms.downstream.OnSubscribe(subscription)
}

func (ms *mapSubscriber) OnNext(item Item) {
	if item.IsError() {
		ms.downstream.OnError(item.Error)
		return
	}

	if item.Value == nil {
		ms.downstream.OnComplete()
		return
	}

	if result, err := ms.transformer(item.Value); err != nil {
		ms.downstream.OnError(err)
	} else {
		ms.downstream.OnNext(CreateItem(result))
	}
}

func (ms *mapSubscriber) OnError(err error) {
	ms.downstream.OnError(err)
}

func (ms *mapSubscriber) OnComplete() {
	ms.downstream.OnComplete()
}

// ============================================================================
// Filter操作符订阅者
// ============================================================================

// filterSubscriber Filter操作符的订阅者实现
type filterSubscriber struct {
	BaseSubscriber
	downstream Subscriber
	predicate  Predicate
}

func (fs *filterSubscriber) OnSubscribe(subscription FlowableSubscription) {
	fs.BaseSubscriber.OnSubscribe(subscription)
	fs.downstream.OnSubscribe(subscription)
}

func (fs *filterSubscriber) OnNext(item Item) {
	if item.IsError() {
		fs.downstream.OnError(item.Error)
		return
	}

	if item.Value == nil {
		fs.downstream.OnComplete()
		return
	}

	if fs.predicate(item.Value) {
		fs.downstream.OnNext(item)
	}
	// 如果谓词为false，忽略该项目，继续请求下一个
}

func (fs *filterSubscriber) OnError(err error) {
	fs.downstream.OnError(err)
}

func (fs *filterSubscriber) OnComplete() {
	fs.downstream.OnComplete()
}

// ============================================================================
// Take操作符订阅者
// ============================================================================

// takeSubscriber Take操作符的订阅者实现
type takeSubscriber struct {
	BaseSubscriber
	downstream Subscriber
	remaining  int64
	mu         sync.Mutex
}

func (ts *takeSubscriber) OnSubscribe(subscription FlowableSubscription) {
	ts.BaseSubscriber.OnSubscribe(subscription)
	ts.downstream.OnSubscribe(subscription)
}

func (ts *takeSubscriber) OnNext(item Item) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.remaining <= 0 {
		return
	}

	if item.IsError() {
		ts.downstream.OnError(item.Error)
		return
	}

	if item.Value == nil {
		ts.downstream.OnComplete()
		return
	}

	ts.remaining--
	ts.downstream.OnNext(item)

	if ts.remaining <= 0 {
		ts.downstream.OnComplete()
		ts.Cancel()
	}
}

func (ts *takeSubscriber) OnError(err error) {
	ts.downstream.OnError(err)
}

func (ts *takeSubscriber) OnComplete() {
	ts.downstream.OnComplete()
}

// ============================================================================
// Skip操作符订阅者
// ============================================================================

// skipSubscriber Skip操作符的订阅者实现
type skipSubscriber struct {
	BaseSubscriber
	downstream Subscriber
	toSkip     int64
	mu         sync.Mutex
}

func (ss *skipSubscriber) OnSubscribe(subscription FlowableSubscription) {
	ss.BaseSubscriber.OnSubscribe(subscription)
	ss.downstream.OnSubscribe(subscription)
}

func (ss *skipSubscriber) OnNext(item Item) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if item.IsError() {
		ss.downstream.OnError(item.Error)
		return
	}

	if item.Value == nil {
		ss.downstream.OnComplete()
		return
	}

	if ss.toSkip > 0 {
		ss.toSkip--
		return // 跳过此项目
	}

	ss.downstream.OnNext(item)
}

func (ss *skipSubscriber) OnError(err error) {
	ss.downstream.OnError(err)
}

func (ss *skipSubscriber) OnComplete() {
	ss.downstream.OnComplete()
}

// ============================================================================
// FlatMap操作符订阅者
// ============================================================================

// flatMapSubscriber FlatMap操作符的订阅者实现
type flatMapSubscriber struct {
	BaseSubscriber
	downstream     Subscriber
	transformer    func(interface{}) Flowable
	maxConcurrency int
	active         map[int]FlowableSubscription
	nextIndex      int32
	completed      int32
	sourceComplete bool
	mu             sync.RWMutex
}

func (fms *flatMapSubscriber) OnSubscribe(subscription FlowableSubscription) {
	fms.BaseSubscriber.OnSubscribe(subscription)
	fms.downstream.OnSubscribe(subscription)
}

func (fms *flatMapSubscriber) OnNext(item Item) {
	if item.IsError() {
		fms.downstream.OnError(item.Error)
		return
	}

	if item.Value == nil {
		fms.mu.Lock()
		fms.sourceComplete = true
		if len(fms.active) == 0 {
			fms.downstream.OnComplete()
		}
		fms.mu.Unlock()
		return
	}

	// 检查并发限制
	if fms.maxConcurrency > 0 {
		fms.mu.RLock()
		activeCount := len(fms.active)
		fms.mu.RUnlock()

		if activeCount >= fms.maxConcurrency {
			// 暂时缓存或丢弃，这里简化为丢弃
			return
		}
	}

	// 转换为Flowable
	innerFlowable := fms.transformer(item.Value)
	index := int(atomic.AddInt32(&fms.nextIndex, 1))

	// 订阅内部Flowable
	innerSubscription := innerFlowable.SubscribeWithCallbacks(
		func(value interface{}) {
			fms.downstream.OnNext(CreateItem(value))
		},
		func(err error) {
			fms.downstream.OnError(err)
		},
		func() {
			fms.mu.Lock()
			delete(fms.active, index)
			allComplete := fms.sourceComplete && len(fms.active) == 0
			fms.mu.Unlock()

			if allComplete {
				fms.downstream.OnComplete()
			}
		},
	)

	fms.mu.Lock()
	fms.active[index] = innerSubscription
	fms.mu.Unlock()

	// 请求数据
	innerSubscription.Request(1)
}

func (fms *flatMapSubscriber) OnError(err error) {
	fms.downstream.OnError(err)
}

func (fms *flatMapSubscriber) OnComplete() {
	fms.mu.Lock()
	fms.sourceComplete = true
	if len(fms.active) == 0 {
		fms.downstream.OnComplete()
	}
	fms.mu.Unlock()
}

// ============================================================================
// ObserveOn操作符订阅者
// ============================================================================

// observeOnSubscriber ObserveOn操作符的订阅者实现
type observeOnSubscriber struct {
	BaseSubscriber
	downstream Subscriber
	scheduler  Scheduler
	buffer     chan Item
	done       chan struct{}
	started    int32
}

func (oos *observeOnSubscriber) OnSubscribe(subscription FlowableSubscription) {
	oos.BaseSubscriber.OnSubscribe(subscription)

	// 启动调度器工作
	if atomic.CompareAndSwapInt32(&oos.started, 0, 1) {
		go oos.scheduleWork()
	}

	oos.downstream.OnSubscribe(subscription)
}

func (oos *observeOnSubscriber) scheduleWork() {
	oos.scheduler.Schedule(func() {
		for {
			select {
			case item, ok := <-oos.buffer:
				if !ok {
					return
				}
				if item.IsError() {
					oos.downstream.OnError(item.Error)
				} else if item.Value == nil {
					oos.downstream.OnComplete()
				} else {
					oos.downstream.OnNext(item)
				}
			case <-oos.done:
				return
			}
		}
	})
}

func (oos *observeOnSubscriber) OnNext(item Item) {
	select {
	case oos.buffer <- item:
	case <-oos.done:
	}
}

func (oos *observeOnSubscriber) OnError(err error) {
	select {
	case oos.buffer <- CreateErrorItem(err):
	case <-oos.done:
	}
}

func (oos *observeOnSubscriber) OnComplete() {
	select {
	case oos.buffer <- CreateItem(nil):
	case <-oos.done:
	}
	close(oos.buffer)
}

// ============================================================================
// 背压操作符订阅者
// ============================================================================

// backpressureBufferSubscriber 缓冲背压订阅者
type backpressureBufferSubscriber struct {
	BaseSubscriber
	downstream Subscriber
	buffer     []Item
	capacity   int
	mu         sync.Mutex
}

func (bbs *backpressureBufferSubscriber) OnSubscribe(subscription FlowableSubscription) {
	bbs.BaseSubscriber.OnSubscribe(subscription)
	bbs.downstream.OnSubscribe(subscription)
}

func (bbs *backpressureBufferSubscriber) OnNext(item Item) {
	bbs.mu.Lock()
	defer bbs.mu.Unlock()

	// 检查容量限制
	if bbs.capacity > 0 && len(bbs.buffer) >= bbs.capacity {
		// 缓冲区已满，发送错误
		bbs.downstream.OnError(NewBackpressureException("缓冲区溢出"))
		return
	}

	bbs.buffer = append(bbs.buffer, item)
	bbs.drainBuffer()
}

func (bbs *backpressureBufferSubscriber) OnError(err error) {
	bbs.downstream.OnError(err)
}

func (bbs *backpressureBufferSubscriber) OnComplete() {
	bbs.mu.Lock()
	defer bbs.mu.Unlock()
	bbs.drainBuffer()
	bbs.downstream.OnComplete()
}

func (bbs *backpressureBufferSubscriber) drainBuffer() {
	for len(bbs.buffer) > 0 {
		item := bbs.buffer[0]
		bbs.buffer = bbs.buffer[1:]

		if item.IsError() {
			bbs.downstream.OnError(item.Error)
			return
		} else if item.Value == nil {
			bbs.downstream.OnComplete()
			return
		} else {
			bbs.downstream.OnNext(item)
		}
	}
}

// backpressureDropSubscriber 丢弃背压订阅者
type backpressureDropSubscriber struct {
	BaseSubscriber
	downstream Subscriber
	requested  int64
	mu         sync.Mutex
}

func (bds *backpressureDropSubscriber) OnSubscribe(subscription FlowableSubscription) {
	bds.BaseSubscriber.OnSubscribe(subscription)

	// 包装subscription以跟踪请求
	wrappedSubscription := &requestTrackingSubscription{
		delegate: subscription,
		onRequest: func(n int64) {
			bds.mu.Lock()
			bds.requested += n
			bds.mu.Unlock()
		},
	}

	bds.downstream.OnSubscribe(wrappedSubscription)
}

func (bds *backpressureDropSubscriber) OnNext(item Item) {
	bds.mu.Lock()
	defer bds.mu.Unlock()

	if bds.requested > 0 {
		bds.requested--
		bds.downstream.OnNext(item)
	}
	// 否则丢弃项目
}

func (bds *backpressureDropSubscriber) OnError(err error) {
	bds.downstream.OnError(err)
}

func (bds *backpressureDropSubscriber) OnComplete() {
	bds.downstream.OnComplete()
}

// backpressureLatestSubscriber 最新背压订阅者
type backpressureLatestSubscriber struct {
	BaseSubscriber
	downstream Subscriber
	latest     *Item
	requested  int64
	done       bool
	mu         sync.Mutex
}

func (bls *backpressureLatestSubscriber) OnSubscribe(subscription FlowableSubscription) {
	bls.BaseSubscriber.OnSubscribe(subscription)

	// 包装subscription以跟踪请求
	wrappedSubscription := &requestTrackingSubscription{
		delegate: subscription,
		onRequest: func(n int64) {
			bls.mu.Lock()
			bls.requested += n
			bls.tryEmit()
			bls.mu.Unlock()
		},
	}

	bls.downstream.OnSubscribe(wrappedSubscription)
}

func (bls *backpressureLatestSubscriber) OnNext(item Item) {
	bls.mu.Lock()
	defer bls.mu.Unlock()

	bls.latest = &item
	bls.tryEmit()
}

func (bls *backpressureLatestSubscriber) OnError(err error) {
	bls.downstream.OnError(err)
}

func (bls *backpressureLatestSubscriber) OnComplete() {
	bls.mu.Lock()
	defer bls.mu.Unlock()

	bls.done = true
	if bls.latest == nil || bls.requested > 0 {
		bls.downstream.OnComplete()
	}
}

func (bls *backpressureLatestSubscriber) tryEmit() {
	if bls.latest != nil && bls.requested > 0 {
		item := *bls.latest
		bls.latest = nil
		bls.requested--

		if item.IsError() {
			bls.downstream.OnError(item.Error)
		} else if item.Value == nil {
			bls.downstream.OnComplete()
		} else {
			bls.downstream.OnNext(item)
		}

		if bls.done && bls.latest == nil {
			bls.downstream.OnComplete()
		}
	}
}

// ============================================================================
// 辅助结构体
// ============================================================================

// requestTrackingSubscription 跟踪请求的订阅包装器
type requestTrackingSubscription struct {
	delegate  FlowableSubscription
	onRequest func(int64)
}

func (rts *requestTrackingSubscription) Request(n int64) {
	if rts.onRequest != nil {
		rts.onRequest(n)
	}
	rts.delegate.Request(n)
}

func (rts *requestTrackingSubscription) Cancel() {
	rts.delegate.Cancel()
}

func (rts *requestTrackingSubscription) IsCancelled() bool {
	return rts.delegate.IsCancelled()
}

// BackpressureException 背压异常
type BackpressureException struct {
	message string
}

func NewBackpressureException(message string) *BackpressureException {
	return &BackpressureException{message: message}
}

func (e *BackpressureException) Error() string {
	return "BackpressureException: " + e.message
}
