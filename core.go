// Package rxgo provides reactive programming primitives for Go
// 基于Go语言特性的响应式编程库版本，专注于异步和事件驱动的程序构建
package rxgo

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// 核心类型定义
// ============================================================================

// Item 表示流中的一个数据项，包含值或错误
type Item struct {
	Value interface{} // 数据值
	Error error       // 错误信息
}

// IsError 检查项目是否包含错误
func (item Item) IsError() bool {
	return item.Error != nil
}

// GetValue 获取项目的值，如果是错误则返回nil
func (item Item) GetValue() interface{} {
	if item.IsError() {
		return nil
	}
	return item.Value
}

// ============================================================================
// 函数类型定义
// ============================================================================

// Observer 观察者函数类型
type Observer func(item Item)

// OnNext 处理下一个值的函数
type OnNext func(value interface{})

// OnError 处理错误的函数
type OnError func(err error)

// OnComplete 处理完成的函数
type OnComplete func()

// Predicate 谓词函数，用于过滤
type Predicate func(value interface{}) bool

// Transformer 转换函数，用于映射
type Transformer func(value interface{}) (interface{}, error)

// Reducer 归约函数，用于聚合
type Reducer func(accumulator, current interface{}) interface{}

// ============================================================================
// 生命周期管理
// ============================================================================

// Subscription 订阅接口，管理订阅的生命周期
type Subscription interface {
	// Unsubscribe 取消订阅
	Unsubscribe()
	// IsUnsubscribed 检查是否已取消订阅
	IsUnsubscribed() bool
}

// Disposable 可释放资源的接口
type Disposable interface {
	// Dispose 释放资源
	Dispose()
	// IsDisposed 检查是否已释放
	IsDisposed() bool
}

// CompositeDisposable 组合式资源管理器
type CompositeDisposable struct {
	mu        sync.RWMutex
	disposed  bool
	resources []Disposable
}

// NewCompositeDisposable 创建组合式资源管理器
func NewCompositeDisposable() *CompositeDisposable {
	return &CompositeDisposable{
		resources: make([]Disposable, 0),
	}
}

// Add 添加可释放资源
func (cd *CompositeDisposable) Add(disposable Disposable) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	if cd.disposed {
		disposable.Dispose()
		return
	}

	cd.resources = append(cd.resources, disposable)
}

// Dispose 释放所有资源
func (cd *CompositeDisposable) Dispose() {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	if cd.disposed {
		return
	}

	cd.disposed = true
	for _, resource := range cd.resources {
		resource.Dispose()
	}
	cd.resources = nil
}

// IsDisposed 检查是否已释放
func (cd *CompositeDisposable) IsDisposed() bool {
	cd.mu.RLock()
	defer cd.mu.RUnlock()
	return cd.disposed
}

// ============================================================================
// 调度器接口
// ============================================================================

// Scheduler 调度器接口，控制任务执行时机和方式
type Scheduler interface {
	// Schedule 调度一个任务
	Schedule(action func()) Disposable
	// ScheduleWithDelay 延迟调度一个任务
	ScheduleWithDelay(action func(), delay time.Duration) Disposable
	// ScheduleWithContext 带上下文的调度
	ScheduleWithContext(ctx context.Context, action func()) Disposable
}

// ============================================================================
// Observable 核心接口
// ============================================================================

