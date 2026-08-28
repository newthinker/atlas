package hestia

// Context Checkpoint: done_criteria → test mapping (TASK-005 profiles)
// functional[1]  名称捕获禁用贪婪 `(.+)`（判据 A）        → TestNoGreedyCaptureInTemplates
// functional[1]  交替片段两两互不为前缀（判据 B 的本地份） → TestProfileAlternationsHaveNoPrefixPairs
// functional[2]  模板复用 T4 的 directionAlt / unitAlt    → TestTemplatesReuseAmountFragments
// functional[3]  2025 写「加权均利率」缺「平」字          → TestRateRegexpsToleratePingTypo
// functional[3]  容错不得放松成 `加权.*利率为`（会取到 1.36）→ TestRateRegexpsRejectLoosePatterns
// boundary[0]    本外币/外币/人民币 三者都被匹配并按限定词分类
//                                                        → TestBalanceRegexpClassifiesCurrencyQualifier
// boundary[0]①   期次前缀切开 YTD 与月度孪生句            → TestFlowRegexpClassifiesPeriodPrefix
// non_functional[0] 清单是数据：覆盖度 / 无重复 / 字段名合法
//                                                        → TestTemplateTablesCoverAllFields、
//                                                          TestTemplateTablesHaveNoDuplicateFields、
//                                                          TestTemplateFieldsAreDeclaredInFieldsGo
// functional[*]  每条生成的正则捕获组数与真实句子命中      → TestGeneratedRegexpsAreWellFormed

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// templateFields 汇总所有模板表声明的字段，供覆盖度与重复检查共用。
func templateFields() []string {
	var out []string
	for _, it := range tsfStockItems {
		out = append(out, it.balanceField, it.yoyField)
	}
	for _, it := range tsfFlowItems {
		out = append(out, it.field)
	}
	for _, it := range moneyItems {
		out = append(out, it.balanceField, it.yoyField)
	}
	for _, it := range depositItems {
		out = append(out, it.field)
	}
	for _, sc := range loanScopes {
		if sc.totalField != "" {
			out = append(out, sc.totalField)
		}
		for _, it := range sc.items {
			out = append(out, it.field)
		}
	}
	return append(out, singletonFields()...)
}

// TestTemplateTablesCoverAllFields 是本任务与 fields.go 之间的接缝守卫。
//
// 新增一个字段常量却忘了给它写模板，解析结果就会永远缺那一项——而缺失在
// M1b-1 的语义里是「本期模板本就没有」，completeness 若也漏配就完全无声。
func TestTemplateTablesCoverAllFields(t *testing.T) {
	covered := map[string]bool{}
	for _, f := range templateFields() {
		covered[f] = true
	}
	var missing []string
	for _, f := range fieldOrder {
		if !covered[f] {
			missing = append(missing, f)
		}
	}
	assert.Emptyf(t, missing, "这些字段没有任何模板负责抽取：%v", missing)
	assert.Equal(t, len(fieldOrder), len(covered), "模板覆盖的字段数应等于 fieldOrder")
}

func TestTemplateTablesHaveNoDuplicateFields(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range templateFields() {
		require.Falsef(t, seen[f],
			"字段 %s 被两张模板表同时声明——运行时会触发重复赋值错误", f)
		seen[f] = true
	}
}

// TestTemplateFieldsAreDeclaredInFieldsGo 钉住「字段名合法」这一半。
//
// 覆盖度测试只保证 fieldOrder ⊆ 模板；反向若模板声明了一个 fieldOrder 里没有的
// 名字（比如手滑打错常量），覆盖度测试的 len 相等那条会红，但错在哪不明显。
func TestTemplateFieldsAreDeclaredInFieldsGo(t *testing.T) {
	known := map[string]bool{}
	for _, f := range fieldOrder {
		known[f] = true
	}
	for _, f := range templateFields() {
		assert.Truef(t, known[f], "模板声明了 fieldOrder 里不存在的字段 %q", f)
	}
}

// TestNoGreedyCaptureInTemplates 是 DoD functional[1] 的判据 A。
//
// dev-agent-45 在 T4 实测证明：真正会让「企业债券净融资2.39万亿元」被切成
// name=`企业债券净` 的，是**贪婪的名称捕获** `^(.+)(dir)…`——引擎优先让名称吃掉
// 「净」、再用「融资」收尾。与交替顺序无关，两种顺序下都发生（见
// TestGreedyNameCaptureSwallowsNetPrefix）。
//
// 本包的做法是名称一律 QuoteMeta 固定，压根不做名称捕获。这条测试守住它：
// 既扫每条已生成正则的源串，也扫 profiles.go 里**每一个字符串字面量**，
// 让将来新增的模板也被覆盖。
//
// 源文件那一半走 go/parser 只取字面量，而不是整文件子串匹配——注释里为了讲清
// 失效模式必须写出 `(.+)` 这个形态（上面第二段就写了），整文件扫描会把说明文字
// 判成违规。恒响的检查会被训练成忽略，那比没有检查更糟。
func TestNoGreedyCaptureInTemplates(t *testing.T) {
	greedy := []string{`(.+)`, `(.*)`}

	for _, re := range allTemplateRegexps() {
		for _, g := range greedy {
			assert.NotContainsf(t, re.String(), g,
				"模板 %s 含贪婪捕获 %s —— 名称必须 QuoteMeta 固定或用非贪婪 (.+?)", re, g)
		}
	}

	lits := stringLiteralsOf(t, "profiles.go")
	require.NotEmpty(t, lits, "没解析出任何字符串字面量，本测试的绿色是假的")
	for _, lit := range lits {
		for _, g := range greedy {
			assert.NotContainsf(t, lit.text, g,
				"%s: 字面量 %s 含贪婪捕获 %s", lit.pos, lit.text, g)
		}
	}
}

type sourceLiteral struct {
	pos  string
	text string
}

// stringLiteralsOf 取出一个 Go 源文件里全部字符串字面量（含反引号原始串）。
//
// 用它而非整文件子串匹配，是为了让「源码里不许出现某形态」这类检查只作用于
// **代码**，不误伤注释与文档。
func stringLiteralsOf(t *testing.T, filename string) []sourceLiteral {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, nil, 0)
	require.NoError(t, err)

	var out []sourceLiteral
	ast.Inspect(f, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			out = append(out, sourceLiteral{pos: fset.Position(lit.Pos()).String(), text: lit.Value})
		}
		return true
	})
	return out
}

// TestProfileAlternationsHaveNoPrefixPairs 把 T4 的前缀不变量延伸到本包新增的
// 两个交替片段上。
//
// 判据 B 的原件 TestNoAlternativeIsPrefixOfAnother 只看 directionAlt / unitAlt；
// 本包新增了 currencyAlt 与 periodAlt，同一条不变量必须同样成立——否则
// 「顺序即语义」会在这两个片段上悄悄开始生效。
//
// # 判据保持 prefix，**没有**扩成 substring —— 这是实测后的决定（M1b-4b / TASK-004）
//
// 独立 reviewer 的 C2 建议把本条从 prefix 扩到 substring，用来防季报的「写成
// `三季度` 而非 `前三季度`」。**实测表明那个扩法既抓不到目标、又会误伤既有词表**，
// 故不采纳。四条实测（Go regexp，leftmost-first）：
//
//	A 前缀对，顺序有语义：  `人民|人民币` 对「人民币贷款」→ 捕获 "人民"
//	                        `人民币|人民` 对「人民币贷款」→ 捕获 "人民币"
//	B 子串但非前缀，顺序**无**语义：
//	                        `外币|本外币` 对「本外币存款」→ 捕获 "本外币"
//	                        `本外币|外币` 对「本外币存款」→ 捕获 "本外币"
//	C 真正的陷阱是**只写了短词、根本没有长词**：
//	                        `全年|上半年|三季度`   对「前三季度人民币贷款」→ 捕获 "三季度"
//	                        `全年|上半年|前三季度` 对「前三季度人民币贷款」→ 捕获 "前三季度"
//	D 而 C 里那个错**构不成任何子串对**（全年/上半年/三季度 两两无包含）
//	   ⇒ 子串判据对它**全绿放行**。
//
// 结论：
//   - **prefix 才是「顺序即语义」的真实边界**（A 有、B 无）——本条判据不动。
//   - 扩成 substring 会把 currencyAlt 既有的 `外币` ⊂ `本外币` 判成违规（实测转红），
//     而那一对**是安全的**（B）：那不是收紧，是误报。
//   - 陷阱 C 是**单个词写错**、不是词对关系，任何交替**内部**的两两比对都看不见它。
//     守住它的是语料锚定的正向断言 —— 见 TestFlowRECapturesQuarterlyPeriodVerbatim，
//     它拿真实前三季度正文断言捕获组**逐字等于** "前三季度"，写成 `三季度` 立刻红。
func TestProfileAlternationsHaveNoPrefixPairs(t *testing.T) {
	for _, tc := range []struct{ name, alt string }{
		{"currencyAlt", currencyAlt},
		{"periodAlt", periodAlt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			words := strings.Split(tc.alt, "|")
			for _, short := range words {
				for _, long := range words {
					if short == long {
						continue
					}
					assert.Falsef(t, strings.HasPrefix(long, short),
						"%s: %q 是 %q 的前缀 —— 交替顺序从此有语义，长词须排前",
						tc.name, short, long)
				}
			}
		})
	}
}

