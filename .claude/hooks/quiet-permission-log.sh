#!/bin/bash
# quiet-permission-log.sh — PermissionRequest 钩子:静默期内把实际弹出的权限提示记一行。
#
# **只记录,永不输出决策。** 不输出 JSON 且 exit 0 = 正常的权限提示照常出现(官方 hooks 文档)。
# 本期不让决策模型批准权限请求:硬禁清单需要真实的提示样本才定得下来,这份记录就是样本;
# 它同时回答「本钩子是否对 teammate 的权限请求触发、在哪个目录触发」。
#
# 它在权限流程的关键路径上,所以任何失败都静默:没有 python3、脚本缺失、配置损坏,一律 exit 0。
# CDPATH 置空:本钩子以相对路径注册,导出了 CDPATH 的环境里 cd 会先去那里找同名目录,
# 找到了还会把路径打到 stdout——DIR 于是指向别处或变成两行,记录静默失效。
DIR="$(CDPATH= cd "$(dirname "$0")" 2>/dev/null && pwd)" || exit 0
PY="$DIR/quiet/quiet_permission_log.py"
[ -f "$PY" ] || exit 0
command -v python3 >/dev/null 2>&1 || exit 0
python3 -B "$PY" >/dev/null 2>&1
exit 0
