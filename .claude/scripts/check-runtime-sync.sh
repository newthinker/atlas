#!/bin/bash
# check-runtime-sync.sh — 模板↔运行时零漂移机械化校验(backlog-sprint-c #1,TASK-C3)。
# 供 arcforge-archive.sh 前置门禁与人类收尾核验调用。
#
# 退出码携带「我到底验了什么」(TASK-017)——修前 0 兼表「逐字节比对全过」与「根本没比对」,
# 归档硬门禁只看 `bash check || ABORT`,于是**无从验证被当成了验证通过**:
#   0 = 已实际比对 N>0 项且零漂移        ← 唯一的「验过且全过」
#   1 = 漂移 / 缺失 / 非法 JSON / 非对象,stderr 指名文件
#   2 = **无从验证**:无运行时布局,或模板侧存在却一项都没比成 ⇒ 归档必须阻断
#   3 = **不适用**:无模板侧(project-template/ 与 templates/write-matrix.json 均无),
#       即已安装 Arcforge 的消费项目的正常形态 ⇒ 归档放行,但调用方须显式声明未经校验
# 2 与 3 刻意分开:两者的处置方向相反 —— 2 要去查「是不是站错目录 / 运行时没装」,
# 3 是「本仓库压根没有模板可比」,把 3 也阻断就会卡死每一个消费项目的归档。
# 老调用方若用 `check || ...`,2/3 都落入失败分支 —— 失效方向安全(宁可多拦不可静默放行)。
# 校验范围:project-template/hooks ↔ .claude/hooks、project-template/scripts ↔
# .claude/scripts(逐字节 cmp)、templates/write-matrix.json ↔ .arcforge/write-matrix.json
# (jq -S 语义,键序无关,**逐顶层字段**比对并在文案里指名字段)。
# settings 漂移由 test-settings-sync.sh 独立覆盖,不在此。
# bash 3.2 兼容:不用数组;失败路径显式处理。
set -uo pipefail
RC=0
drift() { echo "DRIFT: $1" >&2; RC=1; }
# 「验了几项」的计数器。零漂移的绿必须由它背书:RC 保持 0 既可能是「逐项比对全过」,
# 也可能是「一项都没进循环」——后者正是 8f548ac 静默放行回归的机制,也是本脚本
# 无运行时布局时 exit 0 的机制。计数让这两者在退出码上分道扬镳。
CHECKED_FILES=0
CHECKED_FIELDS=0

# ---- 运行时专有字段(TASK-016)------------------------------------------------
# 只在运行时矩阵里有内容、模板侧恒为空的顶层字段,不参与语义比对。
# 背景:tokens 装的是本 session 每个实例的 sha256,模板永远是 {}。整文件比对因此
# **只要注册过一个 token 就永远红**,而归档硬门禁依赖本检查(--force 不豁免)⇒ 归不了档。
# 更坏的是恒红会训练人忽略输出:实测四条 DRIFT 里三真一假,而人无从分辨。
#
# 判据选**显式黑名单**,而非白名单 / 命名前缀 / 模板侧存在性:
#  · 白名单(只比对列出的字段):漏登记一个新的策略字段 ⇒ 该字段**静默**不再比对 = 假绿,
#    正是本 sprint 反复出现的「空真通过」;
#  · 命名前缀(如 runtime_*):要把既有的 tokens 改名,牵连写通道,代价与收益不成比例;
#  · 模板侧存在性:不成立 —— tokens 在模板里**是存在的**(值为 {}),模板正是靠这个空值
#    声明「该字段归运行时所有」。
# 黑名单的失效方向是安全的:将来新增的运行时专有字段若忘了登记,只会被**报成漂移**
# (噪音,人一眼看见并来改这里),而不会被静默跳过。
# ⇒ 是的,新增运行时专有字段需要改下面这一行常量。这是刻意的:每一次「不比对」都应当是
#   一次显式、可 review 的决定。
RUNTIME_ONLY_FIELDS="tokens"

