# TASK-018 验证报告（四包收口 policy 错误映射）

- 验证者：test-agent-17
- 交付：**两个 commit** —— `253c1bc6ffcd49847624febc31455f5f8bfe6bc5`（yahoo + twelvedata）
  与 `4c92611ec5e4c375f66b15f3ea3e14e5ed2afedf`（lixinger + tushare）
- 承接时 `assignment_epoch`：**2**
- 裁决：**verified**

## 基线锚（先解决一处分歧）

Leader 给的锚是 `4c54e77`，我先前备的是 `fb29d71`。**`4c54e77` 是对的**（它是首个 018
commit `253c1bc` 的父提交）；但两者对本任务这四包**等价**——
`git log fb29d71..4c54e77 -- <四包>` 为空，中间三个 commit（`11b7d56` eastmoney /
`0a1ed48` crypto / `4c54e77` baostock）都没碰这四包。我在 `4c54e77` 重测，数字与
`fb29d71` 逐位相同，故锚的分歧对本次判定无影响。

⚠ 另一处口径陷阱：`415f900`（yahoo 缓存键时间精度，**属别的任务**）夹在两个交付
commit 之间且改了 yahoo。故本报告的归因一律用**逐 commit** `git show`，不用区间 diff。

## 一、基线 vs 交付（口径：**单包**，非合并）

| 包 | 基线 `4c54e77` | 交付 `4c92611` | RUN 数 | SKIP | 哨兵数 |
|---|---|---|---|---|---|
| yahoo | 88.0% | **89.3%** | 58 → 83 | 0 | 0 → 0 |
| twelvedata | 92.7% | **95.5%** | 23 → 31 | 0 | 0 → 0 |
| lixinger | 92.2% | **93.4%** | 59 → 69 | 0 | 0 → 0 |
| tushare | 95.2% | **95.3%** | 36 → 43 | 0 | 2 → 2 |

四包 `-race` 全绿；`go build -buildvcs=false ./...` exit 0；`go vet ./internal/collector/...` exit 0；
全量回归 **62 包 ok / 0 FAIL**；本任务 9 个文件 `gofmt` 全过。

（yahoo 的 RUN 增量 58→83 里含 `415f900` 那个**别的任务**新增的用例，不全归本任务；
覆盖率同理。这里只作「不低于基线」的门槛判定，不做增量归因。）

## 二、完成标准覆盖矩阵

**全部变异由我自己注入并复跑，未采信 Dev 报告的任何一条输出。** 每条过五道门：
diff 非空且语义改变 / 编译 vet 通过 / 断言行匹配 `^\s+\w+_test\.go:\d+:\s+\S` /
`=== RUN` 计数 > 0 / 还原后工作区 `git status --porcelain` 为空。

