package bank

// Context Checkpoint: done_criteria → test mapping（TASK-005）
// functional[0]  段落与行按序出现                         → TestRenderSections, TestRenderAliasesAndMissingFieldsOutsideCurrent
// functional[1]  golden 逐字节比对（-update 重生成）       → TestRenderGolden
// functional[2]  预警行方向/精度/期次标注                  → TestRenderSections, TestRenderDeteriorationAlert
// functional[3]  头部「领先 N」与分项和 / 统计行「（n=K）」   → TestRenderSections, TestRenderHeaderSumsToTotal, TestRenderStatN
// boundary[0]    无预警 / 无当期 / NaN / 全文无 NaN、Inf    → TestRenderNoAlertsNoCurrent, TestRenderNaNShowsNA, TestRenderGolden
// boundary[1]    Split 精确边界 / 超长单行 / 行序与空行性质 / 空串 → TestSplit*（性质见 assertSplitProps）
// boundary[2]    Ahead 不进同期统计（Q27）                  → summary_test.go TestSummarizeAheadFallbackAtPeriodNotInStats
// non_functional vet / 全绿 / 覆盖率                       → go vet / go test -cover

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "重新生成 testdata 下的 golden 文件")

// renderSample 覆盖 golden 要求的全部形态：
// C 拨备 149.96（展示精度边缘）且环比 -20.04；B 的 CET1 回退到 2026-03-31（截至 + 期次≠统计期的预警）；
// D 未更新且字段缺失；E 失败且配了 H 股别名；F 无数据；G 已披露更晚一期（Ahead）；A 带 H 股别名。
func renderSample() string {
	rs := sample() // summary_test.go：C/A/B 当期，D 未更新，G 领先，E 失败，F 无数据
	for i := range rs {
		switch rs[i].Name {
		case "A银行":
			rs[i].Symbol, rs[i].Aliases = "600036.SH", []string{"A银行H"}
			rs[i].Ind[IndNPL] = Change{0.94, -0.01, 0.01, day("2026-06-30")}
		case "B银行":
			rs[i] = withFallback(rs[i], IndCET1, 8.49, "2026-03-31")
		case "C银行":
			rs[i].Ind[IndCoverage] = Change{149.96, -20.04, nan, day("2026-06-30")}
		case "D银行":
			rs[i].MissingFields = []string{"HXYJBCZL"}
		case "E银行":
			rs[i].Symbol, rs[i].Aliases = "601658.SH", []string{"E银行H"}
		}
	}
	var alerts []Alert
	for _, r := range rs {
		if r.Err != nil {
			continue // 同 executeBankReport：失败主体不参与预警
		}
		alerts = append(alerts, Alerts(r, stdThresholds)...)
	}
	return Render(Summarize(rs, IndNPL), alerts, day("2026-10-01"))
}

func TestRenderSections(t *testing.T) {
	out := renderSample()
	want := []string{
		"🏦 银行关键指标月报 2026-10-01\n",
		"覆盖 7 家（统计期 2026-06-30：3 家；领先 1；未更新 2；失败 1）\n",
		"⚠️ 预警 (4)\n",
		"· C银行 不良率 1.62% > 1.50%\n",
		"· C银行 拨备覆盖率 149.96% < 150.00%\n",
		"· C银行 拨备覆盖率 环比 -20.04pp（超 20.00pp）\n",
		"· B银行 CET1 8.49% < 8.50%（2026-03-31）\n",
		"📊 同期统计 2026-06-30（n=3）\n",
		"不良率 均值 1.19% | 中位 1.00% | 最优 A银行 0.94% | 最差 C银行 1.62%\n",
		"拨备覆盖率 均值 249.96% | 中位 214.93% | 最优 A银行 385.00% | 最差 C银行 149.96%\n",
		"CET1（n=2） 均值 11.48% | 中位 11.48% | 最优 A银行 14.07% | 最差 C银行 8.90%\n",
		"🏷 排名（按不良率由优到劣）\n",
		"1. A银行 不良率 0.94%(环比-0.01/同比+0.01) 拨备覆盖率 385.00%(环比—/同比—) CET1 14.07%(环比—/同比—)\n",
		"  （A银行H 同 600036.SH）\n",
		"2. B银行 不良率 1.00%(环比—/同比—) 拨备覆盖率 214.93%(环比—/同比—) CET1 8.49%（截至 2026-03-31）(环比—/同比—)\n",
		"3. C银行 不良率 1.62%(环比—/同比—) 拨备覆盖率 149.96%(环比-20.04/同比—) CET1 8.90%(环比—/同比—)\n",
		"ℹ️ 未更新/失败\n",
		"· D银行 最新 2026-03-31（未披露 2026-06-30）\n",
		"· F银行 无可用数据\n",
		"· E银行 拉取失败：timeout\n",
		"  （E银行H 同 601658.SH）\n",
		"· D银行 字段缺失：HXYJBCZL（疑似数据源结构变化）\n",
		"· G银行 已披露 2026-09-30（晚于统计期）\n",
	}
	pos := 0
	for _, w := range want {
		i := strings.Index(out[pos:], w)
		if !assert.GreaterOrEqual(t, i, 0, "缺失或乱序：%q", w) {
			continue
		}
		pos += i + len(w)
	}
	assert.NotContains(t, out, "G银行 不良率", "Ahead 主体不进排名")
}

