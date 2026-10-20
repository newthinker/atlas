package pricewatch

// Context Checkpoint: done_criteria → test mapping (TASK-007)
// functional[0]     同轮两规则合成 1 条、状态推进、LLM 1 次、多周期五数、Sent24h  → TestCheckAsset_TriggersAndMerges
// functional[1]     发送失败状态不变、同轮清零照常落盘、恢复后补发                → TestCheckAsset_SendFailureKeepsState
// functional[2]     同库新 Engine、未满间隔不重发                                 → TestCheckAsset_RestartNoDuplicate
// functional[3]     7d 缺口 20% 跳过且状态不变、day_* 正常、无发送时 LLM 0 次      → TestCheckAsset_GapSkipsRule
// boundary[0]       首源 0/NaN/+Inf/−1/error/(nil,nil) 换第二源；首源正常用首源     → TestCheckAsset_BadPriceFallsBack
// error_handling[0] Advisor 出错照发且含「AI 分析不可用」；Advisor==nil 照发        → TestCheckAsset_AIFailureStillSends
// error_handling[1] 取价/缺口/同步三类健康告警各 1 条异常、1 条恢复                 → TestCheckAsset_HealthAlerts
// non_functional[0] DryRun 写 Out 不发送不写状态、K 线入库；Snapshot 与并发          → TestCheckAsset_DryRun、TestEngine_Snapshot、TestEngine_ConcurrentSnapshot
// rework r1         并发用例读写真正交错（读侧持续到写侧结束、每轮都追加 sent）    → TestEngine_ConcurrentSnapshot
// rework r1（非 DoD）E34 1m 线来自取价成功源；E38 缺口跳过规则不进 Active         → TestCheckAsset_AIMinuteBarsFromPriceSource、TestEngine_SnapshotExcludesGapSkipped
// QA-10（非 DoD）   涨跌同时触发、2 条 advice 都进消息；同方向重复只取第一条        → TestCheckAsset_TwoSidesAdvice

import (
	"bytes"
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/collector/crypto"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/llm"
)

var engT0 = time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)

const engDay = 24 * time.Hour

type engProvider struct {
	name string

	mu      sync.Mutex
	quote   *core.Quote
	qerr    error
	hist    map[string][]core.OHLCV
	histErr error
	m1Calls int
}

func (p *engProvider) Name() string { return p.name }

func (p *engProvider) FetchQuote(string) (*core.Quote, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.quote, p.qerr
}

func (p *engProvider) FetchHistory(_ string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if interval == "1m" {
		p.m1Calls++
	}
	if p.histErr != nil {
		return nil, p.histErr
	}
	var out []core.OHLCV
	for _, c := range p.hist[interval] {
		if !c.Time.Before(start) && !c.Time.After(end) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (p *engProvider) setPrice(price float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.quote, p.qerr = &core.Quote{Price: price}, nil
}

func (p *engProvider) fail(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.quote, p.qerr = nil, err
}

type engSender struct {
	mu    sync.Mutex
	texts []string // 每次调用都记录，包括失败的
	err   error
}

func (s *engSender) SendText(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.texts = append(s.texts, text)
	return s.err
}

func (s *engSender) sent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.texts...)
}

func (s *engSender) setErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

// engCount 统计以 prefix 开头的文本条数（按开头匹配，避免被正文别处的字样顺带满足）。
func engCount(texts []string, prefix string) int {
	n := 0
	for _, s := range texts {
		if strings.HasPrefix(s, prefix) {
			n++
		}
	}
	return n
}

type engLLM struct {
	mu      sync.Mutex
	calls   int
	content string
	err     error
}

func (l *engLLM) Name() string { return "eng-fake" }

func (l *engLLM) Chat(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	if l.err != nil {
		return nil, l.err
	}
	return &llm.ChatResponse{Content: l.content}, nil
}

func (l *engLLM) callCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

