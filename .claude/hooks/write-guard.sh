#!/bin/bash
# write-guard.sh — PreToolUse 封口(防线 2,ISSUE-4):
# 拦截绕过 arcforge-write.sh 的 .arcforge/ 直接写,以及 .claude/ 运行时资产
# (hooks/scripts/settings)的修改。只拦写,读永远放行。
# ⚠️ 口径如实:Bash 侧是「常见写动词启发式」,非完备拦截(python/perl/heredoc/
#    变量拼接可逃逸);深度防御依赖单写者矩阵 + validator 审计(防线 3)。
#    身份(--as)是声明式的,hook 无法核验真实调用者。
# exit 0 = 放行;exit 2 = 拒绝(stderr 注入给 agent 作纠偏提示)。
# 原则:宁可漏(防线 3 兜底)不可误拦(会卡死正常流程);自身异常 fail-open。
# .claude/ 运行时资产对全体 agent 只读(含 Leader),不依赖矩阵。
# sprint-001 实测:提示层不足以约束(QA 越权直改 hook)。
set -uo pipefail

INPUT=$(cat) || exit 0
TOOL=$(echo "$INPUT" | jq -r '.tool_name // empty' 2>/dev/null) || exit 0
[ -n "$TOOL" ] || exit 0

MATRIX_ACTIVE="false"
[ -f ".arcforge/write-matrix.json" ] && MATRIX_ACTIVE="true"

DENY_MSG="DENY: 直接写 .arcforge/ 受保护文件被禁止(单写者模型,ISSUE-4)。
  → 请改用: bash .claude/hooks/arcforge-write.sh --as <你的实例名> ...(用法见脚本头注释)
  → 你没有实例名(子代理)? 禁止写状态文件——把结论写进最终回复带回父 agent,由其落盘。"

RUNTIME_DENY_MSG="DENY: .claude/ 运行时资产(hooks/scripts/settings)对所有 agent 只读(含 Leader)。
  → 需要变更? 修改 project-template/ 对应文件并走 TDD;运行时同步由人类确认后在会话外执行。"

# 规范化路径并判断是否落在 .arcforge/ 内
protected() {
    local p="$1"
    case "$p" in /*) p="${p#"$PWD"/}" ;; esac   # 绝对路径 → 相对(仅当前项目)
    p="${p#./}"
    case "$p" in .arcforge/*) return 0 ;; esac
    return 1
}

# 判定 .claude/ 受保护运行时资产(不依赖矩阵,对全体 agent 生效)
runtime_protected() {
    local p="$1"
    case "$p" in /*) p="${p#"$PWD"/}" ;; esac
    p="${p#./}"
    case "$p" in
        .claude/hooks/*|.claude/scripts/*|.claude/settings.json|.claude/settings.local.json) return 0 ;;
    esac
    return 1
}

case "$TOOL" in
  Write|Edit)
    FP=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty' 2>/dev/null) || exit 0
    [ -n "$FP" ] || exit 0
    if runtime_protected "$FP"; then
        echo "$RUNTIME_DENY_MSG" >&2; exit 2
    fi
    if [ "$MATRIX_ACTIVE" = "true" ] && protected "$FP"; then
        echo "$DENY_MSG" >&2; exit 2
    fi
    ;;
  Bash)
    CMD=$(echo "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null) || exit 0
    [ -n "$CMD" ] || exit 0
    # .claude/ 运行时写检测(先于白名单——白名单脚本也不允许写运行时)。
    # 只保护本项目的 .claude/:家目录(~、$HOME)及其他项目外绝对路径下的
    # .claude/ 是用户级配置,不在本 hook 职责内,不得误拦——先把项目绝对
    # 路径归一化为相对,再把其余前缀带 ~ / $HOME / 绝对路径的 .claude/
    # 中和为无害 token,后续启发式只对项目内引用生效。
    PROJ_CLAUDE="$PWD/.claude/"
    SCRUBBED=${CMD//"$PROJ_CLAUDE"/.claude/}
    SCRUBBED=$(printf '%s' "$SCRUBBED" | sed -E 's#(~|\$HOME|/[^[:space:]"'\''/][^[:space:]"'\'']*)/\.claude/#\1/_ext_claude_/#g') || SCRUBBED="$CMD"
    if echo "$SCRUBBED" | grep -qE '\.claude/(hooks|scripts|settings)'; then
        if echo "$SCRUBBED" | grep -qE '(>>?\|?[[:space:]]*[^[:space:]]*\.claude/(hooks|scripts|settings)|\btee\b.*\.claude/(hooks|scripts|settings)|\b(cp|mv|rsync|install)\b.*\.claude/(hooks|scripts|settings)|\bln\b.*[[:space:]]-[a-z]*f[a-z]*.*\.claude/(hooks|scripts|settings)|\bsed[[:space:]]+-i.*\.claude/(hooks|scripts|settings)|\b(rm|chmod|truncate)\b.*\.claude/(hooks|scripts|settings))'; then
            echo "$RUNTIME_DENY_MSG" >&2; exit 2
        fi
    fi
    [ "$MATRIX_ACTIVE" = "true" ] || exit 0
    # 白名单:合法写入通道与框架自有脚本
    if echo "$CMD" | grep -qE '(arcforge-write\.sh|arcforge-archive\.sh|arcforge-validate)'; then
        exit 0
    fi
    # 「写动词 × .arcforge 路径」启发式(宁漏不误拦)
    if echo "$CMD" | grep -q '\.arcforge/'; then
        if echo "$CMD" | grep -qE '(>>?\|?[[:space:]]*[^[:space:]]*\.arcforge/|\btee\b.*\.arcforge/|\b(cp|mv|rsync|install)\b.*\.arcforge/|\bln\b.*[[:space:]]-[a-z]*f[a-z]*.*\.arcforge/|\bsed[[:space:]]+-i.*\.arcforge/|\brm\b.*\.arcforge/|\btruncate\b.*\.arcforge/)'; then
            echo "$DENY_MSG" >&2
            exit 2
        fi
    fi
    ;;
esac
exit 0
