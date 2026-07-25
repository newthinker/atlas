package sankey

// Context Checkpoint: done_criteria → test mapping
// functional[0] Analyze 走 DefaultSelection，Periods 含 Period/Graph/Metrics；
//               **AD-22 Graph.Notes 原样透传，装配层不得丢弃** →
//               TestAnalyzeDefaultSelection, TestAnalyzePassesThroughGraphNotes
// functional[1] from/to 范围过滤；granularity=fy 为财年聚合 →
//               TestFilterRange, TestAnalyzeGranularityAndRange,
//               TestAnalyzeMatrixComparesAgainstQuartersOutsideRange
// functional[2] AD-15 截断：>10 期取最近 10 且 Truncated=true；≤10 期为 false →
//               TestTruncateToMaxPeriods, TestAnalyzeTruncates（期望值写死 10，不引用常量）
// functional[3] 未配置模板 → ErrNoTemplate（errors.Is 可判定） →
//               TestAnalyzeNoTemplate, TestAnalyzeUnknownSymbol, TestAnalyzeTemplatedButNotInStore
// functional[4] Fundamental 季度序列 + ROE(TTM) + 价格；无数据 → prismstore.ErrNotFound →
//               TestFundamental, TestFundamentalNoData, TestFundamentalUnknownSymbol
// boundary[0]   AD-14 ROE(TTM) 分母为 0/NaN → NaN 而非 Inf；Template.Lang 随 lang 切换 →
//               TestROETTM, TestLangMap, TestAnalyzeGranularityAndRange/english_labels
// boundary[1]   迁移后新 5 列全 NULL 时不报错、节点为 NaN 且无 Inf →
//               TestAnalyzeWithUnrefreshedColumns

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	prismstore "github.com/newthinker/atlas/internal/storage/prism"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterRange(t *testing.T) {
	ps := periodsNamed("2025Q1", "2025Q2", "2025Q3", "2025Q4", "2026Q1")

	cases := []struct {
		name     string
		from, to string
		want     []string
	}{
		{"both empty keeps all", "", "", []string{"2025Q1", "2025Q2", "2025Q3", "2025Q4", "2026Q1"}},
		{"from only", "2025Q3", "", []string{"2025Q3", "2025Q4", "2026Q1"}},
		{"to only", "", "2025Q2", []string{"2025Q1", "2025Q2"}},
		{"both bounds are inclusive", "2025Q2", "2025Q4", []string{"2025Q2", "2025Q3", "2025Q4"}},
		// periodNames 对空结果返回 []string{}，故这里期望空切片而非 nil。
		{"range matching nothing", "2027Q1", "2027Q4", []string{}},
		{"inverted range yields nothing", "2026Q1", "2025Q1", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, periodNames(filterRange(ps, tc.from, tc.to)))
		})
	}
}

// AD-15：上限是 10。**期望值在这里必须写死**——若断言写成 require.Len(got, MaxPeriods)，
// 把常量改成 8 时测试会跟着变、永远为绿，变异体就杀不掉了。
func TestTruncateToMaxPeriods(t *testing.T) {
	t.Run("over the limit keeps the latest ten", func(t *testing.T) {
		ps := periodsNamed("FY2014", "FY2015", "FY2016", "FY2017", "FY2018", "FY2019",
			"FY2020", "FY2021", "FY2022", "FY2023", "FY2024", "FY2025")
		require.Len(t, ps, 12)

		got, truncated := truncate(ps)
		assert.True(t, truncated)
		require.Len(t, got, 10)
		assert.Equal(t, "FY2016", got[0].Period, "应保留最近的 10 期")
		assert.Equal(t, "FY2025", got[len(got)-1].Period)
	})

	t.Run("exactly at the limit is not truncated", func(t *testing.T) {
		ps := periodsNamed("FY2016", "FY2017", "FY2018", "FY2019", "FY2020",
			"FY2021", "FY2022", "FY2023", "FY2024", "FY2025")
		require.Len(t, ps, 10)

		got, truncated := truncate(ps)
		assert.False(t, truncated, "恰好 10 期不算截断")
		assert.Len(t, got, 10)
	})

	t.Run("under the limit is not truncated", func(t *testing.T) {
		got, truncated := truncate(periodsNamed("FY2024", "FY2025"))
		assert.False(t, truncated)
		assert.Len(t, got, 2)
	})

	t.Run("empty", func(t *testing.T) {
		got, truncated := truncate(nil)
		assert.False(t, truncated)
		assert.Empty(t, got)
	})
}

