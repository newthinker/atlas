# TASK-001 验证报告（verifier: test-bk-a）

- 被验对象：`d647428d4b543ed42b8d1f51e8899df717a40517`（= verify_baseline.head；判定时主仓库 HEAD 相同，声明范围内零漂移）
- discovery sha256：`388414d099df5d8aff36dfe4d41aae976bfd26fa6f367f04287b27fede9f3e23`（= baseline，未漂移）
- 验证环境：隔离 worktree `../wt-verify-TASK-001`（detached @ 上述全 sha），go1.24.4 darwin/arm64
- 结论：**VERIFIED**

## 范围核对（越界申报）
`git diff --stat b2f8546 d647428d4b543ed42b8d1f51e8899df717a40517` 仅 5 个文件，与 `writes` 完全一致：
types.go(+47) / config.go(+144) / config_test.go(+202) / helpers_test.go(+12) / configs/bank-monitor.yaml(+19)。
排除这两个目录后 diff 为空；`go build ./...` 通过。

## 非功能（重采，不采信自证数字）
| 命令 | 结果 |
|---|---|
| `go vet ./internal/bank/` | 无输出，rc=0 |
| `go test -count=1 -v -coverprofile ./internal/bank/` | rc=0，`--- PASS` 42 行 / `--- FAIL` 0 行 |
| 计数互验 | 尺 1（输出）：顶层 `^--- PASS` 11 + 缩进子测试 31 = 42 = `=== RUN` 42；尺 2（源码）：`func Test*` 11 个，子测试 RankBy 3 + Rejects 10 + BadThresholds 6×3=18 = 31。两尺一致 |
| 覆盖率 | 97.9%（LoadConfig 92.9%，未覆盖为 `v.Unmarshal` 错误分支；其余函数 100%） ≥ 80% |

## done_criteria 覆盖矩阵
| # | 条目 | 对应测试 | 断言是否测到 | 变异证据 | 判定 |
|---|---|---|---|---|---|
| functional[0] | 缺省 aktools_url、缺省 rank_by⇒IndNPL、HK DataSymbol=a_share_ref / CN_A=symbol、DisplayName 回退、阈值 6 字段解析、rank_by cet1/coverage 映射 | TestLoadConfigValid、TestLoadConfigRankBy(npl/coverage/cet1)、TestLoadConfigExplicitAktoolsURL | 逐字段 assert.Equal；6 个阈值逐一断言数值 | M3/M4/M5/M6/M7/M21 全 KILLED | PASS |
| functional[1] | 自带 yaml 可加载，3 条目 + 阈值 1.5/150/8.5、0.10/20/0.50 | TestShippedConfigLoads | 整结构体相等（条目全字段 + ThresholdsCfg 全字段），非仅条数 | M23（cet1_min 8.5→8）、M24（改名）KILLED | PASS |
| functional[2] | Indicators 顺序、Label、HigherIsWorse 仅 NPL；day() 只在 helpers_test.go | TestIndicators、TestDayHelper；`grep -rn "func day(" internal/bank/` 仅 helpers_test.go:6 一处 | 顺序整切片相等、Label 拼接相等、三个 HigherIsWorse 各自断言 | M8/M9/M10 KILLED | PASS |
| boundary[0] | 拒绝表 9 类文案 | TestLoadConfigRejects（10 例：9 类 + 缺阈值） | 每例 `assert.Contains(err, 文案)`，文案与 DoD 逐字一致 | M2（港股前导零）、M15、M16、M17、M18、M19、M20、M25 KILLED，且红的正是对应子测试 | PASS |
| boundary[1] | 6 字段 × {0,-1,.nan} 各一例，错误含字段名 | TestLoadConfigRejectsBadThresholds（18 子测试 + 合法对照组） | `Contains "thresholds.<全名> 须 > 0"`；对照组证明拒绝只由被替换值引起 | M1（!(v>0)→v<=0）红恰好 6 个 `.nan` 子测试；M13/M14（漏校某字段）红该字段 3 例 | PASS |
| error_handling[0] | 不存在⇒`reading bank config`；YAML 语法错⇒错误；校验失败含 `invalid bank config <path>` | TestLoadConfigMissingFile、TestLoadConfigBadYAML、TestLoadConfigInvalidIncludesPath | 前后两者断言文案；BadYAML 仅 `require.Error`（见观察项） | M11/M12 KILLED；M22 SURVIVED（见下） | PASS |
| non_functional[0] | vet 无输出、test 全绿、覆盖率 ≥80% | 命令行重采 | 见上表 | — | PASS |

