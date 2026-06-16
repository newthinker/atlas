# Per-Market Fundamentals Adapter Contract

This document describes how each market produces the normalized fundamentals CSV
consumed by `fundamentals.parse_dir()` and written atomically into
`fundamentals_pit` alongside OHLCV data.

## Normalized CSV Contract

All fundamentals CSV files must use the following fixed header:

```
symbol,report_period,observe_date,eps_ttm,pe,pb,ps,roe,dividend_yield
```

Column rules:
- `symbol`: ticker, uppercase (e.g. `AAPL`, `600519`)
- `report_period`: quarter-end date in `YYYY-MM-DD` (e.g. 2024-Q1 → `2024-03-31`)
- `observe_date`: the date the value became **publicly known** (filing/disclosure
  date), `YYYY-MM-DD`; must be >= `report_period`; this is the PIT anchor that
  eliminates look-ahead bias
- `eps_ttm`: trailing-twelve-months diluted EPS; **mandatory** — rows with an
  empty `eps_ttm` are skipped by `parse_file` and never enter the warehouse
- `pe`, `pb`, `ps`, `roe`, `dividend_yield`: optional; empty string is accepted
  and stored as NULL

One CSV file per symbol, named `<symbol>.csv` (lowercase filename is fine;
`parse_file` uppercases the `symbol` column value).

---

## A-Share (recommended — qlib native PIT)

**Source:** qlib `dump_pit` / financial data export with report period and first
disclosure date.

- `observe_date` = first public disclosure date (披露日) from the qlib PIT dataset
- `eps_ttm` = rolling four-quarter diluted EPS (稀释 EPS TTM)
- This is the most accurate PIT source because qlib records the actual filing
  date, not the report period end.

Command skeleton (adjust paths to your qlib data directory):

```bash
# Example: export A-share PIT fundamentals from qlib
QLIB_DATA_DIR=~/.qlib/qlib_data/atlas_cn
python - <<'EOF'
import qlib
from qlib.data import D
qlib.init(provider_uri=QLIB_DATA_DIR)
# Use qlib's PIT financial API to export eps_ttm per symbol
# Output: fundamentals_csv_cn/<symbol>.csv following the contract above
EOF
```

Output directory: `fundamentals_csv_cn/` (one CSV per symbol).

---

## US Equities (Yahoo — best-effort approximate PIT)

**Source:** Yahoo Finance `trailingDilutedEPS` / quarterly earnings history.

**Limitation:** Yahoo only provides `asOfDate` (the report-period end date), not
the actual SEC filing date. The true 10-Q/10-K filing typically lags the quarter
end by 30–60 days.

**Approximation:** `observe_date = asOfDate + 45 days` (empirical median lag for
US quarterly filings). This is materially better than using `report_period` itself
(which is the period end, guaranteed to precede the filing), but is not exact PIT.

Mark the approximation in the CSV by adding a comment or `source` column if your
adapter supports it. The warehouse schema does not store `source` per row, so the
distinction lives only in the adapter documentation.

Adapter skeleton:

```python
# scripts/adapters/fetch_us_fundamentals.py  (not yet implemented)
# Uses yfinance / yahoo_fin to pull trailingDilutedEPS history per symbol,
# computes observe_date = asOfDate + timedelta(days=45), writes contract CSV.
```

Output directory: `fundamentals_csv_us/` (one CSV per symbol).

Makefile variable: `FUNDAMENTALS_US_DIR ?= fundamentals_csv_us`

---

## HK Equities (lixinger — best-effort)

**Source:** lixinger fundamentals API (already integrated in atlas via
`internal/collector/lixinger`).

- `observe_date` = the data point date returned by the lixinger API
- `eps_ttm` = EPS TTM from lixinger response
- `pe`, `pb`, etc. populated from the corresponding lixinger fields where
  available

Adapter skeleton:

```python
# scripts/adapters/fetch_hk_fundamentals.py  (not yet implemented)
# Calls atlas lixinger collector (or lixinger HTTP API directly) per symbol,
# maps response fields to contract columns, writes fundamentals_csv_hk/<symbol>.csv
```

Output directory: `fundamentals_csv_hk/` (one CSV per symbol).

---

## Fallback Behavior (missing fundamentals)

If a symbol has no CSV in the fundamentals directory (or the directory is not
passed to `--fundamentals-dir`), the warehouse simply contains no
`fundamentals_pit` rows for that symbol. The Go `qlibpit.Source` detects this via
`hasFundamentals()` and automatically delegates to the configured fallback EPS
source (Yahoo), with zero change to valuation output. No action needed on the
Python side.

## Consumed columns

The current Go consumer (`qlibpit.Source`) reads **only `eps_ttm`** (for PE
percentile reconstruction). The other normalized columns — `pe`, `pb`, `ps`,
`roe`, `dividend_yield` — are persisted to `fundamentals_pit` for forward
compatibility but are **not consumed** this phase. Adapters should still
populate them when cheaply available, but a CSV with only the required columns
plus `eps_ttm` is fully functional today.

---

## Lookback modes

The `lookback_years` parameter (in `config.yaml` under `valuation.lookback_years`
or per-strategy `params.lookback_years`) controls the historical window used for
price and PE percentile calculations.

| Value | Meaning |
|-------|---------|
| `> 0` | Rolling N-year window (e.g. `5` = trailing 5 years) |
| `0`   | Since inception — use all available history from the earliest data point |

### Per-path capability table

| Data path | Price percentile | PE percentile | Effective history |
|-----------|-----------------|---------------|-------------------|
| Price (all markets via qlib_csv) | Full market history | N/A | True since-IPO if `export-ohlcv --from 1970-01-01` used (see below) |
| US/HK individual equities — PE via Yahoo EPS rebuild | N/A | Full history | True since-IPO; Yahoo provides complete quarterly EPS history |
| A-share individual stocks + all indices — PE via lixinger | N/A | Capped at 10 years | lixinger API maximum lookback is `y10` (10 years); `lookback_years: 0` is equivalent to "at most 10 years" for these symbols, not true full history |

### Data prerequisite for full-history price percentile

By default, `qlib_csv_us/` is populated by `make qlib-data-us` which uses
`SIGNAL_FROM` (default `2021-01-01`), giving only ~5 years of price history.

To enable true since-inception price percentile for US equities, regenerate the
CSV directory with the full-history start date before running `make warehouse-dump`:

```bash
# Step 1: rebuild qlib_csv_us with full history
./bin/atlas export-ohlcv --from $(WAREHOUSE_FROM) --market us --out-dir qlib_csv_us
# WAREHOUSE_FROM defaults to 1970-01-01 (set in Makefile)

# Step 2: build the warehouse
make warehouse-dump
```

`SIGNAL_FROM` is intentionally left unchanged so that `make signal-eval-us` and
related targets continue to operate on their existing date range.
