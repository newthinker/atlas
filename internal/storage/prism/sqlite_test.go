package prism

import (
	"database/sql"
	"errors"
	"math"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping
//   functional[0] 增量锚点 LatestDate(无数据 "" → 两行后 2026-07-22) → TestLatestDateEmptyThenAdvances
//   functional[1] NaN↔NULL 往返(NaN 落 NULL,读回 IsNaN;非 NaN 一致)  → TestUpsertValuationsNaNRoundtrip
//   functional[2] Board 每标的最新行(含元数据,AsOf=MAX(d))           → TestBoardReturnsLatestPerInstrument
//   functional[3] Series 按 from 过滤(from="" 全部,升序)             → TestSeriesFrom
//   boundary[0]   UpsertInstrument (symbol,type) 幂等同 id+更新元数据   → TestUpsertInstrumentIdempotent
//   boundary[1]   UpsertValuations (instrument_id,d) 幂等 ON CONFLICT   → TestUpsertValuationsIdempotent
//   error[0]      Open 自动创建多级不存在的父目录(TempDir/a/b/prism.db) → TestOpenCreatesNestedParentDirs

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "prism.db"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestLatestDateEmptyThenAdvances(t *testing.T) {
	s := openTemp(t)
	id, err := s.UpsertInstrument(Instrument{Symbol: "000300.SH", Type: "index", Market: "CN_A", Name: "沪深300", Group: "A股指数", Source: "lixinger"})
	require.NoError(t, err)

	d, err := s.LatestDate(id)
	require.NoError(t, err)
	assert.Equal(t, "", d)

	require.NoError(t, s.UpsertValuations(id, []ValuationRow{
		{D: "2026-07-21", PETTM: 12.1, PB: 1.3, PSTTM: 1.1, Pctl5Y: 40, Pctl10Y: 35},
		{D: "2026-07-22", PETTM: 12.2, PB: 1.3, PSTTM: 1.1, Pctl5Y: 41, Pctl10Y: 36},
	}))
	d, err = s.LatestDate(id)
	require.NoError(t, err)
	assert.Equal(t, "2026-07-22", d)
}

func TestUpsertValuationsNaNRoundtrip(t *testing.T) {
	s := openTemp(t)
	id, _ := s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "NVIDIA", Group: "美股公司", Source: "engine"})
	require.NoError(t, s.UpsertValuations(id, []ValuationRow{
		{D: "2026-07-22", PETTM: 45.5, PB: math.NaN(), PSTTM: math.NaN(), Pctl5Y: 88, Pctl10Y: math.NaN()},
	}))
	got, err := s.Series("NVDA", "")
	require.NoError(t, err)
	require.Len(t, got.Dates, 1)
	assert.Equal(t, 45.5, got.PETTM[0])
	assert.True(t, math.IsNaN(got.Pctl10Y[0]))
	assert.Equal(t, 88.0, got.Pctl5Y[0])
}

func TestBoardReturnsLatestPerInstrument(t *testing.T) {
	s := openTemp(t)
	a, _ := s.UpsertInstrument(Instrument{Symbol: "000300.SH", Type: "index", Market: "CN_A", Name: "沪深300", Group: "A股指数", Source: "lixinger"})
	b, _ := s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "NVIDIA", Group: "美股公司", Source: "engine"})
	require.NoError(t, s.UpsertValuations(a, []ValuationRow{
		{D: "2026-07-21", PETTM: 12.1, Pctl5Y: 40, Pctl10Y: 35, PB: 1.3, PSTTM: 1.1},
		{D: "2026-07-22", PETTM: 12.2, Pctl5Y: 41, Pctl10Y: 36, PB: 1.3, PSTTM: 1.1},
	}))
	require.NoError(t, s.UpsertValuations(b, []ValuationRow{
		{D: "2026-07-22", PETTM: 45.5, Pctl5Y: 88, Pctl10Y: math.NaN(), PB: math.NaN(), PSTTM: math.NaN()},
	}))
	rows, err := s.Board()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	byID := map[string]BoardRow{}
	for _, r := range rows {
		byID[r.Symbol] = r
	}
	assert.Equal(t, "2026-07-22", byID["000300.SH"].AsOf)
	assert.Equal(t, 12.2, byID["000300.SH"].PETTM)
	assert.Equal(t, "NVIDIA", byID["NVDA"].Name)
}

