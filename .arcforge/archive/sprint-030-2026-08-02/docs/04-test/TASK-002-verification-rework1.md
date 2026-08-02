# TASK-002 复验报告(rework 1)— C1 CRITICAL 密钥外泄修复

验证者: test-agent-15 | 日期: 2026-08-02 | epoch=2 | 判定: **VERIFIED (PASS)**

首轮报告见 `TASK-002-verification.md`(8 条 DoD 全过)。本轮为**定向复验**:C1 是否真堵住
+ 有无回归。首轮结论不重复,仅复核未被破坏。

## DoD 修订的适用声明
Leader 已裁决:原 `functional[0]` 的「query 含 …/apikey」修订为
**「apikey 经 Authorization 请求头传递,不得出现在 query」**,其余五个 query 参数不变。
理由是 key 留在 query 正是 C1 外泄根因,DoD 字面与安全要求不可兼得。因写通道不允许在
`dev_done` 后改 `done_criteria` 字段,该修订以 Leader 派单消息与 final-report 为准
——**本报告按修订后口径判定**,任务文件内 `functional[0]` 文本仍为旧字面,属已知且经批准的
文档滞后,不作为 reject 依据。dev 已在 discovery `dod_deviation` 节与
`client_test.go:20-23` 映射注释两处显式标注偏离,未隐瞒。

## 亲跑输出摘要
```
GOTOOLCHAIN=local go test ./internal/collector/twelvedata/ -v -count=1 -cover
  TestFetchHistoryParsesAndSorts                                  PASS (0.00s)
  TestFetchHistoryEmptyValues            {empty, missing}         PASS (0.00s)
  TestFetchHistorySkipsUnparsableClose                            PASS (0.00s)
  TestFetchHistoryAPIError                                        PASS (0.00s)
  TestAPIKeyNeverInURLOrErrors  {凭证走请求头 / 传输层失败 /
                                 wrapErr 脱敏 / 空 key}           PASS (0.00s)  ← C1 新增
  TestThrottleMinInterval                                         PASS (0.05s)  ← 实耗仍在
PASS  coverage: 92.7% of statements  (首轮 90.9%,dev_minimum 80%)
go tool cover -func: New/NewWithBaseURL/wrapErr/throttle 均 100%,FetchHistory 90.0%
go build ./... exit=0;go vet exit=0
```
未覆盖块仅 `client.go:117`(build request 失败)、`129`(读 body 失败)、`134`(JSON 解码失败)
——均为无法在 httptest 下自然触发的传输/解析故障路径,不对应任何 done_criteria。

---

## 一、C1 鉴别力验证(本轮核心,不止「看测试变绿」)

Leader 要求这条要有鉴别力。我做了**四重独立验证**,而非采信 dev 的「探针已删」说法。

### ① 独立探针:证实外泄机理为真(仓库外,仅 stdlib)
在 scratchpad 建独立 Go module(不 import 被验仓库,排除被验代码影响),对同一场景
`dial 127.0.0.1:1` 跑两种写法:

| 写法 | error 文本是否含 key |
|---|---|
| 旧:`hc.Get(".../time_series?apikey=<KEY>&symbol=NVDA")` | **true** |
| 新:query 无 key + `Authorization: apikey <KEY>` 头 | **false** |

旧写法的原始文本(key 已打码):
`Get "http://127.0.0.1:1/time_series?apikey=<KEY-WAS-HERE>&symbol=NVDA": dial tcp 127.0.0.1:1: connect: connection refused`
⇒ **QA 的 C1 判断成立**(Go 的 `*url.Error` 确实携带完整 URL、query 不被脱敏),
且新写法从根上消除该文本。

### ② 变异 M1:把 apikey 塞回 query → 测试必须报红
在真实 `client.go` 的 `url.Values` 里加回 `"apikey": {c.apiKey}`,其余不动:

```
--- FAIL: TestFetchHistoryParsesAndSorts        client_test.go:73  Should be empty, but was k1
--- FAIL: TestAPIKeyNeverInURLOrErrors/凭证走请求头,query_不含_apikey
      "apikey=SUPERSECRETKEY123456&end_date=..." should not contain "SUPERSECRETKEY123456"
      "apikey=SUPERSECRETKEY123456&end_date=..." should not contain "apikey"
```
⇒ query 侧断言**有牙**,不是摆设。

