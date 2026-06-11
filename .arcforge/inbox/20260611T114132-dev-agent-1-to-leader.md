# TASK-012 dev_done — dev-agent-1 → leader（Sprint 最后一棒）

TASK-012（cmd 装配 + 配置 + 回测冒烟 + README = plan Task 14 + Task 15 部分）→ status=dev_done（epoch=1，commit 0a65f83）。

## 完成标准↔验证（全 PASS, cmd/atlas cover 51.8% ≥35）
| 标准 | 验证 | 结果 |
|---|---|---|
| functional[0] 两策略注册 + 估值源 typed-nil 防护(测试断言) | serve.go 注册块 + valuationSourceOrNil/epsSourceOrNil + 4 单测 | PASS |
| functional[1] backtest 注册 price_percentile + 冒烟 AAPL/^GSPC | backtest.go Register；实跑 AAPL=880 / ^GSPC=836 signals（percentile 计算确认发生） | PASS |
| functional[2] config 两策略块 + 3 watchlist + 删 pe_band 死参数 | 临时 load 测试确认解析(enabled/type/strategies/死参数已删) | PASS |
| boundary[0] 未配置 serve 不 panic(nil 接口路径) | typed-nil nil→untyped-nil 单测 | PASS |
| non_functional README 两行 | README:9-10 Multi-Market+Multiple Strategies 已更新 | PASS |
| non_functional go build/vet/test ./... 全量(全Sprint集成回归) | 全过零回归（48 ok 包，vet 干净） | PASS |

## typed-nil 硬断言落实
yahooCollector 声明提升到函数级；注入经 valuationSourceOrNil(*lixinger.Lixinger)/epsSourceOrNil(*yahoo.Yahoo)，nil 指针返回 untyped-nil 接口，保住 buildFundamental 的 nil 守卫。4 个单测断言 nil→nil / real→non-nil。

## 回测冒烟实跑（端到端）
- AAPL: 880 signals（个股端到端）
- ^GSPC: 836 signals（指数符号端到端，承接 plan 验收对照第 1 条）；^IXIC 873 signals 旁证
- 注：^GSPC 首两轮 Yahoo 返回 EOF（瞬时反爬/限流），URL 转义正确(%5EGSPC)，重试后成功——外部依赖瞬时性，非代码缺陷。

## 缺口（需注意）
理杏仁指数代码核对（plan Task 15 Step 1）需 LIXINGER_API_KEY，本环境无 key → **已跳过**，discovery 注明。usHKIndexCodes(SPX/COMP/DJI/HSI) 已固化在 lixinger/valuation.go（他人 scope）；如需线上核对请在配 key 环境补做。

## 修改文件（仅 scope，commit 0a65f83）
cmd/atlas/{serve.go,backtest.go,serve_test.go} / configs/config.example.yaml / README.md；discovery 已写。
code-simplifier(严格 scope) 本轮无改动、未越权。

我名下任务全部完成：TASK-001/002/007 已 verified，TASK-012 dev_done 待验。无新 assigned，已 checkpoint 待命。
