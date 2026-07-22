#!/bin/bash
# check-runtime-sync.sh — 模板↔运行时零漂移机械化校验(backlog-sprint-c #1,TASK-C3)。
# exit 0 = 零漂移(或无运行时布局);exit 1 = 漂移/缺失/非法 JSON,stderr 指名文件。
# 供 arcforge-archive.sh 前置门禁与人类收尾核验调用。
# 校验范围:project-template/hooks ↔ .claude/hooks、project-template/scripts ↔
# .claude/scripts(逐字节 cmp)、templates/write-matrix.json ↔ .arcforge/write-matrix.json
# (jq -S 语义,键序无关)。settings 漂移由 test-settings-sync.sh 独立覆盖,不在此。
# bash 3.2 兼容:不用数组;失败路径显式处理。
set -uo pipefail
RC=0
drift() { echo "DRIFT: $1" >&2; RC=1; }

if [ ! -d ".claude" ] && [ ! -d ".arcforge" ]; then
    echo "WARN: 无运行时布局(.claude/.arcforge),跳过同步校验。" >&2
    exit 0
fi
command -v jq >/dev/null 2>&1 || { echo "ERROR: 需要 jq" >&2; exit 1; }

for PAIR in "project-template/hooks:.claude/hooks" "project-template/scripts:.claude/scripts"; do
    SRC="${PAIR%%:*}"; DST="${PAIR#*:}"
    [ -d "$SRC" ] || continue
    for F in "$SRC"/*.sh; do
        [ -e "$F" ] || continue
        B="$(basename "$F")"
        if [ ! -f "$DST/$B" ]; then drift "$DST/$B 缺失(模板側存在)"
        elif ! cmp -s "$F" "$DST/$B"; then drift "$DST/$B 与 $F 字节不一致"
        fi
    done
done

if [ -f "templates/write-matrix.json" ] && [ -d ".arcforge" ]; then
    if [ ! -f ".arcforge/write-matrix.json" ]; then
        drift ".arcforge/write-matrix.json 缺失"
    else
        T=$(jq -S . templates/write-matrix.json 2>/dev/null) || { echo "ERROR: 模板矩阵非法 JSON" >&2; exit 1; }
        R=$(jq -S . .arcforge/write-matrix.json 2>/dev/null) || { echo "ERROR: 运行时矩阵非法 JSON" >&2; exit 1; }
        [ "$T" = "$R" ] || drift ".arcforge/write-matrix.json 与模板语义不一致"
    fi
fi

[ "$RC" -eq 0 ] && echo "SYNC OK: 模板与运行时零漂移"
exit "$RC"
