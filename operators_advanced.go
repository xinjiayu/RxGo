// Advanced operators and factory functions for RxGo
// 高级操作符和工厂函数实现，包含Create, Defer等
package rxgo

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// 高级工厂函数
// ============================================================================

// 注意：Create和Defer函数已在factory.go中定义

// Generate 使用生成器函数创建Observable
func Generate(initialState interface{}, condition func(interface{}) bool, iterate func(interface{}) interface{}, resultSelector func(interface{}) interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			defer cancel()

			state := initialState
			for condition(state) {
				select {
				case <-ctx.Done():
					return
				default:
					result := resultSelector(state)
					observer(CreateItem(result))
					state = iterate(state)
				}
			}

			observer(CreateItem(nil)) // 完成信号
		}()

		return NewBaseSubscription(NewBaseDisposable(cancel))
	})
}

// Using 使用资源创建Observable，确保资源被正确释放
func Using(resourceFactory func() interface{}, observableFactory func(interface{}) Observable, disposeAction func(interface{})) Observable {
	return NewObservable(func(observer Observer) Subscription {
		resource := resourceFactory()
		observable := observableFactory(resource)

		var disposed int32

		// 包装observer，在完成或错误时也释放资源
		wrappedObserver := func(item Item) {
			observer(item)

			// 如果是终止信号（完成或错误），释放资源
			if (item.IsError() || item.Value == nil) && atomic.CompareAndSwapInt32(&disposed, 0, 1) {
				if disposeAction != nil {
					disposeAction(resource)
				}
			}
		}

		subscription := observable.Subscribe(wrappedObserver)

		return NewBaseSubscription(NewBaseDisposable(func() {
			subscription.Unsubscribe()
			// 确保资源被释放（如果还没有被释放的话）
			if atomic.CompareAndSwapInt32(&disposed, 0, 1) {
				if disposeAction != nil {
					disposeAction(resource)
				}
			}
		}))
	})
}

// ============================================================================
// 高级操作符
// ============================================================================

// GroupBy 根据键选择器对Observable的项进行分组
func (o *observableImpl) GroupBy(keySelector func(interface{}) interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		groups := make(map[interface{}]*PublishSubject)
		mu := &sync.RWMutex{}

		return o.Subscribe(func(item Item) {
			if item.IsError() {
				// 向所有组发送错误
				mu.RLock()
				for _, group := range groups {
					group.OnError(item.Error)
				}
				mu.RUnlock()
				observer(item)
				return
			}

			if item.Value == nil {
				// 向所有组发送完成信号
				mu.RLock()
				for _, group := range groups {
					group.OnComplete()
				}
				mu.RUnlock()
				observer(item)
				return
			}

			// 获取键
			key := keySelector(item.Value)

			mu.Lock()
			group, exists := groups[key]
			if !exists {
				group = NewPublishSubject()
				groups[key] = group

				// 创建分组Observable
				groupedObservable := &GroupedObservable{
					Key:        key,
					Observable: group,
				}
				observer(CreateItem(groupedObservable))
			}
			mu.Unlock()

			// 向对应的组发送值
			group.OnNext(item.Value)
		})
	})
}

// GroupByDynamic 动态分组，支持自定义分组逻辑
func (o *observableImpl) GroupByDynamic(keySelector func(interface{}) string, elementSelector func(interface{}) interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		groups := make(map[string]*PublishSubject)
		mu := &sync.RWMutex{}

		return o.Subscribe(func(item Item) {
			if item.IsError() {
				// 向所有组发送错误
				mu.RLock()
				for _, group := range groups {
					group.OnError(item.Error)
				}
				mu.RUnlock()
				observer(item)
				return
			}

			if item.Value == nil {
				// 向所有组发送完成信号
				mu.RLock()
				for _, group := range groups {
					group.OnComplete()
				}
				mu.RUnlock()
				observer(item)
				return
			}

			// 获取键和元素
			key := keySelector(item.Value)
			element := elementSelector(item.Value)

			mu.Lock()
			group, exists := groups[key]
			if !exists {
				group = NewPublishSubject()
				groups[key] = group

				// 创建分组Observable
				groupedObservable := &GroupedObservable{
					Key:        key,
					Observable: group,
				}
				observer(CreateItem(groupedObservable))
			}
			mu.Unlock()

			// 向对应的组发送元素
			group.OnNext(element)
		})
	})
}

// SwitchMap 切换映射，每次发射新的Observable时取消订阅之前的Observable
func (o *observableImpl) SwitchMap(selector func(interface{}) Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		var currentSubscription Subscription
		mu := &sync.Mutex{}

		return o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				observer(item)
				return
			}

			// 取消之前的订阅
			mu.Lock()
			if currentSubscription != nil {
				currentSubscription.Unsubscribe()
			}
			mu.Unlock()

			// 创建新的Observable并订阅
			innerObservable := selector(item.Value)
			newSubscription := innerObservable.Subscribe(observer)

			mu.Lock()
			currentSubscription = newSubscription
			mu.Unlock()
		})
	})
}

