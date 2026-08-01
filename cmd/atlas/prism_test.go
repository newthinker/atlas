package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/collector/edgar"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/prism"
	prismstore "github.com/newthinker/atlas/internal/storage/prism"
)

// Context Checkpoint: done_criteria → test mapping
//   functional[0] 全部成功输出摘要、不发告警            → TestPrismRefreshAllSuccessNoAlert
//   functional[1] 部分失败发告警(含标的摘要)+ exit 0   → TestPrismRefreshNotifiesOnPartialFailure
//   boundary[0]   sender 未配置(nil)→ 退化为打印        → TestPrismRefreshPrintsWithoutSender
//   error[0]      SendText 失败 → errOut warning 不失败    → TestPrismRefreshWarnsOnSendFailure
//   (boundary[1] enabled=false / error[config/Open] 为 runPrismRefresh shell 守卫,见 review)
// [TASK-006] functional[0] 完整计数段 "N ok, M failed, K degraded"        → TestPrismRefreshCountLineFull
// [TASK-006] functional[1] 仅 Degraded 非空→发告警含兜底+symbol, exit 0    → TestPrismRefreshNotifiesOnDegraded
// [TASK-006] boundary      sender=nil 且有 Degraded→退化打印含兜底段       → TestPrismRefreshPrintsDegradedWithoutSender
// [TASK-006] error_handling Degraded 路径 SendText 失败仍仅 warn 不失败     → TestPrismRefreshWarnsOnSendFailureDegraded
// [TASK-004] functional[0] sender 非 nil 时 out 仍含 Failed+Degraded 明细    → TestPrismRefreshAlwaysPrintsDetail
// [TASK-004] functional[1] sender=nil 行为不变(明细进 out 且返回 nil)       → TestPrismRefreshDetailWithoutSenderUnchanged
// [TASK-004] boundary      Failed+Degraded 皆空 → 只有汇总行,无明细段(负向)  → TestPrismRefreshNoDetailSectionWhenClean
// [TASK-004] error_handling SendText 失败仍 warn 到 errOut、返回 nil,明细已进 out → TestPrismRefreshPrintsDetailEvenWhenSendFails

type fakeSender struct {
	sent []string
	err  error
}

func (f *fakeSender) SendText(msg string) error {
	f.sent = append(f.sent, msg)
	return f.err
}

func TestPrismRefreshAllSuccessNoAlert(t *testing.T) {
	sender := &fakeSender{}
	var out, errOut bytes.Buffer
	deps := prismRefreshDeps{
		refresh: func() prism.Report { return prism.Report{Refreshed: 5} },
		sender:  sender, out: &out, errOut: &errOut,
	}
	assert.NoError(t, runPrismRefreshWith(deps))
	assert.Contains(t, out.String(), "prism refresh: 5 ok, 0 failed, 0 degraded", "完整计数段含 degraded")
	assert.Empty(t, sender.sent, "全部成功(Failed+Degraded 均空)不应发告警")
}

func TestPrismRefreshNotifiesOnPartialFailure(t *testing.T) {
	sender := &fakeSender{}
	var out, errOut bytes.Buffer
	deps := prismRefreshDeps{
		refresh: func() prism.Report {
			return prism.Report{Refreshed: 3, Failed: []string{"000300.SH: boom"}}
		},
		sender: sender, out: &out, errOut: &errOut,
	}
	err := runPrismRefreshWith(deps)
	assert.NoError(t, err, "部分失败不算命令失败")
	assert.Contains(t, out.String(), "prism refresh: 3 ok, 1 failed")
	// require 而非 assert:assert 失败不中断,下一行 sender.sent[0] 会越界 panic,
	// 而 panic 会打断整个测试二进制、掩盖其后所有用例结果(test-agent-6 做变异测试时
	// 真被这个模式误导过:全包跑看不到新测试失败,单独跑才发现其实已捕获)。
	require.Len(t, sender.sent, 1)
	assert.Contains(t, sender.sent[0], "000300.SH")
}

