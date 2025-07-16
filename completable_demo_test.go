// Completable demo test for RxGo
// Completable示例演示测试
package rxgo

import (
	"fmt"
	"testing"
	"time"
)

// TestCompletableDemo 演示Completable的基础使用
func TestCompletableDemo(t *testing.T) {
	fmt.Println("🚀 Completable Demo 开始")

	// 1. 基础用法演示
	t.Run("基础用法", func(t *testing.T) {
		fmt.Println("\n=== 基础用法演示 ===")

		// 立即完成
		CompletableComplete().Subscribe(func() {
			fmt.Println("✓ 立即完成")
		}, func(err error) {
			t.Errorf("不应该发生错误: %v", err)
		})

		// 延迟完成
		start := time.Now()
		CompletableTimer(50*time.Millisecond).Subscribe(func() {
			elapsed := time.Since(start)
			fmt.Printf("✓ 延迟完成，耗时: %v\n", elapsed)
		}, func(err error) {
			t.Errorf("不应该发生错误: %v", err)
		})

		time.Sleep(100 * time.Millisecond)
	})

	// 2. 链式操作演示
	t.Run("链式操作", func(t *testing.T) {
		fmt.Println("\n=== 链式操作演示 ===")

		step1 := CompletableFromAction(func() error {
			fmt.Println("  步骤1: 准备工作...")
			time.Sleep(30 * time.Millisecond)
			return nil
		})

		step2 := CompletableFromAction(func() error {
			fmt.Println("  步骤2: 主要处理...")
			time.Sleep(50 * time.Millisecond)
			return nil
		})

		step3 := CompletableFromAction(func() error {
			fmt.Println("  步骤3: 清理工作...")
			time.Sleep(20 * time.Millisecond)
			return nil
		})

		err := step1.AndThen(step2).AndThen(step3).BlockingAwait()
		if err != nil {
			t.Errorf("链式操作失败: %v", err)
		}

		fmt.Println("✓ 所有步骤完成")
	})

	// 3. 并行操作演示
	t.Run("并行操作", func(t *testing.T) {
		fmt.Println("\n=== 并行操作演示 ===")

		tasks := []Completable{
			CompletableFromAction(func() error {
				fmt.Println("  任务A开始...")
				time.Sleep(40 * time.Millisecond)
				fmt.Println("  任务A完成")
				return nil
			}),
			CompletableFromAction(func() error {
				fmt.Println("  任务B开始...")
				time.Sleep(30 * time.Millisecond)
				fmt.Println("  任务B完成")
				return nil
			}),
			CompletableFromAction(func() error {
				fmt.Println("  任务C开始...")
				time.Sleep(50 * time.Millisecond)
				fmt.Println("  任务C完成")
				return nil
			}),
		}

		start := time.Now()
		err := CompletableMerge(tasks...).BlockingAwait()
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("并行操作失败: %v", err)
		}

		fmt.Printf("✓ 所有任务完成，总耗时: %v\n", elapsed)
	})

	// 4. 错误处理演示
	t.Run("错误处理", func(t *testing.T) {
		fmt.Println("\n=== 错误处理演示 ===")

		// 模拟不稳定的操作
		attempts := 0
		unstableOperation := CompletableDefer(func() Completable {
			attempts++
			return CompletableFromAction(func() error {
				fmt.Printf("  尝试 #%d...\n", attempts)
				if attempts < 3 {
					return fmt.Errorf("模拟失败 #%d", attempts)
				}
				return nil
			})
		})

		// 重试直到成功
		err := unstableOperation.Retry(3).BlockingAwait()
		if err != nil {
			t.Errorf("重试操作失败: %v", err)
		}

		fmt.Printf("✓ 操作成功，共尝试 %d 次\n", attempts)
	})

	// 5. 转换演示
	t.Run("类型转换", func(t *testing.T) {
		fmt.Println("\n=== 类型转换演示 ===")

		completable := CompletableFromAction(func() error {
			fmt.Println("  执行Completable操作...")
			return nil
		})

		// 转换为Single
		value, err := completable.ToSingle("成功结果").BlockingGet()
		if err != nil {
			t.Errorf("转换为Single失败: %v", err)
		}

		fmt.Printf("✓ Single结果: %v\n", value)

		// 转换为Observable
		itemCount := 0
		completable.ToObservable().Subscribe(func(item Item) {
			itemCount++
			if item.IsError() {
				t.Errorf("Observable不应该发生错误: %v", item.Error)
				return
			}
			if item.Value == nil {
				fmt.Printf("✓ Observable完成，接收到 %d 个项目\n", itemCount)
			}
		})

		time.Sleep(50 * time.Millisecond)
	})

	fmt.Println("\n🎉 Completable Demo 完成!")
}

// BenchmarkCompletableOperations 性能基准测试
func BenchmarkCompletableOperations(b *testing.B) {
	b.Run("CompletableComplete", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			CompletableComplete().BlockingAwait()
		}
	})

	b.Run("CompletableFromAction", func(b *testing.B) {
		action := func() error { return nil }
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			CompletableFromAction(action).BlockingAwait()
		}
	})

	b.Run("CompletableAndThen", func(b *testing.B) {
		c1 := CompletableComplete()
		c2 := CompletableComplete()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c1.AndThen(c2).BlockingAwait()
		}
	})

	b.Run("CompletableMerge", func(b *testing.B) {
		completables := []Completable{
			CompletableComplete(),
			CompletableComplete(),
			CompletableComplete(),
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			CompletableMerge(completables...).BlockingAwait()
		}
	})
}
