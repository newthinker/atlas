package hestia

import (
	"context"
	"database/sql"
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
	obs.Meta.IngestedAt = s.now().UTC().Format(time.RFC3339)

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

	if err := s.insert(ctx, obs); err != nil {
		return Outcome{}, err
	}
	return Outcome{Verdict: verdict, Table: TableObservations}, nil
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

// savePending 在 TASK-006 实现真正的逻辑。
//
// 这是一个**有意的桩**：TASK-005 的全部测试都走 passing() 报告，不触达它。
// 桩返回错误而不是静默成功，是为了让任何提前走到这条路径的调用大声失败。
func (s *Store) savePending(ctx context.Context, obs Observation, rep ValidationReport) error {
	return fmt.Errorf("hestia: savePending not implemented yet")
}
