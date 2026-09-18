# TASK-003 验证报告（第 2 轮：QA 返工 W-2 collector 侧）

- 验证者：test-m4-a（两轮同一验证者）；第 1 轮报告原文完整保留在文末「附：第 1 轮验证报告（epoch 1）」
- 判定：**VERIFIED**（functional[0] 新增的 W-2 要求 + 2 条 fix_items 全部通过；原有 8 条标准回归全绿）
- 判定对象：master @ `bd31159ba70b9d664fed5b66b787b26a0a26cfb8`（= verify_baseline.head，判定时主仓库 HEAD 相同）；本轮提交 `c3678bf32a197dfcb41018825948914f41163dc4` 是其祖先，`git diff --stat c3678bf..bd31159 -- internal/metrics cmd/atlas` 为空
- discovery sha256：`30688a336301661ff4ecd36dc2cdbf30dd618f13aecb2891634039d731ef26c2`（与 baseline 一致）；assignment_epoch=2
- 声明范围：`git diff --numstat c3678bf^ c3678bf` = hestia_collector.go 12/5、hestia_collector_test.go 26/0，均在 `writes` 内，无越界
- 验证环境：隔离 worktree `../wt-verify-TASK-003b`（detached @ 上述全 sha），GOTOOLCHAIN=local

## 1. 亲自复跑

| 命令 | 结果 |
|---|---|
| `go build ./...` | 通过 |
| `go test -count=1 -cover ./internal/metrics ./cmd/atlas` | metrics ok 99.3%；cmd/atlas ok 78.0% |
| 全量 `go test -count=1 ./...` | rc=0，65 个包 ok，FAIL 行 0 |
| `go vet ./internal/metrics` / `gofmt -l` 两个文件 | 通过 / 无输出 |
| 状态名字面量：取 `collectQueue` 函数体、剔除注释行后正则找 `"pending\|processing\|done\|failed"` | **0 处**（新测试 `TestHestiaCollector_QueueItemsFollowByState` 的函数体同样 0 处） |

## 2. 两种形态分别复现（按 Leader 与 dev 指出的措辞差别）

在隔离 worktree 里构造「hestia 侧加第五状态 `archived`」的两种改法，再叠加 collector 侧的变异，共 8 组（含对照），每组都先 `go vet`：

| 组 | 构造 | 结果 | 读法 |
|---|---|---|---|
| control | 原样 | metrics + hestia 全绿 | — |
| **S1** | 第五状态**已接线**（queueStates + 字段 + switch case + ByState case），collector 不动 | rc=1，红的是 `QueueFullOutput`、`QueueEmptyOmitsAges`、`DBFailureKeepsQueueMetrics`；`QueueItemsFollowByState` **绿** | 件数**没有**丢：collector 自动输出了第五个状态；红的是 DoD 钉死「恰四个序列」的夹具期望 ⇒ 需人更新期望的**可见信号**，不是缺陷。与 dev 的 key_findings 末条、Leader 的提醒一致 |
| **M1** | S1 + collector 撤回字面量 map（即撤销本次修复） | rc=1，**恰** `TestHestiaCollector_QueueItemsFollowByState` 红 | 新守卫确有牙：撤销修复后，「hestia 改齐而 collector 漏跟」这一形态被单独抓住 |
| **S2** | 第五状态**未接线**（ByState 不给该键） | collector 侧 rc=0（无红） | collector 不假红，符合 fix_items[1] 的选择（缺键不补零） |
| S2（hestia 侧） | 同上 | `TestQueueHealthByStateCoversAllStates` 红 | 接线缺口由 hestia 侧报，分工正确 |
| **M2** | collector 改为「固定名单 + `by[state]` 补零」（名单含未接线的 archived） | rc=1，红 4 条：`QueueFullOutput`、`QueueEmptyOmitsAges`、`DBFailureKeepsQueueMetrics`、`QueueItemsFollowByState` | 补零会被当场抓到 |
| M2b | M2 + 第五状态未接线（把「不知道」说成「0 件」） | 同上 4 条红 | 同上 |
| M3 | 只撤回字面量 map、不加第五状态 | rc=0，存活 | 等价变异：四状态全部接线时两种写法输出相同。**这正是为什么必须叠加第五状态才验得出来**——只做 M3 会得到「修复无差别」的错误印象 |