var (
	engDayDrop  = Rule{ID: "day_drop", Type: DropFromHigh, Window: Window24h, ThresholdPct: 2, StepPct: 2, Notify: NotifyPolicy{time.Hour, 3}}
	engWeekDrop = Rule{ID: "week_drop", Type: DropFromHigh, Window: Window7d, ThresholdPct: 5, StepPct: 5, Notify: NotifyPolicy{time.Hour, 3}}
	engDayRise  = Rule{ID: "day_rise", Type: RiseFromLow, Window: Window24h, ThresholdPct: 2, StepPct: 2, Notify: NotifyPolicy{time.Hour, 3}}
)

const engBuyJSON = `{"advice":[{"side":"低买","verdict":"再等等","target_price":90,"term":"3天","reasoning":"engine-buy-reason"}]}`

type engFixture struct {
	e        *Engine
	st       *Store
	asset    Asset
	okx, bin *engProvider
	snd      *engSender
	llm      *engLLM

	mu  sync.Mutex
	now time.Time
}

// newEngFixture 建一个本地 K 线齐全且平坦（high=low=close=100）的库：5m 覆盖 8 天，1d 覆盖 60 天。
func newEngFixture(t *testing.T, rules ...Rule) *engFixture {
	t.Helper()
	st, _ := newTestStore(t)
	var m5, d1 []core.OHLCV
	for ts := engT0.Add(-8 * engDay); !ts.After(engT0); ts = ts.Add(5 * time.Minute) {
		m5 = append(m5, candle(ts, 100, 100, 100))
	}
	for ts := engT0.Truncate(engDay).Add(-59 * engDay); !ts.After(engT0); ts = ts.Add(engDay) {
		d1 = append(d1, candle(ts, 100, 100, 100))
	}
	mustUpsert(t, st, "BTCUSDT", "5m", m5...)
	mustUpsert(t, st, "BTCUSDT", "1d", d1...)

	f := &engFixture{
		st:    st,
		asset: Asset{Symbol: "BTC", Name: "比特币", Pair: "BTCUSDT", Quote: "USDT", Interval: 5 * time.Minute, Rules: rules},
		okx:   &engProvider{name: "okx"},
		bin:   &engProvider{name: "binance"},
		snd:   &engSender{},
		llm:   &engLLM{content: engBuyJSON},
		now:   engT0,
	}
	f.okx.setPrice(100)
	f.bin.setPrice(100)
	cfg := &Config{}
	cfg.Assets.Crypto.Sources = []string{"okx", "binance"}
	cfg.Assets.Crypto.Quote = "USDT"
	cfg.Health.FailAlertAfter = 30 * time.Minute
	cfg.cryptoAssets = []Asset{f.asset}
	f.e = f.newEngine(cfg)
	return f
}

func (f *engFixture) newEngine(cfg *Config) *Engine {
	return &Engine{
		Cfg:       cfg,
		Store:     f.st,
		Providers: []crypto.Provider{f.okx, f.bin},
		Advisor:   &Advisor{LLM: f.llm},
		Sender:    f.snd,
		Now:       f.clock,
	}
}

func (f *engFixture) clock() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *engFixture) at(d time.Duration) time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = engT0.Add(d)
	return f.now
}

func (f *engFixture) run(t *testing.T) error {
	t.Helper()
	return f.e.CheckAsset(context.Background(), f.asset)
}

func (f *engFixture) mustRun(t *testing.T) {
	t.Helper()
	if err := f.run(t); err != nil {
		t.Fatalf("CheckAsset: %v", err)
	}
}