// ConcatMap 连接映射，按顺序处理每个Observable
func (o *observableImpl) ConcatMap(selector func(interface{}) Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		queue := make([]Observable, 0)
		mu := &sync.Mutex{}
		processing := int32(0)

		var processNext func()
		processNext = func() {
			mu.Lock()
			if len(queue) == 0 {
				atomic.StoreInt32(&processing, 0)
				mu.Unlock()
				return
			}

			next := queue[0]
			queue = queue[1:]
			mu.Unlock()

			next.Subscribe(func(item Item) {
				observer(item)
				if item.IsError() || item.Value == nil {
					// 处理下一个
					go processNext()
				}
			})
		}

		return o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				observer(item)
				return
			}

			innerObservable := selector(item.Value)

			mu.Lock()
			queue = append(queue, innerObservable)
			mu.Unlock()

			if atomic.CompareAndSwapInt32(&processing, 0, 1) {
				go processNext()
			}
		})
	})
}

// MergeMap 合并映射，同时处理多个Observable
func (o *observableImpl) MergeMap(selector func(interface{}) Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		activeCount := int32(0)
		completed := int32(0)

		return o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				atomic.StoreInt32(&completed, 1)
				// 如果没有活跃的内部Observable，发送完成信号
				if atomic.LoadInt32(&activeCount) == 0 {
					observer(item)
				}
				return
			}

			innerObservable := selector(item.Value)
			atomic.AddInt32(&activeCount, 1)

			innerObservable.Subscribe(func(innerItem Item) {
				observer(innerItem)

				if innerItem.IsError() || innerItem.Value == nil {
					// 内部Observable完成
					remaining := atomic.AddInt32(&activeCount, -1)
					if remaining == 0 && atomic.LoadInt32(&completed) == 1 {
						observer(CreateItem(nil)) // 发送完成信号
					}
				}
			})
		})
	})
}

// ============================================================================
// 分组Observable
// ============================================================================

// GroupedObservable 分组Observable，包含键和Observable
type GroupedObservable struct {
	Key        interface{}
	Observable Observable
}

// GetKey 获取分组键
func (g *GroupedObservable) GetKey() interface{} {
	return g.Key
}

// GetObservable 获取Observable
func (g *GroupedObservable) GetObservable() Observable {
	return g.Observable
}

// ============================================================================
// 其他高级操作符
// ============================================================================

// Amb 歧义操作符，返回第一个发射项的Observable
func Amb(observables ...Observable) Observable {
	if len(observables) == 0 {
		return Empty()
	}

	if len(observables) == 1 {
		return observables[0]
	}

	return NewObservable(func(observer Observer) Subscription {
		winner := int32(-1)
		subscriptions := make([]Subscription, len(observables))

		for i, obs := range observables {
			index := i
			subscriptions[i] = obs.Subscribe(func(item Item) {
				// 检查是否是第一个发射的Observable
				if atomic.CompareAndSwapInt32(&winner, -1, int32(index)) {
					// 取消其他订阅
					for j, sub := range subscriptions {
						if j != index {
							sub.Unsubscribe()
						}
					}
				}

				// 如果是获胜者，转发项目
				if atomic.LoadInt32(&winner) == int32(index) {
					observer(item)
				}
			})
		}

		return NewBaseSubscription(NewBaseDisposable(func() {
			for _, sub := range subscriptions {
				sub.Unsubscribe()
			}
		}))
	})
}

// Race 竞争操作符，与Amb相同
func Race(observables ...Observable) Observable {
	return Amb(observables...)
}

// ============================================================================
// Phase 3 转换操作符扩展
// ============================================================================

// StartWith 在Observable序列开始前发射指定的值
func (o *observableImpl) StartWith(values ...interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		// 创建复合订阅
		compositeSubscription := NewCompositeSubscription()

		// 先发射StartWith的值
		go func() {
			for _, value := range values {
				select {
				case <-compositeSubscription.Context().Done():
					return
				default:
					observer(CreateItem(value))
				}
			}

			// 然后订阅原Observable
			subscription := o.Subscribe(observer)
			compositeSubscription.Add(subscription)
		}()

		return compositeSubscription
	})
}

// StartWithIterable 在Observable序列开始前发射可迭代对象的值
func (o *observableImpl) StartWithIterable(iterable func() <-chan interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		compositeSubscription := NewCompositeSubscription()

		go func() {
			// 先发射iterable的值
			ch := iterable()
			for value := range ch {
				select {
				case <-compositeSubscription.Context().Done():
					return
				default:
					observer(CreateItem(value))
				}
			}

			// 然后订阅原Observable
			subscription := o.Subscribe(observer)
			compositeSubscription.Add(subscription)
		}()

		return compositeSubscription
	})
}