func TestSeriesFrom(t *testing.T) {
	s := openTemp(t)
	id, _ := s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "NVIDIA", Group: "美股公司", Source: "engine"})
	require.NoError(t, s.UpsertValuations(id, []ValuationRow{
		{D: "2021-07-22", PETTM: 60},
		{D: "2026-07-22", PETTM: 45},
	}))
	got, err := s.Series("NVDA", "2025-01-01")
	require.NoError(t, err)
	require.Len(t, got.Dates, 1)
	assert.Equal(t, "2026-07-22", got.Dates[0])

	// from="" 返回全部且升序
	all, err := s.Series("NVDA", "")
	require.NoError(t, err)
	require.Len(t, all.Dates, 2)
	assert.Equal(t, []string{"2021-07-22", "2026-07-22"}, all.Dates)
}

func TestUpsertInstrumentIdempotent(t *testing.T) {
	s := openTemp(t)
	id1, err := s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "NVIDIA", Group: "美股公司", Source: "engine"})
	require.NoError(t, err)
	// 同 (symbol,type) 重复 upsert:返回同一 id 且更新元数据
	id2, err := s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "英伟达", Group: "美股龙头", Source: "engine"})
	require.NoError(t, err)
	assert.Equal(t, id1, id2)

	require.NoError(t, s.UpsertValuations(id2, []ValuationRow{{D: "2026-07-22", PETTM: 45.5}}))
	rows, err := s.Board()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "英伟达", rows[0].Name)
	assert.Equal(t, "美股龙头", rows[0].Group)
}

func TestUpsertValuationsIdempotent(t *testing.T) {
	s := openTemp(t)
	id, _ := s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "NVIDIA", Group: "美股公司", Source: "engine"})
	require.NoError(t, s.UpsertValuations(id, []ValuationRow{{D: "2026-07-22", PETTM: 45.5}}))
	// 同 (instrument_id,d) 重复写:幂等更新,不产生重复行
	require.NoError(t, s.UpsertValuations(id, []ValuationRow{{D: "2026-07-22", PETTM: 50.0}}))
	got, err := s.Series("NVDA", "")
	require.NoError(t, err)
	require.Len(t, got.Dates, 1)
	assert.Equal(t, 50.0, got.PETTM[0])
}

func TestOpenCreatesNestedParentDirs(t *testing.T) {
	// error_handling[0]: 多级不存在的父目录应被自动创建
	path := filepath.Join(t.TempDir(), "a", "b", "prism.db")
	s, err := Open(path)
	require.NoError(t, err)
	defer s.Close()
	_, err = s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "NVIDIA", Group: "美股公司", Source: "engine"})
	require.NoError(t, err)
}