func fundEquity(period string, netIncome, equity float64) prismstore.FundamentalRow {
	return prismstore.FundamentalRow{FiscalPeriod: period, NetIncome: netIncome, Equity: equity}
}

func TestROETTM(t *testing.T) {
	t.Run("trailing four quarters over period end equity", func(t *testing.T) {
		rows := []prismstore.FundamentalRow{
			fundEquity("2025Q1", 10*b, 200*b),
			fundEquity("2025Q2", 11*b, 210*b),
			fundEquity("2025Q3", 12*b, 220*b),
			fundEquity("2025Q4", 13*b, 230*b),
			fundEquity("2026Q1", 14*b, 240*b),
		}

		got := roeTTM(rows)
		require.Len(t, got, 5, "应与输入等长，一一对应")
		for i := 0; i < 3; i++ {
			assert.True(t, math.IsNaN(got[i]), "第 %d 季不足 4 季，无法算 TTM，实得 %v", i, got[i])
		}
		assert.InDelta(t, (10.0+11+12+13)/230.0, got[3], 1e-9, "TTM(NetIncome) / 该季末 Equity")
		assert.InDelta(t, (11.0+12+13+14)/240.0, got[4], 1e-9, "窗口应随季度滚动")
	})

	t.Run("zero or NaN equity yields NaN not Inf", func(t *testing.T) {
		rows := []prismstore.FundamentalRow{
			fundEquity("2025Q1", 10*b, 200*b),
			fundEquity("2025Q2", 11*b, 210*b),
			fundEquity("2025Q3", 12*b, 220*b),
			fundEquity("2025Q4", 13*b, 0),
			fundEquity("2026Q1", 14*b, math.NaN()),
		}

		got := roeTTM(rows)
		require.Len(t, got, 5)
		for _, i := range []int{3, 4} {
			assert.True(t, math.IsNaN(got[i]), "第 %d 季分母不可用 → NaN，实得 %v", i, got[i])
			assert.False(t, math.IsInf(got[i], 0),
				"±Inf 会让 encoding/json 报错并使整个 API 返回 500，jf() 只映射 NaN")
		}
	})

	t.Run("missing net income propagates as NaN", func(t *testing.T) {
		rows := []prismstore.FundamentalRow{
			fundEquity("2025Q1", math.NaN(), 200*b),
			fundEquity("2025Q2", 11*b, 210*b),
			fundEquity("2025Q3", 12*b, 220*b),
			fundEquity("2025Q4", 13*b, 230*b),
		}

		got := roeTTM(rows)
		require.Len(t, got, 4)
		assert.True(t, math.IsNaN(got[3]), "窗口内有缺值时不得把缺失当 0 计入，实得 %v", got[3])
	})

	t.Run("fewer than four quarters", func(t *testing.T) {
		got := roeTTM([]prismstore.FundamentalRow{fundEquity("2025Q1", 10*b, 200*b)})
		require.Len(t, got, 1)
		assert.True(t, math.IsNaN(got[0]))
	})

	t.Run("empty", func(t *testing.T) {
		assert.Empty(t, roeTTM(nil))
	})
}

func TestLangMap(t *testing.T) {
	tmpl := twoSegTemplate()
	assert.Equal(t, map[string]string{"cloud": "云业务", "devices": "硬件设备"}, langMap(tmpl, "zh"))
	assert.Equal(t, map[string]string{"cloud": "Cloud", "devices": "Devices"}, langMap(tmpl, "en"))
	assert.Empty(t, langMap(nil, "zh"))
}

