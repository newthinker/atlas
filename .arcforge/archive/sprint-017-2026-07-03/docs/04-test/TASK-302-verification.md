# TASK-302 storage.signals 配置 + serve 装配 — 验证报告

- 验证者: test-agent-1
- 结论: **VERIFIED（PASS）**
- commit: f030a29（epoch=1, dev-agent-2）/ 分支 feature/audit-optimization-wave1-cleanup

## 反向验收
- 改动 5 文件 = 4 estimated_files（config.go/config_test.go/serve.go/config.example.yaml）+ signalstore_wiring_test.go
  （serve.go 的 TDD 装配测试，属"serve.go+测试"范围）。无越界。
- 无 retention、无迁移代码（grep 确认）；interface.go 未涉及。

## Done Criteria 覆盖矩阵（测试 fresh，全量 50 包全绿）

| # | 维度 | 标准 | 证据 | 判定 |
|---|---|---|---|---|
| F0 | functional | config 支持 storage.signals.backend(memory\|sqlite)/path；缺省 sqlite + data/signals.db | SignalStorageConfig 新增；Load 归一空值→sqlite/data/signals.db；Defaults 同。TestLoad_SignalStorage_FromYAML(显式解析)、TestDefaults_SignalStorage、TestConfig_Validate_SignalBackendValid | **PASS** |
| F1 | functional | backend=sqlite→SQLiteStore(用配置 path)；backend=memory→MemoryStore，行为不变 | buildSignalStore 分支；TestBuildSignalStore_Sqlite(*SQLiteStore + Save→Count=1 往返 + 文件创建)、TestBuildSignalStore_Memory(*MemoryStore+非nil cleanup) | **PASS** |
| B0 | boundary | 老配置无 storage 节→缺省 sqlite+data/signals.db 不报错 | TestLoad_SignalStorage_DefaultsToSqlite：yaml 仅 server.port→backend==sqlite && path==data/signals.db（核心行为变化实证） | **PASS** |
| E0 | error_handling | sqlite 打开失败→serve 启动即返错退出，不降级内存 | buildSignalStore sqlite 分支 open 失败 return error，**无 fallback memory 分支**；serve.go `if err != nil { return fmt.Errorf(...) }` + `defer closeSignalStore()`；TestBuildSignalStore_SqliteOpenFailure：坏 path→err!=nil、store==nil、显式断言无内存降级 | **PASS** |
| E1 | error_handling | backend 非法值报错且信息含非法值 | Validate switch default 报 "invalid storage.signals.backend: %s"；TestConfig_Validate_SignalBackendInvalid("postgres" + errors.Is ErrConfigInvalid)、TestBuildSignalStore_InvalidBackend("bogus" in err) | **PASS** |
| N0 | non_functional | config.example.yaml 注两 backend/默认/行为变化(原内存)；go test ./... 全绿 | example.yaml storage.signals 节含 ⚠️ 行为变化标注(内存→sqlite 持久化、打开失败即退出不降级)；全量 50 包全绿 | **PASS** |

## Leader 六关注点确认
1. 缺省双路径：TestLoad_SignalStorage_DefaultsToSqlite 实证无 storage 节→sqlite/data/signals.db。
2. 快速失败无降级：读 buildSignalStore 确认 sqlite 分支 open 失败 return error，无 fallback 分支；SqliteOpenFailure 测试钉死 store==nil 无内存降级。
3. 非法值报错含值 + memory 行为不变：Invalid 测试含 "postgres"/"bogus"；Memory 测试构造 *MemoryStore（原语义）。
4. sqlite 生命周期：serve.go `defer closeSignalStore()`，buildSignalStore 返回 cleanup 关闭 store，无句柄泄漏。
5. example.yaml 显著标注行为变化：⚠️ 行为变化注释齐备。
6. 存储侧闭环：TestBuildSignalStore_Sqlite 用配置 path 真实建库 + Save→Count=1 读回 + os.Stat 确认文件；
   结合 TASK-301 TestSQLite_Persistence（关闭重开数据在），"重启进程信号仍可查"链路成立。

## 覆盖率
- config 包 94.7%（≥80 门槛达标；Validate/Defaults 100%、Load 89.3%）。
- buildSignalStore 92.3%（新码）；cmd/atlas 整包 65.3%（cov_min=35 裁决，沿用 TASK-203 先例）。

## 非阻断观察
- buildSignalStore 有一处 `if path=="" { path="data/signals.db" }` 防御性缺省（Load 已归一），冗余但无害（defense-in-depth）。

全部条目 PASS，证据充分 → VERIFIED。（本 Sprint 8 任务全部 verified。）
