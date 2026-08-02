# TASK-004 验证报告 — 配置接线(BaostockBaseURL + example 密钥条目)

验证者: test-agent-15 | 日期: 2026-08-02 | 判定: **VERIFIED (PASS)**

## 亲跑输出摘要
```
GOTOOLCHAIN=local go test ./internal/config/ -count=1 -cover
  ok  github.com/newthinker/atlas/internal/config  0.318s  coverage: 95.9% of statements

定向复跑:
  TestPrismConfigAkshareDefaults   PASS (既有用例未被破坏)
  TestPrismConfigBaostockDefaults  PASS (本任务新增)

go build ./... exit=0;go vet(config) exit=0
ruby -ryaml 解析 configs/config.example.yaml → OK(语法有效)
```

## 完成标准覆盖矩阵

| # | 完成标准 | verify_by | 对应测试/证据 | 判定 |
|---|---|---|---|---|
| functional[0] | PrismConfig.BaostockBaseURL 空时 ApplyDefaults 置为 http://127.0.0.1:8181 | test | `internal/config/prism_test.go:TestPrismConfigBaostockDefaults`:对零值 `PrismConfig{}` 调 `ApplyDefaults()` 后 `assert.Equal("http://127.0.0.1:8181", c.BaostockBaseURL)`。**断言非空洞**:零值结构体上该字段只可能由 ApplyDefaults 赋值,删掉 `config.go:509-511` 的 if 分支该断言必失败。实现见 `config.go:509` | PASS |
| boundary[0] | 显式配置的 BaostockBaseURL 不被默认值覆盖 | test | 同一用例第二段:`PrismConfig{BaostockBaseURL:"http://127.0.0.1:9999"}` 调 ApplyDefaults 后仍为 9999。覆盖「if 写成无条件赋值」这一失败形态 | PASS |
| non_functional[0] | config.example.yaml 增 collectors.tushare 与 collectors.twelvedata 的 `api_key: ""` 空值 + 密钥卫生注释;不含任何真实 key 片段 | review | 验证者独立用 ruby YAML 解析实证键值:`collectors.tushare = enabled:false api_key:"" markets:["CN_A","HK"]`、`collectors.twelvedata = enabled:false api_key:"" markets:["US"]`——两条 api_key 确为**空字符串**而非占位或 ${ENV}。卫生注释三行在 diff 中可见:「真实密钥只写 runtime 的 configs/config.yaml(已被 .gitignore 忽略),不入本示例文件、不入 launchd plist、不入日志」。真实 key 片段:哨兵扫描命中 0(见下节) | PASS |
| non_functional[1] | 密钥哨兵:runtime key 前 8 位在暂存区出现次数为 0 | manual | 验证者独立复跑,`git diff --cached` 及其余全部扫描范围命中数均为 0(见下节) | PASS |

## 密钥哨兵独立复跑(与 TASK-002 同一次扫描,结论共用)
SENT1/SENT2 = runtime `configs/config.yaml` 第 30/34 行两个 api_key 的前 8 字符
(现取,长度各 8,全程未打印)。`grep -F` 命中数:`internal/config/` 0、
`configs/config.example.yaml` 0、`git diff` 0、`git diff --cached` 0、
未跟踪新文件 0。**反向对照**:同一对哨兵在 `configs/config.yaml` 命中 2 次
(证明 grep 有效、非假阴性);`git check-ignore -v configs/config.yaml` 命中
`.gitignore:30`,真实密钥文件确被忽略。

## 越界申报核对
TASK-004 声明 `packages=[./internal/config]`、`writes=[./internal/config, configs/config.example.yaml]`。
`git diff --stat` 实际改动仅三文件:`internal/config/config.go`(+4)、
`internal/config/prism_test.go`(+18)、`configs/config.example.yaml`(+14),
**全部落在声明范围内,无越界未申报**。

## 自主决策复核(dev 主动申报的 DoD 外增量)
discovery `decisions[0]` 申报:example 的 prism 段额外补了
`baostock_base_url: "http://127.0.0.1:8181"` 一行,DoD 未要求。验证者核对:
该行落在已声明的 `configs/config.example.yaml` 内、与紧邻的 `akshare_base_url`
风格一致、值与 ApplyDefaults 默认值相同,YAML 解析实证 `prism.baostock_base_url`
= `http://127.0.0.1:8181`,无行为影响且不引入不一致。**不构成越界,不影响判定**;
是否保留由 Leader 定夺。

另,dev 额外在测试中断言了 mapstructure 标签为 `baostock_base_url`(照
`TestPrismInstrumentFallbackSourceTag` 既有惯例)。这是加强而非削弱:标签写错时
默认值断言仍会过、但 YAML 配置会静默失效,该断言正好堵住这一失败形态。验证者
以 ruby 解析出的 YAML 键名 `baostock_base_url` 与之交叉印证,两者一致。

## 观察(非阻断)
- 任务描述提到的 `TestPrismConfigApplyDefaults` 在本仓不存在,dev 按既有命名惯例
  改用 `TestPrismConfigBaostockDefaults` 并与 `TestPrismConfigAkshareDefaults` 并列,
  已在 discovery `key_findings[1]` 申报。验证者确认既有 Akshare 用例仍 PASS,
  未被改写或削弱。
- example 中 tushare/twelvedata 用空串 `""` 而非既有 `${ENV_VAR}` 语法,系 DoD 明文
  要求;discovery 已说明二者语义差异。记录备查。

## 判定
4 条 done_criteria(2 test + 1 review + 1 manual)**逐条通过**;config 包覆盖率
95.9% 高于 80% 门槛;既有用例无回归;`go build ./...` 通过;example.yaml 语法有效
且键值符合预期;无越界申报;密钥哨兵干净。→ **VERIFIED**
