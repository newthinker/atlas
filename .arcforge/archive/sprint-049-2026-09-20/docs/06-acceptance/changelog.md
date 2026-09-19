# Changelog — sprint M4-crisis（crisis 库双时态迁移）

- **日期**：2026-09-19
- **需求文档**：`hestia/docs/superpowers/plans/2026-09-18-crisis-bitemporal.md`
- **采样锚**：本文件全部实测数字取自 `89dc94e40f64fc89240ca5694a73e1a24fbaf567`（分支 `master`）。
  复核方式：`git diff --numstat 89dc94e4 <当前> -- internal/crisis cmd/atlas`，非空则本文件的数字需重采。

---

## 破坏性变更

### `crisis.db` 的 `macro_observations` 主键由两段改三段

| | 迁移前 | 迁移后 |
| --- | --- | --- |
| 主键 | `(ts, indicator)` | `(ts, indicator, fetched_at)` |
| 同一 `(ts, indicator)` 的多次抓取 | 覆盖（`INSERT OR REPLACE`） | **全部保留**，按 `fetched_at` 分版本 |
| 读取路径 | 裸表 | 视图 `v_macro_current`（每键取 `fetched_at` 最大的一版） |

**运维必读**：升级二进制后**必须**先跑一次迁移，否则 `atlas crisis *` 全部拒绝启动：

```
atlas crisis migrate-bitemporal --config configs/config.yaml --crisis-config configs/crisis-monitor.yaml
```

启动守卫（TASK-003）刻意做成**拒绝而不是自动迁移** —— 自动迁移会在运维没有备份的时候改写生产库。
守卫对**全新空库**放行（表不存在 ⇒ 放行），否则谁都建不了新库；这也意味着**跑错目录会建出一个空库并「通过」**，
判据是首行为 `system state: …` 而不是 `rc=0`（详见 `docs/deployment.md`）。

### 回滚

回滚 SQL 在 `internal/hestia/CONTRACTS.md` §B。🔴 **必须按 §B 写明的单参数形态执行**：

```bash
sqlite3 crisis.db "<整段 SQL>"      # ✅ 出错即 bail 并回滚
sqlite3 crisis.db < rollback.sql    # ❌ 出错继续执行，COMMIT 落盘 ⇒ 永久丢表
```

两种形态的退出码都是 `rc=1`，**区分不了**。本 sprint 在生产库副本上实测过全损路径（31098 行）。

---

## 交付清单

| 任务 | 交付 | 合并点 |
| --- | --- | --- |
| TASK-001 | 三段主键 schema + `v_macro_current` 视图 DDL | `96a85c3` |
| TASK-002 | `MigrateBitemporal` + `atlas crisis migrate-bitemporal` | `e93c950` `b43832e` |
| TASK-003 | `NewStore` 漂移守卫：老形状拒绝启动并指路 | `62d3d2e` |
| TASK-004 | 五个读取点全部改走视图（含 `EvalDates`） | `5664ef7` `0e261cf` |
| TASK-005 | 写路径改追加：裸 `INSERT` + 冲突按 `value` 三分 | `850d632` |
| TASK-006 | `Store.AsOf` 只读副本，`obsFrom()` 按 `asOf` 选形态 | `a3e7d32` `50024a1` |
| TASK-007 | `crisis replay --as-of` | `3187337` `739bfde` `47d7084` `2e340ad` |
| TASK-008 | 生产迁移执行 + 判据一～四实测（人类在场） | `1a754b2` `0c37144` |
| TASK-009 | 收掉三条无 owner 的运维认知缺口（V-4 / V-6 / V-8） | `abada6f` |
| TASK-010 | 列级校验参照系修正：守卫与被守护对象不共用来源 | `80338a2` `ecae20c` |

## 新增命令 / flag

- `atlas crisis migrate-bitemporal` —— 幂等；已是三段主键时报「already migrated」并 `rc=0`。
- `atlas crisis replay --as-of <RFC3339>` —— 按**修订时点**重放。
  **只接受 UTC（`Z` 结尾）形态**：`fetched_at` 是 `TEXT`，SQLite 按字典序比较，
  带偏移量的 `+08:00` 形态与库里的 `Z` 形态字典序不可比，会静默错位。

## 覆盖率（锚 `89dc94e4`）

| package | statements |
| --- | --- |
| `./internal/crisis` | **93.3%** |
| `./cmd/atlas` | **78.4%** |

## 未进本 sprint 的已知缺口

见 `final-report.md` 的「遗留缺口」一节 —— 其中 **V-2（`crisis` 缺 `verifyCurrentView`，视图定义漂移无守卫）**
是本 sprint QA 两个 reviewer 独立收敛的 HIGH，已按人类裁决转下个 sprint 立项。
