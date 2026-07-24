# TASK-008 复验报告 (review_fix / QA WARNING-1) — 拆股归一化

- **验证者**: test-agent-4 (Reality Checker)
- **任务状态**: verifying(epoch=1, rework 1/3) → **VERIFIED**
- **commit**: fdb5664（在首验 a59d2af 基础上修 QA WARNING-1）
- **判定**: **PASS** — 同比例多次拆股按生效日聚簇支持正确;10:1 无退化;NVDA 无回归
- 说明:首验 7 条 DoD 此前已 VERIFIED,本节聚焦 WARNING-1 修复。

## QA WARNING-1 修复核查

**问题**:detectSplits 按 ratio 建键(effByRatio),两次同比例拆股(如两次 2:1)被合并为一个事件、丢失第二次 → pre-both 条目只 ÷2 而非 ÷4。
**修复**:collectEffObs 收集每比例的生效日观测列表 → clusterEvents 按时间间隔(splitClusterGap=365天)聚簇为独立事件 → splitFactor 累乘。

### 1. TestFetchCompanyFactsRepeatedSameRatioSplit（关键防「只检测一次」断言）
手工追踪 double_split.json:annual EPS(end 2020-01-25) 三条 4.0/2.0/1.0,两相邻对各给 ratio-2 一票 → votes[2]=2 触发;观测 [2021-02-20, 2024-02-20] 相隔 3 年 >365 → clusterEvents 分成**两个** ratio-2 事件。Q1 单季 {8.0, filed 2019-05-28} 早于两事件 → factor=2×2=4:
- `assert.InDelta(2.0, q1.EPSDiluted)` = 8.0÷4，**明确非 ÷2=4.0**——强区分「两次 vs 一次」,若修复失败(只 1 事件)则 =4.0 断言失败。✓
- `assert.InDelta(4000, DilutedShares)` = 1000×4 ✓;NetIncome 5000 绝对值不变 ✓
- 亲跑 PASS。

### 2. clusterEvents 语义 + 已知限制（item 2）
- 同一次拆股多观测(间隔<1 年)合并取最早生效日;>1 年分属独立事件 → 代码审读正确,double_split 实证(3 年间隔分两事件)。
- **反例/已知限制**:两次**真实**同比例拆股相隔 <365 天(罕见但存在)会被误合并为一事件 → 欠归一(pre-both 仅 ÷ratio 而非 ÷ratio²)。代码注释隐含「相隔数年」但**未列入显式限制清单**。按 Leader 裁定不阻塞;**建议补记为已知限制**(follow-up)。

### 3. 设计变更严查:eff 观测源改「年度 EPS+股本」（去季度 EPS）（item 3）
- 去掉季度 EPS 观测(低值舍入会造同比例假观测),保留年度 EPS + 股本(季度股本大整数无舍入噪声,紧边界)。
- **10:1 场景无退化**:TestFetchCompanyFactsSplitNormalization 仍 PASS——split.json 年度 EPS 给出 eff=2025-02-26(与原 refineEffDates 取最早值一致),原断言 Q1=0.30/shares=25000/Q4=0.15 不变。生效日精度未退化。✓

### 4. NVDA 无回归（item 5）
discovery smoke_verification 仍:R=4 eff=2021-08-20、R=10 eff=2024-08-28、pe_ttm=32、符号矛盾 0、71 行。rework_note 明确「NVDA 无回归...不变」。生效日由股本观测保持(设计变更未动 shares 源),精度不变。✓

### 5. discovery 同步（item 4）
- 限制列表(2):「同比例多次拆股已支持(review_fix 按生效日聚簇)」——已从限制改已支持。✓
- rework_note 在位:载明 clusterEvents/splitClusterGap=365、eff 源改年度 EPS+股本、新增 TestFetchCompanyFactsRepeatedSameRatioSplit、NVDA 无回归、12 测试全绿 92.2%、commit fdb5664。✓
- 注:顶层 verification 块仍显首验 11 PASS/91.6%/a59d2af（增量记录模式,rework_note 为刷新载体,非缺陷;建议整洁化时刷新顶层块）。

## 回归/覆盖/越界（item 6）
- 全量 **12 测试全 PASS**(11 首验 + 新 RepeatedSameRatio)。
- 既有测试断言**零删改**(diff 删除行无 assert,纯新增 fixture+测试+算法重构)。
- 覆盖率 **92.2%** ≥ 80。
- 越界(git show --name-only):仅 internal/collector/edgar/(client.go/client_test.go/新 fixture),无越界。

## 结论

WARNING-1 修复正确:clusterEvents 按生效日聚簇支持同比例多次拆股,double_split ÷4 强断言证实(非 ÷2);设计变更(eff 去季度 EPS)不退化 10:1、NVDA 无回归;12 测试全绿 92.2%;既有断言零改动;无越界。两条非阻塞建议:①补记「两次同比例拆股<1年会误合并」已知限制(item 2);②整洁化时刷新顶层 verification 块(item 4)。**VERIFIED**。
