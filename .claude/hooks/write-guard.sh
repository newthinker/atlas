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

SUBAGENT_DENY_MSG="DENY: 子代理禁止调用 .arcforge/ 写通道(单写者模型,sprint-011)。
  → 子代理只做读/分析/写业务代码;结论写进最终回复带回父实例,由具名 teammate 本体落盘。
  → 若你就是具名 teammate 本体而被误拦:说明本实例未登记 token、且实例名与 agent_id 不匹配。
    请让 Leader 跑 arcforge-write.sh --as leader matrix set-token <你的实例名> 登记。"

# 子代理冒名写通道判别(sprint-011 第 3a 条;取证 spike-s1-probe.jsonl)。
# hook stdin 的 agent_id/agent_type 由 harness 填写,是**唯一不可自称**的身份信号——
# --as 与 ARCFORGE_TOKEN 都来自被冒名者自己声明的身份。Sprint E 实撞过一次子代理冒名
# --as 抢跑 dev_done,当时令牌机制是启用的。
# 四层判别(命中即停):
#   L1 无 agent_id                     ⇒ lead 本体,放行
#   L2 agent_type ∈ .tokens            ⇒ 已登记 teammate 本体,放行(权威判据)
#   L3 agent_id 以 a{agent_type}- 开头 ⇒ 未登记 teammate 本体,放行(结构兜底)
#   L4 其余                            ⇒ 子代理,DENY
# 取证形态:本体 aprobe-mate-07cdb8dffc154301/probe-mate、adev-agent-65-269af323…/dev-agent-65;
# 子代理 a83bba4eb58de545f/Explore(不含类型名);lead 两键皆无。
# L3 依赖的标识格式官方无文档、只观测过一种 teammate 名,但**失败方向是放行**(等于旧行为),
# 不会误拦合法实例;若格式将来变化,已登记实例仍由 L2 兜住。
# .tokens 为空 ⇒ 整个机制关闭(全新目标项目就是这个形态,贴合「未登记保持旧行为」)。
deny_subagent_write_channel() {
    # 只对写通道脚本生效;只读的 arcforge-validate / validator-run.sh / quiet_briefing.py 不在内。
    # quiet_apply.py 在内:它以 leader 身份调写通道,命令里却不含 arcforge-write.sh 字样。
    echo "$CMD" | grep -qE '(arcforge-write\.sh|arcforge-archive\.sh|with-task-lock\.sh|quiet_apply\.py)' || return 0
    AID=$(echo "$INPUT" | jq -r '.agent_id // empty' 2>/dev/null) || return 0
    [ -n "$AID" ] || return 0                                   # L1
    [ "$MATRIX_ACTIVE" = "true" ] || return 0                   # 无矩阵:bootstrap 期,放行
    # 先判类型再取 length。写成 `(.tokens // {}) | length` 时,.tokens 被写成数组/字符串
    # 会返回 >0(实测 ["x"]⇒1、"abc"⇒3)⇒ 机制被**打开**,而随后的 `jq -e … has($t)` 在
    # 非对象上报错(退出码 5)⇒ L2 权威判据恒失败,只剩 L3 兜底。净效果是把设计要的
    # fail-open 翻成 fail-closed:一个被写坏的矩阵会开始拒绝合法实例。
    NTOK=$(jq -r 'if (.tokens | type) == "object" then (.tokens | length) else 0 end' \
           .arcforge/write-matrix.json 2>/dev/null || echo 0)
    [ "${NTOK:-0}" -gt 0 ] || return 0                          # 未登记/矩阵损坏:保持旧行为
    ATYPE=$(echo "$INPUT" | jq -r '.agent_type // empty' 2>/dev/null) || return 0
    if [ -n "$ATYPE" ]; then
        jq -e --arg t "$ATYPE" '(.tokens // {}) | has($t)' \
            .arcforge/write-matrix.json >/dev/null 2>&1 && return 0        # L2
        case "$AID" in "a${ATYPE}-"*) return 0 ;; esac                     # L3
    fi
    echo "$SUBAGENT_DENY_MSG" >&2                                          # L4
    exit 2
}

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
# 写动词必须是独立的词(#25):原先用 \b…\b,而 `.` 与 `/` 都算单词边界 ⇒ 文件名 install.sh 被当成
# install 动词。现在两侧都只**额外**排除会让它变成另一个词的字符,其余与 \b 相同 —— 不比原先少拦:
#   前:不能紧贴字母数字 / _ / . / -(x.cp、how-to-cp 是别的词);刻意**允许** `/`:/bin/cp 仍是 cp。
#   后:不能紧贴字母数字 / _ / . / / / -(install.sh、cp-x、cp/x 是别的词);引号**允许**:cp'' 与 cp"" 仍是 cp。
VB='(^|[^[:alnum:]_.-])'
VE='[^[:alnum:]_./-]'
WRITE_RE='(>>?\|?[[:space:]]*[^[:space:]]*\.arcforge/|'"${VB}(tee|cp|mv|rsync|install|rm|truncate)${VE}"'.*\.arcforge/|'"${VB}ln${VE}"'(.*[[:space:]])?-[a-z]*f[a-z]*.*\.arcforge/|'"${VB}sed${VE}"'+-i.*\.arcforge/)'

