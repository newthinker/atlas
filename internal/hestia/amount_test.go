package hestia

// Context Checkpoint: done_criteria → test mapping (TASK-004 amount)
// functional[0]      符号只来自方向词，数字本身恒正        → TestNewAmountSignDimensionOnly
// functional[0]      多增/少增/少减 修饰同比，必须被拒     → TestNewAmountRejectsComparativeDirections、
//                                                          TestComparativeSentenceDoesNotMatchDirectionTemplate
// functional[0]      toYi/toWanYi 按单位换算              → TestNewAmountScaleDimensionOnly、TestAmountConversionKeepsPrecision
// functional[1]      parseRatio 百分数 / parsePlainNumber 裸数字
//                                                        → TestParseRatio、TestParseRatioRejects、TestParsePlainNumber
// functional[2]      directionAlt/unitAlt 是可嵌入片段常量 → TestDirectionAltAndUnitAltAreEmbeddable
// functional[2]      两张表与两个片段不得脱节              → TestDirectionSignCoversDirectionAlt、TestUnitScaleCoversUnitAlt
// boundary[0]        「净融资」交替顺序（实测无语义）      → TestAlternationOrderHasNoEffect
// boundary[0]        真正的守卫条件：无前缀关系            → TestNoAlternativeIsPrefixOfAnother
// boundary[0]        真正的失效模式：贪婪名称捕获          → TestGreedyNameCaptureSwallowsNetPrefix
// error_handling[0]  表外方向词报错且错误信息含该词原文    → TestNewAmountRejectsUnknownDirection
// error_handling[1]  单位空串/表外报错，不默认某单位        → TestNewAmountRejectsUnknownUnit
// error_handling[*]  数字不可解析/带符号/非有限一律报错     → TestNewAmountRejectsBadNumber、
//                                                          TestNewAmountRejectsSignedNumber、TestNewAmountRejectsNonFiniteNumber
// error_handling[2]  G6：无方向词句式走独立恒正入口        → TestParsePlainAmount
// error_handling[2]  G6：字面量方向词是可静态检出的违规    → TestNoLiteralDirectionWordAtProductionCallSites
// error_handling[*]  零值 amount 不得静默算成 0            → TestAmountZeroValueRefusesToScale
// non_functional[0]  符号与量纲各自独立可测 —— 由 TestNewAmountSignDimensionOnly（只断言未换算的
//                    符号与量值，不碰 unitScale）与 TestNewAmountScaleDimensionOnly（只断言绝对值，
//                    不碰 directionSign）这一对钉住：改坏任一张表，只会有对应的那一组转红。

