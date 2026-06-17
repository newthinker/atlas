#!/usr/bin/env bash
#
# refresh-market.sh <us|cnhk> — 刷新指定市场组的行情 CSV，然后全量重建本地 SQLite 仓库。
#
#   us    刷新美股 OHLCV（make qlib-data-us）
#   cnhk  刷新 A 股 + 港股 OHLCV（make qlib-data + qlib-data-hk）
#
# 重建用 `warehouse-dump-all`：从 US/CN/HK 三个 CSV 目录原子重建单一仓库，缺目录自动跳过。
# 因此两个分时段任务（美股早 8 点 / A 股·港股晚 8 点）各自刷新本市场后，仓库始终含全部市场。
#
#   runtime 不含 Go 源码：用 `-o build` 让 make 跳过 go build，复用已部署的 ./bin/atlas。
#
# 由 launchd 调度（见 deploy/launchd/com.newthinker.atlas.refresh-*.plist）；
# 也可手动：bash scripts/ops/refresh-market.sh us
# 运维手册：docs/ops/qlib-warehouse-runbook.md
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${REPO_ROOT}"

PY="${QLIB_PY:-scripts/qlib_eval/.venv/bin/python}"
WAREHOUSE_DB="${WAREHOUSE_DB:-data/qlib_warehouse.db}"
GROUP="${1:?usage: refresh-market.sh <us|cnhk>}"

ts() { date '+%Y-%m-%dT%H:%M:%S%z'; }
log() { echo "[$(ts)] [$GROUP] $*"; }

log "refresh-market start (repo=${REPO_ROOT})"

# 外部源（Yahoo/eastmoney）偶发 403/限流是常态；对 export 步骤重试，吸收瞬时失败。
retry() {
  local n=0 max=3
  until "$@"; do
    n=$((n + 1))
    [ "$n" -ge "$max" ] && return 1
    log "  「$*」失败（第 $n/$max 次），$((n * 20))s 后重试 ..."
    sleep $((n * 20))
  done
}

# [1] 刷新该市场组的 OHLCV CSV（-o build：复用已部署二进制，不重新编译）。
#     export 最终失败时不中止任务——既有 CSV 仍在，step 2 会用它们重建，
#     宁可数据不更新，也不让仓库停摆（贴合 atlas 可降级哲学）。
export_ok=1
case "$GROUP" in
  us)
    log "step 1: make -o build qlib-data-us"
    retry make -o build qlib-data-us || export_ok=0
    ;;
  cnhk)
    log "step 1a: make -o build qlib-data (A 股)"
    retry make -o build qlib-data || export_ok=0
    log "step 1b: make -o build qlib-data-hk (港股)"
    retry make -o build qlib-data-hk || export_ok=0
    ;;
  *)
    echo "unknown group: $GROUP (use: us | cnhk)" >&2
    exit 2
    ;;
esac
[ "$export_ok" = 1 ] || log "WARN: OHLCV 刷新未完全成功（外部源限流？），改用现存 CSV 重建"

# [2] 全量重建仓库：吃 US/CN/HK 三个 CSV 目录（缺目录跳过），原子写。
log "step 2: make warehouse-dump-all"
make warehouse-dump-all

# [3] 健康校验：库非空且 last_date 可解析。
log "step 3: health check"
"${PY}" - "${WAREHOUSE_DB}" <<'PY'
import sqlite3, sys, datetime
db = sys.argv[1]
c = sqlite3.connect(db)
n = c.execute("SELECT COUNT(*) FROM ohlcv").fetchone()[0]
last = c.execute("SELECT MAX(last_date) FROM warehouse_meta").fetchone()[0]
mkts = c.execute("SELECT COUNT(DISTINCT market) FROM warehouse_meta").fetchone()[0]
funds = c.execute("SELECT COUNT(*) FROM fundamentals_pit").fetchone()[0]
if n == 0 or last is None:
    print("WAREHOUSE EMPTY", file=sys.stderr)
    sys.exit(1)
age = (datetime.date.today() - datetime.date.fromisoformat(last)).days
print(f"ok: {n} ohlcv rows, {mkts} markets, {funds} fundamentals, last_date={last} (age {age}d)")
PY

# [4] 校验通过 → 备份「上一份验证通过」的库（回滚用，见 runbook §9）。
log "step 4: backup verified DB -> ${WAREHOUSE_DB}.bak"
cp -f "${WAREHOUSE_DB}" "${WAREHOUSE_DB}.bak"

log "refresh-market done"
