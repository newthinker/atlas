# 架构决策（M4-crisis）

- **AD-1 键形状 `(ts, indicator, fetched_at)`，`fetched_at NOT NULL`**（spec C1）：SQLite 主键允许 NULL，
  空值会同时绕开唯一性约束与 `MAX(fetched_at)` 关联。生产库实测零空值 ⇒ 现在加得上，以后有了空值就加不上。
- **AD-2 当前行走视图 `v_macro_current`，由 `bitemporal.CurrentQuery(obsSpec)` 生成**：当前行从 revision 列派生，
  不引 `is_current` 列——乱序写入自动正确，且视图 SQL 的每个标识符都来自校验过的 Spec。
- **AD-3 `obsSpec` 包级唯一实例**：两份 Spec = 表名/键列/revision 列各有两个副本，改一处不会让另一处变红。
  测试钉住 `bitemporal.NewSpec(` 在包内只出现一次。
- **AD-4 迁移是显式命令，不在 `NewStore` 里自动跑**（C9）：启动时静默改表结构，会把一个本该被人盯着的动作
  变成副作用，出问题时连「什么时候变的」都查不到。照 hestia 先例，漂移即拒绝启动并指路。
- **AD-5 迁移命令不经 `NewStore`**：守卫会拒绝老形状的库，而迁移恰恰要在那种库上跑——经 NewStore 就是鸡生蛋。
- **AD-6 读全部改走视图，而不是给每个读点各加 WHERE**（C5 + 裁决 R1）：后者是同一规则的 N 份副本，
  加第 N+1 个读点时必漏。**范围含 `EvalDates`**（它内联了裸表查询，计划遗漏，Leader 核出）。
- **AD-7 写用裸 `INSERT`，冲突按 value 分流**（C6）：三段全同且值同 ⇒ 采集器重跑，吞掉；
  三段全同而值不同 ⇒ 「同一次取回给出两个值」，取回时刻语义坏了 ⇒ 整批回滚并报错。
  **不用 `OR IGNORE`**——它把这两种揉成一种，后者会被静默吞掉。⚠️ 比对 value 要处理 NULL（`NULL != NULL`）。
- **AD-8 as-of 走 `Store.AsOf(t)` 只读副本，共用同一个 `*sql.DB`**（C7）：各自开库会让连接数随调用增长、
  Close 语义含糊。副本不得写入（写路径不看 asOf）。
- **AD-9 `AsOfQuery` 不建视图**（C4）：它带 `?` 占位符。`obsFrom()` 返回「SQL 片段 + 前置参数」两个形态。
  ⚠️ 参数顺序：`?` 在子查询里，**位置先于**外层 WHERE 的参数；弄反不报错，只会把日期当指标名去查、返回空。
- **AD-10 `--as-of` 的时间轴是「我们抓到的时刻」，不是「数据源发布的时刻」**（spec §7）：
  帮助文案写 `visible at`，不写 `published at`——一字之差会让人以为它能回答 vintage 问题。
- **AD-11 部署顺序与任务顺序相反**：先用新二进制跑迁移，再部署重启。反过来会让 crisis 的 launchd 任务
  全部启动失败（守卫按设计拒绝老形状）。
- **AD-12（人类 R2）** agent 在人类在场时执行迁移与判据一～四；判据五～七留给人类。
