package hestia

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/macro/bitemporal"
	_ "modernc.org/sqlite"
)

// Store 是 Hestia 观测数据的唯一写入通道。
//
// 它不导出任何绕过校验的写方法：Save（TASK-005）是唯一入口，且签名强制要求
// ValidationReport。DB() 暴露的句柄供只读用途——Grafana 直连、Go 侧派生计算、
// M1b-4 的 article_id 幂等检查都要自己发查询。
//
// # 不支持 schema 迁移，这是显式非目标
//
// 本包不做 ALTER TABLE 式的自动迁移。理由是迁移一旦自动化，「加一列」和
// 「改一列的语义」在代码里长得一模一样，而后者需要回填历史数据。
// 取而代之的是 NewStore 开库后做一次一致性检查（见 verifyObservationsSchema）：
// 库比本版代码少列就**当场失败并给迁移提示**，而不是等到第一次 Save 才炸。
type Store struct {
	db   *sql.DB
	spec bitemporal.Spec
	now  func() time.Time // 便于测试固定 ingested_at
}

// NewStore 打开（或创建）库并建好表与视图。
func NewStore(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating hestia db dir: %w", err)
		}
	}

	// 业务键与 revision 列必须与 schema.go 的视图、Lookup 的幂等判断同源：
	// 少一个业务键会让「当前行」跨期混算，而那种错不报错，只给出错误的数据。
	spec, err := bitemporal.NewSpec(TableObservations,
		[]string{"period", "period_type"}, "published_at")
	if err != nil {
		return nil, fmt.Errorf("hestia: building bitemporal spec: %w", err)
	}

	// DSN 沿用 crisis 的约定：WAL + busy_timeout
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening hestia db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting hestia db: %w", err)
	}
	for _, ddl := range []string{observationsDDL(), pendingDDL(), currentViewDDL(spec)} {
		if _, err := db.Exec(ddl); err != nil {
			db.Close()
			return nil, fmt.Errorf("creating hestia schema: %w", err)
		}
	}
	if err := verifyObservationsSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, spec: spec, now: time.Now}, nil
}

