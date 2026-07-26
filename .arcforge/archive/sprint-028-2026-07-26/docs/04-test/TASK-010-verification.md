# TASK-010 验证报告（cmd 分部刷新接线 + Report 合并）

- 验证者: test-agent-6 ／ assignment_epoch: 1 ／ 交付 commit: `0981d90` + `75bbe98`
- **判定: VERIFIED（7 条 done_criteria 全部通过）**

## 1. 实跑证据

```
GOTOOLCHAIN=local go vet ./cmd/atlas/ ./internal/prism/   → ok
go test ./cmd/atlas/ ./internal/prism/ -count=1 -cover
  ok cmd/atlas       1.215s  coverage: 74.6%
  ok internal/prism  0.964s  coverage: 95.5%
go build ./... ok；go test ./... -count=1 全仓绿
```

新增函数覆盖率**全部 100%**：`mergeReports` / `prefixAll` / `segmentReport` / `runPrismRefreshWith`。
唯一 0% 是既有装配壳 `runPrismRefresh`（cobra RunE，做真实配置装载与网络 client 构造）。
**dev 没有为凑覆盖率给生产入口壳造 seam** —— 与 Leader 的放行复核一致。

**gofmt**：`cmd/atlas/backtest_test.go` 与 `crisis_test.go` 未格式化，但经 `0981d90^` worktree 实测
**改动前就已如此**（源自无关提交 `f5d7b82`），本任务三个文件 `gofmt -l` 为空。非本任务引入。

## 2. done_criteria 逐条覆盖矩阵（判据：变异体死没死 + **红对地方**）

| # | 完成标准 | 对应测试 | 变异验证（含失败信息核对） | 判定 |
|---|---|---|---|---|
| F0 | 模板非空时调用 RefreshSegments；两个 Report 的 Failed 与 Degraded 均在合并结果中 | `TestMergeReportsKeepsBothSides`、`TestPrismRefreshShowsBothFailureSources` | M2 Degraded 不合并 → 杀死，信息 `"[...]" should have 2 item(s), but has 1`；M4 前缀丢失 → 杀死，信息 `"MSFT: sec 503" does not contain "segments" ｜ 分部条目须可与估值条目区分` | **PASS** |
| F1 | AD-12 `--full-segments` 透传 force=true | `TestSegmentReportPassesForceAndManualDir`（**正负成对**：force=true→`since.IsZero()`；force=false→`since=="2026-03-31"` 锚点）、`TestFullSegmentsFlagRegistered` | M6 force 恒 false → 杀死，信息 `Should be true ｜ force=true 须忽略锚点全量重拉`；M7 templates 传 nil → 杀死，信息 `有模板时须发起分部拉取` | **PASS** |
| F2 | AD-16 LoadTemplates error 不得丢弃（负向：不得静默） | `TestSegmentReportBadTemplateDegradesNotSilently` | M5 error 被吞 → 杀死，信息 `"[]" should have 1 item(s), but has 0 ｜ 模板加载失败必须可观测` | **PASS**（口径见 §3） |
| B0 | 目录不存在/为空 → 跳过且不报错；**测试不得依赖真实 configs 目录** | `TestSegmentReportSkipsWhenNoTemplates` 两子例（目录不存在／目录为空），断言零值 Report **且** `seg.called == false`；用 `t.TempDir()` 而非真实 configs | M10 见 §4（等价变异体） | **PASS** |
| B1 | 合并输出 ok 数不得超过标的总数 | `TestMergeReportsDoesNotInflateOkCount` | M1 `Refreshed` 相加 → 杀死，信息 `Refreshed 不得相加(3+2=5 会超过 3 个标的的总数)` | **PASS** |
| E0 | 估值 Failed 与分部 Failed 同时可见、不互相覆盖 | `TestMergeReportsKeepsBothSides`、`TestPrismRefreshShowsBothFailureSources` | M3 Failed 覆盖而非拼接 → 杀死，信息 `"[segments: MSFT: sec 503]" should have 2 item(s), but has 1` | **PASS** |
| N0 | 全绿；TASK-004 行为不回退；汇总行格式不变 | `TestPrismRefreshAlwaysPrintsDetail` 仍 PASS；4 处 `assert.Contains(out, "prism refresh: N ok, N failed, N degraded")` 逐字锁定格式 | M11 `Fprintln` 挪回 `sender==nil` 分支 → 杀死，信息 `有 sender 时 Failed 明细仍须进 out` | **PASS** |

**新判据（test-agent-7 提炼）已逐条执行**：不只看「有测试红了」，而是核对
**① 被点名的测试在变红列表里；② 失败信息指向该规则本身**。上表每格都附了失败信息原文，
无一是被连带效应误伤后凑数的。

## 3. 两条口径裁决的核实

**① F2 只断言 Degraded 侧、未单独断言 `errOut` —— 我同意 Leader 的裁定。**
读实现确认：`segmentReport` 把加载错误降级为 `Degraded` 条目，经 `runPrismRefreshWith`
打到 **`out`**。这与 TASK-004 确立的语义一致（Failed/Degraded 明细无条件进 `out`，
`errOut` 只承载「通知发送失败」这类元错误）。AD-16 的核心是**不得静默吞错**，
错误文本含 `broken.yaml` 且必然出现在 stdout，可观测性达成。**未因「没写 errOut」扣分。**