// TestTemplatesReuseAmountFragments 覆盖 functional[2]：不得另写一份词表。
func TestTemplatesReuseAmountFragments(t *testing.T) {
	assert.Equal(t, `(`+directionAlt+`)`, dirPat, "dirPat 必须由 T4 的 directionAlt 拼出")
	assert.Equal(t, `(`+unitAlt+`)`, unitPat, "unitPat 必须由 T4 的 unitAlt 拼出")

	// 同样只看字面量：注释里引用方向词讲解句式是正常的，抄进代码才是分叉。
	for _, lit := range stringLiteralsOf(t, "profiles.go") {
		for _, w := range strings.Split(directionAlt, "|") {
			assert.NotEqualf(t, "`"+w+"`", lit.text,
				"%s: 方向词 %q 被抄成了独立字面量——词表只此一份，见 amount.go", lit.pos, w)
		}
	}
}

// TestGeneratedRegexpsAreWellFormed 用真实原文逐条验捕获组。
func TestGeneratedRegexpsAreWellFormed(t *testing.T) {
	t.Run("社融存量分项", func(t *testing.T) {
		re := tsfStockRE("委托贷款")
		assert.Equal(t, 4, re.NumSubexp())
		m := re.FindStringSubmatch("委托贷款余额为11.35万亿元，同比增长1.3%")
		require.Len(t, m, 5)
		assert.Equal(t, []string{"11.35", "万亿元", "增长", "1.3"}, m[1:])

		// 「余额为?」要同时吃得下带「为」与不带「为」两种写法
		m = tsfStockRE("政府债券").FindStringSubmatch("政府债券余额94.92万亿元，同比增长17.1%")
		require.Len(t, m, 5, "「余额」后不带「为」也应命中")
	})

	t.Run("社融存量分项不得命中占比句", func(t *testing.T) {
		// 2025 社融存量板块第二段：「委托贷款余额占比2.6%，同比低0.1个百分点」
		// 若被命中会得到一个占比数字冒充余额——量级完全不同，但字段是对的。
		got := tsfStockRE("委托贷款").FindString("委托贷款余额占比2.6%，同比低0.1个百分点")
		assert.Empty(t, got, "占比句没有单位，不得被余额模板命中")
	})

	t.Run("社融增量分项", func(t *testing.T) {
		re := tsfFlowRE("企业债券")
		assert.Equal(t, 3, re.NumSubexp())
		m := re.FindStringSubmatch("企业债券净融资2.39万亿元，同比多4825亿元")
		require.Len(t, m, 4)
		assert.Equal(t, []string{"净融资", "2.39", "万亿元"}, m[1:],
			"「净融资」须整体作为方向词，且不得取到同比干扰值 4825")

		m = tsfFlowRE("非金融企业境内股票").FindStringSubmatch("非金融企业境内股票融资4763亿元，同比多1863亿元")
		require.Len(t, m, 4)
		assert.Equal(t, []string{"融资", "4763", "亿元"}, m[1:])
	})

	t.Run("货币供应量", func(t *testing.T) {
		m := moneyRE("广义货币", "M2").FindStringSubmatch("广义货币(M2)余额340.29万亿元，同比增长8.5%")
		require.Len(t, m, 5)
		assert.Equal(t, "340.29", m[1])
		assert.Equal(t, "8.5", m[4])
	})

	t.Run("外汇两句", func(t *testing.T) {
		m := fxReserveRE.FindStringSubmatch("12月末，国家外汇储备余额为3.36万亿美元。")
		require.Len(t, m, 2)
		assert.Equal(t, "3.36", m[1])

		m = fxRateRE.FindStringSubmatch("12月末，人民币汇率为1美元兑7.0288元人民币。")
		require.Len(t, m, 2)
		assert.Equal(t, "7.0288", m[1])
	})
}

// TestBalanceRegexpClassifiesCurrencyQualifier 是本 Sprint 最高风险点的守卫。
//
// 两份样本的存/贷板块里各有**三**句同形余额句，且本外币在前：
//
//	12月末，本外币存款余额336.14万亿元，同比增长9%。
//	月末人民币存款余额328.64万亿元，同比增长8.7%。
//	12月末，外币存款余额1.07万亿美元，同比增长25%。
//
// 本包的机制**不是**「靠 `人民币存款余额` 这个子串恰好不出现在 `本外币存款余额` 里」
// ——那是巧合，去掉守卫也照样通过（reviewer G8 实证）。机制是：
// **口径限定词是一个显式捕获组，三句都被匹配、按限定词的值选择，并要求人民币句唯一。**
//
// 这条测试同时钉住 boundary[0]② 的币种守卫：外币句用的是**美元单位**，
// 若只按最终值比对，`toWanYi()` 在 1.07 万亿美元与 1.07 万亿元上完全相等
// （dev-agent-45 T4 实测），浮点数不带币种信号——只有断言捕获组才抓得住。
func TestBalanceRegexpClassifiesCurrencyQualifier(t *testing.T) {
	const depositText = "12月末，本外币存款余额336.14万亿元，同比增长9%。月末人民币存款余额328.64万亿元，" +
		"同比增长8.7%。全年人民币存款增加26.41万亿元。12月末，外币存款余额1.07万亿美元，同比增长25%。"

	all := depositBalanceRE.FindAllStringSubmatch(depositText, -1)
	require.Len(t, all, 3, "三句同形余额句都必须被匹配到——漏掉一句就无从按限定词排除它")

	byCurrency := map[string][]string{}
	for _, m := range all {
		byCurrency[m[1]] = m
	}
	require.Contains(t, byCurrency, "本外币")
	require.Contains(t, byCurrency, "外币")
	require.Contains(t, byCurrency, "人民币")

	assert.Equal(t, "336.14", byCurrency["本外币"][2])
	assert.Equal(t, "328.64", byCurrency["人民币"][2])
	assert.Equal(t, "1.07", byCurrency["外币"][2])

	// 断言捕获组本身：人民币句的单位必须是人民币量纲，外币句必须是美元量纲
	assert.Equal(t, "万亿元", byCurrency["人民币"][3])
	assert.Equal(t, "万亿美元", byCurrency["外币"][3],
		"外币句的单位是万亿美元——归一成浮点后与万亿元完全无法区分")
	assert.Equal(t, "增长", byCurrency["人民币"][4])
	assert.Equal(t, "8.7", byCurrency["人民币"][5])

	const loanText = "12月末，本外币贷款余额275.74万亿元，同比增长6.2%。月末人民币贷款余额271.91万亿元，" +
		"同比增长6.4%。12月末，外币贷款余额5450亿美元，同比增长0.5%。"
	all = loanBalanceRE.FindAllStringSubmatch(loanText, -1)
	require.Len(t, all, 3)
	for _, m := range all {
		if m[1] == "人民币" {
			assert.Equal(t, "271.91", m[2])
			assert.Equal(t, "万亿元", m[3])
		}
	}
}

// TestFlowRegexpClassifiesPeriodPrefix 钉住 boundary[0]① 的期次孪生句（G3）。
//
// 2020 贷款板块同一段里有两句都能被「人民币贷款+方向词+数值」命中：
//
//	上半年人民币贷款增加12.09万亿元   ← 期内合计，要它
//	6月份，人民币贷款增加1.81万亿元   ← 单月，不要它
//
// 计划书只用 `人民币贷款(dir)(num)(unit)` + FindStringSubmatch，**靠最左优先
// 碰巧选中了对的那句**；把两句顺序对调就会取到 1.81。本包把期次前缀做成显式
// 捕获组并按值选择，与文本顺序无关。
//
// 另外，外币孪生句的期次前缀同样是「全年/上半年」（如「全年外币存款增加2135亿美元」），
// 所以期次与口径两个维度都要判——只判其一都会漏。
func TestFlowRegexpClassifiesPeriodPrefix(t *testing.T) {
	const loanText = "上半年人民币贷款增加12.09万亿元，同比多增2.42万亿元。分部门看，住户部门贷款增加3.56万亿元。" +
		"6月份，人民币贷款增加1.81万亿元，同比多增1474亿元。上半年外币贷款增加774亿美元。"

	all := loanFlowRE.FindAllStringSubmatch(loanText, -1)
	require.Len(t, all, 3, "两句人民币 + 一句外币都要被匹配，才谈得上按期次与口径排除")

	got := map[string][2]string{} // period+currency → num, unit
	for _, m := range all {
		got[m[1]+"/"+m[2]] = [2]string{m[4], m[5]}
	}
	assert.Equal(t, [2]string{"12.09", "万亿元"}, got["上半年/人民币"])
	assert.Equal(t, [2]string{"1.81", "万亿元"}, got["6月份/人民币"])
	assert.Equal(t, [2]string{"774", "亿美元"}, got["上半年/外币"])

	// 顺序对调后结论必须不变——这是「碰巧对」与「真的对」的分界
	const swapped = "6月份，人民币贷款增加1.81万亿元，同比多增1474亿元。" +
		"上半年人民币贷款增加12.09万亿元，同比多增2.42万亿元。"
	all = loanFlowRE.FindAllStringSubmatch(swapped, -1)
	require.Len(t, all, 2)
	for _, m := range all {
		if m[1] == "上半年" {
			assert.Equal(t, "12.09", m[4], "顺序对调后仍须取到期内合计值")
		}
	}
	assert.Equal(t, "6月份", all[0][1], "对调后最左的确实是单月句——最左优先在这里会取错")
}

