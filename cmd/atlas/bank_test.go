package main

// Context Checkpoint: done_criteria → test mapping（TASK-006）
// functional[0]     全成功 0 / 只拉一次 / 推 1 条；部分失败 2 且照推     → TestBankReportAllOK, TestBankReportPartialFailure
// functional[1]     全部失败：error + 1 条摘要，每家「名称：错误」各一行 → TestBankReportAllFailed, TestRunBankReportAllFailed, TestBankReportAllFailedSendError
// functional[2]     --dry-run 经 runBankReport：不构造 sender、打印、exit 2 → TestRunBankReportDryRun
// boundary[0]       150 家 ⇒ 多段、每段 ≤ 4000、与 bank.Split 一致         → TestBankReportLongSplits
// error_handling[0] D1：非 dry-run 拿不到 sender ⇒ 打印 + stderr 原因 + 错误 → TestRunBankReportNoSender
// error_handling[1] 推送失败文案；非法 bank 配置不 Fetch 不推送            → TestBankReportSendError, TestRunBankReportBadConfig
// non_functional[0] 命令注册 / flag / --help；buildBankSender 各形态         → TestBankCommandRegistered, TestBankReportHelp, TestBuildBankSender

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/bank"
	"github.com/newthinker/atlas/internal/collector/policy"
)

type bankFakeSource struct {
	errs  map[string]error
	calls map[string]int
}

// Fetch 对任意代码返回同一期数据（不良率 1.62、拨备 142 ⇒ 每个主体触发两条阈值预警）。
func (f *bankFakeSource) Fetch(symbol string) (bank.Series, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[symbol]++
	if err := f.errs[symbol]; err != nil {
		return bank.Series{}, err
	}
	p, _ := time.Parse("2006-01-02", "2026-06-30")
	return bank.Series{Obs: []bank.Observation{{Period: p, Values: [3]float64{1.62, 142, 10}}}}, nil
}

type bankFakeSender struct {
	msgs []string
	err  error
}

func (f *bankFakeSender) SendText(text string) error {
	f.msgs = append(f.msgs, text)
	return f.err
}

var bankTestThresholds = bank.ThresholdsCfg{
	NPLMax: 1.5, CoverageMin: 150, CET1Min: 8.5,
	Deterioration: bank.DeteriorationCfg{NPLUp: 0.1, CoverageDown: 20, CET1Down: 0.5},
}

func bankTestCfg() *bank.Config {
	return &bank.Config{
		RankBy: "npl",
		Banks: []bank.BankCfg{
			{Market: "CN_A", Symbol: "600036.SH", Name: "招商银行"},
			{Market: "HK", Symbol: "3968.HK", Name: "招商银行H", AShareRef: "600036.SH"},
			{Market: "CN_A", Symbol: "601658.SH", Name: "邮储银行"},
		},
		Thresholds: bankTestThresholds,
	}
}

var bankTestNow, _ = time.Parse("2006-01-02", "2026-10-01")

func bankTestDeps(src bank.Source, snd bank.Sender, out io.Writer) bankReportDeps {
	return bankReportDeps{source: src, sender: snd, now: func() time.Time { return bankTestNow }, out: out}
}

func TestBankReportAllOK(t *testing.T) {
	src, snd := &bankFakeSource{}, &bankFakeSender{}
	code, err := executeBankReport(bankTestCfg(), bankTestDeps(src, snd, io.Discard))
	require.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.Equal(t, 1, src.calls["600036.SH"], "A+H 只拉一次")
	require.Len(t, snd.msgs, 1)
	assert.Contains(t, snd.msgs[0], "🏦 银行关键指标月报 2026-10-01")
	assert.Contains(t, snd.msgs[0], "⚠️ 预警 (4)", "两主体各触发不良率与拨备两条")
	assert.Contains(t, snd.msgs[0], "（招商银行H 同 600036.SH）")
}

func TestBankReportPartialFailure(t *testing.T) {
	src := &bankFakeSource{errs: map[string]error{"601658.SH": errors.New("timeout")}}
	snd := &bankFakeSender{}
	code, err := executeBankReport(bankTestCfg(), bankTestDeps(src, snd, io.Discard))
	require.NoError(t, err)
	assert.Equal(t, 2, code, "部分失败退出码 2，报告照常推送")
	require.Len(t, snd.msgs, 1)
	assert.Contains(t, snd.msgs[0], "· 邮储银行 拉取失败：timeout")
}

