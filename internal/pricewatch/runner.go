package pricewatch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

const heartbeatKey = "heartbeat_date"

// Runner 是 `pricewatch run` 的常驻调度器（spec §9）。
// 每个 Tick 检查各资产是否到期，到期的在调度 goroutine 内依次执行 CheckAsset，然后检查心跳。
// 所有检测都在同一个 goroutine 里串行，因此同一资产不会并发 CheckAsset（TASK-007 的使用约束）。
// 同一个 Runner 不得并发调用 Run/RunOnce。
type Runner struct {
	Engine      *Engine
	Assets      []Asset
	HeartbeatAt string        // 本地时间 "HH:MM"；空表示不发心跳
	Tick        time.Duration // 到期检查的间隔

	check func(ctx context.Context, a Asset) error // 测试替身；nil 时用 Engine.CheckAsset
	due   []time.Time                              // 各资产下次到期时间，零值表示立即到期
}

// Run 立即跑第一轮，之后每个 Tick 跑一次 step；ctx 取消时返回 nil。
func (r *Runner) Run(ctx context.Context) error {
	t := time.NewTicker(r.Tick)
	defer t.Stop()
	for {
		r.step(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// RunOnce 对每个资产各执行一次 CheckAsset；某资产出错不影响其余资产，错误合并返回。不发心跳。
func (r *Runner) RunOnce(ctx context.Context) error {
	var errs []error
	for _, a := range r.Assets {
		if err := r.checkAsset(ctx, a); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", a.Symbol, err))
		}
	}
	return errors.Join(errs...)
}

// step 依次执行到期资产的检测，再检查心跳。到期时间按「上次到期 + Interval」推进，
// 不累积 tick 的抖动；落后超过一个 Interval（如检测本身很慢）时从当前时刻重新起算。
func (r *Runner) step(ctx context.Context) {
	if r.due == nil {
		r.due = make([]time.Time, len(r.Assets))
	}
	for i, a := range r.Assets {
		if ctx.Err() != nil {
			return
		}
		now := r.Engine.Now()
		if now.Before(r.due[i]) {
			continue
		}
		r.due[i] = r.due[i].Add(a.Interval)
		if !r.due[i].After(now) {
			r.due[i] = now.Add(a.Interval)
		}
		if err := r.checkAsset(ctx, a); err != nil {
			r.Engine.logger().Warn("pricewatch check failed", zap.String("symbol", a.Symbol), zap.Error(err))
		}
	}
	if ctx.Err() == nil {
		r.heartbeat()
	}
}

func (r *Runner) checkAsset(ctx context.Context, a Asset) error {
	if r.check != nil {
		return r.check(ctx, a)
	}
	return r.Engine.CheckAsset(ctx, a)
}

// heartbeat 到点发送每日心跳；发送成功后才写 heartbeat_date，失败则下一 tick 重试。
func (r *Runner) heartbeat() {
	if r.HeartbeatAt == "" {
		return
	}
	e := r.Engine
	now := e.Now()
	last, err := e.Store.GetKV(heartbeatKey)
	if err != nil {
		e.logger().Warn("pricewatch heartbeat: read date failed", zap.Error(err))
		return
	}
	if !dueHeartbeat(now, r.HeartbeatAt, last) {
		return
	}
	if err := e.Sender.SendText(FormatHeartbeat(now, e.Snapshot(), e.Sent24h())); err != nil {
		e.logger().Warn("pricewatch heartbeat: send failed", zap.Error(err))
		return
	}
	if err := e.Store.PutKV(heartbeatKey, now.Format(time.DateOnly)); err != nil {
		e.logger().Warn("pricewatch heartbeat: save date failed", zap.Error(err))
	}
}

// dueHeartbeat：按 now 自身的时区（运行时为本地时区）判断当天是否已过 at 且今天还没发过。
// 睡眠错过 at 的，醒来后当天补发一次。at 为空或格式不对时恒为 false。
func dueHeartbeat(now time.Time, at string, lastDate string) bool {
	if at == "" {
		return false
	}
	hm, err := time.Parse("15:04", at)
	if err != nil {
		return false
	}
	if lastDate == now.Format(time.DateOnly) {
		return false
	}
	y, m, d := now.Date()
	return !now.Before(time.Date(y, m, d, hm.Hour(), hm.Minute(), 0, 0, now.Location()))
}
