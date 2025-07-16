// Blocking operators for RxGo
// 阻塞操作符实现，包含BlockingSubscribe, BlockingFirst, BlockingLast等
package rxgo

import (
	"context"
	"sync"
	"time"
)

// ============================================================================
// 阻塞操作符实现
// ============================================================================

// BlockingSubscribe 阻塞订阅，等待Observable完成
func (o *observableImpl) BlockingSubscribe(observer Observer) {
	done := make(chan struct{})

	subscription := o.Subscribe(func(item Item) {
		observer(item)

		// 如果是错误或完成信号，结束阻塞
		if item.IsError() || item.Value == nil {
			close(done)
		}
	})

	defer subscription.Unsubscribe()

	// 等待完成
	<-done
}

// BlockingFirst 阻塞获取第一个值
func (o *observableImpl) BlockingFirst() (interface{}, error) {
	return o.BlockingFirstWithTimeout(0)
}

// BlockingFirstWithTimeout 阻塞获取第一个值，带超时
func (o *observableImpl) BlockingFirstWithTimeout(timeout time.Duration) (interface{}, error) {
	resultCh := make(chan interface{}, 1)
	errorCh := make(chan error, 1)

	subscription := o.Subscribe(func(item Item) {
		if item.IsError() {
			errorCh <- item.Error
			return
		}

		if item.Value == nil {
			// 完成信号但没有值
			errorCh <- NewNoSuchElementError("Observable is empty")
			return
		}

		// 第一个值
		resultCh <- item.Value
	})

	defer subscription.Unsubscribe()

	if timeout > 0 {
		select {
		case result := <-resultCh:
			return result, nil
		case err := <-errorCh:
			return nil, err
		case <-time.After(timeout):
			return nil, NewTimeoutError("BlockingFirst timed out")
		}
	} else {
		select {
		case result := <-resultCh:
			return result, nil
		case err := <-errorCh:
			return nil, err
		}
	}
}

// BlockingLast 阻塞获取最后一个值
func (o *observableImpl) BlockingLast() (interface{}, error) {
	return o.BlockingLastWithTimeout(0)
}

// BlockingLastWithTimeout 阻塞获取最后一个值，带超时
func (o *observableImpl) BlockingLastWithTimeout(timeout time.Duration) (interface{}, error) {
	var lastValue interface{}
	hasValue := false
	done := make(chan struct{})
	errorCh := make(chan error, 1)

	subscription := o.Subscribe(func(item Item) {
		if item.IsError() {
			errorCh <- item.Error
			close(done)
			return
		}

		if item.Value == nil {
			// 完成信号
			close(done)
			return
		}

		// 更新最后一个值
		lastValue = item.Value
		hasValue = true
	})

	defer subscription.Unsubscribe()

	if timeout > 0 {
		select {
		case <-done:
			if hasValue {
				return lastValue, nil
			}
			return nil, NewNoSuchElementError("Observable is empty")
		case err := <-errorCh:
			return nil, err
		case <-time.After(timeout):
			return nil, NewTimeoutError("BlockingLast timed out")
		}
	} else {
		select {
		case <-done:
			if hasValue {
				return lastValue, nil
			}
			return nil, NewNoSuchElementError("Observable is empty")
		case err := <-errorCh:
			return nil, err
		}
	}
}

// BlockingToSlice 阻塞收集所有值到切片
func (o *observableImpl) BlockingToSlice() ([]interface{}, error) {
	return o.BlockingToSliceWithTimeout(0)
}

