# 独立验收评审结论（只读需求，未看 DoD）

评审员：独立 agent，仅读需求文档 + 源码，未接触任何 .arcforge/ DoD 文件。

## 已核验属实并已处置的发现

| # | 等级 | 发现 | 真实代码核验 | 处置 |
|---|------|------|-------------|------|
| 1 | HIGH | 串行路径(`workers<=1`)在 `g.Wait()` 前 return，计划仅在 Wait 后 flush → 串行配置下 batch 信号永不发送+跨轮泄漏 | ✅ 属实（app.go:349-357 串行 return；376 才 Wait） | 已改 TASK-004：改用 `defer FlushNotifications()` 覆盖全部出口；加串行 flush 回归 DoD |
| 3/4 | HIGH/MED | `sendMessage` 强制 `escapeMarkdown`(仅转义 `_`)+`parse_mode=Markdown`，可能破坏 ``` 表格 | ✅ 机制属实，但 LOW 实际风险（股票 symbol/中文名罕含 `_`/反引号；legacy Markdown 支持 ``` 围栏） | 已给 TASK-002 加边界 DoD + 部署人工核验项 |

## 留待人类在 dod-gate 定夺的设计判断

| # | 发现 | 性质 |
|---|------|------|
| 2 | `batch_notify` 默认 true 改变现网行为（逐条即时→末尾汇总、富文本→表格） | ADR-3 既定，但需人类确认是否接受现网灰度/回退 |
| 8 | 表内无 Action 列，strong_buy 与 buy 合并后用户无法区分强弱 | 信息丢失，spec 既定分组，确认可接受 |
| 9 | renderTable 用 `s.Symbol`，digest 内 HK 代码不再像 formatSignal 那样补零显示 | 与逐条格式不一致，确认可接受 |

## 评审员列出但已被现有 DoD/约束覆盖的点（无需额外动作）

- 空轮不发(R7/B1)、单组缺失省略、仅持有组、空 name、Price<=0、末列不补尾空格、nil registry、notifier 失败隔离、flush 后清空、并发 `-race`、零回归、无新依赖 —— 均已在 TASK-001/002/003 的 DoD 中。

## 可测试性提示（转入部署/集成验证）

- 等宽对齐的**视觉效果**依赖 Telegram 客户端 CJK 字体渲染，CI 无法断言 → 保留计划「部署验证」人工看截图。
- `parse_mode=Markdown` 实际解析结果、现网通知节奏切换 → 集成环境 `analysis-now` 触发一轮人工核对「收到一条而非数十条」。
