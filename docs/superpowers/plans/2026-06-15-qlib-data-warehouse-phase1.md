# Qlib 数据仓库 第一期（Schema + Python dump 管线 + Part A collector）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立本地 SQLite 历史行情仓库，让 atlas 的 `FetchHistory` 以仓库为权威主源、外部 API 仅补新鲜尾巴，降低对 Yahoo/eastmoney 的实时依赖。

**Architecture:** Python dump 管线（仅 stdlib）读取方向① 已产出的 `qlib_csv_*` per-instrument CSV，归一化后原子写入统一 SQLite（`ohlcv` + `warehouse_meta` 两表，`fundamentals_pit` 表建空留待第二期）。atlas 新增 `internal/collector/qlib` collector，只读打开该库，实现「仓库主源 + 外部 API 补尾 + 完全可降级」。

**Tech Stack:** Python 3.11（stdlib `sqlite3`/`csv`，pytest）；Go 1.24（`modernc.org/sqlite` 纯 Go 驱动，无 cgo）。

> 关联 spec：`docs/superpowers/specs/2026-06-15-qlib-data-warehouse-design.md`
> 第二期（Part B PIT 基本面源）不在本计划内。

---

## 文件结构

**Python 侧（新建 `scripts/qlib_warehouse/`）：**
- `scripts/qlib_warehouse/schema.py` — SQLite DDL 常量（建表）
- `scripts/qlib_warehouse/ingest.py` — 从 `qlib_csv_*` CSV 解析、归一化为 OHLCV 行
- `scripts/qlib_warehouse/writer.py` — 原子写 SQLite（临时库 → `os.replace`）+ 写 `warehouse_meta`
- `scripts/qlib_warehouse/build_warehouse.py` — CLI 入口，串起 ingest + writer
- `scripts/qlib_warehouse/tests/test_schema.py`
- `scripts/qlib_warehouse/tests/test_ingest.py`
- `scripts/qlib_warehouse/tests/test_writer.py`
- `scripts/qlib_warehouse/tests/test_build_warehouse.py`

**Go 侧（新建 `internal/collector/qlib/`）：**
- `internal/collector/qlib/qlib.go` — collector 实现（DB 打开、`Covers`、`FetchHistory`、委托）
- `internal/collector/qlib/qlib_test.go`
- 修改 `internal/collector/selector.go` — 新增 `SelectExternalForSymbol`，`SelectForSymbol` 优先 qlib
- 修改 `internal/config/config.go` — 新增 `QlibConfig`
- 修改 `cmd/atlas/serve.go` — 装配 qlib collector
- 修改 `Makefile` — 新增 `warehouse-dump` target

**Python 复用约定**：`QLIB_PY = scripts/qlib_eval/.venv/bin/python`（系统 python3 已损坏，统一走此 venv）。本期 Python 仅用 stdlib，无需新增 pip 依赖。

**SQLite schema（最终形态，全表建好，本期只填 `ohlcv`/`warehouse_meta`）：**
```sql
CREATE TABLE IF NOT EXISTS ohlcv (
  symbol TEXT NOT NULL, date TEXT NOT NULL,
  open REAL, high REAL, low REAL, close REAL,
  volume INTEGER, adj_close REAL,
  PRIMARY KEY (symbol, date)
);
CREATE TABLE IF NOT EXISTS fundamentals_pit (
  symbol TEXT NOT NULL, report_period TEXT NOT NULL, observe_date TEXT NOT NULL,
  eps_ttm REAL, pe REAL, pb REAL, ps REAL, roe REAL, dividend_yield REAL,
  PRIMARY KEY (symbol, report_period, observe_date)
);
CREATE TABLE IF NOT EXISTS warehouse_meta (
  symbol TEXT PRIMARY KEY, market TEXT NOT NULL, source TEXT NOT NULL,
  last_date TEXT NOT NULL, dumped_at TEXT NOT NULL
);
```
现有 CSV 表头：`symbol,date,open,high,low,close,volume,factor`；`adj_close = close * factor`。

---

## Task 1: SQLite schema 模块（Python）

**Files:**
- Create: `scripts/qlib_warehouse/__init__.py`（空文件）
- Create: `scripts/qlib_warehouse/schema.py`
- Test: `scripts/qlib_warehouse/tests/__init__.py`（空文件）, `scripts/qlib_warehouse/tests/test_schema.py`

- [ ] **Step 1: 写失败测试**

`scripts/qlib_warehouse/tests/test_schema.py`:
```python
import sqlite3
from scripts.qlib_warehouse import schema


def test_apply_creates_all_tables():
    conn = sqlite3.connect(":memory:")
    schema.apply(conn)
    names = {r[0] for r in conn.execute(
        "SELECT name FROM sqlite_master WHERE type='table'"
    )}
    assert {"ohlcv", "fundamentals_pit", "warehouse_meta"} <= names


def test_ohlcv_primary_key_is_symbol_date():
    conn = sqlite3.connect(":memory:")
    schema.apply(conn)
    conn.execute("INSERT INTO ohlcv(symbol,date,close) VALUES('AAPL','2024-01-02',1.0)")
    # 同 (symbol,date) 再插入应因主键冲突失败
    import pytest
    with pytest.raises(sqlite3.IntegrityError):
        conn.execute("INSERT INTO ohlcv(symbol,date,close) VALUES('AAPL','2024-01-02',2.0)")
```

