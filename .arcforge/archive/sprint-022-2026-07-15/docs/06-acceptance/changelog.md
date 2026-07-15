# Changelog — Sprint 022（feature/crisis-replay-report，master..857ecb3）

## Added
- `atlas crisis report --from --to --form daily|monthly [--send]`：历史回放报告子命令；
  文本报告逐条输出 + 标准总结 + 自包含 HTML 落盘 `reports/crisis-replay-<from>-<to>.html`；
  `--send` 经 telegram 逐条发送（间隔 3s、单条失败继续、量控 31 条启动前拦截、HTML 经 sendDocument，
  不支持时降级为总结尾附本机路径）。
- `crisis.ReplayRange`：暖机回放引擎（库内最早 vix 观测日起逐日推进、零写入、窗口切片、StateDays）。
- `crisis.ReplayReport`：daily/monthly 文本报告装配（忽略消息矩阵门控、PrevDay 差异链、21 日 Trends）。
- `crisis.RenderReplaySummary`：回放窗口标准总结（转移/停留/极值方向/AMBER 峰值/STALE，≤4096，回放专用尾注）。
- `crisis.RenderReplayHTML`：自包含单文件详细报告（状态点阵/7 指标折线含阈值线与 STALE 打点/月度汇总/转移明细，无外链、亮暗兼容）。
- `telegram.SendDocument`：multipart/form-data 文件上传（caption 按 rune 截 1024，错误语义同 sendPayload）。

## Changed
- `crisis replay` 子命令统一暖机语义（设计 v1.1）：引擎从库内最早观测日推进，窗口期初态为暖机结果；
  **全期窗口输出逐字节不变**（黄金对照验证），非全期窗口期初态变化为预期内行为修正。

## 提交清单
d2591fe feat(telegram) · 1156257/283ed97 feat+test(crisis engine) · f72e7f3 feat(report assembly) ·
5c6ca07 feat(summary) · 61dad9e/5dc312e feat+refactor(HTML) · c5f2d86/857ecb3 feat+refactor(subcommand)