// Observable 可观察序列的核心接口
type Observable interface {
	// Subscribe 订阅观察者
	Subscribe(observer Observer) Subscription

	// SubscribeWithCallbacks 使用回调函数订阅
	SubscribeWithCallbacks(onNext OnNext, onError OnError, onComplete OnComplete) Subscription

	// SubscribeOn 指定订阅时使用的调度器
	SubscribeOn(scheduler Scheduler) Observable

	// ObserveOn 指定观察时使用的调度器
	ObserveOn(scheduler Scheduler) Observable

	// 转换操作符
	Map(transformer Transformer) Observable
	FlatMap(transformer func(interface{}) Observable) Observable
	Filter(predicate Predicate) Observable
	Take(count int) Observable
	TakeLast(count int) Observable
	TakeUntil(other Observable) Observable
	TakeWhile(predicate Predicate) Observable
	Skip(count int) Observable
	SkipLast(count int) Observable
	SkipUntil(other Observable) Observable
	SkipWhile(predicate Predicate) Observable
	Distinct() Observable
	DistinctUntilChanged() Observable
	Buffer(count int) Observable
	BufferWithTime(timespan time.Duration) Observable
	Window(count int) Observable
	WindowWithTime(timespan time.Duration) Observable

	// 组合操作符
	Merge(other Observable) Observable
	Concat(other Observable) Observable
	Zip(other Observable, zipper func(interface{}, interface{}) interface{}) Observable
	CombineLatest(other Observable, combiner func(interface{}, interface{}) interface{}) Observable

	// 聚合操作符
	Reduce(reducer Reducer) Single
	Scan(reducer Reducer) Observable
	Count() Single
	First() Single
	Last() Single
	Min() Single
	Max() Single
	Sum() Single
	Average() Single
	All(predicate Predicate) Single
	Any(predicate Predicate) Single
	Contains(value interface{}) Single
	ElementAt(index int) Single

	// 时间操作符
	Debounce(duration time.Duration) Observable
	Throttle(duration time.Duration) Observable
	Delay(duration time.Duration) Observable
	Timeout(duration time.Duration) Observable

	// 错误处理
	Catch(handler func(error) Observable) Observable
	Retry(count int) Observable
	RetryWhen(handler func(Observable) Observable) Observable

	// 副作用操作符
	DoOnNext(action OnNext) Observable
	DoOnError(action OnError) Observable
	DoOnComplete(action OnComplete) Observable

	// 转换为其他类型
	ToSlice() Single
	ToChannel() <-chan Item

	// 多播支持
	Publish() ConnectableObservable
	Share() Observable

	// 阻塞操作
	BlockingSubscribe(observer Observer)
	BlockingFirst() (interface{}, error)
	BlockingLast() (interface{}, error)

	// Phase 3 新增操作符
	StartWith(values ...interface{}) Observable
	StartWithIterable(iterable func() <-chan interface{}) Observable
	Timestamp() Observable
	TimeInterval() Observable
	Cast(targetType reflect.Type) Observable
	OfType(targetType reflect.Type) Observable
	WithLatestFrom(other Observable, combiner func(interface{}, interface{}) interface{}) Observable
	DoFinally(action func()) Observable
	DoAfterTerminate(action func()) Observable
	Cache() Observable
	CacheWithCapacity(capacity int) Observable
	Amb(other Observable) Observable

	// 高级组合操作符 - 对标RxJava
	Join(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, interface{}) interface{}) Observable
	GroupJoin(other Observable, leftWindowSelector func(interface{}) time.Duration, rightWindowSelector func(interface{}) time.Duration, resultSelector func(interface{}, Observable) interface{}) Observable
	SwitchOnNext() Observable
}

// ============================================================================
// ConnectableObservable 可连接的Observable
// ============================================================================

// ConnectableObservable 可连接的Observable接口，支持多播
type ConnectableObservable interface {
	Observable

	// Connect 开始发射数据给订阅者
	Connect() Disposable

	// ConnectWithContext 带上下文的连接
	ConnectWithContext(ctx context.Context) Disposable

	// RefCount 返回一个自动连接/断开的Observable
	RefCount() Observable

	// AutoConnect 当有指定数量的订阅者时自动连接
	AutoConnect(subscriberCount int) Observable

	// IsConnected 检查是否已连接
	IsConnected() bool
}

// ============================================================================
// Subject 主题接口
// ============================================================================

// Subject 既是Observable又是Observer的接口
type Subject interface {
	Observable

	// 作为Observer的方法
	AsObserver() Observer

	// OnNext 发送下一个值
	OnNext(value interface{})

	// OnError 发送错误
	OnError(err error)

	// OnComplete 发送完成信号
	OnComplete()

	// HasObservers 检查是否有观察者
	HasObservers() bool

	// ObserverCount 获取观察者数量
	ObserverCount() int
}

// ============================================================================
// Single 单值Observable
// ============================================================================

// Single 发射单个值的Observable
type Single interface {
	// Subscribe 订阅单值观察者
	Subscribe(onSuccess func(interface{}), onError OnError) Subscription

	// Map 转换操作符
	Map(transformer Transformer) Single

	// FlatMap 平铺映射
	FlatMap(transformer func(interface{}) Single) Single

	// Filter 过滤（可能返回空）
	Filter(predicate Predicate) Maybe

	// 错误处理
	Catch(handler func(error) Single) Single
	Retry(count int) Single

	// 转换
	ToObservable() Observable

	// 阻塞操作
	BlockingGet() (interface{}, error)
}

