package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/newthinker/atlas/internal/prism"
)

// Context Checkpoint: done_criteria → test mapping
//   functional[0] 全部成功输出摘要、不发告警            → TestPrismRefreshAllSuccessNoAlert
//   functional[1] 部分失败发告警(含标的摘要)+ exit 0   → TestPrismRefreshNotifiesOnPartialFailure
//   boundary[0]   sender 未配置(nil)→ 退化为打印        → TestPrismRefreshPrintsWithoutSender
//   error[0]      SendText 失败 → errOut warning 不失败    → TestPrismRefreshWarnsOnSendFailure
//   (boundary[1] enabled=false / error[config/Open] 为 runPrismRefresh shell 守卫,见 review)

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
	assert.Contains(t, out.String(), "prism refresh: 5 ok, 0 failed")
	assert.Empty(t, sender.sent, "全部成功不应发告警")
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