func TestSeriesUnknownSymbolReturnsErrNotFound(t *testing.T) {
	// error_handling: 未知 symbol 返回包装的 ErrNotFound(errors.Is 可判)
	s := openTemp(t)
	_, err := s.Series("NOPE", "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotFound), "未知 symbol 应返回可判定的 ErrNotFound")
	assert.Contains(t, err.Error(), "NOPE")
}

// TASK-002 done_criteria → test mapping:
//   functional[0]/boundary[1] 幂等 upsert + period_end 升序读回 → TestUpsertFundamentalsRoundtrip
//   functional[1]             FundamentalRow 字段契约(逐字段相等) → TestUpsertFundamentalsRoundtrip
//   boundary[0]               Equity NaN 落 NULL 读回 IsNaN        → TestUpsertFundamentalsRoundtrip
//   error_handling[0]         空 rows 不报错且不产生行             → TestUpsertFundamentalsEmpty

func TestUpsertFundamentalsRoundtrip(t *testing.T) {
	s := openTemp(t)
	id, _ := s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "NVIDIA", Group: "美股公司", Source: "edgar"})
	rows := []FundamentalRow{
		{FiscalPeriod: "2026Q1", PeriodEnd: "2026-04-27", FilingDate: "2026-05-28",
			Revenue: 44e9, NetIncome: 18e9, EPSDiluted: 0.76, Equity: 80e9, DilutedShares: 24.6e9,
			GrossProfit: 26e9, RnD: 4e9, SGnA: 1e9, OperatingIncome: 21e9, IncomeTax: 3e9, Source: "edgar"},
		{FiscalPeriod: "2025Q4", PeriodEnd: "2026-01-26", FilingDate: "2026-02-26",
			Revenue: 39e9, NetIncome: 22e9, EPSDiluted: 0.89, Equity: math.NaN(), DilutedShares: 24.7e9,
			GrossProfit: math.NaN(), RnD: 3.5e9, SGnA: math.NaN(), OperatingIncome: 24e9, IncomeTax: math.NaN(), Source: "edgar"},
	}
	require.NoError(t, s.UpsertFundamentals(id, rows))
	require.NoError(t, s.UpsertFundamentals(id, rows)) // 幂等

	got, err := s.QuarterlyFundamentals(id)
	require.NoError(t, err)
	require.Len(t, got, 2)
	// period_end 升序:2026-01-26 在前,2026-04-27 在后
	assert.Equal(t, "2025Q4", got[0].FiscalPeriod, "period_end 升序")
	assert.True(t, math.IsNaN(got[0].Equity), "Equity NaN 落 NULL 读回 IsNaN")
	assert.Equal(t, "2026-02-26", got[0].FilingDate)
	// 第二行(2026Q1)字段逐一相等
	want := rows[0]
	assert.Equal(t, want.FiscalPeriod, got[1].FiscalPeriod)
	assert.Equal(t, want.PeriodEnd, got[1].PeriodEnd)
	assert.Equal(t, want.FilingDate, got[1].FilingDate)
	assert.Equal(t, want.Revenue, got[1].Revenue)
	assert.Equal(t, want.NetIncome, got[1].NetIncome)
	assert.Equal(t, want.EPSDiluted, got[1].EPSDiluted)
	assert.Equal(t, want.Equity, got[1].Equity)
	assert.Equal(t, want.DilutedShares, got[1].DilutedShares)
	assert.Equal(t, want.Source, got[1].Source)

	// TASK-002 functional[2]: M3 新增 5 字段 NaN↔NULL 双向往返
	assert.Equal(t, want.GrossProfit, got[1].GrossProfit)
	assert.Equal(t, want.RnD, got[1].RnD)
	assert.Equal(t, want.SGnA, got[1].SGnA)
	assert.Equal(t, want.OperatingIncome, got[1].OperatingIncome)
	assert.Equal(t, want.IncomeTax, got[1].IncomeTax)
	assert.True(t, math.IsNaN(got[0].GrossProfit), "GrossProfit NaN 落 NULL 读回 IsNaN")
	assert.True(t, math.IsNaN(got[0].SGnA), "SGnA NaN 落 NULL 读回 IsNaN")
	assert.True(t, math.IsNaN(got[0].IncomeTax), "IncomeTax NaN 落 NULL 读回 IsNaN")
	assert.Equal(t, 3.5e9, got[0].RnD)
	assert.Equal(t, 24e9, got[0].OperatingIncome)
}

func TestUpsertFundamentalsEmpty(t *testing.T) {
	// error_handling[0]: 空 rows 切片 Upsert 不报错且不产生行
	s := openTemp(t)
	id, _ := s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "NVIDIA", Group: "美股公司", Source: "edgar"})
	require.NoError(t, s.UpsertFundamentals(id, nil))
	require.NoError(t, s.UpsertFundamentals(id, []FundamentalRow{}))
	got, err := s.QuarterlyFundamentals(id)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// ---------------------------------------------------------------------------