// TestRateRegexpsToleratePingTypo 覆盖 functional[3] 的容错半边。
//
// 2025 原文正文写的是「质押式回购加权**均**利率」（缺「平」字），2020 写的是
// 「加权平均利率」。同一条模板要两边都命中。
func TestRateRegexpsToleratePingTypo(t *testing.T) {
	m := rateRepoRE.FindStringSubmatch("质押式回购加权均利率为1.4%，分别比上月和上年同期低0.04个和0.25个百分点。")
	require.Len(t, m, 2, "2025 原文缺「平」字，必须容忍")
	assert.Equal(t, "1.4", m[1])

	m = rateRepoRE.FindStringSubmatch("质押式回购加权平均利率为1.89%，分别比上月和上年同期高0.6个和0.15个百分点。")
	require.Len(t, m, 2)
	assert.Equal(t, "1.89", m[1])

	m = rateIBORE.FindStringSubmatch("12月份同业拆借加权平均利率为1.36%，分别比上月和上年同期低0.06个和0.21个百分点。")
	require.Len(t, m, 2)
	assert.Equal(t, "1.36", m[1], "不得取到紧邻的 0.06 / 0.21")
}

// TestRateRegexpsRejectLoosePatterns 覆盖 functional[3] 更要紧的那半边。
//
// 容错的**修法**才是危险所在：同板块还有
//
//	同业拆借加权平均利率为1.36%          ← 另一个字段，与 1.4 同量级同格式
//	质押式回购日均成交同比增长2.9%        ← 同样以「质押式回购」开头
//	（标题）质押式债券回购月加权平均利率为1.4%
//
// ⚠️ 本注释原先写着「放松成 `质押式回购加权.*利率为` 会静默取到 1.36」——**那是错的**。
// 在真实 2025 利率板块正文上逐条实测（TASK-006 复核，与 test-agent-22 的读数一致）：
//
//	质押式回购加权平?均利率为   → 1.4   ← 现行实现
//	质押式回购加权.*利率为      → 1.4   ← 原注释断言会取到 1.36，实际不会
//	质押式回购.*利率为         → 1.4
//	加权.*利率为              → 1.4   ← DoD 原文断言会取到 1.36，实际不会
//	加权.*?利率为             → 1.36  ← 只有**非贪婪**这一种真的取错
//
// 机制：保留 `质押式回购` 前缀把匹配**起点**锚在回购句上，而 1.36 在它**之前**，
// 结构上取不到；`加权.*利率为` 虽然起点落在拆借句，但贪婪的 `.*` 会一路吃到本段
// **最后**一个「利率为」，那正是回购句。⇒ 放松锚点的危险程度取决于**贪婪性**，
// 不能一概而论，而两份样本里回购句恰好都排在最后。
//
// 结论与守护都不受影响：本测试把近邻句**逐条孤立**喂给两条正则，与全文顺序无关，
// 四种放松形态全都抓得住。**这条测试比催生它的那个理由更强。**
//
// 记这一段是因为：这是同一份 DoD 里第二次「把一条正确的引擎机制套到不适用的语境上」
// （第一次是交替顺序，见 amount.go 的 directionAlt）。结论对会让理由永不被复查，
// 而理由是别人复现时唯一的入口。
func TestRateRegexpsRejectLoosePatterns(t *testing.T) {
	for _, tc := range []struct {
		name string
		re   *regexp.Regexp
		text string
	}{
		{"同业拆借句不得被回购模板命中", rateRepoRE, "12月份同业拆借加权平均利率为1.36%。"},
		{"日均成交句不得被命中", rateRepoRE, "质押式回购日均成交同比增长2.9%。"},
		{"标题的债券回购月加权不得被命中", rateRepoRE, "质押式债券回购月加权平均利率为1.4%"},
		{"回购句不得被拆借模板命中", rateIBORE, "质押式回购加权均利率为1.4%。"},
		{"拆借日均成交句不得被命中", rateIBORE, "同业拆借日均成交同比下降12.1%。"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Empty(t, tc.re.FindString(tc.text))
		})
	}

	// 两条正则的源串里都不得出现 `.*` / `.+` —— 那是「放松锚点」的典型形态
	for _, re := range []*regexp.Regexp{rateIBORE, rateRepoRE} {
		assert.NotContains(t, re.String(), `.*`)
		assert.NotContains(t, re.String(), `.+`)
	}
}

// —— M1b-4b / TASK-004：季报两个期次前缀 ——
//
// Context Checkpoint: done_criteria → test mapping (TASK-004)
// functional[0]     periodAlt 与 cumulativePeriods 两处都加「一季度/前三季度」
//                                   → TestQuarterlyPeriodsAreCumulative、TestFlowRECapturesQuarterlyPeriodVerbatim
// boundary[0]       交替顺序陷阱：捕获组须逐字等于样本里的词
//                                   → TestFlowRECapturesQuarterlyPeriodVerbatim
// boundary[1]       外币孪生句仍被正确区分（两个季度各一条，实测都存在）
//                                   → TestQuarterlyFXTwinSentencesStayDistinct
// error_handling[0] 「一季度」与「前三季度」各一条断言（reviewer D8：后者有后缀陷阱、前者没有）
//                                   → 上面两条测试都按 q1 / q1_q3 分子测试

// 🔴 交替顺序陷阱的**真正**守卫：捕获组必须**逐字等于**样本里的那个词。
//
// 陷阱本身（reviewer C2）：若 periodAlt 里写的是 `三季度` 而不是 `前三季度`，
// Go 的 leftmost-first 会对「前三季度人民币贷款…」在位置 0 匹不上、位置 1 匹上
// ⇒ 捕获 `三季度`。此时若为了让它绿而把 `三季度` 也登记进 cumulativePeriods，
// 就等于断言「三季度 = 1–9 月累计」——**没有任何样本支持**（前三季度是 1–9 月
// 累计，第三季度单季是 7–9 月，期末月同为 09 而月均除数是 9 与 3）。
//
// ⚠️ **这条断言是唯一抓得到该陷阱的**：写成 `三季度` 时交替内部**构不成任何词对**
// （全年/上半年/一季度/三季度 两两无包含关系），所以 TestProfileAlternationsHaveNoPrefixPairs
// 与任何「子串对」式的判据都会**全绿放行**（实测，见那条测试的注释）。
// 断言「能匹配」同样不够——写成 `三季度` 时它照样匹配得上，只是捕获错了。
func TestFlowRECapturesQuarterlyPeriodVerbatim(t *testing.T) {
	for _, tc := range []struct{ name, file, want string }{
		{"q1 一季度", "pboc-2026-03-q1.html", "一季度"},
		{"q1_q3 前三季度", "pboc-2025-09-q3.html", "前三季度"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := stripHTML(readTestdata(t, tc.file))

			for _, re := range []struct {
				name string
				re   *regexp.Regexp
			}{{"贷款", loanFlowRE}, {"存款", depositFlowRE}} {
				all := re.re.FindAllStringSubmatch(text, -1)
				require.NotEmptyf(t, all, "%s：真实正文里必须有能命中的期内合计句，否则下面的断言平凡为真", re.name)

				var periods []string
				for _, m := range all {
					periods = append(periods, m[1])
				}
				assert.Containsf(t, periods, tc.want,
					"%s：捕获组必须**逐字**等于 %q。捕获到 %v —— 若其中出现 %q 而非 %q，"+
						"就是 periodAlt 写成了短词、leftmost 从中间起匹",
					re.name, tc.want, periods, strings.TrimPrefix(tc.want, "前"), tc.want)
			}
		})
	}
}

// 两个季度前缀都必须被认成**期内累计**口径。
//
// 这是与 periodAlt 相互独立的第二个硬卡点：periodAlt 决定「句子能不能被匹配」，
// 本表决定「匹中的句子算不算累计」。只加前者会让候选句涨起来却全被口径判定筛掉
// （命中 0），只加后者则正则根本不命中（候选恒为 1）——两者缺一都抽不到值。
func TestQuarterlyPeriodsAreCumulative(t *testing.T) {
	for _, p := range []string{"一季度", "前三季度"} {
		assert.Truef(t, cumulativePeriods[p], "%q 必须在 cumulativePeriods 里，否则匹中的句子会被当成单月句筛掉", p)
	}
	assert.Falsef(t, cumulativePeriods["三季度"],
		"「三季度」**不得**登记为累计口径：前三季度是 1–9 月累计、第三季度单季是 7–9 月，"+
			"把它登记进来等于用一个没有样本支持的判断去掩盖 periodAlt 写错")
}

