package hestia

import (
	"fmt"
	"regexp"
	"strings"
)

// section 是报告的一个板块：一行标题加它下面的正文。
type section struct {
	Title string
	Body  string
}

// —— 关于曾经存在于此的 section.has ——
//
// 它是「标题或正文含 kw」的宽松判据，当初留给 T5 在已定位板块之后判断某个可选
// 句式在不在。**那个理由已随 T5 交付而过期**：T5 实测七类句式全部靠固定锚点定位，
// 一次都没消费它。删除依据（QA 报告 S1 节）比「当下无调用者」强一层：
//
//   - M1b-3 结构上够不到它：校验层消费 Observation，而 section 是解析层的**未导出**
//     类型；让它拿到 []section 等于让它重新解析一遍 HTML。
//   - has 是另一套设计的残件：extract.go 的纪律是「任何模板未命中一律报错」，
//     extractFields 要么全成要么返回 nil。「本板块有没有某个**可选**句式」正是
//     这个设计明确拒绝的模型。
//   - 它该回来的时机只有一个：M1b-3 若选了「部分成功」那条路，届时应**带着确定的
//     消费者**重新引入，而不是先留着等人来用。
//
// 留这段注释而不是无声删掉：它已经被独立发现过两次（dev-agent-45 在 T5 记过一笔
// 「不是死代码，免得后续被清理」，test-agent-22 确认过），不写明理由为何过期，
// 下一个人会重新发现同一件事再争第三次。

// sectionTitleRE 匹配板块标题，形如「一、广义货币增长8.5%」。
//
// (?m) 加行首锚定是必需的：正文中间也会出现顿号列举（「分为一、二两个阶段」），
// 漏掉锚定会把它切成新板块，板块数随之错乱，而板块数正是版本探测的依据之一——
// 一个排版巧合就会让 detectExtractor 误判模板。
//
// 依赖 stripHTML 把 <p> 这类块级标签换成换行（T2 已锁定该性质）：原文的标题
// 形态是 `<p><strong>一、…`，<strong> 被删除后标题才落在行首。
var sectionTitleRE = regexp.MustCompile(`(?m)^[一二三四五六七八九十]+、.*$`)

// splitSections 按板块标题切分全文。标题行之前的内容（网页头部、报告标题）被丢弃。
//
// 纯函数：不读写任何 I/O，不依赖包级可变状态，同一输入必得同一结果。
//
// 切不出任何板块时返回 nil 而不是 error：生产链路上它的唯一下游是
// detectExtractor（T6 的 Parse 里两者相邻），由那里统一报错能同时带上板块数、
// 有无社融与两个已知形态，诊断信息比单独一句「切不出板块」丰富得多；而
// detectExtractor 无论如何都得处理 len==0 这一支。
func splitSections(text string) []section {
	locs := sectionTitleRE.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return nil
	}
	secs := make([]section, 0, len(locs))
	for i, loc := range locs {
		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		secs = append(secs, section{
			Title: strings.TrimSpace(text[loc[0]:loc[1]]),
			Body:  strings.TrimSpace(text[loc[1]:end]),
		})
	}
	return secs
}

// findSection 返回**标题**含 kw 的第一个板块，供 T5 定位抽取作用域。
//
// # 为什么只认标题，不看正文
//
// 「正文提到某词」与「本板块的主题是某词」是两回事，而 T5 要的是后者。
// 真实反例（2025 样本，第一板块「社会融资规模存量」的正文）：
//
//	其中，对实体经济发放的人民币贷款余额为268.4万亿元
//
// 若按「标题或正文命中的第一个板块」，findSection("人民币贷款") 会返回第 0 板块
// （社融）而不是第 4 板块（贷款）。2020 样本没有社融板块，那份样本上结果正确——
// 「一份样本过、另一份炸」，只测一份发现不了。落到 T5 就是 loan_* 十个字段在
// 社融板块里找句式：要么全炸，要么抽到社融的数（把 268.4 当成 loan_balance），
// 后者完全无声。
//
// 第二个理由是尾注段。2025 第八板块正文含注 1–注 5，其中：
//
//	注2：社会融资规模存量是指…社会融资规模增量是指…
//	注4：…对社会融资规模中"对实体经济发放的人民币贷款"和"贷款核销"数据进行了调整
//
// 7 个关键词里有 3 个能在尾注正文里命中完整字面。只要 findSection 看正文，尾注段
// 就是一个随时可能被选中的假板块；它至今没被选中只是**顺序上的巧合**——那 3 个词
// 恰好在更靠前的板块也能命中。只认标题之后，尾注段在结构上不可能被选中。
//
// # 代价
//
// 若将来某版报告的板块标题不再含关键词，findSection 会返回 false，T5 随即报
// 「missing section containing …」而不是猜一个板块。那是本项目一贯的取舍：
// 宁可这一期人工看一眼，也不要静默定位到错误的作用域。
func findSection(secs []section, kw string) (section, bool) {
	for _, s := range secs {
		if strings.Contains(s.Title, kw) {
			return s, true
		}
	}
	return section{}, false
}

