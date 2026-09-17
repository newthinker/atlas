#!/bin/bash
# hestia-warp-trigger.sh —— 有活才唤起 Warp 解读 agent。
#
# 🔴 **先判队列再唤起，不照抄 hestia-ingest 的「无条件跑、空跑靠幂等」。**
# ingest 的空跑是一次 HTTP 请求；唤起 agent 的空跑要**烧 token**，而一个月只有
# 1–2 天真有新契约 ⇒ 无条件跑等于绝大多数唤起是纯浪费。
#
# 互斥用 processing/ 非空判定，不引锁文件：文件系统状态本来就是这套队列的真相源，
# 没有锁文件也就没有锁文件泄漏。
# ⚠️ 代价：agent 卡死会让 processing/ 永久非空、本脚本从此静默跳过。那由告警规则
#    hestia_queue_processing_stuck 兜（processing 滞留 > 30 分钟）。**别在这里加超时
#    自动清理**——脚本自己把卡住的契约移回 pending/ 会让同一份反复重试，而真正的
#    成因（agent 为什么死）永远不会被看见。
#
# 计件判据与 internal/hestia 的 QueueHealthOf 一致：只数普通文件，子目录、以 `.` 开头
# 的文件（如 .DS_Store）与以 `.tmp` 结尾的文件（writeAtomic 的中间态）都不算——否则
# 一个 .DS_Store 就会让本脚本每轮空唤起一次 agent。
set -euo pipefail

# 默认值 = serve plist 的 WorkingDirectory + configs/hestia.yaml 的 queue.dir。
QUEUE_DIR="${HESTIA_QUEUE_DIR:-/Users/zuowei/workspace/runtime/atlas/queue/hestia}"
NANOCLAW_DIR="${NANOCLAW_DIR:-/Users/zuowei/workspace/ai/nanoclaw}"
# TRIGGER_CMD 存在是为了让自测能换成桩。生产不设它，走下面的默认。
TRIGGER_CMD="${TRIGGER_CMD:-}"

count_of() { # count_of <state> —— 目录读不到就非零退出
  local d="$QUEUE_DIR/$1"
  if [ ! -d "$d" ]; then
    echo "hestia-warp: 队列目录不存在: ${d}" >&2
    exit 1
  fi
  find "$d" -mindepth 1 -maxdepth 1 -type f ! -name '.*' ! -name '*.tmp' | wc -l | tr -d ' '
}

pending=$(count_of pending)
processing=$(count_of processing)

if [ "$pending" -eq 0 ]; then
  echo "hestia-warp: queue empty, nothing to do (${QUEUE_DIR})"
  exit 0
fi
if [ "$processing" -ne 0 ]; then
  echo "hestia-warp: busy (${processing} in processing/), skipping this round (${QUEUE_DIR})"
  exit 0
fi

echo "hestia-warp: triggering (${pending} pending in ${QUEUE_DIR})"
if [ -n "$TRIGGER_CMD" ]; then
  eval "$TRIGGER_CMD"
else
  # ⚠️ 客户端 120s 硬超时而 agent 常跑更久 ⇒ 退出码**不代表**处理结果。
  #    真实结果看队列流转与 vault，由告警规则盯着，不在这里判。
  cd "$NANOCLAW_DIR" && pnpm run chat 处理 hestia 队列 || true
fi