- [ ] **Step 2: 运行测试确认失败**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_warehouse/tests/test_schema.py -v`
Expected: FAIL（`ModuleNotFoundError: scripts.qlib_warehouse.schema`）

- [ ] **Step 3: 实现 schema.py**

`scripts/qlib_warehouse/schema.py`:
```python
"""SQLite schema for the qlib local data warehouse."""
import sqlite3

DDL = """
CREATE TABLE IF NOT EXISTS ohlcv (
  symbol TEXT NOT NULL, date TEXT NOT NULL,
  open REAL, high REAL, low REAL, close REAL,
  volume INTEGER, adj_close REAL,
  PRIMARY KEY (symbol, date)
);
CREATE TABLE IF NOT EXISTS fundamentals_pit (
  symbol TEXT NOT NULL, report_period TEXT NOT NULL, observe_date TEXT NOT NULL,
  eps_ttm REAL, pe REAL, pb REAL, ps REAL, roe REAL, dividend_yield REAL,
  PRIMARY KEY (symbol, report_period, observe_date)
);
CREATE TABLE IF NOT EXISTS warehouse_meta (
  symbol TEXT PRIMARY KEY, market TEXT NOT NULL, source TEXT NOT NULL,
  last_date TEXT NOT NULL, dumped_at TEXT NOT NULL
);
"""


def apply(conn: sqlite3.Connection) -> None:
    """Create all warehouse tables if absent."""
    conn.executescript(DDL)
    conn.commit()
```
同时创建空文件 `scripts/qlib_warehouse/__init__.py` 与 `scripts/qlib_warehouse/tests/__init__.py`。

- [ ] **Step 4: 运行测试确认通过**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_warehouse/tests/test_schema.py -v`
Expected: PASS（2 passed）

- [ ] **Step 5: 提交**

```bash
git add scripts/qlib_warehouse/__init__.py scripts/qlib_warehouse/schema.py scripts/qlib_warehouse/tests/__init__.py scripts/qlib_warehouse/tests/test_schema.py
git commit -m "feat(warehouse): SQLite schema module"
```

---

## Task 2: CSV 摄取与归一化（Python）

**Files:**
- Create: `scripts/qlib_warehouse/ingest.py`
- Test: `scripts/qlib_warehouse/tests/test_ingest.py`

数据模型：`OhlcvRow = namedtuple("OhlcvRow", "symbol date open high low close volume adj_close")`。
`adj_close = close * factor`；缺 `factor` 列时 `adj_close = close`。`symbol` 统一大写。

- [ ] **Step 1: 写失败测试**

`scripts/qlib_warehouse/tests/test_ingest.py`:
```python
import textwrap
from scripts.qlib_warehouse import ingest


def _write(tmp_path, name, content):
    p = tmp_path / name
    p.write_text(textwrap.dedent(content).lstrip())
    return p


def test_parse_csv_computes_adj_close_and_uppercases(tmp_path):
    _write(tmp_path, "aapl.csv", """
        symbol,date,open,high,low,close,volume,factor
        aapl,2021-01-04,133.52,133.61,126.76,129.41,143301900,2
    """)
    rows = ingest.parse_dir(tmp_path)
    assert len(rows) == 1
    r = rows[0]
    assert r.symbol == "AAPL"
    assert r.date == "2021-01-04"
    assert r.close == 129.41
    assert r.volume == 143301900
    assert r.adj_close == 129.41 * 2


def test_parse_csv_without_factor_defaults_adj_close_to_close(tmp_path):
    _write(tmp_path, "x.csv", """
        symbol,date,open,high,low,close,volume
        x,2024-01-02,1,2,0.5,1.5,100
    """)
    rows = ingest.parse_dir(tmp_path)
    assert rows[0].adj_close == 1.5


def test_parse_dir_skips_non_csv(tmp_path):
    (tmp_path / "README.md").write_text("not csv")
    assert ingest.parse_dir(tmp_path) == []
```

- [ ] **Step 2: 运行测试确认失败**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_warehouse/tests/test_ingest.py -v`
Expected: FAIL（`ModuleNotFoundError`）

- [ ] **Step 3: 实现 ingest.py**

`scripts/qlib_warehouse/ingest.py`:
```python
"""Parse qlib_csv_* per-instrument CSV into normalized OHLCV rows."""
import csv
from collections import namedtuple
from pathlib import Path
from typing import List

OhlcvRow = namedtuple("OhlcvRow", "symbol date open high low close volume adj_close")


def _f(v):
    return float(v) if v not in (None, "") else None


def parse_file(path: Path) -> List[OhlcvRow]:
    rows = []
    with open(path, newline="") as fh:
        for d in csv.DictReader(fh):
            close = _f(d.get("close"))
            factor = _f(d.get("factor"))
            adj = close * factor if (close is not None and factor is not None) else close
            rows.append(OhlcvRow(
                symbol=d["symbol"].strip().upper(),
                date=d["date"].strip(),
                open=_f(d.get("open")), high=_f(d.get("high")),
                low=_f(d.get("low")), close=close,
                volume=int(float(d["volume"])) if d.get("volume") else None,
                adj_close=adj,
            ))
    return rows


def parse_dir(directory) -> List[OhlcvRow]:
    """Parse every *.csv in a directory into OHLCV rows."""
    out = []
    for p in sorted(Path(directory).glob("*.csv")):
        out.extend(parse_file(p))
    return out
