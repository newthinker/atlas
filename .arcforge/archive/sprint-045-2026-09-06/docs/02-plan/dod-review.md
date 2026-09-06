# 独立 DoD 反审 · Sprint M2a（契约队列与信号快照）

**reviewer**：Agent tool 只读子代理（未命名，非 teammate）；先只读需求与 spec 写出自己的验收要点并在 `d27791c` 上核实前提（56 次工具调用，含 `go test`、`sqlite3 -readonly`），再读 7 个任务文件与 AD-1～13 比对
**判定**：NEEDS WORK —— 8 条阻断 + 8 条建议 + AD-4b 独立意见
**Leader 处置**：每条打开它给的文件:行号核实后**全部采纳**；DoD 已经写通道 `update` 落盘（7 条审计行，`transitions.jsonl` 2026-09-05T12:24Z），validator 重跑通过。处置表见 `01-design/architecture-decisions.md` AD-15；新增裁决 AD-14（OutOfOrder）。

## 阻断（8，全部核实成立）

| # | 任务·维度 | 问题 | 修法（已落盘） | Leader 核实 |
|---|---|---|---|---|
| B1 | 001 f[0] / AD-10 | `DefaultSignals` 是导出函数，AST 守卫精确集合 ⇒ 001 合入即红；`writes` 无 `store_test.go` | 登记 `"DefaultSignals"`（26）；`writes` 加 `store_test.go` | `store_test.go:433-435` `Recv==nil` 直接入 `got` |
| B2 | 003 f[0] | `articleURL` 与既有测试 helper `ingest_test.go:82` 同名同签名 ⇒ `redeclared` | 生产函数改名 `pbocArticleURL` | grep：19 处调用点 |
| B3 | 003 f[0] | 需求 `contract_test.go` import `context` 无使用 | 去掉；「import 按需增删」 | 需求 import 块 vs 五条用例 |
| B4 | 003 f[2] / AD-10 | `Contract.JSON`/`Contract.FileName` 进导出面 ⇒ 「29 项」必红 | 登记两者（32）；终值 34/14 | `store_test.go:437-441` `ast.IsExported(recv)` |
| B5 | 001 f[1] | `config_test.go:387` 钉 `"2026-09-03"` | DoD 明写改 `"2026-09-05"` | 打开 `:387` |
| B6 | 006 f[1] | 直传 `hestiaContractEmitCmd` ⇒ `Context()` nil ⇒ `database/sql` panic | 用例改 `newCapturingCmd()` | `hestia_test.go:131-139` `SetContext`；cobra v1.10.2 |
| B7 | 001 f[2] / AD-5 | AD-5 说的 `writeHestiaYAMLWithDB` 在 `hestia_test.go` **零处**使用；真入库用例是 `:1225`/`:1342` 内联 yaml | 改到那两处；AD-5 订正并保留错误原文 | `grep -c writeHestiaYAML hestia_test.go` = 0 |
| B8 | 001/005 判据 | `git status --short` 对空目录恒空 | 改 `find … -type d` | git 语义 |

## 建议（8，全部采纳）

| # | 任务 | 缺口 | 处置 |
|---|---|---|---|
| S1 | 005 | `ingest_test.go` 无 `encoding/json` import | DoD 点名补 |
| S2 | 005 / spec | OutOfOrder 满足 `!= Duplicate` ⇒ 旧数据覆盖 `pending/` | **AD-14**：只在 New/Revision 写契约；新增测试；007 A9 |
| S3 | 006 | `SilenceUsage`/`IsRegistered` 守卫不含新命令 | 列表加两者 |
| S4 | 005 | `writeAtomic` 文案 `snapshot write/rename` 进错误串 | 接受不改；007 §A 记 |
| S5 | 007 | 回归只查仓库根 `queue/` | 加 `find` |
| S6 | 002 | golden 溯源可直接只读查 `data/hestia.db` | 改用 `sqlite3 -readonly`（reviewer 已核逐值相等） |
| S7 | 006 | `--period-type` 错误串未定 | 定格式 |
| S8 | 002/004/007 | 守卫计数顺延 | 27 / 34 / §B 34 |

## AD-4b 独立意见

reviewer 独立选**方案一**（记 `ingested` + `stage=contract` + P1 照发），理由：spec §3.3/§6 明写 `outcome=ingested`；`health.go:30` 的 `LastIngest` 按 `RunIngested` 取，记 `failed` 会让健康度说假话；P1 措辞是「处理失败」不构成 D6 要防的误导；方案三只剩退出码与 err.log。附带：`Notified==true` 断言依赖 P1 发送成功（已写进 005 f[2]）。**最终仍由人类在 dod-gate 拍板。**

## 备注（reviewer 核实成立的前提）

覆盖率基线 96.6 / 76.4；`fieldOrder` 76；AD-2/3/4a/8 前提成立；需求引用的全部既有符号存在；`store.go`/`hestia.go` 新代码不需新 import；`backfill_load.go` 不调 `Ingest`；三期 golden 与 `data/hestia.db` 逐值相等。
