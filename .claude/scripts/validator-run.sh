#!/bin/bash
# validator-run.sh — validator CLI 统一解析链(单一真相源,Sprint E 修订①)。
# 用法: validator-run.sh <validate|progress> [args...]
# 解析顺序(首个可用者胜出):
#   1. $HOME/.arcforge/bin/arcforge-<tool> 存在且可执行 → 直接执行;
#      业务退出码(0/1/2)原样透传;执行层失败(rc ≥ 126,如架构不符)降级下一级。
#   2. 仓库内存在 ./validator/ 且有 Go → go build 到临时文件再执行(dogfooding,依赖根 go.work)。
#      不用 go run:go run 会把子程序的非零退出码一律塌缩为 1,破坏 0/1/2 透传契约;
#      先 build 再直接执行则退出码原样透传,与第 1 级二进制行为一致。
#   3. 均不可用 → exit 127 + 如实文案。
# CWD 恒为调用时目录(须从仓库根调用,discovery 相对路径语义天然正确)。
# ⚠️ 解析逻辑只此一处;各命令文档仅一行引用,禁止复述(防三处漂移,Sprint C 教训)。
set -uo pipefail

TOOL="${1:?用法: validator-run.sh <validate|progress> [args...]}"
shift
case "$TOOL" in
  validate|progress) : ;;
  *) echo "错误: 未知工具「${TOOL}」(可选 validate|progress)" >&2; exit 64 ;;
esac

BIN="$HOME/.arcforge/bin/arcforge-$TOOL"
if [ -x "$BIN" ]; then
  "$BIN" "$@"
  RC=$?
  if [ "$RC" -lt 126 ]; then
    exit "$RC"
  fi
  echo "WARN: $BIN 执行失败(rc=$RC,可能架构不符/二进制损坏),降级源码运行" >&2
fi

if [ -d "./validator" ] && command -v go >/dev/null 2>&1; then
  BUILD=$(mktemp "${TMPDIR:-/tmp}/arcforge-$TOOL.XXXXXX") || { echo "错误: mktemp 失败" >&2; exit 1; }
  trap 'rm -f "$BUILD"' EXIT
  if go build -o "$BUILD" "./validator/cmd/arcforge-$TOOL"; then
    "$BUILD" "$@"
    exit $?   # 业务退出码 0/1/2 原样透传(不经 go run 塌缩)
  fi
  echo "WARN: go build 失败,源码不可用,降级为未分发" >&2
fi

echo "validator 未分发(安装时无 Go 工具链或未重跑 install.sh),请回退手工统计" >&2
exit 127
