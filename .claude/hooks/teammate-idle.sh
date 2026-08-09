#!/bin/bash
# TeammateIdle hook — 仅当存在「与本实例相关」的非终态任务时保活(F6)
# exit 0 = 允许空闲, exit 2 = 保活并把 stderr 反馈注入 teammate
#
# ⚠️ exit 2 不分配任务;任务分配仍由 Leader 写 tasks/*.json 完成。
#    按实例过滤避免 QA 阶段 Dev Agent 被全局开放任务反复唤醒空转烧 token。
#
# ⚠️ exit 0 是**不可逆停机点**,不是「这一轮先歇着」:TeammateIdle 是 teammate 转 idle 前的
#    一次性拦截点,不是周期性心跳。放行后该实例停机,在收到新消息前本 hook 不会再被调用。
#    实测取证(atlas sprint-028,idle-hook-debug.jsonl):test-agent-6 于 2026-07-25T11:49:01Z
#    因当时无 verifying/dev_done 被放行,此后 89 分钟零调用;期间 12:45:00Z 派下的 TASK-016
#    验证任务一直无人认领,直到 Leader 直接发消息才唤醒。
#    ⇒ 两条推论,改本 hook 之前先读:
#      1. 派发可靠性**不能**指望本 hook 兜底。已停机 agent 的自愈只能靠 Leader 侧活性确认
#         + 消息唤醒(活性判据应锚定本轮派发的产物,而非 agent 的全局活动痕迹)。
#      2. 不要用「宁可保活」来补偿 —— 那会让无关 agent 空转烧 token,正是上面 F6 要防的。
#         放行条件本身是对的,问题不在这里。
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

# UNLOCK 是**本角色的解锁条件**——「做什么这个 hook 才会放你走」。
#
# 为什么每个分支都要单独写:保活文案原先只有通用的一句「按你实例的角色职责继续」。
# 判定是对的,但读者拿不到可行动信息。atlas sprint-033 实测:qa-agent-10 被连续唤醒
# 约 1500 次(01–07 时每小时 190–321 次),每次都重扫、每次都确认"自己有相关任务",
# 而它是被派为只读 lens 的实例——**结构上产不出解锁条件**,于是无限空转。
#
# 那次的判定完全正确(有 verified 任务且无裁决 ⇒ 该保活),错的是输出没说出
# 「写一份 docs/05-review/*.md 就能停」。**判定正确而输出不可行动,等于把诊断留给读者**,
# 而读者恰恰是那个信息最少的实例。
case "$ME" in
  dev-*)
    MINE=$(query_tasks 'select(.assigned_to == $me and (.status | IN("assigned","in_progress","review_fix"))) | .id' --arg me "$ME")
    UNLOCK="把名下任务推进到 dev_done(经写通道 transition dev_done;门禁不过则先修测试/覆盖率)" ;;
  test-*)
    MINE=$(query_tasks 'select((.verifier // "") == $me and .status == "verifying") | .id' --arg me "$ME")
    # dev_done 待领验证的任务对 Test Agent 也算相关
    PENDING_VERIFY=$(query_tasks 'select(.status == "dev_done") | .id')
    MINE=$(printf '%s\n%s' "$MINE" "$PENDING_VERIFY")
    UNLOCK="把 verifier 指向你的 verifying 任务判成 verified 或 rejected;dev_done 任务需等 Leader 派验(dev_done->verifying 是 leader 专属边,你无法自领)" ;;
  qa-*)
    MINE=$(query_tasks 'select(.status == "verified") | .id')
    UNLOCK="在 .arcforge/docs/05-review/ 下落一份裁决产物(*.md,须晚于全部 verified 任务文件的 mtime);若你是只读 lens 子代理则产不出它,把结论交回父实例由其落盘"
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
    MINE=""; UNLOCK="" ;;
esac

if [ -n "$(echo "$MINE" | tr -d '[:space:]')" ]; then
    echo "你($ME)仍有相关任务未走完:重读 .arcforge/tasks/,按你实例的角色职责继续(文件是真相源;你只能写你 own 的状态,写入一律经 .claude/hooks/arcforge-write.sh --as $ME)。" >&2
    [ -n "${UNLOCK:-}" ] && echo "  解锁条件:${UNLOCK}。" >&2
    exit 2
fi
exit 0