// Timestamp 为每个发射的项目添加时间戳
func (o *observableImpl) Timestamp() Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				observer(item) // 完成信号
				return
			}

			// 创建带时间戳的值
			timestampedValue := Timestamped{
				Value:     item.Value,
				Timestamp: time.Now(),
			}

			observer(CreateItem(timestampedValue))
		})
	})
}

// TimeInterval 计算每个发射项目之间的时间间隔
func (o *observableImpl) TimeInterval() Observable {
	return NewObservable(func(observer Observer) Subscription {
		var lastTime int64 = time.Now().UnixNano()

		return o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				observer(item) // 完成信号
				return
			}

			// 计算时间间隔
			currentTime := time.Now().UnixNano()
			interval := time.Duration(currentTime - atomic.SwapInt64(&lastTime, currentTime))

			// 创建带时间间隔的值
			timeIntervalValue := TimeInterval{
				Value:    item.Value,
				Interval: interval,
			}

			observer(CreateItem(timeIntervalValue))
		})
	})
}

// Cast 将Observable的类型转换为指定类型
func (o *observableImpl) Cast(targetType reflect.Type) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				observer(item) // 完成信号
				return
			}

			// 类型转换
			value := reflect.ValueOf(item.Value)
			if value.Type().ConvertibleTo(targetType) {
				convertedValue := value.Convert(targetType).Interface()
				observer(CreateItem(convertedValue))
			} else {
				observer(CreateErrorItem(NewCastError("Cannot cast %T to %v", item.Value, targetType)))
			}
		})
	})
}

// OfType 过滤Observable，只发射指定类型的项目
func (o *observableImpl) OfType(targetType reflect.Type) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				observer(item) // 完成信号
				return
			}

			// 类型检查
			value := reflect.ValueOf(item.Value)
			if value.Type() == targetType || value.Type().AssignableTo(targetType) {
				observer(item)
			}
			// 不匹配的类型被忽略
		})
	})
}

// ============================================================================
// Phase 3 组合操作符扩展
// ============================================================================

// WithLatestFrom 与另一个Observable的最新值组合
func (o *observableImpl) WithLatestFrom(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		var latestFromOther interface{}
		var hasValueFromOther int32
		var mutex sync.RWMutex

		compositeSubscription := NewCompositeSubscription()

		// 订阅other Observable来获取最新值
		otherSubscription := other.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				// other完成，但不影响主流
				return
			}

			mutex.Lock()
			latestFromOther = item.Value
			atomic.StoreInt32(&hasValueFromOther, 1)
			mutex.Unlock()
		})
		compositeSubscription.Add(otherSubscription)

		// 订阅主Observable
		mainSubscription := o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				observer(item) // 完成信号
				return
			}

			// 只有当other有值时才发射组合结果
			if atomic.LoadInt32(&hasValueFromOther) == 1 {
				mutex.RLock()
				latestValue := latestFromOther
				mutex.RUnlock()

				if combiner != nil {
					result := combiner(item.Value, latestValue)
					observer(CreateItem(result))
				} else {
					// 默认组合为数组
					observer(CreateItem([]interface{}{item.Value, latestValue}))
				}
			}
		})
		compositeSubscription.Add(mainSubscription)

		return compositeSubscription
	})
}

// ============================================================================
// Phase 3 工具操作符扩展
// ============================================================================

// DoFinally 在Observable终止时（完成或错误）执行最终操作
func (o *observableImpl) DoFinally(action func()) Observable {
	return NewObservable(func(observer Observer) Subscription {
		subscription := o.Subscribe(func(item Item) {
			observer(item)

			// 如果是终止信号（错误或完成），执行finally操作
			if item.IsError() || item.Value == nil {
				if action != nil {
					action()
				}
			}
		})

		// 包装订阅，确保取消时也执行finally
		return NewSubscription(func() {
			subscription.Unsubscribe()
			if action != nil {
				action()
			}
		})
	})
}

// DoAfterTerminate 在Observable终止后执行操作
func (o *observableImpl) DoAfterTerminate(action func()) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() || item.Value == nil {
				observer(item)
				// 在发射终止信号后执行操作
				if action != nil {
					action()
				}
			} else {
				observer(item)
			}
		})
	})
}

// Cache 缓存Observable的发射序列，多个订阅者共享相同的序列
func (o *observableImpl) Cache() Observable {
	return o.CacheWithCapacity(-1) // 无限容量
}

