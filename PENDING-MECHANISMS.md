# 待决机制变更（Arcforge）

**这份文件只放「等人拍板的机制变更」，不放证据、不放教训、不放叙事。**

超过 40 行就是失败了 —— 它存在的全部理由是 `wisdom/decisions-leader.md` 已经 33KB、
`wisdom/_digest.md` 已经 91KB，而下一个 leader 需要的是这 4 行。

证据在 `.arcforge/wisdom/decisions-leader.md`（本文件每条都给了节标题，可 grep 定位）。

⚠️ **改动落点是 Arcforge 上游仓库** —— 本仓库是消费项目，无 `project-template/`。

---

| # | 变更 | 建议 | 状态 |
|---|---|---|---|
| 1 | `arcforge-write.sh` 的 `update` 子命令加 `--expect-status <S>`，语义与 `--expect-epoch` 同构（锁内重读 `status`，不符即 DENY） | 🔴 **做** | 待决 |
| 2 | validator 加审计：归档目录在 sprint tag 打出后不应再有改动（`git diff <tag> HEAD -- .arcforge/archive/<同名目录>` 非空即告警） | ⚠️ **做**（审计而非拦截） | 待决 |
| 3 | validator 加审计：`transitions.jsonl` 里出现过、但 `write-matrix.json` 的 `tokens` 里没登记的实例名 | ⚠️ **做**（审计而非拦截） | 待决 |
| 4 | CLAUDE.md 的派验固定动作加一句「请回一句确认收到」 | ✅ **做**（零成本，已实测 4/4 有效） | 待决 |

## 各条的一句话理由

1. **`update` 没有乐观锁** ⇒ 状态切换会切在 dev 的「读」与「写」之间。M1c-4 连续五次「`dev_done` 后补充落空」，最后一次窗口只有 4 分钟。
   （`decisions-leader.md` → grep `expect-status`）
2. **`>>` 重定向能绕过 write-guard** 写进 `.arcforge/`（含归档目录）。不建议堵——堵它要么做完备语法分析、要么禁掉 Bash 工具。
   （→ `## 2026-09-03 · 🔴 第三个写通道缺口`）
3. **`writers` 用通配（`qa-*`）× token 只对已登记名字生效** ⇒ 换个没登记过的名字就能写。射程是全体角色。
   （→ `## 2026-09-02 · 🔴 两个机制缺口，都由 QA 主动报告而非利用`）
4. M1c-4 前两次派验不加这句，丢了 **48 分钟**和 **138 分钟**；加了之后 **4 次全部当场回执**。
   （→ `## 2026-09-02 · ✅ 派验要求回执`）

## 明确不做

- **不堵 `>>`、不堵未登记名字** —— 用审计代替拦截：**不禁止变化，只保证变化不会被静默吞掉**
- **不给 `in_progress` 加 `stale-dispatch` 阈值** —— 刻意的，dev 正常干活 p90 就 65 分钟
- **不改 `git log --grep` 的实现** —— 判据走文档：按 sprint 起点截断 + 只看 `%s`（标题），因为它可能命中 body
