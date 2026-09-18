package crisis

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
)

// MigrateResult 是一次迁移的取证。
//
// 三个计数是运维现场唯一的判据：三者相等才算好。RowsAfter 少于 RowsBefore
// 意味着搬丢了行；ViewRows 少于 RowsAfter 意味着库里本就有重复修订（迁移前
// 的两段主键不允许这种事，所以真出现就说明有别的问题）。
type MigrateResult struct {
	AlreadyMigrated bool
	RowsBefore      int // 旧表行数
	RowsAfter       int // 新表行数
	ViewRows        int // v_macro_current 行数
}

// legacyTableName 是迁移后旧表的名字。保留而非丢弃：回滚靠两次 RENAME，
// 迁移正确性的逐值对照也靠它（设计 C3）。
const legacyTableName = "macro_observations_v1"

// MigrateBitemporal 把 macro_observations 迁到三段主键，幂等。
//
// 🔴 刻意不经 NewStore（AD-5）：TASK-003 的守卫会拒绝老形状的库，而迁移恰恰
// 要在那种库上跑。经 NewStore 就成了鸡生蛋。
//
// 迁移顺序是 rename-first 而不是「建 _new 再改名」：新表由 TASK-001 的
// schemaDDL() 原样生成，表形状因此只有一个副本（在别处重写一份建表 SQL 会让
// C1 在迁移路径上悄悄漂移，同 AD-3 的风险源）。代价是必须先腾出
// macro_observations 这个名字，于是有了 DROP VIEW / DROP INDEX 那两步——
// 它们各自还堵着一个静默失败，理由写在 migrate 的步骤表里。
//
// 全程一个事务：sqlite 的 DDL 是事务性的（已实测 ALTER TABLE RENAME 能被
// 回滚），所以中途失败不会留下半迁移的库。
func MigrateBitemporal(ctx context.Context, dbPath string) (MigrateResult, error) {
	if dbPath == "" {
		return MigrateResult{}, fmt.Errorf("crisis: migrate needs a db path")
	}
	// sql.Open("sqlite", "file:"+path) 默认会**建库**，所以「文件不存在」必须
	// 在这里显式拦下。不然迁移一个空库还会报成功——最糟的结果。
	if _, err := os.Stat(dbPath); err != nil {
		return MigrateResult{}, fmt.Errorf("crisis: migrate: db %s: %w", dbPath, err)
	}

	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return MigrateResult{}, fmt.Errorf("crisis: migrate: opening %s: %w", dbPath, err)
	}
	defer db.Close()

	migrated, err := isBitemporal(ctx, db)
	if err != nil {
		return MigrateResult{}, err
	}
	if migrated {
		return MigrateResult{AlreadyMigrated: true}, nil
	}
	return migrate(ctx, db)
}

// isBitemporal 报告 macro_observations 是否已是三段主键。
//
// 判据是主键里有没有 fetched_at，**不是有没有 macro_observations_v1 表**：
// 回滚之后库里可能既是老形状、又留着一张 _v1，按后者判会误判成「已迁移」
// 而拒绝干活——那正是最需要迁移的时候。
func isBitemporal(ctx context.Context, db *sql.DB) (bool, error) {
	var ddl string
	err := db.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='macro_observations'`).Scan(&ddl)
	if err == sql.ErrNoRows {
		return false, fmt.Errorf("crisis: migrate: no macro_observations table")
	}
	if err != nil {
		return false, fmt.Errorf("crisis: migrate: reading schema: %w", err)
	}
	flat := strings.Join(strings.Fields(ddl), " ")
	return strings.Contains(flat, "PRIMARY KEY (ts, indicator, fetched_at)"), nil
}

func migrate(ctx context.Context, db *sql.DB) (MigrateResult, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("crisis: migrate: beginning tx: %w", err)
	}
	defer tx.Rollback()

	// _v1 占名时停手，不删它。它可能是上一次迁移留下的唯一历史副本——回滚靠
	// 它、逐值对照也靠它（C3）。这种「老形状与 _v1 并存」是半个回滚留下的异常
	// 状态，自作主张删表的代价远大于让人来看一眼。
	var occupied int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE name = ?`, legacyTableName).Scan(&occupied); err != nil {
		return MigrateResult{}, fmt.Errorf("crisis: migrate: checking %s: %w", legacyTableName, err)
	}
	if occupied > 0 {
		return MigrateResult{}, fmt.Errorf(
			"crisis: migrate: %s already exists but macro_observations is still legacy-shaped; "+
				"resolve it by hand (it may be the only copy of the pre-migration data)", legacyTableName)
	}

	var res MigrateResult
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM macro_observations`).Scan(&res.RowsBefore); err != nil {
		return MigrateResult{}, fmt.Errorf("crisis: migrate: counting rows: %w", err)
	}

	// 步骤表而非一串 tx.Exec：每步的失败文案要能指出是哪一步炸的，运维现场
	// 「migrate: renaming legacy table: ...」比一个裸 sqlite 错误有用得多。
	for _, step := range []struct{ what, stmt string }{
		// 视图必须先删：ALTER TABLE RENAME 会**静默改写视图定义**（已实测，
		// v_macro_current 的 SQL 被自动改成引用 macro_observations_v1），迁移
		// 后视图就指向旧表了。而它不报错——无重复修订时 ViewRows 仍等于
		// RowsAfter，计数全对，只是视图从此再也看不到新写入的数据。
		{"dropping stale view", `DROP VIEW IF EXISTS v_macro_current`},
		// 索引也必须先删：索引名在 sqlite 里是全局唯一的。RENAME 后
		// idx_macro_obs_ind_ts 跟着旧表走，此时 schemaDDL() 里的
		// CREATE INDEX IF NOT EXISTS 会因同名已存在而静默跳过，新表永远没有
		// 索引——不报错，只变慢（设计 C2）。
		{"dropping stale index", `DROP INDEX IF EXISTS idx_macro_obs_ind_ts`},
		{"renaming legacy table", `ALTER TABLE macro_observations RENAME TO ` + legacyTableName},
		{"creating new schema", schemaDDL()},
		{"copying rows", `INSERT INTO macro_observations (ts, indicator, value, source, fetched_at)
		                  SELECT ts, indicator, value, source, fetched_at FROM ` + legacyTableName},
	} {
		if _, err := tx.ExecContext(ctx, step.stmt); err != nil {
			return MigrateResult{}, fmt.Errorf("crisis: migrate: %s: %w", step.what, err)
		}
	}

	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM macro_observations`).Scan(&res.RowsAfter); err != nil {
		return MigrateResult{}, fmt.Errorf("crisis: migrate: counting migrated rows: %w", err)
	}
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM v_macro_current`).Scan(&res.ViewRows); err != nil {
		return MigrateResult{}, fmt.Errorf("crisis: migrate: counting view rows: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return MigrateResult{}, fmt.Errorf("crisis: migrate: committing: %w", err)
	}
	return res, nil
}
