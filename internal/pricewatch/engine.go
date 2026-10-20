package pricewatch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/newthinker/atlas/internal/collector/crypto"
	"github.com/newthinker/atlas/internal/core"
)

// maxGapRatio 是规则可计算的最大缺口比例；超过即本轮跳过该规则（spec §6）。
const maxGapRatio = 0.10

// Sender 发送纯文本消息；*telegram.Telegram 天然满足。
type Sender interface{ SendText(text string) error }

// Engine 执行单资产一轮检测（spec §5、§6、§7、§9）。
//
// 健康状态、各资产快照与发送记录只放内存，进程重启后清空（AD-15）。
// 不同资产可以并发调用 CheckAsset；同一资产不应并发。
type Engine struct {
	Cfg       *Config
	Store     *Store
	Syncer    *Syncer // nil 表示不同步 K 线
	Providers []crypto.Provider
	Advisor   *Advisor // nil 表示 AI 关闭
	Sender    Sender
	Now       func() time.Time
	Log       *zap.Logger
	DryRun    bool      // 消息与健康告警写到 Out，不发送，也不写规则状态（K 线照常入库）
	Out       io.Writer // DryRun 时的输出

	healthOnce sync.Once
	health     HealthTracker

	mu    sync.Mutex
	snaps map[string]assetSnap // 按 Symbol
	sent  []time.Time          // 提醒的发送时间，供 Sent24h
}

type assetSnap struct {
	price  float64
	lastOK time.Time
	active []string
}

// pendingSend 是本轮需要发送的一条规则；发送成功后才写入 next。
type pendingSend struct {
	rule    Rule
	next    RuleState
	trigger Trigger
}

// CheckAsset 按以下顺序执行一轮：
//  1. 按 Cfg.Assets.Crypto.Sources 顺序取价，价格须 > 0 且有限；全部失败则返回 error；
//  2. 同步 1d、5m K 线，出错只记日志；
//  3. 每条规则算 WindowStatsFor，GapRatio > 0.10 跳过且状态不变；否则 Measure → Tier → Decide；
//  4. 不需要发送的状态变化（含清零）立即落盘；
//  5. 有需要发送的规则时算 Overview、调 Advisor、生成消息发送，发送成功后才写被发送规则的状态。
//
// 三类健康状况（取价、缺口跳过、K 线同步）各自计时，满 health.fail_alert_after 告警一次，恢复后再发一次。
func (e *Engine) CheckAsset(ctx context.Context, a Asset) error {
	now := e.Now()

	price, src, err := e.fetchPrice(a)
	e.track("price:"+a.Symbol, a.Symbol+" 取价", err, now)
	if err != nil {
		return fmt.Errorf("fetch price %s: %w", a.Symbol, err)
	}

	if e.Syncer != nil {
		for _, bar := range []string{"1d", "5m"} {
			err := e.Syncer.Sync(ctx, a.Pair, bar)
			if err != nil {
				e.logger().Warn("pricewatch sync failed", zap.String("pair", a.Pair), zap.String("bar", bar), zap.Error(err))
			}
			e.track("sync:"+a.Symbol+"/"+bar, a.Symbol+" "+bar+" K 线同步", err, now)
		}
	}

	var sends []pendingSend
	var active []string
	for _, r := range a.Rules {
		ws, err := WindowStatsFor(e.Store, a.Pair, r.Window, price, now)
		if err != nil {
			e.logger().Warn("pricewatch window stats failed", zap.String("pair", a.Pair), zap.String("rule", r.ID), zap.Error(err))
			continue
		}
		var gapErr error
		if ws.GapRatio > maxGapRatio {
			gapErr = fmt.Errorf("%s 窗口缺口 %.1f%%，本轮跳过", r.Window, ws.GapRatio*100)
			e.logger().Warn("pricewatch rule skipped for gap", zap.String("pair", a.Pair), zap.String("rule", r.ID), zap.Float64("gap_ratio", ws.GapRatio))
		}
		e.track("gap:"+a.Symbol+"/"+r.ID, a.Symbol+" "+r.ID+" 数据缺口", gapErr, now)
		if gapErr != nil {
			continue
		}

		x := Measure(r.Type, price, ws.High, ws.Low)
		t := Tier(x, r.ThresholdPct, r.StepPct)
		if t > 0 {
			active = append(active, fmt.Sprintf("%s 第%d档", r.ID, t))
		}
		prev, err := e.Store.GetRuleState(a.Pair, r.ID)
		if err != nil {
			return fmt.Errorf("load rule state %s/%s: %w", a.Pair, r.ID, err)
		}
		next, kind := Decide(prev, t, now, r.Notify)
		if kind != SendNone {
			ref := ws.High
			if r.Type == RiseFromLow {
				ref = ws.Low
			}
			trig := Trigger{Rule: r, X: x, Tier: t, SentInTier: next.SentInTier, Ref: ref}
			if ws.Short {
				trig.ShortDays = ws.Days
			}
			sends = append(sends, pendingSend{rule: r, next: next, trigger: trig})
			continue
		}
		if !e.DryRun && !sameState(prev, next) {
			if err := e.Store.PutRuleState(a.Pair, r.ID, next); err != nil {
				return fmt.Errorf("save rule state %s/%s: %w", a.Pair, r.ID, err)
			}
		}
	}
	e.recordSnap(a.Symbol, assetSnap{price: price, lastOK: now, active: active})

	if len(sends) == 0 {
		return nil
	}
	return e.sendAlert(ctx, a, price, src, sends, now)
}

