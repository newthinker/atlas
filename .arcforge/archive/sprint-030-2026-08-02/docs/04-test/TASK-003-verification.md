# TASK-003 验证报告 — Baostock 桥(Python 侧车 + Go 客户端)

- **验证者**: test-agent-14 (Reality Checker 模式)
- **验证时间**: 2026-08-02T04:16Z
- **判定**: **PASS → status=verified**(已 jq 直读核实)
- **承接 epoch**: 2(裁决迁移携带 `--expect-epoch 2`,通过)

## 1. 完成标准覆盖矩阵

| # | 完成标准 | 对应测试 / 证据 | 判定 |
|---|---|---|---|
| functional[0] | Go FetchDaily 请求桥 /daily 并把 [{date,close}] 解析为 []core.OHLCV(Time,Close) | `TestFetchDailyParsesResponse` — 断言 path=/daily、start/end query、两行解析为 OHLCV(Time/Close/Symbol/Interval)。突变 M4(Symbol 回填成桥形态)、M5(Interval 置空)、M7(start 误传 end)均转红。**另经真桥实测**:bridge.py 实际输出 `[{"date": "2026-07-30", "close": 1350.6}, ...]`,与单测 fixture 的键名与类型逐字一致(date 为 YYYY-MM-DD 字符串、close 为 JSON 数值),证明假桥 fixture 忠实于真桥形态 | **PASS** |
| functional[1] | symbol 转换 "600519.SH"→sh.600519、"000423.SZ"→sz.000423(query 断言) | `TestFetchDailySymbolConversion` — 表驱动 3 子用例断言 query 中 code 值,含桥形态原样透传。突变 M2b(转换顺序颠倒成 600519.sh)使该用例与 ParsesResponse 双双转红 | **PASS** |
| functional[2]<br>`verify_by: review` | bridge.py:GET /daily 经 query_history_k_data_plus(adjustflag=3) 返回 [{date,close}];仅绑 127.0.0.1:8181;close 空字符串行跳过;非 /daily 返回 404 | **不止 review — 我实跑了真 bridge.py**(注入 baostock 测试替身以隔离上游),逐条行为验证:① `adjustflag="3"`:替身把实参落盘,实测 `{"code":"sh.600519","fields":"date,close","frequency":"d","adjustflag":"3"}`;② **仅绑 127.0.0.1:8181**:`lsof -nP -i TCP:8181` 仅一条 `TCP 127.0.0.1:8181 (LISTEN)`,无 `*:8181`;③ **close 空跳过**:替身返回 3 行(中间行 close="")→ 响应仅 2 行,空行被丢弃;④ **非 /daily → 404**:`/nope` 与 `/` 均返回 404 | **PASS** |
| boundary[0] | 桥响应空数组时返回空切片且 err=nil | `TestFetchDailyEmpty` — `assert.Empty` + `require.NoError`;实现用 `make([]core.OHLCV, 0, len(rows))` 确为非 nil 空切片(直读代码核实)。⚠ **测试判别力有缺口**:突变 M6(改成 `var out []core.OHLCV` 即返回 nil 切片)后测试**仍全绿**,因该用例缺 `assert.NotNil`(tushare 包的同类用例有)。实现正确、标准满足,但 nil 回归测不出 | **PASS**(测试弱,见观察项 1) |
| error_handling[0] | 桥返回 500 时 FetchDaily 返回含状态与正文的 error | `TestFetchDailyServerError` — 断言 error 同时含 "500" 与正文 "baostock login failed"。突变 M1(跳过非 200 分支)、M3b(去掉正文)、M8(去掉状态码)全部转红,证明两个断言各自有判别力 | **PASS** |
| non_functional[0]<br>`verify_by: review` | setup.sh 幂等建 venv(仿 akshare);requirements.txt 版本冻结 | 结构核对:与 `scripts/akshare/setup.sh` 做归一化 diff 后**仅差 2 行**(注释与末尾提示),幂等骨架完全一致。**另实跑第二次**:`bash scripts/baostock/setup.sh` 真实 exit=0,输出显示走 `requirements.lock` 复现安装分支(全部 "Requirement already satisfied"),验后 `requirements.lock` 内容未变、`.venv` inode 未变(119861982)⇒ 未重建、未覆写 lock。requirements.txt 为 `baostock==0.8.9`,**已钉版**(严于 akshare 的未钉版) | **PASS** |
| non_functional[1]<br>`verify_by: manual` | 本机 live:setup.sh + 启桥 + curl /daily 近一月返回行情 JSON;验后杀进程;结论记 discovery | **PARTIAL(按 Leader 明示口径判 PASS)** — 详见 §4。字面要求的「返回行情 JSON」**未达成**(样本 0 行、HTTP 500),根因为上游端口不可达的环境阻断。我**独立复核**了该根因:DNS 解析 `114.94.20.92`(与 discovery 逐字一致)、`nc -z -w 8 www.baostock.com 10030` 超时失败、端口 80 连接成功——三项均与 discovery 记载吻合。进程清理已核实(`pgrep -fl 'python.*bridge\.py'` 无结果、8181 已释放)。discovery 记录完整自洽,满足记录义务 | **PASS**(Leader 口径;残留风险见观察项 2) |

