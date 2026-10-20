package pricewatch

import (
	"fmt"
	"strconv"
	"time"
)

// WindowStats 是某个窗口在本地 K 线上的统计（spec §4、§6）。
type WindowStats struct {
	High, Low float64 // 已并入现价
	Days      int     // 有效窗口的天数（历史不够长时小于窗口长度）
	Short     bool    // 历史不够长：记录的上线日晚于窗口起点（AD-21）
	GapRatio  float64 // 有效窗口内没有任何 K 线的槽占比（AD-13）
}

func slotLength(bar string) time.Duration {
	if bar == "5m" {
		return 5 * time.Minute
	}
	return 24 * time.Hour
}

// WindowStatsFor 计算 pair 在窗口 w 内的极值、有效天数与缺口比例。
//
// 有效起点：非 all 窗口为 now−Span，若记录的上线日更晚则取上线日并标记 Short；
// all 窗口为 max(上线日, 本地最早一根)，不标记 Short（AD-14）。
// 未记录上线日时，本地缺头一律算缺口（AD-21）。
//
// GapRatio 把有效窗口切成 bar 时长的槽，统计没有任何 K 线的槽占比：与 K 线对齐方式无关，
// 同一槽里的重复 K 线也不会抵消别处的缺口。没有任何 K 线时为 1。
func WindowStatsFor(st *Store, pair string, w Window, price float64, now time.Time) (WindowStats, error) {
	bar := w.Bar()

	listing, hasListing, err := readListing(st, pair, bar)
	if err != nil {
		return WindowStats{}, err
	}

	var ws WindowStats
	var start time.Time // all 窗口零值即不设下界
	if w != WindowAll {
		start = now.Add(-w.Span())
	}
	if hasListing && listing.After(start) {
		start = listing
		ws.Short = w != WindowAll
	}

	rows, err := st.db.Query(
		`SELECT ts, high, low FROM candles WHERE pair = ? AND bar = ? AND ts >= ? AND ts <= ? ORDER BY ts`,
		pair, bar, start.UnixMilli(), now.UnixMilli(),
	)
	if err != nil {
		return WindowStats{}, fmt.Errorf("window candles %s/%s: %w", pair, bar, err)
	}
	defer rows.Close()

	type point struct {
		ts        time.Time
		high, low float64
	}
	var pts []point
	ws.High, ws.Low = price, price
	for rows.Next() {
		var ms int64
		var p point
		if err := rows.Scan(&ms, &p.high, &p.low); err != nil {
			return WindowStats{}, fmt.Errorf("scan window candle %s/%s: %w", pair, bar, err)
		}
		p.ts = fromMs(ms)
		pts = append(pts, p)
		ws.High = max(ws.High, p.high)
		ws.Low = min(ws.Low, p.low)
	}
	if err := rows.Err(); err != nil {
		return WindowStats{}, fmt.Errorf("window candles %s/%s: %w", pair, bar, err)
	}

	if w == WindowAll && !hasListing {
		// 有效起点 = 本地最早一根；一根都没有时窗口为空
		start = now
		if len(pts) > 0 {
			start = pts[0].ts
		}
	} else if w == WindowAll && len(pts) > 0 && pts[0].ts.After(start) {
		start = pts[0].ts
	}

	span := now.Sub(start)
	ws.Days = int(ceilDiv(span, 24*time.Hour))

	slot := slotLength(bar)
	n := max(1, ceilDiv(span, slot))
	covered := make(map[int64]bool, len(pts))
	for _, p := range pts {
		covered[min(int64(p.ts.Sub(start)/slot), n-1)] = true
	}
	ws.GapRatio = float64(n-int64(len(covered))) / float64(n)
	return ws, nil
}

func ceilDiv(d, unit time.Duration) int64 {
	return int64((d + unit - 1) / unit)
}

func readListing(st *Store, pair, bar string) (time.Time, bool, error) {
	v, err := st.GetKV(ListingKey(pair, bar))
	if err != nil || v == "" {
		return time.Time{}, false, err
	}
	ms, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("invalid %s value %q: %w", ListingKey(pair, bar), v, err)
	}
	return fromMs(ms), true, nil
}