## 3. done_criteria 与 fix_items 覆盖

| 项 | 证据 | 判定 |
|---|---|---|
| functional[0] 新增的 W-2 要求：四个序列由 `q.ByState()` 驱动、collectQueue 内无第二份状态名字面量、有一条测试断言 state 标签集合 == ByState 键集合且「hestia 加状态、collector 不动」必变红 | 实现第 171 行 `for state, n := range q.ByState()`；字面量 0 处（第 1 节）；`TestHestiaCollector_QueueItemsFollowByState` 用 `reflect.DeepEqual` 做**集合与值逐项相等**（非 Contains），期望值取自 `q.ByState()`，测试内不列第二份名单；有效性由 M1 实证 | PASS |
| functional[0] 原有部分 / functional[1] / functional[2] / boundary / error_handling / non_functional | 第 1 轮已逐条验过（见附录），本轮 metrics 包全绿、全量 65 包绿即回归证据；`Collect` 本体仍只有 `collectDB`/`collectQueue` 两句 | PASS（回归） |
| fix_items[1] 接线提醒：确认「缺键」与「值为 0」的处理并在注释里写明选择与理由 | `collectQueue` 第 163-170 行注释写了：缺键 ⇒ Snapshot 无该键 ⇒ 规则求值 false、不假红；补零会把「不知道」说成「确认为 0 件」并掩盖 hestia 侧接线缺口。S2 与 M2b 两组实测正好分别对应这两种处置的后果 | PASS |

## 4. 观察（不在 DoD 内，不作判定依据）

1. S1 暴露的是**测试夹具的口径**问题：三条断言把「恰四个序列」钉死在测试里，将来 hestia 侧加状态时它们必然变红，需要人工更新期望。这与 W-2 要堵的「静默」相反，是可见信号；但如果那时有人图省事把断言改成「至少包含四个」，守卫就会退化。建议在那三条断言旁留一句「变红时应更新期望值，不要放宽成包含」。
2. 第 1 轮报告第四节的观察 1（collectQueue 重列状态名）本轮已被修复；第 3 节关于 `queueStates` 未导出的说明现在的答案是：不导出常量，而是导出 `QueueHealth.ByState()` 这个方法作为单一口径。

---

## 附：第 1 轮验证报告（epoch 1，原文保留）

### TASK-003 验证报告（第 1 轮原文）

- 验证者：test-m4-a
- 判定对象：master @ `c4d651c8abb0463fd9be070df93b44be3fcd5373`（= verify_baseline.head，判定时主仓库 HEAD 相同）；实现提交 `b5aacd4c08b69dec692579b4220e25d587922fe1` 是其祖先，`git diff --stat b5aacd4..c4d651c` 在三个声明文件上为空
- discovery sha256：`dfbc5d3f2b350be8753f9d1302f370cd929d8bd1283f019e1c5433404287c31a`（与 baseline 一致）
- 声明范围核对：`git diff --numstat b5aacd4^ b5aacd4` 只涉及 `writes` 里的三个文件（hestia_collector.go 74/8、hestia_collector_test.go 255/7、cmd/atlas/hestia_health.go 1/1），无越界
- 运行环境：独立 worktree `../wt-verify-TASK-003`（detached @ 上述全 sha），GOTOOLCHAIN=local
- 结论：**VERIFIED**（8/8 条 done_criteria 通过）

#### 一、验证者亲自运行的证据

| 检查 | 结果 |
|---|---|
| `go build ./...` | 通过 |
| `go test -count=1 -cover ./internal/metrics ./cmd/atlas` | metrics ok 99.3%；cmd/atlas ok 78.0% |
| 全量回归 `go test -count=1 ./...` | rc=0，65 个包 ok，0 个 FAIL |
| `go vet ./internal/metrics ./cmd/atlas` | 通过 |
| `gofmt -l` 三个改动文件 | 无输出 |
| `NewHestiaCollector(` 非测试调用方 | 仅 `cmd/atlas/hestia_health.go:44`，改动为新参数位传 `nil` 一处 |
| `go test -v -run TestHestiaCollector` | 16 个 PASS 行（13 个顶层 + Pedantic 3 个子测试），0 FAIL |

