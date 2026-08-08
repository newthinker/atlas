package hestia

import (
	"fmt"
	"regexp"

	"github.com/newthinker/atlas/internal/macro/bitemporal"
)

// Meta 是一期观测的元数据，全部为字符串。
//
// 前六项由调用方提供且必填（对应列均为 NOT NULL）；IngestedAt 由 Store.Save
// 填写，调用方传值一律被覆盖——它是入库时刻，只有 Store 知道，让调用方传
// 等于允许它撒谎。
//
// # 字段声明顺序是跨任务契约
//
// 下面七个字段的**声明顺序**被另外两处复用，三处必须同序：
//   - schema.go 的 `metaColumns`（七个元数据列，也是 INSERT 的列顺序）
//   - store.go 的 `insert`（按同样顺序从 Meta 取值填 args）
//
// 任一处不同序，写入会**静默错位**：这些列都是 TEXT，把 extractor 的值塞进
// caliber_version 列不触发任何数据库错误，只让下游读到胡话。增删或重排字段
// 时必须同步改另外两处；TestMetaFieldOrderIsCrossTaskContract 会在本处变动
// 时转红，把「记得同步」从自觉变成机制。
type Meta struct {
	Period         string // 必填，"2026-06"，期末月
	PeriodType     string // 必填，monthly | h1 | annual
	PublishedAt    string // 必填，取自 <meta name="PubDate">，双时态的时间轴
	ArticleID      string // 必填，一级幂等键；会随站点迁移而变，故需二级键兜底
	CaliberVersion string // 必填，2015-01 | 2023-01 | 2025-01
	Extractor      string // 必填，rule@v1 | rule@v2 | llm-fallback@v1
	IngestedAt     string // 由 Save 填，RFC3339
}

// validPeriodTypes 的三个取值决定月均折算的除数（1 / 6 / 12）。
// 写错会让信号判定错一个量级，所以这里严格枚举而不是随便放行。
var validPeriodTypes = map[string]bool{"monthly": true, "h1": true, "annual": true}

// periodRE / publishedAtRE 落实 M1a 明文指派给写入方的形态契约（G1）。
//
// bitemporal.Lookup 用 SQL 的 MAX() 取最大 revision、Classify 用 Go 的字符串
// 比较，两者都是字典序——只有 ISO 8601 形态时字典序才等于时间序。那个包的文档
// 注释写死：喂进 "2026-7-15"（少补零）会**静默判错、零告警**，且「明确不在这里
// 加形态校验……由建表方与写入方保证」。Store 就是那个写入方，而 PublishedAt 取自
// 外部 HTML 的 <meta name="PubDate">，形态不受控恰是最可能发生的事。
//
// 不校验的三种后果全都无声：①少补零 → 字典序失效 → 当前行视图长期返回旧版本；
// ②混入 RFC3339 → 同一报告判成 Revision 而非 Duplicate → 多一行假修订且
// refreshArticleID 永不触发；③Period 写成 "2026-6" → 变成另一个业务键 → 同一
// 日历月在视图里出现两次，下游环比同比静默算错。
//
// 只校形态，不校日历有效性："2026-13" / "2026-02-31" 仍会放行。形态是字典序失效
// 的**充分**修复（定长补零即可），日历有效性是另一个问题，留给闸门层（M1b-3）。
var (
	periodRE      = regexp.MustCompile(`\A[0-9]{4}-[0-9]{2}\z`)
	publishedAtRE = regexp.MustCompile(`\A[0-9]{4}-[0-9]{2}-[0-9]{2}\z`)
)

// validate 是小写的：校验是 Save 的内部环节。导出它等于给调用方一个
// 「先自己校验、再绕过 Save 直接写库」的暗示，而 Save 是 ADR-0003 在同机
// 场景下唯一的防线。
func (m Meta) validate() error {
	// 逐项按声明顺序检查，错误信息带 "meta." 前缀并指名字段——注意 "period" 是
	// "period_type" 的子串，调用方若用子串匹配判断是哪个字段会踩坑，故给完整前缀。
	required := []struct{ name, val string }{
		{"period", m.Period},
		{"period_type", m.PeriodType},
		{"published_at", m.PublishedAt},
		{"article_id", m.ArticleID},
		{"caliber_version", m.CaliberVersion},
		{"extractor", m.Extractor},
	}
	for _, f := range required {
		if f.val == "" {
			return fmt.Errorf("hestia: meta.%s must not be empty", f.name)
		}
	}
	if !validPeriodTypes[m.PeriodType] {
		return fmt.Errorf("hestia: invalid period_type %q (want monthly|h1|annual)", m.PeriodType)
	}
	if !periodRE.MatchString(m.Period) {
		return fmt.Errorf("hestia: meta.period %q must match YYYY-MM", m.Period)
	}
	if !publishedAtRE.MatchString(m.PublishedAt) {
		return fmt.Errorf("hestia: meta.published_at %q must match YYYY-MM-DD", m.PublishedAt)
	}
	return nil
}

// Observation 是一期报告解析出的全部数据。
//
// Values 只含实际存在的字段——键不存在即该字段缺失。这个表示是刻意的：
// 2020 年的报告只有六个板块、没有社融，54 个字段中只有约 20 个可填。
// 若用零值表示缺失，「本就没有」与「解析漏了」就无法区分，而后者是
// 必须拦下的解析故障。
type Observation struct {
	Meta   Meta
	Values map[string]float64
}

// CheckStatus 是单道闸门的结果。三态而非布尔：有些检查在某些期次上
// 无法执行（字段不存在、库中无前期），那既不是通过也不是失败。
type CheckStatus string

const (
	CheckPassed  CheckStatus = "passed"
	CheckFailed  CheckStatus = "failed"
	CheckSkipped CheckStatus = "skipped"
)

type Check struct {
	ID     string // "deposit_sum"
	Status CheckStatus
	Value  *float64 // 实测值；无意义时为 nil
	Reason string   // Skipped 必填：absent_field:<name> | no_prior_period
}

// ValidationReport 是七道闸门的汇总。
//
// 本子迭代只定义它，闸门实现在 M1b-3。但 Store.Save 的签名从第一天就
// 要求它——否则中间状态下会存在一个绕过校验的写入口，而那正是 ADR-0003
// 在同机场景下唯一的防线。
type ValidationReport struct {
	Passed bool
	Checks []Check
}

// Outcome 告诉调用方这次写入落在哪、是什么性质，用于日志与告警分级。
type Outcome struct {
	Verdict bitemporal.Verdict // New | Duplicate | Revision | OutOfOrder
	Table   string             // TableObservations | TablePending
}
