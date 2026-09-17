#!/bin/bash
# hestia-warp-trigger.sh 的自测。造各种队列形态，断言脚本的退出码、输出与是否唤起 agent。
#
# 唤起动作被替换成一个**记录调用的桩**（TRIGGER_CMD 环境变量；默认唤起路径则用
# PATH 里的假 pnpm），所以本测试不碰真的 nanoclaw、不烧 token。
#
# 用法：bash scripts/ops/hestia-warp-trigger-test.sh   末行输出 `N passed, M failed`。
set -u
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/hestia-warp-trigger.sh"
DEFAULT_QUEUE_DIR=/Users/zuowei/workspace/runtime/atlas/queue/hestia
pass=0; fail=0
TMPROOT=$(mktemp -d); trap 'rm -rf "$TMPROOT"' EXIT

setup() { # 每例一个全新队列与桩记录文件
  Q=$(mktemp -d "$TMPROOT/q.XXXXXX"); mkdir -p "$Q"/{pending,processing,done,failed}
  CALLED=$(mktemp "$TMPROOT/called.XXXXXX")
  export HESTIA_QUEUE_DIR="$Q"
  export TRIGGER_CMD="echo called >> $CALLED"
}

run() { # 跑被测脚本，分别收 stdout / stderr / 退出码
  "$SCRIPT" >"$TMPROOT/out" 2>"$TMPROOT/err"; rc=$?
  out=$(cat "$TMPROOT/out"); err=$(cat "$TMPROOT/err")
  calls=$(wc -l <"$CALLED" | tr -d ' ')
}

# expect <名称> <期望退出码: 0|nonzero> <stdout 须含(空=不查)> <stderr 须含(空=不查)> <期望调用次数>
expect() {
  local name=$1 want_rc=$2 want_out=$3 want_err=$4 want_calls=$5 ok=yes
  if [ "$want_rc" = 0 ]; then [ "$rc" -eq 0 ] || ok=no; else [ "$rc" -ne 0 ] || ok=no; fi
  [ -z "$want_out" ] || [[ "$out" == *"$want_out"* ]] || ok=no
  [ -z "$want_err" ] || [[ "$err" == *"$want_err"* ]] || ok=no
  [ "$calls" = "$want_calls" ] || ok=no
  if [ "$ok" = yes ]; then
    echo "  PASS ${name}"; pass=$((pass+1))
  else
    echo "  FAIL ${name}：rc=${rc}（期望 ${want_rc}） calls=${calls}（期望 ${want_calls}）"
    echo "       stdout: ${out}"; echo "       stderr: ${err}"; fail=$((fail+1))
  fi
}

echo "== 四目录全空 ⇒ 不唤起（C4）=="
setup; run; expect "空队列" 0 "queue empty" "" 0

echo "== pending 1 个普通文件、processing 空 ⇒ 恰唤起一次 =="
setup; touch "$Q/pending/y.json"; run; expect "有活" 0 "triggering" "" 1

echo "== pending 与 processing 均非空 ⇒ busy、不唤起（C5）=="
setup; touch "$Q/pending/y.json" "$Q/processing/x.json"; run; expect "agent 在跑" 0 "busy" "" 0

echo "== pending 空而 processing 非空 ⇒ 不唤起 =="
setup; touch "$Q/processing/x.json"; run; expect "只有 processing" 0 "" "" 0

echo "== pending 只有子目录 / 点文件 / .tmp ⇒ 视为空、不唤起 =="
setup; mkdir "$Q/pending/tmp"; touch "$Q/pending/.DS_Store" "$Q/pending/a.json.tmp"
run; expect "非计件条目" 0 "queue empty" "" 0

echo "== pending 缺失 ⇒ 非零退出、stderr 含路径、不唤起 =="
setup; rm -rf "$Q/pending"; touch "$Q/processing/x.json"
run; expect "pending 缺失" nonzero "" "$Q/pending" 0

echo "== processing 缺失 ⇒ 非零退出、stderr 含路径、不唤起 =="
setup; rm -rf "$Q/processing"; touch "$Q/pending/y.json"
run; expect "processing 缺失" nonzero "" "$Q/processing" 0

echo "== 不设 HESTIA_QUEUE_DIR ⇒ 用默认队列目录（只读判定；桩兜底不唤起真 agent）=="
setup; unset HESTIA_QUEUE_DIR; run
if [[ "$out$err" == *"$DEFAULT_QUEUE_DIR"* ]] && [ "$calls" -le 1 ]; then
  echo "  PASS 默认队列目录"; pass=$((pass+1))
else
  echo "  FAIL 默认队列目录：输出未含 ${DEFAULT_QUEUE_DIR}"; echo "       stdout: ${out}"; echo "       stderr: ${err}"; fail=$((fail+1))
fi

echo "== 不设 TRIGGER_CMD ⇒ 在 NANOCLAW_DIR 下跑 pnpm run chat，失败也 exit 0 =="
setup; unset TRIGGER_CMD; touch "$Q/pending/y.json"
FAKEBIN=$(mktemp -d "$TMPROOT/bin.XXXXXX"); NC=$(mktemp -d "$TMPROOT/nanoclaw.XXXXXX")
NC=$(cd "$NC" && pwd -P)
cat >"$FAKEBIN/pnpm" <<STUB
#!/bin/bash
echo "\$(pwd -P) \$*" >> "$CALLED"
exit 7
STUB
chmod +x "$FAKEBIN/pnpm"
PATH="$FAKEBIN:$PATH" NANOCLAW_DIR="$NC" run
if [ "$rc" -eq 0 ] && [ "$calls" = 1 ] && grep -q "^$NC run chat" "$CALLED"; then
  echo "  PASS 默认唤起路径"; pass=$((pass+1))
else
  echo "  FAIL 默认唤起路径：rc=${rc} calls=${calls} 记录=$(cat "$CALLED")"; echo "       stdout: ${out}"; echo "       stderr: ${err}"; fail=$((fail+1))
fi

echo; echo "${pass} passed, ${fail} failed"; [ "$fail" -eq 0 ]