import (
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// numPatForTest 复刻 T5 模板里的数字片段，用于把 directionAlt / unitAlt 嵌进
// 真实形态的模板里测。它**不是**给生产代码用的常量——T5 自己声明 numPat。
const numPatForTest = `([0-9]+(?:\.[0-9]+)?)`

// --- 符号维度：只断言未换算的值，完全不经过 unitScale ---

// TestNewAmountSignDimensionOnly 只考察「方向词 → 符号」这一个维度。
//
// 断言对象是 a.value（尚未换算），因此**任何对 unitScale 的改动都不会影响本测试**。
// 这正是 non_functional[0] 要的独立性：单位表写对了也掩盖不了符号表写错。
//
// 「非银行业金融机构存款减少7446亿元」若漏读方向词会得到 +7446 而非 −7446：
// 量级正确、方向相反，七道闸门一道都拦不住。
func TestNewAmountSignDimensionOnly(t *testing.T) {
	for _, tc := range []struct {
		dir  string
		want float64
	}{
		{"增加", +1},
		{"减少", -1},
		{"增长", +1},
		{"下降", -1},
		{"净融资", +1},
		{"融资", +1},
	} {
		t.Run(tc.dir, func(t *testing.T) {
			a, err := newAmount(tc.dir, "1103", "亿元")
			require.NoError(t, err)
			assert.Equal(t, tc.want*1103, a.value,
				"符号只能来自方向词——数字本身永远是正的")
		})
	}
}

// TestNewAmountScaleDimensionOnly 只考察「单位 → 倍率」这一个维度。
//
// 断言对象一律取绝对值，因此**任何对 directionSign 的改动都不会影响本测试**——
// 与 TestNewAmountSignDimensionOnly 互为正交的一半。
func TestNewAmountScaleDimensionOnly(t *testing.T) {
	for _, tc := range []struct {
		unit              string
		num               string
		wantYi, wantWanYi float64
	}{
		// 「住户存款增加14.64万亿元」与「财政性存款增加6579亿元」在同一句里
		{"万亿元", "14.64", 146400, 14.64},
		{"亿元", "6579", 6579, 0.6579},
		// fx_reserve 用「万亿美元」、外币存贷款用「亿美元」：各自成列、不与
		// 人民币混算，所以只需要量级换算，不需要币种区分
		{"万亿美元", "3.29", 32900, 3.29},
		{"亿美元", "980", 980, 0.098},
	} {
		t.Run(tc.unit, func(t *testing.T) {
			a, err := newAmount("增加", tc.num, tc.unit)
			require.NoError(t, err)
			assert.Equal(t, tc.wantYi, math.Abs(a.toYi()), "归一到亿元")
			assert.Equal(t, tc.wantWanYi, math.Abs(a.toWanYi()), "归一到万亿元")
		})
	}
}

// TestAmountConversionKeepsPrecision 钉住两条真实换算路径的精度。
func TestAmountConversionKeepsPrecision(t *testing.T) {
	t.Run("余额类归一到万亿元", func(t *testing.T) {
		a, err := newAmount("增长", "328.64", "万亿元")
		require.NoError(t, err)
		assert.Equal(t, 328.64, a.toWanYi())

		// 若某期把余额写成亿元，也要能正确降级
		b, err := newAmount("增长", "3286400", "亿元")
		require.NoError(t, err)
		assert.InDelta(t, 328.64, b.toWanYi(), 1e-9)
	})

	t.Run("增量类归一到亿元", func(t *testing.T) {
		// 15.91 万亿 → 159100，浮点乘法不应引入可见误差
		a, err := newAmount("增加", "15.91", "万亿元")
		require.NoError(t, err)
		assert.InDelta(t, 159100.0, a.toYi(), 1e-6)
	})
}

// TestNewAmountCombinesSignAndScale 是符号×量纲的合成断言。
//
// 两个维度各自独立可测之外，仍需要一处把它们合起来钉住，否则「各自对、合起来
// 乘反了」这类错误没人接。
func TestNewAmountCombinesSignAndScale(t *testing.T) {
	a, err := newAmount("减少", "7446", "亿元")
	require.NoError(t, err)
	assert.Equal(t, -7446.0, a.toYi())

	b, err := newAmount("下降", "1.5", "万亿元")
	require.NoError(t, err)
	assert.Equal(t, -15000.0, b.toYi())
}

// --- 方向词白名单 ---

// TestNewAmountRejectsComparativeDirections 钉住 functional[0] 的那条修正。
//
// 「多增」「少增」「少减」修饰的是**同比**——「同比多增1780亿元」「同比少减1873亿元」
// ——它们是必须被排除的干扰词，不是方向词。若把它们加进白名单，T5 的 tsfFlowRE
// 等模板会匹配到「同比多增1780亿元」，直接触发重复赋值或取错值。
func TestNewAmountRejectsComparativeDirections(t *testing.T) {
	for _, dir := range []string{"多增", "少增", "少减"} {
		t.Run(dir, func(t *testing.T) {
			_, err := newAmount(dir, "1780", "亿元")
			require.Errorf(t, err, "%q 修饰同比，不是方向词，必须被拒", dir)

			assert.NotContains(t, directionAlt, dir,
				"这些词一旦进入 directionAlt，模板就会匹配到「同比」子句")
		})
	}
}

// TestComparativeSentenceDoesNotMatchDirectionTemplate 把上一条的**后果**直接演给看。
//
// 用 directionAlt 拼一个 T5 形态的模板去扫真实的同比句：必须一无所获。
// 只断言 newAmount 拒收还不够——真正会出事的是模板在同比子句上命中。
func TestComparativeSentenceDoesNotMatchDirectionTemplate(t *testing.T) {
	re := regexp.MustCompile(`(` + directionAlt + `)` + numPatForTest + `(` + unitAlt + `)`)

	for _, sentence := range []string{
		"同比多增1780亿元",
		"同比少增2043亿元",
		"同比少减1873亿元",
	} {
		assert.Emptyf(t, re.FindString(sentence), "同比子句 %q 不得被方向词模板命中", sentence)
	}
}

func TestNewAmountRejectsUnknownDirection(t *testing.T) {
	// 央行若某期改用「净减少」「回落」，必须响亮失败。默认为正会让
	// −7446 变 +7446，而那是完全无声的错误。
	for _, dir := range []string{"净减少", "回落", "上升", "", "增", "持平"} {
		t.Run(strconv.Quote(dir), func(t *testing.T) {
			_, err := newAmount(dir, "100", "亿元")
			require.Errorf(t, err, "方向词 %q 应被拒", dir)
			assert.Contains(t, err.Error(), "direction")
			// 用 %q 形态断言「错误信息含该词原文」：直接 Contains(err, dir)
			// 在 dir 为空串时**恒真**，那是一条假绿的断言。
			assert.Contains(t, err.Error(), strconv.Quote(dir),
				"错误信息必须带上被拒的词本身，否则排障时看不出是哪个词")
		})
	}
}

// --- 单位白名单 ---

func TestNewAmountRejectsUnknownUnit(t *testing.T) {
	// 默认某个单位 = 10000× 误差风险，而 SQLite 的 REAL 亲和性正确，没有任何类型信号
	for _, unit := range []string{"", "元", "万元", "百万元", "亿", "万亿"} {
		t.Run(strconv.Quote(unit), func(t *testing.T) {
			_, err := newAmount("增加", "100", unit)
			require.Errorf(t, err, "单位 %q 应被拒", unit)
			assert.Contains(t, err.Error(), "unit")
			assert.Contains(t, err.Error(), strconv.Quote(unit))
		})
	}
}

// --- 数字 ---

func TestNewAmountRejectsBadNumber(t *testing.T) {
	for _, num := range []string{"", "abc", "1.2.3", "一点五", "N/A"} {
		t.Run(strconv.Quote(num), func(t *testing.T) {
			_, err := newAmount("增加", num, "亿元")
			require.Errorf(t, err, "数字 %q 应被拒", num)
		})
	}
}

// TestNewAmountRejectsSignedNumber 把「符号只来自方向词」钉在**原语**上，
// 而不是寄望于 T5 的 numPat 恰好捕不到负号。
//
// 今天的 numPat = ([0-9]+(?:\.[0-9]+)?) 确实表示不了前导负号，但那是另一个文件里
// 的另一条约束。2025 年原文注5 的「M1 可比口径回溯」表就含显式负号数字（−0.8%、
// −1.7%）；哪天有人为了够到那张表把 numPat 放宽成 (-?[0-9]+…)，符号就会同时从
// 两个来源进来——本测试会在那一刻转红。
func TestNewAmountRejectsSignedNumber(t *testing.T) {
	for _, num := range []string{"-100", "+100", "-0.8", "-0"} {
		t.Run(num, func(t *testing.T) {
			_, err := newAmount("增加", num, "亿元")
			require.Errorf(t, err, "带符号的数字 %q 应被拒——符号只能来自方向词", num)
		})
	}
}

// TestNewAmountRejectsNonFiniteNumber：ParseFloat 接受 "NaN"/"Inf" 且不报错。
// 一个 NaN 落进 Observation.Values 是典型的静默坏值——下游校验查的是恒等式与
// 量级区间，NaN 参与的比较一律为 false，闸门会直接放行。
func TestNewAmountRejectsNonFiniteNumber(t *testing.T) {
	for _, num := range []string{"NaN", "nan", "Inf", "inf", "+Inf", "Infinity"} {
		t.Run(num, func(t *testing.T) {
			_, err := newAmount("增加", num, "亿元")
			require.Errorf(t, err, "非有限值 %q 应被拒", num)
		})
	}
}

// --- 比率与裸数字 ---

func TestParseRatio(t *testing.T) {
	v, err := parseRatio("增长", "8.3")
	require.NoError(t, err)
	assert.Equal(t, 8.3, v)

	v, err = parseRatio("下降", "18")
	require.NoError(t, err)
	assert.Equal(t, -18.0, v, "「同比下降18%」的符号同样来自方向词")
}

// ⚠️ **`持平` 曾经在下面这张拒绝清单里，M1c-3a 的 TASK-005 把它移走了** —— 这是一次
// 有意的语义反转，不是把守卫改松，理由留在原地免得后人「修回去」：
//
//   - 当时它被拒，是因为它**不在方向词白名单里**，而不是因为「持平句永远不该被解析」。
//     那时也确实没有任何模板会把 `持平` 送进来。
//   - 现在 tsfStockRE 会送它进来（3 处真实语料，见 TestTSFStockREAcceptsFlatYoY），
//     语义是明确的：变化率为零。
//   - **它仍然不在白名单里** —— 这条连续性才是关键，由
//     TestParseRatioTreatsFlatAsZeroWithoutMakingItADirectionWord 钉住。
//
// 与它形近的 `走平` / `回落` 等仍在本清单内：只有 `持平` 这一个词被特判。
func TestParseRatioRejects(t *testing.T) {
	t.Run("方向词走同一张白名单", func(t *testing.T) {
		for _, dir := range []string{"多增", "回落", ""} {
			_, err := parseRatio(dir, "0")
			require.Errorf(t, err, "比率的方向词 %q 应被拒", dir)
			assert.Contains(t, err.Error(), strconv.Quote(dir))
		}
	})

	t.Run("数字同样不得自带符号或非有限", func(t *testing.T) {
		for _, num := range []string{"-0.8", "+1", "NaN", "abc", ""} {
			_, err := parseRatio("增长", num)
			require.Errorf(t, err, "比率数字 %q 应被拒", num)
		}
	})
}

func TestParsePlainNumber(t *testing.T) {
	// 利率与汇率没有方向词也没有单位：「加权平均利率为1.36%」「1美元兑7.0288元人民币」
	v, err := parsePlainNumber("1.36")
	require.NoError(t, err)
	assert.Equal(t, 1.36, v)

	v, err = parsePlainNumber("7.0288")
	require.NoError(t, err)
	assert.Equal(t, 7.0288, v)

	for _, num := range []string{"N/A", "", "NaN", "Inf", "一点五"} {
		_, err := parsePlainNumber(num)
		require.Errorf(t, err, "%q 应被拒", num)
	}
}

// --- G6：无方向词句式的独立恒正入口 ---

// TestParsePlainAmount 覆盖 error_handling[2]。
//
// 余额/存量句原文根本没有方向词：「社会融资规模存量为430.22万亿元」「广义货币
// (M2)余额328.64万亿元」。需求文档在这类句子上写的是把字面量「增加」传进
// newAmount ——那等于在约四分之一的调用点上把方向词白名单**结构性关闭**：
// 白名单在那里永远不可能触发，而孤立测 newAmount 全绿，看不见喂进去的是常量。
//
// parsePlainAmount 让「这里没有方向词可读」成为一件写在函数名上、可枚举、可审计
// 的事，而不是一个伪装成正常调用的常量。
func TestParsePlainAmount(t *testing.T) {
	t.Run("恒正", func(t *testing.T) {
		a, err := parsePlainAmount("430.22", "万亿元")
		require.NoError(t, err)
		assert.Equal(t, 430.22, a.toWanYi())
		assert.Positive(t, a.value)
	})

	t.Run("与 newAmount 共用同一张单位表", func(t *testing.T) {
		p, err := parsePlainAmount("6579", "亿元")
		require.NoError(t, err)
		n, err := newAmount("增加", "6579", "亿元")
		require.NoError(t, err)
		assert.Equal(t, n.toYi(), p.toYi(), "两个入口的量纲换算必须完全一致")
	})

	t.Run("单位与数字的校验一条不少", func(t *testing.T) {
		for _, tc := range []struct{ num, unit string }{
			{"100", ""},
			{"100", "万元"},
			{"", "亿元"},
			{"abc", "亿元"},
			{"-100", "亿元"}, // 恒正入口收到负号 = 上游 numPat 被放宽了，必须响亮失败
			{"NaN", "亿元"},
		} {
			_, err := parsePlainAmount(tc.num, tc.unit)
			require.Errorf(t, err, "parsePlainAmount(%q, %q) 应被拒", tc.num, tc.unit)
		}
	})
}

// TestNoLiteralDirectionWordAtProductionCallSites 让 G6 成为**可静态检出**的违规。
//
// 只扫非测试文件：测试本身必须能用字面量直接喂 newAmount，受约束的是生产调用点。
// 这条断言在 T5 写 extract.go 时才真正开始发力——那 6 处
// 「把字面量『增加』传进 newAmount」会在这里立刻转红，而不是等到某期央行把
// 余额句改成「较上年末减少」时，用一个硬编码的正号静默算错。
func TestNoLiteralDirectionWordAtProductionCallSites(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	// 取符号的两个入口；parsePlainNumber / parsePlainAmount 不读方向词，不在此列。
	needles := []string{`newAmount("`, `parseRatio("`}

	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		require.NoError(t, err)
		scanned++

		for _, needle := range needles {
			assert.NotContainsf(t, string(src), needle,
				"%s 把字面量方向词传进了取符号的入口——无方向词的句式请改用 parsePlainAmount，"+
					"硬编码一个白名单内的词等于把方向词校验关掉", name)
		}
	}

	// 自证：扫到 0 个文件说明扫描本身失效，此时「零命中」毫无意义。
	require.NotZero(t, scanned, "没有扫到任何生产文件，本测试的绿色是假的")
}

