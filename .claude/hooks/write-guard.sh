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
#
# A-1 严格模式(ARCFORGE_GUARD_STRICT=1,灰度开关):
#   仅影响 Bash 分支的 .arcforge/ 判定(.claude/ 运行时保护逻辑不动、先于严格分支执行)。
#   借 grok bash_command_splitting 语义:含 .arcforge/ 的命令先做复杂结构短路
#   (heredoc/命令替换/eval/-c/-e/xargs/source → 整体「无法归类」DENY),否则按
#   && || ; | 换行拆段逐段分类,任一段命中写模式或无法归类即整体 exit 2(fail-closed)。
#   未设置/非 "1" = 现行宽松行为,完全向后兼容(回归测试逐用例保证)。
#   已知限制:git 首词整体在读白名单内,git checkout -- .arcforge/ 类恢复可改文件
#     ——git 操作可回溯且有防线 3(validator 审计)兜底,本轮接受;引号内的 ;|
#     会被拆段(如 echo "a;b .arcforge/x"),可能 fail-closed 误拦,属严格模式接受
#     成本(绕行走 scratchpad 文件 + '< 文件' 重定向,勿用 heredoc)。
#   异常语义:严格模式下解析不确定视为无法归类(fail-closed 本意);hook 自身运行
#     故障(如 jq 缺失)仍 fail-open + stderr 警告,防线故障不得卡死全部 Bash 调用。
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

