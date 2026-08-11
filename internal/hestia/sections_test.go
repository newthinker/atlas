package hestia

// Context Checkpoint: done_criteria → test mapping (sections)
//
// functional[0]     2025 切 8 段 / 2020 切 6 段，Title 与原文小节标题一致
//                                        → TestSplitSectionsOnRealSamples
// functional[0]     标题与正文正确分离，正文不越界到下一节
//                                        → TestSplitSectionsSeparatesTitleAndBody
// functional[1]     detectExtractor 三分支（v2 / v1 / error）
//                                        → TestDetectExtractor
//                                        + TestDetectExtractorRejectsUnknownLayout
// functional[2]     **T5 的全部 7 个关键词 × 两份样本**，逐个断言命中板块
//                                        → TestFindSectionResolvesAllT5Keywords
// functional[2]     正文提及 ≠ 板块主题（2025「人民币贷款」的真实反例）
//                                        → TestFindSectionIgnoresBodyOnlyMentions
// boundary[0]       中间态形态一律 error，不降级
//                                        → TestDetectExtractorRejectsUnknownLayout
// boundary[*]       正文里的顿号列举不被误判为板块标题
//                                        → TestSplitSectionsIgnoresInlineEnumeration
// error_handling[0] 切不出板块时错误自述实际板块数
//                                        → TestDetectExtractorErrorNamesActualCount
//                                        + TestSplitSectionsReturnsNilWhenNoTitle
// non_functional[0] 纯函数无 I/O：同一输入重复调用结果相等
//                                        → TestSplitSectionsIsPure
// non_functional[1] 板块数**硬断言**（require.Len），非范围断言——多切一个与
//                   少切一个同样是 bug   → TestSplitSectionsOnRealSamples
// non_functional[2] 尾注段（2025 第八板块）不参与抽取
//                                        → TestFootnoteSectionNeverWinsFindSection
//
// 以下三条不直接对应某条 DoD，是变异实测暴露的守护缺口（各自注释里记了成因）：
// 板块边界上界（正文不吃进下一节标题）  → TestSectionBodyStopsBeforeNextTitle
// Title/Body 两端无空白（T5 依赖此性质）→ TestSplitSectionsTrimsTitleAndBody
// 社融判定只看标题不看正文              → TestDetectExtractorJudgesTSFByTitleOnly
//
// 复用 strip_test.go 的 readSample / nonEmptyLines，不重复定义（同包会编译冲突）。

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// t5Keywords 是 T5 的 extractFields 实际用于定位作用域的全部 7 个关键词。
//
// 前两个只在 rule@v2 期次出现（社融当时单独发布），后五个两版都有。
// 顺序与 T5 的抽取顺序一致，便于对照。
var t5Keywords = []string{
	"社会融资规模存量",
	"社会融资规模增量",
	"广义货币",
	"人民币存款",
	"人民币贷款",
	"加权平均利率",
	"国家外汇储备",
}

func sectionsOf(t *testing.T, sample string) []section {
	t.Helper()
	return splitSections(stripHTML(readSample(t, sample)))
}

func TestSplitSectionsOnRealSamples(t *testing.T) {
	t.Run("2025 八板块", func(t *testing.T) {
		secs := sectionsOf(t, "pboc-2025-12-annual.html")

		// 硬断言而非 >= 6：多切一个板块与少切一个同样是 bug，而板块数正是
		// detectExtractor 的判据之一，范围断言会放过「多切」这一半。
		require.Len(t, secs, 8)

		for i, want := range []string{
			"社会融资规模存量", "社会融资规模增量", "广义货币", "人民币存款",
			"人民币贷款", "加权平均利率", "国家外汇储备", "跨境人民币结算",
		} {
			assert.Containsf(t, secs[i].Title, want, "第 %d 板块标题", i)
		}
		// 正文确实跟着自己的标题走
		assert.Contains(t, secs[0].Body, "442.12万亿元")
		assert.Contains(t, secs[4].Body, "271.91万亿元")
	})

	t.Run("2020 六板块", func(t *testing.T) {
		secs := sectionsOf(t, "pboc-2020-06-h1.html")

		require.Len(t, secs, 6)

		for i, want := range []string{
			"广义货币", "人民币贷款", "人民币存款", "加权平均利率",
			"国家外汇储备", "跨境贸易人民币结算",
		} {
			assert.Containsf(t, secs[i].Title, want, "第 %d 板块标题", i)
		}
		// 那期社融单独发布，全篇不该出现社融板块
		for i, s := range secs {
			assert.NotContainsf(t, s.Title, "社会融资规模", "第 %d 板块标题", i)
		}
		assert.Contains(t, secs[0].Body, "213.49万亿元")
	})
}

