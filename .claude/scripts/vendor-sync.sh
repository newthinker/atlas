#!/bin/bash
# vendor-sync.sh — 第三方 prompt 资产 vendor+pin(F10①)。
# 用法: vendor-sync.sh --check          # 校验 manifest 与 vendor 目录一致
# manifest.json: [{repo, sha, license, files:[{src,dst}]}]
# 资产快照由人类 review 后提交入库;本脚本只做一致性校验(供 CI/验收调用)。
#
# 双向校验:
#   正向 — manifest 声明的每个 dst 必须存在,且 repo/sha/license 齐全;
#   反向 — vendor/ 下每个实际文件必须被 manifest 声明(供应链漂移检出,W2),
#          manifest.json 与 README.md 自身除外。
set -uo pipefail
MANIFEST="vendor/manifest.json"

[ "${1:-}" = "--check" ] || { echo "用法: vendor-sync.sh --check" >&2; exit 1; }
if [ ! -f "$MANIFEST" ]; then echo "无 vendor 条目(vendor/ 未启用)"; exit 0; fi

jq -e . "$MANIFEST" >/dev/null 2>&1 || { echo "FAIL: $MANIFEST 非法 JSON" >&2; exit 1; }

RC=0
COUNT=$(jq 'length' "$MANIFEST")

# 正向:字段齐全 + 声明文件存在
i=0
while [ "$i" -lt "$COUNT" ]; do
    ENTRY=$(jq -c ".[$i]" "$MANIFEST")
    for KEY in repo sha license; do
        [ "$(echo "$ENTRY" | jq -r ".$KEY // empty")" ] || \
            { echo "FAIL: 条目 $i 缺 $KEY" >&2; RC=1; }
    done
    while read -r DST; do
        # C5(backlog-c #5):dst 限域——存在性检查之前先约束落点,防 manifest 把文件写/校验到
        # vendor/ 之外(绝对路径由 vendor/ 前缀约束自然拒绝)。
        case "$DST" in
            vendor/*) : ;;
            *) echo "FAIL: 条目 $i 的 dst「${DST}」不在 vendor/ 下" >&2; RC=1; continue ;;
        esac
        case "$DST" in
            *..*) echo "FAIL: 条目 $i 的 dst「${DST}」含 ..(路径穿越)" >&2; RC=1; continue ;;
        esac
        [ -f "$DST" ] || { echo "FAIL: 条目 $i 声明的 $DST 不存在" >&2; RC=1; }
    done < <(echo "$ENTRY" | jq -r '.files[]?.dst')
    i=$((i+1))
done

# 反向:vendor/ 下每个文件必须被声明(manifest.json/README.md 自身除外)
DECLARED=$(jq -r '.[].files[]?.dst' "$MANIFEST" 2>/dev/null)
while IFS= read -r ACTUAL; do
    case "$ACTUAL" in
        vendor/manifest.json|vendor/README.md) continue ;;
    esac
    printf '%s\n' "$DECLARED" | grep -qxF "$ACTUAL" || \
        { echo "FAIL: 未声明的多余文件 $ACTUAL(供应链漂移)" >&2; RC=1; }
done < <(find vendor -type f 2>/dev/null)

[ "$RC" -eq 0 ] && echo "PASS: vendor 与 manifest 一致($COUNT 条目)"
exit "$RC"