// 外币孪生句在两个季度里都真实存在，必须仍被按币种区分开。
//
// ⚠️ 这条**不会**落到「未观察到」分支：实测两份快照里都有外币句
// （一季度：外币存款增加703亿美元 / 外币贷款增加329亿美元；
//
//	前三季度：外币存款增加1658亿美元 / 外币贷款增加123亿美元）。
//
// 期次与口径两个维度都判之后，币种仍必须靠捕获组 m[2] 分开——
// 归一成浮点后「703 亿美元」与「703 亿元」完全无法区分。
func TestQuarterlyFXTwinSentencesStayDistinct(t *testing.T) {
	for _, tc := range []struct{ name, file, period string }{
		{"q1 一季度", "pboc-2026-03-q1.html", "一季度"},
		{"q1_q3 前三季度", "pboc-2025-09-q3.html", "前三季度"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := stripHTML(readTestdata(t, tc.file))

			for _, re := range []struct {
				name string
				re   *regexp.Regexp
			}{{"贷款", loanFlowRE}, {"存款", depositFlowRE}} {
				byCurrency := map[string]string{} // 币种 → 单位
				for _, m := range re.re.FindAllStringSubmatch(text, -1) {
					if m[1] == tc.period {
						byCurrency[m[2]] = m[5]
					}
				}
				assert.Equalf(t, "万亿元", byCurrency[currencyRMB],
					"%s：人民币句的单位应是万亿元", re.name)
				assert.Equalf(t, "亿美元", byCurrency["外币"],
					"%s：外币孪生句必须被单独认出且单位是亿美元 —— 实测两份季报里它都存在，"+
						"这里为空说明币种维度没分开", re.name)
			}
		})
	}
}

// —— M1c-3a 的 TASK-001：月报累计前缀「前N个月」与 1 月特例 ——
//
// Context Checkpoint: done_criteria → test mapping（M1c-3a 的 TASK-001）
// functional[0]     cumulativeMonthAlt 穷举十项，periodAlt 由它拼出（不另抄一份）
//                                    → TestCumulativeMonthAltEnumeratesEveryCumulativePrefix
// functional[1]     cumulativePeriods 增 11 键（10 个「前N个月」+「1月份」特例）
//                                    → TestCumulativePeriodsKeySet
// functional[2]     两处硬卡点一一对应的**机械**守卫（双向）
//                                    → TestCumulativeMonthAltAndCumulativePeriodsAgree
// boundary[0]       当月句被正则认出、却被口径表排除（1..12 逐个）
//                                    → TestSingleMonthPrefixesAreMatchedButNotCumulative
// boundary[1]       「11月份」不得退化成「1月份」（捕获组逐字）
//                                    → TestFlowRECapturesMonthlyPeriodVerbatim
// boundary[2]       既有四键原样保留    → TestCumulativePeriodsKeySet（含全年/上半年/一季度/前三季度）
// error_handling[0] 两处硬卡点在**消费点**真的接上了（selectRMBCumulativeFlow）
//                                    → TestSelectRMBCumulativeFlowPicksMonthlyCumulative

// cumulativeMonthAlt 必须**穷举**十项，且 periodAlt 由它拼出。
//
// 穷举而不写成 `前[一二三四五六七八九十]+个月`：正则能匹配的每一个累计前缀都必须
// 在 cumulativePeriods 里有对应键，否则会出现「正则认得、口径表不认」的组合——
// 那种句子被静默筛掉，表现为整篇命中 0（见 TestCumulativeMonthAltAndCumulativePeriodsAgree）。
//
// # 交替顺序：本形态下**实测无关**，但仍把「前十一个月」排在「前十个月」前
//
// DoD 提示「`十一|十` 的顺序不能反」。本任务对四种形态各跑一遍（枚举×两序、
// 提取公因式 `前(…|十一|十)个月`×两序），**四组捕获逐字相同**：Go 的 leftmost-first
// 是对**整体匹配**而言的，某个交替分支走到一半失败会退回去试下一个分支，所以
// 「前十个月」这一支在「前十一个月…」上匹到 `前十` 后卡在 `个` vs `一` 即被放弃。
// 顺序真正有语义的前提是**两个分支都能走完整条正则**（`人民|人民币` 那类前缀对），
// 这里的十项两两无前缀关系，不构成那个前提。
//
// ⇒ 保留「十一在前」是零成本的防御（将来若被改写成提取公因式形态也仍然安全），
// 但**它不是这里的安全性来源**。真正的守卫是下面两条：一一对应（漏词会被查出）
// 与逐字捕获（捕获错会被查出）。另实测：交替里**缺**「前十一个月」时结果是
// NO MATCH（selectUnique 报 0 candidate sentence、响亮失败），不是静默捕获成「前十个月」。
func TestCumulativeMonthAltEnumeratesEveryCumulativePrefix(t *testing.T) {
	// 「前N个月」族十项（M1c-3a 的 TASK-001）
	want := []string{
		"前两个月", "前三个月", "前四个月", "前五个月", "前六个月",
		"前七个月", "前八个月", "前九个月", "前十一个月", "前十个月",
	}
	// 「N-M月」族十一项（M1c-3a 的 TASK-012，QA R3）——2022 年月报的写法。
	// 实测只出现 1-7/1-8/1-10/1-11 四种，其余七种一并登记：只登记见过的，
	// 将来出现 `1-9月` 会是**静默 0 命中**，正是 R3 要修的那个失效方式。
	want = append(want, "1-10月", "1-11月", "1-12月", "1-2月", "1-3月",
		"1-4月", "1-5月", "1-6月", "1-7月", "1-8月", "1-9月")

	assert.Equal(t, want, strings.Split(cumulativeMonthAlt, "|"),
		"cumulativeMonthAlt 必须穷举这两族共 21 项：真实语料里不出现的（前三/前六/前九个月走 "+
			"q1/h1/q1_q3 报告；N-M月 只见过四种）仍然列上——多一个无害，漏一个是静默失效")

	// 🔴 **等值**而不是 Contains —— 收口 M1c-3a 的 TASK-001 验证者发现的残余缺口 V6。
	//
	// Contains 只断言「cumulativeMonthAlt 出现在 periodAlt 里」，管不住**另外**塞进去的分支：
	// 实测把一个累计前缀绕开常量直接写进 periodAlt（`…|前十二个月|[0-9]{1,2}月份`），
	// 本节全部新测试无一转红（V6 SURVIVED）——因为下面那条正向断言遍历的是
	// cumulativeMonthAlt，看不见常量之外的分支。而**注释比断言宽**正是这类缺口的来源：
	// 「periodAlt 认得的每个累计前缀都必须在口径表里」这句话，只有把 periodAlt 钉死成
	// 「四个既有 + cumulativeMonthAlt + 数字月份」之后才**真的**成立。
	//
	// 等值断言是本包既有惯用法（见 TestTemplatesReuseAmountFragments 对 dirPat/unitPat）。
	// 代价是后续扩 periodAlt（如 M1c-3a 的 TASK-007）必须同步改这一行 —— 那正是要的信号：
	// 让「periodAlt 的组成变了」成为一次显式的、有人看见的改动，而不是静默扩面。
	assert.Equal(t, `全年|上半年|一季度|前三季度|`+cumulativeMonthAlt+`|[0-9]{1,2}月份`, periodAlt,
		"periodAlt 的组成必须逐字等于「四个既有期次 + cumulativeMonthAlt + 数字月份」："+
			"累计前缀一律经 cumulativeMonthAlt 登记，绕开常量直接插分支会让它躲过下面的一一对应守卫")
}

// cumulativePeriods 的键集必须**逐字**等于这 26 个。
//
// 这条是整张表的**性质**断言（哪些前缀算累计），不是条数断言：
//   - 既有四键「全年/上半年/一季度/前三季度」原样保留（3 月报靠「一季度」工作）；
//   - 10 个「前N个月」+「1月份」（M1c-3a 的 TASK-001）；
//   - 11 个「N-M月」（M1c-3a 的 TASK-012，QA R3：2022 年月报的写法）；
//   - 除「1月份」外**没有**任何 `N月份` 键——当月数落进累计表会让 *_ytd 装上当月数。
//
// 「1月份」是特例：1 月的累计就是当月，报告直接写「1月份人民币贷款增加5.13万亿元」，
// 与当月句同形。安全性依赖一条实测前提——**非 1 月报里不出现「1月份+指标」的句子**，
// Leader 用全部 218 篇实测得 0 条（需求文档只验了 55 篇，结论一致）。
func TestCumulativePeriodsKeySet(t *testing.T) {
	want := map[string]bool{
		"全年": true, "上半年": true, "一季度": true, "前三季度": true,
		"前两个月": true, "前三个月": true, "前四个月": true, "前五个月": true, "前六个月": true,
		"前七个月": true, "前八个月": true, "前九个月": true, "前十一个月": true, "前十个月": true,
		"1月份": true,
		// M1c-3a 的 TASK-012（QA R3）：2022 年月报的「N-M月」族
		"1-2月": true, "1-3月": true, "1-4月": true, "1-5月": true, "1-6月": true,
		"1-7月": true, "1-8月": true, "1-9月": true, "1-10月": true, "1-11月": true, "1-12月": true,
	}
	assert.Equal(t, want, cumulativePeriods,
		"cumulativePeriods 的键集不对：既有四键必须原样保留，`N月份` 只许有「1月份」一个，"+
			"「N-M月」族必须与 cumulativeRangeAlt 一一对应")
}

