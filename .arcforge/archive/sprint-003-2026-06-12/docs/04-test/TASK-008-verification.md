# TASK-008 验证报告 — 端到端串联 + Makefile + README 收尾（Sprint 收口）

- **验证人**: test-agent-1 (Reality Checker)
- **判定**: ✅ **VERIFIED**
- **plan 对应**: Task 9 / commit 038f49b
- **包**: ./scripts/qlib_eval + Makefile

## 实测命令与输出（全部亲自跑，不信 dev 自述）
```
go build ./...            → BUILD OK (exit 0)
go vet ./...              → clean
go test ./...             → 48 packages ok, NO FAIL/panic/race
pytest（仓库根 hook 同款） → 28 passed
evaluate.py --qlib-dir <不存在> → 打印 get_data 下载指引, exit 1
```

## Done Criteria 覆盖矩阵
| # | 完成标准 | 对应测试/证据 | 判定 |
|---|---|---|---|
| functional[0] | make signal-eval target 存在且命令链正确（export-signals 依赖 + venv python 非裸 python 调 evaluate.py） | test_makefile.py 4 测试（target存在/依赖export-signals/invoke evaluate.py/展开后用 venv python 且无裸 python token）+ 亲 grep Makefile L24-26 | PASS |
| functional[1] | 端到端 go build/vet/test ./... + qlib_eval pytest 全绿 | 亲跑：build OK、vet clean、48 pkg ok 无 FAIL/race、28 passed | PASS |
| boundary[0] | qlib 数据缺失 make signal-eval 走 evaluate.py 指引输出且非 0 退出（不 panic 不静默） | 亲跑 evaluate.py --qlib-dir <不存在> → 打印 SunsetWolf get_data 指引 + exit 1 | PASS |
| non_functional (review) | README 口径六要素完整 | README §评估口径 L74-90：入场L77/顺延近似L78-79/超额L81/规避L82/胜率L87-88/数据局限L36+缺口分类L89-90 | PASS |

## Reality Check
- **全量回归亲自执行**（非信 discovery 自述）：Go 48 包 ok、vet clean、build OK、零 FAIL/panic/race；pytest 28 passed。
- **venv python 守门为真**：Makefile signal-eval recipe 用 `$(QLIB_PY)=scripts/qlib_eval/.venv/bin/python`；
  test_signal_eval_uses_venv_python_not_bare_python 展开 $(QLIB_PY) 变量后断言含 venv python 且去掉该路径后无裸 `python` token——
  机制性防回归（系统 python3 dyld 损坏，裸 python 会崩）。grep Makefile 仅 L11 变量定义出现 python 路径，recipe 无裸 python。
- **缺数据路径非空洞**：evaluate.py 启动即检测 qlib-dir（design §5），缺失→stderr 打印带 target_dir 的 get_data 命令→exit 1；
  非 panic 非静默 0 退出。test_main_exits_when_qlib_dir_missing 单测亦覆盖。
- **README 六要素逐条核对**：入场次日开盘规避前视 / max_defer*2 日历日近似（5交易日≈7-10取*2上界）/
  超额相对 SH000300 / sell 规避 -(ret-bench) / 胜率超额>0 buy-sell 统一 / 数据截止局限 + 缺口三分类。全部具体可查。

## 备注（非阻塞）
- plan T9 的 gitnexus analyze / code-simplifier / 最终集成提交由后续 QA 阶段承接（不在 TASK-008 DoD 内）。

## 结论
四条 DoD 全部真实证据覆盖，全 Sprint 集成回归（Go 48 包 + Python 28 测试）亲自跑全绿，
缺数据指引路径与 venv python 守门均为真实机制。VERIFIED。本 Sprint 我负责的全部任务核验完毕。
