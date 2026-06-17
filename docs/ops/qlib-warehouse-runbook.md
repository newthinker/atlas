# ATLAS × Qlib 数据仓库 运维手册（Runbook）

> 日期：2026-06-17
> 适用：扩展方向①（qlib_eval 离线评估）+ 方向③（qlib 数据仓库，phase 1/2）已落地后的日常运维
> 关联：`docs/superpowers/specs/2026-06-15-qlib-data-warehouse-design.md`、
> `docs/reviews/2026-06-11-qlib-integration-analysis.md`

## 0. 一句话心智模型

**atlas 与 qlib 之间没有运行时服务依赖，只有数据文件依赖。**
qlib 永远是离线/批处理角色（跑完即退），atlas 是常驻在线系统。二者唯一接触面是
本地 SQLite 文件 `data/qlib_warehouse.db`（方向③）和离线评估报告（方向①）。
**任何 qlib 环节挂掉，atlas 都能独立降级运行**——这是整套设计的核心保证。

```
qlib（离线 Python，批处理，cron 每晚跑完即退）       atlas（在线 Go，7×24 常驻）
──────────────────────────────────────────        ─────────────────────────────
① qlib_eval 评估管线   → markdown IC/回测报告（人看，按需）
② warehouse dump 管线  → 原子写 data/qlib_warehouse.db ──┐
                                                          │ 只读
                                        atlas serve ◄─────┘
                                        仓库主源 + 外部 API 补新鲜尾巴
```

## 1. 前置依赖

| 依赖 | 说明 | 校验命令 |
|---|---|---|
| Go 1.24+ | 构建 atlas | `go version` |
| 预置 venv | 系统 python3 已损坏，统一走 `scripts/qlib_eval/.venv` | `scripts/qlib_eval/.venv/bin/python --version` |
| qlib 数据目录 | 方向① 评估用 `.bin`（`~/.qlib/qlib_data/atlas_us`） | `ls ~/.qlib/qlib_data/atlas_us` |
| 外部行情可用 | `export-ohlcv` 拉历史 OHLCV（Yahoo/eastmoney） | 网络可达 |

> dump 管线（warehouse-dump）本身**仅用 Python stdlib**（`sqlite3`/`csv`），不依赖
> pyqlib/pandas；它消费的是 `export-ohlcv` 已产出的 `qlib_csv_us/` CSV。

## 2. 关键路径与变量（Makefile 默认值）

| 变量 / 路径 | 默认 | 含义 |
|---|---|---|
| `QLIB_PY` | `scripts/qlib_eval/.venv/bin/python` | 统一 Python 解释器 |
| `QLIB_CSV_US_DIR` | `qlib_csv_us` | 美股 per-instrument OHLCV CSV（warehouse 的输入） |
| `QLIB_DATA_US_DIR` | `~/.qlib/qlib_data/atlas_us` | 方向① 评估用 `.bin` 数据目录 |
| `WAREHOUSE_DB` | `data/qlib_warehouse.db` | **atlas 读取的 SQLite 仓库** |
| `FUNDAMENTALS_US_DIR` | `fundamentals_csv_us` | 美股 PIT 基本面 CSV（可选，best-effort） |
| `SIGNAL_FROM` | `2021-01-01` | OHLCV/信号导出起始日 |

## 3. 一次性部署（首次）

