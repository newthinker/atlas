// Package prism orchestrates the daily Prism valuation refresh (design doc
// docs/prism/atlas_prism_design.md §9 M1).
package prism

import (
	"fmt"
	"math"
	"sort"
	"time"

	akshare "github.com/newthinker/atlas/internal/collector/akshare"
	"github.com/newthinker/atlas/internal/collector/lixinger"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/core"
	prismstore "github.com/newthinker/atlas/internal/storage/prism"
	"github.com/newthinker/atlas/internal/valuation"
)

// Store is the subset of *prismstore.Store used by Refresh.
type Store interface {
	UpsertInstrument(inst prismstore.Instrument) (int64, error)
	LatestDate(instrumentID int64) (string, error)
	UpsertValuations(instrumentID int64, rows []prismstore.ValuationRow) error
	Series(symbol, from string) (*prismstore.SeriesData, error)
}

// LixingerClient is the subset of *lixinger.Lixinger used by Refresh.
type LixingerClient interface {
	FetchValuationSeries(symbol string, start, end time.Time) ([]lixinger.ValuationPoint, error)
}

// AkshareClient is the subset of *akshare.Client used by Refresh.
type AkshareClient interface {
	FetchStockValuationSeries(symbol, market string, start, end time.Time) ([]akshare.ValuationPoint, error)
	FetchIndexValuationSeries(symbol string, start, end time.Time) ([]akshare.ValuationPoint, error)
}

// USClient is the subset of *yahoo.Yahoo used by the US engine path (Task 6).
type USClient interface {
	FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error)
	FetchEPSHistory(symbol string, start, end time.Time) ([]core.EPSPoint, error)
}

// Report summarizes one refresh run. Partial failures do not abort the run.
type Report struct {
	Refreshed int
	Failed    []string // "SYMBOL: error" 摘要
	Degraded  []string // 主源失败但兜底成功的标的(可观测降级,进告警提示不算失败)
}

// Refresh updates every configured instrument: lixinger-sourced instruments
// fetch incrementally (理杏豆计费), engine-sourced US stocks rebuild via yahoo.
func Refresh(cfg config.PrismConfig, store Store, lix LixingerClient, us USClient, ak AkshareClient, now time.Time) Report {
	var rep Report
	for _, inst := range cfg.Instruments {
		var err error
		switch inst.Source {
		case "lixinger":
			err = refreshLixinger(cfg, store, lix, inst, now)
			if err != nil && inst.FallbackSource == "akshare" {
				if fbErr := refreshAkshare(cfg, store, ak, inst, now); fbErr == nil {
					rep.Degraded = append(rep.Degraded,
						fmt.Sprintf("%s: lixinger failed (%v), akshare fallback ok", inst.Symbol, err))
					err = nil
				} else {
					err = fmt.Errorf("lixinger: %v; akshare fallback: %v", err, fbErr)
				}
			}
		case "engine":
			err = refreshEngine(cfg, store, us, inst, now)
		case "akshare":
			err = refreshAkshare(cfg, store, ak, inst, now)
		default:
			err = fmt.Errorf("unknown source %q", inst.Source)
		}
		if err != nil {
			rep.Failed = append(rep.Failed, fmt.Sprintf("%s: %v", inst.Symbol, err))
			continue
		}
		rep.Refreshed++
	}
	return rep
}

func upsertMeta(store Store, inst config.PrismInstrument) (int64, error) {
	return store.UpsertInstrument(prismstore.Instrument{
		Symbol: inst.Symbol, Type: inst.Type, Market: inst.Market,
		Name: inst.Name, Group: inst.Group, Source: inst.Source,
	})
}

