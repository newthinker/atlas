#!/usr/bin/env bash
#
# deploy.sh — 从代码目录构建 atlas，并把「运行时产物（不含 Go 源码）」部署到 runtime 目录。
#
# 幂等、可重复执行。只覆盖二进制/脚本/配置；**绝不动 runtime 本地数据**
# （data/ logs/ qlib_csv*/ fundamentals_csv*/ signals*.csv reports/、含明文密钥的
# configs/config.yaml，以及 runtime 侧独立安装的 scripts/akshare/.venv/、
# scripts/baostock/.venv/、scripts/qlib_eval/.venv/ 均被排除并受 --delete 保护）。
#
# 🔴 **只能在主仓库执行，不能在 linked worktree 里执行**（脚本开头有判别，会拒绝）。
#    成因：被 gitignore 的运行时内容（config.yaml、各 .venv）只存在于主仓库工作区，
#    worktree 是 checkout 出来的、没有它们，于是 `--delete` 会把 runtime 那份删掉。
#    Sprint 043 QA 背对背干跑实测：worktree 源 30202 条 deleting（其中
#    scripts/qlib_eval/.venv 30177 条、configs/config.yaml 1 条），主仓库源 8 条。
#    排除表补两条只堵住**已知**的两个症状；这道判别堵的是成因——下一个被 gitignore
#    的运行时目录出现时，排除表不会保护它。详见 internal/hestia/CONTRACTS.md §A7。
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

# 0. 拒绝在 linked worktree 里执行（判别式逐字照抄 .claude/hooks/arcforge-write.sh
#    的 assert_main_worktree —— 那份有测试覆盖）。
#
#    ⚠️ 与写通道**刻意相反**：写通道判别不出时 fail-open（误拒会阻断唯一写入通道），
#    这里判别不出时 **fail-closed**（误动作要删掉三万个文件且不可逆，而误拒的代价
#    只是人手动 cd 一次）。不提供环境变量绕过——从 worktree 部署没有正当用例。
assert_main_worktree() {
  local gd gcd main
  # 两者一个可能相对、一个可能绝对（git 的输出形态随 cwd 变），且 macOS 的 /var 是
  # 符号链接，故统一成物理绝对路径再比，避免把主工作区误判成 worktree。
  if command -v git >/dev/null 2>&1 \
     && gd=$(git rev-parse --git-dir 2>/dev/null) \
     && gcd=$(git rev-parse --git-common-dir 2>/dev/null) \
     && [ -n "$gd" ] && [ -n "$gcd" ] \
     && gd=$(cd "$gd" 2>/dev/null && pwd -P) \
     && gcd=$(cd "$gcd" 2>/dev/null && pwd -P); then
    [ "$gd" = "$gcd" ] && return 0
    main=$(dirname "$gcd")
    # ⚠️ 变量一律写 ${var}：紧跟 CJK 的 $var 会被 macOS bash 3.2 把多字节字节
    #    并进变量名（实测 "$gcd）" → `gcd\xef\xbc\x89: unbound variable`）。
    echo "[deploy] 拒绝执行：当前在 linked worktree（--git-dir=${gd} ≠ --git-common-dir=${gcd}）。" >&2
    echo "[deploy] worktree 里没有被 gitignore 的运行时内容（configs/config.yaml、各 .venv），" >&2
    echo "[deploy] rsync --delete 会把 runtime 那份删掉（实测一次 30202 项）。" >&2
    echo "[deploy] 请 cd ${main} 回主仓库再执行。" >&2
    exit 2
  fi
  # 主判别式无法决断（非 git 仓库 / git 不在 PATH / rev-parse 报错）——走兜底探针：
  # linked worktree 的 .git 是「gitdir:」形态的**文件**而非目录。
  # （该探针对 git 子模块同样命中，是已知假阳；此处 fail-closed，宁可误拒。）
  if [ -f .git ] && head -c 7 .git 2>/dev/null | grep -q '^gitdir:'; then
    echo "[deploy] 拒绝执行：git 判别式不可用，但 .git 是「gitdir:」形态的文件——" >&2
    echo "[deploy] 极可能是 linked worktree（或 git 子模块）。请在主仓库根执行。" >&2
    exit 2
  fi
  return 0
}
assert_main_worktree

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
  --exclude='/configs/config.yaml' \
  --exclude='/scripts/akshare/.venv/' \
  --exclude='/scripts/baostock/.venv/' \
  --exclude='/scripts/qlib_eval/.venv/' \
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