测试辅助核查：`gaugeValue`/`counterValue` 经 `firstMetric` 在指标族缺席时 `t.Fatalf`，所以 `== 0` 类断言（如 `db_up==0`、`queue_errors_total==0`）同时断言了「输出存在」，不会因缺席读出默认 0 而空洞通过。W5、W6 变异实证了这一点。

#### 二、done_criteria 覆盖矩阵

| # | 完成标准 | 对应测试 / 证据 | 判定 |
|---|---|---|---|
| functional[0] | DB+queue 成功：items 恰四序列 2/1/3/1、age 3/1、queue_up/db_up=1、queue_errors 输出且 0、DB 指标照常 | `TestHestiaCollector_QueueFullOutput`（序列数 ==4 + 逐值 + 两个 age + 两个 up + errors 0 + hours_since_last_run 2）；W2/W3/W4/W6/W7/W11/W17 被杀 | PASS |
| functional[1] | 注册进 Registry 后 Snapshot 含 `hestia_queue_items_failed==1`、`hestia_queue_up==1` | `TestHestiaCollector_QueueVisibleInSnapshot`（`NewRegistry()` + `Snapshot()`）；W2（state 值改名）、W16（label 名改 kind，TASK-002 展开失效）被杀 | PASS |
| functional[2] | Collect 只顺序调用 collectDB 与 collectQueue，任一方法内 return 跳不过另一方法 | review：`hestia_collector.go:106-109` Collect 函数体只有 `c.collectDB(ch)` 和 `c.collectQueue(ch)` 两句；两个方法里的 `return` 都只退出本方法。W18（DB 失败时跳过 queue）被 C2 用例杀死；W19（二者调换顺序）存活，属等价变异 | PASS |
| boundary[0] | 零值 QueueHealth：四个 items 序列都为 0、两个 age 不输出、queue_up==1 | `TestHestiaCollector_QueueEmptyOmitsAges` | PASS |
| boundary[1] | queue==nil：无任何 `hestia_queue_` 前缀指标（含 up 与 errors）；DB 指标与 db_up 照常 | `TestHestiaCollector_QueueNilSkipsQueue`（遍历全部族名做前缀检查 + hours_since + db_up==1）；W9 被杀 | PASS |
| error_handling[0] | ①queue 失败：DB 指标照常、queue_up==0、queue_errors==1、无 items 和两个 age；②DB 失败：items 照常、db_up==0、collect_errors==1 | `TestHestiaCollector_QueueFailureKeepsDBMetrics`、`TestHestiaCollector_DBFailureKeepsQueueMetrics`；W5/W14/W15/W10/W18 被杀 | PASS |
| error_handling[1] | 连续两次 Collect：先失败后成功 ⇒ up 回到 1、errors 保持 1；DB 侧同构 | `TestHestiaCollector_QueueUpRecovers`、`TestHestiaCollector_DBUpRecovers`（同一 collector 实例 gather 两次） | PASS |
| non_functional[0] | PedanticRegistry 在成功 / queue 失败 / DB 失败三形态下 Gather 无 error；`go build ./...` 与 `go test -count=1 ./internal/metrics ./cmd/atlas` 全绿 | `TestHestiaCollector_PedanticRegistry` 三个子测试；W12（Describe 漏 queueItems）、W13（漏 queueErrors）只被 Pedantic 用例杀死，印证 dev 的提示「非 pedantic registry 抓不到 Describe 漏声明」；build/test 为验证者亲跑（见第一节） | PASS |

#### 三、验证者独立变异（在我专用的 worktree 里做，每个变异后还原并比对 sha256，最后 `git status --porcelain` 0 行；与 dev 的 16 个变异互补，不重复）

对照组全绿。18 个变异 `go vet` 全部通过，没有 panic 致红：

