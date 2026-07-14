# QA 终审报告（Sprint 019 · qa-agent-1 · 2026-07-14）

**Verdict: CONDITIONAL PASS**（0 CRITICAL / 2 WARNING / 3 INFO）
审查方式：两轮（第一轮 Architect 六关注点 + Global Constraints；第二轮 Minimalist + 攻击者跨视角，纯 Claude 降级——codex/gemini CLI 不可用）。范围：internal/crisis/*.go、collector/fred/fred.go、cmd/atlas/crisis.go、configs/crisis-monitor.yaml、3 个 crisis plist。

## WARNING 与 Leader 裁决

| # | 发现 | 位置 | 裁决 |
|---|---|---|---|
| SEC-1 | FRED api_key 经传输错误泄漏：fred.go:59 把 key 拼进 URL query；传输层失败（连接/TLS/超时/DNS）时 *url.Error 的 Error() 含完整 URL（Go stripPassword 不屏蔽 query），经 :74/:79 冒泡至 stderr → launchd crisis-daily.err.log 落盘。HTTP 状态码路径安全 | fred.go:87-89 | **采纳 → review_fix**（TASK-002 重开，dev-agent-2） |
| CLEAN-1 | LatestObservation 死导出（生产零调用者，仅测试引用） | store.go:98 | **降级 INFO 不修**：该方法属方案「核心接口契约」（行 123）明确规定的 API 面，本 Sprint 契约冻结（执行者不得偏离）；删除违反契约。记录待契约修订时处理 |

## INFO（记录，不修）

1. crisis.go:79/126-129 backfillIndicator 未校验 ∈ AllIndicators，拼错静默写入不可读指标名（操作者本机 CLI，数据完整性提示）。
2. rules.go:172 t10y2y 倒挂红线 `Value < 0` 硬编码——0 是倒挂的内生数学定义而非可调阈值，设计注释已记；与 NFCI red_above:0（配置项）语义不同。
3. crisis.go:421-424 盘中 wow 重实现未复用 derive.WowPct（live-quote+库存窗口形态不同）；mustAddDays 复制未导出 addDays（跨 package）。均可接受。
4. （由 CLEAN-1 降级）store.go LatestObservation 契约内死导出。

## 通过项摘要

- Architect：WAL/事务/资源关闭/context 透传无缺陷；契约签名与方案行 79–189 逐条一致；依赖方向干净（crisis 经窄接口解耦 yahoo/telegram）；范围外五项未越界。
- 攻击者：CSV 导入无遍历/公式/SQL 注入（参数化+ParseFloat）；telegram 纯文本无格式串注入；plist 不含密钥。
- 合规：禁词"必然/一定/即将"零出现且有测试强制；[P0-P2] 前缀 + 边界声明 footer 齐全；阈值全在 YAML（仅 minPercentileObs=60 属偏差 3 许可）；go build/vet/test 全绿。

## 补录（2026-07-14，归档后裁决）

qa-agent-1 关闭前送达修订版 verdict：**CONTESTED**（0C/1W/4I，SEC-1 经其复验确认已修复）。唯一 WARNING——WATCH→NORMAL/BREWING→WATCH 退出冷却按字面语义全局回看，危机康复尾段可将 20 日观察期压缩至 ~1 日。**人工裁决（用户）：修复为态内计数**。落地 commit 7d84524（systemDetailStreak 增同态约束 + TestExitStreakRequiresInStateHistory 三半判别用例 + 修正两处既有 fixture 防判别力静默降级），全分支 100% 覆盖、53 包全绿，已推送 PR #45。4 条 INFO（SOFR/USDJPY 边界含义、盘中 wow 口径、validate 零值健壮性）记录不阻断。