# #25:按顶层的 && || ; | 与换行拆段,每段一行输出。宽松模式据此要求「写动词与受保护路径出现在
# 同一段」——原先对整条命令匹配,`.*` 会跨过 && 把第 1 段的动词和第 3 段的只读路径连成一次命中。
#
# ⚠ 只对**可确定解析的简单命令**拆段,其余整条退回原先的整条匹配(见 segments_or_whole)。
# 拆段的风险是把写动词与它的目标路径拆进不同段而漏拦,而「哪个 ; | 是段界」取决于 shell 语法与方言:
# PR #28 的三轮 review 依次撞出 $(…)/反引号/${…} 内的分隔符、双引号里的 $( 套内层引号、
# $'…' 与 \$'/$$' 的区分、双引号里 ${…} 中的 '(zsh 与 bash ≥4.3 当字面,而本工具的命令正是 zsh 在执行)。
# 逐个补状态机证明不了「不比原先弱」,故改为:命令里出现 ( ` ${ $' 任一即不拆。剩下的命令里能改变
# 段界的只有 '…'、"…" 与反斜杠转义,bash 与 zsh 对它们的处理一致,下面的扫描对它们是完备的。
# 代价:带子 shell / 命令替换的组合命令仍按原先的整条匹配误拦 —— 与改动前逐字相同,不会更弱。
#
# 引号内不拆(`cp x "a;b" .claude/hooks/y` 的 ; 不是段界);`>|`(强制覆盖重定向)不是管道;
# 单个 & 不拆(2>&1、&> 都含它)。引号未闭合时其后整段不再拆分,失败方向是多拦。
# 换行与 || 不需要单独处理:换行原样输出就是段界(下游 grep 逐行匹配),|| 按两个 | 拆出一个空段。
# 引号内的换行与反斜杠续行由 join_logical 另跑一遍兜住(#33),这里不处理。
# LC_ALL=C:逐字节扫描,多字节字符原样透传(分隔符都是 ASCII,不会落在多字节序列中间)。
split_segments() { # <cmd>
    printf '%s' "$1" | LC_ALL=C awk '
        BEGIN { RS = "\001" }
        {
            n = length($0); out = ""; sq = 0; dq = 0; esc = 0
            for (i = 1; i <= n; i++) {
                c = substr($0, i, 1); nx = substr($0, i + 1, 1)
                if (esc)            { out = out c; esc = 0; continue }
                if (sq)             { out = out c; if (c == "\047") sq = 0; continue }
                if (c == "\\")      { out = out c; esc = 1; continue }
                if (dq)             { out = out c; if (c == "\"") dq = 0; continue }
                if (c == "\047")    { out = out c; sq = 1; continue }
                if (c == "\"")      { out = out c; dq = 1; continue }
                if (c == ";")       { out = out "\n"; continue }
                if (c == "&" && nx == "&") { out = out "\n"; i++; continue }
                if (c == "|") {
                    if (substr(out, length(out), 1) == ">") { out = out c; continue }
                    out = out "\n"; if (nx == "&") i++; continue
                }
                out = out c
            }
            print out
        }'
}

# 不拆段的结构:( 覆盖 $( <( >( 子 shell 与 zsh 的 (a|b) 通配,` 是命令替换,${ 与 $' 各有自己的引号规则。
# $"…" 不在其中:它对段界的影响与 "…" 相同,下面的扫描按双引号处理即正确。
# 刻意不区分出现在引号内还是外:判别「在不在引号里」本身就要用到上面那套会出错的解析。
# 拆段失败、拆出空串或命令含不可拆结构时,退回整条命令(多拦不漏拦)。
segments_or_whole() { # <cmd>
    local segs
    case "$1" in
        *'('*|*'`'*|*'${'*|*"\$'"*) printf '%s\n' "$1"; return ;;
    esac
    segs=$(split_segments "$1") && [ -n "$segs" ] || segs="$1"
    printf '%s\n' "$segs"
}