func (f *engFixture) state(t *testing.T, id string) RuleState {
	t.Helper()
	st, err := f.st.GetRuleState("BTCUSDT", id)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func (f *engFixture) putState(t *testing.T, id string, st RuleState) {
	t.Helper()
	if err := f.st.PutRuleState("BTCUSDT", id, st); err != nil {
		t.Fatal(err)
	}
}

func wantState(t *testing.T, id string, got, want RuleState) {
	t.Helper()
	if got.Tier != want.Tier || got.SentInTier != want.SentInTier || !got.LastSentAt.Equal(want.LastSentAt) {
		t.Errorf("%s state = %+v, want %+v", id, got, want)
	}
}

func TestCheckAsset_TriggersAndMerges(t *testing.T) {
	f := newEngFixture(t, engDayDrop, engWeekDrop)
	// 多周期 fixture：24h 前那根收 99；7d 前那根高/收 102（同时成为 7d 最高）；
	// 30d 起点后的第一根日线收 90；40 天前日线最高 150（历史最高）；50 天前日线最低 50（4 年最低，1500 天前另有更低的 10）
	mustUpsert(t, f.st, "BTCUSDT", "5m",
		candle(engT0.Add(-engDay), 100, 98, 99),
		candle(engT0.Add(-7*engDay), 102, 100, 102))
	d30 := engT0.Add(-30 * engDay).Truncate(engDay).Add(engDay)
	mustUpsert(t, f.st, "BTCUSDT", "1d",
		candle(d30, 100, 90, 90),
		candle(engT0.Truncate(engDay).Add(-40*engDay), 150, 100, 100),
		candle(engT0.Truncate(engDay).Add(-50*engDay), 100, 50, 100),
		// 4 年之外的更低点：只影响 all，不得进入「距 4 年最低」
		candle(engT0.Truncate(engDay).Add(-1500*engDay), 100, 10, 100))
	f.okx.setPrice(95)

	now := f.at(0)
	f.mustRun(t)

	sent := f.snd.sent()
	if len(sent) != 1 {
		t.Fatalf("Sender got %d messages, want 1: %q", len(sent), sent)
	}
	msg := sent[0]
	for _, want := range []string{
		"· 1天窗口：较 24h 最高价 100.00 下跌 5.0%（第 2 档）",
		"· 1周窗口：较 7d 最高价 102.00 下跌 6.9%（第 1 档）",
		"【多周期】24h −4.0% · 7d −6.9% · 30d +5.6% · 距历史最高 −36.7% · 距 4 年最低 +90%",
		"engine-buy-reason",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
	if !strings.HasPrefix(msg, "🔻 BTC 95.00 USDT（okx）") {
		t.Errorf("message title wrong:\n%s", msg)
	}
	wantState(t, "day_drop", f.state(t, "day_drop"), RuleState{2, 1, now})
	wantState(t, "week_drop", f.state(t, "week_drop"), RuleState{1, 1, now})
	if n := f.llm.callCount(); n != 1 {
		t.Errorf("LLM calls = %d, want 1", n)
	}
	if n := f.okx.m1Calls; n != 1 {
		t.Errorf("1m fetches from source provider = %d, want 1", n)
	}

	if n := f.e.Sent24h(); n != 1 {
		t.Errorf("Sent24h = %d, want 1", n)
	}
	f.at(25 * time.Hour)
	if n := f.e.Sent24h(); n != 0 {
		t.Errorf("Sent24h after 25h = %d, want 0", n)
	}
}

func TestCheckAsset_SendFailureKeepsState(t *testing.T) {
	f := newEngFixture(t, engDayDrop, engDayRise)
	f.okx.setPrice(95) // 24h 低点被现价拉到 95 ⇒ day_rise 不满足
	f.putState(t, "day_rise", RuleState{2, 1, engT0.Add(-2 * time.Hour)})
	f.snd.setErr(errors.New("telegram down"))

	f.at(0)
	if err := f.run(t); err == nil {
		t.Error("CheckAsset should report send failure")
	}
	if n := len(f.snd.sent()); n != 1 {
		t.Fatalf("send attempts = %d, want 1", n)
	}
	wantState(t, "day_drop", f.state(t, "day_drop"), RuleState{})
	wantState(t, "day_rise", f.state(t, "day_rise"), RuleState{0, 0, engT0.Add(-2 * time.Hour)})
	if n := f.e.Sent24h(); n != 0 {
		t.Errorf("Sent24h = %d after failed send, want 0", n)
	}

	f.snd.setErr(nil)
	now := f.at(5 * time.Minute)
	f.mustRun(t)
	sent := f.snd.sent()
	if len(sent) != 2 || !strings.Contains(sent[1], "1天窗口：较 24h 最高价 100.00 下跌 5.0%（第 2 档）") {
		t.Fatalf("retry not sent: %q", sent)
	}
	wantState(t, "day_drop", f.state(t, "day_drop"), RuleState{2, 1, now})
}

func TestCheckAsset_RestartNoDuplicate(t *testing.T) {
	f := newEngFixture(t, engDayDrop)
	f.okx.setPrice(95)
	f.at(0)
	f.mustRun(t)
	if n := len(f.snd.sent()); n != 1 {
		t.Fatalf("first round sent %d, want 1", n)
	}

	f.e = f.newEngine(f.e.Cfg) // 同库新建 Engine，内存状态全部丢失
	f.at(30 * time.Minute)
	f.mustRun(t)
	if n := len(f.snd.sent()); n != 1 {
		t.Errorf("after restart sent %d in total, want 1 (no duplicate)", n)
	}
}

// engDeleteCandles 删掉 BTCUSDT 某周期 [from, to) 内的 K 线。
func engDeleteCandles(t *testing.T, st *Store, bar string, from, to time.Time) {
	t.Helper()
	if _, err := st.db.Exec(`DELETE FROM candles WHERE pair = ? AND bar = ? AND ts >= ? AND ts < ?`,
		"BTCUSDT", bar, from.UnixMilli(), to.UnixMilli()); err != nil {
		t.Fatal(err)
	}
}

// engPunchGap 删掉 7d 窗口里 24h 之外的一段，使 7d 缺口约 20%、24h 不受影响。
func engPunchGap(t *testing.T, st *Store) {
	t.Helper()
	from := engT0.Add(-6 * engDay)
	engDeleteCandles(t, st, "5m", from, from.Add(7*engDay/5))
}

func TestCheckAsset_GapSkipsRule(t *testing.T) {
	t.Run("skipped rule keeps state, day rule sends", func(t *testing.T) {
		f := newEngFixture(t, engDayDrop, engWeekDrop)
		engPunchGap(t, f.st)
		prev := RuleState{2, 1, engT0.Add(-10 * time.Minute)}
		f.putState(t, "week_drop", prev)
		f.okx.setPrice(95)

		now := f.at(0)
		f.mustRun(t)
		sent := f.snd.sent()
		if len(sent) != 1 {
			t.Fatalf("sent %d, want 1", len(sent))
		}
		if strings.Contains(sent[0], "1周窗口") {
			t.Errorf("skipped week_drop appears in message:\n%s", sent[0])
		}
		if !strings.Contains(sent[0], "1天窗口") {
			t.Errorf("day_drop missing:\n%s", sent[0])
		}
		wantState(t, "week_drop", f.state(t, "week_drop"), prev)
		wantState(t, "day_drop", f.state(t, "day_drop"), RuleState{2, 1, now})
	})

	t.Run("skip is not a clear", func(t *testing.T) {
		// 价格不满足 week_drop：若把跳过当成 t=0，状态会被清零
		f := newEngFixture(t, engDayDrop, engWeekDrop)
		engPunchGap(t, f.st)
		prev := RuleState{2, 1, engT0.Add(-10 * time.Minute)}
		f.putState(t, "week_drop", prev)
		f.at(0)
		f.mustRun(t)
		wantState(t, "week_drop", f.state(t, "week_drop"), prev)
		if n := len(f.snd.sent()); n != 0 {
			t.Errorf("sent %d, want 0", n)
		}
		if n := f.llm.callCount(); n != 0 {
			t.Errorf("LLM calls = %d on a round with nothing to send, want 0", n)
		}
	})
}

func TestCheckAsset_BadPriceFallsBack(t *testing.T) {
	bad := []struct {
		name  string
		quote *core.Quote
		err   error
	}{
		{"zero", &core.Quote{Price: 0}, nil},
		{"NaN", &core.Quote{Price: math.NaN()}, nil},
		{"+Inf", &core.Quote{Price: math.Inf(1)}, nil},
		{"negative", &core.Quote{Price: -1}, nil},
		{"error", nil, errors.New("okx down")},
		{"nil quote nil error", nil, nil},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			f := newEngFixture(t, engDayDrop)
			f.okx.mu.Lock()
			f.okx.quote, f.okx.qerr = c.quote, c.err
			f.okx.mu.Unlock()
			f.bin.setPrice(95)
			f.mustRun(t)
			sent := f.snd.sent()
			if len(sent) != 1 || !strings.HasPrefix(sent[0], "🔻 BTC 95.00 USDT（binance）") {
				t.Fatalf("want one alert from binance at 95, got %q", sent)
			}
		})
	}

	t.Run("first source ok", func(t *testing.T) {
		f := newEngFixture(t, engDayDrop)
		f.okx.setPrice(95)
		f.bin.setPrice(90)
		f.mustRun(t)
		sent := f.snd.sent()
		if len(sent) != 1 || !strings.HasPrefix(sent[0], "🔻 BTC 95.00 USDT（okx）") {
			t.Fatalf("want one alert from okx at 95, got %q", sent)
		}
	})

	t.Run("source without provider is skipped", func(t *testing.T) {
		f := newEngFixture(t, engDayDrop)
		f.e.Cfg.Assets.Crypto.Sources = []string{"coingecko", "binance"}
		f.bin.setPrice(95)
		f.mustRun(t)
		if sent := f.snd.sent(); len(sent) != 1 || !strings.Contains(sent[0], "（binance）") {
			t.Fatalf("want alert from binance, got %q", sent)
		}
	})

	t.Run("sources order wins over Providers order", func(t *testing.T) {
		f := newEngFixture(t, engDayDrop)
		f.e.Providers = []crypto.Provider{f.bin, f.okx}
		f.okx.setPrice(95)
		f.bin.setPrice(90)
		f.mustRun(t)
		if sent := f.snd.sent(); len(sent) != 1 || !strings.Contains(sent[0], "（okx）") {
			t.Fatalf("want okx first per Cfg sources, got %q", sent)
		}
	})
}