// --- 正则片段：可嵌入性、两表一致、交替顺序 ---

// TestDirectionAltAndUnitAltAreEmbeddable 覆盖 functional[2]。
//
// T5 的 7 类模板会把这两个片段拼进自己的正则；本测试用 T5 的真实拼法
// （QuoteMeta(名称) + 方向 + 数字 + 单位）跑真实样本句，确保片段可直接嵌入、
// 捕获组能原样喂进 newAmount。片段只此一份，T5 不得再写第二份。
func TestDirectionAltAndUnitAltAreEmbeddable(t *testing.T) {
	for _, tc := range []struct {
		name     string
		sentence string
		wantYi   float64
	}{
		{"委托贷款", "其中，委托贷款增加1203亿元", 1203},
		{"企业债券", "企业债券净融资2.39万亿元", 23900},
		{"非金融企业境内股票", "非金融企业境内股票融资4763亿元", 4763},
		{"非银行业金融机构存款", "非银行业金融机构存款减少7446亿元", -7446},
	} {
		t.Run(tc.name, func(t *testing.T) {
			re := regexp.MustCompile(
				regexp.QuoteMeta(tc.name) + `(` + directionAlt + `)` + numPatForTest + `(` + unitAlt + `)`)

			m := re.FindStringSubmatch(tc.sentence)
			require.Len(t, m, 4, "片段必须能嵌进模板并各自成组")

			a, err := newAmount(m[1], m[2], m[3])
			require.NoError(t, err)
			assert.Equal(t, tc.wantYi, a.toYi())
		})
	}
}