// BlockingToSliceWithTimeout 阻塞收集所有值到切片，带超时
func (o *observableImpl) BlockingToSliceWithTimeout(timeout time.Duration) ([]interface{}, error) {
	var result []interface{}
	done := make(chan struct{})
	errorCh := make(chan error, 1)
	mu := &sync.Mutex{}

	subscription := o.Subscribe(func(item Item) {
		if item.IsError() {
			errorCh <- item.Error
			close(done)
			return
		}

		if item.Value == nil {
			// 完成信号
			close(done)
			return
		}

		// 添加值到结果切片
		mu.Lock()
		result = append(result, item.Value)
		mu.Unlock()
	})

	defer subscription.Unsubscribe()

	if timeout > 0 {
		select {
		case <-done:
			return result, nil
		case err := <-errorCh:
			return nil, err
		case <-time.After(timeout):
			return nil, NewTimeoutError("BlockingToSlice timed out")
		}
	} else {
		select {
		case <-done:
			return result, nil
		case err := <-errorCh:
			return nil, err
		}
	}
}

// BlockingForEach 阻塞遍历所有值
func (o *observableImpl) BlockingForEach(action func(interface{})) error {
	return o.BlockingForEachWithTimeout(action, 0)
}

// BlockingForEachWithTimeout 阻塞遍历所有值，带超时
func (o *observableImpl) BlockingForEachWithTimeout(action func(interface{}), timeout time.Duration) error {
	done := make(chan struct{})
	errorCh := make(chan error, 1)

	subscription := o.Subscribe(func(item Item) {
		if item.IsError() {
			errorCh <- item.Error
			close(done)
			return
		}

		if item.Value == nil {
			// 完成信号
			close(done)
			return
		}

		// 执行操作
		if action != nil {
			action(item.Value)
		}
	})

	defer subscription.Unsubscribe()

	if timeout > 0 {
		select {
		case <-done:
			return nil
		case err := <-errorCh:
			return err
		case <-time.After(timeout):
			return NewTimeoutError("BlockingForEach timed out")
		}
	} else {
		select {
		case <-done:
			return nil
		case err := <-errorCh:
			return err
		}
	}
}

// BlockingSubscribeWithCallbacks 阻塞订阅，使用回调函数
func (o *observableImpl) BlockingSubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) {
	done := make(chan struct{})

	subscription := o.SubscribeWithCallbacks(
		func(value interface{}) {
			if onNext != nil {
				onNext(value)
			}
		},
		func(err error) {
			if onError != nil {
				onError(err)
			}
			close(done)
		},
		func() {
			if onComplete != nil {
				onComplete()
			}
			close(done)
		},
	)

	defer subscription.Unsubscribe()

	// 等待完成
	<-done
}

// BlockingWait 阻塞等待Observable完成
func (o *observableImpl) BlockingWait() error {
	return o.BlockingWaitWithTimeout(0)
}

// BlockingWaitWithTimeout 阻塞等待Observable完成，带超时
func (o *observableImpl) BlockingWaitWithTimeout(timeout time.Duration) error {
	done := make(chan struct{})
	errorCh := make(chan error, 1)

	subscription := o.Subscribe(func(item Item) {
		if item.IsError() {
			errorCh <- item.Error
			close(done)
			return
		}

		if item.Value == nil {
			// 完成信号
			close(done)
			return
		}

		// 忽略正常值，只等待完成
	})

	defer subscription.Unsubscribe()

	if timeout > 0 {
		select {
		case <-done:
			return nil
		case err := <-errorCh:
			return err
		case <-time.After(timeout):
			return NewTimeoutError("BlockingWait timed out")
		}
	} else {
		select {
		case <-done:
			return nil
		case err := <-errorCh:
			return err
		}
	}
}

// BlockingWithContext 带上下文的阻塞操作基础结构
func (o *observableImpl) BlockingWithContext(ctx context.Context, action func(Observer) error) error {
	done := make(chan struct{})
	errorCh := make(chan error, 1)

	subscription := o.Subscribe(func(item Item) {
		if err := action(func(i Item) { /* 这里可以自定义处理 */ }); err != nil {
			errorCh <- err
			close(done)
			return
		}

		if item.IsError() {
			errorCh <- item.Error
			close(done)
			return
		}

		if item.Value == nil {
			// 完成信号
			close(done)
			return
		}
	})

	defer subscription.Unsubscribe()

	select {
	case <-done:
		return nil
	case err := <-errorCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
