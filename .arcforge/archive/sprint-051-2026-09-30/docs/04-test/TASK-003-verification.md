# TASK-003 验证报告（美股代码白名单 Supported / toTicker）— 返工 R1 复验

- 验证者: test-tg-a
- 判定对象: feature/tiingo-source @ 46bc54a3456b8137597ebd8e4923d935f56932ef（= verify_baseline.head；主仓库 HEAD 同值，无漂移）
- 返工提交: 5ce608f6d700da70acd2699b577acd52d77e7364；`git diff --stat b6c6a25 46bc54a` 仅 symbols.go（3/3 行注释）与 symbols_test.go（+7/-1），均在 writes 内，无越界。正则 `^[A-Z]{1,5}([.-][A-C])?$` 未改。
- 运行环境: 隔离 worktree `git worktree add --detach <scratchpad>/test-tg-a-wt3 46bc54a3456b8137597ebd8e4923d935f56932ef`，GOTOOLCHAIN=local

## 覆盖矩阵（R1）

| # | done_criteria | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | AAPL/SPY/QQQ/GOOGL/BRK-B/BRK.B → true | TestSupported「美股/ETF」（未改动） | PASS |
| functional[1] | toTicker | TestToTicker（未改动） | PASS |
| boundary[0] | A 股/中证/港股 → false | TestSupported（未改动） | PASS |
| boundary[1] | 指数/期货/加密 → false | TestSupported（未改动；ETH 仍独家守卫市场合取，N11） | PASS |
| boundary[2] | 形态类 → false | TestSupported（未改动） | PASS |
| boundary[3] | P11 六条 + 7203/7203.A/9988 false + 基底 [A-Z0-9] 变异须转红 | TestSupported「P11」新增 3 条；N8 KILLED 于 7203、7203.A、9988 | PASS |
| non_functional[0] | go test ok；gofmt/vet 空；覆盖率 ≥80% | go test ok（TestSupported/TestToTicker PASS）；gofmt -l 空；vet rc=0；包 97.0% | PASS |

diff 只新增用例、改注释，上轮 30 条用例一条未删未改 ⇒ 其余 DoD 无回退（且全部通过）。

## 变异测试（scratchpad/test-tg-a-TASK-003-mut.py，同一套 13 个变异；每个 vet rc=0；还原后 sha256 一致，worktree `git status --porcelain` 0 行）

| 变异 | 结果 | 致红 |
|---|---|---|
| N1 后缀 [A-C]→[A-Z] | KILLED | BRK.D, HSBA.L |
| N2 基底 {1,6} / N3 {1,4} | KILLED | ABCDEF / GOOGL |
| N4 去 `^` / N5 去 `$` | KILLED | 多条 |
| N6 仅 `.` / N7 仅 `-` | KILLED | BRK-B / BRK.B, BF.B |
| **N8 基底 [A-Z0-9]** | **KILLED** | 7203, 7203.A, 9988 |
| N9 `[A-C]+` / N10 `(?i)` | KILLED | BRK.BB / aapl |
| N11 市场合取 `\|\| true`（本轮一次性用合法写法，vet rc=0） | KILLED | ETH |
| N12 正则合取恒真 | KILLED | 多条 |
| N13 toTicker 恒等 | KILLED | toTicker("BRK.B") |

13/13 KILLED，无存活变异。注释更正（symbols_test.go 关于 7203.T 已不区分基底规则）与 N8 实测一致。

## 结论

VERIFIED（R1）。

---

## 历史：首轮验证（@ b6c6a25740823b5e1b02f67e084bf6e7ad7f4f90，REJECTED task_defect）

## TASK-003 验证报告（美股代码白名单 Supported / toTicker）

- 验证者: test-tg-a
- 判定对象: feature/tiingo-source @ b6c6a25740823b5e1b02f67e084bf6e7ad7f4f90（= verify_baseline.head，无漂移）
- 交付提交: 1a1573f8bc0311149ef83caa709b233ef6f7d78d（仅 symbols.go / symbols_test.go，与 writes 一致；`git diff 1a1573f b6c6a25 -- <两文件>` 为空，合入内容与提交逐字节一致）
- 运行环境: 隔离 worktree（detach @ b6c6a25740823b5e1b02f67e084bf6e7ad7f4f90），GOTOOLCHAIN=local
- 正则按 AD-22：`^[A-Z]{1,5}([.-][A-C])?$`（偏离计划有 questions[0] 裁决为据）

### 覆盖矩阵

