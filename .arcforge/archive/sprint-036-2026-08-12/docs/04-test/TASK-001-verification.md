# TASK-001 验证报告 — Fetcher 接口与绕代理的 PBOC client

- **验证者**：test-agent-25（Reality Checker，默认判定 NEEDS WORK）
- **判定对象**：`verify_baseline.head = cfcbdbb668496da25a6a8dd7cca012258e1d23e7`
- **TASK-001 自身提交**：`bf0ddf11d501c7ffe14c0d24da5aa7e7e4d2944f`
- **验证 worktree**：`git worktree add --detach /tmp/verify-036-1 cfcbdbb668496da25a6a8dd7cca012258e1d23e7`
- **结论：VERIFIED（8/8 DoD 全部 PASS）**

## 0. 漂移核验（零漂移，未使用 `--ack-drift`）

```
$ git rev-parse HEAD
cfcbdbb668496da25a6a8dd7cca012258e1d23e7
$ jq -r '.verify_baseline.head' .arcforge/tasks/TASK-001.json
cfcbdbb668496da25a6a8dd7cca012258e1d23e7
$ shasum -a 256 .arcforge/discoveries/TASK-001.json
5d74c7ca8f0d6a6b78e71786aba49bb5a671cea32f5e5673ec437824ccd8c661
$ jq -r '.verify_baseline.discovery_sha256' .arcforge/tasks/TASK-001.json
5d74c7ca8f0d6a6b78e71786aba49bb5a671cea32f5e5673ec437824ccd8c661
```

HEAD 与 discovery sha256 **均与基线逐字相同** ⇒ 判定对象即交付物，**本次未使用 `--ack-drift`**。

范围核验：`git show --numstat --format= bf0ddf1` → `fetch.go 72/0`、`fetch_test.go 147/0`、
`store_test.go 16/2`，与 `writes` 三项**逐项一致，无越界**。
（`dev_done` 门禁的两组 WARN 点名 `internal/broker/paper/` 等文件，成因是门禁按
`^[a-z]+\(TASK-001\):` 认提交而各 Sprint 复用同一批任务编号，捞到 6–8 月历史同号提交。
经 numstat 核实**不是范围漂移**。）

## 1. DoD 逐条覆盖矩阵

| # | DoD 条目 | 对应测试 | 承重证据（消融） | 判定 |
|---|---|---|---|---|
| F1 | Transport 对真实 PBOC 域名不走代理 | `TestPBOCFetcherDoesNotProxyPBOC` | M1、M10 均 KILLED | **PASS** |
| F2 | `Get` 返回响应体原文 | `TestPBOCFetcherGet/正常返回体` | `assert.Equal` 精确字节 | **PASS** |
| B1 | 上限 10MB，超过报错含 `exceeds`；`LimitReader` 多读一字节 | `超大响应被拒` + `恰好等于上限被接受` | M5、M6、M7 KILLED | **PASS** |
| E1 | 非 200 报错，**同时**带状态码与 URL | `非 200 报错且带状态码与 URL` | M2（去状态码）、M3（去 URL）**分别** KILLED | **PASS** |
| E2 | `ctx` 取消可中断，`errors.Is(err, context.Canceled)` | `ctx 取消能中断` | M4 KILLED，因果为 `ErrorIs` 断言 | **PASS** |
| N1 | 导出面守卫**登记不放松** | `store_test.go:406` 精确集合相等 | M9 KILLED | **PASS** |
| N2 | RED 因预期原因失败 | 独立复现 | 见 §4 | **PASS** |
| N3 | gofmt/vet 无输出、整包绿、覆盖率 ≥ 92.1% | 见 §2 | — | **PASS** |

## 2. N3 的命令与输出

```
$ cd /tmp/verify-036-1
$ GOTOOLCHAIN=local go vet ./internal/hestia/     → 无输出，exit 0
$ gofmt -l internal/hestia/                       → 无输出，exit 0
$ GOTOOLCHAIN=local go build ./...                → 无输出，exit 0
$ GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover
ok  github.com/newthinker/atlas/internal/hestia  0.824s  coverage: 92.3% of statements
$ GOTOOLCHAIN=local go tool cover -func=cov.out | grep -E 'fetch.go|total'
internal/hestia/fetch.go:35:  NewPBOCFetcher  100.0%
internal/hestia/fetch.go:46:  Get             100.0%
total:                        (statements)     92.3%
```

覆盖率 **92.3% ≥ 92.1%** 下限；`fetch.go` 两个函数各 100.0%。
（我这一侧 `go tool cover -func` 也读到 92.3%，dev 报的是 92.2%——0.1 的差是真值落在
~92.25% 上的舍入产物，两个读数均高于下限，不影响判定。）

