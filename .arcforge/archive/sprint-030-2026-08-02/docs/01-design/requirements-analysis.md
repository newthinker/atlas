# M3.5a 需求分析(Sprint-030)

> 需求文档: `docs/superpowers/plans/2026-08-02-prism-m3.5a-datasources.md`
> 设计依据: `docs/superpowers/specs/2026-08-02-prism-m3.5-design.md`(实测记录 §0 为事实基础)
> 分析模式: ECC 不可用(config capabilities.ecc=false),降级为直接提炼——输入已是带
> Self-Review 的精炼实施计划,brainstorming 交互探索不适用,设计已在 spec 定案。

## 目标

接入三个新数据源并扩展 Prism 降级链(spec §2 总表):

1. **Tushare**(A/H 估值+行情备源)— 纯 Go POST JSON 客户端,4 个能力方法
2. **Twelve Data**(美股价格备源)— 纯 Go GET 客户端,8s 节流
3. **Baostock 桥**(A 股行情第三跳)— Python 侧车 + Go 客户端(仿 aktools 模式)
4. **编排层**: `internal/prism/refresh.go` 降级分派逐跳扩展,每跳记 `Report.Degraded`

## 功能模块与复杂度

| 模块 | 复杂度 | 说明 |
|------|--------|------|
| tushare 客户端 | 中等 | 计划含完整参考实现;含 live 探针(index_dailybasic 权限) |
| twelvedata 客户端 | 简单 | 同构骨架;live 校验依赖用户注册 key(未注册则跳过并注明) |
| baostock 桥 | 中等 | Python 侧车(计划含完整 bridge.py)+ Go 客户端 + launchd plist |
| 配置接线 | 简单 | PrismConfig.BaostockBaseURL + example yaml 空 key 条目 |
| 编排层降级链 | 复杂 | Refresh 签名扩到 9 参、4 条新降级跳、永久性错误语义 |
| 部署演练文档 | 中等 | deploy 脚本、断源演练(spec §6 验收)、§10 风险表 |

## 关键约束(全任务生效)

- **密钥卫生(spec §5)**: token/key 只入 runtime `configs/config.yaml`(gitignored);
  example 只留空值+注释;每次 commit 前跑哨兵 grep(SENT 取自 runtime config 前 8 位)。
- **降级链原则(spec §2)**: 相邻两跳跨故障域;临时错误触发下一跳;永久性错误
  (tushare 40203 权限)不重试、Degraded 文案注明「配置性问题」。
- **限频**: tushare 200ms 最小间隔;twelvedata 8s 最小间隔,缺 key 该跳直接跳过。
- **零回归**: 既有链路(yahoo/akshare/lixinger/edgar)零行为变更;新跳只在主源失败后触发。
- **技术栈**: Go 1.24.4 / testify+httptest;sqlite 固定 v1.38.2 + GOTOOLCHAIN=local;
  NaN↔NULL 约定;Context Checkpoint 注释;`feat(prism):` commit 规范。

## 范围外(M3.5b,另一份计划)

N-PORT 持仓解析、ETF 加权调和平均聚合、SPDR 11 ETF——不在本 Sprint。
