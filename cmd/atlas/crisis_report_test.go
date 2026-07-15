package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/crisis"
)

// Context Checkpoint: done_criteria → test mapping (TASK-006 report 子命令)
// functional[0]     stdout 逐条报告+前缀+总结+HTML 落盘打印路径 → TestExecuteCrisisReportStdout
// functional[1]     monthly 每月首交易日 3 条 + 月份前缀       → TestExecuteCrisisReportMonthly
// functional[2]     --send 通过量控:31 报告+1 总结+HTML doc   → TestExecuteCrisisReportSendQuota(后半)
// boundary[0]       量控 32 拒绝字面值/sent 空、31 通过         → TestExecuteCrisisReportSendQuota(前半)
// boundary[1]       无 SendDocument → 降级尾附文件路径          → TestExecuteCrisisReportNoDocSenderDegrades
// error_handling[0] 参数校验逐项 + 早于最早观测日报可用起点    → TestExecuteCrisisReportValidation
// error_handling[1] send=nil 报 notifiers.telegram / 单条失败继续 → TestExecuteCrisisReportSendNeedsSender / SendFailureContinues
// nf(test)          snapshotCrisisFlags 保存恢复 / send=true 仍打印 stdout → SendQuota+Stdout 断言

// docStubSender 在 stubSender 上扩展 SendDocument 记录。
type docStubSender struct {
	stubSender
	docs [][2]string // {path, caption}
}

func (s *docStubSender) SendDocument(path, caption string) error {
	s.docs = append(s.docs, [2]string{path, caption})
	return nil
}

func reportDeps(t *testing.T, st *crisis.Store, sender crisis.Sender) (crisisReportDeps, *strings.Builder, *strings.Builder) {
	t.Helper()
	var out, errOut strings.Builder
	return crisisReportDeps{
		cfg: crisisTestConfig(), store: st,
		out: &out, errOut: &errOut,
		sender: sender, sleep: func(time.Duration) {}, htmlDir: t.TempDir(),
	}, &out, &errOut
}

// error_handling: 参数校验（必填/格式/顺序/枚举/早于库内最早日）。
func TestExecuteCrisisReportValidation(t *testing.T) {
	st := newCrisisTestStore(t)
	seedObservations(t, st, "2026-07-10", 10)
	d, _, _ := reportDeps(t, st, nil)
	ctx := context.Background()

	for _, tc := range []struct{ from, to, form, want string }{
		{"", "2026-07-10", "daily", "--from and --to are required"},
		{"2026-7-1", "2026-07-10", "daily", "bad date"},
		{"2026-07-10", "2026-07-01", "daily", "is after"},
		{"2026-07-01", "2026-07-10", "weekly", "--form"},
	} {
		err := executeCrisisReport(ctx, d, tc.from, tc.to, tc.form, false)
		assert.ErrorContains(t, err, tc.want, tc.want)
	}

	err := executeCrisisReport(ctx, d, "2020-01-01", "2026-07-10", "daily", false)
	assert.ErrorContains(t, err, "实际可用起点")
	assert.ErrorContains(t, err, "2026-07-01") // seedObservations 10 日的首日
}

// functional: stdout 模式——逐条报告 + 总结 + HTML 落盘并打印路径；无量控。
func TestExecuteCrisisReportStdout(t *testing.T) {
	st := newCrisisTestStore(t)
	seedReplayWatch(t, st) // 末日 2026-07-10，末 3 日 NFCI 红 → NORMAL→WATCH
	d, out, _ := reportDeps(t, st, nil)

	err := executeCrisisReport(context.Background(), d, "2026-07-06", "2026-07-10", "daily", false)
	require.NoError(t, err)

	s := out.String()
	assert.Contains(t, s, "【历史回放 2026-07-10 · 非实时告警】")
	assert.Contains(t, s, "日报 第")
	assert.Contains(t, s, "【回放总结 2026-07-06 ~ 2026-07-10】")

	htmlPath := filepath.Join(d.htmlDir, "crisis-replay-2026-07-06-2026-07-10.html")
	assert.Contains(t, s, htmlPath)
	data, err := os.ReadFile(htmlPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "<!DOCTYPE html>")
}

// 量控: 31 条恰通过、32 条拒绝（消息字面值），校验发生在启动前。
func TestExecuteCrisisReportSendQuota(t *testing.T) {
	st := newCrisisTestStore(t)
	seedObservations(t, st, "2026-07-10", 120)
	sender := &docStubSender{}
	d, _, _ := reportDeps(t, st, sender)
	ctx := context.Background()

	err := executeCrisisReport(ctx, d, mustAddDays("2026-07-10", -31), "2026-07-10", "daily", true)
	require.Error(t, err)
	assert.Equal(t,
		"daily 回放共 32 条报告，超过 --send 上限 31。请缩短周期，或用 --form monthly，或去掉 --send 输出到 stdout。",
		err.Error())
	assert.Empty(t, sender.sent, "量控须在任何发送前拦截")

	err = executeCrisisReport(ctx, d, mustAddDays("2026-07-10", -30), "2026-07-10", "daily", true)
	require.NoError(t, err)
	assert.Len(t, sender.sent, 31+1, "31 条报告 + 1 条总结")
	require.Len(t, sender.docs, 1)
	assert.Equal(t, "【回放总结", sender.docs[0][1][:len("【回放总结")], "caption = 总结首行前缀")
	assert.Contains(t, sender.docs[0][0], "crisis-replay-")
}

