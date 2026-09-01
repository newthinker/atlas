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
	"slices"
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
		got, err := detectExtractor(sectionsOf(t, "pboc-2025-12-annual.html"), "annual")
		require.NoError(t, err)
		assert.Equal(t, "rule@v2", got)
	})

	t.Run("六板块无社融 → rule@v1", func(t *testing.T) {
		got, err := detectExtractor(sectionsOf(t, "pboc-2020-06-h1.html"), "h1")
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
//
// ⚠️ M1c-3a 的 TASK-004 改判据后，本测试的用例集**做过一次实质修订**，不是简单
// 换个期望字串。原来的四个「N 板块但…」用例是冲着旧判据的节数魔数去的，且它们的
// 合成标题被缩写成「二、贷款」「三、C」——在新判据下这些标题连核心板块都凑不齐，
// 于是**因为另一个理由**变红。测试仍然绿，但守的不再是它名字说的那件事。
// （require 失败即中止，这类「换了理由的红」不会有任何东西提醒你。）
//
// 修订后每个用例的标题都写成真实报告的形态，各自只缺它名字说的那一样：
//   - 缺核心板块 → 报错（本测试）
//   - 有增量节没有存量节 → 报错（本测试，守卫见 detectExtractor 内注释）
//   - 节数多出来但核心齐全 → **不再报错**，是刻意接受的
//     （移到 TestDetectExtractorAcceptsExtraIrrelevantSections，理由写在那里）
func TestDetectExtractorRejectsUnknownLayout(t *testing.T) {
	cases := map[string]struct {
		secs []section
		want string
	}{
		"缺核心板块之一（人民币贷款）": {mkSections(
			"一、广义货币增长8.5%", "二、人民币存款增加26万亿元",
			"三、同业拆借月加权平均利率为1.4%", "四、国家外汇储备余额3.3万亿美元"),
			"missing core section"},
		"核心板块一个都没有（抓到的是别的页面）": {mkSections(
			"一、A", "二、B", "三、C", "四、D"), "missing core section"},
		// 只有社融增量节、没有存量节：形态是没见过的。判据锚在「社会融资规模存量」
		// 而不是更宽的「社会融资规模」是刻意的；这条守的是那个窄锚点的另一侧——
		// 否则它会因「核心齐 + 有外汇」被判成 rule@v1，把增量数据静默丢掉。
		"有社融增量节但没有存量节": {mkSections(
			"一、社会融资规模增量累计为35万亿元", "二、广义货币增长8.5%",
			"三、人民币贷款增加16万亿元", "四、人民币存款增加26万亿元",
			"五、同业拆借月加权平均利率为1.4%", "六、国家外汇储备余额3.3万亿美元"),
			"found a 社融 section but not"},
		"空文档": {nil, "missing core section"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := detectExtractor(tc.secs, "annual")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want,
				"错误须自述是哪一类形态问题，便于人工判断是改版还是抓残")
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
	// 标题写成真实报告的形态（M1c-3a 的 TASK-004 改判据后，缩写标题会因
	// 「核心板块不齐」先红，本测试就测不到它自称的那个性质了）
	secs := mkSections("一、广义货币增长8.5%", "二、人民币贷款增加16万亿元",
		"三、人民币存款增加26万亿元", "四、同业拆借月加权平均利率为1.4%",
		"五、国家外汇储备余额3.3万亿美元", "六、跨境贸易人民币结算业务发生3万亿元")
	require.Empty(t, missingCoreSections(secs),
		"前提：核心四节齐全——否则红的是核心板块闸，不是「按标题判社融」这条")
	// 末节挂上尾注，正文里出现社融锚点的完整字面——正如 2025 样本的注 2
	secs[5].Body = "注2：社会融资规模存量是指一定时期末实体经济从金融体系获得的资金余额。"

	require.Contains(t, secs[5].Body, "社会融资规模存量",
		"前提：该板块正文确实含社融字面——按正文判定的话就会命中它")

	got, err := detectExtractor(secs, "annual")
	require.NoError(t, err, "六板块 + 仅正文提到社融，仍应是 rule@v1")
	assert.Equal(t, extractorV1, got)
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
			_, err := detectExtractor(tc.secs, "annual")
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
	// 变异原为「一、二 前各插一个 &nbsp;」。M1c-4 的 TASK-001 给 stripHTML 补了
	// lineLeadSpaceRE，前导空白（含 &nbsp;、全角空格）不再让标题脱锚 —— 实测该变异
	// 现在得 8 段，构造不出本条要的形态。改插一个「·」：它不是空白、不被折叠，
	// 标题行照样匹配不上 (?m)^[一-十]、，退化形态与原变异逐字相同（6 段、首个是三、）。
	//
	// 🔴 换的是**构造手法**，不是被测的守卫：序号连续性守的是「丢了一节」这件事本身，
	// 与丢失原因无关（见上方「为什么修法是序号连续性」）。前导空白只是被关掉的
	// **一种**成因，而这正是那段注释预先说明过的情形。
	t.Run("QA 反例：2025 样本仅在一、二前各插一个 ·", func(t *testing.T) {
		raw := readSample(t, "pboc-2025-12-annual.html")
		mut := bytes.Replace(raw, []byte("<strong>一、"), []byte("<strong>·一、"), 1)
		mut = bytes.Replace(mut, []byte("<strong>二、"), []byte("<strong>·二、"), 1)
		require.NotEqual(t, raw, mut, "变异须真的改到了输入")

		// 前提：这个输入确实退化成「6 段、无社融」——与合法 v1 在旧判据下不可分
		secs := splitSections(stripHTML(mut))
		require.Len(t, secs, 6, "反例的形态前提")
		require.Contains(t, secs[0].Title, "三、", "首个板块的序号是三而不是一")

		_, err := detectExtractor(secs, "annual")
		require.Error(t, err, "残缺的 v2 不得被当成合法 v1")

		// fix_items 的判据：Parse 必须 error 且不产出任何 Values
		obs, err := Parse(mut)
		require.Error(t, err, "Parse 必须失败")
		assert.Empty(t, obs.Values, "失败时不得产出任何 Values")
		assert.Empty(t, obs.Meta.Extractor, "失败时不得产出 extractor")
	})

	t.Run("错误信息含实际首个序号与实际板块数", func(t *testing.T) {
		_, err := detectExtractor(mkSections("三、广义货币", "四、存款", "五、贷款",
			"六、利率", "七、外汇", "八、跨境"), "annual")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "三", "须说出实际首个序号")
		assert.Contains(t, err.Error(), "6 sections", "须说出实际板块数")
	})

	t.Run("中间缺一节同样被抓", func(t *testing.T) {
		// 丢的不是开头而是中间：起始序号仍是一，只有连续性能发现
		_, err := detectExtractor(mkSections("一、A", "二、B", "四、D",
			"五、E", "六、F", "七、G"), "annual")
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
		for _, tc := range []struct{ sample, periodType, want string }{
			{"pboc-2025-12-annual.html", "annual", "rule@v2"},
			{"pboc-2020-06-h1.html", "h1", "rule@v1"},
		} {
			got, err := detectExtractor(sectionsOf(t, tc.sample), tc.periodType)
			require.NoErrorf(t, err, "%s 的序号是 一…N 连续的既有性质", tc.sample)
			assert.Equal(t, tc.want, got, tc.sample)
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

// ── M1c-3a 的 TASK-004：判据从「数节数」换成「板块集合 × period_type」（AD-2）──
//
// Context Checkpoint: done_criteria → test mapping
// functional[0]     "三份快照进 testdata"                       → TestDetectExtractorNewLayouts（消费它们）
// functional[1]     "签名加 periodType，六条出口"                → TestDetectExtractorNewLayouts / …RejectsTruncatedFXOutsideMonthly / …RejectsMissingCoreSection
// functional[2]     "coreSectionKeywords 从 sectionRules 派生"   → TestCoreSectionKeywordsIsDerivedNotHandwritten
// boundary[0]       "三份新快照判定正确，既有用例不变"            → TestDetectExtractorNewLayouts
// boundary[1] 下界  "真截断仍被拒"                              → TestDetectExtractorRejectsTruncatedFXOutsideMonthly
// boundary[1] 上界  "节数上界没了 —— 显式决定接受"                → TestDetectExtractorAcceptsExtraIrrelevantSections
// boundary[2]       "缺核心板块一律拒，不论 periodType"           → TestDetectExtractorRejectsMissingCoreSection
// error_handling[0] "两类错误措辞可区分"                         → TestDetectExtractorErrorsAreDistinguishable

// coreSectionKeywords() 必须**真的遍历 sectionRules**，不是手抄第二份清单。
//
// 分成两条断言，各防一种走样：
//   - 逐字面量的 ElementsMatch 防「派生规则写错」（比如漏掉 v2Only 判断）
//   - 哨兵防「手抄清单」——手抄版在 sectionRules 变化时纹丝不动
//
// ⚠️ 只有前者会漏掉手抄实现（它给出一模一样的四个词，全绿）；只有后者会漏掉
// 「派生对了但排错了板块」。两条互补，别当重复删掉。
func TestCoreSectionKeywordsIsDerivedNotHandwritten(t *testing.T) {
	core := coreSectionKeywords()

	// 肯定式：与 AD-2 判据描述的四个核心板块逐字对应
	assert.ElementsMatch(t,
		[]string{"广义货币", "人民币存款", "人民币贷款", "加权平均利率"}, core)

	// 否定式在空集上平凡为真 ⇒ 先证明被排除的那两类**确实存在**于 sectionRules。
	var nV2Only, nFX int
	for _, r := range sectionRules {
		if r.v2Only {
			nV2Only++
		}
		if r.keyword == fxSectionKeyword {
			nFX++
		}
	}
	require.Equal(t, 2, nV2Only, "前提：sectionRules 里确实有两条 v2Only（社融存量/增量）")
	require.Equal(t, 1, nFX, "前提：sectionRules 里确实有一条外汇节")
	assert.NotContains(t, core, tsfSectionKeyword, "社融节只在含社融的期次有，不是核心")
	assert.NotContains(t, core, fxSectionKeyword,
		"53/55 篇月报没有外汇节（AD-1/G1），把它算进核心会让全部月报被拒")
}

// 哨兵：往 sectionRules 临时插两条，看派生有没有跟着动、且**落在正确的一边**。
//
// 手抄清单的实现对本测试免疫不了——它不会长出哨兵。这与 TASK-003 里钉
// tsfStockFields 用的是同一手法。
func TestCoreSectionKeywordsFollowsSectionRules(t *testing.T) {
	orig := sectionRules
	t.Cleanup(func() { sectionRules = orig })

	sectionRules = append(slices.Clone(orig),
		sectionRule{keyword: "哨兵核心节", v2Only: false},
		sectionRule{keyword: "哨兵社融节", v2Only: true},
	)

	core := coreSectionKeywords()
	assert.Contains(t, core, "哨兵核心节",
		"往 sectionRules 加了一条非 v2Only 的板块，core 却没跟着长——没在遍历模板表")
	assert.NotContains(t, core, "哨兵社融节", "v2Only 的板块不属于核心")
}

// 六条出口逐条走一遍：三份新快照 + 两份既有样本 + 一条合成的 7 节月报。
//
// 既有两份必须**逐字不变**——本任务换的是判据，不是结论。
func TestDetectExtractorNewLayouts(t *testing.T) {
	for _, tc := range []struct{ name, periodType, want string }{
		// —— 本任务新增的三份快照 ——
		{"pboc-2020-q1q3.html", "q1_q3", extractorV1},
		{"pboc-2025-08-monthly.html", "monthly", extractorMonthlyV1},
		{"pboc-2020-04-monthly.html", "monthly", extractorMonthlyV1},
		// —— 既有样本，结论必须不变 ——
		{"pboc-2025-12-annual.html", "annual", extractorV2},
		{"pboc-2020-06-h1.html", "h1", extractorV1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := detectExtractor(sectionsOf(t, tc.name), tc.periodType)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	// 第六条出口（含社融的月报）没有真实快照——AD-1 记了 7 篇，取快照是
	// TASK-009 的事。合成输入先把这条分支钉住，免得它无人走过。
	t.Run("7 节月报（含社融、无外汇）→ rule-monthly@v2", func(t *testing.T) {
		got, err := detectExtractor(mkSections(
			"一、社会融资规模存量同比增长", "二、社会融资规模增量累计为",
			"三、广义货币增长", "四、人民币贷款增加", "五、人民币存款增加",
			"六、同业拆借月加权平均利率", "七、经常项下跨境人民币结算"), "monthly")
		require.NoError(t, err)
		assert.Equal(t, extractorMonthlyV2, got)
	})
}

// 🔴 新判据引入的缝：「无外汇节」原先是未知布局（响亮失败），现在成了月报的**特征**。
//
// 一篇累计期报告若外汇节恰好被抓取截断，会被静默降级成 25 字段的月报 extractor，
// completeness 认为它本就不该有外汇字段 —— **没有任何闸门会响**。
//
// 用真实的 5 节季报砍掉末节（外汇节正是第五节）来构造，而不是合成标题：
// 合成输入证明不了「真实报告被截断后长什么样」。砍末节不破坏序号连续性，
// 所以 checkSectionOrdinals 不会抢先报错 —— 本条测的确实是 FX 那道闸。
func TestDetectExtractorRejectsTruncatedFXOutsideMonthly(t *testing.T) {
	secs := sectionsOf(t, "pboc-2020-q1q3.html")
	require.Len(t, secs, 5, "前提：这份季报是 5 节")
	_, hasFX := findSection(secs, fxSectionKeyword)
	require.True(t, hasFX, "前提：它确实有外汇节")

	truncated := secs[:4]
	_, stillFX := findSection(truncated, fxSectionKeyword)
	require.False(t, stillFX, "前提：外汇节确实被砍掉了")
	require.Empty(t, missingCoreSections(truncated),
		"前提：核心四节仍齐全——否则红的是别的闸，本条就测不到它自称的性质")

	_, err := detectExtractor(truncated, "q1_q3")
	require.Error(t, err,
		"累计期报告缺外汇节 = 抓取截断，必须响亮失败，不得静默降级成 25 字段的月报 extractor")
	assert.Contains(t, err.Error(), fxSectionKeyword)

	// 🔴 **M1c-3a 的 TASK-012（QA fix 5）改了这里的第二格。**
	//
	// 原先这一格是「同一份输入换成 monthly ⇒ 放行」，用来证明判据真的用上了 periodType。
	// 那个前提**现在不成立**：这份输入的合计句自称「前三季度」，而真实月报永远不会那么写。
	// 豁免的依据是「月报本就不含外汇节」，一篇自称季度口径的报告不在那个前提里。
	//
	// 拆成两格之后，原来的用意（判据真用上了 periodType，不是碰巧对）由第三格承担，
	// 而第二格新增覆盖 fix 5：**period_type=monthly 但正文自称累计期 ⇒ 仍然拒**。
	// 这正是 2024-03 / 2025-03 那两篇 3 月报被截断时的形态。
	got, err := detectExtractor(truncated, "monthly")
	require.Error(t, err,
		"period_type 是 monthly 但合计句自称「前三季度」——豁免的前提（月报本就没有外汇节）"+
			"对它不成立，截断必须仍被拦下，否则 rule@v1 会静默降级成 25 字段的 rule-monthly@v1")
	assert.Contains(t, err.Error(), "declare a quarter-or-longer period",
		"错误信息要说清是**哪条路径**触发的：真累计期报告 vs 自称季度口径的月报，"+
			"两者排障方向不同（前者查抓取，后者还要确认这篇到底是不是 3 月报）")
	assert.Empty(t, got)

	// 第三格：**真正的**月报形态（合计句用「前八个月」），缺外汇节仍然放行。
	// 它与上一格合起来才说明豁免是按**报告自称的口径**决定的，而不是「monthly 一律放行」
	// 或「一律不放行」——单独任何一格都区分不出这三种实现。
	monthly := sectionsOf(t, "pboc-2025-08-monthly.html")
	if _, hasFXMonthly := findSection(monthly, fxSectionKeyword); hasFXMonthly {
		t.Fatal("前提：这份月报不该有外汇节")
	}
	require.False(t, declaresQuarterOrLonger(monthly),
		"前提：真实月报的合计句用「前八个月」这类月度累计前缀，不自称季度口径")
	gotM, errM := detectExtractor(monthly, "monthly")
	require.NoError(t, errM, "月报没有外汇节是正常形态（55 篇里 53 篇如此），豁免必须仍在")
	assert.Equal(t, extractorMonthlyV1, gotM)
}

// 🔴 同一处的第二条缝：原判据 len(secs)==6 同时是下界和**上界**，
// 换成「核心板块齐全」后上界没了。
//
// 【显式决定：接受多出来的无关板块，不设节数上界】理由三条——
//
//  1. 节数从来不是模板身份的代理。真实语料里同族报告是 4/5/6/7/8 节，而
//     5 节季报正是被那条上界**误拒**的（本任务要修的就是它）。任何基于节数的
//     上界都会把这个 bug 换个数字重新引入一遍。
//  2. 真正的上界在别处，而且更强：checkSectionOrdinals 要求序号从「一、」连续
//     （残缺的 v2 靠它挡下），extractFields 对每条模板要求**恰好命中一次**
//     （换了模板的报告会在抽取层响亮失败，不会静默产出错值）。
//  3. 多出来的板块对抽取是**结构性无害**的：extractFields 按关键词 findSection，
//     从不遍历「所有板块」，没被任何规则认领的节永远不会被读到。
//
// 下面第三组断言把理由 3 变成可核查的观察，而不是一句声称。
func TestDetectExtractorAcceptsExtraIrrelevantSections(t *testing.T) {
	secs := sectionsOf(t, "pboc-2020-q1q3.html")
	extra := append(slices.Clone(secs),
		section{Title: "六、无关板块甲"}, section{Title: "七、无关板块乙"})

	got, err := detectExtractor(extra, "q1_q3")
	require.NoError(t, err, "多出无关板块不改变判定——节数不是模板身份的代理")
	assert.Equal(t, extractorV1, got)

	// 理由 3 的观察：没有任何 sectionRule 的关键词会命中这两节，
	// 所以它们在抽取层结构上不可达。
	for _, r := range sectionRules {
		for _, s := range extra[len(secs):] {
			assert.NotContainsf(t, s.Title, r.keyword,
				"多出的板块 %q 命中了规则关键词 %q，理由 3 不成立", s.Title, r.keyword)
		}
	}
}

// 核心四节谁都不能缺 —— 与上一条是**不同的一格**：
// 上一条是「外汇节可缺，但仅限月报」，这一条是「核心节任何 periodType 都不能缺」。
//
// ⚠️ 构造时重排了序号：直接从真实样本里删掉中间一节会让序号断掉，
// checkSectionOrdinals 抢先报错，本条就变成「因为别的理由红」——
// require 失败即中止，它后面的断言对那种消融完全不可见。
func TestDetectExtractorRejectsMissingCoreSection(t *testing.T) {
	// 完整的 v1 形态（序号连续），再去掉「人民币贷款」那节并重排序号
	missingLoan := mkSections(
		"一、广义货币增长8.5%", "二、人民币存款增加26万亿元",
		"三、同业拆借月加权平均利率为1.4%", "四、国家外汇储备余额3.3万亿美元")
	require.NoError(t, checkSectionOrdinals(missingLoan),
		"前提：序号连续——否则红的是序号闸，不是核心板块闸")

	for _, pt := range []string{"annual", "h1", "q1", "q1_q3", "monthly"} {
		t.Run(pt, func(t *testing.T) {
			_, err := detectExtractor(missingLoan, pt)
			require.Error(t, err, "缺核心板块，不论 periodType 是什么都必须拒")
			assert.Contains(t, err.Error(), "人民币贷款", "错误须说出缺的是哪一节")
		})
	}

	// 缺**多**节时必须**全部**列出。
	//
	// 上面每个用例都只缺一节，所以「只报第一个缺失项」的实现照样全绿——
	// 而错误信息的全部意义正是「缺了哪几个」。这条是那半边的钉子。
	t.Run("缺多节时逐个列出", func(t *testing.T) {
		_, err := detectExtractor(mkSections(
			"一、广义货币增长8.5%", "二、国家外汇储备余额3.3万亿美元"), "annual")
		require.Error(t, err)
		for _, kw := range []string{"人民币贷款", "人民币存款", "加权平均利率"} {
			assert.Containsf(t, err.Error(), kw, "缺失清单里少了 %s", kw)
		}
		assert.NotContains(t, err.Error(), "广义货币", "在场的板块不该出现在缺失清单里")
	})
}

// 两类错误的措辞必须**可区分**：「缺核心板块 X」与「累计期报告缺外汇节」
// 指向完全不同的排障方向（前者改切分/模板，后者重抓页面）。
//
// 做法是**交叉断言**：每条错误既要含自己的标志串，又要**不含**另一条的。
// 只断言各自含有什么，两条错误合并成同一句话时照样全绿。
func TestDetectExtractorErrorsAreDistinguishable(t *testing.T) {
	_, errCore := detectExtractor(mkSections(
		"一、广义货币增长8.5%", "二、人民币存款增加26万亿元",
		"三、同业拆借月加权平均利率为1.4%", "四、国家外汇储备余额3.3万亿美元"), "annual")
	require.Error(t, errCore)

	_, errFX := detectExtractor(mkSections(
		"一、广义货币增长8.5%", "二、人民币贷款增加16万亿元",
		"三、人民币存款增加26万亿元", "四、同业拆借月加权平均利率为1.4%"), "annual")
	require.Error(t, errFX)

	assert.Contains(t, errCore.Error(), "missing core section")
	assert.NotContains(t, errCore.Error(), "cumulative-period report",
		"缺核心板块被说成缺外汇节，会把人支去重抓页面")

	assert.Contains(t, errFX.Error(), "cumulative-period report")
	assert.NotContains(t, errFX.Error(), "missing core section",
		"缺外汇节被说成缺核心板块，会把人支去改切分规则")
}

// monthly 必须是**唯一**豁免外汇节的 period_type。
//
// 遍历的是 validPeriodTypes 本身而不是手写五个值：将来加第六种 period_type 时，
// 它会**自动**进入本测试并被要求表态（新类型若也该豁免，本测试会红，逼人明确决定）。
// 这与 types.go 的 TestEveryPeriodTypeHasAnExplicitSupportDecision 是同一个用意。
func TestMonthlyIsTheOnlyPeriodTypeExemptFromFX(t *testing.T) {
	require.True(t, validPeriodTypes[periodTypeMonthly],
		"前提：periodTypeMonthly 得是 validPeriodTypes 里的合法取值。两处字面量分叉时，"+
			"豁免要么永不触发（全部月报被拒），要么永远触发（截断检测失效）——都是静默的")

	// 核心四节齐全、无外汇节、无社融：只有 period_type 这一个变量在动
	noFX := mkSections("一、广义货币增长8.5%", "二、人民币贷款增加16万亿元",
		"三、人民币存款增加26万亿元", "四、同业拆借月加权平均利率为1.4%")
	require.Empty(t, missingCoreSections(noFX), "前提：核心四节齐全")

	var exempt int
	for pt := range validPeriodTypes {
		t.Run(pt, func(t *testing.T) {
			got, err := detectExtractor(noFX, pt)
			if pt == periodTypeMonthly {
				require.NoError(t, err, "月报没有外汇节是正常形态（55 篇里 53 篇如此）")
				assert.Equal(t, extractorMonthlyV1, got)
				return
			}
			require.Error(t, err, "累计期报告缺外汇节 = 抓取截断，必须拒")
			assert.Contains(t, err.Error(), "cumulative-period report")
		})
		if pt == periodTypeMonthly {
			exempt++
		}
	}
	assert.Equal(t, 1, exempt, "豁免的必须**恰好**是一个 period_type")
	assert.Len(t, validPeriodTypes, 5, "取值域增删了——请确认新类型该不该豁免外汇节")
}

// TestQuarterOrLongerPeriodsIsExactly 钉住派生结果（M1c-3a 的 TASK-012，QA fix 5）。
//
// quarterOrLongerPeriods() 是从 cumulativePeriods **减**出来的（去掉月度累计与单月），
// 派生保证了「新增月度前缀不会误入」，但**不保证减法本身是对的**——比如某天
// numericMonth 那条正则写错，`1月份` 会漏进来，于是所有月报都被当成自称季度口径、
// 全部被截断守卫拒掉。逐字钉住结果让这类改动必须显式表态。
func TestQuarterOrLongerPeriodsIsExactly(t *testing.T) {
	assert.Equal(t, map[string]bool{
		"全年": true, "上半年": true, "一季度": true, "前三季度": true,
	}, quarterOrLongerPeriods(),
		"季度及以上的累计前缀恰好是这四个：新增一个累计期形态时要显式表态，"+
			"漏掉它 ⇒ 该形态的截断不再被拦（放行错数据）；多进一个月度前缀 ⇒ 月报被全部误拒")

	// 反向：月度累计与单月一个都不许在里面——它们进来会让豁免失效
	for _, w := range append(strings.Split(cumulativeMonthAlt, "|"), "1月份") {
		assert.Falsef(t, quarterOrLongerPeriods()[w],
			"%q 是月度口径，混进「季度及以上」会让月报的外汇节豁免失效", w)
	}
}

// TestDeclaresQuarterOrLongerReadsTheReportsOwnClaim 钉住 fix 5 判据的**性质**：
// 它读的是报告**自己**合计句里的期次前缀，不是标题里的日期。
//
// 三格覆盖三种实现：读正文（正确）、读日期黑名单、恒真/恒假。
// 只有「同一份 secs 只改合计句前缀就翻转结论」能把它们区分开。
func TestDeclaresQuarterOrLongerReadsTheReportsOwnClaim(t *testing.T) {
	mk := func(prefix string) []section {
		return []section{
			{Title: "一、广义货币增长8.5%", Body: "略"},
			{Title: "二、人民币贷款", Body: prefix + "人民币贷款增加9.78万亿元。"},
			{Title: "三、人民币存款", Body: prefix + "人民币存款增加12.9万亿元。"},
		}
	}
	for _, tc := range []struct {
		prefix string
		want   bool
		why    string
	}{
		{"一季度", true, "3 月报按季度口径写，它在口径上就是一篇累计期报告"},
		{"全年", true, "季度及以上"},
		{"前八个月", false, "月度累计，是真正的月报形态"},
		{"8月份", false, "单月"},
		{"1-8月", false, "2022 年月报的月度累计写法——本次 R3 新增的形态也必须留在月报侧"},
	} {
		t.Run(tc.prefix, func(t *testing.T) {
			assert.Equalf(t, tc.want, declaresQuarterOrLonger(mk(tc.prefix)), "%s", tc.why)
		})
	}

	// 缺节的那条分支：贷款/存款节任一缺失时跳过它，不因此判成「自称季度口径」。
	// 判成 true 会让一篇缺核心节的月报被截断守卫拒掉，而缺核心节应当由
	// missingCoreSections 报——两条错误的排障方向不同。
	t.Run("缺存款节_只看得到的那节", func(t *testing.T) {
		onlyLoan := []section{
			{Title: "一、广义货币增长8.5%", Body: "略"},
			{Title: "二、人民币贷款", Body: "一季度人民币贷款增加9.78万亿元。"},
		}
		assert.True(t, declaresQuarterOrLonger(onlyLoan), "看得到的那节自称季度口径")
	})
	t.Run("两节都缺_不表态", func(t *testing.T) {
		assert.False(t, declaresQuarterOrLonger([]section{{Title: "一、广义货币", Body: "略"}}),
			"没有合计句可读时不得判成自称季度口径——那会把「缺核心节」误报成「截断」")
	})
}
