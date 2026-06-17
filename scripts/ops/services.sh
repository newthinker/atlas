#!/usr/bin/env bash
#
# services.sh — atlas 服务日常管理封装。
#
# 用法： bash scripts/ops/services.sh <command>
#   status      查看两个服务状态 / serve pid
#   restart     重启 serve（改配置或部署新二进制后）
#   stop        停止 serve
#   start       启动 serve
#   dump-now    立即触发一次每夜仓库重建（不等 23:30）
#   logs        跟随 serve 日志（stderr）
#   dump-logs   跟随每夜 dump 日志
#   uninstall   卸载两个服务（不删 runtime 目录）
#
# 运维手册：docs/ops/qlib-warehouse-runbook.md
set -euo pipefail

UID_NUM="$(id -u)"
ATLAS_RUNTIME="${ATLAS_RUNTIME:-/Users/zuowei/workspace/runtime/atlas}"
LA="$HOME/Library/LaunchAgents"
SERVE=com.newthinker.atlas.serve
DUMP=com.newthinker.atlas.warehouse-dump

case "${1:-status}" in
  status)
    launchctl list | grep atlas || echo "(no atlas services loaded)"
    launchctl print "gui/$UID_NUM/$SERVE" 2>/dev/null | grep -E 'state =|pid =' || true
    ;;
  restart)  launchctl kickstart -k "gui/$UID_NUM/$SERVE"; echo "restarted $SERVE" ;;
  stop)     launchctl bootout "gui/$UID_NUM/$SERVE" 2>/dev/null || true; echo "stopped $SERVE" ;;
  start)    launchctl bootstrap "gui/$UID_NUM" "$LA/$SERVE.plist"; echo "started $SERVE" ;;
  dump-now) launchctl kickstart "gui/$UID_NUM/$DUMP"; echo "triggered $DUMP" ;;
  logs)       tail -n 50 -f "$ATLAS_RUNTIME/logs/atlas.err.log" ;;
  dump-logs)  tail -n 50 -f "$ATLAS_RUNTIME/logs/warehouse.out.log" ;;
  uninstall)
    for L in "$SERVE" "$DUMP"; do
      launchctl bootout "gui/$UID_NUM/$L" 2>/dev/null || true
      rm -f "$LA/$L.plist"
    done
    echo "uninstalled atlas services (runtime dir untouched)"
    ;;
  *)
    echo "usage: services.sh {status|restart|stop|start|dump-now|logs|dump-logs|uninstall}" >&2
    exit 2
    ;;
esac
