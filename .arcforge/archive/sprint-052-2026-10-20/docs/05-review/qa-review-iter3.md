# QA Review 迭代 3/3（最后一轮）— pricewatch sprint

- 审查者：qa-pw ｜ 日期：2026-10-20
- 对象：`git diff 8ed58bc17d3179e32c56b9d5caf9199484dcc32e..bc33f71827179242e06d2e74129b0f603bd9f699`（TASK-005：f9920a2、0cac171 及两个 merge；message.go +4/−3、message_test.go +93/−4）
- 取证环境：隔离 worktree detached @ bc33f71827179242e06d2e74129b0f603bd9f699；`GOTOOLCHAIN=local go test -race ./internal/pricewatch/ ./cmd/atlas/` 两包 ok；探针/变异全部经 `-overlay`，worktree `status --porcelain` 每步核对为 0 行，主工作区未改动
- 审查组成（中等规模）：Skeptic + Architect（Claude 只读子代理，结论落盘 scratchpad，我从文件取）+ codex 跨模型（源码内联经 stdin，返回「None」）

## 1. N1 闭合证据

| 层次 | 证据 | 结论 |
|---|---|---|
| 实现 | message.go:225 `FormatHealth` 返回 `capUTF16(s)`；标题在 detail 之前，截尾即保留标题 | ✔ |
| **系统级复现探针（迭代 2 原样重跑）** | 迭代 2 用的探针（binance 走真实 HTTP 客户端连 `127.0.0.1:1`，得到真实连接拒绝错误文本；okx 成功 0 根；冷启动 1d `Sync` → `FormatHealth`）：8ed58bc 上为 **5229**，bc33f71 上为 **4096** | ✔ 闭合 |
| 测试守卫（我的变异） | 去掉 FormatHealth 的 capUTF16 ⇒ `TestFormatHealth_Limit`、`TestFormatHealth_Boundary` 红；capUTF16 比较改 `<= max+1` ⇒ `TestFormatHealth_Boundary` 红；去掉 FormatHeartbeat 的 capUTF16 ⇒ `TestFormatHeartbeat_Limit` 红 | ✔ 已守住 |
| Skeptic 变异（独立） | 比较改 `< max` 被杀；预算 −2、+1、去掉截断标记均被杀；**只有预算 −1 存活**（见 L1） | ✔ |
| 结构性封顶（Architect grep） | pricewatch 生产代码的 SendText 调用点只有 3 处：engine.go:156 ← `FormatAlert`；engine.go:300 ← `FormatHealth`；runner.go:102 ← `FormatHeartbeat`；三者所有出口都经 capUTF16，调用方之后都不再拼接文本 ⇒ 「因长度被 Telegram 400、AD-30/heartbeat_date 下永久重试必败」这一类失效**在结构上消除** | ✔ |
| 非法 UTF-8 与线上计数（Skeptic 实测） | 输入含孤立 `\xff`、不完整序列、过长编码，每种再加 0–3 字节前缀错位；用 `json.Marshal` 后再 `Unmarshal`（SendText 无 parse_mode，走的正是 json.Marshal）模拟线上长度。各情形下 `utf16Len` 都等于模拟的线上长度，最大 4096；截断结果始终是原文的字节前缀，不劈代理对 | ✔ |

## 2. 心跳截断是否合理（Leader 追加项）
**合理，采纳。**
- 修复前心跳超长 ⇒ Telegram 400 ⇒ 不写 `heartbeat_date` ⇒ 每个 tick 都失败且必然失败，与 N1 同构（Architect 核对 runner.go:99-108）。这种情况下用户收不到心跳，会把它误判为进程已死；截断后首行「💓 监控仍在运行」始终保留，心跳的核心用途（证明进程活着）完好。
- 触发阈值（Skeptic 实测，资产名 3 个汉字）：每个资产触发 0 条规则时 91 个资产才截断，2 条时 56 个，7 条（spec 示例策略全部触发）时 29 个。spec 示例配置只有 2 个资产，个人使用很难到这个规模。
- 已知代价（Leader 已列明）：超长时末行「过去 24 小时发出提醒 N 条」最先被截掉，最后一个资产的价格可能被截成半截数字。记为 L2，不阻断。

## 3. 新发现（全部 LOW，均可记入已知限制）

| # | 级别 | 位置 | 描述与证据 | 建议 | 归属 |
|---|---|---|---|---|---|
| L1 | LOW | message_test.go `TestFormatHealth_Boundary` | 截断预算 −1 的变异存活（Skeptic 实测）：新测试只断言 n ≤ 4096，或 n ≥ 4094（给代理对留的余量）。该变异只会少用 1 个码元，方向保守、无害；与复验报告中「HB4 心跳自写 4095 存活」同类。测试注释称守住偏移，实际只守住了往超限方向的偏移 | 4097 用例输入全为 ASCII，可直接断言 `n == 4096`（一行） | TASK-005 |
| L2 | LOW | message.go:208-212 | 心跳超长时最先截掉汇总行，可能在行中间截断 | 汇总行移到首行之后、资产行之前；或按整行截断并写「另有 N 个资产未列出」 | TASK-005 |
| L3 | LOW | engine.go:22（Sender 接口）/ message.go 三个 Format* | 长度保证建立在「每个调用点都恰好经过 Format*」的约定上，不由类型保证；以后若直接 `SendText(err.Error())` 会绕开上限 | 在 Engine 里包一个统一的 `send()` 再兜底一次 capUTF16（幂等），或加一条测试断言 SendText 调用点集合 | TASK-007 |
| L4 | LOW | architecture-decisions.md AD-31 | AD-31 只记录了 FormatAlert；本轮把兜底扩到了健康和心跳消息，代码注释引用的是 N1，不是 AD 编号 | 在 AD-31 追加一句「同样适用于健康/心跳消息」 | Leader（文档） |

无 CRITICAL / WARNING。codex 跨模型对本轮 diff 的结论是「None」。

## 4. 总判定

**VERDICT: PASS**

- **必须修才能交付**：**无**。迭代 1、2 的全部必修项（QA-8、QA-9/AD-29、QA-14、R1-1/AD-30、R1-2、R1-5、QA-6/AD-31、N1）均已闭合，各有变异或探针证据：迭代 1、2 的证据见 `qa-review-iter2.md` §1，N1 的证据见本报告 §1。
- **可记入已知限制**：本轮 L1–L4；以及迭代 2 已定的 N2、N3、N4、QA-16、QA-17、迟到 Ack 跨周期误报、backfill 遇 Sync 错误直接 return；以及第 1 轮 Leader 已列入 final-report 的各项（QA-3、QA-5 测试半边、QA-7、QA-12、QA-13、QA-15、R1-4、R1-6、R2-1、Minimalist F1/F3、AD-15）。
- **非代码事项**：TASK-007 的 rework_count 少计 1 次（收回边改派绕开自动 +1），属机制问题，记入 final-report / PENDING-MECHANISMS。
