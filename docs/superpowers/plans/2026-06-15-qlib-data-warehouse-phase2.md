# Qlib 数据仓库 第二期（Part B：PIT 财务数据库 + atlas EPS 源）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用本地 SQLite 仓库的 PIT 双轴基本面（报告期 × 真实可知日）为 atlas 提供消除前视偏差的 EPS(TTM) 历史，作为 PE 分位重建的权威主源，Yahoo/lixinger 退为兜底。

**Architecture:** Python dump 管线消费一个「归一化基本面 CSV 契约」（`symbol,report_period,observe_date,eps_ttm,...`），与第一期 OHLCV 在**同一次原子写**中落入 `fundamentals_pit` 表。atlas 新增 `internal/collector/qlibpit` 源，实现现有 `app.EPSSource` 接口（`FetchEPSHistory`），用 `observe_date <= 窗口末` 的点对时间查询产出 EPSPoint 序列，喂给未改动的 `valuation.ReconstructPEPercentile`；仓库无该符号基本面时委托内层 Yahoo 源。

**Tech Stack:** Python 3.11（stdlib `sqlite3`/`csv`，pytest）；Go 1.24（`modernc.org/sqlite`，复用第一期已开的只读 DB 句柄）。

> 关联 spec：`docs/superpowers/specs/2026-06-15-qlib-data-warehouse-design.md`（§5 Part B、§3 schema）
> 前置：第一期（`2026-06-15-qlib-data-warehouse-phase1.md`）已完成——`fundamentals_pit` 表已由第一期 schema 建好（建空）、`internal/collector/qlib` 与 `cfg.Qlib`（`enabled`/`db_path`/`max_staleness_days`）已存在。

---

## 关键设计：为什么 PIT 修了前视偏差

现 Yahoo 路径用 `trailingDilutedEPS.asOfDate`（**报告期末日**）当对齐日，把某季度收盘对齐到该季 EPS——但该 EPS 实际要到数周后才公布，构成前视偏差。
Part B 的 `observe_date` 存**真实可知日**（备案/发布日，≠ `report_period` 期末），Go 查询用 `observe_date <= 窗口末` 截断，使「站在某日只能看到当日及之前已公布的数据」。
`ReconstructPEPercentile` 对每个收盘取「≤该日的最近 EPS 点」做阶梯对齐：把 `(observe_date, eps_ttm)` 升序喂入即天然正确，且同一 `report_period` 的修订（更晚 `observe_date`）会在其后的收盘自动接管，更早的收盘仍用旧值——这正是 PIT 正确性。

**归一化基本面 CSV 契约**（`fundamentals_csv/<symbol>.csv`，表头固定）：
```
symbol,report_period,observe_date,eps_ttm,pe,pb,ps,roe,dividend_yield
```
必填：`symbol,report_period,observe_date,eps_ttm`；其余可空。`observe_date`/`report_period` 均 `YYYY-MM-DD`（报告期用季末日表示，如 2024-Q1 → `2024-03-31`）。

---

## 文件结构

**Python 侧（`scripts/qlib_warehouse/`）：**
- Create: `scripts/qlib_warehouse/fundamentals.py` — 解析基本面 CSV → `FundRow`
- Modify: `scripts/qlib_warehouse/writer.py` — `write` 增加可选 `fundamentals` 参数，同一原子写落 `fundamentals_pit`
- Modify: `scripts/qlib_warehouse/build_warehouse.py` — 增加 `--fundamentals-dir`（可选）
- Create: `scripts/qlib_warehouse/tests/test_fundamentals.py`
- Modify: `scripts/qlib_warehouse/tests/test_writer.py`（追加用例）
- Modify: `scripts/qlib_warehouse/tests/test_build_warehouse.py`（追加用例）

**Go 侧：**
- Create: `internal/collector/qlibpit/qlibpit.go` — `EPSSource` 实现（PIT 查询 + 兜底委托）
- Create: `internal/collector/qlibpit/qlibpit_test.go`
- Modify: `cmd/atlas/serve.go` — 用 qlibpit 包装 yahoo EPS 源注入

**适配器（best-effort，单独 Task 文档化，不阻塞主干）：** 各市场如何产出 `fundamentals_csv/`。

---

## Task 1: 基本面 CSV 摄取（Python）

