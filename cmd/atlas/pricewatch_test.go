package main

// Context Checkpoint: done_criteria → test mapping (TASK-009)
// functional[0]     子命令与 flag 注册                                   → TestPricewatchCmd_Registered
// functional[1]     取价 okx→binance、回填 binance→okx；未知名报错；AI 降级  → TestPricewatchProviders_Order、TestPricewatchProviders_Unknown、TestPricewatchAdvisor
// functional[2]     plist（manual）                                        → plutil -lint 与人工核对，见 discovery
// error_handling[0] 配置缺失/非法时 once 报错且不发消息；backfill 资产不存在报错、大小写不敏感 → TestPricewatchOnce_BadConfig、TestPricewatchBackfill_Symbol
// error_handling[1] ensurePolicyGate 先于 provider 构造（review）           → TestPricewatchProviders_GateFirst（辅助证据）
// error_handling[2] run 收到信号取消 ctx 并返回 nil（review）               → TestPricewatchRun_ReturnsNilOnCancel（辅助证据）
// QA-11（非 DoD）   once 把 RunOnce 的错误传成命令错误                       → TestPricewatchOnce_PropagatesCheckError

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/newthinker/atlas/internal/collector/crypto"
	"github.com/newthinker/atlas/internal/collector/policy"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/llm"
	"github.com/newthinker/atlas/internal/pricewatch"
)

// pwFakeProvider：FetchQuote 返回固定价格（price<=0 时报错）；FetchHistory 返回 [start,end) 内的日线/5 分钟线。
type pwFakeProvider struct {
	name  string
	price float64
}

func (p *pwFakeProvider) Name() string { return p.name }

func (p *pwFakeProvider) FetchQuote(sym string) (*core.Quote, error) {
	if p.price <= 0 {
		return nil, errors.New(p.name + ": down")
	}
	return &core.Quote{Symbol: sym, Price: p.price}, nil
}

func (p *pwFakeProvider) FetchHistory(_ string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if p.price <= 0 {
		return nil, errors.New(p.name + ": down")
	}
	step := 24 * time.Hour
	if interval == "5m" {
		step = 5 * time.Minute
	}
	var out []core.OHLCV
	for t := start.Truncate(step).Add(step); t.Before(end) && len(out) < 1000; t = t.Add(step) {
		out = append(out, core.OHLCV{Open: 100, High: 101, Low: 99, Close: 100, Time: t})
	}
	return out, nil
}

type pwSenderSpy struct{ texts []string }

func (s *pwSenderSpy) SendText(text string) error { s.texts = append(s.texts, text); return nil }

// pwFixture 把 cmd 层的全局状态换成测试替身，并在测试结束时恢复。
type pwFixture struct {
	dir      string
	sender   *pwSenderSpy
	newCalls int
}

func newPWFixture(t *testing.T, price float64) *pwFixture {
	t.Helper()
	f := &pwFixture{dir: t.TempDir(), sender: &pwSenderSpy{}}

	prevCfgFile, prevGate, prevPath := cfgFile, policy.Default(), pricewatchCfgPath
	prevCtors, prevSender, prevLLM := pricewatchProviderCtors, pricewatchNewSender, pricewatchNewLLM
	prevDry, prevSym := pricewatchDryRun, pricewatchSymbol
	t.Cleanup(func() {
		cfgFile, pricewatchCfgPath = prevCfgFile, prevPath
		policy.SetDefault(prevGate)
		pricewatchProviderCtors, pricewatchNewSender, pricewatchNewLLM = prevCtors, prevSender, prevLLM
		pricewatchDryRun, pricewatchSymbol = prevDry, prevSym
	})

	// 主配置只为把 collector 配额账本落到临时目录
	mainCfg := filepath.Join(f.dir, "config.yaml")
	require.NoError(t, os.WriteFile(mainCfg,
		[]byte("collector:\n  quota:\n    path: "+filepath.Join(f.dir, "quota.json")+"\n"), 0o600))
	cfgFile = mainCfg

	pricewatchProviderCtors = map[string]func() crypto.Provider{
		"okx":     func() crypto.Provider { return &pwFakeProvider{name: "okx", price: price} },
		"binance": func() crypto.Provider { return &pwFakeProvider{name: "binance", price: price} },
	}
	pricewatchNewSender = func(*pricewatch.Config) pricewatch.Sender { f.newCalls++; return f.sender }
	pricewatchNewLLM = func() (llm.Provider, error) { return nil, errors.New("no llm in tests") }

	t.Setenv("PW_TEST_TOKEN", "test-token")
	t.Setenv("PW_TEST_CHAT", "12345")
	pricewatchCfgPath = f.writeConfig(t, "ETH")
	return f
}

