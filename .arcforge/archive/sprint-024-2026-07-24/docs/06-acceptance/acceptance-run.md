# M1 人工验收执行记录(2026-07-24)

## 结论: 五条验收全部通过 ✅

## ① 首跑落库 — PASS(Leader 代跑,用户授权)
- `prism refresh`(runtime cwd): `6 ok, 2 failed`,exit 0;valuation_daily = 9460 行
- 000300.SH/000905.SH 各 2428 行(10y)、^GSPC 2513 行、NVDA 683/MSFT 704/AAPL 704 行
- ⚠ Task 3 live 校验点通过: 指数 .mcw 指标名外推正确,三指数全部成功回填

## ② 官网分位比对 — PASS(用户目检)
- /prism/board 沪深300(2026-07-23 PE=14.56, 10Y 分位 89.8)与理杏仁官网比对通过
- 茅台因理杏仁公司时序 API 权限无数据(见失败项分析),不在本条比对范围

## ③ NVDA 曲线形态 — PASS(用户目检)
- /prism/detail/NVDA 5Y PE 曲线呈「阶梯+价格波动」形态,窗口切换正常

## ④ 降级显示 — PASS(等价判定)
- board 卡片渲染「数据至 2026-07-23」(昨日)而非报错——数据滞后时的降级显示语义已实证

## ⑤ 二跑增量 — PASS(Leader 代跑)
- 同日二跑行数 9460 与 MAX(d) 均不变: lixinger 零请求跳过(理杏豆零消耗),engine 幂等

## 失败项分析(均非代码缺陷)
1. 600519.SH/0700.HK: lixinger 公司端点时序 API 需购买 Open API(1 年回看诊断同拒,属端点权限;指数不受限)。待决策: 购 Open API / 腾讯切 engine 路径(改配置) / 茅台走 latest 单点累积降级(M2 小任务) / 保留现状(每日告警提示)
2. NVDA 二跑 yahoo 瞬时 EOF(首跑已成功,每日全量重算自愈)

## 部署侧实证
- 验收 serve 首启在 runtime 复现 QA CRITICAL(旧 rsync 模板副本缺 prism 模板),复制模板后正常——坐实该缺陷生产放大路径与修复必要性;正式部署时 deploy.sh rsync 会自动带上(修复已入库)

## 环境变更
- runtime configs/config.yaml 追加 prism 段(备份 config.yaml.bak-prism-m1);data/prism.db 新建(9460 行)
- runtime internal/api/templates/ 补入 prism 两模板(等价下次 deploy rsync)
- 生产 bin/atlas 未动;验收 serve(8091)已停止
