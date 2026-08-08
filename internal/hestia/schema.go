package hestia

import (
	"strings"

	"github.com/newthinker/atlas/internal/macro/bitemporal"
)

const (
	// TableObservations 是权威观测表：双时态，修订产生新行而非覆盖。
	TableObservations = "hestia_observations"
	// TablePending 收未过闸的数据。它的消费者只有人。
	TablePending = "hestia_pending"

	viewCurrent = "v_hestia_current"
)

// metaColumns 是七个元数据列，顺序与 INSERT 的参数顺序一致。
// 前三个同时是双时态主键。
//
// # 这个顺序是跨任务契约的第二端
//
// 三处必须同序：types.go 的 Meta 字段声明顺序、本处、store.go 的 insert 取值顺序。
// 任一处不同序，写入会**静默错位**——七列都是 TEXT，把 extractor 的值塞进
// caliber_version 列不触发任何数据库错误，只让下游读到胡话。
//
// TestMetaFieldOrderIsCrossTaskContract 钉住 Meta 那一端，
// TestMetaColumnsMatchMetaStructByReflect（本文件的测试）钉住这一端：它用 reflect
// 从 Meta 取字段名转 snake_case 与本切片逐一比对，所以这份清单**不能**手工与
// Meta 对齐后就算数，改任一端另一端会立刻转红。
var metaColumns = []string{
	"period", "period_type", "published_at",
	"article_id", "caliber_version", "extractor", "ingested_at",
}

// observationsDDL 由 fieldOrder 生成建表语句。
//
// 业务列逐列从 fieldOrder 派生、按其顺序排列——手写一份列清单就是 DDL 之外的
// 第二份 schema 定义，必然与 INSERT 的列序不同步。
//
// 没有 is_current 列：当前行由 published_at 派生（ADR-0009）。存一份可派生
// 的状态只会多一个与事实不符的机会，且乱序回填时还要额外判断。
//
// 业务列是 REAL 而非 TEXT：亲和性错了不会报错，Scan 进 float64 照样成功，
// 但 SQLite 会按字典序比较，MAX() 与范围查询全部静默算错。
func observationsDDL() string {
	var b strings.Builder
	b.WriteString("CREATE TABLE IF NOT EXISTS " + TableObservations + " (\n")
	for _, c := range metaColumns {
		b.WriteString("  " + c + " TEXT NOT NULL,\n")
	}
	for _, f := range fieldOrder {
		b.WriteString("  " + f + " REAL,\n")
	}
	b.WriteString("  PRIMARY KEY (period, period_type, published_at)\n)")
	return b.String()
}

// pendingDDL 建未过闸数据表。
//
// 它存 JSON 而不是 54 列，理由与观测表相反：消费者只有人，不做查询、不做
// 聚合、不进 Grafana。给它 54 列会让「被拒绝的数据」和「权威数据」看起来
// 一样正规——ADR-0003 方案 D 担心的正是「下游迟早忘记过滤标记」。
// 结构上就不像观测表，是有意的。
//
// 主键带 ingested_at：同一期反复失败会留下多条记录，那本身是诊断信息。
func pendingDDL() string {
	return `CREATE TABLE IF NOT EXISTS ` + TablePending + ` (
  period       TEXT NOT NULL,
  period_type  TEXT NOT NULL,
  published_at TEXT NOT NULL,
  article_id   TEXT NOT NULL,
  extractor    TEXT NOT NULL,
  ingested_at  TEXT NOT NULL,
  report       TEXT NOT NULL,
  values_json  TEXT NOT NULL,
  PRIMARY KEY (period, period_type, published_at, ingested_at)
)`
}

// currentViewDDL 建「当前行」视图。
//
// 视图主体由 bitemporal.CurrentQuery 生成而非手写，保证它与 Lookup 用的是
// 同一套业务键——两者不一致时，「当前行」和幂等判断会对不上，而这种错
// 不会报错，只会给出错误的数据。
func currentViewDDL(spec bitemporal.Spec) string {
	return "CREATE VIEW IF NOT EXISTS " + viewCurrent + " AS " + bitemporal.CurrentQuery(spec)
}
