package pricewatch

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/newthinker/atlas/internal/collector/crypto"
	"github.com/newthinker/atlas/internal/core"
)

const (
	chunkBars     = 300 // 每段根数（okx 单次上限）
	coverageOK    = 0.9 // 段内槽位覆盖率达到即视为完整
	fiveMinRetain = 8 * 24 * time.Hour
)

var (
	dailyBackfillStart = time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)
	minCandleTime      = time.Date(2009, 1, 1, 0, 0, 0, 0, time.UTC)
)

// Syncer 把本地 K 线补到当前时刻（spec §6，AD-12/20/21/22）。
// Providers 的顺序即回填优先级（AD-20：历史最长的源在前）。
type Syncer struct {
	Providers []crypto.Provider
	Store     *Store
	Now       func() time.Time
}

// ListingKey 是上线日在 kv 中的键（AD-21），值为 UTC Unix 毫秒的十进制字符串。
func ListingKey(pair, bar string) string { return "listing:" + pair + ":" + bar }

func barDuration(bar string) (time.Duration, error) {
	switch bar {
	case "1d":
		return 24 * time.Hour, nil
	case "5m":
		return 5 * time.Minute, nil
	}
	return 0, fmt.Errorf("unsupported bar %q", bar)
}

// Sync 按段扫描 [backfillStart, now)：
//   - 尾段起点 = 本地最新一根 − 1 个 bar（AD-12，刷新未收盘那根；okx 区间两端不含）；
//     本地为空或最新一根早于一段之前时，尾段就是最后一个整段；
//   - 尾段之前的各段：本地槽位覆盖率 ≥90% 且不含本地最新一根则跳过，否则重取（AD-22）；
//   - 一段按 Providers 顺序取，报错或不足 90% 换下一个，全不足取槽位最多的；
//   - 开头连续的全 0 段之后第一段有数据时，记录上线日（AD-21），下次从上线日开始。
//
// 某段所有源都报错时继续补其余段，最后返回含 pair 与 bar 的错误。
func (s *Syncer) Sync(ctx context.Context, pair, bar string) error {
	step, err := barDuration(bar)
	if err != nil {
		return fmt.Errorf("sync %s/%s: %w", pair, bar, err)
	}
	now := s.Now()
	start, err := s.backfillStart(pair, bar, now)
	if err != nil {
		return fmt.Errorf("sync %s/%s: %w", pair, bar, err)
	}
	latest, hasLatest, err := s.Store.LatestCandle(pair, bar)
	if err != nil {
		return fmt.Errorf("sync %s/%s: %w", pair, bar, err)
	}

	chunk := chunkBars * step
	var segs [][2]time.Time
	for from := start; from.Before(now); from = from.Add(chunk) {
		segs = append(segs, [2]time.Time{from, minTime(from.Add(chunk), now)})
	}
	if len(segs) == 0 {
		return nil
	}
	tailFrom := segs[len(segs)-1][0]
	if hasLatest && now.Sub(latest.Add(-step)) <= chunk {
		tailFrom = latest.Add(-step)
	}

	var (
		errs         []error
		leadingEmpty = true // 到目前为止每段都取过且所有源都是 0 根
	)
	fetchSeg := func(from, to time.Time) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		cs, allZero, err := s.fetch(pair, bar, step, from, to)
		if err != nil {
			errs = append(errs, err)
			leadingEmpty = false
			return nil
		}
		if allZero {
			return nil
		}
		if len(cs) == 0 {
			leadingEmpty = false
			return nil
		}
		if err := s.Store.UpsertCandles(pair, bar, cs); err != nil {
			return fmt.Errorf("sync %s/%s: %w", pair, bar, err)
		}
		if leadingEmpty && from.After(start) {
			if err := s.Store.PutKV(ListingKey(pair, bar), strconv.FormatInt(earliest(cs).UnixMilli(), 10)); err != nil {
				return fmt.Errorf("sync %s/%s: %w", pair, bar, err)
			}
		}
		leadingEmpty = false
		return nil
	}

	for _, seg := range segs {
		from, to := seg[0], minTime(seg[1], tailFrom)
		if !from.Before(tailFrom) {
			break
		}
		containsLatest := hasLatest && !latest.Before(from) && latest.Before(to)
		if !containsLatest {
			cov, err := s.localCoverage(pair, bar, step, from, to)
			if err != nil {
				return fmt.Errorf("sync %s/%s: %w", pair, bar, err)
			}
			if cov >= coverageOK {
				leadingEmpty = false
				continue
			}
		}
		if err := fetchSeg(from, to); err != nil {
			return err
		}
	}
	if err := fetchSeg(tailFrom, now); err != nil {
		return err
	}

	if bar == "5m" {
		if err := s.Store.PruneCandles(pair, bar, now.Add(-fiveMinRetain)); err != nil {
			return fmt.Errorf("sync %s/%s: %w", pair, bar, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("sync %s/%s: %w", pair, bar, errors.Join(errs...))
	}
	return nil
}

// backfillStart：1d 为 2010-01-01，5m 为 now−8d；已记录上线日且更晚时从上线日开始。
func (s *Syncer) backfillStart(pair, bar string, now time.Time) (time.Time, error) {
	start := dailyBackfillStart
	if bar == "5m" {
		start = now.Add(-fiveMinRetain)
	}
	v, err := s.Store.GetKV(ListingKey(pair, bar))
	if err != nil || v == "" {
		return start, err
	}
	ms, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return start, fmt.Errorf("bad listing value %q: %w", v, err)
	}
	if listing := time.UnixMilli(ms).UTC(); listing.After(start) {
		start = listing
	}
	return start, nil
}