// CacheWithCapacity 带容量限制的缓存
func (o *observableImpl) CacheWithCapacity(capacity int) Observable {
	var (
		items        []Item
		subscribers  []Observer
		mutex        sync.RWMutex
		subscription Subscription
		once         sync.Once
		terminated   bool
	)

	return NewObservable(func(observer Observer) Subscription {
		mutex.Lock()
		defer mutex.Unlock()

		// 如果已经有缓存的数据，立即发射
		if terminated {
			// 已完成或错误，发射缓存的数据
			go func() {
				mutex.RLock()
				cachedItems := make([]Item, len(items))
				copy(cachedItems, items)
				mutex.RUnlock()

				for _, item := range cachedItems {
					observer(item)
				}
			}()
			return NewEmptySubscription()
		}

		// 添加到订阅者列表
		subscribers = append(subscribers, observer)

		// 第一次订阅时，启动源Observable
		once.Do(func() {
			subscription = o.Subscribe(func(item Item) {
				mutex.Lock()
				defer mutex.Unlock()

				// 缓存项目（如果有容量限制且不是终止信号）
				if !item.IsError() && item.Value != nil {
					if capacity > 0 && len(items) >= capacity {
						// 移除最老的项目
						items = items[1:]
					}
				}
				items = append(items, item)

				// 广播给所有订阅者
				for _, sub := range subscribers {
					sub(item)
				}

				// 如果是终止信号，更新状态
				if item.IsError() || item.Value == nil {
					terminated = true
				}
			})
		})

		return NewSubscription(func() {
			mutex.Lock()
			defer mutex.Unlock()

			// 从订阅者列表中移除
			for i, sub := range subscribers {
				if reflect.ValueOf(sub).Pointer() == reflect.ValueOf(observer).Pointer() {
					subscribers = append(subscribers[:i], subscribers[i+1:]...)
					break
				}
			}

			// 如果没有更多订阅者，取消源订阅
			if len(subscribers) == 0 && subscription != nil {
				subscription.Unsubscribe()
			}
		})
	})
}

// Replay 重放操作符，缓存指定数量的项目并重放给新订阅者
func (o *observableImpl) Replay(bufferSize int) ConnectableObservable {
	return ReplayWithBufferSize(o, bufferSize)
}

// ReplayWithBufferSize 创建可重放的ConnectableObservable，指定缓冲区大小
func ReplayWithBufferSize(source Observable, bufferSize int) ConnectableObservable {
	subject := &replaySubject{
		buffer:     make([]Item, 0, bufferSize),
		bufferSize: bufferSize,
		observers:  make([]Observer, 0),
	}

	return &connectableReplayObservable{
		source:  source,
		subject: subject,
	}
}

// ============================================================================
// Replay实现相关类型
// ============================================================================

// replaySubject 重放主题，缓存指定数量的项目
type replaySubject struct {
	buffer     []Item
	bufferSize int
	observers  []Observer
	completed  bool
	errored    bool
	error      error
	mu         sync.RWMutex
}

func (rs *replaySubject) Subscribe(observer Observer) Subscription {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	// 立即重放缓冲的项目
	for _, item := range rs.buffer {
		observer(item)
	}

	// 如果已经完成或出错，立即发送终止信号
	if rs.completed {
		observer(CreateItem(nil))
		return NewEmptySubscription()
	}
	if rs.errored {
		observer(CreateItem(rs.error))
		return NewEmptySubscription()
	}

	// 添加到观察者列表
	rs.observers = append(rs.observers, observer)

	return NewSubscription(func() {
		rs.mu.Lock()
		defer rs.mu.Unlock()
		// 从观察者列表中移除
		for i, obs := range rs.observers {
			if reflect.ValueOf(obs).Pointer() == reflect.ValueOf(observer).Pointer() {
				rs.observers = append(rs.observers[:i], rs.observers[i+1:]...)
				break
			}
		}
	})
}

func (rs *replaySubject) OnNext(value interface{}) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.completed || rs.errored {
		return
	}

	item := CreateItem(value)

	// 添加到缓冲区
	if len(rs.buffer) >= rs.bufferSize && rs.bufferSize > 0 {
		// 移除最老的项目
		rs.buffer = rs.buffer[1:]
	}
	rs.buffer = append(rs.buffer, item)

	// 广播给所有观察者
	for _, observer := range rs.observers {
		observer(item)
	}
}

func (rs *replaySubject) OnError(err error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.completed || rs.errored {
		return
	}

	rs.errored = true
	rs.error = err
	item := CreateItem(err)

	// 广播给所有观察者
	for _, observer := range rs.observers {
		observer(item)
	}

	rs.observers = nil
}

func (rs *replaySubject) OnComplete() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.completed || rs.errored {
		return
	}

	rs.completed = true
	item := CreateItem(nil)

	// 广播给所有观察者
	for _, observer := range rs.observers {
		observer(item)
	}

	rs.observers = nil
}

