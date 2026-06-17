#!/usr/bin/env bash
#
# deploy.sh — 从代码目录构建 atlas，并把「运行时产物（不含 Go 源码）」部署到 runtime 目录。
#
# 幂等、可重复执行。只覆盖二进制/脚本/配置；**绝不动 runtime 本地数据**
# （data/ logs/ qlib_csv*/ fundamentals_csv*/ signals*.csv reports/ 均被排除并受 --delete 保护）。
#
# 用法：
#   bash scripts/ops/deploy.sh                 # 部署到默认 runtime
#   ATLAS_RUNTIME=/path/to/runtime bash scripts/ops/deploy.sh   # 覆盖目标
#
# 运维手册：docs/ops/qlib-warehouse-runbook.md
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEV_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
ATLAS_RUNTIME="${ATLAS_RUNTIME:-/Users/zuowei/workspace/runtime/atlas}"
cd "$DEV_ROOT"

echo "[deploy] dev=$DEV_ROOT"
echo "[deploy] runtime=$ATLAS_RUNTIME"

# 1. 构建二进制（运行时不含源码，故在代码目录构建后投递产物）。
echo "[deploy] building bin/atlas ..."
make build

# 2. 同步运行时产物。
#    剥离 Go 源码用全局排除 `*.go`（而非排除 internal/ 整目录）——因为 serve 运行时
#    需要 internal/api/templates/ 下的 HTML 模板，整目录排除会连模板一起删掉导致启动失败。
#    `-m`（--prune-empty-dirs）清理只剩 .go 而被掏空的目录（如 cmd/、validator/）。
#    runtime 本地数据（data/ logs/ qlib_csv*/ ...）排除，避免被 --delete 清掉。
mkdir -p "$ATLAS_RUNTIME"
echo "[deploy] syncing runtime artifacts (rsync --delete) ..."
rsync -a -m --delete \
  --exclude='/.git/' --exclude='/.worktrees/' --exclude='/.idea/' --exclude='/.vscode/' \
  --exclude='/.gitignore' --exclude='/.gitnexus/' --exclude='/.github/' --exclude='/.kanban/' \
  --exclude='/.arcforge/' --exclude='/.claude/' --exclude='/.codex/' --exclude='/.agents/' \
  --exclude='/arcforge.config.json' \
  --exclude='*.go' \
  --exclude='/go.mod' --exclude='/go.sum' --exclude='/cover.out' \
  --exclude='/docs/' --exclude='/AGENTS.md' --exclude='/CLAUDE.md' --exclude='/README.md' \
  --exclude='/scripts/qlib_eval/tests/' --exclude='/scripts/qlib_eval/conftest.py' \
  --exclude='/scripts/qlib_eval/.pytest_cache/' \
  --exclude='/scripts/qlib_warehouse/tests/' --exclude='/scripts/qlib_warehouse/.pytest_cache/' \
  --exclude='__pycache__/' --exclude='*.pyc' --exclude='.DS_Store' \
  --exclude='/data/' --exclude='/logs/' \
  --exclude='/qlib_csv/' --exclude='/qlib_csv_hk/' --exclude='/qlib_csv_us/' \
  --exclude='/fundamentals_csv/' --exclude='/fundamentals_csv_us/' \
  --exclude='/signals*.csv' --exclude='/reports/' \
  "$DEV_ROOT/" "$ATLAS_RUNTIME/"

# 3. 确保运行时目录存在，并收紧含明文密钥的配置权限。
mkdir -p "$ATLAS_RUNTIME/logs" "$ATLAS_RUNTIME/data"
if [ -f "$ATLAS_RUNTIME/configs/config.yaml" ]; then
  chmod 600 "$ATLAS_RUNTIME/configs/config.yaml"
  echo "[deploy] secured configs/config.yaml (600)"
else
  echo "[deploy] WARNING: $ATLAS_RUNTIME/configs/config.yaml 不存在——serve 启动需要它（含密钥，gitignore 不入库）"
fi

echo "[deploy] done. binary -> $ATLAS_RUNTIME/bin/atlas"
echo "[deploy] 重启服务以加载新二进制："
echo "         launchctl kickstart -k gui/\$(id -u)/com.newthinker.atlas.serve"
echo "[deploy] 首次部署还需安装服务： bash scripts/ops/install-services.sh"