| # | 完成标准 | 对应测试 | 我注入的变异 → 转红位置 | 判定 |
|---|---|---|---|---|
| functional[0] | 全部 `policy.Fetch` 返回错误经映射，判据 `errors.Is==false` | `TestPolicySentinelDoesNotLeak`（四包各一） | V-TD 摘掉 twelvedata 映射层 → `:55`；V-TS7 删 tushare ErrTimeout 分支 → `:62`；V-L2 → `:58`；逐调用点各摘一处 → 恰好对应子用例红 | PASS |
| functional[1] | 映射后保留可诊断信息 | `TestMappedErrorKeepsDiagnosis` | V-NA 丢原文 → 红；V-T10 → `:99`；V-T2 → `:80` | PASS |
| boundary[0] | 临时性不得映射成永久性（按有无哨兵分形态） | tushare `TestMappedTimeoutIsRateLimitedNotPermission`；其余三家 `TestMappedErrorNotConfusableWithPermanent` | **V-TS8 → `:81` 与 `:84` 同时红**；V-Y1 → `:166`（三个子用例全红）；V-T1 → `:116`；V-L1 → `:127`；V-G1 清空判别集合 → `:98` | PASS |
| boundary[1] | 非 policy 错误路径不受影响 | `TestNonPolicyErrorsUnaffected` + `TestMapPolicyErrPassesThroughNonPolicyErrors` | V-Y6 统一 catch 保原文 → **仅** `:241` 红；V-NA 统一 catch 丢原文 → 6 条红 | PASS |
| boundary[2] | 文案传达临时性 | `TestMappedTimeoutReadsAsTemporary` | V-T10 保留正确哨兵、只改文案 → `:113` | PASS |
| error_handling[0] | 映射在 `Fetch` 返回值处，不在更外层统一 catch | `TestMappingHappensAtFetchNotOuterLayer` + `PassesThrough` | V-Y6 / V-NA；另经源码逐处核对（六处映射全在 `policy.Fetch` 紧邻的 `if err != nil` 内） | PASS |
| error_handling[1] | 两种口径按「本包有无哨兵」分派 | — | V-L2 无哨兵包改 `%w` → `:58` 红（**实证「只查文本会被 `%w` 骗过」，判定必须落在 `errors.Is`**）；V-TS9 有哨兵包改 `%v` → `:81` 红 | PASS |
| error_handling[2] | 不新增导出 API | — | grep 核实：yahoo/twelvedata/lixinger 仍各 0 个哨兵；tushare 仍恰为 `ErrNoPermission`/`ErrRateLimited` 两个；四包 `mapPolicyErr` 均未导出 | PASS |
| error_handling[3] | twelvedata 必须走 `c.wrapErr`（唯一出口，负责脱敏） | `TestMappedErrorKeepsDiagnosis` 的前缀断言 | V-T2 绕过 wrapErr 自起一套 → `:80` | PASS |
| error_handling[4] | tushare `ErrTimeout` → `ErrRateLimited`（`%w`），绝不 `ErrNoPermission` | `TestMappedTimeoutIsRateLimitedNotPermission` | V-TS8 → 正反两条同时红；V-TS9 → `:81` | PASS |
| non_functional[0] | 既有测试一字不改 / `-race` / 0 SKIP / 覆盖率不低于水位 / **注明口径** | — | 四个 `errmap_test.go` 全是**新增文件**（`--diff-filter=A`，0 删除行），无任何既有 `_test.go` 被改；口径已注明为单包 | PASS |

## 三、Leader 点名的三处，逐条独立复现

### 1. twelvedata 的 `errors.Is` 断言是否空真 —— **担忧不成立，Dev 的证伪属实**

我和 Leader 的假设是：`wrapErr`（`client.go:65-71`）刻意用 `%v` 断链，所以
`errors.Is(ret, policy.ErrTimeout)==false` 可能在**没有映射层时也成立**。

**读码即可排除，实测确认**：`wrapErr` 只在 `fetchHistory`（也就是被缓存的 `fn`）内部使用；
而 `policy.Fetch` 自己产生的 timeout 是**直接交给上层的**，那条路径上根本没有 `wrapErr`。

实测（V-TD，把 `return nil, c.mapPolicyErr(err)` 换成 `return nil, err`）：

    errmap_test.go:55: policy 哨兵错误外泄到上层: policy: fn timeout
    errmap_test.go:80: 映射应经本包唯一 error 出口 wrapErr（含脱敏），got: policy: fn timeout

外泄是真的，断言有内容。**这条我按 Leader 要求单独跑，没有因为 yahoo 转红就一并采信。**

### 2. 下界守卫是否有效 —— **对「清空」有效；对「写错」无效（观察项）**

- V-G1 把判别集合清空 → `errmap_test.go:98` 红（`永久性判别集合为空，本测试空转`）。守卫生效。
- 判别串本身**确实是本包真实存在的永久性错误**（我逐条 grep 源码核对，如
  `no data for symbol` 见 `yahoo.go:279`/`:357`、`build request` 见
  `twelvedata/client.go:150`）。DoD 这条要求满足。

**但守卫只查非空，不查内容。** 观察项实验 V-OBS：把 `tdPermanentPhrases` 换成
`{"zzz_never_occurs"}`，**同时**把映射文案改成 `decode failure`（`decode` 是本包真实的
永久性判别串）—— `TestMappedErrorNotConfusableWithPermanent` **仍然全绿**。

即：列表被换成永不出现的串时，这条断言会静默退化成恒真，而守卫不会报警。
今天列表是对的，所以**不影响本次判定**；作为加固建议登记（见第五节）。

### 3. tushare 的更高标准 —— **正反两面确实同时红**

V-TS8（`ErrTimeout` 映射成 `ErrNoPermission`）：

    errmap_test.go:81: 超时属临时性，必须可被 errors.Is(ErrRateLimited) 判定（降级链据此重试）
    errmap_test.go:84: 临时性错误被映射成永久性 ErrNoPermission —— 降级链将永不重试该标的