# C4:纯字符串路径规范化——折叠 //、/./、/../,不依赖目标存在(不能用
# `cd $(dirname) && pwd -P`:对尚不存在的目录会失败,而「写一个尚不存在的目录下的
# 新文件」正是要堵的形态,失败即 fail-open)。不解析符号链接——那需要目标存在,
# 符号链接绕过属已知未覆盖面(见 discovery)。
# ⚠ unset "out[...]" 与 ${out[@]+"${out[@]}"} 是 bash 3.2 下空数组安全的写法,
#   写成 "${out[@]}" 在空数组上会报 unbound variable(与 W8 同一个坑)。
normalize_path() {
    local p="$1" seg out=() IFS=/
    case "$p" in /*) : ;; *) p="$PWD/$p" ;; esac
    for seg in $p; do
        case "$seg" in
            ''|.) continue ;;
            ..)   [ ${#out[@]} -gt 0 ] && unset "out[$(( ${#out[@]} - 1 ))]" ;;
            *)    out+=("$seg") ;;
        esac
    done
    printf '/%s' ${out[@]+"${out[@]}"}
}

PWD_REAL=$(normalize_path "$PWD")

# C4(Bash 分支专用):把字符串转义成可安全嵌入 ERE 的字面量。下面折叠
# "$PWD//.claude/"、"$PWD/../<自身目录名>/.claude/" 这两种路径变体时,要把
# $PWD(及其 basename)当字面量整体嵌进正则,必须先转义其中的正则特殊字符,
# 否则目录名里出现 . + ( ) [ ] 等字符会让正则语义跑偏。
# ⚠ 转义集只对**匹配侧**成立,不要把 ${PWD_ESC} 再塞进 s/// 的**替换侧**:替换侧
#   的元字符集不同(& = 整个匹配),而 & 在 ERE 里不是元字符、故这里刻意不转义它,
#   于是 PWD 含 & 时替换侧会把整段匹配回填进去,折叠静默失效(实测 /tmp/a&b:
#   "cp /tmp/x /tmp/a&b//.claude/hooks/y" 折叠不掉 → 绕过重新成立)。替换侧一律
#   用反向引用 \1 回填 $PWD 原文,不依赖「转义后的串在替换侧恰好还原」这一巧合。
ere_escape() { printf '%s' "$1" | sed 's/[][\.^$*+?(){}|\\]/\\&/g'; }
PWD_ESC=$(ere_escape "$PWD")
PWD_BASENAME_ESC=$(ere_escape "$(basename "$PWD")")

# 规范化 → 转成相对项目根的路径。项目外的路径前缀剥不掉,原样保留绝对形式
# (以 / 开头),必不匹配下面两个判定里的相对模式 —— 仓库外的同名文件不误拦。
repo_rel() {
    local p; p=$(normalize_path "$1")
    printf '%s' "${p#"$PWD_REAL"/}"
}

# 判断是否落在项目 .arcforge/ 内
protected() {
    case "$(repo_rel "$1")" in .arcforge/*) return 0 ;; esac
    return 1
}

# 判定 .claude/ 受保护运行时资产(不依赖矩阵,对全体 agent 生效)
runtime_protected() {
    case "$(repo_rel "$1")" in
        .claude/hooks/*|.claude/scripts/*|.claude/settings.json|.claude/settings.local.json) return 0 ;;
    esac
    return 1
}

# ---- A-1 严格模式:拆段分类,无法归类 fail-closed(ARCFORGE_GUARD_STRICT=1 启用) ----
# WRITE_RE 与宽松分支共用(避免两处正则漂移);READ_CMDS 为读白名单首词。
STRICT_WHY=""
READ_CMDS=" jq cat ls grep egrep fgrep head tail wc find diff stat sort uniq cut tr file du comm cmp column test [ [[ md5sum shasum sha256sum echo printf date git basename dirname readlink realpath env true : "
WRITE_RE='(>>?\|?[[:space:]]*[^[:space:]]*\.arcforge/|\btee\b.*\.arcforge/|\b(cp|mv|rsync|install)\b.*\.arcforge/|\bln\b.*[[:space:]]-[a-z]*f[a-z]*.*\.arcforge/|\bsed[[:space:]]+-i.*\.arcforge/|\brm\b.*\.arcforge/|\btruncate\b.*\.arcforge/)'

strict_deny() { # <触发段> <归类原因>
    echo "$DENY_MSG" >&2
    echo "  [严格模式] 触发段: $1" >&2
    echo "  [严格模式] 归类: $2(写通道内容请用 scratchpad 文件 + '< 文件' 重定向,勿用 heredoc)" >&2
    exit 2
}

classify_segment() { # <seg>;0=放行,1=拒(STRICT_WHY 置原因)
    local seg="$1" first rest
    while :; do   # 剥离控制字前缀后归类真实命令
        first="${seg%%[[:space:]]*}"
        case "$first" in
            if|then|else|elif|while|until|do)
                rest="${seg#"$first"}"; seg="${rest#"${rest%%[![:space:]]*}"}"
                [ -n "$seg" ] || return 0 ;;
            *) break ;;
        esac
    done
    case "$seg" in *".arcforge/"*) ;; *) return 0 ;; esac
    case "$first" in for|done|fi|esac) return 0 ;; esac   # 纯控制结构,无执行语义
    if printf '%s\n' "$seg" | grep -qE '(arcforge-write\.sh|arcforge-archive\.sh|arcforge-validate|validator-run\.sh|with-task-lock\.sh)'; then
        return 0   # 合法写通道(段级,整条命令不再因提及而全放行)
    fi
    if printf '%s\n' "$seg" | grep -qE "$WRITE_RE"; then
        STRICT_WHY="命中写模式"; return 1
    fi
    case "$READ_CMDS" in *" $first "*)
        if printf '%s\n' "$seg" | grep -qE '>>?[[:space:]]*[^[:space:]]*\.arcforge/'; then
            STRICT_WHY="白名单命令但重定向写入 .arcforge/"; return 1
        fi
        return 0 ;;
    esac
    STRICT_WHY="无法归类(fail-closed)"; return 1
}

strict_check_arcforge() { # <cmd>;放行则正常返回,拒则 exit 2
    local cmd="$1" seg
    case "$cmd" in *".arcforge/"*) ;; *) return 0 ;; esac
    if printf '%s\n' "$cmd" | grep -qE '<<|\$\(|`|\beval\b|\b(bash|sh|zsh)\b[^|&;]*[[:space:]]-c\b|\b(python3?|perl|ruby)\b[^|&;]*[[:space:]]-[ec]\b|\bxargs\b|\bsource\b|(^|[;&|[:space:]])\.[[:space:]]'; then
        strict_deny "<整条命令>" "含复杂结构(heredoc/命令替换/eval/-c/-e/xargs/source),无法安全归类"
    fi
    while IFS= read -r seg; do
        seg="${seg#"${seg%%[![:space:]]*}"}"; seg="${seg%"${seg##*[![:space:]]}"}"
        [ -n "$seg" ] || continue
        classify_segment "$seg" || strict_deny "$seg" "$STRICT_WHY"
    done <<SEGEOF
$(printf '%s\n' "$cmd" | sed -E 's/\|\|/\n/g; s/&&/\n/g; s/;/\n/g; s/\|/\n/g')
SEGEOF
    return 0
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
    #
    # C4 补丁:原先只认单斜杠字面量 "$PWD/.claude/",对 "$PWD//.claude/"、
    # "$PWD/../<自身目录名>/.claude/" 这类自然变体,会被下面「外部路径无害化」
    # 那一步误判成仓库外(不是漏拦,是被误分类成外部,危害更高——那一步是刻意
    # 放行仓库外同名路径的规则)。故先局部折叠这两种形态,只在 $PWD 紧邻
    # .claude/ 的这一段做替换,不做全命令级的 // 折叠,避免误伤 http:// 等
    # 无关内容。变量拼接(D=.claude; cp x $D/hooks/y)不在本次收口范围内——
    # 那需要主动构造,文件头注释已声明本分支是「宁漏不误拦」的启发式。
    SCRUBBED=$(printf '%s' "$CMD" | sed -E \
        -e "s#(${PWD_ESC})/+\\.claude/#\\1/.claude/#g" \
        -e "s#(${PWD_ESC})/\\.\\./${PWD_BASENAME_ESC}/+\\.claude/#\\1/.claude/#g") || SCRUBBED="$CMD"
    PROJ_CLAUDE="$PWD/.claude/"
    SCRUBBED=${SCRUBBED//"$PROJ_CLAUDE"/.claude/}
    SCRUBBED=$(printf '%s' "$SCRUBBED" | sed -E 's#(~|\$HOME|/[^[:space:]"'\''/][^[:space:]"'\'']*)/\.claude/#\1/_ext_claude_/#g') || SCRUBBED="$CMD"
    # C4 补丁 round 2(reviewer 查出):上面的折叠只堵了 $PWD 与 .claude 之间的噪声,
    # .claude 与 hooks|scripts|settings 之间同样可以插入 // 或 /./ ——纯相对路径形态
    # (如 .claude//hooks/y、.claude/./hooks/y)压根不经过 $PWD 相关的折叠/外部化两步
    # 就直接落到这里的检测正则,而原正则 `\.claude/(hooks|scripts|settings)` 要求
    # .claude 与目标目录名之间**恰好一个斜杠**,// 或 /./ 都会让它失配、判定被跳过。
    # .arcforge/ 侧对此免疫(它的正则只要求 `\.arcforge/` 出现,不要求后面紧跟特定
    # 目录名,不受影响,不要动那一侧)。这条检测先于下面的 STRICT 分支,故此处一次性
    # 修复同时覆盖宽松与严格两种模式,无需在 strict 分支再补一份。
    # 资产路径只写一次:门禁与「六选一写动词」模式共用同一份。六个动词分支共享同一
    # 后缀,故按 (动词1|…|动词6)后缀 提取(与上面 .arcforge/ 侧的 WRITE_RE 同型),
    # 而不是逐个分支插值 —— 后者要写 6 遍,漏改一处就是一个静默绕点。
    CLAUDE_ASSET_RE='\.claude(/\.)*/+(hooks|scripts|settings)'
    CLAUDE_WRITE_RE='(>>?\|?[[:space:]]*[^[:space:]]*|\btee\b.*|\b(cp|mv|rsync|install)\b.*|\bln\b.*[[:space:]]-[a-z]*f[a-z]*.*|\bsed[[:space:]]+-i.*|\b(rm|chmod|truncate)\b.*)'"$CLAUDE_ASSET_RE"
    if echo "$SCRUBBED" | grep -qE "$CLAUDE_ASSET_RE"; then
        if echo "$SCRUBBED" | grep -qE "$CLAUDE_WRITE_RE"; then
            echo "$RUNTIME_DENY_MSG" >&2; exit 2
        fi
    fi
    [ "$MATRIX_ACTIVE" = "true" ] || exit 0
    # A-1:严格模式启用时走拆段分类器(fail-closed),否则维持现行宽松启发式。
    if [ "${ARCFORGE_GUARD_STRICT:-0}" = "1" ]; then
        strict_check_arcforge "$CMD"
        exit 0
    fi
    # 白名单:合法写入通道与框架自有脚本
    if echo "$CMD" | grep -qE '(arcforge-write\.sh|arcforge-archive\.sh|arcforge-validate)'; then
        exit 0
    fi
    # 「写动词 × .arcforge 路径」启发式(宁漏不误拦)
    if echo "$CMD" | grep -q '\.arcforge/'; then
        if echo "$CMD" | grep -qE "$WRITE_RE"; then
            echo "$DENY_MSG" >&2
            exit 2
        fi
    fi
    ;;
esac
exit 0