func TestRenderGolden(t *testing.T) {
	out := renderSample()
	golden := filepath.Join("testdata", "render_golden.txt")
	if *update {
		require.NoError(t, os.WriteFile(golden, []byte(out), 0o644))
	}
	want, err := os.ReadFile(golden)
	require.NoError(t, err, "golden 缺失时用 go test -run TestRenderGolden -update 生成，并人工核对")
	assert.Equal(t, string(want), out)
	for _, bad := range []string{"NaN", "Inf"} {
		assert.NotContains(t, out, bad)
	}
}

// DoD ⑤：别名在 Stale（含无数据）/Ahead 主体上同样要出现（Failed 由 golden 覆盖）；Ahead 的字段缺失也要报。
func TestRenderAliasesAndMissingFieldsOutsideCurrent(t *testing.T) {
	stale := br("D银行", "2026-03-31", 1, 200, 10)
	stale.Symbol, stale.Aliases = "600001.SH", []string{"D银行H"}
	empty := br("F银行", "", nan, nan, nan)
	empty.Symbol, empty.Aliases = "600002.SH", []string{"F银行H"}
	ahead := br("G银行", "2026-09-30", 1, 200, 10)
	ahead.Symbol, ahead.Aliases, ahead.MissingFields = "600003.SH", []string{"G银行H"}, []string{"BLDKBBL"}
	rs := []BankResult{br("A银行", "2026-06-30", 1, 200, 10), br("B银行", "2026-06-30", 1, 200, 10), empty, stale, ahead}
	s := Summarize(rs, IndNPL)
	require.Equal(t, []string{"G银行"}, names(s.Ahead), "前提：G 是 Ahead")
	out := Render(s, nil, day("2026-10-01"))
	for _, w := range []string{
		"· D银行 最新 2026-03-31（未披露 2026-06-30）\n  （D银行H 同 600001.SH）\n",
		"· F银行 无可用数据\n  （F银行H 同 600002.SH）\n",
		"· G银行 字段缺失：BLDKBBL（疑似数据源结构变化）\n",
		"· G银行 已披露 2026-09-30（晚于统计期）\n  （G银行H 同 600003.SH）\n",
	} {
		assert.Contains(t, out, w)
	}
	// 配置里无数据的 F 排在 D 之前，信息区仍须「未更新」在前、「无可用数据」在后。
	assert.Less(t, strings.Index(out, "· D银行 最新"), strings.Index(out, "· F银行 无可用数据"))
}

var headerRe = regexp.MustCompile(`覆盖 (\d+) 家（统计期 [^：]+：(\d+) 家(?:；领先 (\d+))?；未更新 (\d+)；失败 (\d+)）`)

