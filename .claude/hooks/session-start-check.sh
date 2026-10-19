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

# ---- 静默期决策委托:配置非法 / 未呈现的夜间简报 / 放权指纹不符(只报告,不阻断) ----
# 判定全在 quiet_briefing.py status 里,本脚本不复述任何规则。
# **这一段必须排在下面「找不到 check-runtime-sync.sh」的提前 exit 0 之前**:下游目标项目没有
# 那个脚本,排在后面就永远执行不到。
#
# **这一段的输出走 stdout,不是 stderr。** 官方 hooks 文档:退出码 0 时 hook 的 stderr 只进 debug
# 日志,人和 Claude 都看不到;而 SessionStart 的纯文本 stdout 会作为上下文加给 Claude。写到 stderr
# 等于没说。(下面既有的漂移报告仍在 stderr,是存量,去留由人裁定,本段不动它。)
# stdout 的内容会进 Claude 的上下文,所以取自 status 的文本一律先清洗(见 QUIET_CLEAN)。
#
# 三种「没有结果」要分开:
#   · 没装这个功能(脚本不存在),或跑它要用的 python3 / jq 不在 —— 静默跳过;
#   · 装了、工具也在,status 却没给出可用的结果(退出码非 0、超时、stdout 为空或不是预期的 JSON)——
#     **出声**。主仓库根不可决断时 status 正是 exit 1 且 stdout 为空;把它当成「未配置」放过,
#     该提示的简报与指纹问题就一起没了声音;
#   · 上一种情形里,能确定这个项目**从没配过** quiet_hours 的 —— 不出声(见 quiet_unconfigured)。
#     否则 python3 太旧的机器上,与这个功能毫无关系的项目每次开会话都要被告警一次。
# 自己再定位一次、不用上面的 SELF_DIR:本钩子以相对路径注册(bash .claude/hooks/…),导出了 CDPATH
# 且其下有同名目录时,cd 会先进那里并把路径打到 stdout——SELF_DIR 于是不对,「装了」被看成「没装」。
QUIET_DIR="$(CDPATH= cd "$(dirname "$0")" 2>/dev/null && pwd)" || QUIET_DIR="$SELF_DIR"
QUIET_PY="$QUIET_DIR/quiet/quiet_briefing.py"
if [ -f "$QUIET_PY" ] && command -v python3 >/dev/null 2>&1 && command -v jq >/dev/null 2>&1; then
    # 取自 status 的文本进提示前一律经 jq 清洗并限长:换行、C0 / C1 控制符、行与段分隔符、零宽与
    # 方向控制符、以及其他不显示也不占位的字符(软连字符、变体选择符、行间注释符、标签字符……)
    # 都换成空格——一个值不得冒充成另一条提示,不得带出终端控制序列,也不得夹带人看不见的内容
    # (这些文本现在会进 Claude 的上下文;标签字符 U+E0000 一段是已知的隐形注入通道)。
    # 在 jq 里按码点做(不用正则,不依赖 jq 带不带 oniguruma):shell 的 ${VAR:0:N} 在 C locale 下
    # 是按字节截的,会切在多字节字符中间;tr 在 UTF-8 locale 下遇到坏字节会直接报错。
    QUIET_CLEAN='def clean: explode | map(if . < 32 or (. >= 127 and . < 160) or . == 173 or . == 1564
        or . == 6158 or (. >= 8203 and . <= 8207) or (. >= 8232 and . <= 8238) or (. >= 8288 and . <= 8303)
        or (. >= 65024 and . <= 65039) or . == 65279 or (. >= 65529 and . <= 65531)
        or (. >= 917504 and . <= 917999) then 32 else . end) | implode;'
    quiet_field() { printf '%s' "$QS" | jq -r "$QUIET_CLEAN ($1) | tostring | clean | .[0:$2]" 2>/dev/null; }
    # 返回 0 = 能确定这个项目没配过 quiet_hours;其余一律返回非 0,由调用方照常出声。
    # **判错的代价不对称**:把「配了」判成「没配过」是静默掉一个真问题。所以这个函数的形状是
    # 「只有正面确认了才返回 0」——每一步都在确认一件看得见的事实,确认不了就出声;
    # 不是「排除掉已知的怪布局,剩下的都算没配过」(那样每多一种没想到的布局就多一处静默:
    # 裸仓库加兄弟工作树、--separate-git-dir 指向某个也叫 .git 的目录,都是这么漏出来的)。
    # 能确认的只有两种情形:
    #   (甲) **当前这棵工作树**就是主工作树——它的根恰是 git 公共目录的父目录。眼前这棵树的根下
    #        没有 arcforge.config.json,或那份配置读得出没有 quiet_hours:没配过。
    #   (乙) 当前是一棵 linked worktree(它的 git 目录在 <公共目录>/worktrees/ 下)。这里的配置是
    #        checkout 出来的副本,说了不算,要看主工作树的。可是「公共目录的父目录」是不是主工作树,
    #        从这一侧确认不了(--separate-git-dir 时它不是)。所以只有那里**确有**一份配置文件、
    #        且读得出没有 quiet_hours 时才算数;那里没有配置文件 = 拿不准。
    # 「主仓库根 = 公共目录的父目录」与 quiet_common.main_root() 是同一个推断:本函数不比 status
    # 自己更聪明,只保证 status 会报「主仓库根不可决断」的地方这里不静默。
    # 上面两种情形确认的都是「git 推出的主仓库根没配过」,而项目不一定在 git 顶层:项目放在仓库的
    # 子目录里时(配置与运行时 hooks 都在 repo/sub/ 下),git 顶层自然没有配置,status 报「主仓库根
    # 不可决断」,这里却会判成没配过。所以在那之前先看**项目自己**的位置,任何一处过不了就出声:
    #   · 钩子自己所在的项目目录——脚本目录向上两级(<项目>/.claude/hooks/ 下的本脚本)。
    #     「向上两级」有两种走法,两种都看:按字面走(项目里的运行时目录或 hooks 是指向一份共享
    #     安装的符号链接时,项目在字面路径上)与按真实路径走(hooks 是从别处链接进来的时,项目在
    #     真实路径上)。只取一种,另一个方向就看不到项目的配置。
    #   · CLAUDE_PROJECT_DIR(设了且非空时)。
    # 「过得了」= 那是个进得去的目录,且其下没有 arcforge.config.json 这个名字,或那份配置读得出
    # 没有 quiet_hours。解析不出来或进不去的目录(里面有什么看不见,「看不见」不等于「没有」)、
    # 读不出来的配置、带 quiet_hours 的配置(哪怕 enabled=false)都过不了。
    quiet_real() { [ -n "$1" ] && CDPATH= cd "$1" 2>/dev/null && pwd -P; }
    # 内嵌的 python3 一律带 -I:-c 会把当前目录放在模块搜索路径最前面,项目根下恰有 json.py 之类
    # 与标准库同名的文件时,不带 -I 就把它执行了;-I 同时不读 PYTHON* 环境变量。
    quiet_reads_as_unconfigured() {     # <配置文件>:用 status 自己的读法读得出、且没有 quiet_hours
        python3 -B -I -c 'import json, sys
data = json.load(open(sys.argv[1], encoding="utf-8"))
sys.exit(0 if isinstance(data, dict) and data.get("quiet_hours") is None else 1)' "$1" >/dev/null 2>&1
    }
    quiet_place_clear() {               # <目录>:这一处确认没配过
        [ -d "$1" ] && [ -x "$1" ] || return 1
        [ -e "$1/arcforge.config.json" ] || [ -L "$1/arcforge.config.json" ] || return 0
        [ -f "$1/arcforge.config.json" ] || return 1
        quiet_reads_as_unconfigured "$1/arcforge.config.json"
    }
    quiet_project_clear() {             # <-L|-P>:脚本目录按这种走法向上两级所到的目录,确认没配过
        local up=""
        # 脚本目录须是绝对路径:它为空或是定位失败后的回落值 "." 时,「向上两级」指向的是文件系统
        # 的根或当前目录的祖父,与项目无关。末尾垫一个 x 再去掉:命令替换会吃掉结尾的换行,
        # 目录名以换行结尾时会认到同名的兄弟目录上去。解析不出来就留空,过不了。
        case "$QUIET_DIR" in /*) up=$(cd "$1" "$QUIET_DIR/../.." 2>/dev/null && pwd "$1" && echo x) ;; esac
        quiet_place_clear "${up%$'\n'x}"
    }
    quiet_unconfigured() {
        local info top common gitdir root config
        quiet_project_clear -L || return 1
        quiet_project_clear -P || return 1
        [ -z "${CLAUDE_PROJECT_DIR:-}" ] || quiet_place_clear "$CLAUDE_PROJECT_DIR" || return 1
        # 环境里设了这三个变量中的任何一个,git 眼里的「当前工作树」就由它们决定,与人站在哪里无关:
        # 一律不做判断。三个都要管——只设 GIT_WORK_TREE 且它恰好指向公共目录的父目录时
        # (--separate-git-dir 指向 X/.git、变量指向 X),下面的判据会把 X 当成眼前这棵主工作树。
        [ -z "${GIT_DIR:-}${GIT_WORK_TREE:-}${GIT_COMMON_DIR:-}" ] || return 1
        # 一次问齐三样:当前工作树的根、公共目录、本工作树的 git 目录。不在工作树里
        # (没有 git、不是仓库、裸仓库、人在 .git 里面)时这一步就失败。
        info=$(git rev-parse --show-toplevel --git-common-dir --git-dir 2>/dev/null) || return 1
        top=$(quiet_real "${info%%$'\n'*}")
        common=${info#*$'\n'}; gitdir=$(quiet_real "${common#*$'\n'}"); common=$(quiet_real "${common%%$'\n'*}")
        # 输出须恰好三行(路径里有换行就切不准),且公共目录叫 .git(子模块、--separate-git-dir
        # 指向 store/repo.git 时不是):否则推不出主仓库根。
        [ "$(printf '%s\n' "$info" | wc -l | tr -d ' ')" = 3 ] && [ "${common##*/}" = ".git" ] || return 1
        root=${common%/.git}
        config="$root/arcforge.config.json"
        if [ "$top" = "$root" ]; then
            # (甲) 眼前这棵树的根下没有这个名字(悬空的符号链接也算「有」)。
            [ -e "$config" ] || [ -L "$config" ] || return 0
        else
            # 既不是主工作树、git 目录又不在 <公共目录>/worktrees/ 下:--separate-git-dir 指向
            # 「某处/.git」就是这样——公共目录的父目录与当前工作树毫无关系。
            case "$gitdir" in "$common"/worktrees/*) : ;; *) return 1 ;; esac
            # (乙) 那个父目录得真是一棵工作树的根(裸仓库 proj/.git 旁边不是)。
            [ "$(quiet_real "$(git -C "$root" rev-parse --show-toplevel 2>/dev/null)")" = "$root" ] || return 1
        fi
        # 须是一份普通文件:(乙) 下不存在即拿不准;目录读不了;命名管道会让读它的人一直等下去。
        [ -f "$config" ] || return 1
        # 用 status 自己的读法来读(UTF-8 严格、不认 BOM、恰好一个顶层值):读不出来的配置
        # 不算「读得出没配过」。quiet_hours 为 null 与缺键同义,与 parse() 一致。
        python3 -B -I -c 'import json, sys
data = json.load(open(sys.argv[1], encoding="utf-8"))
sys.exit(0 if isinstance(data, dict) and data.get("quiet_hours") is None else 1)' "$config" >/dev/null 2>&1
    }
    # status 的时限(秒)。它挂住时(决策日志是个 FIFO 就会这样)不得把整个钩子拖到被杀——那样下面的
    # 漂移报告也一起没了。macOS 没有 timeout 命令,这里让 python3 自己看表(QUIET_RUN):
    #   · status 起在独立的会话里,超时就连同它起的子进程整组杀掉,以 124 退出;
    #   · status 的 stdout / stderr 由启动器经管道转交,而不是让它直接继承本钩子的捕获管道:
    #     它的后代若另起了会话、手里还攥着管道,杀进程组杀不到,下面的 $(…) 会一直等到那个后代退出;
    #   · 本钩子自己被杀时(钩子超时、会话退出)启动器收到同一个信号,先把 status 那一组杀掉再走——
    #     status 在独立的会话里收不到这个信号,不管它就成了孤儿;
    #   · 超时说明写在转交的 stderr **之前**,不会被后面的限长截掉。
    # 只认 1–999 的十进制整数(10# 防止 08 被当成八进制),其余一律回落到缺省值。
    QUIET_LIMIT="${ARCFORGE_QUIET_STATUS_TIMEOUT:-20}"
    case "$QUIET_LIMIT" in
        [0-9]|[0-9][0-9]|[0-9][0-9][0-9]) QUIET_LIMIT=$((10#$QUIET_LIMIT)) ;;
        *) QUIET_LIMIT=20 ;;
    esac
    [ "$QUIET_LIMIT" -ge 1 ] || QUIET_LIMIT=20
    QUIET_RUN='import os, signal, subprocess, sys
limit, child = int(sys.argv[2]), None
def stop():
    try:
        os.killpg(child.pid, signal.SIGKILL)
    except (AttributeError, OSError):
        pass
def killed(signum, _frame):
    stop()
    os._exit(128 + signum)
for signum in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):
    signal.signal(signum, killed)
child = subprocess.Popen([sys.executable, "-B", "-I", sys.argv[1], "status"], stdin=subprocess.DEVNULL,
                         stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
try:
    out, err = child.communicate(timeout=limit)
    code = child.returncode if child.returncode >= 0 else 128 - child.returncode
except subprocess.TimeoutExpired:
    stop()
    try:
        err = child.communicate(timeout=2)[1]
    except subprocess.TimeoutExpired as late:
        err = late.stderr
    out, code = b"", 124
    err = ("status 超过 %d 秒没有返回,已终止。" % limit).encode("utf-8") + (err or b"")
sys.stderr.buffer.write(err)
sys.stdout.buffer.write(out)
sys.stderr.flush()
sys.stdout.flush()
os._exit(code)'
    # 一次调用同时拿到 stderr、退出码与 stdout,不经临时文件(不留垃圾,也不依赖 TMPDIR 可用):
    # status 的 stderr 经 fd 3 先流进外层的捕获,随后追加一行退出码与 stdout;末尾的句点
    # 保住 stdout 为空时的那个换行。
    QALL=$( { QOUT=$(python3 -B -I -c "$QUIET_RUN" "$QUIET_PY" "$QUIET_LIMIT" 2>&3); printf '\n%s\n%s.' "$?" "$QOUT"; } 3>&1 )
    QALL=${QALL%.}
    QS=${QALL##*$'\n'}; QALL=${QALL%$'\n'*}
    QRC=${QALL##*$'\n'}; QALL=${QALL%$'\n'*}
    # stdout 多于一行时,上面取到的「退出码」其实是它的倒数第二行:不是纯数字就按失败算。
    case "$QRC" in ''|*[!0-9]*) QRC="?" ;; esac
    QSHAPE=$(printf '%s' "$QS" | jq -r 'if type == "object" and (.pending_nights | type) == "array"
        and (.fp_mismatch | type) == "array" then "ok" else "bad" end' 2>/dev/null) || QSHAPE="bad"
    if [ "$QRC" != 0 ] || [ "$QSHAPE" != "ok" ]; then
        if ! quiet_unconfigured; then
            QDIAG=$(printf '%s' "$QALL" | jq -Rrs "$QUIET_CLEAN clean | .[0:300]" 2>/dev/null) || QDIAG=""
            echo "⚠ 静默期状态检查没有跑成(quiet_briefing.py status 退出码 ${QRC}),本次会话无从报告静默期的配置、简报与指纹状况: ${QDIAG}"
        fi
    else
        QERR=$(quiet_field '.error // empty' 300)
        QPEND=$(quiet_field '.pending_nights | map(tostring) | join(" ")' 300)
        QMIS=$(quiet_field '.fp_mismatch | map(tostring) | join(" ")' 300)
        QEXP=$(quiet_field '.enforce_fp_expected // empty' 64)
        case "$QERR" in
            "") : ;;
            # status 的 error 有两种文案:以「内部错误: 」开头的是它自己出了错,不是配置写错了。
            "内部错误: "*)
                echo "⚠ 静默期状态检查内部出错,下面的报告可能不完整: ${QERR}" ;;
            *)  echo "⚠ quiet_hours 配置非法,静默期决策委托已按关闭处理: ${QERR}" ;;
        esac
        if [ -n "$QPEND" ]; then
            echo "⚠ 有未呈现的夜间简报: ${QPEND} —— 请先呈现简报、处理「等你决定」的条目,再继续推进。"
        fi
        if [ -n "$QMIS" ]; then
            echo "⚠ 以下类别配置为 enforce 但放权指纹不符,已按影子档处理: ${QMIS}"
            # 这里直接给出了该抄的值,所以先指向清单:放权会打开哪些只在放权档下才执行的路径。
            echo "    → 放权前先读「只在放权档下执行的路径」清单: docs/quiet-hours-enforce-only-paths.md(目标项目里是 ~/.arcforge/docs/quiet-hours-enforce-only-paths.md)"
            echo "    → 读过并确认要放权时,把 quiet_hours.policy.<决策模型>.enforce_fp 设为: ${QEXP:-（status 没有给出这个值）}"
        fi
    fi
fi

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
