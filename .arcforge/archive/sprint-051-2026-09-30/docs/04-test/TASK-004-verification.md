# TASK-004 验证报告（test-tg-b）

# 返工 R1 复验（当前结论）

- 判定对象：`77398b4e54bed75f4a87cd2094ec3d1d73b1e37e`（= verify_baseline.head，= 验证时主仓库 HEAD），即返工提交 `c36ef2b3bcedf4dc3b1f3ee2901421412547c613` 的 merge
- `37027bf..77398b4` 只有这一个返工提交，只改 `client.go` 和 `client_test.go`，都在 `writes` 内
- discovery sha256 = `9cff3c2b…0523`，与 verify_baseline 一致
- assignment_epoch = 2，rework_count = 1
- 环境：隔离 detached worktree（测试一个、变异/探针另一个），`GOTOOLCHAIN=local`

## 结论：VERIFIED

### 修复内容（diff 已读）
- 抽出 `redact()`（空 key 时不替换），`wrapErr` 改为调用它。
- `statusErr` 的 body 分支改为先 `c.redact(string(body))`、再截断到 200 字节并做 `ToValidUTF8`。
- 新增两个用例：`TestFetchHistoryStatusErrorKeyAcrossTruncation`（190 个 x + key，断言 key 的任何 ≥4 字符前缀都不出现）和 `TestFetchHistoryEmptyKeyNotRedacted`。

### Leader 点名三点
1. **我的探针在新代码上不再泄露**：把探针扩展成扫描后，在新旧两棵树上用同一份探针文件背对背跑（探针存于 `scratchpad/test-tg-b-TASK-004-probe_test.go.txt`，跑完删除，porcelain 为空）。扫描维度：
   - 2 种 key：14 字符的 testKey、40 位十六进制 key
   - key 起始偏移 150–215
   - 4 种 body：裸 body、多字节 `é` 前缀（走 ToValidUTF8）、JSON detail 分支、key 出现两次
   - 4 个状态码：400/404/429/500
   
   判据：错误文本含 key 的任何 ≥4 字符前缀即算泄露。
   ```
   77398b4（新）PROBE cases=2112 leaks=0
   37027bf（旧）PROBE cases=2112 leaks=612
   ```
   旧代码上有泄露，说明探针有区分力；新代码为 0。
2. **有没有别的截断或格式化路径发生在脱敏之前**：
   - **detail 取自 JSON 的分支**：detail 不截断，整段进入 `wrapErr`，由 `wrapErr` 对整条消息做 redact，所以安全。探针第 3 种 body 覆盖了这个分支，0 泄露。
   - **ToValidUTF8 或截断把 `<redacted>` 截成一半**：无害。截断发生在替换之后，被截掉的只是占位符的一部分，占位符里不含任何 key 字符。多字节前缀加各种偏移，探针也没有发现泄露。
   - **`io.LimitReader(maxBody=32MB)`**：它先于脱敏截断 body，但非 200 的 body 最终只取前 200 字节，200 的 body 只进 json 解码。json 的错误信息最多带出单个字符，不会带出 ≥4 字符的片段，所以不构成泄露路径。
   - **其余出口**（build request / transport / read body / decode / normalize / Gate 哨兵）都经过 `wrapErr` 对整条消息脱敏，没有截断。首轮已经确认 policy.fetch 原样透传的错误只有两个哨兵错误和 fn 自身的错误。
3. **上轮其余 DoD 无回退**：`-race` 全绿，覆盖率 95.2%，gofmt 和 vet 都没有输出，`-v` 下 FAIL 行为 0。上轮 32 个有效变异在新代码上重跑后全部仍被杀（A25「不脱敏」和 A30「缺前缀」的原写法因代码改动匹配不到了，分别换成 R3/R4 和 A30b）。

### 返工部分的变异（隔离 worktree；脚本 `scratchpad/test-tg-b-TASK-004-mut-r1.py`）
| 变异 | 结果 | 杀死它的测试 |
|---|---|---|
| R1 脱敏挪回截断之后（DoD 点名） | KILLED | 只有 TestFetchHistoryStatusErrorKeyAcrossTruncation |
| R2 空 key 也做替换 | KILLED | 只有 TestFetchHistoryEmptyKeyNotRedacted |
| R3 redact 空操作 | KILLED | StatusErrorsRedacted、KeyAcrossTruncation、ErrorsRedactedAndUnchained |
| R4 wrapErr 不调用 redact | KILLED | StatusErrorsRedacted、ErrorsRedactedAndUnchained |
| A30b 去掉 `tiingo: ` 前缀 | KILLED | 5 个用例 |

合计：**有效变异 35/35 KILLED**。A33 编译不过，属无效变异。

