package hestia

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

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
	Extractor      string // 必填，取值域见 validExtractors（本行刻意不抄一份）
	IngestedAt     string // 由 Save 填，RFC3339
}

// validPeriodTypes 的五个取值决定月均折算的除数（monthly 1 / q1 3 / h1 6 /
// q1_q3 9 / annual 12）。写错会让信号判定错一个量级，所以这里严格枚举而不是随便放行。
//
// ⚠️ 该折算在本包**没有实现**，这行只是声明取值的含义。
//
// q1 / q1_q3 是 TASK-001 加的，对应央行的「一季度」与「前三季度」报告 —— 两者都是
// **年初起累计**口径，与 h1（上半年）、annual（全年）同族。
//
// ⚠️ **`q1_q3` 不是 `q3`**：「前三季度」是 1–9 月累计，第三季度单季是 7–9 月，
// 两者期末月同为 09，除数却是 9 与 3。取名写成 q3 就是在这行注释警告的那个量级上
// 埋一个谁也看不出来的错。
var validPeriodTypes = map[string]bool{
	"monthly": true,
	"q1":      true,
	"h1":      true,
	"q1_q3":   true,
	"annual":  true,
}

// periodTypeList 把白名单排成定序切片，供错误信息使用。
//
// 存在的理由与 checkEnum 完全一样：合法取值只该有**一份**定义。此前有两处把
// "monthly|h1|annual" 抄进了错误文案（本文件的 Meta.validate 与 thresholds.go 的
// PeriodTypes 校验）—— 那正是 checkEnum 的注释警告过的写法，而这两处恰是它自己
// 没用 checkEnum 的地方（那个函数收 []string，白名单这边是 map）。加季度类型时
// 两份副本会**静默过期**：白名单放行了新值，错误信息却还在说旧的三个。
//
// 排序不是洁癖：map 的迭代序随机，不排的话同一个错误每次打印的取值顺序都不同。
func periodTypeList() []string {
	return slices.Sorted(maps.Keys(validPeriodTypes))
}

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

// periodEndMonth 规定期末月：年度数据的期末月必然是 12 月，上半年必然是 6 月，
// 一季度 3 月，前三季度 9 月。
//
// **monthly 是唯一不在表内的合法 period_type**，因为任意月份都合法；下面
// Meta.validate 的「查不到就跳过」（`if want, ok := ...; ok && ...`）正是靠这一点
// 实现豁免。
//
// ⚠️ 因此两张表的一致性守卫必须是**单向**的，别写成「反之亦然」：给 monthly 编一个
// 期末月，会让**除该月外每一期月报都被拒**。TestPeriodTypeMapsAreConsistent 把这两条
// 分开钉住 —— ① 本表的键都必须是合法 period_type（否则是永不执行的死配置）；
// ② 除 monthly 外每个合法 period_type 都要有期末月（否则荒谬配对静默放行）。
//
// 为什么必须查这个组合：period 与 period_type 单独看都合法，配对却可能荒谬。
// "2026-06/annual" 会用 12 去除半年报的期末月，让月均折算错一个量级——而本文件
// 的 validPeriodTypes 正写着「这三个值决定除数 1/6/12，写错会让信号判定错一个
// 量级」。更糟的是同一份年报若一次写 2026-12/annual、一次写 2026-06/annual，
// 会成为两个不同的业务键：修订链就此分叉，下游双计，而两条记录各自看起来都正常。
//
// YYYY-12/monthly 刻意放行：央行每年 1 月的年报同时含全年与 12 月单月数据，
// 解析器若把两者都抽出来，那就是真实期次；它与 YYYY-12/annual 的业务键本就
// 不同（period_type 是主键的一部分），不会混淆。
var periodEndMonth = map[string]string{
	"q1":     "03",
	"h1":     "06",
	"q1_q3":  "09",
	"annual": "12",
}

// validCaliberVersions 是央行口径变更的三个已知版本，全部来自报告原文的注释段：
//
//	2015-01  存贷款含非银行业金融机构存放/拆放款项（2020H1 报告注 2）
//	2023-01  三类非存款类金融机构纳入统计（2025 年报告注 4）
//	2025-01  M1 口径修订，纳入个人活期存款与非银支付机构备付金（2025 年报告注 5）
//
// 用枚举而不是形态正则（^\d{4}-\d{2}$）：形态检查会放行 "2024-07" 这种不对应任何
// 真实变更的版本号，而 fields.go 里写着口径断裂「只能靠 caliber_version 标注」——
// 它是那道断裂的唯一防线，放行一个虚构的边界等于让下游按它做跨期对比。
//
// 代价是央行下次改口径要改这里。那是数年一次的事，且届时本来就要人工整理受影响的
// 字段清单。M1c 建起 hestia_caliber_changes 表（豁免机制需要它）之后，这个白名单
// 应改为引用那张表。
var validCaliberVersions = []string{"2015-01", "2023-01", "2025-01"}

