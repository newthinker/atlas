package prism

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/collector/akshare"
	"github.com/newthinker/atlas/internal/collector/edgar"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/prism/sankey"
	prismstore "github.com/newthinker/atlas/internal/storage/prism"
)

// ---------------------------------------------------------------------------
// TASK-008 done_criteria → test mapping
//   functional[0] since==LatestSegmentPeriodEnd 锚点;已映射 member 落 edgar_segment;
//                 未映射 member 进 Degraded 且整体不失败
//                                        → TestRefreshSegmentsIncrementalAndMapping
//   functional[1] AD-12 force=true 忽略锚点、传零值 since 全量重拉
//                                        → TestRefreshSegmentsForceIgnoresAnchor
//   functional[2] AD-9 主键 period_end;fiscal_period ±3 天反查;反查失败仍落库+Degraded;
//                 容差内命中 ≥2 条报错。容差量值由 −4/−3/0/+3/+4 五档钉死 ——
//                 必须取到「正好 ±3」这一档,否则 3→2 与 <= 改 < 两类退化无人拦
//                                        → TestRefreshSegmentsFiscalPeriodLookup
//   functional[3] Q4 = FY − 已存 3 季(逐 segment);凑不齐跳过;负值不落库+Degraded;
//                 AD-17 FY 期本身不落库
//                                        → TestRefreshSegmentsQ4Derivation
//   functional[4] manual 最后 upsert 覆盖同键自动行;下一次 auto 刷新再次覆盖 manual
//                                        → TestRefreshSegmentsManualOverride
//   boundary[0]   无模板的标的被跳过;templates 为空 map → 零值 Report 且不报错
//                                        → TestRefreshSegmentsSkipsUntemplated
//   error[0]      单标的 FetchSegmentRevenue 失败只进 Failed,其余标的继续落库
//                                        → TestRefreshSegmentsPartialFailure
//
// 超出 DoD 的补充用例(理由见各自注释):AD-16(3) 模板串号校验、存储各步失败、
// manual 文件写坏、只报 10-K 时的未映射 member、fundamental_q 坏日期、
// Q4 跨轮次重算(自吞噬防护)、财年级季度数守卫、Degraded 与落库行的顺序确定性。
//
// 跨轮次纪律:凡「读回自己刚写的数据」的逻辑(此处是 Q4 推导)都必须连跑两轮再断言 ——
// 这类 bug 首轮结构上就不可能显形,单轮用例再多也看不见。
// ---------------------------------------------------------------------------

