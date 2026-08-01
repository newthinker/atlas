package akshare

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: A 股财报采集(桑基 A 股支持,2026-08-01):
//   functional[0] 利润表累计→单季差分 + 财期标签 + 披露日     → TestFetchProfitQuartersDifferencing
//   functional[1] 银行营收回退链(TOTAL 缺→OPERATE_INCOME)     → TestFetchProfitQuartersBankRevenueFallback
//   functional[2] 主营构成季披露差分为单季、半年披露成半年块   → TestFetchSegmentsQuarterlyAndHalfYear
//   boundary[0]   跨年重置:次年 Q1 = 当期累计值本身            → TestFetchProfitQuartersDifferencing
//   boundary[1]   差分操作数缺失 → NaN(不产生假 0)             → TestFetchProfitQuartersDifferencing
//   boundary[2]   异常间隔(非季非半年)的构成差分块被丢弃        → TestFetchSegmentsQuarterlyAndHalfYear

func finServer(t *testing.T, profit, zygc string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/public/stock_profit_sheet_by_report_em":
			assert.Equal(t, "SH600519", r.URL.Query().Get("symbol"))
			w.Write([]byte(profit))
		case "/api/public/stock_zygc_em":
			w.Write([]byte(zygc))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestFetchProfitQuartersDifferencing(t *testing.T) {
	// 三期同年累计(Q1/H1/9M) + 上一年 FY。9M 的 RESEARCH_EXPENSE 缺失 → Q3 RnD 无法差分 = NaN。
	profit := `[
	 {"REPORT_DATE":"2026-09-30 00:00:00","NOTICE_DATE":"2026-10-28 00:00:00",
	  "TOTAL_OPERATE_INCOME":300,"OPERATE_COST":90,"OPERATE_PROFIT":150,"INCOME_TAX":30,
	  "PARENT_NETPROFIT":120,"SALE_EXPENSE":15,"MANAGE_EXPENSE":15,"DILUTED_EPS":3.0},
	 {"REPORT_DATE":"2026-06-30 00:00:00","NOTICE_DATE":"2026-08-20 00:00:00",
	  "TOTAL_OPERATE_INCOME":190,"OPERATE_COST":60,"OPERATE_PROFIT":95,"INCOME_TAX":19,
	  "PARENT_NETPROFIT":76,"RESEARCH_EXPENSE":4,"SALE_EXPENSE":10,"MANAGE_EXPENSE":10,"DILUTED_EPS":1.9},
	 {"REPORT_DATE":"2026-03-31 00:00:00","NOTICE_DATE":"2026-04-25 00:00:00",
	  "TOTAL_OPERATE_INCOME":100,"OPERATE_COST":30,"OPERATE_PROFIT":50,"INCOME_TAX":10,
	  "PARENT_NETPROFIT":40,"RESEARCH_EXPENSE":2,"SALE_EXPENSE":5,"MANAGE_EXPENSE":5,"DILUTED_EPS":1.0},
	 {"REPORT_DATE":"2025-12-31 00:00:00","NOTICE_DATE":"2026-03-30 00:00:00",
	  "TOTAL_OPERATE_INCOME":320,"OPERATE_COST":100,"OPERATE_PROFIT":160,"INCOME_TAX":32,
	  "PARENT_NETPROFIT":128,"RESEARCH_EXPENSE":8,"SALE_EXPENSE":16,"MANAGE_EXPENSE":16,"DILUTED_EPS":3.2}
	]`
	srv := finServer(t, profit, `[]`)
	defer srv.Close()

	qs, err := New(srv.URL).FetchProfitQuarters("600519.SH")
	require.NoError(t, err)
	require.Len(t, qs, 3, "3 个差分单季;上一年孤立 FY 期(跨度整年,无 9M 可差分)必须丢弃")

	byFP := map[string]ProfitQuarter{}
	for _, q := range qs {
		byFP[q.FiscalPeriod] = q
	}
	_, hasFY := byFP["2025Q4"]
	assert.False(t, hasFY, "全年累计不得冒充 Q4 单季")
	q1 := byFP["2026Q1"]
	assert.InDelta(t, 100, q1.Revenue, 1e-9, "年内首期(Q1)= 累计值本身")
	assert.InDelta(t, 70, q1.GrossProfit, 1e-9, "毛利 = 营收−营业成本")
	assert.Equal(t, "2026-04-25", q1.NoticeDate.Format("2006-01-02"))

	q2 := byFP["2026Q2"]
	assert.InDelta(t, 90, q2.Revenue, 1e-9, "Q2 = H1 − Q1")
	assert.InDelta(t, 36, q2.NetIncome, 1e-9)
	assert.InDelta(t, 10, q2.SGnA, 1e-9, "SG&A = 销售+管理 差分")
	assert.InDelta(t, 0.9, q2.EPSDiluted, 1e-9)
	assert.Equal(t, "2026-08-20", q2.NoticeDate.Format("2006-01-02"))

	q3 := byFP["2026Q3"]
	assert.InDelta(t, 110, q3.Revenue, 1e-9)
	assert.True(t, math.IsNaN(q3.RnD), "9M 的 RnD 缺失 → 差分 NaN,不得产假 0")
}

func TestFetchProfitQuartersBankRevenueFallback(t *testing.T) {
	profit := `[
	 {"REPORT_DATE":"2026-03-31 00:00:00","NOTICE_DATE":"2026-04-29 00:00:00",
	  "TOTAL_OPERATE_INCOME":null,"OPERATE_INCOME":869,"OPERATE_COST":null,
	  "OPERATE_PROFIT":446,"INCOME_TAX":66,"PARENT_NETPROFIT":378}
	]`
	srv := finServer(t, profit, `[]`)
	defer srv.Close()
	qs, err := New(srv.URL).FetchProfitQuarters("600519.SH")
	require.NoError(t, err)
	require.Len(t, qs, 1)
	assert.InDelta(t, 869, qs[0].Revenue, 1e-9, "银行口径回退 OPERATE_INCOME")
	assert.True(t, math.IsNaN(qs[0].GrossProfit), "无营业成本 → 毛利 NaN(银行降级)")
}

func TestFetchSegmentsQuarterlyAndHalfYear(t *testing.T) {
	// 甲: 季披露(03/06),差分成单季;乙: 半年披露(06/12),成 Q2/Q4 半年块;
	// 丙: 与上期间隔 ~120 天(异常),差分块丢弃。
	zygc := `[
	 {"报告日期":"2026-03-31T00:00:00.000","分类类型":"按产品分类","主营构成":"甲","主营收入":100},
	 {"报告日期":"2026-06-30T00:00:00.000","分类类型":"按产品分类","主营构成":"甲","主营收入":190},
	 {"报告日期":"2026-06-30T00:00:00.000","分类类型":"按产品分类","主营构成":"乙","主营收入":50},
	 {"报告日期":"2026-12-31T00:00:00.000","分类类型":"按产品分类","主营构成":"乙","主营收入":120},
	 {"报告日期":"2026-02-28T00:00:00.000","分类类型":"按产品分类","主营构成":"丙","主营收入":10},
	 {"报告日期":"2026-06-30T00:00:00.000","分类类型":"按产品分类","主营构成":"丙","主营收入":30},
	 {"报告日期":"2026-03-31T00:00:00.000","分类类型":"按行业分类","主营构成":"甲行业","主营收入":999}
	]`
	srv := finServer(t, `[]`, zygc)
	defer srv.Close()

	points, err := New(srv.URL).FetchSegments("600519.SH")
	require.NoError(t, err)
	ident := func(name string) (string, bool) {
		if name == "丙" || name == "甲" || name == "乙" {
			return name, true
		}
		return "", false
	}
	blocks, unmapped := DiffSegments(points, ident)
	assert.Empty(t, unmapped)

	type k struct{ fp, key string }
	got := map[k]float64{}
	for _, b := range blocks {
		got[k{b.FiscalPeriod, b.Key}] = b.Revenue
	}
	assert.InDelta(t, 100, got[k{"2026Q1", "甲"}], 1e-9)
	assert.InDelta(t, 90, got[k{"2026Q2", "甲"}], 1e-9, "季披露差分单季")
	assert.InDelta(t, 50, got[k{"2026Q2", "乙"}], 1e-9, "半年披露:H1 块标 Q2")
	assert.InDelta(t, 70, got[k{"2026Q4", "乙"}], 1e-9, "H2 = FY − H1,标 Q4")
	for kk := range got {
		assert.NotEqual(t, "丙", kk.key, "异常间隔差分块必须丢弃")
		assert.NotEqual(t, "甲行业", kk.key, "只取按产品分类")
	}
}

// [跨名差分修复 2026-08-01] 同一业务在年内改名(茅台实锤:09-30「系列酒」→年报
// 「其他系列酒」),差分必须发生在**映射到 segment_key 之后**——按原始名分组会在改名处
// 断裂(旧名年内无后继、新名成年内首期跨度异常,双双丢块)。
func TestDiffSegmentsMergesRenamedNames(t *testing.T) {
	zygc := `[
	 {"报告日期":"2026-09-30T00:00:00.000","分类类型":"按产品分类","主营构成":"系列酒","主营收入":178},
	 {"报告日期":"2026-06-30T00:00:00.000","分类类型":"按产品分类","主营构成":"系列酒","主营收入":110},
	 {"报告日期":"2026-03-31T00:00:00.000","分类类型":"按产品分类","主营构成":"系列酒","主营收入":60},
	 {"报告日期":"2026-12-31T00:00:00.000","分类类型":"按产品分类","主营构成":"其他系列酒","主营收入":222},
	 {"报告日期":"2026-12-31T00:00:00.000","分类类型":"按产品分类","主营构成":"神秘业务","主营收入":9}
	]`
	srv := finServer(t, `[]`, zygc)
	defer srv.Close()
	points, err := New(srv.URL).FetchSegments("600519.SH")
	require.NoError(t, err)

	keyOf := func(name string) (string, bool) {
		if name == "系列酒" || name == "其他系列酒" {
			return "series", true
		}
		return "", false
	}
	blocks, unmapped := DiffSegments(points, keyOf)
	got := map[string]float64{}
	for _, b := range blocks {
		got[b.FiscalPeriod] = b.Revenue
	}
	assert.InDelta(t, 44, got["2026Q4"], 1e-9, "Q4 = 222(新名累计) − 178(旧名 9M 累计)")
	assert.InDelta(t, 68, got["2026Q3"], 1e-9)
	assert.Equal(t, []string{"神秘业务"}, unmapped, "未映射名由 DiffSegments 聚合上报")
}