// incrementalStart 计算增量拉取起点并处理「已是最新」的日历日零请求守卫,供
// lixinger/akshare 两路复用。返回 (start, skip, err);skip=true 表示已是最新、
// 应零请求直接成功。首次(latest="")从 now-lookbackYears 回填。
//
// 日历日语义比较(ISO 日期串按字典序即时间序,与宿主时区无关):增量起点已达今天
// 或更新 → 零请求。避免西经宿主机把 latest 的 UTC 午夜与本地 now 做绝对时刻比较,
// 产生 startDate>endDate 的倒挂请求。
func incrementalStart(store Store, id int64, lookbackYears int, now time.Time) (time.Time, bool, error) {
	latest, err := store.LatestDate(id)
	if err != nil {
		return time.Time{}, false, err
	}
	start := now.AddDate(-lookbackYears, 0, 0)
	if latest != "" {
		d, perr := time.Parse("2006-01-02", latest)
		if perr != nil {
			return time.Time{}, false, fmt.Errorf("bad latest date %q: %w", latest, perr)
		}
		start = d.AddDate(0, 0, 1) // 增量:从 latest 的次日开始
		if start.Format("2006-01-02") >= now.Format("2006-01-02") {
			return time.Time{}, true, nil
		}
	}
	return start, false, nil
}

func refreshLixinger(cfg config.PrismConfig, store Store, lix LixingerClient, inst config.PrismInstrument, now time.Time) error {
	id, err := upsertMeta(store, inst)
	if err != nil {
		return err
	}
	start, skip, err := incrementalStart(store, id, cfg.LookbackYears, now)
	if err != nil || skip {
		return err
	}
	pts, err := lix.FetchValuationSeries(inst.Symbol, start, now)
	if err != nil {
		return err
	}
	rows := make([]prismstore.ValuationRow, 0, len(pts))
	for _, p := range pts {
		rows = append(rows, prismstore.ValuationRow{
			D:     p.Date.Format("2006-01-02"),
			PETTM: p.PETTM, PB: p.PB, PSTTM: p.PSTTM,
			Pctl5Y: p.Pctl5Y, Pctl10Y: p.Pctl10Y,
		})
	}
	return store.UpsertValuations(id, rows)
}

// currentEPS returns the EPS of the latest-dated point (current TTM EPS),
// mirroring valuation.ReconstructPEPercentile's currentEPS. ok is false for an
// empty slice. Does not assume eps is sorted.
func currentEPS(eps []core.EPSPoint) (float64, bool) {
	var latest core.EPSPoint
	found := false
	for _, p := range eps {
		if !found || p.Date.After(latest.Date) {
			latest, found = p, true
		}
	}
	return latest.EPS, found
}

// rollingMinPoints: 滚动分位窗口内最少样本数(约 1 年交易日)。
const rollingMinPoints = 252

// refreshEngine 每日全量重算美股公司近 us_lookback_years 年 PE 序列并整段 upsert
// (幂等,无增量状态)。yahoo 路径 M1 口径:无 PB/PSTTM,仅 5Y 滚动分位。
func refreshEngine(cfg config.PrismConfig, store Store, us USClient, inst config.PrismInstrument, now time.Time) error {
	id, err := upsertMeta(store, inst)
	if err != nil {
		return err
	}
	start := now.AddDate(-cfg.USLookbackYears, 0, 0)
	// EPS 需要比价格多回看 1 年,保证首个交易日有可对齐的 EPS 点
	eps, err := us.FetchEPSHistory(inst.Symbol, start.AddDate(-1, 0, 0), now)
	if err != nil {
		return fmt.Errorf("eps history: %w", err)
	}
	closes, err := us.FetchHistory(inst.Symbol, start, now, "1d")
	if err != nil {
		return fmt.Errorf("price history: %w", err)
	}
	// 熔断:当前 EPS(TTM)<=0(真实亏损)→ 不入库,记入 Report.Failed,语义对齐
	// valuation.ReconstructPEPercentile 的 ErrNonPositiveEPS。否则历史有 >=8 正 EPS
	// 季但最新亏损的标的会静默入库截断到最后盈利日的过期序列,误导 board 展示。
	if e, ok := currentEPS(eps); ok && e <= 0 {
		return valuation.ErrNonPositiveEPS
	}
	dates, pe, err := valuation.ReconstructPESeries(closes, eps)
	if err != nil {
		return fmt.Errorf("reconstruct: %w", err)
	}
	p5 := valuation.RollingPercentile(dates, pe, 5, rollingMinPoints)

	rows := make([]prismstore.ValuationRow, len(dates))
	for i := range dates {
		rows[i] = prismstore.ValuationRow{
			D: dates[i].Format("2006-01-02"), PETTM: pe[i],
			PB: math.NaN(), PSTTM: math.NaN(), // yahoo 路径 M1 无 PB/PS(设计 §9 M1 口径标注)
			Pctl5Y: p5[i], Pctl10Y: math.NaN(), // 仅 5Y 数据,10Y 分位无意义
		}
	}
	return store.UpsertValuations(id, rows)
}

