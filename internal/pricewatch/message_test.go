package pricewatch

// Context Checkpoint: done_criteria → test mapping (TASK-005)
// functional[0]     spec §8 示例逐字相等                         → TestFormatAlert_Golden
// functional[1]     Advice=nil：「AI 分析不可用」且无【AI 分析】  → TestFormatAlert_NoAI
// functional[2]     图标 🔺/🔀、rise 行文案；两条 Advice 分行   → TestFormatAlert_Icons、TestFormatAlert_TwoSides
// functional[3]     ShortDays=100 追加、=0 不追加               → TestFormatAlert_ShortWindow
// functional[4]     心跳与健康消息关键字                         → TestFormatHeartbeat、TestFormatHealth
// boundary[0]       价格/百分比格式、−0.0、档位写法              → TestFormatPrice、TestFormatPct、TestFormatTier
// error_handling[0] UTF-16 ≤4096、触发规则完整、「…（已截断）」  → TestFormatAlert_Truncate、TestFormatAlert_NoTruncateNormal
// QA-6（AD-31）     整条兜底 ≤4096：恰 4096 不截 / 4097 截；AI 结论字段超长；非 AI 部分超长 → TestFormatAlert_Limit*
// QA iter2 N1       健康消息 detail 超长 ⇒ ≤4096 且标题保留；正常长度不截 → TestFormatHealth_Limit

