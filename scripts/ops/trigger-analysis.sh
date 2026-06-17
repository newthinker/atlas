#!/usr/bin/env bash
#
# trigger-analysis.sh — 周期性触发 atlas 跑一轮分析（POST /api/v1/analysis/run）。
#
# atlas serve 本身不跑分析循环（App.Start 未接线），分析需外部触发；本脚本由 launchd
# 定时调用，使 atlas 持续产出信号 → router → 通知器（Telegram）。
# router 的 cooldown_hours（默认 4h）天然防止同一信号刷屏，故较密的触发也不会重复轰炸。
#
# 端口/api_key 从 configs/config.yaml 读取（运行目录由 launchd 设为 runtime 根）。
# 运维手册：docs/ops/qlib-warehouse-runbook.md
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${REPO_ROOT}"

PY="${QLIB_PY:-scripts/qlib_eval/.venv/bin/python}"
CFG="${ATLAS_CONFIG:-configs/config.yaml}"

PORT="$("$PY" -c "import yaml;print(yaml.safe_load(open('$CFG'))['server']['port'])")"
KEY="$("$PY" -c "import yaml;print(yaml.safe_load(open('$CFG')).get('server',{}).get('api_key','') or '')")"

ts() { date '+%Y-%m-%dT%H:%M:%S%z'; }

code="$(curl -s -o /dev/null -w '%{http_code}' -X POST \
  -H "X-API-Key: ${KEY}" \
  "http://127.0.0.1:${PORT}/api/v1/analysis/run" || echo 000)"

echo "[$(ts)] analysis trigger -> http=${code}"
[ "${code}" = "200" ] || { echo "[$(ts)] WARN: trigger failed (atlas serve 未运行?)" >&2; exit 1; }