func TestSplitSectionsSeparatesTitleAndBody(t *testing.T) {
	text := "一、广义货币增长8.5%\n月末，广义货币(M2)余额340.29万亿元。\n" +
		"二、全年人民币存款增加26.41万亿元\n其中，住户存款增加14.64万亿元。"
	secs := splitSections(text)

	require.Len(t, secs, 2)
	assert.Equal(t, "一、广义货币增长8.5%", secs[0].Title)
	assert.Contains(t, secs[0].Body, "340.29万亿元")
	assert.NotContains(t, secs[0].Body, "住户存款", "板块正文不得越界到下一节")
	assert.Contains(t, secs[1].Body, "14.64万亿元")
	assert.NotContains(t, secs[1].Title, "广义货币")
}

// TestSectionBodyStopsBeforeNextTitle 钉住板块边界的**上界**。
//
// 「正文不越界」不能只靠「下一节的某个词没出现」来断言：那种断言在正文多吃掉
// 一整行标题时仍可能是绿的（变异实测 M7：把正文终点从「下一标题的起点」改成
// 「下一标题的终点」，原断言全绿）。这里直接断言正文不含下一节的标题原文。
//
// 多吃一行标题不是无害的排版问题：2025 第六板块的标题里带着 1.36% 与 1.4%
// 两个利率数字，被并进第五板块（贷款）的正文后，T5 在贷款作用域里找句式就可能
// 抽到利率的数——而那是静默的错值，不是报错。
func TestSectionBodyStopsBeforeNextTitle(t *testing.T) {
	for _, sample := range []string{
		"pboc-2025-12-annual.html", "pboc-2020-06-h1.html",
	} {
		t.Run(sample, func(t *testing.T) {
			secs := sectionsOf(t, sample)
			for i := 0; i < len(secs)-1; i++ {
				assert.NotContainsf(t, secs[i].Body, secs[i+1].Title,
					"第 %d 板块正文吃进了第 %d 板块的标题", i, i+1)
			}
		})
	}
}

// TestSplitSectionsTrimsTitleAndBody 钉住 Title/Body 两侧无空白。
//
// 这是 T5 依赖的跨任务性质：计划书写着「板块正文已由 splitSections 做过
// TrimSpace，这里不需要 strings」——T5 因此不会自己再 Trim 一次。
//
// 真实样本的 Body 两端都带 "\n\n \n"（stripHTML 的块级换行残留），所以 Body 的
// TrimSpace 有真实作用；Title 则因为正则按行匹配、样本里也没有行尾空白，
// 在真实样本上是空操作——用合成输入补一条，否则那个 TrimSpace 无人守护
// （变异实测：去掉它，两份真实样本全绿）。
func TestSplitSectionsTrimsTitleAndBody(t *testing.T) {
	t.Run("真实样本", func(t *testing.T) {
		for _, sample := range []string{
			"pboc-2025-12-annual.html", "pboc-2020-06-h1.html",
		} {
			for i, s := range sectionsOf(t, sample) {
				assert.Equalf(t, strings.TrimSpace(s.Body), s.Body,
					"%s 第 %d 板块正文两端应无空白", sample, i)
				assert.Equalf(t, strings.TrimSpace(s.Title), s.Title,
					"%s 第 %d 板块标题两端应无空白", sample, i)
				assert.NotEmptyf(t, s.Body, "%s 第 %d 板块正文不应为空", sample, i)
			}
		}
	})

	t.Run("标题行尾带空白", func(t *testing.T) {
		// 行首空白会让正则整个匹配不上（首字符必须是汉字数字），
		// 所以可被 TrimSpace 修掉的只有行尾这一侧。
		secs := splitSections("一、广义货币增长8.5%   \n  月末，M2 余额340.29万亿元。  ")
		require.Len(t, secs, 1)
		assert.Equal(t, "一、广义货币增长8.5%", secs[0].Title)
		assert.Equal(t, "月末，M2 余额340.29万亿元。", secs[0].Body)
	})
}

