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

## 3. 部署与服务脚本（代码目录 → runtime → 系统服务）

**职责分离**：
- **代码目录**（dev）：`/Users/zuowei/workspace/go/src/github.com/newthinker/atlas` —— 源码、构建、测试
- **运行时目录**（runtime）：`/Users/zuowei/workspace/runtime/atlas` —— 仅编译产物 + 运行依赖（**无 Go 源码**）

脚本均在代码目录 `scripts/ops/` 下（自包含、可重复执行，路径从脚本位置推导；
runtime 可用 `ATLAS_RUNTIME` 环境变量覆盖）：

| 脚本 | 作用 |
|---|---|
| `deploy.sh` | 代码目录 `make build` → 把运行时产物 rsync 到 runtime。剥离 `*.go` 但**保留 `internal/api/templates/` 等运行时资产**；`--delete` 同步代码/脚本，但排除并保护 runtime 本地数据（`data/ logs/ qlib_csv*/ fundamentals_csv*/ signals*.csv reports/`）；收紧 `config.yaml` 为 600 |
| `install-services.sh` | 安装并加载 3 个 LaunchAgent（serve + refresh-us + refresh-cnhk）。幂等；清理旧的单一 warehouse-dump；已处理 bootout→bootstrap 竞态（EIO 重试） |
| `services.sh <cmd>` | 日常管理：`status / restart / stop / start / refresh-us / refresh-cnhk / logs / refresh-logs <us\|cnhk> / uninstall` |
| `refresh-market.sh <us\|cnhk>` | 刷新指定市场组 OHLCV（`-o build` 免编译复用部署二进制）→ `warehouse-dump-all` 全量重建含全部市场的仓库。由 launchd 分时段调度（见 §5） |

### 首次部署

```bash
cd /Users/zuowei/workspace/go/src/github.com/newthinker/atlas

# 1. 确保代码目录 configs/config.yaml 已配置（含密钥，gitignore 不入库；qlib 段见 §4）。
#    deploy.sh 会把它 rsync 到 runtime 并 chmod 600。

# 2. 构建 + 部署运行时产物到 runtime
bash scripts/ops/deploy.sh

# 3. 安装并加载系统服务（用户级 LaunchAgent，无需 sudo）
bash scripts/ops/install-services.sh

# 4. 首次需要数据：手动各触发一次刷新+重建（不等定时点），跟随日志等 "ok: N ohlcv rows ..."
bash scripts/ops/services.sh refresh-us       # 美股
bash scripts/ops/services.sh refresh-logs us
bash scripts/ops/services.sh refresh-cnhk     # A 股 + 港股
bash scripts/ops/services.sh refresh-logs cnhk

# 5. 校验服务（期望 health=200）
bash scripts/ops/services.sh status
curl -s -o /dev/null -w "health=%{http_code}\n" http://127.0.0.1:8090/health
```

### 升级（改代码后重新部署）

```bash
bash scripts/ops/deploy.sh             # 重建 + 同步（不动 runtime 数据）
bash scripts/ops/services.sh restart   # 重启 serve 加载新二进制
```

### 校验仓库内容（可选）

```bash
scripts/qlib_eval/.venv/bin/python - <<'PY'
import sqlite3
c = sqlite3.connect("/Users/zuowei/workspace/runtime/atlas/data/qlib_warehouse.db")
print("ohlcv rows     :", c.execute("SELECT COUNT(*) FROM ohlcv").fetchone()[0])
print("symbols        :", c.execute("SELECT COUNT(*) FROM warehouse_meta").fetchone()[0])
print("fundamentals   :", c.execute("SELECT COUNT(*) FROM fundamentals_pit").fetchone()[0])
print("latest last_date:", c.execute("SELECT MAX(last_date) FROM warehouse_meta").fetchone()[0])
PY
```

## 4. atlas 配置

`config.example.yaml` 已含 qlib 段（默认 `enabled: false`，缺省即关闭、行为等同今天）。
要启用本地仓库，在 `configs/config.yaml` 顶层设：

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

## 5. 日常工作流（两个分时段定时任务）

atlas serve **常驻不重启**；备料由两个 LaunchAgent 按本地时间（**+0800**，launchd 用系统时区）
分时段触发，原子 rename 覆盖库文件，atlas 下个监控周期自然只读到新库（SQLite 只读重连，
无需重启 atlas）。

