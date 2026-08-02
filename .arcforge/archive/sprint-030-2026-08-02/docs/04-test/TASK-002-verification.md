# TASK-002 验证报告 — Twelve Data 客户端(美股价格备源)

验证者: test-agent-15 | 日期: 2026-08-02 | 判定: **VERIFIED (PASS)**

## 验证环境说明
被验代码为工作区未提交状态(`internal/collector/twelvedata/` 为未跟踪新目录),
无 commit 可供 worktree 隔离,故在主工作区就地验证。同期其它任务(TASK-001/003)
的改动位于不同 package,不影响本包测试结果(`go test` 按包隔离)。

## 亲跑输出摘要
```
GOTOOLCHAIN=local go test ./internal/collector/twelvedata/ -v -count=1 -coverprofile=...
  TestFetchHistoryParsesAndSorts        PASS (0.00s)
  TestFetchHistoryEmptyValues           PASS (0.00s)  含子用例 empty / missing
  TestFetchHistorySkipsUnparsableClose  PASS (0.00s)
  TestFetchHistoryAPIError              PASS (0.00s)
  TestThrottleMinInterval               PASS (0.05s)   ← 实耗证明节流真实生效
PASS  coverage: 90.9% of statements
go tool cover -func: New 100% / NewWithBaseURL 100% / throttle 100% / FetchHistory 88.5%
go build ./... exit=0;go vet(twelvedata+config) exit=0
```
未覆盖的 3 个语句块为 client.go:93(http.Get 传输失败)、98(读 body 失败)、
103(JSON 解码失败)——均为传输层错误路径,不对应任何 done_criteria 条目。

## 完成标准覆盖矩阵

| # | 完成标准 | verify_by | 对应测试/证据 | 判定 |
|---|---|---|---|---|
| functional[0] | GET {base}/time_series,query 含 symbol/interval=1day/start_date/end_date/outputsize/apikey;字符串 close 解析为 float64 | test | `TestFetchHistoryParsesAndSorts`:httptest handler 断言 path=/time_series;逐参断言 symbol=NVDA、interval=1day、start_date=2026-07-31、end_date=2026-08-02、outputsize 非空、apikey=k1;`assert.InDelta(198.20/200.75)` 证明字符串 close 已解析为 float64 | PASS |
| functional[1] | 返回 []core.OHLCV 按 Time 升序(响应 values 为倒序) | test | 同上用例:stub 响应刻意给倒序(08-01 在前),断言 `pts[0].Time.Before(pts[1].Time)` 且 pts[0]=07-31、pts[1]=08-01 | PASS |
| boundary[0] | values 为空或缺失时返回空切片且 err=nil | test | `TestFetchHistoryEmptyValues` 表驱动两子用例:`values:[]` 与响应完全无 values 字段,均 `require.NoError` + `assert.Empty` | PASS |
| boundary[1] | close 不可解析(空串/"null")跳过该行,不中断整段解析 | test | `TestFetchHistorySkipsUnparsableClose`:**测试数据确实混有坏行**——空串 close、`"null"` close、`datetime:"bad-date"` 三条坏行与两条好行交错;断言 `len(pts)==2` 且好行日期 07-29/08-01 仍在、顺序正确。非空洞断言 | PASS |
| error_handling[0] | status=error 响应返回含 message 的 error | test | `TestFetchHistoryAPIError`:HTTP 200 + `{"status":"error","code":401,"message":"**symbol** not found: XYZ"}`;`require.Error` + 断言 err 文本含原始 message。与 discovery 记录的「TD 错误是 200+body」形态一致 | PASS |
| non_functional[0] | 最小间隔默认 8s;注入 50ms 断言两次调用耗时 ≥50ms | test | `TestThrottleMinInterval`:先断言 `New("k1").minInterval == 8*time.Second`(生产默认),再注入 50ms 连发两次,断言 `time.Since(start) >= 50ms`。**非空转证据**:该用例实耗 0.05s,同包其余用例均 0.00s;两次 httptest 本地请求约 1ms 量级,去掉节流必然低于阈值而失败 | PASS |
| non_functional[1] | live 校验复权口径,结论记入计划文档与 discovery | manual | 计划文档 `docs/superpowers/plans/2026-08-02-prism-m3.5a-datasources.md` Task 2 Step 5 已勾选并记:NVDA 2026-07-16..07-31 共 11 交易日与库内 price_daily 逐日对比,最大相对偏差 ~2e-8(float32 存储舍入),远低于 1% 阈值 ⇒ 与 yahoo 同为后复权口径,无需 adjust 参数。discovery `key_findings[1]` 记载同一结论、同一数字、同一区间,两处**自洽** | PASS |
| non_functional[2] | 密钥哨兵:runtime key 前 8 位出现次数为 0 | manual | 验证者独立复跑(见下节),全部命中数 0 | PASS |

