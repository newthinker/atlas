"""Context Checkpoint: done_criteria -> test mapping (TASK-003)
functional[0] "mock subprocess: dump_bin.py + dump_all + --data_path + --qlib_dir
               + --exclude_fields symbol,date (紧邻值断言)"
              -> test_dump_bin_command_construction
functional[1] "date_span_from_csvs: 从多 CSV 数据行推导 (min,max) 日期"
              -> test_date_span_from_csvs
functional[2] "verify_bundle: tab 三字段大写 fixture 通过；缺 instrument / calendar
               区间不足 -> ValueError 含缺失项；只读(前后零修改)"
              -> test_verify_bundle_passes / test_verify_bundle_missing_instrument
                 / test_verify_bundle_calendar_too_narrow / test_verify_bundle_is_read_only
boundary[0]   "csv 目录含空文件/仅 header 文件 -> 进 dump 前 raise"
              -> test_csv_dir_empty_file_rejected / test_csv_dir_header_only_rejected
error_handling[0] "dump_bin returncode!=0 -> raise(含 stderr 摘要)"
              -> test_dump_bin_failure_propagates

TASK-004 review_fix FP-1 "dump 前校验 csv_dir 文件集合与预期符号一致，旧 CSV 残留 -> raise"
              -> test_verify_csv_dir_symbols_passes_exact / test_verify_csv_dir_symbols_rejects_extra
                 / test_main_expected_symbols_mismatch_aborts_before_dump
                 / test_main_expected_symbols_match_proceeds

零 qlib 依赖：build_data 仅在 run_dump_bin 内 subprocess.run 官方 dump_bin.py，
测试全部 mock subprocess（hook 同款命令从仓库根运行，conftest 注入 sys.path）。
"""

import hashlib
from pathlib import Path
from unittest import mock

import pytest

import build_data


# --------------------------------------------------------------------------
# fixtures / helpers
# --------------------------------------------------------------------------
def _write_csv(path: Path, rows):
    """rows: list of (date, open) tuples; header 固定 symbol,date,open,...格式。"""
    symbol = path.stem.upper()
    lines = ["symbol,date,open,high,low,close,volume,factor"]
    for d, o in rows:
        lines.append(f"{symbol},{d},{o},{o},{o},{o},1000,1")
    path.write_text("\n".join(lines) + "\n")


def _make_bundle(target: Path, instruments, calendar_dates):
    """造一个 dump_bin 真实格式的假 bundle（只供只读校验）。

    instruments: dict instrument(大写) -> (begin, end)
    calendar_dates: list[str] YYYY-MM-DD
    """
    inst_dir = target / "instruments"
    cal_dir = target / "calendars"
    inst_dir.mkdir(parents=True, exist_ok=True)
    cal_dir.mkdir(parents=True, exist_ok=True)
    # instruments/all.txt: tab 三字段，symbol 大写（save_instruments 真实格式）
    all_lines = [f"{ins}\t{b}\t{e}" for ins, (b, e) in instruments.items()]
    (inst_dir / "all.txt").write_text("\n".join(all_lines) + "\n")
    # calendars/day.txt: 每行一个日期
    (cal_dir / "day.txt").write_text("\n".join(calendar_dates) + "\n")


def _tree_digest(root: Path):
    """目录下所有文件路径+内容的指纹，用于断言只读（前后零修改）。"""
    h = hashlib.sha256()
    for p in sorted(root.rglob("*")):
        if p.is_file():
            h.update(str(p.relative_to(root)).encode())
            h.update(p.read_bytes())
    return h.hexdigest()


# --------------------------------------------------------------------------
# functional[0] — dump_bin 命令构造
# --------------------------------------------------------------------------
def test_dump_bin_command_construction(tmp_path):
    csv_dir = tmp_path / "csv"
    csv_dir.mkdir()
    _write_csv(csv_dir / "sh600519.csv", [("2024-01-02", "1")])
    target = tmp_path / "bundle"
    with mock.patch("build_data.subprocess.run") as run:
        run.return_value = mock.Mock(returncode=0, stderr="")
        build_data.run_dump_bin(csv_dir, target, scripts_dir=tmp_path)
        cmd = run.call_args[0][0]

    # cmd[0] = python 解释器，cmd[1] = .../dump_bin.py，cmd[2] = dump_all
    assert cmd[1].endswith("dump_bin.py")
    assert cmd[2] == "dump_all"
    # ⚠ 本地副本参数名 --data_path（非 csv_path，C2-1 BLOCKER）；紧邻值断言
    i = cmd.index("--data_path")
    assert cmd[i + 1] == str(csv_dir)
    j = cmd.index("--qlib_dir")
    assert cmd[j + 1] == str(target)
    k = cmd.index("--exclude_fields")
    assert cmd[k + 1] == "symbol,date"  # 字符串列不排除 dump 必崩


