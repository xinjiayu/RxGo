// Basic tests for RxGo
// RxGo 基本测试，验证核心功能
package rxgo

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// 基础功能测试
// ============================================================================

// TestBasicObservable 测试基础Observable功能
func TestBasicObservable(t *testing.T) {
	fmt.Println("=== TestBasicObservable ===")

	// 测试Just操作符
	values := []interface{}{}
	var mu sync.Mutex

	observable := Just(1, 2, 3, 4, 5)

	subscription := observable.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			values = append(values, value)
			mu.Unlock()
		},
		func(err error) {
			t.Errorf("不应该有错误: %v", err)
		},
		func() {
			fmt.Println("Observable完成")
		},
	)

	// 等待异步操作完成
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if len(values) != 5 {
		t.Errorf("期望5个值，实际得到%d个", len(values))
	}
	mu.Unlock()

	subscription.Unsubscribe()
	fmt.Println("TestBasicObservable 通过")
}

// TestMapOperator 测试Map操作符
func TestMapOperator(t *testing.T) {
	fmt.Println("=== TestMapOperator ===")

	values := []interface{}{}
	var mu sync.Mutex

	observable := Just(1, 2, 3).Map(func(value interface{}) (interface{}, error) {
		return value.(int) * 2, nil
	})

	subscription := observable.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			values = append(values, value)
			mu.Unlock()
		},
		func(err error) {
			t.Errorf("不应该有错误: %v", err)
		},
		func() {
			fmt.Println("Map操作完成")
		},
	)

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	expected := []interface{}{2, 4, 6}
	if len(values) != len(expected) {
		t.Errorf("期望%d个值，实际得到%d个", len(expected), len(values))
	}
	for i, v := range values {
		if v != expected[i] {
			t.Errorf("索引%d处期望%v，实际得到%v", i, expected[i], v)
		}
	}
	mu.Unlock()

	subscription.Unsubscribe()
	fmt.Println("TestMapOperator 通过")
}

// TestFilterOperator 测试Filter操作符
func TestFilterOperator(t *testing.T) {
	fmt.Println("=== TestFilterOperator ===")

	values := []interface{}{}
	var mu sync.Mutex

	observable := Just(1, 2, 3, 4, 5).Filter(func(value interface{}) bool {
		return value.(int)%2 == 0
	})

	subscription := observable.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			values = append(values, value)
			mu.Unlock()
		},
		func(err error) {
			t.Errorf("不应该有错误: %v", err)
		},
		func() {
			fmt.Println("Filter操作完成")
		},
	)

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	expected := []interface{}{2, 4}
	if len(values) != len(expected) {
		t.Errorf("期望%d个值，实际得到%d个", len(expected), len(values))
	}
	for i, v := range values {
		if v != expected[i] {
			t.Errorf("索引%d处期望%v，实际得到%v", i, expected[i], v)
		}
	}
	mu.Unlock()

	subscription.Unsubscribe()
	fmt.Println("TestFilterOperator 通过")
}

// TestTakeOperator 测试Take操作符
func TestTakeOperator(t *testing.T) {
	fmt.Println("=== TestTakeOperator ===")

	values := []interface{}{}
	var mu sync.Mutex

	observable := Just(1, 2, 3, 4, 5).Take(3)

	subscription := observable.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			values = append(values, value)
			mu.Unlock()
		},
		func(err error) {
			t.Errorf("不应该有错误: %v", err)
		},
		func() {
			fmt.Println("Take操作完成")
		},
	)

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if len(values) != 3 {
		t.Errorf("期望3个值，实际得到%d个", len(values))
	}
	expected := []interface{}{1, 2, 3}
	for i, v := range values {
		if v != expected[i] {
			t.Errorf("索引%d处期望%v，实际得到%v", i, expected[i], v)
		}
	}
	mu.Unlock()

	subscription.Unsubscribe()
	fmt.Println("TestTakeOperator 通过")
}

// ============================================================================
// Subject测试
// ============================================================================

// TestPublishSubject 测试PublishSubject
func TestPublishSubject(t *testing.T) {
	fmt.Println("=== TestPublishSubject ===")

	subject := NewPublishSubject()

	values1 := []interface{}{}
	values2 := []interface{}{}
	var mu sync.Mutex

	// 第一个订阅者
	sub1 := subject.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			values1 = append(values1, value)
			mu.Unlock()
		},
		nil,
		nil,
	)

	// 发射一些值
	subject.OnNext("A")
	subject.OnNext("B")

	// 第二个订阅者（不会收到之前的值）
	sub2 := subject.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			values2 = append(values2, value)
			mu.Unlock()
		},
		nil,
		nil,
	)

	// 发射更多值
	subject.OnNext("C")
	subject.OnNext("D")

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	// 第一个订阅者应该收到所有值
	if len(values1) != 4 {
		t.Errorf("订阅者1期望4个值，实际得到%d个", len(values1))
	}
	// 第二个订阅者应该只收到后面的值
	if len(values2) != 2 {
		t.Errorf("订阅者2期望2个值，实际得到%d个", len(values2))
	}
	mu.Unlock()

	sub1.Unsubscribe()
	sub2.Unsubscribe()
	subject.Dispose()
	fmt.Println("TestPublishSubject 通过")
}

