# 架构决策 — Qlib 数据仓库 第二期（Part B PIT）

## AD-1: PIT 双轴 + observe_date 截断防前视
`fundamentals_pit(symbol,report_period,observe_date,...)`。Go 查询 `observe_date <= end` 截断，只见已公布数据。升序喂 `ReconstructPEPercentile` 阶梯对齐天然正确。**理由**: 现 Yahoo 用报告期末对齐有前视偏差，PIT 真实可知日修正之。

## AD-2: 修订保留不去重
同 report_period 不同 observe_date 的多行（财报修订）全部入库、按 observe_date 升序保留。更晚 observe_date 在其后收盘自动接管，更早收盘仍用旧值。**理由**: 这正是 PIT 正确性，去重会破坏历史可知性。

## AD-3: 仓库主源 + 兜底委托
qlibpit `FetchEPSHistory`：仓库有该符号基本面→PIT 主源；无→委托内层 EPSSource(yahoo)；fallback nil→空切片。**理由**: 完全可降级，仓库未覆盖的符号零影响退回现状。

## AD-4: 同次原子写（向后兼容）
`writer.write(..., fundamentals=None)`：第一期调用不传仍可用；fundamentals 与 OHLCV 同一临时库 + `os.replace`。**理由**: 不破坏第一期；原子性保证 atlas 不读半成品。

## AD-5: T6 适配第一期 wireQlibWarehouse 封装（关键偏差）
计划 T6 假设第一期 serve 内 db 为函数级变量 `qlibWarehouseDB`；但第一期 T12（QA 决策「加装配单测」）把开库封装进可测函数 `wireQlibWarehouse(cfg,reg,openFn,log) bool`，db 句柄不外露。
**决策**: T6 重构 `wireQlibWarehouse` 暴露已开的 `*sql.DB`（推荐返回 `(*sql.DB, bool)`，nil 表示未启用/打开失败），serve.go 用该 db 构造 `qlibpit.New(db, epsSourceOrNil(yahoo))` 注入；保持：①collector 注册仍在外部源之后（补尾需外部源先注册）②`SetValuationSources` 仍在 `Start` 之前（QA S1 不变量）③装配四降级分支单测（第一期 T12 的）零回归。
**理由**: 复用同一只读句柄（不重复开库），尊重第一期既有可测封装，单 package(cmd/atlas)内完成。

## AD-6: 沿用第一期降级与环境约束
validator/arcforge-write hook 缺失→Leader 手工校验 + with-task-lock 锁；sqlite v1.38.2 已在 go.mod（不再 go get/tidy）；go 命令 GOTOOLCHAIN=local；venv python + PYTHONPATH=. 跑 pytest。

## AD-7: T7 best-effort 不写单测
适配器依赖外部源/网络属集成，主干 T1-T6 不依赖其精确性即可交付验证。Makefile 用 `$(wildcard FUNDAMENTALS_US_DIR)` 守卫：目录不存在不传 `--fundamentals-dir`，dump 仍只写 OHLCV，零破坏。
