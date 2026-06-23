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


def _target_block(name: str) -> str:
    """提取指定 Make 目标行 + 其后所有以 Tab 开头的 recipe 行。"""
    lines = _makefile_text().splitlines()
    block = []
    capturing = False
    for line in lines:
        if re.match(rf"^{re.escape(name)}\s*:", line):
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
    block = _target_block("signal-eval")
    assert re.search(r"^signal-eval\s*:", block, re.MULTILINE), "缺少 signal-eval 目标"


def test_signal_eval_depends_on_export_signals():
    block = _target_block("signal-eval")
    target_line = block.splitlines()[0]
    assert "export-signals" in target_line, "signal-eval 必须依赖 export-signals"


def test_signal_eval_invokes_evaluate_py():
    block = _target_block("signal-eval")
    assert "evaluate.py" in block, "signal-eval recipe 必须调用 evaluate.py"


def test_signal_eval_uses_venv_python_not_bare_python():
    block = _target_block("signal-eval")
    recipe = "\n".join(block.splitlines()[1:])  # 仅 recipe 行
    expanded = _expand(recipe, _var_defs(_makefile_text()))  # 展开 $(QLIB_PY) 等变量
    assert VENV_PYTHON in expanded, f"recipe 展开后必须用 {VENV_PYTHON}"
    # 去掉 venv 路径后，不得再出现任何裸 python token（系统 python3 已损坏）
    stripped = expanded.replace(VENV_PYTHON, "")
    assert not re.search(r"\bpython[0-9.]*\b", stripped), (
        "signal-eval recipe 不得出现裸 python 调用，必须经 venv python"
    )


def test_qlib_data_target_flags():
    # C1-1 BLOCKER 防线：qlib-data recipe 不带 --config 时 CLI 拿不到 watchlist，
    # 必须显式 --symbols $(SIGNAL_SYMBOLS) 才不会退化为只导基准；spec 钉死不传 --to。
    block = _target_block("qlib-data")
    assert "--symbols $(SIGNAL_SYMBOLS)" in block  # C1-1 防线
    assert "--from $(SIGNAL_FROM)" in block
    assert "--to" not in block  # spec: 不传 --to
    assert "$(QLIB_PY) scripts/qlib_eval/build_data.py" in block


def test_qlib_dir_default_is_atlas_cn():
    # TASK-004 functional[0]：signal-eval 默认 QLIB_DIR 必须指向自建包 atlas_cn
    # （而非社区包 cn_data，社区包截止 2020-09 → 默认 2021-2026 区间产不出结果）。
    defs = _var_defs(_makefile_text())
    assert "QLIB_DIR" in defs, "Makefile 缺少 QLIB_DIR 变量"
    expanded = _expand(defs["QLIB_DIR"], defs)
    assert expanded.endswith("atlas_cn"), f"QLIB_DIR 默认须指向 atlas_cn，实得 {expanded!r}"


# --- TASK-005: 美股 target 守门测试（镜像 hk） ---

def test_signal_eval_us_target_exists_and_correct():
    block = _target_block("signal-eval-us")
    assert block, "signal-eval-us target 缺失"
    defs = _var_defs(_makefile_text())
    expanded = _expand(block, defs)
    assert "export-signals" in block
    assert "--benchmark ^GSPC" in expanded
    assert "--region us" in expanded
    assert "atlas_us" in expanded
    assert VENV_PYTHON in expanded
    assert "evaluate.py" in block


def test_qlib_data_us_target_exists():
    block = _target_block("qlib-data-us")
    assert block, "qlib-data-us target 缺失"
    expanded = _expand(block, _var_defs(_makefile_text()))
    assert "--market us" in expanded
    assert "atlas_us" in expanded
    assert "build_data.py" in block


def test_us_targets_in_phony():
    first_line = _makefile_text().splitlines()[0]
    assert "signal-eval-us" in first_line and "qlib-data-us" in first_line


# --- TASK-007: 时序 IC 评估 target 守门测试 ---

def test_ic_targets_in_phony():
    first_line = _makefile_text().splitlines()[0]
    assert "signal-ic" in first_line and "baseline-scores" in first_line


def test_signal_ic_uses_venv_python_and_ic_evaluate():
    block = _target_block("signal-ic")
    assert block, "signal-ic target 缺失"
    recipe = "\n".join(block.splitlines()[1:])  # 仅 recipe 行
    expanded = _expand(recipe, _var_defs(_makefile_text()))
    assert "ic_evaluate.py" in expanded, "signal-ic recipe 必须调用 ic_evaluate.py"
    assert VENV_PYTHON in expanded, f"signal-ic recipe 展开后必须用 {VENV_PYTHON}"
    # 去掉 venv 路径后不得再出现任何裸 python token（系统 python3 已损坏）
    stripped = expanded.replace(VENV_PYTHON, "")
    assert not re.search(r"\bpython[0-9.]*\b", stripped), (
        "signal-ic recipe 不得出现裸 python 调用，必须经 venv python"
    )


def test_baseline_scores_recipe_content():
    block = _target_block("baseline-scores")
    assert block, "baseline-scores target 缺失"
    expanded = _expand(block, _var_defs(_makefile_text()))
    assert "load_prices" in expanded
    assert "reversal_scores" in expanded
    assert "baseline_scores.csv" in expanded
