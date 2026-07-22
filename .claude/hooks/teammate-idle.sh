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

# --- Sprint E 2a 观测取证:完整 hook 输入落盘,用于确认子代理触发事件是否有可区分字段 ---
# 只观测不过滤(硬过滤等取证一个 sprint 后再定);追加失败静默——观测是 best-effort,
# 不得影响保活主逻辑。hook 由 harness 执行,不经 PreToolUse write-guard,可直写 coverage/。
# 修订④:HOOK_INPUT 是多行 JSON,必须 jq -c 压单行再落盘;jq 失败(非法 JSON)则整条
# base64 作 raw_b64 落行,保证 jsonl 行语义永不被破坏(ParseTransitions 按行解析的教训)。
{
    TS=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    mkdir -p .arcforge/coverage
    if LINE=$(printf '%s' "$HOOK_INPUT" | jq -c . 2>/dev/null) && [ -n "$LINE" ]; then
        printf '{"at":"%s","ppid":%d,"input":%s}\n' "$TS" "$PPID" "$LINE"
    else
        B64=$(printf '%s' "$HOOK_INPUT" | base64 | tr -d '\n')
        printf '{"at":"%s","ppid":%d,"raw_b64":"%s"}\n' "$TS" "$PPID" "$B64"
    fi >> .arcforge/coverage/idle-hook-debug.jsonl
} 2>/dev/null || true

ME=$(echo "$HOOK_INPUT" | jq -r '.teammate_name // .teammate.name // empty' 2>/dev/null)

# query_tasks <jq-filter> [--arg name value ...]:对每个任务文件套用过滤器,输出命中任务的 .id
query_tasks() {
    local filter="$1"; shift
    find "$TASK_DIR" -name "*.json" -exec jq -r "$@" "$filter" {} \; 2>/dev/null
}

if [ -z "$ME" ]; then
    # 拿不到实例名 → 放行 idle(ISSUE-4 教训:hook 兜底分支也必须遵守单写者模型,
    # 「保守」的方向是不动文件/不催促推进——无名调用方多为子代理,催促会诱发越权)
    echo "WARN: teammate 名未知,放行 idle。请核实 hook stdin 字段。" >&2
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
    MINE=$(query_tasks 'select(.status == "verified") | .id')
    # F6 防空转:最新裁决产物晚于全部 verified 任务文件 = 本轮已出过裁决,允许空闲
    if [ -n "$(echo "$MINE" | tr -d '[:space:]')" ]; then
        LATEST_REVIEW=$(ls -t .arcforge/docs/05-review/*.md 2>/dev/null | head -1)
        if [ -n "$LATEST_REVIEW" ]; then
            NEWER=$(find "$TASK_DIR" -name '*.json' -newer "$LATEST_REVIEW" \
                    -exec jq -r 'select(.status == "verified") | .id' {} \; 2>/dev/null)
            [ -z "$(echo "$NEWER" | tr -d '[:space:]')" ] && MINE=""
        fi
    fi ;;
  *)
    MINE="" ;;
esac

if [ -n "$(echo "$MINE" | tr -d '[:space:]')" ]; then
    echo "你($ME)仍有相关任务未走完:重读 .arcforge/tasks/,按你实例的角色职责继续(文件是真相源;你只能写你 own 的状态,写入一律经 .claude/hooks/arcforge-write.sh --as $ME)。" >&2
    exit 2
fi
exit 0