func TestPrismRefreshPrintsWithoutSender(t *testing.T) {
	var out, errOut bytes.Buffer
	deps := prismRefreshDeps{
		refresh: func() prism.Report { return prism.Report{Failed: []string{"X: down"}} },
		sender:  nil, out: &out, errOut: &errOut,
	}
	assert.NoError(t, runPrismRefreshWith(deps))
	assert.Contains(t, out.String(), "X: down")
}

func TestPrismRefreshWarnsOnSendFailure(t *testing.T) {
	sender := &fakeSender{err: errors.New("telegram down")}
	var out, errOut bytes.Buffer
	deps := prismRefreshDeps{
		refresh: func() prism.Report { return prism.Report{Failed: []string{"Y: err"}} },
		sender:  sender, out: &out, errOut: &errOut,
	}
	assert.NoError(t, runPrismRefreshWith(deps), "通知失败不应使命令失败")
	assert.Contains(t, errOut.String(), "warning")
	assert.Contains(t, errOut.String(), "telegram down")
}

// functional[0]:混合报告下 out 显式含完整计数段 "N ok, M failed, K degraded"。
func TestPrismRefreshCountLineFull(t *testing.T) {
	sender := &fakeSender{}
	var out, errOut bytes.Buffer
	deps := prismRefreshDeps{
		refresh: func() prism.Report {
			return prism.Report{Refreshed: 8, Failed: []string{"AAA: boom"},
				Degraded: []string{"000300.SH: lixinger failed (quota), akshare fallback ok"}}
		},
		sender: sender, out: &out, errOut: &errOut,
	}
	require.NoError(t, runPrismRefreshWith(deps))
	assert.Contains(t, out.String(), "prism refresh: 8 ok, 1 failed, 1 degraded", "完整计数段")
}

// functional[1]:仅 Degraded 非空也发告警(消息含兜底语义与 symbol),返回 nil。
func TestPrismRefreshNotifiesOnDegraded(t *testing.T) {
	sender := &fakeSender{}
	var out, errOut bytes.Buffer
	deps := prismRefreshDeps{
		refresh: func() prism.Report {
			return prism.Report{Refreshed: 8, Degraded: []string{"000300.SH: lixinger failed (quota), akshare fallback ok"}}
		},
		sender: sender, out: &out, errOut: &errOut,
	}
	require.NoError(t, runPrismRefreshWith(deps))
	assert.Contains(t, out.String(), "prism refresh: 8 ok, 0 failed, 1 degraded")
	require.Len(t, sender.sent, 1, "仅 Degraded 非空也应发告警")
	assert.Contains(t, sender.sent[0], "兜底")
	assert.Contains(t, sender.sent[0], "000300.SH")
}

// boundary:sender=nil 且有 Degraded → 退化打印含兜底段(直接断言 out,不依赖既有用例)。
func TestPrismRefreshPrintsDegradedWithoutSender(t *testing.T) {
	var out, errOut bytes.Buffer
	deps := prismRefreshDeps{
		refresh: func() prism.Report {
			return prism.Report{Refreshed: 2, Degraded: []string{"000905.SH: lixinger failed (down), akshare fallback ok"}}
		},
		sender: nil, out: &out, errOut: &errOut,
	}
	assert.NoError(t, runPrismRefreshWith(deps))
	assert.Contains(t, out.String(), "兜底", "sender=nil 应退化打印兜底段")
	assert.Contains(t, out.String(), "000905.SH")
}

// error_handling:仅 Degraded 路径 SendText 失败仍仅 warn 不失败(覆盖新消息路径)。
func TestPrismRefreshWarnsOnSendFailureDegraded(t *testing.T) {
	sender := &fakeSender{err: errors.New("telegram down")}
	var out, errOut bytes.Buffer
	deps := prismRefreshDeps{
		refresh: func() prism.Report {
			return prism.Report{Degraded: []string{"000300.SH: lixinger failed (q), akshare fallback ok"}}
		},
		sender: sender, out: &out, errOut: &errOut,
	}
	assert.NoError(t, runPrismRefreshWith(deps), "仅降级且通知失败也不应使命令失败")
	require.Len(t, sender.sent, 1, "降级路径也应尝试发送")
	assert.Contains(t, errOut.String(), "warning")
	assert.Contains(t, errOut.String(), "telegram down")
}

