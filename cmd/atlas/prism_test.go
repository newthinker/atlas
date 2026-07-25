package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/prism"
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
	assert.Len(t, sender.sent, 1)
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