func (f *pwFixture) writeConfig(t *testing.T, symbols ...string) string {
	t.Helper()
	var items strings.Builder
	for _, s := range symbols {
		items.WriteString("      - {symbol: " + s + ", strategy: s}\n")
	}
	body := "check_interval: 5m\n" +
		"storage:\n  path: " + filepath.Join(f.dir, "pw.db") + "\n" +
		"telegram:\n  bot_token: \"${PW_TEST_TOKEN}\"\n  chat_id: \"${PW_TEST_CHAT}\"\n" +
		"ai:\n  enabled: false\n" +
		"notify_defaults: {repeat_interval: 1h, max_per_tier: 3}\n" +
		"health: {fail_alert_after: 30m, heartbeat_at: \"\"}\n" +
		"strategies:\n  s:\n    - {id: day_drop, type: drop_from_high, window: 24h, threshold_pct: 2, step_pct: 2}\n" +
		"assets:\n  crypto:\n    sources: [okx, binance]\n    quote: USDT\n    items:\n" + items.String()
	p := filepath.Join(f.dir, "pricewatch.yaml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

func TestPricewatchCmd_Registered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"pricewatch"})
	require.NoError(t, err)
	require.Equal(t, "pricewatch", cmd.Name())
	var subs []string
	for _, c := range cmd.Commands() {
		subs = append(subs, c.Name())
	}
	assert.ElementsMatch(t, []string{"run", "once", "backfill"}, subs)

	f := pricewatchCmd.PersistentFlags().Lookup("pricewatch-config")
	require.NotNil(t, f, "--pricewatch-config 应为持久 flag")
	assert.Equal(t, "configs/pricewatch.yaml", f.DefValue)
	assert.NotNil(t, pricewatchOnceCmd.Flags().Lookup("dry-run"))
	assert.NotNil(t, pricewatchBackfillCmd.Flags().Lookup("symbol"))
}

func providerNames(ps []crypto.Provider) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Name())
	}
	return out
}

func TestPricewatchProviders_Order(t *testing.T) {
	newPWFixture(t, 100)
	for _, sources := range [][]string{{"okx", "binance"}, {"binance", "okx"}} {
		price, backfill, err := pricewatchProviders(sources)
		require.NoError(t, err)
		assert.Equal(t, sources, providerNames(price), "取价顺序 = 配置 sources")
		assert.Equal(t, []string{"binance", "okx"}, providerNames(backfill), "K 线回填按历史长度：binance 先（AD-20）")
		// 两种顺序共用同一组实例
		for i, p := range price {
			assert.Same(t, p, backfill[indexOf(providerNames(backfill), sources[i])])
		}
	}
	price, backfill, err := pricewatchProviders([]string{"okx"})
	require.NoError(t, err)
	assert.Equal(t, []string{"okx"}, providerNames(price))
	assert.Equal(t, []string{"okx"}, providerNames(backfill))
}

func indexOf(xs []string, x string) int {
	for i, v := range xs {
		if v == x {
			return i
		}
	}
	return -1
}