// —— 关于曾经存在于此的 sectionsV1 / sectionsV2（板块数 6 与 8）——
//
// 它们是旧判据 `len(secs)==6 → v1 / len(secs)==8 → v2` 的两个魔数，随 M1c-3a 的
// TASK-004 换判据一起删除（AD-2）。
//
// **不要加回来当「节数上界」**：节数从来不是模板身份的代理。218 篇实测里同一族
// 报告是 4/5/6/7/8 节，差别只在有没有「经常项下跨境人民币结算」这类**不产出任何
// 字段**的板块——那条上界正是把 5 节季报（2020-09/2022-09）和全部四种月报布局
// 挡在门外的原因，共 74 篇。任何基于节数的上界都会把同一个 bug 换个数字重来一遍。
//
// 真正的上界在别处，而且更强：checkSectionOrdinals 要求序号从「一、」连续
// （残缺的 v2 靠它挡），extractFields 对每条模板要求恰好命中一次（换了模板的
// 报告会在抽取层响亮失败）。见 TestDetectExtractorAcceptsExtraIrrelevantSections。

// tsfSectionKeyword 是判定「本期含社融板块」的标题锚点。
const tsfSectionKeyword = "社会融资规模存量"

// periodTypeMonthly 是**唯一**豁免外汇节的 period_type（M1c-3a 的 TASK-004）。
//
// 与 types.go 的 validPeriodTypes 是同一个取值域，由
// TestMonthlyIsTheOnlyPeriodTypeExemptFromFX 的前提断言绑住：这个字串若与白名单
// 分叉，豁免就永远不触发（全部月报被拒），或永远触发（截断检测失效）。
const periodTypeMonthly = "monthly"

// fxSectionKeyword 是外汇板块的标题锚点（M1c-3a 的 TASK-004）。
//
// 它在判据里有**独立于其它板块**的地位：53/55 篇月报正文根本没有这一节
// （AD-1 / 缺口 G1 实测），所以它既不是核心板块、也不像社融那样由 v2Only 标出，
// 只能单独点名排除。
//
// ⚠️ extract.go 的 sectionRules 里那一条仍写着同样的字面量（那个文件本任务只读）。
// 两处分叉时 coreSectionKeywords() 会把外汇当成核心板块，于是**全部月报被拒**——
// 由 TestCoreSectionKeywordsIsDerivedNotHandwritten 里
// `require.Equal(t, 1, nFX)` 那条前提断言钉住：分叉时它先红。
const fxSectionKeyword = "国家外汇储备"

// coreSectionKeywords 返回「任何一期《金融统计数据报告》都必须有」的板块关键词，
// 从 sectionRules **派生**（M1c-3a 的 TASK-004，AD-2）。
//
// 派生规则：非 v2Only（社融两节只在含社融的期次有）且非外汇节（见上）。
//
// 不手写第二份清单：sectionRules 才是「本包认得哪些板块」的事实，手抄的那份会在
// 下次加板块时静默过期——而它的过期方式是**判据变松**（少要求一个板块），
// 不会有任何东西转红。这与 required.go 头部「模板表才是事实」是同一条理由。
func coreSectionKeywords() []string {
	out := make([]string, 0, len(sectionRules))
	for _, r := range sectionRules {
		if r.v2Only || r.keyword == fxSectionKeyword {
			continue
		}
		out = append(out, r.keyword)
	}
	return out
}

// hasAnyTSFSection 回答「社融那两节里出现了任意一节吗」，从 sectionRules 的 v2Only
// 标记派生（M1c-3a 的 TASK-004），不另抄一份社融关键词。
//
// 它与 findSection(secs, tsfSectionKeyword) 的差别正是下面那道守卫要用的：
// 前者宽（存量**或**增量），后者只认存量。
func hasAnyTSFSection(secs []section) bool {
	for _, r := range sectionRules {
		if !r.v2Only {
			continue
		}
		if _, ok := findSection(secs, r.keyword); ok {
			return true
		}
	}
	return false
}