// TestDirectionSignCoversDirectionAlt 确保两处不脱节。
//
// directionAlt 用于正则、directionSign 用于查符号；一个加了词而另一个没加，
// 会让匹配成功却查不到符号——那是运行时错误，不是编译错误。
func TestDirectionSignCoversDirectionAlt(t *testing.T) {
	words := strings.Split(directionAlt, "|")
	for _, w := range words {
		_, ok := directionSign[w]
		assert.Truef(t, ok, "directionAlt 里的 %q 在 directionSign 中没有符号", w)
	}
	assert.Len(t, directionSign, len(words),
		"两处词数必须一致，多出的项说明有词只在一处登记")
}

// TestUnitScaleCoversUnitAlt 是单位侧的同一条约束。
func TestUnitScaleCoversUnitAlt(t *testing.T) {
	units := strings.Split(unitAlt, "|")
	for _, u := range units {
		_, ok := unitScale[u]
		assert.Truef(t, ok, "unitAlt 里的 %q 在 unitScale 中没有倍率", u)
	}
	assert.Len(t, unitScale, len(units),
		"两处词数必须一致，多出的项说明有单位只在一处登记")
}

// TestAlternationOrderHasNoEffect 记录一条**被实测推翻的假设**。
//
// 需求文档与任务 DoD 都断言：「净融资」必须排在「融资」之前，否则
// "企业债券净融资2.39万亿元" 会在「融资」处切开、名称变成「企业债券净」而静默
// 漏掉一个字段。实测：把交替顺序完全写反，四种模板形态（无锚点扫描、固定名称、
// 非贪婪名称捕获、贪婪名称捕获）的输出**逐字节相同**。
//
// 根因：Go 的 regexp 是 leftmost-first，交替顺序只在两个分支能在**同一起始位置**
// 都匹配时才有语义——也就是一个是另一个的**前缀**时。「融资」是「净融资」的
// **后缀**，不是前缀；在「净」那个位置上「融资」压根匹配不上。
//
// 所以「顺序即语义」在当前词表下不成立。真正的守卫条件是
// TestNoAlternativeIsPrefixOfAnother，真正的失效模式是
// TestGreedyNameCaptureSwallowsNetPrefix。本测试留在这里，是为了让下一个想按
// 「长词在前」这条错误理由改动代码的人先看到证据。
func TestAlternationOrderHasNoEffect(t *testing.T) {
	const sentence = "企业债券净融资2.39万亿元"
	reversed := reverseAlternation(directionAlt)
	require.NotEqual(t, directionAlt, reversed, "对照组必须真的和原表不同")

	build := func(alt string) []*regexp.Regexp {
		return []*regexp.Regexp{
			regexp.MustCompile(`(` + alt + `)`),
			regexp.MustCompile(regexp.QuoteMeta("企业债券") + `(` + alt + `)` + numPatForTest + `(` + unitAlt + `)`),
			regexp.MustCompile(`^(.+?)(` + alt + `)` + numPatForTest + `(` + unitAlt + `)$`),
		}
	}

	good, bad := build(directionAlt), build(reversed)
	for i := range good {
		assert.Equalf(t, good[i].FindStringSubmatch(sentence), bad[i].FindStringSubmatch(sentence),
			"形态 %d：交替顺序写反后结果仍然相同 —— 顺序在当前词表下没有语义", i)
	}
}