// connectableReplayObservable 可连接的重放Observable
type connectableReplayObservable struct {
	source     Observable
	subject    *replaySubject
	connected  bool
	connection Subscription
	mu         sync.Mutex
}

func (cro *connectableReplayObservable) Subscribe(observer Observer) Subscription {
	return cro.subject.Subscribe(observer)
}

func (cro *connectableReplayObservable) SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) Subscription {
	observer := func(item Item) {
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
	}
	return cro.Subscribe(observer)
}

func (cro *connectableReplayObservable) Connect() Disposable {
	cro.mu.Lock()
	defer cro.mu.Unlock()

	if cro.connected {
		return NewBaseDisposable(func() {})
	}

	cro.connected = true
	cro.connection = cro.source.Subscribe(func(item Item) {
		if item.IsError() {
			cro.subject.OnError(item.Error)
		} else if item.Value == nil {
			cro.subject.OnComplete()
		} else {
			cro.subject.OnNext(item.Value)
		}
	})

	return NewBaseDisposable(func() {
		cro.mu.Lock()
		defer cro.mu.Unlock()
		if cro.connection != nil {
			cro.connection.Unsubscribe()
			cro.connected = false
		}
	})
}

func (cro *connectableReplayObservable) ConnectWithContext(ctx context.Context) Disposable {
	disposable := cro.Connect()

	go func() {
		<-ctx.Done()
		disposable.Dispose()
	}()

	return disposable
}

func (cro *connectableReplayObservable) RefCount() Observable {
	return NewObservable(func(observer Observer) Subscription {
		subscription := cro.Subscribe(observer)
		cro.Connect()
		return subscription
	})
}

func (cro *connectableReplayObservable) AutoConnect(subscriberCount int) Observable {
	var subscriptions int32

	return NewObservable(func(observer Observer) Subscription {
		subscription := cro.Subscribe(observer)

		if atomic.AddInt32(&subscriptions, 1) == int32(subscriberCount) {
			cro.Connect()
		}

		return NewSubscription(func() {
			subscription.Unsubscribe()
			atomic.AddInt32(&subscriptions, -1)
		})
	})
}

func (cro *connectableReplayObservable) IsConnected() bool {
	cro.mu.Lock()
	defer cro.mu.Unlock()
	return cro.connected
}

