// 类别维度 EPS 回退路径(TASK-002)。
//
// # 为什么需要这条路径
//
// SEC 的 companyfacts API 是从 XBRL 实例文档抽取的**聚合视图**,只收录**无维度**的事实。
// 实测(2026-07-26,V/Visa CIK 1403161,完整 companyfacts 3.56 MB):
//
//   - us-gaap 下 625 个 tag,**没有任何** EarningsPerShareBasic/Diluted,
//     也没有任何 WeightedAverageNumberOf*SharesOutstanding*;
//   - 但同期 NetIncomeLoss 有 227 条。
//
// 打开 V 的 10-K 实例文档才看清原因:V 的 EPS 与加权股本**每一条都带**
// us-gaap:StatementClassOfStockAxis 维度(CommonClassA / v:CommonClassB1 /
// CommonClassB2 / CommonClassC),**不存在无维度的合并 EPS** —— Visa 是多类别股权结构,
// 各类别转换比例不同,"合并 EPS"没有意义,故它按类别分别披露。而 NetIncomeLoss
// **有**无维度版本,这就是「同一份文档里净利进得了 companyfacts、EPS 进不了」的确切原因。
//
// 这与 M3 分部营收走实例文档路线是**同一个结构性原因**(companyfacts 丢维度),
// 故本文件复用 segments.go 已有的 submissions → 实例文档取数管道
// (recentFilings / instanceURL / httpGet / countingReader),但回看上限用自己的
// maxClassEPSFilings —— 理由见该常量注释。
//
// # 触发条件(回归安全的结构性保证)
//
// 只有在 companyfacts **完全取不到 EPS** 时才走本路径。COST/CRM/WMT/AVGO 的 EPS 在
// companyfacts 里正常存在,因此对它们这条路径连一次 HTTP 请求都不会发出 —— 回归安全
// 由「分支根本不进入」保证,而不是靠「结果碰巧一样」(TestFetchCompanyFactsWithEPSNeverTouchesArchives)。
package edgar

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// classOfStockAxis 是股份类别轴。EPS 按类别披露时挂在这个轴上。
const classOfStockAxis = "StatementClassOfStockAxis"

// maxClassEPSFilings 是类别维度 EPS 路径的回看上限。
//
// # 结构性上限与当日快照是两件事,别混
//
// **结构性事实(永远成立)**:reportDate ≤ 2019-03-31 的 filing 是老式非 inline XBRL,
// `_htm.xml` 推导根本不成立(AD-3 已记载的降级场景,也正是 segments.go 文件头解析规则 7
// 说的那一类),实例文档一律 404。**真正的上限是这条时间线**,不是下面那个数字 ——
// 要往 2019-03-31 之前延伸,得先解决老式 filing 的文档定位方式,改这个常量做不到。
//
// **当日快照(会随时间失效)**:2026-07-26 对 V(CIK 1403161)最近 40 份 10-K/10-Q 逐份
// 探测,序号 0–27 全部 200、序号 28 起连续 404 —— 即**在那一天**恰好 28 份落在
// 2019-03-31 之后,28 正好等于「全部可用份数」,故当时调大它一条数据也多不出来。
//
// **但这个巧合不会持续:将来调大是有效的。** 每新增一季申报,可用份数 +1(约每 3 个月),
// 28 会从「恰好等于全部可用」变成一道**真实的截断窗口**,那时调大它确实能多拿到数据。
// 重估上限时请按「2019-06-30 至今的季度报告份数」重算,**不要沿用 28 这个数字**,
// 也不要把它当成不可调参数。代价按下方实测线性外推:每多一份约 +1.9 MB / +1.6 s。
//
// # 为什么与 maxSegmentFilings 分开
//
// 实测:cap=12 时 V 的 valuation_daily 只有 370 天 / pctl_5y 118 点,**比它替换掉的
// engine 兜底(705 天 / 453 点)还短**;cap=28 时是 1377 天 / 1125 点,双轴严格优于兜底。
// 但 maxSegmentFilings 由 FetchSegmentRevenue 共用,抬高它会连带放大分部营收的拉取量,
// 而那条路径的影响在本任务的 live 环境里**无法测量**(两次运行 segment_revenue 均 0 行)。
// 在测不到的地方不动共享状态,故各用各的常量。
//
// # 成本
//
// 28 份 ≈ 52.3 MB / 45 s(curl 顺序拉取实测,冷缓存;前 12 份 18.4 MB,平均 1.87 MB/份)。
// 该成本**只落在 companyfacts 缺 EPS 的公司**上 —— 有 EPS 的公司连一次请求都不发出。
const maxClassEPSFilings = 28