## 2. 亲跑测试输出摘要

```
GOTOOLCHAIN=local go test ./internal/collector/baostock/ -v -count=1 -coverprofile=...
--- PASS: TestFetchDailyParsesResponse (0.00s)
--- PASS: TestFetchDailySymbolConversion (0.00s)   [600519.SH / 000423.SZ / sh.600519 三子用例全 PASS]
--- PASS: TestFetchDailyEmpty (0.00s)
--- PASS: TestFetchDailyServerError (0.00s)
--- PASS: TestFetchDailyBadJSON (0.00s)
--- PASS: TestFetchDailyUnreachableBridge (0.00s)
PASS  coverage: 95.8% of statements
ok  github.com/newthinker/atlas/internal/collector/baostock  0.484s
```

- **用例数**: 6 个顶层用例 / 8 个含子用例,0 失败 0 跳过(与 dev 自报一致)
- **覆盖率**: **95.8%**(门槛 80%);函数级 New 100% / bridgeCode 100% / FetchDaily 94.4%
- `go vet` exit=0;`gofmt -l` 无输出;`go test -race` **ok**

## 3. 突变验证(测试判别力证明)

在仓库内临时副本 `internal/collector/baostock_mut`(基线全绿,验后已 `rm -rf` 并核实目录不存在、原包重跑仍绿)上注入 8 项缺陷:

| 突变 | 注入内容 | 转红用例 | 结论 |
|---|---|---|---|
| M1 | 跳过 `resp.StatusCode != 200` 分支 | TestFetchDailyServerError | 错误分支有效 |
| M2b | symbol 转换顺序颠倒(→600519.sh) | ParsesResponse, SymbolConversion | 转换断言有效 |
| M3b | 错误文本去掉响应正文 | TestFetchDailyServerError | 正文断言有效 |
| M4 | Symbol 回填成桥形态 | ParsesResponse | Symbol 语义有效 |
| M5 | Interval 置空 | ParsesResponse | Interval 断言有效 |
| M6 | 空结果返回 nil 切片 | **无(仍全绿)** | ⚠ **唯一存活突变** |
| M7 | start 参数误传 end | ParsesResponse | query 参数断言有效 |
| M8 | 错误文本去掉状态码 | TestFetchDailyServerError | 状态码断言有效 |

**8 项中 7 项被杀死,1 项(M6)存活**,对应观察项 1。除此之外未发现自引用测试或无判别力断言。

## 4. live 条目(non_functional[1])的判定说明

**这一条我不做无保留的 PASS,须记录在案:**

done_criteria 的**字面文本**是「curl /daily?code=sh.600519 近一月**返回行情 JSON**」。实测**没有**返回行情 JSON —— 返回的是 HTTP 500,样本 0 行。dev 的 discovery 对此如实记载,未粉饰。

Leader 在派验时给出了明确判定口径:该条按「返回行情 JSON;网络/环境失败则如实记录原因」理解,discovery 如实记录即视为满足记录义务,不要求重跑 live。**本条 PASS 是依据该口径作出的,而非依据字面文本达成。**

我独立复核了 discovery 的环境阻断主张(非采信其文字):