// 财年布局(MSFT 式 6/30 结算),供全部用例复用:
//
//	Q1 2025-07-01~2025-09-30  Q2 2025-10-01~2025-12-31
//	Q3 2026-01-01~2026-03-31  FY 2025-07-01~2026-06-30(Q4 end 即 FY end)
var (
	q1End          = date(2025, 9, 30)
	q2End          = date(2025, 12, 31)
	q3Start, q3End = date(2026, 1, 1), date(2026, 3, 31)
	fyStart, fyEnd = date(2025, 7, 1), date(2026, 6, 30)
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

type fakeSegClient struct {
	periods map[string][]edgar.SegmentPeriod // cik → 返回的期
	fail    map[string]error                 // cik → 注入失败
	calls   map[string]segCall               // cik → 参数捕获
}

type segCall struct {
	axis  string
	since time.Time
}

func (f *fakeSegClient) FetchSegmentRevenue(cik, axis string, since time.Time) ([]edgar.SegmentPeriod, error) {
	if f.calls == nil {
		f.calls = map[string]segCall{}
	}
	f.calls[cik] = segCall{axis, since}
	if err := f.fail[cik]; err != nil {
		return nil, err
	}
	return f.periods[cik], nil
}

func segInst(symbol, cik string) config.PrismInstrument {
	return config.PrismInstrument{Symbol: symbol, Name: symbol, Type: "stock",
		Market: "US", Group: "美股公司", Source: "edgar", CIK: cik}
}

func segCfg(insts ...config.PrismInstrument) config.PrismConfig {
	c := config.PrismConfig{Instruments: insts}
	c.ApplyDefaults()
	return c
}

// msftTemplate 映射两个 member;第三个 member(UnmappedMember)刻意不在模板里。
func msftTemplate() map[string]*sankey.Template {
	return map[string]*sankey.Template{
		"MSFT": {
			Company:     "MSFT",
			CIK:         "789019",
			SegmentAxis: sankey.DefaultSegmentAxis,
			Segments: []sankey.Segment{
				{Key: "intelligent_cloud", NameZH: "智能云", NameEN: "Intelligent Cloud", Member: "IntelligentCloudMember"},
				{Key: "productivity", NameZH: "生产力", NameEN: "Productivity", Member: "ProductivityMember"},
			},
		},
	}
}

// seedFundamentals 预置 fundamental_q,供 fiscal_period 反查。
func seedFundamentals(t *testing.T, s *fakeStore, symbol string, byEnd map[string]string) {
	t.Helper()
	id, err := s.UpsertInstrument(prismstore.Instrument{Symbol: symbol, Type: "stock"})
	require.NoError(t, err)
	rows := make([]prismstore.FundamentalRow, 0, len(byEnd))
	for end, fp := range byEnd {
		rows = append(rows, prismstore.FundamentalRow{PeriodEnd: end, FiscalPeriod: fp, Source: "edgar"})
	}
	require.NoError(t, s.UpsertFundamentals(id, rows))
}

// rowsByKey 把 SegmentRows 结果转成 period_end|segment_key → row,便于精确断言。
func rowsByKey(t *testing.T, s *fakeStore, symbol string) map[string]prismstore.SegmentRow {
	t.Helper()
	out := map[string]prismstore.SegmentRow{}
	for _, r := range s.segments[symbol] {
		out[r.PeriodEnd+"|"+r.SegmentKey] = r
	}
	return out
}

func TestRefreshSegmentsIncrementalAndMapping(t *testing.T) {
	store := newFakeStore()
	seedFundamentals(t, store, "MSFT", map[string]string{"2026-03-31": "2026Q3"})
	// 预置一条历史分部行 → 锚点 = 2025-12-31
	require.NoError(t, store.UpsertSegments(1, []prismstore.SegmentRow{
		{PeriodEnd: "2025-12-31", SegmentKey: "productivity", Revenue: 29.4e9, Source: "edgar_segment"},
	}))

	seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{
		"789019": {{
			PeriodStart: q3Start, PeriodEnd: q3End, FilingDate: date(2026, 4, 24), Form: "10-Q",
			Values: map[string]float64{
				"IntelligentCloudMember": 26.7e9,
				"ProductivityMember":     29.9e9,
				"UnmappedMember":         1.2e9, // 模板未映射
			},
		}},
	}}

	rep := RefreshSegments(segCfg(segInst("MSFT", "789019")), store, seg, nil, msftTemplate(), "", false)

	require.Empty(t, rep.Failed, "未映射 member 不得让整体失败")
	assert.Equal(t, 1, rep.Refreshed)
	// functional[0] since 等于锚点,axis 取模板值
	assert.Equal(t, "2025-12-31", seg.calls["789019"].since.Format("2006-01-02"), "since 须等于 LatestSegmentPeriodEnd 锚点")
	assert.Equal(t, sankey.DefaultSegmentAxis, seg.calls["789019"].axis)
	// 已映射 member 落库
	got := rowsByKey(t, store, "MSFT")
	ic := got["2026-03-31|intelligent_cloud"]
	assert.Equal(t, 26.7e9, ic.Revenue)
	assert.Equal(t, "edgar_segment", ic.Source)
	assert.Equal(t, "2026Q3", ic.FiscalPeriod, "fiscal_period 由 fundamental_q 反查")
	assert.Equal(t, 29.9e9, got["2026-03-31|productivity"].Revenue)
	// 未映射 member 既不落库,也不静默 —— 进 Degraded 且文本可定位
	assert.NotContains(t, got, "2026-03-31|UnmappedMember")
	require.Len(t, rep.Degraded, 1)
	assert.Contains(t, rep.Degraded[0], "MSFT")
	assert.Contains(t, rep.Degraded[0], "UnmappedMember")
	assert.Contains(t, rep.Degraded[0], "2026-03-31", "降级说明须能定位到具体期间")
}

func TestRefreshSegmentsForceIgnoresAnchor(t *testing.T) {
	// functional[1]/AD-12:纯增量下「跑一次看 member → 改模板 → 再跑」第二次拉不到数据,
	// 模板迭代流程走不通。force=true 必须忽略锚点传零值 since 全量重拉。
	store := newFakeStore()
	seedFundamentals(t, store, "MSFT", map[string]string{"2026-03-31": "2026Q3"})
	require.NoError(t, store.UpsertSegments(1, []prismstore.SegmentRow{
		{PeriodEnd: "2026-03-31", SegmentKey: "productivity", Revenue: 1, Source: "edgar_segment"},
	}))
	seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{
		"789019": {{PeriodStart: q3Start, PeriodEnd: q3End, Form: "10-Q",
			Values: map[string]float64{"ProductivityMember": 29.9e9}}},
	}}
	cfg := segCfg(segInst("MSFT", "789019"))

	// force=false:锚点已推进到 2026-03-31
	RefreshSegments(cfg, store, seg, nil, msftTemplate(), "", false)
	assert.Equal(t, "2026-03-31", seg.calls["789019"].since.Format("2006-01-02"))

	// force=true:忽略锚点
	RefreshSegments(cfg, store, seg, nil, msftTemplate(), "", true)
	assert.True(t, seg.calls["789019"].since.IsZero(), "force=true 须传零值 since 全量重拉")
	// 全量重拉的值照常覆盖
	assert.Equal(t, 29.9e9, rowsByKey(t, store, "MSFT")["2026-03-31|productivity"].Revenue)
}