### 覆盖矩阵（R1）
| # | 对应测试 | 判定 |
|---|---|---|
| functional[0] | TestFetchHistoryRequestShape / TickerMapping | PASS |
| functional[1] | SplitNormalizedAndClipped / EmptyBody / NonUTCDates | PASS |
| functional[2] | TestTopicMatchesBuiltinPolicy / TestNewSnapshotsDefaultGate | PASS |
| boundary[0] | CachedAndIndependent / CacheKeyCoversStartAndEnd / ErrorNotCached | PASS |
| error_handling[0] | StatusErrorsRedacted + **StatusErrorKeyAcrossTruncation**（R1 变异由它独家杀） | PASS |
| error_handling[1] | ErrorsRedactedAndUnchained | PASS |
| error_handling[2] | QuotaExceededIsRetryable / TimeoutIsRetryable | PASS |
| non_functional[0] | ClientTransportIgnoresProxy + race/gofmt/vet/95.2% | PASS |

### 观察（不影响判定，留给 QA）
- 脱敏只匹配字面 key。如果原始 body 里的 key 是 JSON 转义形式（如 `\u002d`），或者 `*url.Error` 里的 key 被 URL 转义，都匹配不到。Tiingo token 是十六进制，前者需要服务端刻意转义才会出现，目前属于理论上的边缘情况。
- discovery 的 `verification.commit` 仍写首版 `b66631a`，返工提交记在 `commits` 数组里，不影响判定。

---

# 历史：首轮验证（37027bf，已被 R1 取代）

- 任务：Tiingo HTTP 客户端 client.go（直连、Gate、错误映射）
- 判定对象：`37027bf2e6bb010a3c88bdeba081b00639eda2a6`（= verify_baseline.head，= 验证时主仓库 HEAD），即 dev 提交 `b66631a0c8a94acfd26b6095f29aaf2d4cd327db` 的 merge
- `git diff --stat b66631a 37027bf -- internal/collector/tiingo/` 为空；提交只改 4 个文件，都在 `writes` 内，无越界
- discovery sha256 = `58e9854f…a823`，与 verify_baseline 一致
- 环境：隔离 detached worktree（测试一个、变异/探针另一个），`GOTOOLCHAIN=local`

## 首轮结论：REJECTED（task_defect）

DoD 的 8 条都有测试、全部通过，32 个有效变异全部被杀。但 Review Focus 3「所有错误出口不含 key」有一个出口会泄露部分 key，属于 error_handling[0]「body 里带 key 的情形」的范围。

### 缺陷：body 先截断后脱敏 ⇒ key 跨 200 字节边界时部分泄露

`client.go` 的 `statusErr`：
```go
detail = strings.ToValidUTF8(string(body[:min(len(body), 200)]), "")
...
return c.wrapErr("%s: HTTP %d: %s", symbol, code, detail)   // wrapErr 在这里才做 ReplaceAll(apiKey)
```
截断发生在脱敏之前。如果 key 在 body 中跨过第 200 字节，截断后剩下的是 key 的前缀，与完整 key 不再匹配，`ReplaceAll` 替换不到，这段前缀就原样进入错误文本（按设计，错误文本会经 prism 报告外发到 Telegram）。

**复现**（隔离 worktree 里的一次性探针测试，跑完已删除，porcelain 为空）：body = 190 个 `x` + `testKey`，status 500
```
ERR="tiingo: AAPL: HTTP 500: xxx…xxxsecret-key"   containsFull=false
```
14 字符的 key 泄露了前 10 个字符 `secret-key`。现有测试之所以通过，是因为用例里的 key 都在 body 前 200 字节之内（`oops secret-key-123`、`token secret-key-123 invalid`），而 `NotContains(err, testKey)` 只检查完整 key 是否出现。真实 Tiingo token 为 40 位时，最多会泄露 39 位。

**修复方向**：在截断之前先对完整 body（和 detail）做 key 替换，或者先 `wrapErr` 再截断。补一个用例，让 key 横跨 200 字节边界（如 `strings.Repeat("x", 190)+testKey`），断言错误文本不含 key 的任何前缀（例如 key 的前 4 个字符）。

## done_criteria 覆盖矩阵