func TestPricewatchProviders_Unknown(t *testing.T) {
	newPWFixture(t, 100)
	_, _, err := pricewatchProviders([]string{"okx", "kraken"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"kraken"`)
}

// 辅助证据：provider 构造时 policy.Default() 已是 ensurePolicyGate 装好的闸门（DoD e[1] 为 review 项）。
func TestPricewatchProviders_GateFirst(t *testing.T) {
	newPWFixture(t, 100)
	sentinel := policy.Default()
	var seen *policy.Gate
	pricewatchProviderCtors = map[string]func() crypto.Provider{
		"okx": func() crypto.Provider { seen = policy.Default(); return &pwFakeProvider{name: "okx"} },
	}
	_, _, err := pricewatchProviders([]string{"okx"})
	require.NoError(t, err)
	require.NotNil(t, seen)
	assert.NotSame(t, sentinel, seen, "构造 provider 之前应已重新装配闸门")
}

func TestPricewatchAdvisor(t *testing.T) {
	newPWFixture(t, 100)
	cfg := &pricewatch.Config{}
	cfg.AI.Timeout = 7 * time.Second

	t.Run("ai.enabled=false", func(t *testing.T) {
		calls := 0
		pricewatchNewLLM = func() (llm.Provider, error) { calls++; return nil, nil }
		obs, logs := observer.New(zap.WarnLevel)
		assert.Nil(t, pricewatchAdvisor(cfg, zap.New(obs)))
		assert.Zero(t, calls, "AI 关闭时不应构造 LLM")
		assert.Zero(t, logs.Len())
	})
	t.Run("工厂失败 → nil + 一条 warn", func(t *testing.T) {
		cfg.AI.Enabled = true
		pricewatchNewLLM = func() (llm.Provider, error) { return nil, errors.New("bad llm config") }
		obs, logs := observer.New(zap.WarnLevel)
		assert.Nil(t, pricewatchAdvisor(cfg, zap.New(obs)))
		require.Equal(t, 1, logs.Len())
		assert.Contains(t, logs.All()[0].ContextMap()["error"], "bad llm config")
	})
	t.Run("工厂成功", func(t *testing.T) {
		cfg.AI.Enabled = true
		var p llm.Provider = pwFakeLLM{}
		pricewatchNewLLM = func() (llm.Provider, error) { return p, nil }
		a := pricewatchAdvisor(cfg, zap.NewNop())
		require.NotNil(t, a)
		assert.Equal(t, p, a.LLM)
		assert.Equal(t, 7*time.Second, a.Timeout)
	})
}

type pwFakeLLM struct{}

func (pwFakeLLM) Name() string { return "fake" }
func (pwFakeLLM) Chat(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, errors.New("unused")
}

func TestPricewatchOnce_BadConfig(t *testing.T) {
	cases := map[string]func(f *pwFixture, t *testing.T) string{
		"文件不存在": func(f *pwFixture, _ *testing.T) string { return filepath.Join(f.dir, "missing.yaml") },
		"校验失败": func(f *pwFixture, t *testing.T) string {
			p := f.writeConfig(t, "BTC")
			b, _ := os.ReadFile(p)
			require.NoError(t, os.WriteFile(p, []byte(strings.Replace(string(b), "threshold_pct: 2", "threshold_pct: 0", 1)), 0o600))
			return p
		},
		"未知数据源": func(f *pwFixture, t *testing.T) string {
			p := f.writeConfig(t, "BTC")
			b, _ := os.ReadFile(p)
			require.NoError(t, os.WriteFile(p, []byte(strings.Replace(string(b), "[okx, binance]", "[okx, kraken]", 1)), 0o600))
			return p
		},
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			f := newPWFixture(t, 100)
			pricewatchCfgPath = mk(f, t)
			for _, dry := range []bool{false, true} {
				pricewatchDryRun = dry
				var out bytes.Buffer
				pricewatchOnceCmd.SetOut(&out)
				err := pricewatchOnceCmd.RunE(pricewatchOnceCmd, nil)
				require.Error(t, err)
				assert.Empty(t, f.sender.texts, "配置错误时不应发送任何消息")
				assert.Zero(t, f.newCalls, "配置错误时不应构造 sender")
				assert.Empty(t, out.String())
			}
		})
	}
}

// 现价 90、K 线最高价 101 ⇒ day_drop 触发：dry-run 把提醒写到输出而不发送；非 dry-run 则经 Sender 发送。
func TestPricewatchOnce_DryRun(t *testing.T) {
	f := newPWFixture(t, 90)
	pricewatchCfgPath = f.writeConfig(t, "BTC")
	pricewatchDryRun = true
	var out bytes.Buffer
	pricewatchOnceCmd.SetOut(&out)
	require.NoError(t, pricewatchOnceCmd.RunE(pricewatchOnceCmd, nil))
	assert.Empty(t, f.sender.texts, "dry-run 不发送")
	assert.Contains(t, out.String(), "🔻 BTC 90.00 USDT", "dry-run 应把提醒写到输出")
	assert.Contains(t, out.String(), "\nBTC 90 USDT · 触发中：", "dry-run 应打印各资产现价")
}

func TestPricewatchOnce_Sends(t *testing.T) {
	f := newPWFixture(t, 90)
	pricewatchCfgPath = f.writeConfig(t, "BTC")
	pricewatchDryRun = false
	var out bytes.Buffer
	pricewatchOnceCmd.SetOut(&out)
	require.NoError(t, pricewatchOnceCmd.RunE(pricewatchOnceCmd, nil))
	require.Len(t, f.sender.texts, 1)
	assert.Contains(t, f.sender.texts[0], "🔻 BTC 90.00 USDT")
	assert.Empty(t, out.String(), "非 dry-run 不打印")
}

func TestPricewatchOnce_PropagatesCheckError(t *testing.T) {
	f := newPWFixture(t, 0) // 所有源取价失败
	pricewatchCfgPath = f.writeConfig(t, "BTC")
	pricewatchDryRun = true
	pricewatchOnceCmd.SetOut(&bytes.Buffer{})
	err := pricewatchOnceCmd.RunE(pricewatchOnceCmd, nil)
	require.Error(t, err, "取价全部失败时 once 不能以 nil 返回（QA-11）")
	assert.Contains(t, err.Error(), "BTC")
}

func TestPricewatchBackfill_Symbol(t *testing.T) {
	f := newPWFixture(t, 100)
	pricewatchCfgPath = f.writeConfig(t, "BTC", "ETH")

	t.Run("资产不存在", func(t *testing.T) {
		pricewatchSymbol = "DOGE"
		pricewatchBackfillCmd.SetOut(&bytes.Buffer{})
		err := pricewatchBackfillCmd.RunE(pricewatchBackfillCmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "DOGE")
	})
	t.Run("小写 btc 只回填 BTC", func(t *testing.T) {
		pricewatchSymbol = "btc"
		var out bytes.Buffer
		pricewatchBackfillCmd.SetOut(&out)
		require.NoError(t, pricewatchBackfillCmd.RunE(pricewatchBackfillCmd, nil))
		assert.Regexp(t, `(?m)^BTC 1d: [1-9][0-9]* 根$`, out.String())
		assert.Regexp(t, `(?m)^BTC 5m: [1-9][0-9]* 根$`, out.String())
		assert.NotContains(t, out.String(), "ETH")
	})
	t.Run("不指定则全部资产", func(t *testing.T) {
		pricewatchSymbol = ""
		var out bytes.Buffer
		pricewatchBackfillCmd.SetOut(&out)
		require.NoError(t, pricewatchBackfillCmd.RunE(pricewatchBackfillCmd, nil))
		assert.Regexp(t, `(?m)^ETH 1d: [1-9][0-9]* 根$`, out.String())
	})
	assert.Empty(t, f.sender.texts)
}

// 辅助证据：ctx 取消时 run 以 nil 返回（信号 → ctx 取消由 signal.NotifyContext 完成，DoD e[2] 为 review 项）。
func TestPricewatchRun_ReturnsNilOnCancel(t *testing.T) {
	f := newPWFixture(t, 100)
	pricewatchCfgPath = f.writeConfig(t, "BTC")
	ctx, cancel := context.WithCancel(context.Background())
	pricewatchRunCmd.SetContext(ctx)
	pricewatchRunCmd.SetOut(&bytes.Buffer{})
	done := make(chan error, 1)
	go func() { done <- pricewatchRunCmd.RunE(pricewatchRunCmd, nil) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("ctx 取消后 run 未返回")
	}
}