// fakeServiceStore 沿用 refresh_test.go 的内存 fake 惯例。
type fakeServiceStore struct {
	ids    map[string]int64
	funds  map[int64][]prismstore.FundamentalRow
	segs   map[int64][]prismstore.SegmentRow
	dates  map[string][]string
	closes map[string][]float64
	err    error
}

func (f *fakeServiceStore) InstrumentID(symbol string) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	id, ok := f.ids[symbol]
	if !ok {
		return 0, fmt.Errorf("%w: %s", prismstore.ErrNotFound, symbol)
	}
	return id, nil
}

func (f *fakeServiceStore) QuarterlyFundamentals(id int64) ([]prismstore.FundamentalRow, error) {
	return f.funds[id], nil
}

func (f *fakeServiceStore) SegmentRows(id int64) ([]prismstore.SegmentRow, error) {
	return f.segs[id], nil
}

func (f *fakeServiceStore) PriceSeries(symbol, from string) ([]string, []float64, error) {
	return f.dates[symbol], f.closes[symbol], nil
}

// storeWith 造一个 ACME：4 个季度齐（可聚合成 FY2026），分部数据故意让 Σsegments < Revenue，
// 使 other_segment 残差非零。
func storeWith(t *testing.T) *fakeServiceStore {
	t.Helper()
	return &fakeServiceStore{
		ids: map[string]int64{"ACME": 7},
		funds: map[int64][]prismstore.FundamentalRow{7: {
			fq("2026Q1", "2025-09-30", 100*b, 60*b, 30*b, 24*b, 8*b, 7*b, 5*b),
			fq("2026Q2", "2025-12-31", 110*b, 66*b, 33*b, 26*b, 8*b, 7*b, 5*b),
			fq("2026Q3", "2026-03-31", 120*b, 70*b, 34*b, 28*b, 9*b, 8*b, 6*b),
			fq("2026Q4", "2026-06-30", 130*b, 78*b, 39*b, 30*b, 9*b, 8*b, 6*b),
		}},
		segs: map[int64][]prismstore.SegmentRow{7: {
			sr("2026Q1", "2025-09-30", "cloud", 50*b),
			sr("2026Q1", "2025-09-30", "devices", 30*b),
			sr("2026Q2", "2025-12-31", "cloud", 55*b),
			sr("2026Q3", "2026-03-31", "cloud", 60*b),
			sr("2026Q4", "2026-06-30", "cloud", 65*b),
		}},
		dates:  map[string][]string{"ACME": {"2026-06-29", "2026-06-30"}},
		closes: map[string][]float64{"ACME": {101.5, 102.5}},
	}
}

func newTestService(t *testing.T, store ServiceStore) *Service {
	t.Helper()
	return NewService(store, map[string]*Template{"ACME": twoSegTemplate()})
}

func TestAnalyzeDefaultSelection(t *testing.T) {
	svc := newTestService(t, storeWith(t))

	got, err := svc.Analyze("ACME", "", "", "", "zh")
	require.NoError(t, err)
	assert.Equal(t, "ACME", got.Symbol)

	// 4 季齐 → DefaultSelection 走年度分支。
	assert.Equal(t, "fy", got.Granularity)
	require.Len(t, got.Periods, 1)
	assert.Equal(t, "FY2026", got.Periods[0].Period)
	assert.Equal(t, 460*b, got.Periods[0].Metrics.Revenue, "PeriodView 应带上该期的 Metrics")
	assert.NotEmpty(t, got.Periods[0].Graph.Links, "PeriodView 应带上该期的 Graph")
	assert.False(t, got.Truncated, "只有 1 期，不应判为截断")

	assert.NotEmpty(t, got.Matrix)
	assert.Equal(t, 1, got.Template.Version)
	assert.Equal(t, map[string]string{"cloud": "云业务", "devices": "硬件设备"}, got.Template.Lang)
}