```

- [ ] **Step 4: 运行测试确认通过**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_warehouse/tests/test_ingest.py -v`
Expected: PASS（3 passed）

- [ ] **Step 5: 提交**

```bash
git add scripts/qlib_warehouse/ingest.py scripts/qlib_warehouse/tests/test_ingest.py
git commit -m "feat(warehouse): CSV ingest and normalization"
```

---

## Task 3: 原子 SQLite 写入器（Python）

**Files:**
- Create: `scripts/qlib_warehouse/writer.py`
- Test: `scripts/qlib_warehouse/tests/test_writer.py`

`write(db_path, rows, market, source, dumped_at)`：写入临时库 `<db_path>.tmp` 再 `os.replace` 覆盖目标，保证 atlas 不会读到半成品。`warehouse_meta.last_date` = 每 symbol 的最大 `date`。`dumped_at` 由调用方传入（便于测试，不在库内取系统时间）。

- [ ] **Step 1: 写失败测试**

`scripts/qlib_warehouse/tests/test_writer.py`:
```python
import sqlite3
from scripts.qlib_warehouse import writer
from scripts.qlib_warehouse.ingest import OhlcvRow


def _rows():
    return [
        OhlcvRow("AAPL", "2024-01-02", 1, 2, 0.5, 1.5, 100, 1.5),
        OhlcvRow("AAPL", "2024-01-03", 1, 2, 0.5, 1.6, 110, 1.6),
    ]


def test_write_persists_ohlcv_and_meta(tmp_path):
    db = tmp_path / "w.db"
    writer.write(str(db), _rows(), market="US", source="yahoo",
                 dumped_at="2024-01-04T00:00:00Z")
    conn = sqlite3.connect(str(db))
    assert conn.execute("SELECT COUNT(*) FROM ohlcv").fetchone()[0] == 2
    meta = conn.execute(
        "SELECT market, source, last_date FROM warehouse_meta WHERE symbol='AAPL'"
    ).fetchone()
    assert meta == ("US", "yahoo", "2024-01-03")


def test_write_is_atomic_no_tmp_left(tmp_path):
    db = tmp_path / "w.db"
    writer.write(str(db), _rows(), market="US", source="yahoo",
                 dumped_at="2024-01-04T00:00:00Z")
    assert not (tmp_path / "w.db.tmp").exists()


def test_write_overwrites_existing(tmp_path):
    db = tmp_path / "w.db"
    writer.write(str(db), _rows(), "US", "yahoo", "2024-01-04T00:00:00Z")
    writer.write(str(db), _rows()[:1], "US", "yahoo", "2024-01-05T00:00:00Z")
    conn = sqlite3.connect(str(db))
    assert conn.execute("SELECT COUNT(*) FROM ohlcv").fetchone()[0] == 1
```

- [ ] **Step 2: 运行测试确认失败**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_warehouse/tests/test_writer.py -v`
Expected: FAIL（`ModuleNotFoundError`）

- [ ] **Step 3: 实现 writer.py**

`scripts/qlib_warehouse/writer.py`:
```python
"""Atomic SQLite writer for the warehouse."""
import os
import sqlite3
from typing import Iterable

from . import schema
from .ingest import OhlcvRow


def write(db_path: str, rows: Iterable[OhlcvRow], market: str,
          source: str, dumped_at: str) -> None:
    """Write all rows to a temp DB then atomically replace db_path."""
    rows = list(rows)
    tmp = db_path + ".tmp"
    if os.path.exists(tmp):
        os.remove(tmp)
    conn = sqlite3.connect(tmp)
    try:
        schema.apply(conn)
        conn.executemany(
            "INSERT INTO ohlcv(symbol,date,open,high,low,close,volume,adj_close)"
            " VALUES(?,?,?,?,?,?,?,?)",
            [(r.symbol, r.date, r.open, r.high, r.low, r.close, r.volume, r.adj_close)
             for r in rows],
        )
        last_date = {}
        for r in rows:
            if r.date > last_date.get(r.symbol, ""):
                last_date[r.symbol] = r.date
        conn.executemany(
            "INSERT INTO warehouse_meta(symbol,market,source,last_date,dumped_at)"
            " VALUES(?,?,?,?,?)",
            [(sym, market, source, ld, dumped_at) for sym, ld in last_date.items()],
        )
        conn.commit()
    finally:
        conn.close()
    os.replace(tmp, db_path)
```

- [ ] **Step 4: 运行测试确认通过**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_warehouse/tests/test_writer.py -v`
Expected: PASS（3 passed）

- [ ] **Step 5: 提交**

```bash
git add scripts/qlib_warehouse/writer.py scripts/qlib_warehouse/tests/test_writer.py
git commit -m "feat(warehouse): atomic SQLite writer"
```

---

## Task 4: CLI 入口 build_warehouse.py + Makefile target（Python）

**Files:**
- Create: `scripts/qlib_warehouse/build_warehouse.py`
- Test: `scripts/qlib_warehouse/tests/test_build_warehouse.py`
- Modify: `Makefile`（在 `.PHONY` 行追加 `warehouse-dump`，文件末尾追加 target）

CLI：`build_warehouse.py --csv-dir DIR --market US --source yahoo --db PATH [--dumped-at ISO]`。`--dumped-at` 缺省时用当前 UTC（仅 CLI 层取系统时间，便于测试时显式传入）。

- [ ] **Step 1: 写失败测试**

