// Test implementation for RxGo
// 测试实现的操作符功能
package rxgo

import (
	"fmt"
	"time"
)

// TestBasicOperators 测试基础操作符
func TestBasicOperators() {
	fmt.Println("=== 测试基础操作符 ===")

	// 测试Just和Map
	fmt.Println("1. 测试Just和Map:")
	Just(1, 2, 3, 4, 5).
		Map(func(value interface{}) (interface{}, error) {
			return value.(int) * 2, nil
		}).
		SubscribeWithCallbacks(
			func(value interface{}) {
				fmt.Printf("  值: %v\n", value)
			},
			func(err error) {
				fmt.Printf("  错误: %v\n", err)
			},
			func() {
				fmt.Println("  完成")
			},
		)

	time.Sleep(100 * time.Millisecond)

	// 测试Filter
	fmt.Println("\n2. 测试Filter:")
	Just(1, 2, 3, 4, 5).
		Filter(func(value interface{}) bool {
			return value.(int)%2 == 0
		}).
		SubscribeWithCallbacks(
			func(value interface{}) {
				fmt.Printf("  偶数: %v\n", value)
			},
			nil,
			func() {
				fmt.Println("  完成")
			},
		)

	time.Sleep(100 * time.Millisecond)

	// 测试Take
	fmt.Println("\n3. 测试Take:")
	Just(1, 2, 3, 4, 5).
		Take(3).
		SubscribeWithCallbacks(
			func(value interface{}) {
				fmt.Printf("  前3个: %v\n", value)
			},
			nil,
			func() {
				fmt.Println("  完成")
			},
		)

	time.Sleep(100 * time.Millisecond)
}

// TestAggregationOperators 测试聚合操作符
func TestAggregationOperators() {
	fmt.Println("\n=== 测试聚合操作符 ===")

	// 测试Count
	fmt.Println("1. 测试Count:")
	Just(1, 2, 3, 4, 5).
		Count().
		Subscribe(
			func(value interface{}) {
				fmt.Printf("  计数: %v\n", value)
			},
			func(err error) {
				fmt.Printf("  错误: %v\n", err)
			},
		)

	time.Sleep(100 * time.Millisecond)

	// 测试First
	fmt.Println("\n2. 测试First:")
	Just(10, 20, 30).
		First().
		Subscribe(
			func(value interface{}) {
				fmt.Printf("  第一个: %v\n", value)
			},
			func(err error) {
				fmt.Printf("  错误: %v\n", err)
			},
		)

	time.Sleep(100 * time.Millisecond)

	// 测试Sum
	fmt.Println("\n3. 测试Sum:")
	Just(1, 2, 3, 4, 5).
		Sum().
		Subscribe(
			func(value interface{}) {
				fmt.Printf("  总和: %v\n", value)
			},
			func(err error) {
				fmt.Printf("  错误: %v\n", err)
			},
		)

	time.Sleep(100 * time.Millisecond)
}

// TestTimeOperators 测试时间操作符
func TestTimeOperators() {
	fmt.Println("\n=== 测试时间操作符 ===")

	// 测试Delay
	fmt.Println("1. 测试Delay:")
	start := time.Now()
	Just(1, 2, 3).
		Delay(200*time.Millisecond).
		SubscribeWithCallbacks(
			func(value interface{}) {
				elapsed := time.Since(start)
				fmt.Printf("  延迟值: %v (耗时: %v)\n", value, elapsed)
			},
			nil,
			func() {
				fmt.Println("  延迟完成")
			},
		)

	time.Sleep(1 * time.Second)

	// 测试Debounce
	fmt.Println("\n2. 测试Debounce:")
	subject := NewPublishSubject()

	subject.
		Debounce(300*time.Millisecond).
		SubscribeWithCallbacks(
			func(value interface{}) {
				fmt.Printf("  防抖值: %v\n", value)
			},
			nil,
			func() {
				fmt.Println("  防抖完成")
			},
		)

	// 快速发送多个值
	go func() {
		subject.OnNext(1)
		time.Sleep(100 * time.Millisecond)
		subject.OnNext(2)
		time.Sleep(100 * time.Millisecond)
		subject.OnNext(3)
		time.Sleep(500 * time.Millisecond) // 等待防抖
		subject.OnNext(4)
		time.Sleep(500 * time.Millisecond)
		subject.OnComplete()
	}()

	time.Sleep(2 * time.Second)
}

