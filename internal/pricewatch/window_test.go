package pricewatch

// Context Checkpoint: done_criteria → test mapping (TASK-010)
// functional[0] 上线日 100 天前 + 日线齐全、4y → Short/Days==100/Gap 0          → TestWindowStats_Short
// functional[1] 无上线日、5m 只有最近 5 天、7d → 非 Short、Gap≈2/7                → TestWindowStats_HeadMissing
// functional[2] 5m 中间缺 20%；日线一段每天两根另一段缺 30%                       → TestWindowStats_Gap
// functional[3] 现价并入 High/Low，否则取窗口内 max(high)/min(low)               → TestWindowStats_PriceIncluded
// boundary[0]   all：非 Short、Days 为有效起点到 now；无 K 线 Gap==1 且无 NaN     → TestWindowStats_All

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// winNow 刻意不落在 5m/1d 边界上，让槽位起点与 K 线对齐方式错开。
var winNow = time.Date(2026, 10, 20, 8, 3, 17, 0, time.UTC)

// winSeries 生成 (from, winNow] 内按 dur 对齐（自 offset 起）的 K 线，keep 为 nil 时全保留。
func winSeries(from time.Time, dur, offset time.Duration, keep func(time.Time) bool) []core.OHLCV {
	var cs []core.OHLCV
	for ts := from.Truncate(dur).Add(offset); !ts.After(winNow); ts = ts.Add(dur) {
		if ts.After(from) && (keep == nil || keep(ts)) {
			cs = append(cs, candle(ts, 100, 90, 95))
		}
	}
	return cs
}

func winPutListing(t *testing.T, s *Store, pair, bar string, at time.Time) {
	t.Helper()
	if err := s.PutKV("listing:"+pair+":"+bar, strconv.FormatInt(at.UnixMilli(), 10)); err != nil {
		t.Fatal(err)
	}
}

func winStats(t *testing.T, s *Store, w Window, price float64) WindowStats {
	t.Helper()
	ws, err := WindowStatsFor(s, "BTCUSDT", w, price, winNow)
	if err != nil {
		t.Fatalf("WindowStatsFor(%s): %v", w, err)
	}
	return ws
}

func winNear(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.Abs(got-want) > 0.01 {
		t.Errorf("%s = %v, want %v±0.01", name, got, want)
	}
}

const winDay = 24 * time.Hour

func TestWindowStats_Short(t *testing.T) {
	cases := []struct {
		name    string
		listing func(first time.Time) time.Time
	}{
		{"listing exactly 100 days ago", func(time.Time) time.Time { return winNow.Add(-100 * winDay) }},
		{"listing at first candle", func(first time.Time) time.Time { return first }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _ := newTestStore(t)
			cs := winSeries(winNow.Add(-100*winDay), winDay, 0, nil)
			if len(cs) != 100 {
				t.Fatalf("fixture has %d daily candles, want 100", len(cs))
			}
			mustUpsert(t, s, "BTCUSDT", "1d", cs...)
			winPutListing(t, s, "BTCUSDT", "1d", c.listing(cs[0].Time))

			ws := winStats(t, s, Window4y, 95)
			if !ws.Short || ws.Days != 100 || ws.GapRatio != 0 {
				t.Errorf("got %+v, want Short=true Days=100 GapRatio=0", ws)
			}
		})
	}

	t.Run("listing before window start is not short", func(t *testing.T) {
		s, _ := newTestStore(t)
		mustUpsert(t, s, "BTCUSDT", "1d", winSeries(winNow.Add(-30*winDay), winDay, 0, nil)...)
		winPutListing(t, s, "BTCUSDT", "1d", winNow.Add(-2000*winDay))
		ws := winStats(t, s, Window30d, 95)
		if ws.Short || ws.Days != 30 || ws.GapRatio != 0 {
			t.Errorf("got %+v, want Short=false Days=30 GapRatio=0", ws)
		}
	})

	t.Run("listing keyed by bar", func(t *testing.T) {
		// 只记录了 5m 的上线日，日线窗口不应据此判 Short
		s, _ := newTestStore(t)
		mustUpsert(t, s, "BTCUSDT", "1d", winSeries(winNow.Add(-100*winDay), winDay, 0, nil)...)
		winPutListing(t, s, "BTCUSDT", "5m", winNow.Add(-100*winDay))
		if ws := winStats(t, s, Window4y, 95); ws.Short {
			t.Errorf("got %+v, want Short=false", ws)
		}
	})
}