# --------------------------------------------------------------------------
# error_handling[0] — 失败透传
# --------------------------------------------------------------------------
def test_dump_bin_failure_propagates(tmp_path):
    csv_dir = tmp_path / "csv"
    csv_dir.mkdir()
    _write_csv(csv_dir / "sh600519.csv", [("2024-01-02", "1")])
    with mock.patch("build_data.subprocess.run") as run:
        run.return_value = mock.Mock(returncode=1, stderr="boom: bad column")
        with pytest.raises(RuntimeError) as ei:
            build_data.run_dump_bin(csv_dir, tmp_path / "bundle", scripts_dir=tmp_path)
    assert "boom" in str(ei.value)  # stderr 摘要透传，不静默


# --------------------------------------------------------------------------
# boundary[0] — 空 CSV / 仅 header 拒绝（进 dump 前）
# --------------------------------------------------------------------------
def test_csv_dir_empty_file_rejected(tmp_path):
    csv_dir = tmp_path / "csv"
    csv_dir.mkdir()
    _write_csv(csv_dir / "sh600519.csv", [("2024-01-02", "1")])
    (csv_dir / "sh000001.csv").write_text("")  # 0 字节
    with mock.patch("build_data.subprocess.run") as run:
        with pytest.raises((ValueError, RuntimeError)):
            build_data.run_dump_bin(csv_dir, tmp_path / "bundle", scripts_dir=tmp_path)
        run.assert_not_called()  # 进 dump 前就拒绝


def test_csv_dir_header_only_rejected(tmp_path):
    csv_dir = tmp_path / "csv"
    csv_dir.mkdir()
    _write_csv(csv_dir / "sh600519.csv", [("2024-01-02", "1")])
    (csv_dir / "sh000002.csv").write_text(
        "symbol,date,open,high,low,close,volume,factor\n"
    )  # 仅 header，无数据行
    with mock.patch("build_data.subprocess.run") as run:
        with pytest.raises((ValueError, RuntimeError)):
            build_data.run_dump_bin(csv_dir, tmp_path / "bundle", scripts_dir=tmp_path)
        run.assert_not_called()


def test_csv_dir_empty_dir_rejected(tmp_path):
    csv_dir = tmp_path / "csv"
    csv_dir.mkdir()  # 完全没有 CSV
    with mock.patch("build_data.subprocess.run") as run:
        with pytest.raises((ValueError, RuntimeError)):
            build_data.run_dump_bin(csv_dir, tmp_path / "bundle", scripts_dir=tmp_path)
        run.assert_not_called()


# --------------------------------------------------------------------------
# functional[1] — date_span_from_csvs
# --------------------------------------------------------------------------
def test_date_span_from_csvs(tmp_path):
    csv_dir = tmp_path / "csv"
    csv_dir.mkdir()
    _write_csv(csv_dir / "sh600519.csv", [("2021-01-04", "1"), ("2021-03-10", "2")])
    _write_csv(csv_dir / "sh000300.csv", [("2020-12-31", "1"), ("2021-02-01", "2")])
    start, end = build_data.date_span_from_csvs(csv_dir)
    assert start == "2020-12-31"  # 全局 min
    assert end == "2021-03-10"  # 全局 max


# --------------------------------------------------------------------------
# functional[2] — verify_bundle
# --------------------------------------------------------------------------
def test_verify_bundle_passes(tmp_path):
    target = tmp_path / "bundle"
    _make_bundle(
        target,
        instruments={"SH600519": ("2021-01-04", "2021-03-10")},
        calendar_dates=["2021-01-04", "2021-02-01", "2021-03-10"],
    )
    # 不抛即通过
    build_data.verify_bundle(
        target, expected_instruments={"SH600519"}, start="2021-01-04", end="2021-03-10"
    )


