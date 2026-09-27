# TASK-002 验证报告（verifier: test-bk-a）

- 被验对象：`7f51cd4c4e91c80021a958d6e11f9516181b2625`（= verify_baseline.head）
- 判定时主仓库 HEAD 为 `ea8e406789fac8c90cfc24a324bfff3967089748`，比 baseline 多出的是 TASK-003 的合入（analyze.go/analyze_test.go）。`git diff --stat 7f51cd4…ea8e406 -- <本任务 3 个 writes>` 为空，也就是声明范围内没有漂移。
- discovery sha256：`b7acc547422f5c6a1723792ae4ac46ae1d44d521b4108e6be6484c2229cde3d1`（= baseline）
- 验证环境：隔离 worktree `../wt-verify-TASK-002`（detached，钉在上述全 sha），go1.24.4 darwin/arm64
- 结论：**VERIFIED**

## 范围核对
`git diff --stat d647428d4b543ed42b8d1f51e8899df717a40517 7f51cd4c4e91c80021a958d6e11f9516181b2625` 只有 3 个文件（source.go +127 / source_test.go +162 / testdata/em_600036_sample.json +8），与 `writes` 完全一致。

## 非功能（全部自己重采）
| 检查 | 结果 |
|---|---|
| `go vet ./internal/bank/` | 无输出，rc=0 |
| `go test -count=1 -v -coverprofile ./internal/bank/` | rc=0，`--- PASS` 64 行 / `--- FAIL` 0 行 |
| 计数互验 | 尺 1（测试输出）：顶层 20 + 子测试 44 = 64 = `=== RUN` 64。尺 2（源码）：`^func Test` 20 个；子测试 = TASK-001 的 31 + toFloat 8 + 错误表 5 = 44。两尺一致 |
| 覆盖率 | 包 99.0%；source.go 的 NewEMSource/Fetch/parseEMRows/anyRowHas/toFloat 均为 100% |
| 超时 | TestNewEMSourceTimeout 断言 `hc.Timeout == 60*time.Second`，变异 N4 能杀掉 |
| 字段名 grep（internal/bank 非测试文件） | `NONPERLOAN|BLDKBBL|HXYJBCZL|REPORT_DATE` 只出现在 source.go:23,27,28,29 的常量处；`LOAN_PROVISION_RATIO|NEWCAPITALADER|FIRST_ADEQUACY_RATIO` 只出现在 source.go:19-20，两行都是 `//` 注释 |
| 全仓扩展 grep | 非测试文件中只有 `internal/collector/akshare/financials.go` 另含 `REPORT_DATE`。该文件最后一次提交 `1229243` 是 d647428 的祖先，属于 sprint 前的既有代码，与本任务无关 |

## done_criteria 覆盖矩阵
| # | 条目 | 对应测试 | 断言是否测到 | 变异证据 | 判定 |
|---|---|---|---|---|---|
| functional[0] | 路径与 query、6 期升序、最新期 NPL 0.94 / Cov 385.1 / CET1 14.07（避开 3 个诱饵）、MissingFields 为空、尾斜杠不产生 `//api` | TestEMSourceFetchParsesFixture | 路径做等值断言；symbol 与 indicator 在 handler 内断言；Len 6 并逐对检查 Before；首末期日期与 3 个取值做等值断言；样本首行确实含 3.63 / 18.33 / 16.59 三个诱饵 | N5 N6 N7 N8 N9 N10 N11 N23 N24 N26 KILLED | PASS |
| boundary[0] | "1.23"⇒1.23、null⇒NaN、缺键⇒NaN、非法日期行跳过、MissingFields 只列全缺键且顺序固定 | TestParseEMRowsTolerant、TestParseEMRowsMissingFieldsOrder | 两条非法日期（长度不足、2026-02-30）都被跳过，`Len 2`；MissingFields 做整切片等值断言 | N12 N13 N14 N25 N26 KILLED | PASS |
| boundary[1] | 全 null 列不算缺失，值全为 NaN | TestParseEMRowsAllNullColumnNotMissing | `Empty(MissingFields)`，并逐期检查 IsNaN | N1（「值非 null 才算存在」，DoD 点名的变异）KILLED | PASS |
| boundary[2] | `"-"`、`""`、`"Inf"`、`"-Infinity"`、`"NaN"`、布尔、对象 ⇒ NaN | TestToFloatNonNumericIsNaN（8 个子例，比 DoD 多一个 nil） | 每例 IsNaN（NaN 本身就排除了 0 和 ±Inf）；另有正例 1.23 和「真 0 仍是 0」 | N2（去掉 IsInf 守卫）让 Inf 与 -Infinity 转红；N15、N16（返回 0）KILLED | PASS |
| error_handling[0] | HTTP 500、`[]`/`null`⇒无数据、全坏日期⇒无可解析报告期、非 JSON⇒decode、连接失败不 panic、每条错误含 600036.SH、UTF-8 合法 | TestEMSourceFetchErrors（5 例，每例断言文案 + symbol + ValidString）、TestEMSourceFetchConnectionRefused、TestEMSourceFetchErrorBodyValidUTF8 | 均为文案 Contains 断言；UTF-8 用例 100×「错」= 300 字节，截断点 200 恰好落在字符中间 | N3 N17 N18 N19 N20 N22 N27 KILLED；N21 SURVIVED（见观察项） | PASS |
| non_functional[0] | 超时 60s 有断言；字段名只在常量处；禁用字段只在注释里 | TestNewEMSourceTimeout + 上表 grep | 见上 | N4 KILLED | PASS |
| non_functional[1] | vet 无输出、test 全绿、覆盖率 ≥80% | 命令行重采 | 见上 | — | PASS |

