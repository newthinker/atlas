# TASK-007 验证报告（verifier: test-bk-a）

- 被验对象：`8fdd59ba008f81562b1cf789674cc376ebe99fe7`，与 verify_baseline.head 以及判定时的主仓库 HEAD 相同。
- discovery sha256：`a69863f89ae0ad4d374269c0b378300c94c26db57156c297e35edbc82c92f0dc`，与 baseline 一致。
- 验证环境：隔离 worktree `../wt-verify-TASK-007`（detached，钉在上述全 sha）；go1.24.4 darwin/arm64；本机 aktools 127.0.0.1:8180 在线。
- 结论：**VERIFIED**
- 安全：全程**只用了 `--dry-run`，没有碰 launchctl**。两次 dry-run 都放在 `sandbox-exec` 沙箱里运行，沙箱拒绝对外连接 `*:443`、`*:80` 和本地代理 `localhost:7897`，同时把 `HTTPS_PROXY/HTTP_PROXY` 指向 `127.0.0.1:1`。这样即便代码路径出错，也不可能有消息到达 Telegram。Telegram 推送 0 次。

## 范围
`git diff --numstat 459d9b109f1f32cd6085e3c83ee4c1364529d9d8 8fdd59ba008f81562b1cf789674cc376ebe99fe7` 只有两个文件：plist `47/0`，source_integration_test.go `36/0`。与 `writes` 一致，都是新文件。bank.go 和整个 cmd/atlas 未改动，所以 TASK-006 对 dry-run 路径的结论（V1、V14）仍然适用：dry-run 时 sender 工厂的调用次数为 0。

## done_criteria 覆盖矩阵
| # | 条目 | verify_by | 证据（均为本人在 8fdd59b 上重新采集） | 判定 |
|---|---|---|---|---|
| functional[0] | 集成测试对本机 aktools 通过；600036 的 MissingFields 为空、最新期在 270 天内、三项非 NaN；`ATLAS_AKTOOLS_URL` 可以覆盖地址 | test | 2026-09-27 16:02 运行 `go test -tags integration -count=1 ./internal/bank/ -run Integration -v`，rc=0，`--- PASS: TestEMSourceIntegration600036`。设 `ATLAS_AKTOOLS_URL=http://localhost:8180/` 时 rc=0；设 `http://127.0.0.1:1` 时失败（见 error_handling），说明覆盖确实生效。I4 变异（窗口从 270 天改成 27 天）让正例转红，说明时效断言确实在比较 | PASS |
| functional[1] | 真实数据 dry-run：rc=0；招商银行三项与同时刻直查一致；包含 H 股别名行；不出现 NaN；在临时配置里再加 600919 和 002142 核对 CET1 | manual | 见下文「真实数据 dry-run」。两次 rc=0，stderr 为空，NaN 0 处。按 spec 规则从直查数据独立推算出 15 项（值、环比、同比、是否标「截至」），**15 项全部一致**。完整输出与直查结果 dev 已经写进 discovery（DoD 要求的落盘位置），我的这一份附在本报告中 | PASS |
| boundary[0] | 不带 tag 时不参与编译，单测不发网络请求 | test | `go list -f '{{.TestGoFiles}}' ./internal/bank` 的结果是 7 个文件，不含 source_integration_test.go；加 `-tags integration` 后是 8 个，包含它。**网络请求用沙箱实测**：`sandbox-exec` 拒绝对外连接 `localhost:8180`、`*:80`、`*:443`。先跑对照组：把带 tag 编译的集成测试二进制放进这个沙箱，结果 FAIL，报错 `operation not permitted`，证明沙箱确实拦住了 aktools。再把不带 tag 编译的全部单测放进同一个沙箱，rc=0，PASS。I3 变异（删掉 build tag）后，不带 tag 的 go list 就包含了这个文件 | PASS |
| error_handling[0] | 设 `ATLAS_AKTOOLS_URL=http://127.0.0.1:1` 时测试**失败**而不是 skip，且报错里有连接失败信息 | manual | rc=1，`--- FAIL` 1 行，`--- SKIP` 0 行，报错含 `dial tcp 127.0.0.1:1: connect: connection refused`。这条手工检查是有效的：I1 变异（连不上时改成 `t.Skip`）在负例下得到 rc=0、SKIP=1；I2 变异（忽略环境变量）在负例下得到 rc=0。两者都能被这一步检查发现 | PASS |
| non_functional[0] | plist 各项 | review | 见下文 plist 对照 | PASS |
| non_functional[1] | 单测全绿、覆盖率 ≥80%、零 Telegram 推送 | test | `go test -count=1 -cover ./internal/bank/` 结果 ok，覆盖率 99.7%，PASS 共 117 行。带 tag 和不带 tag 的 `go vet` 都没有输出。Telegram 推送 0 次，见上文「安全」一条 | PASS |

