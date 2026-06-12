# TASK-004 验证报告 — QLIB_DIR 切换 + README + e2e 收口（必跑）

- **判定: ✅ VERIFIED**
- 验证者: test-agent-2 | 日期: 2026-06-12 | commit: 36d476d | epoch: 1
- 包: scripts/qlib_eval (+ Makefile) | 心智: Reality Checker
- **关键纪律: 全部 e2e 亲自复跑（先把 Dev 留下的 atlas_cn 移走，clean slate 重建），非看产物。**

## Done Criteria 覆盖矩阵
| # | 完成标准 | 验证方式（亲跑证据） | 判定 |
|---|---|---|---|
| functional[0] | Makefile QLIB_DIR 默认=atlas_cn（test_makefile 更新） | test_qlib_dir_default_is_atlas_cn PASSED（_expand 后 endswith atlas_cn）；源码 `QLIB_DIR ?= $(QLIB_DATA_DIR)` | PASS |
| functional[1] | make qlib-data 成功产出 atlas_cn（instruments/calendar 校验通过） | **亲跑** clean rebuild：`qlib 数据包就绪 instruments:2 区间[2021-01-04,2026-06-12]`，build_data.verify_bundle 未抛 | PASS |
| functional[2] | D.features SH600519 首尾两天 open/close 与源 CSV 逐值一致 | **亲跑** D.features：首 2021-01-04 open1785.77/close1782.79、尾 2026-06-12 open1271.18/close1291.36，对源 CSV 对应行 **MATCH/MATCH** | PASS |
| functional[3] | make signal-eval（默认 2021-2026）报告含 ma_crossover+price_percentile 非空结果表 | **亲跑** make signal-eval→reports/signal-eval-20260612.md：1457 信号，ma_crossover 6 行 + price_percentile 393/392/370... 行非空表 | PASS |
| boundary[0] | 全量回归 go build/vet/test ./... + hook pytest 全绿 | **亲跑**：build OK / vet OK / test ./... NO FAIL / cmd/atlas -race cov 59.8%(≥35) / pytest 44 passed | PASS |
| non_functional[0] (review) | README 五要素 | review 通过（见下） | PASS |

## e2e 亲跑证据链（clean slate）
```
$ mv ~/.qlib/qlib_data/atlas_cn <移走Dev产物>; rm -rf qlib_csv   # 强制重建
$ make qlib-data
  → export-ohlcv 真实采集 600519.SH,000300.SH --from 2021-01-01（不传 --to，覆盖至当天）
  → build_data: qlib 数据包就绪 instruments:2 区间[2021-01-04, 2026-06-12]
$ D.features(['SH600519'],['$open','$close'])  # .venv python
  qlib first 2021-01-04 open=1785.77 close=1782.79  vs csv 1785.77/1782.79 → MATCH
  qlib last  2026-06-12 open=1271.18 close=1291.36  vs csv 1271.18/1291.36 → MATCH
$ make signal-eval  # 默认 QLIB_DIR=atlas_cn
  → reports/signal-eval-20260612.md: 信号总数 1457, ma_crossover 非空(6行) + price_percentile 非空(393/392/370...)
$ go build ./... = OK; go vet ./... = OK; go test ./... = NO FAIL
$ go test ./cmd/atlas -race -cover = ok coverage 59.8%
$ scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/ -q = 44 passed
```

## README 五要素 review（non_functional[0]）
1. **建包用法**：「自建数据包」§用法 — make qlib-data 链路说明 ✓
2. **复权口径**：§复权口径 — fqt=1 前复权 / factor 恒为 1 / 与「评估口径」交叉引用 ✓
3. **社区包 vs 自建包对比**：§表格 cn_data vs atlas_cn + 「QLIB_DIR 切换方法」(默认/回退命令) ✓
4. **crontab 示例**：§定时重建 cron 代码块 ✓
5. **evaluate.py 直调注意**：§直接调用 — DEFAULT_QLIB_DIR 仍 cn_data，绕 make 必须自带 --qlib-dir ✓

## plan 坑核查
- Makefile QLIB_DIR 单一真相源 `?= $(QLIB_DATA_DIR)`，未硬编码第二份路径；?= 保留覆盖回退能力 ✓
- evaluate.py DEFAULT_QLIB_DIR 故意保持 cn_data（plan T4 钉死），由 README 文档化而非静默改默认 ✓
- qlib-data recipe 显式 `--symbols $(SIGNAL_SYMBOLS)`（C1-1 BLOCKER 规避），只传 --from 不传 --to ✓
- test_makefile 无 cn_data 锚定断言残留（仅注释说明），新增机制锁 endswith atlas_cn ✓

## 备注
- 数值与 Dev discovery 记录的尾日 close(1288.91) 不同（我实测 1291.36）——live/盘中数据正常漂移；DoD 要求的是 **bundle↔源CSV 内部逐值一致**（已 MATCH），非匹配 Dev 旧值。本质未受影响。
- cmd/atlas -race 编译期 macOS ld LC_DYSYMTAB warning 为已知链接器噪声，测试 ok，非阻断。
- 本 Sprint「存在理由」（社区包默认区间产不出结果）经 e2e 实证消除：自建包默认即产出 1457 信号非空报告。
