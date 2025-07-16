// RxGo ConditionalSubscriber微融合机制
// 对标RxJava的ConditionalSubscriber接口，提供操作符级别的微优化
package rxgo

import (
	"sync"
	"sync/atomic"
)

// ============================================================================
// ConditionalSubscriber接口定义
// ============================================================================

// ConditionalSubscriber 条件订阅者接口
// 对标RxJava的ConditionalSubscriber，支持条件性数据传递
type ConditionalSubscriber interface {
	// Call 处理Item（实现Observer接口）
	Call(item Item)

	// TryOnNext 尝试传递下一个值
	// 返回true表示值被接受，false表示被拒绝（例如被filter过滤）
	TryOnNext(value interface{}) bool

	// OnError 处理错误
	OnError(err error)

	// OnComplete 处理完成
	OnComplete()

	// SetSubscription 设置订阅
	SetSubscription(subscription Subscription)

	// IsUnsubscribed 检查是否已取消订阅
	IsUnsubscribed() bool
}

// ConditionalObserver 条件观察者函数类型
type ConditionalObserver func(item Item) bool

// ============================================================================
// ConditionalSource接口
// ============================================================================

// ConditionalSource 条件源接口
// 表示可以与ConditionalSubscriber高效交互的源
type ConditionalSource interface {
	// SubscribeConditional 订阅条件订阅者
	SubscribeConditional(subscriber ConditionalSubscriber)
}

// ============================================================================
// 基础ConditionalSubscriber实现
// ============================================================================

// conditionalSubscriberImpl 条件订阅者的基础实现
type conditionalSubscriberImpl struct {
	onNext       ConditionalObserver
	onError      OnError
	onComplete   OnComplete
	subscription Subscription
	cancelled    int32
	mu           sync.RWMutex
}

// NewConditionalSubscriber 创建新的条件订阅者
func NewConditionalSubscriber(
	onNext ConditionalObserver,
	onError OnError,
	onComplete OnComplete,
) ConditionalSubscriber {
	return &conditionalSubscriberImpl{
		onNext:     onNext,
		onError:    onError,
		onComplete: onComplete,
	}
}

// TryOnNext 实现ConditionalSubscriber接口
func (cs *conditionalSubscriberImpl) TryOnNext(value interface{}) bool {
	if atomic.LoadInt32(&cs.cancelled) != 0 {
		return false
	}

	if cs.onNext != nil {
		return cs.onNext(Item{Value: value})
	}
	return true
}

// Call 实现Observer接口
func (cs *conditionalSubscriberImpl) Call(item Item) {
	if item.IsError() {
		cs.OnError(item.Error)
	} else if item.Value == nil {
		cs.OnComplete()
	} else {
		cs.TryOnNext(item.Value)
	}
}

// OnError 实现ConditionalSubscriber接口
func (cs *conditionalSubscriberImpl) OnError(err error) {
	if atomic.LoadInt32(&cs.cancelled) != 0 {
		return
	}

	if cs.onError != nil {
		cs.onError(err)
	}
}

// OnComplete 实现ConditionalSubscriber接口
func (cs *conditionalSubscriberImpl) OnComplete() {
	if atomic.LoadInt32(&cs.cancelled) != 0 {
		return
	}

	if cs.onComplete != nil {
		cs.onComplete()
	}
}

// SetSubscription 实现ConditionalSubscriber接口
func (cs *conditionalSubscriberImpl) SetSubscription(subscription Subscription) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.subscription = subscription
}

// IsUnsubscribed 实现ConditionalSubscriber接口
func (cs *conditionalSubscriberImpl) IsUnsubscribed() bool {
	return atomic.LoadInt32(&cs.cancelled) != 0
}

// Cancel 取消订阅
func (cs *conditionalSubscriberImpl) Cancel() {
	atomic.StoreInt32(&cs.cancelled, 1)
	cs.mu.RLock()
	sub := cs.subscription
	cs.mu.RUnlock()

	if sub != nil {
		sub.Unsubscribe()
	}
}

// ============================================================================
// Observer到ConditionalSubscriber的适配器
// ============================================================================

// observerToConditionalAdapter 普通Observer到ConditionalSubscriber的适配器
type observerToConditionalAdapter struct {
	observer     Observer
	subscription Subscription
	cancelled    int32
}

// NewObserverToConditionalAdapter 创建适配器
func NewObserverToConditionalAdapter(observer Observer) ConditionalSubscriber {
	return &observerToConditionalAdapter{
		observer: observer,
	}
}

// TryOnNext 实现ConditionalSubscriber接口
func (adapter *observerToConditionalAdapter) TryOnNext(value interface{}) bool {
	if atomic.LoadInt32(&adapter.cancelled) != 0 {
		return false
	}

	adapter.observer(Item{Value: value})
	return true
}

// Call 实现Observer接口
func (adapter *observerToConditionalAdapter) Call(item Item) {
	if item.IsError() {
		adapter.OnError(item.Error)
	} else if item.Value == nil {
		adapter.OnComplete()
	} else {
		adapter.TryOnNext(item.Value)
	}
}

// OnError 实现ConditionalSubscriber接口
func (adapter *observerToConditionalAdapter) OnError(err error) {
	if atomic.LoadInt32(&adapter.cancelled) != 0 {
		return
	}
	adapter.observer(Item{Error: err})
}

