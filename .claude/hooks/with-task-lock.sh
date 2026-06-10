#!/bin/bash
# 用法: with-task-lock.sh <TASK-ID> <command...>
# 在任务级互斥锁内执行命令——认领协议的「读-校验-写」临界区(F5/R1)。
# 优先 flock(Linux);无 flock 的环境(macOS)用 mkdir 自旋锁(mkdir 原子)。
set -uo pipefail
TASK_ID="${1:?用法: with-task-lock.sh <TASK-ID> <command...>}"; shift
LOCK_BASE=".arcforge/tasks/.${TASK_ID}.lock"

if command -v flock >/dev/null 2>&1; then
    exec flock -w 30 "${LOCK_BASE}file" "$@"
fi

for _ in $(seq 1 300); do
    if mkdir "$LOCK_BASE" 2>/dev/null; then
        trap 'rmdir "$LOCK_BASE" 2>/dev/null' EXIT
        "$@"
        exit $?
    fi
    sleep 0.1
done
echo "ERROR: 获取 $TASK_ID 锁超时(疑似死锁残留: $LOCK_BASE)" >&2
echo "       确认无活动 agent 进程后手动清理: rmdir $LOCK_BASE" >&2
exit 1
