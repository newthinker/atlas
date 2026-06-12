"""Build a qlib data bundle from atlas-exported CSVs via the official dump_bin.

Thin orchestration only: dump_bin.py owns the bin format, calendars and
instruments; we construct its command, propagate failures, and VERIFY (never
rewrite) the outputs. See spec 2026-06-12-qlib-data-bundle-design.md.

零 qlib 依赖：唯一外部调用是 subprocess 跑官方 dump_bin.py（仅运行时）；
单测全部 mock subprocess。系统 python3 已损坏——run_dump_bin 用 sys.executable
（即调用本脚本的 venv python，hook/Makefile 均经 scripts/qlib_eval/.venv/bin/python）。
"""

import argparse
import subprocess
import sys
from pathlib import Path

from qlib_eval.symbols import to_qlib_instrument  # 纯字符串映射，零 qlib 依赖

DEFAULT_QLIB_SCRIPTS = "/Users/zuowei/workspace/python/qlib/scripts"
DEFAULT_TARGET = "~/.qlib/qlib_data/atlas_cn"

INSTRUMENTS_SEP = "\t"  # dump_bin save_instruments 真实分隔符
HEADER_FIELD = "date"  # CSV 日期列名（export-ohlcv header: symbol,date,open,...）


def _data_csvs(csv_dir):
    """csv_dir 下所有 *.csv，按文件名排序。"""
    return sorted(Path(csv_dir).glob("*.csv"))


def verify_csv_dir_symbols(csv_dir, expected_instruments):
    """dump 前校验 csv_dir 的文件集合与预期符号一致（FP-1）。

    防呆场景：用户改了 SIGNAL_SYMBOLS 后，qlib_csv 里上一轮导出的旧符号 CSV 仍残留，
    会被 dump_bin 一并打进新包——而 main 原本从文件名反推 expected，旧 CSV 静默混入、
    verify_bundle 也照样通过。这里用「外部预期符号集」独立比对磁盘文件集合，发现
    非预期 .csv 即 raise 并提示清理。

    expected_instruments: 大写 qlib instrument 集合（如 {"SH600519"}）。
    """
    actual = {p.stem.upper() for p in _data_csvs(csv_dir)}
    extra = sorted(actual - set(expected_instruments))
    if extra:
        raise ValueError(
            f"{csv_dir} 含非预期符号的残留 CSV {extra}（不在本次导出符号集内）；"
            f"请先清理（rm {csv_dir}/* 后重跑 make qlib-data）再构建数据包。"
        )


def _csv_lines(path):
    """返回 CSV 的非空行（含 header）。"""
    return [ln for ln in Path(path).read_text().splitlines() if ln.strip()]


def _csv_data_rows(path):
    """返回 CSV 的数据行（去掉空行与 header）。"""
    return _csv_lines(path)[1:]


def _date_col_index(header_line):
    cols = [c.strip() for c in header_line.split(",")]
    return cols.index(HEADER_FIELD)


def run_dump_bin(csv_dir, target_dir, scripts_dir=DEFAULT_QLIB_SCRIPTS):
    """调官方 dump_bin.py dump_all 构建 bundle；非 0 返回码透传为 RuntimeError。

    前置：csv_dir 必须非空且无空 CSV（每个 CSV >= header + 1 数据行），否则进
    dump 前 raise ValueError——防空 instrument 污染产物（boundary[0]）。
    """
    csv_dir = Path(csv_dir)
    csvs = _data_csvs(csv_dir)
    if not csvs:
        raise ValueError(f"no CSV files under {csv_dir}; refusing to dump")
    for c in csvs:
        if len(_csv_data_rows(c)) < 1:
            raise ValueError(f"empty CSV (no data rows): {c}; refusing to dump")

    dump_script = str(Path(scripts_dir) / "dump_bin.py")
    cmd = [
        sys.executable,
        dump_script,
        "dump_all",
        "--data_path",  # ⚠ 本地副本签名 DumpDataBase.__init__(data_path,...)，非 csv_path
        str(csv_dir),
        "--qlib_dir",
        str(target_dir),
        "--exclude_fields",
        "symbol,date",  # 字符串列不排除 → astype(float32) 必崩
    ]
    proc = subprocess.run(cmd, capture_output=True, text=True)
    if proc.returncode != 0:
        stderr = (proc.stderr or "").strip()
        raise RuntimeError(
            f"dump_bin failed (returncode={proc.returncode}): {stderr}"
        )
    return proc