// 🔴 两处硬卡点的**机械**一一对应守卫 —— 把「记得两边都改」这件事变成可查的。
//
// 两个方向各自对应一个硬卡点，故意分开断言（消融时失败信息不同，见 DoD error_handling）：
//
//	正向：periodAlt 认得的每个累计前缀，cumulativePeriods 里必须有
//	      —— 缺了就是「候选句涨了但口径判定全筛掉」，命中 0
//	反向：cumulativePeriods 里的每个键，periodPat 必须能逐字整段捕获
//	      —— 缺了就是「正则根本不命中」，与现状逐字相同的 no-op
func TestCumulativeMonthAltAndCumulativePeriodsAgree(t *testing.T) {
	// ⚠️ 本条遍历的是 **cumulativeMonthAlt**，不是 periodAlt 的全部分支。两者等价**依赖**
	// TestCumulativeMonthAltEnumeratesEveryCumulativePrefix 里那条把 periodAlt 钉死的等值断言——
	// 少了它，绕开常量插进 periodAlt 的累计分支对本条不可见（V6）。两条互补，别当重复删掉。
	t.Run("正向：正则认得的必须在口径表里", func(t *testing.T) {
		for _, w := range strings.Split(cumulativeMonthAlt, "|") {
			assert.Truef(t, cumulativePeriods[w],
				"%q 在 cumulativeMonthAlt 里、却不在 cumulativePeriods 里："+
					"这类句子会被匹中后当成单月句静默筛掉，表现为整篇命中 0", w)
		}
	})

	t.Run("反向：口径表里的必须被正则整段认出", func(t *testing.T) {
		anchored := regexp.MustCompile(`^` + periodPat + `$`)
		for k := range cumulativePeriods {
			m := anchored.FindStringSubmatch(k)
			if assert.NotNilf(t, m, "%q 登记在 cumulativePeriods 里、periodPat 却整段匹不上它："+
				"正则不命中 ⇒ 候选句一条都不涨，是与现状逐字相同的 no-op", k) {
				assert.Equalf(t, k, m[1], "periodPat 对 %q 必须逐字整段捕获", k)
			}
		}
	})
}

// 当月句前缀必须「被正则认出、却不算累计」——两个性质缺一都会出错。
//
// 认不出 ⇒ 2020 那种「上半年…／6月份，…」孪生段里单月句根本不进候选，口径判定
// 也就无从生效；算累计 ⇒ *_ytd 装上当月数（该表存在的全部理由）。
//
// 「1月份」是唯一的例外，见 TestCumulativePeriodsKeySet。
func TestSingleMonthPrefixesAreMatchedButNotCumulative(t *testing.T) {
	anchored := regexp.MustCompile(`^` + periodPat + `$`)
	for i := 1; i <= 12; i++ {
		p := fmt.Sprintf("%d月份", i)
		t.Run(p, func(t *testing.T) {
			m := anchored.FindStringSubmatch(p)
			require.NotNilf(t, m, "%q 必须仍被 periodAlt 认出——认不出的话单月句连候选都进不了", p)
			assert.Equal(t, p, m[1], "捕获组必须逐字等于整个前缀")

			assert.Equalf(t, i == 1, cumulativePeriods[p],
				"%q 的累计口径判定错了：只有「1月份」是累计特例（1 月的累计=当月），"+
					"其余月份落进累计表会让 *_ytd 装上当月数", p)
		})
	}
}

// 🔴 捕获组必须**逐字**等于样本里的那个前缀 —— 这是「11月份 退化成 1月份」的守卫。
//
// 机制：`[0-9]{1,2}月份` 的 `{1,2}` 贪婪 + Go 取最左匹配 ⇒ 在「11月份」处从第一个 1
// 开始匹配、捕获完整的「11月份」，查表不命中、正确排除。**Go 侧因此不需要 lookbehind**
// （而用 python 核对语料时必须带 `(?<![0-9])`，否则「11月份」里的子串会给出 12 条假阳性）。
// 把 `{1,2}` 改成 `{1}`、或在前面插入别的分支让最左匹配从第二个 1 起匹，这条立刻红。
//
// ⚠️ 本条用**合成句**：形态取自 Leader 实读 55 篇月报的记述，但本任务不抓 html 快照
// （端到端由 M1c-3a 的 TASK-007 用真实月报确认）。所以这里只断言正则与口径表的行为，
// 不宣称任何语料事实。
func TestFlowRECapturesMonthlyPeriodVerbatim(t *testing.T) {
	for _, tc := range []struct {
		name, sentence, wantPeriod string
		wantCumulative             bool
	}{
		{"累计：前十一个月", "前十一个月人民币贷款增加1.09万亿元", "前十一个月", true},
		{"累计：前十个月（与上一条只差一个字）", "前十个月人民币贷款增加1.00万亿元", "前十个月", true},
		{"累计：前八个月", "前八个月人民币贷款增加1.45万亿元", "前八个月", true},
		{"累计特例：1月份", "1月份人民币贷款增加5.13万亿元", "1月份", true},
		{"当月：11月份不得退化成1月份", "11月份，人民币贷款增加5000亿元", "11月份", false},
		{"当月：前面带年份数字也不得退化", "2025年11月份，人民币贷款增加5000亿元", "11月份", false},
		{"当月：2月份", "2月份，人民币贷款增加1.81万亿元", "2月份", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := loanFlowRE.FindStringSubmatch(tc.sentence)
			require.NotNilf(t, m, "loanFlowRE 必须命中 %q —— 匹不上说明 periodAlt 里缺词", tc.sentence)

			assert.Equalf(t, tc.wantPeriod, m[1],
				"捕获组必须**逐字**等于 %q。捕获到 %q —— 若把「11月份」捕成「1月份」，"+
					"当月数会被当成累计口径装进 *_ytd", tc.wantPeriod, m[1])

			assert.Equalf(t, tc.wantCumulative, cumulativePeriods[m[1]],
				"%q 的口径判定错了", m[1])
		})
	}
}

// 🔴 两处硬卡点在**消费点**接上了 —— selectRMBCumulativeFlow 同时用两者。
//
// 上面几条分别验正则与口径表；这条验它们合起来真的能从一段含孪生句的正文里
// 挑出唯一正确的那句。判据 `cumulativePeriods[m[1]] && m[2] == currencyRMB`
// 两个维度都要过：外币孪生句的期次前缀与本币句相同，只判期次会取到亿美元那句。
//
// ⚠️ 合成正文，形态取自 Leader 实读的记述；真实月报端到端由 M1c-3a 的 TASK-007 确认。
func TestSelectRMBCumulativeFlowPicksMonthlyCumulative(t *testing.T) {
	t.Run("前十一个月：从三句里挑出人民币累计句", func(t *testing.T) {
		const body = "前十一个月人民币贷款增加1.09万亿元，同比多增1474亿元。" +
			"11月份，人民币贷款增加5000亿元。" +
			"前十一个月外币贷款增加123亿美元。"

		m, err := selectRMBCumulativeFlow(loanFlowRE, body, "贷款期内合计")
		require.NoError(t, err)
		assert.Equal(t, []string{"前十一个月", "人民币", "增加", "1.09", "万亿元"}, m[1:],
			"必须取到人民币累计句：期次维度筛掉 11月份，币种维度筛掉外币孪生句")
	})

	t.Run("1月份：累计特例走同一条路", func(t *testing.T) {
		const body = "1月份人民币贷款增加5.13万亿元，同比多增3800亿元。" +
			"1月份外币贷款增加200亿美元。"

		m, err := selectRMBCumulativeFlow(loanFlowRE, body, "贷款期内合计")
		require.NoError(t, err)
		assert.Equal(t, []string{"1月份", "人民币", "增加", "5.13", "万亿元"}, m[1:])
	})

	t.Run("阴性对照：只有当月句时必须报错、不得当成累计", func(t *testing.T) {
		const body = "11月份，人民币贷款增加5000亿元。前十一个月外币贷款增加123亿美元。"

		_, err := selectRMBCumulativeFlow(loanFlowRE, body, "贷款期内合计")
		require.Errorf(t, err, "当月句被当成累计口径取走了 —— 那正是 cumulativePeriods 要挡的")
		assert.Contains(t, err.Error(), "not found among 2 candidate sentence(s)",
			"两句都该进候选（正则认得），再被口径/币种两个维度筛光")
	})

	t.Run("存款侧同构", func(t *testing.T) {
		const body = "前八个月人民币存款增加15.6万亿元。8月份，人民币存款增加2.1万亿元。"

		m, err := selectRMBCumulativeFlow(depositFlowRE, body, "存款期内合计")
		require.NoError(t, err)
		assert.Equal(t, []string{"前八个月", "人民币", "增加", "15.6", "万亿元"}, m[1:])
	})
}

// —— M1c-3a 的 TASK-005：模板措辞变体（企业锚点 / 全角括号 / 余额为后空格）——
//
// Context Checkpoint: done_criteria → test mapping（M1c-3a 的 TASK-005）
// functional[1]     企业锚点认两版称呼，且**必须**是非捕获组
//                                 → TestLoanScopeAnchorsCoverBothVintages
//                                 → TestScopeTotalREKeepsCaptureGroupsAfterAnchorAlternation
// functional[2]     moneyRE 认全角与半角括号，M2/M1/M0 三项都断言
//                                 → TestMoneyREAcceptsBothParenWidths
// functional[3](a)  tsfStockRE 容忍「余额为」后的一个空格
//                                 → TestTSFStockREToleratesSpaceAfterBalancePrefix
// boundary[0]       五个子项字段**各归各位**（真实 2019 年报逐字段）
//                                 → TestLoanScopeAnchorsCoverBothVintages/2019年报
// boundary[1]       两种括号各有用例，半角既有形态保持绿（扩大而非替换）
//                                 → TestMoneyREAcceptsBothParenWidths（两份真实快照）
// error_handling[0] 锚点找不到时响亮失败，且错误信息含锚点原文
//                                 → TestLoanScopeAnchorMissingIsLoudFailure

