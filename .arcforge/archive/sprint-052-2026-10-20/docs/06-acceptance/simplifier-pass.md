# code-simplifier 统一补跑（AD-6，Leader，2026-10-20）

- 起点：`5fc087c921c58c7e23dab9d5dc1b27bb5ac5fcfb`；范围：`git diff --name-only fb1fc62..HEAD -- '*.go'` 共 24 个文件
- 判据：改动前后 sha256 指纹比对（不采信子代理自述）
- 结果：**3 个文件变化**，与子代理自述一致；其余 21 个逐字节不变

    internal/pricewatch/advisor.go: FAILED
    internal/pricewatch/message.go: FAILED
    internal/pricewatch/window.go: FAILED

## 改动（纯提取，输出不变）
- `window.go`：删私有 `listingKey`，改用 TASK-004 导出的 `ListingKey`（核实两者均为 `"listing:" + pair + ":" + bar`）→ 提交 `0dbad5f` refactor(TASK-010)
- `message.go` / `advisor.go`：抽出 `directions`、`refVerb`，消除 alertIcon/Sides 与 triggerLines/buildAdvicePrompt 的重复判断 → 提交 `53d255a` refactor(TASK-005)（含 TASK-006 的 advisor.go）

## 未合并（子代理判断，Leader 同意）
- sync.go 的 localCoverage 与 window.go 的 GapRatio 口径不同（向下/向上取整、末尾不足一槽、无 K 线报 1），合并会改变一侧边界结果（与 test-pw-a 验 TASK-004 §6(3) 结论一致）

## 复核（Leader 在 `53d255a` 上）
- `go vet ./internal/pricewatch/ ./cmd/atlas/` 无输出；`go test -race -count=1 ./internal/pricewatch/ ./cmd/atlas/` 两包 ok；`gofmt -l` 无输出；`go build ./...` ok
- gitnexus detect-changes：6 files、无已索引符号重叠——**索引停在 b3232d6，早于整个 pricewatch 包，此结果不是干净信号**；impact 对这些符号为 UNKNOWN，子代理已用全仓文本搜索确认只在 internal/pricewatch 内使用
- ⚠ 这两次提交发生在 11 个任务 verified 之后：TASK-005/006/010 的 verify_baseline 范围内文件已变，验收前漂移核对需显式说明（QA 审查对象含此二提交）