## 变异表
脚本：scratchpad/tbka/mut_tbka_task002.py。每个变异的锚点都只命中一处，先 go vet 通过再跑测试；每轮用 `git checkout` 还原，并比对主仓库指纹。收尾时 worktree 干净，主仓库指纹一致。

| ID | 变异 | 结果 | 转红测试 |
|---|---|---|---|
| N1 | anyRowHas 改成「值非 null 才算存在」 | KILLED | AllNullColumnNotMissing |
| N2 | 去掉 IsInf 守卫 | KILLED | toFloat/Inf、/-Infinity |
| N3 | 去掉 ToValidUTF8 | KILLED | ErrorBodyValidUTF8 |
| N4 | 超时改 30s | KILLED | NewEMSourceTimeout |
| N5 | 去掉 TrimRight | KILLED | ParsesFixture |
| N6 | 去掉排序 | KILLED | ParsesFixture、Tolerant |
| N7 | 排序反向 | KILLED | ParsesFixture、Tolerant |
| N8 | Coverage 键换成 LOAN_PROVISION_RATIO | KILLED | 4 个测试 |
| N9 | CET1 键换成 FIRST_ADEQUACY_RATIO | KILLED | 4 个测试 |
| N10 | CET1 键换成 NEWCAPITALADER | KILLED | 4 个测试 |
| N11 | NPL 键改错 | KILLED | 4 个测试 |
| N12 | time.Parse 失败时用零值日期而非跳过 | KILLED | Tolerant、Errors/无有效期 |
| N13 | 去掉长度检查 | KILLED | Tolerant |
| N14 | 字符串数值解析失效 | KILLED | Tolerant、toFloat |
| N15 | 非数值类型返回 0 | KILLED | toFloat/布尔、/对象、/nil 等 |
| N16 | 字符串解析失败返回 0 | KILLED | toFloat/横线、/空串 |
| N17 | 「无数据」文案改掉 | KILLED | Errors/空数组、/null |
| N18 | 「无可解析报告期」文案改掉 | KILLED | Errors/无有效期 |
| N19 | 非 200 不报错 | KILLED | Errors/非200、ErrorBodyValidUTF8 |
| N20 | decode 错误去掉 symbol | KILLED | Errors/非JSON |
| N21 | 连接错误去掉 symbol | **SURVIVED** | — |
| N22 | parse 错误去掉 symbol | KILLED | Errors/空数组、/null、/无有效期 |
| N23 | 路径改成 /api/ | KILLED | ParsesFixture |
| N24 | indicator 参数改掉 | KILLED | ParsesFixture |
| N25 | MissingFields 逆序 | KILLED | MissingFieldsOrder |
| N26 | MissingFields 无条件全列 | KILLED | 3 个测试 |
| N27 | HTTP 状态码不进文案 | KILLED | Errors/非200、ErrorBodyValidUTF8 |

结果 26/27 KILLED。

## 观察项（不构成拒绝）
- **N21 存活，对 DoD 是等价变异。** 实测连接失败时的错误为 `stock_financial_analysis_indicator_em 600036.SH: Get "http://127.0.0.1:57236/api/public/…&symbol=600036.SH": dial tcp …: connection refused`。`*url.Error` 本身带着请求 URL，URL 的 query 里就有 symbol，所以即使手动拼的 symbol 被去掉，错误里仍然含 600036.SH，DoD「每条错误都含 600036.SH」照样满足。不过 TestEMSourceFetchConnectionRefused 里的 symbol 断言因此守不住 Fetch 自己拼的前缀。如果以后改成不带 URL 的错误包装，这条断言不会报警。
- TestEMSourceFetchErrorBodyValidUTF8（HTTP 502）没有断言 symbol。另外 5 个错误表用例和连接失败用例都断言了，而且 N27 证明 502 这条路径和 500 共用同一行代码，所以不算覆盖缺口。
- discovery key_findings[3] 已说明 questions[0] 问句里的「被人类中断」以 discovery 为准（实为 Leader TaskStop），这不影响验证。
