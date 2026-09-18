package crisis

import (
	"fmt"

	"github.com/newthinker/atlas/internal/macro/bitemporal"
)

// obsSpec 是 macro_observations 的双时态 Spec（M4，2026-09-18）。
//
// 包级唯一实例：视图 DDL 与 as-of 查询都从它生成。**不要在别处再 NewSpec 一次**
// ——两份 Spec 意味着表名/键列/revision 列有两个副本，改一处不会让另一处变红。
//
// 键形状不是本次才定的：bitemporal 的包注释里早就写着
// `macro_observations: (ts, indicator) + fetched_at`，Spec 本就是为它预留的。
var obsSpec = mustSpec()

// mustSpec 装配 obsSpec，只在 NewSpec 出错时 panic。
//
// 不把错误吞成零值 Spec：零值 Spec 会生成表名为空的 SQL，报的是一个离成因很远的
// 语法错误。而这里的三个标识符都是本文件里的字面量，过不了 identRE 只可能是改错
// 了字 —— 那是编程错误，应当在包初始化时就带着原始错误炸开。
func mustSpec() bitemporal.Spec {
	s, err := bitemporal.NewSpec("macro_observations", []string{"ts", "indicator"}, "fetched_at")
	if err != nil {
		panic(fmt.Sprintf("crisis: obsSpec: %v", err))
	}
	return s
}

// tablesDDL 建表与建索引。crisis_evaluations 与两个索引沿用原 store.go 的定义。
//
// macro_observations 的主键扩为三段 (ts, indicator, fetched_at)：同一观测日的
// 同一指标可以有多个修订，当前行从 fetched_at 派生。
//
// fetched_at 加 NOT NULL —— SQLite 的主键**允许 NULL**，空值会同时绕开唯一性
// 约束与 MAX(fetched_at) 关联。
//
// ⚠️ `CREATE TABLE IF NOT EXISTS` 对已存在的表什么都不做 ⇒ 以上只对**新建**的库
// 生效；老库靠 TASK-002 的迁移改形、TASK-003 的守卫拦住未迁移就启动。
const tablesDDL = `
CREATE TABLE IF NOT EXISTS macro_observations (
	ts          TEXT NOT NULL,
	indicator   TEXT NOT NULL,
	value       REAL,
	source      TEXT,
	fetched_at  TEXT NOT NULL,
	PRIMARY KEY (ts, indicator, fetched_at)
);
CREATE TABLE IF NOT EXISTS crisis_evaluations (
	ts            TEXT NOT NULL,
	eval_at       TEXT NOT NULL,
	indicator     TEXT NOT NULL DEFAULT '',
	status        TEXT NOT NULL DEFAULT '',
	tag           TEXT NOT NULL DEFAULT '',
	value         REAL NOT NULL DEFAULT 0,
	pct_5y        REAL NOT NULL DEFAULT 0,
	system_state  TEXT NOT NULL DEFAULT '',
	detail        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_macro_obs_ind_ts   ON macro_observations(indicator, ts);
CREATE INDEX IF NOT EXISTS idx_crisis_eval_ind_ts ON crisis_evaluations(indicator, ts);`

// schemaDDL 返回建表、建索引、建视图的全部 DDL。
//
// 不返回 error —— 唯一的错误源是 NewSpec，而它已在 mustSpec 里 panic 掉了。
//
// 视图由基座生成 ⇒ SQL 里每个标识符都来自校验过的 Spec，没有未校验的拼接输入
// （同 hestia 的 v_hestia_current 手法）。
func schemaDDL() string {
	return tablesDDL + "\nCREATE VIEW IF NOT EXISTS v_macro_current AS " +
		bitemporal.CurrentQuery(obsSpec) + ";"
}