```bash
# 1. 构建
make build

# 2. 生成美股 OHLCV CSV + qlib .bin（方向①数据包，顺带产出 warehouse 的 CSV 输入）
make qlib-data-us

# 3. （可选）产出 PIT 基本面 CSV → fundamentals_csv_us/
#    见 scripts/qlib_warehouse/ADAPTERS.md（A股 qlib dump_pit / 美股 Yahoo 近似 / 港股 lixinger）
#    缺该目录时 warehouse-dump 自动跳过基本面，仅写 OHLCV，零破坏。

# 4. 构建 SQLite 仓库（原子写：先写 .tmp 再 rename）
make warehouse-dump
# 期望输出：wrote N rows to data/qlib_warehouse.db  (N>0)

# 5. 校验仓库
scripts/qlib_eval/.venv/bin/python - <<'PY'
import sqlite3
c = sqlite3.connect("data/qlib_warehouse.db")
print("ohlcv rows     :", c.execute("SELECT COUNT(*) FROM ohlcv").fetchone()[0])
print("symbols        :", c.execute("SELECT COUNT(*) FROM warehouse_meta").fetchone()[0])
print("fundamentals   :", c.execute("SELECT COUNT(*) FROM fundamentals_pit").fetchone()[0])
print("latest last_date:", c.execute("SELECT MAX(last_date) FROM warehouse_meta").fetchone()[0])
PY
```

## 4. atlas 配置

`config.example.yaml` 默认**不含** qlib 段（缺省即关闭，行为等同今天）。要启用本地仓库，
在 `configs/config.yaml` 顶层加入：

```yaml
qlib:
  enabled: true                       # false 或整段缺省 → atlas 纯走外部 API（零行为变化）
  db_path: data/qlib_warehouse.db     # 与 WAREHOUSE_DB 一致
  max_staleness_days: 7               # last_date 超过该天数仅记 warning，仍服务（缺省 7）
```

启用后启动 atlas：

```bash
./bin/atlas serve --config configs/config.yaml
```

启动日志确认（两条都出现才算仓库真正接上）：
- `qlib warehouse collector registered`（OHLCV 仓库主源已挂）
- `qlib PIT EPS source enabled (yahoo fallback)`（PIT 基本面源已挂）

> 若库文件缺失/损坏，atlas 打印 warning 跳过注册、继续以纯外部 API 运行——**不会启动失败**。

## 5. 日常工作流（每日盘后）

atlas serve **常驻不重启**；备料由 cron 每晚跑一次，原子 rename 覆盖库文件，atlas 下个
监控周期自然只读到新库（SQLite 只读重连，无需重启 atlas）。

**严格顺序**：先 `qlib-data-us`（产出/刷新 CSV），再 `warehouse-dump`（CSV→SQLite）。
落盘脚本 **`scripts/ops/nightly-warehouse.sh`** 已实现这条流水线（仓库根目录自脚本位置推导，
免硬编码绝对路径），各步骤：

1. `make qlib-data-us` — 刷新美股 OHLCV CSV（+ `.bin` 数据包）
2. （可选）刷新 PIT 基本面 CSV → `fundamentals_csv_us/`（脚本内注释处接入适配器，缺则自动跳过）
3. `make warehouse-dump` — 重建 SQLite 仓库（原子写）
4. 健康校验 — 库非空且 `last_date` 可解析，否则非零退出
5. 校验通过 → `cp` 一份 `.bak`（回滚永远指向「上一份验证通过」的库，见 §9）

手动跑一次：

```bash
bash scripts/ops/nightly-warehouse.sh
# 末行期望：ok: N ohlcv rows, M fundamentals, last_date=YYYY-MM-DD (age Nd)
```

> 可用环境变量覆盖默认：`QLIB_PY`、`WAREHOUSE_DB`（与 Makefile 同名变量对齐）。
> 脚本是该流程的唯一真相源；本节只描述步骤，不再内联 shell，避免两处漂移。

cron（美股收盘 16:00 ET 之后，取 23:30 ET；按服务器时区调整）：

```cron
# m h dom mon dow  command
30 23 * * 1-5  /bin/bash /Users/zuowei/workspace/go/src/github.com/newthinker/atlas/scripts/ops/nightly-warehouse.sh >> /var/log/atlas/warehouse.log 2>&1
```

> 仅工作日（`1-5`）。脚本失败（`set -e` + 校验非零退出）会在日志留痕，但
> **不影响正在运行的 atlas**——它继续用上一份库 + 外部 API 补尾。

## 6. 离线策略评估（方向①，按需，非每日）