// 实现Observable接口的所有其他方法（委托给subject）
func (cro *connectableReplayObservable) Map(transformer Transformer) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Map(transformer)
}
func (cro *connectableReplayObservable) Filter(predicate Predicate) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Filter(predicate)
}
func (cro *connectableReplayObservable) Take(count int) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Take(count)
}
func (cro *connectableReplayObservable) Skip(count int) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Skip(count)
}
func (cro *connectableReplayObservable) TakeLast(count int) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).TakeLast(count)
}
func (cro *connectableReplayObservable) TakeUntil(other Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).TakeUntil(other)
}
func (cro *connectableReplayObservable) TakeWhile(predicate Predicate) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).TakeWhile(predicate)
}
func (cro *connectableReplayObservable) SkipLast(count int) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).SkipLast(count)
}
func (cro *connectableReplayObservable) SkipUntil(other Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).SkipUntil(other)
}
func (cro *connectableReplayObservable) SkipWhile(predicate Predicate) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).SkipWhile(predicate)
}
func (cro *connectableReplayObservable) Distinct() Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Distinct()
}
func (cro *connectableReplayObservable) DistinctUntilChanged() Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).DistinctUntilChanged()
}
func (cro *connectableReplayObservable) FlatMap(transformer func(interface{}) Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).FlatMap(transformer)
}
func (cro *connectableReplayObservable) Merge(other Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Merge(other)
}
func (cro *connectableReplayObservable) Concat(other Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Concat(other)
}
func (cro *connectableReplayObservable) Zip(other Observable, zipper func(interface{}, interface{}) interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Zip(other, zipper)
}
func (cro *connectableReplayObservable) CombineLatest(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).CombineLatest(other, combiner)
}
func (cro *connectableReplayObservable) Reduce(reducer Reducer) Single {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Reduce(reducer)
}
func (cro *connectableReplayObservable) Scan(reducer Reducer) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Scan(reducer)
}
func (cro *connectableReplayObservable) Count() Single {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Count()
}
func (cro *connectableReplayObservable) First() Single {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).First()
}
func (cro *connectableReplayObservable) Last() Single {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Last()
}
func (cro *connectableReplayObservable) Min() Single {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Min()
}
func (cro *connectableReplayObservable) Max() Single {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Max()
}
func (cro *connectableReplayObservable) Sum() Single {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Sum()
}
func (cro *connectableReplayObservable) Average() Single {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Average()
}
func (cro *connectableReplayObservable) All(predicate Predicate) Single {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).All(predicate)
}
func (cro *connectableReplayObservable) Any(predicate Predicate) Single {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Any(predicate)
}
func (cro *connectableReplayObservable) Contains(value interface{}) Single {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Contains(value)
}
func (cro *connectableReplayObservable) ElementAt(index int) Single {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).ElementAt(index)
}
func (cro *connectableReplayObservable) Buffer(count int) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Buffer(count)
}
func (cro *connectableReplayObservable) BufferWithTime(timespan time.Duration) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).BufferWithTime(timespan)
}
func (cro *connectableReplayObservable) Window(count int) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Window(count)
}
func (cro *connectableReplayObservable) WindowWithTime(timespan time.Duration) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).WindowWithTime(timespan)
}
func (cro *connectableReplayObservable) Debounce(duration time.Duration) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Debounce(duration)
}
func (cro *connectableReplayObservable) Throttle(duration time.Duration) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Throttle(duration)
}
func (cro *connectableReplayObservable) Delay(duration time.Duration) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Delay(duration)
}
func (cro *connectableReplayObservable) Timeout(duration time.Duration) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Timeout(duration)
}
func (cro *connectableReplayObservable) Catch(handler func(error) Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Catch(handler)
}
func (cro *connectableReplayObservable) Retry(count int) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Retry(count)
}
func (cro *connectableReplayObservable) RetryWhen(handler func(Observable) Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).RetryWhen(handler)
}
func (cro *connectableReplayObservable) DoOnNext(action OnNext) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).DoOnNext(action)
}
func (cro *connectableReplayObservable) DoOnError(action OnError) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).DoOnError(action)
}
func (cro *connectableReplayObservable) DoOnComplete(action OnComplete) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).DoOnComplete(action)
}
func (cro *connectableReplayObservable) SubscribeOn(scheduler Scheduler) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).SubscribeOn(scheduler)
}
func (cro *connectableReplayObservable) ObserveOn(scheduler Scheduler) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).ObserveOn(scheduler)
}
func (cro *connectableReplayObservable) ToSlice() Single {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).ToSlice()
}
func (cro *connectableReplayObservable) ToChannel() <-chan Item {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).ToChannel()
}
func (cro *connectableReplayObservable) Publish() ConnectableObservable { return cro }
func (cro *connectableReplayObservable) Share() Observable              { return cro.RefCount() }
func (cro *connectableReplayObservable) BlockingSubscribe(observer Observer) {
	NewObservable(func(obs Observer) Subscription {
		return cro.Subscribe(obs)
	}).BlockingSubscribe(observer)
}
func (cro *connectableReplayObservable) BlockingFirst() (interface{}, error) {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).BlockingFirst()
}
func (cro *connectableReplayObservable) BlockingLast() (interface{}, error) {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).BlockingLast()
}
func (cro *connectableReplayObservable) StartWith(values ...interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).StartWith(values...)
}
func (cro *connectableReplayObservable) StartWithIterable(iterable func() <-chan interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).StartWithIterable(iterable)
}
func (cro *connectableReplayObservable) Timestamp() Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Timestamp()
}
func (cro *connectableReplayObservable) TimeInterval() Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).TimeInterval()
}
func (cro *connectableReplayObservable) Cast(targetType reflect.Type) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Cast(targetType)
}
func (cro *connectableReplayObservable) OfType(targetType reflect.Type) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).OfType(targetType)
}
func (cro *connectableReplayObservable) WithLatestFrom(other Observable, combiner func(interface{}, interface{}) interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).WithLatestFrom(other, combiner)
}
func (cro *connectableReplayObservable) DoFinally(action func()) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).DoFinally(action)
}
func (cro *connectableReplayObservable) DoAfterTerminate(action func()) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).DoAfterTerminate(action)
}
func (cro *connectableReplayObservable) Cache() Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Cache()
}
func (cro *connectableReplayObservable) CacheWithCapacity(capacity int) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).CacheWithCapacity(capacity)
}
func (cro *connectableReplayObservable) Amb(other Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Amb(other)
}

// 高级组合操作符
func (cro *connectableReplayObservable) Join(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, interface{}) interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).Join(other, leftWindowSelector, rightWindowSelector, resultSelector)
}
func (cro *connectableReplayObservable) GroupJoin(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, Observable) interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).GroupJoin(other, leftWindowSelector, rightWindowSelector, resultSelector)
}
func (cro *connectableReplayObservable) SwitchOnNext() Observable {
	return NewObservable(func(observer Observer) Subscription {
		return cro.Subscribe(observer)
	}).SwitchOnNext()
}

// ============================================================================
// 辅助类型定义
// ============================================================================

// Timestamped 带时间戳的值
type Timestamped struct {
	Value     interface{}
	Timestamp time.Time
}