// [TASK-004] functional[0]:配置了 sender 时明细也必须落到 stdout(生产观测缺口),
// 且同一条消息仍然发给 sender。
func TestPrismRefreshAlwaysPrintsDetail(t *testing.T) {
	sender := &fakeSender{}
	var out, errOut bytes.Buffer
	deps := prismRefreshDeps{
		refresh: func() prism.Report {
			return prism.Report{Refreshed: 4,
				Failed:   []string{"AAPL: edgar 404"},
				Degraded: []string{"000300.SH: lixinger failed (quota), akshare fallback ok"}}
		},
		sender: sender, out: &out, errOut: &errOut,
	}
	require.NoError(t, runPrismRefreshWith(deps))
	assert.Contains(t, out.String(), "prism refresh: 4 ok, 1 failed, 1 degraded")
	assert.Contains(t, out.String(), "AAPL: edgar 404", "有 sender 时 Failed 明细仍须进 out")
	assert.Contains(t, out.String(), "000300.SH: lixinger failed (quota), akshare fallback ok",
		"有 sender 时 Degraded 明细仍须进 out")
	require.Len(t, sender.sent, 1, "打印不取代通知")
	assert.Contains(t, sender.sent[0], "AAPL: edgar 404")
	assert.Contains(t, sender.sent[0], "000300.SH")
}

// [TASK-004] functional[1]:sender=nil 时行为零变更——明细进 out 且返回 nil。
func TestPrismRefreshDetailWithoutSenderUnchanged(t *testing.T) {
	var out, errOut bytes.Buffer
	deps := prismRefreshDeps{
		refresh: func() prism.Report {
			return prism.Report{Refreshed: 1,
				Failed:   []string{"MSFT: timeout"},
				Degraded: []string{"000905.SH: lixinger failed (down), akshare fallback ok"}}
		},
		sender: nil, out: &out, errOut: &errOut,
	}
	assert.NoError(t, runPrismRefreshWith(deps))
	assert.Contains(t, out.String(), "MSFT: timeout")
	assert.Contains(t, out.String(), "000905.SH: lixinger failed (down), akshare fallback ok")
	assert.Empty(t, errOut.String(), "无 sender 不产生 warning")
}

// [TASK-004] boundary(负向断言):Failed 与 Degraded 皆空时只打印汇总行,不打印明细段。
func TestPrismRefreshNoDetailSectionWhenClean(t *testing.T) {
	sender := &fakeSender{}
	var out, errOut bytes.Buffer
	deps := prismRefreshDeps{
		refresh: func() prism.Report { return prism.Report{Refreshed: 7} },
		sender:  sender, out: &out, errOut: &errOut,
	}
	require.NoError(t, runPrismRefreshWith(deps))
	assert.Equal(t, "prism refresh: 7 ok, 0 failed, 0 degraded\n", out.String(),
		"干净报告只输出汇总行,不得有明细段")
	assert.Empty(t, sender.sent)
}

// [TASK-004] error_handling:SendText 失败仍仅 warn 到 errOut 并返回 nil,
// 而明细此时已经打印到 out(通知失败不影响观测)。
func TestPrismRefreshPrintsDetailEvenWhenSendFails(t *testing.T) {
	sender := &fakeSender{err: errors.New("telegram down")}
	var out, errOut bytes.Buffer
	deps := prismRefreshDeps{
		refresh: func() prism.Report {
			return prism.Report{Failed: []string{"Z: boom"}}
		},
		sender: sender, out: &out, errOut: &errOut,
	}
	assert.NoError(t, runPrismRefreshWith(deps), "通知失败不应使命令失败")
	assert.Contains(t, out.String(), "Z: boom", "通知失败时明细仍在 out")
	assert.Contains(t, errOut.String(), "warning")
	assert.Contains(t, errOut.String(), "telegram down")
}