func TestRefreshSegmentsFiscalPeriodLookup(t *testing.T) {
	tpl := msftTemplate()
	cfg := segCfg(segInst("MSFT", "789019"))
	period := func(end time.Time) []edgar.SegmentPeriod {
		return []edgar.SegmentPeriod{{PeriodStart: end.AddDate(0, -3, 0), PeriodEnd: end, Form: "10-Q",
			Values: map[string]float64{"ProductivityMember": 29.9e9}}}
	}

	// 容差边界:分部期末固定 2026-03-31,平移 fundamental_q 的 period_end 逐档验。
	// ±3 必须命中、±4 必须不命中 —— 必须取到「正好等于阈值」这一档,否则容差常量
	// 改小(3→2)与边界改排他(<= 改 <)两类退化都不会有任何断言变红:只测「明显命中」
	// (+2)与「明显不命中」(+4)时,两者恰好都落在退化后的行为区间里。
	for _, tc := range []struct {
		offsetDays int
		wantHit    bool
	}{
		{-4, false}, {-3, true}, {0, true}, {3, true}, {4, false},
	} {
		t.Run(fmt.Sprintf("容差边界offset%+dd", tc.offsetDays), func(t *testing.T) {
			store := newFakeStore()
			fundEnd := q3End.AddDate(0, 0, tc.offsetDays).Format("2006-01-02")
			seedFundamentals(t, store, "MSFT", map[string]string{fundEnd: "2026Q3"})
			seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{"789019": period(q3End)}}

			rep := RefreshSegments(cfg, store, seg, nil, tpl, "", false)

			require.Empty(t, rep.Failed)
			row, ok := rowsByKey(t, store, "MSFT")["2026-03-31|productivity"]
			// AD-9 负向断言:反查命中与否都不影响落库,只影响展示标签
			require.True(t, ok, "反查结果不得影响是否落库")
			assert.Equal(t, 29.9e9, row.Revenue)
			assert.Equal(t, "2026-03-31", row.PeriodEnd, "AD-9:主键用分部期自身的 period_end,不用 fundamental_q 的")
			if tc.wantHit {
				assert.Equal(t, "2026Q3", row.FiscalPeriod, "偏差 %+dd 在 ±%d 容差内须命中", tc.offsetDays, fiscalPeriodToleranceDays)
				assert.Empty(t, rep.Degraded)
			} else {
				assert.Equal(t, "", row.FiscalPeriod, "偏差 %+dd 超出 ±%d 容差须落空标签", tc.offsetDays, fiscalPeriodToleranceDays)
				require.Len(t, rep.Degraded, 1)
				assert.Contains(t, rep.Degraded[0], "2026-03-31")
			}
		})
	}

	t.Run("容差内命中≥2 条报错而非取首个", func(t *testing.T) {
		store := newFakeStore()
		seedFundamentals(t, store, "MSFT", map[string]string{
			"2026-03-30": "2026Q3", "2026-04-01": "2026Q4",
		})
		seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{"789019": period(q3End)}}
		rep := RefreshSegments(cfg, store, seg, nil, tpl, "", false)

		assert.Equal(t, 0, rep.Refreshed)
		require.Len(t, rep.Failed, 1)
		assert.Contains(t, rep.Failed[0], "MSFT")
		assert.Contains(t, rep.Failed[0], "2026-03-31", "错误须能定位到歧义期间")
	})
}