# #33:下游 grep 逐行匹配,写动词与目标路径分处两行(反斜杠续行、引号内换行)时哪一行都凑不齐。
# 这里按 shell 的逻辑行折叠出另一份命令,Bash 分支对它**再跑一遍**同样的检查 —— 原始各行照旧
# 匹配,只增不减,故不比改动前少拦。折叠规则:
#   - 引号外与双引号内的 `\<换行>` 是续行,连同反斜杠一起删掉(c\<换行>p 就是 cp);单引号内是字面。
#   - 引号内含真换行的串整体换成 _nl_:路径里带换行就不是受保护路径本身,换掉它既把两侧接上,
#     又不会把多行提交说明这类散文连成一次假命中。串里有 $( 或 ` 时改为把换行换成空格 ——
#     命令替换里的写入是真写入,此时宁可多拦。
#   - 遇到引号外的 heredoc(<< 而非 <<<)就停:正文是数据,不按 shell 词法解析 —— 往脚本文件里
#     写一段带续行的 cp,折叠后就成了一次假命中;正文里的撇号也会让后面的行全被当成引号内。
#     注释(# 起头)原样抄到行尾,理由相同。
# 已知仍漏:写在 heredoc 正文**之后**的跨行写入;$((1<<2)) 也会被当成 heredoc 而停;$'…' 里的
# \' 会让引号配对错位(三者都只影响这份折叠结果,原始各行照旧匹配)。
join_logical() { # <cmd>
    printf '%s' "$1" | LC_ALL=C awk '
        function flush(  b) {
            b = (nl && index(buf, "$(") == 0 && index(buf, "`") == 0) ? "_nl_" : buf
            out = out b; buf = ""; nl = 0
        }
        BEGIN { RS = "\001" }
        {
            n = length($0); out = ""; q = ""; buf = ""; nl = 0
            for (i = 1; i <= n; i++) {
                c = substr($0, i, 1); nx = substr($0, i + 1, 1)
                if (q != "") {
                    if (c == q)                 { flush(); out = out c; q = ""; continue }
                    if (q == "\"" && c == "\\") { if (nx != "\n") buf = buf c nx; i++; continue }
                    if (c == "\n") { nl = 1; c = " " }
                    buf = buf c; continue
                }
                if (c == "\\")                 { if (nx != "\n") out = out c nx; i++; continue }
                if (c == "\047" || c == "\"") { out = out c; q = c; qs = i; continue }
                if (c == "#" && (out == "" || substr(out, length(out), 1) ~ /[ \t\n;&|(]/)) {
                    j = index(substr($0, i), "\n")
                    if (j == 0) { out = out substr($0, i); break }
                    out = out substr($0, i, j); i += j - 1; continue
                }
                if (c == "<" && nx == "<") {
                    if (substr($0, i + 2, 1) == "<") { out = out "<<<"; i += 2; continue }
                    break
                }
                out = out c
            }
            if (q != "") out = out substr($0, qs + 1)   # 引号未闭合:剩余部分原样保留
            print out
        }'
}

# #25:.arcforge/ 侧的项目外路径中和(.claude/ 侧的对应处理见 Bash 分支的 SCRUBBED)。
# ~/.arcforge/ 是框架安装目录,Leader 与人类频繁从里面复制文件;原先任何含 .arcforge/ 的
# 词都按项目内对待。这里逐个取出含 .arcforge/ 的路径词,展开 ~ / $HOME 后用 repo_rel 规范化:
# 落在项目 .arcforge/ 下的原样保留,其余换成 _ext_arcforge_。
# 与 .claude/ 侧的正则中和不同,这里走 normalize_path:`$PWD/sub/../.arcforge/` 这类绕回
# 项目内的写法仍被认出(.arcforge/ 侧原先对它是拦的,收窄不能把它放出去)。
# 含 $ ` * ? [ ~ { } \ 的词无法静态判定指向哪里,一律按项目内保留(多拦不漏拦)。
# 按出现顺序逐词拼接,不做全局替换:否则项目外的 ../.arcforge/x 会顺带把项目内
# sub/../.arcforge/x 里的同一段子串也换掉。
# ⚠ 正则切出的「词」不是 shell 的词:"$PWD"/.arcforge/x、'.'/.arcforge/x、foo=/../.arcforge/x、
# $(pwd)/.arcforge/x、/tmp/.arcforge/"../../proj/.arcforge/x" 里切出的那段单看在项目外,拼回整个词
# 却在项目内。所以只中和**独立成词**的路径:前后紧邻的都是边界(行首/行尾、空格/制表符、< > | ; &;
# 换行不算 —— 失败方向是多拦),
# 或者整个词恰好是 "…" / '…'。展开按引号区分:裸词展开 ~ 与 $HOME,"…" 只展开 $HOME(~ 不展开),
# '…' 都不展开(展开后仍含 $ 或 ~,按上一条保留)。
AF_TOK_RE='[^[:space:]"'\''=<>|;&()`]*\.arcforge/[^[:space:]"'\''<>|;&()`]*'
# word_boundary <字符>:0 = 该字符(空串即行首/行尾)在 shell 里一定是词的边界。
word_boundary() { case "$1" in ''|' '|$'\t'|'<'|'>'|'|'|';'|'&') return 0 ;; esac; return 1; }
# word_quote <词之前的全部文本> <词之后的全部文本>:回显 bare / " / ',表示这个路径词独立成词的方式;
# 不独立成词(与引号、$(…)、= 等拼接)时返回 1。
word_quote() {
    local before="$1" after="$2" p1 p2 n1 n2 b2 a2
    p1="${before#"${before%?}"}"; b2="${before%?}"; p2="${b2#"${b2%?}"}"
    n1="${after%"${after#?}"}";   a2="${after#?}";  n2="${a2%"${a2#?}"}"
    if word_boundary "$p1" && word_boundary "$n1"; then echo bare; return 0; fi
    if { [ "$p1" = '"' ] || [ "$p1" = "'" ]; } && [ "$n1" = "$p1" ] \
       && word_boundary "$p2" && word_boundary "$n2"; then echo "$p1"; return 0; fi
    return 1
}
scrub_arcforge_ext() { # <cmd>
    local rest="$1" out="" tok exp q
    while IFS= read -r tok; do
        [ -n "$tok" ] || continue
        case "$rest" in *"$tok"*) ;; *) continue ;; esac
        out="$out${rest%%"$tok"*}"; rest="${rest#*"$tok"}"
        q=$(word_quote "$out" "$rest") || { out="$out$tok"; continue; }
        exp="$tok"
        case "$q" in
            bare) case "$exp" in '~/'*|'$HOME/'*|'${HOME}/'*) exp="$HOME/${exp#*/}" ;; esac ;;
            '"')  case "$exp" in '$HOME/'*|'${HOME}/'*)       exp="$HOME/${exp#*/}" ;; esac ;;
        esac
        case "$exp" in
            *'$'*|*'`'*|*'*'*|*'?'*|*'['*|*'~'*|*'{'*|*'}'*|*'\'*) out="$out$tok"; continue ;;
        esac
        case "$(repo_rel "$exp")" in
            .arcforge/*) out="$out$tok" ;;
            *)           out="${out}_ext_arcforge_" ;;
        esac
    done <<EOF
$(printf '%s\n' "$1" | grep -oE "$AF_TOK_RE")
EOF
    printf '%s' "$out$rest"
}

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

# .claude/ 运行时写检测。
# 只保护本项目的 .claude/:家目录(~、$HOME)及其他项目外绝对路径下的
# .claude/ 是用户级配置,不在本 hook 职责内,不得误拦——先把项目绝对
# 路径归一化为相对,再把其余前缀带 ~ / $HOME / 绝对路径的 .claude/
# 中和为无害 token,后续启发式只对项目内引用生效。
#
# C4 补丁 round 2(reviewer 查出):上面的折叠只堵了 $PWD 与 .claude 之间的噪声,
# .claude 与 hooks|scripts|settings 之间同样可以插入 // 或 /./ ——纯相对路径形态
# (如 .claude//hooks/y、.claude/./hooks/y)压根不经过 $PWD 相关的折叠/外部化两步
# 就直接落到这里的检测正则,而原正则 `\.claude/(hooks|scripts|settings)` 要求
# .claude 与目标目录名之间**恰好一个斜杠**,// 或 /./ 都会让它失配、判定被跳过。
# .arcforge/ 侧对此免疫(它的正则只要求 `\.arcforge/` 出现,不要求后面紧跟特定
# 目录名,不受影响,不要动那一侧)。这条检测先于 STRICT 分支,故此处一次性
# 修复同时覆盖宽松与严格两种模式,无需在 strict 分支再补一份。
# 资产路径只写一次:门禁与「六选一写动词」模式共用同一份。六个动词分支共享同一
# 后缀,故按 (动词1|…|动词6)后缀 提取(与上面 .arcforge/ 侧的 WRITE_RE 同型),
# 而不是逐个分支插值 —— 后者要写 6 遍,漏改一处就是一个静默绕点。
CLAUDE_ASSET_RE='\.claude(/\.)*/+(hooks|scripts|settings)'
CLAUDE_WRITE_RE='(>>?\|?[[:space:]]*[^[:space:]]*|'"${VB}(tee|cp|mv|rsync|install|rm|chmod|truncate)${VE}"'.*|'"${VB}ln${VE}"'(.*[[:space:]])?-[a-z]*f[a-z]*.*|'"${VB}sed${VE}"'+-i.*)'"$CLAUDE_ASSET_RE"
claude_runtime_check() { # <cmd>;命中则 exit 2
    local scrubbed
    # C4 补丁:原先只认单斜杠字面量 "$PWD/.claude/",对 "$PWD//.claude/"、
    # "$PWD/../<自身目录名>/.claude/" 这类自然变体,会被下面「外部路径无害化」
    # 那一步误判成仓库外(不是漏拦,是被误分类成外部,危害更高——那一步是刻意
    # 放行仓库外同名路径的规则)。故先局部折叠这两种形态,只在 $PWD 紧邻
    # .claude/ 的这一段做替换,不做全命令级的 // 折叠,避免误伤 http:// 等
    # 无关内容。变量拼接(D=.claude; cp x $D/hooks/y)不在本次收口范围内——
    # 那需要主动构造,文件头注释已声明本分支是「宁漏不误拦」的启发式。
    scrubbed=$(printf '%s' "$1" | sed -E \
        -e "s#(${PWD_ESC})/+\\.claude/#\\1/.claude/#g" \
        -e "s#(${PWD_ESC})/\\.\\./${PWD_BASENAME_ESC}/+\\.claude/#\\1/.claude/#g") || scrubbed="$1"
    PROJ_CLAUDE="$PWD/.claude/"
    # 先赋给变量再展开:bash 3.2 下 ${x//"$PWD/.claude/"/…} 会把引号里的 / 当成分隔符。
    scrubbed=${scrubbed//"$PROJ_CLAUDE"/.claude/}
    scrubbed=$(printf '%s' "$scrubbed" | sed -E 's#(~|\$HOME|/[^[:space:]"'\''/][^[:space:]"'\'']*)/\.claude/#\1/_ext_claude_/#g') || scrubbed="$1"
    if echo "$scrubbed" | grep -qE "$CLAUDE_ASSET_RE"; then
        # #25:逐段匹配(见 split_segments)。
        if segments_or_whole "$scrubbed" | grep -qE "$CLAUDE_WRITE_RE"; then
            echo "$RUNTIME_DENY_MSG" >&2; exit 2
        fi
    fi
}

# 「写动词 × .arcforge 路径」启发式(宁漏不误拦)
# #25:先中和项目外的 .arcforge/ 路径,再逐段匹配;任一步失败都退回原始整条命令。
arcforge_lax_check() { # <cmd>;命中则 exit 2
    local acmd
    acmd=$(scrub_arcforge_ext "$1") || acmd="$1"
    if echo "$acmd" | grep -q '\.arcforge/'; then
        if segments_or_whole "$acmd" | grep -qE "$WRITE_RE"; then
            echo "$DENY_MSG" >&2
            exit 2
        fi
    fi
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
    # 子代理冒名写通道 DENY(sprint-011 第 3a 条)。位置刻意在**两条白名单之前**:
    # 严格模式的 classify_segment 与宽松模式的行内 grep 是两个**各自独立**的白名单,
    # 判别插在任一之后,另一条就是现成的绕过口。
    deny_subagent_write_channel
    # .claude/ 运行时写检测先于白名单——白名单脚本也不允许写运行时(见 claude_runtime_check)。
    # #33:对原始命令与折叠出的逻辑行各跑一遍(见 join_logical);两者相同时只跑一遍。
    JCMD=$(join_logical "$CMD") || JCMD="$CMD"
    claude_runtime_check "$CMD"
    [ "$JCMD" = "$CMD" ] || claude_runtime_check "$JCMD"
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
    arcforge_lax_check "$CMD"
    [ "$JCMD" = "$CMD" ] || arcforge_lax_check "$JCMD"
    ;;
esac
exit 0
