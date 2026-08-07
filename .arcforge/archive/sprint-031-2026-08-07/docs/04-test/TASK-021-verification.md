# TASK-021 验证报告（五家补「非 policy 错误的链保留」守护）

- 验证者：test-agent-17
- 交付：`3c0299b`（5 个 `_test.go`，**+212 / -0**，无源文件）
- 基线锚：**`f6a78afeba34b152922da818c58f3c1f04f167a1`**（`3c0299b` 的父提交）
- 承接时 `assignment_epoch`：**1**
- 裁决：**verified**

**已按 Leader 在派验这一刻落盘的订正判**（我核对过文件原文，两处订正确实在 task JSON 里）：
`non_functional[0]` 为「**五包**」；`error_handling[0]` 中「yahoo 与 lixinger 判据不同」已标作废，
**真正不同的是 crypto**。未按旧文本核。

## 一、基线 vs 交付（口径：**单包**）

| 包 | 覆盖率 | `=== RUN` | SKIP | FAIL |
|---|---|---|---|---|
| yahoo | 89.9% → **89.9%** | 92 → 96（+4：1 父 + 3 子用例） | 0 | 0 |
| lixinger | 93.4% → **93.4%** | 69 → 70 | 0 | 0 |
| eastmoney | 87.6% → **88.1%** | 42 → 43 | 0 | 0 |
| crypto | 75.0% → **75.0%** | 60 → 61 | 0 | 0 |
| baostock | 96.4% → **96.4%** | 31 → 32 | 0 | 0 |

五包 `-race` 全绿；`go build -buildvcs=false ./...` exit 0；`go vet ./internal/collector/...` exit 0；
全仓回归 **62 包 ok / 0 FAIL**；5 个交付文件 `gofmt` 全过。

**「生产代码零改动」我按 md5 逐文件核，不采信自报**：`yahoo/yahoo.go`、`yahoo/eps.go`、
`lixinger/client.go`、`eastmoney/eastmoney.go`、`crypto/crypto.go`、`baostock/collector.go`
六个源文件与基线**逐字节一致**。

**「既有测试一字不改」结构上即成立**：5 个文件的 diff 全是 `+`，**删除行为 0**。

## 二、完成标准覆盖矩阵

| # | 完成标准 | 对应测试 | 我注入的变异 → 结果 | 判定 |
|---|---|---|---|---|
| functional[0] | 判据统一 `errors.As(*json.SyntaxError)` + `not-json` 式 body（crypto 例外） | 五包同名 `TestNonPolicyErrorChainPreserved` | 见下逐包 | PASS |
| functional[1] | 逐包现读判据，不照搬 | — | crypto 用 `errors.Is(errProbeUpstream)`、另四家用 `errors.As(*json.SyntaxError)`，且注释写明「现读实测本包解码方式」 | PASS |
| functional[2] | 变异判据：调用点 `%v` 多包一层，新断言须红而 `PassesThrough`/`NonPolicyErrorsUnaffected` **仍绿** | — | 五包变异下**恰好只红新测试**，两条既有守护保持绿 | PASS |
| functional[3] | 范围为五家 | — | 五包各有一条新测试，命名一致 | PASS |
| functional[4] | 三处结构差异（crypto 注入点不同／两处假注入点／判据与 body 配套） | — | 逐条核实，见第四节 | PASS |
| boundary[0] | 探针先在基线确认为真 | — | 五条新测试在交付态全绿（格A），变异后全红（格B） | PASS |
| boundary[1] | 不得改动生产代码 | — | 六个源文件 md5 与基线一致 | PASS |
| boundary[2] | 既有测试一字不改，只追加；不碰 twelvedata/tushare | — | diff 零删除行；交付未触碰那两包 | PASS |
| error_handling[0] | 新断言锚定本包真实会产生的上游可判定错误 | — | 四家锚 `*json.SyntaxError`（真实解码路径）；crypto 锚测试哨兵，**Leader 已裁定可接受**并写进 DoD | PASS |
| non_functional[0] | 五包 `-race` 绿、0 SKIP、覆盖率不低于水位、注明口径 | — | 见第一节，口径已注明**单包** | PASS |

## 三、逐包对抗变异（全部由我自己注入，未采信 Dev 任何一条输出）

变异统一形态：**调用点用 `%v` 多包一层，映射函数一个字不动**。crypto 例外（见下）。

| 包 | 注入点 | 变异后整包转红的测试 |
|---|---|---|
| lixinger | `client.go:51 return nil, mapPolicyErr(err)` | **仅** `TestNonPolicyErrorChainPreserved` |
| eastmoney | `eastmoney.go:441 return nil, mapPolicyError(symbol, err)` | **仅** `TestNonPolicyErrorChainPreserved` |
| baostock | `collector.go:51 return nil, mapPolicyError(symbol, err)` | **仅** `TestNonPolicyErrorChainPreserved` |
| crypto | **那条裸 `return nil, err`**（本包无 `mapPolicy*` 函数，映射是内联的） | **仅** `TestNonPolicyErrorChainPreserved` |
| yahoo | 三处齐注（`yahoo.go:243` / `yahoo.go:317` / `eps.go:54`） | **仅** `TestNonPolicyErrorChainPreserved` 的**三个子用例全红** |

