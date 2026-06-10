# TASK-006 验证报告 — CachedCollector OHLCV TTL 缓存装饰器

- **Verifier**: test-agent-1 (Reality Checker)
- **验证时间**: 2026-06-10
- **判定**: ✅ **VERIFIED**
- **被验包**: `./internal/collector`
- **Dev 自报**: ok / -race 通过 → **独立复核一致**：ok，包覆盖率 98.9%

## 验证方法（亲自运行）
```
go build ./internal/collector/                          # OK
go vet ./internal/collector/                            # OK
gofmt -l cache.go cache_test.go                          # clean
go test ./internal/collector/ -race -cover -count=1      # ok, 98.9%
go test ./internal/collector/ -race -v -run TestCachedCollector  # 9/9 PASS, 无 SKIP
go tool cover -func                                      # 函数级核对
git ls-files / status                                    # 改动范围核对
```

## Done Criteria 覆盖矩阵（8 条）

| # | 维度 | 完成标准 | 对应测试 | 实测断言 | 判定 |
|---|------|----------|----------|----------|------|
| 1 | functional[0] | TTL 内相同 key 第二次命中，底层调用计数=1 | `TestCachedCollector_HitWithinTTL` | 2 次 fetch → backing calls==1 | **PASS** |
| 2 | functional[1] | 不同 symbol/interval/时间范围互不命中 | `TestCachedCollector_DistinctKeys` | 5 个维度各异的 key → calls==5（无串槽） | **PASS** |
| 3 | functional[2] | 命中返回副本，调用方修改不影响后续命中 | `TestCachedCollector_ReturnsCopy` | first[0].Close=-999 后 second[0].Close==100 且 calls==1（确为命中而非穿透） | **PASS** |
| 4 | functional[3] | Name/SupportedMarkets 等其余方法透传 | `TestCachedCollector_PassThrough` | Name/SupportedMarkets/FetchQuote(quoteCalls==1)/Init/Start/Stop 全透传 | **PASS** |
| 5 | boundary[0]-a | TTL 过期重新穿透刷新 | `TestCachedCollector_TTLExpiry` | TTL内 calls==1，sleep 后 calls==2 | **PASS** |
| 6 | boundary[0]-b | 超 256 淘汰最旧，总数不超上限 | `TestCachedCollector_CapacityEviction` | 插 257 → entryCount<=256；最旧(i=0)被淘汰→穿透；最近(i=256)仍命中 | **PASS** |
| 7 | boundary[0]-c | 同分钟亚秒差共享缓存槽（key 截断到分钟） | `TestCachedCollector_KeyTruncatedToMinute` | 9:30:15 与 9:30:59 → calls==1 | **PASS** |
| 8 | error_handling[0] | 底层 error 不入缓存，下次仍穿透 | `TestCachedCollector_ErrorNotCached` | 两次均报错 → calls==2 | **PASS** |
| 9 | non_functional[0] | 并发 FetchHistory（混合 key）-race 通过 | `TestCachedCollector_ConcurrentRace` | 64 goroutine 混合 key，-race 无竞争报告 | **PASS** |

> 注：boundary[0] 一条标准被拆为 3 个测试覆盖（TTL过期/容量淘汰/分钟截断），覆盖更充分。

## 覆盖率证据
- 包整体：**98.9%**（-race）
- cache.go 函数级：NewCached 100% / FetchHistory 100% / store 100% / evictOldest 100% /
  cacheKey 100% / entryCount 100% / cloneOHLCV 80%
- cloneOHLCV 未覆盖部分 = `if in == nil { return nil }` 空输入早返回分支，非 done_criteria 项，可接受。

## 实现实查
- CachedCollector 嵌入 Collector 接口仅重写 FetchHistory，其余方法零样板透传（符合 functional[3] 与 ADR-5/news.go 模式）。
- 命中路径与回填路径均 cloneOHLCV 返回副本；OHLCV 为扁平值类型，浅拷贝即深拷贝（functional[2]）。
- 底层 error 时 `return nil, err` 不写缓存（error_handling[0]）。
- 单调 seq 标记插入序，evictOldest 线性取最小 seq；store 新 key 且满容量先淘汰（boundary[0]）。
- key 时间 Truncate(time.Minute).UTC().RFC3339（boundary[0]-c）。
- 单 mu 保护 map，clone 在锁外执行；-race 通过（non_functional[0]）。
- 改动范围 = cache.go + cache_test.go（git 已跟踪提交，clean），无越界。

## 备注（不影响本任务判定）
- `go build ./...` 全量在 **`internal/collector/lixinger`** 仍报 `undefined: baseURL`，
  系其他在途任务（dev-agent-3 修改 lixinger.go + 新增 lixinger_httptest_test.go）的 WIP，
  与 TASK-006（`./internal/collector` 顶层包，build/vet/test 全绿）无关。再次提请 Leader 跟踪。

## 结论
8/8 done_criteria 均有真实、非空洞断言的对应测试，9/9 实跑通过，-race 干净，覆盖率 98.9%，
改动范围合规。**压倒性证据满足 → VERIFIED。**
