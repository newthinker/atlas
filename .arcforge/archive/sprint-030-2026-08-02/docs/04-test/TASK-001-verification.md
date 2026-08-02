# TASK-001 验证报告(复验轮 2 / review_fix 轮次 2)— S2 窗口无关性守护

- **验证者**: test-agent-14 (Reality Checker 模式)
- **验证时间**: 2026-08-02T08:15Z
- **判定**: **PASS → status=verified**(已 jq 直读核实)
- **epoch**: 2(`--expect-epoch 2` 通过)/ **rework_count**: 2
- **本轮范围**: 仅 S2 一条(上轮我报的唯一存活突变)+ M5/S1 与首轮 8 条 DoD 回归。
- **验证环境**: 本轮起改用 **git worktree**(`../wt-verify-T001`,detached @ fe7e1f6 + 覆盖未提交改动),**主仓零派生目录**,不再干扰他人门禁。

## 1. S2 判定:上轮存活突变已被封堵(独立复跑确认)

我按指派**独立复跑**了突变,不采信 dev 的自报数字。结果与 dev 所述**完全一致**:

| 突变变体 | 注入内容 | `TestErrRateLimitedIsWindowAgnostic` | 旧 `TestErrRateLimited` | 结论 |
|---|---|---|---|---|
| **C**(上轮存活的那个) | `rateLimitMarker` 写死成完整串「频率超限(1次/分钟)」 | **3 子用例红 2**(1次/小时 ✗、200次/天 ✗;1次/分钟 ✓) | **仍绿** | ⭐ **上轮存活突变现已被杀死** |
| **C2**(我另加的) | 按**已知窗口枚举**(匹配「1次/分钟」**或**「1次/小时」) | **仅 200次/天 子用例红** | 仍绿 | ⭐ 第三个「未出现过的窗口」用例**独立有价值** |

两点值得记下:

1. **C 变体正是上轮的存活突变**,现在能被杀死 —— S2 的全部价值(守住「判据与窗口口径无关」这条性质)已兑现。旧 `TestErrRateLimited` 在两种突变下都仍绿,说明新用例不是靠改旧用例取胜,而是**净增了一层守护**。
2. **C2 变体证明了「200次/天」这个上游未出现过的窗口不是凑数**:如果只用「1次/分钟 + 1次/小时」两个已知窗口,一个「按已知窗口枚举」的错误实现(这是很自然的退化写法)**能全身而退**;正是第三个未见窗口把它挡下。dev 加这一条的判断是对的。

## 2. 回归确认

worktree 内亲跑,**18 个顶层用例全 PASS,0 FAIL**,覆盖率 **94.2%**(与 dev 自报一致):

```
TestFetchDailyBasic / TestFetchPriceAPIs / TestErrNoPermission / TestBusinessError
TestNetworkError / TestFetchDailyBasicEmpty / TestThrottleMinInterval (0.20s)
TestDefaultBaseURLIsHTTPS / TestErrRateLimited / TestErrNoPermissionStillPermanentWhenNotRateLimited
TestErrRateLimitedIsWindowAgnostic  ← S2 新增
+ Collector 适配层 7 个用例
顶层 PASS: 18   FAIL: 0   coverage: 94.2%
```

- **M5 回归**:`client.go:22` 仍为 `https://api.tushare.pro`,`TestDefaultBaseURLIsHTTPS` 绿。
- **S1 回归**:`client.go:39` 仍为 `const rateLimitMarker = "频率超限"`(四字,未退化),两哨兵互斥用例绿。
- **首轮 8 条 DoD 行为**:升序、空 items、业务错误文本用例均绿;`TestThrottleMinInterval` **仍实耗 0.20s** ⇒ 节流未破坏。
- `go vet` exit=0;`gofmt -l` 无输出。
- **`client_test.go` 本轮 diff 零删除行(纯新增)** ⇒ 既有用例无一被改动或弱化;`client.go` 本轮**未被改动**(相对 HEAD 的 23+/4- 即上轮已验收的 M5+S1)。
- 密钥哨兵:2 个文件、3 个长 key,**0 命中**;管道有效性已自检。

## 3. 观察项

无新增。上轮报告的观察项 2/3/4(港股 %05s 归一按裁决不做、限频 Degraded 文案由 TASK-005 收口、两处跨包建议待路由)状态不变,其中 Degraded 文案一条已在 TASK-005 本轮 fix 中处理(见 TASK-005 报告)。

## 4. 判定

**S2 PASS。** 上轮唯一存活突变已被封堵,且新用例的第三个 case 经我另设的 C2 变体证明有独立价值。M5/S1 与首轮 8 条 DoD 行为回归全部保持。
证据为 worktree 内亲跑 + 2 个突变变体独立复跑 + diff 零删除行的机制化核实,非采信 dev 报告文字。
`status=verified` 已落盘并经 `jq` 直读核实(`verifying→verified by test-agent-14 @ 2026-08-02T08:15:50Z`,epoch=2 / rework_count=2)。