// TestCombinationOperators 测试组合操作符
func TestCombinationOperators() {
	fmt.Println("\n=== 测试组合操作符 ===")

	// 测试Merge
	fmt.Println("1. 测试Merge:")
	obs1 := Just(1, 2, 3)
	obs2 := Just(4, 5, 6)

	obs1.Merge(obs2).
		SubscribeWithCallbacks(
			func(value interface{}) {
				fmt.Printf("  合并值: %v\n", value)
			},
			nil,
			func() {
				fmt.Println("  合并完成")
			},
		)

	time.Sleep(100 * time.Millisecond)

	// 测试Zip
	fmt.Println("\n2. 测试Zip:")
	obs3 := Just(1, 2, 3)
	obs4 := Just("a", "b", "c")

	obs3.Zip(obs4, func(left, right interface{}) interface{} {
		return fmt.Sprintf("%v-%v", left, right)
	}).
		SubscribeWithCallbacks(
			func(value interface{}) {
				fmt.Printf("  压缩值: %v\n", value)
			},
			nil,
			func() {
				fmt.Println("  压缩完成")
			},
		)

	time.Sleep(100 * time.Millisecond)
}

// TestErrorHandlingOperators 测试错误处理操作符
func TestErrorHandlingOperators() {
	fmt.Println("\n=== 测试错误处理 ===")

	// 测试Catch
	fmt.Println("1. 测试Catch:")
	Create(func(observer Observer) {
		observer(CreateItem(1))
		observer(CreateItem(2))
		observer(CreateErrorItem(fmt.Errorf("测试错误")))
	}).
		Catch(func(err error) Observable {
			fmt.Printf("  捕获错误: %v\n", err)
			return Just(999) // 恢复值
		}).
		SubscribeWithCallbacks(
			func(value interface{}) {
				fmt.Printf("  值: %v\n", value)
			},
			func(err error) {
				fmt.Printf("  最终错误: %v\n", err)
			},
			func() {
				fmt.Println("  完成")
			},
		)

	time.Sleep(100 * time.Millisecond)

	// 测试Retry
	fmt.Println("\n2. 测试Retry:")
	attempts := 0
	Create(func(observer Observer) {
		attempts++
		fmt.Printf("  尝试 #%d\n", attempts)
		if attempts < 3 {
			observer(CreateErrorItem(fmt.Errorf("尝试 %d 失败", attempts)))
		} else {
			observer(CreateItem("成功!"))
			observer(CreateItem(nil)) // 完成
		}
	}).
		Retry(3).
		SubscribeWithCallbacks(
			func(value interface{}) {
				fmt.Printf("  最终值: %v\n", value)
			},
			func(err error) {
				fmt.Printf("  最终错误: %v\n", err)
			},
			func() {
				fmt.Println("  重试完成")
			},
		)

	time.Sleep(100 * time.Millisecond)
}

// TestSideEffects 测试副作用操作符
func TestSideEffects() {
	fmt.Println("\n=== 测试副作用操作符 ===")

	// 测试DoOnNext
	fmt.Println("1. 测试DoOnNext:")
	Just(1, 2, 3).
		DoOnNext(func(value interface{}) {
			fmt.Printf("  副作用: 处理 %v\n", value)
		}).
		Map(func(value interface{}) (interface{}, error) {
			return value.(int) * 10, nil
		}).
		SubscribeWithCallbacks(
			func(value interface{}) {
				fmt.Printf("  最终值: %v\n", value)
			},
			nil,
			func() {
				fmt.Println("  副作用完成")
			},
		)

	time.Sleep(100 * time.Millisecond)
}

// TestBlockingOperators 测试阻塞操作符
func TestBlockingOperators() {
	fmt.Println("\n=== 测试阻塞操作符 ===")

	// 测试BlockingFirst
	fmt.Println("1. 测试BlockingFirst:")
	first, err := Just(10, 20, 30).BlockingFirst()
	if err != nil {
		fmt.Printf("  错误: %v\n", err)
	} else {
		fmt.Printf("  第一个值: %v\n", first)
	}

	// 测试BlockingLast
	fmt.Println("\n2. 测试BlockingLast:")
	last, err := Just(10, 20, 30).BlockingLast()
	if err != nil {
		fmt.Printf("  错误: %v\n", err)
	} else {
		fmt.Printf("  最后一个值: %v\n", last)
	}

	// 测试ToSlice
	fmt.Println("\n3. 测试ToSlice:")
	Just(1, 2, 3, 4, 5).
		ToSlice().
		Subscribe(
			func(value interface{}) {
				fmt.Printf("  切片: %v\n", value)
			},
			func(err error) {
				fmt.Printf("  错误: %v\n", err)
			},
		)

	time.Sleep(100 * time.Millisecond)
}

// RunAllImplementationTests 运行所有实现测试
func RunAllImplementationTests() {
	fmt.Println("开始测试RxGo 实现...")

	TestBasicOperators()
	TestAggregationOperators()
	TestTimeOperators()
	TestCombinationOperators()
	TestErrorHandlingOperators()
	TestSideEffects()
	TestBlockingOperators()

	fmt.Println("\n所有测试完成!")
}
