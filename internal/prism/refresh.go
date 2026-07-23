// Package prism orchestrates the daily Prism valuation refresh (design doc
// docs/prism/atlas_prism_design.md §9 M1).
package prism

import (
	"fmt"
	"math"
	"time"

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
}

// LixingerClient is the subset of *lixinger.Lixinger used by Refresh.
type LixingerClient interface {
	FetchValuationSeries(symbol string, start, end time.Time) ([]lixinger.ValuationPoint, error)
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
}

// Refresh updates every configured instrument: lixinger-sourced instruments
// fetch incrementally (理杏豆计费), engine-sourced US stocks rebuild via yahoo.
func Refresh(cfg config.PrismConfig, store Store, lix LixingerClient, us USClient, now time.Time) Report {
	var rep Report
	for _, inst := range cfg.Instruments {
		var err error
		switch inst.Source {
		case "lixinger":
			err = refreshLixinger(cfg, store, lix, inst, now)
		case "engine":
			err = refreshEngine(cfg, store, us, inst, now)
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

func refreshLixinger(cfg config.PrismConfig, store Store, lix LixingerClient, inst config.PrismInstrument, now time.Time) error {
	id, err := upsertMeta(store, inst)
	if err != nil {
		return err
	}
	latest, err := store.LatestDate(id)
	if err != nil {
		return err
	}
	start := now.AddDate(-cfg.LookbackYears, 0, 0)
	if latest != "" {
		d, perr := time.Parse("2006-01-02", latest)
		if perr != nil {
			return fmt.Errorf("bad latest date %q: %w", latest, perr)
		}
		start = d.AddDate(0, 0, 1) // 增量:从 latest 的次日开始
		if !start.Before(now) {
			return nil // 已是最新,零请求(理杏豆)
		}
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