// AD-22：BuildSankey 记录的负流 footnote 必须原样透传到 PeriodView.Graph.Notes。
// 装配层若丢弃它，没有任何别的测试会红，而负残差会退回「图上凭空少一块且无人知情」。
func TestAnalyzePassesThroughGraphNotes(t *testing.T) {
	store := storeWith(t)
	// NetIncome > OperatingIncome（利息收入大的公司）→ tax_other 为负 → 必产生 Notes。
	rows := store.funds[7]
	for i := range rows {
		rows[i].NetIncome = rows[i].OperatingIncome + 5*b
	}

	got, err := newTestService(t, store).Analyze("ACME", "", "", "", "zh")
	require.NoError(t, err)
	require.NotEmpty(t, got.Periods)

	var withNotes int
	for _, pv := range got.Periods {
		if len(pv.Graph.Notes) > 0 {
			withNotes++
			joined := strings.Join(pv.Graph.Notes, "\n")
			assert.Contains(t, joined, "税项及其他", "footnote 应点明为负的节点")
		}
		for _, l := range pv.Graph.Links {
			assert.GreaterOrEqual(t, l.Value, 0.0, "负流不得画出来")
		}
	}
	assert.Positive(t, withNotes, "tax_other 为负时 Notes 必须一路传到 PeriodView，不得在装配层丢弃")
}

func TestAnalyzeGranularityAndRange(t *testing.T) {
	svc := newTestService(t, storeWith(t))

	t.Run("explicit quarterly", func(t *testing.T) {
		got, err := svc.Analyze("ACME", "", "", "q", "zh")
		require.NoError(t, err)
		assert.Equal(t, "q", got.Granularity)
		assert.Equal(t, []string{"2026Q1", "2026Q2", "2026Q3", "2026Q4"}, viewNames(got.Periods))
	})

	t.Run("range filter is inclusive", func(t *testing.T) {
		got, err := svc.Analyze("ACME", "2026Q2", "2026Q3", "q", "zh")
		require.NoError(t, err)
		assert.Equal(t, []string{"2026Q2", "2026Q3"}, viewNames(got.Periods))
	})

	t.Run("fy aggregates", func(t *testing.T) {
		got, err := svc.Analyze("ACME", "", "", "fy", "zh")
		require.NoError(t, err)
		assert.Equal(t, "fy", got.Granularity)
		require.Len(t, got.Periods, 1)
		assert.Equal(t, "FY2026", got.Periods[0].Period)
		assert.Equal(t, 274*b, got.Periods[0].Metrics.GrossProfit, "财年应为 4 季之和")
	})

	t.Run("english labels", func(t *testing.T) {
		got, err := svc.Analyze("ACME", "", "", "fy", "en")
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"cloud": "Cloud", "devices": "Devices"}, got.Template.Lang)
		assert.True(t, hasNode(got.Periods[0].Graph, "Revenue"), "图上节点名也应随 lang 切换")
	})
}

// AD-15：超过 10 期只留最近 10 期。期望值写死 10，不引用 MaxPeriods。
func TestAnalyzeTruncates(t *testing.T) {
	store := storeWith(t)
	var rows []prismstore.FundamentalRow
	for y := 2015; y <= 2026; y++ { // 12 个财年 × 4 季
		for q := 1; q <= 4; q++ {
			rows = append(rows, fq(fmt.Sprintf("%dQ%d", y, q), fmt.Sprintf("%d-03-31", y),
				100*b, 60*b, 30*b, 24*b, 8*b, 7*b, 5*b))
		}
	}
	store.funds[7] = rows

	got, err := newTestService(t, store).Analyze("ACME", "", "", "fy", "zh")
	require.NoError(t, err)
	assert.True(t, got.Truncated, "12 个财年超过上限，应置 Truncated")
	require.Len(t, got.Periods, 10)
	assert.Equal(t, "FY2017", got.Periods[0].Period, "应保留最近 10 期")
	assert.Equal(t, "FY2026", got.Periods[len(got.Periods)-1].Period)
}

func TestAnalyzeNoTemplate(t *testing.T) {
	svc := NewService(storeWith(t), map[string]*Template{})
	_, err := svc.Analyze("ACME", "", "", "", "zh")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoTemplate), "未配置模板应返回可 errors.Is 判定的哨兵错误")
	assert.Contains(t, err.Error(), "ACME")
}

