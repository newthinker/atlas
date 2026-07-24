#!/usr/bin/env bash
# scripts/akshare/setup.sh — 幂等创建 akshare 独立 venv 并安装依赖。
# 与 qlib_eval 的 venv 完全隔离(两者依赖树庞大且各自随上游更新)。
# 用法: bash scripts/akshare/setup.sh [python3 解释器,默认 python3.11]
set -euo pipefail
cd "$(dirname "$0")"
PY="${1:-python3.11}"
command -v "$PY" >/dev/null || PY=python3
[ -d .venv ] || "$PY" -m venv .venv
./.venv/bin/pip install --upgrade pip
./.venv/bin/pip install -r requirements.txt
# 版本冻结留档(升级时对比回滚依据)
./.venv/bin/pip freeze > requirements.lock
echo "OK: $(./.venv/bin/python -c 'import akshare; print("akshare", akshare.__version__)')"
echo "启动测试: ./.venv/bin/python -m aktools --help"
