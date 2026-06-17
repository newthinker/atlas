#!/usr/bin/env bash
#
# nightly-warehouse.sh — 每日盘后重建本地 qlib 数据仓库（方向③）。
#
# 严格顺序：qlib-data-us（刷新 OHLCV CSV）→ warehouse-dump（CSV→SQLite，原子写）
#            → 健康校验 → 校验通过后备份 .bak。
# 任一步失败即非零退出并在日志留痕；不影响正在运行的 atlas（它继续用上一份库 + API 补尾）。
#
# 运维手册：docs/ops/qlib-warehouse-runbook.md
# cron 示例（美股收盘后，按服务器时区调整）：
#   30 23 * * 1-5  /bin/bash <repo>/scripts/ops/nightly-warehouse.sh >> /var/log/atlas/warehouse.log 2>&1
set -euo pipefail

# 从脚本位置推导仓库根目录（scripts/ops/ 上两级），避免硬编码绝对路径。
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${REPO_ROOT}"

PY="${QLIB_PY:-scripts/qlib_eval/.venv/bin/python}"
WAREHOUSE_DB="${WAREHOUSE_DB:-data/qlib_warehouse.db}"

ts() { date '+%Y-%m-%dT%H:%M:%S%z'; }
log() { echo "[$(ts)] $*"; }

log "nightly-warehouse start (repo=${REPO_ROOT})"

# [1] 刷新美股 OHLCV CSV（+ qlib .bin 数据包）。
log "step 1: make qlib-data-us"
make qlib-data-us

# [2] （可选）刷新 PIT 基本面 CSV → fundamentals_csv_us/。
#     接入适配器后取消注释；缺该目录时 warehouse-dump 自动跳过基本面（零破坏）。
#     见 scripts/qlib_warehouse/ADAPTERS.md。
# log "step 2: refresh fundamentals_csv_us"
# <在此调用你的基本面适配器>

# [3] 重建 SQLite 仓库（原子：先写 .tmp 再 rename）。
log "step 3: make warehouse-dump"
make warehouse-dump

# [4] 健康校验：库非空且 last_date 可解析；通过则打印 age。
log "step 4: health check"
"${PY}" - "${WAREHOUSE_DB}" <<'PY'
import sqlite3, sys, datetime
db = sys.argv[1]
c = sqlite3.connect(db)
n = c.execute("SELECT COUNT(*) FROM ohlcv").fetchone()[0]
last = c.execute("SELECT MAX(last_date) FROM warehouse_meta").fetchone()[0]
funds = c.execute("SELECT COUNT(*) FROM fundamentals_pit").fetchone()[0]
if n == 0 or last is None:
    print("WAREHOUSE EMPTY", file=sys.stderr)
    sys.exit(1)
age = (datetime.date.today() - datetime.date.fromisoformat(last)).days
print(f"ok: {n} ohlcv rows, {funds} fundamentals, last_date={last} (age {age}d)")
PY

# [5] 校验通过 → 备份「上一份验证通过」的库，供回滚（见 runbook §9）。
log "step 5: backup verified DB -> ${WAREHOUSE_DB}.bak"
cp -f "${WAREHOUSE_DB}" "${WAREHOUSE_DB}.bak"

log "nightly-warehouse done"