// [TASK-005] boundary[0] EdgarUserAgent 为空的报错门由 hasEdgarInstrument 判定:
// 有 source==edgar 标的 → true(runPrismRefresh 据此在 UA 空时报错);否则 false。
func TestHasEdgarInstrument(t *testing.T) {
	withEdgar := []config.PrismInstrument{
		{Symbol: "600519.SH", Source: "akshare"},
		{Symbol: "NVDA", Source: "edgar", CIK: "1045810"},
	}
	assert.True(t, hasEdgarInstrument(withEdgar), "存在 edgar 标的 → true")

	noEdgar := []config.PrismInstrument{
		{Symbol: "600519.SH", Source: "akshare"},
		{Symbol: "^GSPC", Source: "lixinger"},
	}
	assert.False(t, hasEdgarInstrument(noEdgar), "无 edgar 标的 → false")
	assert.False(t, hasEdgarInstrument(nil), "空清单 → false")
}

// ---------------------------------------------------------------------------
// TASK-010 done_criteria → test mapping
//   functional[0] refresh 闭包合并两个 Report,双方 Failed 与 Degraded 都在结果里
//                                    → TestMergeReportsKeepsBothSides
//   functional[1] AD-12 --full-segments 透传 force=true(TASK-014 模板迭代依赖)
//                                    → TestFullSegmentsFlagRegistered / TestSegmentReportPassesForceAndManualDir
//   functional[2] AD-16 LoadTemplates 的 error 不得丢弃:坏 YAML → 进 Degraded 且含文件名,
//                 且不跑分部刷新(负向断言:不得静默,否则表现为 sankey 全站 404 且日志无痕)
//                                    → TestSegmentReportBadTemplateDegradesNotSilently
//   boundary[0]   模板目录不存在/为空 → 跳过分部刷新且不报错(不依赖真实 configs 目录)
//                                    → TestSegmentReportSkipsWhenNoTemplates
//   boundary[1]   合并后汇总行的 ok 数不得超过标的总数(Refreshed 不相加)
//                                    → TestMergeReportsDoesNotInflateOkCount
//   error[0]      估值 Failed 与分部 Failed 同时存在时两者都可见
//                                    → TestPrismRefreshShowsBothFailureSources
// ---------------------------------------------------------------------------

func TestMergeReportsKeepsBothSides(t *testing.T) {
	val := prism.Report{Refreshed: 3,
		Failed:   []string{"000300.SH: lixinger down"},
		Degraded: []string{"000905.SH: lixinger failed (quota), akshare fallback ok"}}
	seg := prism.Report{Refreshed: 2,
		Failed:   []string{"MSFT: sec 503"},
		Degraded: []string{"MSFT: unmapped segment member FooMember in period 2026-03-31"}}

	got := mergeReports(val, seg)

	require.Len(t, got.Failed, 2)
	assert.Contains(t, got.Failed[0], "000300.SH: lixinger down")
	assert.Contains(t, got.Failed[1], "MSFT: sec 503")
	assert.Contains(t, got.Failed[1], "segments", "分部条目须可与估值条目区分")
	require.Len(t, got.Degraded, 2)
	assert.Contains(t, got.Degraded[0], "000905.SH")
	assert.Contains(t, got.Degraded[1], "FooMember")
	assert.Contains(t, got.Degraded[1], "segments")
}

func TestMergeReportsDoesNotInflateOkCount(t *testing.T) {
	// boundary[1]: 两次刷新跑的是**同一批标的**,Refreshed 相加会让汇总行的 "N ok"
	// 超过标的总数,误导 launchd 日志与告警(design-spec §4.2 要求分开表述)。
	// 汇总行 ok 的口径固定为「估值刷新成功的标的数」。
	val := prism.Report{Refreshed: 3}
	seg := prism.Report{Refreshed: 2}

	got := mergeReports(val, seg)

	assert.Equal(t, 3, got.Refreshed, "Refreshed 不得相加(3+2=5 会超过 3 个标的的总数)")
}

func TestMergeReportsDoesNotMutateInputs(t *testing.T) {
	// 合并用 append 时若共用底层数组,会就地改写调用方的切片 —— 估值 Report 在
	// 合并后仍可能被复用/断言,静默改写是典型的「值合法但来源错误」。
	val := prism.Report{Refreshed: 1, Failed: []string{"A: x"}, Degraded: []string{"B: y"}}
	seg := prism.Report{Failed: []string{"C: z"}, Degraded: []string{"D: w"}}

	_ = mergeReports(val, seg)

	assert.Equal(t, []string{"A: x"}, val.Failed, "合并不得改写入参")
	assert.Equal(t, []string{"B: y"}, val.Degraded)
	assert.Equal(t, []string{"C: z"}, seg.Failed)
}

