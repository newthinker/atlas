package tiingo

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// priceRow 用指针区分 null 与 0。
type priceRow struct {
	Date        string   `json:"date"`
	Open        *float64 `json:"open"`
	High        *float64 `json:"high"`
	Low         *float64 `json:"low"`
	Close       *float64 `json:"close"`
	Volume      *float64 `json:"volume"`
	SplitFactor *float64 `json:"splitFactor"`
}

// normalize 把 Tiingo 原始价折算为 yahoo chart close 口径（拆股调整、不含分红，设计 §2.4）
// 并截取 [start, end]（按日，闭区间）。折算须在截取之前对全量行进行：
// t 日 O/H/L/C ÷ t 之后（不含 t）所有行 splitFactor 之积，Volume × 同一积。
// 坏日期的行整行丢弃；O/H/L/C 任一缺失的行不输出，但其 splitFactor 仍参与累乘——
// 拆股当日恰好缺价时若连因子一起丢，此前全部价格会错一个倍数。
// 任一 splitFactor 非有限或 ≤0 → 整段失败。
func normalize(symbol string, rows []priceRow, start, end time.Time) ([]core.OHLCV, error) {
	type bar struct {
		t time.Time
		r priceRow
	}
	var bars []bar
	for _, r := range rows {
		if len(r.Date) < 10 {
			continue
		}
		t, err := time.Parse("2006-01-02", r.Date[:10])
		if err != nil {
			continue
		}
		bars = append(bars, bar{t, r})
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].t.Before(bars[j].t) })

	lo := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	hi := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	out := make([]core.OHLCV, 0, len(bars))
	cum := 1.0
	for i := len(bars) - 1; i >= 0; i-- {
		b := bars[i]
		hasPrice := b.r.Open != nil && b.r.High != nil && b.r.Low != nil && b.r.Close != nil
		if hasPrice && !b.t.Before(lo) && !b.t.After(hi) {
			vol := 0.0
			if b.r.Volume != nil {
				vol = *b.r.Volume
			}
			out = append(out, core.OHLCV{
				Symbol: symbol, Interval: "1d", Time: b.t,
				Open: *b.r.Open / cum, High: *b.r.High / cum, Low: *b.r.Low / cum, Close: *b.r.Close / cum,
				Volume: int64(math.Round(vol * cum)),
			})
		}
		sf := 1.0
		if b.r.SplitFactor != nil {
			sf = *b.r.SplitFactor
		}
		if math.IsNaN(sf) || math.IsInf(sf, 0) || sf <= 0 {
			return nil, fmt.Errorf("%s: invalid splitFactor %v on %s", symbol, sf, b.t.Format("2006-01-02"))
		}
		cum *= sf
	}
	slices.Reverse(out)
	return out, nil
}
