# TASK-001 验证报告（QA 返工轮：W-1 / W-2 / W-6）

- 验证者：test-m4-a（返工轮）；首轮报告由 test-m4-b 出具，原文完整保留在文末「附：首轮验证报告（epoch 1）」
- 判定：**VERIFIED**（6 条 done_criteria + 3 条 fix_items 全部通过；2 条不阻断观察见第 6 节）
- 判定对象：master @ `e6f4e9d493fb9e3bcebb0adef9a9edc3fbae8101`（= verify_baseline.head，判定时主仓库 HEAD 相同）；实现提交 `a680de1bed0bb372b75aa2a08f0f8be1c6c40b21`，`git diff --numstat 4cd6ccc e6f4e9d -- internal/hestia/` = queue_health.go 55/5、queue_health_test.go 130/0、store_test.go 8/1，无越界（与 `writes` 三项一致）
- discovery sha256：`c8ee89e5d52a3447efc3c1318986409bac9cea771c493cd1dcef562e2975cfca`（与 baseline 一致）；assignment_epoch=2
- 验证环境：隔离 worktree `../wt-verify-TASK-001`（detached @ 上述全 sha）；对照 worktree `../wt-verify-TASK-001-base`（detached @ `4cd6ccc`，返工前那一版）；GOTOOLCHAIN=local，go1.24.4，euid≠0

## 1. 亲自复跑

| 命令（在 worktree @ `e6f4e9d493fb9e3bcebb0adef9a9edc3fbae8101`） | 结果 |
|---|---|
| `go build ./...` | 通过 |
| `go test -count=1 -cover ./internal/hestia` | ok，96.5% |
| `go test -count=1 -run 'QueueHealth\|ExposesNoWrite' -v ./internal/hestia` | 12 个 PASS（含 MissingDir 四个子测试），0 FAIL，无 SKIP |
| 全量 `go test -count=1 ./...` | rc=0，65 个包 ok，0 FAIL |
| `go vet ./internal/hestia` / `gofmt -l` 三个文件 | 通过 / 无输出 |

## 2. 三条 fix_items 的「会变红」验证（不停在读代码）

### W-1 计件口径分歧 —— 两侧同判，不是各自符合我的期望

我自己造夹具（不复用 dev 的测试），pending/ 下放：指向普通文件的 symlink、悬空 symlink、指向目录的 symlink、FIFO、子目录、`.DS_Store`、`x.json.tmp`。同一夹具喂两侧（触发脚本带 TRIGGER_CMD 桩）：

| 侧 | 输出 |
|---|---|
| Go：`QueueHealthOf(夹具)` | `err=nil pending=0 processing=0 done=0 failed=0 oldestPending=zero byState=map[done:0 failed:0 pending:0 processing:0]` |
| 脚本：`HESTIA_QUEUE_DIR=夹具 bash scripts/ops/hestia-warp-trigger.sh` | `queue empty`，rc=0，桩调用 **0** 次 |

正向对照（同一夹具再放一个真普通文件 `genuine.json`）：Go 侧 `pending=1`，脚本 `triggering (1 pending)`、桩调用 1 次 ⇒ 两侧**同时**翻转，排除「两边都恒为空」这种假一致。
变异 Z1（把 `!e.Type().IsRegular()` 换回 `e.IsDir()`）⇒ `TestQueueHealthIgnoresIrregularFiles` 变红。

### W-2 状态名副本 —— QA 那次变异的形态二现在会红

形态二 = hestia 侧加第五个状态 `archived` 并**改齐**（`queueStates` + `QueueHealth` 字段 + `QueueHealthOf` 的 switch case），`internal/metrics` 一行不动。分别在两棵树上跑全量 `go test ./...`：