# ---- 文件级忽略名单(TASK-005 review I3)---------------------------------------
# W3 把 hooks/scripts 比对从「只认模板侧 *.sh」改成「两侧文件名并集、不限扩展名」后,
# 系统/编辑器噪音文件(Finder 的 .DS_Store、补丁残留 *.orig/*.rej、编辑器 swap *.swp/*~)
# 会被判成「运行时独有」DRIFT ⇒ 归档 ABORT 且 --force 不豁免——在 macOS 上 Finder 打开一次
# 那个目录就够,触发者与门禁想防的东西(模板↔运行时真漂移)毫无关系。
#
# 判据同样是**显式黑名单**,与上面 RUNTIME_ONLY_FIELDS 同一口径:漏登记一种新的噪音文件
# 只会被**报成 DRIFT**(可见噪音,人一眼看见来改这里),不会被静默放过;不得改成「只放行
# 认识的扩展名」的白名单——那正是 W3 要消灭的原病(旧代码只认 .sh,新噪音一样会撞上,
# 只是换了个白名单形状)。
IGNORE_FILE_NAMES=".DS_Store *.orig *.rej *.swp *~"
is_ignored_file() { # <basename> —— 命中黑名单里任一 glob 即忽略,不比对也不计入 CHECKED_FILES
    local b="$1" pat
    for pat in $IGNORE_FILE_NAMES; do
        case "$b" in
            $pat) return 0 ;;
        esac
    done
    return 1
}

if [ ! -d ".claude" ] && [ ! -d ".arcforge" ]; then
    echo "WARN: 无运行时布局(.claude/.arcforge 均不存在),同步校验**无从进行**——这不是零漂移。" >&2
    echo "      处置:确认是否在仓库根执行,或运行时是否尚未安装;不要去同步文件。" >&2
    exit 2
fi
command -v jq >/dev/null 2>&1 || { echo "ERROR: 需要 jq" >&2; exit 1; }

# 适用性判据 = **模板侧是否存在**。消费项目(已安装 Arcforge 的业务仓库)只有运行时副本,
# 没有 project-template/ 与 templates/,此时本校验无对象可比,阻断归档纯属误伤。
# 与「零比对项」分开判:模板侧在却没比成,是该验没验成(2);模板侧压根不在,是不该验(3)。
if [ -d "project-template" ] || [ -f "templates/write-matrix.json" ]; then
    APPLICABLE=1
else
    APPLICABLE=0
fi

# W3(TASK-005):改单向遍历模板侧 *.sh 为**两侧文件名并集**、不限扩展名——旧写法只从
# SRC 出发,运行时独有文件(模板侧不存在)与非 .sh 资产因此对本脚本整体隐形,仍报 SYNC OK。
# 与下面 :111-113 的矩阵字段比对(两侧键并集)同口径,同一份脚本不该有两种口径。
#
# 闸门分两层,粒度必须与 APPLICABLE 对齐(review I1,替换掉本任务早前 `[ -d "$SRC" ] ||
# continue` 那版回退——诊断对了方向,粒度错了层级):
#   1. `[ "$APPLICABLE" -eq 1 ] || continue` —— **仓库级**闸门,与矩阵比对同口径。已安装
#      Arcforge 的消费项目(project-template/ 整个不存在,APPLICABLE=0)天然没有模板对应物,
#      那是「不适用」不是漂移;这层守住 V4/G4(exit 3 不被 exit 1 覆盖)。
#   2. `[ -d "$SRC" ] || [ -d "$DST" ] || continue` —— **目录级**闸门,两侧任一存在即比对。
#      若只有第 1 层没有第 2 层(APPLICABLE=1 时仍是 `[ -d "$SRC" ] || continue`),会在
#      「project-template/ 存在但 project-template/hooks 这个子目录不存在、.claude/hooks
#      却存在」时把整个子目录判成隐形——这正是 W3 要消灭的形态,只是从文件粒度挪到了目录
#      粒度,且 `CHECKED_FILES==0 ⇒ exit 2` 接不住(另一对还在比,总数不会归零)。
for PAIR in "project-template/hooks:.claude/hooks" "project-template/scripts:.claude/scripts"; do
    SRC="${PAIR%%:*}"; DST="${PAIR#*:}"
    [ "$APPLICABLE" -eq 1 ] || continue
    [ -d "$SRC" ] || [ -d "$DST" ] || continue
    # 并集:两侧文件名取并,消除「运行时独有 ⇒ 隐形」。不限 .sh:非脚本资产同样会漂移。
    NAMES=$( { [ -d "$SRC" ] && find "$SRC" -maxdepth 1 -type f -exec basename {} \; ; \
               [ -d "$DST" ] && find "$DST" -maxdepth 1 -type f -exec basename {} \; ; } | sort -u )
    while IFS= read -r B; do
        [ -n "$B" ] || continue
        is_ignored_file "$B" && continue   # 黑名单命中:不比对,也不计入 CHECKED_FILES
        if   [ ! -f "$SRC/$B" ]; then drift "$DST/$B 是运行时独有(模板侧不存在)"
        elif [ ! -f "$DST/$B" ]; then drift "$DST/$B 缺失(模板侧存在)"
        elif ! cmp -s "$SRC/$B" "$DST/$B"; then drift "$DST/$B 与 $SRC/$B 字节不一致"
        fi
        # 四个分支都是「对该文件下了判断」⇒ 都算比对过一项(缺失/独有也是一种结论)
        CHECKED_FILES=$((CHECKED_FILES + 1))
    done <<< "$NAMES"
