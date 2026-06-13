# 架构决策记录（ADR）— 美股 signal-eval

## ADR-1：镜像 HK 模式而非抽象市场注册表
- **决策**：沿用 sprint-009 的 `switch market` 分支风格，给每个键控点加 `case "us"`，
  不引入市场配置表/插件机制。
- **理由**：CN/HK 已用此风格跑通；US 是第 3 个市场，分支成本低、零回归风险最小、
  reviewer 易核对。过度抽象反而增加 blast radius。

## ADR-2：`usTickerRe` 包级 var 由 Task 1 引入、Task 2 复用
- **决策**：正则 `^[A-Z]{1,5}$` 定义在 Task 1（`toQlibInstrument`），Task 2 的 `inMarket`
  直接复用同一 var。
- **理由**：单一真相源，避免两处正则漂移。**导致 Task 2 强依赖 Task 1**（且同改
  `export_ohlcv.go` 同文件），二者必须串行。

## ADR-3：跨语言契约用全串锚定（fullmatch / `^...$`）
- **决策**：Go `^[A-Z]{1,5}$`、Python `re.fullmatch(r"[A-Z]{1,5}")`，注释互相引用。
- **理由**：防 `AAPL123`/`AAPL.B` 被部分匹配误接。两侧锚定方式等价，契约测试用相同
  负向样本双向验证。

## ADR-4：region 参数默认 cn（向后兼容优先）
- **决策**：`QlibPriceSource(region="cn")`、`evaluate.py --region default=cn`。
- **理由**：CN/HK 现有调用零改动即保持原行为；US 显式传 `us`。降级时 US 也可回退 cn。

## ADR-5：Python 侧任务串行（scope 互斥）
- **决策**：Task 3(symbols)、Task 4(prices/eval)、Task 5(makefile) 虽改不同文件，但同属
  `scripts/qlib_eval` package（hook 按目录跑 pytest），故用 dependency 串成链，
  保证任意时刻该 package 只有一个在途任务。
- **理由**：两个 Dev 同时在 `scripts/qlib_eval` 跑 pytest 会互相看到未完成改动 → 假失败。
  Task 3 依赖 Task 4（串行），Task 5 依赖 Task 3+Task 4。

## ADR-6：能力降级（ecc/codex/gemini 不可用）
- `arcforge.config.json` capabilities：ecc=false、codex_cli=false、gemini_cli=false。
- **多模型规划降级**：不调 ECC `/multi-plan`，需求已是终版实现计划，Leader 直接拆分。
- **对抗审查降级**：QA 跨模型 → 纯 Claude 跨视角（correctness / regression / contract-symmetry）。
- **validator 缺失**：项目未安装 `validator/`，Leader 手动执行其校验规则（DAG/wave/scope）。
- 均在 final-report 注明。

## ADR-7：调度模式 = dag
- config `scheduling: dag`：就绪条件 = dependencies 全部 `verified`，就绪即派。
- 依赖图已保证任意就绪集 packages 互斥（Python 链串行、Go 链串行、跨语言并行）。