// refreshAkshare 每日增量拉取 akshare(经 aktools)公司/指数估值,并在本地计算滚动
// 分位——akshare 无官方分位,故读回 store 已存全历史与新拉取点合并后整段算 5Y/10Y
// 分位,但只为新拉取的日期写行(历史行不回写)。
func refreshAkshare(cfg config.PrismConfig, store Store, ak AkshareClient, inst config.PrismInstrument, now time.Time) error {
	id, err := upsertMeta(store, inst)
	if err != nil {
		return err
	}
	start, skip, err := incrementalStart(store, id, cfg.LookbackYears, now)
	if err != nil || skip {
		return err
	}
	var pts []akshare.ValuationPoint
	if inst.Type == "index" {
		pts, err = ak.FetchIndexValuationSeries(inst.Symbol, start, now)
	} else {
		pts, err = ak.FetchStockValuationSeries(inst.Symbol, inst.Market, start, now)
	}
	if err != nil {
		return err
	}
	if len(pts) == 0 {
		return nil // 无新数据,零写入
	}

	// 本地分位:历史(剔除 NaN PE)+ 新点合并(同日新值覆盖),升序后整段算 5Y/10Y 分位。
	hist, err := store.Series(inst.Symbol, "")
	if err != nil {
		return fmt.Errorf("read back series: %w", err)
	}
	peByDay := make(map[string]float64, len(hist.Dates)+len(pts))
	for i, ds := range hist.Dates {
		if !math.IsNaN(hist.PETTM[i]) {
			peByDay[ds] = hist.PETTM[i]
		}
	}
	newDays := make(map[string]akshare.ValuationPoint, len(pts))
	for _, p := range pts {
		ds := p.Date.Format("2006-01-02")
		newDays[ds] = p
		if !math.IsNaN(p.PETTM) {
			peByDay[ds] = p.PETTM
		}
	}
	days := make([]string, 0, len(peByDay))
	for ds := range peByDay {
		days = append(days, ds)
	}
	sort.Strings(days)
	dates := make([]time.Time, len(days))
	pe := make([]float64, len(days))
	idx := make(map[string]int, len(days))
	for i, ds := range days {
		d, perr := time.Parse("2006-01-02", ds)
		if perr != nil {
			return fmt.Errorf("bad stored date %q: %w", ds, perr)
		}
		dates[i], pe[i], idx[ds] = d, peByDay[ds], i
	}
	p5 := valuation.RollingPercentile(dates, pe, 5, rollingMinPoints)
	p10 := valuation.RollingPercentile(dates, pe, 10, rollingMinPoints)

	rows := make([]prismstore.ValuationRow, 0, len(newDays))
	for ds, p := range newDays {
		row := prismstore.ValuationRow{
			D: ds, PETTM: p.PETTM, PB: p.PB, PSTTM: p.PSTTM,
			Pctl5Y: math.NaN(), Pctl10Y: math.NaN(),
		}
		if i, ok := idx[ds]; ok { // PE 为 NaN 的新点不在分位序列中,保持 NaN
			row.Pctl5Y, row.Pctl10Y = p5[i], p10[i]
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].D < rows[j].D })
	return store.UpsertValuations(id, rows)
}
