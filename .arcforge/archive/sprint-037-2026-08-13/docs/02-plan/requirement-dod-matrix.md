# Sprint 037 需求 ↔ DoD 双向追溯矩阵

**需求源**：`hestia/docs/superpowers/plans/2026-08-12-hestia-cli.md`（1882 行）
**+ 人类 2026-08-12 定案**（Sprint 036 归档的「🔴 人类定案」节）

---

## 一、计划自审的 Spec 覆盖表 → 本 Sprint 任务

| Spec 要求 | 计划落点 | **本 Sprint 任务** |
|---|---|---|
| 第 2 节 一级键定案 | T2 + T4 | **TASK-003** + **TASK-007** |
| 第 3 节 编排放 internal | T3 | **TASK-005** |
| 4.1 接缝① ArticleID | T3 Step 1/3 | **TASK-005** `functional[1]` |
| 4.2 接缝② 交叉校验 | T3 Step 6 | **TASK-005** `functional[2]` |
| 4.3 单期失败不中断 | T3 Step 3 + T4 Step 6 | **TASK-005** `boundary[0]` + **TASK-007** `error_handling[0]` |
| 第 5 节 三个只读方法 | T2 + T5 | **TASK-003**（HasArticle）+ **TASK-006**（两个 Recent） |
| 6.1 storage 段 | T1 | **TASK-002** |
| 6.2 配置实例 | T7 Step 1 | **TASK-009** `functional[0]` |
| 第 7 节 plist 不设代理 | T7 Step 2/3 | **TASK-009** `functional[1]` |
| 第 8 节 status 输出 | T5 | **TASK-006** |
| 9.1 合成 index | T3 Step 1、T4 Step 1 | **TASK-005** / **TASK-007** |
| 9.3 端到端验收 | T7 Step 9 | **Leader 归档前执行**（不在任何 DoD 内，见下「刻意的非覆盖」） |

## 二、人类定案 → 本 Sprint 任务（计划**未**覆盖的部分）

| 人类定案（2026-08-12） | 计划有没有 | 本 Sprint 落点 |
|---|---|---|
| ① **季报支持，4b 之前补上** | ❌ **零命中**（八个关键词 / 三个文件均 0 次） | **TASK-001**（discover 侧四处）+ **TASK-004**（extract 侧硬卡点）+ **TASK-009**（登记 CONTRACTS） |
| ② 月报「跳过记日志，不中止本轮」 | ✅ Global Constraint 26 行 | **TASK-005** `boundary[0]` + **TASK-007** `error_handling[0]` |
| ②配套「重试记账」 | — | **不需要**：计划核实情形 A 抓取/解析失败**根本没调 `Save`**、不写 pending 行 ⇒ Sprint 036 W3 的「1460 行/年」量化前提不成立 |
| ③ **修订版暂不支持，写进契约** | ❌ 计划把情形 B 标成 **✅**（Leader 核实不成立） | **TASK-009** `non_functional[0]` 第 ③ 条（强制原样写明）+ **TASK-007** `functional[1]` 明令不写端到端修订测试 |

## 三、Sprint 036 结转的四件「4b 自己能闭合的」

| 结转项 | 本 Sprint 落点 |
|---|---|
| 倒序处理候选（否则漂移检测一次都不真正执行且零告警） | **TASK-005** `non_functional[0]`（含钉住的测试） |
| pending 重试记账 | **不需要**（见上 ②配套） |
| 候选期次 vs 正文期次交叉核对 | **TASK-005** `functional[2]`（= 计划的接缝②） |
| 包一层限速 Fetcher | ⚠️ **本 Sprint 未覆盖**，见下 |

---

## 四、机器检查结果

### 孤儿需求（有需求、无 DoD 覆盖）

| # | 需求 | 状态 |
|---|---|---|
| 1 | **限速 Fetcher**（Sprint 036 S5：`Discover` 对 index 页无限速连发，首跑可达 408 次） | 🟡 **本 Sprint 未覆盖，Leader 主动申报**。理由：本 Sprint 的 `MaxPages` 按计划配置为 3（计划 1820 行「翻满 3 页」），408 次那个场景是空库全量回填，属 **M1c** 的问题。⇒ **结转 M1c**，不在本 Sprint 开任务。 |
| 2 | 端到端手工验收（计划 Step 9） | 🟢 **刻意不入 DoD**：它需要真实网络与 launchd，无法由 dev 在测试里覆盖。**由 Leader 在归档前亲自执行并记录结果**。 |
| 3 | `timeout: 30` → 30ns 且过校验（Sprint 036 S1） | 🟡 **未覆盖**。TASK-002 只加 `db_path`，未动 `Timeout` 的下界。⇒ 结转，**在 TASK-002 的 discovery 里登记**。 |

