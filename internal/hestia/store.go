package hestia

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
