# TASK-003 验证报告（SQLite 存储）

- 验证者：test-pw-b；`assignment_epoch`=1
- **被测树**：`feature/pricewatch` @ `f3d3223136084113ea104d57891d827d5f768419`（= `verify_baseline.head`），独立 worktree `../wt-verify-TASK-003-b`（detached），主工作区未改动
- 被测文件 sha256（与 discovery 声明逐一致）：
  - `internal/pricewatch/store.go` `31e9169e78ea92f856761d8ae2b73f66b47a93553e87e3a028fffd38aca2da64`
  - `internal/pricewatch/store_test.go` `6ff935295415ea289e31cd676396974bf0133e5d07e90e76af7354e405ed4dfa`
- 范围：`1854aa8 feat(TASK-003)` 只改上述两个文件，与 `writes` 一致；无越界
- 工具链：`GOTOOLCHAIN=local`，`modernc.org/sqlite v1.38.2`（go.mod 核实）

## 结论：VERIFIED

## 1. 套件 / 静态检查（独立重跑，不采信 dev 自证）

| 命令 | 结果 |
|---|---|
| `GOTOOLCHAIN=local go test -race -count=1 -coverprofile ./internal/pricewatch/` | `ok`，包覆盖率 **93.1%**（含同包 TASK-001/002/011） |
| `go vet ./internal/pricewatch/` | 无输出，rc=0 |
| `gofmt -l store.go store_test.go` | 无输出 |
| `go test -race -count=10 -run TestStore_Concurrent` | `ok`（无 race、无 `database is locked`） |
| 探针（overlay 加测试文件，查池中多连接） | `journal_mode=wal`、`busy_timeout=5000`，3 次一致 |

### 本任务文件函数覆盖（store.go，同一 profile）

NewStore 72.7%｜Close 100%｜fromMs 100%｜UpsertCandles 71.4%｜LatestCandle 85.7%｜Stats 85.7%｜FirstCloseSince 85.7%｜PruneCandles 66.7%｜GetRuleState 87.5%｜PutRuleState 75.0%｜GetKV 85.7%｜PutKV 66.7%

store.go 单文件语句覆盖 **58/73 = 79.5%**。未覆盖的 14 个块**全部**是 DB 调用失败后的 `return fmt.Errorf(...)` 错误包装分支（62、65、79、84、89、93、103、124、143、151、169、182、195、202 行起）。DoD 的覆盖率口径是**包覆盖率**（门禁口径）= 93.1%，达标；单文件数字仅供参考。

## 2. 完成标准覆盖矩阵

| # | done_criteria | 对应测试 | 守卫证据（变异） | 判定 |
|---|---|---|---|---|
| functional[0] | 乱序 3 根写两次 + 同 ts 更高 high → Count==3、High 取新值 | `TestUpsertCandles_Idempotent` | M02 OR IGNORE KILLED | PASS |
| functional[1] | [from,to] 两端含；from 零值统计全部；空区间 Count==0；First 最早；取 high/low 非 close | `TestStats_Range`（6 子用例） | M03/M04 close 代替 high/low、M05/M06 端点开区间、M07 零值 from 被改写、M09 First=MAX、M10 去空集守卫 均 KILLED；M08 存活（见 §4，等价） | PASS |
| functional[2] | Put {2,1,T0} → Close → 同路径 NewStore → Get 相等（毫秒）；不存在返回零值 err==nil | `TestRuleState_SurvivesReopen`（T0 带 123456789ns，比 Truncate(ms)） | M14 秒精度、M20 不存在返回 err、M21 不存在返回非零 KILLED | PASS |
| functional[3] | Prune（< before 删）；KV 不存在 ""/写后读/覆盖；LatestCandle 空表 ok=false；FirstCloseSince ts≥from 最早、无数据 ok=false | `TestPruneCandles` `TestKV` `TestLatestCandle` `TestFirstCloseSince` | M40 Prune<=、M44 Prune 反向、M22/M23 GetKV 不存在、M24/M25 PutKV 不覆盖、M32 Latest MIN、M35 空表 ok=true、M36 DESC、M37 >、M38 无数据 ok=true KILLED | PASS |
| boundary[0] | pair/bar 隔离（Stats/Prune/Latest）、同 ts 1d 与 5m 各自计数、rule_state 按 (pair,rule_id) | `TestStore_Isolation` | M01 PK 去 bar、M11 Stats 去 bar、M17 rule_state PK 去 pair、M18 Get 去 pair、M33/M34 Latest 去 pair/bar、M39 FirstClose 去 pair、M41/M42 Prune 去 pair/bar KILLED | PASS |
| boundary[1] | 非 UTC Location 写入读回 Equal（毫秒截断） | `TestStore_NonUTCRoundTrip`（UTC+8、本地/UTC 日期不同、987654321ns） | M12 Upsert 秒精度、M13 Upsert 墙钟当 UTC、M15 PutRuleState 墙钟当 UTC KILLED | PASS |
| error_handling[0] | NewStore 自动建父目录；父路径为普通文件返回 error 不 panic | `TestNewStore_CreatesDir` `TestNewStore_BadPath` | M29 不建目录 KILLED；M30 吞 MkdirAll 错误存活（见 §4，DoD 层面等价） | PASS |
| non_functional[0] | -race 通过、vet 无输出、gofmt 无输出、包覆盖率 ≥80%；并发 Upsert/Stats/PutRuleState 无 race、无 locked | 实跑（§1）+ `TestStore_Concurrent`（8×25 轮，三种操作） | M26 去 busy_timeout、M28 去 WAL+busy KILLED（-count=5）；M27 仅去 WAL 存活（见 §4） | PASS |