// TestSplitSectionsReturnsNilWhenNoTitle 钉住文档承诺的返回值。
//
// 注释写着「切不出任何板块时返回 nil」，而 nil 与空切片对 len/range 都一样——
// 不断言就没人守着这句话（变异实测：改成返回空切片，全套仍绿）。
func TestSplitSectionsReturnsNilWhenNoTitle(t *testing.T) {
	assert.Nil(t, splitSections("没有任何板块标题的一段正文。"))
	assert.Nil(t, splitSections(""))
}

// TestSplitSectionsIgnoresInlineEnumeration 防止把正文里的顿号列举误判为板块。
//
// 板块标题必须**独占一行且行首**。若正则漏掉行首锚定，正文中间的
// 「……分为一、二两个阶段」会被切成新板块，板块数随之错乱，进而让
// detectExtractor 误判版本——一个排版巧合就能改变抽取模板。
func TestSplitSectionsIgnoresInlineEnumeration(t *testing.T) {
	text := "一、广义货币增长8.5%\n本月分为一、二两个阶段统计，余额340.29万亿元。"
	secs := splitSections(text)

	require.Len(t, secs, 1, "正文中间的顿号列举不是板块标题")
	assert.Contains(t, secs[0].Body, "一、二两个阶段")
}

func TestSectionHas(t *testing.T) {
	s := section{Title: "一、社会融资规模存量同比增长8.3%", Body: "初步统计，对实体经济发放的人民币贷款…"}

	assert.True(t, s.has("社会融资规模存量"), "标题命中")
	assert.True(t, s.has("人民币贷款"), "正文命中——has 是 Title 或 Body")
	assert.False(t, s.has("广义货币"))
}

// TestSplitSectionsIsPure 钉住 non_functional[0]：纯函数、无 I/O、
// 不依赖包级可变状态，故同一输入重复调用必然相等。
func TestSplitSectionsIsPure(t *testing.T) {
	text := stripHTML(readSample(t, "pboc-2025-12-annual.html"))

	first := splitSections(text)
	second := splitSections(text)

	assert.Equal(t, first, second)
	// 输入未被就地改写
	assert.Equal(t, text, stripHTML(readSample(t, "pboc-2025-12-annual.html")))
}

func TestDetectExtractor(t *testing.T) {
	t.Run("八板块含社融 → rule@v2", func(t *testing.T) {
		got, err := detectExtractor(sectionsOf(t, "pboc-2025-12-annual.html"))
		require.NoError(t, err)
		assert.Equal(t, "rule@v2", got)
	})

	t.Run("六板块无社融 → rule@v1", func(t *testing.T) {
		got, err := detectExtractor(sectionsOf(t, "pboc-2020-06-h1.html"))
		require.NoError(t, err)
		assert.Equal(t, "rule@v1", got)
	})
}

func mkSections(titles ...string) []section {
	out := make([]section, 0, len(titles))
	for _, ti := range titles {
		out = append(out, section{Title: ti})
	}
	return out
}