// TimeInterval 带时间间隔的值
type TimeInterval struct {
	Value    interface{}
	Interval time.Duration
}

// CastError 类型转换错误
type CastError struct {
	message string
}

func NewCastError(format string, args ...interface{}) *CastError {
	return &CastError{
		message: fmt.Sprintf(format, args...),
	}
}

func (e *CastError) Error() string {
	return e.message
}

// CompositeSubscription 复合订阅，用于管理多个订阅
type CompositeSubscription struct {
	subscriptions []Subscription
	unsubscribed  int32
	mutex         sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewCompositeSubscription() *CompositeSubscription {
	ctx, cancel := context.WithCancel(context.Background())
	return &CompositeSubscription{
		subscriptions: make([]Subscription, 0),
		ctx:           ctx,
		cancel:        cancel,
	}
}

func (cs *CompositeSubscription) Add(subscription Subscription) {
	if atomic.LoadInt32(&cs.unsubscribed) == 1 {
		subscription.Unsubscribe()
		return
	}

	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	if atomic.LoadInt32(&cs.unsubscribed) == 1 {
		subscription.Unsubscribe()
		return
	}

	cs.subscriptions = append(cs.subscriptions, subscription)
}

func (cs *CompositeSubscription) Unsubscribe() {
	if atomic.CompareAndSwapInt32(&cs.unsubscribed, 0, 1) {
		cs.cancel()

		cs.mutex.RLock()
		subscriptions := cs.subscriptions
		cs.mutex.RUnlock()

		for _, subscription := range subscriptions {
			subscription.Unsubscribe()
		}
	}
}

func (cs *CompositeSubscription) IsUnsubscribed() bool {
	return atomic.LoadInt32(&cs.unsubscribed) == 1
}

func (cs *CompositeSubscription) Context() context.Context {
	return cs.ctx
}

// EmptySubscription 空订阅实现
type EmptySubscription struct{}

func NewEmptySubscription() *EmptySubscription {
	return &EmptySubscription{}
}

func (es *EmptySubscription) Unsubscribe() {}

func (es *EmptySubscription) IsUnsubscribed() bool {
	return true
}

// SimpleSubscription 简单订阅实现
type SimpleSubscription struct {
	unsubscribed int32
	action       func()
}

func NewSubscription(unsubscribeAction func()) *SimpleSubscription {
	return &SimpleSubscription{
		action: unsubscribeAction,
	}
}

func (ss *SimpleSubscription) Unsubscribe() {
	if atomic.CompareAndSwapInt32(&ss.unsubscribed, 0, 1) {
		if ss.action != nil {
			ss.action()
		}
	}
}

func (ss *SimpleSubscription) IsUnsubscribed() bool {
	return atomic.LoadInt32(&ss.unsubscribed) == 1
}

// ============================================================================
// Join操作符 - 对标RxJava的join/groupJoin
// ============================================================================

// JoinWindow 定义连接窗口
type JoinWindow struct {
	Start time.Time
	End   time.Time
}

// WindowSelector 窗口选择函数类型
type WindowSelector func(interface{}) time.Duration

// Join 基于时间窗口连接两个Observable
func (o *observableImpl) Join(
	other Observable,
	leftWindowSelector func(interface{}) time.Duration,
	rightWindowSelector func(interface{}) time.Duration,
	resultSelector func(interface{}, interface{}) interface{},
) Observable {
	return NewObservable(func(observer Observer) Subscription {
		// 存储左右Observable的数据项和其窗口
		type TimedItem struct {
			Value  interface{}
			Window JoinWindow
		}

		var leftItems []TimedItem
		var rightItems []TimedItem
		var mu sync.RWMutex
		var completed int32

		// 检查并发射匹配的组合
		checkAndEmitMatches := func() {
			mu.RLock()
			defer mu.RUnlock()

			now := time.Now()
			for _, left := range leftItems {
				// 清理过期的左侧项目
				if now.After(left.Window.End) {
					continue
				}

				for _, right := range rightItems {
					// 清理过期的右侧项目
					if now.After(right.Window.End) {
						continue
					}

					// 检查时间窗口是否重叠
					if left.Window.Start.Before(right.Window.End) &&
						right.Window.Start.Before(left.Window.End) {
						result := resultSelector(left.Value, right.Value)
						observer(CreateItem(result))
					}
				}
			}
		}

		// 订阅左Observable（源）
		leftSubscription := o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				// 左Observable完成
				if atomic.AddInt32(&completed, 1) == 2 {
					observer(item) // 两个都完成时才发送完成信号
				}
				return
			}

			// 添加到左侧项目列表
			now := time.Now()
			duration := leftWindowSelector(item.Value)
			timedItem := TimedItem{
				Value: item.Value,
				Window: JoinWindow{
					Start: now,
					End:   now.Add(duration),
				},
			}

			mu.Lock()
			leftItems = append(leftItems, timedItem)
			mu.Unlock()

			checkAndEmitMatches()
		})

		// 订阅右Observable
		rightSubscription := other.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				// 右Observable完成
				if atomic.AddInt32(&completed, 1) == 2 {
					observer(item) // 两个都完成时才发送完成信号
				}
				return
			}

			// 添加到右侧项目列表
			now := time.Now()
			duration := rightWindowSelector(item.Value)
			timedItem := TimedItem{
				Value: item.Value,
				Window: JoinWindow{
					Start: now,
					End:   now.Add(duration),
				},
			}

			mu.Lock()
			rightItems = append(rightItems, timedItem)
			mu.Unlock()

			checkAndEmitMatches()
		})

		return NewBaseSubscription(NewBaseDisposable(func() {
			leftSubscription.Unsubscribe()
			rightSubscription.Unsubscribe()
		}))
	})
}

