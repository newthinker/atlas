# TASK-009 验证报告（test-tg-b）

- 任务：拆股折算与截取 normalize.go（internal/collector/tiingo）
- 判定对象：`b6c6a25740823b5e1b02f67e084bf6e7ad7f4f90`（= verify_baseline.head，= 验证时主仓库 HEAD）
- 被验文件在 baseline 树上与 dev 提交 `ce80996e32f1c8da5755232d49359baf81c9f3c0` 逐字节一致（`git diff --stat ce80996 b6c6a25 -- <3 个文件>` 空输出）
- dev 提交 ce80996 只改 3 个文件，全在 `writes` 内，无越界
- discovery sha256 = `624484db…d763`，与 verify_baseline.discovery_sha256 一致
- 环境：隔离 detached worktree（测试一个、变异另一个），`GOTOOLCHAIN=local`

## 结论：VERIFIED

## done_criteria 覆盖矩阵

| # | 完成标准 | 对应测试 | 守卫证据（变异） | 判定 |
|---|---|---|---|---|
| functional[0] | NVDA [06-05,06-11]：5 根、升序、NVDA/1d；06-05 O/H/L/C=122.44/123.44/121.44/122.44、Vol 10000；06-10 不折算 Close 121.79、Vol 10000 | TestNormalizeNVDASplitAndClip | M1 Open 不折算、M18 High、M19 Low、M16 Interval、M20 Symbol、M7 含 t 自身因子 均被杀 | PASS |
| functional[1] | 先折算后截取：[06-03,06-07] 首根 Close 115.0、末根 120.888 | TestNormalizeEndBeforeLaterSplitStillAdjusted | M17 先截取后折算 只被这条杀 | PASS |
| boundary[0] | volume=3、其后 sf=1.5 ⇒ Volume 5，价格 ÷1.5 | TestNormalizeNonIntegerFactorRoundsVolume | M2 Volume 截断 只被这条杀 | PASS |
| boundary[1] | 乱序输入仍严格升序、按日期累乘，与有序结果逐根相等 | TestNormalizeUnorderedInput（倒序 + 打乱两子测试，逐根比 Time/O/H/L/C/Volume） | M4 去排序 被杀 | PASS |
| boundary[2] | 坏日期 / close null / O·H·L 任一 null 不输出；volume null 记 0；sf null 视为 1；拆股当日价格全 null（sf=2）此前 100→50 | TestNormalizeSkipsBadRows（含 2 个子测试） | M6 null 因子视 0、M8 缺价行丢因子、M11 volume null 记 1 被杀 | PASS |
| boundary[3] | 按日闭区间、带时分秒端点按日截断、0 根返回空切片与 nil 错误 | TestNormalizeClipInclusiveByDay | M10 lo 不截断、M15 hi 开区间、M9 空结果返回 nil 被杀；M5 见下 | PASS |
| error_handling[0] | sf 为 0、-2 整段失败：nil 切片，错误含 `invalid splitFactor` 与该行日期 | TestNormalizeRejectsInvalidSplitFactor（0/-2/NaN/+Inf，坏因子行在窗口外） | M3 sf<=0→sf==0、M12 错误无日期、M21 错误无关键字、M13/M14 被杀 | PASS |
| non_functional[0] | race 全绿；gofmt/vet 无输出；包覆盖率 ≥80% | 实跑（见证据） | — | PASS |

## 证据

```
$ go test -count=1 -race -cover ./internal/collector/tiingo/      @ b6c6a25
ok  github.com/newthinker/atlas/internal/collector/tiingo  1.399s  coverage: 97.0% of statements
gofmt -l internal/collector/tiingo/  → 空
go vet ./internal/collector/tiingo/  → rc=0 无输出
go test -v -run TestNormalize → 15 PASS / 0 FAIL（7 个顶层 + 8 个子测试）
只跑 TestNormalize* 时 normalize 函数覆盖率 96.8%
```

### 独立数值验算（Python Fraction 精确有理数，按 spec §2.4 自己实现，不看测试期望值）

t 日 OHLC ÷ t 之后（不含 t）所有 splitFactor 之积，Volume × 同积四舍五入：

```
2024-06-03 (115.0, 116.0, 114.0, 115.0) vol 10000
2024-06-05 (122.44, 123.44, 121.44, 122.44) vol 10000
2024-06-07 (120.888, 121.888, 119.888, 120.888) vol 10000
2024-06-10 (121.79, 122.79, 120.79, 121.79) vol 10000
[06-05, 06-11] 内根数 5
```

与 DoD 数字、测试期望逐项一致。boundary[0] 手算：3×1.5=4.5（二进制下精确），Round=5、截断=4；价格 3/1.5=2。

### 独立变异（隔离 detached worktree，脚本 scratchpad/test-tg-b-TASK-009-mut.py；每个变异先 go vet 确认可编译，匹配次数须 ==1，收尾校验源文件还原）

**20/22 KILLED**，其中 Leader 点名的 4 个（M1 Open 不折算、M2 Volume 截断、M3 sf<=0→sf==0、M4 去排序）全部被杀，且杀死它们的是对应 DoD 条目的测试。dev 自报的 12/12 没有照搬，这里是独立复核。

存活 2 个，均判为等价变异，不是测试缺口：
- **M5 `hi := end`（end 不按日截断）**：bar 时间恒为 UTC 零点。对 UTC 的 end 来说，`t ≤ end` 与 `t ≤ trunc(end)` 是等价的（trunc(end) ≤ end < trunc(end)+24h），所以 DoD 范围内没有测试能区分二者。只有 end 带非 UTC 时区时两者才有差别（例如 06-10 00:00 +08:00：原实现包含 06-10，变异排除它）。DoD 和 spec 都没规定时区语义，所以不算本任务的缺陷。**提示 TASK-004**：如果调用方可能传入非 UTC 的 start/end，要在那一层明确口径。
- **M22 短日期补字符**：这是我设计的变异本身有问题（补齐后 Parse 照样失败、行照样被跳过），属于等价变异。长度检查的真正作用是防 `[:10]` 越界 panic，测试里的 `"bad"`、`"2024-6"` 已经覆盖。

## 观察（不影响判定）
- 错误文本带 `symbol:` 前缀（`NVDA: invalid splitFactor -2 on 2024-06-20`），与 discovery 的「无 tiingo: 前缀，由 TASK-004 wrapErr 包装」一致。