// headerParts 解析头部，返回覆盖家数与各分项之和。
func headerParts(t *testing.T, out string) (total, sum int, hasAhead bool) {
	t.Helper()
	m := headerRe.FindStringSubmatch(out)
	require.NotNil(t, m, "头部格式不符：%q", strings.SplitN(out, "\n", 3)[1])
	atoi := func(s string) int {
		if s == "" {
			return 0
		}
		n, err := strconv.Atoi(s)
		require.NoError(t, err)
		return n
	}
	for _, g := range m[2:] {
		sum += atoi(g)
	}
	return atoi(m[1]), sum, m[3] != ""
}

func TestRenderHeaderSumsToTotal(t *testing.T) {
	total, sum, hasAhead := headerParts(t, renderSample())
	assert.True(t, hasAhead, "含 Ahead 主体时须列「领先 N」")
	assert.Equal(t, total, sum, "golden 样本：分项和 = 覆盖家数")

	rs := []BankResult{br("A银行", "2026-06-30", 1, 200, 10), br("B银行", "2026-06-30", 1, 200, 10), br("G银行", "2026-09-30", 1, 200, 10)}
	total, sum, hasAhead = headerParts(t, Render(Summarize(rs, IndNPL), nil, day("2026-10-01")))
	assert.True(t, hasAhead)
	assert.Equal(t, 3, total)
	assert.Equal(t, total, sum)

	noAhead := Render(Summarize(sample()[:3], IndNPL), nil, day("2026-10-01"))
	assert.Contains(t, noAhead, "覆盖 3 家（统计期 2026-06-30：2 家；未更新 1；失败 0）\n", "无 Ahead 时头部字符串不变")
	total, sum, hasAhead = headerParts(t, noAhead)
	assert.False(t, hasAhead)
	assert.Equal(t, total, sum)
}

func TestRenderStatN(t *testing.T) {
	out := renderSample()
	assert.Contains(t, out, "\n不良率 均值 ", "N 等于当期主体数时不标 n")
	assert.Contains(t, out, "\n拨备覆盖率 均值 ")
	assert.Contains(t, out, "\nCET1（n=2） 均值 ", "B 的 CET1 是回退值不计入，N=2 ≠ 3")
	assert.NotContains(t, out, "（n=3） 均值", "统计行不标与当期主体数相等的 n（标题里的 n=3 除外）")
}

func TestRenderDeteriorationAlert(t *testing.T) {
	out := Render(Summary{Period: day("2026-06-30")}, []Alert{
		{"X银行", IndCET1, AlertDeterioration, -0.62, 0.5, day("2026-06-30")},
		{"Y银行", IndNPL, AlertDeterioration, 0.11, 0.1, day("2026-03-31")},
	}, day("2026-10-01"))
	assert.Contains(t, out, "· X银行 CET1 环比 -0.62pp（超 0.50pp）\n")
	assert.Contains(t, out, "· Y银行 不良率 环比 +0.11pp（超 0.10pp）（2026-03-31）\n")
}

func TestRenderNoAlertsNoCurrent(t *testing.T) {
	out := Render(Summarize([]BankResult{br("D银行", "", nan, nan, nan)}, IndNPL), nil, day("2026-10-01"))
	assert.Contains(t, out, "⚠️ 预警 (0)\n无\n")
	assert.Contains(t, out, "统计期 —")
	assert.NotContains(t, out, "📊", "无当期主体时不出统计段")
	assert.NotContains(t, out, "🏷", "无当期主体时不出排名段")
}

func TestRenderNaNShowsNA(t *testing.T) {
	rs := []BankResult{br("A银行", "2026-06-30", 0.94, 385, nan)}
	out := Render(Summarize(rs, IndNPL), nil, day("2026-10-01"))
	assert.Contains(t, out, "CET1 N/A(环比—/同比—)")
	assert.Contains(t, out, "CET1 无数据")
	for _, bad := range []string{"NaN", "Inf"} {
		assert.NotContains(t, out, bad)
	}
}

func TestSplitRespectsLimitAndLines(t *testing.T) {
	var lines []string
	for i := range 300 {
		lines = append(lines, fmt.Sprintf("%d. 某某农村商业银行 不良率 1.23%%(环比+0.01/同比-0.02)", i))
	}
	text := strings.Join(lines, "\n") + "\n"
	chunks := Split(text, MaxMessageRunes)
	require.Greater(t, len(chunks), 1)
	assertSplitProps(t, text, MaxMessageRunes, chunks)
}

