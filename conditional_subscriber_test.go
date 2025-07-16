package rxgo

import (
	"testing"
)

// ============================================================================
// ConditionalSubscriber基础功能测试
// ============================================================================

func TestConditionalSubscriber_Basic(t *testing.T) {
	// 测试基本的条件订阅者功能
	received := make([]interface{}, 0)
	var lastError error
	completed := false

	subscriber := NewConditionalSubscriber(
		func(item Item) bool {
			if item.IsError() {
				return false
			}
			received = append(received, item.Value)
			return true
		},
		func(err error) {
			lastError = err
		},
		func() {
			completed = true
		},
	)

	// 测试TryOnNext
	result := subscriber.TryOnNext(1)
	if !result {
		t.Error("TryOnNext应该返回true")
	}

	result = subscriber.TryOnNext(2)
	if !result {
		t.Error("TryOnNext应该返回true")
	}

	// 测试OnComplete
	subscriber.OnComplete()

	// 验证结果
	if len(received) != 2 {
		t.Errorf("期望接收2个值，实际接收%d个", len(received))
	}

	if received[0] != 1 || received[1] != 2 {
		t.Errorf("接收的值不正确: %v", received)
	}

	if !completed {
		t.Error("应该标记为完成")
	}

	if lastError != nil {
		t.Errorf("不应该有错误: %v", lastError)
	}
}

func TestConditionalSubscriber_WithFiltering(t *testing.T) {
	// 测试带过滤功能的条件订阅者
	received := make([]interface{}, 0)

	subscriber := NewConditionalSubscriber(
		func(item Item) bool {
			if item.IsError() {
				return false
			}
			// 只接受偶数
			if val, ok := item.Value.(int); ok && val%2 == 0 {
				received = append(received, item.Value)
				return true
			}
			return false // 拒绝奇数
		},
		nil,
		nil,
	)

	// 发送数据
	subscriber.TryOnNext(1) // 被拒绝
	subscriber.TryOnNext(2) // 被接受
	subscriber.TryOnNext(3) // 被拒绝
	subscriber.TryOnNext(4) // 被接受

	// 验证结果
	if len(received) != 2 {
		t.Errorf("期望接收2个值，实际接收%d个", len(received))
	}

	if received[0] != 2 || received[1] != 4 {
		t.Errorf("接收的值不正确: %v", received)
	}
}

// ============================================================================
// Observer适配器测试
// ============================================================================

func TestObserverToConditionalAdapter(t *testing.T) {
	received := make([]interface{}, 0)

	observer := func(item Item) {
		if !item.IsError() && item.Value != nil {
			received = append(received, item.Value)
		}
	}

	adapter := NewObserverToConditionalAdapter(observer)

	// 测试TryOnNext
	result := adapter.TryOnNext(1)
	if !result {
		t.Error("适配器应该总是返回true")
	}

	result = adapter.TryOnNext(2)
	if !result {
		t.Error("适配器应该总是返回true")
	}

	// 验证结果
	if len(received) != 2 {
		t.Errorf("期望接收2个值，实际接收%d个", len(received))
	}

	if received[0] != 1 || received[1] != 2 {
		t.Errorf("接收的值不正确: %v", received)
	}
}

// ============================================================================
// 微融合统计测试
// ============================================================================

func TestMicroFusionStats(t *testing.T) {
	// 重置统计
	ResetMicroFusionStats()

	// 检查初始状态
	stats := GetMicroFusionStats()
	if stats.TotalAttempts != 0 {
		t.Error("初始尝试次数应该为0")
	}
	if stats.SuccessfulFusions != 0 {
		t.Error("初始成功融合次数应该为0")
	}

	// 增加一些统计
	IncrementFusionAttempt()
	IncrementSuccessfulFusion()
	IncrementFilterOptimized()
	IncrementMapOptimized()
	IncrementFallback()

	// 检查更新后的统计
	stats = GetMicroFusionStats()
	if stats.TotalAttempts != 1 {
		t.Errorf("期望尝试次数为1，实际为%d", stats.TotalAttempts)
	}
	if stats.SuccessfulFusions != 1 {
		t.Errorf("期望成功融合次数为1，实际为%d", stats.SuccessfulFusions)
	}
	if stats.FilterOptimized != 1 {
		t.Errorf("期望Filter优化次数为1，实际为%d", stats.FilterOptimized)
	}
	if stats.MapOptimized != 1 {
		t.Errorf("期望Map优化次数为1，实际为%d", stats.MapOptimized)
	}
	if stats.FallbackCount != 1 {
		t.Errorf("期望回退次数为1，实际为%d", stats.FallbackCount)
	}
}
