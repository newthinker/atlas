package main

// Context Checkpoint: done_criteria → test mapping (TASK-002 migrate-bitemporal 子命令)
// functional[0]     子命令打印 RowsBefore / RowsAfter / ViewRows 三个计数     → TestExecuteCrisisMigratePrintsCounts
// functional[1]     已迁移的库走 no-op 分支并说明                             → TestExecuteCrisisMigrateAlreadyMigrated
// error_handling[0] 缺 --db 报错且不开库；--db 指向不存在的文件报错含路径且不建库 → TestExecuteCrisisMigrateRequiresExistingDB
// non_functional[0] 本文件为 cmd/atlas 新增代码自带测试，不拉低该包覆盖率

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// legacyMigrateDB 建一个老形状（两段主键）的 crisis 库并塞 n 行，返回路径。
//
// 刻意不经 crisis.NewStore：NewStore 建出来的已经是新形状，迁移就没得测了。
func legacyMigrateDB(t *testing.T, rows int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", "file:"+path)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`
CREATE TABLE macro_observations (
	ts          TEXT NOT NULL,
	indicator   TEXT NOT NULL,
	value       REAL,
	source      TEXT,
	fetched_at  TEXT,
	PRIMARY KEY (ts, indicator)
);
CREATE INDEX idx_macro_obs_ind_ts ON macro_observations(indicator, ts);`)
	require.NoError(t, err)
	for i := 0; i < rows; i++ {
		_, err := db.Exec(`INSERT INTO macro_observations VALUES (?,?,?,?,?)`,
			fmt.Sprintf("2026-01-%02d", i+1), "vix", float64(10+i), "test", "2026-07-14T00:00:00Z")
		require.NoError(t, err)
	}
	return path
}

// snapshotMigrateFlag 保存并恢复本子命令的包级 flag 变量。
//
// 与 crisis_test.go 的 snapshotCrisisFlags 同一手法，但只管本任务新增的那个
// ——那个函数不在本任务的改动范围内，不去动它。
func snapshotMigrateFlag(t *testing.T) {
	t.Helper()
	prev := migrateDBPath
	t.Cleanup(func() { migrateDBPath = prev })
}

func TestExecuteCrisisMigratePrintsCounts(t *testing.T) {
	path := legacyMigrateDB(t, 3)
	var out strings.Builder

	require.NoError(t, executeCrisisMigrate(context.Background(), &out, path))

	s := out.String()
	// 三个计数都要露面：运维要靠它们判断「三者相等才算好」。
	assert.Contains(t, s, "RowsBefore")
	assert.Contains(t, s, "RowsAfter")
	assert.Contains(t, s, "ViewRows")
	assert.Contains(t, s, "3", "3 行库迁移后三个计数都应是 3")
	assert.Contains(t, s, path, "打印被迁移的库路径，免得运维迁错了库还不知道")
}

func TestExecuteCrisisMigrateAlreadyMigrated(t *testing.T) {
	path := legacyMigrateDB(t, 1)
	var first strings.Builder
	require.NoError(t, executeCrisisMigrate(context.Background(), &first, path))

	var out strings.Builder
	require.NoError(t, executeCrisisMigrate(context.Background(), &out, path))
	assert.Contains(t, strings.ToLower(out.String()), "already",
		"第二次必须明说是 no-op，不能打印得像又迁了一遍")
}

func TestExecuteCrisisMigrateRequiresExistingDB(t *testing.T) {
	snapshotMigrateFlag(t)
	var out strings.Builder

	// 缺 --db：报错且不开任何库
	err := executeCrisisMigrate(context.Background(), &out, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--db")

	// --db 指向不存在的文件：报错含路径，且不得顺手建一个空库
	missing := filepath.Join(t.TempDir(), "nope.db")
	err = executeCrisisMigrate(context.Background(), &out, missing)
	require.Error(t, err)
	assert.Contains(t, err.Error(), missing)
	_, statErr := os.Stat(missing)
	assert.True(t, os.IsNotExist(statErr), "不得顺手建出一个空库")
}

// TestCrisisMigrateCmdWiring 钉住子命令挂在 crisis 下且 flag 已注册 ——
// RunE 只是薄包装，真正的逻辑由上面几个测试覆盖。
func TestCrisisMigrateCmdWiring(t *testing.T) {
	snapshotMigrateFlag(t)
	assert.Equal(t, "migrate-bitemporal", crisisMigrateCmd.Use)
	assert.NotNil(t, crisisMigrateCmd.Flags().Lookup("db"), "--db 必须注册")

	var found bool
	for _, c := range crisisCmd.Commands() {
		if c == crisisMigrateCmd {
			found = true
		}
	}
	assert.True(t, found, "migrate-bitemporal 必须挂在 crisis 子命令下")

	migrateDBPath = ""
	assert.Error(t, runCrisisMigrate(crisisMigrateCmd, nil), "缺 --db 时 RunE 也要报错")
}