// TestNoAlternativeIsPrefixOfAnother 是「顺序即语义」真正该守的那条不变量。
//
// 只要没有任何一项是另一项的前缀，交替顺序就无关紧要（见
// TestAlternationOrderHasNoEffect）。一旦有人往 directionAlt 加「增」（「增加」
// 「增长」的前缀）或往 unitAlt 加「亿」「万亿」（「亿元」「万亿元」的前缀），
// 顺序**立刻**开始决定语义，届时本测试转红，提示必须把长词排到前面。
func TestNoAlternativeIsPrefixOfAnother(t *testing.T) {
	for _, tc := range []struct{ name, alt string }{
		{"directionAlt", directionAlt},
		{"unitAlt", unitAlt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			words := strings.Split(tc.alt, "|")
			for _, short := range words {
				for _, long := range words {
					if short == long {
						continue
					}
					assert.Falsef(t, strings.HasPrefix(long, short),
						"%s: %q 是 %q 的前缀 —— 从此刻起交替顺序有语义，长词必须排在前面",
						tc.name, short, long)
				}
			}
		})
	}
}

// TestGreedyNameCaptureSwallowsNetPrefix 演示**真正**会漏掉企业债券那一项的写法。
//
// 名称捕获若用贪婪的 (.+)，引擎优先让名称吃掉「净」、再用「融资」收尾，于是名称
// 变成「企业债券净」——匹配不上任何清单项，**静默漏一个字段**。这与交替顺序无关：
// 两种顺序下都会发生。T5 若用固定名称（QuoteMeta）或非贪婪 (.+?) 就不会踩到。
func TestGreedyNameCaptureSwallowsNetPrefix(t *testing.T) {
	const sentence = "企业债券净融资2.39万亿元"
	// 清单名的最小对照样本；字段清单本体在 T5 的 profiles.go，这里只需要
	// 「『企业债券净』不在清单里」这一个事实。
	listNames := []string{"企业债券", "非金融企业境内股票", "委托贷款", "信托贷款"}

	greedy := regexp.MustCompile(`^(.+)(` + directionAlt + `)` + numPatForTest + `(` + unitAlt + `)$`)
	m := greedy.FindStringSubmatch(sentence)
	require.Len(t, m, 5)
	assert.Equal(t, "企业债券净", m[1], "贪婪捕获会把「净」吃进名称")
	assert.NotContains(t, listNames, m[1],
		"名称被吃掉一个字后匹配不上任何清单项 —— 字段静默丢失，没有任何报错")

	lazy := regexp.MustCompile(`^(.+?)(` + directionAlt + `)` + numPatForTest + `(` + unitAlt + `)$`)
	m = lazy.FindStringSubmatch(sentence)
	require.Len(t, m, 5)
	assert.Equal(t, "企业债券", m[1], "非贪婪捕获才拿得到正确名称")
	assert.Equal(t, "净融资", m[2])
	assert.Contains(t, listNames, m[1])
}