| 树 | 子形态 | 结果 |
|---|---|---|
| **首轮 `4cd6ccc`**（返工前） | hestia 侧改齐（该版无 ByState） | **rc=0，全绿** —— 正是 QA 报告的失效形态 |
| 本轮 `e6f4e9d` | (a) hestia 侧改齐、ByState 也加了 case | rc=1，红：`TestQueueHealthByStateCoversAllStates`（`require.Equal` 的四键精确值断言被五键打破） |
| 本轮 `e6f4e9d` | (b) hestia 侧改齐、ByState 漏跟 | rc=1，红：`TestQueueHealthByStateCoversAllStates`（`ElementsMatch` 键集合断言） |
| 本轮 `e6f4e9d` | (c) 第五状态 + **去掉** `switch` 的 default | rc=1，红：同一条用例 |

(c) 单独做是为了回答「这条性质是不是只靠 switch default 撑着」：不是，去掉 default 后键集合守卫独立地把它拦下。
另：只加 `queueStates`、hestia 侧不改齐时两棵树都红（目录/状态不匹配先报错），所以这一格不区分两版，形态二才是有判别力的那个。

### W-6 ErrNotExist 只吞这一种 —— 守卫本身也验了一次

| 变异 | 结果 |
|---|---|
| Z2 去掉 `if os.IsNotExist(err) { continue }`（回到返工前行为） | KILLED：`TestQueueHealthEntryVanishingIsNotAnError` |
| Z3 把条件放宽成 `if err != nil { continue }`（顺手把守卫吞掉） | KILLED：`TestQueueHealthOtherInfoErrorsAreFatal` |

即「放过得太少」和「放过得太多」两个方向各有一条断言守着。注入点是包内接缝 `queueReadDir`（生产路径恒为 `os.ReadDir`），测的是 `QueueHealthOf` 对 Info 错误的处置。

## 3. done_criteria 覆盖矩阵

| # | 完成标准 | 对应测试 / 证据 | 判定 |
|---|---|---|---|
| functional[0] | 件数与最旧年龄（取最旧不是最新） | `TestQueueHealthCountsAndAges`；变异 Z9（`Before`→`After`）、Z6（`needOldest` 只留 pending）均被它杀死 | PASS |
| functional[1] | 复用 `queueStates`、switch 带 default 返回 error、新增 `ByState()` 且键集合断言等于 `queueStates` | review：`queue_health.go` 无任何状态名切片字面量（`grep -cE '\[\]string\{'` = 0），遍历用 `queueStates`；switch 的 default 在第 108-110 行返回 error；`ByState` 在第 28-43 行。断言性质由第 2 节 W-2 四格实验实证；变异 Z4（ByState 漏 done）被杀 | PASS |
| functional[2] | AST 守卫登记 `QueueHealth.ByState` 并附「为什么是登记不是放宽」注释；两条守卫精确集合相等且绿；reflect 守卫 want 零改动 | `store_test.go` 8/1：want 行加一项 + 7 行注释块；`TestPackageExposesNoWriteFunctions`、`TestStoreExposesNoWriteMethods` 均 PASS；变异 Z10（从 want 删掉 ByState）⇒ AST 守卫变红，证明它是精确集合比较 | PASS |
| boundary[0] | 四目录全空 ⇒ nil error、四件数 0、两个年龄 IsZero | `TestQueueHealthEmptyIsNotAnError` | PASS |
| boundary[1] | 非常规文件（symlink 三例 + FIFO 等）、点文件、`.tmp` 不计件；与触发脚本同判；注释写全判据 | `TestQueueHealthIgnoresIrregularFiles`（含同夹具跑触发脚本）、`TestQueueHealthIgnoresNonItems`；我的独立夹具见第 2 节 W-1；变异 Z1/Z11b（点文件）/Z12（`.tmp`）均被杀。注释：`queue_health.go:53-62` 写全了判据、symlink 为什么靠 `IsDir()` 挡不住、以及分叉的后果 | PASS |
| error_handling[0] | 四状态各删一次 ⇒ error 含子目录名且返回零值；条目在 ReadDir 与 Info 之间消失 ⇒ 不是错误；其它 Info 错误仍响亮失败 | `TestQueueHealthMissingDirIsAnError`（四个子测试）、`TestQueueHealthEntryVanishingIsNotAnError`、`TestQueueHealthOtherInfoErrorsAreFatal`；变异 Z8（目录读不到退化成零件数）、Z2、Z3 均被杀 | PASS |
| error_handling[1] | 子目录权限 000 ⇒ error；root 下 Skip | `TestQueueHealthUnreadableDirIsAnError` PASS（本机 euid≠0，实际执行未 Skip）；Z8 也杀它 | PASS |
| non_functional[0] | 纯文件系统读 | review：`queue_health.go` 的 import 只有 fmt/os/path/filepath/strings/time；`database/sql` 0 处；`Store` 的 2 处命中都在注释里（「不接收 *Store」与「.DS_Store」）；SQL 关键字的 1 处命中是注释里的 `rsync --delete` | PASS |

