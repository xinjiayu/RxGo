// Aggregation operators for RxGo
// 聚合操作符实现，包含Count, First, Last, Min, Max, Sum, Average等
package rxgo

import (
	"errors"
	"reflect"
	"sync/atomic"
)

// ============================================================================
// 聚合操作符实现
// ============================================================================

// Count 计算Observable发射的项目数量
func (o *observableImpl) Count() Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		count := int32(0)
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				if onError != nil {
					onError(item.Error)
				}
				return
			}

			if item.Value == nil {
				// 完成信号
				onSuccess(int(atomic.LoadInt32(&count)))
				return
			}

			// 计数
			atomic.AddInt32(&count, 1)
		})
	})
}

// First 获取Observable发射的第一个项目
func (o *observableImpl) First() Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		hasValue := int32(0)
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				if onError != nil {
					onError(item.Error)
				}
				return
			}

			if item.Value == nil {
				// 完成信号
				if atomic.LoadInt32(&hasValue) == 0 {
					if onError != nil {
						onError(NewNoSuchElementError("Observable is empty"))
					}
				}
				return
			}

			// 第一个值
			if atomic.CompareAndSwapInt32(&hasValue, 0, 1) {
				onSuccess(item.Value)
			}
		})
	})
}

// Last 获取Observable发射的最后一个项目
func (o *observableImpl) Last() Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		var lastValue interface{}
		hasValue := int32(0)
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				if onError != nil {
					onError(item.Error)
				}
				return
			}

			if item.Value == nil {
				// 完成信号
				if atomic.LoadInt32(&hasValue) == 0 {
					if onError != nil {
						onError(NewNoSuchElementError("Observable is empty"))
					}
				} else {
					onSuccess(lastValue)
				}
				return
			}

			// 更新最后一个值
			lastValue = item.Value
			atomic.StoreInt32(&hasValue, 1)
		})
	})
}

// Min 获取Observable发射的最小值
func (o *observableImpl) Min() Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		var minValue interface{}
		hasValue := int32(0)
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				if onError != nil {
					onError(item.Error)
				}
				return
			}

			if item.Value == nil {
				// 完成信号
				if atomic.LoadInt32(&hasValue) == 0 {
					if onError != nil {
						onError(NewNoSuchElementError("Observable is empty"))
					}
				} else {
					onSuccess(minValue)
				}
				return
			}

			// 比较并更新最小值
			if atomic.CompareAndSwapInt32(&hasValue, 0, 1) {
				minValue = item.Value
			} else {
				if compareValues(item.Value, minValue) < 0 {
					minValue = item.Value
				}
			}
		})
	})
}

// Max 获取Observable发射的最大值
func (o *observableImpl) Max() Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		var maxValue interface{}
		hasValue := int32(0)
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				if onError != nil {
					onError(item.Error)
				}
				return
			}

			if item.Value == nil {
				// 完成信号
				if atomic.LoadInt32(&hasValue) == 0 {
					if onError != nil {
						onError(NewNoSuchElementError("Observable is empty"))
					}
				} else {
					onSuccess(maxValue)
				}
				return
			}

			// 比较并更新最大值
			if atomic.CompareAndSwapInt32(&hasValue, 0, 1) {
				maxValue = item.Value
			} else {
				if compareValues(item.Value, maxValue) > 0 {
					maxValue = item.Value
				}
			}
		})
	})
}

// Sum 计算Observable发射的所有数值的和
func (o *observableImpl) Sum() Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		var sum interface{}
		hasValue := int32(0)
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				if onError != nil {
					onError(item.Error)
				}
				return
			}

			if item.Value == nil {
				// 完成信号
				if atomic.LoadInt32(&hasValue) == 0 {
					onSuccess(0) // 空序列的和为0
				} else {
					onSuccess(sum)
				}
				return
			}

			// 累加
			if atomic.CompareAndSwapInt32(&hasValue, 0, 1) {
				sum = item.Value
			} else {
				var err error
				sum, err = addValues(sum, item.Value)
				if err != nil {
					if onError != nil {
						onError(err)
					}
					return
				}
			}
		})
	})
}

// Average 计算Observable发射的所有数值的平均值
func (o *observableImpl) Average() Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		var sum interface{}
		count := int32(0)
		hasValue := int32(0)
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				if onError != nil {
					onError(item.Error)
				}
				return
			}

			if item.Value == nil {
				// 完成信号
				if atomic.LoadInt32(&hasValue) == 0 {
					if onError != nil {
						onError(NewNoSuchElementError("Observable is empty"))
					}
				} else {
					avg, err := divideValues(sum, int(atomic.LoadInt32(&count)))
					if err != nil {
						if onError != nil {
							onError(err)
						}
					} else {
						onSuccess(avg)
					}
				}
				return
			}

			// 累加并计数
			if atomic.CompareAndSwapInt32(&hasValue, 0, 1) {
				sum = item.Value
			} else {
				var err error
				sum, err = addValues(sum, item.Value)
				if err != nil {
					if onError != nil {
						onError(err)
					}
					return
				}
			}
			atomic.AddInt32(&count, 1)
		})
	})
}