done

if [ -f "templates/write-matrix.json" ] && [ -d ".arcforge" ]; then
    if [ ! -f ".arcforge/write-matrix.json" ]; then
        drift ".arcforge/write-matrix.json 缺失"
    else
        jq -S . templates/write-matrix.json >/dev/null 2>&1 || { echo "ERROR: 模板矩阵非法 JSON" >&2; exit 1; }
        jq -S . .arcforge/write-matrix.json >/dev/null 2>&1 || { echo "ERROR: 运行时矩阵非法 JSON" >&2; exit 1; }

        # **合法 JSON ≠ 可比对**:顶层必须是对象。null / 标量 / 数组上 `keys` 会报错,
        # 该错误一旦被吞掉,取键结果为空 ⇒ 下面的循环零次迭代 ⇒ RC 保持 0 ⇒ 输出 SYNC OK
        # 且 exit 0,即「因新增过滤逻辑退化成静默放行」——归档硬门禁被整个绕过。
        # 这不是臆造输入:仓库到处用 `jq <filter> f > tmp && mv`,filter 写错一个路径就产出
        # 字面 null,而人类同步矩阵用的正是这个模式。必须置于一切过滤逻辑之前。
        for MF in "templates/write-matrix.json" ".arcforge/write-matrix.json"; do
            MT=$(jq -r 'type' "$MF")
            [ "$MT" = "object" ] || { echo "ERROR: $MF 顶层不是 JSON 对象(实为 ${MT:-未知}),无法做字段比对" >&2; exit 1; }
        done

        # 运行时专有字段在模板侧必须为空;非空 = 运行时状态被写进了模板(反向污染),
        # 文案与普通字段漂移刻意不同,免得又变成分辨不出真假的一行。
        for F in $RUNTIME_ONLY_FIELDS; do
            # 不吞 jq 的错误、也不给 N 兜底默认值:兜底 = 把读取失败悄悄读成「没有污染」,
            # 与上面刚堵掉的静默放行同一族(返工教训)。
            N=$(jq --arg f "$F" '(.[$f] // {}) | length' templates/write-matrix.json) \
                || { echo "ERROR: templates/write-matrix.json 读取字段 $F 失败" >&2; exit 1; }
            # ${F} 的花括号不可省:紧跟其后的「」是多字节字符,bash 会把它并进变量名(实撞)
            [ "$N" -eq 0 ] || drift "templates/write-matrix.json 顶层字段「${F}」非空(该字段仅属运行时,模板疑似被运行时状态污染)"
        done

        # 逐顶层字段比对(排除运行时专有字段):文案指名字段,人能一眼分辨真假漂移。
        # 取两侧键的并集 ⇒ 任一侧独有的**未知**字段也会被报出(黑名单外无静默跳过)。
        # 刻意**不加** 2>/dev/null:取键失败必须显形并中止。8f548ac 的回归就是这个重定向
        # 把非对象输入上 `keys` 的报错吞掉,取键为空 ⇒ 零字段可比 ⇒ 静默 SYNC OK。
        # 黑名单就地 split(空串 ⇒ 空数组),不经中间变量:少一条**没有失败检查**的命令,
        # 与本轮「不留静默失败路径」的口径一致。
        KEYS=$(jq -rs --arg ex "$RUNTIME_ONLY_FIELDS" \
            '(((.[0]|keys) + (.[1]|keys)) | unique) - ($ex | split(" ")) | .[]' \
            templates/write-matrix.json .arcforge/write-matrix.json) \
            || { echo "ERROR: 矩阵顶层键提取失败,无法完成字段比对" >&2; exit 1; }
        # here-doc 喂 while(不是管道):drift 的 RC=1 必须落在当前 shell,不能进子 shell
        while IFS= read -r K; do
            [ -n "$K" ] || continue
            # 同样不留静默失败路径:jq 失败会让两侧都成空串 ⇒ 相等 ⇒ 判成「无漂移」,
            # 与被打回的那个回归同一族。上面的类型断言已让这条路不可达,补检查是纵深防御
            # (文件在两次读之间被改写时仍可能触发)。
            TV=$(jq -S --arg k "$K" '.[$k]' templates/write-matrix.json) \
                && RV=$(jq -S --arg k "$K" '.[$k]' .arcforge/write-matrix.json) \
                || { echo "ERROR: 读取顶层字段 $K 失败,无法完成字段比对" >&2; exit 1; }
            [ "$TV" = "$RV" ] || drift ".arcforge/write-matrix.json 顶层字段「${K}」与模板不一致"
            CHECKED_FIELDS=$((CHECKED_FIELDS + 1))
        done <<EOF
