# QA Review Round 2 — 银行股关键指标月度监控（TASK-006 review_fix 复核）

- 审查者：qa-bk-a（本体直审，本轮**未派** lens 子代理）
- 范围：`git diff 8fdd59ba008f81562b1cf789674cc376ebe99fe7..7234cd3f2e883a7f8d68d99fe749064ce6ff74dc`，只有两个文件：cmd/atlas/bank.go（+11/-4）、bank_test.go（+28/-9）
- 取证树：`git rev-parse HEAD` = `7234cd3f2e883a7f8d68d99fe749064ce6ff74dc`；`git diff --quiet 7234cd3f -- cmd/atlas internal/bank` 成立，说明被测文件与该 sha 逐字节一致
- 变异手法：`go test -overlay`，变异体放在 scratchpad `qa-bk-a-r2/`，主仓库零改动（事后 `git status --porcelain` 除 `.arcforge/` 外为空）

## 0. 订正第 1 轮报告

第 1 轮（qa-review-round1.md）的「审查方式与降级说明」写道 3 个 lens 子代理的派发「被用户中断」，**这是错的**。实际是 **Leader 用 TaskStop 停掉了它们**：它们已经写完结论文件，却被 idle hook 以 qa-bk-a 的名义循环催写 verdict。这不是人类的动作。第 1 轮的结论不受影响：结论本来就是从三份已写完的文件里取的，MEDIUM 也都由本体复核过。

## 1. FIX-1 复核：生产路径部分失败 ⇒ bankExit(2)

- 修复内容：新增子测试 `TestRunBankReportSends/部分失败`，条件是非 dry-run、sender 注入成功、601658.SH 拉取失败。断言有四条：
  - `NoError`
  - `factoryCalls==1`
  - 推送 1 条，且含「· 邮储银行 拉取失败：」
  - stdout 为空，`exits == []int{2}`
- 验收变异（本体独立实测）：
  ```
  $ diff bank.go m1.go
  83c83
  < 	if code != 0 {
  > 	if code != 0 && bankDryRun {
  $ go test -count=1 -overlay m1.json -run Bank ./cmd/atlas/   → rc=1
  --- FAIL: TestRunBankReportSends/部分失败
      expected: []int{2}   actual: []int(nil)
      Messages: 部分失败以 2 调用 bankExit 恰一次，否则 launchd 记为成功
  ```
  第 1 轮时这个变异存活，现在被杀，失败原因是目标断言本身。**FIX-1 关闭。**

## 2. FIX-2 复核：全失败摘要带 H 股别名附注

- 修复内容：失败行后按 `r.Aliases` 追加「  （别名 同 代码）」，格式与 bank.writeAliases 一致；新增 `nFailed` 独立计数，「全部失败」判等、摘要里的「全部 N 家」、错误文案三处都改用它。
- 两个反例（本体实测）：
  - m2：删掉别名循环（改成 `_ = r.Aliases`）⇒ rc=1。`TestBankReportAllFailed` 报「别名附注紧跟其主体」失败，`TestRunBankReportAllFailed` 报「H 股条目不能在摘要里消失」失败。
  - m3：把判等改回 `len(failed) == len(results)` ⇒ rc=1。两个 AllFailed 测试都报「全部失败 ⇒ 退出码 1」失败，说明计数与附注行耦合的回归被守住了。第 1 轮 Architect 提示过这个坑，这次修复规避了它，也加了守卫。
- 断言质量：`TestBankReportAllFailed` 用 `slices.Index` 定位主体行，再断言下一行恰好是附注，比单纯的 `Contains` 更严，能挡住「附注位置错乱」这类回归。**FIX-2 关闭。**

## 3. 回归与新问题检查

```
$ go vet ./cmd/atlas/ ./internal/bank/                  → vet_rc=0
$ go test -count=1 ./cmd/atlas/ ./internal/bank/        → 两包 ok（cmd/atlas 整包不带 -shuffle）
$ go test -count=1 -shuffle={1,2,3} -run Bank ./cmd/atlas/ → rc=0 ×3
```
- 逐行审 diff，未引入新问题：
  - 非全失败路径只把 `len(failed) > 0` 换成了 `nFailed > 0`，语义等价（每个失败主体至少贡献一行，两者同为零或同为非零）。
  - 部分失败时，别名附注行只进 `failed` 切片，而 `failed` 只在全失败分支被使用，所以部分失败的报告正文仍由 Render 生成，不受影响。
  - Checkpoint 注释新增的两行映射与实际测试名一致。
- 残留 LOW（第 1 轮已列，不阻断）：
  - 标题串和别名格式仍在 cmd 层与 render.go 各写一份（第 1 轮 S1/S12）。代码里已注释「与 bank.writeAliases 同格式」，漂移风险由两处测试各自守着。
  - 第 1 轮 S2–S11 这次没有改动，维持 SUGGESTION。

## 4. Verdict

**PASS**。第 1 轮唯一的 WARNING（FIX-1）和建议同轮的 FIX-2 都已修复：本体独立复现了验收变异转红，两个 FIX-2 反例也都成立；本次改动没有引入新问题，CRITICAL/WARNING 均为 0。第 1 轮 S2–S12 的 LOW 建议留待 Leader 决定是否排期。