### ③ 变异 M2:M1 + 关掉 `wrapErr` 脱敏 → 兜底层是否也有牙
```
--- FAIL: TestAPIKeyNeverInURLOrErrors/传输层失败的_error_文本不含_apikey
      "twelvedata: NVDA: Get \"http://127.0.0.1:1/time_series?apikey=SUPERSECRETKEY123456&...\":
       dial tcp 127.0.0.1:1: connect: connection refused" should not contain "SUPERSECRETKEY123456"
--- FAIL: TestAPIKeyNeverInURLOrErrors/wrapErr_对任意含_key_的文本脱敏
```
⇒ 传输层子用例**能失败**(不是恒真),且失败时的泄漏文本与 QA 报告的 C1 形态**逐字一致**
——等于在受控条件下复现了原漏洞,再确认修复把它消除。
**附带证实两道防线是真·独立的**:M1 单独作用时传输层子用例仍 PASS(脱敏兜住了),
必须 M1+M2 叠加才泄漏 ⇒ 请求头与脱敏各自都能独立堵住这条路径。

### ④ 变异 M3:去掉空 key 保护 → 空 key 用例是否被钉死
```
--- FAIL: TestAPIKeyNeverInURLOrErrors/空_key_不破坏_error_文本
      expected: "twelvedata: boom"
      actual  : "twelvedata: <redacted>b<redacted>o<redacted>o<redacted>m<redacted>"
```
⇒ 复现了 `strings.ReplaceAll(s, "", x)` 的病理行为,与 dev `decisions[2]` 的理由**完全吻合**,
该保护确被用例钉死。

### 变异后的还原核验(防止我自己污染被验对象)
变异前 `cp` 备份到 scratchpad;三次变异后逐次还原。
最终 `md5 client.go` = `af8b750426487b36b2a5a1675d06ab2e`,与变异前**逐字节一致**;
还原后全量复跑 `ok ... coverage: 92.7%` 绿。我的探针目录在仓库外,
`git ls-files --others` 中 `leakprobe` 命中 0,未污染工作区。

### ⑤ 结构性核验(比测试更强的保证)
测试只能证明「已测路径不泄漏」。我进一步核了代码结构,得到更强的结论:

| 核查项 | 实测结果 |
|---|---|
| 包内 `fmt.Errorf` / `errors.New` 出现处 | **仅 1 处**(`client.go:74`,在 `wrapErr` 内) |
| 包内日志出口(`log.` / `fmt.Print` / `os.Std`) | **0 处** |
| URL 拼装处 | **仅 1 处**(`client.go:116`) |
| `c.apiKey` 的全部使用点 | 字段声明(39)、构造(49/54)、脱敏(71-72)、**Authorization 头(121)** |
| `FetchHistory` 的 error 返回点 | 5 处(118/125/130/135/140)**全部**经 `c.wrapErr` |

⇒ `c.apiKey` **在结构上不出现在任何 URL 拼装路径中**,且全包只有一个 error 出口。
这不是「测到的路径没泄漏」,而是「没有第二条路径可泄漏」。dev key_findings[4] 的同款
自查结论经我独立复核成立。

---

## 二、两处设计细节复核(Leader 点名)

### ① 刻意不保留 error 链(`%v` 而非 `%w`)—— 站得住
跨包核对结果:

- twelvedata 包内 **无 sentinel error**(grep `errors.New` / `var Err` 均无命中),
  故没有任何可供 `errors.Is` 匹配的目标。
- 消费方 `internal/prism/refresh.go:322` 对 twelvedata 的 error 用的是
  `fmt.Errorf("price history: %v; twelvedata fallback: %v", err, fbErr)` —— **纯文本格式化**,
  不做 `errors.Is/As`。
- 全仓唯一在 refresh 路径上的 `errors.Is` 在 `refresh.go:450`,作用对象是
  **`tushare.ErrNoPermission`**(A 股跳),与 twelvedata 无关。

⇒ 不留链**无功能损失**,而留链会让 `errors.Unwrap` 取回未脱敏原文使脱敏失效。决策成立,
且已在 `client.go:66-68` 写明理由防后人「好心」改回 `%w`。

### ② 空 key 跳过脱敏 —— 已被用例钉死
见上文变异 M3:去掉该保护后用例立即报红并复现 `<redacted>b<redacted>o...` 病理文本。
真实场景对应「未配置 TD 备源」(空 key),此时 error 文本仍须可读。

---

## 三、回归核验(首轮已验行为未被破坏)