def test_verify_bundle_missing_instrument(tmp_path):
    target = tmp_path / "bundle"
    _make_bundle(
        target,
        instruments={"SH600519": ("2021-01-04", "2021-03-10")},
        calendar_dates=["2021-01-04", "2021-03-10"],
    )
    with pytest.raises(ValueError) as ei:
        build_data.verify_bundle(
            target,
            expected_instruments={"SH600519", "SH000300"},
            start="2021-01-04",
            end="2021-03-10",
        )
    assert "SH000300" in str(ei.value)  # 消息含缺失项


def test_verify_bundle_calendar_too_narrow(tmp_path):
    target = tmp_path / "bundle"
    _make_bundle(
        target,
        instruments={"SH600519": ("2021-01-04", "2021-03-10")},
        calendar_dates=["2021-02-01", "2021-02-02"],  # 不覆盖 [start,end]
    )
    with pytest.raises(ValueError):
        build_data.verify_bundle(
            target,
            expected_instruments={"SH600519"},
            start="2021-01-04",
            end="2021-03-10",
        )


def test_verify_bundle_is_read_only(tmp_path):
    target = tmp_path / "bundle"
    _make_bundle(
        target,
        instruments={"SH600519": ("2021-01-04", "2021-03-10")},
        calendar_dates=["2021-01-04", "2021-02-01", "2021-03-10"],
    )
    before = _tree_digest(target)
    build_data.verify_bundle(
        target, expected_instruments={"SH600519"}, start="2021-01-04", end="2021-03-10"
    )
    assert _tree_digest(target) == before  # 校验前后产物文件零修改


# --------------------------------------------------------------------------
# review_fix FP-1 — dump 前校验 csv_dir 文件集合与预期符号一致
# --------------------------------------------------------------------------
def test_verify_csv_dir_symbols_passes_exact(tmp_path):
    csv_dir = tmp_path / "csv"
    csv_dir.mkdir()
    _write_csv(csv_dir / "sh600519.csv", [("2024-01-02", "1")])
    _write_csv(csv_dir / "sh000300.csv", [("2024-01-02", "1")])
    # 预期集合与磁盘文件集合一致 → 不抛
    build_data.verify_csv_dir_symbols(csv_dir, {"SH600519", "SH000300"})


def test_verify_csv_dir_symbols_rejects_extra(tmp_path):
    csv_dir = tmp_path / "csv"
    csv_dir.mkdir()
    _write_csv(csv_dir / "sh600519.csv", [("2024-01-02", "1")])
    _write_csv(csv_dir / "sh000001.csv", [("2024-01-02", "1")])  # 旧符号残留
    with pytest.raises(ValueError) as ei:
        build_data.verify_csv_dir_symbols(csv_dir, {"SH600519"})
    msg = str(ei.value)
    assert "SH000001" in msg  # 指明多余符号
    assert "清理" in msg or "clean" in msg.lower()  # 提示清理


def test_main_expected_symbols_mismatch_aborts_before_dump(tmp_path):
    csv_dir = tmp_path / "csv"
    csv_dir.mkdir()
    _write_csv(csv_dir / "sh600519.csv", [("2024-01-02", "1")])
    _write_csv(csv_dir / "sh000001.csv", [("2024-01-02", "1")])  # 残留
    with mock.patch("build_data.subprocess.run") as run:
        with pytest.raises(ValueError):
            build_data.main(
                [
                    "--csv-dir",
                    str(csv_dir),
                    "--target-dir",
                    str(tmp_path / "bundle"),
                    "--expected-symbols",
                    "600519.SH",  # atlas 形式；仅期望 SH600519
                ]
            )
        run.assert_not_called()  # 在 dump 前就中止


def test_main_expected_symbols_match_proceeds(tmp_path):
    csv_dir = tmp_path / "csv"
    csv_dir.mkdir()
    _write_csv(csv_dir / "sh600519.csv", [("2021-01-04", "1")])
    target = tmp_path / "bundle"
    # 预期与磁盘一致 → 越过校验进入 dump（subprocess mock 成功），再 mock verify_bundle
    with mock.patch("build_data.subprocess.run") as run, mock.patch(
        "build_data.verify_bundle"
    ) as vb:
        run.return_value = mock.Mock(returncode=0, stderr="")
        rc = build_data.main(
            [
                "--csv-dir",
                str(csv_dir),
                "--target-dir",
                str(target),
                "--qlib-scripts",
                str(tmp_path),
                "--expected-symbols",
                "600519.SH",
            ]
        )
        assert rc == 0
        run.assert_called_once()  # 通过校验，确实进 dump
        vb.assert_called_once()