| discovery 主张 | 我的复核结果 | 一致? |
|---|---|---|
| DNS 解析正常,114.94.20.92 | `dig +short` → `114.94.20.92` | ✅ |
| TCP 10030 不可达 | `nc -z -w 8 ... 10030` → 超时失败 | ✅ |
| HTTP 80 可达 | `nc -z -w 8 ... 80` → succeeded | ✅ |
| 桥进程已全部清理 | `pgrep -fl 'python.*bridge\.py'` → 无;`lsof -i TCP:8181` → 空 | ✅ |

另外,我用测试替身把上游故障(`error_code=10002007`)注入真 bridge.py,实测返回 **HTTP 500 + 正文 `baostock query 10002007: 网络接收错误。`**,与 discovery 记载的 live 现象逐字吻合,且证明**已获 Leader 批准的偏离**(`rs.error_code != "0"` 显式 raise)确实生效——上游故障不会被静默成 `200 []`。这一点很重要:若照抄参考实现,降级链第三跳的失败会伪装成「无数据」而不触发下一跳。

**残留风险**:真实行情数据路径(baostock 实际返回的 `date` 格式与 `close` 数值 → OHLCV)至今**只经假桥/替身覆盖,未经真上游端到端验证**。discovery 已将其登记并建议在 TASK-006 部署演练时补做。我认同该定性,并建议 Leader 在 TASK-005 消费该跳时不要假定其已实证。

## 5. 额外核查

**声明范围一致性** — `writes` 声明为 `internal/collector/baostock/`、`scripts/baostock/`、`.gitignore`,与工作区实际改动**完全一致**:`.gitignore` 的 diff 恰为新增一行 `scripts/baostock/.venv/`(已获 Leader 批准的越界申报);两个新目录为 untracked。无未申报的溢出。
(注:工作区另有 `.claude/hooks/task-completed.sh`、`docs/collector/` 等改动,经核对在本 session 起始的 git status 中即已存在,**非本任务引入**。)

**密钥卫生** — 本任务不涉密钥。仍执行:对 bridge.py / setup.sh / requirements.txt / 两个 .go 文件扫描 `token|api_key|secret|password` 赋值 **0 命中**;`>=20` 字符连续字母数字串仅命中 Go/Python 标识符;用 runtime `configs/config.yaml` 中全部长 key 逐个交叉哨兵 grep,**0 命中**。

**bridge.py 其他错误面** — `/daily` 缺 `code` 参数时返回 HTTP 500(正文为 KeyError 的 `'code'`),与「桥的全部错误面都转 500 文本」的设计一致,Go 客户端会将其作为临时性错误触发降级链下一跳。

## 6. 观察项(均**不构成** DoD 失败)

1. **`TestFetchDailyEmpty` 缺 `assert.NotNil`**(建议修):突变 M6 把返回值改成 nil 切片后测试仍全绿。当前实现正确(`make(...,0,cap)`),但该属性没有测试锁定,未来重构可能静默回归。tushare 包的同类用例有 NotNil,建议对齐。属可选加固,boundary[0] 的「空切片 + err=nil」在实现层与测试层均已满足。
2. **真桥数据路径未实证**(中等风险,已有归属):见 §4 残留风险。请在 TASK-005 消费第三跳、TASK-006 部署演练时留意。
3. **`bs.login()` 无返回码检查且会阻塞 ~30s**:上游不可达时桥进程启动后有约 30s 窗口端口未监听。discovery 已将其交给 TASK-006(launchd 健康检查须探 8181 端口而非「进程存在」,KeepAlive 需谨慎以免反复重启)。本任务 DoD 未涉及,不影响判定。
4. **`setup.sh` 输出 `baostock 00.8.90`**:系 baostock 上游 `__version__` 字符串本身的怪异格式(实际版本 0.8.9,lock 与 pip 均正确),非缺陷,仅提示阅读输出时勿误判。

## 7. 判定

**7/7 条 done_criteria 全部 PASS**(其中 non_functional[1] 为按 Leader 明示口径的条件性 PASS,已在 §4 完整记录其字面未达成部分与残留风险)。
证据为亲跑命令输出 + 真 bridge.py 行为验证 + 8 项突变 + 环境根因独立复核,非采信 dev 报告文字。
`status=verified` 已落盘并经 `jq` 直读核实(`last_transition: verifying→verified by test-agent-14 @ 2026-08-02T04:16:55Z`)。
