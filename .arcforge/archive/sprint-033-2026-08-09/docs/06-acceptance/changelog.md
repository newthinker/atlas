# Sprint 033 Changelog — M1b-1 `internal/hestia` types + store

**分支** `feat/hestia-store`（基于 `origin/master` `d7c9c69`）
**交付** 9 commit / 11 files / +3561 行 / 新增包 `internal/hestia`
**测试** 61 顶层 PASS / 75 含子测试 / 0 FAIL / 0 SKIP / 0 DATA RACE / 覆盖率 88.7%
**回归** 全仓 64 包 0 FAIL；`go.mod` / `go.sum` 未动；既有包一个文件未碰

## 交付内容

| commit | 内容 |
|---|---|
| `5dcc802` | 54 个 `Field*` 常量、`fieldOrder`（唯一真相源）、`allFields`（白名单） |
| `87acf57` | 纳入 M0 抓取的央行报告样本 testdata（解除门禁阻塞，见下） |
| `a087b98` | `Meta`（含形态校验）、`Observation`、`Check`、`ValidationReport`、`Outcome` |
| `878660e` | 返工：常量↔字面量绑定表，钉住「哪个常量名对应哪个值」 |
| `fb15260` | 三段 DDL 由 `fieldOrder` 生成；结构断言从真实库读 `PRAGMA table_info` |
| `5e4d217` | `NewStore`（幂等建表 + schema 漂移主动检查）、`Close`、`DB` |
| `39aa8af` | `Save` 唯一写入口、四道前置校验、写口守卫扩到包导出面 |
| `c1050d7` | `refreshArticleID`、真正的 `savePending`（替换桩） |
| `ed776a0` | 收尾验证 + G9 并发契约写入包注释 |

## 架构决策（均有测试钉住）

- **字段清单只写一次**：`fieldOrder` 是唯一真相源，DDL / INSERT 列 / 白名单全部派生。`fields.go` 之外的非测试文件不得出现业务字段名字面量，由 `go/ast` 静态检查守住（能抓嵌套括号拼接，抓不到变量拼接 / `Sprintf` / `[]byte` 往返——上限已显式声明）。
- **`Save` 是唯一写入口**，签名强制要求 `ValidationReport`。两条互补守卫：`reflect` 看方法集（含嵌入类型提升上来的）、`go/ast` 看包级函数，**删任一条都重开一个缺口**（T5 双向实证）。
- **当前行由 `published_at` 派生**，无 `is_current` 列，写入是普通 INSERT。
- **`Meta` 七字段顺序是三处同序契约**（结构体声明 / `metaColumns` / `insert` 取值），三端各有 reflect 断言。失效后果是静默写错位数据——所有列都是 TEXT，不触发任何数据库错误。
- **单位不入库**（余额=万亿元、增量=亿元、比率=百分数），改单位属 breaking change，不设 `units_version` 列。

## 相对需求文档的修正

| 项 | 需求文档 | 实际 |
|---|---|---|
| 分支基点 | 从 `feat/macro-bitemporal` 拉 | **从 `origin/master` 拉**（M1a 已于本 Sprint 前合并，PR#53） |
| T7 的 diff 基准 | `feat/macro-bitemporal` | **`origin/master`**（本地 `master` 落后三个 merge，用它会多出 M1a 的 10 个文件） |
| `PRIMARY KEY` 计入列数 | 「+主键等固定列」 | **恰好 61 列**（PK 是约束不是列） |
| T7 产出物路径 | `docs/04-test/…` | **`discoveries/TASK-007.json`**（`dev-*` 在 docs 树下无任何可写路径，原声明结构上不可完成） |

## 独立 reviewer 在 DoD 阶段补入的 5 条判据

需求文档未覆盖、由 reviewer 用真实 SQLite 实测后补入，全部在交付中闭合：

- **G1** `published_at` / `period` 形态校验——M1a `lookup.go` 包注释明文指派给写入方的契约
- **G2** pending 路径的 `Verdict` 必须与零值可区分（`bitemporal.New == 0`，只测 New 等于没测）
- **G3** `Duplicate` 携带不同 `Values` 时的行为必须钉死
- **G4/G5** 从真实库读 `PRAGMA` 断言 PK 与列类型（删掉 `PRIMARY KEY` 整行时需求文档全部测试仍绿；业务列建成 TEXT 时读回测试也全绿而 `MAX()` 按字典序算错）
- **G10** 拒绝自相矛盾的 `ValidationReport`（`Passed=true` 却含 failed check）

## 已登记未解决

见 `.arcforge/docs/01-design/handoff-to-later-iterations.md`（H1–H7）。其中 **H3 需单列任务**：`rule@v2` 回填重跑历史时全判 `Duplicate`，新字段一个不写、`extractor` 仍旧值，而 `Save` 返回 nil——已从「意外」变成「看得见的决定」，但未解决。