// TestDetectExtractorRejectsUnknownLayout 是本任务最重要的一条（boundary[0]）。
//
// 中间态一律报错，不降级。七个板块缺社融可能是央行改版，也可能是页面抓残了；
// 静默当 v1 会让 M1b-3 的 completeness 用少字段的必填集，于是「解析漏了」被当成
// 「本期模板本就没有」——那是完全无声的一类错。
func TestDetectExtractorRejectsUnknownLayout(t *testing.T) {
	cases := map[string][]section{
		"七板块缺社融（可能抓残了）": mkSections("一、广义货币", "二、贷款", "三、存款",
			"四、利率", "五、外汇", "六、跨境", "七、多出来的"),
		"八板块但无社融（新模板？）": mkSections("一、A", "二、B", "三、C", "四、D",
			"五、E", "六、F", "七、G", "八、H"),
		"六板块却有社融": mkSections("一、社会融资规模存量", "二、B", "三、C",
			"四、D", "五、E", "六、F"),
		// 只有社融增量节、没有存量节：八板块的数字对上了，但形态是没见过的。
		// 判据锚在「社会融资规模存量」而不是更宽的「社会融资规模」正是为了这里——
		// 用宽锚点会把它当成正常的 rule@v2，按已知模板去抽一份结构未知的报告。
		"八板块但只有社融增量节": mkSections("一、社会融资规模增量", "二、B", "三、C",
			"四、D", "五、E", "六、F", "七、G", "八、H"),
		"空文档": {},
	}
	for name, secs := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := detectExtractor(secs)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unrecognized layout",
				"错误须自述形态不可识别，便于人工判断是改版还是抓残")
		})
	}
}

// TestDetectExtractorJudgesTSFByTitleOnly 钉住「按标题判定有无社融」这个决定。
//
// 2025 样本的尾注（注 2）正文里写着「社会融资规模存量是指…」——是社融锚点的
// 完整字面。若 detectExtractor 按「标题或正文」判定，任何**带这类尾注的六板块
// 期次**都会被认成「有社融」，于是 6 板块 + hasTSF 落进 default 分支报错，
// 一份本来能正常解析的 v1 期次被判为形态不可识别。
//
// 真实样本抓不到这个错：2020 样本通篇不含「社会融资规模」，2025 样本按哪种判据
// 都是 v2（变异实测：改成按正文判定，两份样本全绿）。所以用合成输入钉。
func TestDetectExtractorJudgesTSFByTitleOnly(t *testing.T) {
	secs := mkSections("一、广义货币", "二、贷款", "三、存款",
		"四、利率", "五、外汇", "六、跨境")
	// 末节挂上尾注，正文里出现社融锚点的完整字面——正如 2025 样本的注 2
	secs[5].Body = "注2：社会融资规模存量是指一定时期末实体经济从金融体系获得的资金余额。"

	require.True(t, secs[5].has("社会融资规模存量"),
		"前提：该板块正文确实含社融字面，has 看得见它")

	got, err := detectExtractor(secs)
	require.NoError(t, err, "六板块 + 仅正文提到社融，仍应是 rule@v1")
	assert.Equal(t, "rule@v1", got)
}

// TestDetectExtractorErrorNamesActualCount 钉住 error_handling[0]。
//
// 切不出任何板块时（正文形态变了、或抓到的是错误页面），错误必须说出**实际**
// 切出几段。只说「不认识」会让人分不清是样本变了还是切分规则错了，而这两者的
// 处置完全不同：前者改模板，后者改正则。
//
// 为什么由 detectExtractor 报而不是 splitSections 返回 error：
// 生产调用链上 splitSections 的唯一下游就是 detectExtractor（T6 的 Parse 里两者
// 相邻），错误在这里报既能带上板块数，又能同时带上「有无社融」与两个已知形态，
// 诊断信息比单独一个「切不出板块」丰富。让 splitSections 也返回 error 会为同一个
// 条件造出两条错误路径，而 detectExtractor 无论如何都得处理 len==0。
func TestDetectExtractorErrorNamesActualCount(t *testing.T) {
	for _, tc := range []struct {
		name string
		secs []section
		want string
	}{
		{"切不出任何板块", nil, "0 sections"},
		{"只切出三段（样本形态变了）",
			mkSections("一、A", "二、B", "三、C"), "3 sections"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := detectExtractor(tc.secs)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want,
				"错误须说出实际板块数，才能分辨是样本变了还是切分规则错了")
		})
	}
}