func TestRefreshSegmentsQ4Derivation(t *testing.T) {
	tpl := msftTemplate()
	cfg := segCfg(segInst("MSFT", "789019"))
	fyPeriod := func(vals map[string]float64) []edgar.SegmentPeriod {
		return []edgar.SegmentPeriod{{PeriodStart: fyStart, PeriodEnd: fyEnd, Form: "10-K", Values: vals}}
	}
	// 三季已存:ic 10+11+12=33,prod 20+21+22=63
	seedQuarters := func(t *testing.T, s *fakeStore) {
		t.Helper()
		require.NoError(t, s.UpsertSegments(1, []prismstore.SegmentRow{
			{PeriodEnd: q1End.Format("2006-01-02"), SegmentKey: "intelligent_cloud", Revenue: 10, Source: "edgar_segment"},
			{PeriodEnd: q2End.Format("2006-01-02"), SegmentKey: "intelligent_cloud", Revenue: 11, Source: "edgar_segment"},
			{PeriodEnd: q3End.Format("2006-01-02"), SegmentKey: "intelligent_cloud", Revenue: 12, Source: "edgar_segment"},
			{PeriodEnd: q1End.Format("2006-01-02"), SegmentKey: "productivity", Revenue: 20, Source: "edgar_segment"},
			{PeriodEnd: q2End.Format("2006-01-02"), SegmentKey: "productivity", Revenue: 21, Source: "edgar_segment"},
			{PeriodEnd: q3End.Format("2006-01-02"), SegmentKey: "productivity", Revenue: 22, Source: "edgar_segment"},
		}))
	}

	t.Run("三季齐备逐 segment 推导 Q4", func(t *testing.T) {
		store := newFakeStore()
		seedFundamentals(t, store, "MSFT", map[string]string{"2026-06-30": "2026Q4"})
		seedQuarters(t, store)
		seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{
			"789019": fyPeriod(map[string]float64{"IntelligentCloudMember": 50, "ProductivityMember": 90}),
		}}
		rep := RefreshSegments(cfg, store, seg, nil, tpl, "", true)
		require.Empty(t, rep.Failed)

		got := rowsByKey(t, store, "MSFT")
		q4ic := got["2026-06-30|intelligent_cloud"]
		require.NotZero(t, q4ic.PeriodEnd, "Q4 行须落库")
		// AD-17 负向断言:落的是差值 17(=50−33),不是 FY 总额 50
		assert.Equal(t, 17.0, q4ic.Revenue, "Q4=FY−Σ三季;若落成 50 说明 FY 期被直接落库")
		assert.Equal(t, "edgar_segment", q4ic.Source)
		assert.Equal(t, 27.0, got["2026-06-30|productivity"].Revenue, "90−63")
		// FY 期不产生除 Q4 外的额外行:该财年共 3 季 + 1 个 Q4 = 每 segment 4 行
		assert.Len(t, store.segments["MSFT"], 8, "2 segment × 4 季,FY 期本身不额外落库")
	})

	t.Run("跨轮次:Q4 已存在时仍可重算并传播重述", func(t *testing.T) {
		// Q4 自吞噬防护(严格上界 d.Before(FY.PeriodEnd))**只在第二轮显形**:
		// 首轮 Q4 尚不存在,落在 FY 区间内的季度正好 3 个,闭上界与严格上界行为完全相同。
		// 第二轮 Q4 已存在且其 period_end 恰等于 FY 期末 —— 闭上界会把 Q4 自身数进来
		// (len(ends)==4)→ 整个财年被跳过 → Q4 从此永不重算,重述数据永远进不来,
		// 且值不变、无报错、无 Degraded,是彻底静默的失效。
		//
		// 单轮用例结构上看不见这个 bug,故必须连跑两轮;第二轮同时改 FY 值,
		// 一并钉住「重述可传播」——只断言「第二轮仍是 17」的话,一个「第二轮整个跳过」
		// 的实现也会通过(值恰好没变)。
		store := newFakeStore()
		seedFundamentals(t, store, "MSFT", map[string]string{"2026-06-30": "2026Q4"})
		seedQuarters(t, store)
		seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{
			"789019": fyPeriod(map[string]float64{"IntelligentCloudMember": 50, "ProductivityMember": 90}),
		}}

		require.Empty(t, RefreshSegments(cfg, store, seg, nil, tpl, "", true).Failed)
		require.Equal(t, 17.0, rowsByKey(t, store, "MSFT")["2026-06-30|intelligent_cloud"].Revenue,
			"第一轮:50−33=17")

		// 第二轮:年报重述,FY 从 50 改为 60 → Q4 应随之更新为 60−33=27
		seg.periods["789019"] = fyPeriod(map[string]float64{"IntelligentCloudMember": 60, "ProductivityMember": 90})
		require.Empty(t, RefreshSegments(cfg, store, seg, nil, tpl, "", true).Failed)

		got := rowsByKey(t, store, "MSFT")
		assert.Equal(t, 27.0, got["2026-06-30|intelligent_cloud"].Revenue,
			"第二轮:Q4 须能重算并吸收重述(仍为 17 说明该财年被整个跳过,Q4 已被自己吞掉)")
		assert.Equal(t, 27.0, got["2026-06-30|productivity"].Revenue, "未重述的 segment 保持 90−63")
		assert.Len(t, store.segments["MSFT"], 8, "重算不得产生额外行")
	})

	t.Run("财年内季度数不为 3 时整期跳过", func(t *testing.T) {
		// 财年级守卫 len(ends)!=3:区间内出现第 4 个季度末(重述/过渡期/口径变更)时,
		// 逐 segment 守卫挡不住「某 segment 恰好只有 3 个值」的组合 —— 那 3 个值配的是
		// 4 季跨度,减出来的不是 Q4。此时必须整期跳过。
		store := newFakeStore()
		seedFundamentals(t, store, "MSFT", map[string]string{"2026-06-30": "2026Q4"})
		seedQuarters(t, store)
		// 区间内插入第 4 个季度末,只给 productivity 供数:
		// ends 变成 4 个,而 intelligent_cloud 仍恰好是 3 个值 —— 正是逐 segment 守卫
		// 放行、财年级守卫才拦得住的那个组合。
		require.NoError(t, store.UpsertSegments(1, []prismstore.SegmentRow{
			{PeriodEnd: "2026-02-15", SegmentKey: "productivity", Revenue: 5, Source: "edgar_segment"},
		}))
		seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{
			"789019": fyPeriod(map[string]float64{"IntelligentCloudMember": 50, "ProductivityMember": 90}),
		}}

		require.Empty(t, RefreshSegments(cfg, store, seg, nil, tpl, "", true).Failed)

		got := rowsByKey(t, store, "MSFT")
		_, ok := got["2026-06-30|intelligent_cloud"]
		assert.False(t, ok, "财年内季度数不为 3 时整期跳过,不得拿 3 个值配 4 季跨度硬推")
		_, ok = got["2026-06-30|productivity"]
		assert.False(t, ok, "同期其余 segment 一并跳过")
	})

	t.Run("某 segment 三季凑不齐则跳过该 segment", func(t *testing.T) {
		store := newFakeStore()
		seedFundamentals(t, store, "MSFT", map[string]string{"2026-06-30": "2026Q4"})
		seedQuarters(t, store)
		// 抹掉 productivity 的 Q2 → 该 segment 只有 2 季(其余两 segment 仍是 3 季)
		store.segments["MSFT"] = removeRow(store.segments["MSFT"], q2End.Format("2006-01-02"), "productivity")

		seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{
			"789019": fyPeriod(map[string]float64{"IntelligentCloudMember": 50, "ProductivityMember": 90}),
		}}
		rep := RefreshSegments(cfg, store, seg, nil, tpl, "", true)
		require.Empty(t, rep.Failed)

		got := rowsByKey(t, store, "MSFT")
		assert.Equal(t, 17.0, got["2026-06-30|intelligent_cloud"].Revenue, "齐备的 segment 照常推导")
		_, ok := got["2026-06-30|productivity"]
		assert.False(t, ok, "凑不齐的 segment 跳过,不得用 2 季凑出错误的 Q4")
	})

	t.Run("推导得负值不落库并记 Degraded", func(t *testing.T) {
		store := newFakeStore()
		seedFundamentals(t, store, "MSFT", map[string]string{"2026-06-30": "2026Q4"})
		seedQuarters(t, store)
		// FY 小于三季之和(重述/口径变化)→ ic 得 −3
		seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{
			"789019": fyPeriod(map[string]float64{"IntelligentCloudMember": 30, "ProductivityMember": 90}),
		}}
		rep := RefreshSegments(cfg, store, seg, nil, tpl, "", true)
		require.Empty(t, rep.Failed)

		got := rowsByKey(t, store, "MSFT")
		_, ok := got["2026-06-30|intelligent_cloud"]
		assert.False(t, ok, "负值不得落库")
		assert.Equal(t, 27.0, got["2026-06-30|productivity"].Revenue, "同期其余 segment 不受影响")
		require.Len(t, rep.Degraded, 1)
		assert.Contains(t, rep.Degraded[0], "intelligent_cloud")
	})
}