| # | 完成标准 | 对应测试 | 守卫证据（变异） | 判定 |
|---|---|---|---|---|
| functional[0] | 路径 / 仅 startDate / Token 头 / RawQuery 不含 key / BRK.B→BRK-B | TestFetchHistoryRequestShape、TestFetchHistoryTickerMapping | A23 带 endDate、A26 丢 Token 前缀、A27 key 进 query、A28 不映射 ticker、A31 路径错 | PASS |
| functional[1] | 端到端 5 根且 06-05 Close 122.44；`[]` 返回空切片；非 UTC 时区的 startDate 与截取下界都取自身时区日历日 | TestFetchHistorySplitNormalizedAndClipped、TestFetchHistoryEmptyBody、TestFetchHistoryNonUTCDates | A20 startDate 取 UTC、A21 截取下界取 UTC、A22 截取上界取 UTC（end 侧也被守住，bars[4]==06-11） | PASS |
| functional[2] | 主题常量 × 生产内置表 40/1h；构造时快照 Default | TestTopicMatchesBuiltinPolicy、TestNewSnapshotsDefaultGate | A1 常量拼错只被 TopicMatchesBuiltinPolicy 杀；A32 不快照被杀 | PASS |
| boundary[0] | 缓存（TTL>0）：同 key 只发 1 次 HTTP 且返回值隔离；start/end 各自进键；失败不缓存 | TestFetchHistoryCachedAndIndependent、…CacheKeyCoversStartAndEnd、…ErrorNotCached | A2 去 Clone、A3/A4/A5 键缺字段、A24 每次取 Default | PASS |
| error_handling[0] | 404/400/401/403/429/500 分类、`tiingo: ` 前缀、不含 key | TestFetchHistoryStatusErrorsRedacted | A6–A10、A25 不脱敏、A29 不解析 detail、A30 缺前缀 | PASS（字面上）；**不通过的是上面的截断泄露** |
| error_handling[1] | transport / decode / HTTP 错误：前缀、不含 key、Unwrap==nil | TestFetchHistoryErrorsRedactedAndUnchained | A11/A12/A13：构造「文本已脱敏但 Unwrap 能取回原错误」的 chainErr 变异，专门检验断链断言（不借助脱敏断言），全部被杀 | PASS |
| error_handling[2] | ErrQuotaExceeded / ErrTimeout 映射为 retryable 且 errors.Is 为 false；配额 1 时第 2 次失败且服务端计数为 1 | TestFetchHistoryQuotaExceededIsRetryable、TestFetchHistoryTimeoutIsRetryable | A14、A15、A16 哨兵留链、A17 文案无 retryable | PASS |
| non_functional[0] | Transport 为 *http.Transport、Proxy==nil、Timeout 30s；race/gofmt/vet/覆盖率 ≥80% | TestClientTransportIgnoresProxy + 实跑 | A18 继承代理、A19 Timeout 60s | PASS |

## 证据
```
@ 37027bf  go test -count=1 -race -cover ./internal/collector/tiingo/ → ok  coverage 95.1%
gofmt -l → 空；go vet → rc=0
client.go 各函数覆盖率：fetchHistory 86.4%，其余 100%
Quota|Timeout|Cache 用例在 -race 下连跑 3 次 → 全 ok
```

### 独立变异（隔离 worktree；脚本 scratchpad/test-tg-b-TASK-004-mut.py；每个变异先 go vet 确认可编译，匹配次数 ==1，收尾源文件还原、porcelain 为空）
共 33 个，其中 A33（换 DefaultClient）写法错误、编译不过，属无效变异，其他变异已覆盖同一行为。**有效变异 32/32 KILLED**。dev 自报 21/21，我没有照搬，逐个独立构造。另外新增了断链专用 chainErr 变异、截取上界取 UTC、401/403 各自不归类、key 进 query、不解析 detail、缺前缀、路径错、不快照。

## Leader 点名的三个关注点
1. **缓存测试是否用了 TTL>0 的 Gate**：是。三个缓存用例都有 `c.gate = cachingGate()`，即 `Policy{TTL: time.Minute, Coalesce: true}`。证据是 A2–A5 只被这三个用例杀掉，而且它们断言的是 HTTP 请求次数（n==1 / 累计 1→2→3→3），缓存确实被使用了；在零策略闸门下这些断言必红。
2. **配额测试「跨整点换新 Gate 重跑最多 3 次」会不会掩盖真失败**：不会。跳过一轮的唯一条件是 `time.Now().UTC().Truncate(time.Hour)` 在两次调用前后发生了变化，这与 `policy.windowStart` 对小于 24h 窗口的算法（UTC Truncate）同口径，只由时钟决定，被测代码影响不了。只要有一轮没跨整点，就会完整断言 err1 为 nil、err2 为 retryable、服务端计数为 1。3 轮都跨整点则直接 Fatal，不会静默通过。A15 和 A16 被这条测试确定性地杀掉。
3. **所有错误出口不含 key**：逐个出口检查：
   - build request：wrapErr
   - transport：wrapErr
   - read body：wrapErr
   - status：statusErr → wrapErr，**有截断泄露**
   - decode：wrapErr
   - normalize：wrapErr
   - Gate 哨兵：mapPolicyErr → wrapErr
   - 其余经 mapPolicyErr 原样返回的错误：policy.fetch 只会返回 ErrTimeout、ErrQuotaExceeded 或 fn 自身的错误（gate.go:140–200 已读；账本故障时 fail-open 返回 nil），fn 的错误都已经过 wrapErr。
   
   结论：只有 statusErr 的截断路径有缺陷，见上。

## 其他观察（不影响判定）
- wrapErr 用字面 ReplaceAll 脱敏。如果 key 含 URL 需要转义的字符，`*url.Error` 打印出的是转义后的形式，可能漏替换。Tiingo token 为十六进制，目前没有影响。