func (e *Engine) sendAlert(ctx context.Context, a Asset, price float64, src crypto.Provider, sends []pendingSend, now time.Time) error {
	triggers := make([]Trigger, len(sends))
	for i, s := range sends {
		triggers[i] = s.trigger
	}
	ov := e.overview(a.Pair, price, now)
	alert := Alert{Asset: a, Price: price, Source: src.Name(), Triggers: triggers, Overview: ov,
		Advice: e.advise(ctx, a, src, price, triggers, ov, now)}
	text := FormatAlert(alert)

	if e.DryRun {
		fmt.Fprintln(e.Out, text)
		return nil
	}
	if err := e.Sender.SendText(text); err != nil {
		return fmt.Errorf("send alert %s: %w", a.Symbol, err)
	}
	e.mu.Lock()
	e.sent = append(e.sent, now)
	e.mu.Unlock()
	for _, s := range sends {
		if err := e.Store.PutRuleState(a.Pair, s.rule.ID, s.next); err != nil {
			return fmt.Errorf("save rule state %s/%s: %w", a.Pair, s.rule.ID, err)
		}
	}
	return nil
}

// advise 调用 AI；任何失败都返回 nil（消息照发，写「AI 分析不可用」）。
// 返回的建议按 Sides 顺序、每个方向只保留第一条。
func (e *Engine) advise(ctx context.Context, a Asset, src crypto.Provider, price float64, ts []Trigger, ov Overview, now time.Time) []SideAdvice {
	if e.Advisor == nil {
		return nil
	}
	m1, err := src.FetchHistory(a.Pair, now.Add(-time.Hour), now, "1m")
	if err != nil {
		e.logger().Warn("pricewatch fetch 1m failed", zap.String("pair", a.Pair), zap.Error(err))
	}
	m5, err := e.recentCandles(a.Pair, "5m", now.Add(-24*time.Hour), now)
	if err != nil {
		e.logger().Warn("pricewatch load 5m failed", zap.String("pair", a.Pair), zap.Error(err))
	}
	adv, err := e.Advisor.Advise(ctx, AdviceInput{Asset: a, Price: price, Triggers: ts, Overview: ov, M1: m1, M5: m5})
	if err != nil {
		e.logger().Warn("pricewatch advisor failed", zap.String("pair", a.Pair), zap.Error(err))
		return nil
	}
	var out []SideAdvice
	for _, side := range Sides(ts) {
		for _, s := range adv {
			if s.Side == side {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

// overview 计算【多周期】五个数；缺数据的项记 0。
func (e *Engine) overview(pair string, price float64, now time.Time) Overview {
	chg := func(bar string, span time.Duration) float64 {
		c, ok, err := e.Store.FirstCloseSince(pair, bar, now.Add(-span))
		if err != nil || !ok || c <= 0 {
			return 0
		}
		return (price - c) / c * 100
	}
	ov := Overview{
		Chg24h: chg("5m", 24*time.Hour),
		Chg7d:  chg("5m", Window7d.Span()),
		Chg30d: chg("1d", Window30d.Span()),
	}
	if ws, err := WindowStatsFor(e.Store, pair, WindowAll, price, now); err == nil {
		ov.FromATH = (price - ws.High) / ws.High * 100
	}
	if ws, err := WindowStatsFor(e.Store, pair, Window4y, price, now); err == nil {
		ov.From4yLow = (price - ws.Low) / ws.Low * 100
	}
	return ov
}

func (e *Engine) recentCandles(pair, bar string, from, to time.Time) ([]core.OHLCV, error) {
	rows, err := e.Store.db.Query(
		`SELECT ts, open, high, low, close FROM candles WHERE pair = ? AND bar = ? AND ts >= ? AND ts <= ? ORDER BY ts`,
		pair, bar, from.UnixMilli(), to.UnixMilli(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.OHLCV
	for rows.Next() {
		var ms int64
		c := core.OHLCV{Symbol: pair, Interval: bar}
		if err := rows.Scan(&ms, &c.Open, &c.High, &c.Low, &c.Close); err != nil {
			return nil, err
		}
		c.Time = fromMs(ms)
		out = append(out, c)
	}
	return out, rows.Err()
}

// fetchPrice 按 Cfg.Assets.Crypto.Sources 顺序取价；价格 ≤0、非有限或报价为空都视为该源失败。
func (e *Engine) fetchPrice(a Asset) (float64, crypto.Provider, error) {
	var errs []error
	for _, name := range e.Cfg.Assets.Crypto.Sources {
		p := e.provider(name)
		if p == nil {
			errs = append(errs, fmt.Errorf("%s: no provider", name))
			continue
		}
		q, err := p.FetchQuote(a.Pair)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		case q == nil:
			errs = append(errs, fmt.Errorf("%s: empty quote", name))
		case q.Price <= 0 || math.IsInf(q.Price, 0) || math.IsNaN(q.Price):
			errs = append(errs, fmt.Errorf("%s: bad price %v", name, q.Price))
		default:
			return q.Price, p, nil
		}
	}
	return 0, nil, errors.Join(errs...)
}

func (e *Engine) provider(name string) crypto.Provider {
	for _, p := range e.Providers {
		if p.Name() == name {
			return p
		}
	}
	return nil
}

// track 更新一类健康状况；err 非 nil 记失败，否则记成功。产生事件时发送（或 DryRun 写 Out）健康消息。
func (e *Engine) track(key, subject string, err error, now time.Time) {
	e.healthOnce.Do(func() { e.health.After = e.Cfg.Health.FailAlertAfter })
	var ev HealthEvent
	detail := ""
	if err != nil {
		ev = e.health.Fail(key, now)
		detail = err.Error()
	} else {
		ev = e.health.OK(key, now)
	}
	if ev.Kind == HealthNone {
		return
	}
	// 只在送达（或 dry-run 已写出）后确认；发送失败不确认，下一轮重发（AD-30）
	text := FormatHealth(ev.Kind == HealthDown, subject, detail)
	if e.DryRun {
		fmt.Fprintln(e.Out, text)
		e.health.Ack(ev)
		return
	}
	if err := e.Sender.SendText(text); err != nil {
		e.logger().Warn("pricewatch health alert send failed", zap.String("key", key), zap.Error(err))
		return
	}
	e.health.Ack(ev)
}

func (e *Engine) recordSnap(sym string, s assetSnap) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.snaps == nil {
		e.snaps = make(map[string]assetSnap)
	}
	e.snaps[sym] = s
}

// Snapshot 返回心跳用的各资产行（按配置顺序）：最近一次成功检测的价格、时间与触发中的规则。
// 尚未成功检测过的资产价格为 0、LastOK 为零值。
func (e *Engine) Snapshot() []HeartbeatRow {
	e.mu.Lock()
	defer e.mu.Unlock()
	assets := e.Cfg.CryptoAssets()
	rows := make([]HeartbeatRow, len(assets))
	for i, a := range assets {
		s := e.snaps[a.Symbol]
		rows[i] = HeartbeatRow{Asset: a, Price: s.price, LastOK: s.lastOK, Active: append([]string(nil), s.active...)}
	}
	return rows
}

// Sent24h 返回过去 24 小时内成功发出的提醒条数（不含健康告警与 DryRun）。
func (e *Engine) Sent24h() int {
	cutoff := e.Now().Add(-24 * time.Hour)
	e.mu.Lock()
	defer e.mu.Unlock()
	kept := e.sent[:0]
	for _, t := range e.sent {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	e.sent = kept
	return len(kept)
}

func (e *Engine) logger() *zap.Logger {
	if e.Log == nil {
		return zap.NewNop()
	}
	return e.Log
}

func sameState(a, b RuleState) bool {
	return a.Tier == b.Tier && a.SentInTier == b.SentInTier && a.LastSentAt.Equal(b.LastSentAt)
}
