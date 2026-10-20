package pricewatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/llm"
)

// AdviceInput 是一次 AI 分析的输入（spec §7）。
type AdviceInput struct {
	Asset    Asset
	Price    float64
	Triggers []Trigger
	Overview Overview
	M1       []core.OHLCV // 最近 1 小时的 1 分钟线（调用方现取，不入库）
	M5       []core.OHLCV // 最近 24 小时的 5 分钟线
}

// Advisor 调用 LLM 给出下单建议。
type Advisor struct {
	LLM     llm.Provider
	Timeout time.Duration // >0 时限制单次调用时长
}

// maxTermRunes 是 term 的长度上限；超过视为 AI 失败（QA-6：AI 字段无界会让整条消息超过 Telegram 上限）。
// side、verdict 已限定为枚举值，reasoning 由 FormatAlert 截断。
const maxTermRunes = 20

const (
	sideBuy  = "低买"
	sideSell = "高卖"
)

var validVerdicts = map[string]bool{"现在下单": true, "再等等": true}

const advisorSystemPrompt = "你是加密货币短线交易分析助手。只输出一个 JSON 对象，不要输出任何其他文字。"

// Sides 返回需要分析的方向：只有跌 → ["低买"]，只有涨 → ["高卖"]，都有 → ["低买","高卖"]。
func Sides(ts []Trigger) []string {
	var drop, rise bool
	for _, t := range ts {
		if t.Rule.Type == RiseFromLow {
			rise = true
		} else {
			drop = true
		}
	}
	var out []string
	if drop {
		out = append(out, sideBuy)
	}
	if rise {
		out = append(out, sideSell)
	}
	return out
}

// Advise 请求 LLM 并校验结果。以下情况返回 error（调用方按「AI 分析不可用」处理）：
// LLM 为 nil、调用出错或超时、回复因长度被截断、找不到可解析的 JSON、advice 为空、
// side 不在 Sides(in.Triggers) 中、verdict 不是「现在下单」/「再等等」、term 超过 maxTermRunes。
func (a *Advisor) Advise(ctx context.Context, in AdviceInput) ([]SideAdvice, error) {
	if a.LLM == nil {
		return nil, errors.New("advisor: LLM not configured")
	}
	if a.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.Timeout)
		defer cancel()
	}
	sides := Sides(in.Triggers)
	req := llm.ChatRequest{
		SystemPrompt: advisorSystemPrompt,
		Messages:     []llm.Message{{Role: "user", Content: buildAdvicePrompt(in, sides)}},
		MaxTokens:    1500,
		Temperature:  0.3,
		JSONMode:     true,
	}
	resp, err := chatWithContext(ctx, a.LLM, req)
	if err != nil {
		return nil, fmt.Errorf("advisor: %w", err)
	}
	// claude 报 max_tokens，openai/ollama 报 length：JSON 多半不完整
	if resp.FinishReason == "max_tokens" || resp.FinishReason == "length" {
		return nil, fmt.Errorf("advisor: response truncated (%s)", resp.FinishReason)
	}
	return parseAdvice(resp.Content, sides)
}

