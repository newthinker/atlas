# TASK-006 复验报告(rework 1)— QA D1-D4 返工

验证者: test-agent-15 | 日期: 2026-08-02 | epoch=1 | 判定: **VERIFIED (PASS)**

首轮报告见 `TASK-006-verification.md`(5 条 DoD 全过,产物已提交于 **356d07c**)。
本轮为**定向复验**:QA 的 D1-D4 四条返工项是否落实,以及有无引入新的不准确表述。
首轮结论不重复,仅复核未被破坏。

## 一、fix_items 逐条核对

| # | 严重度 | 要求 | 落实位置与实证 | 判定 |
|---|---|---|---|---|
| D1 | MAJOR | 40203 措辞由「行为错误」降为「文案误导」;TASK-001 修复后同步状态 | design §10 该条重写 + 计划文档验收记录同步改写 | PASS |
| D2 | MAJOR | 容量边界入 spec §2 表 + design §9/§10 | 三处齐,§10 为新增独立风险条 | PASS |
| D3 | MINOR | 「三跳三故障域」措辞降级,不本轮实现 | spec §2 行情行改写 + design §9 新增 ❌ 行 | PASS |
| D4 | MINOR | 后续工作登记**一个**(不是两个)任务 | design §9 新增「M3.5 后续工作」节,恰 1 个任务含三面 | PASS |

### D1 —— 措辞降级(PASS)

**独立核实 QA 的技术依据(不采信 dev 陈述)**:`grep ErrNoPermission --include=*.go`
显示全仓**唯一非测试消费点**在 `internal/prism/refresh.go`;读该分支确认
`errors.Is(...)` 命中与否**都落到同一个 `return degs, err`**,命中分支只多 `append`
一条 Degraded 文案。⇒ **误分类不改变任何执行路径**,QA 的 D1 判定成立,
dev 首轮「而不重试」的写法确属夸大。

**改写后的表述准确**:§10 现写「**影响限于运维文案,不改变执行路径**——`ErrNoPermission`
全仓唯一消费点只用于拼 Degraded 文案;该跳本就只调用一次、永不重试,故误分类**不会**
让本该重试的调用被跳过。后果是运维会去误查积分档,实际只需等限频窗口」,并保留了
复现证据与判别线索。措辞与代码事实一致。

**旧夸大措辞已彻底清除**(全 docs/ 目录 grep,命中数均为 0):

| 旧措辞 | 残留 |
|---|---|
| 「应往下跳」 | 0 |
| 「与真实原因完全相反」 | 0 |
| 「生产危害」 | 0 |
| 「判成永久配置问题」 | 0 |

**dev 主动扩展的一致性修复值得肯定**:fix_items 只点名 design §10,但 dev 发现计划文档
验收记录根因第 1 条有同样措辞,一并改写(discovery `decisions[3]` 申报)。理由成立——
两份文档对同一事实给出不同严重度,下次读者无从判断以哪份为准。

**「修复进行中」状态属实**:`ErrRateLimited` 确已存在于
`internal/collector/tushare/client.go`(按 msg 含「频率超限」判别),有对应单测
`TestErrRateLimited`。TASK-001 已 verified,故「TASK-001 新增 ErrRateLimited」为真。

### D2 —— 容量边界入文档(PASS)

Leader 要求「spec §2 表加注记 + design §9/§10 同步」,dev 落到**三处**:

1. **spec §2 表「A股公司·估值」行**:加容量边界注记(1 次/分钟 × 串行遍历 ⇒ 只有第一个
   A 股标的能兜底;窗口自升级 1 次/小时;覆盖率约 1/3)。
2. **design §9 tushare 行**:状态由「✅ 已落地」改为「✅ 已落地,**兜底容量受限**」并附同款数据。
3. **design §10 新增独立风险条**「tushare 兜底容量边界」,与既有「tushare 积分边界」条并列。

dev 在 `decisions[1]` 说明了不并入既有条的理由:积分边界回答「有没有权限」,容量边界回答
「有权限也不够用」,合并会把「覆盖率仅 1/3」这个本次演练最有价值的结论埋进权限讨论里。
**该判断成立**,两条风险的运维动作确实不同(前者充积分,后者改架构/退避)。