**Files:**
- Create: `scripts/qlib_warehouse/fundamentals.py`
- Test: `scripts/qlib_warehouse/tests/test_fundamentals.py`

`FundRow = namedtuple("FundRow", "symbol report_period observe_date eps_ttm pe pb ps roe dividend_yield")`。
数值列空字符串→`None`；`symbol` 大写。

- [ ] **Step 1: 写失败测试**

`scripts/qlib_warehouse/tests/test_fundamentals.py`:
```python
import textwrap
from scripts.qlib_warehouse import fundamentals


def _write(tmp_path, name, content):
    p = tmp_path / name
    p.write_text(textwrap.dedent(content).lstrip())
    return p


def test_parse_dir_reads_required_and_optional(tmp_path):
    _write(tmp_path, "aapl.csv", """
        symbol,report_period,observe_date,eps_ttm,pe,pb,ps,roe,dividend_yield
        aapl,2024-03-31,2024-05-02,6.42,28.1,,,1.5,0.5
    """)
    rows = fundamentals.parse_dir(tmp_path)
    assert len(rows) == 1
    r = rows[0]
    assert r.symbol == "AAPL"
    assert r.report_period == "2024-03-31"
    assert r.observe_date == "2024-05-02"
    assert r.eps_ttm == 6.42
    assert r.pb is None
    assert r.roe == 1.5


def test_parse_dir_skips_non_csv(tmp_path):
    (tmp_path / "notes.txt").write_text("x")
    assert fundamentals.parse_dir(tmp_path) == []
```

- [ ] **Step 2: 运行测试确认失败**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_warehouse/tests/test_fundamentals.py -v`
Expected: FAIL（`ModuleNotFoundError`）

- [ ] **Step 3: 实现 fundamentals.py**

`scripts/qlib_warehouse/fundamentals.py`:
```python
"""Parse normalized point-in-time fundamentals CSV into rows."""
import csv
from collections import namedtuple
from pathlib import Path
from typing import List

FundRow = namedtuple(
    "FundRow",
    "symbol report_period observe_date eps_ttm pe pb ps roe dividend_yield",
)


def _f(v):
    return float(v) if v not in (None, "") else None


def parse_file(path: Path) -> List[FundRow]:
    rows = []
    with open(path, newline="") as fh:
        for d in csv.DictReader(fh):
            rows.append(FundRow(
                symbol=d["symbol"].strip().upper(),
                report_period=d["report_period"].strip(),
                observe_date=d["observe_date"].strip(),
                eps_ttm=_f(d.get("eps_ttm")),
                pe=_f(d.get("pe")), pb=_f(d.get("pb")), ps=_f(d.get("ps")),
                roe=_f(d.get("roe")), dividend_yield=_f(d.get("dividend_yield")),
            ))
    return rows


def parse_dir(directory) -> List[FundRow]:
    out = []
    for p in sorted(Path(directory).glob("*.csv")):
        out.extend(parse_file(p))
    return out
```

- [ ] **Step 4: 运行测试确认通过**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_warehouse/tests/test_fundamentals.py -v`
Expected: PASS（2 passed）

- [ ] **Step 5: 提交**

```bash
git add scripts/qlib_warehouse/fundamentals.py scripts/qlib_warehouse/tests/test_fundamentals.py
git commit -m "feat(warehouse): point-in-time fundamentals CSV ingest"
```

---

## Task 2: writer 同次原子写入 fundamentals_pit（Python）

**Files:**
- Modify: `scripts/qlib_warehouse/writer.py`
- Modify: `scripts/qlib_warehouse/tests/test_writer.py`

`write(db_path, rows, market, source, dumped_at, fundamentals=None)`：向后兼容（第一期调用不传 `fundamentals` 仍可用）；`fundamentals` 行在同一临时库内一并写 `fundamentals_pit`，原子 `os.replace`。修订（同 `report_period` 不同 `observe_date`）**原样保留**，不去重。

- [ ] **Step 1: 写失败测试（追加到 test_writer.py）**

