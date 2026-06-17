#!/usr/bin/env bash
#
# services.sh — atlas 服务日常管理封装。
#
# 用法： bash scripts/ops/services.sh <command>
#   status            查看所有服务状态 / serve pid
#   restart           重启 serve（改配置或部署新二进制后）
#   stop              停止 serve
#   start             启动 serve
#   refresh-us        立即触发一次美股刷新+重建（不等 08:00）
#   refresh-cnhk      立即触发一次 A 股/港股刷新+重建（不等 20:00）
#   analysis-now      立即触发一轮分析（产信号 → 通知，不等 30min）
#   logs              跟随 serve 日志（stderr）
#   refresh-logs <us|cnhk>   跟随对应刷新任务日志
#   analysis-logs     跟随定时分析触发日志
#   uninstall         卸载全部服务（不删 runtime 目录）
#
# 运维手册：docs/ops/qlib-warehouse-runbook.md
set -euo pipefail

UID_NUM="$(id -u)"
ATLAS_RUNTIME="${ATLAS_RUNTIME:-/Users/zuowei/workspace/runtime/atlas}"
LA="$HOME/Library/LaunchAgents"
SERVE=com.newthinker.atlas.serve
REFRESH_US=com.newthinker.atlas.refresh-us
REFRESH_CNHK=com.newthinker.atlas.refresh-cnhk
ANALYSIS=com.newthinker.atlas.analysis

case "${1:-status}" in
  status)
    launchctl list | grep atlas || echo "(no atlas services loaded)"
    launchctl print "gui/$UID_NUM/$SERVE" 2>/dev/null | grep -E 'state =|pid =' || true
    ;;
  restart)  launchctl kickstart -k "gui/$UID_NUM/$SERVE"; echo "restarted $SERVE" ;;
  stop)     launchctl bootout "gui/$UID_NUM/$SERVE" 2>/dev/null || true; echo "stopped $SERVE" ;;
  start)    launchctl bootstrap "gui/$UID_NUM" "$LA/$SERVE.plist"; echo "started $SERVE" ;;
  refresh-us)   launchctl kickstart "gui/$UID_NUM/$REFRESH_US";   echo "triggered $REFRESH_US" ;;
  refresh-cnhk) launchctl kickstart "gui/$UID_NUM/$REFRESH_CNHK"; echo "triggered $REFRESH_CNHK" ;;
  analysis-now) launchctl kickstart "gui/$UID_NUM/$ANALYSIS";     echo "triggered $ANALYSIS" ;;
  logs)       tail -n 50 -f "$ATLAS_RUNTIME/logs/atlas.err.log" ;;
  refresh-logs)
    case "${2:-}" in
      us)   tail -n 50 -f "$ATLAS_RUNTIME/logs/refresh-us.out.log" ;;
      cnhk) tail -n 50 -f "$ATLAS_RUNTIME/logs/refresh-cnhk.out.log" ;;
      *) echo "usage: services.sh refresh-logs <us|cnhk>" >&2; exit 2 ;;
    esac
    ;;
  analysis-logs) tail -n 50 -f "$ATLAS_RUNTIME/logs/analysis.out.log" ;;
  uninstall)
    for L in "$SERVE" "$REFRESH_US" "$REFRESH_CNHK" "$ANALYSIS"; do
      launchctl bootout "gui/$UID_NUM/$L" 2>/dev/null || true
      rm -f "$LA/$L.plist"
    done
    echo "uninstalled atlas services (runtime dir untouched)"
    ;;
  *)
    echo "usage: services.sh {status|restart|stop|start|refresh-us|refresh-cnhk|analysis-now|logs|refresh-logs <us|cnhk>|analysis-logs|uninstall}" >&2
    exit 2
    ;;
esac
