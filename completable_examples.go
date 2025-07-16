// Completable examples for RxGo
// Completable使用示例，展示各种实际应用场景
package rxgo

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// 基础使用示例
// ============================================================================

// ExampleCompletableBasics 演示Completable的基础用法
func ExampleCompletableBasics() {
	fmt.Println("=== Completable基础用法示例 ===")

	// 1. 立即完成的Completable
	fmt.Println("1. 立即完成:")
	CompletableComplete().Subscribe(func() {
		fmt.Println("  ✓ 立即完成")
	}, func(err error) {
		fmt.Printf("  ✗ 错误: %v\n", err)
	})

	// 2. 立即错误的Completable
	fmt.Println("2. 立即错误:")
	CompletableError(fmt.Errorf("示例错误")).Subscribe(func() {
		fmt.Println("  ✓ 完成")
	}, func(err error) {
		fmt.Printf("  ✗ 错误: %v\n", err)
	})

	// 3. 自定义Completable
	fmt.Println("3. 自定义操作:")
	CompletableCreate(func(onComplete OnComplete, onError OnError) {
		go func() {
			// 模拟异步操作
			time.Sleep(100 * time.Millisecond)
			fmt.Println("  正在执行异步操作...")
			onComplete()
		}()
	}).Subscribe(func() {
		fmt.Println("  ✓ 异步操作完成")
	}, func(err error) {
		fmt.Printf("  ✗ 错误: %v\n", err)
	})

	time.Sleep(200 * time.Millisecond)
}

// ============================================================================
// 实际应用场景示例
// ============================================================================

// ExampleDatabaseOperation 演示数据库操作场景
func ExampleDatabaseOperation() {
	fmt.Println("\n=== 数据库操作示例 ===")

	// 模拟数据库连接
	connectDB := CompletableFromAction(func() error {
		fmt.Println("连接数据库...")
		time.Sleep(50 * time.Millisecond)
		return nil
	})

	// 模拟数据迁移
	migrateSchema := CompletableFromAction(func() error {
		fmt.Println("执行数据库迁移...")
		time.Sleep(100 * time.Millisecond)
		return nil
	})

	// 模拟索引创建
	createIndexes := CompletableFromAction(func() error {
		fmt.Println("创建索引...")
		time.Sleep(75 * time.Millisecond)
		return nil
	})

	// 按顺序执行数据库初始化操作
	dbInit := connectDB.
		AndThen(migrateSchema).
		AndThen(createIndexes).
		DoOnComplete(func() {
			fmt.Println("✓ 数据库初始化完成")
		})

	// 执行初始化
	err := dbInit.BlockingAwait()
	if err != nil {
		fmt.Printf("✗ 数据库初始化失败: %v\n", err)
	}
}

// ExampleFileOperations 演示文件操作场景
func ExampleFileOperations() {
	fmt.Println("\n=== 文件操作示例 ===")

	// 模拟多个文件操作
	operations := []Completable{
		CompletableFromAction(func() error {
			fmt.Println("创建临时目录...")
			time.Sleep(30 * time.Millisecond)
			return nil
		}),
		CompletableFromAction(func() error {
			fmt.Println("下载文件1...")
			time.Sleep(80 * time.Millisecond)
			return nil
		}),
		CompletableFromAction(func() error {
			fmt.Println("下载文件2...")
			time.Sleep(60 * time.Millisecond)
			return nil
		}),
		CompletableFromAction(func() error {
			fmt.Println("下载文件3...")
			time.Sleep(90 * time.Millisecond)
			return nil
		}),
	}

	// 并行执行所有文件下载
	start := time.Now()
	CompletableMerge(operations...).Subscribe(func() {
		elapsed := time.Since(start)
		fmt.Printf("✓ 所有文件操作完成，耗时: %v\n", elapsed)
	}, func(err error) {
		fmt.Printf("✗ 文件操作失败: %v\n", err)
	})

	time.Sleep(200 * time.Millisecond)
}

// ============================================================================
// 错误处理示例
// ============================================================================