```python
from scripts.qlib_warehouse.fundamentals import FundRow


def _funds():
    return [
        FundRow("AAPL", "2023-12-31", "2024-03-01", 3.0, None, None, None, None, None),
        FundRow("AAPL", "2024-03-31", "2024-05-15", 4.0, None, None, None, None, None),
        # 修订：同一 report_period 更晚 observe_date
        FundRow("AAPL", "2024-03-31", "2024-08-01", 4.2, None, None, None, None, None),
    ]


def test_write_persists_fundamentals_keeping_revisions(tmp_path):
    db = tmp_path / "w.db"
    writer.write(str(db), _rows(), "US", "yahoo", "2024-09-01T00:00:00Z",
                 fundamentals=_funds())
    conn = sqlite3.connect(str(db))
    assert conn.execute("SELECT COUNT(*) FROM fundamentals_pit").fetchone()[0] == 3
    assert conn.execute("SELECT COUNT(*) FROM ohlcv").fetchone()[0] == 2  # ohlcv 不受影响


def test_write_without_fundamentals_is_backward_compatible(tmp_path):
    db = tmp_path / "w.db"
    writer.write(str(db), _rows(), "US", "yahoo", "2024-09-01T00:00:00Z")
    conn = sqlite3.connect(str(db))
    assert conn.execute("SELECT COUNT(*) FROM fundamentals_pit").fetchone()[0] == 0
```
（`_rows()` 复用第一期 test_writer.py 已有的辅助函数。）

- [ ] **Step 2: 运行测试确认失败**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_warehouse/tests/test_writer.py -v`
Expected: FAIL（`write() got an unexpected keyword argument 'fundamentals'`）

- [ ] **Step 3: 修改 writer.write**

在 `scripts/qlib_warehouse/writer.py` 的 `write` 签名加 `fundamentals=None`，并在 `warehouse_meta` 写入之后、`conn.commit()` 之前插入 fundamentals：
```python
def write(db_path: str, rows: Iterable[OhlcvRow], market: str,
          source: str, dumped_at: str, fundamentals=None) -> None:
    rows = list(rows)
    funds = list(fundamentals or [])
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
        if funds:
            conn.executemany(
                "INSERT INTO fundamentals_pit"
                "(symbol,report_period,observe_date,eps_ttm,pe,pb,ps,roe,dividend_yield)"
                " VALUES(?,?,?,?,?,?,?,?,?)",
                [(f.symbol, f.report_period, f.observe_date, f.eps_ttm,
                  f.pe, f.pb, f.ps, f.roe, f.dividend_yield) for f in funds],
            )
        conn.commit()
    finally:
        conn.close()
    os.replace(tmp, db_path)
```
`from .fundamentals import FundRow` 不强制导入（按 duck-typing 取属性即可）；如需类型提示可加。

- [ ] **Step 4: 运行测试确认通过（含第一期既有用例零回归）**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_warehouse/tests/test_writer.py -v`
Expected: PASS（第一期 3 + 本期 2 = 5 passed）

- [ ] **Step 5: 提交**

```bash
git add scripts/qlib_warehouse/writer.py scripts/qlib_warehouse/tests/test_writer.py
git commit -m "feat(warehouse): writer persists fundamentals_pit atomically"
```

---

## Task 3: build CLI 增加 --fundamentals-dir（Python）

**Files:**
- Modify: `scripts/qlib_warehouse/build_warehouse.py`
- Modify: `scripts/qlib_warehouse/tests/test_build_warehouse.py`

`--fundamentals-dir`（可选）：给定则解析并随 OHLCV 同次写入。

- [ ] **Step 1: 写失败测试（追加）**

```python
def test_main_ingests_fundamentals_when_dir_given(tmp_path):
    csv_dir = tmp_path / "csv"; csv_dir.mkdir()
    (csv_dir / "aapl.csv").write_text(textwrap.dedent("""
        symbol,date,open,high,low,close,volume,factor
        aapl,2024-01-02,1,2,0.5,1.5,100,1
    """).lstrip())
    f_dir = tmp_path / "fund"; f_dir.mkdir()
    (f_dir / "aapl.csv").write_text(textwrap.dedent("""
        symbol,report_period,observe_date,eps_ttm,pe,pb,ps,roe,dividend_yield
        aapl,2023-12-31,2024-02-01,3.0,,,,,
    """).lstrip())
    db = tmp_path / "w.db"
    rc = build_warehouse.main([
        "--csv-dir", str(csv_dir), "--fundamentals-dir", str(f_dir),
        "--market", "US", "--source", "yahoo", "--db", str(db),
        "--dumped-at", "2024-03-01T00:00:00Z",
    ])
    assert rc == 0
    conn = sqlite3.connect(str(db))
    assert conn.execute("SELECT eps_ttm FROM fundamentals_pit WHERE symbol='AAPL'").fetchone()[0] == 3.0
```

