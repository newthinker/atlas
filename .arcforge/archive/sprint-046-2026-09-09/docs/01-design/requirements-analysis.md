# Hestia M3 需求分析（warp-hestia 解读 skill）

**需求文档**：`/Users/zuowei/workspace/go/src/github.com/newthinker/hestia/docs/superpowers/plans/2026-09-06-hestia-m3-warp-hestia.md`（1039 行）
**上游 spec**：`superpowers/specs/2026-09-06-hestia-m3-warp-hestia-design.md`
**分析日期**：2026-09-08 · **Leader**：主 session

## 1. 需求性质

这份需求文档**不是待细化的需求**，而是一份**已定稿的实施计划**：设计已经过 brainstorming → spec → writing-plans 全流程，
每个任务的接口签名、测试代码、实现代码骨架都已写死在文档里。

⇒ **本 sprint 的 Leader 工作不是重做设计，而是把它映射到 Arcforge 任务图**，并处理三个框架层面的承载问题（见
`architecture-decisions.md` 的 AD-M3-1/2/3）。凡计划已给出的实现细节，任务 DoD **引用而不复制**——复制会产生
第二份可能漂移的真相源。

## 2. 目标（计划原文）

> 人在 Warp 会话说一句「处理 hestia 队列」，skill 取最旧一份契约，用 `prepare.py` 填好 frontmatter 与派生指标、
> 模型写叙述、经 Spool 写进 `Wiki/Macro/PBOC/`，笔记带 `reviewed: false` 等人审；契约进 `done/`。

## 3. 三仓库改动面

| 仓库 | 改什么 | 语言/形态 | 本机路径 |
|---|---|---|---|
| **atlas** | `internal/hestia` 新增 history 侧车；`cmd/atlas` 的 `contract emit` 同产侧车 | Go | `~/workspace/go/src/github.com/newthinker/atlas` |
| **loom** | `spool.archive` 加受白名单限制的 `source` 参数；写白名单加 `Wiki/Macro/PBOC` | Go | `~/workspace/go/src/github.com/newthinker/loom` |
| **nanoclaw fork** | Warp 组队列读写挂载 + 触发段；新 skill `container/skills/warp-hestia/`（文档 + Python 脚本 + 测试） | Markdown / Python 3 标准库 | `~/workspace/ai/nanoclaw` |

## 4. 环境侦察实测（2026-09-08，全部实跑核实）

| 项 | 实测值 | 取证方式 |
|---|---|---|
| atlas `internal/hestia` 覆盖率 | **96.6%** | `GOTOOLCHAIN=local go test ./internal/hestia/ -cover` |
| `Store.Preceding` 签名 | `(ctx, period, periodType string, n int) ([]Observation, error)` — 与计划一致 | `store.go:354` |
| AST 守卫 `want` 列表位置 | `internal/hestia/store_test.go:453`，现 33 项 | 实读 |
| loom spool 现有测试 | 35 条（archive 18 · commit 6 · taint 11） | `grep -c '^func Test'` |
| loom `injectTaint(` 调用点 | **12 处**（改签名要全改） | `grep -rn` |
| loom `spool_write_allow` | `config.yaml:17` 与 `config.local.yaml:6`，**两处都只有** `Wiki/Loom-Research` | 实读 |
| nanoclaw `groups/*` | **被 `.gitignore:15` 忽略** ⇒ 计划 TASK-003 的两处改动是本机直接改、不进 PR | `git ls-files` 返回空 |
| nanoclaw 本机 checkout | `feat/vendor-agent-reach-skill`，工作区脏（2 改 2 删） | `git status --short` |
| nanoclaw `fork/main` | 已含 `container/skills/warp-research`（M3 的先例）；本地 `main` == `fork/main` @ `d791101` | `git ls-tree` |

## 5. 覆盖率下限的含义

`internal/hestia` 现为 **96.6%**，而计划的 DoD 也写 **≥ 96.6%**。这不是余量，是**贴地下限**：TASK-001 新增的
`history.go` 若有任何一行未被测试覆盖，整包覆盖率就会跌破。⇒ TASK-001 的 DoD 必须把「错误分支也要测」
写成显式条目，而不是靠「跑一下看看」。

## 6. 范围裁决（人类 2026-09-08 确认）

- **本 sprint 交付计划的 TASK-001…005**，映射为 Arcforge 的 TASK-001…006（拆分理由见 `design-spec.md` §2）。
- **计划的 TASK-006（集成冒烟）结转**：它自述前置是「M1.5+M2a+M3 一次投递之后」，而那次投递又排在
  2026-08 月报首期验收（09-09 ~ 09-15）之后。今天 09-08 ⇒ **本 sprint 内结构上不可能完成**，
  写进 `06-acceptance/final-report.md` 的结转段，不建任务文件。

## 7. 不做（YAGNI，计划已明确）

- 不改 `Parse` / `Validate` / `Save`；`store.go` 不新增方法
- 不装任何第三方 Python 包（容器内只有标准库）
- 不放松 `reviewed: false` 的无条件覆写——`source` 是溯源不是信任标记
- agent 不碰 `~/.config/nanoclaw/mount-allowlist.json`、不 `launchctl kickstart`、不在 Warp 会话里替人说话