// ============================================================================
// Maybe 可能为空的单值Observable
// ============================================================================

// Maybe 可能发射0个或1个值的Observable
type Maybe interface {
	// Subscribe 订阅Maybe观察者
	Subscribe(onSuccess func(interface{}), onError OnError, onComplete OnComplete) Subscription

	// Map 转换操作符
	Map(transformer Transformer) Maybe

	// FlatMap 平铺映射
	FlatMap(transformer func(interface{}) Maybe) Maybe

	// Filter 过滤
	Filter(predicate Predicate) Maybe

	// DefaultIfEmpty 如果为空则使用默认值
	DefaultIfEmpty(defaultValue interface{}) Single

	// 错误处理
	Catch(handler func(error) Maybe) Maybe

	// 转换
	ToObservable() Observable
	ToSingle() Single

	// 阻塞操作
	BlockingGet() (interface{}, error)
}

// ============================================================================
// Completable 表示无返回值的异步操作
// ============================================================================

// Completable 表示一个无返回值的异步操作，只会发射完成或错误信号
type Completable interface {
	// Subscribe 订阅完成和错误回调
	Subscribe(onComplete OnComplete, onError OnError) Subscription

	// SubscribeWithCallbacks 订阅（为了兼容性）
	SubscribeWithCallbacks(onComplete OnComplete, onError OnError) Subscription

	// SubscribeOn 指定订阅时使用的调度器
	SubscribeOn(scheduler Scheduler) Completable

	// ObserveOn 指定观察时使用的调度器
	ObserveOn(scheduler Scheduler) Completable

	// ============================================================================
	// 组合操作符
	// ============================================================================

	// AndThen 完成后执行另一个Completable
	AndThen(next Completable) Completable

	// AndThenObservable 完成后执行Observable
	AndThenObservable(next Observable) Observable

	// AndThenSingle 完成后执行Single
	AndThenSingle(next Single) Single

	// AndThenMaybe 完成后执行Maybe
	AndThenMaybe(next Maybe) Maybe

	// Concat 顺序连接多个Completable
	Concat(other Completable) Completable

	// Merge 合并多个Completable
	Merge(other Completable) Completable

	// ============================================================================
	// 错误处理
	// ============================================================================

	// Catch 捕获错误并返回新的Completable
	Catch(handler func(error) Completable) Completable

	// Retry 重试指定次数
	Retry(count int) Completable

	// RetryWhen 条件重试
	RetryWhen(handler func(error) bool) Completable

	// OnErrorResumeNext 错误时恢复执行另一个Completable
	OnErrorResumeNext(resumeWith Completable) Completable

	// OnErrorComplete 错误时作为完成处理
	OnErrorComplete() Completable

	// ============================================================================
	// 时间操作符
	// ============================================================================

	// Delay 延迟执行
	Delay(duration time.Duration) Completable

	// Timeout 超时处理
	Timeout(duration time.Duration) Completable

	// ============================================================================
	// 副作用操作符
	// ============================================================================

	// DoOnComplete 完成时执行副作用
	DoOnComplete(action OnComplete) Completable

	// DoOnError 错误时执行副作用
	DoOnError(action OnError) Completable

	// DoOnSubscribe 订阅时执行副作用
	DoOnSubscribe(action func()) Completable

	// DoOnDispose 释放时执行副作用
	DoOnDispose(action func()) Completable

	// ============================================================================
	// 转换操作符
	// ============================================================================

	// ToObservable 转换为Observable（发射完成信号）
	ToObservable() Observable

	// ToSingle 转换为Single（提供默认值）
	ToSingle(defaultValue interface{}) Single

	// ToMaybe 转换为Maybe（空完成）
	ToMaybe() Maybe

	// ============================================================================
	// 阻塞操作
	// ============================================================================

	// BlockingAwait 阻塞等待完成
	BlockingAwait() error

	// BlockingAwaitWithTimeout 阻塞等待完成（带超时）
	BlockingAwaitWithTimeout(timeout time.Duration) error

	// ============================================================================
	// 生命周期管理
	// ============================================================================

	// IsDisposed 检查是否已释放
	IsDisposed() bool

	// Dispose 释放资源
	Dispose()
}

