// Package prism orchestrates the daily Prism valuation refresh (design doc
// docs/prism/atlas_prism_design.md §9 M1).
package prism

import (
	"fmt"
	"time"

	"github.com/newthinker/atlas/internal/collector/lixinger"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/core"
	prismstore "github.com/newthinker/atlas/internal/storage/prism"
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

// refreshEngine is filled in by the US-engine task (TASK-006); keeping the stub
// here lets the lixinger path land independently.
func refreshEngine(cfg config.PrismConfig, store Store, us USClient, inst config.PrismInstrument, now time.Time) error {
	return fmt.Errorf("engine source not implemented yet")
}