`scripts/qlib_warehouse/tests/test_build_warehouse.py`:
```python
import sqlite3
import textwrap
from scripts.qlib_warehouse import build_warehouse


def test_main_builds_warehouse_from_csv_dir(tmp_path):
    csv_dir = tmp_path / "csv"
    csv_dir.mkdir()
    (csv_dir / "aapl.csv").write_text(textwrap.dedent("""
        symbol,date,open,high,low,close,volume,factor
        aapl,2024-01-02,1,2,0.5,1.5,100,1
    """).lstrip())
    db = tmp_path / "w.db"
    rc = build_warehouse.main([
        "--csv-dir", str(csv_dir), "--market", "US",
        "--source", "yahoo", "--db", str(db),
        "--dumped-at", "2024-01-03T00:00:00Z",
    ])
    assert rc == 0
    conn = sqlite3.connect(str(db))
    assert conn.execute(
        "SELECT last_date FROM warehouse_meta WHERE symbol='AAPL'"
    ).fetchone()[0] == "2024-01-02"


def test_main_errors_on_missing_csv_dir(tmp_path):
    rc = build_warehouse.main([
        "--csv-dir", str(tmp_path / "nope"), "--market", "US",
        "--source", "yahoo", "--db", str(tmp_path / "w.db"),
    ])
    assert rc != 0
```

- [ ] **Step 2: 运行测试确认失败**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_warehouse/tests/test_build_warehouse.py -v`
Expected: FAIL（`ModuleNotFoundError`）

- [ ] **Step 3: 实现 build_warehouse.py**

`scripts/qlib_warehouse/build_warehouse.py`:
```python
"""CLI: build the SQLite warehouse from a qlib_csv_* directory."""
import argparse
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import List, Optional

from . import ingest, writer