func TestAnalyzeUnknownSymbol(t *testing.T) {
	svc := newTestService(t, storeWith(t))
	_, err := svc.Analyze("NOPE", "", "", "", "zh")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoTemplate), "无模板的 symbol 先于取数被拒")
}

// 迁移后尚未重刷时，fundamental_q 有行但主干流 5 列全为 NULL（读回 NaN）：
// 不得报错，节点保持 NaN 交前端渲染「—」，且绝不能出现 Inf。
func TestAnalyzeWithUnrefreshedColumns(t *testing.T) {
	store := storeWith(t)
	nan := math.NaN()
	store.funds[7] = []prismstore.FundamentalRow{
		{FiscalPeriod: "2026Q1", PeriodEnd: "2025-09-30", Revenue: 100 * b, NetIncome: 24 * b,
			GrossProfit: nan, RnD: nan, SGnA: nan, OperatingIncome: nan, IncomeTax: nan},
	}

	got, err := newTestService(t, store).Analyze("ACME", "", "", "q", "zh")
	require.NoError(t, err, "新列未回填不是错误")
	require.Len(t, got.Periods, 1)
	assert.True(t, math.IsNaN(got.Periods[0].Metrics.GrossProfit))
	for _, l := range got.Periods[0].Graph.Links {
		assert.False(t, math.IsInf(l.Value, 0), "link %s→%s 是 Inf 会让 json.Marshal 失败、API 500",
			l.Source, l.Target)
	}
	for _, r := range got.Matrix {
		assert.False(t, math.IsInf(r.Change, 0), "row %s 的 Change 是 Inf", r.Key)
	}
}

func TestFundamental(t *testing.T) {
	store := storeWith(t)
	rows := store.funds[7]
	for i := range rows {
		rows[i].Equity = float64(200+i*10) * b
	}
	svc := newTestService(t, store)

	got, err := svc.Fundamental("ACME")
	require.NoError(t, err)
	assert.Equal(t, "ACME", got.Symbol)
	assert.Equal(t, []string{"2026Q1", "2026Q2", "2026Q3", "2026Q4"}, got.Periods, "fiscal_period 升序")
	assert.Equal(t, []string{"2026-06-29", "2026-06-30"}, got.Dates)
	assert.Equal(t, []float64{101.5, 102.5}, got.Closes)

	require.Contains(t, got.Metrics, "revenue")
	assert.Equal(t, []float64{100 * b, 110 * b, 120 * b, 130 * b}, got.Metrics["revenue"])
	require.Contains(t, got.Metrics, "roe_ttm")
	roe := got.Metrics["roe_ttm"]
	require.Len(t, roe, 4)
	for i := 0; i < 3; i++ {
		assert.True(t, math.IsNaN(roe[i]), "不足 4 季不得用部分和冒充 TTM，第 %d 季实得 %v", i, roe[i])
	}
	assert.InDelta(t, (24.0+26+28+30)/230.0, roe[3], 1e-9)
}

func TestFundamentalNoData(t *testing.T) {
	store := storeWith(t)
	store.funds[7] = nil
	_, err := newTestService(t, store).Fundamental("ACME")
	require.Error(t, err)
	assert.True(t, errors.Is(err, prismstore.ErrNotFound), "无 fundamental_q 数据应返回 ErrNotFound")
}

func TestFundamentalUnknownSymbol(t *testing.T) {
	_, err := newTestService(t, storeWith(t)).Fundamental("NOPE")
	require.Error(t, err)
	assert.True(t, errors.Is(err, prismstore.ErrNotFound))
}

func viewNames(vs []PeriodView) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Period)
	}
	return out
}