// realSection 从整篇正文里切出一个真实板块喂给模板层。
//
// 本任务测的是**模板/锚点层**，板块切分是 sections.go 的职责（本 wave 归 M1c-3a 的
// TASK-004），这里不重复实现、也不依赖它：只按「`一、`…`六、`」这种小标题行把正文
// 切开，取标题含 kw 的那一段。
//
// ⚠️ 标题必须与正文分开：小标题本身就是一个完整句（「二、全年人民币贷款增加16.81万亿元」），
// 把它留在 Body 里会让 loanFlowRE 命中两次、`selectUnique` 直接报 matched 2 sentences
// ——实测过，这正是 section 类型区分 Title / Body 的理由。
func realSection(t *testing.T, text, kw string) section {
	t.Helper()
	headRE := regexp.MustCompile(`(?m)^[一二三四五六七八九十]+、.*$`)
	locs := headRE.FindAllStringIndex(text, -1)
	require.NotEmpty(t, locs, "正文里一个小标题都没有——切分前提不成立")

	for i, loc := range locs {
		title := text[loc[0]:loc[1]]
		if !strings.Contains(title, kw) {
			continue
		}
		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		return section{Title: title, Body: text[loc[1]:end]}
	}
	t.Fatalf("没有标题含 %q 的板块", kw)
	return section{}
}

// 企业作用域锚点必须同时认两版称呼，且五个子项字段**各归各位**。
//
// 2019 年报用旧称呼「非金融企业及机关团体贷款」，2025 起用「企（事）业单位贷款」。
// 旧称呼认不出时整个贷款板块抽不出来（`loan scope anchor … not found`）。
//
// 🔴 **为什么是逐字段断言而不是「无 error」**：锚点错位的后果不是「抽不到」，是
// **住户的短期贷款跑进企业字段** —— 两个值都是合法量级，而 corp_loan_reconcile 是
// **加总**校验，错位后总和不变，七道闸门一道也拦不住。所以这里逐条钉住五个字段。
//
// 🔴 **住户侧一个字都没改**（Leader 实测 C5）：现有 `住户(?:部门)?贷款` 已覆盖两版。
// 我在 2019 快照上复核过：`住户部门贷款` 出现 1 次、`住户贷款` 0 次 ⇒ 已命中，改它是空动作。
func TestLoanScopeAnchorsCoverBothVintages(t *testing.T) {
	t.Run("2019年报：旧称呼「非金融企业及机关团体贷款」", func(t *testing.T) {
		text := stripHTML(readTestdata(t, "pboc-2019-annual.html"))
		got, err := extractLoanSection(realSection(t, text, "人民币贷款增加"))
		require.NoError(t, err, "旧称呼认不出会让整个贷款板块抽不出来")

		// 原文：住户部门贷款增加7.43万亿元，其中，短期贷款增加1.98万亿元，中长期贷款增加5.45万亿元；
		//       非金融企业及机关团体贷款增加9.45万亿元，其中，短期贷款增加1.52万亿元，
		//       中长期贷款增加5.88万亿元，票据融资增加1.84万亿元；非银行业金融机构贷款减少933亿元。
		for _, tc := range []struct {
			field string
			want  float64
		}{
			{FieldLoanHHShortYTD, 19800},   // 住户 短期 1.98 万亿
			{FieldLoanHHMLTYTD, 54500},     // 住户 中长期 5.45 万亿
			{FieldLoanCorpShortYTD, 15200}, // 企业 短期 1.52 万亿
			{FieldLoanCorpMLTYTD, 58800},   // 企业 中长期 5.88 万亿
			{FieldLoanBillYTD, 18400},      // 票据融资 1.84 万亿
		} {
			assert.InDeltaf(t, tc.want, got[tc.field], 1e-6,
				"%s 取到 %v —— 锚点错位时住户与企业的同名子项会互换，两个值都是合法量级、加总校验也拦不住",
				tc.field, got[tc.field])
		}

		assert.InDelta(t, 94500.0, got[FieldLoanCorpTotalYTD], 1e-6, "企业合计 9.45 万亿")
		assert.InDelta(t, -933.0, got[FieldLoanNBFIYTD], 1e-6, "非银行业金融机构贷款减少933亿元（方向为负）")
	})

	t.Run("2025 式新称呼「企（事）业单位贷款」仍绿（扩大而非替换）", func(t *testing.T) {
		got, err := extractLoanSection(section{Body: "月末人民币贷款余额271.91万亿元，同比增长6.4%。" +
			"全年人民币贷款增加16.27万亿元。分部门看，住户贷款增加4417亿元，" +
			"其中，短期贷款减少8351亿元，中长期贷款增加1.28万亿元；" +
			"企（事）业单位贷款增加15.47万亿元，其中，短期贷款增加4.81万亿元，" +
			"中长期贷款增加8.82万亿元，票据融资增加1.66万亿元；" +
			"非银行业金融机构贷款减少1103亿元。"})
		require.NoError(t, err)
		assert.InDelta(t, -8351.0, got[FieldLoanHHShortYTD], 1e-6)
		assert.InDelta(t, 48100.0, got[FieldLoanCorpShortYTD], 1e-6)
		assert.InDelta(t, 16600.0, got[FieldLoanBillYTD], 1e-6)
	})
}

// 🔴 锚点里的交替**必须**是非捕获组 —— 这条守的是 `scopeTotalRE` 的字符串拼接。
//
//	scopeTotalRE: regexp.MustCompile(sc.anchorRE.String() + dirPat + numPat + unitPat)
//
// 裸交替 `A|B` 拼接后结合律错位 ⇒ `A|B(dir)(num)(unit)`：在 2025 式报告里 `A` 单独
// 命中、三个捕获组全空，`newAmount("","","")` 报「unknown direction word」。
// 独立 reviewer 照 DoD 原文实现时实撞过这个坑。
//
// 断言写成「三个捕获组都非空」而不是「能匹配」：**裸交替下它照样匹配得上**，
// 只是捕获组是空的 —— 这与 M1c-3a 的 TASK-001 里「能匹配不等于捕获对」是同一课。
func TestScopeTotalREKeepsCaptureGroupsAfterAnchorAlternation(t *testing.T) {
	var corp loanScope
	for _, sc := range loanScopes {
		if sc.totalField == FieldLoanCorpTotalYTD {
			corp = sc
		}
	}
	require.NotNil(t, corp.anchorRE, "没找到企业作用域")

	// 模板只取决于作用域本身、与句子无关，故建一次给两个 vintage 共用。
	re := scopeTotalRE(corp)
	require.Equal(t, 3, re.NumSubexp(), "作用域合计句模板必须恰好三个捕获组：方向词/数值/单位")

	for _, tc := range []struct{ name, sentence string }{
		{"v2 企（事）业单位贷款", "企（事）业单位贷款增加15.47万亿元，其中，短期贷款增加4.81万亿元"},
		{"v1 非金融企业及机关团体贷款", "非金融企业及机关团体贷款增加9.45万亿元，其中，短期贷款增加1.52万亿元"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := re.FindStringSubmatch(tc.sentence)
			require.NotNilf(t, m, "%q 必须命中", tc.sentence)
			for i, what := range []string{"方向词", "数值", "单位"} {
				assert.NotEmptyf(t, m[i+1], "%s 捕获组为空 —— 锚点交替没有用 `(?:…)`，"+
					"拼接后结合律错位成 `A|B(dir)(num)(unit)`，锚点单独命中而三个组全空", what)
			}
		})
	}
}

// moneyRE 必须同时认全角「（M2）」与半角「(M2)」。M1/M0 与 M2 共用 moneyRE，三项都断言。
//
// 全角实测 4 篇（2026-07 / 2026-04 / 2023-11 / 2023-10），半角 76 篇。
// ⚠️ `stripHTML` 的 punctNormalizer **不碰括号**（只归一逗号、分号、全角空格），
// 所以全角括号会原样到达模板层 —— 这个前提我读 strip.go 核实过，不是推断。
//
// ⚠️ 判据不能写成「能匹配到数字就行」：**两个值都要断言**，否则一个把括号连同代码
// 整段吞掉的正则（如 `广义货币.*余额`）照样绿。
func TestMoneyREAcceptsBothParenWidths(t *testing.T) {
	for _, tc := range []struct {
		name, file string
		want       map[string]float64
	}{
		{"全角（）：2026-07 月报", "pboc-2026-07-monthly.html", map[string]float64{
			FieldM2: 355.51, FieldM2YoY: 7.7,
			FieldM1: 115.46, FieldM1YoY: 4,
			FieldM0: 14.82, FieldM0YoY: 11.6,
		}},
		{"半角()：2025-12 年报（既有形态，必须仍绿）", "pboc-2025-12-annual.html", map[string]float64{
			FieldM2: 340.29, FieldM2YoY: 8.5,
			FieldM1: 115.51, FieldM1YoY: 3.8,
			FieldM0: 14.13, FieldM0YoY: 10.2,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := stripHTML(readTestdata(t, tc.file))
			got, err := extractMoneySection(realSection(t, text, "广义货币"))
			require.NoError(t, err)

			for field, want := range tc.want {
				assert.InDeltaf(t, want, got[field], 1e-6,
					"%s 取到 %v —— 余额与同比两个值都要对，只断言「有值」挡不住吞掉括号的正则",
					field, got[field])
			}
		})
	}
}

