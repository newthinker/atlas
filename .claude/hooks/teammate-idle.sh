#!/bin/bash
# TeammateIdle hook — 仅当存在「与本实例相关」的非终态任务时保活(F6)
# exit 0 = 允许空闲, exit 2 = 保活并把 stderr 反馈注入 teammate
#
# ⚠️ exit 2 不分配任务;任务分配仍由 Leader 写 tasks/*.json 完成。
#    按实例过滤避免 QA 阶段 Dev Agent 被全局开放任务反复唤醒空转烧 token。
set -uo pipefail

TASK_DIR=".arcforge/tasks"
[ -d "$TASK_DIR" ] || exit 0

HOOK_INPUT=$(cat)
ME=$(echo "$HOOK_INPUT" | jq -r '.teammate_name // .teammate.name // empty' 2>/dev/null)

# query_tasks <jq-filter> [--arg name value ...]:对每个任务文件套用过滤器,输出命中任务的 .id
query_tasks() {
    local filter="$1"; shift
    find "$TASK_DIR" -name "*.json" -exec jq -r "$@" "$filter" {} \; 2>/dev/null
}

if [ -z "$ME" ]; then
    # 拿不到实例名 → 退化为保守保活(原行为),但打印警告便于排查
    OPEN=$(query_tasks 'select(.status | IN("pending","assigned","in_progress","dev_done","verifying","rejected","review_fix","blocked_clarification")) | .id')
    [ -n "$OPEN" ] && { echo "WARN: teammate 名未知,按全局任务保活。请核实 hook stdin 字段。" >&2; exit 2; }
    exit 0
fi

case "$ME" in
  dev-*)
    MINE=$(query_tasks 'select(.assigned_to == $me and (.status | IN("assigned","in_progress","review_fix"))) | .id' --arg me "$ME") ;;
  test-*)
    MINE=$(query_tasks 'select((.verifier // "") == $me and .status == "verifying") | .id' --arg me "$ME")
    # dev_done 待领验证的任务对 Test Agent 也算相关
    PENDING_VERIFY=$(query_tasks 'select(.status == "dev_done") | .id')
    MINE=$(printf '%s\n%s' "$MINE" "$PENDING_VERIFY") ;;
  qa-*)
    # QA 仅在「终审就绪」时保活:全部任务 verified/accepted/skipped 且至少一个 verified。
    # 存在任何在途任务(assigned/in_progress/review_fix/...)时 QA 等待回流,放行 idle,
    # 否则 verified 任务常驻会让 QA 无限唤醒空转(2026-06-10 实测 8 次/40s)。
    IN_FLIGHT=$(query_tasks 'select(.status | IN("pending","assigned","in_progress","dev_done","verifying","rejected","blocked_clarification","review_fix")) | .id')
    if [ -z "$(echo "$IN_FLIGHT" | tr -d '[:space:]')" ]; then
        MINE=$(query_tasks 'select(.status == "verified") | .id')
    else
        MINE=""
    fi ;;
  *)
    MINE="" ;;
esac

if [ -n "$(echo "$MINE" | tr -d '[:space:]')" ]; then
    echo "你($ME)仍有相关任务未走完:重读 .arcforge/tasks/ 按状态机继续(文件是真相源)。" >&2
    exit 2
fi
exit 0