// chatWithContext 在 provider 无视 ctx 时也能按 ctx 返回（provider 的 goroutine 自行结束）。
func chatWithContext(ctx context.Context, p llm.Provider, req llm.ChatRequest) (*llm.ChatResponse, error) {
	type result struct {
		resp *llm.ChatResponse
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		resp, err := p.Chat(ctx, req)
		ch <- result{resp, err}
	}()
	select {
	case r := <-ch:
		return r.resp, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// parseAdvice 宽松提取（AD-23）：取第一个 '{' 到最后一个 '}' 之间的内容解析，
// 这样 ``` 围栏与前后说明文字都被去掉；再逐条校验。
func parseAdvice(content string, sides []string) ([]SideAdvice, error) {
	start, end := strings.Index(content, "{"), strings.LastIndex(content, "}")
	if start < 0 || end < start {
		return nil, errors.New("advisor: no JSON object in response")
	}
	var out struct {
		Advice []SideAdvice `json:"advice"`
	}
	if err := json.Unmarshal([]byte(content[start:end+1]), &out); err != nil {
		return nil, fmt.Errorf("advisor: parse response: %w", err)
	}
	if len(out.Advice) == 0 {
		return nil, errors.New("advisor: empty advice")
	}
	allowed := map[string]bool{}
	for _, s := range sides {
		allowed[s] = true
	}
	for _, adv := range out.Advice {
		switch {
		case !allowed[adv.Side]:
			return nil, fmt.Errorf("advisor: unexpected side %q", adv.Side)
		case !validVerdicts[adv.Verdict]:
			return nil, fmt.Errorf("advisor: invalid verdict %q", adv.Verdict)
		case utf8.RuneCountInString(adv.Term) > maxTermRunes:
			return nil, fmt.Errorf("advisor: term longer than %d characters", maxTermRunes)
		}
	}
	return out.Advice, nil
}

func buildAdvicePrompt(in AdviceInput, sides []string) string {
	var b strings.Builder
	b.WriteString("用户在 okx 做「双币赢」，需要判断现在是否适合下单。\n")
	fmt.Fprintf(&b, "资产：%s（%s），现价 %s %s\n", in.Asset.Symbol, in.Asset.Name, plainNum(in.Price), in.Asset.Quote)
	fmt.Fprintf(&b, "请分析的方向：%s\n", strings.Join(sides, "、"))
	b.WriteString("\n本轮触发的规则：\n")
	for _, t := range in.Triggers {
		ref, verb := "最高价", "下跌"
		if t.Rule.Type == RiseFromLow {
			ref, verb = "最低价", "上涨"
		}
		fmt.Fprintf(&b, "- %s：%s窗口内较%s %s %s %.1f%%，第 %d 档\n",
			t.Rule.ID, t.Rule.Window, ref, plainNum(t.Ref), verb, t.X, t.Tier)
	}
	ov := in.Overview
	fmt.Fprintf(&b, "\n多周期概况：24h %s，7d %s，30d %s，距历史最高 %s，距 4 年最低 %s\n",
		formatPct(ov.Chg24h, 1), formatPct(ov.Chg7d, 1), formatPct(ov.Chg30d, 1),
		formatPct(ov.FromATH, 1), formatPct(ov.From4yLow, 0))
	writeCandles(&b, "最近 1 小时的 1 分钟线", in.M1)
	writeCandles(&b, "最近 24 小时的 5 分钟线", in.M5)

	fmt.Fprintf(&b, "\n每个方向给一条建议，只输出如下结构的 JSON：\n"+
		`{"advice": [{"side": "%s", "verdict": "现在下单|再等等", "target_price": 0, "term": "1天|3天|7天…", "reasoning": "…"}]}`+"\n",
		strings.Join(sides, "|"))
	fmt.Fprintf(&b, "side 只能取：%s；verdict 只能取「现在下单」或「再等等」；term 不超过 %d 个字。\n",
		strings.Join(sides, "、"), maxTermRunes)
	return b.String()
}

// writeCandles 一行一根：时间,开,高,低,收（时间为 UTC）。
func writeCandles(b *strings.Builder, title string, cs []core.OHLCV) {
	fmt.Fprintf(b, "\n%s（每行：时间 开 高 低 收，逗号分隔，时间为 UTC）：\n", title)
	for _, c := range cs {
		fmt.Fprintf(b, "%s,%s,%s,%s,%s\n", c.Time.UTC().Format("2006-01-02 15:04"),
			plainNum(c.Open), plainNum(c.High), plainNum(c.Low), plainNum(c.Close))
	}
}

// plainNum 不带千分位、不补零，供 LLM 读。
func plainNum(x float64) string { return strconv.FormatFloat(x, 'f', -1, 64) }