// TASK-002 done_criteria → test mapping
//   functional[0] 旧库 Open 后 5 新列存在                → TestOpenMigratesFundamentalQ
//   functional[1] 迁移不损坏存量数据(行数/行值/新列 NaN) → TestMigratePreservesExistingData
//   functional[2] 新 5 字段 NaN↔NULL 往返                → TestUpsertFundamentalsRoundtrip(扩断言)
//   functional[3] 分部 PK=(id,period_end,segment_key)/幂等/manual 覆盖/升序/锚点
//                                                        → TestSegmentRoundtripAndAnchor
//   functional[4] 价格幂等 + from 过滤 + 升序 + NaN 往返 → TestPriceSeriesFrom
//   boundary[0]   无分部锚点 ""、未知 symbol ErrNotFound、from="" 全量
//                                → TestLatestSegmentPeriodEndEmpty / TestPriceSeriesBoundaries
//   boundary[1]   AD-10 并发迁移容错 + Open 幂等 + 新旧库 schema 一致
//                                → TestMigrateToleratesDuplicateColumn / TestOpenIsIdempotent /
//                                  TestConcurrentOpenAllSucceed / TestMigratedSchemaMatchesFreshSchema
// ---------------------------------------------------------------------------

// oldSchema 是 M2(本次迁移之前)的 fundamental_q 定义,用于构造「存量库」。
// 刻意写成字面量而非引用 const schema —— 引用会随 schema 演进而失去回归意义。
const oldSchema = `
CREATE TABLE IF NOT EXISTS instrument (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  symbol  TEXT NOT NULL,
  type    TEXT NOT NULL,
  market  TEXT NOT NULL,
  name    TEXT,
  grp     TEXT,
  source  TEXT NOT NULL,
  UNIQUE(symbol, type)
);
CREATE TABLE IF NOT EXISTS valuation_daily (
  instrument_id INTEGER NOT NULL REFERENCES instrument(id),
  d        TEXT NOT NULL,
  pe_ttm   REAL,
  pb       REAL,
  ps_ttm   REAL,
  pctl_5y  REAL,
  pctl_10y REAL,
  PRIMARY KEY (instrument_id, d)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS fundamental_q (
  instrument_id  INTEGER NOT NULL REFERENCES instrument(id),
  fiscal_period  TEXT NOT NULL,
  period_end     TEXT NOT NULL,
  filing_date    TEXT NOT NULL,
  revenue        REAL,
  net_income     REAL,
  eps_diluted    REAL,
  equity         REAL,
  diluted_shares REAL,
  source         TEXT,
  PRIMARY KEY (instrument_id, period_end)
) WITHOUT ROWID;
`

// makeLegacyDB 建一个 M2 旧 schema 的库(可选预置数据),返回其路径。
func makeLegacyDB(t *testing.T, seed bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", "file:"+path)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(oldSchema)
	require.NoError(t, err)
	if seed {
		_, err = db.Exec(`INSERT INTO instrument(symbol,type,market,name,grp,source)
			VALUES('NVDA','stock','US','NVIDIA','美股公司','edgar')`)
		require.NoError(t, err)
		_, err = db.Exec(`INSERT INTO fundamental_q
			(instrument_id,fiscal_period,period_end,filing_date,revenue,net_income,
			 eps_diluted,equity,diluted_shares,source) VALUES
			(1,'2025Q4','2026-01-26','2026-02-26',39e9,22e9,0.89,75e9,24.7e9,'edgar'),
			(1,'2026Q1','2026-04-27','2026-05-28',44e9,18e9,0.76,NULL,24.6e9,'edgar')`)
		require.NoError(t, err)
		_, err = db.Exec(`INSERT INTO valuation_daily(instrument_id,d,pe_ttm,pb,ps_ttm,pctl_5y,pctl_10y)
			VALUES(1,'2026-07-21',45.5,30.1,22.0,88.0,NULL),(1,'2026-07-22',46.5,30.5,22.4,89.0,NULL)`)
		require.NoError(t, err)
	}
	return path
}

// tableInfo 返回 (列名, 声明类型) 的有序列表,顺序即 pragma_table_info 的 cid 顺序。
func tableInfo(t *testing.T, db *sql.DB, table string) [][2]string {
	t.Helper()
	rows, err := db.Query(`SELECT name, type FROM pragma_table_info(?) ORDER BY cid`, table)
	require.NoError(t, err)
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var name, typ string
		require.NoError(t, rows.Scan(&name, &typ))
		out = append(out, [2]string{name, typ})
	}
	require.NoError(t, rows.Err())
	return out
}

func columnNames(info [][2]string) []string {
	out := make([]string, len(info))
	for i, c := range info {
		out[i] = c[0]
	}
	return out
}