func TestCheckAsset_AIFailureStillSends(t *testing.T) {
	t.Run("advisor error", func(t *testing.T) {
		f := newEngFixture(t, engDayDrop)
		f.llm.err = errors.New("llm down")
		f.okx.setPrice(95)
		now := f.at(0)
		f.mustRun(t)
		sent := f.snd.sent()
		if len(sent) != 1 || !strings.Contains(sent[0], "【AI 结论】AI 分析不可用") {
			t.Fatalf("want alert with AI unavailable, got %q", sent)
		}
		wantState(t, "day_drop", f.state(t, "day_drop"), RuleState{2, 1, now})
	})

	t.Run("advisor nil", func(t *testing.T) {
		f := newEngFixture(t, engDayDrop)
		f.e.Advisor = nil
		f.okx.setPrice(95)
		f.mustRun(t)
		sent := f.snd.sent()
		if len(sent) != 1 || !strings.Contains(sent[0], "【AI 结论】AI 分析不可用") {
			t.Fatalf("want alert with AI unavailable, got %q", sent)
		}
		if n := f.llm.callCount(); n != 0 {
			t.Errorf("LLM calls = %d, want 0", n)
		}
		if n := f.okx.m1Calls; n != 0 {
			t.Errorf("1m fetches = %d with AI off, want 0", n)
		}
	})
}

