# 架构决策 — Sprint 036（Hestia M1b-4a）

技术设计由上游计划给定。本文件只记 **Arcforge 流程层**必须回答、而计划未覆盖的决策。

---

## AD-036-1：计划的「T1–T4、T6 可并行」**在文件层面不成立**

计划的依赖图写着「T1–T4、T6 相互独立可并行」。那是**逻辑独立**，但：

| 任务 | 触碰 `discover.go` |
|---|---|
| T2 | ✅ 分页解析部分 |
| T3 | ✅ 标题正则 + 条目提取（还要**重构** `pageURL` 抽出 `resolveURL`） |
| T4 | ✅ 加 `PeriodChecker` 接口定义 |
| T5 | ✅ 主循环 |

**四个任务全部写同一个文件。** 并行会互相覆盖，且 T3 还要重构 T2 写的 `pageURL`。

**这与 Sprint 035 的 `store_test.go` 冲突是同一形态**（那次 T1/T3/T4 都要改同一处导出面守卫断言，
Leader 把 TASK-003 串行化到 wave 2）。

### 裁定：按**文件级**而非逻辑级排 wave

| wave | 任务 | writes（零重叠） |
|---|---|---|
| 1 | **T1、T2、T6** | T1{fetch.go, fetch_test.go, **store_test.go**} · T2{discover.go, discover_test.go, testdata/*} · T6{thresholds.go, validate.go, thresholds_test.go, validate_test.go} |
| 2 | T3 | discover.go, discover_test.go |
| 3 | T4 | discover.go, store.go, **store_test.go** |
| 4 | T5 | discover.go, discover_test.go, **store_test.go** |
| 5 | T7 | config.go, config_test.go, thresholds.go, CONTRACTS.md, **store_test.go** |

**并行度 3**（wave 1）。T7 改 `thresholds.go` 而 T6 也改 —— 两者分处 wave 1 与 wave 5，不冲突。

---

## AD-036-2：**四个任务会打红导出面守卫**，每个都要登记

`store_test.go` 有两条导出面守卫，均为**精确集合相等**断言：

- `:361` reflect 版 `TestStoreExposesNoWriteMethods` → 当前 `[Close, DB, Preceding, Save]`
- `:406` AST 版 `TestPackageExposesNoWriteFunctions` → 当前 8 项

本 Sprint 新增的导出物与它们的关系（Leader 已按 Sprint 035 的实测规律推定，**dev 须实测确认**）：

| 新增 | 类别 | 打红哪条 |
|---|---|---|
| `NewPBOCFetcher`（T1） | 包级函数 | **AST 版** |
| `Fetcher`（T1）、`Candidate`（T3）、`PeriodChecker`（T4）、`DiscoverCfg`（T5）、`Config`（T7） | 接口/结构体类型 | **都不打红**（守卫只看 `FuncDecl`） |
| `Store.HasPeriod`（T4） | `*Store` 方法 | **两条都打红** |
| `Discover`（T5） | 包级函数 | **AST 版** |
| `LoadConfig`（T7） | 包级函数 | **AST 版** |

⇒ **T1、T4、T5、T7 的 `writes` 都必须含 `store_test.go`**，并各自**登记**（按字典序追加 + 补一行说明）。

**绝不允许改成 `assert.Contains` 或删断言** —— 该测试注释自己写着
「新增任何导出方法都必须在这里显式登记一次，让『又开了一个写口』成为一个需要动手改测试的决定」。

Sprint 035 实证：`Store.Preceding` 打红两条、`DefaultThresholds`/`Validate` 各只打红 AST 版
—— **两条测试互补不可互替**（reflect 看得见嵌入提升的方法，AST 看得见包级函数）。

---

## AD-036-3：T6 的两类改动**暴露方式不同**，顺序不可颠倒

计划已指出，Leader 实测确认：

| 改动 | 暴露方式 | 实测数量 |
|---|---|---|
| `exemptionFor` 签名加 `periodType` | **编译错误** | 调用点 **4 处**（生产代码仅 `validate.go:93`） |
| `CaliberExemption` 加必填 `PeriodTypes` | **不是编译错误**（Go 允许省略字段） | 构造点 **10 处**（两测试文件各 5），要靠**跑测试**才发现 |

⇒ **先做签名、再加字段**。顺序反了会漏掉构造点，而漏掉的那些会以「豁免不命中」的形式
在别处红，排查方向完全错。

**另一个易漏点**：`thresholds_test.go` 的 `mutate` 对 `SkipChecks` 做了 `slices.Clone` 浅拷贝防护，
**`PeriodTypes` 也要一并 Clone**，否则某个用例改动它会污染其余用例。

---

## AD-036-4：快照有时效性，且**必须由 T2 的 dev 当场抓**

Leader 在需求分析阶段**只验证了可达性**（p1/p2 均 HTTP 200、报告在 p2、干扰项同页），
**没有代抓** —— 快照是 T2 的交付物，代抓会让「抓取日期」与交付脱节。

T2 的 DoD 要求 `testdata/README.md` **记录抓取日期**，并写明
「测试断言的是『翻页直到找到』，不是『报告在第 2 页』」 —— 后者会随时间失效。

---

## AD-036-5：沿用 Sprint 035 的 worktree 方案 C（**待人类确认**）

Sprint 035 经人类批准采用：**wave1 用 worktree（Leader 先 merge、dev 后 transition），
wave2+ 主工作区直连（先 transition、后 commit）**。

理由与上次相同且已被验证：
- `dev_done` 门禁在**主工作区**跑 `go test`，代码只在 worktree 时它测的是**不含新代码的旧树**
  —— 门禁会**成功**（这是本项目复发过两次的坑）
- wave1 三个任务并行写同一个包目录，半成品文件会让彼此整包测试编译失败

**Sprint 035 全程照此执行，无一次假绿。** 本 Sprint 沿用，在 DoD 确认门一并请人类确认。

---

## AD-036-6：四条派单硬要求全部保留（Sprint 035 制度化，每条都有实测依据）

1. **消融 harness 必带编译失败闸 + 计数自证**（`变异条数 == 结论行数`）
   —— 「KILLED 但因果是错的」出现 6 次；**变异「根本没跑」时只是少一行输出**，四道闸拦不住
2. **断言「包住了」用 `require.NotNil(t, errors.Unwrap(err))`**，不得用 `NotErrorIs(Unwrap(err), err)`（平凡为真）
3. **写断言时就问「有没有一个我想排除的实现，能让这条断言照样绿？」** —— 比事后消融早一轮
4. **别把理由写强**：「因为 X 所以 Y」先问「X 我实测过吗」；
   「全部都要 Y」先问「**我的实测覆盖了这个『全部』里的几个？**」

**最强形态**：为自己写下的理由**预先声明证伪条件**再去跑。
界线是**变异对象必须是生产代码，不能是判据**。

`.arcforge/archive/sprint-035-2026-08-11/docs/02-plan/findings-carryover.md` 的 F1–F43
是这些要求的完整实测依据，**dev 与验证者的派单都会指向它**。

---

## AD-036-7：不重写上游设计，但也不盲从

同 Sprint 035 的 AD-035-5。计划**有错**（编译不过、断言与实现矛盾、行号漂移、与 spec 冲突）
走 `blocked_clarification`；计划**沉默**而 DoD 有要求 → 直接按 DoD 做。

**已发现一处行号漂移**：计划写 `checkEnum`（types.go:110），实际在 **:116**。
这类漂移不影响语义，但说明**计划的行号引用不可尽信**，以符号名为准。