func TestOpenMigratesFundamentalQ(t *testing.T) {
	// functional[0]: 旧 schema 库 Open 成功,且 M3 五个新列全部存在
	path := makeLegacyDB(t, false)
	s, err := Open(path)
	require.NoError(t, err)
	defer s.Close()

	names := columnNames(tableInfo(t, s.db, "fundamental_q"))
	for _, col := range []string{"gross_profit", "rnd", "sgna", "operating_income", "income_tax"} {
		assert.Contains(t, names, col, "迁移后应存在列 %s", col)
	}
}

func TestMigratePreservesExistingData(t *testing.T) {
	// functional[1]: 迁移只加列,存量行数与行值完全不变,新字段读回 NaN
	path := makeLegacyDB(t, true)
	s, err := Open(path)
	require.NoError(t, err)
	defer s.Close()

	var nf, nv int
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM fundamental_q`).Scan(&nf))
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM valuation_daily`).Scan(&nv))
	assert.Equal(t, 2, nf, "fundamental_q 行数不变")
	assert.Equal(t, 2, nv, "valuation_daily 行数不变")

	got, err := s.QuarterlyFundamentals(1)
	require.NoError(t, err)
	require.Len(t, got, 2)
	// 抽样行值完全不变
	assert.Equal(t, "2025Q4", got[0].FiscalPeriod)
	assert.Equal(t, "2026-01-26", got[0].PeriodEnd)
	assert.Equal(t, "2026-02-26", got[0].FilingDate)
	assert.Equal(t, 39e9, got[0].Revenue)
	assert.Equal(t, 22e9, got[0].NetIncome)
	assert.Equal(t, 0.89, got[0].EPSDiluted)
	assert.Equal(t, 75e9, got[0].Equity)
	assert.Equal(t, 24.7e9, got[0].DilutedShares)
	assert.Equal(t, "edgar", got[0].Source)
	assert.True(t, math.IsNaN(got[1].Equity), "原本为 NULL 的 equity 仍读回 NaN")
	assert.Equal(t, 44e9, got[1].Revenue)
	// 新列对存量行为 NULL → NaN
	for i, r := range got {
		assert.True(t, math.IsNaN(r.GrossProfit), "行%d GrossProfit 应为 NaN", i)
		assert.True(t, math.IsNaN(r.RnD), "行%d RnD 应为 NaN", i)
		assert.True(t, math.IsNaN(r.SGnA), "行%d SGnA 应为 NaN", i)
		assert.True(t, math.IsNaN(r.OperatingIncome), "行%d OperatingIncome 应为 NaN", i)
		assert.True(t, math.IsNaN(r.IncomeTax), "行%d IncomeTax 应为 NaN", i)
	}
	// valuation_daily 抽样值不变
	var pe, pb float64
	require.NoError(t, s.db.QueryRow(
		`SELECT pe_ttm, pb FROM valuation_daily WHERE instrument_id=1 AND d='2026-07-22'`).Scan(&pe, &pb))
	assert.Equal(t, 46.5, pe)
	assert.Equal(t, 30.5, pb)
}

func TestMigratedSchemaMatchesFreshSchema(t *testing.T) {
	// boundary[1]: 防 const schema 与 migrate 列表漂移 —— 新建库与迁移后旧库列名/类型完全一致
	fresh, err := Open(filepath.Join(t.TempDir(), "fresh.db"))
	require.NoError(t, err)
	defer fresh.Close()

	migrated, err := Open(makeLegacyDB(t, true))
	require.NoError(t, err)
	defer migrated.Close()

	for _, table := range []string{"instrument", "valuation_daily", "fundamental_q", "segment_revenue", "price_daily"} {
		info := tableInfo(t, fresh.db, table)
		require.NotEmpty(t, info, "表 %s 必须存在(空列表说明表根本没建,比较将失去意义)", table)
		assert.Equal(t, info, tableInfo(t, migrated.db, table),
			"表 %s 的列名与类型在新建库与迁移库间应完全一致", table)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	// boundary[1]: 同一库连续两次 Open 均成功(第二次 migrate 走「列已存在」分支)
	path := makeLegacyDB(t, true)
	s1, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, s1.Close())

	s2, err := Open(path)
	require.NoError(t, err)
	defer s2.Close()
	assert.Contains(t, columnNames(tableInfo(t, s2.db, "fundamental_q")), "income_tax")
}