// —— 月报与社融独立报告的抽取器标识（M1c-3a 的 TASK-003，AD-1）——
//
// 命名与定义方式沿用 profiles.go 的 extractorV1 / extractorV2；定义在本文件而不是
// 那里，只是因为 M1c-3a 这一 wave 里 profiles.go 被另一个任务占用。
//
// 为什么不让月报复用 rule@v1：requiredFields 是 completeness 的唯一依据。月报正文
// **根本没有**国家外汇储备板块（55 篇实测只有 2 篇有），复用 rule@v1 会让每篇月报
// 恒报「缺 fx_reserve / fx_rate」；社融独立报告复用 rule@v1 则恒报「缺 27 个字段」。
// 那些字段在那种报告里不是缺失，是 absent-by-design——把 by-design 的缺席记成
// failed，completeness 这个指标就废了。
const (
	extractorMonthlyV1 = "rule-monthly@v1" // 4/5 节月报，25 个字段
	extractorMonthlyV2 = "rule-monthly@v2" // 7 节月报（含社融两节），52 个字段
	extractorTSFStock  = "tsf-stock@v1"    // 社融存量独立报告，18 个字段
	extractorTSFFlow   = "tsf-flow@v1"     // 社融增量独立报告，9 个字段
)

// validExtractors 是已定义的抽取器标识。
//
// llm-fallback@v1 到 M1c 才实现，先列入是因为 Meta 的取值域应当由数据模型定义，
// 而不是由「当前实现到哪一步」定义。
//
// 新增 rule@vN 时必须同步更新这里——那一步正是提醒去补 completeness 的 profile
// （M1b-3）：两者不同步会让新模板的期次用错必填集，而那是静默的。
//
// 后四个是 M1c-3a 的 TASK-003 加的（AD-1），它们的必填集见 required.go。
// ⚠️ 加完之后**还没有任何生产代码会返回它们**：detectExtractor 认出它们是
// TASK-004、extractFields 消费是 TASK-006、Parse 分派是 TASK-007。取值域先行
// 是刻意的——Meta 的取值域应当由数据模型定义，而不是由「当前实现到哪一步」
// 定义（llm-fallback@v1 早就是这么处理的）。
var validExtractors = []string{
	"rule@v1", "rule@v2", "llm-fallback@v1",
	extractorMonthlyV1, extractorMonthlyV2, extractorTSFStock, extractorTSFFlow,
}

// checkEnum 是取值域校验的共同形状：不在白名单里就报错，并把白名单**逐字**列进
// 错误信息。
//
// 用切片而不是 map[string]bool，是为了让「合法取值」只有一份定义：错误信息里的
// "want a|b|c" 由白名单本身拼出来，而不是另抄一遍。抄一遍的版本会在下次加口径版本
// 或抽取器时静默过期——白名单放行了新值，错误信息却还在说旧的三个，而那条信息正是
// 调用方判断自己该填什么的唯一依据。
//
// field 要传**完整**字段名（含 "meta." 之类的前缀）。前缀曾硬编码在这里，
// M1b-3 挪到了调用点：Thresholds 的口径豁免复用本函数，而豁免不在 Meta 里，
// 硬编码会让它输出 "unknown meta.caliber_exemptions[0].Version" —— 那是错的，
// 它指向一个不存在的字段路径，照它去 Meta 里找只会白找一轮。
// 两处 Meta 调用的输出因此逐字不变，前缀只是从函数体挪到了调用点。
func checkEnum(field, val string, allowed []string) error {
	if slices.Contains(allowed, val) {
		return nil
	}
	return fmt.Errorf("hestia: unknown %s %q (want %s)",
		field, val, strings.Join(allowed, "|"))
}

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
		return fmt.Errorf("hestia: invalid period_type %q (want %s)",
			m.PeriodType, strings.Join(periodTypeList(), "|"))
	}
	if !periodRE.MatchString(m.Period) {
		return fmt.Errorf("hestia: meta.period %q must match YYYY-MM", m.Period)
	}
	// 排在 periodRE 之后：那道检查已保证形如 YYYY-MM，这里才能安全切片取月份。
	if want, ok := periodEndMonth[m.PeriodType]; ok && m.Period[5:] != want {
		return fmt.Errorf(
			"hestia: period %q is not a valid %s period (month must be %s)",
			m.Period, m.PeriodType, want)
	}
	if !publishedAtRE.MatchString(m.PublishedAt) {
		return fmt.Errorf("hestia: meta.published_at %q must match YYYY-MM-DD", m.PublishedAt)
	}
	if err := checkEnum("meta.caliber_version", m.CaliberVersion, validCaliberVersions); err != nil {
		return err
	}
	if err := checkEnum("meta.extractor", m.Extractor, validExtractors); err != nil {
		return err
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