// OnComplete 实现ConditionalSubscriber接口
func (adapter *observerToConditionalAdapter) OnComplete() {
	if atomic.LoadInt32(&adapter.cancelled) != 0 {
		return
	}
	adapter.observer(Item{})
}

// SetSubscription 实现ConditionalSubscriber接口
func (adapter *observerToConditionalAdapter) SetSubscription(subscription Subscription) {
	adapter.subscription = subscription
}

// IsUnsubscribed 实现ConditionalSubscriber接口
func (adapter *observerToConditionalAdapter) IsUnsubscribed() bool {
	return atomic.LoadInt32(&adapter.cancelled) != 0
}

// ============================================================================
// ConditionalSubscriber到Subscription的适配器
// ============================================================================

// conditionalSubscriberToSubscriptionAdapter ConditionalSubscriber到Subscription的适配器
type conditionalSubscriberToSubscriptionAdapter struct {
	subscriber ConditionalSubscriber
}

// NewConditionalSubscriberToSubscriptionAdapter 创建适配器
func NewConditionalSubscriberToSubscriptionAdapter(subscriber ConditionalSubscriber) Subscription {
	return &conditionalSubscriberToSubscriptionAdapter{
		subscriber: subscriber,
	}
}

// Unsubscribe 实现Subscription接口
func (adapter *conditionalSubscriberToSubscriptionAdapter) Unsubscribe() {
	if cancellable, ok := adapter.subscriber.(*conditionalSubscriberImpl); ok {
		cancellable.Cancel()
	}
}

// IsUnsubscribed 实现Subscription接口
func (adapter *conditionalSubscriberToSubscriptionAdapter) IsUnsubscribed() bool {
	return adapter.subscriber.IsUnsubscribed()
}

// ============================================================================
// 微融合工具函数
// ============================================================================

// IsConditionalSource 检查Observable是否支持条件订阅
func IsConditionalSource(source Observable) bool {
	_, ok := source.(ConditionalSource)
	return ok
}

// TrySubscribeConditional 尝试使用条件订阅，失败则回退到普通订阅
func TrySubscribeConditional(source Observable, subscriber ConditionalSubscriber) Subscription {
	if conditionalSource, ok := source.(ConditionalSource); ok {
		// 支持条件订阅
		conditionalSource.SubscribeConditional(subscriber)
		return NewConditionalSubscriberToSubscriptionAdapter(subscriber)
	} else {
		// 回退到普通订阅
		subscription := source.Subscribe(subscriber.Call)
		subscriber.SetSubscription(subscription)
		return subscription
	}
}

// WrapConditionalObserver 将条件观察者包装为普通观察者
func WrapConditionalObserver(conditionalObserver ConditionalObserver) Observer {
	return func(item Item) {
		conditionalObserver(item)
	}
}

// ============================================================================
// 微融合统计
// ============================================================================

// MicroFusionStats 微融合统计信息
type MicroFusionStats struct {
	TotalAttempts     int64 // 总尝试次数
	SuccessfulFusions int64 // 成功融合次数
	FallbackCount     int64 // 回退到普通订阅的次数
	FilterOptimized   int64 // Filter操作符优化次数
	MapOptimized      int64 // Map操作符优化次数
}

// globalMicroFusionStats 全局微融合统计
var globalMicroFusionStats = &MicroFusionStats{}

// GetMicroFusionStats 获取微融合统计信息
func GetMicroFusionStats() MicroFusionStats {
	return MicroFusionStats{
		TotalAttempts:     atomic.LoadInt64(&globalMicroFusionStats.TotalAttempts),
		SuccessfulFusions: atomic.LoadInt64(&globalMicroFusionStats.SuccessfulFusions),
		FallbackCount:     atomic.LoadInt64(&globalMicroFusionStats.FallbackCount),
		FilterOptimized:   atomic.LoadInt64(&globalMicroFusionStats.FilterOptimized),
		MapOptimized:      atomic.LoadInt64(&globalMicroFusionStats.MapOptimized),
	}
}

// ResetMicroFusionStats 重置微融合统计信息
func ResetMicroFusionStats() {
	atomic.StoreInt64(&globalMicroFusionStats.TotalAttempts, 0)
	atomic.StoreInt64(&globalMicroFusionStats.SuccessfulFusions, 0)
	atomic.StoreInt64(&globalMicroFusionStats.FallbackCount, 0)
	atomic.StoreInt64(&globalMicroFusionStats.FilterOptimized, 0)
	atomic.StoreInt64(&globalMicroFusionStats.MapOptimized, 0)
}

// IncrementFusionAttempt 增加融合尝试计数
func IncrementFusionAttempt() {
	atomic.AddInt64(&globalMicroFusionStats.TotalAttempts, 1)
}

// IncrementSuccessfulFusion 增加成功融合计数
func IncrementSuccessfulFusion() {
	atomic.AddInt64(&globalMicroFusionStats.SuccessfulFusions, 1)
}

// IncrementFallback 增加回退计数
func IncrementFallback() {
	atomic.AddInt64(&globalMicroFusionStats.FallbackCount, 1)
}

// IncrementFilterOptimized 增加Filter优化计数
func IncrementFilterOptimized() {
	atomic.AddInt64(&globalMicroFusionStats.FilterOptimized, 1)
}

// IncrementMapOptimized 增加Map优化计数
func IncrementMapOptimized() {
	atomic.AddInt64(&globalMicroFusionStats.MapOptimized, 1)
}
