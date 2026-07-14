# TASK-101 FutuBroker 遗留清理 — 验证报告

- 验证者: test-agent-1
- 结论: **VERIFIED（PASS）**
- commit: 18f16fb（epoch=1, dev-agent-1）/ 分支 feature/audit-optimization-wave1-cleanup

## 反向验收
- 改动 7 文件 = 4 estimated_files（broker.go / config.go / config.example.yaml / m4 design.md）+ 3 测试文件。
- 3 个测试文件均为必要连带：broker_test.go↔broker.go、config_test.go↔config.go 的 TDD 单测；
  executor_test.go 因原引用已删除的 `cfg.Broker.Futu.Env` 字段必须改（否则不编译）。非越界。
- 未实现被撤回的 FutuBroker（无新增 futu 实现代码）。

## Done Criteria 覆盖矩阵

| # | 维度 | 标准 | verify_by | 证据 | 判定 |
|---|---|---|---|---|---|
| F0 | functional | getBroker(futu) 走 default 返回 `unknown broker provider: futu` | test | broker.go 删 case "futu"；TestGetBroker_UnknownProviderNamesProvider 断言精确等于该串，PASS | **PASS** |
| F1 | functional | FutuConfig 结构体 + BrokerConfig.Futu 字段 + WarnHardcodedSecrets futu 条目删除；全仓无残留；go build 通过 | review | config.go diff 三处均删；`grep -rni futu --include=*.go` 无 FutuConfig/Futu 字段残留（余为"期货 futures"及测试用例）；go build ./... OK | **PASS** |
| F2 | functional | Broker.Provider 缺省 mock：enabled=true 未配 provider 时 getBroker 成功返回 mock | test | Defaults() Provider "futu"→"mock"；TestGetBroker "default provider falls back to mock" PASS | **PASS** |
| B0 | boundary | 含 futu: 段老配置 yaml 不报错（viper 静默忽略） | test | TestLoad_LegacyFutuSection_Ignored PASS | **PASS** |
| E0 | error_handling | unknown provider 错误含实际传入名 | test | 同 F0，断言含 "futu" | **PASS** |
| E1 | error_handling | Mode=live 报 `live trading not supported (paper-only)`，不引用 Futu.Env（AD-15） | test | config.go Validate 改为 Mode=="live" 直接报错，不再读 Futu.Env；TestConfig_Validate_LiveNotSupported PASS | **PASS** |
| N0 | non_functional | example.yaml 无 futu 段；m4 design 头部有撤回标注（FutuBroker 不实现/paper-only/2026-07-02） | review | example.yaml 删 futu 段、provider→mock；design.md 头部 WITHDRAWN 注记点名三要素 + AD-15 | **PASS** |
| N1 | non_functional | go test ./... 全绿；除 estimated_files 外无其他改动 | test | 全量 50 包全部 ok，无 FAIL；仅 estimated_files + 其配套/必要测试改动（见反向验收） | **PASS** |

## 非阻断观察（不影响判定，超出 TASK-101 scope）
1. 本地 `configs/config.yaml`（被 .gitignore，未跟踪）仍有 `provider: "futu"` 与 `futu:` 段。
   不在 estimated_files / done_criteria（仅点名 config.example.yaml）内，且 broker.enabled=false +
   viper 忽略 futu 段，无实际影响。建议 owner 顺手把本地 config.yaml 的 provider 改为 mock。
2. `internal/broker/types.go:252` 有一条既有注释以 "futu" 作 broker 名举例，非 FutuConfig 引用，
   dev 按外科手术原则未触碰，正确。

全部条目 PASS，证据充分 → VERIFIED。
