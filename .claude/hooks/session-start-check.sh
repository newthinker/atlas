#!/bin/bash
# session-start-check.sh — SessionStart 钩子:会话起始报告「模板 ↔ 运行时」漂移(TASK-009)。
#
# **它是报告器,不是门禁**:恒 exit 0。理由是 SessionStart 拦不住任何具体的错误动作 ——
# 真正该阻断的点在写通道(见 arcforge-write.sh 的 leader 派发路径刹车)。在会话起始处阻断
# 只会让人打不开会话去修,而修它恰恰需要一个能用的会话。
#
# 与 check-runtime-sync.sh 的分工:比对逻辑**只在那里**,本脚本不复述(防两处漂移)。
# 它的退出码约定:0 零漂移 / 1 有漂移 / 2 无从进行 / 3 不适用。
set -uo pipefail

SELF_DIR="$(cd "$(dirname "$0")" 2>/dev/null && pwd)" || SELF_DIR="."
SYNC=""
for cand in "$SELF_DIR/../scripts/check-runtime-sync.sh" \
            "./project-template/scripts/check-runtime-sync.sh" \
            "./.claude/scripts/check-runtime-sync.sh"; do
    [ -f "$cand" ] && { SYNC="$cand"; break; }
done

if [ -z "$SYNC" ]; then
    # 缺件 fail-open + **出声**:与写通道对 dev_done 门禁缺失的处置同档
    # (封口层缺件不阻断主功能),但不静默 —— 本 sprint 口径是「放行可以,静默不行」。
    echo "WARN: 未找到 check-runtime-sync.sh,本次会话跳过模板↔运行时漂移检查。" >&2
    exit 0
fi

OUT=$(bash "$SYNC" 2>&1); RC=$?
case "$RC" in
    0) : ;;                                   # 零漂移:安静。会话起始的噪音要留给真信号。
    1) echo "⚠ 模板 ↔ 运行时存在漂移(会话起始检查,不阻断):" >&2
       printf '%s\n' "$OUT" | sed 's/^/    /' >&2
       echo "    → 同步前请先确认改动方向:改 project-template/ 是交付物,改 .claude/ 是运行时副本。" >&2
       ;;
    3) echo "INFO: 本仓库不适用模板↔运行时同步检查(非 dogfooding 布局),跳过。" >&2 ;;
    *) echo "WARN: 漂移检查无从进行(check-runtime-sync.sh exit=$RC):" >&2
       printf '%s\n' "$OUT" | sed 's/^/    /' >&2
       ;;
esac
exit 0
