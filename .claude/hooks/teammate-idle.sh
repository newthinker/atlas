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
    # 拿不到实例名 → 放行 idle。原「按全局任务保活」会把 QA/Leader spawn 的
    # 临时子代理（无实例名）推去“推进任务”，诱发越权写 tasks/*.json
    # （2026-06-12 sprint-004 实测：对抗子代理被此提示误导，擅自 verified→accepted）。
    # 临时子代理的生命周期由 spawn 方负责，不归保活机制管。
    exit 0
fi

case "$ME" in
  dev-*)
    MINE=$(query_tasks 'select(.assigned_to == $me and (.status | IN("assigned","in_progress","review_fix"))) | .id' --arg me "$ME") ;;
  test-*)
    MINE=$(query_tasks 'select((.verifier // "") == $me and .status == "verifying") | .id' --arg me "$ME")
    # dev_done 仅当 verifier 指派给自己才相关。verifier 为空 = Leader 尚未派验，
    # 该窗口属 Leader 职责，不唤醒任何 test 实例——否则 test agent 醒来发现
    # verifier 非自己又 idle，hook 再唤醒，空转循环（2026-06-11 实测两轮）。
    PENDING_VERIFY=$(query_tasks 'select(.status == "dev_done" and (.verifier // "") == $me) | .id' --arg me "$ME")
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