func TestPrismRefreshShowsBothFailureSources(t *testing.T) {
	// error_handling[0]: 估值失败与分部失败同时存在时,两者在 out 与通知里都可见,
	// 不互相覆盖(合并走的是 append 而非替换)。
	sender := &fakeSender{}
	var out, errOut bytes.Buffer
	deps := prismRefreshDeps{
		refresh: func() prism.Report {
			return mergeReports(
				prism.Report{Refreshed: 4, Failed: []string{"000300.SH: lixinger down"}},
				prism.Report{Refreshed: 1, Failed: []string{"MSFT: sec 503"}})
		},
		sender: sender, out: &out, errOut: &errOut,
	}
	require.NoError(t, runPrismRefreshWith(deps))

	assert.Contains(t, out.String(), "prism refresh: 4 ok, 2 failed, 0 degraded")
	assert.Contains(t, out.String(), "000300.SH: lixinger down", "估值失败须可见")
	assert.Contains(t, out.String(), "MSFT: sec 503", "分部失败须可见")
	require.Len(t, sender.sent, 1)
	assert.Contains(t, sender.sent[0], "000300.SH")
	assert.Contains(t, sender.sent[0], "MSFT")
}

func TestFullSegmentsFlagRegistered(t *testing.T) {
	// functional[1]/AD-12: TASK-014 的模板迭代(跑一次看 member → 改模板 → 再跑)
	// 在纯增量下第二次拉不到数据,必须有这个全量重拉入口。
	f := prismRefreshCmd.Flags().Lookup("full-segments")
	require.NotNil(t, f, "--full-segments flag 必须注册")
	assert.Equal(t, "false", f.DefValue, "默认不全量重拉(日常刷新仍走增量)")
}

// --- segmentReport 的模板加载分支 -------------------------------------------

// fakeSegStore 是 prism.Store 的最小实现:只记录分部落库,其余方法返回零值。
type fakeSegStore struct {
	anchor   string
	segments []prismstore.SegmentRow
}

func (f *fakeSegStore) UpsertInstrument(prismstore.Instrument) (int64, error)       { return 1, nil }
func (f *fakeSegStore) LatestDate(int64) (string, error)                            { return "", nil }
func (f *fakeSegStore) UpsertValuations(int64, []prismstore.ValuationRow) error     { return nil }
func (f *fakeSegStore) UpsertFundamentals(int64, []prismstore.FundamentalRow) error { return nil }
func (f *fakeSegStore) UpsertPrices(int64, []prismstore.PriceRow) error             { return nil }
func (f *fakeSegStore) Series(string, string) (*prismstore.SeriesData, error) {
	return &prismstore.SeriesData{}, nil
}
func (f *fakeSegStore) UpsertSegments(_ int64, rows []prismstore.SegmentRow) error {
	f.segments = append(f.segments, rows...)
	return nil
}
func (f *fakeSegStore) SegmentRows(int64) ([]prismstore.SegmentRow, error) { return f.segments, nil }
func (f *fakeSegStore) LatestSegmentPeriodEnd(int64) (string, error)       { return f.anchor, nil }
func (f *fakeSegStore) QuarterlyFundamentals(int64) ([]prismstore.FundamentalRow, error) {
	return nil, nil
}

type fakeSegFetcher struct {
	called bool
	cik    string
	since  time.Time
}

func (f *fakeSegFetcher) FetchSegmentRevenue(cik, axis string, since time.Time) ([]edgar.SegmentPeriod, error) {
	f.called, f.cik, f.since = true, cik, since
	return nil, nil
}

func segTestCfg() config.PrismConfig {
	c := config.PrismConfig{Instruments: []config.PrismInstrument{
		{Symbol: "ACME", Name: "Acme", Type: "stock", Market: "US", Group: "美股公司",
			Source: "edgar", CIK: "0000123456"},
	}}
	c.ApplyDefaults()
	return c
}

const validTemplateYAML = `company: acme
cik: "0000123456"
segment_axis: StatementBusinessSegmentsAxis
segments:
  - key: cloud
    name_zh: 云业务
    name_en: Cloud
    xbrl_member: CloudMember
`