// --- 零值 ---

// TestAmountZeroValueRefusesToScale：零值 amount 只可能来自「忽略了构造函数的
// error」。此时若让 toYi 返回 0，就会有一个看起来完全合理的 0 静默流进 Values，
// 而这正是本任务通篇在防的那类错误。宁可炸在调用点。
func TestAmountZeroValueRefusesToScale(t *testing.T) {
	assert.Panics(t, func() { _ = amount{}.toYi() })
	assert.Panics(t, func() { _ = amount{}.toWanYi() })
	assert.Panics(t, func() { _ = amount{value: 1, unit: "万元"}.toYi() })
}

// reverseAlternation 把交替片段的分支顺序整个颠倒，用作对照组。
func reverseAlternation(alt string) string {
	words := strings.Split(alt, "|")
	for i, j := 0, len(words)-1; i < j; i, j = i+1, j-1 {
		words[i], words[j] = words[j], words[i]
	}
	return strings.Join(words, "|")
}

// —— M1c-3a 的 TASK-005：`同比持平` ⇒ 变化率为零 ——
//
// Context Checkpoint: done_criteria → test mapping（M1c-3a 的 TASK-005 functional[3](b)）
// 约束①  `持平` **不得**进入 directionSign / directionAlt
//                                   → TestParseRatioTreatsFlatAsZeroWithoutMakingItADirectionWord
// 约束②  「parseRatio 认它」与「它不是方向词」同时为真，需一条断言钉住
//                                   → 同上
// 约束③  特判必须能区分「持平」与「解析失败」
//                                   → 同上的「空方向词仍须报错」子测试
//
// 三条约束的来源：team-lead 2026-08-26 的裁决消息，已由我逐字转录进
// `.arcforge/tasks/TASK-005.json` 的 `questions[0].answer`（标注了非 leader 亲笔）。