| # | done_criteria | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | AAPL/SPY/QQQ/GOOGL/BRK-B/BRK.B → true | TestSupported「美股/ETF」6 条（+BF.B） | PASS |
| functional[1] | toTicker BRK.B→BRK-B、AAPL 不变 | TestToTicker | PASS |
| boundary[0] | 600036.SH/000001.SZ/930713.CSI/0700.HK/03968.HK → false | TestSupported「A 股」「港股」 | PASS |
| boundary[1] | ^GSPC/^HSI/GC=F/BTC-USD/BTCUSDT/ETH → false | TestSupported「指数」「期货」「加密」 | PASS |
| boundary[2] | 空串/aapl/ABCDEF/TOOLONGTICKER1/BRK.BB → false | TestSupported「形态」 | PASS |
| boundary[3] | P11：SAP.DE/HSBA.L/RY.TO/**7203.T（纯数字基底）**/000300.SS/931151.CSI → false | TestSupported「P11」6 条字面存在且通过 | **FAIL（行为未守卫，见下）** |
| non_functional[0] | go test ok；gofmt/vet 空；覆盖率 ≥80% | go test ok；gofmt -l 空；vet rc=0；包 97.0%（symbols.go 两函数 100%） | PASS |

### 变异测试（scratchpad/test-tg-a-TASK-003-mut.py；每个变异还原后 sha256 一致，worktree `git status --porcelain` 0 行）

| 变异 | 结果 | 致红 |
|---|---|---|
| N1 后缀 [A-C]→[A-Z] | KILLED | BRK.D, HSBA.L |
| N2 基底 {1,5}→{1,6} | KILLED | ABCDEF |
| N3 基底 {1,5}→{1,4} | KILLED | GOOGL |
| N4 去 `^` / N5 去 `$` | KILLED | 多条 |
| N6 分隔符仅 `.` / N7 仅 `-` | KILLED | BRK-B / BRK.B, BF.B |
| **N8 基底 [A-Z]→[A-Z0-9]** | **SURVIVED** | 无 |
| N9 后缀 `[A-C]+` | KILLED | BRK.BB |
| N10 `(?i)` | KILLED | aapl |
| N11 市场合取恒真（`\|\| true`，保留 import；首版删 import 致编译失败属错误理由，已重做） | KILLED | ETH |
| N12 正则合取恒真 | KILLED | 多条 |
| N13 toTicker 恒等 | KILLED | toTicker("BRK.B") |

#### N8 非等价证明

探针（临时测试文件，跑完已删）在基线实现与 N8 下对比：

| 输入 | 原实现 | N8 | MarketForSymbol |
|---|---|---|---|
| 7203 | false | **true** | US |
| 7203.A | false | **true** | US |
| 1234.B | false | **true** | US |
| 3690 / 9988 / 00700 / 1A | false | **true** | US |

即「基底排除数字」这条规则（AD-19 人类裁决 P11；Leader 细化原话「7203.T 的 .T 本身是单字母，故基底须排除数字」）在实现里确实存在，但**测试集中没有任何一条用例守卫它**：删掉它，全部测试照绿，而数字基底代码会被放行、白白消耗 Tiingo 小时配额（正是 P11 要防的）。

成因：AD-22 把后缀收紧到 [A-C] 后，`7203.T` 被**后缀规则**拒（T ∉ A–C），不再经过基底规则。DoD boundary[3] 对它的标注「（纯数字基底）」表明这条用例的意图是守卫基底规则，AD-22 之后它失去了区分力。`symbols_test.go:52` 的注释「7203.T 的 '.T' 本身是单字母后缀，靠基底排除数字」在 AD-22 之后为假（N8 下 7203.T 仍为 false 即反证）。discovery 的变异清单也未覆盖基底字符集。

### 结论

REJECTED（reason_class=task_defect）。

修复方向（改动很小）：
1. TestSupported 补至少一条「数字基底 + 合法后缀/无后缀」的 false 用例，使 N8 转红，例如 `7203`、`7203.A` 或 `1234.B`（MarketForSymbol 对它们均判 US，只能由基底规则拒）。
2. 更正 `symbols_test.go:52` 注释：7203.T 在 AD-22 后同时被后缀规则拒，基底规则由新补用例守卫。
3. 在隔离副本上复跑 N8（`[A-Z]`→`[A-Z0-9]`）证明其 KILLED，写进 discovery。

其余 6 条 DoD 行为与测试质量无问题（12/13 变异 KILLED，唯一存活即上述 N8）。
