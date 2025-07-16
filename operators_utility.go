// Utility operators for RxGo
// 工具操作符实现，包含ToSlice, ToChannel等
package rxgo

import (
	"sync"
)

// ============================================================================
// 工具操作符实现
// ============================================================================

// ToSlice 将Observable转换为Single<[]interface{}>
func (o *observableImpl) ToSlice() Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		var result []interface{}
		mu := &sync.Mutex{}

		return o.Subscribe(func(item Item) {
			if item.IsError() {
				if onError != nil {
					onError(item.Error)
				}
				return
			}

			if item.Value == nil {
				// 完成信号
				onSuccess(result)
				return
			}

			// 添加值到结果切片
			mu.Lock()
			result = append(result, item.Value)
			mu.Unlock()
		})
	})
}

// 注意：ToChannel方法已在observable.go中实现

// ToMap 将Observable转换为Single<map[K]V>，使用键选择器
func (o *observableImpl) ToMap(keySelector func(interface{}) interface{}) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		result := make(map[interface{}]interface{})
		mu := &sync.Mutex{}

		return o.Subscribe(func(item Item) {
			if item.IsError() {
				if onError != nil {
					onError(item.Error)
				}
				return
			}

			if item.Value == nil {
				// 完成信号
				onSuccess(result)
				return
			}

			// 添加键值对到结果map
			key := keySelector(item.Value)
			mu.Lock()
			result[key] = item.Value
			mu.Unlock()
		})
	})
}

// ToMapWithValueSelector 将Observable转换为Single<map[K]V>，使用键选择器和值选择器
func (o *observableImpl) ToMapWithValueSelector(keySelector func(interface{}) interface{}, valueSelector func(interface{}) interface{}) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		result := make(map[interface{}]interface{})
		mu := &sync.Mutex{}

		return o.Subscribe(func(item Item) {
			if item.IsError() {
				if onError != nil {
					onError(item.Error)
				}
				return
			}

			if item.Value == nil {
				// 完成信号
				onSuccess(result)
				return
			}

			// 添加键值对到结果map
			key := keySelector(item.Value)
			value := valueSelector(item.Value)
			mu.Lock()
			result[key] = value
			mu.Unlock()
		})
	})
}

// Materialize 将Observable转换为发射Notification的Observable
func (o *observableImpl) Materialize() Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				// 发射错误通知
				notification := ErrorNotification{Error: item.Error}
				observer(CreateItem(notification))
				// 然后发射完成通知
				completionNotification := CompletionNotification{}
				observer(CreateItem(completionNotification))
				observer(CreateItem(nil)) // 完成信号
				return
			}

			if item.Value == nil {
				// 发射完成通知
				notification := CompletionNotification{}
				observer(CreateItem(notification))
				observer(CreateItem(nil)) // 完成信号
				return
			}

			// 发射值通知
			notification := ValueNotification{Value: item.Value}
			observer(CreateItem(notification))
		})
	})
}

// Dematerialize 将发射Notification的Observable转换为普通Observable
func (o *observableImpl) Dematerialize() Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				observer(item)
				return
			}

			// 根据通知类型处理
			switch notification := item.Value.(type) {
			case ValueNotification:
				observer(CreateItem(notification.Value))
			case ErrorNotification:
				observer(CreateErrorItem(notification.Error))
			case CompletionNotification:
				observer(CreateItem(nil)) // 完成信号
			default:
				// 不是通知类型，直接传递
				observer(item)
			}
		})
	})
}

// DefaultIfEmpty 如果Observable为空，则发射默认值
func (o *observableImpl) DefaultIfEmpty(defaultValue interface{}) Observable {
	return NewObservable(func(observer Observer) Subscription {
		hasValue := false

		return o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				// 完成信号
				if !hasValue {
					observer(CreateItem(defaultValue))
				}
				observer(item)
				return
			}

			// 正常值
			hasValue = true
			observer(item)
		})
	})
}

// SwitchIfEmpty 如果Observable为空，则切换到另一个Observable
func (o *observableImpl) SwitchIfEmpty(other Observable) Observable {
	return NewObservable(func(observer Observer) Subscription {
		hasValue := false

		subscription1 := o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				// 完成信号
				if !hasValue {
					// 切换到另一个Observable
					other.Subscribe(observer)
				} else {
					observer(item)
				}
				return
			}

			// 正常值
			hasValue = true
			observer(item)
		})

		return subscription1
	})
}

// IgnoreElements 忽略所有值，只传递错误和完成信号
func (o *observableImpl) IgnoreElements() Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() || item.Value == nil {
				// 只传递错误和完成信号
				observer(item)
				return
			}

			// 忽略正常值
		})
	})
}

// SequenceEqual 比较两个Observable的序列是否相等
func (o *observableImpl) SequenceEqual(other Observable) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {

		var queue1 []interface{}
		var queue2 []interface{}
		mu := &sync.Mutex{}
		completed1 := false
		completed2 := false

		checkEquality := func() {
			mu.Lock()
			defer mu.Unlock()

			// 比较当前队列
			if len(queue1) != len(queue2) {
				if completed1 && completed2 {
					onSuccess(false)
				}
				return
			}

			for i := 0; i < len(queue1); i++ {
				if queue1[i] != queue2[i] {
					onSuccess(false)
					return
				}
			}

			// 如果都完成了且队列相等
			if completed1 && completed2 {
				onSuccess(true)
			}
		}

		subscription1 := o.Subscribe(func(item Item) {
			if item.IsError() {
				if onError != nil {
					onError(item.Error)
				}
				return
			}

			if item.Value == nil {
				mu.Lock()
				completed1 = true
				mu.Unlock()
				checkEquality()
				return
			}

			mu.Lock()
			queue1 = append(queue1, item.Value)
			mu.Unlock()
			checkEquality()
		})

		subscription2 := other.Subscribe(func(item Item) {
			if item.IsError() {
				if onError != nil {
					onError(item.Error)
				}
				return
			}

			if item.Value == nil {
				mu.Lock()
				completed2 = true
				mu.Unlock()
				checkEquality()
				return
			}

			mu.Lock()
			queue2 = append(queue2, item.Value)
			mu.Unlock()
			checkEquality()
		})

		return NewBaseSubscription(NewBaseDisposable(func() {
			subscription1.Unsubscribe()
			subscription2.Unsubscribe()
		}))
	})
}

// ============================================================================
// 通知类型定义
// ============================================================================

// ValueNotification 值通知
type ValueNotification struct {
	Value interface{}
}

// ErrorNotification 错误通知
type ErrorNotification struct {
	Error error
}

// CompletionNotification 完成通知
type CompletionNotification struct{}

// ============================================================================
// 其他工具函数
// ============================================================================

// 注意：CreateItem和CreateErrorItem函数已在其他文件中定义