func TestCheckAsset_TwoSidesAdvice(t *testing.T) {
	f := newEngFixture(t, engDayDrop, engDayRise)
	// 24h 内高 110、低 90，现价 100 ⇒ 跌 9.1%、涨 11.1% 同时触发
	mustUpsert(t, f.st, "BTCUSDT", "5m",
		candle(engT0.Add(-2*time.Hour), 110, 100, 100),
		candle(engT0.Add(-3*time.Hour), 100, 90, 100))
	f.llm.content = `{"advice":[` +
		`{"side":"低买","verdict":"再等等","target_price":95,"term":"3天","reasoning":"R-buy-first"},` +
		`{"side":"高卖","verdict":"现在下单","target_price":105,"term":"1天","reasoning":"R-sell"},` +
		`{"side":"低买","verdict":"现在下单","target_price":96,"term":"1天","reasoning":"R-buy-dup"}]}`
	f.mustRun(t)
	sent := f.snd.sent()
	if len(sent) != 1 {
		t.Fatalf("sent %d, want 1", len(sent))
	}
	msg := sent[0]
	if !strings.HasPrefix(msg, "🔀") {
		t.Errorf("want 🔀 icon:\n%s", msg)
	}
	for _, want := range []string{
		"低买：R-buy-first", "高卖：R-sell", "低买：再等等", "高卖：现在下单",
		"· 1天窗口：较 24h 最低价 90.00 上涨 11.1%（第 5 档）",
		"· 1天窗口：较 24h 最高价 110.00 下跌 9.1%（第 4 档）",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "R-buy-dup") {
		t.Errorf("duplicate 低买 advice should be dropped:\n%s", msg)
	}
}