正向（必须 `Is(ErrRateLimited)`）与否定（不得 `Is(ErrNoPermission)`）两条**同时**转红，
「有哨兵带来的可测性红利」名副其实。V-TS9（改 `%v` 断链）单独红 `:81`，实证有哨兵的包
必须保 `%w`——否则 `refresh.go:450/:453` 的降级链两个分支都匹配不上。

## 四、我另外补的完整性核查

### 调用点清点：六处，全覆盖

| 包 | `policy.Fetch` 调用点 | 映射处 |
|---|---|---|
| yahoo | `eps.go:50` / `yahoo.go:239` / `yahoo.go:313` | `eps.go:54` / `yahoo.go:243` / `yahoo.go:317` |
| twelvedata | `client.go:123` | `client.go:127` |
| lixinger | `client.go:47` | `client.go:51` |
| tushare | `client.go:91` | `client.go:99`（Quota）/ `:109`（Timeout） |

**逐调用点分辨率实测**（yahoo）：分别只摘掉一处映射，每次**恰好**对应那一个子用例转红
（`FetchEPSHistory/eps`、`FetchQuote/quote`、`FetchHistory/chart`），无串扰、无遗漏。

### 哨兵来源清点：无第三个泄漏口

`policy` 包**恰有 2 个**导出哨兵（`gate.go:16 ErrTimeout`、`quota.go:14 ErrQuotaExceeded`），
四包的映射条件均同时判这两个。且四包对 policy 的 API 使用只有 `Fetch`（另一个会返错的
`Gate.Do` 四包都没用），故不存在绕过映射的哨兵来源。

### 越界申报：无

两个 commit 合计 9 个文件，全部落在声明的四个 `packages` 内，与 discovery 的
`files_modified` 逐条一致。

## 五、观察项（不影响本次判定）

1. **DoD 自身有两处与事实不符，建议订正**（Dev 已发现并按后到的裁定执行，实现是对的）：
   - `error_handling[1]` 末段写「**yahoo 有哨兵错误 ⇒ 映射**」——实测 yahoo **0 个哨兵**。
     该句与 `boundary[0]`（Leader 后到的裁定，明写「yahoo 与 twelvedata 各有 0 个」）矛盾。
     交付按 `boundary[0]` 做，正确。
   - `functional[0]` 写「yahoo 全部 **4 处**」——实为 **3 处**（我独立 grep 核实）。交付覆盖全 3 处。
2. **判别串列表的内容无守护**（详见第三节 2）。加固建议：为每个 phrase 加一条断言，
   要求它确实出现在本包某个真实错误的文本里（例如驱动一次会产生该错误的调用），
   把「列表是真的」从注释升级为断言。
3. **注释里的行号引用已漂移**：`yahooPermanentPhrases` 标 `yahoo.go:259/:337` 实为 `:279/:357`；
   `tdPermanentPhrases` 标 `client.go:129/141/146/151` 实为 `:150/:162/:167/…`。
   串本身全部正确，只是行号锚过期——与仓库近期「行号引用改函数名」的做法一致，建议照办。
4. Dev 自报的两处方法问题（`-run` 正则漏掉新测试导致 RUN 数不变而误判绿、zsh 不对未加引号
   变量分词导致 `go test $P` 根本没跑却统计出 `FAIL=0`）**都属实且都被它自己抓住了**，
   抓住的手段都是**核对 RUN 数**而非只看红绿。这与我在 TASK-009 踩的「`-run` 目标过期」
   是同一形态。

## 六、原始产物指针

- 任务文件：`.arcforge/tasks/TASK-018.json`
- 上游 discovery：`.arcforge/discoveries/TASK-015.json`；本任务 discovery：`.arcforge/discoveries/TASK-018.json`
- git ref：`4c92611ec5e4c375f66b15f3ea3e14e5ed2afedf`（基线 `4c54e77560c689cd40756c741c7661ffae6a43f1`）
- 复现命令（锚一律钉全 sha，不得写 `HEAD`/分支名）：

      git worktree add --detach ../wt-v018  4c92611ec5e4c375f66b15f3ea3e14e5ed2afedf
      git worktree add --detach ../wt-base018 4c54e77560c689cd40756c741c7661ffae6a43f1
      for p in yahoo twelvedata lixinger tushare; do
        GOTOOLCHAIN=local go test ./internal/collector/$p/ -count=1 -cover
      done
      # V-TD（最关键那条）：把 twelvedata/client.go:127 的 c.mapPolicyErr(err) 换成 err，
      # TestPolicySentinelDoesNotLeak 须转红。
