# Sprint 036 交付报告 — Hestia M1b-4a：discover + fetch + config

**需求**：`hestia/docs/superpowers/plans/2026-08-12-hestia-discover.md`（2143 行）
**目标包**：`internal/hestia` · **调度**：`dag` · **基线**：`f5a17d5`（92.1%）→ **HEAD `c101d6125d76ce1a8863342072a703c4c206d002`（93.2%）**

---

## 一、交付摘要

**7 个任务全部 `accepted`，零返工、零 rejected、零 blocked。**

| 任务 | 交付 | 合并点 |
|---|---|---|
| TASK-001 | `Fetcher` 接口与绕代理的 PBOC client | `bf0ddf1` |
| TASK-006 | 口径豁免的键补上 `PeriodTypes` | `0601b63` / `cfcbdbb` |
| TASK-002 | 分页模板解析与 index 页快照 | `99c8c6a` / `0597fca` |
| TASK-003 | 从 index 页提取报告条目（标题正则、期次映射） | `7576ad3` |
| TASK-004 | `Store.HasPeriod` 与 `PeriodChecker` 接口 | `2b93ccd` |
| TASK-005 | `Discover` 主循环，翻页直到找到未入库的期次 | `7b49b13` |
| TASK-007 | `LoadConfig` 与 Sprint 036 契约 | `c101d61` |

**最终数字（在最后一次改动之后统一重采）**：

```
覆盖率  92.1% → 93.2%      discover.go 六个函数 + LoadConfig/validate 全部 100.0%
测试    271 顶层 / 577 含子测试 / 0 FAIL
-race   绿      gofmt 0 项      go vet 0 行
变异    93 条 / 90 KILLED / 3 SURVIVED（三条存活全部经独立验证，非「大概等价」）
```

**越权审计（全绿，计数自洽）**：`leader` 只写 `pending→assigned`(7) 与 `dev_done→verifying`(7)；
`dev-49/50/51` 只写 dev 边（2+4+1=7 各）；`test-agent-25` 只写 `verifying→verified`(7)。
`.claude/` 运行时资产零改动，worktree 零残留。

---

## 二、Code Review 两轮

产物：`docs/05-review/code-review-2026-08-12-round1.md`（reviewer 位，229 行）
　　　`docs/05-review/code-review-2026-08-12-round2-consumer.md`（消费者位，343 行）

| 轮次 | 位置 | 结论 | CRITICAL | 其他 |
|---|---|---|---|---|
| 1 | 常规 reviewer | PASS | 0 | 2 MAJOR + 5 MINOR |
| 2 | **M1b-4b 消费者位** | **CONTESTED** | **2** | 6 WARNING + 5 SUGGESTION |

**Leader 裁决：CONTESTED，不走 `review_fix`，全部结转 + 三件呈报人类。**

理由：两条 CRITICAL **都不是任何任务的 DoD 违反** —— 季报从来不在本 Sprint 任何 DoD 里
（`validPeriodTypes` 从 M1b 设计之初就只有 `{monthly,h1,annual}`；需求文档唯一提「季度」处是把
`2026年二季度金融机构贷款投向统计报告` 列为**要拒绝的干扰项**）。拿 DoD 没写的标准判 dev 不合规是越界。
且现在修会打破 `verify_baseline`（T5/T7 已 `verified`，基线钉在 `c101d61`）。

### 第二轮的头条（Leader 已独立复现）

**按当前交付，管线每年只能真正入库 2 篇报告。** 央行年度节律是 12 篇：

| | 现状 | 后果 |
|---|---|---|
| 上半年 + 年报（2/12） | ✅ 正常 | — |
| **月报（8/12）** | discover 收下 → `Parse` 恒拒 | **永久重试循环** |
| **季报（2/12）** | 两侧文法都不认 | **静默消失，零报错** |