def date_span_from_csvs(csv_dir):
    """扫全部 CSV 数据行的 date 列，返回 (min_date, max_date) 字符串。"""
    csv_dir = Path(csv_dir)
    dates = []
    for c in _data_csvs(csv_dir):
        lines = _csv_lines(c)
        if not lines:
            continue
        di = _date_col_index(lines[0])
        for row in lines[1:]:
            dates.append(row.split(",")[di].strip())
    if not dates:
        raise ValueError(f"no data rows found under {csv_dir}")
    return min(dates), max(dates)


def _read_instruments(target_dir):
    """读 instruments/all.txt（tab 三字段），返回 {instrument: (begin, end)}。"""
    path = Path(target_dir) / "instruments" / "all.txt"
    if not path.exists():
        raise ValueError(f"missing instruments file: {path}")
    out = {}
    for line in path.read_text().splitlines():
        if not line.strip():
            continue
        parts = line.split(INSTRUMENTS_SEP)
        out[parts[0]] = (parts[1], parts[2]) if len(parts) >= 3 else (None, None)
    return out


def _read_calendar(target_dir):
    """读 calendars/day.txt，返回排序后的日期列表。"""
    path = Path(target_dir) / "calendars" / "day.txt"
    if not path.exists():
        raise ValueError(f"missing calendar file: {path}")
    return sorted(d.strip() for d in path.read_text().splitlines() if d.strip())


def verify_bundle(target_dir, expected_instruments, start, end):
    """只读校验 bundle：每个 expected instrument 有行 + calendar 覆盖 [start,end]。

    绝不写文件（instruments/calendar 由 dump_bin 自动生成）。不满足 → ValueError。
    """
    instruments = _read_instruments(target_dir)
    missing = sorted(set(expected_instruments) - set(instruments))
    if missing:
        raise ValueError(f"missing instruments in bundle: {missing}")

    calendar = _read_calendar(target_dir)
    if not calendar:
        raise ValueError("empty calendar in bundle")
    if calendar[0] > start or calendar[-1] < end:
        raise ValueError(
            f"calendar [{calendar[0]}, {calendar[-1]}] does not cover "
            f"expected span [{start}, {end}]"
        )


def main(argv=None):
    parser = argparse.ArgumentParser(
        description="Build a qlib data bundle from atlas CSVs via dump_bin."
    )
    parser.add_argument("--csv-dir", required=True)
    parser.add_argument("--target-dir", default=DEFAULT_TARGET)
    parser.add_argument("--qlib-scripts", default=DEFAULT_QLIB_SCRIPTS)
    parser.add_argument(
        "--expected-symbols",
        default="",
        help="逗号分隔的 atlas 符号清单（如 600519.SH,000300.SH）；提供时在 dump 前"
        "校验 csv-dir 文件集合无残留旧符号 CSV（FP-1 防呆）。",
    )
    args = parser.parse_args(argv)

    csv_dir = Path(args.csv_dir).expanduser()
    target_dir = Path(args.target_dir).expanduser()

    expected = {p.stem.upper() for p in _data_csvs(csv_dir)}
    start, end = date_span_from_csvs(csv_dir)

    # FP-1：提供 --expected-symbols 时，用外部预期符号集独立校验磁盘文件集合，
    # 早于 dump 拦截残留旧符号 CSV（不提供则跳过，向后兼容）。
    if args.expected_symbols.strip():
        wanted = {
            to_qlib_instrument(s.strip())
            for s in args.expected_symbols.split(",")
            if s.strip()
        }
        verify_csv_dir_symbols(csv_dir, wanted)

    run_dump_bin(csv_dir, target_dir, scripts_dir=args.qlib_scripts)
    verify_bundle(target_dir, expected_instruments=expected, start=start, end=end)

    print(f"qlib 数据包就绪: {target_dir}")
    print(f"  instruments: {len(expected)}  区间: [{start}, {end}]")
    print(f"  运行评估: make signal-eval QLIB_DIR={target_dir}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
