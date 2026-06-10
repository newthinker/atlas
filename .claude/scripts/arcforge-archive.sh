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
exit 0
