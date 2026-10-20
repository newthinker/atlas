package pricewatch

// Context Checkpoint: done_criteria → test mapping (TASK-006)
// functional[0]     解析 1 条 SideAdvice；JSONMode/MaxTokens/Temperature；含现价与规则 id → TestAdvise_ParsesJSON
// functional[1]     提示词含双币赢、各方向、JSON 键名；K 线行数与 5 字段             → TestAdvise_Prompt、TestAdvise_PromptOnlyRequestedSides
// functional[2]     Sides 三种组合                                                    → TestSides
// boundary[0]       围栏 / 前置说明可解析；FinishReason==max_tokens 报错               → TestAdvise_LenientExtract、TestAdvise_Invalid
// error_handling[0] 非 JSON、空 advice、verdict 非法、side 不在 Sides、LLM error      → TestAdvise_Invalid
// error_handling[1] Timeout=50ms 1 秒内报错；LLM==nil 报错不 panic                    → TestAdvise_Timeout、TestAdvise_TimeoutProviderIgnoresCtx、TestAdvise_NilLLM
// QA-6（非 DoD）    term 超长视为 AI 失败                                             → TestAdvise_Invalid（term 超长）

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/llm"
)

type fakeLLM struct {
	mu      sync.Mutex
	content string
	finish  string
	err     error
	block   bool          // 阻塞直到 ctx 结束
	ignore  time.Duration // 无视 ctx，睡这么久再返回
	req     llm.ChatRequest
}

func (f *fakeLLM) Name() string { return "fake" }

func (f *fakeLLM) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	f.mu.Lock()
	f.req = req
	f.mu.Unlock()
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.ignore > 0 {
		time.Sleep(f.ignore)
	}
	if f.err != nil {
		return nil, f.err
	}
	return &llm.ChatResponse{Content: f.content, FinishReason: f.finish}, nil
}

func (f *fakeLLM) lastReq() llm.ChatRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.req
}

const okAdviceJSON = `{"advice":[{"side":"低买","verdict":"再等等","target_price":66500,"term":"3天","reasoning":"近 30 分钟仍在创新低"}]}`

var advT0 = time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)

func advCandles(n int, step time.Duration, base float64) []core.OHLCV {
	out := make([]core.OHLCV, n)
	for i := range out {
		p := base + float64(i)
		out[i] = core.OHLCV{Open: p, High: p + 0.5, Low: p - 0.5, Close: p + 0.25, Time: advT0.Add(time.Duration(i) * step)}
	}
	return out
}

func advInput(triggers ...Trigger) AdviceInput {
	return AdviceInput{
		Asset:    Asset{Symbol: "BTC", Name: "比特币", Pair: "BTCUSDT", Quote: "USDT"},
		Price:    68120,
		Triggers: triggers,
		Overview: Overview{Chg24h: -3.6, Chg7d: -5.1, Chg30d: 2.4, FromATH: -31.2, From4yLow: 312},
		M1:       advCandles(60, time.Minute, 50001),
		M5:       advCandles(288, 5*time.Minute, 40001),
	}
}

var (
	advDrop = Trigger{Rule: msgRule("day_drop", DropFromHigh, Window24h), X: 4.1, Tier: 2, SentInTier: 1, Ref: 71040}
	advRise = Trigger{Rule: msgRule("week_rise", RiseFromLow, Window7d), X: 6.3, Tier: 1, SentInTier: 1, Ref: 64000}
)

func userContent(req llm.ChatRequest) string {
	var b strings.Builder
	for _, m := range req.Messages {
		b.WriteString(m.Content + "\n")
	}
	return b.String()
}

func TestAdvise_ParsesJSON(t *testing.T) {
	f := &fakeLLM{content: okAdviceJSON}
	got, err := (&Advisor{LLM: f, Timeout: time.Second}).Advise(context.Background(), advInput(advDrop))
	if err != nil {
		t.Fatal(err)
	}
	want := SideAdvice{Side: "低买", Verdict: "再等等", TargetPrice: 66500, Term: "3天", Reasoning: "近 30 分钟仍在创新低"}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %+v, want [%+v]", got, want)
	}
	req := f.lastReq()
	if !req.JSONMode || req.MaxTokens != 1500 || req.Temperature != 0.3 {
		t.Fatalf("请求参数不符: JSONMode=%v MaxTokens=%d Temperature=%v", req.JSONMode, req.MaxTokens, req.Temperature)
	}
	msg := userContent(req)
	// 68120 不出现在 K 线（K 线取值 40001–50060）与其他段落中，只能来自现价
	for _, want := range []string{"68120", "day_drop"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Messages 缺少 %q", want)
		}
	}
}

// candleLines 返回恰为「时间,开,高,低,收」且后四个字段可解析为数字的行。
func candleLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		f := strings.Split(line, ",")
		if len(f) != 5 {
			continue
		}
		ok := true
		for _, x := range f[1:] {
			if _, err := strconv.ParseFloat(x, 64); err != nil {
				ok = false
			}
		}
		if ok {
			out = append(out, line)
		}
	}
	return out
}

