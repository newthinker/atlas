#!/bin/bash
# TaskCompleted 唯一入口:硬门禁 → Jev 质量闸门。
#
# 为什么是 wrapper 而不是在 task-completed.sh 里调用:
# 那个脚本 690 行、十几个分散的 exit 出口(结尾是 case detect_language,每个分支各自 exit),
# 控制流永远到不了文件末尾。包一层是唯一不碰它控制流的接法。
#
# 为什么不注册成第二个 TaskCompleted hook:
# Claude Code 并行执行同一事件的全部 hook —— 测试没过也会调 Jev,退出码还互相覆盖。
# 顺序只能在一个进程里串起来。
set -uo pipefail

# stdin 只能读一次,先存下来再分发给两个消费者。
INPUT="$(cat)"

printf '%s' "$INPUT" | bash .claude/hooks/task-completed.sh
RC=$?
# 硬门禁的任何一个出口非 0 → **原样**透传,Jev 根本不介入。
# 分层原则(确定性检查永远在前)在这里落地,也保证门禁的 stderr 反馈不被 Jev 的输出盖掉。
[ "$RC" -ne 0 ] && exit "$RC"

JEV_HOOK=".claude/hooks/jev/task_completed_jev.py"
# Jev 尚未安装是正常状态,不出声(否则每次 TaskCompleted 都刷一行噪音)。
[ -f "$JEV_HOOK" ] || exit 0
# 不可读则**不能交给 python3**:python3 打不开脚本文件时退出码恰为 2,与下面契约里
# 「Jev 打回」用的是同一个码,退出码分流无从分辨。一个权限问题会因此被判成质量不合格。
if [ ! -r "$JEV_HOOK" ]; then
    echo "task-completed-jev: $JEV_HOOK not readable; skipping Jev quality gate." >&2
    exit 0
fi

# Jev 跑不起来 ≠ Jev 判定不通过。缺 python3 时直接调用会 exit 127,那是把「没装解释器」
# 当成了「质量不合格」—— 与 fail-open 原则相反,且会阻断一个硬门禁已经放行的 dev_done。
# 故先探 PATH,探不到就有声降级:退化成没有 Jev 的 arcforge。
if ! command -v python3 >/dev/null 2>&1; then
    echo "task-completed-jev: python3 not available; skipping Jev quality gate." >&2
    exit 0
fi

printf '%s' "$INPUT" | python3 "$JEV_HOOK"
JRC=$?
# 退出码契约:0 = Jev 判定通过;2 = Jev **给出了判定**且判定不通过(保持阻断)。
# 其余非零一律是「Jev 没能给出判定」——python3 对 SyntaxError 与模块级抛异常**都返回 1**,
# 那时 hook 根本没加载起来,把这个 1 当成质量结论就是把 fail-open 做成了 fail-closed。
# 不写成「恒 exit 0」:那会连 2 一起放行,打掉 Jev 主动打回的阻断语义。
case "$JRC" in
    0) exit 0 ;;
    2) exit 2 ;;
    *)
        echo "task-completed-jev: Jev hook exited $JRC without a verdict; failing open." >&2
        exit 0
        ;;
esac