// missingCoreSections 返回缺失的核心板块关键词，按 coreSectionKeywords 的顺序。
// 返回空切片表示核心齐全。
//
// 返回**清单**而不是 bool：新判据下「一共几节」已不再是充分的诊断信息，
// 排障要的是「缺了哪几节」。
func missingCoreSections(secs []section) []string {
	var missing []string
	for _, kw := range coreSectionKeywords() {
		if _, ok := findSection(secs, kw); !ok {
			missing = append(missing, kw)
		}
	}
	return missing
}

// sectionOrdinals 是板块标题的中文序号，按顺序。
//
// 写成字面表而不是做汉字数字解析：本校验只需要**生成**期望序列去比对，不需要读懂
// 任意汉字数字。二十条远超任何已知模板（已知 6 与 8），超出即属未知形态，由
// detectExtractor 的版式分支去报错。
var sectionOrdinals = []string{
	"一", "二", "三", "四", "五", "六", "七", "八", "九", "十",
	"十一", "十二", "十三", "十四", "十五", "十六", "十七", "十八", "十九", "二十",
}

// checkSectionOrdinals 校验板块序号从「一、」起连续。
//
// # 它挡的是什么
//
// v2 恰好 = v1 + 社融两节，所以「丢掉且**仅**丢掉这两节」的残缺 v2，其
// (板块数, 有无社融) = (6, false)，与一份**合法的 v1** 逐位相同。只看这两个量的
// 判据无法区分二者，会把一份年报判成 rule@v1：27 个字段各自正确地流进权威表，
// 另 27 个在 M1b-1 的语义里读作「本期模板本就没有」——完全无声。
//
// # 为什么校验序号，而不是去修「标题带前导空白」
//
// 触发那个反例的直接原因是 sectionTitleRE 的 (?m)^ 要求标题落在行首，而 T2 的
// spaceRE **折叠但从不删除**行首空白（实测：stripHTML("<p> 一、甲</p><p>二、乙</p>")
// 只得 1 个板块；2025 样本剥离后有 578 行以空白开头）。但那只是**一种**成因：
// 全角空格、实体、标签嵌套变化、抓取截断，任何让某个标题行匹配不上的原因，后果
// 都是同一个——板块被静默丢掉。
//
// 序号是报告**自带的冗余**。校验它等于让「丢了一节」这件事本身可检测，与丢失的
// 原因无关；而逐个去堵成因，永远只能堵住已经见过的那些。
//
// # 代价
//
// 若将来某版报告改用别的序号体系（阿拉伯数字、无序号），本校验会报错而不是猜。
// 与 detectExtractor 的整体取向一致：宁可这一期人工看一眼。
func checkSectionOrdinals(secs []section) error {
	for i, s := range secs {
		if i >= len(sectionOrdinals) {
			break // 超出已知模板量级，交给版式分支报「未知形态」
		}
		// 期望前缀必须带「、」：少了它，「十」会前缀匹配「十一」，于是
		// 「一…九、十一」（第十节丢了）被判成连续——正是本函数要抓的那类缺节。
		want := sectionOrdinals[i] + "、"
		if strings.HasPrefix(s.Title, want) {
			continue
		}
		return fmt.Errorf(
			"hestia: section ordinals are not consecutive from 一: got %d sections, "+
				"section[%d] is %q but should start with %q. "+
				"A section title that fails to anchor at line start is dropped silently, "+
				"and a rule@v2 report missing exactly its two 社融 sections becomes "+
				"indistinguishable from a valid rule@v1 one — so this is refused rather "+
				"than guessed. Common cause: leading whitespace before the ordinal "+
				"(stripHTML folds runs of spaces but does not remove them at line start)",
			len(secs), i, firstOrdinalOf(s.Title), want)
	}
	return nil
}

// firstOrdinalOf 取标题里「、」之前的部分用于报错，取不到就退回整个标题。
// 只为让错误信息说出**实际**首个序号，不参与任何判定。
func firstOrdinalOf(title string) string {
	if i := strings.Index(title, "、"); i > 0 {
		return title[:i] + "、"
	}
	return title
}

