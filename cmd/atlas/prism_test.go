package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