## 变异表（脚本 scratchpad/tbka/mut_tbka_task001.py；每个变异体锚点唯一命中、go vet 通过后才跑测试；每轮 `git checkout` 还原并比对主仓库指纹，收尾 worktree 干净、主仓库指纹一致）
| ID | 变异 | 结果 | 转红测试 |
|---|---|---|---|
| M1 | `!(f.v > 0)` → `f.v <= 0` | KILLED | BadThresholds 6 个 `=.nan` |
| M2 | dedupKey 不去前导零 | KILLED | Rejects/港股前导零重复 |
| M3 | 缺省 aktools_url 置空 | KILLED | Valid |
| M4 | rank_by 缺省改 cet1 | KILLED | Valid |
| M5 | rankKeys coverage→IndCET1 | KILLED | RankBy/coverage |
| M6 | HK DataSymbol 取 symbol | KILLED | Valid、Shipped |
| M7 | DisplayName 不回退 | KILLED | Valid |
| M8 | Label 顺序互换 | KILLED | Indicators |
| M9 | HigherIsWorse 含 Coverage | KILLED | Indicators |
| M10 | Indicators 顺序互换 | KILLED | Indicators |
| M11 | reading 文案改写 | KILLED | MissingFile |
| M12 | invalid 错误去掉 path | KILLED | InvalidIncludesPath |
| M13 | 漏校 deterioration.cet1_down | KILLED | cet1_down 三例 |
| M14 | 漏校 coverage_min | KILLED | coverage_min 三例 |
| M15 | 港股正则放宽为 `^.*$` | KILLED | Rejects/港股格式 |
| M16 | A 股正则后缀可选 | KILLED | Rejects/A股格式、映射格式 |
| M17 | a_share_ref 只判空 | KILLED | Rejects/映射格式 |
| M18 | 判重关闭 | KILLED | Rejects/重复、港股前导零重复 |
| M19 | 空列表不拒 | KILLED | Rejects/空列表、InvalidIncludesPath |
| M20 | rank_by 不校验 | KILLED | Rejects/rank_by |
| M21 | coverage_down 的 mapstructure tag 改名 | KILLED | 8 条 |
| M22 | 吞掉 YAML 语法错误（仅 `While parsing`） | **SURVIVED** | — |
| M23 | 自带 yaml cet1_min 8.5→8 | KILLED | Shipped |
| M24 | 自带 yaml 邮储名改写 | KILLED | Shipped |
| M25 | 非法 market 不拒 | KILLED | Rejects/市场非法 |

24/25 KILLED。

## 观察项（不构成拒绝）
- **M22 存活**：吞掉 YAML 语法错误后，空配置随即被 validate 以「banks 不能为空」拒绝，`LoadConfig` 仍返回错误。DoD 该条只要求「返回错误」，所以对 DoD 来说这是等价变异，不算缺陷。测试断言弱于测试名：TestLoadConfigBadYAML 无法区分「语法错被报出」与「语法错被吞、后续校验失败」。实测语法错的真实文案为 `reading bank config: While parsing config: yaml: line 1: did not find expected node content`，若后续要加强，可断言 `While parsing` 或 `reading bank config`。
- 判重在格式校验之前执行，所以格式非法且重复的条目报「重复」而不是格式错误；DoD 未作约束，不影响判定。