func TestBankReportAllFailed(t *testing.T) {
	src := &bankFakeSource{errs: map[string]error{
		"600036.SH": errors.New("connection refused"),
		"601658.SH": errors.New("timeout"),
	}}
	snd := &bankFakeSender{}
	_, err := executeBankReport(bankTestCfg(), bankTestDeps(src, snd, io.Discard))
	require.Error(t, err, "全部失败 ⇒ 退出码 1")
	require.Len(t, snd.msgs, 1, "全部失败也推一条错误摘要")
	lines := strings.Split(snd.msgs[0], "\n")
	assert.Contains(t, snd.msgs[0], "全部 2 家拉取失败")
	assert.Contains(t, lines, "· 招商银行：connection refused", "每家的名称与错误各占一行")
	assert.Contains(t, lines, "· 邮储银行：timeout")
}

func TestBankReportAllFailedSendError(t *testing.T) {
	boom := errors.New("down")
	src := &bankFakeSource{errs: map[string]error{"600036.SH": boom, "601658.SH": boom}}
	_, err := executeBankReport(bankTestCfg(), bankTestDeps(src, &bankFakeSender{err: errors.New("403")}, io.Discard))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "telegram 推送第 1/1 段失败", "摘要推送失败时报推送错误")
}

func TestBankReportSendError(t *testing.T) {
	snd := &bankFakeSender{err: errors.New("403")}
	_, err := executeBankReport(bankTestCfg(), bankTestDeps(&bankFakeSource{}, snd, io.Discard))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "telegram 推送第 1/1 段失败")
	assert.Contains(t, err.Error(), "403")
}

func TestBankReportNilSenderPrints(t *testing.T) {
	var out bytes.Buffer
	code, err := executeBankReport(bankTestCfg(), bankTestDeps(&bankFakeSource{}, nil, &out))
	require.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.Contains(t, out.String(), "🏦 银行关键指标月报 2026-10-01")
}

func TestBankReportLongSplits(t *testing.T) {
	cfg := bankTestCfg()
	cfg.Banks = nil
	for i := range 150 {
		cfg.Banks = append(cfg.Banks, bank.BankCfg{Market: "CN_A", Symbol: fmt.Sprintf("600%03d.SH", i), Name: fmt.Sprintf("某某农村商业银行%03d", i)})
	}
	snd := &bankFakeSender{}
	code, err := executeBankReport(cfg, bankTestDeps(&bankFakeSource{}, snd, io.Discard))
	require.NoError(t, err)
	assert.Equal(t, 0, code)

	want := bank.Split(bankReportText(cfg, bank.Collect(cfg.Banks, &bankFakeSource{}), bankTestNow), bank.MaxMessageRunes)
	require.Greater(t, len(want), 1, "150 家当期银行应超过单条上限")
	assert.Equal(t, want, snd.msgs, "按序推送，段数与内容与 bank.Split 一致")
	for i, m := range snd.msgs {
		assert.LessOrEqual(t, utf8.RuneCountInString(m), bank.MaxMessageRunes, "第 %d 段", i)
	}
}

// —— 经 runBankReport 的端到端用例（临时 bank 配置 + httptest 假 aktools） ——

type bankE2E struct {
	stdout, stderr bytes.Buffer
	factoryCalls   int
	exits          []int
	hits           int
	sender         *bankFakeSender
}