## 3. 变异/消融独立复验（harness 由我自写，不复用 dev 的）

Harness：`scratchpad/test25-TASK-001-ablation.sh`，锚点 `ARCFORGE_MUT_REF` 可覆写，
默认**全 sha** `cfcbdbb…`；变异作用在 `git worktree add --detach /tmp/mut-036-1` 的隔离树上。

四道闸全部内建并全部通过：
- **基线闸**：未变异全绿，`--- PASS` 行数 = **499**
- **生效闸**：每条变异 `git diff` 非空，且逐条打印 diff 原文供语义核对
- **编译失败闸**：`go test -c -o /dev/null` 不过则记 INVALID 不记 KILLED（0 条命中）
- **计数自证**：变异条数 8 == 结论行数 8 → **OK**

### dev 声称的 8 条（全部独立复现，因果逐条核对）

| 变异 | 结果 | 我实测的**死因原文** |
|---|---|---|
| M1 Transport 改回 `ProxyFromEnvironment` | KILLED | `TestPBOCFetcherDoesNotProxyPBOC`：`Expected nil, but got: &url.URL{... Host:"127.0.0.1:1" ...}` |
| M2 非 200 去掉状态码 | KILLED | `"hestia fetch http://127.0.0.1:63724: 抓取失败" does not contain "404"` |
| M3 非 200 去掉 URL | KILLED | `"hestia fetch: HTTP 404" does not contain "http://127.0.0.1:63736"` |
| M4 改用不带 ctx 的 `NewRequest` | KILLED | `Target error should be in err chain: expected "context canceled"` |
| M5 `LimitReader` 不多读一字节 | KILLED | `超大响应被拒`：`An error is expected but got nil.` |
| M6 `>` 改 `>=` | KILLED | **唯一**失败子测试 = `恰好等于上限被接受` |
| M7 `exceeds` 换词 | KILLED | `"... response is too large: 10485760 bytes" does not contain "exceeds"` |
| M8 `%w` 改 `%v`（三处） | KILLED | `URL 非法` 与 `读响应体中途断流` 两条：`Expected value not to be nil.` |

**M4 的因果专门核对过**：该子测试耗时 5.00s（走到 client Timeout），但打红它的是
`assert.ErrorIs(err, context.Canceled)` 这条断言（拿到的是 timeout 而非 canceled），
**不是被测试框架超时打死**——与 dev 声称一致。

### 我补的 3 条（dev 未做的方向）

| 变异 | 结果 | 说明 |
|---|---|---|
| **M9** 从导出面守卫期望列表删掉 `NewPBOCFetcher` | **KILLED** | `Not equal: expected 八项 / actual 九项` ⇒ 守卫是**精确集合相等**且承重，「登记」不是形式 |
| **M10** 整行删掉 `Transport: &http.Transport{}`（即「忘了配」，隐式用 `DefaultTransport`） | **KILLED** | `transportOf` 的 `require.True(..., "Transport 应是 *http.Transport")` 打红。最自然的错误实现被挡住 |
| **M11** 把守卫从 `assert.Equal` 放宽成 `assert.Subset` | **SURVIVED** | **预期内**，见 §5 观察 1 |

### 主工作区完整性（双重核实）

变异窗口内与收尾各校验一次：
```
4a0b16df9de0824455ee67bb47c4c607c54b70d92ab4ee7a6c33c7e94f654be0  internal/hestia/fetch.go
9b7c4bef2dbba613b6c2a396abf6fff0b3ad463c5809c506372dc58c476fec27  internal/hestia/fetch_test.go
39ad80388a261b99a532518780c6f43dc2146a1b0ba1f0199c000e335fef5f7a  internal/hestia/store_test.go
$ git status --porcelain   → 与变异前逐字相同（仅 .arcforge/ 的既有未提交项）
```
变异树收尾 sha256 与原文一致（`OK`），`/tmp/mut-036-1` 已 `worktree remove --force` + `prune`。

## 4. 四件特别要查的事 — 逐条结论

### ① 代理测试的鉴别力 —— **有鉴别力，且不依赖测试执行顺序**

测试写法**没有**掉进「httptest + Setenv + 断言成功」的无效陷阱：它取出 `*http.Transport`，
`Proxy == nil` 直接早退（直连，正是要的），否则**拿真实 PBOC 域名**
`https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html` 去问 `tr.Proxy(req)`
并断言 `assert.Nil(u)`。M1 复验致红的**正是这条代理断言**（死因原文见上表，
不是别的原因）。