## 4. 变异汇总（15 个，隔离 worktree，逐个还原并比对 sha256，收尾 `git status --porcelain` 0 行）

KILLED 13：Z1 去 IsRegular、Z2 ErrNotExist 不 continue、Z3 吞掉所有 Info 错误、Z4 ByState 漏 done、Z6 needOldest 只留 pending、Z8 目录读不到退化成零件数、Z9 最旧取最新、Z10 AST want 删 ByState、Z11b 点文件过滤失效、Z12 `.tmp` 过滤失效、Z13 第五状态+去 default，以及第 2 节 W-2 的形态二 (a)(b) 两格。
SURVIVED 2（都在四状态下不可达，不构成缺口）：

- Z5 `ByState` 加 `default: by[state] = 0`：四个状态全部接线时行为不变；它要破坏的性质（加第五状态时变红）已由形态二 (a)/(b) 证明仍然成立。
- Z7 去掉 `QueueHealthOf` 的 switch default：四状态下这条分支不可达；加第五状态时由 Z13 证明键集合守卫仍会红。DoD 对这条的要求是 review（代码里确实带 default 并返回 error），不是要有一条直接踩中它的测试。

`go vet` 在每个变异体上都跑过；一个语法上合法但会导致编译失败的变异（删掉过滤条件致 `strings` 未使用）被 vet 当场发现并替换为 Z11b/Z12，未计入 KILLED。

## 5. 对 Leader 已知事项的核对

`scripts/ops/hestia-warp-trigger.sh` 的注释未改（Leader 裁决）。我读了它：第 15-17 行写的是「只数普通文件，子目录、以 `.` 开头的文件与 `.tmp` 结尾的文件都不算」——「只数普通文件」这句与 `find -type f` 的实际行为一致，没有错误陈述，也没有和 Go 侧分叉。DoD boundary[1] 那句「两处注释同步写全判据」按 Leader 裁决处理：Go 侧已写全，脚本侧维持原状不影响判据一致性（第 2 节 W-1 的双侧实测就是直接证据）。

## 6. 不阻断观察

1. `ByState()` 对未接线状态**故意不给键**，这是守卫得以变红的机制；但它同时意味着消费侧按 `queueStates` 遍历取值时会遇到缺键（Go 取 map 缺键得 0，不会 panic）。TASK-003 的 review_fix 接线时要留意「缺键」和「值为 0」在下游语义上是否需要区分。
2. `needOldest` 优化后，done/ 与 failed/ 不再调 `e.Info()`，所以这两个目录里「条目在两步之间消失」既不会报错也不会漏计——它会被计成一件（计数发生在 Info 之前）。与 pending/processing 的处置（消失就不计）方向相反。件数是瞬时量，两种处置都说得通，只是口径在同一个函数里不统一，值得在注释里点一句。

---

## 附：首轮验证报告（epoch 1，验证者 test-m4-b，原文保留）

### TASK-001 验证报告 —— QueueHealthOf（首轮原文）