// classFact 是实例文档里一条带股份类别维度的事实。
// filed 由取数循环按所属报告回填(解析层不知道 filing 日期)。
type classFact struct {
	tag        string // 去命名空间的 local name,如 EarningsPerShareDiluted
	member     string // 去命名空间的类别 member,如 CommonClassAMember
	start, end string // 期间(YYYY-MM-DD);instant 型不收
	filed      string // 所属报告的 filingDate
	form       string // "10-K" | "10-Q"
	val        float64
}

// classTags 是本路径关心的科目:EPS 与加权稀释股本两条既有回退链的并集。
// 复用 client.go 的 epsTags / sharesTags,保证与 companyfacts 主路径**口径一致**。
var classTags = slices.Concat(epsTags, sharesTags)

// parseClassFacts 单遍流式解析实例文档,抽出 classTags 在 axis 轴上的**带维度**事实。
//
// 与 parseSegmentInstance 的三点差异,都是刻意的:
//
//  1. **不做 unit 过滤**。分部营收必须过滤 unit —— 实测 V 的实例文档同时声明了 usd/eur,
//     营收 tag 在不同货币下重复出现,不过滤就会混币种求和。而 EPS/股本的 tag 名在
//     us-gaap 里已唯一确定量纲(每股金额 / 股数),不存在同 tag 多量纲的情形;
//     真要出现外币 EPS,那是 IFRS/20-F 申报人,在上游就已被 ErrNotUSGAAP 挡掉。
//  2. **不在解析层做期间过滤**。累计期(YTD)在这里保留,由 classRawFacts 统一按
//     下游过滤器的口径丢弃 —— 让「过滤口径」只有一处定义(见 classRawFacts 注释)。
//  3. **不做 tag 优先级消解**。同 (tag, member, period) 在 inline XBRL 里会被渲染两次
//     (同 contextRef 同值),重复无害:下游 applyDuration 按 period_end 去重。
//     跨 tag 的优先级留给 classRawFacts,那里才知道最终选中哪个类别。
func parseClassFacts(r io.Reader, axis string) ([]classFact, error) {
	type pendingFact struct {
		contextRef string
		tag        string
		val        float64
	}
	type ctxInfo struct{ start, end, member string }

	contexts := map[string]ctxInfo{}
	var pending []pendingFact

	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch {
		case se.Name.Local == "context":
			var xc xmlContext
			if err := dec.DecodeElement(&xc, &se); err != nil {
				return nil, err
			}
			// 恰好一个 explicitMember 且轴匹配才收:排除交叉维(如 类别×权益构成)
			// 与单维但轴不匹配(如 StatementEquityComponentsAxis)两类干扰。
			if len(xc.Members) != 1 || localName(strings.TrimSpace(xc.Members[0].Dimension)) != axis {
				continue
			}
			// instant 型(只有 <instant>,无 startDate)不是期间,跳过。
			if xc.Period.StartDate == "" || xc.Period.EndDate == "" {
				continue
			}
			contexts[xc.ID] = ctxInfo{
				start:  xc.Period.StartDate,
				end:    xc.Period.EndDate,
				member: localName(strings.TrimSpace(xc.Members[0].Value)),
			}
		case slices.Contains(classTags, se.Name.Local):
			ref := attrValue(se, "contextRef")
			var text string
			if err := dec.DecodeElement(&text, &se); err != nil {
				return nil, err
			}
			val, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
			if err != nil {
				continue
			}
			pending = append(pending, pendingFact{contextRef: ref, tag: se.Name.Local, val: val})
		}
	}

	// 流末关联:不假设 context 一定先于 fact 出现(AD-11)。
	out := make([]classFact, 0, len(pending))
	for _, f := range pending {
		ctx, ok := contexts[f.contextRef]
		if !ok {
			continue // 无维度事实或轴不匹配 —— 本路径不关心
		}
		out = append(out, classFact{
			tag: f.tag, member: ctx.member,
			start: ctx.start, end: ctx.end, val: f.val,
		})
	}
	return out, nil
}