$KEYS
EOF
    fi
fi

# ---- 指令资产:CLAUDE.md 的段落级存在性检查 ----
#
# 为什么单独一段而不是并进上面的 cmp 循环:运行时 CLAUDE.md 不是模板的副本,而是
# 「模板内容**追加**到项目原有 CLAUDE.md 之后」生成的,逐字节比对必然失败。上面的
# 校验范围因此一直把它排除在外——而它恰恰是 agent 行为的唯一实际来源。
#
# 实测后果(atlas sprint-032):运行时 CLAUDE.md 整节缺失「派发后活性确认」,
# `stale-dispatch` 零命中,Leader 完全不知道该机制存在,撞上故障后重新分析一遍
# 并给出了已被否决的方案。而归档照常打印 SYNC OK —— 因为这个文件不在校验范围里。
#
# ⚠ **能力边界(必须如实声明,不得夸大)**:本检查只比对**二至四级标题是否存在**。
#   它挡得住:任一层级的整节丢失。
#   它挡不住:①标题保留但正文被改写或删空;②标题被改名(会报成缺失,至少会响);
#            ③两侧同名节的内容实质冲突。
#   声称超出这个边界的能力比不检查更糟——读者会以为 CLAUDE.md 已被完整比对,
#   从而不再人工核对。
#
# 范围为何是 2-4 而非 3-4:初版按「本次故障的形态」(一个 #### 节丢失)定范围,
# 结果 atlas 同步时实测另有 3 个 ## 级整节缺失(每 dev 独立 worktree / 变异测试纪律 /
# 矩阵字段登记规则),初版一条都报不出。**按已发生故障的形态划范围,会正好漏掉
# 尚未发生的那些。** 一级标题(#)不纳入:它是文档标题,运行时侧本就不会同名。
CLAUDE_TPL="templates/CLAUDE.md.template"
CHECKED_SECTIONS=0
if [ -f "$CLAUDE_TPL" ]; then
    APPLICABLE=1
    if [ -f "CLAUDE.md" ]; then
        # grep -Fx:标题含反引号/括号/中文,必须固定字符串整行匹配,不能当正则。
        while IFS= read -r H; do
            [ -n "$H" ] || continue
            CHECKED_SECTIONS=$((CHECKED_SECTIONS + 1))
            grep -Fxq "$H" "CLAUDE.md" || \
                drift "运行时 CLAUDE.md 缺失模板中的章节: $H"
        done <<EOF
$(grep -E '^#{2,4} ' "$CLAUDE_TPL")
EOF
    else
        drift "存在 $CLAUDE_TPL 但运行时根目录无 CLAUDE.md"
    fi
fi

# ---- 结论:先报漂移,再区分「没得验」与「不该验」,最后才允许说零漂移 ----------------
#
# 本行必须在**全部**检查之后。它曾经位于 CLAUDE.md 段落检查之前,后果是:只要
# hooks/scripts 侧存在任何漂移,段落检查就整段被跳过——而输出与"章节全对"时
# 完全一致。这正是本次要治的病(沉默的排除)在本脚本自身的复发,靠验收测试才发现。
[ "$RC" -eq 0 ] || exit 1

if [ "$APPLICABLE" -eq 0 ]; then
    echo "WARN: 未发现模板侧(project-template/ 与 templates/write-matrix.json 均不存在)," >&2
    echo "      本仓库对模板↔运行时同步校验**不适用**(已安装 Arcforge 的消费项目即此形态)。" >&2
    exit 3
fi
if [ "$CHECKED_FILES" -eq 0 ] && [ "$CHECKED_FIELDS" -eq 0 ] && [ "$CHECKED_SECTIONS" -eq 0 ]; then
    echo "WARN: 模板侧存在,却一项都没比对成(0 个文件 / 0 个矩阵字段 / 0 个章节),校验**无从进行**。" >&2
    echo "      处置:检查 project-template/ 是否为空、.arcforge/ 是否缺失;这不是零漂移。" >&2
    exit 2
fi
# 比对量随绿一起打印:一句没有数字的「零漂移」无法与「零比对」区分,而这正是本次要根治的病。
# ${} 花括号不可省:紧随其后的「个」是多字节字符,bash 会把它并进变量名(TASK-016 实撞)。
echo "SYNC OK: 已比对 ${CHECKED_FILES} 个文件 + ${CHECKED_FIELDS} 个矩阵字段 + ${CHECKED_SECTIONS} 个 CLAUDE.md 章节(仅二至四级标题存在性),模板与运行时零漂移"
exit 0