- 验证者：test-m4-b（本 sprint 第二验证者）
- 判定：**VERIFIED**（附 3 条不阻断观察，见末节）
- 判定对象：master @ `717c627d09f35251c247ad0fc5f2d0f79322745c`（= verify_baseline.head；判定时主仓库 HEAD 相同，三个声明文件 `git diff --stat` 为空；discovery sha256 `de401689…f8f2` 与基线一致，无漂移）
- 验证环境：隔离 worktree `../wt-verify-TASK-001-b`（detached @ 上述全 sha），euid=501（非 root，chmod 测试实际执行未 Skip）
- assignment_epoch：1

#### 1. 亲自复跑（不采信 discovery 回显）

| 命令（在 worktree @ 717c627d09f35251c247ad0fc5f2d0f79322745c） | 结果 |
|---|---|
| `GOTOOLCHAIN=local go test -count=1 ./internal/hestia/...` | `ok hestia 4.695s` / `ok hestia/sheets 4.878s` |
| `go test -count=1 -v -run 'TestQueueHealth\|TestPackageExposesNoWriteFunctions\|TestStoreExposesNoWriteMethods'` | 7 个顶层 + 4 个子测试全部 PASS，无 SKIP |
| `go test -coverprofile` + `go tool cover -func` | `QueueHealthOf 91.3%`，包 total `96.4%`（与 discovery 一致） |
| `go vet ./internal/hestia/` | 通过 |

范围核对：merge `717c627` 相对第一父的改动恰为 `queue_health.go`(+64) / `queue_health_test.go`(+126) / `store_test.go`(+10/-1)，与 `writes` 三项完全一致，无越界。

#### 2. Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 / 核对方式 | 判定 |
|---|---|---|---|
| functional[0] | 件数与最旧年龄 | `TestQueueHealthCountsAndAges`（fixture 与 DoD 逐字一致，含 processing 两件、DoneCount=1） | PASS |
| functional[1] | 复用 queueStates、无第二份名单字面量、switch 带 default 报错 | review：`for _, state := range queueStates`（queue_health.go:29）；四个状态名只作为 `case` 标签出现，无切片/数组字面量；`default` 返回零值 + error（:57-59）。default 在当前 queueStates 下不可达，覆盖率缺口属预期 | PASS |
| functional[2] | AST 守卫登记 + 注释；两个守卫精确相等且绿；reflect want 零改动 | `TestPackageExposesNoWriteFunctions` PASS、`TestStoreExposesNoWriteMethods` PASS；diff 核对：reflect want 行未改动，AST want 仅按字母序插入 `"QueueHealthOf"`，附「登记而不是放宽」注释块 | PASS |
| boundary[0] | 四目录空 ⇒ nil、零件数、零年龄 | `TestQueueHealthEmptyIsNotAnError` | PASS |
| boundary[1] | 子目录 / `.` 开头 / `.tmp` 结尾不计件不计龄 | `TestQueueHealthIgnoresNonItems`；三类各有变异 KILLED（M2/M3/M4） | PASS |
| error_handling[0] | 四状态各删一次 ⇒ error 含名、零值 | `TestQueueHealthMissingDirIsAnError` 四子测试；M5/M6/M8 KILLED | PASS |
| error_handling[1] | chmod 000 ⇒ error；root Skip | `TestQueueHealthUnreadableDirIsAnError`（本机非 root，实际执行）；M7 KILLED | PASS |
| non_functional[0] | 纯文件系统读 | review：import 仅 fmt/os/path/filepath/strings/time；`grep -nE 'database/sql\|Store\|SELECT\|INSERT\|sql'` 仅命中 :19/:25 两行**注释**，代码中无 `*Store`、无 SQL | PASS |

#### 3. 变异测试（独立设计，作用于隔离 worktree，逐个还原 sha256 一致，收尾 `git status --porcelain` 与开始时相同）