func TestIsDuplicateColumnErr(t *testing.T) {
	// boundary[1]/AD-10: 用 sqlite 真实产生的错误(不伪造字符串)验证容错判定,
	// 并确认其他 DDL 错误不会被误判为「已被并发迁移」而吞掉。
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "dup.db"))
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE t (a REAL)`)
	require.NoError(t, err)

	_, dupErr := db.Exec(`ALTER TABLE t ADD COLUMN a REAL`)
	require.Error(t, dupErr)
	assert.True(t, isDuplicateColumnErr(dupErr), "真实 duplicate column 错误应被识别: %v", dupErr)

	_, otherErr := db.Exec(`ALTER TABLE nosuchtable ADD COLUMN b REAL`)
	require.Error(t, otherErr)
	assert.False(t, isDuplicateColumnErr(otherErr), "非 duplicate column 错误不得被吞: %v", otherErr)
	assert.False(t, isDuplicateColumnErr(nil))
}

func TestConcurrentOpenAllSucceed(t *testing.T) {
	// boundary[1]/AD-10: 真实拓扑 serve(常驻) + prism-daily(定时) 并发 Open 同一库。
	// check-then-act 竞态下第二个 ALTER 报 duplicate column,必须容错而非让 Open 失败。
	path := makeLegacyDB(t, true)
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	stores := make([]*Store, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start // 尽量让所有 goroutine 同时进入 migrate 的 check-then-act 窗口
			stores[i], errs[i] = Open(path)
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		assert.NoError(t, err, "并发 Open #%d 应成功", i)
		if stores[i] != nil {
			stores[i].Close()
		}
	}

	verify, err := Open(path)
	require.NoError(t, err)
	defer verify.Close()
	names := columnNames(tableInfo(t, verify.db, "fundamental_q"))
	for _, col := range []string{"gross_profit", "rnd", "sgna", "operating_income", "income_tax"} {
		assert.Contains(t, names, col)
	}
	// 并发迁移不得重复加列
	assert.Len(t, names, 15, "10 个旧列 + 5 个新列,不多不少")
}

func TestSegmentRoundtripAndAnchor(t *testing.T) {
	// functional[3]: PK=(instrument_id,period_end,segment_key)(AD-9)、幂等、
	// manual 覆盖 edgar_segment、按 period_end+segment_key 升序、fiscal_period 可空、锚点推进
	s := openTemp(t)
	id, _ := s.UpsertInstrument(Instrument{Symbol: "MSFT", Type: "stock", Market: "US", Name: "Microsoft", Group: "美股公司", Source: "edgar"})

	rows := []SegmentRow{
		{PeriodEnd: "2026-03-31", FiscalPeriod: "2026Q3", SegmentKey: "intelligent_cloud", Revenue: 26.7e9, Source: "edgar_segment"},
		{PeriodEnd: "2026-03-31", FiscalPeriod: "2026Q3", SegmentKey: "productivity", Revenue: 29.9e9, Source: "edgar_segment"},
		{PeriodEnd: "2025-12-31", FiscalPeriod: "", SegmentKey: "productivity", Revenue: 29.4e9, Source: "edgar_segment"}, // fiscal_period 可空
	}
	require.NoError(t, s.UpsertSegments(id, rows))
	require.NoError(t, s.UpsertSegments(id, rows)) // 幂等:重复写不产生新行

	got, err := s.SegmentRows(id)
	require.NoError(t, err)
	require.Len(t, got, 3, "幂等 upsert 后行数不增长")
	// ORDER BY period_end, segment_key
	assert.Equal(t, []string{"2025-12-31", "2026-03-31", "2026-03-31"},
		[]string{got[0].PeriodEnd, got[1].PeriodEnd, got[2].PeriodEnd})
	assert.Equal(t, "intelligent_cloud", got[1].SegmentKey)
	assert.Equal(t, "productivity", got[2].SegmentKey)
	assert.Equal(t, "", got[0].FiscalPeriod, "fiscal_period 允许为空")
	assert.Equal(t, "2026Q3", got[1].FiscalPeriod)
	assert.Equal(t, 26.7e9, got[1].Revenue)
	assert.Equal(t, "edgar_segment", got[1].Source)

	// AD-9: fiscal_period 不参与主键 —— 同 (period_end, segment_key) 换个展示标签仍是同一行
	require.NoError(t, s.UpsertSegments(id, []SegmentRow{
		{PeriodEnd: "2026-03-31", FiscalPeriod: "2026Q4", SegmentKey: "productivity", Revenue: 30.0e9, Source: "manual"},
	}))
	got, err = s.SegmentRows(id)
	require.NoError(t, err)
	require.Len(t, got, 3, "主键含 period_end 而非 fiscal_period:换标签不新增行")
	assert.Equal(t, "manual", got[2].Source, "source=manual 覆盖同键 edgar_segment 行")
	assert.Equal(t, 30.0e9, got[2].Revenue)
	assert.Equal(t, "2026Q4", got[2].FiscalPeriod)

	// 反向:同 fiscal_period 标签但 period_end 不同 → 两行独立共存(标签碰撞不会静默覆盖)
	require.NoError(t, s.UpsertSegments(id, []SegmentRow{
		{PeriodEnd: "2026-06-30", FiscalPeriod: "2026Q4", SegmentKey: "productivity", Revenue: 31.0e9, Source: "edgar_segment"},
	}))
	got, err = s.SegmentRows(id)
	require.NoError(t, err)
	assert.Len(t, got, 4, "同 fiscal_period 标签、不同 period_end 应各自成行")

	// 锚点为 MAX(period_end),随数据推进(AD-9:不 JOIN fundamental_q)
	anchor, err := s.LatestSegmentPeriodEnd(id)
	require.NoError(t, err)
	assert.Equal(t, "2026-06-30", anchor)

	// 分部锚点与 fundamental_q 无关:该 instrument 完全没有 fundamental_q 行也能返回锚点
	var nf int
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM fundamental_q WHERE instrument_id=?`, id).Scan(&nf))
	require.Zero(t, nf)

	// revenue 的 NaN↔NULL 往返
	require.NoError(t, s.UpsertSegments(id, []SegmentRow{
		{PeriodEnd: "2026-06-30", FiscalPeriod: "2026Q4", SegmentKey: "other", Revenue: math.NaN(), Source: "manual"},
	}))
	got, err = s.SegmentRows(id)
	require.NoError(t, err)
	require.Len(t, got, 5)
	// segment_key 升序:同 period_end 下 "other" 排在 "productivity" 之前
	assert.Equal(t, "other", got[3].SegmentKey)
	assert.True(t, math.IsNaN(got[3].Revenue), "Revenue NaN 落 NULL 读回 IsNaN")
}