我额外查了一个 dev 与 reviewer 都没查的脆弱点：`http.ProxyFromEnvironment` 内部有
`envProxyOnce`（每进程只读一次环境变量），若包内更早的测试先触发它，`t.Setenv` 可能失效
⇒ M1 会**假存活**。实测证否：

```
$ go test ./internal/hestia/ -count=1 -run 'TestPBOCFetcherDoesNotProxyPBOC' -v   # M1 变异下隔离运行
    Error:  Expected nil, but got: &url.URL{... Host:"127.0.0.1:1" ...}
--- FAIL: TestPBOCFetcherDoesNotProxyPBOC (0.00s)
```
隔离运行同样红 ⇒ 鉴别力不依赖包内测试顺序。

### ② dev 主动补的三处 —— 三处都**正当且承重**

1. **`gofmt -w` 修正计划代码**：`findings-carryover.md:87` 已记录同一发现（计划 Step 6 五处
   `NewPBOCFetcher(5 * time.Second)` 非 gofmt-clean）。纯格式、零语义变化，且 DoD 明写
   `gofmt -l` 必须无输出 ⇒ 属「计划沉默而 DoD 有要求」，处置正当，且**已在 discovery 申报，非静默修正**。
2. **补两条错误出口用例**：确认是**补测试而非放宽阈值**——92.1% 的下限写在 `tasks/TASK-001.json`
   的 DoD 里，dev 无权也未曾改动；`store_test.go` 的 16/2 改动**全部**是导出面守卫的登记
   （已逐行看 diff）；没有任何断言被删除或弱化。而且这两条新用例**不是凑覆盖率的空洞用例**：
   M8（`%w`→`%v`）**只被它们俩**（外加 ctx 那条）杀死，是承重断言。
3. **补「恰好等于上限被接受」**：M6（`>`→`>=`）实测**唯一**失败子测试就是这一条
   （`--- FAIL: TestPBOCFetcherGet/恰好等于上限被接受`，其余子测试全绿）
   ⇒ dev 声称的「只被这条杀死」**成立**，该用例承重、非冗余。

### ③ 「包住了」的断言写法 —— **无平凡为真写法**

```
$ grep -n "Unwrap" internal/hestia/fetch_test.go
125:  require.NotNil(t, errors.Unwrap(err), "底层错误必须被 %w 包住，调用方才能 errors.As 出去")
145:  require.NotNil(t, errors.Unwrap(err), "底层错误必须被 %w 包住")
```
两处均为 `require.NotNil(t, errors.Unwrap(err))`，**全文无** `NotErrorIs(errors.Unwrap(err), err)`。
M8 独立证明这两条非平凡（`%w`→`%v` 时它们红）。

### ④ 消融的因果 —— 见 §3 表格，逐条核对断言失败原文，**不止看 FAIL 行**

编译失败闸 0 命中；计数自证 8==8。

## 5. 观察项（不影响本次判定，供 Leader / QA 参考）

1. **「守卫不得放宽成 `assert.Contains`」这条 DoD 约束本身没有机制守卫。**
   M11 把 `assert.Equal(精确九项)` 换成 `assert.Subset(got, ["NewPBOCFetcher"])` 后
   **SURVIVED**——测试全绿。这是元层面的固有限制（守卫的守卫无人守），只能靠 review 与
   CLAUDE.md 纪律。**本次 dev 未放宽**（M9 已证明精确相等仍在），故 DoD `non_functional[0]` 满足。
   记此项是因为它是**下一个改这条守卫的人**的风险点。
2. **存量的 F8 形态**：`internal/hestia/store_test.go:1697` 有
   `require.NotErrorIs(t, errors.Unwrap(err), err)`——正是 Sprint 035 F8 指出的平凡为真写法。
   它**不在** `bf0ddf1` 的 diff 内（是既有代码），**不属本任务范围**，但值得单开一条清理。
3. **`scope-writes-outside-packages` 12 条告警**：validator `exit 0`，其中 TASK-001 的 3 条为
   本仓库已知形状级假阳（AD-036-4），**未据此判不通过**。
4. `git worktree list` 里有一个 `wt-TASK-002`（在途）与四个历史遗留 `.worktrees/*`；
   后者与本 Sprint 无关，Leader 可在阶段边界一并清理。

## 6. 结论

**VERIFIED。** 8 条 done_criteria 逐条有对应测试、逐条有消融证据证明断言承重；
11 条变异中 10 条 KILLED、1 条 SURVIVED 且属元层面固有限制（非本任务缺陷）；
四道闸全通过、计数自证 8==8；主工作区零污染；判定对象与交付物零漂移。