func TestCheckAsset_HealthAlerts(t *testing.T) {
	// steps 依次在 0、29、30、31 分钟失败，然后在 32、33 分钟恢复
	check := func(t *testing.T, f *engFixture, fail, recover func(), subject string) {
		t.Helper()
		down, up := "⚠️ 监控异常："+subject, "✅ 已恢复："+subject
		fail()
		for _, m := range []int{0, 29} {
			f.at(time.Duration(m) * time.Minute)
			_ = f.run(t)
		}
		if n := engCount(f.snd.sent(), down); n != 0 {
			t.Fatalf("alert before 30m: %d", n)
		}
		for _, m := range []int{30, 31} {
			f.at(time.Duration(m) * time.Minute)
			_ = f.run(t)
		}
		if n := engCount(f.snd.sent(), down); n != 1 {
			t.Fatalf("down alerts = %d, want 1: %q", n, f.snd.sent())
		}
		recover()
		for _, m := range []int{32, 33} {
			f.at(time.Duration(m) * time.Minute)
			_ = f.run(t)
		}
		if n := engCount(f.snd.sent(), up); n != 1 {
			t.Fatalf("recovered alerts = %d, want 1: %q", n, f.snd.sent())
		}
		if n := engCount(f.snd.sent(), down); n != 1 {
			t.Fatalf("down alerts = %d after recovery, want 1", n)
		}
	}

	t.Run("price", func(t *testing.T) {
		f := newEngFixture(t, engDayDrop)
		check(t, f, func() {
			f.okx.fail(errors.New("okx down"))
			f.bin.fail(errors.New("binance down"))
		}, func() { f.bin.setPrice(100) }, "BTC 取价")
	})

	t.Run("gap", func(t *testing.T) {
		f := newEngFixture(t, engWeekDrop)
		check(t, f, func() { engPunchGap(t, f.st) }, func() {
			var m5 []core.OHLCV
			for ts := engT0.Add(-7 * engDay); !ts.After(engT0); ts = ts.Add(5 * time.Minute) {
				m5 = append(m5, candle(ts, 100, 100, 100))
			}
			mustUpsert(t, f.st, "BTCUSDT", "5m", m5...)
		}, "BTC week_drop 数据缺口")
	})

	t.Run("sync", func(t *testing.T) {
		f := newEngFixture(t, engDayDrop)
		hp := &engProvider{name: "binance", histErr: errors.New("history down")}
		f.e.Syncer = &Syncer{Providers: []crypto.Provider{hp}, Store: f.st, Now: f.clock}
		check(t, f, func() {}, func() {
			hp.mu.Lock()
			hp.histErr = nil
			hp.mu.Unlock()
		}, "BTC 1d K 线同步")
		if n := engCount(f.snd.sent(), "⚠️ 监控异常：BTC 5m K 线同步"); n != 1 {
			t.Errorf("5m sync down alerts = %d, want 1 (tracked separately)", n)
		}
	})
}