**② `manualDir=""` 变异的注入点局限 —— 处置合理，且守护实际有效。**
真实传参点在 0% 覆盖的装配壳里，那里的变异本就抓不到。dev 退而锁常量：
- M8 `sankeySegmentsDir` 置空 → **杀死**，信息 `manual 兜底目录不得为空:空值会静默禁用 manual 覆盖`
- M9 `sankeyTemplatesDir` 置空 → **杀死**，信息 `模板目录不得为空:空值会让分部刷新整体静默跳过`

我原本预告要打的正是 `manualDir==""` 这个静默失效点（`segments.go:249` 直接 `return nil`），
**锁常量确实堵住了它**。为过门禁给入口壳造 seam 会让指标反向驱动设计，这个取舍我认可。

## 4. 唯一存活变异体 M10：**等价变异体，非缺口**

M10 = 删掉 `segmentReport` 里 `if len(templates) == 0 { return prism.Report{} }` 的提前返回。

**实证判定**（不止于推理）：
- 原实现与变异实现分别跑全部 `cmd/atlas` 用例，`=== RUN` / `--- PASS|FAIL` 结果集
  **逐行 diff 仅有耗时抖动**（0.00s ↔ 0.01s），无任何用例结果差异；
- 读 `RefreshSegments` 确认：空 templates map 下每个标的都 `templates[symbol]` 取不到 → `continue`，
  **零请求、零值 Report、无副作用**，与提前返回可观测等价。

该提前返回是短路优化而非行为守卫。DoD boundary[0] 要求的「跳过分部刷新且不报错」
两种写法都成立（`seg.called == false` 在两种实现下都为真）。

## 5. 两个顺带项：均**目的达成**（不止于「改了」）

**① `cmd/atlas/prism_test.go` 的 `assert.Len` → `require.Len`**（我在 TASK-004 发现的 panic 陷阱）
验的是目的而非改动本身：施加使 `sender.sent` 为空的变异后——

```
panic 出现次数: 0
变红用例: NotifiesOnPartialFailure, WarnsOnSendFailure, NotifiesOnDegraded,
          WarnsOnSendFailureDegraded, AlwaysPrintsDetail, PrintsDetailEvenWhenSendFails,
          ShowsBothFailureSources      ← 7 个用例各自正常报告失败
```
**掩盖效应已消除**：此前同类变异会在第一个用例 index 越界 panic、中断整个二进制，
后续用例结果全部不可见（我在 TASK-004 做变异测试时真被误导过一次）。

**② X16 确定性守护**：施加「`sortedKeys` 去排序」变异连跑 **30 次 → 30 次全红，存活率 0%**
（加固前实测 30 次存活 4 次 = 13%）。dev 改用**用例内循环 20 轮**（存活率 `p^N`，
不依赖对 p 的任何假设）而非我建议的 member 3→5，**该偏离正确，见 §6**。

## 6. 我的概率模型被证伪 —— 独立复现并给出正确机制

我在 TASK-008 复验中建议「member 3→5，偶然升序概率由 1/6 降到 1/120」。
**该模型错误**，我自己写探针复现（10 万次采样），数字与 dev/Leader 一致：

```
n=3  实际顺序种类=3（全排列应为 6）    升序占比 74.75%   ← 不是 16.67%
n=5  实际顺序种类=5（全排列应为 120）  升序占比 49.83%   ← 不是 0.83%
n=8  实际顺序种类=8                   升序占比 12.49%
n=9  （跨桶）                          升序占比  0.00%
```

**进一步坐实了具体机制**（打印各顺序频率验证）：
Go 对单桶小 map 在**桶的 8 个 cell** 上取随机起点（**不是在 n 个已填槽上**），
扫描时跳过空槽并回绕。已填槽位于 0..n−1 时，起点 r ∈ {0} ∪ {n..7} **都**产出插入序，
故 **P(升序) = (9−n)/8**：

| n | 预测 (9−n)/8 | 实测 |
|---|---|---|
| 3 | 75.0% | 75.05% |
| 5 | 50.0% | 50.09% |
| 8 | 12.5% | 12.37%（其余 7 种旋转各 ≈12.5%） |

**结论**：我建议的方向（增加 member）是对的，**量级完全错**——3→5 只把单次升序概率
从 75% 降到 50%（与 Leader 观察的「存活率 13%→10%」吻合），远非 1/120。
**dev 改用循环 20 轮是更好的方案**：存活率 `p^20` 与 p 的真实取值无关，
不依赖任何随机源模型。实测 30/30 全红即为验证。

**教训**（我与 Leader 独立算出的数都错，且错向一致）：双方用的是**同一个未经检验的前提**
（「map 迭代 ≈ 均匀随机排列」）。这推翻了「两人独立得出同一结论 = 强证据」——
该强度**只在双方前提互相独立时成立**。

## 7. 判定

**VERIFIED** —— 7 条 done_criteria 全部通过；11 个变异体 10 杀 1 等价（M10 经实证判定），
每条杀死均按新判据核对了「红对地方」；两个顺带项均验证目的达成而非仅确认改动；
新增函数覆盖率全部 100%，未为凑指标给入口壳造 seam。