// GroupJoin 基于时间窗口的分组连接操作符
func (o *observableImpl) GroupJoin(
	other Observable,
	leftWindowSelector func(interface{}) time.Duration,
	rightWindowSelector func(interface{}) time.Duration,
	resultSelector func(interface{}, Observable) interface{},
) Observable {
	return NewObservable(func(observer Observer) Subscription {
		type TimedItem struct {
			Value  interface{}
			Window JoinWindow
		}

		var rightItems []TimedItem
		var mu sync.RWMutex

		// 订阅右Observable收集所有右侧数据
		rightSubscription := other.Subscribe(func(item Item) {
			if item.IsError() || item.Value == nil {
				return
			}

			now := time.Now()
			duration := rightWindowSelector(item.Value)
			timedItem := TimedItem{
				Value: item.Value,
				Window: JoinWindow{
					Start: now,
					End:   now.Add(duration),
				},
			}

			mu.Lock()
			rightItems = append(rightItems, timedItem)
			mu.Unlock()
		})

		// 订阅左Observable
		leftSubscription := o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				observer(item)
				return
			}

			// 为每个左侧项目创建一个包含匹配右侧项目的Observable
			now := time.Now()
			duration := leftWindowSelector(item.Value)
			leftWindow := JoinWindow{
				Start: now,
				End:   now.Add(duration),
			}

			// 创建匹配的右侧项目Observable
			matchingRightItems := NewObservable(func(rightObserver Observer) Subscription {
				mu.RLock()
				defer mu.RUnlock()

				for _, right := range rightItems {
					// 检查时间窗口是否重叠
					if leftWindow.Start.Before(right.Window.End) &&
						right.Window.Start.Before(leftWindow.End) {
						rightObserver(CreateItem(right.Value))
					}
				}
				rightObserver(CreateItem(nil)) // 完成信号

				return NewBaseSubscription(NewBaseDisposable(func() {}))
			})

			// 发射结果
			result := resultSelector(item.Value, matchingRightItems)
			observer(CreateItem(result))
		})

		return NewBaseSubscription(NewBaseDisposable(func() {
			leftSubscription.Unsubscribe()
			rightSubscription.Unsubscribe()
		}))
	})
}

// ============================================================================
// SwitchOnNext操作符 - 对标RxJava的switchOnNext
// ============================================================================

// SwitchOnNext 切换到最新的内部Observable
// 这是一个静态方法，处理Observable<Observable<T>>
func SwitchOnNext(source Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		var currentInnerSubscription Subscription
		var mu sync.RWMutex
		var completed int32

		// 订阅外部Observable
		outerSubscription := source.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				// 外部Observable完成
				atomic.StoreInt32(&completed, 1)
				observer(item)
				return
			}

			// 检查项目是否是Observable
			if innerObservable, ok := item.Value.(Observable); ok {
				mu.Lock()
				// 取消之前的内部订阅
				if currentInnerSubscription != nil {
					currentInnerSubscription.Unsubscribe()
				}

				// 订阅新的内部Observable
				currentInnerSubscription = innerObservable.Subscribe(func(innerItem Item) {
					// 只有在外部Observable还没完成时才转发内部数据
					if atomic.LoadInt32(&completed) == 0 {
						observer(innerItem)
					}
				})
				mu.Unlock()
			} else {
				// 如果不是Observable，直接发射
				observer(item)
			}
		})

		return NewBaseSubscription(NewBaseDisposable(func() {
			outerSubscription.Unsubscribe()
			mu.RLock()
			if currentInnerSubscription != nil {
				currentInnerSubscription.Unsubscribe()
			}
			mu.RUnlock()
		}))
	})
}

// SwitchOnNext 作为Observable的实例方法
func (o *observableImpl) SwitchOnNext() Observable {
	return SwitchOnNext(o)
}