func TestWindowStats_HeadMissing(t *testing.T) {
	s, _ := newTestStore(t)
	mustUpsert(t, s, "BTCUSDT", "5m", winSeries(winNow.Add(-5*winDay), 5*time.Minute, 0, nil)...)
	ws := winStats(t, s, Window7d, 95)
	if ws.Short {
		t.Errorf("Short = true, want false (head missing is a gap)")
	}
	if ws.Days != 7 {
		t.Errorf("Days = %d, want 7", ws.Days)
	}
	winNear(t, "GapRatio", ws.GapRatio, 2.0/7)
}

func TestWindowStats_Gap(t *testing.T) {
	t.Run("5m middle 20% missing", func(t *testing.T) {
		s, _ := newTestStore(t)
		start := winNow.Add(-7 * winDay)
		holeFrom := start.Add(2 * winDay)
		holeTo := holeFrom.Add(7 * winDay / 5)
		mustUpsert(t, s, "BTCUSDT", "5m", winSeries(start, 5*time.Minute, 0, func(ts time.Time) bool {
			return ts.Before(holeFrom) || !ts.Before(holeTo)
		})...)
		winNear(t, "GapRatio", winStats(t, s, Window7d, 95).GapRatio, 0.2)
	})

	t.Run("1d doubled segment does not mask 30% gap", func(t *testing.T) {
		s, _ := newTestStore(t)
		start := winNow.Add(-30 * winDay)
		// 前 15 天每天 00:00 与 16:00 各一根（两种对齐混存）
		mid := start.Add(15 * winDay)
		mustUpsert(t, s, "BTCUSDT", "1d", winSeries(start, winDay, 0, nil)...)
		mustUpsert(t, s, "BTCUSDT", "1d", winSeries(start, winDay, 16*time.Hour, func(ts time.Time) bool {
			return ts.Before(mid)
		})...)
		// 后段删掉 9 天（30 槽的 30%）：这几天只有 00:00 一根
		holeFrom := mid.Add(3 * winDay)
		holeTo := holeFrom.Add(9 * winDay)
		if _, err := s.db.Exec(`DELETE FROM candles WHERE pair = ? AND bar = ? AND ts >= ? AND ts < ?`,
			"BTCUSDT", "1d", holeFrom.UnixMilli(), holeTo.UnixMilli()); err != nil {
			t.Fatal(err)
		}
		if st := mustStats(t, s, "BTCUSDT", "1d", start, winNow); st.Count <= 30*7/10 {
			t.Fatalf("fixture Count = %d, want > 21 so that counting candles would hide the gap", st.Count)
		}
		winNear(t, "GapRatio", winStats(t, s, Window30d, 95).GapRatio, 0.3)
	})
}

func TestWindowStats_PriceIncluded(t *testing.T) {
	s, _ := newTestStore(t)
	base := winNow.Truncate(5 * time.Minute)
	mustUpsert(t, s, "BTCUSDT", "5m",
		candle(base.Add(-2*time.Hour), 110, 95, 100),
		candle(base.Add(-time.Hour), 105, 90, 100),
		candle(base, 108, 92, 100),
		// 窗口外（早于 24h）的极端值不得计入
		candle(winNow.Add(-25*time.Hour), 1000, 1, 100),
	)
	cases := []struct {
		price, high, low float64
	}{
		{200, 200, 90},
		{50, 110, 50},
		{100, 110, 90},
	}
	for _, c := range cases {
		ws := winStats(t, s, Window24h, c.price)
		if ws.High != c.high || ws.Low != c.low {
			t.Errorf("price %v: High/Low = %v/%v, want %v/%v", c.price, ws.High, ws.Low, c.high, c.low)
		}
	}
}

