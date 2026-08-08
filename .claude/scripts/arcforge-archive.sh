#!/bin/bash
# arcforge-archive.sh — Sprint 归档:把本轮运行时产物移入 .arcforge/archive/,
# 重置运行时目录供下一 Sprint。交付完成后由 Leader 经 /arcforge-archive 执行。
# 用法: arcforge-archive.sh [--force] [--dry-run]
# exit 0 = 归档完成/dry-run;exit 1 = 校验不通过或 mv 失败
#
# 兼容 macOS 自带 bash 3.2:累加用字符串,不用数组(空数组 + set -u 在 bash<4.4 会炸)。
# 刻意不用 -e:与「[ 条件 ] && 赋值」循环模式及 find 管道语义冲突,失败路径均显式处理。
set -uo pipefail

ARC=".arcforge"
TASK_DIR="$ARC/tasks"
ARCHIVE_DIR="$ARC/archive"
FORCE=0
DRY_RUN=0

for arg in "$@"; do
  case "$arg" in
    --force)   FORCE=1 ;;
    --dry-run) DRY_RUN=1 ;;
    *) echo "未知参数: $arg(支持 --force / --dry-run)" >&2; exit 1 ;;
  esac
done

# jq 缺失会让终态校验静默放空(find -exec jq 2>/dev/null 输出为空),必须前置检查
command -v jq >/dev/null 2>&1 || { echo "ERROR: 本脚本依赖 jq(终态校验),请先安装。" >&2; exit 1; }

# ---- 0. 前置校验:模板↔运行时漂移禁止归档(backlog-sprint-c #1,--force 不豁免) ----
# 漂移归档会固化「运行时跑的不是入库版本」的不一致快照。同目录 check-runtime-sync.sh
# 缺失(如脚本被单独分发)则跳过本门禁,不阻断归档。
#
# TASK-017:改按**退出码分派**,不再一句 `check || ABORT`。原写法把 check 的 exit 0
# 当作「验证通过」,而 exit 0 在无运行时布局时表示「根本没比对」——门禁于是在零验证下
# 完整放行(本任务 RED 实测:--force --dry-run exit 0 且无 ABORT)。绿必须能区分
# 「验了且全过」与「没得验」,否则硬门禁就是摆设。
SYNC_CHECK="$(dirname "$0")/check-runtime-sync.sh"
if [ -f "$SYNC_CHECK" ]; then
    bash "$SYNC_CHECK"; SYNC_RC=$?
    case "$SYNC_RC" in
        0) : ;;   # 已实际比对 N>0 项且零漂移 —— 唯一的放行通道
        3) # 不适用:本仓库没有模板侧(消费项目的正常形态)。放行,但把「未经校验」说出口,
           # 不再靠一句 SYNC OK 冒充验证通过。
           echo "WARN: 模板↔运行时同步校验不适用(本仓库无模板侧),本次归档未经漂移校验。" >&2 ;;
        2) echo "ABORT: 模板↔运行时同步校验无从进行(无运行时布局或零比对项),归档不建立在「没验过」之上。" >&2
           echo "       处置:确认是否在仓库根执行、运行时布局是否已安装。这不是检出差异,别去同步文件。" >&2
           exit 1 ;;
        *) echo "ABORT: 模板↔运行时存在漂移,先完成人类同步再归档(--force 不豁免)。" >&2
           exit 1 ;;
    esac
else
    # 缺件与 rc=3 同档:都是「没得验」。放行,但把它说出口——
    # W4 的教训是静默,不是放行(零输出的 exit 0 读起来和「验过且全过」一模一样)。
    echo "WARN: $SYNC_CHECK 缺失,本次归档未经模板↔运行时漂移校验。" >&2
fi

# ---- 1. 前置校验:tasks 缺失或无任务文件 = 无可归档内容(防误调/重复调用) ----
TASK_COUNT=$(find "$TASK_DIR" -name '*.json' 2>/dev/null | wc -l | tr -d ' ')
if [ "$TASK_COUNT" -eq 0 ]; then
    if [ "$FORCE" -ne 1 ]; then
        echo "BLOCKED: $TASK_DIR 缺失或没有任何任务文件,无可归档内容。" >&2
        echo "确认要归档空 Sprint 请加 --force。" >&2
        exit 1
    fi
    echo "WARN: 无任务文件,--force 强制归档(将产生空 sprint 目录)。" >&2
fi

# ---- 2. 终态校验:所有任务必须 accepted/skipped ----
OPEN=$(find "$TASK_DIR" -name '*.json' -exec jq -r \
    'select(.status != "accepted" and .status != "skipped") | "\(.id // "?")  \(.status // "?")"' {} \; 2>/dev/null)