**而这一切不会让任何测试变红** —— 仓库里两份 index 快照恰好只含 h1 一种形态，
**从任何一个任务内部、任何一份文档里都看不见**。两条 CRITICAL 全部来自消费者位**自己决定去线上普查 22 页**那一步。

---

## 三、人类定案（2026-08-12）— M1b-4b 的强制输入

| # | 事项 | 决定 |
|---|---|---|
| ① | **季报支持** | **做**，在 4b 之前。需动四处：`reportTitleRE` / `validPeriodTypes` / `periodEndMonth`(q1→03,q3→09) / **`cumulativePeriods`（唯一硬卡点）**。**必须新开任务**，不是 `review_fix` |
| ② | **月报「发现了却抓不了」** | **4b 跳过并记日志，不中止本轮**（月报总是最新那条，中止则永远走不到 h1/年报）。**必须配合重试记账**，走只读 `DB()` 查 pending 计数，否则 6h 一跑 ≈1460 行/年灌满诊断面 |
| ③ | **修订版** | **暂不支持，明确写进契约**，注明「Store 侧双时态设计当前无生产者」，`refreshArticleID` 正常路径永不触发一并写明。⚠️ 消费者位的限定必须一并记入：**只证明了「若发生则管线看不见」，没有证据说明它发生过** |

**4b 自己就能闭合的四件**（消费者位已给可落地写法，建议全进 4b 的 DoD）：
倒序处理候选（否则漂移检测一次都不真正执行且零告警）、pending 重试记账、
候选期次 vs 正文期次交叉核对（否则静默永久循环）、包一层限速 Fetcher。

**配置侧未定案**：db 路径进 `Config` 还是走 flag；`configs/hestia.yaml` 现在不存在且不在任何 `writes` 里，需要认领；`timeout: 30` → 30ns 且过校验，建议加下界 ≥1s。

详见 `docs/02-plan/findings-carryover.md` 的「🔴 人类定案」一节。

---

## 四、机制问题（附录 A，供人类判断是否落地）

**全部属「agent 不能自改运行时资产」范畴。M1–M3 的共同形状：错了也不会有信号回来。**

| # | 问题 | 证据 | 候选处置 |
|---|---|---|---|
| M1 | 派验通知丢失 | 本 Sprint **3 次**全靠 verifier 的 idle hook + 重扫兜住，**无一次是收到通知发现的** | — |
| M2 | `TeammateIdle` 保活一个**无权行动**的人 | 任务停在 `dev_done` 时 hook 持续唤醒 verifier，而该边是 leader 专属 ⇒ **它无法自行解除保活条件**；文件层面完全正常，validator 抓不到 | 保活条件加一层「被唤醒者有无推进权限」 |
| M3 | 交棒窗口窄于消息延迟 | 实测 `dev_done`(04:45:25) → 派验(04:48:20) = **2 分 55 秒**，五条消息在窗口关闭后才到 | dev 提议 `verifying` 下允许 `test-*` 写 `done_criteria`。⚠️ **反面：让验证者写自己的验收标准有自证风险** |
| M4 | `go doc` 缺陷无任何自动门禁 | 注释与 `func` 间一个空行 ⇒ `go doc` 只输出签名，`gofmt` **与** `go vet` **双双不响** | 加 AST 守卫。**限定：必须限定在 `FuncDecl` 并把理由写进注释**（扩到 type/var/const 会红 51 项 ⇒ 下一个人会放宽断言） |
| M5 | discovery 内部矛盾无人比对 | dev 的 `key_findings` 直接证伪同一文件的 `decisions`，隔几行，三次经过未见 | 写通道落盘时**并排打印**两个字段 |

---

## 五、方法论：位置对照实验**失败**（如实记录）

设计：两个 QA 位拿**完全相同**的背景材料，唯一变量是职责描述，验证「跨视角对抗必须换位置而非换人」。

**失败原因：两位都没读全材料，而 Leader 从未核实过 —— 只是写了「请全部读」，然后当成它成立了。**