// 🔴 `同比持平` = 变化率为零，但 `持平` **不是方向词**——两件事同时为真是刻意的。
//
// 报告里有 3 处分项余额句写「…余额为2.55万亿元，同比持平」（2023-07 / 2025-06 存量，
// 以及 2026-05 v2 月报社融板块那句**没有「为」字**的）。它们既无方向词也无数字。
//
// # 为什么在 parseRatio 开头特判，而不是把 `持平` 加进那两张表
//
// ⚠️ **裁决当时给的理由已被实测证伪，这里是订正版**：原说法是「`持平` 进 directionAlt
// ⇒ 占比段那 45+ 处全变成候选 ⇒ mustMatch 多命中报错」。验证者把 `持平` 加进 directionAlt
// 真跑了一遍，**全语料结果与未变异逐字相同**——占比句「余额**占**比2.6%」在 tsfStockRE 的
// **numPat 处即刻失败**，早于 dirPat，够不着方向词那一段。
//
// 结论不变，理由换成实测支持的三条：① `持平` **没有符号语义**（directionSign 每项都是 ±1）；
// ② 那两张表**多族模板共用**，加词的影响面远大于本任务；③ 两张表由
// TestDirectionSignCoversDirectionAlt 钉成双射，只加一处就会红——**那条既有测试才是真守卫**。
//
// 顺带：不动那两张表，TestDirectionSignCoversDirectionAlt 的双射断言也不受影响。
//
// ⚠️ **这条断言存在的理由**：后人看到 parseRatio 认得 `持平`，很自然会想「那它怎么
// 不在 directionAlt 里」并顺手加进去——那恰好破坏上面那条。把否定结论钉在这里。
func TestParseRatioTreatsFlatAsZeroWithoutMakingItADirectionWord(t *testing.T) {
	t.Run("持平 ⇒ 0，且无 error", func(t *testing.T) {
		got, err := parseRatio(directionFlat, "")
		require.NoError(t, err)
		assert.Equal(t, 0.0, got, "「同比持平」的变化率就是 0，不是缺失、不是跳过")
	})

	t.Run("但它不是方向词：两张表里都没有", func(t *testing.T) {
		_, ok := directionSign[directionFlat]
		assert.Falsef(t, ok, "%q 不得登记进 directionSign", directionFlat)
		assert.NotContainsf(t, directionAlt, directionFlat,
			"%q 不得进入 directionAlt：它没有符号语义（directionSign 每项都是 ±1），"+
				"而这两张表是多族模板共用的词表；且它们被 TestDirectionSignCoversDirectionAlt "+
				"钉成双射，只加一处必红", directionFlat)
	})

	t.Run("约束③：空方向词仍须报错，不得被特判吞成 0", func(t *testing.T) {
		// 正则没匹上时捕获组是空串。若把特判写成「num 为空就返回 0」，
		// 这里会静默返回 0 —— 把「无从解析」变成「解析出 0」，是最坏的那种沉默。
		_, err := parseRatio("", "")
		require.Error(t, err, "空方向词是解析失败，必须与「持平」走不同的路")
		assert.Contains(t, err.Error(), "unknown direction word", "错误形态保持不变")
	})

	t.Run("约束③续：表外方向词仍须报错", func(t *testing.T) {
		_, err := parseRatio("走平", "")
		require.Error(t, err, "只有 `持平` 这一个词被特判，形近词不得跟着放行")
	})
}