// ============================================================================
// 工厂函数接口
// ============================================================================

// ObservableFactory Observable工厂接口
type ObservableFactory interface {
	// 创建操作符
	Create(emitter func(Observer)) Observable
	Just(values ...interface{}) Observable
	Empty() Observable
	Never() Observable
	Error(err error) Observable

	// 从数据源创建
	FromSlice(slice []interface{}) Observable
	FromChannel(ch <-chan interface{}) Observable
	FromIterable(iterable func() <-chan interface{}) Observable

	// 时间相关
	Interval(duration time.Duration) Observable
	Timer(duration time.Duration) Observable
	Range(start, count int) Observable

	// 组合操作符
	Merge(observables ...Observable) Observable
	Concat(observables ...Observable) Observable
	Zip(observables []Observable, zipper func(...interface{}) interface{}) Observable
	CombineLatest(observables []Observable, combiner func(...interface{}) interface{}) Observable
}

// ============================================================================
// 内部实现基础结构
// ============================================================================

// baseSubscription 基础订阅实现
type baseSubscription struct {
	unsubscribed int32
	disposable   Disposable
}

// NewBaseSubscription 创建基础订阅
func NewBaseSubscription(disposable Disposable) *baseSubscription {
	return &baseSubscription{
		disposable: disposable,
	}
}

// Unsubscribe 取消订阅
func (s *baseSubscription) Unsubscribe() {
	if atomic.CompareAndSwapInt32(&s.unsubscribed, 0, 1) {
		if s.disposable != nil {
			s.disposable.Dispose()
		}
	}
}

// IsUnsubscribed 检查是否已取消订阅
func (s *baseSubscription) IsUnsubscribed() bool {
	return atomic.LoadInt32(&s.unsubscribed) == 1
}

// baseDisposable 基础可释放资源实现
type baseDisposable struct {
	disposed int32
	action   func()
}

// NewBaseDisposable 创建基础可释放资源
func NewBaseDisposable(action func()) *baseDisposable {
	return &baseDisposable{
		action: action,
	}
}

// Dispose 释放资源
func (d *baseDisposable) Dispose() {
	if atomic.CompareAndSwapInt32(&d.disposed, 0, 1) {
		if d.action != nil {
			d.action()
		}
	}
}

// IsDisposed 检查是否已释放
func (d *baseDisposable) IsDisposed() bool {
	return atomic.LoadInt32(&d.disposed) == 1
}

// ============================================================================
// 工具函数
// ============================================================================

// CreateItem 创建包含值的项目
func CreateItem(value interface{}) Item {
	return Item{Value: value}
}

// CreateErrorItem 创建包含错误的项目
func CreateErrorItem(err error) Item {
	return Item{Error: err}
}

// SafeExecute 安全执行函数，捕获panic
func SafeExecute(action func()) (recovered interface{}) {
	defer func() {
		if r := recover(); r != nil {
			recovered = r
		}
	}()

	action()
	return nil
}

// WithTimeout 为上下文添加超时
func WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout)
}

// WithCancel 为上下文添加取消功能
func WithCancel(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(ctx)
}

// ============================================================================
// 配置选项
// ============================================================================

// Option 配置选项接口
type Option interface {
	Apply(config *Config)
}

// Config 配置结构
type Config struct {
	BufferSize           int
	Scheduler            Scheduler
	ErrorStrategy        ErrorStrategy
	BackpressureStrategy BackpressureStrategy
	Context              context.Context
}

// ErrorStrategy 错误处理策略
type ErrorStrategy int

const (
	// StopOnError 遇到错误时停止
	StopOnError ErrorStrategy = iota
	// ContinueOnError 遇到错误时继续
	ContinueOnError
)

// BackpressureStrategy 背压策略
type BackpressureStrategy int

const (
	// BufferStrategy 缓冲策略
	BufferStrategy BackpressureStrategy = iota
	// DropStrategy 丢弃策略
	DropStrategy
	// BlockStrategy 阻塞策略
	BlockStrategy
)

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		BufferSize:           16,
		ErrorStrategy:        StopOnError,
		BackpressureStrategy: BufferStrategy,
		Context:              context.Background(),
	}
}
