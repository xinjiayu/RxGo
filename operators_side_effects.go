// Side effect operators for RxGo
// 副作用操作符实现，包含DoOnNext, DoOnError, DoOnComplete等
package rxgo

// ============================================================================
// 副作用操作符实现
// ============================================================================

// DoOnNext 在每个值发射时执行副作用操作
func (o *observableImpl) DoOnNext(action OnNext) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				// 完成信号
				observer(item)
				return
			}

			// 执行副作用操作
			if action != nil {
				action(item.Value)
			}

			// 继续传递项目
			observer(item)
		})
	})
}

// DoOnError 在发生错误时执行副作用操作
func (o *observableImpl) DoOnError(action OnError) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				// 执行副作用操作
				if action != nil {
					action(item.Error)
				}
				observer(item)
				return
			}

			// 正常值或完成信号
			observer(item)
		})
	})
}

// DoOnComplete 在完成时执行副作用操作
func (o *observableImpl) DoOnComplete(action OnComplete) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				observer(item)
				return
			}

			if item.Value == nil {
				// 完成信号
				if action != nil {
					action()
				}
				observer(item)
				return
			}

			// 正常值
			observer(item)
		})
	})
}

// DoOnSubscribe 在订阅时执行副作用操作
func (o *observableImpl) DoOnSubscribe(action func()) Observable {
	return NewObservable(func(observer Observer) Subscription {
		// 执行订阅副作用操作
		if action != nil {
			action()
		}

		// 继续订阅原Observable
		return o.Subscribe(observer)
	})
}

// DoOnUnsubscribe 在取消订阅时执行副作用操作
func (o *observableImpl) DoOnUnsubscribe(action func()) Observable {
	return NewObservable(func(observer Observer) Subscription {
		subscription := o.Subscribe(observer)

		// 包装取消订阅操作
		return NewBaseSubscription(NewBaseDisposable(func() {
			subscription.Unsubscribe()

			// 执行取消订阅副作用操作
			if action != nil {
				action()
			}
		}))
	})
}

// DoOnTerminate 在终止时（完成或错误）执行副作用操作
func (o *observableImpl) DoOnTerminate(action func()) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() || item.Value == nil {
				// 终止信号（错误或完成）
				if action != nil {
					action()
				}
				observer(item)
				return
			}

			// 正常值
			observer(item)
		})
	})
}

// Tap 通用的副作用操作符，可以指定多个回调
func (o *observableImpl) Tap(onNext OnNext, onError OnError, onComplete OnComplete) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				// 错误
				if onError != nil {
					onError(item.Error)
				}
				observer(item)
				return
			}

			if item.Value == nil {
				// 完成信号
				if onComplete != nil {
					onComplete()
				}
				observer(item)
				return
			}

			// 正常值
			if onNext != nil {
				onNext(item.Value)
			}
			observer(item)
		})
	})
}

// DoOnEach 对每个项目（包括错误和完成）执行副作用操作
func (o *observableImpl) DoOnEach(action func(Item)) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			// 执行副作用操作
			if action != nil {
				action(item)
			}

			// 继续传递项目
			observer(item)
		})
	})
}

// Log 日志操作符，记录所有事件
func (o *observableImpl) Log(prefix string) Observable {
	return NewObservable(func(observer Observer) Subscription {
		return o.Subscribe(func(item Item) {
			if item.IsError() {
				// 记录错误
				println(prefix + " Error: " + item.Error.Error())
			} else if item.Value == nil {
				// 记录完成
				println(prefix + " Complete")
			} else {
				// 记录值
				println(prefix+" Next: ", item.Value)
			}

			// 继续传递项目
			observer(item)
		})
	})
}
