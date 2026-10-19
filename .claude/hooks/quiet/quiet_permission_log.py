"""
quiet_permission_log — 静默期内记录实际弹出的权限提示。本期只记录:不问决策模型、不出境、不输出决策。

由 quiet-permission-log.sh 调用,hook 输入走 stdin。它在权限流程的关键路径上:
任何失败都必须静默,且永不向 stdout 输出内容(不输出 = 正常提示照常出现)。
"""
from __future__ import annotations

import sys

sys.dont_write_bytecode = True

import json  # noqa: E402
import os  # noqa: E402
import time  # noqa: E402
from pathlib import Path  # noqa: E402
from typing import Any, Dict, Optional  # noqa: E402

_HERE = str(Path(__file__).resolve().parent)
if _HERE not in sys.path:
    sys.path.insert(0, _HERE)
import quiet_common as qc  # noqa: E402

LOG = "quiet-permissions"
# 各字段的字节配额。合计须给 480 字节的行长上限留出余量(其余字段与 JSON 标点约 115 字节)。
TOOL, HEAD, SUBJECT, CWD, KEYS = 24, 24, 140, 80, 60
SUBJECT_KEYS = ("command", "file_path", "path", "pattern", "url")


def _clip(text: str, limit: int, tail: bool = False) -> str:
    """整字符取舍,截到落盘后不超过 limit 字节(tail 为真时留末尾)。

    量的是**写进日志行的**字节数:引号与反斜杠落盘占 2 字节、控制字符占 6 字节,只量原文的话,
    一条全是引号的命令会把那一行撑过上限,而超限的行是整行丢弃的。
    孤立代理(JSON 里写成 U+D800 一类的转义)编不成 UTF-8,先换成 "?",否则同样整行记不下来。
    """
    text = text.encode("utf-8", errors="replace").decode("utf-8")
    kept, used = [], 0
    for ch in (reversed(text) if tail else text):
        used += len(json.dumps(ch, ensure_ascii=False).encode("utf-8")) - 2
        if used > limit:
            break
        kept.append(ch)
    return "".join(reversed(kept) if tail else kept)


def record(data: Dict[str, Any], cwd: Optional[Path] = None) -> None:
    here = Path(cwd or os.getcwd()).resolve()
    root = qc.main_root(here)
    st = qc.load(root)
    if st is None or not st.enabled:
        return
    night = qc.window(st, qc.now()[0])
    if night is None:
        return
    tool_input = data.get("tool_input")
    if not isinstance(tool_input, dict):
        tool_input = {}
    # 脱敏只做一遍,且先于截断与取首词:截断在前会留下认不出的半截密钥;
    # 首词不脱敏的话,`API_KEY=… make` 这种写法的首词就是密钥本身。
    subject = qc.redact(next((tool_input[k] for k in SUBJECT_KEYS if isinstance(tool_input.get(k), str)), ""))
    # command 排在 SUBJECT_KEYS 第一位:它是字符串时 subject 就是(脱敏后的)整条命令。
    words = subject.split() if isinstance(tool_input.get("command"), str) else []
    project_dir = os.environ.get("CLAUDE_PROJECT_DIR")
    qc.append(root, LOG, {
        "ts": round(time.time(), 1), "night": night,
        "tool": _clip(str(data.get("tool_name") or ""), TOOL),
        "head": _clip(os.path.basename(words[0]), HEAD) if words else "",
        "subject": _clip(subject, SUBJECT),
        "cwd": _clip(str(here), CWD, tail=True),       # 取末尾:能区分 worktree 的是路径的最后一段
        "at_root": here == root,
        "pd_root": (Path(project_dir).resolve() == root) if project_dir else None,
        "keys": _clip(",".join(sorted(k for k in data if k != "tool_input")), KEYS),
    })


def main() -> int:
    try:
        data = json.load(sys.stdin)
        if isinstance(data, dict):
            record(data)
    except Exception:  # noqa: BLE001 —— 记录失败不得影响权限提示照常出现
        pass
    return 0


if __name__ == "__main__":
    sys.exit(main())