// TestBehaviorSubject 测试BehaviorSubject
func TestBehaviorSubject(t *testing.T) {
	fmt.Println("=== TestBehaviorSubject ===")

	subject := NewBehaviorSubject("初始值")

	values1 := []interface{}{}
	values2 := []interface{}{}
	var mu sync.Mutex

	// 第一个订阅者（会立即收到初始值）
	sub1 := subject.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			values1 = append(values1, value)
			mu.Unlock()
		},
		nil,
		nil,
	)

	// 发射新值
	subject.OnNext("新值1")
	subject.OnNext("新值2")

	// 第二个订阅者（会立即收到最新值）
	sub2 := subject.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			values2 = append(values2, value)
			mu.Unlock()
		},
		nil,
		nil,
	)

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	// 第一个订阅者应该收到初始值和所有新值
	if len(values1) != 3 {
		t.Errorf("订阅者1期望3个值，实际得到%d个", len(values1))
	}
	// 第二个订阅者应该立即收到最新值
	if len(values2) != 1 {
		t.Errorf("订阅者2期望1个值，实际得到%d个", len(values2))
	}
	if values2[0] != "新值2" {
		t.Errorf("订阅者2期望收到'新值2'，实际收到%v", values2[0])
	}
	mu.Unlock()

	sub1.Unsubscribe()
	sub2.Unsubscribe()
	subject.Dispose()
	fmt.Println("TestBehaviorSubject 通过")
}

// ============================================================================
// 组合操作符测试
// ============================================================================

// TestMergeOperator 测试Merge操作符
func TestMergeOperator(t *testing.T) {
	fmt.Println("=== TestMergeOperator ===")

	values := []interface{}{}
	var mu sync.Mutex

	obs1 := Just(1, 2, 3)
	obs2 := Just(4, 5, 6)

	merged := Merge(obs1, obs2)

	subscription := merged.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			values = append(values, value)
			mu.Unlock()
		},
		func(err error) {
			t.Errorf("不应该有错误: %v", err)
		},
		func() {
			fmt.Println("Merge操作完成")
		},
	)

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if len(values) != 6 {
		t.Errorf("期望6个值，实际得到%d个", len(values))
	}
	mu.Unlock()

	subscription.Unsubscribe()
	fmt.Println("TestMergeOperator 通过")
}

// TestConcatOperator 测试Concat操作符
func TestConcatOperator(t *testing.T) {
	fmt.Println("=== TestConcatOperator ===")

	values := []interface{}{}
	var mu sync.Mutex

	obs1 := Just(1, 2, 3)
	obs2 := Just(4, 5, 6)

	concatenated := Concat(obs1, obs2)

	subscription := concatenated.SubscribeWithCallbacks(
		func(value interface{}) {
			mu.Lock()
			values = append(values, value)
			mu.Unlock()
		},
		func(err error) {
			t.Errorf("不应该有错误: %v", err)
		},
		func() {
			fmt.Println("Concat操作完成")
		},
	)

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	expected := []interface{}{1, 2, 3, 4, 5, 6}
	if len(values) != len(expected) {
		t.Errorf("期望%d个值，实际得到%d个", len(expected), len(values))
	}
	for i, v := range values {
		if v != expected[i] {
			t.Errorf("索引%d处期望%v，实际得到%v", i, expected[i], v)
		}
	}
	mu.Unlock()

	subscription.Unsubscribe()
	fmt.Println("TestConcatOperator 通过")
}

// ============================================================================
// 调度器测试
// ============================================================================

// TestSchedulers 测试调度器
func TestSchedulers(t *testing.T) {
	fmt.Println("=== TestSchedulers ===")

	// 测试立即调度器
	immediateScheduler := NewImmediateScheduler()
	executed := false

	immediateScheduler.Schedule(func() {
		executed = true
	})

	if !executed {
		t.Error("立即调度器应该立即执行任务")
	}

	// 测试延迟调度器
	threadScheduler := NewNewThreadScheduler()
	delayExecuted := false

	start := time.Now()
	threadScheduler.ScheduleWithDelay(func() {
		delayExecuted = true
	}, 50*time.Millisecond)

	time.Sleep(100 * time.Millisecond)

	if !delayExecuted {
		t.Error("延迟调度器应该在延迟后执行任务")
	}

	elapsed := time.Since(start)
	if elapsed < 50*time.Millisecond {
		t.Error("延迟调度器执行时间过短")
	}

	fmt.Println("TestSchedulers 通过")
}

// ============================================================================
// 错误处理测试
// ============================================================================

// TestErrorHandling 测试错误处理
func TestErrorHandling(t *testing.T) {
	fmt.Println("=== TestErrorHandling ===")

	errorReceived := false
	var mu sync.Mutex

	// 创建会产生错误的Observable
	errorObservable := Create(func(observer Observer) {
		observer(CreateItem(1))
		observer(CreateErrorItem(fmt.Errorf("测试错误")))
		observer(CreateItem(2)) // 这个不会被发射
	})

	subscription := errorObservable.SubscribeWithCallbacks(
		func(value interface{}) {
			fmt.Printf("接收到值: %v\n", value)
		},
		func(err error) {
			mu.Lock()
			errorReceived = true
			mu.Unlock()
			fmt.Printf("接收到错误: %v\n", err)
		},
		func() {
			fmt.Println("错误处理完成")
		},
	)

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if !errorReceived {
		t.Error("应该接收到错误")
	}
	mu.Unlock()

	subscription.Unsubscribe()
	fmt.Println("TestErrorHandling 通过")
}

// ============================================================================
// 运行所有测试
// ============================================================================

// RunAllTests 运行所有测试
func RunAllTests() {
	fmt.Println("开始运行RxGo基础测试...")
	fmt.Println("==========================")

	// 创建一个简单的测试结构
	t := &testing.T{}

	// 运行测试
	TestBasicObservable(t)
	TestMapOperator(t)
	TestFilterOperator(t)
	TestTakeOperator(t)
	TestPublishSubject(t)
	TestBehaviorSubject(t)
	TestMergeOperator(t)
	TestConcatOperator(t)
	TestSchedulers(t)
	TestErrorHandling(t)

	fmt.Println("\n==========================")
	fmt.Println("所有基础测试完成!")
}