// dominantClass 在有 EPS 披露的类别中确定性地选出**主类别**。
//
// 规则:该类别申报过的**最大**加权稀释股本降序 → 类别名升序。
//
//   - 为什么用「股本最大」而不是写死 CommonClassAMember:与 TASK-001 的 bestUnitName 同理,
//     数据驱动比写死 taxonomy 名健壮。实测 V 的 Class A 稀释股本 ~1.97e9,B1/B2/C 分别只有
//     5e6~2.45e8,差一个数量级以上;而 ticker V 交易的正是 Class A,口径天然对上。
//   - 为什么用 max 而不是 sum:同一事实在 inline XBRL 里会重复渲染,且股本有两条 tag
//     (diluted / basic),求和会按重复次数放大。max 对这两者都免疫。
//   - 候选**限定为有 EPS 事实的类别**:ParticipatingSecuritiesMember 这类只有净利分配、
//     不披露 EPS 的"类别"必须排除。
//   - 类别名升序只是消除并列用的全序尾巴,不承载语义。
//
// 遍历顺序取自排序后的类别名而非 map 迭代,故结果与输入切片排列无关 —— 这是可穷举
// 断言的结构性性质(TestDominantClassIsPermutationInvariant),不是靠重复运行压概率。
func dominantClass(facts []classFact) string {
	hasEPS := map[string]bool{}
	maxShares := map[string]float64{}
	for _, f := range facts {
		switch {
		case slices.Contains(epsTags, f.tag):
			hasEPS[f.member] = true
		case slices.Contains(sharesTags, f.tag):
			if f.val > maxShares[f.member] {
				maxShares[f.member] = f.val
			}
		}
	}
	members := make([]string, 0, len(hasEPS))
	for m := range hasEPS {
		members = append(members, m)
	}
	sort.Strings(members)

	best, bestShares := "", -1.0
	for _, m := range members {
		if s := maxShares[m]; s > bestShares {
			best, bestShares = m, s
		}
	}
	return best
}

// classRawFacts 把选中类别的事实合成 []rawFact,直接喂给 FetchCompanyFacts 既有的
// 季度化流水线(applyDuration → durationEntries → Q4 = FY − ΣQ1..Q3)。
//
// 合成 rawFact 的两个字段需要说明:
//
//   - **FP 只是过滤标志,不承载真实季次**。下游 isSingleQuarter 只看 FP∈{Q1,Q2,Q3},
//     isAnnual 只看 FP=="FY";而 FiscalPeriod 展示标签一律由 period_end 经 fiscalLabel
//     推导(label() 里用 rawFP 的分支只在「companyfacts 无任何年度条目」时才生效,
//     本路径的前提是 companyfacts 可读且含年度条目)。故单季一律标 "Q1"。
//   - **分类用下游过滤器本身来判定**,而不是自己再算一遍天数。这一点是 load-bearing 的:
//     segments.go 的 keepDuration 算天数**含首尾 +1**,client.go 的 durationDays **不含**,
//     两处差一天。若在这里按 keepDuration 分类,边界期间会被标成单季却被下游拒收,
//     表现为「莫名其妙少一季」且极难定位。这里直接拿候选 rawFact 去问 isAnnual /
//     isSingleQuarter,标注与判定必然自洽(TestClassRawFactsFPMatchesDownstreamFilter)。
//
// tag 回退链:EPS 与股本各自沿 epsTags / sharesTags 取**首个有可用条目**的 tag,
// 选中后其余 tag 一律不参与 —— 与 firstQuarterlyTag 同样的「口径不混用」规则。
func classRawFacts(facts []classFact, member string) (eps, shares []rawFact) {
	byTag := map[string][]rawFact{}
	for _, f := range facts {
		if f.member != member {
			continue
		}
		raw := rawFact{
			Start: f.start, End: f.end, Val: f.val,
			Filed: f.filed, Form: f.form,
		}
		if end, err := time.Parse(dateLayout, f.end); err == nil {
			raw.FY = end.Year()
		}
		raw.FP = "FY"
		if !isAnnual(raw) {
			raw.FP = "Q1"
			if !isSingleQuarter(raw) {
				continue // 累计期(YTD)等既非单季也非年度,丢弃
			}
		}
		byTag[f.tag] = append(byTag[f.tag], raw)
	}
	firstNonEmpty := func(tags []string) []rawFact {
		for _, tag := range tags {
			if f := byTag[tag]; len(f) != 0 {
				return earliestFiledPerValue(f)
			}
		}
		return nil
	}
	return firstNonEmpty(epsTags), firstNonEmpty(sharesTags)
}