// removeRow 删掉指定 (period_end, segment_key) 行,用于构造「凑不齐」场景。
func removeRow(rows []prismstore.SegmentRow, periodEnd, key string) []prismstore.SegmentRow {
	var out []prismstore.SegmentRow
	for _, r := range rows {
		if r.PeriodEnd == periodEnd && r.SegmentKey == key {
			continue
		}
		out = append(out, r)
	}
	return out
}

func TestRefreshSegmentsManualOverride(t *testing.T) {
	// functional[4]: manual 最后 upsert 覆盖同 (period_end, segment_key) 的自动行;
	// 并把「下一次 auto 刷新会再次覆盖 manual 行」这一语义定死 —— manual 不是粘性的,
	// 它每轮都从配置重新施加,配置一旦移除,自动值即回归。
	store := newFakeStore()
	seedFundamentals(t, store, "MSFT", map[string]string{"2026-03-31": "2026Q3"})
	manualDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(manualDir, "msft.yaml"),
		[]byte("2026Q3:\n  intelligent_cloud: 99000000000\n"), 0o644))

	seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{
		"789019": {{PeriodStart: q3Start, PeriodEnd: q3End, Form: "10-Q",
			Values: map[string]float64{"IntelligentCloudMember": 26.7e9, "ProductivityMember": 29.9e9}}},
	}}
	cfg := segCfg(segInst("MSFT", "789019"))

	rep := RefreshSegments(cfg, store, seg, nil, msftTemplate(), manualDir, true)
	require.Empty(t, rep.Failed)
	got := rowsByKey(t, store, "MSFT")
	ic := got["2026-03-31|intelligent_cloud"]
	assert.Equal(t, 99e9, ic.Revenue, "manual 覆盖同键自动行")
	assert.Equal(t, "manual", ic.Source)
	assert.Equal(t, "2026Q3", ic.FiscalPeriod, "manual 行的 fiscal_period 即其配置键")
	assert.Equal(t, 29.9e9, got["2026-03-31|productivity"].Revenue, "未被 manual 覆盖的 segment 保持自动值")
	assert.Equal(t, "edgar_segment", got["2026-03-31|productivity"].Source)

	// 下一次 auto 刷新(manual 配置已移除)→ 自动值回归,manual 不粘
	rep = RefreshSegments(cfg, store, seg, nil, msftTemplate(), t.TempDir(), true)
	require.Empty(t, rep.Failed)
	ic = rowsByKey(t, store, "MSFT")["2026-03-31|intelligent_cloud"]
	assert.Equal(t, 26.7e9, ic.Revenue, "下一次 auto 刷新会再次覆盖 manual 行")
	assert.Equal(t, "edgar_segment", ic.Source)
}

func TestRefreshSegmentsManualUnresolvableFiscalPeriod(t *testing.T) {
	// manual 配置按 fiscal_period 组织,而落库主键是 period_end(AD-9)。反查不到
	// 对应季度时无法凭空构造 period_end —— 跳过并记 Degraded,不得静默丢弃。
	store := newFakeStore()
	seedFundamentals(t, store, "MSFT", map[string]string{"2026-03-31": "2026Q3"})
	manualDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(manualDir, "msft.yaml"),
		[]byte("FY2026:\n  intelligent_cloud: 99000000000\n"), 0o644))

	seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{"789019": nil}}
	rep := RefreshSegments(segCfg(segInst("MSFT", "789019")), store, seg, nil, msftTemplate(), manualDir, true)

	require.Empty(t, rep.Failed)
	assert.Empty(t, store.segments["MSFT"], "无法定位 period_end 的 manual 行不得落库")
	require.Len(t, rep.Degraded, 1)
	assert.Contains(t, rep.Degraded[0], "FY2026")
}

