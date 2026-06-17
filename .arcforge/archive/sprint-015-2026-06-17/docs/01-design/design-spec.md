# 设计规格 — digest「PE%」列

## 数据流
```
analyzeSymbol:
  analysisCtx.Fundamental = buildFundamental(...)   // 已有，含 PEPercentile(-1=不可用)
  signals = strategies(...)
  enrichSignalMetadata(signals, item, analysisCtx.Fundamental)  // ← 增参
      ├─ name（不变）
      └─ Fundamental!=nil && PEPercentile>=0 → 每条信号盖 Metadata["pe_percentile_display"]=PEPercentile
  router.Route → FlushNotifications → telegram.SendBatch
      renderTable: 末列 PE% 读 Metadata["pe_percentile_display"]，%.1f%%，无值留空
```

## 组件
- `internal/notifier/telegram/telegram.go` renderTable：header 加 `"PE%"`；行末列取 `pe_percentile_display`(float64)→`%.1f%%`，缺失空串。
- `internal/app/app.go` enrichSignalMetadata：增参 `fundamental *core.Fundamental`；去掉 name=="" 提前返回，改内部条件；盖展示键。调用点传 `analysisCtx.Fundamental`。

## 展示键约定
`Metadata["pe_percentile_display"]` float64 0–100，仅 telegram 读；router/其它 notifier 不依赖。

## 接口（供并行 dev 对齐）
- 生产端（TASK-002/app）写键名 `pe_percentile_display`，值=Fundamental.PEPercentile（float64）。
- 消费端（TASK-001/telegram）读同名键。两端键名/类型必须一致。