// earliestFiledPerValue 对同 (period, 值) 的重复披露只保留 **filed 最早**的那条。
//
// # 解决的是什么
//
// companyfacts 的每条 entry 自带**它自己的** filed;而实例文档路径只能拿到「装着这条事实
// 的那份报告」的 filingDate。一个季度会在**次年同期报告的对比期**里原样再出现一次,不做
// 这一步就会给同一个值打上晚一年的日期 —— 那是**错的申报日**,而整套防前视机制正建立在
// 「Filed 是该值真实公开的日期」这一前提上。本函数让本路径的 Filed 语义与 companyfacts
// 对齐(每条值都带它首次公开的日期),顺带把 V 的 EPS 事实从 54 条去重到 17 条。
//
// # 它**不**解决什么(实测,勿高估)
//
// 它不会让 V 的估值序列变长。QuarterlyFact.FilingDate 是**全部科目取 max**(client.go 的
// get()),而 companyfacts 自己的 NetIncomeLoss 同样含 filed 晚一年的对比期条目
// (实测 2025-01-01~2025-03-31 有 filed=2025-04-30 与 2026-04-29 两条,值都是 4577000000),
// keepLatest 取最新 ⇒ 季度级可用日期仍是 2026-04-29。加本步前后 live 实测
// valuation_daily 均为 ~370 天 / pctl_5y ~118 点,**无差异**。
// 那个约一年的滞后是**既有的、对所有公司一视同仁**的语义(重述以最新披露为准),
// 要改得动 keepLatest 本身 —— 影响全部公司与 golden 基线,不在本任务范围内。
//
// # 为什么按「值」分组而不是无脑取最早
//
// 值**变了**才是真重述,那时两条都要留下,由下游 keepLatest 取 filed 最新的重述值 ——
// 无脑取最早会让重述永远生效不了(TestClassRawFactsKeepsBothOnRestatement)。
// 按值分组后:值没变 ⇒ 只留最早(恢复真实可用日期);值变了 ⇒ 各留各的最早,
// 重述那条仍晚于原始条,keepLatest 语义完全不受影响。
//
// 浮点用精确相等:值来自同一份 XBRL 的十进制文本("2.32"),同文本必然解析成同一个
// float64,不存在需要容差的累积误差。
func earliestFiledPerValue(facts []rawFact) []rawFact {
	type key struct {
		start, end string
		val        float64
	}
	earliest := map[key]int{} // → 在 out 中的下标
	out := make([]rawFact, 0, len(facts))
	for _, f := range facts {
		k := key{f.Start, f.End, f.Val}
		i, seen := earliest[k]
		if !seen {
			earliest[k] = len(out)
			out = append(out, f)
			continue
		}
		if f.Filed < out[i].Filed { // 日期为零填充 YYYY-MM-DD,字典序即时间序
			out[i] = f
		}
	}
	return out
}

// fetchClassInstance 取单份实例文档并解析。上限判定与 fetchSegmentInstance 同构:
// **先判截断再判解析错** —— 截断可能恰好落在结构边界上让解析「成功」,那样得到的是
// 残缺数据,比报错更危险。
func (c *Client) fetchClassInstance(url, axis string) ([]classFact, error) {
	resp, err := c.httpGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	cr := &countingReader{r: io.LimitReader(resp.Body, c.maxInstanceBytes)}
	facts, parseErr := parseClassFacts(cr, axis)
	if cr.n >= c.maxInstanceBytes {
		return nil, fmt.Errorf("response exceeds %d byte limit", c.maxInstanceBytes)
	}
	if parseErr != nil {
		return nil, parseErr
	}
	return facts, nil
}

// fetchClassDimensionedEPS 是本路径的顶层:submissions → 最近若干份报告的实例文档 →
// 选主类别 → 合成 rawFact。
//
// 降级:submissions 不可达 → 返回 error(调用方只记日志,不上抛);单份实例文档 404
// (2019 前老式非 inline XBRL filing,_htm.xml 推导不成立)、超限或解析失败 → 跳过该份
// 并记录,不中断其余。全部失败 ⇒ 返回空切片 ⇒ 调用方保持「EPS 缺失」的现状。
func (c *Client) fetchClassDimensionedEPS(cik, axis string) (eps, shares []rawFact, err error) {
	filings, err := c.recentFilings(cik, time.Time{}, maxClassEPSFilings)
	if err != nil {
		return nil, nil, err
	}
	var all []classFact
	for _, f := range filings {
		url := c.instanceURL(cik, f)
		facts, ferr := c.fetchClassInstance(url, axis)
		if ferr != nil {
			log.Printf("edgar: skipping class-EPS instance %s: %v", url, ferr)
			continue
		}
		filed := f.filingDate.Format(dateLayout)
		for i := range facts {
			facts[i].filed = filed
			facts[i].form = f.form
		}
		all = append(all, facts...)
	}
	member := dominantClass(all)
	if member == "" {
		return nil, nil, nil
	}
	eps, shares = classRawFacts(all, member)
	log.Printf("edgar: CIK %s using class-dimensioned EPS from member %s (%d eps / %d shares facts from %d filings)",
		cik, member, len(eps), len(shares), len(filings))
	return eps, shares, nil
}