func TestRefreshSegmentsSkipsUntemplated(t *testing.T) {
	// boundary[0]: 无模板的标的跳过(零请求);templates 为空 map → 零值 Report 且不报错
	store := newFakeStore()
	seg := &fakeSegClient{}
	cfg := segCfg(segInst("MSFT", "789019"), segInst("NVDA", "1045810"))

	rep := RefreshSegments(cfg, store, seg, nil, msftTemplate(), "", true)
	assert.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed, "只有配了模板的 MSFT 被处理")
	_, called := seg.calls["1045810"]
	assert.False(t, called, "无模板的标的不得发起请求")

	rep = RefreshSegments(cfg, newFakeStore(), &fakeSegClient{}, nil, map[string]*sankey.Template{}, "", true)
	assert.Equal(t, Report{}, rep, "空模板集返回零值 Report")

	rep = RefreshSegments(cfg, newFakeStore(), &fakeSegClient{}, nil, nil, "", true)
	assert.Equal(t, Report{}, rep, "nil 模板集同样返回零值 Report")
}

func TestRefreshSegmentsPartialFailure(t *testing.T) {
	// error_handling[0]: 单标的失败只记 Failed,其余标的继续处理并落库
	store := newFakeStore()
	tpl := msftTemplate()
	tpl["NVDA"] = &sankey.Template{Company: "NVDA", CIK: "1045810", SegmentAxis: sankey.DefaultSegmentAxis,
		Segments: []sankey.Segment{{Key: "datacenter", Member: "DataCenterMember"}}}
	seg := &fakeSegClient{
		fail: map[string]error{"789019": errors.New("sec 503")},
		periods: map[string][]edgar.SegmentPeriod{
			"1045810": {{PeriodStart: q3Start, PeriodEnd: q3End, Form: "10-Q",
				Values: map[string]float64{"DataCenterMember": 39e9}}},
		},
	}

	rep := RefreshSegments(segCfg(segInst("MSFT", "789019"), segInst("NVDA", "1045810")),
		store, seg, nil, tpl, "", true)

	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "MSFT")
	assert.Contains(t, rep.Failed[0], "sec 503", "失败原因不得静默吞掉")
	assert.Equal(t, 1, rep.Refreshed, "其余标的仍计成功")
	assert.Equal(t, 39e9, rowsByKey(t, store, "NVDA")["2026-03-31|datacenter"].Revenue,
		"其余标的仍落库")
}

func TestRefreshSegmentsCIKMismatch(t *testing.T) {
	// AD-16(3):模板 CIK 与 config 不一致意味着模板串号,会把 B 公司的分部数据写进
	// A 公司名下 —— 是「合法值但语义错误」,必须报错而非静默拉取。
	store := newFakeStore()
	seg := &fakeSegClient{}
	tpl := msftTemplate()
	tpl["MSFT"].CIK = "999999" // 与 config 的 789019 不一致

	rep := RefreshSegments(segCfg(segInst("MSFT", "789019")), store, seg, nil, tpl, "", true)

	assert.Equal(t, 0, rep.Refreshed)
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "MSFT")
	assert.Contains(t, rep.Failed[0], "999999")
	assert.Empty(t, seg.calls, "串号模板不得发起任何请求")
}

func TestRefreshSegmentsStoreFailures(t *testing.T) {
	// error_handling[0] 的存储侧展开:任何一步存储失败都必须以「该标的进 Failed」
	// 收场,而不是把半截状态当成功计数(Refreshed 语义会因此虚高)。
	period := []edgar.SegmentPeriod{{PeriodStart: q3Start, PeriodEnd: q3End, Form: "10-Q",
		Values: map[string]float64{"ProductivityMember": 29.9e9}}}
	cfg := segCfg(segInst("MSFT", "789019"))

	tests := []struct {
		name    string
		force   bool
		setup   func(*testing.T, *fakeStore)
		wantSub string
	}{
		{"锚点读取失败", false, func(t *testing.T, s *fakeStore) {
			s.anchorErr["MSFT"] = errors.New("db locked")
		}, "db locked"},
		{"锚点日期损坏", false, func(t *testing.T, s *fakeStore) {
			s.segments["MSFT"] = []prismstore.SegmentRow{{PeriodEnd: "not-a-date", SegmentKey: "productivity"}}
		}, "bad segment anchor"},
		{"fundamental_q 读取失败", true, func(t *testing.T, s *fakeStore) {
			s.fundErr["MSFT"] = errors.New("read back failed")
		}, "read back failed"},
		{"分部行写入失败", true, func(t *testing.T, s *fakeStore) {
			s.segmentErr["MSFT"] = errors.New("disk full")
		}, "disk full"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			seedFundamentals(t, store, "MSFT", map[string]string{"2026-03-31": "2026Q3"})
			tc.setup(t, store)
			seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{"789019": period}}

			rep := RefreshSegments(cfg, store, seg, nil, msftTemplate(), "", tc.force)
			assert.Equal(t, 0, rep.Refreshed, "失败的标的不得计入 Refreshed")
			require.Len(t, rep.Failed, 1)
			assert.Contains(t, rep.Failed[0], "MSFT")
			assert.Contains(t, rep.Failed[0], tc.wantSub, "失败原因不得静默吞掉")
		})
	}
}