// setupBankE2E 起一个假 aktools（failSymbols 中的代码回 HTTP 500），写临时 bank 配置，
// 并替换 bankSenderFactory / bankExit 与相关全局 flag，t.Cleanup 全部复原。
// senderErr 非 nil 时工厂返回 (nil, senderErr)，模拟拿不到 sender。
func setupBankE2E(t *testing.T, cfgBody string, dryRun bool, senderErr error, failSymbols ...string) *bankE2E {
	t.Helper()
	e := &bankE2E{sender: &bankFakeSender{}}
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		e.hits++
		mu.Unlock()
		if slices.Contains(failSymbols, r.URL.Query().Get("symbol")) {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"REPORT_DATE": "2026-06-30 00:00:00", "NONPERLOAN": 1.62, "BLDKBBL": 142, "HXYJBCZL": 10},
		})
	}))
	t.Cleanup(srv.Close)

	path := filepath.Join(t.TempDir(), "bank.yaml")
	require.NoError(t, os.WriteFile(path, []byte(strings.ReplaceAll(cfgBody, "{{URL}}", srv.URL)), 0o644))

	prevPath, prevDry, prevFactory, prevExit := bankCfgPath, bankDryRun, bankSenderFactory, bankExit
	t.Cleanup(func() {
		bankCfgPath, bankDryRun, bankSenderFactory, bankExit = prevPath, prevDry, prevFactory, prevExit
	})
	bankCfgPath, bankDryRun = path, dryRun
	bankSenderFactory = func() (bank.Sender, error) {
		e.factoryCalls++
		if senderErr != nil {
			return nil, senderErr
		}
		return e.sender, nil
	}
	bankExit = func(code int) { e.exits = append(e.exits, code) }
	return e
}

func (e *bankE2E) run() error {
	cmd := &cobra.Command{}
	cmd.SetOut(&e.stdout)
	cmd.SetErr(&e.stderr)
	return runBankReport(cmd, nil)
}

const bankE2ECfg = `source:
  aktools_url: {{URL}}
banks:
  - {market: CN_A, symbol: 600036.SH, name: 招商银行}
  - {market: HK, symbol: 3968.HK, name: 招商银行H, a_share_ref: 600036.SH}
  - {market: CN_A, symbol: 601658.SH, name: 邮储银行}
thresholds:
  npl_max: 1.5
  coverage_min: 150
  cet1_min: 8.5
  deterioration: {npl_up: 0.10, coverage_down: 20, cet1_down: 0.50}
`

func TestRunBankReportDryRun(t *testing.T) {
	t.Run("全成功", func(t *testing.T) {
		e := setupBankE2E(t, bankE2ECfg, true, nil)
		require.NoError(t, e.run())
		assert.Equal(t, 0, e.factoryCalls, "--dry-run 不构造 sender")
		assert.Empty(t, e.sender.msgs)
		assert.Contains(t, e.stdout.String(), "🏦 银行关键指标月报 ")
		assert.Contains(t, e.stdout.String(), "（招商银行H 同 600036.SH）")
		assert.Empty(t, e.exits, "全成功不调 bankExit")
		assert.Equal(t, 2, e.hits, "A+H 只拉一次")
	})
	t.Run("部分失败", func(t *testing.T) {
		e := setupBankE2E(t, bankE2ECfg, true, nil, "601658.SH")
		require.NoError(t, e.run())
		assert.Equal(t, 0, e.factoryCalls)
		assert.Equal(t, []int{2}, e.exits, "部分失败以 2 调用 bankExit 恰一次")
		assert.Contains(t, e.stdout.String(), "· 邮储银行 拉取失败：")
	})
}

func TestRunBankReportSends(t *testing.T) {
	e := setupBankE2E(t, bankE2ECfg, false, nil)
	require.NoError(t, e.run())
	assert.Equal(t, 1, e.factoryCalls)
	require.Len(t, e.sender.msgs, 1)
	assert.Contains(t, e.sender.msgs[0], "⚠️ 预警 (4)")
	assert.Empty(t, e.stdout.String(), "已推送时不再打印")
	assert.Empty(t, e.exits)
}

// D1：拿不到 sender 时照常打印、stderr 说明原因、返回错误（退出码 1），不走 bankExit(0/2)。
func TestRunBankReportNoSender(t *testing.T) {
	e := setupBankE2E(t, bankE2ECfg, false, errors.New("主配置未启用 notifiers.telegram"), "601658.SH")
	err := e.run()
	require.Error(t, err, "未推送的月报不能以成功退出")
	assert.Contains(t, err.Error(), "主配置未启用 notifiers.telegram")
	assert.Contains(t, e.stdout.String(), "🏦 银行关键指标月报 ")
	assert.Contains(t, e.stdout.String(), "⚠️ 预警 (2)", "stdout 是完整报告")
	assert.Contains(t, e.stdout.String(), "· 邮储银行 拉取失败：")
	assert.Contains(t, e.stderr.String(), "主配置未启用 notifiers.telegram", "stderr 说明原因")
	assert.Empty(t, e.exits, "退出码由返回的错误决定为 1，不调 bankExit(2)")
}