// tsfStockRE 必须容忍「余额为」与数字之间的一个空格。
//
// 实测 3 篇带空格（2019年 / 2020-01 / 2020-02 社融存量），全部是「对实体经济发放的
// 外币贷款折合人民币」这一项。
//
// 🔴 **语料里是两种字符，但到模板层只剩一种** —— 这个链条必须有测试连着：
//   - 2020-01 / 2020-02 用 `0x20` 普通空格；**2019 年那篇用 `0xa0` NO-BREAK SPACE**
//   - Go 的 `\s` = `[\t\n\f\r ]`，**不含 U+00A0** ⇒ 写 `\s*` 会静默漏掉 2019 那篇
//   - 但 `strip.go` 的 `spaceRE = [ \t\x{00a0}]+` 会把两者都折叠成**单个普通空格**
//
// ⇒ 模板只需容忍至多一个普通空格。**下面第一个子测试特意从 U+00A0 原文出发、经
// stripHTML 再匹配**：否则「模板只认 0x20」这件事的正确性依赖 strip.go 的折叠规则，
// 而没有任何测试连接两者，strip.go 改了这里会静默坏掉。
func TestTSFStockREToleratesSpaceAfterBalancePrefix(t *testing.T) {
	const item = "对实体经济发放的外币贷款折合人民币"
	re := tsfStockRE(item)

	for _, tc := range []struct{ name, raw string }{
		{"U+00A0（2019年那篇的真实形态），经 stripHTML 折叠", item + "余额为 2.11万亿元，同比下降4.6%"},
		{"普通空格（2020-01/02 的形态）", item + "余额为 2.11万亿元，同比下降4.6%"},
		{"无空格（既有形态，必须仍绿）", item + "余额为2.11万亿元，同比下降4.6%"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := re.FindStringSubmatch(stripHTML([]byte(tc.raw)))
			require.NotNilf(t, m, "必须命中 %q", tc.raw)
			assert.Equal(t, []string{"2.11", "万亿元", "下降", "4.6"}, m[1:],
				"四个捕获组必须逐字对上——空格被吞进数值捕获组会让 parsePlainAmount 报错")
		})
	}

	t.Run("阴性对照：占比句仍不得命中", func(t *testing.T) {
		assert.Nil(t, tsfStockRE("委托贷款").FindStringSubmatch("委托贷款余额占比2.6%，同比低0.1个百分点"),
			"「余额」后是「占」不是数字——放宽空格不得把占比段放进来")
	})
}

// 锚点找不到时必须**响亮失败**，且错误信息带得出是哪个锚点。
//
// 阴性对照的意义：抽不到若被当成「这期本来就没有」，会静默返回空 map，
// 下游拿到一份缺字段但无错误的结果，比报错难查得多。
func TestLoanScopeAnchorMissingIsLoudFailure(t *testing.T) {
	got, err := extractLoanSection(section{Body: "月末人民币贷款余额271.91万亿元，同比增长6.4%。" +
		"全年人民币贷款增加16.27万亿元。本段刻意不含任何作用域锚点。"})

	require.Error(t, err, "两个作用域都没有时必须报错，不能返回空 map 伪装成「这期没有」")
	assert.Nil(t, got, "报错时不得同时返回半份结果")
	assert.Contains(t, err.Error(), "loan scope anchor", "错误形态保持不变")
	assert.Contains(t, err.Error(), "住户", "错误信息必须带出锚点原文，否则无从诊断是哪一版称呼没认出")
}

// tsfStockRE 必须认「，同比持平」这一尾形（M1c-3a 的 TASK-005 functional[3](b)）。
//
// 实测 3 处，逐字取自语料：
//
//	2023年7月社融存量：未贴现的银行承兑汇票余额为2.55万亿元，同比持平
//	2025年6月社融存量：委托贷款余额为11.18万亿元，同比持平
//	2026年5月金融统计数据报告：委托贷款余额11.22万亿元，同比持平   ← v2 月报，**没有「为」字**
//
// 🔴 **放宽的方式有讲究**：全语料 50 处「同比持平」里 45+ 处在「从结构看」占比段。
// 做法是**保持「数值+单位」的结构要求**——占比句是 `余额占比2.6%`，「余额」后面是「占」
// 不是数字，在 **numPat 处即刻失败**，因此原样落在模板之外。阴性对照就钉这条。
//
// ⚠️ 订正（返工轮）：DoD 原先给的理由是「`持平` 进 dirPat 会让占比段变成候选、
// mustMatch 退化成多命中报错」。验证者真跑了那一半——**把 `持平` 加进 directionAlt 后
// 全语料结果与未变异逐字相同**。占比句在 numPat 就断了，**早于** dirPat，够不着方向词那一段。
// 结论（不把 `持平` 放进共用词表）仍然对，理由见 amount.go 的 directionFlat。
func TestTSFStockREAcceptsFlatYoY(t *testing.T) {
	for _, tc := range []struct {
		name, sentence, item string
		want                 []string
	}{
		{"2023-07 未贴现的银行承兑汇票（有「为」）", "未贴现的银行承兑汇票余额为2.55万亿元，同比持平",
			"未贴现的银行承兑汇票", []string{"2.55", "万亿元", "持平", ""}},
		{"2025-06 委托贷款（有「为」）", "委托贷款余额为11.18万亿元，同比持平",
			"委托贷款", []string{"11.18", "万亿元", "持平", ""}},
		{"2026-05 委托贷款（v2 月报，**无「为」**）", "委托贷款余额11.22万亿元，同比持平",
			"委托贷款", []string{"11.22", "万亿元", "持平", ""}},
		{"既有数值形态必须仍绿", "委托贷款余额为11.18万亿元，同比下降26.6%",
			"委托贷款", []string{"11.18", "万亿元", "下降", "26.6"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := tsfStockRE(tc.item).FindStringSubmatch(tc.sentence)
			require.NotNilf(t, m, "必须命中 %q", tc.sentence)
			assert.Equal(t, tc.want, m[1:],
				"四个捕获组：数值/单位/方向词/同比值。持平句的第四组是空串——"+
					"由 parseRatio 特判成 0，见 TestParseRatioTreatsFlatAsZeroWithoutMakingItADirectionWord")
		})
	}

	t.Run("🔴 阴性对照：占比段的「同比持平」仍不得命中", func(t *testing.T) {
		for _, tc := range []struct{ item, sentence string }{
			{"委托贷款", "委托贷款余额占比2.6%，同比持平"},
			{"未贴现的银行承兑汇票", "未贴现的银行承兑汇票余额占比1.2%，同比持平"},
		} {
			assert.Nilf(t, tsfStockRE(tc.item).FindStringSubmatch(tc.sentence),
				"%q 必须仍不命中：「余额」后面是「占」不是数字——"+
					"全语料 50 处「同比持平」有 45+ 处是这个形态——它们在 numPat 处即刻失败", tc.sentence)
		}
	})

	t.Run("阴性对照：`持平` 没有泄漏进全局 dirPat", func(t *testing.T) {
		assert.NotContains(t, dirPat, "持平",
			"dirPat 由 amount.go 的 directionAlt 拼出，`持平` 一旦进去，"+
				"社融增量/存款/贷款那几族模板都会跟着放宽")
	})
}

// tsfFlowRE 必须容忍方向词与数字之间的一个空格（M1c-3a 的 TASK-005 返工轮，boundary[1]）。
//
// 这是 `余额为 2.11万亿元` 那个变体的**同族**，但落在**另一条模板**上：
//
//	对实体经济发放的人民币贷款增加 16.88万亿元   ← 2019年 社融增量
//	对实体经济发放的人民币贷款增加 3.49万亿元    ← 2020年1月 社融增量
//
// 🔴 **码位实测：这 2 处都是 `0x20` 普通空格**，与存量那 3 篇不同（存量里 2019 年那份是
// `0xa0` NBSP）。两者最终都由 strip.go 的 `spaceRE = [ \t\x{00a0}]+` 折叠成单个普通空格，
// 所以模板侧统一用 ` ?`；但**码位是数出来的，不是推出来的**——DoD 要求先数再决定改法。
//
// # 为什么只改这一条模板，不动 sectorFlowRE / scopeTotalRE
//
// 全语料 218 篇扫「名称+方向词+空白+数字」这个形态：**命中恰好 2 处，全在社融增量报告、
// 全是 `0x20`、全是「对实体经济发放的人民币贷款增加」这一项**；金融统计数据报告类
// **0 处** ⇒ 存款分部门（sectorFlowRE）与贷款作用域（scopeTotalRE）不受影响，不动它们。
// 这是**范围的封闭性**，不是「我只找到这些」。
func TestTSFFlowREToleratesSpaceBeforeNumber(t *testing.T) {
	const item = "对实体经济发放的人民币贷款"
	re := tsfFlowRE(item)

	for _, tc := range []struct {
		name, raw string
		want      []string
	}{
		{"2019年 社融增量（0x20）", item + "增加 16.88万亿元，同比多增1.19万亿元", []string{"增加", "16.88", "万亿元"}},
		{"2020年1月 社融增量（0x20）", item + "增加 3.49万亿元，同比少增7442亿元", []string{"增加", "3.49", "万亿元"}},
		{"无空格（既有形态，必须仍绿）", item + "增加15.91万亿元", []string{"增加", "15.91", "万亿元"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := re.FindStringSubmatch(stripHTML([]byte(tc.raw)))
			require.NotNilf(t, m, "必须命中 %q", tc.raw)
			assert.Equal(t, tc.want, m[1:],
				"三个捕获组：方向词/数值/单位。空格被吞进数值捕获组会让 newAmount 报错")
		})
	}

	t.Run("阴性对照：同比干扰句仍不得命中", func(t *testing.T) {
		// 「同比多增1780亿元」的「多增」不在方向词白名单里，放宽空格不得把它放进来。
		assert.Nil(t, re.FindStringSubmatch(item+"同比多增 1780亿元"),
			"「多增」修饰的是同比，不是方向词——容忍空格不等于容忍别的词")
	})
}

