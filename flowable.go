// Flowable implementation for RxGo
// 支持背压处理的高吞吐量数据流实现，基于Reactive Streams规范
package rxgo

import (
	"sync"
	"sync/atomic"
)

// ============================================================================
// 背压溢出策略定义
// ============================================================================

// BackpressureOverflowStrategy 缓冲区溢出策略
type BackpressureOverflowStrategy int

const (
	// OnOverflowError 溢出时抛出错误（默认）
	OnOverflowError BackpressureOverflowStrategy = iota
	// OnOverflowDropLatest 溢出时丢弃最新的数据
	OnOverflowDropLatest
	// OnOverflowDropOldest 溢出时丢弃最旧的数据
	OnOverflowDropOldest
)

// ============================================================================
// Subscriber 接口定义
// ============================================================================

// FlowableSubscription 订阅接口，支持请求管理
type FlowableSubscription interface {
	// Request 请求指定数量的数据项
	Request(n int64)
	// Cancel 取消订阅
	Cancel()
	// IsCancelled 检查是否已取消
	IsCancelled() bool
}

// Subscriber Flowable的订阅者接口
type Subscriber interface {
	// OnSubscribe 订阅开始时调用
	OnSubscribe(subscription FlowableSubscription)
	// OnNext 接收到新数据时调用
	OnNext(item Item)
	// OnError 发生错误时调用
	OnError(err error)
	// OnComplete 数据流完成时调用
	OnComplete()
}

// Publisher 发布者接口，符合Reactive Streams规范
type Publisher interface {
	// Subscribe 订阅Subscriber
	Subscribe(subscriber Subscriber)
}

// ============================================================================
// Flowable 接口定义
// ============================================================================

// Flowable 支持背压的响应式数据流接口
type Flowable interface {
	Publisher

	// ============================================================================
	// 核心订阅方法
	// ============================================================================

	// SubscribeWithCallbacks 使用回调函数订阅
	SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) FlowableSubscription

	// SubscribeOn 指定订阅操作运行的调度器
	SubscribeOn(scheduler Scheduler) Flowable

	// ObserveOn 指定观察操作运行的调度器
	ObserveOn(scheduler Scheduler) Flowable

	// ============================================================================
	// 转换操作符
	// ============================================================================

	// Map 转换每个数据项
	Map(transformer Transformer) Flowable

	// FlatMap 将每个数据项转换为Flowable并合并
	FlatMap(transformer func(interface{}) Flowable, maxConcurrency int) Flowable

	// Filter 过滤数据项
	Filter(predicate Predicate) Flowable

	// Take 取前N个数据项
	Take(count int64) Flowable

	// Skip 跳过前N个数据项
	Skip(count int64) Flowable

	// ============================================================================
	// 背压相关操作符
	// ============================================================================

	// OnBackpressureBuffer 缓冲背压数据
	OnBackpressureBuffer() Flowable

	// OnBackpressureBufferWithCapacity 带容量限制的缓冲
	OnBackpressureBufferWithCapacity(capacity int) Flowable

	// OnBackpressureDrop 丢弃背压数据
	OnBackpressureDrop() Flowable

	// OnBackpressureLatest 保留最新数据
	OnBackpressureLatest() Flowable

	// ============================================================================
	// 转换和终止操作符
	// ============================================================================

	// ToObservable 转换为Observable（忽略背压）
	ToObservable() Observable

	// ToSlice 转换为切片
	ToSlice() Single

	// BlockingFirst 阻塞获取第一个数据项
	BlockingFirst() (interface{}, error)
}

// ============================================================================
// 内部实现结构
// ============================================================================

// subscriptionImpl FlowableSubscription的基础实现
type subscriptionImpl struct {
	requested int64
	cancelled int32
	onRequest func(int64)
	onCancel  func()
	mu        sync.Mutex
}

// NewFlowableSubscription 创建新的FlowableSubscription
func NewFlowableSubscription(onRequest func(int64), onCancel func()) FlowableSubscription {
	return &subscriptionImpl{
		onRequest: onRequest,
		onCancel:  onCancel,
	}
}

// Request 请求指定数量的数据项
func (s *subscriptionImpl) Request(n int64) {
	if n <= 0 || s.IsCancelled() {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 防止溢出
	maxValue := int64(^uint64(0) >> 1) // Long.MAX_VALUE
	if s.requested == maxValue {
		return
	}

	newRequested := s.requested + n
	if newRequested < 0 {
		newRequested = maxValue
	}
	s.requested = newRequested

	if s.onRequest != nil {
		s.onRequest(n)
	}
}

// Cancel 取消订阅
func (s *subscriptionImpl) Cancel() {
	if atomic.CompareAndSwapInt32(&s.cancelled, 0, 1) {
		if s.onCancel != nil {
			s.onCancel()
		}
	}
}

// IsCancelled 检查是否已取消
func (s *subscriptionImpl) IsCancelled() bool {
	return atomic.LoadInt32(&s.cancelled) == 1
}

// ============================================================================
// BaseSubscriber 基础订阅者实现
// ============================================================================

// BaseSubscriber 基础订阅者，提供常用功能
type BaseSubscriber struct {
	subscription FlowableSubscription
	mu           sync.RWMutex
}

// OnSubscribe 订阅开始时调用
func (b *BaseSubscriber) OnSubscribe(subscription FlowableSubscription) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.subscription != nil {
		subscription.Cancel()
		return
	}

	b.subscription = subscription
}

// Request 请求指定数量的数据项
func (b *BaseSubscriber) Request(n int64) {
	b.mu.RLock()
	subscription := b.subscription
	b.mu.RUnlock()

	if subscription != nil {
		subscription.Request(n)
	}
}

// Cancel 取消订阅
func (b *BaseSubscriber) Cancel() {
	b.mu.RLock()
	subscription := b.subscription
	b.mu.RUnlock()

	if subscription != nil {
		subscription.Cancel()
	}
}

// IsCancelled 检查是否已取消
func (b *BaseSubscriber) IsCancelled() bool {
	b.mu.RLock()
	subscription := b.subscription
	b.mu.RUnlock()

	if subscription != nil {
		return subscription.IsCancelled()
	}
	return false
}

// OnNext 默认实现（子类需要重写）
func (b *BaseSubscriber) OnNext(item Item) {
	// 默认空实现
}

// OnError 默认实现（子类需要重写）
func (b *BaseSubscriber) OnError(err error) {
	// 默认空实现
}

// OnComplete 默认实现（子类需要重写）
func (b *BaseSubscriber) OnComplete() {
	// 默认空实现
}