// detectExtractor 按内容判定模板版本。
//
// # 为什么按内容而不按日期
//
// 方案报告 4.6.2 原案是「配置里写每版的适用期间」，但 v1→v2 的切换点目前**未知**
// ——那份文档自己写着要 M1c 回填时二分定位。按内容探测不需要那张表。
//
// # 为什么中间态报错而不降级
//
// 七个板块缺社融，可能是央行又改版了，也可能是页面抓残了。两者都不该静默当 v1：
// 误判 v1 会让 completeness（M1b-3）用少字段的必填集，于是「解析漏了」被当成
// 「本期模板本就没有」——那正是 M0 复盘时列为最危险的一类错，因为它完全无声。
//
// 宁可这一期人工看一眼，也不要让一个错误的 extractor 值流进权威表——它还会被
// 写进 hestia_observations.extractor 列，事后连追查都无从下手。
//
// 只看标题（而非正文）判定社融：2025 的尾注段正文也含「社会融资规模存量」的完整
// 字面（注 2），按正文判定会让任何含尾注的期次都被认成有社融板块。「标题含 kw 的
// 第一个板块」正是 findSection 的语义，直接复用它，免得同一条判据有两份实现、
// 将来只改了一处。
//
// 序号连续性先于版式判定：残缺的 v2 在版式上与合法的 v1 无法区分（见
// checkSectionOrdinals），等到版式分支才判就已经晚了——那时它已经是个合法形态。
// 空输入跳过：那是「一个板块都没切出来」，由下面的版式分支报，信息更全。
func detectExtractor(secs []section, periodType string) (string, error) {
	if len(secs) > 0 {
		if err := checkSectionOrdinals(secs); err != nil {
			return "", err
		}
	}

	// 判据问「必需板块在不在」，不问「一共几节」。四个维度：
	// 核心板块齐全 × hasFX × hasTSF × periodType。
	if missing := missingCoreSections(secs); len(missing) > 0 {
		return "", fmt.Errorf(
			"hestia: missing core section(s) %s: got %d sections. "+
				"Every 金融统计数据报告 carries these four regardless of template "+
				"version, so this is a new template, a different kind of report, or a "+
				"truncated fetch — all three need a human look. Guessing an extractor "+
				"would let a wrong completeness profile treat missing fields as "+
				"absent-by-design",
			strings.Join(missing, ", "), len(secs))
	}

	_, hasFX := findSection(secs, fxSectionKeyword)
	_, hasTSF := findSection(secs, tsfSectionKeyword)

	// 有社融增量节却没有存量节 = 自相矛盾的形态，拒。
	//
	// 这道守卫是**旧判据顺带提供、新判据会弄丢的**，故显式补回（M1c-3a 的 TASK-004）：
	// 旧判据靠 len(secs)==8 && !hasTSF 落进 default 报错；新判据不数节数，于是这种
	// 输入会因「核心齐 + 有外汇」被判成 rule@v1，而 rule@v1 **完全跳过** v2Only 两节
	// ——增量数据被静默丢掉，completeness 认为本期本就没有社融字段。
	//
	// 判据锚在「存量」而不是更宽的「社会融资规模」是刻意的（见 findSection 的注释与
	// TestDetectExtractorJudgesTSFByTitleOnly）；这道守卫补的正是那个窄锚点的另一侧。
	//
	// 实践中它够不到：v2 版式里存量/增量是相邻的第一、二节，丢掉存量会让序号从「二、」
	// 起，checkSectionOrdinals 先一步拦下。留着是防**新模板**，不是防截断。
	if hasAnyTSFSection(secs) && !hasTSF {
		return "", fmt.Errorf(
			"hestia: found a 社融 section but not %q: got %d sections. The v2 layout "+
				"always carries 存量 and 增量 as its first two sections, so this is an "+
				"unseen template. Judging it rule@v1 would skip both 社融 sections "+
				"entirely and read their 27 fields as absent-by-design",
			tsfSectionKeyword, len(secs))
	}

	// 🔴 新判据引入的缝，必须堵在这里。
	//
	// 「无外汇节」原先是**未知布局**（响亮失败），现在成了月报的**特征**。一篇
	// 累计期报告若外汇节恰好被抓取截断，会被静默降级成 25 字段的 rule-monthly@v1，
	// completeness 认为它本就不该有 fx_reserve / fx_rate —— 没有任何闸门会响。
	//
	// 只有 monthly 允许没有外汇节：55 篇月报实测 53 篇正文根本没有这一节（G1），
	// 而累计期报告（q1 / h1 / q1_q3 / annual）四种版式全都有。
	if !hasFX && periodType != periodTypeMonthly {
		return "", fmt.Errorf(
			"hestia: cumulative-period report (period_type=%q) has no %q section: "+
				"got %d sections. Monthly reports legitimately lack it (53 of 55 "+
				"sampled), but every 累计期 report carries it, so this is a truncated "+
				"fetch. Accepting it would silently downgrade the report to the "+
				"25-field monthly profile, under which the two missing 外汇 fields "+
				"read as absent-by-design and no gate ever fires",
			periodType, fxSectionKeyword, len(secs))
	}

	switch {
	case hasTSF && hasFX:
		return extractorV2, nil
	case !hasTSF && hasFX:
		return extractorV1, nil
	case hasTSF && !hasFX:
		return extractorMonthlyV2, nil
	default: // !hasTSF && !hasFX
		return extractorMonthlyV1, nil
	}
}