func TestRunBankReportAllFailed(t *testing.T) {
	e := setupBankE2E(t, bankE2ECfg, false, nil, "600036.SH", "601658.SH")
	err := e.run()
	require.Error(t, err, "全部失败 ⇒ 退出码 1")
	assert.Contains(t, err.Error(), "全部 2 家银行拉取失败")
	require.Len(t, e.sender.msgs, 1, "仍推送一条摘要")
	assert.Contains(t, e.sender.msgs[0], "· 招商银行：")
	assert.Contains(t, e.sender.msgs[0], "· 邮储银行：")
	assert.Empty(t, e.exits)
}

func TestRunBankReportBadConfig(t *testing.T) {
	e := setupBankE2E(t, "source:\n  aktools_url: {{URL}}\nbanks: []\n", false, nil)
	err := e.run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "banks 不能为空")
	assert.Equal(t, 0, e.hits, "非法配置不 Fetch")
	assert.Equal(t, 0, e.factoryCalls, "非法配置不构造 sender")
	assert.Empty(t, e.sender.msgs, "非法配置不推送")
	assert.Empty(t, e.exits)
}

func TestBankCommandRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"bank", "report"})
	require.NoError(t, err)
	assert.Equal(t, "report", cmd.Name())
	assert.NotNil(t, cmd.Flags().Lookup("dry-run"))
	f := cmd.InheritedFlags().Lookup("bank-config")
	require.NotNil(t, f, "继承自 bank 的持久 flag")
	assert.Equal(t, "configs/bank-monitor.yaml", f.DefValue)
}

func TestBankReportHelp(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetArgs([]string{"bank", "report", "--help"})
	rootCmd.SetOut(&buf)
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(io.Discard)
		if h := bankReportCmd.Flags().Lookup("help"); h != nil {
			_ = h.Value.Set("false")
			h.Changed = false
		}
	})
	require.NoError(t, rootCmd.Execute())
	assert.Contains(t, buf.String(), "--dry-run")
	assert.Contains(t, buf.String(), "--bank-config")
}

// buildBankSender 复用主配置 notifiers.telegram；拿不到时返回说明原因的错误（D1 需要把原因写到 stderr）。
func TestBuildBankSender(t *testing.T) {
	prevFile, prevGate := cfgFile, policy.Default()
	t.Cleanup(func() { cfgFile = prevFile; policy.SetDefault(prevGate) })

	write := func(t *testing.T, body string) {
		t.Helper()
		dir := t.TempDir()
		p := filepath.Join(dir, "config.yaml")
		quota := "collector:\n  quota:\n    path: " + filepath.Join(dir, "quota.json") + "\n"
		require.NoError(t, os.WriteFile(p, []byte(quota+body), 0o600))
		cfgFile = p
	}

	t.Run("启用且凭据齐全", func(t *testing.T) {
		write(t, "notifiers:\n  telegram:\n    enabled: true\n    bot_token: fake-token\n    chat_id: \"12345\"\n")
		s, err := buildBankSender()
		require.NoError(t, err)
		assert.NotNil(t, s)
	})
	t.Run("未启用", func(t *testing.T) {
		write(t, "notifiers:\n  telegram:\n    enabled: false\n    bot_token: fake-token\n    chat_id: \"12345\"\n")
		s, err := buildBankSender()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "未启用")
		assert.True(t, s == nil, "必须是字面量 nil，nil 指针装进接口会穿过 == nil 判断")
	})
	t.Run("缺凭据", func(t *testing.T) {
		write(t, "notifiers:\n  telegram:\n    enabled: true\n    bot_token: fake-token\n")
		s, err := buildBankSender()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "chat_id")
		assert.True(t, s == nil)
	})
	t.Run("主配置读不到", func(t *testing.T) {
		bad := filepath.Join(t.TempDir(), "broken.yaml")
		require.NoError(t, os.WriteFile(bad, []byte("notifiers:\n  telegram:\n    enabled: \"unterminated\n"), 0o644))
		cfgFile = bad
		s, err := buildBankSender()
		require.Error(t, err)
		assert.Contains(t, err.Error(), bad, "错误要带出是哪份主配置（YAML 解析错误本身不含路径）")
		assert.True(t, s == nil)
	})
}