func TestLatestSegmentPeriodEndEmpty(t *testing.T) {
	// boundary[0]: 无分部数据时返回 ("", nil)
	s := openTemp(t)
	id, _ := s.UpsertInstrument(Instrument{Symbol: "MSFT", Type: "stock", Market: "US", Name: "Microsoft", Group: "美股公司", Source: "edgar"})
	anchor, err := s.LatestSegmentPeriodEnd(id)
	require.NoError(t, err)
	assert.Equal(t, "", anchor)

	// 未知 instrument 同样是 ("", nil) 而非错误
	anchor, err = s.LatestSegmentPeriodEnd(9999)
	require.NoError(t, err)
	assert.Equal(t, "", anchor)

	// 空 rows 不报错且不产生行
	require.NoError(t, s.UpsertSegments(id, nil))
	rows, err := s.SegmentRows(id)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestPriceSeriesFrom(t *testing.T) {
	// functional[4]: UpsertPrices 幂等、from 过滤、日期升序、close 的 NaN↔NULL 往返
	s := openTemp(t)
	id, _ := s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "NVIDIA", Group: "美股公司", Source: "yahoo"})
	prices := []PriceRow{
		{D: "2026-07-22", Close: 172.5},
		{D: "2021-07-22", Close: 60.1},
		{D: "2026-07-21", Close: math.NaN()},
	}
	require.NoError(t, s.UpsertPrices(id, prices))
	require.NoError(t, s.UpsertPrices(id, prices)) // 幂等

	dates, closes, err := s.PriceSeries("NVDA", "")
	require.NoError(t, err)
	require.Len(t, dates, 3, "幂等 upsert 后行数不增长")
	assert.Equal(t, []string{"2021-07-22", "2026-07-21", "2026-07-22"}, dates, "日期升序")
	assert.Equal(t, 60.1, closes[0], "重复 upsert 后值不变")
	assert.True(t, math.IsNaN(closes[1]), "close NaN 落 NULL 读回 IsNaN")
	assert.Equal(t, 172.5, closes[2])

	// from 过滤生效
	dates, closes, err = s.PriceSeries("NVDA", "2026-01-01")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-07-21", "2026-07-22"}, dates)
	require.Len(t, closes, 2)

	// 覆盖写:同 (id,d) 更新 close
	require.NoError(t, s.UpsertPrices(id, []PriceRow{{D: "2026-07-22", Close: 180.0}}))
	dates, closes, err = s.PriceSeries("NVDA", "2026-07-22")
	require.NoError(t, err)
	require.Len(t, dates, 1)
	assert.Equal(t, 180.0, closes[0])
}

