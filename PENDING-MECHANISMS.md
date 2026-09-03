# 待决机制变更（Arcforge）

**这份文件只放「等人拍板的机制变更」，不放证据、不放教训、不放叙事。**

超过 40 行就是失败了 —— 它存在的全部理由是 `wisdom/decisions-leader.md` 已经 32KB、
`wisdom/_digest.md` 已经 88KB，而下一个 leader 需要的是这几行。

⚠️ **改动落点是 Arcforge 上游仓库**（`newthinker/ArcForge`）—— 本仓库是消费项目，无 `project-template/`。

---

## 当前待决：**无**

## 已落地（M1c-4 提出，2026-09-03 由 PR #6 合入上游 `main` @ `496bbf0`）

| # | 变更 | 落点 |
|---|---|---|
| 1 | `update` 加 `--expect-status`（锁内重读 `status`，与 `--expect-epoch` 同构） | `project-template/hooks/arcforge-write.sh` |
| 2 | validator 规则 `archive-mutated`：归档目录在同名 tag 后不应再变 | `validator/archive_seal.go` |
| 3 | validator 规则 `unregistered-writer`：写过但无 token 登记的实例名 | `validator/audit.go` |
| 4 | 派验固定动作加「请回一句确认收到」 | `templates/CLAUDE.md.template` |

⚠️ **2、3 都是审计而非拦截**：`>>` 绕 write-guard、未登记名字免验 token 这两个洞都**不堵**
—— hook 无法可靠区分调用者身份，堵它们要么做完备语法分析、要么禁掉 Bash 工具。
原则是**不禁止变化，只保证变化不会被静默吞掉**。

## 明确不做（结论仍有效，勿重开）

- **不给 `in_progress` 加 `stale-dispatch` 阈值** —— 刻意的，dev 正常干活 p90 就 65 分钟
- **不改 `git log --grep` 的实现** —— 判据走文档：按 sprint 起点截断 + 只看 `%s`（标题），
  因为它可能命中 body（`0981d90` 标题里根本没有 TASK-014）

## 证据

`.arcforge/wisdom/decisions-leader.md`，grep 这些锚：
`expect-status` / `第三个写通道缺口` / `两个机制缺口，都由 QA 主动报告而非利用` / `派验要求回执`