## 真实数据 dry-run，与 aktools 直查背对背
时间：16:02:57 开始正式配置的 dry-run，16:02:59 直查 600036.SH 与 601658.SH，16:02:59 到 16:03:01 跑临时配置的 dry-run，16:03:01 直查 600919.SH 与 002142.SZ。四次直查都返回 HTTP 200。
命令：`go run ./cmd/atlas bank report --dry-run --bank-config <配置>`，在隔离树里、沙箱内执行。

逐项比对的方法是两把独立的尺：
1. **Python 按 spec 推算**：对每个指标取最近一个非 null 的值；环比等于它减去同一指标上一个非 null 的值；同比等于它减去上年同一天那一期的值；如果该指标的报告期不等于这家银行的最新期，就要标「截至」。
2. **jq 直接读关键期**：取 2025-06-30、2025-12-31、2026-03-31、2026-06-30 这几期的原始值。

两把尺的结论一致。重点项如下：
- 宁波银行 002142 的 2026-03-31 CET1 为 null。环比 +0.19 = 9.53 − 9.34（2025-12-31，跳过了空期）；同比 −0.12 = 9.53 − 9.65（2025-06-30）。
- 江苏银行 600919 的 CET1 为 8.67（2026-06-30）；环比 +0.17，对比的是 2026-03-31 的 8.5；同比 +0.18，对比的是 2025-06-30 的 8.49。

### 正式配置（configs/bank-monitor.yaml）的输出
```
🏦 银行关键指标月报 2026-09-27
覆盖 2 家（统计期 2026-06-30：2 家；未更新 0；失败 0）

⚠️ 预警 (0)
无

📊 同期统计 2026-06-30（n=2）
不良率 均值 0.97% | 中位 0.97% | 最优 招商银行 0.94% | 最差 邮储银行 1.00%
拨备覆盖率 均值 300.01% | 中位 300.01% | 最优 招商银行 385.10% | 最差 邮储银行 214.93%
CET1 均值 12.05% | 中位 12.05% | 最优 招商银行 14.07% | 最差 邮储银行 10.04%

🏷 排名（按不良率由优到劣）
1. 招商银行 不良率 0.94%(环比+0.00/同比+0.01) 拨备覆盖率 385.10%(环比-2.66/同比-25.83) CET1 14.07%(环比-0.06/同比+0.07)
  （招商银行H 同 600036.SH）
2. 邮储银行 不良率 1.00%(环比+0.01/同比+0.08) 拨备覆盖率 214.93%(环比-1.72/同比-45.42) CET1 10.04%(环比-0.14/同比-0.48)
```

### 临时配置（只放在 scratchpad，不进仓库）
```yaml
source:
  aktools_url: http://127.0.0.1:8180
rank_by: npl
banks:
  - {market: CN_A, symbol: 600036.SH, name: 招商银行}
  - {market: HK,   symbol: 3968.HK,   name: 招商银行H, a_share_ref: 600036.SH}
  - {market: CN_A, symbol: 600919.SH, name: 江苏银行}
  - {market: CN_A, symbol: 002142.SZ, name: 宁波银行}
thresholds:
  npl_max: 1.5
  coverage_min: 150
  cet1_min: 8.5
  deterioration: {npl_up: 0.10, coverage_down: 20, cet1_down: 0.50}
```
输出：
```
🏦 银行关键指标月报 2026-09-27
覆盖 3 家（统计期 2026-06-30：3 家；未更新 0；失败 0）

⚠️ 预警 (0)
无

📊 同期统计 2026-06-30（n=3）
不良率 均值 0.84% | 中位 0.81% | 最优 宁波银行 0.76% | 最差 招商银行 0.94%
拨备覆盖率 均值 354.43% | 中位 373.35% | 最优 招商银行 385.10% | 最差 江苏银行 304.83%
CET1 均值 10.76% | 中位 9.53% | 最优 招商银行 14.07% | 最差 江苏银行 8.67%

🏷 排名（按不良率由优到劣）
1. 宁波银行 不良率 0.76%(环比+0.00/同比+0.00) 拨备覆盖率 373.35%(环比+3.96/同比-0.81) CET1 9.53%(环比+0.19/同比-0.12)
2. 江苏银行 不良率 0.81%(环比+0.00/同比-0.03) 拨备覆盖率 304.83%(环比-3.53/同比-26.19) CET1 8.67%(环比+0.17/同比+0.18)
3. 招商银行 不良率 0.94%(环比+0.00/同比+0.01) 拨备覆盖率 385.10%(环比-2.66/同比-25.83) CET1 14.07%(环比-0.06/同比+0.07)
  （招商银行H 同 600036.SH）
```