func TestRefreshSegmentsManualFileMalformed(t *testing.T) {
	// manual 文件写坏时必须报错到 Failed —— 静默忽略等于人工兜底数据无声失效。
	store := newFakeStore()
	seedFundamentals(t, store, "MSFT", map[string]string{"2026-03-31": "2026Q3"})
	manualDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(manualDir, "msft.yaml"),
		[]byte("2026Q3:\n  intelligent_cloud: 1\nNOT_A_PERIOD:\n  productivity: 2\n"), 0o644))

	seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{"789019": nil}}
	rep := RefreshSegments(segCfg(segInst("MSFT", "789019")), store, seg, nil, msftTemplate(), manualDir, true)

	assert.Equal(t, 0, rep.Refreshed)
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "MSFT")
}

func TestRefreshSegmentsAnnualOnlyReportsUnmappedMember(t *testing.T) {
	// 只报 10-K 的窗口里,未映射 member 同样要可观测 —— 否则「本期只拉到年报」的
	// 标的会让模板缺口完全隐身(季度路径根本没机会报)。
	store := newFakeStore()
	seedFundamentals(t, store, "MSFT", map[string]string{"2026-06-30": "2026Q4"})
	require.NoError(t, store.UpsertSegments(1, []prismstore.SegmentRow{
		{PeriodEnd: q1End.Format("2006-01-02"), SegmentKey: "productivity", Revenue: 20, Source: "edgar_segment"},
		{PeriodEnd: q2End.Format("2006-01-02"), SegmentKey: "productivity", Revenue: 21, Source: "edgar_segment"},
		{PeriodEnd: q3End.Format("2006-01-02"), SegmentKey: "productivity", Revenue: 22, Source: "edgar_segment"},
	}))
	seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{
		"789019": {{PeriodStart: fyStart, PeriodEnd: fyEnd, Form: "10-K",
			Values: map[string]float64{"ProductivityMember": 90, "UnmappedMember": 5}}},
	}}

	rep := RefreshSegments(segCfg(segInst("MSFT", "789019")), store, seg, nil, msftTemplate(), "", true)

	require.Empty(t, rep.Failed)
	require.Len(t, rep.Degraded, 1)
	assert.Contains(t, rep.Degraded[0], "UnmappedMember")
	assert.Contains(t, rep.Degraded[0], "2026-06-30")
	assert.Equal(t, 27.0, rowsByKey(t, store, "MSFT")["2026-06-30|productivity"].Revenue, "已映射 segment 照常推导")
}

func TestRefreshSegmentsIgnoresCorruptFundamentalDate(t *testing.T) {
	// fundamental_q 里的坏日期只影响展示标签反查,不得连累分部数据落库。
	store := newFakeStore()
	seedFundamentals(t, store, "MSFT", map[string]string{"garbage": "2026Q3"})
	seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{
		"789019": {{PeriodStart: q3Start, PeriodEnd: q3End, Form: "10-Q",
			Values: map[string]float64{"ProductivityMember": 29.9e9}}},
	}}

	rep := RefreshSegments(segCfg(segInst("MSFT", "789019")), store, seg, nil, msftTemplate(), "", true)

	require.Empty(t, rep.Failed)
	row := rowsByKey(t, store, "MSFT")["2026-03-31|productivity"]
	assert.Equal(t, 29.9e9, row.Revenue, "坏日期不得阻断落库")
	assert.Equal(t, "", row.FiscalPeriod, "反查不到就落空标签")
	require.Len(t, rep.Degraded, 1)
}

func TestRefreshSegmentsDeterministicOrdering(t *testing.T) {
	// sortedKeys 让 map 遍历产生确定顺序,这是刻意加的 —— 但「刻意」不等于「被守护」:
	// 去掉排序后行为仍旧正确,只是 Degraded 文案顺序与落库行顺序在每次运行间随机漂移,
	// 运维无法比对两次运行的输出,测试也没法精确断言。
	//
	// **单次调用只是概率性检出,必须重复调用**。原以为「n 个 member → 偶然升序概率
	// 1/n!」,实测不成立:Go 对单桶小 map(≤8 键)的迭代是**旋转**而非均匀随机排列 ——
	// 实测 n=5 时只出现 5 种不同顺序(全排列应为 120 种),升序占比随键的哈希落位在
	// 10%~75% 之间浮动,与 1/n! 毫无关系。因此靠加 member 个数压低误判率是无效的
	// (3→5 实测只把存活率从 13% 降到 10%)。
	//
	// 正确做法是**在用例内重复调用**:每轮都必须升序,存活率 = p^N。取 N=20,
	// 即便最坏 p≈0.75 也只有 3e-3,实测去排序变异 30 次全红。
	const rounds = 20
	want := []string{"AlphaMember", "MikeMember", "RomeoMember", "XrayMember", "ZuluMember"}
	for round := range rounds {
		store := newFakeStore()
		seedFundamentals(t, store, "MSFT", map[string]string{"2026-03-31": "2026Q3"})
		seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{
			"789019": {{PeriodStart: q3Start, PeriodEnd: q3End, Form: "10-Q", Values: map[string]float64{
				"ZuluMember": 1, "AlphaMember": 2, "MikeMember": 3,
				"RomeoMember": 4, "XrayMember": 5, "ProductivityMember": 29.9e9,
			}}},
		}}

		rep := RefreshSegments(segCfg(segInst("MSFT", "789019")), store, seg, nil, msftTemplate(), "", true)

		require.Empty(t, rep.Failed)
		require.Len(t, rep.Degraded, len(want))
		for i, w := range want {
			require.Contains(t, rep.Degraded[i], w, "第 %d 轮的第 %d 条 Degraded 须按 member 升序", round, i)
		}
	}
}