// fetch 按 Providers 顺序取 [from, to) 的 K 线。请求起点减 1ms，使两端不含的 okx
// 也能取到恰在 from 的那根。allZero 表示所有源都成功且都返回 0 根有效 K 线。
func (s *Syncer) fetch(pair, bar string, step time.Duration, from, to time.Time) (best []core.OHLCV, allZero bool, err error) {
	want := slotCount(from, to, step)
	bestSlots, anyOK := -1, false
	var errs []error
	for _, p := range s.Providers {
		raw, err := p.FetchHistory(pair, from.Add(-time.Millisecond), to, bar)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s [%s, %s): %w", p.Name(), from.Format(time.RFC3339), to.Format(time.RFC3339), err))
			continue
		}
		anyOK = true
		cs := validCandles(raw)
		got := slotsHit(cs, from, to, step)
		if float64(got) >= coverageOK*float64(want) {
			return cs, false, nil
		}
		if got > bestSlots {
			best, bestSlots = cs, got
		}
	}
	if !anyOK {
		return nil, false, errors.Join(errs...)
	}
	return best, bestSlots == 0 && len(errs) == 0, nil
}

// localCoverage 是本地 [from, to) 内有 K 线的槽位占比；槽位按 from 对齐，与 K 线
// 自身的对齐方式无关，同一槽里多根（okx 16:00 与 binance 00:00 混存）只算一次（AD-13）。
func (s *Syncer) localCoverage(pair, bar string, step time.Duration, from, to time.Time) (float64, error) {
	want := slotCount(from, to, step)
	if want == 0 {
		return 1, nil
	}
	var got int
	err := s.Store.db.QueryRow(
		`SELECT COUNT(DISTINCT (ts - ?) / ?) FROM candles WHERE pair = ? AND bar = ? AND ts >= ? AND ts < ?`,
		from.UnixMilli(), step.Milliseconds(), pair, bar, from.UnixMilli(), to.UnixMilli(),
	).Scan(&got)
	if err != nil {
		return 0, fmt.Errorf("local coverage: %w", err)
	}
	return float64(got) / float64(want), nil
}

// slotCount 是 [from, to) 内完整槽位数（末尾不足一个 bar 的部分不计）。
func slotCount(from, to time.Time, step time.Duration) int {
	if !to.After(from) {
		return 0
	}
	return int(to.Sub(from) / step)
}

func slotsHit(cs []core.OHLCV, from, to time.Time, step time.Duration) int {
	seen := map[int64]bool{}
	for _, c := range cs {
		if !c.Time.Before(from) && c.Time.Before(to) {
			seen[int64(c.Time.Sub(from)/step)] = true
		}
	}
	return len(seen)
}

// validCandle 丢弃 provider 吞掉解析错误产生的坏 K 线（AD-20b，reviewer P4）。
func validCandle(c core.OHLCV) bool {
	for _, v := range []float64{c.Open, c.High, c.Low, c.Close} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
			return false
		}
	}
	return c.Low <= c.High && !c.Time.Before(minCandleTime)
}

func validCandles(cs []core.OHLCV) []core.OHLCV {
	out := make([]core.OHLCV, 0, len(cs))
	for _, c := range cs {
		if validCandle(c) {
			out = append(out, c)
		}
	}
	return out
}

func earliest(cs []core.OHLCV) time.Time {
	t := cs[0].Time
	for _, c := range cs[1:] {
		if c.Time.Before(t) {
			t = c.Time
		}
	}
	return t
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