// All 检查Observable的所有项是否都满足谓词
func (o *observableImpl) All(predicate Predicate) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				if onError != nil {
					onError(item.Error)
				}
				return
			}

			if item.Value == nil {
				// 完成信号 - 所有项都满足条件
				onSuccess(true)
				return
			}

			// 检查条件
			if !predicate(item.Value) {
				onSuccess(false)
				return
			}
		})
	})
}

// Any 检查Observable的任何项是否满足谓词
func (o *observableImpl) Any(predicate Predicate) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				if onError != nil {
					onError(item.Error)
				}
				return
			}

			if item.Value == nil {
				// 完成信号 - 没有项满足条件
				onSuccess(false)
				return
			}

			// 检查条件
			if predicate(item.Value) {
				onSuccess(true)
				return
			}
		})
	})
}

// Contains 检查Observable是否包含指定的值
func (o *observableImpl) Contains(value interface{}) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				if onError != nil {
					onError(item.Error)
				}
				return
			}

			if item.Value == nil {
				// 完成信号 - 没有找到值
				onSuccess(false)
				return
			}

			// 检查是否相等
			if reflect.DeepEqual(item.Value, value) {
				onSuccess(true)
				return
			}
		})
	})
}

// ElementAt 获取Observable指定索引处的项
func (o *observableImpl) ElementAt(index int) Single {
	return NewSingle(func(onSuccess func(interface{}), onError OnError) Subscription {
		currentIndex := int32(0)
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				if onError != nil {
					onError(item.Error)
				}
				return
			}

			if item.Value == nil {
				// 完成信号 - 索引超出范围
				if onError != nil {
					onError(NewIndexOutOfBoundsError("Index out of bounds"))
				}
				return
			}

			// 检查索引
			if int(atomic.LoadInt32(&currentIndex)) == index {
				onSuccess(item.Value)
				return
			}
			atomic.AddInt32(&currentIndex, 1)
		})
	})
}

// ============================================================================
// 辅助函数
// ============================================================================

// compareValues 比较两个值的大小
func compareValues(a, b interface{}) int {
	switch va := a.(type) {
	case int:
		if vb, ok := b.(int); ok {
			if va < vb {
				return -1
			} else if va > vb {
				return 1
			}
			return 0
		}
	case int32:
		if vb, ok := b.(int32); ok {
			if va < vb {
				return -1
			} else if va > vb {
				return 1
			}
			return 0
		}
	case int64:
		if vb, ok := b.(int64); ok {
			if va < vb {
				return -1
			} else if va > vb {
				return 1
			}
			return 0
		}
	case float32:
		if vb, ok := b.(float32); ok {
			if va < vb {
				return -1
			} else if va > vb {
				return 1
			}
			return 0
		}
	case float64:
		if vb, ok := b.(float64); ok {
			if va < vb {
				return -1
			} else if va > vb {
				return 1
			}
			return 0
		}
	case string:
		if vb, ok := b.(string); ok {
			if va < vb {
				return -1
			} else if va > vb {
				return 1
			}
			return 0
		}
	}
	return 0
}

// addValues 相加两个值
func addValues(a, b interface{}) (interface{}, error) {
	switch va := a.(type) {
	case int:
		if vb, ok := b.(int); ok {
			return va + vb, nil
		}
	case int32:
		if vb, ok := b.(int32); ok {
			return va + vb, nil
		}
	case int64:
		if vb, ok := b.(int64); ok {
			return va + vb, nil
		}
	case float32:
		if vb, ok := b.(float32); ok {
			return va + vb, nil
		}
	case float64:
		if vb, ok := b.(float64); ok {
			return va + vb, nil
		}
	}
	return nil, errors.New("unsupported type for addition")
}

// divideValues 除法运算
func divideValues(a interface{}, count int) (interface{}, error) {
	switch va := a.(type) {
	case int:
		return float64(va) / float64(count), nil
	case int32:
		return float64(va) / float64(count), nil
	case int64:
		return float64(va) / float64(count), nil
	case float32:
		return va / float32(count), nil
	case float64:
		return va / float64(count), nil
	}
	return nil, errors.New("unsupported type for division")
}

// IndexOutOfBoundsError 索引越界错误
type IndexOutOfBoundsError struct {
	message string
}

func (e *IndexOutOfBoundsError) Error() string {
	return e.message
}

// NewIndexOutOfBoundsError 创建索引越界错误
func NewIndexOutOfBoundsError(message string) *IndexOutOfBoundsError {
	return &IndexOutOfBoundsError{message: message}
}
