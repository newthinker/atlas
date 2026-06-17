#!/usr/bin/env bash
#
# install-services.sh — 安装并加载 atlas 的用户级 LaunchAgent
#   - com.newthinker.atlas.serve         常驻服务
#   - com.newthinker.atlas.refresh-us    每天 08:00 刷新美股 + 重建仓库
#   - com.newthinker.atlas.refresh-cnhk  每天 20:00 刷新 A 股/港股 + 重建仓库
#
# plist 真相源在 deploy/launchd/（路径指向 runtime 目录）。幂等：已加载会先 bootout 再 bootstrap。
# 无需 sudo（用户级 LaunchAgent）。运维手册：docs/ops/qlib-warehouse-runbook.md
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEV_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
LA="$HOME/Library/LaunchAgents"
UID_NUM="$(id -u)"

# 清理已废弃的单一每夜任务（如存在），避免与新的分市场任务并存。
launchctl bootout "gui/$UID_NUM/com.newthinker.atlas.warehouse-dump" 2>/dev/null || true
rm -f "$LA/com.newthinker.atlas.warehouse-dump.plist"

mkdir -p "$LA"
for L in com.newthinker.atlas.serve com.newthinker.atlas.refresh-us com.newthinker.atlas.refresh-cnhk; do
  src="$DEV_ROOT/deploy/launchd/$L.plist"
  [ -f "$src" ] || { echo "[install] 缺少 plist: $src" >&2; exit 1; }
  plutil -lint "$src" >/dev/null
  cp -f "$src" "$LA/$L.plist"

  # 卸载旧实例后，必须等其完全 teardown 再 bootstrap，否则 launchd 报
  # "Bootstrap failed: 5: Input/output error"（bootout 与 bootstrap 的竞态）。
  launchctl bootout "gui/$UID_NUM/$L" 2>/dev/null || true
  for _ in $(seq 1 30); do
    launchctl print "gui/$UID_NUM/$L" >/dev/null 2>&1 || break
    sleep 0.3
  done

  # bootstrap 偶发 EIO 时短暂重试。
  for attempt in 1 2 3; do
    if launchctl bootstrap "gui/$UID_NUM" "$LA/$L.plist" 2>/dev/null; then
      echo "[install] loaded $L"
      break
    fi
    [ "$attempt" = 3 ] && { echo "[install] bootstrap 失败: $L" >&2; exit 1; }
    sleep 1
  done
done

echo "[install] 当前已加载的 atlas 服务："
launchctl list | grep atlas || echo "  (none)"