if [ -n "$OPEN" ]; then
    if [ "$FORCE" -ne 1 ]; then
        echo "BLOCKED: 存在未走到终态(accepted/skipped)的任务,拒绝归档:" >&2
        echo "$OPEN" >&2
        echo "补完上述任务,或人工确认后用 --force(未完成任务随档入库,不丢数据)。" >&2
        exit 1
    fi
    echo "WARN: --force 跨越终态校验,以下未完成任务随档入库:" >&2
    echo "$OPEN" >&2
fi

# ---- 3. 编号:只认 sprint-NNN-* 形态,取最大 NNN+1;不合规目录名忽略。
#         archive/ 缺失无需预建:glob 无匹配 → 从 001 起;目录由后面的
#         mkdir -p "$DEST" 隐式创建,保证 --dry-run 真正零变更。----
MAX=0
for d in "$ARCHIVE_DIR"/sprint-[0-9][0-9][0-9]-*; do
    [ -d "$d" ] || continue            # 无匹配时 glob 字面量落入循环,由此跳过
    n=$(basename "$d" | cut -d- -f2)
    n=$((10#$n))                        # 强制十进制:008/009 按八进制解析会报错
    [ "$n" -gt "$MAX" ] && MAX=$n
done
NNN=$(printf '%03d' $((MAX + 1)))
DEST="$ARCHIVE_DIR/sprint-$NNN-$(date +%F)"

# ---- 4. 归档候选:逐目录 skip-if-missing;wisdom 保留原地 ----
CANDIDATES="docs tasks discoveries checkpoints coverage"

if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 目标: $DEST"
    for d in $CANDIDATES; do
        if [ -d "$ARC/$d" ]; then echo "[dry-run] mv $ARC/$d -> $DEST/$d"
        else echo "[dry-run] skip $ARC/$d(不存在)"; fi
    done
    echo "[dry-run] 保留原地: $ARC/wisdom"
    echo "[dry-run] 重置: tasks discoveries checkpoints coverage docs/{01..07}"
    exit 0
fi

mkdir -p "$DEST"
MOVED=""
SKIPPED=""
for d in $CANDIDATES; do
    if [ -d "$ARC/$d" ]; then
        if ! mv "$ARC/$d" "$DEST/$d"; then
            echo "ERROR: mv $ARC/$d 失败。已归档:${MOVED:-(无)};该目录及之后未处理。" >&2
            echo "       请人工检查 $DEST 后处理,不要直接重复执行。" >&2
            exit 1
        fi
        MOVED="$MOVED $d"
    else
        SKIPPED="$SKIPPED $d"
    fi
done

# ---- 5. 重置运行时目录(全集,不依赖归档前存在与否) ----
mkdir -p "$ARC/tasks" "$ARC/discoveries" "$ARC/checkpoints" "$ARC/coverage" \
         "$ARC/docs/01-design" "$ARC/docs/02-plan" "$ARC/docs/03-progress" \
         "$ARC/docs/04-test" "$ARC/docs/05-review" "$ARC/docs/06-acceptance" \
         "$ARC/docs/07-deploy"

echo "=== Sprint 归档完成 ==="
echo "归档位置: $DEST"
echo "已迁移:${MOVED:-(无)}"
[ -n "$SKIPPED" ] && echo "跳过(不存在):$SKIPPED"
echo "保留原地: $ARC/wisdom"
echo "运行时目录已重置,可开始下一 Sprint。"

# ---- 6. 收口 tag 提示:只给命令,不代打 ----
#         被 tag 的对象是「把归档产物入库的那个 commit」,而它在此刻尚不存在(产物刚落盘、
#         还没提交)。脚本若自己 git tag,只能打在上一个 commit 上 —— 那棵树里没有归档目录,
#         tag 指向的内容与 tag 名描述的东西对不上。故与「待同步 hooks 清单」同构:
#         脚本产出确定性命令,由 Leader/人类在正确时点执行。
SPRINT_TAG=$(basename "$DEST")
echo
echo "=== Sprint 收口 tag(在归档提交之后执行)==="
echo "  1) 提交归档产物"
echo "  2) git tag -a ${SPRINT_TAG} -F <message 文件>"
echo "  3) git push origin ${SPRINT_TAG}"
echo "  tag 名与归档目录同名;message 须如实反映任务终态,"
echo "  不得写与 tasks/*.json 不符的 accepted 计数(--force 归档时尤其注意)。"
exit 0
