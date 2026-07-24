# AKShare 数据源(M1.5)需求分析

> Sprint 输入: docs/superpowers/plans/2026-07-24-prism-akshare-source.md(git 31e645a)
> 上游 spec: docs/superpowers/specs/2026-07-24-prism-akshare-source-design.md(git ac76bcf)

## 降级/流程说明

ECC 不可用;设计精炼已在本会话经 superpowers brainstorming 完成(spec 含完整决策记录:
aktools 侧车/覆盖范围/编排层自动降级链三大分叉均经用户确认),不重复展开。

## 背景(M1 验收实证)

理杏仁公司端点时序 API 需购买 Open API(指数不受限);yahoo 港股 EPS 仅 5 点、qlib
fundamentals_pit 空表,engine 路径无法覆盖 0700.HK——茅台/腾讯在 M1 池中无数据。

## 功能模块(计划 Task 1-7 一一映射)

| # | 模块 | 复杂度 | 依赖 |
|---|------|--------|------|
| R1 | 配置扩展(FallbackSource/AkshareBaseURL) | 简单 | - |
| R2 | akshare collector 基座 + A 股个股(乐咕 lg) | 中等 | - |
| R3 | 港股(百度双指标合并)+ A 股指数(乐咕中文键) | 中等 | R2(同包) |
| R4 | refreshAkshare 路径(增量+本地 5Y/10Y 分位+Store.Series+Refresh 签名变更) | 复杂 | R1,R3 |
| R5 | 指数自动降级链 + Report.Degraded | 中等 | R4(同包) |
| R6 | CLI 接线(client 注入+Degraded 告警) | 简单 | R5 |
| R7 | 部署产物(独立 venv/aktools plist/配置/文档)+ M1.5 验收 | 简单 | R6 |

## 全局约束(传导 DoD)

1. sqlite 固定 v1.38.2;GOTOOLCHAIN=local 兜底;testify+httptest+Context Checkpoint 注释。
2. NaN 缺失约定不变;akshare 无官方分位→本地 RollingPercentile(5/10, 252)。
3. **⚠ AKShare 接口名/字段键均为实现期 live 校验点**(lg/baidu/index 三组),首跑不符以实际响应修正常量+同步测试并 commit 注明。
4. Refresh 签名破坏性变更(加 AkshareClient)——R4 必须同步修全部调用点,go build ./... 始终可过。
5. 主源成功不得触碰兜底源(零多余请求);部分失败不中断。
6. venv 隔离部署(scripts/akshare/.venv,与 qlib_eval 互不影响)。
7. 每 Task 单独 commit;deployment.md 只追加不重排;不动无关代码。

## 风险点

- AKShare 接口漂移(live 校验点覆盖);aktools CLI 旗标名待实测(plist 注明)。
- R4 触碰 cmd/atlas(签名涟漪)与 internal/prism 两包——串行链上无并发冲突。
- 指数兜底口径混合(官方 cvpos vs 本地计算)为 spec 已确认的取舍,文档注明。
