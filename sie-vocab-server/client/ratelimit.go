package client

import (
	"context"
	"math"
	"sync"
	"time"
)

// TokenBucket 令牌桶限流器（标准库实现，无外部依赖）
// 同时控制请求频率（RPM）和并发数（maxConcurrent）
type TokenBucket struct {
	mu         sync.Mutex
	rate       float64 // 每秒生成的 token 数 (RPM / 60)
	burst      float64 // 最大 burst = RPM
	tokens     float64 // 当前可用 token 数
	lastRefill time.Time

	// 并发控制信号量
	sem chan struct{}
}

// NewTokenBucket 创建令牌桶
// rpm: 每分钟允许的请求数
// maxConcurrent: 最大并发请求数
func NewTokenBucket(rpm, maxConcurrent int) *TokenBucket {
	if rpm <= 0 {
		rpm = 10
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}
	return &TokenBucket{
		rate:       float64(rpm) / 60.0,
		burst:      float64(rpm),
		tokens:     float64(rpm), // 初始满桶，允许 burst
		lastRefill: time.Now(),
		sem:        make(chan struct{}, maxConcurrent),
	}
}

// Wait 等待获取令牌和并发槽位。ctx 可用于超时控制。
// 调用方必须在完成后调用 Done() 释放并发槽位。
func (tb *TokenBucket) Wait(ctx context.Context) error {
	// Step 1: 获取并发信号量（阻塞直到有空位）
	select {
	case tb.sem <- struct{}{}:
		// acquired
	case <-ctx.Done():
		return ctx.Err()
	}

	// Step 2: 获取令牌
	tb.mu.Lock()
	tb.refillLocked()

	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		tb.mu.Unlock()
		return nil
	}

	// 计算需要等待的时间
	needed := 1.0 - tb.tokens
	waitDuration := time.Duration(needed/tb.rate*float64(time.Second)) + time.Millisecond*50
	tb.mu.Unlock()

	timer := time.NewTimer(waitDuration)
	select {
	case <-timer.C:
		// 时间到，重新获取令牌
		tb.mu.Lock()
		tb.refillLocked()
		tb.tokens -= 1.0
		if tb.tokens < 0 {
			tb.tokens = 0
		}
		tb.mu.Unlock()
		return nil
	case <-ctx.Done():
		timer.Stop()
		// 释放并发槽位
		<-tb.sem
		return ctx.Err()
	}
}

// Done 释放并发槽位。必须在每次 Wait 成功后调用。
func (tb *TokenBucket) Done() {
	<-tb.sem
}

// refillLocked 根据经过的时间补充令牌（调用方必须持有 mu）
func (tb *TokenBucket) refillLocked() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens = math.Min(tb.burst, tb.tokens+elapsed*tb.rate)
	tb.lastRefill = now
}