func TestSegmentReportBadTemplateDegradesNotSilently(t *testing.T) {
	// functional[2]/AD-16: 模板目录被 rsync --delete 清掉或 YAML 写坏时,若丢弃 error
	// 就表现为 sankey 全站 404 且日志无痕。必须进 Degraded,且不跑分部刷新。
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.yaml"),
		[]byte("company: BROKEN\nsegments:\n  - {key: cloud, name_zh: 云\n    xbrl_member: [unclosed\n"), 0o644))
	seg := &fakeSegFetcher{}

	rep := segmentReport(segTestCfg(), &fakeSegStore{}, seg, nil, dir, "", false)

	require.Len(t, rep.Degraded, 1, "模板加载失败必须可观测")
	assert.Contains(t, rep.Degraded[0], "broken.yaml", "降级说明须含出错文件名(AD-16)")
	assert.Empty(t, rep.Failed)
	assert.Zero(t, rep.Refreshed)
	assert.False(t, seg.called, "模板不可用时不得发起分部拉取")
}

func TestSegmentReportSkipsWhenNoTemplates(t *testing.T) {
	// boundary[0]: 目录不存在或无模板 → 跳过分部刷新且不报错。用 t.TempDir 而非真实
	// configs 目录,因 go test ./cmd/atlas/ 的 cwd 是包目录、相对路径解析不到仓库根。
	for _, tc := range []struct {
		name string
		dir  func(*testing.T) string
	}{
		{"目录不存在", func(t *testing.T) string { return filepath.Join(t.TempDir(), "nope") }},
		{"目录为空", func(t *testing.T) string { return t.TempDir() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seg := &fakeSegFetcher{}
			rep := segmentReport(segTestCfg(), &fakeSegStore{}, seg, nil, tc.dir(t), "", false)
			assert.Equal(t, prism.Report{}, rep, "无模板时返回零值 Report")
			assert.False(t, seg.called, "无模板时不得发起分部拉取")
		})
	}
}

func TestSegmentReportPassesForceAndManualDir(t *testing.T) {
	// functional[1]: --full-segments 必须一路透传到 FetchSegmentRevenue 的 since ——
	// force=true 传零值全量重拉,force=false 用锚点增量。
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "acme.yaml"), []byte(validTemplateYAML), 0o644))

	segForce := &fakeSegFetcher{}
	rep := segmentReport(segTestCfg(), &fakeSegStore{anchor: "2026-03-31"}, segForce, nil, dir, "", true)
	require.Empty(t, rep.Failed)
	require.True(t, segForce.called, "有模板时须发起分部拉取")
	assert.Equal(t, "0000123456", segForce.cik, "CIK 取自模板")
	assert.True(t, segForce.since.IsZero(), "force=true 须忽略锚点全量重拉")

	segIncr := &fakeSegFetcher{}
	rep = segmentReport(segTestCfg(), &fakeSegStore{anchor: "2026-03-31"}, segIncr, nil, dir, "", false)
	require.Empty(t, rep.Failed)
	assert.Equal(t, "2026-03-31", segIncr.since.Format("2006-01-02"), "force=false 须用锚点增量")
}

func TestSankeyDirConstantsArePopulated(t *testing.T) {
	// manualDir 传空串会**静默禁用** manual 覆盖:segments.go 的 manualRows 对 "" 直接
	// return,无报错、无 Degraded、无日志 —— 人工兜底数据从此不生效而没有任何一层会喊,
	// 又一个「合法值但语义错误」。同理 templatesDir 传空会让分部刷新整体静默失效。
	//
	// 真正的传参点在 runPrismRefresh(装配壳,无 seam、历来 0% 覆盖),那里的 "" 变异
	// 无法被行为测试捕获;退而锁住这两个常量的取值,使「先留空回头再填」这类改动变红。
	assert.Equal(t, "configs/prism/segments", sankeySegmentsDir,
		"manual 兜底目录不得为空:空值会静默禁用 manual 覆盖")
	assert.Equal(t, "configs/prism/templates", sankeyTemplatesDir,
		"模板目录不得为空:空值会让分部刷新整体静默跳过")
}
