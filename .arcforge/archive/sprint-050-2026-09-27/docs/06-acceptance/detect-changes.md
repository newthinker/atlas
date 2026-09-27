# GitNexus detect-changes（Leader，QA 阶段）

- 索引：重建于 2026-09-27 16:32，Indexed commit `7234cd3`（此前停在 `072f82d`，早于本特性全部提交——dev 在各任务提交前跑的 `detect-changes --scope staged` 因此看不到 bank 代码，结论无效）。
- 命令：`node .gitnexus/run.cjs detect-changes --scope compare --base-ref master --repo .`
- 结果：Changes 24 files / 324 symbols；Affected processes 11；**Risk level: high**。

## ⚠ HIGH 风险——如实上报，不以推理豁免
CLI 输出两段被截断（`... and 309 more` 符号、`... and 1 more` 流程；`--limit 1000` 无效）⇒ 单凭 CLI 输出**不是干净检查**。闭合方式：
1. `cypher` 查以 bank 命名的 Process：**恰 11 个**，全部以 `RunBankReport →` 开头（CLI 列出的 10 个 + 被截的 `RunBankReport → EMSource`）。11 = Affected processes 11 ⇒ 受影响集合里**没有**任何既有流程。
2. `git diff --name-status master...feature`：**23 个文件全部为 A（新增）**，删除行 0 ⇒ 没有任何既有符号被修改（工具报 24 files 与 git 的 23 差 1，未查明来源，记为未闭合的小差异）。
3. 结论：HIGH 来自「新增一个带 11 条执行流的命令」这一规模，受影响流程**全部是本特性新建的**；未发现对既有流程的影响。**但按项目规则 HIGH 不得自行豁免，交人类知悉。**