def main(argv: Optional[List[str]] = None) -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--csv-dir", required=True)
    ap.add_argument("--market", required=True)
    ap.add_argument("--source", required=True)
    ap.add_argument("--db", required=True)
    ap.add_argument("--dumped-at", default=None)
    args = ap.parse_args(argv)

    csv_dir = Path(args.csv_dir)
    if not csv_dir.is_dir():
        print(f"csv-dir not found: {csv_dir}", file=sys.stderr)
        return 1

    rows = ingest.parse_dir(csv_dir)
    if not rows:
        print(f"no rows parsed from {csv_dir}", file=sys.stderr)
        return 2

    dumped_at = args.dumped_at or datetime.now(timezone.utc).isoformat()
    writer.write(args.db, rows, market=args.market, source=args.source,
                 dumped_at=dumped_at)
    print(f"wrote {len(rows)} rows to {args.db}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
```

> 注：以 `python -m scripts.qlib_warehouse.build_warehouse` 方式运行，确保包导入路径正确。

- [ ] **Step 4: 运行测试确认通过**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_warehouse/tests/test_build_warehouse.py -v`
Expected: PASS（2 passed）

- [ ] **Step 5: 加 Makefile target**

在 `Makefile` 第 1 行 `.PHONY:` 末尾追加 ` warehouse-dump`，并在文件末尾追加：
```makefile
# 历史行情仓库：从 per-instrument CSV 目录构建本地 SQLite 仓库（仅 stdlib）。
WAREHOUSE_DB ?= data/qlib_warehouse.db
warehouse-dump:
	@mkdir -p $(dir $(WAREHOUSE_DB))
	$(QLIB_PY) -m scripts.qlib_warehouse.build_warehouse \
	  --csv-dir $(QLIB_CSV_US_DIR) --market US --source yahoo --db $(WAREHOUSE_DB)
```

- [ ] **Step 6: 验证 target 可运行 + 提交**

Run: `make warehouse-dump`
Expected: 打印 `wrote N rows to data/qlib_warehouse.db`（N>0），生成 `data/qlib_warehouse.db`
```bash
git add scripts/qlib_warehouse/build_warehouse.py scripts/qlib_warehouse/tests/test_build_warehouse.py Makefile
git commit -m "feat(warehouse): build CLI and make target"
```

---

## Task 5: 引入 SQLite 驱动 + qlib collector 骨架与 Covers（Go）

**Files:**
- Modify: `go.mod`（新增 `modernc.org/sqlite`）
- Create: `internal/collector/qlib/qlib.go`
- Test: `internal/collector/qlib/qlib_test.go`

`Collector` 实现 `collector.Collector` 接口（`Name/SupportedMarkets/Init/Start/Stop/FetchQuote/FetchHistory`）。
构造：`New(db *sql.DB, opts ...Option)`；`Option` 设置 `maxStaleness time.Duration`（默认 7 天）、`now func() time.Time`（默认 `time.Now`）、`external ExternalSelector`。
`Covers(symbol)` 查 `warehouse_meta` 是否存在该 symbol（大写匹配）。

- [ ] **Step 1: 加依赖**

Run: `go get modernc.org/sqlite@latest && go mod tidy`
Expected: `go.mod` 出现 `modernc.org/sqlite`

- [ ] **Step 2: 写失败测试**

`internal/collector/qlib/qlib_test.go`:
```go
package qlib

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestDB builds an in-memory warehouse with the given ohlcv rows + meta.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE ohlcv(symbol TEXT,date TEXT,open REAL,high REAL,low REAL,close REAL,volume INTEGER,adj_close REAL,PRIMARY KEY(symbol,date));
CREATE TABLE warehouse_meta(symbol TEXT PRIMARY KEY,market TEXT,source TEXT,last_date TEXT,dumped_at TEXT);`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestCoversReportsWarehouseMembership(t *testing.T) {
	db := newTestDB(t)
	_, _ = db.Exec("INSERT INTO warehouse_meta VALUES('AAPL','US','yahoo','2024-01-03','x')")
	c := New(db)
	if !c.Covers("aapl") {
		t.Error("expected Covers(aapl)=true (case-insensitive)")
	}
	if c.Covers("MSFT") {
		t.Error("expected Covers(MSFT)=false")
	}
}

func TestNameIsQlib(t *testing.T) {
	if New(newTestDB(t)).Name() != "qlib" {
		t.Error("Name should be qlib")
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/collector/qlib/ -run TestCovers -v`
Expected: FAIL（编译错误：`undefined: New`）

- [ ] **Step 4: 实现骨架 qlib.go**

`internal/collector/qlib/qlib.go`:
```go
// Package qlib provides a read-only collector backed by the local SQLite
// data warehouse produced by scripts/qlib_warehouse. It serves historical
// OHLCV from the warehouse and delegates the fresh tail / realtime quote to
// an external collector. Fully degradable: if the DB is missing the caller
// simply does not register this collector.
package qlib

import (
	"database/sql"
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
func (c *Collector) Init(cfg collector.Config) error { return nil }
func (c *Collector) Start(_ interface{ Done() <-chan struct{} }) error { return nil }
func (c *Collector) Stop() error { return nil }

// Covers reports whether the warehouse has data for symbol.
func (c *Collector) Covers(symbol string) bool {
	var n int
	err := c.db.QueryRow(
		"SELECT COUNT(*) FROM warehouse_meta WHERE symbol=?", strings.ToUpper(symbol),
	).Scan(&n)
	return err == nil && n > 0
}
```

> ⚠ `Start` 的签名必须与 `collector.Collector` 接口一致。先看 `internal/collector/interface.go` 的 `Start(ctx context.Context) error`，按真实签名实现（`import "context"`，`func (c *Collector) Start(ctx context.Context) error { return nil }`）。上面的占位签名仅示意，**以接口真实定义为准**。

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/collector/qlib/ -run 'TestCovers|TestName' -v`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add go.mod go.sum internal/collector/qlib/qlib.go internal/collector/qlib/qlib_test.go
git commit -m "feat(qlib-collector): driver dep, skeleton, Covers"
```

---

## Task 6: FetchHistory 仓库主源读取（Go）

**Files:**
- Modify: `internal/collector/qlib/qlib.go`
- Modify: `internal/collector/qlib/qlib_test.go`

读取 `[start, min(end, last_date)]` 区间的 `ohlcv` 行，映射为 `[]core.OHLCV`（`Interval:"1d"`，`Time` 由 `date` 解析，`Volume` int64）。日期格式 `2006-01-02`。本任务先不做补尾（无 external 时仅返回仓库段）。

- [ ] **Step 1: 写失败测试**

追加到 `qlib_test.go`:
```go
import "time"

func d(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func seedOHLCV(t *testing.T, db *sql.DB) {
	t.Helper()
	_, _ = db.Exec("INSERT INTO warehouse_meta VALUES('AAPL','US','yahoo','2024-01-04','x')")
	for _, r := range [][2]string{{"2024-01-02", "1.0"}, {"2024-01-03", "1.1"}, {"2024-01-04", "1.2"}} {
		_, _ = db.Exec("INSERT INTO ohlcv(symbol,date,open,high,low,close,volume,adj_close) VALUES('AAPL',?,?,?,?,?,?,?)",
			r[0], r[1], r[1], r[1], r[1], 100, r[1])
	}
}

func TestFetchHistoryReadsWarehouseRange(t *testing.T) {
	db := newTestDB(t)
	seedOHLCV(t, db)
	c := New(db, WithClock(func() time.Time { return d("2024-01-05") }))
	got, err := c.FetchHistory("aapl", d("2024-01-02"), d("2024-01-04"), "1d")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 bars, got %d", len(got))
	}
	if got[0].Close != 1.0 || got[2].Close != 1.2 {
		t.Errorf("unexpected close sequence: %+v", got)
	}
	if got[0].Interval != "1d" || got[0].Symbol != "AAPL" {
		t.Errorf("bad metadata: %+v", got[0])
	}
}

func TestFetchHistoryCapsAtLastDate(t *testing.T) {
	db := newTestDB(t)
	seedOHLCV(t, db)
	// end beyond last_date, no external -> only warehouse段 (<=2024-01-04)
	c := New(db, WithClock(func() time.Time { return d("2024-01-10") }))
	got, _ := c.FetchHistory("AAPL", d("2024-01-02"), d("2024-01-09"), "1d")
	if len(got) != 3 {
		t.Fatalf("want 3 (capped at last_date), got %d", len(got))
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/collector/qlib/ -run TestFetchHistory -v`
Expected: FAIL（编译错误：`FetchHistory` 未定义）

- [ ] **Step 3: 实现 FetchHistory（仓库段）**

在 `qlib.go` 追加（`import` 增加 `"fmt"`）：
```go
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

// FetchHistory serves daily history warehouse-first; tail-fill added in next task.
func (c *Collector) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	symbol = strings.ToUpper(symbol)
	last, ok := c.lastDate(symbol)
	if !ok {
		return nil, fmt.Errorf("qlib: symbol %s not in warehouse", symbol)
	}
	whEnd := end
	if last.Before(end) {
		whEnd = last
	}
	return c.readRange(symbol, start, whEnd)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/collector/qlib/ -run TestFetchHistory -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/collector/qlib/qlib.go internal/collector/qlib/qlib_test.go
git commit -m "feat(qlib-collector): FetchHistory warehouse range read"
```

---

## Task 7: 补新鲜尾巴 + 降级 + 陈旧度（Go）

**Files:**
- Modify: `internal/collector/qlib/qlib.go`
- Modify: `internal/collector/qlib/qlib_test.go`

`end > last_date` 且配置了 `external` → 调 `external(symbol).FetchHistory(symbol, last+1d, end, interval)` 拼到仓库段后；外部失败则只返回仓库段（不报错）。`last < now - maxStaleness` → 记 warning（用 `internal/logger`），仍返回数据。

- [ ] **Step 1: 写失败测试（用 fake 外部 collector）**

追加到 `qlib_test.go`:
```go
import (
	"errors"
	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

type fakeExt struct {
	bars []core.OHLCV
	err  error
	got  [2]time.Time // captured start,end
}

func (f *fakeExt) Name() string                       { return "fake" }
func (f *fakeExt) SupportedMarkets() []core.Market    { return nil }
func (f *fakeExt) Init(collector.Config) error        { return nil }
func (f *fakeExt) Start(_ context.Context) error      { return nil }
func (f *fakeExt) Stop() error                        { return nil }
func (f *fakeExt) FetchQuote(string) (*core.Quote, error) { return nil, errors.New("no") }
func (f *fakeExt) FetchHistory(_ string, s, e time.Time, _ string) ([]core.OHLCV, error) {
	f.got = [2]time.Time{s, e}
	return f.bars, f.err
}

func TestFetchHistoryAppendsTail(t *testing.T) {
	db := newTestDB(t)
	seedOHLCV(t, db) // last_date 2024-01-04
	ext := &fakeExt{bars: []core.OHLCV{{Symbol: "AAPL", Interval: "1d", Close: 1.3, Time: d("2024-01-05")}}}
	c := New(db, WithClock(func() time.Time { return d("2024-01-06") }),
		WithExternal(func(string) collector.Collector { return ext }))
	got, err := c.FetchHistory("AAPL", d("2024-01-02"), d("2024-01-05"), "1d")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[3].Close != 1.3 {
		t.Fatalf("want 4 bars incl tail, got %d: %+v", len(got), got)
	}
	if !ext.got[0].Equal(d("2024-01-05")) {
		t.Errorf("tail should start day after last_date, got %v", ext.got[0])
	}
}

func TestFetchHistoryTailFailureDegrades(t *testing.T) {
	db := newTestDB(t)
	seedOHLCV(t, db)
	ext := &fakeExt{err: errors.New("api down")}
	c := New(db, WithClock(func() time.Time { return d("2024-01-06") }),
		WithExternal(func(string) collector.Collector { return ext }))
	got, err := c.FetchHistory("AAPL", d("2024-01-02"), d("2024-01-05"), "1d")
	if err != nil {
		t.Fatalf("tail failure must not error, got %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want warehouse段 only (3), got %d", len(got))
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/collector/qlib/ -run TestFetchHistory -v`
Expected: FAIL（尾巴未拼接，`TestFetchHistoryAppendsTail` 期望 4 实得 3）

- [ ] **Step 3: 实现补尾 + 降级**

修改 `FetchHistory`，替换 Task 6 的实现末段（`return c.readRange(...)` 之前）为：
```go
	bars, err := c.readRange(symbol, start, whEnd)
	if err != nil {
		return nil, err
	}
	// 陈旧度告警（仍返回数据，可降级）。
	if c.now().Sub(last) > c.maxStaleness {
		log.Printf("qlib: warehouse stale for %s (last_date=%s)", symbol, last.Format(dateFmt))
	}
	// 补新鲜尾巴：仅当请求 end 超过仓库覆盖且配置了外部源。
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
```
`import` 增加标准库 `"log"`（与 `internal/collector/eastmoney` 的降级日志风格一致）。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/collector/qlib/ -v`
Expected: PASS（全部）

- [ ] **Step 5: 提交**

```bash
git add internal/collector/qlib/qlib.go internal/collector/qlib/qlib_test.go
git commit -m "feat(qlib-collector): tail-fill, staleness warn, degradation"
```

---

## Task 8: FetchQuote 与非日频委托外部源（Go）

**Files:**
- Modify: `internal/collector/qlib/qlib.go`
- Modify: `internal/collector/qlib/qlib_test.go`

`FetchQuote` 委托 `external(symbol)`；无 external 返回错误。`FetchHistory` 当 `interval` 非 `""`/`"1d"` 时整体委托外部源（仓库只存日线）。

- [ ] **Step 1: 写失败测试**

追加到 `qlib_test.go`:
```go
func TestFetchQuoteDelegates(t *testing.T) {
	db := newTestDB(t)
	q := &core.Quote{Symbol: "AAPL", Price: 9.9}
	ext := &fakeExtQuote{q: q}
	c := New(db, WithExternal(func(string) collector.Collector { return ext }))
	got, err := c.FetchQuote("AAPL")
	if err != nil || got.Price != 9.9 {
		t.Fatalf("expected delegated quote, got %+v err=%v", got, err)
	}
}

func TestFetchHistoryNonDailyDelegates(t *testing.T) {
	db := newTestDB(t)
	seedOHLCV(t, db)
	ext := &fakeExt{bars: []core.OHLCV{{Symbol: "AAPL", Interval: "5m", Close: 7}}}
	c := New(db, WithExternal(func(string) collector.Collector { return ext }))
	got, _ := c.FetchHistory("AAPL", d("2024-01-02"), d("2024-01-04"), "5m")
	if len(got) != 1 || got[0].Interval != "5m" {
		t.Fatalf("non-daily should delegate fully, got %+v", got)
	}
}
```
并添加返回 quote 的 fake：
```go
type fakeExtQuote struct{ fakeExt; q *core.Quote }
func (f *fakeExtQuote) FetchQuote(string) (*core.Quote, error) { return f.q, nil }
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/collector/qlib/ -run 'TestFetchQuote|NonDaily' -v`
Expected: FAIL

- [ ] **Step 3: 实现委托**

`FetchQuote` 实现：
```go
// FetchQuote delegates to the external source (warehouse holds no realtime).
func (c *Collector) FetchQuote(symbol string) (*core.Quote, error) {
	if c.external != nil {
		if ext := c.external(symbol); ext != nil {
			return ext.FetchQuote(symbol)
		}
	}
	return nil, fmt.Errorf("qlib: no realtime source for %s", symbol)
}
```
在 `FetchHistory` 顶部（`symbol = strings.ToUpper(symbol)` 之后）加非日频委托：
```go
	if interval != "" && interval != "1d" {
		if c.external != nil {
			if ext := c.external(symbol); ext != nil {
				return ext.FetchHistory(symbol, start, end, interval)
			}
		}
		return nil, fmt.Errorf("qlib: warehouse only stores daily bars")
	}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/collector/qlib/ -v`
Expected: PASS（全部）

- [ ] **Step 5: 提交**

```bash
git add internal/collector/qlib/qlib.go internal/collector/qlib/qlib_test.go
git commit -m "feat(qlib-collector): delegate quote and intraday to external"
```

---

## Task 9: selector 优先 qlib + SelectExternalForSymbol（Go）

**Files:**
- Modify: `internal/collector/selector.go`
- Modify: `internal/collector/selector_test.go`

把现有 `SelectForSymbol` 的路由体抽成 `SelectExternalForSymbol`（永不返回 qlib）；`SelectForSymbol` 先检查注册表里的 `qlib` 是否 `Covers(symbol)`，是则返回它，否则回落 `SelectExternalForSymbol`。用接口避免 import qlib 包：
```go
type warehouseCoverer interface{ Covers(symbol string) bool }
```

- [ ] **Step 1: 写失败测试**

追加到 `internal/collector/selector_test.go`（参考文件内既有 fake collector 风格；若无则定义最小 fake）：
```go
type coveringCollector struct {
	Collector
	covers map[string]bool
}

func (c coveringCollector) Name() string            { return "qlib" }
func (c coveringCollector) Covers(s string) bool     { return c.covers[s] }

func TestSelectForSymbolPrefersQlibWhenCovered(t *testing.T) {
	reg := NewRegistry()
	reg.Register(coveringCollector{covers: map[string]bool{"AAPL": true}})
	// AAPL covered -> qlib
	if got := SelectForSymbol(reg, "AAPL"); got == nil || got.Name() != "qlib" {
		t.Fatalf("covered symbol should route to qlib, got %v", got)
	}
}

func TestSelectExternalForSymbolNeverReturnsQlib(t *testing.T) {
	reg := NewRegistry()
	reg.Register(coveringCollector{covers: map[string]bool{"AAPL": true}})
	if got := SelectExternalForSymbol(reg, "AAPL"); got != nil && got.Name() == "qlib" {
		t.Fatal("SelectExternalForSymbol must never return qlib")
	}
}
```
> 注：`coveringCollector` 内嵌 `Collector` 接口（零值 nil）仅为满足类型；测试只调用 `Name`/`Covers`，不触发其他方法。若编译器要求所有方法可调用，改为内嵌一个已实现全部方法的最小 stub。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/collector/ -run 'PrefersQlib|NeverReturnsQlib' -v`
Expected: FAIL（`SelectExternalForSymbol` 未定义）

- [ ] **Step 3: 重构 selector.go**

将 `SelectForSymbol` 的整段路由逻辑（`upper := ...` 到结尾的 fallback）原样移入新函数 `SelectExternalForSymbol`，然后把 `SelectForSymbol` 改写为：
```go
// warehouseCoverer is implemented by the qlib warehouse collector.
type warehouseCoverer interface{ Covers(symbol string) bool }

// SelectForSymbol prefers the qlib warehouse collector when it covers the
// symbol, otherwise falls back to external routing.
func SelectForSymbol(reg *Registry, symbol string) Collector {
	if reg == nil {
		return nil
	}
	if c, ok := reg.Get("qlib"); ok {
		if cov, ok2 := c.(warehouseCoverer); ok2 && cov.Covers(symbol) {
			return c
		}
	}
	return SelectExternalForSymbol(reg, symbol)
}

// SelectExternalForSymbol routes to an external (non-qlib) collector.
func SelectExternalForSymbol(reg *Registry, symbol string) Collector {
	if reg == nil {
		return nil
	}
	upper := strings.ToUpper(symbol)
	// ...（此处粘贴原 SelectForSymbol 的 switch + 默认 yahoo + fallback 全部逻辑）
}
```
> ⚠ `SelectExternalForSymbol` 的 fallback「return any available collector」分支需跳过 qlib，否则补尾会递归到自己。在 `GetAll()` 兜底处加：`if c.Name()=="qlib" { continue }`。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/collector/ -v`
Expected: PASS（含既有 selector 测试，验证零回归）

- [ ] **Step 5: 提交**

```bash
git add internal/collector/selector.go internal/collector/selector_test.go
git commit -m "feat(collector): selector prefers qlib warehouse when covered"
```

---

## Task 10: 配置 + serve.go 装配（Go）

**Files:**
- Modify: `internal/config/config.go`（新增 `QlibConfig` + 挂到 `Config`）
- Modify: `cmd/atlas/serve.go`（构造并注册 qlib collector）

- [ ] **Step 1: 加配置结构**

在 `internal/config/config.go` 的 `Config` 结构追加字段：
```go
	Qlib QlibConfig `mapstructure:"qlib"`
```
并新增类型：
```go
// QlibConfig configures the local qlib SQLite data warehouse collector.
type QlibConfig struct {
	Enabled          bool   `mapstructure:"enabled"`
	DBPath           string `mapstructure:"db_path"`
	MaxStalenessDays int    `mapstructure:"max_staleness_days"`
}
```

- [ ] **Step 2: 在 serve.go 装配 qlib collector**

在 `cmd/atlas/serve.go` 中、**所有外部 collector（yahoo/eastmoney/crypto）注册完成之后**，追加：
```go
	// Register the qlib warehouse collector last so it can delegate tail-fill /
	// realtime to the already-registered external collectors. Fully degradable:
	// a missing/unopenable DB simply skips registration.
	if cfg.Qlib.Enabled && cfg.Qlib.DBPath != "" {
		if db, derr := sql.Open("sqlite", "file:"+cfg.Qlib.DBPath+"?mode=ro"); derr != nil {
			log.Warn("qlib warehouse open failed; skipping", zap.Error(derr))
		} else if perr := db.Ping(); perr != nil {
			log.Warn("qlib warehouse not readable; skipping", zap.Error(perr))
			_ = db.Close()
		} else {
			stale := time.Duration(cfg.Qlib.MaxStalenessDays) * 24 * time.Hour
			if cfg.Qlib.MaxStalenessDays == 0 {
				stale = 7 * 24 * time.Hour
			}
			appReg := application.CollectorRegistry() // see Step 3
			qc := qlib.New(db,
				qlib.WithMaxStaleness(stale),
				qlib.WithExternal(func(s string) collector.Collector {
					return collector.SelectExternalForSymbol(appReg, s)
				}),
			)
			application.RegisterCollector(qc)
			log.Info("qlib warehouse collector registered", zap.String("db", cfg.Qlib.DBPath))
		}
	}
```
`import` 增补：`"database/sql"`、`_ "modernc.org/sqlite"`、`"github.com/newthinker/atlas/internal/collector/qlib"`、`"time"`（若未引入）。

- [ ] **Step 3: 暴露 app 的 collector 注册表**

qlib 补尾需要 app 内部 registry。在 `internal/app/app.go` 新增导出方法（紧邻 `RegisterCollector`）：
```go
// CollectorRegistry exposes the collector registry for wiring delegating
// collectors (e.g. the qlib warehouse needs it for external tail-fill).
func (a *App) CollectorRegistry() *collector.Registry { return a.collectors }
```
> 先确认 `a.collectors` 字段名（`grep -n "collectors" internal/app/app.go` → app.go:87 `collectors := collector.NewRegistry()`，字段名以结构体定义为准）。

- [ ] **Step 4: 编译 + 全量测试**

Run: `go build ./... && go test ./internal/... ./cmd/...`
Expected: 编译通过；测试 PASS（无回归）

- [ ] **Step 5: 端到端冒烟（仓库已由 Task 4 生成）**

Run:
```bash
make warehouse-dump
go run ./cmd/atlas serve --help >/dev/null 2>&1 || true
```
手工确认（可选）：在配置中设 `qlib.enabled: true`、`qlib.db_path: data/qlib_warehouse.db` 后启动 serve，日志出现 `qlib warehouse collector registered`。

- [ ] **Step 6: 提交**

```bash
git add internal/config/config.go cmd/atlas/serve.go internal/app/app.go
git commit -m "feat(qlib-collector): config and serve wiring"
```

---

## 完成标准（DoD）

- `make warehouse-dump` 从 `qlib_csv_us` 生成 `data/qlib_warehouse.db`，含 `ohlcv` + `warehouse_meta`
- Python 测试全绿：`scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_warehouse/ -v`
- Go 测试全绿：`go test ./internal/collector/... ./internal/config/... ./cmd/...`
- qlib collector：仓库命中走仓库、超 last_date 补尾、补尾失败降级、缺符号/缺库完全回落，均有测试覆盖
- 关库/缺库时系统行为与现状完全一致（零回归）

## 范围边界（本期不做）

- Part B PIT 基本面源（`fundamentals_pit` 表本期建空、不填、Go 不读）→ 第二期
- A 股 / 港股的 dump（本期 Makefile target 仅接 US；A/HK 可手工调 CLI 验证，正式接入留第二期或随 watchlist 扩展）
- 实时行情入库、分钟频入库（始终委托外部源）
