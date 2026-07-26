// 分部营收解析:EDGAR companyfacts **不含**分部维度,故走 submissions API 定位报告,
// 再解析报告实例文档(inline XBRL 抽出的 `_htm.xml`)里的 explicitMember 维度。
//
// 解析规则(design-spec §1.2,全部经 2026-07-25 对 MSFT CIK 789019 的 live 实测校准):
//
//  1. GET {dataURL}/submissions/CIK{10位补零}.json → filings.recent **平行数组**
//     (form/accessionNumber/primaryDocument/reportDate/filingDate),
//     筛 form ∈ {10-Q, 10-K} 且 reportDate > since。
//  2. 实例文档 URL(AD-3):
//     {archivesURL}/Archives/edgar/data/{cik 整数}/{accession 去连字符}/{primaryDocument 的 .htm→_htm.xml}
//     实测可用形如
//     https://www.sec.gov/Archives/edgar/data/789019/000119312526191507/msft-20260331_htm.xml
//     注意 /Archives/ 是**大写 A**,CIK 用整数形式(789019,非补零)。
//  3. XML **单遍** Decoder.Token() 解析(AD-11:HTTP body 不可二次读取,plan 写的「流式两遍」
//     **不可实现**;实测单文档 9.7 MB(10-Q)/10.5 MB(10-K),**禁止 Unmarshal 建全树**)。
//     一次遍历同时收集三类数据,**流结束后在内存中关联**(不假设 context 一定先于 fact 出现):
//     - contexts:<context id=…> 记录 period/startDate,endDate 与 entity/segment/xbrldi:explicitMember。
//     收录条件:**恰好一个 explicitMember** 且其 dimension 的 local name == axis
//     (排除 segment×geography 等交叉维,也排除单维但轴不匹配的 context);
//     无 startDate 的 instant 型跳过。member 值去命名空间前缀
//     (msft:IntelligentCloudMember → IntelligentCloudMember)。
//     - units:<unit id=…> 记录其直接子 <measure>。只有**恰好一个**直接子 measure 且其
//     local name 为 USD 才算 USD 单位 —— 真实文档里 U_UnitedStatesOfAmericaDollarsShare 是
//     <divide> 结构(USD/share,每股口径),其 measure 是孙元素,不会被误判为营收单位。
//     - pending facts:元素 local name ∈ revenueTags 的元素,暂存 (contextRef, tag, value, unitRef)。
//     - 关联与过滤(遍历结束后):contextRef 命中已收 context 的 fact 才计入 (period, member);
//     unitRef 非 USD(或未声明,无从确认)的 fact **排除**;同 (period, member) 多条时取
//     **tag 链优先级最高者**。
//     - 期间过滤:endDate − startDate + 1 落在 70~100 天(单季)或 350~380 天(FY,含 366 闰年)。
//  4. **每个通过过滤的期间产出一条 SegmentPeriod**(AD-2:一份报告可产多条 —— 实测一份 10-Q
//     含本季 90d、去年同季 90d、本年累计 274d、去年累计 274d 四组期间,一份 10-K 含 3 个财年);
//     同 (period, member) 跨报告重复 → 取 FilingDate 更晚者(重述以最新披露为准)。
//  5. 请求间**无并发**(EDGAR 限频);**两个 host 的请求都必须携带 UA**(www.sec.gov 无 UA 同样 403)。
//  6. **双重上限**(AD-11):响应体经 io.LimitReader 限 64 MB,超限该期跳过(不 OOM);
//     单次调用最多处理最近 12 份符合条件的报告(防 since 为零值时数十份 × ~10MB 的首跑风暴)。
//  7. **降级**:实例文档 404(2019 前老式非 inline XBRL filing,_htm.xml 推导不成立)或解析失败
//     → 跳过该期并记录,不中断整家。
package edgar

