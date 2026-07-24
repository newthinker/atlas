# M1.5 人工验收执行记录(2026-07-24,用户授权 Leader 全部代跑)

## 结论: 五条验收全部 PASS ✅(期间触发并闭环一轮 live 接口修正)

## ① setup + launchd + curl — PASS
- runtime 侧 setup.sh: 独立 venv 建成,akshare 1.18.75 + aktools 0.0.91,requirements.lock 生成(48 行)
- launchd 装载 com.newthinker.atlas.aktools;API 端点可达(根路径 500 为 aktools 自身主页模板 bug,不影响 /api/public/*)

## live 校验点触发与修正(计划预期路径,一轮闭环)
- 实测: lg 个股接口已被 akshare 1.18.75 移除;百度系 period=全部 为 15 日降采样(近一年/近三年=日频,近十年≈5日);指数 lg 接口完美零修改
- 修正(8c59cd4,review_fix rework=2,test-agent-3 复验 verified): A/H 统一百度路径 + 窗口跨度 period 策略(≤1y 近一年/≤3y 近三年/更长 近十年+近三年日频覆盖合并)

## ② 茅台/腾讯上墙 + 曲线连续 — PASS
- 修正后首跑: **8 ok, 0 failed, 0 degraded(全池首次满员)**;茅台/腾讯各 1607 行(2016-07-25~2026-07-23 精确 10y)
- 连续性数值核查: 茅台近一年 365 行日频;十年最大间隔 5 天(=近十年采样粒度,设计内)

## ③ 指数兜底演练 — PASS(隔离 scratch 库,不碰生产数据/告警)
- 坏 key → lixinger 真实报「token权限验证错误」→ 降级链当场切 akshare → `1 ok, 0 failed, 1 degraded`
- Degraded 消息格式精确: "000300.SH: lixinger failed (…), akshare fallback ok";乐咕回填 2427 行,本地分位 98.2/85.5 在值域
- 插曲如实记录: 首次演练脚本 key 替换正则未命中,主源真实成功,额外消耗一次理杏仁指数 10y 回填(理杏豆成本);修正后重演成功

## ④ 二跑增量 — PASS
- 行数 12674 → 12674 零变化;akshare/lixinger 标的零请求跳过(latest+1 语义)
- 二跑一个 engine 标的 yahoo 瞬时 EOF(三跑 8 ok 自愈,每日全量重算语义,与 M1 同款)

## ⑤ 分位 sanity — PASS
- akshare 全部行分位值域 [0,100] 违例数 0
- 茅台 2026-07-23: PE 19.53 / PB 5.96 / 5Y 分位 7.1 / 10Y 分位 5.5(历史低位,方向合理);腾讯: PE 15.2 / PB 3.17 / 26.3 / 20.3

## 环境变更
- runtime: scripts/akshare 投递+venv 建成;aktools launchd 常驻(127.0.0.1:8180);configs/config.yaml 池同步(备份 config.yaml.bak-m15);prism.db 12674 行
- 生产 bin/atlas 未动(验收用 scratchpad 临时二进制);演练 scratch 残留已清理
- 待办: requirements.lock 提交入 git(部署机已真实生成,按 T7 约定)