## 密钥哨兵独立复跑(验证者亲跑)
SENT1/SENT2 = runtime `configs/config.yaml` 第 30/34 行 tushare/twelvedata 两个
api_key 的前 8 字符(现取,长度各 8,全程未打印)。`grep -F` 命中数:

| 扫描范围 | 命中 |
|---|---|
| `internal/collector/twelvedata/` | 0 |
| `internal/config/` | 0 |
| `configs/config.example.yaml` | 0 |
| `git diff`(工作区全量) | 0 |
| `git diff --cached` | 0 |
| `docs/superpowers/plans/` | 0 |
| 全部未跟踪新文件(`git ls-files --others --exclude-standard`) | 0 |

**反向对照(证明 grep 本身有效,排除假阴性)**:同一对哨兵在 `configs/config.yaml`
中命中 2 次。且 `git check-ignore -v configs/config.yaml` → `.gitignore:30:config.yaml`,
真实密钥文件确被忽略。

## 自主决策复核:FetchHistory 内部 end+1 天
Leader 已批准该决策,验证者逐项核对留证:
1. **doc comment**:`client.go:78-80` 明写「TD 的 end_date 是**排他**的(2026-08-02 实测
   NVDA:end_date=07-31 拿不到 07-31,=08-01 才拿到),故此处发 end+1 天以兑现闭区间
   契约……start_date 是包含的,不作补偿」。实现 `client.go:88` 为
   `end.AddDate(0,0,1).Format("2006-01-02")`,与注释一致。
2. **测试断言**:`client_test.go:65-66` 传入 end=2026-08-01,断言发出的
   `end_date == "2026-08-02"`,并带注释说明来由;同用例的 stub 响应含 end 当日
   (08-01)数据行,断言 `pts[1]` 即为 2026-08-01 / close 200.75,即**end 当日数据
   包含在结果中**。
3. **discovery**:`decisions[0]` 记录决策与 rationale,并明示「TASK-005 消费方无需自行 +1」。
三处留证齐备且互相自洽。

## 越界申报核对
TASK-002 声明 `packages=[./internal/collector/twelvedata]`、
`writes=[./internal/collector/twelvedata, docs/superpowers/plans/2026-08-02-prism-m3.5a-datasources.md]`。
实际改动:`internal/collector/twelvedata/{client.go,client_test.go}`(新建)+ 上述计划文档。
**全部落在声明范围内,无越界未申报**。discovery `files_modified` 亦对计划文档标注了
「已在 writes 申报,越界写」。live 探针临时文件 `live_probe_test.go` 已删除,
`ls internal/collector/twelvedata/` 确认目录仅剩 client.go + client_test.go。

## 观察(非阻断,不影响判定)
- 计划文档 Task 2 的 Step 1/2/3/4/6 复选框仍为 `[ ]`(Step 5 已勾并附结论)。DoD 未要求
  复选框完整性,属文档整洁性瑕疵,建议后续任务收尾时一并勾选。
- `outputsize` 参数测试仅断言非空而未断言具体值 "5000"。DoD 原文只要求 query「含」
  outputsize,已满足;记录备查。

## 判定
8 条 done_criteria(6 test + 2 manual)**逐条通过**,均有实跑输出或独立复跑证据;
覆盖率 90.9% 高于 80% 门槛;`go build ./...` 与 `go vet` 均通过;无越界申报;
密钥哨兵干净。→ **VERIFIED**