// ExampleCompletableErrorHandling 演示Completable错误处理场景
func ExampleCompletableErrorHandling() {
	fmt.Println("\n=== 错误处理示例 ===")

	// 模拟可能失败的网络操作
	unstableNetworkCall := func(attempt int) Completable {
		return CompletableFromAction(func() error {
			fmt.Printf("网络调用尝试 #%d...\n", attempt)
			// 模拟前两次失败
			if attempt < 3 {
				return fmt.Errorf("网络错误 #%d", attempt)
			}
			return nil
		})
	}

	// 使用自定义重试逻辑
	attempt := 0
	retryableOperation := CompletableDefer(func() Completable {
		attempt++
		return unstableNetworkCall(attempt)
	})

	// 重试操作
	retryableOperation.Retry(3).Subscribe(func() {
		fmt.Printf("✓ 网络操作成功，共尝试 %d 次\n", attempt)
	}, func(err error) {
		fmt.Printf("✗ 网络操作最终失败: %v\n", err)
	})

	time.Sleep(100 * time.Millisecond)

	// 错误恢复示例
	fmt.Println("\n错误恢复示例:")
	primaryService := CompletableError(fmt.Errorf("主服务不可用"))
	fallbackService := CompletableFromAction(func() error {
		fmt.Println("使用备用服务...")
		return nil
	})

	primaryService.Catch(func(err error) Completable {
		fmt.Printf("主服务失败: %v，切换到备用服务\n", err)
		return fallbackService
	}).Subscribe(func() {
		fmt.Println("✓ 服务调用成功")
	}, func(err error) {
		fmt.Printf("✗ 所有服务都失败: %v\n", err)
	})

	time.Sleep(100 * time.Millisecond)
}

// ============================================================================
// 高级用法示例
// ============================================================================

// ExampleAdvancedUsage 演示高级用法
func ExampleAdvancedUsage() {
	fmt.Println("\n=== 高级用法示例 ===")

	// 1. 超时处理
	fmt.Println("1. 超时处理:")
	slowOperation := CompletableTimer(200 * time.Millisecond)
	slowOperation.Timeout(100*time.Millisecond).Subscribe(func() {
		fmt.Println("  ✓ 操作及时完成")
	}, func(err error) {
		fmt.Printf("  ✗ 操作超时: %v\n", err)
	})

	time.Sleep(150 * time.Millisecond)

	// 2. 副作用操作
	fmt.Println("2. 副作用操作:")
	CompletableFromAction(func() error {
		fmt.Println("  执行主要操作...")
		return nil
	}).
		DoOnSubscribe(func() {
			fmt.Println("  开始执行...")
		}).
		DoOnComplete(func() {
			fmt.Println("  清理资源...")
		}).
		Subscribe(func() {
			fmt.Println("  ✓ 操作完成")
		}, func(err error) {
			fmt.Printf("  ✗ 操作失败: %v\n", err)
		})

	time.Sleep(100 * time.Millisecond)

	// 3. 转换为其他类型
	fmt.Println("3. 类型转换:")
	completable := CompletableFromAction(func() error {
		fmt.Println("  执行Completable操作...")
		return nil
	})

	// 转换为Single
	single := completable.ToSingle("操作结果")
	single.Subscribe(func(value interface{}) {
		fmt.Printf("  ✓ Single结果: %v\n", value)
	}, func(err error) {
		fmt.Printf("  ✗ Single错误: %v\n", err)
	})

	time.Sleep(100 * time.Millisecond)
}

// ============================================================================
// 资源管理示例
// ============================================================================

// ExampleResourceManagement 演示资源管理场景
func ExampleResourceManagement() {
	fmt.Println("\n=== 资源管理示例 ===")

	// 模拟资源类型
	type Resource struct {
		name string
	}

	// 使用CompletableUsing管理资源生命周期
	CompletableUsing(
		// 资源创建
		func() interface{} {
			resource := &Resource{name: "数据库连接"}
			fmt.Printf("创建资源: %s\n", resource.name)
			return resource
		},
		// 使用资源的Completable
		func(resource interface{}) Completable {
			res := resource.(*Resource)
			return CompletableFromAction(func() error {
				fmt.Printf("使用资源: %s\n", res.name)
				time.Sleep(50 * time.Millisecond)
				return nil
			})
		},
		// 资源清理
		func(resource interface{}) {
			res := resource.(*Resource)
			fmt.Printf("释放资源: %s\n", res.name)
		},
	).Subscribe(func() {
		fmt.Println("✓ 资源操作完成")
	}, func(err error) {
		fmt.Printf("✗ 资源操作失败: %v\n", err)
	})

	time.Sleep(100 * time.Millisecond)
}

// ============================================================================
// 并发控制示例
// ============================================================================