import (
	"encoding/json"
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

const (
	defaultArchivesURL      = "https://www.sec.gov"
	defaultMaxInstanceBytes = int64(64 << 20)
	// maxSegmentFilings 是单次调用的回看上限。filings.recent 里 10-Q/10-K 实测可达 25 份/家,
	// × ~10 MB/份,不设上限时 since 为零值的首跑必然超时或触发 SEC 限频。
	maxSegmentFilings = 12
	dateLayout        = "2006-01-02"
)

// SegmentPeriod is one reporting period's segment revenue breakdown.
type SegmentPeriod struct {
	PeriodStart, PeriodEnd time.Time
	FilingDate             time.Time
	Form                   string             // "10-Q" | "10-K"
	Values                 map[string]float64 // member local name（去命名空间）→ revenue
}

// filing 是 submissions 里一份通过筛选的报告。
type filing struct {
	form, accession, document string
	reportDate, filingDate    time.Time
}

type submissionsDoc struct {
	Filings struct {
		Recent struct {
			Form            []string `json:"form"`
			AccessionNumber []string `json:"accessionNumber"`
			PrimaryDocument []string `json:"primaryDocument"`
			ReportDate      []string `json:"reportDate"`
			FilingDate      []string `json:"filingDate"`
		} `json:"recent"`
	} `json:"filings"`
}

// periodKey 以字符串日期为键,便于跨报告归并同一期间。
type periodKey struct{ start, end string }

type memberKey struct {
	period periodKey
	member string
}

// xmlContext / xmlUnit 只用于解码**单个** <context> / <unit> 子树(每个约几百字节),
// 外层始终是 Decoder.Token() 流式遍历 —— 不构成全树。
type xmlContext struct {
	ID     string `xml:"id,attr"`
	Period struct {
		StartDate string `xml:"startDate"`
		EndDate   string `xml:"endDate"`
	} `xml:"period"`
	Members []struct {
		Dimension string `xml:"dimension,attr"`
		Value     string `xml:",chardata"`
	} `xml:"entity>segment>explicitMember"`
}

// Measures 只匹配**直接**子元素:<divide> 里的 measure 是孙元素,不会命中,
// 因此 USD/share 这类复合单位天然被排除在「简单 USD 单位」之外。
type xmlUnit struct {
	ID       string   `xml:"id,attr"`
	Measures []string `xml:"measure"`
}

// segmentContext 是一个已通过收录条件的 context。
type segmentContext struct {
	period periodKey
	member string
}

// FetchSegmentRevenue 返回 reportDate > since 的报告中、通过期间过滤的各期分部营收,
// 按 PeriodEnd 升序。axis 为维度轴 local name(模板 segment_axis)。
// 单份报告失败只跳过该份并记录,不影响其余(见文件头解析规则 7)。
func (c *Client) FetchSegmentRevenue(cik, axis string, since time.Time) ([]SegmentPeriod, error) {
	filings, err := c.recentFilings(cik, since, maxSegmentFilings)
	if err != nil {
		return nil, err
	}

	// cell 记录某 (period, member) 当前采纳值及其来源报告,用于跨报告按 FilingDate 择新。
	type cell struct {
		val   float64
		filed time.Time
		form  string
	}
	merged := map[periodKey]map[string]cell{}
	for _, f := range filings { // 串行:EDGAR 限频,请求间无并发
		url := c.instanceURL(cik, f)
		vals, err := c.fetchSegmentInstance(url, axis)
		if err != nil {
			log.Printf("edgar: skipping segment instance %s: %v", url, err)
			continue
		}
		for pk, members := range vals {
			if merged[pk] == nil {
				merged[pk] = map[string]cell{}
			}
			for m, v := range members {
				// 同 (period, member) 已有更晚披露 → 保留旧值(重述以最新披露为准)。
				if prev, ok := merged[pk][m]; ok && prev.filed.After(f.filingDate) {
					continue
				}
				merged[pk][m] = cell{v, f.filingDate, f.form}
			}
		}
	}

	out := make([]SegmentPeriod, 0, len(merged))
	for pk, members := range merged {
		start, e1 := time.Parse(dateLayout, pk.start)
		end, e2 := time.Parse(dateLayout, pk.end)
		if e1 != nil || e2 != nil || len(members) == 0 {
			continue
		}
		sp := SegmentPeriod{PeriodStart: start, PeriodEnd: end, Values: make(map[string]float64, len(members))}
		for m, cl := range members {
			sp.Values[m] = cl.val
			// 一期的数据可能来自多份报告(部分 member 被重述),Form/FilingDate 取其中最新的那份。
			if cl.filed.After(sp.FilingDate) {
				sp.FilingDate, sp.Form = cl.filed, cl.form
			}
		}
		out = append(out, sp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PeriodEnd.Equal(out[j].PeriodEnd) {
			return out[i].PeriodStart.Before(out[j].PeriodStart)
		}
		return out[i].PeriodEnd.Before(out[j].PeriodEnd)
	})
	return out, nil
}

// httpGet 发起带 UA 的 GET。SEC 对 data.sec.gov 与 www.sec.gov **两个 host** 都要求 UA
// 携带可联系方式,缺失一律 403。
func (c *Client) httpGet(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	return c.client.Do(req)
}

// recentFilings 取 filings.recent 中 form 与 reportDate 双重命中的报告,按 reportDate 降序
// 截取最近 limit 份。显式排序而非依赖 SEC 的返回顺序。
//
// limit 由调用方给出而非写死:分部营收(maxSegmentFilings=12)与类别维度 EPS
// (maxClassEPSFilings=28)对回看深度的需求不同,两者**必须各自独立**——详见
// maxClassEPSFilings 的注释。
func (c *Client) recentFilings(cik string, since time.Time, limit int) ([]filing, error) {
	url := fmt.Sprintf("%s/submissions/CIK%010s.json", c.baseURL, cik)
	resp, err := c.httpGet(url)
	if err != nil {
		return nil, fmt.Errorf("edgar: submissions request failed for CIK %s (%s): %w", cik, url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("edgar: unexpected HTTP status %d for CIK %s (%s)", resp.StatusCode, cik, url)
	}
	var doc submissionsDoc
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("edgar: decode submissions for CIK %s (%s): %w", cik, url, err)
	}

	rec := doc.Filings.Recent
	// 平行数组:任一列短于 form 都说明数据不完整,按最短长度安全截断而非越界。
	n := len(rec.Form)
	for _, col := range [][]string{rec.AccessionNumber, rec.PrimaryDocument, rec.ReportDate, rec.FilingDate} {
		n = min(n, len(col))
	}
	var out []filing
	for i := 0; i < n; i++ {
		if rec.Form[i] != "10-Q" && rec.Form[i] != "10-K" {
			continue
		}
		report, err := time.Parse(dateLayout, rec.ReportDate[i])
		if err != nil || !report.After(since) {
			continue
		}
		filed, err := time.Parse(dateLayout, rec.FilingDate[i])
		if err != nil {
			continue
		}
		out = append(out, filing{rec.Form[i], rec.AccessionNumber[i], rec.PrimaryDocument[i], report, filed})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].reportDate.After(out[j].reportDate) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// instanceURL 按 AD-3 推导实例文档地址。primaryDocument 不以 .htm 结尾时推导不成立,
// 保持原名 —— 后续请求会 404 并走降级,比静默拼出错误路径可观测。
func (c *Client) instanceURL(cik string, f filing) string {
	doc := f.document
	if strings.HasSuffix(doc, ".htm") {
		doc = strings.TrimSuffix(doc, ".htm") + "_htm.xml"
	}
	return fmt.Sprintf("%s/Archives/edgar/data/%s/%s/%s",
		c.archivesURL, trimCIKZeros(cik), strings.ReplaceAll(f.accession, "-", ""), doc)
}

// trimCIKZeros 把 CIK 归一为整数形式(0000789019 → 789019);不可解析时原样返回。
func trimCIKZeros(cik string) string {
	if n, err := strconv.ParseInt(strings.TrimSpace(cik), 10, 64); err == nil {
		return strconv.FormatInt(n, 10)
	}
	return cik
}

// countingReader 记录已读字节数,用于区分「响应体触到上限被截断」与普通解析错误。
type countingReader struct {
	r io.Reader
	n int64
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	cr.n += int64(n)
	return n, err
}

func (c *Client) fetchSegmentInstance(url, axis string) (map[periodKey]map[string]float64, error) {
	resp, err := c.httpGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	cr := &countingReader{r: io.LimitReader(resp.Body, c.maxInstanceBytes)}
	vals, parseErr := parseSegmentInstance(cr, axis)
	// 先判上限:截断可能恰好落在结构边界上让解析「成功」,那样得到的是残缺数据,比报错更危险。
	if cr.n >= c.maxInstanceBytes {
		return nil, fmt.Errorf("response exceeds %d byte limit", c.maxInstanceBytes)
	}
	if parseErr != nil {
		return nil, parseErr
	}
	return vals, nil
}

// parseSegmentInstance 单遍遍历实例文档,收集 contexts / units / pending facts,
// 遍历结束后在内存中关联(见文件头解析规则 3)。
func parseSegmentInstance(r io.Reader, axis string) (map[periodKey]map[string]float64, error) {
	type pendingFact struct {
		contextRef, unitRef string
		priority            int
		val                 float64
	}
	contexts := map[string]segmentContext{}
	usdUnits := map[string]bool{}
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
		switch se.Name.Local {
		case "context":
			var xc xmlContext
			if err := dec.DecodeElement(&xc, &se); err != nil {
				return nil, err
			}
			// 恰好一个 explicitMember 且轴匹配才收 —— 这一条同时挡掉交叉维与「单维但轴不匹配」
			// 两类干扰(实测 MSFT 一份 10-Q 的 409 个 context 里,117 个是多维,
			// 276 个单维中只有 18 个落在分部轴上)。
			if len(xc.Members) != 1 || localName(strings.TrimSpace(xc.Members[0].Dimension)) != axis {
				continue
			}
			// instant 型(只有 <instant>,无 startDate)不是期间,跳过。
			if xc.Period.StartDate == "" || xc.Period.EndDate == "" {
				continue
			}
			contexts[xc.ID] = segmentContext{
				period: periodKey{xc.Period.StartDate, xc.Period.EndDate},
				member: localName(strings.TrimSpace(xc.Members[0].Value)),
			}
		case "unit":
			var xu xmlUnit
			if err := dec.DecodeElement(&xu, &se); err != nil {
				return nil, err
			}
			if len(xu.Measures) == 1 && localName(strings.TrimSpace(xu.Measures[0])) == "USD" {
				usdUnits[xu.ID] = true
			}
		default:
			// 复用 client.go 的 revenueTags,保证分部口径与主表营收口径一致;
			// 链上位置即优先级(0 最高),-1 = 不是营收科目。
			priority := slices.Index(revenueTags, se.Name.Local)
			if priority < 0 {
				continue
			}
			var text string
			if err := dec.DecodeElement(&text, &se); err != nil {
				return nil, err
			}
			val, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
			if err != nil {
				continue
			}
			pending = append(pending, pendingFact{
				contextRef: attrValue(se, "contextRef"),
				unitRef:    attrValue(se, "unitRef"),
				priority:   priority,
				val:        val,
			})
		}
	}

	out := map[periodKey]map[string]float64{}
	adopted := map[memberKey]int{} // 已采纳值的 tag 优先级(数值越小优先级越高)
	for _, f := range pending {
		ctx, ok := contexts[f.contextRef]
		if !ok || !usdUnits[f.unitRef] || !keepDuration(ctx.period) {
			continue
		}
		mk := memberKey{ctx.period, ctx.member}
		if prev, seen := adopted[mk]; seen && prev <= f.priority {
			continue
		}
		adopted[mk] = f.priority
		if out[ctx.period] == nil {
			out[ctx.period] = map[string]float64{}
		}
		out[ctx.period][ctx.member] = f.val
	}
	return out, nil
}

// keepDuration 过滤期间:duration = endDate − startDate + 1(含首尾),落在 70~100 天(单季)
// 或 350~380 天(FY,含 366 天闰年)才保留。真实 10-Q 还含 274 天的本年/去年累计期须丢弃。
func keepDuration(k periodKey) bool {
	start, e1 := time.Parse(dateLayout, k.start)
	end, e2 := time.Parse(dateLayout, k.end)
	if e1 != nil || e2 != nil {
		return false
	}
	days := int(end.Sub(start).Hours()/24) + 1
	return (days >= 70 && days <= 100) || (days >= 350 && days <= 380)
}

// localName 去掉 QName 的命名空间前缀(msft:IntelligentCloudMember → IntelligentCloudMember)。
// XBRL 里 dimension 属性与 explicitMember 取值都是 QName **字符串**,encoding/xml 不解析
// 属性值/文本中的前缀,必须自己截断。
func localName(qname string) string {
	if i := strings.LastIndex(qname, ":"); i >= 0 {
		return qname[i+1:]
	}
	return qname
}

func attrValue(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}