### 凭空 DoD（有 DoD、不对应任何需求）

**零条。** 逐条核对：9 个任务共 42 条 DoD，全部可追溯到上表的 Spec 要求、人类定案、或 Sprint 036 结转项。

其中 **11 条是 Leader 加的「机制性防护」**（不来自计划正文，但来自 Sprint 036 实测教训）：

| 防护 | 出现在 | 依据 |
|---|---|---|
| 遍历型断言须有肯定式前置锚点 | TASK-001 | G9（`scanPage` 恒 nil 时平凡为真） |
| 消融判据「看哪条断言红，不是红不红」 | TASK-001/004/007 | G31（四格全红 = 判据信息量为零） |
| `-run` 必须锚定 `^Top$/^Sub$` | TASK-001 | G30（前缀匹配跑起 7 条 RUN） |
| `%w` 自证用 `NotNil(Unwrap)` 而非 `ErrorIs` | TASK-003/005/006 | F8（`ErrorIs` 证不了「包住」，跨 Sprint 存活一轮） |
| 导出面守卫「登记不是放宽」+ 正向自证 | TASK-003/005/006 | Sprint 035–036 八个 commit 的一致做法 |
| 否定式断言须配肯定式锚点 | TASK-009（plist 守卫） | G9 的另一面 |
| 两个 map 互查（改一个忘另一个不会编译错） | TASK-001 | 本 Sprint 新增 |
| 真实样本不可用合成代替 | TASK-001/004 | 季报缺口正是「快照只含 h1」造成的 |
| 前缀词必须读样本不能推测 | TASK-004 | `cumulativePeriods` 注释的「两个维度都要判」 |
| `cmd/atlas` 覆盖率门禁的 AD-6 处置 | TASK-008 | sprint-023 实测 75.9% < 80 |
| 「若某条守卫红了说明上游有疏漏，别改测试迁就实现」 | TASK-007 | 计划 716 行原话 |

---

## 五、Realistic Scope 检查

| 任务 | package 数 | DoD 条数 | writes 文件数 | 判定 |
|---|---|---|---|---|
| TASK-001 | 1 | 6 | 4 | ✅ |
| TASK-002 | 1 | 6 | 2 | ✅ |
| TASK-003 | 1 | 6 | 2 | ✅ |
| TASK-004 | 1 | 6 | 3 | ✅ |
| TASK-005 | 1 | 6 | 2 | ✅ |
| TASK-006 | 1 | 6 | 4 | ✅ |
| TASK-007 | 1 | 6 | 1 | ✅ |
| TASK-008 | 1 | 6 | 2 | ✅ |
| TASK-009 | **2** | 6 | 4 | ⚠️ 见下 |

**TASK-009 声明两个 package**（`./cmd/atlas` + `./internal/hestia`）：它同时改 `cmd/atlas/hestia_test.go`（plist 守卫）与 `internal/hestia/CONTRACTS.md`（契约）。
**这是覆盖率口径的需要**（`packages` 是宽口径），而 `writes` 里 `configs/` 与 `deploy/` 两项**不是 Go 包**——
按记忆里的判据，`packages` 只列 Go 包、非 Go 产物只进 `writes`，故 `packages` 不含它们。

⚠️ validator 可能对此报 `scope-writes-outside-packages` 告警 —— **那是形状级假阳**（记忆已记：照它改会把非 Go 路径塞进 `-coverpkg` 弄坏门禁）。**原样放着。**

---

## 六、scope 互斥（validator 已通过，此处记录人工推理）

| 文件 | 被哪些任务写 | 是否并行 |
|---|---|---|
| `store.go` / `store_test.go` | TASK-003、TASK-006 | ❌ **串行**：TASK-006 的 `dependencies` 加了 TASK-003（**scope 序列化，非逻辑依赖**，已在 description 注明） |
| `ingest_test.go` | TASK-005、TASK-007 | ❌ 串行（TASK-007 依赖 005） |
| `cmd/atlas/hestia_test.go` | TASK-008、TASK-009 | ❌ 串行（TASK-009 依赖 008） |
| `discover.go`/`types.go` (季报) vs 4b 全部文件 | TASK-001 vs 其余 | ✅ **零重叠，可并行** |
| `profiles.go` (季报) vs 4b 全部文件 | TASK-004 vs 其余 | ✅ 零重叠 |
| `testdata/` | TASK-001、TASK-004 | ⚠️ 同目录不同文件；TASK-004 依赖 TASK-001 ⇒ 串行，无冲突 |