上线新策略或调参前，用 qlib 引擎做 IC/回测验证（与线上 atlas 完全解耦）：

```bash
make signal-eval-us
# 产出 markdown 报告（默认 $(SIGNAL_OUT)），含年化收益/最大回撤/换手/IC/IR
```

失败仅意味着少了一份离线报告，**对线上 atlas 零影响**。

## 7. atlas 运行期行为（理解降级）

| 场景 | FetchHistory（OHLCV） | FetchEPSHistory（PE 分位） |
|---|---|---|
| 仓库有该符号 | 读 SQLite 历史段 + 外部 API 补 `last_date→今日` 尾巴 | PIT 点对时间查询（消前视偏差） |
| 补尾 API 失败 | 仅返回仓库段 + warning（可降级） | —— |
| 仓库 `last_date` 陈旧（>`max_staleness_days`） | 仍服务仓库段 + 补尾 + warning | —— |
| 仓库无该符号 | 完全回落外部 API（零行为变化） | 委托 yahoo → reconstruct → lixinger 兜底 |
| 库缺失/损坏/`enabled:false` | 不注册仓库，纯外部 API | 纯 yahoo/lixinger（同今天） |
| 实时 quote / 分钟频 | 始终走外部 API（仓库只存日线） | —— |

## 8. 故障排查

| 症状 | 排查 | 处置 |
|---|---|---|
| 启动无 `qlib warehouse collector registered` | `enabled` 是否 true、`db_path` 是否存在可读 | 修配置或先 `make warehouse-dump` |
| `wrote 0 rows` | `qlib_csv_us/` 是否为空/陈旧 | 先 `make qlib-data-us` 刷新 CSV |
| 健康校验 `WAREHOUSE EMPTY` | dump 上游 export-ohlcv 是否失败 | 查 `warehouse.log`，确认外部行情可达 |
| PE 分位仍走 yahoo（非 PIT） | `fundamentals_pit` 是否有该符号 | 产出 `fundamentals_csv_us/` 后重跑 dump |
| atlas 读到半成品库 | 不应发生（原子 rename）；若见 `.tmp` 残留 | 删 `data/qlib_warehouse.db.tmp`，重跑 dump |
| 数据回归/异常 | —— | 见第 9 节回滚 |

日志过滤：

```bash
# atlas 侧 qlib 相关日志
journalctl -u atlas | grep -iE 'qlib|warehouse|tail-fill|stale'
# 仓库 dump 侧
tail -f /var/log/atlas/warehouse.log
```

## 9. 回滚 / 应急关停

最快的「禁用仓库、回到纯外部 API」有两种，**都不需要改代码、不需要重建库**：

1. **配置关停**（推荐）：`configs/config.yaml` 设 `qlib.enabled: false` → 重启 atlas。
2. **移走库文件**：`mv data/qlib_warehouse.db /tmp/` → 重启 atlas（缺库自动跳过注册）。

回滚到上一份好库（若保留了备份）：

```bash
cp data/qlib_warehouse.db.bak data/qlib_warehouse.db   # 见下方备份建议
# atlas 无需重启，下个周期只读重连即可读到
```

> 备份建议：在 `warehouse-dump` 成功且健康校验通过后，再 `cp` 一份 `.bak`，使回滚永远
> 指向「上一份验证通过」的库。可加进 nightly 脚本第 [4] 步之后。

## 10. 边界与已知约束

- **频率锚点**：仓库仅日线；实时告警路径永远走外部 API，qlib 不介入。
- **美股 PIT 为近似**：`observe_date` 用披露滞后近似（优于现状的报告期末对齐，但非精确
  备案日），见 `scripts/qlib_warehouse/ADAPTERS.md`。
- **A股/港股基本面**：best-effort，缺失时该符号 PE 分位自动回落 yahoo/lixinger。
- **运维成本**：每日 dump 是真实负担；但全链路可降级，失败不阻断线上。
```