**「恰好只红新测试」这一点很重要**：既说明新断言确实是唯一守护者（缺口是真的），
也说明它没有附带打中别的东西（没有把别的失效混进来）。

### yahoo 逐调用点分辨率（我另跑三次）

| 只改 | chart | quote | eps |
|---|---|---|---|
| `yahoo.go:243`（quote） | PASS | **FAIL** | PASS |
| `yahoo.go:317`（chart） | **FAIL** | PASS | PASS |
| `eps.go:54`（eps） | PASS | PASS | **FAIL** |

**三格完全独立，无串扰。** 漏包任何一处都会被恰好对应的那一格抓到。

## 四、Leader 点名要我核的三处

### ① 五家统一新增独立测试函数，而非在既有表驱动用例里加字段 —— 判断成立

既有 case 结构体确为**位置初始化**（如 yahoo 的 `{"HTTP 400", handler, "unexpected status"}`），
加字段会强制改写全部既有 case 行，撞上 `boundary[2]`「场景一字不改」。
**两条 criteria 字面冲突时它选了更严的那条**，且结果在 diff 上可验证：**零删除行**。

顺带：这个选择也让 eastmoney/baostock 那两处「没有既有格可挂」的问题自然消解——
它们本来就必须新写。

### ② crypto 的哨兵与「不得引入人造错误类型」的张力 —— 按 Leader 裁定核，PASS

`errProbeUpstream = errors.New("crypto probe: upstream down")` 是**包级具名哨兵**，
`gate_test.go:496` 声明、仅在新测试内使用。**既有 `TestErrorIsNotCached:447` 仍用内联
`errors.New("upstream down")`，一字未改**（我直读核实，非仅凭 diff）。

裁定理由我认可并复核过：被守护的属性是「调用点是否保链」，**与错误的具体类型无关**——
换成真实的 `*json.SyntaxError` 也是同一条断言，强度不变。故这不是「为可断言而造类型」。

### ③ 新 harness 直接产出断链的类型证据 —— 属实

失败消息里带 `%T`，变异下直接给出 `(*errors.errorString)`。
**这确实把「再推断一次」压掉了**：旧形态只给一行 `--- FAIL`，读的人还得自己想「为什么断了」。

## 五、DoD 里那三处结构差异，我逐条核实

| DoD 声称 | 我的核实 | 结论 |
|---|---|---|
| crypto 无 `mapPolicy*`，变异注入点是那条裸 `return nil, err` | 现读 `crypto.go:155-163` 确为内联；我按这个点注入，转红 | 属实 |
| baostock `TestFetchDailyBadJSON` 是**假注入点**（直测 `FetchDaily`，绕过 Gate 与 mapper） | 交付的新测试走的是 `c.FetchHistory(...)`；且我注入变异时该既有测试**照常 PASS**，正面印证它不在受守护路径上 | 属实，且已避开 |
| eastmoney 既有畸形 JSON 格在 `FetchQuote` 上，而只有 `FetchHistory` 经 Gate | 交付新测试走 `e.FetchHistory(...)` | 属实，且已避开 |
| 判据与 body 配套，五包既有 body 全是非法字符式 ⇒ 全落 `*json.SyntaxError` | 与我在 `TASK-021-premeasure.md` 的实测一致；交付四家用的 body 分别是 `not-json`/`{not json`/`not json` | 属实 |

## 六、观察项（不影响判定）

1. **crypto 的覆盖率停在 75.0%**（五包最低，且新增测试没抬高它）。其 `coverage_floor` 是 70，
   不违反任何 criteria；但它与另四家（88~96%）差距明显，值得终审留意是否有大块未覆盖代码。
   本次未展开——不在 021 范围内。
2. **五包新测试同名 `TestNonPolicyErrorChainPreserved`**，命名一致，便于将来横向 grep。
   这是好事，登记备查。

## 七、原始产物指针

- 任务文件：`.arcforge/tasks/TASK-021.json`
- 上游：`.arcforge/docs/04-test/TASK-018-verification-addendum.md`（缺口的发现与对抗变异形态）、
  `.arcforge/docs/04-test/TASK-021-premeasure.md`（派发前量定）、
  `.arcforge/docs/04-test/sprint-consistency-survey.md`（八家横表）
- git ref：`3c0299b`（基线 `f6a78afeba34b152922da818c58f3c1f04f167a1`）
- 复现命令（锚一律钉全 sha，不得写 `HEAD`/分支名）：

      git worktree add --detach ../wt-v021 3c0299b
      # 变异：把各包调用点的 return nil, mapPolicyErr(err)
      #       换成 return nil, fmt.Errorf("<pkg>: %v", mapPolicyErr(err))
      #       crypto 改那条裸 return nil, err
      # 预期：恰好只有 TestNonPolicyErrorChainPreserved 转红
      for p in yahoo lixinger eastmoney crypto baostock; do
        GOTOOLCHAIN=local go test ./internal/collector/$p/ -count=1 -cover
      done
