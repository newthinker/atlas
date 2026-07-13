package crisis

import (
	"context"
	"fmt"
	"time"

	"github.com/newthinker/atlas/internal/collector/fred"
	"github.com/newthinker/atlas/internal/core"
)

// FREDFetcher / HistoryFetcher are the two upstream dependencies, narrowed to
// interfaces so tests inject fakes (fred.Client and yahoo.Yahoo satisfy them).
type FREDFetcher interface {
	FetchSeries(ctx context.Context, seriesID, start, end string) ([]fred.Observation, error)
}

type HistoryFetcher interface {
	FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error)
}

// IngestReport carries per-indicator row counts plus non-fatal yahoo errors:
// MOVE/USDJPY failures degrade to the STALE path instead of blocking FRED
// ingestion (design §2.1).
type IngestReport struct {
	Counts    map[string]int
	YahooErrs map[string]error
}

type Ingestor struct {
	fred  FREDFetcher
	yahoo HistoryFetcher
	store *Store
	now   func() time.Time
}

func NewIngestor(f FREDFetcher, y HistoryFetcher, s *Store) *Ingestor {
	return &Ingestor{fred: f, yahoo: y, store: s, now: time.Now}
}

// fredDirect are the FRED series stored as-is (scale converts percent→bp per
// the canonical-unit table in the implementation plan).
var fredDirect = []struct {
	indicator string
	series    string
	scale     float64
}{
	{IndVIX, "VIXCLS", 1},
	{IndHYOAS, "BAMLH0A0HYM2", 100},
	{IndT10Y2Y, "T10Y2Y", 100},
	{IndNFCI, "NFCI", 1},
}

var yahooSymbols = map[string]string{IndMOVE: "^MOVE", IndUSDJPY: "JPY=X"}

// IngestAll fetches every indicator for [from, to]: FRED errors abort (the
// daily eval retries next wakeup), yahoo errors are collected in the report.
func (ig *Ingestor) IngestAll(ctx context.Context, from, to string) (*IngestReport, error) {
	rep := &IngestReport{Counts: map[string]int{}, YahooErrs: map[string]error{}}
	stamp := NowStamp(ig.now())

	for _, fs := range fredDirect {
		n, err := ig.ingestFredSeries(ctx, fs.series, fs.indicator, fs.scale, from, to, stamp)
		if err != nil {
			return nil, err
		}
		rep.Counts[fs.indicator] = n
	}

	n, err := ig.ingestSpread(ctx, from, to, stamp)
	if err != nil {
		return nil, err
	}
	rep.Counts[IndSOFREFFR] = n

	for ind, sym := range yahooSymbols {
		n, err := ig.ingestYahoo(ctx, ind, sym, from, to, stamp)
		if err != nil {
			rep.YahooErrs[ind] = err
			continue
		}
		rep.Counts[ind] = n
	}
	return rep, nil
}

// IngestNFCI refreshes only the weekly NFCI series (Wednesday plist, design §4.3).
func (ig *Ingestor) IngestNFCI(ctx context.Context, from, to string) (int, error) {
	return ig.ingestFredSeries(ctx, "NFCI", IndNFCI, 1, from, to, NowStamp(ig.now()))
}

func (ig *Ingestor) ingestFredSeries(ctx context.Context, seriesID, indicator string, scale float64, from, to, stamp string) (int, error) {
	obs, err := ig.fred.FetchSeries(ctx, seriesID, from, to)
	if err != nil {
		return 0, fmt.Errorf("fetching %s: %w", seriesID, err)
	}
	rows := make([]Observation, 0, len(obs))
	for _, o := range obs {
		rows = append(rows, Observation{
			Date: o.Date, Indicator: indicator, Value: o.Value * scale,
			Source: "fred", FetchedAt: stamp,
		})
	}
	return ig.upsert(ctx, rows)
}

// upsert stores rows and returns the row count, the shared tail of every
// ingest helper.
func (ig *Ingestor) upsert(ctx context.Context, rows []Observation) (int, error) {
	if err := ig.store.UpsertObservations(ctx, rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}

// ingestSpread joins SOFR and EFFR by date and stores the spread in bp
// (design §2.2; days missing either leg are skipped).
func (ig *Ingestor) ingestSpread(ctx context.Context, from, to, stamp string) (int, error) {
	sofr, err := ig.fred.FetchSeries(ctx, "SOFR", from, to)
	if err != nil {
		return 0, fmt.Errorf("fetching SOFR: %w", err)
	}
	effr, err := ig.fred.FetchSeries(ctx, "EFFR", from, to)
	if err != nil {
		return 0, fmt.Errorf("fetching EFFR: %w", err)
	}
	effrByDate := make(map[string]float64, len(effr))
	for _, o := range effr {
		effrByDate[o.Date] = o.Value
	}
	rows := make([]Observation, 0, len(sofr))
	for _, o := range sofr {
		e, ok := effrByDate[o.Date]
		if !ok {
			continue
		}
		rows = append(rows, Observation{
			Date: o.Date, Indicator: IndSOFREFFR, Value: SpreadBp(o.Value, e),
			Source: "derived", FetchedAt: stamp,
		})
	}
	return ig.upsert(ctx, rows)
}

func (ig *Ingestor) ingestYahoo(ctx context.Context, indicator, symbol, from, to, stamp string) (int, error) {
	start, err := time.Parse(dateLayout, from)
	if err != nil {
		return 0, fmt.Errorf("parsing from %q: %w", from, err)
	}
	end, err := time.Parse(dateLayout, to)
	if err != nil {
		return 0, fmt.Errorf("parsing to %q: %w", to, err)
	}
	// end+1d：yahoo chart 区间对当日收盘的包含性不稳定，多取一天由 upsert 幂等兜底
	bars, err := ig.yahoo.FetchHistory(symbol, start, end.AddDate(0, 0, 1), "1d")
	if err != nil {
		return 0, err
	}
	rows := make([]Observation, 0, len(bars))
	for _, b := range bars {
		rows = append(rows, Observation{
			Date: b.Time.UTC().Format(dateLayout), Indicator: indicator, Value: b.Close,
			Source: "yahoo", FetchedAt: stamp,
		})
	}
	return ig.upsert(ctx, rows)
}