func TestCheckAsset_DryRun(t *testing.T) {
	f := newEngFixture(t, engDayDrop, engDayRise, engWeekDrop)
	f.okx.setPrice(95) // day_drop 触发；day_rise 从 2 档回落（应清零但 dry-run 不写）
	riseBefore := RuleState{2, 1, engT0.Add(-2 * time.Hour)}
	f.putState(t, "day_rise", riseBefore)
	engPunchGap(t, f.st) // week_drop 缺口跳过 ⇒ After=0 时立即产生健康告警
	f.e.Cfg.Health.FailAlertAfter = 0
	newBar := candle(engT0.Add(5*time.Minute), 100, 100, 100)
	hp := &engProvider{name: "binance", hist: map[string][]core.OHLCV{"5m": {newBar}}}
	f.e.Syncer = &Syncer{Providers: []crypto.Provider{hp}, Store: f.st, Now: func() time.Time { return engT0.Add(6 * time.Minute) }}
	var out bytes.Buffer
	f.e.DryRun, f.e.Out = true, &out

	f.mustRun(t)
	if n := len(f.snd.sent()); n != 0 {
		t.Errorf("Sender called %d times in dry-run", n)
	}
	text := out.String()
	for _, want := range []string{"1天窗口：较 24h 最高价 100.00 下跌 5.0%", "⚠️ 监控异常：BTC week_drop 数据缺口"} {
		if !strings.Contains(text, want) {
			t.Errorf("Out missing %q:\n%s", want, text)
		}
	}
	wantState(t, "day_drop", f.state(t, "day_drop"), RuleState{})
	wantState(t, "day_rise", f.state(t, "day_rise"), riseBefore)
	if latest, ok, err := f.st.LatestCandle("BTCUSDT", "5m"); err != nil || !ok || !latest.Equal(newBar.Time) {
		t.Errorf("latest 5m = %v,%v,%v; want %v (K 线照常入库)", latest, ok, err, newBar.Time)
	}
	if n := f.e.Sent24h(); n != 0 {
		t.Errorf("Sent24h = %d after dry-run, want 0", n)
	}
}

func TestEngine_Snapshot(t *testing.T) {
	f := newEngFixture(t, engDayDrop, engWeekDrop)
	if rows := f.e.Snapshot(); len(rows) != 1 || rows[0].Price != 0 || !rows[0].LastOK.IsZero() {
		t.Fatalf("before any check: %+v", rows)
	}
	f.okx.setPrice(95)
	now := f.at(0)
	f.mustRun(t)
	rows := f.e.Snapshot()
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.Asset.Symbol != "BTC" || r.Price != 95 || !r.LastOK.Equal(now) {
		t.Errorf("row = %+v", r)
	}
	if strings.Join(r.Active, ",") != "day_drop 第2档,week_drop 第1档" {
		t.Errorf("Active = %q", r.Active)
	}

	// 取价失败的一轮：价格与成功时间保留上一次成功的值
	f.okx.fail(errors.New("down"))
	f.bin.fail(errors.New("down"))
	f.at(5 * time.Minute)
	if err := f.run(t); err == nil {
		t.Error("CheckAsset should fail when all sources fail")
	}
	if r := f.e.Snapshot()[0]; r.Price != 95 || !r.LastOK.Equal(now) {
		t.Errorf("after failure row = %+v, want last success kept", r)
	}

	// 价格回升、规则不再满足：Active 清空
	f.okx.setPrice(100)
	f.at(10 * time.Minute)
	f.mustRun(t)
	if r := f.e.Snapshot()[0]; len(r.Active) != 0 {
		t.Errorf("Active = %q, want empty", r.Active)
	}
}

func TestEngine_ConcurrentSnapshot(t *testing.T) {
	// 写侧价格在 95（第 2 档，跳过防抖立即发送）与 100（清零）之间交替，每两轮追加一次 sent；
	// 读侧持续到写侧全部结束，保证 recordSnap / sent 追加与 Snapshot / Sent24h 在时间上真正交错。
	f := newEngFixture(t, engDayDrop)
	const rounds = 20

	done := make(chan struct{})
	var overlapped int // 读到写侧已写入快照的次数
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			if rows := f.e.Snapshot(); rows[0].Price != 0 {
				overlapped++
			}
			_ = f.e.Sent24h()
		}
	}()
	prices := [2]float64{95, 100}
	for i := range rounds {
		f.okx.setPrice(prices[i%2])
		if err := f.e.CheckAsset(context.Background(), f.asset); err != nil {
			t.Fatalf("round %d: %v", i, err)
		}
	}
	close(done)
	wg.Wait()

	if overlapped == 0 {
		t.Fatal("reader never observed a written snapshot: reads and writes did not overlap")
	}
	if n := len(f.snd.sent()); n != rounds/2 {
		t.Fatalf("sent %d alerts, want %d (every other round, so sent is appended while reading)", n, rounds/2)
	}
}

