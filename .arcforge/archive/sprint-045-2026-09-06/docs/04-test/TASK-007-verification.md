# TASK-007 验证报告 · M2a 收口（CONTRACTS `## Sprint M2a`，docs-only，记录员模式）

- **验证者**：test-m2a-b
- **判定**：**REJECTED**（`reason_class=task_defect`）——单一缺口：DoD `error_handling[0]` 要求「实测数字与 §B 不相等 ⇒ 成因查实写进 §B 对应行」，§B 两份回放样本的**字节数**是跨运行不可复现的数字（成因已查实，见 §3），且 3193 vs 3194 的不等在写 §B 之前已知（Leader 派验消息自述），§B 未写成因。其余 7 条 DoD 全部 PASS，修复只需在 §B 两行加一句成因。
- **判定对象**：`verify_baseline.head = f8e9a14d320a2de7ad5f832c4d64c95bbddd0793`（master，含 007 merge；dev 提交 `de2ce7e619e30bf91fe8bf282fbedaaf77ae9974`，记录员 dev-m2a-d；`7022d01..f8e9a14d` 在代码范围内**仅** `internal/hestia/CONTRACTS.md` +74/0）；discovery sha256 `2c2baa4153729f53f2864aa5ee680dbb3cc48203c59092ad78353f481485c0aa`；承接时 `assignment_epoch=4`
- **验证树**：`../wt-verify-TASK-007-b`（detached 于 `f8e9a14d…`）；二进制 `go build` 到会话 scratchpad `m2a-reg-007/`；§B 数字复核在主仓库同 sha 上跑（代码范围 `git status` 为空）
- **§B 采样锚**：`7022d01d9229314f8a43c126d9b9763bbabefe5c`；基线锚 `d27791c695e8ebd0fd5d54c9161782f52d9b12cb`

## 1. 完成标准覆盖矩阵（全部 `verify_by: review|manual`）