func TestWindowStats_All(t *testing.T) {
	t.Run("effective start is local earliest", func(t *testing.T) {
		s, _ := newTestStore(t)
		cs := winSeries(winNow.Add(-50*winDay), winDay, 0, nil)
		mustUpsert(t, s, "BTCUSDT", "1d", cs...)
		ws := winStats(t, s, WindowAll, 95)
		// 最早一根是 49 天前 00:00，到 08:03 跨 49 天多 ⇒ 50 天
		if ws.Short || ws.Days != 50 || ws.GapRatio != 0 {
			t.Errorf("got %+v, want Short=false Days=50 GapRatio=0", ws)
		}

		// 上线日早于本地最早一根：有效起点仍取本地最早一根
		winPutListing(t, s, "BTCUSDT", "1d", winNow.Add(-3000*winDay))
		if ws := winStats(t, s, WindowAll, 95); ws.Short || ws.Days != 50 || ws.GapRatio != 0 {
			t.Errorf("listing earlier: got %+v, want Short=false Days=50 GapRatio=0", ws)
		}

		// 上线日晚于本地最早一根：有效起点取上线日
		winPutListing(t, s, "BTCUSDT", "1d", winNow.Add(-20*winDay))
		if ws := winStats(t, s, WindowAll, 95); ws.Short || ws.Days != 20 || ws.GapRatio != 0 {
			t.Errorf("listing later: got %+v, want Short=false Days=20 GapRatio=0", ws)
		}
	})

	t.Run("latest partial slot missing counts as gap", func(t *testing.T) {
		s, _ := newTestStore(t)
		today := winNow.Truncate(winDay)
		mustUpsert(t, s, "BTCUSDT", "1d", winSeries(winNow.Add(-50*winDay), winDay, 0, func(ts time.Time) bool {
			return ts.Before(today)
		})...)
		// 有效起点 49 天前 00:00 到 08:03 共 50 槽，缺最后一槽（今天那根）
		winNear(t, "GapRatio", winStats(t, s, WindowAll, 95).GapRatio, 1.0/50)
	})

	t.Run("no candles", func(t *testing.T) {
		s, _ := newTestStore(t)
		ws := winStats(t, s, WindowAll, 95)
		if ws.GapRatio != 1 || ws.Days != 0 || ws.Short || ws.High != 95 || ws.Low != 95 {
			t.Errorf("got %+v, want GapRatio=1 Days=0 Short=false High=Low=95", ws)
		}
		for _, w := range []Window{Window24h, Window7d, Window30d, Window4y} {
			if ws := winStats(t, s, w, 95); ws.GapRatio != 1 || ws.High != 95 || ws.Low != 95 {
				t.Errorf("%s: got %+v, want GapRatio=1 High=Low=95", w, ws)
			}
		}
	})

	t.Run("no candles with listing", func(t *testing.T) {
		s, _ := newTestStore(t)
		winPutListing(t, s, "BTCUSDT", "1d", winNow.Add(-10*winDay))
		if ws := winStats(t, s, WindowAll, 95); ws.GapRatio != 1 || ws.Days != 10 || math.IsNaN(ws.GapRatio) {
			t.Errorf("got %+v, want GapRatio=1 Days=10", ws)
		}
	})
}

func TestWindowStats_CandleAtNow(t *testing.T) {
	// 24h 齐全且恰有一根落在 now（窗口跨度是 5m 的整数倍）：仍算末槽，GapRatio 不得为负
	s, _ := newTestStore(t)
	mustUpsert(t, s, "BTCUSDT", "5m", winSeries(winNow.Add(-winDay), 5*time.Minute, 0, nil)...)
	mustUpsert(t, s, "BTCUSDT", "5m", candle(winNow, 100, 90, 95))
	if ws := winStats(t, s, Window24h, 95); ws.GapRatio != 0 {
		t.Errorf("GapRatio = %v, want 0", ws.GapRatio)
	}
}

func TestWindowStats_NonUTCNow(t *testing.T) {
	s, _ := newTestStore(t)
	mustUpsert(t, s, "BTCUSDT", "1d", winSeries(winNow.Add(-100*winDay), winDay, 0, nil)...)
	winPutListing(t, s, "BTCUSDT", "1d", winNow.Add(-100*winDay))
	shanghai := time.FixedZone("UTC+8", 8*3600)
	got, err := WindowStatsFor(s, "BTCUSDT", Window4y, 95, winNow.In(shanghai))
	if err != nil {
		t.Fatal(err)
	}
	if want := winStats(t, s, Window4y, 95); got != want {
		t.Errorf("non-UTC now: got %+v, want %+v", got, want)
	}
}

func TestWindowStats_BadListing(t *testing.T) {
	s, _ := newTestStore(t)
	if err := s.PutKV("listing:BTCUSDT:1d", "not-a-number"); err != nil {
		t.Fatal(err)
	}
	if _, err := WindowStatsFor(s, "BTCUSDT", Window4y, 95, winNow); err == nil {
		t.Error("expected error for malformed listing value")
	}
}