| LaunchAgent | 时刻（本地 +0800） | 动作 | 为何这个点 |
|---|---|---|---|
| `com.newthinker.atlas.refresh-us` | **每天 08:00** | `refresh-market.sh us` | 美股 16:00 ET 收盘 ≈ 次日 04:00–05:00 +0800，08:00 在收盘且数据可用后 |
| `com.newthinker.atlas.refresh-cnhk` | **每天 20:00** | `refresh-market.sh cnhk` | A 股 15:00 / 港股 16:00 收盘后 |

每个任务的流水线（`refresh-market.sh`，仓库根目录自脚本位置推导，免硬编码）：

1. 刷新该市场组 OHLCV CSV：US → `make -o build qlib-data-us`；CNHK → `make -o build qlib-data` + `qlib-data-hk`
2. `make warehouse-dump-all` — 从 **US/CN/HK 三个 CSV 目录全量重建**单一仓库（缺目录自动跳过、原子写）
3. 健康校验 — 库非空且 `last_date` 可解析，否则非零退出
4. 校验通过 → `cp` 一份 `.bak`（回滚永远指向「上一份验证通过」的库，见 §9）

> **关键设计**：重建总是吃全部市场的 CSV，所以两个任务谁先谁后都不会互相覆盖——
> 早 8 点刷美股后重建（含昨晚的 A/港 CSV），晚 8 点刷 A/港后重建（含早上的美股 CSV），
> 仓库始终包含三市场最新可得数据。

手动各跑一次（不等定时点）：

```bash
bash scripts/ops/services.sh refresh-us      # 或直接 bash scripts/ops/refresh-market.sh us
bash scripts/ops/services.sh refresh-cnhk
# 末行期望：ok: N ohlcv rows, M markets, K fundamentals, last_date=YYYY-MM-DD (age Nd)
```

> 调度由 `deploy/launchd/com.newthinker.atlas.refresh-*.plist` 的 `StartCalendarInterval` 定义
> （每天 `Hour`/`Minute`，无 `Weekday` = 每天触发）。改时刻：编辑 plist → `deploy.sh` → `install-services.sh` 重载。
> 任务失败（`set -e` + 校验非零退出）会在 `logs/refresh-*.err.log` 留痕，但
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
| 启动无 `qlib warehouse collector registered` | `enabled` 是否 true、`db_path` 是否存在可读 | 修配置或先 `services.sh refresh-us` 生成库 |
| 健康校验 `WAREHOUSE EMPTY` | 刷新上游 export-ohlcv 是否失败 | 查 `logs/refresh-*.err.log`，确认外部行情可达 |
| 某市场数据没更新 | 对应任务是否触发/失败 | `services.sh refresh-logs us`（或 `cnhk`）看日志，必要时手动 `services.sh refresh-us` |
| PE 分位仍走 yahoo（非 PIT） | `fundamentals_pit` 是否有该符号 | 产出 `fundamentals_csv_us/` 后重跑刷新（best-effort，见 ADAPTERS.md） |
| atlas 读到半成品库 | 不应发生（原子 rename）；若见 `.tmp` 残留 | 删 `data/qlib_warehouse.db.tmp`，重跑刷新 |
| 数据回归/异常 | —— | 见第 9 节回滚 |

日志过滤：

```bash
RT=/Users/zuowei/workspace/runtime/atlas
grep -iE 'qlib|warehouse|tail-fill|stale' "$RT/logs/atlas.err.log"   # atlas serve 侧
tail -f "$RT/logs/refresh-us.out.log"                                # 美股刷新
tail -f "$RT/logs/refresh-cnhk.out.log"                              # A 股/港股刷新
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

> 备份是自动的：`refresh-market.sh` 在健康校验通过后已 `cp` 一份 `.bak`（§5 第 4 步），
> 故 `data/qlib_warehouse.db.bak` 始终指向「上一份验证通过」的库。

## 10. 边界与已知约束

- **频率锚点**：仓库仅日线；实时告警路径永远走外部 API，qlib 不介入。
- **美股 PIT 为近似**：`observe_date` 用披露滞后近似（优于现状的报告期末对齐，但非精确
  备案日），见 `scripts/qlib_warehouse/ADAPTERS.md`。
- **A股/港股基本面**：best-effort，缺失时该符号 PE 分位自动回落 yahoo/lixinger。
- **运维成本**：每日 dump 是真实负担；但全链路可降级，失败不阻断线上。
```
