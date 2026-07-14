# TASK-202 notifier 适配器 + telegram SendText — 验证报告

- 验证者: test-agent-1
- 结论: **VERIFIED（PASS）**
- commit: 813bf4f（epoch=1, dev-agent-2）/ 分支 feature/audit-optimization-wave1-cleanup

## 反向验收
- 改动 4 文件，全部 = estimated_files（alert_adapter.go/.test、telegram.go/.test）。未碰 internal/alert、email、webhook。
- 新增功能无越界，无"明确不做"项被误实现。

## Done Criteria 覆盖矩阵

| # | 维度 | 标准 | verify_by | 证据 | 判定 |
|---|---|---|---|---|---|
| F0 | functional | 底层实现 SendText → 适配器直发文本（格式 [SEVERITY] name: message） | test | Notify 类型断言 textSender 命中即 `return ts.SendText(msg)`；TestAlertAdapter_DirectPath_WhenSendTextSupported 断言 SendText 收到精确 msg 且 Send 未被调用，fresh PASS | **PASS** |
| F1 | functional | 底层不实现 SendText → 回退 core.Signal{Symbol:SYSTEM, Strategy:alert, Reason:文本} 走 Send | test | Notify 回退分支构造该 Signal；TestAlertAdapter_FallbackWrapsSystemSignal 断言三字段精确，fresh PASS | **PASS** |
| F2 | functional | telegram 公开 SendText(string) error，内部复用 sendRaw | test | telegram.go SendText → `t.sendRaw(text)`；TestTelegram_SendText 用 httptest 抓包断言 payload.text 逐字等于 msg、无 `\_` 转义；TestTelegram_ImplementsSendText 钉接口，fresh PASS | **PASS** |
| B0 | boundary | 底层 Send/SendText 返回错误如实上传，不吞错 | test | 两分支均 `return` 底层 error；TestAlertAdapter_DirectPath_PropagatesError / _FallbackPropagatesError 用 errors.Is 验证，fresh PASS | **PASS** |
| N0 | non_functional | internal/alert 零改动且依赖方向不污染（alert 不 import notifier）；email/webhook 零改动 | review | grep 确认 alert 未 import notifier；alert_adapter.go 仅 import core（未 import alert）——AD-12 双向清洁，靠结构满足接口（var _ alertNotifier = (*AlertAdapter)(nil) 编译期钉住）；commit 未碰 alert/email/webhook | **PASS** |
| N1 | non_functional | 适配器双路径单测齐备；go test ./internal/notifier/... 全绿 | test | 双路径+错误透传+Name 委托共 5 用例 + telegram 2 用例，fresh 全 PASS；notifier 全 4 子包 ok | **PASS** |

## 关键实证
- 逐字送达：SendText 走 sendRaw（不经 escapeMarkdown），普通 Send 才转义——alert 文本 `[SEVERITY] name: message` 中下划线保持字面，httptest 抓包断言证实。
- AD-12：alert !import notifier、adapter !import alert，适配器纯结构满足 alert.Notifier(Name/Notify)，依赖单向。

全部条目 PASS，证据充分（fresh 非缓存）→ VERIFIED。

---

## W1 复验（review_fix，fix commit 0f3c366，epoch=2，rework_count=1）

- 结论: **VERIFIED（PASS）** — W1 达成 + 原 6 条零回归。
- 改动仅 2 文件（telegram.go / telegram_test.go），符合 fix_items 约束。

### fix_items W1 达成
- 问题：SendText 原复用 sendRaw（固定 parse_mode=Markdown），告警文本 `[SEVERITY] name: message` 含未配对 `_ * [ ``` 会被 Telegram HTTP 400 拒收、告警静默丢失。
- 修复：抽出 `sendPayload(text, parseMode)`——空 parseMode 省略字段（纯文本）。SendText→sendPayload(text,"") 无 parse_mode；sendRaw→sendPayload(text,"Markdown")、sendMessage→sendRaw(escapeMarkdown) 均保持 Markdown 不变。
- 实证：TestTelegram_SendText 用含未配对 `_`、不平衡 `[ ]`、孤立反引号的消息（"[CRITICAL] disk_low on node_a: free < 5% [urgent] use \`df\`"），断言 ① payload 无 parse_mode 字段 ② payload.text 逐字等于原文 ③ 无 `\_` 转义。PASS。

### 原 6 条零回归
- Markdown 路径不变：TestTelegram_Send（per-signal）、TestSendBatch_UnderscoreNotEscaped/TestTelegram_SendBatch_Formatting（SendBatch）均 PASS——抓包测试通过即证 Markdown 路径 payload（chat_id/text/parse_mode=Markdown）字节级一致。
- 适配器双路径/错误透传：5 个 AlertAdapter 测试全 PASS（直发路径仍用 SendText，无需改动）。
- go test ./internal/notifier/... 4 子包全绿（全量 ./... 待 TASK-204 落地后门禁确认，Leader 指示）。

W1 达成 + 零回归 → 维持 VERIFIED。