// functional: monthly 报告日 = 全库日历的每月首交易日；前缀用月份。
func TestExecuteCrisisReportMonthly(t *testing.T) {
	st := newCrisisTestStore(t)
	seedObservations(t, st, "2026-07-10", 120) // 覆盖 2026-03..07 共 5 个月首日
	sender := &docStubSender{}
	d, _, _ := reportDeps(t, st, sender)

	err := executeCrisisReport(context.Background(), d, "2026-05-01", "2026-07-10", "monthly", true)
	require.NoError(t, err)

	var reports []string
	for _, m := range sender.sent {
		if strings.HasPrefix(m, "【历史回放") {
			reports = append(reports, m)
		}
	}
	require.Len(t, reports, 3) // 2026-05 / 2026-06 / 2026-07
	assert.Contains(t, reports[0], "【历史回放 2026-05 · 非实时告警】")
	assert.Contains(t, reports[0], "Cassandra 月报")
}

// error_handling: --send 单条失败记 stderr 继续（沿评估链错误语义）。
func TestExecuteCrisisReportSendFailureContinues(t *testing.T) {
	st := newCrisisTestStore(t)
	seedObservations(t, st, "2026-07-10", 40)
	sender := &docStubSender{}
	sender.err = assert.AnError
	d, _, errOut := reportDeps(t, st, sender)

	err := executeCrisisReport(context.Background(), d, "2026-07-08", "2026-07-10", "daily", true)
	require.NoError(t, err, "发送失败不失败退出")
	assert.Len(t, sender.sent, 3+1, "全部尝试发送")
	assert.Contains(t, errOut.String(), "warning: notify failed")
}

// boundary: sender 不支持 SendDocument → 降级为总结尾附文件路径。
func TestExecuteCrisisReportNoDocSenderDegrades(t *testing.T) {
	st := newCrisisTestStore(t)
	seedObservations(t, st, "2026-07-10", 40)
	sender := &stubSender{} // 仅 SendText
	d, _, _ := reportDeps(t, st, sender)

	err := executeCrisisReport(context.Background(), d, "2026-07-09", "2026-07-10", "daily", true)
	require.NoError(t, err)
	last := sender.sent[len(sender.sent)-1]
	assert.Contains(t, last, "【回放总结")
	assert.Contains(t, last, "详细报告（本机）：")
	assert.Contains(t, last, "crisis-replay-2026-07-09-2026-07-10.html")
}

// error_handling: --send 且未配置 telegram → 明确报错。
func TestExecuteCrisisReportSendNeedsSender(t *testing.T) {
	st := newCrisisTestStore(t)
	seedObservations(t, st, "2026-07-10", 10)
	d, _, _ := reportDeps(t, st, nil)

	err := executeCrisisReport(context.Background(), d, "2026-07-09", "2026-07-10", "daily", true)
	assert.ErrorContains(t, err, "notifiers.telegram")
}

// firstLine 两分支：有换行取首行；无换行返回原串（新增 helper 覆盖补强）。
func TestFirstLine(t *testing.T) {
	assert.Equal(t, "【回放总结 a ~ b】", firstLine("【回放总结 a ~ b】\n第二行\n第三行"))
	assert.Equal(t, "无换行", firstLine("无换行"))
}

// runCrisisReport RunE wiring：缺参 / 配置错误 / 真库委派（沿 TestRunCrisisReplay 模式）。
func TestRunCrisisReport(t *testing.T) {
	t.Run("missing flags", func(t *testing.T) {
		snapshotCrisisFlags(t)
		reportFrom, reportTo, reportForm = "", "", "daily"
		require.Error(t, runCrisisReport(newDiscardCmd(), nil))
	})

	t.Run("config error", func(t *testing.T) {
		snapshotCrisisFlags(t)
		crisisCfgPath = filepath.Join(t.TempDir(), "nope.yaml")
		reportFrom, reportTo, reportForm = "2026-06-25", "2026-07-10", "daily"
		require.Error(t, runCrisisReport(newDiscardCmd(), nil))
	})

	t.Run("delegates over seeded db", func(t *testing.T) {
		snapshotCrisisFlags(t)
		// 配置读取用相对路径，先建库 seed，再 chdir 隔离 reports/ 落盘。
		cfgPath, dbPath := writeTempCrisisConfigDB(t)
		st, err := crisis.NewStore(dbPath)
		require.NoError(t, err)
		seedReplayWatch(t, st)
		require.NoError(t, st.Close())
		t.Chdir(t.TempDir())

		crisisCfgPath = cfgPath
		reportFrom, reportTo, reportForm, reportSend = "2026-06-25", "2026-07-10", "daily", false
		var buf strings.Builder
		c := newDiscardCmd()
		c.SetOut(&buf)
		require.NoError(t, runCrisisReport(c, nil))
		assert.Contains(t, buf.String(), "【回放总结")
	})
}