// ── M1c-3a 的 TASK-012（QA R3）：「N-M月」累计前缀 ─────────────────────────
//
// Context Checkpoint: done_criteria → test mapping (M1c-3a 的 TASK-012)
// functional[0] periodAlt 认「N-M月」+ flowRE 容忍「累计」二字，**分别断言**
//                            → TestPeriodAltRecognisesRangePrefix（前半）
//                              TestFlowRETargetsCumulativeKeyword（后半）
// functional[1] cumulativePeriods 对新形态返回 true，且与 cumulativeRangeAlt 一一对应
//                            → TestCumulativePeriodsKeySet、TestCumulativeMonthAltAndCumulativePeriodsAgree
// boundary[1]   那 4 篇（2022-07/08/10/11）取值与正文逐字对得上
//                            → TestFlowREExtractsRangePrefixSamples
// boundary[2]   阴性对照：1月份 / 7月份 的判定不因本次放宽而改变
//                            → TestRangePrefixDoesNotDisturbMonthlyVerdicts

// TestPeriodAltRecognisesRangePrefix 只钉**前半个性质**：periodPat 认不认得「N-M月」。
//
// 与下面 TestFlowRETargetsCumulativeKeyword 分开，是因为 R3 的两处硬卡点**各自**
// 都会让整篇命中 0，症状相同而成因不同：这里管「期次前缀认不认得」，那里管
// 「认得之后整条句子能不能匹完」。合成一条断言的话，两处只改一处也会绿。
func TestPeriodAltRecognisesRangePrefix(t *testing.T) {
	anchored := regexp.MustCompile(`^` + periodPat + `$`)
	// 全语料实测出现的四种（各 2 篇：金融统计 + 社融增量，全部是 2022 年）
	for _, w := range []string{"1-7月", "1-8月", "1-10月", "1-11月"} {
		m := anchored.FindStringSubmatch(w)
		if assert.NotNilf(t, m, "periodPat 认不得 %q —— 该形态的累计句一条都不进候选，整篇命中 0", w) {
			assert.Equalf(t, w, m[1], "periodPat 对 %q 必须逐字整段捕获", w)
		}
		assert.Truef(t, cumulativePeriods[w], "%q 是累计口径，口径表里必须有", w)
	}
	// 未观测到但一并登记的七种：只登记见过的，将来出现会是**静默 0 命中**
	for _, w := range []string{"1-2月", "1-3月", "1-4月", "1-5月", "1-6月", "1-9月", "1-12月"} {
		assert.NotNilf(t, anchored.FindStringSubmatch(w), "periodPat 认不得 %q", w)
		assert.Truef(t, cumulativePeriods[w], "%q 不在口径表里", w)
	}
}

// TestFlowRETargetsCumulativeKeyword 只钉**后半个性质**：flowRE 容忍指标与方向词
// 之间的「累计」二字。
//
// 真实句是「1-8月，人民币贷款**累计**增加15.61万亿元」，而 dirPat 不含「累计」。
// **只放宽 periodAlt 不够**——本条用一个期次前缀**已经认得**（`全年`）的句子来测，
// 从而把两处硬卡点彻底分开：若只改了 periodAlt 没改这里，本条会红而上一条仍绿。
func TestFlowRETargetsCumulativeKeyword(t *testing.T) {
	re := flowRE("贷款")
	for _, tc := range []struct{ name, sentence, wantNum string }{
		{"带「累计」二字", "全年，人民币贷款累计增加15.61万亿元", "15.61"},
		{"不带（既有形态，不得回归）", "全年，人民币贷款增加16.27万亿元", "16.27"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := re.FindStringSubmatch(tc.sentence)
			require.NotNil(t, m, "flowRE 匹不上 %q", tc.sentence)
			assert.Equal(t, "全年", m[1])
			assert.Equal(t, currencyRMB, m[2])
			assert.Equal(t, "增加", m[3], "「累计」不该被当成方向词捕获——它是口径词")
			assert.Equal(t, tc.wantNum, m[4])
		})
	}
}

// TestFlowREExtractsRangePrefixSamples 覆盖 boundary[1]：那 4 篇的**取值**与正文逐字对得上。
//
// ⚠️ 断言的是**值**而不是「不报错了」——「不报错」与「抽对了」是两个性质，
// 一个把所有值都填成同一个数的实现同样不报错。
//
// ⚠️ 用逐字抄自正文的句子而不是快照：那 4 篇（2022-07/08/10/11 金融统计数据报告）
// 不在 testdata 里，而 testdata 不在 M1c-3a 的 TASK-012 的 writes 内。
//
// 🔴 **四句连同「同比多增…」尾巴都是从语料正文原样取出的**（正则
// `1-[0-9]{1,2}月[^。]{0,60}` 跑全语料，输出见 discovery）。第一版我照记忆写尾巴，
// 四句错了三句 —— 而 flowRE 到单位就收尾，**两种写法测试都绿**，这个错永远不会被
// 断言抓到。「逐字抄自正文」这句话本身没有守卫，只能靠真去取一次。
func TestFlowREExtractsRangePrefixSamples(t *testing.T) {
	for _, tc := range []struct{ period, sentence, wantNum string }{
		{"1-7月", "1-7月，人民币贷款累计增加14.35万亿元，同比多增5150亿元", "14.35"},
		{"1-8月", "1-8月，人民币贷款累计增加15.61万亿元，同比多增5540亿元", "15.61"},
		{"1-10月", "1-10月，人民币贷款累计增加18.7万亿元，同比多增1.15万亿元", "18.7"},
		{"1-11月", "1-11月，人民币贷款累计增加19.91万亿元，同比多增1.09万亿元", "19.91"},
	} {
		t.Run(tc.period, func(t *testing.T) {
			m, err := selectRMBCumulativeFlow(flowRE("贷款"), tc.sentence, "贷款期内合计")
			require.NoError(t, err,
				"这一句是该期唯一的累计合计句，抽不到它就等于判定「本期没有累计数据」——"+
					"而数据就在正文里，那正是 QA R3 指出的假结论")
			assert.Equal(t, tc.period, m[1], "期次前缀必须逐字捕获")
			assert.Equal(t, currencyRMB, m[2])
			assert.Equal(t, "增加", m[3])
			assert.Equal(t, tc.wantNum, m[4], "数值必须与正文逐字一致")
			assert.Equal(t, "万亿元", m[5])
		})
	}
}

// TestRangePrefixDoesNotDisturbMonthlyVerdicts 是 boundary[2] 的阴性对照：
// 本次放宽**不得**改变既有月份前缀的判定。
//
// ⚠️ 「1月份」是零特判的（M1c-3a 的 TASK-001）：它就是 cumulativePeriods 里的一个普通键，
// 代码里针对 1 月的分支 0 处。本次也**没有**为「N-M月」加任何特判分支——它同样只是
// cumulativeRangeAlt 里的一批普通交替项 + 口径表里的一批普通键。
func TestRangePrefixDoesNotDisturbMonthlyVerdicts(t *testing.T) {
	anchored := regexp.MustCompile(`^` + periodPat + `$`)
	for _, tc := range []struct {
		prefix     string
		cumulative bool
		why        string
	}{
		{"1月份", true, "1 月的年初至今累计就是当月，报告与当月句同形"},
		{"7月份", false, "当月口径，装进 *_ytd 会让累计字段拿到单月数"},
		{"11月份", false, "同上；且它含「1月份」子串，若被切错会误判成累计"},
		{"1-7月", true, "2022 年月报的累计写法"},
	} {
		t.Run(tc.prefix, func(t *testing.T) {
			m := anchored.FindStringSubmatch(tc.prefix)
			require.NotNilf(t, m, "periodPat 必须认得 %q（认不出的话它根本不进候选）", tc.prefix)
			assert.Equalf(t, tc.prefix, m[1], "必须逐字整段捕获 %q", tc.prefix)
			assert.Equalf(t, tc.cumulative, cumulativePeriods[tc.prefix], "%s", tc.why)
		})
	}

	// ⚠️ 「零特判」这条性质本身**没有机械守卫**，只有上面这张表在守它的**后果**
	// （新旧前缀的判定都只由 cumulativePeriods 决定）。我试过用 stringLiteralsOf 扫
	// profiles.go 断言「不出现月份字面量」，但 "1月份" 本来就是口径表里的合法键，
	// 那条断言必然为假 ⇒ 写不出来。这里如实记下，而不是放一句看起来在断言的代码。
}
