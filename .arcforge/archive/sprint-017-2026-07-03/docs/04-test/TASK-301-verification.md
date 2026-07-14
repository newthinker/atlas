# TASK-301 sqlite SignalStore + 契约测试 — 验证报告

- 验证者: test-agent-1
- 结论: **VERIFIED（PASS）**
- commit: 1f39943（epoch=1, dev-agent-1）/ 分支 feature/audit-optimization-wave1-cleanup
- 依据 AD-14a 修订 DoD（from/to 为闭区间，含端点）

## 反向验收
- 改动 3 文件 = estimated_files（sqlite.go 新增、store_contract_test.go 新增、memory.go 仅补排序）。
- interface.go / go.mod / go.sum 零改动（git 确认）；modernc.org/sqlite 保持 v1.38.2。
- memory.go diff 仅新增 sort.SliceStable(generated_at ASC, id ASC) + import sort，offset/limit 为既有逻辑，无其他行为改动。

## Done Criteria 覆盖矩阵（契约测试同套用例跑 memory+sqlite 双实现，-race 全绿）

| # | 维度 | 标准 | 证据 | 判定 |
|---|---|---|---|---|
| F0 | functional | NewSQLiteStore 建库建表(IF NOT EXISTS+两索引+MkdirAll)、Save 赋 sig_<nano>_<counter> ID、GetByID 等值、未命中 errors.Is(ErrSymbolNotFound) | schema 双索引 IF NOT EXISTS；Save 用 atomic counter；GetByID sql.ErrNoRows→core.ErrSymbolNotFound；TestContract_SaveGetByID(双实现) idPattern + errors.Is | **PASS** |
| F1 | functional | List/Count 全字段 filter；排序 generated_at ASC,id ASC；memory 同步补稳定排序，两实现序一致 | buildWhere 覆盖 symbol/strategy/action/from/to；ORDER BY generated_at ASC,id ASC；memory.List 补 SliceStable；TestContract_ListFiltersAndOrder 乱序插入(30,10,20)→[10,20,30] 双实现一致 | **PASS** |
| F2 | functional | 契约测试同套表驱动跑双实现：ID/GetByID/filter/排序/分页/Count/metadata(nil/空/populated) | contractStores() 工厂对 memory+sqlite 跑同套；TestContract_MetadataRoundTrip nil/empty/populated 双实现 DeepEqual | **PASS** |
| B0 | boundary | 空库 List 空集/Count 0；关闭重开同 path 数据在 | TestContract_Empty(双实现)；TestSQLite_Persistence 关闭后重开 List==1 | **PASS** |
| B1 | boundary | limit=0 不限制(不译 LIMIT 0)；offset 越界空；from/to 闭区间(AD-14a) | List limit>0 才加 LIMIT，否则 LIMIT -1 OFFSET；TestContract_Pagination(limit=0→5,offset99→0)；TestContract_TimeRangeEndpoints [10,30]→3、[20,20]→1 双实现钉闭区间 | **PASS** |
| E0 | error_handling | path 不可用返 error 非 panic | NewSQLiteStore MkdirAll/Open/Ping/Exec 均返 error；TestSQLite_OpenError 父为普通文件→error | **PASS** |
| N0 | non_functional | sqlite 保持 v1.38.2；连接设 WAL + busy_timeout | go.mod 仍 v1.38.2；DSN `_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)` | **PASS** |
| N1 | non_functional | interface.go 零改动；memory.go 仅补排序；-race 全绿含并发 | interface.go 未在 diff；memory.go 仅 sort；TestSQLite_ConcurrentSaveList 8×10=80，go test -race 全绿无 data race | **PASS** |

## 验证者补充实证（Leader 明确要求：跨时区/亚秒）
committed 测试 `at(sec)` 仅用整秒 UTC，未覆盖跨时区/亚秒。我写临时测试（跑完已删，工作树干净）对双实现实证：
- 跨时区归一：08:00+08:00（=00:00Z）正确排在 01:00Z 之前——证明 Save/buildWhere 的 .UTC().Format(timeLayout) 归一有效（否则字符串 "08" 会错排到 "01" 后）。
- 亚秒精度：.100Z < .200Z 排序正确（timeLayout 含纳秒 .000000000）。
- 端点：亚秒闭区间 [.100,.200]→2、跨时区 [A,C]→2 均正确。
双实现均 PASS。实现的固定宽度 UTC 布局对跨时区/亚秒的排序与端点判断正确。

## 覆盖率
- signal 包整体 89.7%（TASK-301 coverage_minimum=null 无门槛）；未覆盖多为 marshal/scan/Open/Ping 错误注入路径，主路径与关键错误路径(ErrSymbolNotFound/OpenError)已覆盖。

## 非阻断观察
- committed 契约测试用整秒 UTC；建议后续可把跨时区/亚秒用例并入 store_contract_test.go（实现已正确，本报告已独立实证，不阻断）。

全部条目 PASS，证据充分（-race + 双实现 + 独立跨时区/亚秒实证）→ VERIFIED。