**数字经独立查库核实**:`SELECT type, COUNT(*) FROM instrument WHERE market='CN_A'`
→ `index 2 / stock 3`。池内 **3 个 A 股股票**标的(指数不走 daily_basic),
故「3 标的 ⇒ 覆盖率约 1/3」**准确**。

### D3 —— ADR#10 措辞降级(PASS)

**独立核实路由层现状(dev 与 QA 的技术描述是否属实)**:

| 核查项 | 实测 |
|---|---|
| `Registry.collectors` 字段类型 | `map[string]Collector`(`internal/collector/registry.go:8`) |
| `GetAll()` 实现 | `for _, c := range r.collectors` ⇒ **map 遍历,Go 不保证顺序** |
| `orderedCollectors` | `internal/app/app.go:556`,取 `GetAll()` 后把 `SelectForSymbol` 的结果提到首位,**其余保持 map 序** |
| `SelectExternalForSymbol` | 全部走硬编码 `reg.Get("eastmoney"/"yahoo"/"crypto")` + symbol 形状判定,**从不调用 `SupportedMarkets()`**;末尾兜底 `for _, c := range reg.GetAll()` 直接返回首个(亦为 map 序) |
| `internal/app/` 内 `SupportedMarkets()` 调用 | **0 处**(仅测试 mock 实现该方法) |

⇒ 「顺序不确定 + 完全不读 `SupportedMarkets()`」**属实**,声明 `CN_A` 的 baostock 确有
被拿去跑美股 symbol 的可能。D3 的技术判断准确,措辞降级有事实基础。

**改写到位**:spec §2「A股·行情」行由「三跳三故障域」改为「**三源已登记为备源;跳序与
市场过滤未实现**(见后续任务)」并附机理说明;design §9 新增独立行
「A 股行情链的**跳序与市场过滤** | ❌ **未实现**(措辞更正)」。
**「三跳三故障域」在全 docs/ 目录残留 0 处**。

### D4 —— 后续任务登记(PASS)

design §9 新增「M3.5 后续工作(M3.5a 验收暴露,尚未立计划)」节,登记
**恰好 1 个**任务(grep 顶层条目计数 = 1):「限频感知退避」,涵盖三面——
① 40203 分类的消费(退避重试 + 文案说真话)② A 股批量断源容量改善
③ 路由层跳序与市场过滤。严格符合 Leader「登记一个(不是两个)」的指示,
且把 D3 的路由层工作并入而非另立,符合 `decisions[2]` 的理由(三面共享同一动机)。

---

## 二、⚠ 关键发现:一处文档陈述已被并发在途任务改成过时(**不构成本任务缺陷**)

design §9 后续工作第 1 面写「TASK-001 已新增 `ErrRateLimited` …**但分类目前无人消费**」。
我在核实 D1 时发现 `refresh.go` 现已有
`case errors.Is(err, tushare.ErrRateLimited):` 分支——**该分类已被消费**
(用于分叉 Degraded 文案:「tushare 限频,本次跳过,下次自动重试」)。

**这不是 dev 写错,而是并发在途任务在我验证期间落地。**时间线(UTC,取自
`transitions.jsonl` 与文件 mtime):

| 时刻 | 事件 |
|---|---|
| 07:47:30 – 07:50:19 | dev-agent-34 写三份返工文档(design 文档 07:50:19) |
| 07:53:49 | TASK-006 → `dev_done` |
| 07:54:50 | TASK-006 → `verifying`(交我) |
| 07:54:59 | TASK-005 `blocked_clarification` → `in_progress`(dev-agent-32) |
| **07:56:27** | **`internal/prism/refresh.go` 被修改**——新增 ErrRateLimited 消费分支 |

⇒ dev-agent-34 在 07:50:19 写下「分类目前无人消费」时**该陈述为真**;
TASK-005 在 **6 分钟后、且在 TASK-006 已交付验证之后**才落地消费。

**判定影响**:不作为 reject 依据。理由有三——
1. 陈述在写下与交付时均为真,时间线可证;
2. 变更来自**另一个仍处 `in_progress` 的任务**(TASK-005,rework=1),其改动**未提交**,
   可能再变或被退回。此刻要求 TASK-006 改文档是追移动靶,若 TASK-005 被退回反而写错;
