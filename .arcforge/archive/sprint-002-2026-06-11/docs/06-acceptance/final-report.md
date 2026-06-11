# Sprint 终验收报告 — sprint-002 指数/商品采集 + 历史百分位策略（2026-06-11）

**需求源**: docs/plans/2026-06-11-index-commodity-percentile-implementation.md（rev3 终版实现计划）
**设计依据**: docs/plans/2026-06-11-index-commodity-percentile-design.md（rev6）
**改动规模**: 14 commits / 34 files / **+2347 −43**（基于 master `80c43df`）
**QA 终判**: **PASS**（round1+2 PASS+W1 提请裁决 → W1/S1 修复 → round3 PASS，CARRYOVER I3 消解）

## 需求达成（R1-R9 全部 ✅）

| 能力 | 任务 | 交付 |
|------|------|------|
| 国际指数/商品期货采集 | 001/002/003 | yahoo 接受 ^GSPC/GC=F 类符号（URL 百分号编码），selector 路由 ^/=F→yahoo，^HSI→HK 市场归属 |
| A 股指数采集 | 003/004 | 六指数 secid 共享表（collector），eastmoney parseSymbol 指数/个股区分（000001.SH vs .SZ） |
| EPS 历史序列 | 002 | yahoo FetchEPSHistory（fundamentals-timeseries，UA 头复用，StatusCode 守卫） |
| 理杏仁多市场估值分位 | 005 | cn/hk/us × company/index 端点分派，cvpos×100，y3/y5/y10 粒度 |
| 分位纯函数 | 006 | PercentileRank（strictly-less）+ ReconstructPEPercentile（阶梯对齐/MinEPSPoints=8/双哨兵错误） |
| price_percentile 策略 | 007 | 全六类资产，252 门槛，分档置信度，信号带价 |
| pe_percentile 策略 | 008 | 股票+指数，Source method:fallback_reason 解析，PriceHistory 声明（load-bearing） |
| 既有策略 AssetTypes | 009 | ma_crossover 全资产 / pe_band+dividend_yield 仅股票 |
| app 装配 | 010/011 | 类型识别、绑定过滤+warnOnce、动态历史窗口、估值编排兜底链（亏损不兜底 stub 硬断言） |
| cmd 装配 | 012 | serve 注册双策略（typed-nil 防护）、backtest 冒烟（AAPL + ^GSPC 端到端）、config 示例、README |

## 质量数据

- **12/12 任务 verified→accepted；功能开发阶段零返工零阻塞**（对比 sprint-001 同期 3 返工 2 澄清）——plan rev3 施工图 + 上 Sprint 教训前置注入（ISSUE-1 StatusCode、W1 信号带价）直接见效
- QA 阶段 1 轮 review_fix（W1 仲裁信号补价 + S1 注入不变量，一轮回流）
- 门禁：`go build/vet/test ./...` 全绿；app/collector/valuation/strategy 全家 -race 无竞态；plan 验收对照 6/6 PASS
- DoD 36 条全部测试映射（04-test/ 13 份验证矩阵含复验）

## 计划外修复

1. **QA W1**：仲裁合成信号 Price=0（CARRYOVER I3 升级为条件可达）→ referencePrice 补价 + 反例锁定测试（cc0182a）——sprint-001 的 W1 模式在新场景复发被对抗审查再次拦截
2. **QA S1**：估值源注入不变量文档化（set-once-before-Start）

## 遗留（不阻塞）

- 理杏仁口径核对（cvpos ≤ vs < 边界、usHKIndexCodes 四个候选代码、metricsList 键名）——需 LIXINGER_API_KEY，代码已注明首日核对项（QA I-a）
- 手工 serve 终验（真实 watchlist 加 ^GSPC/GC=F 观察信号）建议上线前做一次（QA 验收对照第 1 条的 serve 侧）
- I-b/I-c/I-d：全同值序列 by-design、小写后缀 trivial、不可达分支备查

## 框架运维记录（本 Sprint）

- teammate-idle hook 两轮修复（test-* 按 verifier 过滤 → 仅匹配自己；ISSUE-4 第二、三例），最终原则：保活条件按「该实例可执行的动作」过滤
- QA agent 自我循环用明确待命指令打断（hook 判定正常时的处置先例）

## 流程统计

- 团队 dev×4 + test×2 + qa×1；dag 调度；wave1（2 任务）→ wave2（8 任务满负荷）→ 011 → 012
- 完成消息遗失多次，文件真相源轮询全部自愈