## 3. 独立变异（44 个，`go test -overlay`，隔离 worktree）

- harness：每个变异锚点须在原文中**恰好出现 1 次**（否则记 INVALID，本轮 0 个）；落盘后先 `go vet -overlay` 确认编译通过（0 个编译失败）；并发类（M26–M28）`-count=5`。
- 源文件 sha256 跑前跑后一致；验证 worktree `git status --porcelain` 前后均 0 行。
- **39 KILLED / 5 SURVIVED**。KILLED 各自命中的测试与 DoD 条目语义对应（见矩阵列），非「全红」型崩溃（M29/M43 全红有其直接因果：库建不起来/数据没提交）。
- 派验要求的必覆盖项：主键去 bar(M01)、INSERT OR IGNORE(M02)、Stats 用 close(M03/M04)、端点开闭(M05/M06/M37/M40)、零值 from(M07/M08)、毫秒/时区(M12–M15)、rule_state 键(M17/M18)、GetKV 不存在(M22/M23)、busy_timeout/WAL(M26–M28)、NewStore 建目录与坏路径(M29/M30) —— 全部覆盖。

## 4. 存活变异分析（均不构成 DoD 缺陷，记录供下游参考）

| 变异 | 为什么存活 | 判断 |
|---|---|---|
| M08 零值 from 钳到 1970-01-02 | 测试数据全在 2026；「无下界」与「下界在 1970」对真实数据不可区分 | 等价变异，非缺陷 |
| M16 `fromMs` 不 `.UTC()` | DoD 只要求 `Equal`，测试全部用 `Equal` 比较；读回的 Location 不受断言约束 | DoD 未要求读回为 UTC（description 的「统一 UTC」指存储口径）；若下游 Task 4/5 依赖读回时间的 Location（如格式化日期），需在那边加断言 |
| M27 仅去 WAL | busy_timeout 单独已能让并发测试不报 locked；WAL 是否开启无断言守卫 | DoD 未要求 WAL；探针实测当前确为 `wal` |
| M30 吞 MkdirAll 错误 | 父路径是文件时后续 sqlite 打开/建表同样失败，仍返回 error | DoD 只要求「返回 error 而非 panic」，行为等价 |
| M31 吞建表错误 | 无测试触发建表失败（BadPath 已在 MkdirAll 处返回） | 非 DoD；错误路径未测 |

## 5. 非 DoD 发现（不据此 reject）

1. store.go 单文件语句覆盖 79.5%，未覆盖部分全部是 DB 错误包装分支（§1）。
2. M16：读回时间 UTC 化无断言守卫（§4）。下游若按日期分组/格式化读回时间，建议在下游加 Location 断言。
3. M27：WAL 无断言守卫（§4）。

## 6. 复现命令（锚为全 sha）

```bash
git worktree add --detach ../wt-verify-TASK-003-b f3d3223136084113ea104d57891d827d5f768419
cd ../wt-verify-TASK-003-b
GOTOOLCHAIN=local go test -race -count=1 -coverprofile=c.out ./internal/pricewatch/ && go tool cover -func=c.out | grep store.go
GOTOOLCHAIN=local go vet ./internal/pricewatch/; gofmt -l internal/pricewatch/store.go internal/pricewatch/store_test.go
GOTOOLCHAIN=local go test -race -count=10 -run TestStore_Concurrent ./internal/pricewatch/
python3 <scratchpad>/test-pw-b-T003-mut/run.py "$PWD" <输出目录>   # 44 变异 overlay harness
```
