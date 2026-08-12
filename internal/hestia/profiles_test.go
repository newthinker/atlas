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