// ---------------------------------------------------------------------------
// [A 股财报桥 2026-08-01]:
//   functional[0] akshare 标的:利润表落 fundamental_q(filing=披露日,Equity 显式 NaN)
//                 + 构成经 xbrl_member/alias 映射落 segment_revenue → TestRefreshSegmentsAkshareSource
//   functional[1] 未映射构成名**按名聚合**一条 Degraded(不按期爆炸,对照 edgar 路径
//                 每期一条的噪音教训)                              → TestRefreshSegmentsAkshareSource
//   error[0]      edgar 标的模板 CIK 为空 → Failed(编排守卫,配合模板层 CIK 放宽) →
//                 TestRefreshSegmentsEdgarTemplateNeedsCIK
// ---------------------------------------------------------------------------

type fakeAkFin struct {
	profits []akshare.ProfitQuarter
	points  []akshare.SegmentPoint
}

func (f *fakeAkFin) FetchProfitQuarters(symbol string) ([]akshare.ProfitQuarter, error) {
	return f.profits, nil
}
func (f *fakeAkFin) FetchSegments(symbol string) ([]akshare.SegmentPoint, error) {
	return f.points, nil
}

func maotaiTemplate() map[string]*sankey.Template {
	return map[string]*sankey.Template{
		"600519.SH": {
			Company: "600519.SH", Version: 1,
			Segments: []sankey.Segment{
				{Key: "maotai", NameZH: "茅台酒", NameEN: "Maotai", Member: "茅台酒"},
				{Key: "series", NameZH: "系列酒", NameEN: "Series", Member: "系列酒", Aliases: []string{"其他系列酒"}},
			},
		},
	}
}

func TestRefreshSegmentsAkshareSource(t *testing.T) {
	store := newFakeStore()
	end := date(2026, 3, 31)
	ak := &fakeAkFin{
		profits: []akshare.ProfitQuarter{{
			FiscalPeriod: "2026Q1", PeriodEnd: end, NoticeDate: date(2026, 4, 25),
			Revenue: 100, GrossProfit: 70, OperatingIncome: 50, IncomeTax: 10,
			NetIncome: 40, EPSDiluted: 1.0, RnD: math.NaN(), SGnA: 10,
		}},
		points: []akshare.SegmentPoint{ // 累计点,差分由 DiffSegments 在映射后进行
			{PeriodEnd: end, Name: "茅台酒", Cum: 80},
			{PeriodEnd: end, Name: "其他系列酒", Cum: 20},
			{PeriodEnd: end, Name: "神秘业务", Cum: 1},
			{PeriodEnd: date(2025, 12, 31), Name: "神秘业务", Cum: 1},
		},
	}
	inst := config.PrismInstrument{Symbol: "600519.SH", Name: "贵州茅台", Type: "stock",
		Market: "CN_A", Group: "A股公司", Source: "akshare"}

	rep := RefreshSegments(segCfg(inst), store, &fakeSegClient{}, ak, maotaiTemplate(), "", false)
	require.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed)

	funds := store.fundamentals["600519.SH"]
	require.Len(t, funds, 1)
	assert.Equal(t, "2026-04-25", funds[0].FilingDate, "filing = 披露日(防前视)")
	assert.Equal(t, "akshare", funds[0].Source)
	assert.InDelta(t, 70, funds[0].GrossProfit, 1e-9)
	assert.True(t, math.IsNaN(funds[0].Equity), "利润表无净资产,必须显式 NaN 而非 0")

	byKey := rowsByKey(t, store, "600519.SH")
	assert.InDelta(t, 80, byKey["2026-03-31|maotai"].Revenue, 1e-9)
	assert.InDelta(t, 20, byKey["2026-03-31|series"].Revenue, 1e-9, "别名「其他系列酒」命中 series")
	require.Len(t, rep.Degraded, 1, "未映射名两期同名只聚合一条")
	assert.Contains(t, rep.Degraded[0], "神秘业务")
}

func TestRefreshSegmentsEdgarTemplateNeedsCIK(t *testing.T) {
	store := newFakeStore()
	tpl := map[string]*sankey.Template{"MSFT": {Company: "MSFT", Version: 1,
		Segments: []sankey.Segment{{Key: "cloud", NameZH: "云", NameEN: "Cloud", Member: "CloudMember"}}}}
	rep := RefreshSegments(segCfg(segInst("MSFT", "")), store, &fakeSegClient{}, nil, tpl, "", false)
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "cik")
}