3. 该文档的**另一处措辞已前向兼容**:§10 写的是「分类**被消费**(限频触发退避而非放弃)
   属后续任务」——即便消费已落地,「退避」仍未实现,§10 无需改。

**建议**(交 Leader 决策,非本任务返工项):TASK-005 验收后,把 §9 后续工作第 1 面的
「分类目前无人消费」改为「分类已用于文案分叉,但**尚未据此退避重试**」一行即可。
需要强调:「需触发退避重试」这半句**至今仍然成立**——新代码只分叉文案,未实现任何退避。

---

## 三、回归与范围核对

**首轮已验产物未被破坏**:本轮 `git diff` 只触及三份文档;首轮验过的
`deploy/launchd/com.newthinker.atlas.baostock.plist`、`scripts/ops/{deploy.sh,install-services.sh}`、
`docs/deployment.md` 已随 **356d07c** 提交且本轮**未再改动**(discovery `files_modified`
末条自陈,`git diff` 印证)。首轮的 5 条 DoD 结论继续成立。

**越界申报**:本轮改动三份文档——
`docs/prism/atlas_prism_design.md`、`docs/superpowers/plans/2026-08-02-prism-m3.5a-datasources.md`
均在原 `writes` 内;`docs/superpowers/specs/2026-08-02-prism-m3.5-design.md` 在
`packages` 外,**已补进 `writes`(现为第 7 项)**,且 discovery `files_modified` 显式标注
「该文件在 packages 外,已补进 writes」。承接时我看到的声明已覆盖全部实际改动,
**符合「dev_done 前自行补报」的要求**。

工作区其余变更(`internal/collector/tushare/`、`internal/collector/twelvedata/`、
`internal/prism/refresh.go`、`.arcforge/` 状态文件等)分属 TASK-001/002/005 与
Leader,**不计入本任务**。未跟踪目录 `docs/collector/` 时间戳 2026-07-26,早于本 sprint。

**密钥哨兵(亲跑)**:两个 key 前 8 位在 `docs/` 全目录、`git diff`、`git diff --cached`、
`git log -p --all -S`(全历史全分支)命中**全为 0**;反向对照 `configs/config.yaml`
命中 **2** 次,证明检索有效。

---

## 四、观察(非阻断,不影响判定)

1. §10 与计划文档引用 `ErrNoPermission` 消费点为 `refresh.go:450`,而当前文件中
   `errors.Is` 实际在 **448 行**(且该处已被 TASK-005 改成 `switch`/`case` 结构,行号
   会继续漂)。行号级引用在并发改动下天然易腐,**建议后续引用改为函数名**
   (`refreshTushareFallback` 的错误分支)而非行号。不影响结论正确性。
2. design §9 的 twelvedata 行仍写「NVDA 2026-07-16..07-31 共 **11** 个交易日」。我在
   TASK-002 复验中查库确认:该闭区间实际有 **12** 个交易日(末日排他才是 11)。该行是
   首轮产物、本轮未触及,且描述的是首轮那次比对的实际覆盖范围,**不算错**,但区间标注
   与条数不自洽。可在下次改该表时顺手校准为「[07-16, 07-30] 共 11 个交易日」。

---

## 五、判定

D1-D4 **四条逐条落实**,且每条的技术依据我都独立核实过而非采信 dev 陈述:
D1 的「不改变执行路径」经读码确认(两分支同一 return)、D2 的「3 标的 ⇒ 1/3」经查库确认、
D3 的「map 序 + 不读 SupportedMarkets」经读码确认、D4 的任务数经计数确认。
旧夸大措辞与「三跳三故障域」在全 docs/ **零残留**。首轮产物未被破坏,越界已合规补报,
密钥哨兵全历史 0 命中。

唯一的不准确表述(§9「分类目前无人消费」)经时间线证明系**并发在途任务 TASK-005 在本任务
交付后才落地**所致,不归责本任务,已在上文记录并给出 Leader 决策建议。

→ **VERIFIED**