func TestCheckAsset_AIMinuteBarsFromPriceSource(t *testing.T) {
	// E34：取价成功的源不是 Providers[0] 时，AI 的 1m 线向取价成功的那个源取
	f := newEngFixture(t, engDayDrop)
	f.okx.fail(errors.New("okx down"))
	f.bin.setPrice(95)
	f.mustRun(t)
	if len(f.snd.sent()) != 1 {
		t.Fatalf("sent %d, want 1", len(f.snd.sent()))
	}
	if f.okx.m1Calls != 0 || f.bin.m1Calls != 1 {
		t.Errorf("1m fetches okx=%d binance=%d, want 0/1", f.okx.m1Calls, f.bin.m1Calls)
	}
}

func TestEngine_SnapshotExcludesGapSkipped(t *testing.T) {
	// E38：缺口跳过的规则即使按价格会触发，也不计入 Active
	f := newEngFixture(t, engDayDrop, engWeekDrop)
	engPunchGap(t, f.st)
	f.okx.setPrice(90)
	f.mustRun(t)
	if got := f.e.Snapshot()[0].Active; strings.Join(got, ",") != "day_drop 第5档" {
		t.Errorf("Active = %q, want [day_drop 第5档] (week_drop skipped for gap)", got)
	}
}

func TestCheckAsset_RepeatUsesCurrentTier(t *testing.T) {
	// AD-28 P16：已提醒到第 2 档，价格回到第 1 档范围且满间隔 ⇒ 重复提醒，消息写当前档位 1
	f := newEngFixture(t, engDayDrop)
	f.putState(t, "day_drop", RuleState{2, 1, engT0.Add(-2 * time.Hour)})
	f.okx.setPrice(97.5)
	now := f.at(0)
	f.mustRun(t)
	sent := f.snd.sent()
	if len(sent) != 1 || !strings.Contains(sent[0], "· 1天窗口：较 24h 最高价 100.00 下跌 2.5%（第 1 档，第 2/3 次提醒）") {
		t.Fatalf("want repeat alert at current tier 1, got %q", sent)
	}
	wantState(t, "day_drop", f.state(t, "day_drop"), RuleState{2, 2, now})
}

func TestCheckAsset_ShortWindow(t *testing.T) {
	f := newEngFixture(t, engWeekDrop)
	if err := f.st.PutKV(ListingKey("BTCUSDT", "5m"), strconv.FormatInt(engT0.Add(-3*engDay).UnixMilli(), 10)); err != nil {
		t.Fatal(err)
	}
	f.okx.setPrice(90)
	f.mustRun(t)
	sent := f.snd.sent()
	if len(sent) != 1 || !strings.Contains(sent[0], "（第 2 档）（窗口数据仅 3 天）") {
		t.Fatalf("want short-window note, got %q", sent)
	}
}

func TestCheckAsset_GapExactlyTenPercent(t *testing.T) {
	// 30d 日线 30 槽缺 3 槽 = 0.10，不超过阈值 ⇒ 规则照常计算
	monthDrop := Rule{ID: "month_drop", Type: DropFromHigh, Window: Window30d, ThresholdPct: 2, StepPct: 2, Notify: NotifyPolicy{time.Hour, 3}}
	f := newEngFixture(t, monthDrop)
	from := engT0.Truncate(engDay).Add(-10 * engDay)
	engDeleteCandles(t, f.st, "1d", from, from.Add(3*engDay))
	if ws := engWinStats(t, f.st, Window30d, 95, engT0); math.Abs(ws.GapRatio-0.1) > 1e-9 {
		t.Fatalf("fixture GapRatio = %v, want exactly 0.1", ws.GapRatio)
	}
	f.okx.setPrice(95)
	f.mustRun(t)
	if sent := f.snd.sent(); len(sent) != 1 || !strings.Contains(sent[0], "30天窗口") {
		t.Fatalf("want month_drop alert at gap 0.10, got %q", sent)
	}
}

func engWinStats(t *testing.T, s *Store, w Window, price float64, now time.Time) WindowStats {
	t.Helper()
	ws, err := WindowStatsFor(s, "BTCUSDT", w, price, now)
	if err != nil {
		t.Fatal(err)
	}
	return ws
}