局限（与 dev 的判断一致）：这四家银行在 2026-06-30 都披露了三项指标，所以真实数据没有走到「截至」回退标注这条路径。这条路径由 TASK-003 和 TASK-005 的单测以及变异覆盖。

## plist 对照（review）
| 项 | DoD 要求 | 实际 | crisis-daily.plist 的约定 | 结论 |
|---|---|---|---|---|
| plutil -lint | OK | `OK` | — | ✓ |
| Label | com.newthinker.atlas.bank-monthly | 相同 | com.newthinker.atlas.<名> | ✓ |
| ProgramArguments | runtime 二进制 `bank report --config …/config.yaml --bank-config …/bank-monitor.yaml` | `/Users/zuowei/workspace/runtime/atlas/bin/atlas bank report --config …/configs/config.yaml --bank-config …/configs/bank-monitor.yaml` | 二进制路径相同，`--config` 路径相同，业务配置同样放在 configs/ 下。`--config` 是根命令的持久 flag（cmd/atlas/main.go:22） | ✓ |
| StartCalendarInterval | Day=1 Hour=9 Minute=0 | 用 plutil 转成 JSON 后是 `{"Day":1,"Hour":9,"Minute":0}` | crisis 用的是数组形式（多个时点），这里是单个 dict，语义上等价 | ✓ |
| no_proxy | localhost,127.0.0.1 | 相同 | 相同 | ✓ |
| RunAtLoad | false | `<false/>` | 相同 | ✓ |
| 日志 | logs/bank-monthly.{out,err}.log | `…/runtime/atlas/logs/bank-monthly.out.log` 与 `.err.log` | 同目录、同命名模式 | ✓ |
| WorkingDirectory、PATH | 路径约定一致 | 与 crisis 逐字相同 | — | ✓ |
| 没有 http(s)_proxy | —（与 crisis 不同） | crisis 需要代理，是因为 Yahoo 直连会 403；本任务只访问本机 aktools，Telegram 的代理由主配置 `notifiers.telegram.proxy` 提供（telegram.WithProxy）。**我核实过 runtime 主配置里 enabled、bot_token、chat_id、proxy 四个键都存在且非空，只检查了键是否存在，没有打印任何值** | — | 合理 |

部署提示（不属于本任务范围）：runtime 目录下还没有 `configs/bank-monitor.yaml`，`logs/` 目录已经存在。Step 6 部署时需要把这个配置文件同步过去，否则第一次触发会因为配置不存在而以 1 退出。

## 集成测试变异（在隔离树中进行，每次用 git checkout 还原，收尾时 worktree 干净）
| ID | 变异 | 由什么检出 | 结果 |
|---|---|---|---|
| I1 | 连不上时 `t.Skip` | 负例手工检查：rc=0，SKIP=1 | 检出 |
| I2 | 忽略 `ATLAS_AKTOOLS_URL` | 负例手工检查：rc=0，没有失败 | 检出 |
| I3 | 删掉 `//go:build integration` | boundary 的 go list 检查：不带 tag 时出现了这个文件 | 检出 |
| I4 | 时效窗口从 270 天改成 27 天 | 正例：FAIL | KILLED |

I1 和 I2 只能靠「127.0.0.1:1 必须失败」这条手工检查发现，测试自己不会报。这与 dev 的 M1、M2 结论相同，也正是 DoD 把 error_handling 标成 manual 的原因。I1 的第一版变异体因为 `require` 未使用，vet 不通过，判为无效；改写后才计入结果。