| # | 完成标准（摘要） | 我的核实 | 判定 |
|---|---|---|---|
| functional[0] 采锚前置 | 代码范围 `git status` 空；§B 锚全 sha；写前 numstat 空；基线 96.6/76.4 注明 | §B 表头锚 `7022d01d…` 全 sha；`git diff --stat 7022d01d f8e9a14d -- internal cmd/atlas configs go.mod go.sum` 恰 `CONTRACTS.md` 一文件 ⇒ §B 采于最后一次代码改动之后属实；§B 首段注明基线锚与 AD-7 | PASS |
| functional[1] §A | A1–A6 按原文，A5/A6 按 AD-4b 实测订正；A7/A8/A9；`writeAtomic` 文案一句；每条带证据 | 九条逐条读：A1 `Contract.FileName`/`TestContractFileName`；A2 `signals.go`；A3 `monthlyAverage`/`monthsInPeriod`/golden/sqlite 复核；A4 `UnknownShrinksDenominator`；A5 `dot` + Duplicate/OutOfOrder `0/0`；A6 `contractError`/`runRow`/P1 照发/`ContractWriteFailure…`/`snapshot` 文案接受；A7 三点 + AD-4a；A8 AD-5 + `RejectsEmptyQueueDir` + `find` 判据（含我 005 报告的变异证据）；A9 AD-14 + `NoContractOnOutOfOrder`。证据名与我 001–005 五份报告所核对象一致 | PASS |
| functional[2] §B | 各行实数 | 我在主仓库 `f8e9a14d`（代码与 `7022d01d` 相同）复核：覆盖率 **96.6 / 76.6**；gofmt 恰两处；vet 0；AST **34** / reflect **14**（+9 名单与 AD-10 一致）；不动文件 + go.mod/go.sum 0 行；`Save` grep 0；golden 三子例 PASS；`-run 'TestBuildContract\|TestContract' -count=3` ok；新增测试 **48** 逐文件（config 4 · signals 9 · contract 6 · store 3 · queue 10 · ingest 8 · notify 1 · cmd 7）与 §B **逐项相同**，差因表六行与我五份报告记录的新增用例一一对得上 | PASS |
| functional[3] 回归 + 回放样本 | 六数与 M1.5 §B 逐字相等；`queue/` 不存在；`find` 空；两份样本各记字节数、n/m、`_mom` 0/20、n+m=76、stdout 全文进 discovery | **回归**：`backfill load --allow-incomplete` exit 0；`218 = 217 + 1 · 217 = 213 + 4 · 97 = 76 + 21（单篇 28 + 合并组 69）· 字段冲突 0 · 口径路由违反 0`，与 M1.5 §B（CONTRACTS.md:3346 段）逐字相等；`hestia_runs` 0 行；主仓库根与验证树根 `queue/` 均不存在；`find` 命中 0。**样本①** 2026-06/h1：n=54 / m=22 / n+m=76 / `_mom` 在 data 0 个、在 absent 22 个 / `generated_by contract@v1/replay` / `checks: []` / `is_revision false` / `scissors_sink -2` / 顶层 17 键 / 末字节 `\n`——与 §B **全部相同**；字节数 **3194**（= §B）。**样本②** 2023-08/monthly：n=53 / m=23 / n+m=76 / `_mom` 在 data **20** 个（`_ytd` 3 个并存）/ 其余同上——与 §B 全部相同；字节数 **3166**（§B **3167**，**不等**，成因见 §3）。两份 stdout 去掉 `extracted_at` 行后与 discovery 里 Leader 的样本**逐字节相同**（`cmp`）。`--stdout` 未落盘（`$S/queue/pending` 不存在） | 内容 PASS；**字节数不等 → 归 e0** |
| functional[4] §C | 四条 + 未 deploy + merge 后 sha | 四条在；「本 Sprint 未跑 deploy.sh」在；merge 后 sha 记在 discovery `verification.merged_master_sha`（= `f8e9a14d`，被 merge 的文件装不下 merge 后的 sha，`decisions[3]` 说明合理）；挂账段与我 003/004/005 报告的残留一致 | PASS |
| boundary[0] 终检 | 只审不改；`git status` 空、HEAD 仍是锚 | 未 spawn（Leader 明令，PENDING-MECHANISMS #3）；本任务 0 个 `.go` 改动（`git show --numstat de2ce7e` 恰一行 `74 0 CONTRACTS.md`）；§B 对应行如实登记「dev 本体只读审查、未 spawn」 | PASS |
| error_handling[0] | 任一实测数字与需求/§B 不等 ⇒ 不改数字迁就，先查成因，**成因查实写进 §B 对应行** | **未满足**：(a) 3193（dev-b 试跑）vs 3194（Leader）在写 §B 前已知，§B 样本①行写 3194 无成因；(b) 我复跑样本② 得 3166 vs §B 3167。成因我已查实（§3），§B 两行都没有。按条款应写进 §B，而不是由验证者在报告里替它解释 | **FAIL** |
| non_functional[0] 交付流程 | 提交只含 `CONTRACTS.md`、锚 `docs(TASK-007): M2a …`、discovery 含 §B 数字 + 两份 stdout 全文 + 终检结论 | `git show --numstat` 恰一行；提交信息 `docs(TASK-007): M2a CONTRACTS …` 匹配 `^docs\(TASK-007\): M2a`；discovery 含 `replay_*_stdout` 全文、`section_b`、`code_simplifier`、`provenance`（前三任挂起、数字由 Leader 采、记录员抽查三项） | PASS |

## 2. 我自采的 §B 复核表（主仓库 `f8e9a14d`，代码范围 dirty=0；回归/回放在验证树二进制上）

