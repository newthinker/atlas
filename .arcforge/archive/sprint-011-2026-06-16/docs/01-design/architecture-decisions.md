# 架构决策 — Qlib 数据仓库 第一期

## AD-1: 仓库为权威主源，外部 API 仅补尾
`FetchHistory` 读 `[start, min(end,last_date)]` 仓库段；`end>last_date` 且配置了 external 时调 `external.FetchHistory(last+1d, end)` 拼接。外部失败只返回仓库段（不报错）。
**理由**: 降低实时依赖；历史数据稳定，外部只需补最近几天。

## AD-2: 完全可降级
缺库/库不可读 → serve.go 跳过注册 qlib，路由退回现状。补尾失败 → 降级返回仓库段。陈旧 → warning 仍返回数据。
**理由**: 零回归是硬约束；外部依赖必有降级路径。

## AD-3: 纯 Go SQLite 驱动 `modernc.org/sqlite`
避免 cgo，保持交叉编译能力。只读 `?mode=ro` 打开。

## AD-4: dump 原子写（临时库 + os.replace）
写 `<db>.tmp` 再 `os.replace` 覆盖，保证 atlas 永不读到半成品库。`dumped_at` 由调用方传入（CLI 层才取系统时间），便于测试。

## AD-5: selector 双函数 + 接口解耦
`SelectForSymbol` 优先 qlib（`Covers` 命中），回落 `SelectExternalForSymbol`（永不返回 qlib，GetAll 兜底跳过 qlib，避免补尾递归到自己）。用 `warehouseCoverer interface{ Covers(string) bool }` 避免 collector 包 import qlib 包（防循环依赖）。

## AD-6: Realistic Scope —— 计划 Task10 拆为 3 任务
计划 Task10 同时改 `internal/config` + `internal/app` + `cmd/atlas`（3 package），违反「≤1 package」。拆为 T10(config) / T11(app registry 导出) / T12(serve 装配)；T12 依赖 T10/T11 及 qlib 链/selector 完成。
**理由**: 单 owner 单 package，三者可并行（T10/T11 与 Go 链并行），仅 T12 汇聚装配。

## AD-7: 降级 —— Go validator 与 arcforge-write hook 缺失
本仓库无 `validator/` 目录、无 `arcforge-write.sh`。
- 任务图校验改为 Leader 手工执行（DAG 无环 / wave 序 / scope 互斥 / 单 owner），结论记入 02-plan。
- 状态写入改用 `.claude/hooks/with-task-lock.sh <TASK-ID> <cmd>` 锁临界区（认领协议 epoch 仍生效）。
**理由**: 外部依赖必有降级路径；机制缺失不阻断流程，但须显式记录与人工兜底。