// assertSplitProps 断言 Split 的全部性质：每段非空、≤ limit（超长单行除外）、不以空行开头或结尾；
// 各段按序是原文在行边界处的连续片段，段与段之间只丢换行。由此每个非空行按原顺序恰好出现一次，
// 段内空行原样保留，只有块边界处的空行被丢弃（对 Telegram 推送是期望行为）。
func assertSplitProps(t *testing.T, text string, limit int, chunks []string) {
	t.Helper()
	pos := 0
	for i, c := range chunks {
		require.NotEmpty(t, c, "limit=%d 第 %d 段", limit, i)
		assert.False(t, strings.HasPrefix(c, "\n") || strings.HasSuffix(c, "\n"), "limit=%d 第 %d 段首尾有空行：%q", limit, i, c)
		if utf8.RuneCountInString(c) > limit {
			assert.NotContains(t, c, "\n", "limit=%d 第 %d 段超长却不是单行", limit, i)
		}
		at := strings.Index(text[pos:], c)
		require.GreaterOrEqual(t, at, 0, "limit=%d 第 %d 段不是原文的有序片段：%q", limit, i, c)
		start := pos + at
		require.Empty(t, strings.Trim(text[pos:start], "\n"), "limit=%d 第 %d 段前丢了非换行内容", limit, i)
		require.True(t, start == 0 || text[start-1] == '\n', "limit=%d 第 %d 段不在行首", limit, i)
		pos = start + len(c)
		require.True(t, pos == len(text) || text[pos] == '\n', "limit=%d 第 %d 段不在行尾", limit, i)
	}
	assert.Empty(t, strings.Trim(text[pos:], "\n"), "limit=%d 末段之后还有内容未输出", limit)
}

func TestSplitBlankLines(t *testing.T) {
	long := strings.Repeat("长", 25)
	cases := []struct {
		text  string
		limit int
		want  []string // nil 表示只断言性质
		msg   string
	}{
		{"a\n\nb", 3, []string{"a", "b"}, "块边界处的空行丢弃"},
		{"a\n\nb", 4, []string{"a\n\nb"}, "段内空行保留"},
		{"\n\na\n\n", 5, []string{"a"}, "原文首尾空行不成段首尾"},
		{long + "\n\n尾\n", 10, []string{long, "尾"}, "超长行之后的空行不成段首"},
		{"a\n\n\nb\nc", 2, nil, ""},
	}
	for _, c := range cases {
		chunks := Split(c.text, c.limit)
		if c.want != nil {
			assert.Equal(t, c.want, chunks, c.msg)
		}
		assertSplitProps(t, c.text, c.limit, chunks)
	}
}

// renderSample 的每个段落标题前都有空行；遍历全部 limit，覆盖块边界落在任意行（含空行）的情形。
func TestSplitRenderSampleAllLimits(t *testing.T) {
	text := renderSample()
	multi := 0
	for limit := 1; limit <= utf8.RuneCountInString(text)+1; limit++ {
		chunks := Split(text, limit)
		if len(chunks) > 1 {
			multi++
		}
		assertSplitProps(t, text, limit, chunks)
	}
	assert.Greater(t, multi, 0)
}

func TestSplitExactBoundary(t *testing.T) {
	// "一二三\n四五六" 共 7 个字符（换行计 1）。
	text := "一二三\n四五六\n"
	assert.Equal(t, []string{"一二三\n四五六"}, Split(text, 7), "恰好等于 limit 不切")
	assert.Equal(t, []string{"一二三", "四五六"}, Split(text, 6), "超出 1 个字符必切")
}

func TestSplitOversizedLineStandsAlone(t *testing.T) {
	chunks := Split("短\n"+strings.Repeat("长", 25)+"\n尾\n", 10)
	assert.Equal(t, []string{"短", strings.Repeat("长", 25), "尾"}, chunks, "超长单行独占一段，不在行内截断")
}

func TestSplitShortAndEmpty(t *testing.T) {
	assert.Equal(t, []string{"a\nb"}, Split("a\nb\n", 100))
	assert.Empty(t, Split("", 100))
}