| 项 | §B | 我采 | 相等 |
|---|---|---|---|
| `internal/hestia` 覆盖率 | 96.6 | 96.6 | ✓ |
| `cmd/atlas` 覆盖率 | 76.6 | 76.6 | ✓ |
| AST / reflect | 34 / 14 | 34 / 14 | ✓ |
| gofmt / vet / 不动文件 / `Save` / go.mod | 两处 / 0 / 空 / 0 / 无 | 同 | ✓ |
| 三期 golden / `-count=3` | PASS / 绿 | PASS / ok | ✓ |
| 新增测试逐文件 | 4·9·6·3·10·8·1·7 = 48 | 4·9·6·3·10·8·1·7 = 48 | ✓ |
| 回归六数 | 218/217/1/213/4/97/76/21/0/0 | 同 | ✓ |
| 样本① n/m/`_mom`/字节 | 54/22/0/**3194** | 54/22/0/**3194** | ✓ |
| 样本② n/m/`_mom`/字节 | 53/23/20/**3167** | 53/23/20/**3166** | 字节 ✗ |

## 3. 字节数不等的成因（查实，非推测）

- 回放契约的 `extracted_at` 取自库里该行的 `ingested_at`，而临时库由本次 `backfill load` 写入，时间戳是**本次回填的时刻**。样本①我采 `2026-09-06T03:10:55.646633Z`（27 字符），样本② `2026-09-06T03:10:55.62379Z`（**26** 字符）——Go 的 RFC3339Nano 会截掉小数秒的尾零，`…62379Z` 少一位。`sqlite3` 直查临时库 `v_hestia_current` 的 `ingested_at` 长度分别为 27 / 26，与 stdout 一致。
- 因此**字节数 = 固定内容 + `extracted_at` 长度**，每次回填都在 ±1 字节内浮动（尾零个数由微秒值决定）。dev-b 的 3193、Leader 的 3194、我的样本② 3166 都是这个机制。除 `extracted_at` 行外，我的两份 stdout 与 Leader 存进 discovery 的两份**逐字节相同**（`cmp`）。
- 推论：字节数不是这份样本的稳定指纹；n / m / `_mom` 数 / 顶层键序才是。§B 两行应改为「3194（/3167）字节，其中 `extracted_at` 为回填时刻、RFC3339Nano 截尾零，跨运行 ±1 字节」，或改记「去掉 `extracted_at` 行后的 sha256」。

## 4. 建议修复（一次返工，只改 `CONTRACTS.md` §B 两行）

在样本①、② 两行的字节数后各加一句：`（extracted_at 取回填时刻，RFC3339Nano 截尾零，跨运行 ±1 字节；dev-b 试跑 3193、验证者复跑样本② 3166，去掉该行后逐字节相同）`。不改其它任何数字。复验时我只核这两行与 numstat。

## 5. 两份回放样本（我在验证树二进制上采，供 M3 对照；与 discovery 里 Leader 的样本仅 `extracted_at` 不同）

### 5.1 `contract emit --period 2026-06 --period-type h1 --stdout`（3194 字节）

```json
{
  "schema_version": "1.0",
  "period": "2026-06",
  "period_type": "h1",
  "published_at": "2026-07-15",
  "source_url": "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/2026071512340454869/index.html",
  "article_id": "2026071512340454869",
  "caliber_version": "2025-01",
  "is_revision": false,
  "supersedes_published_at": null,
  "extracted_at": "2026-09-06T03:10:55.646633Z",
  "extractor": "rule@v2",
  "generated_by": "contract@v1/replay",
  "absent_fields": [
    "tsf_flow_mom",
    "tsf_flow_rmb_loan_mom",
    "tsf_flow_govt_bond_mom",
    "tsf_flow_corp_bond_mom",
    "tsf_flow_fx_loan_mom",
    "tsf_flow_entrust_mom",
    "tsf_flow_trust_mom",
    "tsf_flow_bankaccept_mom",
    "tsf_flow_equity_mom",
    "deposit_flow_mom",
    "deposit_household_mom",
    "deposit_corp_mom",
    "deposit_fiscal_mom",
    "deposit_nbfi_mom",
    "loan_flow_mom",
    "loan_hh_short_mom",
    "loan_hh_mlt_mom",
    "loan_corp_total_mom",
    "loan_corp_short_mom",
    "loan_corp_mlt_mom",
    "loan_bill_mom",
    "loan_nbfi_mom"
  ],
  "units": {
    "balance": "万亿元",
    "flow": "亿元",
    "ratio": "百分数"
  },
  "validation": {
    "passed": true,
    "checks": []
  },
  "thresholds": {
    "config_version": "",
    "temp_scale": "0-4",
    "signals": {
      "scissors_active": 0,
      "scissors_sink": -2,
      "hh_mlt_monthly_warm": 2000,
      "hh_short_monthly_warm": 0,
      "bill_ratio_healthy": 10,
      "bill_ratio_severe": 20,
      "corp_mlt_short_expand": 1.5
    }
  },
  "data": {
    "tsf_stock": 462.06,
    "tsf_stock_yoy": 7.4,
    "tsf_flow_ytd": 208400,
    "tsf_stock_rmb_loan": 279.16,
    "tsf_stock_rmb_loan_yoy": 5.3,
    "tsf_stock_fx_loan": 1.18,
    "tsf_stock_fx_loan_yoy": -2.9,
    "tsf_stock_entrust": 11.24,
    "tsf_stock_entrust_yoy": 0.5,
    "tsf_stock_trust": 4.62,
    "tsf_stock_trust_yoy": 4,
    "tsf_stock_bankaccept": 2.02,
    "tsf_stock_bankaccept_yoy": -2.8,
    "tsf_stock_corp_bond": 36.08,
    "tsf_stock_corp_bond_yoy": 8.9,
    "tsf_stock_govt_bond": 101.36,
    "tsf_stock_govt_bond_yoy": 14.2,
    "tsf_stock_equity": 12.49,
    "tsf_stock_equity_yoy": 5,
    "tsf_flow_rmb_loan_ytd": 107600,
    "tsf_flow_govt_bond_ytd": 64400.00000000001,
    "tsf_flow_corp_bond_ytd": 20700,
    "tsf_flow_fx_loan_ytd": 1609,
    "tsf_flow_entrust_ytd": -788,
    "tsf_flow_trust_ytd": -446,
    "tsf_flow_bankaccept_ytd": -1256,
    "tsf_flow_equity_ytd": 2933,
    "m2": 356.71,
    "m2_yoy": 8,
    "m1": 118.48,
    "m1_yoy": 4,
    "m0": 14.74,
    "m0_yoy": 11.8,
    "deposit_balance": 346.44,
    "deposit_balance_yoy": 8.2,
    "deposit_flow_ytd": 177600.00000000003,
    "deposit_household_ytd": 75800,
    "deposit_corp_ytd": 32000,
    "deposit_fiscal_ytd": 9715,
    "deposit_nbfi_ytd": 46500,
    "loan_balance": 282.63,
    "loan_balance_yoy": 5.2,
    "loan_flow_ytd": 107200,
    "loan_hh_short_ytd": -5881,
    "loan_hh_mlt_ytd": 2212,
    "loan_corp_total_ytd": 111300.00000000001,
    "loan_corp_short_ytd": 45900,
    "loan_corp_mlt_ytd": 55500,
    "loan_bill_ytd": 8143,
    "loan_nbfi_ytd": -4223,
    "rate_ibo": 1.41,
    "rate_repo": 1.43,
    "fx_reserve": 3.42,
    "fx_rate": 6.8109
  }
}
```

### 5.2 `contract emit --period 2023-08 --period-type monthly --stdout`（3166 字节）

```json
{
  "schema_version": "1.0",
  "period": "2023-08",
  "period_type": "monthly",
  "published_at": "2023-09-11",
  "source_url": "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/2025092212553479819/index.html",
  "article_id": "2025092212553479819",
  "caliber_version": "2023-01",
  "is_revision": false,
  "supersedes_published_at": null,
  "extracted_at": "2026-09-06T03:10:55.62379Z",
  "extractor": "merged@v1",
  "generated_by": "contract@v1/replay",
  "absent_fields": [
    "tsf_flow_rmb_loan_ytd",
    "tsf_flow_govt_bond_ytd",
    "tsf_flow_corp_bond_ytd",
    "tsf_flow_fx_loan_ytd",
    "tsf_flow_entrust_ytd",
    "tsf_flow_trust_ytd",
    "tsf_flow_bankaccept_ytd",
    "tsf_flow_equity_ytd",
    "deposit_household_ytd",
    "deposit_corp_ytd",
    "deposit_fiscal_ytd",
    "deposit_nbfi_ytd",
    "deposit_flow_mom",
    "loan_hh_short_ytd",
    "loan_hh_mlt_ytd",
    "loan_corp_total_ytd",
    "loan_corp_short_ytd",
    "loan_corp_mlt_ytd",
    "loan_bill_ytd",
    "loan_nbfi_ytd",
    "loan_flow_mom",
    "fx_reserve",
    "fx_rate"
  ],
  "units": {
    "balance": "万亿元",
    "flow": "亿元",
    "ratio": "百分数"
  },
  "validation": {
    "passed": true,
    "checks": []
  },
  "thresholds": {
    "config_version": "",
    "temp_scale": "0-4",
    "signals": {
      "scissors_active": 0,
      "scissors_sink": -2,
      "hh_mlt_monthly_warm": 2000,
      "hh_short_monthly_warm": 0,
      "bill_ratio_healthy": 10,
      "bill_ratio_severe": 20,
      "corp_mlt_short_expand": 1.5
    }
  },
  "data": {
    "tsf_stock": 368.61,
    "tsf_stock_yoy": 9,
    "tsf_flow_ytd": 252100,
    "tsf_stock_rmb_loan": 230.24,
    "tsf_stock_rmb_loan_yoy": 10.9,
    "tsf_stock_fx_loan": 1.82,
    "tsf_stock_fx_loan_yoy": -16.8,
    "tsf_stock_entrust": 11.33,
    "tsf_stock_entrust_yoy": 2.4,
    "tsf_stock_trust": 3.77,
    "tsf_stock_trust_yoy": -2.9,
    "tsf_stock_bankaccept": 2.67,
    "tsf_stock_bankaccept_yoy": -8.2,
    "tsf_stock_corp_bond": 31.46,
    "tsf_stock_corp_bond_yoy": -0.2,
    "tsf_stock_govt_bond": 65.15,
    "tsf_stock_govt_bond_yoy": 11.5,
    "tsf_stock_equity": 11.28,
    "tsf_stock_equity_yoy": 10.2,
    "tsf_flow_mom": 31200,
    "tsf_flow_rmb_loan_mom": 13400,
    "tsf_flow_govt_bond_mom": 11800,
    "tsf_flow_corp_bond_mom": 2698,
    "tsf_flow_fx_loan_mom": -201,
    "tsf_flow_entrust_mom": 97,
    "tsf_flow_trust_mom": -221,
    "tsf_flow_bankaccept_mom": 1129,
    "tsf_flow_equity_mom": 1036,
    "m2": 286.93,
    "m2_yoy": 10.6,
    "m1": 67.96,
    "m1_yoy": 2.2,
    "m0": 10.65,
    "m0_yoy": 9.5,
    "deposit_balance": 278.76,
    "deposit_balance_yoy": 10.5,
    "deposit_flow_ytd": 202399.99999999997,
    "deposit_household_mom": 7877,
    "deposit_corp_mom": 8890,
    "deposit_fiscal_mom": -88,
    "deposit_nbfi_mom": -7322,
    "loan_balance": 232.28,
    "loan_balance_yoy": 11.1,
    "loan_flow_ytd": 174400,
    "loan_hh_short_mom": 2320,
    "loan_hh_mlt_mom": 1602,
    "loan_corp_total_mom": 9488,
    "loan_corp_short_mom": -401,
    "loan_corp_mlt_mom": 6444,
    "loan_bill_mom": 3472,
    "loan_nbfi_mom": -358,
    "rate_ibo": 1.71,
    "rate_repo": 1.76
  }
}
```

## 6. 结论

- 7/8 条 DoD PASS；`error_handling[0]` FAIL（不等数字未写成因）⇒ **REJECTED / task_defect**。所有代码侧数字与 §B 逐项相同、回归六数与 M1.5 逐字相等、两份样本内容与 AD-16 判据全部相符——本任务的问题只在「§B 登记了一个跨运行不稳定的数字且未按 e0 写成因」，修复是两句话。

---

## 7. 更正（2026-09-06，验证者自纠，写于裁决落盘之后）

- **我的 REJECTED 判据超出了 DoD 文本**。`error_handling[0]` 原文是「任一实测数字与需求/M1.5 §B 不相等**（回归六数、n+m、覆盖率、导出面项数）**⇒ … 成因查实写进 §B 对应行」——括号是判据的**限定范围**，回放样本的**字节数不在其中**；`functional[3]` 对字节数只要求「两份都记字节数」，§B 已记。我裁决时按「任一实测数字」读、漏看了括号限定。⇒ 按 DoD 文本，§1 的 e0 行应为 **PASS**，本任务 **8/8 PASS**，`rejected` 是误判。
- Leader 补充（dev-c 三次独立复采 3192 / 3193 / 3194）与我 §3 查实的成因一致：字节数不是判据，n+m=76 与键族计数才是；§B 的 3194 / 3167 视为 Leader 那次采样值。
- **§4 的修复建议仍成立但性质改为「建议」**：在 §B 两行加一句成因，是给 M3 读者的，不是 DoD 缺口。Leader 已重派返工（epoch 5、rework_count 1），返工内容不变；`rework_count` 的这一次计数源于我的误判，如实记在此。
- 我用 python 而非 jq 数 n/m（§2），未撞 jq 括号优先级问题；「合并后 master sha 在 discovery `verification.merged_master_sha`」我在 §1 f4 行已按此核为 PASS。
- 处方（给自己）：DoD 条款带括号枚举时，枚举就是判据的全集，先对枚举再对措辞；裁决前把「我引用的条款原文」整句贴进报告，而不是复述。

---

## 8. 复验（返工第 1 轮，2026-09-06）

- **判定**：**VERIFIED**
- **判定对象**：`verify_baseline.head = 4d81143487187faf6fc324f7fdc32ad80a0f6ffd`（master，含返工 merge；dev 提交 `89888ad`，记录员 dev-m2a-d）；discovery sha256 `a4aa05397cb0ff104325636cb3fece6318edd31508d4b76e663e8007ddfb103a`；承接时 `assignment_epoch=5`、`rework_count=1`。派验通知未送达，由 idle hook 唤醒后重扫文件发现并承接。
- **复核口径**（与 Leader 事先对齐）：只核 §B 两行改动、numstat、与 §3 成因一致、其它数字未动；回归六数与两份回放样本沿用 §1/§5 已采值，不重跑。

| 项 | 核实 | 结果 |
|---|---|---|
| 改动范围 | `git diff --stat f8e9a14d 4d81143` 恰 `internal/hestia/CONTRACTS.md` 2/2；`git show --numstat 89888ad` 同；提交信息锚 `docs(TASK-007): M2a …` | ✓ |
| 只改 §B 样本①②两行 | diff 的 `-`/`+` 各 2 行，均以 `| 回放样本` 开头；去掉这 4 行后 diff 为空 ⇒ 其它数字一字未动 | ✓ |
| 新句子与 §3 成因一致 | 含「字节数跨运行 ±1–2、非判据」「`extracted_at` 取回填 load 那一刻的时间，RFC3339Nano 截尾零 ⇒ 小数位数逐次不同」「dev-b 3193 / dev-c 3192 / Leader 3194 / 验证者 3194」「去掉 `extracted_at` 行后各次 `cmp` 逐字节相同」「判据是 n + m = 76 与键族计数」；样本②行记「Leader 3167 / 验证者 3166，成因同上」——与我 §3 查实的机制与数字逐项相同 | ✓ |
| discovery | 新增 `review` 节与 `verification.replay_bytes_note`（「内嵌 stdout 的字节数 3194 / 3167 仅对 Leader 那次采样成立」）；`merged_master_sha` 更新为 `4d81143…`；原 `commit` `de2ce7e…`、两份 stdout 全文、`provenance` 保留 | ✓ |
| docs-only 快速门禁 | 四个不动文件 + go.mod/go.sum 自 `d27791c` 0 行；gofmt 仍恰两处既有欠账；代码范围无其它 diff | ✓ |

- 结论：§1 的 e0 行转 **PASS**（无论按 §7 的文本读法还是按 Leader 的裁决读法，返工后两种读法下都满足），**8/8 PASS**。`config_version` 为空串的观察（§6 后已报 Leader）为非 DoD 项，不阻断。