// ExampleConcurrencyControl 演示并发控制
func ExampleConcurrencyControl() {
	fmt.Println("\n=== 并发控制示例 ===")

	var counter int32
	var mu sync.Mutex

	// 创建多个并发任务
	tasks := make([]Completable, 5)
	for i := 0; i < 5; i++ {
		taskID := i + 1
		tasks[i] = CompletableFromAction(func() error {
			mu.Lock()
			counter++
			current := counter
			mu.Unlock()

			fmt.Printf("任务 %d 开始执行 (总计: %d)\n", taskID, current)
			time.Sleep(time.Duration(50+taskID*10) * time.Millisecond)
			fmt.Printf("任务 %d 完成\n", taskID)
			return nil
		})
	}

	// 并行执行所有任务
	start := time.Now()
	CompletableMerge(tasks...).Subscribe(func() {
		elapsed := time.Since(start)
		fmt.Printf("✓ 所有并发任务完成，耗时: %v\n", elapsed)
	}, func(err error) {
		fmt.Printf("✗ 并发任务失败: %v\n", err)
	})

	time.Sleep(300 * time.Millisecond)
}

// ============================================================================
// 条件执行示例
// ============================================================================

// ExampleConditionalExecution 演示条件执行
func ExampleConditionalExecution() {
	fmt.Println("\n=== 条件执行示例 ===")

	condition := true

	conditionalTask := CompletableDefer(func() Completable {
		if condition {
			return CompletableFromAction(func() error {
				fmt.Println("条件为真，执行主要任务...")
				return nil
			})
		} else {
			return CompletableFromAction(func() error {
				fmt.Println("条件为假，执行替代任务...")
				return nil
			})
		}
	})

	conditionalTask.Subscribe(func() {
		fmt.Println("✓ 条件任务完成")
	}, func(err error) {
		fmt.Printf("✗ 条件任务失败: %v\n", err)
	})

	time.Sleep(100 * time.Millisecond)
}

// ============================================================================
// 主示例入口
// ============================================================================

// RunAllCompletableExamples 运行所有Completable示例
func RunAllCompletableExamples() {
	fmt.Println("🚀 RxGo Completable 使用示例")
	fmt.Println("=====================================")

	ExampleCompletableBasics()
	ExampleDatabaseOperation()
	ExampleFileOperations()
	ExampleCompletableErrorHandling()
	ExampleAdvancedUsage()
	ExampleResourceManagement()
	ExampleConcurrencyControl()
	ExampleConditionalExecution()

	fmt.Println("\n🎉 所有示例执行完成！")
}

// ============================================================================
// 实用工具函数示例
// ============================================================================

// WaitForMultipleCompletables 等待多个Completable完成的工具函数
func WaitForMultipleCompletables(completables []Completable, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error, 1)

	go func() {
		merged := CompletableMerge(completables...)
		err := merged.BlockingAwait()
		done <- err
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("等待超时: %v", timeout)
	}
}

// ChainCompletables 链式执行多个Completable的工具函数
func ChainCompletables(completables []Completable) Completable {
	if len(completables) == 0 {
		return CompletableComplete()
	}

	result := completables[0]
	for i := 1; i < len(completables); i++ {
		result = result.AndThen(completables[i])
	}

	return result
}

// ParallelCompletables 并行执行Completable的工具函数
func ParallelCompletables(completables []Completable, maxConcurrency int) Completable {
	if len(completables) == 0 {
		return CompletableComplete()
	}

	if maxConcurrency <= 0 || maxConcurrency >= len(completables) {
		return CompletableMerge(completables...)
	}

	return NewCompletable(func(onComplete OnComplete, onError OnError) Subscription {
		semaphore := make(chan struct{}, maxConcurrency)
		var wg sync.WaitGroup
		hasError := false
		var mu sync.Mutex

		wg.Add(len(completables))

		for _, completable := range completables {
			go func(c Completable) {
				defer wg.Done()

				// 获取信号量
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				c.Subscribe(func() {
					// 完成，不需要特殊处理
				}, func(err error) {
					mu.Lock()
					if !hasError {
						hasError = true
						if onError != nil {
							onError(err)
						}
					}
					mu.Unlock()
				})
			}(completable)
		}

		go func() {
			wg.Wait()
			mu.Lock()
			if !hasError && onComplete != nil {
				onComplete()
			}
			mu.Unlock()
		}()

		return NewBaseSubscription(NewBaseDisposable(func() {}))
	})
}