import (
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

func msgRule(id string, typ RuleType, w Window) Rule {
	return Rule{ID: id, Type: typ, Window: w, ThresholdPct: 2, StepPct: 2,
		Notify: NotifyPolicy{RepeatInterval: time.Hour, MaxPerTier: 3}}
}

func goldenAlert() Alert {
	return Alert{
		Asset:  Asset{Symbol: "BTC", Name: "比特币", Pair: "BTCUSDT", Quote: "USDT"},
		Price:  68120,
		Source: "okx",
		Triggers: []Trigger{
			{Rule: msgRule("day_drop", DropFromHigh, Window24h), X: 4.1, Tier: 2, SentInTier: 1, Ref: 71040},
			{Rule: msgRule("week_drop", DropFromHigh, Window7d), X: 5.8, Tier: 1, SentInTier: 2, Ref: 72300},
		},
		Overview: Overview{Chg24h: -3.6, Chg7d: -5.1, Chg30d: 2.4, FromATH: -31.2, From4yLow: 312},
		Advice: []SideAdvice{{Side: "低买", Verdict: "再等等", TargetPrice: 66500, Term: "3 天",
			Reasoning: "近 30 分钟仍在创新低、5 分钟线未见止跌"}},
	}
}

func TestFormatAlert_Golden(t *testing.T) {
	want := `🔻 BTC 68,120 USDT（okx）

【AI 结论】低买：再等等 · 建议目标价 66,500 · 期限 3 天

【触发规则】
· 1天窗口：较 24h 最高价 71,040 下跌 4.1%（第 2 档）
· 1周窗口：较 7d 最高价 72,300 下跌 5.8%（第 1 档，第 2/3 次提醒）

【多周期】24h −3.6% · 7d −5.1% · 30d +2.4% · 距历史最高 −31.2% · 距 4 年最低 +312%

【AI 分析】近 30 分钟仍在创新低、5 分钟线未见止跌

AI 建议仅供参考`
	if got := FormatAlert(goldenAlert()); got != want {
		t.Fatalf("golden 不一致\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatAlert_NoAI(t *testing.T) {
	for name, adv := range map[string][]SideAdvice{"nil": nil, "空切片": {}} {
		t.Run(name, func(t *testing.T) {
			a := goldenAlert()
			a.Advice = adv
			got := FormatAlert(a)
			if !strings.Contains(got, "\n【AI 结论】AI 分析不可用\n") {
				t.Fatalf("缺少独立的「AI 分析不可用」结论行:\n%s", got)
			}
			if strings.Contains(got, "【AI 分析】") {
				t.Fatalf("AI 不可用时不应出现【AI 分析】段:\n%s", got)
			}
			if !strings.Contains(got, "· 1周窗口：较 7d 最高价 72,300 下跌 5.8%（第 1 档，第 2/3 次提醒）") {
				t.Fatalf("触发规则应照常输出:\n%s", got)
			}
		})
	}
}

func TestFormatAlert_Icons(t *testing.T) {
	rise := Trigger{Rule: msgRule("week_rise", RiseFromLow, Window7d), X: 6.26, Tier: 1, SentInTier: 1, Ref: 64000}
	drop := goldenAlert().Triggers[0]
	cases := []struct {
		name     string
		triggers []Trigger
		icon     string
	}{
		{"只有跌", []Trigger{drop}, "🔻 "},
		{"只有涨", []Trigger{rise}, "🔺 "},
		{"涨跌都有", []Trigger{drop, rise}, "🔀 "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := goldenAlert()
			a.Triggers = c.triggers
			got := FormatAlert(a)
			if !strings.HasPrefix(got, c.icon+"BTC ") {
				t.Fatalf("标题应以 %q 开头:\n%s", c.icon, got)
			}
		})
	}
	a := goldenAlert()
	a.Triggers = []Trigger{rise}
	if got := FormatAlert(a); !strings.Contains(got, "\n· 1周窗口：较 7d 最低价 64,000 上涨 6.3%（第 1 档）\n") {
		t.Fatalf("rise 规则行不符:\n%s", got)
	}
}

func TestFormatAlert_TwoSides(t *testing.T) {
	a := goldenAlert()
	a.Advice = []SideAdvice{
		{Side: "低买", Verdict: "再等等", TargetPrice: 66500, Term: "3 天", Reasoning: "跌势未止"},
		{Side: "高卖", Verdict: "现在下单", TargetPrice: 72000, Term: "1天", Reasoning: "上方压力明显"},
	}
	got := FormatAlert(a)
	for _, want := range []string{
		"\n【AI 结论】\n低买：再等等 · 建议目标价 66,500 · 期限 3 天\n高卖：现在下单 · 建议目标价 72,000 · 期限 1天\n",
		"\n【AI 分析】\n低买：跌势未止\n高卖：上方压力明显\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("缺少 %q:\n%s", want, got)
		}
	}
}

func TestFormatAlert_ShortWindow(t *testing.T) {
	a := goldenAlert()
	a.Triggers[0].ShortDays = 100
	got := FormatAlert(a)
	if !strings.Contains(got, "· 1天窗口：较 24h 最高价 71,040 下跌 4.1%（第 2 档）（窗口数据仅 100 天）\n") {
		t.Fatalf("ShortDays=100 应在行尾追加:\n%s", got)
	}
	if strings.Count(got, "窗口数据仅") != 1 {
		t.Fatalf("ShortDays=0 的规则行不应追加:\n%s", got)
	}
}

func TestFormatAlert_WindowNames(t *testing.T) {
	cases := []struct {
		w    Window
		want string
	}{
		{Window24h, "· 1天窗口：较 24h 最高价"},
		{Window7d, "· 1周窗口：较 7d 最高价"},
		{Window30d, "· 30天窗口：较 30d 最高价"},
		{Window4y, "· 4年窗口：较 4 年最高价"},
		{WindowAll, "· 历史窗口：较历史最高价"},
	}
	for _, c := range cases {
		t.Run(string(c.w), func(t *testing.T) {
			a := goldenAlert()
			a.Triggers = []Trigger{{Rule: msgRule("r", DropFromHigh, c.w), X: 50, Tier: 1, SentInTier: 1, Ref: 100000}}
			if got := FormatAlert(a); !strings.Contains(got, "\n"+c.want+" 100,000 下跌 50.0%") {
				t.Fatalf("缺少 %q:\n%s", c.want, got)
			}
		})
	}
}

func TestFormatPrice(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{68120, "68,120"},
		{1234567.8, "1,234,568"},
		{1000, "1,000"},
		{999.5, "999.50"},
		{2.5, "2.50"},
		{1, "1.00"},
		{0.123456, "0.1235"},
	}
	for _, c := range cases {
		if got := formatPrice(c.in); got != c.want {
			t.Errorf("formatPrice(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatPct(t *testing.T) {
	cases := []struct {
		in       float64
		decimals int
		want     string
	}{
		{-3.6, 1, "−3.6%"},
		{2.4, 1, "+2.4%"},
		{-0.04, 1, "0.0%"},
		{0.04, 1, "0.0%"},
		{0, 1, "0.0%"},
		{312, 0, "+312%"},
		{-0.4, 0, "0%"},
		{-31.26, 1, "−31.3%"},
	}
	for _, c := range cases {
		got := formatPct(c.in, c.decimals)
		if got != c.want {
			t.Errorf("formatPct(%v,%d) = %q, want %q", c.in, c.decimals, got, c.want)
		}
		if strings.Contains(got, "-") {
			t.Errorf("formatPct(%v,%d) = %q 含 ASCII 连字符，负号应为 U+2212", c.in, c.decimals, got)
		}
	}
}

func TestFormatTier(t *testing.T) {
	r := msgRule("r", DropFromHigh, Window24h)
	cases := []struct {
		tier, sent int
		want       string
	}{
		{2, 1, "（第 2 档）"},
		{1, 2, "（第 1 档，第 2/3 次提醒）"},
		{3, 3, "（第 3 档，第 3/3 次提醒）"},
	}
	for _, c := range cases {
		a := goldenAlert()
		a.Triggers = []Trigger{{Rule: r, X: 5, Tier: c.tier, SentInTier: c.sent, Ref: 100}}
		if got := FormatAlert(a); !strings.Contains(got, "下跌 5.0%"+c.want+"\n") {
			t.Errorf("tier=%d sent=%d 缺少 %q:\n%s", c.tier, c.sent, c.want, got)
		}
	}
}

func encodedUTF16Len(s string) int { return len(utf16.Encode([]rune(s))) }

func TestFormatAlert_Truncate(t *testing.T) {
	a := goldenAlert()
	a.Advice[0].Reasoning = strings.Repeat("跌", 10000)
	got := FormatAlert(a)
	if n := encodedUTF16Len(got); n > maxMessageUTF16 || maxMessageUTF16 != 4096 {
		t.Fatalf("UTF-16 长度 %d 超过上限 %d（应为 4096）", n, maxMessageUTF16)
	}
	full := FormatAlert(goldenAlert())
	trig := full[strings.Index(full, "【触发规则】"):strings.Index(full, "【AI 分析】")]
	if !strings.Contains(got, trig) {
		t.Fatalf("【触发规则】段应完整保留:\n%s", trig)
	}
	head, tail, ok := strings.Cut(got, "【AI 分析】")
	if !ok || !strings.HasPrefix(full, head) {
		t.Fatalf("【AI 分析】之前的内容应与未截断时一致")
	}
	if !strings.HasSuffix(tail, "跌…（已截断）\n\nAI 建议仅供参考") {
		t.Fatalf("截断处应以「…（已截断）」结尾，尾部: %q", tail[len(tail)-60:])
	}
	// 「跌」各占 1 个码元，截断应恰好用满预算
	if n := encodedUTF16Len(got); n != maxMessageUTF16 {
		t.Fatalf("截断未用满预算：UTF-16 长度 %d，预算 %d", n, maxMessageUTF16)
	}
}

func TestFormatAlert_TruncateSurrogatePairs(t *testing.T) {
	// 每个 emoji 占 2 个 UTF-16 码元，截断不得劈开代理对、结果仍 ≤ 4096
	a := goldenAlert()
	a.Advice[0].Reasoning = strings.Repeat("📉", 5000)
	got := FormatAlert(a)
	if n := encodedUTF16Len(got); n > maxMessageUTF16 {
		t.Fatalf("UTF-16 长度 %d 超限", n)
	}
	if !strings.Contains(got, "📉…（已截断）") {
		t.Fatalf("截断标记缺失")
	}
}

func TestFormatAlert_NoTruncateNormal(t *testing.T) {
	a := goldenAlert()
	a.Advice[0].Reasoning = strings.Repeat("稳", 1000)
	got := FormatAlert(a)
	if strings.Contains(got, "已截断") || !strings.Contains(got, a.Advice[0].Reasoning) {
		t.Fatalf("正常长度消息不应被截断")
	}
}

func TestFormatHeartbeat(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 10, 20, 9, 0, 0, 0, loc)
	rows := []HeartbeatRow{
		{Asset: Asset{Symbol: "BTC", Name: "比特币", Quote: "USDT"}, Price: 68120,
			LastOK: time.Date(2026, 10, 20, 0, 58, 0, 0, time.UTC), Active: []string{"day_drop 第2档", "week_rise 第1档"}},
		{Asset: Asset{Symbol: "ETH", Name: "以太坊", Quote: "USDT"}, Price: 2512.6},
	}
	got := FormatHeartbeat(now, rows, 7)
	for _, want := range []string{
		"监控仍在运行（2026-10-20 09:00）",
		"· 比特币 BTC 68,120 USDT · 最近成功 08:58 · 触发中：day_drop 第2档、week_rise 第1档",
		"· 以太坊 ETH 2,513 USDT · 最近成功 无 · 触发中：无",
		"过去 24 小时发出提醒 7 条",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("缺少 %q:\n%s", want, got)
		}
	}
}

func TestFormatHealth(t *testing.T) {
	down := FormatHealth(true, "price:BTC", "okx: timeout")
	up := FormatHealth(false, "price:BTC", "")
	for _, c := range []struct {
		got, want, not string
	}{
		{down, "⚠️ 监控异常：price:BTC\nokx: timeout", "已恢复"},
		{up, "✅ 已恢复：price:BTC", "监控异常"},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
		if strings.Contains(c.got, c.not) {
			t.Errorf("%q 不应含 %q", c.got, c.not)
		}
	}
}

func TestFormatAlert_AdviceOptionalFields(t *testing.T) {
	a := goldenAlert()
	a.Advice[0].TargetPrice = 0
	a.Advice[0].Term = ""
	if got := FormatAlert(a); !strings.Contains(got, "\n【AI 结论】低买：再等等\n") {
		t.Fatalf("目标价为 0、期限为空时应省略这两项:\n%s", got)
	}
}

// alertWithReasoningLen 返回整条消息 UTF-16 长度恰为 total 的 Alert（reasoning 用 1 码元的汉字填充）。
func alertWithReasoningLen(t *testing.T, total int) Alert {
	t.Helper()
	a := goldenAlert()
	a.Advice[0].Reasoning = ""
	base := encodedUTF16Len(FormatAlert(a))
	if total < base {
		t.Fatalf("目标长度 %d 小于空理由时的长度 %d", total, base)
	}
	a.Advice[0].Reasoning = strings.Repeat("稳", total-base)
	return a
}

func TestFormatAlert_LimitBoundary(t *testing.T) {
	a := alertWithReasoningLen(t, 4096)
	got := FormatAlert(a)
	if n := encodedUTF16Len(got); n != 4096 || strings.Contains(got, "已截断") || !strings.Contains(got, a.Advice[0].Reasoning) {
		t.Fatalf("恰 4096 码元不应截断：长度 %d", n)
	}
	a = alertWithReasoningLen(t, 4097)
	got = FormatAlert(a)
	if n := encodedUTF16Len(got); n != 4096 || !strings.Contains(got, "稳…（已截断）\n\nAI 建议仅供参考") {
		t.Fatalf("4097 码元应截断到恰 4096：长度 %d，尾部 %q", n, got[len(got)-80:])
	}
}

// AI 结论字段（Term/Verdict/Side）超长时，先截【AI 分析】，仍超则截【AI 结论】，【触发规则】完整保留。
func TestFormatAlert_LimitLongAdviceFields(t *testing.T) {
	full := FormatAlert(goldenAlert())
	trig := full[strings.Index(full, "【触发规则】"):strings.Index(full, "\n\n【多周期】")]
	for name, mutate := range map[string]func(*SideAdvice){
		"Term":    func(s *SideAdvice) { s.Term = strings.Repeat("天", 5000) },
		"Verdict": func(s *SideAdvice) { s.Verdict = strings.Repeat("等", 5000) },
		"Side":    func(s *SideAdvice) { s.Side = strings.Repeat("买", 5000) },
		"Term+长理由": func(s *SideAdvice) {
			s.Term = strings.Repeat("天", 3000)
			s.Reasoning = strings.Repeat("跌", 3000)
		},
	} {
		t.Run(name, func(t *testing.T) {
			a := goldenAlert()
			mutate(&a.Advice[0])
			got := FormatAlert(a)
			if n := encodedUTF16Len(got); n > maxMessageUTF16 {
				t.Fatalf("UTF-16 长度 %d 超过 %d", n, maxMessageUTF16)
			}
			if !strings.Contains(got, trig) || !strings.Contains(got, "【多周期】") {
				t.Fatalf("【触发规则】与【多周期】应完整保留:\n%s", got[:200])
			}
			if !strings.Contains(got, "\n\n【AI 结论】") || !strings.Contains(got, "…（已截断）") {
				t.Fatalf("应保留【AI 结论】段并带截断标记")
			}
			if !strings.HasSuffix(got, "\n\nAI 建议仅供参考") {
				t.Fatalf("末行免责声明应保留")
			}
		})
	}
	// 两个方向的结论都超长
	a := goldenAlert()
	a.Advice = []SideAdvice{
		{Side: "低买", Verdict: "再等等", Term: strings.Repeat("天", 3000), Reasoning: "r"},
		{Side: "高卖", Verdict: "现在下单", Term: strings.Repeat("天", 3000), Reasoning: "r"},
	}
	if n := encodedUTF16Len(FormatAlert(a)); n > maxMessageUTF16 {
		t.Fatalf("两条超长结论：UTF-16 长度 %d 超限", n)
	}
}

// 非 AI 部分（如触发规则极多）本身超限时，整条消息兜底截断，有无 AI 都 ≤4096。
func TestFormatAlert_LimitWholeMessage(t *testing.T) {
	for _, advice := range [][]SideAdvice{nil, goldenAlert().Advice} {
		a := goldenAlert()
		a.Advice = advice
		for i := 0; i < 200; i++ {
			a.Triggers = append(a.Triggers, a.Triggers[0])
		}
		got := FormatAlert(a)
		if n := encodedUTF16Len(got); n > maxMessageUTF16 {
			t.Fatalf("advice=%v：UTF-16 长度 %d 超限", advice != nil, n)
		}
		if !strings.HasPrefix(got, "🔻 BTC 68,120 USDT（okx）") || !strings.HasSuffix(got, "…（已截断）") {
			t.Fatalf("整条截断应保留开头并以截断标记结尾，尾部 %q", got[len(got)-40:])
		}
	}
}

// N1：sync 多段失败的 errors.Join 可远超 4096；健康消息须截断且保留标题行。
func TestFormatHealth_Limit(t *testing.T) {
	detail := strings.Repeat("同步失败📉", 2000) // 中文 + 代理对，约 1.2 万码元
	for _, c := range []struct {
		down  bool
		title string
	}{
		{true, "⚠️ 监控异常：BTC 1d K 线同步\n"},
		{false, "✅ 已恢复：BTC 1d K 线同步\n"},
	} {
		got := FormatHealth(c.down, "BTC 1d K 线同步", detail)
		if n := encodedUTF16Len(got); n > maxMessageUTF16 || n < maxMessageUTF16-2 {
			t.Fatalf("down=%v：UTF-16 长度 %d，应截到 %d 以内且接近上限", c.down, n, maxMessageUTF16)
		}
		if !strings.HasPrefix(got, c.title) || !strings.HasSuffix(got, truncatedMark) {
			t.Fatalf("down=%v：应以标题开头、以截断标记结尾：%q…%q", c.down, got[:60], got[len(got)-40:])
		}
		if !utf8.ValidString(got) {
			t.Fatalf("down=%v：截断劈开了字符", c.down)
		}
	}
	normal := strings.Repeat("稳", 4000)
	if got := FormatHealth(true, "k", normal); got != "⚠️ 监控异常：k\n"+normal {
		t.Fatalf("正常长度不应截断（长度 %d）", encodedUTF16Len(got))
	}
}
