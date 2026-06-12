"""Context Checkpoint: done_criteria -> test mapping
functional[0] "make signal-eval target 存在且命令链正确（依赖 export-signals + venv python 调 evaluate.py，非裸 python）"
              -> test_signal_eval_target_exists / test_signal_eval_depends_on_export_signals
                 / test_signal_eval_uses_venv_python_not_bare_python / test_signal_eval_invokes_evaluate_py

functional[0] 由 Leader 强调：python 必须用 scripts/qlib_eval/.venv/bin/python（系统 python3 损坏），
绝不写裸 python——本守门测试机制性锁死，防回归。
"""

import pathlib
import re

VENV_PYTHON = "scripts/qlib_eval/.venv/bin/python"


def _repo_root() -> pathlib.Path:
    for parent in pathlib.Path(__file__).resolve().parents:
        if (parent / "Makefile").exists() and (parent / "go.mod").exists():
            return parent
    raise RuntimeError("repo root (Makefile + go.mod) not found")


def _makefile_text() -> str:
    return (_repo_root() / "Makefile").read_text()


def _var_defs(text: str) -> dict:
    """解析 Makefile 变量定义（VAR = / := / ?= value）。"""
    defs = {}
    for m in re.finditer(r"^([A-Za-z_][A-Za-z0-9_]*)\s*[:?]?=\s*(.*)$", text, re.MULTILINE):
        defs[m.group(1)] = m.group(2).strip()
    return defs


def _expand(s: str, defs: dict) -> str:
    """递归展开 $(VAR) 引用（足够覆盖本 Makefile 的简单变量）。"""
    for _ in range(10):
        new = re.sub(r"\$\(([A-Za-z_][A-Za-z0-9_]*)\)",
                     lambda m: defs.get(m.group(1), m.group(0)), s)
        if new == s:
            break
        s = new
    return s


def _signal_eval_block() -> str:
    """提取 signal-eval 目标行 + 其后所有以 Tab 开头的 recipe 行。"""
    lines = _makefile_text().splitlines()
    block = []
    capturing = False
    for line in lines:
        if re.match(r"^signal-eval\s*:", line):
            capturing = True
            block.append(line)
            continue
        if capturing:
            if line.startswith("\t") or line.strip() == "":
                block.append(line)
            else:
                break
    return "\n".join(block)


def test_signal_eval_target_exists():
    block = _signal_eval_block()
    assert re.search(r"^signal-eval\s*:", block, re.MULTILINE), "缺少 signal-eval 目标"


def test_signal_eval_depends_on_export_signals():
    block = _signal_eval_block()
    target_line = block.splitlines()[0]
    assert "export-signals" in target_line, "signal-eval 必须依赖 export-signals"


def test_signal_eval_invokes_evaluate_py():
    block = _signal_eval_block()
    assert "evaluate.py" in block, "signal-eval recipe 必须调用 evaluate.py"


def test_signal_eval_uses_venv_python_not_bare_python():
    block = _signal_eval_block()
    recipe = "\n".join(block.splitlines()[1:])  # 仅 recipe 行
    expanded = _expand(recipe, _var_defs(_makefile_text()))  # 展开 $(QLIB_PY) 等变量
    assert VENV_PYTHON in expanded, f"recipe 展开后必须用 {VENV_PYTHON}"
    # 去掉 venv 路径后，不得再出现任何裸 python token（系统 python3 已损坏）
    stripped = expanded.replace(VENV_PYTHON, "")
    assert not re.search(r"\bpython[0-9.]*\b", stripped), (
        "signal-eval recipe 不得出现裸 python 调用，必须经 venv python"
    )