// 配了模板但该标的还没被刷进 instrument 表（never refreshed）：错误应原样透出，
// 而不是被伪装成「没有模板」。
func TestAnalyzeTemplatedButNotInStore(t *testing.T) {
	store := storeWith(t)
	delete(store.ids, "ACME")
	_, err := newTestService(t, store).Analyze("ACME", "", "", "", "zh")
	require.Error(t, err)
	assert.True(t, errors.Is(err, prismstore.ErrNotFound), "应透出 store 的 ErrNotFound")
	assert.False(t, errors.Is(err, ErrNoTemplate), "模板是存在的，不得报成 ErrNoTemplate")
}

// LoadTemplates 的 map key 是大写 symbol（TASK-003 契约），查模板时必须归一，
// 否则小写入参会被误判成「未配置模板」。
func TestAnalyzeSymbolCaseInsensitiveTemplateLookup(t *testing.T) {
	store := storeWith(t)
	store.ids["acme"] = 7
	got, err := newTestService(t, store).Analyze("acme", "", "", "fy", "zh")
	require.NoError(t, err, "小写 symbol 也应命中大写 key 的模板")
	assert.Equal(t, "acme", got.Symbol, "Symbol 原样回显调用方传入的写法")
	assert.NotEmpty(t, got.Periods)
}

// 季度同比的对照期通常**落在选择范围之外**（选 2026Q1~Q2 时要对比 2025Q1~Q2）。
// 装配层必须把「全部季度」而不是「选中的季度」交给 BuildMatrix，否则一旦用户缩小
// 范围，同比就会静默退化为 none —— 页面上看不出错，只是那一列空了。
func TestAnalyzeMatrixComparesAgainstQuartersOutsideRange(t *testing.T) {
	store := storeWith(t)
	store.funds[7] = append([]prismstore.FundamentalRow{
		fq("2025Q1", "2024-09-30", 80*b, 48*b, 24*b, 20*b, 6*b, 5*b, 4*b),
		fq("2025Q2", "2024-12-31", 88*b, 52*b, 26*b, 21*b, 6*b, 5*b, 4*b),
		fq("2025Q3", "2025-03-31", 96*b, 56*b, 28*b, 22*b, 7*b, 6*b, 5*b),
		fq("2025Q4", "2025-06-30", 104*b, 60*b, 30*b, 23*b, 7*b, 6*b, 5*b),
	}, store.funds[7]...)

	// 只选 2026Q2，其对照期 2025Q2 不在选择范围内。
	got, err := newTestService(t, store).Analyze("ACME", "2026Q2", "2026Q2", "q", "zh")
	require.NoError(t, err)
	require.Equal(t, []string{"2026Q2"}, viewNames(got.Periods))

	rev := rowByKey(t, got.Matrix, "revenue")
	assert.Equal(t, "yoy", rev.ChangeKind, "对照期在范围外也应算出同比，而不是退化成 none")
	assert.InDelta(t, 110.0/88.0-1, rev.Change, 1e-9, "应对比 2025Q2 的 88b")
}

// 拒绝聚合本身不够——拒绝的理由必须一路传到调用方，否则页面只会「少了一年」而无人知情。
func TestAnalyzeSurfacesPeriodConflicts(t *testing.T) {
	store := storeWith(t)
	// 让 FY2026 收到 6 个季度：2026Q4 这个标签被三份报告重复使用（真实 MSFT 形态）。
	store.funds[7] = append(store.funds[7],
		fq("2026Q4", "2024-06-30", 90*b, 54*b, 27*b, 21*b, 7*b, 6*b, 4*b),
		fq("2026Q4", "2023-06-30", 80*b, 48*b, 24*b, 19*b, 6*b, 5*b, 4*b),
	)

	got, err := newTestService(t, store).Analyze("ACME", "", "", "fy", "zh")
	require.NoError(t, err, "上游标签冲突不是本服务的错误，应降级而不是报错")
	assert.Empty(t, got.Periods, "6 个季度的财年必须被拒绝")

	require.NotEmpty(t, got.Conflicts, "冲突必须透传到 Analysis，否则页面上只是「少了一年」")
	text := conflictText(got.Conflicts)
	assert.Contains(t, text, "FY2026", "应点名被拒的财年")
	assert.Contains(t, text, "2026Q4", "也应点名撞车的季度标签")
}