func TestPriceSeriesBoundaries(t *testing.T) {
	// boundary[0]: 未知 symbol 返回 ErrNotFound(与既有 Series 一致);已知但无价格返回空序列
	s := openTemp(t)
	_, _, err := s.PriceSeries("NOPE", "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotFound), "未知 symbol 应返回可判定的 ErrNotFound")
	assert.Contains(t, err.Error(), "NOPE")

	id, _ := s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "NVIDIA", Group: "美股公司", Source: "yahoo"})
	require.NoError(t, s.UpsertPrices(id, nil)) // 空 rows 不报错
	dates, closes, err := s.PriceSeries("NVDA", "")
	require.NoError(t, err)
	assert.Empty(t, dates)
	assert.Empty(t, closes)
}

func TestExecRetryBusy(t *testing.T) {
	// boundary[1]: WAL 转换/写锁竞争下 Open 不得直接失败 —— 重试到锁释放为止;
	// 锁始终不放时如实返回 BUSY(不静默吞错)。用真实持锁连接构造,不伪造错误。
	path := filepath.Join(t.TempDir(), "busy.db")
	// busy_timeout(0):让写锁冲突立刻返回 SQLITE_BUSY,使重试路径可确定性触发
	holder, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(0)")
	require.NoError(t, err)
	defer holder.Close()
	_, err = holder.Exec(`CREATE TABLE t(a REAL)`)
	require.NoError(t, err)

	other, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(0)")
	require.NoError(t, err)
	defer other.Close()

	lock := func(t *testing.T) *sql.Tx {
		t.Helper()
		tx, err := holder.Begin()
		require.NoError(t, err)
		_, err = tx.Exec(`INSERT INTO t VALUES(1)`) // 取 RESERVED 写锁
		require.NoError(t, err)
		return tx
	}

	// 持锁期间确实是 BUSY,且 isBusyErr 认得出来
	tx := lock(t)
	_, err = other.Exec(`CREATE TABLE IF NOT EXISTS t2(b REAL)`)
	require.Error(t, err)
	assert.True(t, isBusyErr(err), "持锁期间应为 SQLITE_BUSY: %v", err)
	assert.False(t, isBusyErr(nil))

	// 锁在重试窗口内释放 → execRetryBusy 最终成功
	go func() {
		time.Sleep(50 * time.Millisecond)
		tx.Commit()
	}()
	require.NoError(t, execRetryBusy(other, `CREATE TABLE IF NOT EXISTS t2(b REAL)`))

	// 锁一直不放 → 重试耗尽后如实返回 BUSY
	tx2 := lock(t)
	defer tx2.Rollback()
	err = execRetryBusy(other, `CREATE TABLE IF NOT EXISTS t3(c REAL)`)
	require.Error(t, err)
	assert.True(t, isBusyErr(err), "重试耗尽应返回原始 BUSY 错误: %v", err)

	// 非 BUSY 错误立即返回,不进重试
	err = execRetryBusy(other, `THIS IS NOT SQL`)
	require.Error(t, err)
	assert.False(t, isBusyErr(err))
}
