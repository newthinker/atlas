# Changelog — `internal/macro/bitemporal`（M1a）

> 新增包，**零生产代码改动、零新增依赖**。面向读代码的人；每条断言标注可核实位置。

## 这个包解决什么

双时态表的时态语义：判断一次写入相对库中现状是**新增/重复/修订/乱序**，并构造
「当前行」与「某时点视图」的查询。

**它不做的事**（刻意，不是遗漏）：不导出写操作、不建表、不管事务。
「当前行」由 revision 列派生（`MAX`）而非 `is_current` 列 ⇒ **写入是调用方一句普通 INSERT**。

## 公开面

```go
type Spec struct{ /* 字段全部未导出 */ }
func NewSpec(table string, businessKey []string, revisionCol string) (Spec, error)
type Key map[string]string

type State struct{ Exists bool; LatestRevision string }
type Verdict int   // New / Duplicate / Revision / OutOfOrder，含 String()
func Classify(s State, incoming string) Verdict      // 纯函数，不碰数据库

func CurrentQuery(spec Spec) string
func AsOfQuery(spec Spec) string

type Querier interface{ /* *sql.DB 与 *sql.Tx 共同满足 */ }
func Lookup(ctx context.Context, q Querier, s Spec, k Key) (State, error)
```

**没有别的**——`go doc` 实测 15 个导出标识符，无写操作、无建表函数、无 `Validate`。

## 用它要知道的三件事

**① `Spec` 必须走 `NewSpec` 构造。** 字段未导出，**零值 `Spec{}` 是唯一绕过校验的途径**——
`Lookup` 会拒绝它（`spec.go` / `lookup.go`）。

**② 拼进 SQL 的每个标识符都过 `^[A-Za-z_][A-Za-z0-9_]*$`，业务键取值一律走 `?` 占位符。**
这条声明由**三处**守卫共同支撑，缺一处即不成立：

| 守卫 | 位置 | 守什么 |
|---|---|---|
| identRE | `spec.go` `NewSpec` | 非法标识符进不了 `Spec` |
| 来源核查 + `queryAlias` 过同一道闸门 | `query.go` | SQL 里的标识符只能来自 `Spec` |
| 只用 `Spec` 的字段拼 WHERE | `lookup.go` | 同上，且取值走 `args` |

**另有一条抗回归的静态检查**（`identifiers_test.go`）：用 `go/ast` 扫**包内全部非测试源文件**，
SQL 片段里的每个标识符样式的词必须已登记。新增 SQL 语法会被报成裸标识符，
**迫使加它的人显式登记——那一刻正是审视来源的时机**。
**限度**：只检查字符串**字面量**；`c + " = " + someVar` 这类变量拼接抓不到。

**③ revision 的形态由调用方保证。** `Classify` 用**字符串比较**，前提是 ISO 8601
（字典序 == 时间序）。喂进 `2026-7-15`（少补零）或纯数字版本号（`"9" > "10"`）
会**静默判错、零告警**。**本包明确不管这件事**——由建表方与写入方保证，
`lookup.go` 有文档注释 + 一条可执行测试钉住这个决定。

## M1b 接入时要做的三件事（本包刻意不做）

1. 建 `hestia_observations` 表（含 55 列，**本包不管**）
2. `Duplicate` 分支的 `UPDATE … SET article_id`（设计 §4.2 明确属 M1b）
3. **ADR-0009 的登记**——需求文档 L1158 写明它是「本计划之外的一个待办」，**目前无 owner**

## 验证强度

六个任务、7 个 commit、**零返工**。包内 **125 PASS / 0 SKIP**，覆盖率 **100%**，
`go vet` 干净，全仓 **63 包 0 FAIL**，`go.mod`/`go.sum` 未改动。

**每条 DoD 都有变异实证**，且交付末期由验证者补了四处修复——**没有一处是被红色发现的**：

| 修复 | 修复前的实证 |
|---|---|
| 注入载荷缺双引号系 | `%q` 拼接形态下 `x" OR 1=1 --` **能实际命中**，返回全表最大 revision |
| SQL 字面量断言只否定单引号 | `AND "x"="x"` 变异下 120 条全绿 |
| 无抗回归机制 | 加 `ORDER BY rowid` 后 91 条全绿 |
| 一处空断言 | `"hestia"` 恰是表名 `"hestia_observations"` 的子串，那一套形状空转 |