| 首轮行为 | 对应用例 | 本轮状态 |
|---|---|---|
| 按 Time 升序(响应倒序) | TestFetchHistoryParsesAndSorts | PASS,断言未改 |
| 坏 close/datetime 跳过该行 | TestFetchHistorySkipsUnparsableClose | PASS,用例逐字未改 |
| values 空/缺失 → 空切片 err=nil | TestFetchHistoryEmptyValues | PASS,用例逐字未改 |
| status=error → 含 message 的 error | TestFetchHistoryAPIError | PASS,用例逐字未改 |
| 默认 8s + 注入 50ms 节流 | TestThrottleMinInterval | PASS,**实耗仍为 0.05s**(其余用例 0.00s) |
| end+1 天兑现闭区间 | TestFetchHistoryParsesAndSorts:70 | PASS,`end_date == "2026-08-02"` 断言未改 |

测试文件的**唯一实质改动**是 `client_test.go:73`:
`assert.Equal(t, "k1", q[0].Get("apikey"))` → `assert.Empty(t, q[0].Get("apikey"))`
——正是修订后 DoD 要求的方向,且新增了独立的 C1 用例组。**无删除、无弱化既有断言**。
对外签名(`New`/`NewWithBaseURL`/`FetchHistory`)与返工前完全一致,`go build ./...` 通过
⇒ TASK-005 消费方无需改动。

---

## 四、真机端到端复验的记录自洽性(按 Leader 口径不重跑)

discovery 称换头后真机复验:NVDA 拿到 **12 个交易日**、含 **07-31=200.75** 与库一致。

**发现一处表面矛盾并查实**:首轮 discovery/计划文档记的是同一区间
`2026-07-16..07-31` 共 **11 个交易日**,本轮记 **12**。同区间不同计数,须解释清楚才能采信。
只读查库结果:

| 区间 | 库内 NVDA 交易日数 |
|---|---|
| `[2026-07-16, 2026-07-31]`(闭区间,含末日) | **12** |
| `[2026-07-16, 2026-07-30]`(末日排他) | **11** |

且库内 `2026-07-31` 的 NVDA close = **200.75**,与 discovery 所述**逐字相同**。

⇒ 11 与 12 的差恰好是**末日 07-31 这一根**:首轮的 live 比对跑在未补偿 end 的调用上
(得 11,正是 TD `end_date` 排他的表现),本轮换头后的客户端带 end+1 补偿,取到 12 根含
07-31。**这不是记录矛盾,反而是 end+1 闭区间补偿在真机生效的佐证**,同时证明鉴权方式
从 query 改到请求头后取数未被破坏。首轮的「最大相对偏差 ~2e-8」也与库值吻合
(07-16:TD 207.399990 vs 库 207.399993896484)。

真实凭证下的 API 错误路径(NO_SUCH_SYMBOL_XYZ → code 404)文本不含 key 一条,
属真机观察,与包内 `TestFetchHistoryAPIError` + 结构性核验(error 唯一出口经 wrapErr)
互为印证,采信。

---

## 五、密钥哨兵(亲跑)与残留检查

SENT1/SENT2 = runtime `configs/config.yaml` 第 30/34 行两个 api_key 前 8 字符
(现取,长度各 8,全程未打印):

| 扫描范围 | 命中 |
|---|---|
| `internal/collector/twelvedata/` | 0 |
| `git diff`(工作区) / `git diff --cached` | 0 / 0 |
| `git log -p --all -S "$SENT1"` / `"$SENT2"`(全历史全分支) | 0 / 0 |
| 全部未跟踪新文件 | 0 |

**反向对照**:同一对哨兵在 `configs/config.yaml` 命中 **2** 次,证明检索有效。

**探针残留检查**:`ls internal/collector/twelvedata/` 仅
`client.go` 与 `client_test.go` 两个文件——此前诊断见过的 `live_probe_test.go` **已不存在**,
本轮的鉴别力探针也确认已删。我自己的探针建在仓库外的 scratchpad,
`git ls-files --others` 中命中 0。

注:C1 用例里的 `probeKey = "SUPERSECRETKEY123456"` 是**假 key 常量**(测试专用),
与 runtime 真实凭证无关,不构成泄漏。

---

## 六、判定

- C1 修复**真实堵住且经四重鉴别力验证**:独立探针证实机理、三个变异证实测试有牙且
  能复现原漏洞、结构性核验证明「无第二条泄漏路径」。
- 两处设计细节**均站得住**:无 sentinel error 且 prism 只做文本格式化,`%v` 不留链无损失;
  空 key 保护经变异钉死。
- **无回归**:首轮 6 项行为断言逐字未改且全绿,对外签名不变,build/vet 通过。
- 覆盖率 90.9% → **92.7%**,用例 5 → 6 顶层(新增 4 个 C1 子用例)。
- 真机复验记录**自洽**,表面的 11 vs 12 矛盾经查库证实是 end+1 补偿生效的佐证。
- 密钥哨兵全历史 0 命中,无探针残留。

→ **VERIFIED**