| 变异 | 结果 | 杀死它的测试 |
|---|---|---|
| M1 `Before`→`After`（取最新） | KILLED | CountsAndAges |
| M2 去点文件过滤 | KILLED | IgnoresNonItems |
| M3 去 `.tmp` 过滤 | KILLED | IgnoresNonItems |
| M4 去 `IsDir` 过滤 | KILLED | IgnoresNonItems |
| M5 ReadDir 出错 `continue`（C3 退化） | KILLED | MissingDir×4 + Unreadable |
| M6 仅 `IsNotExist` 当空 | KILLED | MissingDir×4 |
| M7 仅 `IsPermission` 当空 | KILLED | Unreadable |
| M8 出错返回部分结果 `h` | KILLED | MissingDir/processing,done,failed（pending 首个读取、h 本就为零，预期存活于该子测试） |
| M9 错误信息里 `state` 换成 `dir` | **SURVIVED** | 见观察 2 |
| M9b 错误信息改成不含路径与状态的固定串 | KILLED | MissingDir + Unreadable |
| M10 不写 OldestPending | KILLED | CountsAndAges |
| M11 不写 OldestProcessing | KILLED | CountsAndAges |
| M12 Done/Failed 串位 | KILLED | CountsAndAges |
| M13 `n++`→`n += 1 + len%1` | 存活，**等价变异**（`len%1≡0`），本人设计失误，不计 | — |
| M14 AST want 去掉 QueueHealthOf | KILLED | PackageExposesNoWriteFunctions |
| M15 年龄取「目录序第一个」而非最旧 | **SURVIVED** | 见观察 1 |

有效变异 14 个（剔除等价 M13），KILLED 12，SURVIVED 2；两个存活均经探针证实**实现行为正确**，缺口在测试区分力而非实现。

#### 4. 不阻断观察（建议后续补强，不构成本任务缺陷）

1. **最旧年龄的 fixture 名字序与年龄序同向（M15 存活）。** `os.ReadDir` 按文件名排序，DoD 规定的 fixture 恰为 `a.json`(3h) 先于 `b.json`(1h)、`d.json`(2h) 先于 `e.json`(30m)，于是「取第一个」的实现能过。临时探针（名字序与年龄序相反：pending `a`1h/`m`2h/`z`3h、processing `a`30m/`z`2h）在真实实现上 PASS，证明**实现正确**；测试只挡住了「取最新」、没挡住「取首个」。fixture 是 DoD 逐字规定的，故不判 task_defect；建议后续给该测试加一组名字序反向的样本。探针未提交、已删除。
2. **「err 含子目录名」断言由 `*PathError` 自带路径兜底（M9 存活）。** 错误链 `%w` 里的 path 已含 `.../done`，所以即使去掉显式 `state` 断言仍过；M9b（完全不含名字）被杀，断言并非空洞，只是无法区分「显式写名」与「靠底层路径带出」。行为本身满足 DoD。
3. **与 TASK-006 触发脚本的判据在符号链接上分叉。** 脚本用 `find ... -type f ! -name '.*' ! -name '*.tmp'`，Go 用 `!e.IsDir()`：探针在 pending/ 放一个指向文件的符号链接，`QueueHealthOf` 计 `PendingCount=1`，脚本 `-type f` 不计（其它非普通文件如 socket/FIFO 同理）。DoD boundary[1] 列举的三类两边一致，故本条不在 DoD 内；队列由 `writeAtomic` rename 普通文件产生，现实中不太可能出现符号链接，但实现注释自称「判据与触发脚本一致，否则两边对『空』的认定会分叉」，严格来说并不完全一致，提请 Leader 知悉。

#### 5. 复现命令（锚为全 sha）

```bash
git worktree add --detach ../wt-verify 717c627d09f35251c247ad0fc5f2d0f79322745c
cd ../wt-verify && GOTOOLCHAIN=local go test -count=1 -v ./internal/hestia/ -run 'TestQueueHealth|TestPackageExposesNoWriteFunctions|TestStoreExposesNoWriteMethods'
```