| | qa-reviewer | qa-consumer |
|---|---|---|
| findings（G1–G35） | 全文 | 约 2/3 |
| 七份验证报告 | **0** | **0** |
| 七份 discovery | 0（仅 grep 片段） | 仅 TASK-003 一段 |
| 七份 DoD | **0** | **0** |
| CONTRACTS | 两段 | 约 100/543 行 |
| **额外读的** | 代码探针 | **源码读了个遍 + 线上普查 22 页** |

**最终结论（qa-consumer 自己给的收窄，Leader 原样采纳，不写「获支持」也不写「打折」）**：

> **「消费者位 + 直接跑源码与线上语料」这个组合，产出了 8 条与全部预置提示无关的发现。**
> **「位置」单独的贡献，本次实验分不出来** —— 因为两位的材料基线都不是设计的那一份。

**一个副产品**：Leader 精心准备的五份材料包，对消费者位的实际产出贡献是
**验证报告 0 / DoD 0 / dev 交代的三处 0**。⇒ 下个 Sprint 该给这个位置的不是文档，
而是「**把下一步真的写一遍**」这个指令 + 源码 + 网络访问。

⚠️ **但下次必须先做阴性对照**（qa-consumer 提）：**同样位置、同样指令、不给网络访问** ——
否则「网络访问」与「消费者位」还是分不开，**和这次「位置」与「材料」分不开是同一个形状**。
**从一个混杂实验里推出的改进方案，本身可能就是下一个混杂实验的设计。**

---

## 六、结转知识

`docs/02-plan/findings-carryover.md` 累计 **G1–G43 + 附录 A + 人类定案**（约 2100 行）。
其中反复出现、值得下个 Sprint 直接用的几条：

- **G31/G39**：聚合信号（KILLED / 红 / PASS / 退出码）**不携带因果**，必须下钻到「是哪一条产生的」。
  **自证数字与自证判据本身不受任何自动门禁检查** —— 三人各踩一次，无一被机制挡下。
- **G33**：**DoD 的定稿时点早于「验收标准被想清楚」的时点**，这是结构性的、不可能靠更认真消除。
  新验收要求**在写第一条消息之前**先问「它现在能写进谁的 DoD」。
- **G35**：判据订正为「**错了的话，什么东西会告诉我？**」（原措辞在问后果大小，实际要问的是反馈回路存不存在）。
- **G43**：**别把机制效应写成个人品质** —— 三次由三个不同的人更正。判据：
  「换一个人来，什么东西会强迫他也这么做？」答不出来的不能写进方法论。

---

## 七、致谢与人员

`dev-agent-49`（T1/T6）、`dev-agent-51`（T4）、`dev-agent-50`（T2/T3/T5/T7，承担最多）、
`test-agent-25`（全部 7 份验证 + 93 条变异）、`qa-reviewer`、`qa-consumer`。

**本 Sprint 的多数发现来自 teammate 主动上报自己的问题，而非被他人挑出。**
按 G37 的框架：那不是美德，是**唯一能让这类问题进入视野的通道**。

---

### 待同步 hooks 清单（人类执行）

| 文件 | 变更摘要 | 同步命令 |
| --- | --- | --- |
| （无） | **本 Sprint 未改动任何 `project-template/hooks/`、`project-template/scripts/` 或 `templates/CLAUDE.md.template`** | — |

**核实依据**：`git log f5a17d5..HEAD -- project-template/ .claude/` 输出为空；
`git status --porcelain project-template/ .claude/` 输出为空。

⇒ **无需同步，也无需运行 `check-runtime-sync.sh`。**

⚠️ **但上面第四节的 M1–M5 是本 Sprint 新提出的机制问题，它们尚未变成任何 `project-template/` 改动** ——
需要人类决定是否落地。**「清单为空」只说明本 Sprint 没改运行时资产，不说明没有待办。**