- [ ] **Step 2: 运行测试确认失败**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_warehouse/tests/test_build_warehouse.py -v`
Expected: FAIL（`unrecognized arguments: --fundamentals-dir`）

- [ ] **Step 3: 修改 build_warehouse.main**

在 `argparse` 增 `ap.add_argument("--fundamentals-dir", default=None)`，并在 `writer.write(...)` 前解析：
```python
    from . import fundamentals  # local import to keep ingest path lazy
    funds = None
    if args.fundamentals_dir:
        fdir = Path(args.fundamentals_dir)
        if not fdir.is_dir():
            print(f"fundamentals-dir not found: {fdir}", file=sys.stderr)
            return 3
        funds = fundamentals.parse_dir(fdir)
    dumped_at = args.dumped_at or datetime.now(timezone.utc).isoformat()
    writer.write(args.db, rows, market=args.market, source=args.source,
                 dumped_at=dumped_at, fundamentals=funds)
```
（替换原先无 `fundamentals=` 的 `writer.write(...)` 调用。）

- [ ] **Step 4: 运行测试确认通过**

Run: `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_warehouse/ -v`
Expected: PASS（全部，含第一期）

- [ ] **Step 5: 提交**

```bash
git add scripts/qlib_warehouse/build_warehouse.py scripts/qlib_warehouse/tests/test_build_warehouse.py
git commit -m "feat(warehouse): build CLI ingests fundamentals_pit"
```

---

## Task 4: qlibpit EPS 源 — PIT 点对时间查询（Go）

**Files:**
- Create: `internal/collector/qlibpit/qlibpit.go`
- Test: `internal/collector/qlibpit/qlibpit_test.go`

实现 `FetchEPSHistory(symbol, start, end) ([]core.EPSPoint, error)`：查 `fundamentals_pit` 中 `observe_date <= end` 且 `eps_ttm` 非空的行，按 `observe_date` 升序，映射为 `EPSPoint{Date: observe_date, EPS: eps_ttm}`。`end` 截断防前视。本任务先不接兜底。

- [ ] **Step 1: 写失败测试**

`internal/collector/qlibpit/qlibpit_test.go`:
```go
package qlibpit

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func d(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE fundamentals_pit(
symbol TEXT,report_period TEXT,observe_date TEXT,eps_ttm REAL,
pe REAL,pb REAL,ps REAL,roe REAL,dividend_yield REAL,
PRIMARY KEY(symbol,report_period,observe_date));`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func ins(t *testing.T, db *sql.DB, sym, rp, od string, eps float64) {
	t.Helper()
	if _, err := db.Exec(
		"INSERT INTO fundamentals_pit(symbol,report_period,observe_date,eps_ttm) VALUES(?,?,?,?)",
		sym, rp, od, eps); err != nil {
		t.Fatal(err)
	}
}

func TestFetchEPSHistoryExcludesFutureObserveDate(t *testing.T) {
	db := newTestDB(t)
	ins(t, db, "AAPL", "2023-12-31", "2024-03-01", 3.0)
	ins(t, db, "AAPL", "2024-03-31", "2024-05-15", 4.0) // 未来：end=2024-04-01 不可见
	src := New(db, nil)
	got, err := src.FetchEPSHistory("aapl", d("2020-01-01"), d("2024-04-01"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].EPS != 3.0 || !got[0].Date.Equal(d("2024-03-01")) {
		t.Fatalf("lookahead leak: %+v", got)
	}
}

func TestFetchEPSHistoryKeepsRevisionsOrdered(t *testing.T) {
	db := newTestDB(t)
	ins(t, db, "AAPL", "2024-03-31", "2024-05-15", 4.0)
	ins(t, db, "AAPL", "2024-03-31", "2024-08-01", 4.2) // 修订
	src := New(db, nil)
	got, _ := src.FetchEPSHistory("AAPL", d("2020-01-01"), d("2024-12-31"))
	if len(got) != 2 || got[0].EPS != 4.0 || got[1].EPS != 4.2 {
		t.Fatalf("revisions must be kept ascending by observe_date: %+v", got)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/collector/qlibpit/ -v`
Expected: FAIL（`undefined: New`）

- [ ] **Step 3: 实现 qlibpit.go**

`internal/collector/qlibpit/qlibpit.go`:
```go
// Package qlibpit serves point-in-time EPS(TTM) history from the local SQLite
// warehouse (fundamentals_pit). It implements app.EPSSource. Each row's
// observe_date is the date the value became publicly known, so querying
// observe_date <= window-end eliminates the look-ahead bias present in the
// report-period-dated Yahoo path. Falls back to an inner EPSSource when the
// warehouse has no fundamentals for a symbol.
package qlibpit

import (
	"database/sql"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

const dateFmt = "2006-01-02"

// EPSSource is the fallback shape (satisfied by *yahoo.Yahoo); mirrors
// app.EPSSource without importing the app package.
type EPSSource interface {
	FetchEPSHistory(symbol string, start, end time.Time) ([]core.EPSPoint, error)
}

// Source reads PIT EPS history from the warehouse, delegating to a fallback
// when a symbol has no fundamentals stored.
type Source struct {
	db       *sql.DB
	fallback EPSSource
}

// New builds a PIT EPS source. fallback may be nil.
func New(db *sql.DB, fallback EPSSource) *Source {
	return &Source{db: db, fallback: fallback}
}

func (s *Source) hasFundamentals(symbol string) bool {
	var n int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM fundamentals_pit WHERE symbol=?", symbol,
	).Scan(&n)
	return err == nil && n > 0
}

// FetchEPSHistory returns the PIT EPS(TTM) series for symbol, ascending by
// observe_date, including only points knowable on or before end.
func (s *Source) FetchEPSHistory(symbol string, start, end time.Time) ([]core.EPSPoint, error) {
	symbol = strings.ToUpper(symbol)
	if !s.hasFundamentals(symbol) {
		if s.fallback != nil {
			return s.fallback.FetchEPSHistory(symbol, start, end)
		}
		return []core.EPSPoint{}, nil
	}
	rows, err := s.db.Query(
		"SELECT observe_date,eps_ttm FROM fundamentals_pit "+
			"WHERE symbol=? AND observe_date<=? AND eps_ttm IS NOT NULL ORDER BY observe_date",
		symbol, end.Format(dateFmt),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.EPSPoint
	for rows.Next() {
		var od string
		var eps float64
		if err := rows.Scan(&od, &eps); err != nil {
			return nil, err
		}
		t, perr := time.Parse(dateFmt, od)
		if perr != nil {
			continue
		}
		out = append(out, core.EPSPoint{Date: t, EPS: eps})
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/collector/qlibpit/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/collector/qlibpit/
git commit -m "feat(qlibpit): point-in-time EPS history source"
```

---

## Task 5: 仓库缺基本面时委托兜底（Go）

**Files:**
- Modify: `internal/collector/qlibpit/qlibpit_test.go`

兜底逻辑已在 Task 4 的 `New(db, fallback)` 实现；本任务补测试钉死契约（缺符号→委托；fallback 为 nil→空切片）。

- [ ] **Step 1: 写失败/回归测试（追加）**

```go
type fakeEPS struct {
	pts    []core.EPSPoint
	called string
}

func (f *fakeEPS) FetchEPSHistory(sym string, _, _ time.Time) ([]core.EPSPoint, error) {
	f.called = sym
	return f.pts, nil
}

func TestFallbackUsedWhenNoWarehouseFundamentals(t *testing.T) {
	db := newTestDB(t) // 空 fundamentals_pit
	fb := &fakeEPS{pts: []core.EPSPoint{{Date: d("2024-01-01"), EPS: 9.9}}}
	src := New(db, fb)
	got, err := src.FetchEPSHistory("MSFT", d("2020-01-01"), d("2024-12-31"))
	if err != nil || len(got) != 1 || got[0].EPS != 9.9 {
		t.Fatalf("should delegate to fallback, got %+v err=%v", got, err)
	}
	if fb.called != "MSFT" {
		t.Errorf("fallback should be called with symbol, got %q", fb.called)
	}
}

func TestNoFundamentalsNoFallbackReturnsEmpty(t *testing.T) {
	src := New(newTestDB(t), nil)
	got, err := src.FetchEPSHistory("X", d("2020-01-01"), d("2024-12-31"))
	if err != nil || len(got) != 0 {
		t.Fatalf("want empty/nil, got %+v err=%v", got, err)
	}
}

func TestWarehouseFundamentalsPreemptFallback(t *testing.T) {
	db := newTestDB(t)
	ins(t, db, "AAPL", "2024-03-31", "2024-05-15", 4.0)
	fb := &fakeEPS{pts: []core.EPSPoint{{Date: d("2024-01-01"), EPS: 9.9}}}
	src := New(db, fb)
	got, _ := src.FetchEPSHistory("AAPL", d("2020-01-01"), d("2024-12-31"))
	if len(got) != 1 || got[0].EPS != 4.0 {
		t.Fatalf("warehouse should preempt fallback, got %+v", got)
	}
	if fb.called != "" {
		t.Error("fallback must not be called when warehouse has data")
	}
}

import "github.com/newthinker/atlas/internal/core" // 若上方未引入则加
```

- [ ] **Step 2: 运行测试确认通过**

Run: `go test ./internal/collector/qlibpit/ -v`
Expected: PASS（全部）

- [ ] **Step 3: 提交**

```bash
git add internal/collector/qlibpit/qlibpit_test.go
git commit -m "test(qlibpit): fallback delegation and warehouse preemption"
```

---

## Task 6: serve.go 装配（用 qlibpit 包装 yahoo EPS 源）（Go）

**Files:**
- Modify: `cmd/atlas/serve.go`

第一期已在 `if cfg.Qlib.Enabled && cfg.Qlib.DBPath != ""` 块内打开只读 `db` 并注册 qlib collector。本期：在同一 `db` 上构造 qlibpit，包装现有 yahoo EPS 源，作为注入 app 的 EPS 源。

- [ ] **Step 1: 先确认现状**

Run: `grep -n "SetValuationSources\|epsSourceOrNil\|cfg.Qlib.Enabled" cmd/atlas/serve.go`
Expected: 看到第 147 行附近 `application.SetValuationSources(valuationSourceOrNil(lixingerCollector), epsSourceOrNil(yahooCollector))`，以及第一期加入的 `if cfg.Qlib.Enabled` 块。

- [ ] **Step 2: 改注入逻辑**

将原 `application.SetValuationSources(valuationSourceOrNil(lixingerCollector), epsSourceOrNil(yahooCollector))` 一行替换为：
```go
	// EPS 源：仓库 PIT 优先、yahoo 兜底。仅当 qlib 仓库已开（第一期在 qlibWarehouseDB
	// 中保存了打开的 *sql.DB；未启用时为 nil）才包装，否则维持纯 yahoo。
	var epsSrc app.EPSSource = epsSourceOrNil(yahooCollector)
	if qlibWarehouseDB != nil {
		epsSrc = qlibpit.New(qlibWarehouseDB, epsSrc)
		log.Info("qlib PIT EPS source enabled (yahoo fallback)")
	}
	application.SetValuationSources(valuationSourceOrNil(lixingerCollector), epsSrc)
```
> 前置改动：把打开的只读 `db` 赋给一个函数级变量 `var qlibWarehouseDB *sql.DB`（成功打开后 `qlibWarehouseDB = db`），使其在本注入点可见。`import` 增加 `"github.com/newthinker/atlas/internal/collector/qlibpit"`。

> ⚠ **顺序约束（关键）**：本注入点是 serve.go:147 的 `SetValuationSources`，而第一期把 qlib **collector 注册**放在「外部 collector 注册之后」——可能落在 147 之后。EPS 源需要 `qlibWarehouseDB` 在 147 之前就绪，因此须**拆分两件事的时机**：
> 1. **打开 `db`**（`sql.Open(...)+Ping`，赋给 `qlibWarehouseDB`）提前到 `SetValuationSources` 之前；
> 2. **注册 qlib collector**（`application.RegisterCollector(qc)`）仍保持在所有外部 collector 注册之后（它的补尾需要外部源先在注册表里）。
> 即把第一期那个 `if cfg.Qlib.Enabled` 块拆成「先开库」+「后注册 collector」两段，中间夹着外部 collector 注册与本 EPS 注入。

> ⚠ QA S1 不变量：`SetValuationSources` 必须在 `Start` 之前调用。本替换不改变调用时机，仅改变传入的 EPS 源，满足约束。

- [ ] **Step 3: 编译 + 全量测试**

Run: `go build ./... && go test ./internal/... ./cmd/...`
Expected: 编译通过；PASS（含 `serve_test.go` 既有 `epsSourceOrNil` 用例零回归）

- [ ] **Step 4: 提交**

```bash
git add cmd/atlas/serve.go
git commit -m "feat(qlibpit): wire PIT EPS source with yahoo fallback in serve"
```

---

## Task 7: 各市场基本面适配器（best-effort，文档 + Makefile）

**Files:**
- Create: `scripts/qlib_warehouse/ADAPTERS.md`
- Modify: `Makefile`（增 `FUNDAMENTALS_*_DIR` 变量与可选 `warehouse-dump` 透传）

> 本任务**不写单元测试**（数据获取依赖外部源/网络，属集成）。主干（Task 1-6）已不依赖它即可工作与验证；本任务把「如何产出 `fundamentals_csv/`」文档化，并打通 US 的 make 透传。

- [ ] **Step 1: 写 ADAPTERS.md**

`scripts/qlib_warehouse/ADAPTERS.md` 内容要点：
- 归一化契约表头（与 Task 1 一致）：`symbol,report_period,observe_date,eps_ttm,pe,pb,ps,roe,dividend_yield`
- **A 股（推荐，qlib 原生）**：用 qlib `dump_pit` / 财务数据导出报告期与首次披露日；`observe_date` 取披露日，`eps_ttm` 取滚动四季稀释 EPS。命令骨架与 `qlib` 数据目录路径占位。
- **美股（Yahoo，best-effort）**：Yahoo `trailingDilutedEPS` 仅给报告期末 `asOfDate`，缺真实备案日；近似 `observe_date = asOfDate + 45 天`（季报披露滞后经验值），并在 CSV `source` 注明近似。明确这是「优于现状」的近似而非精确 PIT。
- **港股（lixinger，best-effort）**：lixinger 基本面接口（atlas 已集成）取 PE/EPS 历史；`observe_date` 取数据点日期。
- 缺基本面的符号：不产出该 symbol 的 CSV → Go 侧自动回落 yahoo（零影响）。

- [ ] **Step 2: Makefile 透传（US）**

在 `warehouse-dump` target 增加可选 fundamentals 目录变量：
```makefile
FUNDAMENTALS_US_DIR ?= fundamentals_csv_us
warehouse-dump:
	@mkdir -p $(dir $(WAREHOUSE_DB))
	$(QLIB_PY) -m scripts.qlib_warehouse.build_warehouse \
	  --csv-dir $(QLIB_CSV_US_DIR) --market US --source yahoo --db $(WAREHOUSE_DB) \
	  $(if $(wildcard $(FUNDAMENTALS_US_DIR)),--fundamentals-dir $(FUNDAMENTALS_US_DIR),)
```
（`$(wildcard ...)` 守卫：目录不存在时不传 `--fundamentals-dir`，dump 仍只写 OHLCV，零破坏。）

- [ ] **Step 3: 验证 dump 仍可运行（无 fundamentals 目录时）+ 提交**

Run: `make warehouse-dump`
Expected: 仍打印 `wrote N rows`，无 fundamentals 目录时不报错
```bash
git add scripts/qlib_warehouse/ADAPTERS.md Makefile
git commit -m "docs(warehouse): per-market fundamentals adapter contract + US make passthrough"
```

---

## 完成标准（DoD）

- Python 测试全绿：`scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_warehouse/ -v`（含第一期）
- Go 测试全绿：`go test ./internal/collector/qlibpit/ ./cmd/...`
- PIT 正确性有测试钉死：未来 `observe_date` 不泄漏（防前视）、修订按 `observe_date` 升序保留
- 仓库有某符号基本面 → PIT 主源；无 → 委托 yahoo；仓库未启用 → 行为与现状完全一致（零回归）
- `make warehouse-dump` 在有/无 `fundamentals_csv_us/` 时均成功

## 范围边界（本期不做 / best-effort）

- 各市场 `fundamentals_csv/` 的**实际生产**为 best-effort 适配器（Task 7 文档化）；主干不依赖其精确性即可交付与验证
- 美股 `observe_date` 为披露滞后近似（优于现状的报告期末对齐，但非精确备案日）
- PB/PS/ROE 等列入库但本期 Go 侧只消费 `eps_ttm`（PE 分位）；其余列留作后续估值扩展
