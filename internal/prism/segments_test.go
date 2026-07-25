package prism

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
//                 容差内命中 ≥2 条报错
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
// manual 文件写坏、只报 10-K 时的未映射 member、fundamental_q 坏日期。
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

	rep := RefreshSegments(segCfg(segInst("MSFT", "789019")), store, seg, msftTemplate(), "", false)

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
	RefreshSegments(cfg, store, seg, msftTemplate(), "", false)
	assert.Equal(t, "2026-03-31", seg.calls["789019"].since.Format("2006-01-02"))

	// force=true:忽略锚点
	RefreshSegments(cfg, store, seg, msftTemplate(), "", true)
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

	t.Run("±3 天容差内命中", func(t *testing.T) {
		store := newFakeStore()
		// fundamental_q 的 period_end 比分部期晚 2 天(申报口径差),仍应命中
		seedFundamentals(t, store, "MSFT", map[string]string{"2026-04-02": "2026Q3"})
		seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{"789019": period(q3End)}}
		rep := RefreshSegments(cfg, store, seg, tpl, "", false)
		require.Empty(t, rep.Failed)
		row := rowsByKey(t, store, "MSFT")["2026-03-31|productivity"]
		assert.Equal(t, "2026Q3", row.FiscalPeriod)
		assert.Equal(t, "2026-03-31", row.PeriodEnd, "AD-9:主键用分部期自身的 period_end,不用 fundamental_q 的")
		assert.Empty(t, rep.Degraded)
	})

	t.Run("容差外未命中仍落库并记 Degraded", func(t *testing.T) {
		store := newFakeStore()
		// 差 4 天 → 超出 ±3 容差
		seedFundamentals(t, store, "MSFT", map[string]string{"2026-04-04": "2026Q3"})
		seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{"789019": period(q3End)}}
		rep := RefreshSegments(cfg, store, seg, tpl, "", false)

		require.Empty(t, rep.Failed)
		row, ok := rowsByKey(t, store, "MSFT")["2026-03-31|productivity"]
		require.True(t, ok, "AD-9 负向断言:反查失败不得丢数据")
		assert.Equal(t, 29.9e9, row.Revenue)
		assert.Equal(t, "", row.FiscalPeriod, "反查失败落空标签")
		require.Len(t, rep.Degraded, 1)
		assert.Contains(t, rep.Degraded[0], "2026-03-31")
	})

	t.Run("容差内命中≥2 条报错而非取首个", func(t *testing.T) {
		store := newFakeStore()
		seedFundamentals(t, store, "MSFT", map[string]string{
			"2026-03-30": "2026Q3", "2026-04-01": "2026Q4",
		})
		seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{"789019": period(q3End)}}
		rep := RefreshSegments(cfg, store, seg, tpl, "", false)

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
		rep := RefreshSegments(cfg, store, seg, tpl, "", true)
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

	t.Run("某 segment 三季凑不齐则跳过该 segment", func(t *testing.T) {
		store := newFakeStore()
		seedFundamentals(t, store, "MSFT", map[string]string{"2026-06-30": "2026Q4"})
		seedQuarters(t, store)
		// 抹掉 productivity 的 Q2 → 该 segment 只有 2 季(其余两 segment 仍是 3 季)
		store.segments["MSFT"] = removeRow(store.segments["MSFT"], q2End.Format("2006-01-02"), "productivity")

		seg := &fakeSegClient{periods: map[string][]edgar.SegmentPeriod{
			"789019": fyPeriod(map[string]float64{"IntelligentCloudMember": 50, "ProductivityMember": 90}),
		}}
		rep := RefreshSegments(cfg, store, seg, tpl, "", true)
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
		rep := RefreshSegments(cfg, store, seg, tpl, "", true)
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

	rep := RefreshSegments(cfg, store, seg, msftTemplate(), manualDir, true)
	require.Empty(t, rep.Failed)
	got := rowsByKey(t, store, "MSFT")
	ic := got["2026-03-31|intelligent_cloud"]
	assert.Equal(t, 99e9, ic.Revenue, "manual 覆盖同键自动行")
	assert.Equal(t, "manual", ic.Source)
	assert.Equal(t, "2026Q3", ic.FiscalPeriod, "manual 行的 fiscal_period 即其配置键")
	assert.Equal(t, 29.9e9, got["2026-03-31|productivity"].Revenue, "未被 manual 覆盖的 segment 保持自动值")
	assert.Equal(t, "edgar_segment", got["2026-03-31|productivity"].Source)

	// 下一次 auto 刷新(manual 配置已移除)→ 自动值回归,manual 不粘
	rep = RefreshSegments(cfg, store, seg, msftTemplate(), t.TempDir(), true)
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
	rep := RefreshSegments(segCfg(segInst("MSFT", "789019")), store, seg, msftTemplate(), manualDir, true)

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

	rep := RefreshSegments(cfg, store, seg, msftTemplate(), "", true)
	assert.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed, "只有配了模板的 MSFT 被处理")
	_, called := seg.calls["1045810"]
	assert.False(t, called, "无模板的标的不得发起请求")

	rep = RefreshSegments(cfg, newFakeStore(), &fakeSegClient{}, map[string]*sankey.Template{}, "", true)
	assert.Equal(t, Report{}, rep, "空模板集返回零值 Report")

	rep = RefreshSegments(cfg, newFakeStore(), &fakeSegClient{}, nil, "", true)
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
		store, seg, tpl, "", true)

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

	rep := RefreshSegments(segCfg(segInst("MSFT", "789019")), store, seg, tpl, "", true)

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

			rep := RefreshSegments(cfg, store, seg, msftTemplate(), "", tc.force)
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
	rep := RefreshSegments(segCfg(segInst("MSFT", "789019")), store, seg, msftTemplate(), manualDir, true)

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

	rep := RefreshSegments(segCfg(segInst("MSFT", "789019")), store, seg, msftTemplate(), "", true)

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

	rep := RefreshSegments(segCfg(segInst("MSFT", "789019")), store, seg, msftTemplate(), "", true)

	require.Empty(t, rep.Failed)
	row := rowsByKey(t, store, "MSFT")["2026-03-31|productivity"]
	assert.Equal(t, 29.9e9, row.Revenue, "坏日期不得阻断落库")
	assert.Equal(t, "", row.FiscalPeriod, "反查不到就落空标签")
	require.Len(t, rep.Degraded, 1)
}
