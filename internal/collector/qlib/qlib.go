// Package qlib provides a read-only collector backed by the local SQLite
// data warehouse produced by scripts/qlib_warehouse. It serves historical
// OHLCV from the warehouse and delegates the fresh tail / realtime quote to
// an external collector. Fully degradable: if the DB is missing the caller
// simply does not register this collector.
package qlib

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

// ExternalSelector returns the external collector to use for tail-fill /
// realtime for a symbol (must never return the qlib collector itself).
type ExternalSelector func(symbol string) collector.Collector

// Collector reads historical OHLCV from the local SQLite warehouse.
type Collector struct {
	db           *sql.DB
	maxStaleness time.Duration
	now          func() time.Time
	external     ExternalSelector
}

// Option configures a Collector.
type Option func(*Collector)

// WithMaxStaleness sets how old last_date may be before a warning is logged.
func WithMaxStaleness(d time.Duration) Option { return func(c *Collector) { c.maxStaleness = d } }

// WithClock overrides the time source (testing).
func WithClock(f func() time.Time) Option { return func(c *Collector) { c.now = f } }

// WithExternal sets the tail-fill / realtime delegate selector.
func WithExternal(s ExternalSelector) Option { return func(c *Collector) { c.external = s } }

// New builds a warehouse collector over an already-open read-only DB.
func New(db *sql.DB, opts ...Option) *Collector {
	c := &Collector{db: db, maxStaleness: 7 * 24 * time.Hour, now: time.Now}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Name implements collector.Collector.
func (c *Collector) Name() string { return "qlib" }

// SupportedMarkets implements collector.Collector (warehouse is market-agnostic).
func (c *Collector) SupportedMarkets() []core.Market {
	return []core.Market{core.MarketUS, core.MarketCNA, core.MarketHK}
}

// Init/Start/Stop are no-ops; the collector is constructed via New.
func (c *Collector) Init(cfg collector.Config) error      { return nil }
func (c *Collector) Start(_ context.Context) error        { return nil }
func (c *Collector) Stop() error                          { return nil }

// Covers reports whether the warehouse has data for symbol.
func (c *Collector) Covers(symbol string) bool {
	var n int
	err := c.db.QueryRow(
		"SELECT COUNT(*) FROM warehouse_meta WHERE symbol=?", strings.ToUpper(symbol),
	).Scan(&n)
	return err == nil && n > 0
}

const dateFmt = "2006-01-02"

// lastDate returns the warehouse coverage end for symbol; ok=false if absent.
func (c *Collector) lastDate(symbol string) (time.Time, bool) {
	var s string
	err := c.db.QueryRow(
		"SELECT last_date FROM warehouse_meta WHERE symbol=?", symbol,
	).Scan(&s)
	if err != nil {
		return time.Time{}, false
	}
	t, perr := time.Parse(dateFmt, s)
	return t, perr == nil
}

// readRange reads warehouse ohlcv bars in [start,end] inclusive.
func (c *Collector) readRange(symbol string, start, end time.Time) ([]core.OHLCV, error) {
	rows, err := c.db.Query(
		"SELECT date,open,high,low,close,volume FROM ohlcv WHERE symbol=? AND date>=? AND date<=? ORDER BY date",
		symbol, start.Format(dateFmt), end.Format(dateFmt),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.OHLCV
	for rows.Next() {
		var ds string
		var o, h, l, cl float64
		var vol int64
		if err := rows.Scan(&ds, &o, &h, &l, &cl, &vol); err != nil {
			return nil, err
		}
		ts, _ := time.Parse(dateFmt, ds)
		out = append(out, core.OHLCV{
			Symbol: symbol, Interval: "1d",
			Open: o, High: h, Low: l, Close: cl, Volume: vol, Time: ts,
		})
	}
	return out, rows.Err()
}

// FetchHistory serves daily history warehouse-first with optional tail-fill.
func (c *Collector) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	symbol = strings.ToUpper(symbol)
	// Non-daily intervals: delegate entirely to the external source.
	if interval != "" && interval != "1d" {
		if c.external != nil {
			if ext := c.external(symbol); ext != nil {
				return ext.FetchHistory(symbol, start, end, interval)
			}
		}
		return nil, fmt.Errorf("qlib: warehouse only stores daily bars")
	}
	last, ok := c.lastDate(symbol)
	if !ok {
		return nil, fmt.Errorf("qlib: symbol %s not in warehouse", symbol)
	}
	whEnd := end
	if last.Before(end) {
		whEnd = last
	}
	bars, err := c.readRange(symbol, start, whEnd)
	if err != nil {
		return nil, err
	}
	// Staleness warning (still returns data, fully degradable).
	if c.now().Sub(last) > c.maxStaleness {
		log.Printf("qlib: warehouse stale for %s (last_date=%s)", symbol, last.Format(dateFmt))
	}
	// Append fresh tail: only when request end exceeds warehouse coverage and external is set.
	if end.After(last) && c.external != nil {
		if ext := c.external(symbol); ext != nil {
			tail, terr := ext.FetchHistory(symbol, last.AddDate(0, 0, 1), end, interval)
			if terr != nil {
				log.Printf("qlib: tail-fill failed for %s, serving warehouse only: %v", symbol, terr)
			} else {
				bars = append(bars, tail...)
			}
		}
	}
	return bars, nil
}

// FetchQuote delegates to the external source (warehouse holds no realtime).
func (c *Collector) FetchQuote(symbol string) (*core.Quote, error) {
	if c.external != nil {
		if ext := c.external(symbol); ext != nil {
			return ext.FetchQuote(symbol)
		}
	}
	return nil, fmt.Errorf("qlib: no realtime source for %s", symbol)
}