// TestDetectExtractorRejectsNonConsecutiveOrdinals 关掉「残缺的 v2 长得像合法的 v1」这条静默错路。
//
// # 缺陷形态（QA 的 architect lens 发现，Leader 与我各独立复现一次）
//
// v2 恰好 = v1 + 社融两节。所以「丢掉且**仅**丢掉这两节」的残缺 v2，其 (板块数,
// 有无社融) = (6, false) —— 与一份**合法的 v1** 完全一致。原判据只看这两个量，
// 于是一份 2025 年报被判成 rule@v1：27 个字段各自正确、过下游每一道闸门，另 27 个
// 在 M1b-1 的语义里读作「本期模板本就没有」。这正是 detectExtractor 注释里点名要防、
// 而 M0 复盘列为最危险的那一类——完全无声。
//
// # 根因不在判据，在一条跨层假设
//
// sectionTitleRE 的 (?m)^ 要求标题落在行首，而 T2 的 spaceRE **折叠但从不删除**行首
// 空白。实测 stripHTML("<p> 一、甲</p><p>二、乙</p>") 只得 1 个板块（首个被整个吞掉），
// 2025 样本剥离后有 578 行以空白开头 —— 机制一直是活的，只是尚未落到标题行上。
// 这条依赖此前从未被任何注释或测试声明过。
//
// # 为什么修法是「序号连续性」而不是「别让标题带前导空白」
//
// 前导空白只是**一种**成因。任何让某个标题行匹配不上的原因（实体、全角空格、
// 标签嵌套变化、抓取截断），后果都是同一个：板块被静默丢掉。序号是报告**自带的
// 冗余**，校验它等于让「丢了一节」这件事本身可检测，与丢失原因无关。
func TestDetectExtractorRejectsNonConsecutiveOrdinals(t *testing.T) {
	t.Run("QA 反例：2025 样本仅在一、二前各插一个 &nbsp;", func(t *testing.T) {
		raw := readSample(t, "pboc-2025-12-annual.html")
		mut := bytes.Replace(raw, []byte("<strong>一、"), []byte("<strong>&nbsp;一、"), 1)
		mut = bytes.Replace(mut, []byte("<strong>二、"), []byte("<strong>&nbsp;二、"), 1)
		require.NotEqual(t, raw, mut, "变异须真的改到了输入")

		// 前提：这个输入确实退化成「6 段、无社融」——与合法 v1 在旧判据下不可分
		secs := splitSections(stripHTML(mut))
		require.Len(t, secs, 6, "反例的形态前提")
		require.Contains(t, secs[0].Title, "三、", "首个板块的序号是三而不是一")

		_, err := detectExtractor(secs)
		require.Error(t, err, "残缺的 v2 不得被当成合法 v1")

		// fix_items 的判据：Parse 必须 error 且不产出任何 Values
		obs, err := Parse(mut)
		require.Error(t, err, "Parse 必须失败")
		assert.Empty(t, obs.Values, "失败时不得产出任何 Values")
		assert.Empty(t, obs.Meta.Extractor, "失败时不得产出 extractor")
	})

	t.Run("错误信息含实际首个序号与实际板块数", func(t *testing.T) {
		_, err := detectExtractor(mkSections("三、广义货币", "四、存款", "五、贷款",
			"六、利率", "七、外汇", "八、跨境"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "三", "须说出实际首个序号")
		assert.Contains(t, err.Error(), "6 sections", "须说出实际板块数")
	})

	t.Run("中间缺一节同样被抓", func(t *testing.T) {
		// 丢的不是开头而是中间：起始序号仍是一，只有连续性能发现
		_, err := detectExtractor(mkSections("一、A", "二、B", "四、D",
			"五、E", "六、F", "七、G"))
		require.Error(t, err, "中间断号也是丢了板块")
		assert.Contains(t, err.Error(), "6 sections")
	})

	t.Run("两位数序号：十 不得前缀匹配 十一", func(t *testing.T) {
		// 期望序号必须带「、」再比前缀。少了它，「十」会前缀匹配「十一」，于是
		// 「一…九、十一」（第十节丢了）被判成连续——正是本次要堵的那类缺节，
		// 只是发生在两位数段。变异实测：去掉 +"、" 时全套仍绿，唯本条能杀。
		secs := mkSections("一、A", "二、B", "三、C", "四、D", "五、E",
			"六、F", "七、G", "八、H", "九、I", "十一、K")
		err := checkSectionOrdinals(secs)
		require.Error(t, err, "第十节缺失，十一 顶上了")
		assert.Contains(t, err.Error(), "十、", "错误须指出期望的是 十、")
	})

	t.Run("真实样本的序号本就连续，不受影响", func(t *testing.T) {
		for sample, want := range map[string]string{
			"pboc-2025-12-annual.html": "rule@v2",
			"pboc-2020-06-h1.html":     "rule@v1",
		} {
			got, err := detectExtractor(sectionsOf(t, sample))
			require.NoErrorf(t, err, "%s 的序号是 一…N 连续的既有性质", sample)
			assert.Equal(t, want, got, sample)
		}
	})
}

// TestSectionOrdinalsAreConsecutiveInRealSamples 把「序号连续」这条**既有性质**
// 单独钉住，与上面那条守卫分开。
//
// 分开的理由：上面测的是「不连续时会报错」（守卫生效），这条测的是「真实样本确实
// 连续」（守卫的前提成立）。只有前者时，若哪天真实样本的序号规律变了，会表现为
// 一个语焉不详的 detectExtractor 报错；有了后者，转红的是这条，直接指出前提没了。
func TestSectionOrdinalsAreConsecutiveInRealSamples(t *testing.T) {
	for sample, ordinals := range map[string][]string{
		"pboc-2025-12-annual.html": {"一", "二", "三", "四", "五", "六", "七", "八"},
		"pboc-2020-06-h1.html":     {"一", "二", "三", "四", "五", "六"},
	} {
		t.Run(sample, func(t *testing.T) {
			secs := sectionsOf(t, sample)
			require.Len(t, secs, len(ordinals))
			for i, ord := range ordinals {
				want := ord + "、"
				assert.Truef(t, strings.HasPrefix(secs[i].Title, want),
					"第 %d 个板块标题应以 %q 起头，实际 %q", i, want, secs[i].Title)
			}
		})
	}
}

// TestFindSectionResolvesAllT5Keywords 是 DoD functional[2] 的判据。
//
// 对 T5 实际使用的**全部 7 个关键词**、在**两份样本**上分别断言命中哪个板块。
// 逐个钉死而不是只测一两个，是因为关键词在 v2 样本里并不唯一——只测抽样的
// 那几个正好会漏掉出问题的那个（见 TestFindSectionIgnoresBodyOnlyMentions）。
func TestFindSectionResolvesAllT5Keywords(t *testing.T) {
	t.Run("2025", func(t *testing.T) {
		secs := sectionsOf(t, "pboc-2025-12-annual.html")
		// 关键词 → 期望板块序号。八板块期次七个关键词全部有主
		for kw, want := range map[string]int{
			"社会融资规模存量": 0,
			"社会融资规模增量": 1,
			"广义货币":     2,
			"人民币存款":    3,
			"人民币贷款":    4,
			"加权平均利率":   5,
			"国家外汇储备":   6,
		} {
			t.Run(kw, func(t *testing.T) {
				got, ok := findSection(secs, kw)
				require.True(t, ok, "%q 应能定位到板块", kw)
				assert.Equal(t, secs[want].Title, got.Title,
					"%q 应落在第 %d 板块", kw, want)
			})
		}
	})

	t.Run("2020", func(t *testing.T) {
		secs := sectionsOf(t, "pboc-2020-06-h1.html")
		for kw, want := range map[string]int{
			"广义货币":   0,
			"人民币贷款":  1,
			"人民币存款":  2,
			"加权平均利率": 3,
			"国家外汇储备": 4,
		} {
			t.Run(kw, func(t *testing.T) {
				got, ok := findSection(secs, kw)
				require.True(t, ok, "%q 应能定位到板块", kw)
				assert.Equal(t, secs[want].Title, got.Title,
					"%q 应落在第 %d 板块", kw, want)
			})
		}

		// 那期没有社融板块，两个社融关键词必须找不到——T5 靠 rule@v1 分支
		// 跳过社融，这里确认「找不到」而不是「找到了别的」。
		for _, kw := range []string{"社会融资规模存量", "社会融资规模增量"} {
			_, ok := findSection(secs, kw)
			assert.Falsef(t, ok, "2020 期次不该定位到 %q", kw)
		}
	})

	t.Run("不存在的关键词返回 false", func(t *testing.T) {
		secs := sectionsOf(t, "pboc-2025-12-annual.html")
		_, ok := findSection(secs, "根本不存在的板块")
		assert.False(t, ok)
	})
}

// TestFindSectionIgnoresBodyOnlyMentions 是本任务的核心回归守护。
//
// 「正文提到某词」与「本板块的主题是某词」是两回事。真实反例（2025 样本）：
// 第一板块（社会融资规模存量）正文写着
//
//	其中，对实体经济发放的人民币贷款余额为268.4万亿元
//
// 于是「Title 或 Body 命中的第一个板块」会把 findSection("人民币贷款") 解析成
// 第 0 板块（社融），而不是第 4 板块（贷款）。2020 样本没有社融板块，所以那份
// 样本上结果正确——典型的「一份样本过、另一份炸」，只测一份样本发现不了。
//
// 落到 T5 的后果：loan_* 十个字段会在社融板块的正文里找句式，要么全炸、
// 要么抽到社融的数（如把 268.4 当成 loan_balance）——后者是静默错误。
//
// 因此 findSection 只认标题。本测试同时断言「第 0 板块正文**确实**含这个字面」，
// 好让守护本身不会因为样本变化而变成空转：若哪天正文不再提「人民币贷款」，
// 这条会红，提醒重新评估这条守护是否还必要。
func TestFindSectionIgnoresBodyOnlyMentions(t *testing.T) {
	secs := sectionsOf(t, "pboc-2025-12-annual.html")

	require.Contains(t, secs[0].Body, "对实体经济发放的人民币贷款",
		"守护的前提：社融板块正文确实提到了「人民币贷款」")
	require.NotContains(t, secs[0].Title, "人民币贷款",
		"但它不是社融板块的主题")

	got, ok := findSection(secs, "人民币贷款")
	require.True(t, ok)
	assert.Equal(t, secs[4].Title, got.Title,
		"必须定位到贷款板块，而不是正文提到该词的社融板块")
	assert.Contains(t, got.Body, "271.91万亿元", "取到的应是贷款板块的正文")
	assert.NotContains(t, got.Body, "442.12万亿元", "不得是社融板块的正文")
}

// TestFootnoteSectionNeverWinsFindSection 钉住 non_functional[2]：尾注段不参与抽取。
//
// 2025 第八板块的正文 = 跨境结算 + 注1–注5 + 整张 M1 月度回溯表，其中注 2 与注 4
// 含两条模板锚点名的**完整字面**：
//
//	注2：社会融资规模存量是指…社会融资规模增量是指…
//	注4：…对社会融资规模中"对实体经济发放的人民币贷款"和"贷款核销"数据进行了调整
//
// 也就是说，7 个关键词里有 3 个能在尾注段正文里命中。只要 findSection 会看正文，
// 尾注段就是一个随时可能被选中的假板块——目前它没被选中，仅仅因为那 3 个关键词
// 恰好在更靠前的板块里也能命中，是**顺序上的巧合而非设计**。
//
// findSection 只认标题之后，尾注段在结构上不可能被选中（它的标题是跨境结算）。
// 本测试把这件事钉成断言，并同时断言尾注段正文**确实**含那 3 个字面——否则这条
// 守护会在样本变化后悄悄退化成空转。
func TestFootnoteSectionNeverWinsFindSection(t *testing.T) {
	secs := sectionsOf(t, "pboc-2025-12-annual.html")
	footnote := secs[len(secs)-1]

	require.Contains(t, footnote.Title, "跨境人民币结算", "尾注段挂在第八板块下")
	for _, kw := range []string{"社会融资规模存量", "社会融资规模增量", "人民币贷款"} {
		require.Containsf(t, footnote.Body, kw,
			"守护的前提：尾注正文确实含 %q 的完整字面", kw)
	}

	for _, kw := range t5Keywords {
		got, ok := findSection(secs, kw)
		if !ok {
			continue
		}
		assert.NotEqualf(t, footnote.Title, got.Title,
			"%q 定位到了尾注段——尾注不得参与抽取", kw)
	}
}