func TestAdvise_Prompt(t *testing.T) {
	f := &fakeLLM{content: `{"advice":[{"side":"低买","verdict":"现在下单","target_price":1,"term":"1天","reasoning":"r"},{"side":"高卖","verdict":"再等等","target_price":2,"term":"1天","reasoning":"r"}]}`}
	in := advInput(advDrop, advRise)
	if _, err := (&Advisor{LLM: f}).Advise(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	msg := userContent(f.lastReq())
	for _, want := range []string{"双币赢", "低买", "高卖", "week_rise",
		`"advice"`, `"side"`, `"verdict"`, `"target_price"`, `"term"`, `"reasoning"`} {
		if !strings.Contains(msg, want) {
			t.Errorf("提示词缺少 %q", want)
		}
	}
	lines := candleLines(msg)
	if len(lines) != len(in.M1)+len(in.M5) {
		t.Fatalf("K 线行数 %d，期望 %d", len(lines), len(in.M1)+len(in.M5))
	}
	// 首根 1 分钟线与首根 5 分钟线都在，且值按原样写出
	for _, want := range []string{",50001,50001.5,50000.5,50001.25", ",40001,40001.5,40000.5,40001.25"} {
		found := false
		for _, l := range lines {
			if strings.HasSuffix(l, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("K 线缺少以 %q 结尾的行", want)
		}
	}
}

func TestAdvise_PromptOnlyRequestedSides(t *testing.T) {
	f := &fakeLLM{content: okAdviceJSON}
	if _, err := (&Advisor{LLM: f}).Advise(context.Background(), advInput(advDrop)); err != nil {
		t.Fatal(err)
	}
	req := f.lastReq()
	all := req.SystemPrompt + "\n" + userContent(req)
	if !strings.Contains(all, "低买") || strings.Contains(all, "高卖") {
		t.Fatalf("只有跌类时提示词应含「低买」且不含「高卖」:\n%s", all)
	}
}

func TestSides(t *testing.T) {
	cases := []struct {
		name string
		ts   []Trigger
		want []string
	}{
		{"只有跌", []Trigger{advDrop}, []string{"低买"}},
		{"只有涨", []Trigger{advRise}, []string{"高卖"}},
		{"都有", []Trigger{advRise, advDrop}, []string{"低买", "高卖"}},
		{"同向多条", []Trigger{advDrop, advDrop}, []string{"低买"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Sides(c.ts); strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestAdvise_LenientExtract(t *testing.T) {
	cases := map[string]string{
		"json 围栏": "```json\n" + okAdviceJSON + "\n```",
		"裸围栏":     "```\n" + okAdviceJSON + "\n```",
		"前置说明":    "好的，以下是分析结果：\n" + okAdviceJSON,
		"前后都有说明":  "结果如下：" + okAdviceJSON + "\n以上仅供参考。",
		"理由里含花括号": `{"advice":[{"side":"低买","verdict":"再等等","target_price":1,"term":"3天","reasoning":"形如 {a} 的结构"}]}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := (&Advisor{LLM: &fakeLLM{content: content}}).Advise(context.Background(), advInput(advDrop))
			if err != nil || len(got) != 1 || got[0].Side != "低买" {
				t.Fatalf("got %+v, err %v", got, err)
			}
		})
	}
}

func TestAdvise_Invalid(t *testing.T) {
	cases := []struct {
		name string
		f    *fakeLLM
	}{
		{"非 JSON", &fakeLLM{content: "我认为应该再等等"}},
		{"JSON 不完整", &fakeLLM{content: `{"advice":[{"side":"低买"`}},
		{"花括号内非法 JSON", &fakeLLM{content: `{advice: [{side: 低买}]}`}},
		{"advice 类型错", &fakeLLM{content: `{"advice":"再等等"}`}},
		{"右括号在左括号前", &fakeLLM{content: "} 无内容 {"}},
		{"advice 为空", &fakeLLM{content: `{"advice":[]}`}},
		{"缺 advice", &fakeLLM{content: `{"result":"ok"}`}},
		{"verdict 非法", &fakeLLM{content: strings.Replace(okAdviceJSON, "再等等", "观望", 1)}},
		{"side 不在 Sides", &fakeLLM{content: strings.Replace(okAdviceJSON, "低买", "高卖", 1)}},
		{"max_tokens 截断", &fakeLLM{content: okAdviceJSON, finish: "max_tokens"}},
		{"length 截断", &fakeLLM{content: okAdviceJSON, finish: "length"}},
		{"LLM 返回 error", &fakeLLM{err: errors.New("boom")}},
		{"term 超长", &fakeLLM{content: strings.Replace(okAdviceJSON, `"3天"`, `"`+strings.Repeat("天", maxTermRunes+1)+`"`, 1)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := (&Advisor{LLM: c.f}).Advise(context.Background(), advInput(advDrop))
			if err == nil {
				t.Fatalf("期望 error，得到 %+v", got)
			}
		})
	}
}

func TestAdvise_TermAtLimit(t *testing.T) {
	term := strings.Repeat("天", maxTermRunes)
	f := &fakeLLM{content: strings.Replace(okAdviceJSON, `"3天"`, `"`+term+`"`, 1)}
	got, err := (&Advisor{LLM: f}).Advise(context.Background(), advInput(advDrop))
	if err != nil || got[0].Term != term {
		t.Fatalf("恰为上限的 term 应接受: %+v, %v", got, err)
	}
}

func TestAdvise_Timeout(t *testing.T) {
	start := time.Now()
	_, err := (&Advisor{LLM: &fakeLLM{block: true}, Timeout: 50 * time.Millisecond}).Advise(context.Background(), advInput(advDrop))
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("应在 1 秒内超时返回 error: err=%v, 用时 %v", err, time.Since(start))
	}
}

func TestAdvise_TimeoutProviderIgnoresCtx(t *testing.T) {
	start := time.Now()
	_, err := (&Advisor{LLM: &fakeLLM{content: okAdviceJSON, ignore: 2 * time.Second}, Timeout: 50 * time.Millisecond}).
		Advise(context.Background(), advInput(advDrop))
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("provider 无视 ctx 时也应按 Timeout 返回: err=%v, 用时 %v", err, time.Since(start))
	}
}

func TestAdvise_NilLLM(t *testing.T) {
	if _, err := (&Advisor{}).Advise(context.Background(), advInput(advDrop)); err == nil {
		t.Fatal("LLM 为 nil 时应返回 error")
	}
}
