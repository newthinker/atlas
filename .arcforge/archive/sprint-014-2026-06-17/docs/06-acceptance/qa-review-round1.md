# QA Code Review 报告（feature/telegram-digest-table, master..HEAD 4 提交）

## 门禁基线
- go build ./... 干净；go vet ./internal/... 无输出
- 全量 -race：router/config/notifier(all)/app 全 PASS，整包零回归

## verdict: PASS（无 CRITICAL）

## WARNING（severity_threshold=warning → 经人类裁定做一轮 review_fix 修复）
- **W1 [telegram.go sendMessage/escapeMarkdown]**：全文 `_`→`\_` + parse_mode=Markdown，代码块内含 `_` 的 symbol/name 字面显示反斜杠。现网 atlas 数据不含 `_` 故不触发，但属潜在渲染错误。
- **W2 [telegram.go formatBatch]**：latest=max(GeneratedAt) 为零值时标题显示纪元时间 0001-01-01。

## INFO
- I1：formatBatch 内层 action 匹配可用 slices.Contains
- I2：倒数第二列分隔 padding 产生尾随空格（代码块内无视觉影响，DoD「末列无尾随补空格」满足）
- I3：formatSignal 用 ⏸️(VS16)、digest 用 ⏸，hold 图标单发/汇总不一致（观感）

## Round 2 跨视角（纯 Claude 多视角，cross-model 降级）
- correctness：分组/排序/CJK 对齐/空轮不发/串行 flush 均实测正确
- concurrency：pending 全程 mu 保护，flush 锁内 swap 后释放再 IO，-race 全绿
- security：无新增注入面
- maintainability：职责清晰、表驱动易扩展
- 未发现 Round 1 漏掉的 high-severity 项

## 处置（人类裁定）
做一轮 review_fix 修 W1+W2，并捎带 I1/I3 清理。详见 TASK-002 fix_items。
