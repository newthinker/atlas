#!/usr/bin/env bash
# scripts/akshare/setup.sh — 幂等创建 akshare 独立 venv 并安装依赖。
# 与 qlib_eval 的 venv 完全隔离(两者依赖树庞大且各自随上游更新)。
#
# 用法:
#   bash scripts/akshare/setup.sh [python3 解释器]
#       默认解释器 python3.11。存在 requirements.lock 时按 lock 复现安装(锁定版本、
#       不改 lock);无 lock(首次)时按 requirements.txt 安装并 freeze 生成 lock。
#   bash scripts/akshare/setup.sh --upgrade [python3 解释器]
#       显式升级:忽略 lock,按 requirements.txt 重装到最新并刷新 lock(提交更新的 lock)。
set -euo pipefail
cd "$(dirname "$0")"

UPGRADE=0
PY=""
for arg in "$@"; do
	case "$arg" in
		--upgrade) UPGRADE=1 ;;
		*) PY="$arg" ;;
	esac
done
PY="${PY:-python3.11}"
command -v "$PY" >/dev/null || PY=python3

[ -d .venv ] || "$PY" -m venv .venv
./.venv/bin/pip install --upgrade pip

if [ "$UPGRADE" -eq 0 ] && [ -f requirements.lock ]; then
	# 复现安装:按锁定版本装,不覆写 lock(升级对比的基准不被顺手改动)
	./.venv/bin/pip install -r requirements.lock
else
	# 首次或 --upgrade:按 requirements.txt 装最新并刷新 lock 留档(升级/回滚依据)
	./.venv/bin/pip install -r requirements.txt
	./.venv/bin/pip freeze > requirements.lock
fi

echo "OK: $(./.venv/bin/python -c 'import akshare; print("akshare", akshare.__version__)')"
echo "启动测试: ./.venv/bin/python -m aktools --help"
