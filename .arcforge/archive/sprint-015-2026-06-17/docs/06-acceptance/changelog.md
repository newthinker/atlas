# Changelog — digest「PE%」列

## Added
- digest 表格新增末列 `PE%`：每个标的「当前市盈率在其历史 PE 中的百分位」（0–100%）。
- `internal/app` 富化：`enrichSignalMetadata` 给每条信号盖展示专用键 `Metadata["pe_percentile_display"]`（来自 `Fundamental.PEPercentile`），使 PE% 列对所有有 PE 的标的填充，不限策略。

## Changed
- `enrichSignalMetadata` 增参 `fundamental *core.Fundamental`；去掉 `name==""` 提前返回，name 与 PE 各自独立盖键（不覆盖既有）。
- `renderTable` 表头 `[SYMBOL,NAME,CONF,PRICE]` → `[SYMBOL,NAME,CONF,PRICE,PE%]`。

## Behavior
- ETF / 金融指数（PE 分位一期不可用）/ 无 Fundamental 的行 → PE% 列留空。
- 同一标的两条信号（price_percentile + pe_percentile）显示相同 PE 百分位。
- PE 处历史最低（0.0%）正常显示，不被当作无值。
- router 门控行为不变（`pe_percentile_display` 非门控键）。

## Commits
- 7889c8b feat(TASK-001): PE% historical-percentile column in digest table
- cebea8a feat(TASK-002): stamp pe_percentile_display on signals for digest PE% column
