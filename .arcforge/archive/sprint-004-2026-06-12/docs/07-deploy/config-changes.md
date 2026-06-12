# 部署说明 — sprint-004（2026-06-12）

无 DB 迁移、无新依赖、无 atlas 运行时配置变更（离线工具链）。

## 行为变更

- `make signal-eval` 的 `QLIB_DIR` 默认值：`~/.qlib/qlib_data/cn_data` → `~/.qlib/qlib_data/atlas_cn`（自建包）。需用社区包时 `make signal-eval QLIB_DIR=~/.qlib/qlib_data/cn_data` 显式覆盖。

## 新工具链

```bash
make qlib-data     # 拉行情 → CSV → dump_bin → atlas_cn 数据包（分钟级）
make signal-eval   # 此后默认用自建包评估（2021-2026 默认区间有效）
```

## 定时调度（README 详述）

```cron
30 16 * * 1-5 cd /path/to/atlas && make qlib-data >> /tmp/qlib-data.log 2>&1
```

注意：cron 失败响亮（非 0），但当前**非原子换包**（OPS-1 fast-follow）——dump 进行中读包有混龄窗口；评估是手动触发，实践风险低。

## 回滚

纯增量：QLIB_DIR 覆盖回 cn_data 即回到旧行为；删除 export_ohlcv*.go/build_data.py/两个 Makefile target 即完全移除。
