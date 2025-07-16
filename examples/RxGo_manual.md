# RxGo 开发手册

> 基于Go语言特性的响应式编程库完整开发指南

## 📋 目录

1. [概述](#概述)
2. [核心概念](#核心概念)
3. [基础类型系统](#基础类型系统)
4. [工厂函数](#工厂函数)
5. [操作符详解](#操作符详解)
6. [Subject系统](#subject系统)
7. [调度器系统](#调度器系统)
8. [错误处理](#错误处理)
9. [背压处理](#背压处理)
10. [并行处理](#并行处理)
11. [性能优化](#性能优化)
12. [最佳实践](#最佳实践)
13. [常见问题](#常见问题)

## 概述

RxGo是ReactiveX在Go语言上的完整实现，提供了一套强大的响应式编程API。本库专为Go语言设计，充分利用了goroutines、channels和context等Go语言特性。

### 核心特性

- **完整的响应式类型**: Observable, Single, Maybe, Completable, Flowable, ParallelFlowable
- **Subject热流系统**: PublishSubject, BehaviorSubject, ReplaySubject, AsyncSubject
- **丰富的操作符**: 145+个操作符，涵盖转换、过滤、组合、聚合等
- **高性能调度器**: 包含工作窃取调度器等6种调度器
- **背压支持**: 完整的Flowable背压处理机制
- **Go原生优化**: Context集成、Goroutine池化、内存优化

### 安装

```bash
go get github.com/xinjiayu/rxgo
```

```go
import "github.com/xinjiayu/rxgo"
```

## 核心概念

### 响应式流

响应式流是一个处理数据流的标准，包含以下核心组件：

```
Publisher → Subscription → Subscriber
    ↓            ↓            ↓
  源数据流    请求/取消      背压控制
```

### Observable vs Hot/Cold

- **Cold Observable**: 每个订阅者都会收到完整的数据序列
- **Hot Observable**: 多个订阅者共享同一个数据源（如Subject）

## 基础类型系统

### Item - 数据项

```go
type Item struct {
    Value interface{} // 数据值
    Error error       // 错误信息
}

// 创建数据项
item := rxgo.CreateItem(42)
errorItem := rxgo.CreateErrorItem(errors.New("error"))

// 检查错误
if item.IsError() {
    fmt.Println("错误:", item.Error)
} else {
    fmt.Println("值:", item.GetValue())
}
```

### Observable - 0..N项数据流

```go
// 基本用法
observable := rxgo.Just(1, 2, 3, 4, 5)

subscription := observable.SubscribeWithCallbacks(
    func(value interface{}) {
        fmt.Printf("接收到: %v\n", value)
    },
    func(err error) {
        fmt.Printf("错误: %v\n", err)
    },
    func() {
        fmt.Println("完成")
    },
)

// 记得取消订阅
defer subscription.Unsubscribe()
```

### Single - 单值类型

```go
// 创建Single
single := rxgo.SingleJust(42)

// 订阅
single.Subscribe(
    func(value interface{}) {
        fmt.Printf("Single值: %v\n", value)
    },
    func(err error) {
        fmt.Printf("Single错误: %v\n", err)
    },
)

// 阻塞获取值
value, err := single.BlockingGet()
if err != nil {
    fmt.Printf("错误: %v\n", err)
} else {
    fmt.Printf("值: %v\n", value)
}
```

### Maybe - 可选值类型

```go
// 创建Maybe
maybe := rxgo.MaybeJust(42)
emptyMaybe := rxgo.MaybeEmpty()

// 订阅
maybe.Subscribe(
    func(value interface{}) {
        fmt.Printf("Maybe值: %v\n", value)
    },
    func(err error) {
        fmt.Printf("Maybe错误: %v\n", err)
    },
    func() {
        fmt.Println("Maybe空值")
    },
)

// 提供默认值
single := emptyMaybe.DefaultIfEmpty("默认值")
```

### Completable - 完成信号类型

```go
// 创建Completable
completable := rxgo.CompletableFromAction(func() error {
    fmt.Println("执行异步操作...")
    time.Sleep(100 * time.Millisecond)
    return nil // 返回nil表示成功
})

// 订阅
completable.Subscribe(
    func() {
        fmt.Println("操作完成")
    },
    func(err error) {
        fmt.Printf("操作失败: %v\n", err)
    },
)

// 阻塞等待完成
err := completable.BlockingAwait()
if err != nil {
    fmt.Printf("操作失败: %v\n", err)
}
```

### Flowable - 背压支持的数据流

```go
// 创建Flowable
flowable := rxgo.FlowableRange(1, 1000)

// 背压处理
buffered := flowable.OnBackpressureBuffer()
dropped := flowable.OnBackpressureDrop()
latest := flowable.OnBackpressureLatest()

// 订阅时控制请求速率
subscription := buffered.SubscribeWithCallbacks(
    func(value interface{}) {
        fmt.Printf("处理: %v\n", value)
    },
    func(err error) {
        fmt.Printf("错误: %v\n", err)
    },
    func() {
        fmt.Println("完成")
    },
)

// 请求特定数量的元素
subscription.Request(10)
```

## 工厂函数

### 基础工厂函数

```go
// Just - 发射指定的值
obs := rxgo.Just(1, 2, 3, 4, 5)

// Empty - 立即完成的空Observable
empty := rxgo.Empty()

// Never - 永不发射任何值
never := rxgo.Never()

// Error - 立即发射错误
error := rxgo.Error(errors.New("something wrong"))

// Range - 发射整数序列
range := rxgo.Range(1, 10) // 从1到10

// Create - 自定义发射器
custom := rxgo.Create(func(observer rxgo.Observer) {
    observer(rxgo.CreateItem(1))
    observer(rxgo.CreateItem(2))
    observer(rxgo.CreateItem(nil)) // 完成信号
})
```

### 从数据源创建

```go
// FromSlice - 从切片创建
slice := []interface{}{1, 2, 3, 4, 5}
obs := rxgo.FromSlice(slice)

// FromChannel - 从Go channel创建
ch := make(chan interface{}, 5)
go func() {
    for i := 1; i <= 5; i++ {
        ch <- i
    }
    close(ch)
}()
obs := rxgo.FromChannel(ch)

// FromIterable - 从可迭代函数创建
obs := rxgo.FromIterable(func() <-chan interface{} {
    ch := make(chan interface{}, 5)
    go func() {
        defer close(ch)
        for i := 1; i <= 5; i++ {
            ch <- i
        }
    }()
    return ch
})
```

### 时间相关工厂函数

```go
// Interval - 定期发射递增整数
interval := rxgo.Interval(100 * time.Millisecond)

// Timer - 延迟后发射单个值
timer := rxgo.Timer(1 * time.Second)

// Defer - 延迟创建Observable
deferred := rxgo.Defer(func() rxgo.Observable {
    return rxgo.Just(time.Now().Unix())
})
```

## 操作符详解

### 转换操作符

#### Map - 转换每个元素

```go
observable := rxgo.Range(1, 5)

// 基本映射
mapped := observable.Map(func(value interface{}) (interface{}, error) {
    return value.(int) * 2, nil
})

// 错误处理
mapped := observable.Map(func(value interface{}) (interface{}, error) {
    if value.(int) == 3 {
        return nil, errors.New("不能是3")
    }
    return value.(int) * 2, nil
})
```

#### FlatMap - 平铺映射

```go
observable := rxgo.Just(1, 2, 3)

// 每个元素转换为Observable并合并
flatMapped := observable.FlatMap(func(value interface{}) rxgo.Observable {
    return rxgo.Range(value.(int), 3)
})
// 输出: 1,2,3,2,3,4,3,4,5
```

#### Filter - 过滤元素

```go
observable := rxgo.Range(1, 10)

// 只保留偶数
evens := observable.Filter(func(value interface{}) bool {
    return value.(int)%2 == 0
})
```

#### Take/Skip - 取前N个/跳过前N个

```go
observable := rxgo.Range(1, 10)

// 取前3个
first3 := observable.Take(3) // 1,2,3

// 跳过前3个
rest := observable.Skip(3) // 4,5,6,7,8,9,10

// 条件取值
takeWhile := observable.TakeWhile(func(value interface{}) bool {
    return value.(int) < 5
}) // 1,2,3,4
```

#### Buffer/Window - 缓冲和窗口

```go
observable := rxgo.Range(1, 10)

// 按数量缓冲
buffered := observable.Buffer(3)
// 输出: [1,2,3], [4,5,6], [7,8,9], [10]

// 按时间缓冲
timeBuffered := observable.BufferWithTime(100 * time.Millisecond)

// 按数量开窗
windowed := observable.Window(3)
// 输出: Observable{1,2,3}, Observable{4,5,6}, ...
```

#### Distinct - 去重

```go
observable := rxgo.Just(1, 2, 2, 3, 3, 3, 4)

// 去除重复
distinct := observable.Distinct()
// 输出: 1, 2, 3, 4

// 去除连续重复
distinctUntilChanged := observable.DistinctUntilChanged()
// 输出: 1, 2, 3, 4
```

### 组合操作符

#### Merge - 合并多个Observable

```go
obs1 := rxgo.Just(1, 3, 5)
obs2 := rxgo.Just(2, 4, 6)

// 合并（并发）
merged := obs1.Merge(obs2)
// 可能输出: 1,2,3,4,5,6 (顺序不确定)

// 静态合并多个
merged := rxgo.Merge(obs1, obs2, rxgo.Just(7, 8, 9))
```

#### Concat - 连接多个Observable

```go
obs1 := rxgo.Just(1, 2, 3)
obs2 := rxgo.Just(4, 5, 6)

// 顺序连接
concatenated := obs1.Concat(obs2)
// 输出: 1,2,3,4,5,6

// 静态连接多个
concatenated := rxgo.Concat(obs1, obs2, rxgo.Just(7, 8, 9))
```

#### Zip - 组合对应位置的元素

```go
obs1 := rxgo.Just(1, 2, 3)
obs2 := rxgo.Just("A", "B", "C")

// 组合
zipped := obs1.Zip(obs2, func(a, b interface{}) interface{} {
    return fmt.Sprintf("%d-%s", a.(int), b.(string))
})
// 输出: "1-A", "2-B", "3-C"
```

#### CombineLatest - 组合最新值

```go
obs1 := rxgo.Interval(100 * time.Millisecond).Take(3)
obs2 := rxgo.Interval(150 * time.Millisecond).Take(3)

// 组合最新值
combined := obs1.CombineLatest(obs2, func(a, b interface{}) interface{} {
    return fmt.Sprintf("%v-%v", a, b)
})
```

### 聚合操作符

#### Reduce - 归约

```go
observable := rxgo.Range(1, 5)

// 求和
sum := observable.Reduce(func(acc, curr interface{}) interface{} {
    return acc.(int) + curr.(int)
})

result, _ := sum.BlockingGet()
fmt.Println("总和:", result) // 15
```

#### Scan - 扫描（累积）

```go
observable := rxgo.Range(1, 5)

// 累积求和
scanned := observable.Scan(func(acc, curr interface{}) interface{} {
    return acc.(int) + curr.(int)
})
// 输出: 1, 3, 6, 10, 15
```

#### Count, First, Last

```go
observable := rxgo.Range(1, 10)

// 计数
count := observable.Count()
result, _ := count.BlockingGet()
fmt.Println("数量:", result) // 10

// 第一个
first := observable.First()
result, _ = first.BlockingGet()
fmt.Println("第一个:", result) // 1

// 最后一个
last := observable.Last()
result, _ = last.BlockingGet()
fmt.Println("最后一个:", result) // 10
```

#### Min, Max, Sum, Average

```go
observable := rxgo.Just(3, 1, 4, 1, 5, 9)

// 最小值
min := observable.Min()
result, _ := min.BlockingGet()
fmt.Println("最小值:", result)

// 最大值
max := observable.Max()
result, _ = max.BlockingGet()
fmt.Println("最大值:", result)

// 求和
sum := observable.Sum()
result, _ = sum.BlockingGet()
fmt.Println("总和:", result)

// 平均值
avg := observable.Average()
result, _ = avg.BlockingGet()
fmt.Println("平均值:", result)
```

#### All, Any, Contains

```go
observable := rxgo.Range(1, 10)

// 所有元素都满足条件
allPositive := observable.All(func(value interface{}) bool {
    return value.(int) > 0
})

// 任何元素满足条件
hasEven := observable.Any(func(value interface{}) bool {
    return value.(int)%2 == 0
})

// 包含特定值
contains5 := observable.Contains(5)
```

### 时间操作符

#### Debounce - 防抖

```go
// 模拟用户输入
input := rxgo.Create(func(observer rxgo.Observer) {
    observer(rxgo.CreateItem("a"))
    time.Sleep(50 * time.Millisecond)
    observer(rxgo.CreateItem("ab"))
    time.Sleep(50 * time.Millisecond)
    observer(rxgo.CreateItem("abc"))
    time.Sleep(200 * time.Millisecond) // 大于防抖时间
    observer(rxgo.CreateItem("abcd"))
    observer(rxgo.CreateItem(nil))
})

// 防抖100ms
debounced := input.Debounce(100 * time.Millisecond)
// 只输出: "abc", "abcd"
```

#### Throttle - 节流

```go
observable := rxgo.Interval(50 * time.Millisecond).Take(10)

// 节流200ms
throttled := observable.Throttle(200 * time.Millisecond)
// 每200ms最多发射一个值
```

#### Delay - 延迟

```go
observable := rxgo.Just(1, 2, 3)

// 延迟500ms
delayed := observable.Delay(500 * time.Millisecond)
```

#### Timeout - 超时

```go
// 模拟慢速Observable
slow := rxgo.Create(func(observer rxgo.Observer) {
    time.Sleep(2 * time.Second)
    observer(rxgo.CreateItem("slow"))
    observer(rxgo.CreateItem(nil))
})

// 1秒超时
timedOut := slow.Timeout(1 * time.Second)
```

### 副作用操作符

#### DoOnNext, DoOnError, DoOnComplete

```go
observable := rxgo.Just(1, 2, 3)

// 添加副作用
withSideEffects := observable.
    DoOnNext(func(value interface{}) {
        fmt.Printf("处理中: %v\n", value)
    }).
    DoOnError(func(err error) {
        fmt.Printf("发生错误: %v\n", err)
    }).
    DoOnComplete(func() {
        fmt.Println("处理完成")
    })
```

### 高级操作符

#### StartWith - 在序列开始前添加值

```go
observable := rxgo.Just(4, 5, 6)

// 在开始前添加值
withPrefix := observable.StartWith(1, 2, 3)
// 输出: 1, 2, 3, 4, 5, 6
```

#### Timestamp - 添加时间戳

```go
observable := rxgo.Just(1, 2, 3)

// 添加时间戳
timestamped := observable.Timestamp()
// 输出: Timestamped{Value: 1, Timestamp: time}
```

#### WithLatestFrom - 与最新值组合

```go
source := rxgo.Interval(100 * time.Millisecond)
other := rxgo.Interval(200 * time.Millisecond)

// 与other的最新值组合
combined := source.WithLatestFrom(other, func(s, o interface{}) interface{} {
    return fmt.Sprintf("source:%v, latest:%v", s, o)
})
```

#### Join - 基于时间窗口的连接

```go
left := rxgo.Interval(100 * time.Millisecond).Take(5)
right := rxgo.Interval(150 * time.Millisecond).Take(5)

// 时间窗口连接
joined := left.Join(
    right,
    func(item interface{}) time.Duration { return 200 * time.Millisecond }, // 左窗口
    func(item interface{}) time.Duration { return 200 * time.Millisecond }, // 右窗口
    func(l, r interface{}) interface{} { return fmt.Sprintf("L%v-R%v", l, r) },
)
```

## Subject系统

Subject既是Observable又是Observer，用于创建热流。

### PublishSubject - 发布主题

```go
// 创建PublishSubject
subject := rxgo.NewPublishSubject()

// 创建多个订阅者
sub1 := subject.SubscribeWithCallbacks(
    func(value interface{}) {
        fmt.Printf("订阅者1: %v\n", value)
    },
    func(err error) {
        fmt.Printf("订阅者1错误: %v\n", err)
    },
    func() {
        fmt.Println("订阅者1完成")
    },
)

sub2 := subject.SubscribeWithCallbacks(
    func(value interface{}) {
        fmt.Printf("订阅者2: %v\n", value)
    },
    func(err error) {
        fmt.Printf("订阅者2错误: %v\n", err)
    },
    func() {
        fmt.Println("订阅者2完成")
    },
)

// 发射数据
subject.OnNext("Hello")
subject.OnNext("World")

// 新订阅者只能收到后续的值
sub3 := subject.Subscribe(func(item rxgo.Item) {
    if !item.IsError() && item.Value != nil {
        fmt.Printf("订阅者3: %v\n", item.Value)
    }
})

subject.OnNext("Late")
subject.OnComplete()

// 清理
sub1.Unsubscribe()
sub2.Unsubscribe()
sub3.Unsubscribe()
```

### BehaviorSubject - 行为主题

```go
// 创建带初始值的BehaviorSubject
subject := rxgo.NewBehaviorSubject("初始值")

// 新订阅者立即收到最新值
sub1 := subject.SubscribeWithCallbacks(
    func(value interface{}) {
        fmt.Printf("订阅者1: %v\n", value)
    },
    nil, nil,
)
// 立即输出: "初始值"

// 发射新值
subject.OnNext("新值1")
subject.OnNext("新值2")

// 新订阅者立即收到最新值
sub2 := subject.SubscribeWithCallbacks(
    func(value interface{}) {
        fmt.Printf("订阅者2: %v\n", value)
    },
    nil, nil,
)
// 立即输出: "新值2"

// 获取当前值
if value, hasValue := subject.GetValue(); hasValue {
    fmt.Printf("当前值: %v\n", value)
}
```

### ReplaySubject - 重放主题

```go
// 创建ReplaySubject，缓存最近5个值
subject := rxgo.NewReplaySubject(5)

// 发射一些值
subject.OnNext("值1")
subject.OnNext("值2")
subject.OnNext("值3")

// 新订阅者会收到所有缓存的值
subscriber := subject.SubscribeWithCallbacks(
    func(value interface{}) {
        fmt.Printf("重放: %v\n", value)
    },
    nil, nil,
)
// 立即输出: "值1", "值2", "值3"

// 继续发射
subject.OnNext("值4")
// 输出: "值4"

// 获取缓存的值
buffered := subject.GetBufferedValues()
fmt.Printf("缓存的值: %v\n", buffered)
```

### AsyncSubject - 异步主题

```go
// 创建AsyncSubject
subject := rxgo.NewAsyncSubject()

// 订阅者
subscriber := subject.SubscribeWithCallbacks(
    func(value interface{}) {
        fmt.Printf("最终值: %v\n", value)
    },
    nil,
    func() {
        fmt.Println("AsyncSubject完成")
    },
)

// 发射多个值（只有最后一个会被发送）
subject.OnNext("值1")
subject.OnNext("值2")
subject.OnNext("最终值")

// 只有调用OnComplete后，订阅者才会收到最后一个值
subject.OnComplete()
// 输出: "最终值", "AsyncSubject完成"
```

## 调度器系统

调度器控制Observable在哪里以及如何执行。

### 内置调度器

```go
// 立即调度器 - 在当前goroutine中执行
observable.SubscribeOn(rxgo.ImmediateScheduler)

// 新线程调度器 - 每次创建新goroutine
observable.SubscribeOn(rxgo.NewThreadScheduler)

// 线程池调度器 - 使用goroutine池
observable.SubscribeOn(rxgo.ThreadPoolScheduler)

// 当前线程调度器 - 队列执行
observable.SubscribeOn(rxgo.CurrentThreadScheduler)

// 测试调度器 - 虚拟时间
testScheduler := rxgo.NewTestScheduler()
observable.SubscribeOn(testScheduler)
```

### 调度器使用示例

```go
// IO密集型操作使用新线程调度器
ioObservable := rxgo.Create(func(observer rxgo.Observer) {
    // 模拟IO操作
    data, err := ioutil.ReadFile("large_file.txt")
    if err != nil {
        observer(rxgo.CreateErrorItem(err))
        return
    }
    observer(rxgo.CreateItem(string(data)))
    observer(rxgo.CreateItem(nil))
}).SubscribeOn(rxgo.NewThreadScheduler)

// CPU密集型操作使用线程池调度器
cpuObservable := rxgo.Range(1, 1000000).
    Map(func(value interface{}) (interface{}, error) {
        // 复杂计算
        return complexCalculation(value.(int)), nil
    }).
    SubscribeOn(rxgo.ThreadPoolScheduler)

// 简单操作使用立即调度器
simpleObservable := rxgo.Just(1, 2, 3).
    Map(func(value interface{}) (interface{}, error) {
        return value.(int) * 2, nil
    }).
    SubscribeOn(rxgo.ImmediateScheduler)
```

### 工作窃取调度器（RxGo独有）

```go
// 创建工作窃取调度器
wsScheduler := rxgo.NewWorkStealingScheduler(runtime.NumCPU())
wsScheduler.Start()

// 使用工作窃取调度器
observable := rxgo.Range(1, 10000).
    Map(func(value interface{}) (interface{}, error) {
        // CPU密集型任务
        return heavyComputation(value.(int)), nil
    }).
    SubscribeOn(wsScheduler)

// 获取性能统计
stats := rxgo.GetWorkStealingStats()
fmt.Printf("负载均衡比例: %.2f\n", stats.LoadBalanceRatio)
fmt.Printf("工作窃取次数: %d\n", stats.WorkStealingCount)

// 停止调度器
defer wsScheduler.Stop()
```

### ObserveOn vs SubscribeOn

```go
observable := rxgo.Just(1, 2, 3)

// SubscribeOn: 控制订阅操作在哪个调度器上执行
subscribedOn := observable.SubscribeOn(rxgo.NewThreadScheduler)

// ObserveOn: 控制观察者接收通知在哪个调度器上执行
observedOn := subscribedOn.ObserveOn(rxgo.ImmediateScheduler)

// 可以链式调用
chained := rxgo.Just(1, 2, 3).
    SubscribeOn(rxgo.NewThreadScheduler).   // 订阅在新线程
    Map(func(v interface{}) (interface{}, error) {
        return v.(int) * 2, nil
    }).
    ObserveOn(rxgo.CurrentThreadScheduler). // 观察在当前线程
    Filter(func(v interface{}) bool {
        return v.(int) > 3
    })
```

## 错误处理

### Catch - 错误捕获

```go
// 创建可能出错的Observable
errorProne := rxgo.Create(func(observer rxgo.Observer) {
    observer(rxgo.CreateItem(1))
    observer(rxgo.CreateItem(2))
    observer(rxgo.CreateErrorItem(errors.New("网络错误")))
    observer(rxgo.CreateItem(3)) // 不会被发射
})

// 捕获错误并提供恢复值
recovered := errorProne.Catch(func(err error) rxgo.Observable {
    fmt.Printf("捕获错误: %v，使用备用数据\n", err)
    return rxgo.Just("备用数据1", "备用数据2")
})

recovered.SubscribeWithCallbacks(
    func(value interface{}) {
        fmt.Printf("接收: %v\n", value)
    },
    func(err error) {
        fmt.Printf("最终错误: %v\n", err)
    },
    func() {
        fmt.Println("完成")
    },
)
// 输出: 1, 2, "备用数据1", "备用数据2", 完成
```

### Retry - 重试

```go
attempt := 0
retryableOperation := rxgo.Create(func(observer rxgo.Observer) {
    attempt++
    fmt.Printf("尝试 #%d\n", attempt)
    
    if attempt < 3 {
        observer(rxgo.CreateErrorItem(fmt.Errorf("尝试%d失败", attempt)))
    } else {
        observer(rxgo.CreateItem("成功!"))
        observer(rxgo.CreateItem(nil))
    }
})

// 重试2次（总共3次尝试）
retried := retryableOperation.Retry(2)

retried.SubscribeWithCallbacks(
    func(value interface{}) {
        fmt.Printf("最终结果: %v\n", value)
    },
    func(err error) {
        fmt.Printf("最终失败: %v\n", err)
    },
    func() {
        fmt.Println("重试完成")
    },
)
```

### RetryWhen - 条件重试

```go
errorObservable := rxgo.Create(func(observer rxgo.Observer) {
    observer(rxgo.CreateErrorItem(errors.New("临时错误")))
})

// 使用指数退避重试
retried := errorObservable.RetryWhen(func(errors rxgo.Observable) rxgo.Observable {
    return errors.
        Take(3). // 最多重试3次
        FlatMap(func(err interface{}) rxgo.Observable {
            delay := time.Duration(1<<uint(0)) * time.Second // 指数退避
            fmt.Printf("重试前等待 %v\n", delay)
            return rxgo.Timer(delay)
        })
})
```

### OnErrorReturn - 错误时返回默认值

```go
errorObservable := rxgo.Create(func(observer rxgo.Observer) {
    observer(rxgo.CreateItem(1))
    observer(rxgo.CreateErrorItem(errors.New("错误")))
})

// 错误时返回默认值
withDefault := errorObservable.OnErrorReturn("默认值")

withDefault.SubscribeWithCallbacks(
    func(value interface{}) {
        fmt.Printf("值: %v\n", value)
    },
    func(err error) {
        fmt.Printf("错误: %v\n", err) // 不会被调用
    },
    func() {
        fmt.Println("完成")
    },
)
// 输出: 1, "默认值", 完成
```

### 复合错误处理策略

```go
// 复杂的错误处理管道
resilientObservable := rxgo.Create(func(observer rxgo.Observer) {
    // 模拟不稳定的网络请求
    if rand.Float32() < 0.7 {
        observer(rxgo.CreateErrorItem(errors.New("网络不稳定")))
    } else {
        observer(rxgo.CreateItem("数据"))
        observer(rxgo.CreateItem(nil))
    }
}).
    Retry(3).                                    // 重试3次
    Catch(func(err error) rxgo.Observable {     // 重试失败后的兜底
        return rxgo.Just("缓存数据")
    }).
    Timeout(5 * time.Second).                  // 总体超时控制
    OnErrorReturn("最终兜底数据")                 // 最后的保障
```

## 背压处理

背压是指生产者产生数据的速度超过消费者处理数据的速度时的处理机制。

### Flowable基础

```go
// 创建大量数据的Flowable
producer := rxgo.FlowableCreate(func(subscriber rxgo.Subscriber) {
    // 模拟快速生产数据
    for i := 0; i < 1000000; i++ {
        subscriber.OnNext(rxgo.CreateItem(i))
    }
    subscriber.OnComplete()
})

// 慢速消费者
subscription := producer.SubscribeWithCallbacks(
    func(value interface{}) {
        // 模拟慢速处理
        time.Sleep(1 * time.Millisecond)
        fmt.Printf("处理: %v\n", value)
    },
    func(err error) {
        fmt.Printf("错误: %v\n", err)
    },
    func() {
        fmt.Println("完成")
    },
)

// 控制请求速率
subscription.Request(10) // 只请求10个元素
```

### 背压策略

#### Buffer - 缓冲策略

```go
flowable := rxgo.FlowableRange(1, 1000)

// 无限制缓冲（谨慎使用）
buffered := flowable.OnBackpressureBuffer()

// 有容量限制的缓冲
limitedBuffer := flowable.OnBackpressureBufferWithCapacity(100)

subscription := buffered.SubscribeWithCallbacks(
    func(value interface{}) {
        time.Sleep(10 * time.Millisecond) // 慢速处理
        fmt.Printf("处理: %v\n", value)
    },
    func(err error) {
        fmt.Printf("背压错误: %v\n", err)
    },
    func() {
        fmt.Println("处理完成")
    },
)

subscription.Request(int64(^uint64(0) >> 1)) // 请求所有
```

#### Drop - 丢弃策略

```go
flowable := rxgo.FlowableRange(1, 1000)

// 丢弃无法处理的数据
dropped := flowable.OnBackpressureDrop()

subscription := dropped.SubscribeWithCallbacks(
    func(value interface{}) {
        time.Sleep(10 * time.Millisecond) // 慢速处理
        fmt.Printf("处理: %v\n", value)
    },
    func(err error) {
        fmt.Printf("错误: %v\n", err)
    },
    func() {
        fmt.Println("处理完成")
    },
)

subscription.Request(10) // 只能处理10个
```

#### Latest - 保留最新策略

```go
flowable := rxgo.FlowableRange(1, 1000)

// 只保留最新的数据
latest := flowable.OnBackpressureLatest()

subscription := latest.SubscribeWithCallbacks(
    func(value interface{}) {
        time.Sleep(10 * time.Millisecond)
        fmt.Printf("处理最新: %v\n", value)
    },
    func(err error) {
        fmt.Printf("错误: %v\n", err)
    },
    func() {
        fmt.Println("处理完成")
    },
)

subscription.Request(1) // 每次只请求1个
```

### Flowable与Observable转换

```go
// Flowable转Observable（失去背压控制）
flowable := rxgo.FlowableRange(1, 100)
observable := flowable.ToObservable()

// Observable无法直接转为Flowable，需要重新创建
observableToFlowable := rxgo.FlowableFromObservable(observable)
```

## 并行处理

### ParallelFlowable基础

```go
// 创建并行Flowable
parallel := rxgo.ParallelRange(1, 1000, runtime.NumCPU())

// 并行Map操作
parallelMapped := parallel.Map(func(value interface{}) (interface{}, error) {
    // CPU密集型计算
    result := complexCalculation(value.(int))
    return result, nil
})

// 并行Filter操作
parallelFiltered := parallelMapped.Filter(func(value interface{}) bool {
    return value.(int) > 100
})

// 合并结果
sequential := parallelFiltered.Sequential()

sequential.SubscribeWithCallbacks(
    func(value interface{}) {
        fmt.Printf("结果: %v\n", value)
    },
    func(err error) {
        fmt.Printf("错误: %v\n", err)
    },
    func() {
        fmt.Println("并行处理完成")
    },
)
```

### 并行Reduce

```go
// 大数据集并行求和
parallel := rxgo.ParallelRange(1, 1000000, runtime.NumCPU())

// 每个分区独立计算
partialSums := parallel.Reduce(func(acc, curr interface{}) interface{} {
    return acc.(int) + curr.(int)
})

// 获取最终结果
result, err := partialSums.BlockingGet()
if err != nil {
    fmt.Printf("错误: %v\n", err)
} else {
    fmt.Printf("并行求和结果: %v\n", result)
}
```

### 自定义并行度

```go
// 根据任务特性调整并行度
cpuIntensiveTasks := rxgo.ParallelFromFlowable(
    rxgo.FlowableRange(1, 10000),
    runtime.NumCPU(), // CPU密集型：使用CPU核心数
)

ioIntensiveTasks := rxgo.ParallelFromFlowable(
    rxgo.FlowableRange(1, 10000),
    runtime.NumCPU() * 2, // IO密集型：使用更多线程
)

networkTasks := rxgo.ParallelFromFlowable(
    rxgo.FlowableRange(1, 10000),
    100, // 网络任务：高并发
)
```

### 工作窃取并行处理

```go
// 使用工作窃取算法的并行处理
enhanced := rxgo.ParallelRange(1, 100000, runtime.NumCPU()).
    WithWorkStealing(). // 启用工作窃取
    Map(func(value interface{}) (interface{}, error) {
        // 不均匀的工作负载
        if value.(int)%10 == 0 {
            time.Sleep(1 * time.Millisecond) // 某些任务更耗时
        }
        return value.(int) * value.(int), nil
    })

// 获取工作窃取统计
stats := enhanced.GetStats()
for i, stat := range stats {
    fmt.Printf("Worker %d: 执行=%d, 窃取=%d, 被窃取=%d\n",
        i, stat.ExecutedTasks, stat.StolenTasks, stat.LostTasks)
}
```

## 性能优化

### Assembly-time优化

```go
// 标量优化 - Just操作符在编译时优化
scalar := rxgo.Just(42).Map(func(v interface{}) (interface{}, error) {
    return v.(int) * 2, nil // 编译时直接计算为84
})

// 空Observable优化
empty := rxgo.Empty().Map(transform) // 直接返回Empty，无运行时开销

// 融合优化
fused := rxgo.Range(1, 1000000).
    Filter(func(v interface{}) bool { return v.(int)%2 == 0 }).
    Map(func(v interface{}) (interface{}, error) { return v.(int)*2, nil }).
    Take(1000)
// 运行时会自动融合操作，减少中间分配
```

### 内存管理

```go
// 使用对象池
item := rxgo.GetPooledItem()
item.Value = data
defer rxgo.ReturnPooledItem(item)

// 获取性能统计
stats := rxgo.GetPerformanceStats()
fmt.Printf("内存效率: %.1f%%\n", stats.MemoryEfficiency)
fmt.Printf("GC压力减少: %d%%\n", stats.GCReduction)
```

### 缓冲区大小优化

```go
// 根据场景调整缓冲区大小
config := rxgo.DefaultConfig()

// 高吞吐量场景
config.BufferSize = 1024

// 低延迟场景
config.BufferSize = 1

// 内存受限场景
config.BufferSize = 16

observable := rxgo.Just(1, 2, 3).WithConfig(config)
```

## 最佳实践

### 1. 选择合适的类型

```go
// 单个异步值 -> Single
userProfile := fetchUserProfile(userID) // returns Single<UserProfile>

// 可选值 -> Maybe
cachedData := getCachedData(key) // returns Maybe<CachedData>

// 无返回值的异步操作 -> Completable
saveOperation := saveToDatabase(data) // returns Completable

// 事件流 -> Observable
userClicks := observeUserClicks() // returns Observable<ClickEvent>

// 大数据流 -> Flowable
dataStream := processLargeDataset() // returns Flowable<DataChunk>

// 并行处理 -> ParallelFlowable
parallelProcessing := processInParallel(data) // returns ParallelFlowable<Result>
```

### 2. 正确使用调度器

```go
// IO操作
dbQuery.SubscribeOn(rxgo.NewThreadScheduler)

// CPU密集型
heavyComputation.SubscribeOn(rxgo.ThreadPoolScheduler)

// 简单操作
simpleTransform.SubscribeOn(rxgo.ImmediateScheduler)

// UI更新（如果有UI线程）
result.ObserveOn(rxgo.MainScheduler) // 假设的UI调度器
```

### 3. 生命周期管理

```go
type Service struct {
    subscriptions *rxgo.CompositeDisposable
}

func NewService() *Service {
    return &Service{
        subscriptions: rxgo.NewCompositeDisposable(),
    }
}

func (s *Service) Start() {
    // 添加订阅到组合disposable
    sub := rxgo.Interval(1 * time.Second).
        Subscribe(func(item rxgo.Item) {
            // 处理定时任务
        })
    
    s.subscriptions.Add(rxgo.NewBaseDisposable(func() {
        sub.Unsubscribe()
    }))
}

func (s *Service) Stop() {
    // 一次性清理所有订阅
    s.subscriptions.Dispose()
}
```

### 4. 错误处理策略

```go
// 分层错误处理
result := apiCall().
    Retry(3).                    // 网络层重试
    Catch(func(err error) rxgo.Observable {
        return fallbackApiCall()  // 服务层降级
    }).
    OnErrorReturn(defaultValue). // 应用层兜底
    Timeout(30 * time.Second)   // 整体超时控制
```

### 5. 背压处理

```go
// 根据下游处理能力选择策略
fastProducer := rxgo.FlowableRange(1, 1000000)

// 内存充足 + 不能丢数据 -> Buffer
critical := fastProducer.OnBackpressureBuffer()

// 实时性要求高 -> Drop
realtime := fastProducer.OnBackpressureDrop()

// 只关心最新状态 -> Latest
status := fastProducer.OnBackpressureLatest()
```

### 6. 测试

```go
func TestObservableChain(t *testing.T) {
    // 使用测试调度器
    testScheduler := rxgo.NewTestScheduler()
    
    // 创建测试Observable
    source := rxgo.Just(1, 2, 3).SubscribeOn(testScheduler)
    
    // 收集结果
    var results []interface{}
    source.SubscribeWithCallbacks(
        func(value interface{}) {
            results = append(results, value)
        },
        func(err error) {
            t.Errorf("Unexpected error: %v", err)
        },
        func() {
            // 验证结果
            expected := []interface{}{1, 2, 3}
            if !reflect.DeepEqual(results, expected) {
                t.Errorf("Expected %v, got %v", expected, results)
            }
        },
    )
    
    // 推进虚拟时间
    testScheduler.AdvanceTimeBy(1 * time.Second)
}
```

### 7. 避免常见陷阱

```go
// ❌ 忘记取消订阅（内存泄漏）
observable.Subscribe(observer) // 可能造成泄漏

// ✅ 管理订阅生命周期
subscription := observable.Subscribe(observer)
defer subscription.Unsubscribe()

// ❌ 在错误的调度器上执行
ioOperation.SubscribeOn(rxgo.ImmediateScheduler) // 阻塞当前线程

// ✅ 选择合适的调度器
ioOperation.SubscribeOn(rxgo.NewThreadScheduler)

// ❌ 忽略背压
rxgo.Range(1, 1000000).Subscribe(slowObserver) // 可能内存溢出

// ✅ 使用Flowable和背压控制
rxgo.FlowableRange(1, 1000000).
    OnBackpressureBuffer().
    SubscribeWithCallbacks(slowObserver)

// ❌ 不处理错误
observable.SubscribeWithCallbacks(onNext, nil, onComplete) // 错误被忽略

// ✅ 完整的错误处理
observable.SubscribeWithCallbacks(onNext, onError, onComplete)
```

## 常见问题

### Q: 何时使用Observable vs Flowable？

**A:** 
- **Observable**: 适用于UI事件、少量数据、低频率的事件流
- **Flowable**: 适用于大数据处理、高频事件、需要背压控制的场景

```go
// UI事件 -> Observable
buttonClicks := rxgo.Create(func(observer rxgo.Observer) {
    // 监听按钮点击
})

// 大数据流 -> Flowable
dataProcessing := rxgo.FlowableCreate(func(subscriber rxgo.Subscriber) {
    // 处理大型数据集
})
```

### Q: Subject与Observable的区别？

**A:**
- **Observable**: 冷流，每个订阅者独立接收完整序列
- **Subject**: 热流，多个订阅者共享同一数据源

```go
// Cold Observable - 每个订阅者都从头开始
cold := rxgo.Just(1, 2, 3)
cold.Subscribe(observer1) // 收到: 1, 2, 3
cold.Subscribe(observer2) // 收到: 1, 2, 3

// Hot Subject - 共享数据流
hot := rxgo.NewPublishSubject()
hot.Subscribe(observer1)
hot.OnNext(1) // observer1收到: 1
hot.Subscribe(observer2)
hot.OnNext(2) // 两个观察者都收到: 2
```

### Q: 如何处理长时间运行的操作？

**A:** 使用适当的调度器和超时控制：

```go
longRunningOp := rxgo.Create(func(observer rxgo.Observer) {
    // 长时间运行的操作
    result := heavyComputation()
    observer(rxgo.CreateItem(result))
    observer(rxgo.CreateItem(nil))
}).
    SubscribeOn(rxgo.NewThreadScheduler). // 异步执行
    Timeout(30 * time.Second).            // 超时控制
    Catch(func(err error) rxgo.Observable {
        // 错误处理
        return rxgo.Just("默认结果")
    })
```

### Q: 如何取消正在进行的操作？

**A:** 使用Context或Subscription：

```go
// 方法1: 使用Context
ctx, cancel := context.WithCancel(context.Background())
observable := rxgo.Create(func(observer rxgo.Observer) {
    for i := 0; i < 1000; i++ {
        select {
        case <-ctx.Done():
            return // 取消操作
        default:
            observer(rxgo.CreateItem(i))
            time.Sleep(10 * time.Millisecond)
        }
    }
    observer(rxgo.CreateItem(nil))
})

subscription := observable.Subscribe(observer)

// 5秒后取消
time.AfterFunc(5*time.Second, cancel)

// 方法2: 直接取消订阅
subscription.Unsubscribe()
```

### Q: 内存使用过高怎么办？

**A:** 
1. 使用背压控制
2. 调整缓冲区大小
3. 及时取消订阅
4. 使用对象池

```go
// 1. 背压控制
flowable.OnBackpressureLatest() // 只保留最新

// 2. 小缓冲区
config := rxgo.DefaultConfig()
config.BufferSize = 16
observable.WithConfig(config)

// 3. 自动取消
subscription := observable.Subscribe(observer)
time.AfterFunc(30*time.Second, func() {
    subscription.Unsubscribe()
})

// 4. 对象池
item := rxgo.GetPooledItem()
defer rxgo.ReturnPooledItem(item)
```

---

这份开发手册涵盖了RxGo的所有核心功能和最佳实践。建议根据具体需求选择合适的模式和操作符，并始终注意资源管理和错误处理。 