| 变异 | 结果 / 杀死它的测试 |
|---|---|
| W2 state 值 failed→failure | KILLED：QueueFullOutput、QueueVisibleInSnapshot |
| W3 pending 件数取 ProcessingCount | KILLED：QueueFullOutput、DBFailureKeepsQueueMetrics |
| W4 done 件数取 FailedCount | KILLED：QueueFullOutput |
| W5 queue 失败时不输出 errors counter | KILLED：QueueFailureKeepsDBMetrics |
| W6 queue 成功时不输出 errors counter | KILLED：QueueFullOutput、QueueUpRecovers |
| W7 processing 年龄取 OldestPending | KILLED：QueueFullOutput |
| W9 nil 时输出 queue_up=1 | KILLED：QueueNilSkipsQueue |
| W10 DB 失败时 db_up=1 | KILLED：DBError…AndDBUp、DBFailureKeepsQueueMetrics、DBUpRecovers |
| W11 queue 成功时 queue_up=0 | KILLED：5 个用例 |
| W12 Describe 漏 queueItems | KILLED：只有 PedanticRegistry（success、db_failure） |
| W13 Describe 漏 queueErrors | KILLED：只有 PedanticRegistry（三个子测试） |
| W14 queue 失败时仍输出 0 件 items | KILLED：QueueFailureKeepsDBMetrics |
| W15 queue 失败时 Inc 两次 | KILLED：QueueFailureKeepsDBMetrics、QueueUpRecovers |
| W16 items 的 label 名 state→kind | KILLED：4 个用例（含 Snapshot 端到端） |
| W17 年龄用 Minutes 计算 | KILLED：QueueFullOutput |
| W18 DB 失败时跳过 collectQueue | KILLED：DBFailureKeepsQueueMetrics、DBUpRecovers |
| W19 Collect 里二者调换顺序 | SURVIVED：等价变异（输出集合不依赖顺序） |

#### 四、观察（不在 DoD 内，不作判定依据）

1. **状态名在 collectQueue 里又列了一遍**（Leader 读 diff 时指出，我认同）：`hestia_collector.go:162-163` 用 map 字面量重新列出 pending/processing/done/failed 四个状态，没有复用 `internal/hestia/queue.go:12` 的 `queueStates`。补充一点：`queueStates` 是**未导出**的，metrics 包本来就引用不了；而 `QueueHealth` 是四个固定字段，不是按状态名索引的 map，所以直接复用做不到，只能先在 hestia 侧导出一个状态名 → 件数的访问方式。风险：以后 queueStates 增加第五个状态时，`QueueHealthOf` 会大声报错（queue_health.go:59），但 collector 这边不会产生对应序列，也没有任何检查会报出来。
2. map 遍历顺序是随机的，所以四个 items 序列的输出顺序每次抓取都不同。prometheus 的 Gather 会按 label 排序，对外输出不受影响，只是记一笔。
3. Pedantic 用例没覆盖 `queue == nil` 这一形态（DoD 只要求三形态）。nil 时只是少输出、声明照旧，对注册表是合法的，风险很低。

#### 五、对观察 1 的订正（Leader 2026-09-17 补充，经验证者读代码核对，不改结论）

观察 1 里「collector 这边静默缺序列」说得太笼统，实际要分两种形态（依据：`internal/hestia/queue_health.go` 的 switch，@ `c4d651c8abb0463fd9be070df93b44be3fcd5373`）：

| 加第五个状态时怎么改 | 结果 | 响不响 |
|---|---|---|
| 只往 `queueStates` 加，switch 没跟上 | 走到 `default` ⇒ `QueueHealthOf` 返回 error ⇒ collector 输出 `queue_up=0` ⇒ `hestia_queue_blind` 告警 | **响**（Leader 指出的就是这种） |
| hestia 侧改齐（`queueStates` + `QueueHealth` 字段 + switch case），collector 的 map 字面量没跟上 | `QueueHealthOf` 正常返回，`queue_up=1`，新状态的件数不输出 | **静默** |

所以「静默」只在第二种形态下成立，而第二种恰恰是 hestia 侧改得**完整**时才会出现。另外：`hestia_queue_blind` 规则在本 HEAD 上还没有入库（由 TASK-005 交付，目前只出现在 TASK-005 的任务描述和 AD-2 里），所以第一种形态「会响」要等 TASK-005 合入后才成立。