// verifyObservationsSchema 比对库中实际的列与本版代码期望的列（G7）。
//
// 存在的理由：上面那句 CREATE TABLE IF NOT EXISTS 对**已存在**的表静默无操作。
// 业务字段清单刚从约 20 涨到 54 且还会再涨，不检查的话老库能一路开到第一次
// Save 才炸，错误是驱动层的 no such column，而且只在恰好含新字段的那一期出现
// ——上线后可能几周才复现一次，且现场早已不在。
//
// 只拦「库比代码**少**列」这一个方向。多出来的列（用旧版二进制打开新版建的库）
// 刻意放行：INSERT 显式列名，多余列取 NULL，无害；在这里一并失败会让一次回滚
// 变成硬故障。两个方向的危害不对称，处置也就不该对称。
// TestNewStoreToleratesUnknownExtraColumns 把这个决定钉成可执行契约。
//
// 只查观测表：它是唯一随 fieldOrder 增长的表。pending 表 8 列固定，同类漂移
// 的可能性远低——这是范围裁剪，不是判它安全。
func verifyObservationsSchema(db *sql.DB) error {
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, TableObservations)
	if err != nil {
		return fmt.Errorf("hestia: reading %s schema: %w", TableObservations, err)
	}
	defer rows.Close()

	have := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("hestia: reading %s schema: %w", TableObservations, err)
		}
		have[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("hestia: reading %s schema: %w", TableObservations, err)
	}

	var missing []string
	for _, col := range slices.Concat(metaColumns, fieldOrder) {
		if !have[col] {
			missing = append(missing, col)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	sample := missing[:min(len(missing), 5)]
	return fmt.Errorf(
		"hestia: %s is missing %d column(s) (e.g. %s): this database was created by an "+
			"older schema and CREATE TABLE IF NOT EXISTS does not add columns. "+
			"Automatic migration is an explicit non-goal; migrate manually with ALTER TABLE "+
			"or rebuild the database at a new path",
		TableObservations, len(missing), strings.Join(sample, ", "))
}

func (s *Store) Close() error { return s.db.Close() }

// DB 暴露只读用途的句柄：Grafana 走插件直连，Go 侧的派生计算（窗口函数）
// 与 M1b-4 的 article_id 一级幂等检查都需要自己发查询。
// 写入仍然只能走 Save。
func (s *Store) DB() *sql.DB { return s.db }

// Save 是唯一的写入口。
//
// 没有 ValidationReport 就调不了它——这是 ADR-0003 在同机场景下的类型级表达。
// 物理隔离消失后（Atlas 自带 LLM、两侧同机），「LLM 不得写入事实来源」只剩代码
// 纪律，这个签名就是那条纪律。
//
// 未过闸的数据不报错，而是分流到 hestia_pending。若只是拒绝，调用方总有可能忘记
// 写 pending，那期数据就彻底消失了。一个入口、两个目的地。
//
// 四道输入校验全部排在写库之前，且任何一道不通过都**零行落盘**——「先写再报错」
// 在只看 error 的测试下完全隐形，而脏数据一旦进权威表就没有痕迹说明它没过闸。
func (s *Store) Save(ctx context.Context, obs Observation, rep ValidationReport) (Outcome, error) {
	if err := obs.Meta.validate(); err != nil {
		return Outcome{}, err
	}
	if err := checkValues(obs.Values); err != nil {
		return Outcome{}, err
	}
	if err := checkReportConsistency(rep); err != nil {
		return Outcome{}, err
	}

	// 入库时刻只有 Store 知道，调用方传的一律覆盖——让调用方决定它等于允许它撒谎。
	// 改的是值参数的副本，调用方手里的 Observation 不受影响。
	//
	// 用 RFC3339Nano 而非 RFC3339（G8）：pending 的主键含 ingested_at，而 RFC3339
	// 只有秒精度——即时重试循环里同一期两次过闸失败会撞 UNIQUE constraint，第二次
	// Save 直接报错，那次尝试的诊断信息全部丢失，而 pending 存在的唯一理由就是
	// 保留诊断信息。
	//
	// 代价（已登记，见 TestIngestedAtLexicalOrderIsNotTimeOrder）：RFC3339Nano 去掉
	// 小数尾零，于是整秒渲染成 "…:00Z"、半秒渲染成 "…:00.5Z"，而 '.' < 'Z' ——
	// **字典序与时间序相反**。当前无害：ingested_at 不是 revision 列（published_at
	// 才是），bitemporal 的 MAX() 与 Classify 都不碰它，pending 主键也只要求唯一。
	// 但不要对 ingested_at 做 ORDER BY 或 MAX()。
	obs.Meta.IngestedAt = s.now().UTC().Format(time.RFC3339Nano)

	// 用 s.spec 而不是另造一个：NewStore 用它部署了视图，另造的那个与视图不同源时，
	// 「当前行」和幂等判断会对不上，且不报错。
	key := bitemporal.Key{
		"period":      obs.Meta.Period,
		"period_type": obs.Meta.PeriodType,
	}
	state, err := bitemporal.Lookup(ctx, s.db, s.spec, key)
	if err != nil {
		return Outcome{}, err // Lookup 的错误已带表名
	}
	verdict := bitemporal.Classify(state, obs.Meta.PublishedAt)

	// Passed 检查排在 Classify 之后：若先返回，Outcome.Verdict 会是零值，
	// 而 bitemporal.New 恰好是 0——未过闸的数据会被标成「新增」。
	// 多一次查询换来告警能说清「是新一期被拦，还是一次修订被拦」。
	if !rep.Passed {
		if err := s.savePending(ctx, obs, rep); err != nil {
			return Outcome{}, err
		}
		return Outcome{Verdict: verdict, Table: TablePending}, nil
	}

	if verdict == bitemporal.Duplicate {
		// 站点迁移换了 article_id，发布日不变。写新行会造出一个假修订；
		// 什么都不做则一级幂等检查（按 article_id 查）永远 miss，每月重抓一次。
		// 正确动作是刷新那一行的 id——它记录「这行数据最后一次在哪个 URL 被看到」。
		if err := s.refreshArticleID(ctx, obs); err != nil {
			return Outcome{}, err
		}
		return Outcome{Verdict: verdict, Table: TableObservations}, nil
	}

	if err := s.insert(ctx, obs); err != nil {
		return Outcome{}, err
	}
	return Outcome{Verdict: verdict, Table: TableObservations}, nil
}

// refreshArticleID 处置 Duplicate：只更新 article_id，不新增行。
//
// # 只更新 article_id 意味着重跑抽取的新值会被丢弃（G3，已登记的取舍）
//
// 上线 rule@v2 后回填重跑历史是必然操作，届时每期 published_at 都没变 → 全判
// Duplicate → 走到这里 → **v2 新抽出来的字段一个都没写进去，extractor 列还写着
// 旧抽取器**，而 Save 返回 nil，运维看到「N 期处理完毕、零错误」。
//
// 之所以仍然只刷 id：本任务的 DoD 明文要求「只更新 article_id，不新增行」，
// 覆盖式更新会直接违反它；而覆盖也不是无代价的——它会无声地毁掉上一次抽取的
// 结果，而本表没有「抽取器版本」这根轴来承载两次不同的读数。
//
// TestSaveDuplicateDiscardsRicherValues 把丢弃的**全部可观测后果**钉成契约：
// 若将来决定改成覆盖式更新，那条会转红，迫使他显式推翻这个取舍。
func (s *Store) refreshArticleID(ctx context.Context, obs Observation) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE `+TableObservations+` SET article_id = ?
		 WHERE period = ? AND period_type = ? AND published_at = ?`,
		obs.Meta.ArticleID, obs.Meta.Period, obs.Meta.PeriodType, obs.Meta.PublishedAt)
	if err != nil {
		return fmt.Errorf("hestia store refresh article_id %s/%s: %w",
			obs.Meta.Period, obs.Meta.PeriodType, err)
	}
	return nil
}

// checkValues 把 Values 的键与值分别过一遍闸门。
//
// 键：白名单之外的键会拼进 INSERT 的列名，与 bitemporal 的 identRE 同一把关态度。
//
// 值：非有限值（NaN / ±Inf）必须挡在库外。NaN 写进 SQLite 后 typeof 是 null、
// IS NULL 为真——**与「字段缺失」完全不可区分**，而「用 map 的键是否存在表示缺失」
// 正是本设计区分「本就没有」与「解析漏了」的核心，NaN 恰好把它绕过去。比率型字段
// 的 0/0 是最常见来源。另：json.Marshal(NaN) 直接报错，NaN 若流到 pending 路径会让
// savePending 失败 → Save 返回 error → 两张表都没有这条数据，而 pending 存在的
// 理由正是「不让那期数据彻底消失」。
func checkValues(values map[string]float64) error {
	for _, f := range fieldOrder {
		v, ok := values[f]
		if !ok {
			continue
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf(
				"hestia: field %q has non-finite value %v: NaN and Inf are indistinguishable "+
					"from a missing field once stored, and JSON encoding rejects them", f, v)
		}
	}
	// 未知键单独一轮：上面按 fieldOrder 遍历只看得见白名单内的键。
	// 报错列出全部未知键并排序，让同一份坏输入每次给出同样的错误串。
	var unknown []string
	for k := range values {
		if !allFields[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("hestia: unknown field(s) %s (not in fieldOrder)",
			strings.Join(unknown, ", "))
	}
	return nil
}

// checkReportConsistency 拒绝自相矛盾的报告（G10）。
//
// 签名要求 ValidationReport 是同机场景下的唯一防线，但类型只要求报告**存在**：
// ValidationReport{Passed: true} 是 22 个字符的旁路。这里挡的是一个很具体的
// bug 类——加了失败检查却忘了把 Passed 置 false。观测表不存任何校验痕迹
// （pending 才存 report），那种行一旦落库，事后无从审计它是否真跑过闸门。
//
// 只拦「Passed 却有 CheckFailed」这一个方向：Passed=false 带失败检查是正常的
// 未过闸，CheckSkipped 也不是失败。
func checkReportConsistency(rep ValidationReport) error {
	if !rep.Passed {
		return nil
	}
	var failed []string
	for _, c := range rep.Checks {
		if c.Status == CheckFailed {
			failed = append(failed, c.ID)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf(
			"hestia: validation report contradicts itself: Passed is true but check(s) %s failed",
			strings.Join(failed, ", "))
	}
	return nil
}

// insertSQL 生成一行观测的 INSERT 语句与参数。
//
// 抽成独立函数而不是埋在 insert 里，是为了让「按 fieldOrder 遍历而非按 map」
// 成为**可断言**的事实：没有这个观测点，包内就只能靠读代码确认，那是 review
// 不是 test。
//
// 元数据取值顺序必须与 metaColumns 一致，而 metaColumns 又与 Meta 的字段声明
// 顺序一致——三处同序。这七列都是 TEXT，把 extractor 的值塞进 caliber_version
// 列不触发任何数据库错误，只让下游读到胡话。
//
// 只写 Values 中实际存在的列，其余由 SQLite 置 NULL——与「键不存在即字段缺失」
// 的表示一致，且让显式写入的 0（「增量为零」）与未提供（「未披露」）可区分。
func insertSQL(obs Observation) (string, []any) {
	cols := make([]string, 0, len(metaColumns)+len(obs.Values))
	args := make([]any, 0, len(metaColumns)+len(obs.Values))

	cols = append(cols, metaColumns...)
	args = append(args,
		obs.Meta.Period, obs.Meta.PeriodType, obs.Meta.PublishedAt,
		obs.Meta.ArticleID, obs.Meta.CaliberVersion, obs.Meta.Extractor,
		obs.Meta.IngestedAt)

	for _, f := range fieldOrder {
		if v, ok := obs.Values[f]; ok {
			cols = append(cols, f)
			args = append(args, v)
		}
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",")
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		TableObservations, strings.Join(cols, ", "), placeholders), args
}

// insert 写一行观测。
func (s *Store) insert(ctx context.Context, obs Observation) error {
	q, args := insertSQL(obs)
	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("hestia store insert %s/%s: %w",
			obs.Meta.Period, obs.Meta.PeriodType, err)
	}
	return nil
}

// savePending 收未过闸的数据。
//
// 存 JSON 而非 54 列：消费者只有人，不做查询也不进 Grafana。让它结构上就
// 不像观测表，是为了防止下游把「被拒绝的数据」当成权威数据用。
//
// # pending 表不做 schema 漂移检查，这是有论证的决定（boundary[2]）
//
// TASK-004 的 verifyObservationsSchema 只覆盖观测表。这里仍然不加列存在性校验，
// 理由是 G7 那条「静默到某一期才炸」的危害在 pending 上**结构性地不成立**：
//
//	观测表：INSERT 的列由 Values 里实际存在的字段决定 ⇒ 列清单随数据变化 ⇒
//	        缺某一列只在恰好含该字段的那一期才炸，可能上线数周后才复现。
//	pending：下面这八列**固定不变**，每次写 pending 都用同一份列清单 ⇒ 任何不符
//	        在第一次写 pending 时就确定性地炸，不存在「特定期次才暴露」。
//
// 剩下的要求只是「失败要可定位」，由下面的错误包装（步骤名 + 期次）满足。
// TestSavePendingFailsLoudlyOnDriftedPendingTable 把这两点变成可执行证据，
// 而不是停留在声明。
//
// 未覆盖的残余：某个版本只改 pendingDDL 而不动 fieldOrder（观测表检查不会触发），
// 或有人手工改了 pending 表结构。两者都只会导致第一次写 pending 时确定性失败。
//
// 不加校验的代价评估（复核 TASK-004 的结论，仍然成立）：加校验需要一份 pending
// 的期望列清单，而 pendingDDL 在 schema.go 里已经有一份——那会引入本包明确反对的
// 「DDL 之外的第二份 schema 定义」，两份必然不同步。
func (s *Store) savePending(ctx context.Context, obs Observation, rep ValidationReport) error {
	reportJSON, err := json.Marshal(rep)
	if err != nil {
		return fmt.Errorf("hestia: marshaling validation report: %w", err)
	}
	// Values 里的非有限值已被 checkValues 在 Save 入口拦下，所以这里不会因 NaN 失败。
	// 两者是绑定的：放宽入口校验会让这一行开始报错，而那会让未过闸的数据两张表都进不去。
	valuesJSON, err := json.Marshal(obs.Values)
	if err != nil {
		return fmt.Errorf("hestia: marshaling values: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO `+TablePending+`
		 (period, period_type, published_at, article_id, extractor, ingested_at, report, values_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		obs.Meta.Period, obs.Meta.PeriodType, obs.Meta.PublishedAt,
		obs.Meta.ArticleID, obs.Meta.Extractor, obs.Meta.IngestedAt,
		string(reportJSON), string(valuesJSON))
	if err != nil {
		return fmt.Errorf("hestia store pending %s/%s: %w",
			obs.Meta.Period, obs.Meta.PeriodType, err)
	}
	return nil
}
