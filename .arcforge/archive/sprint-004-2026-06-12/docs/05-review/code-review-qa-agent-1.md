# Code Review — sprint-004 自建 qlib 数据包
**Reviewer**: qa-agent-1 (Adversarial / Design Soundness lens)
**Date**: 2026-06-12
**Scope**: git log 41c5e75..HEAD (4 commits: TASK-001 ~ TASK-004)
**Files**: cmd/atlas/export_ohlcv.go (+_test.go), scripts/qlib_eval/build_data.py (+tests/), Makefile, scripts/qlib_eval/README.md
**Lens**: OPERATIONS, STALE STATE, WARM-UP/COVERAGE, HARD-CODED PATHS, DEPENDENCY DIRECTION

---

## Summary Verdict

**NEEDS WORK** — 2 CRITICAL issues must be addressed before the daily cron is trusted in production. 4 lower-severity findings documented. Several design decisions are genuinely well-engineered and worth preserving.

---

## Findings

### F1 [CRITICAL] Partial rebuild leaves a corrupt bundle readable by signal-eval

**File**: `Makefile:41-43`, `scripts/qlib_eval/build_data.py:44-76`

The `qlib-data` recipe is two sequential shell commands with no rollback:

```makefile
./bin/atlas export-ohlcv ...   # writes qlib_csv/*.csv
$(QLIB_PY) scripts/qlib_eval/build_data.py ...  # overwrites atlas_cn in-place
```

`dump_bin dump_all` writes directly into `QLIB_DATA_DIR`. If it crashes mid-run (OOM, disk full, network cut, KeyboardInterrupt), the target directory is left in a partially-overwritten state — some feature `.bin` files from this run, some from the previous run. `verify_bundle` only checks instrument names and calendar bounds (`build_data.py:117-134`); it does not verify that every per-instrument `.bin` file is non-empty or internally consistent. A subsequent `make signal-eval` will silently read the mixed-vintage bundle and produce wrong results — no error, no warning.

**Suggested direction**: Write `dump_bin` output to a temp directory (e.g. `atlas_cn.tmp.$$`), run `verify_bundle` on the temp dir, then atomically `mv` to the final path only on success:
```makefile
# in Makefile or build_data.py main():
target_tmp = f"{target_dir}.tmp.{os.getpid()}"
run_dump_bin(csv_dir, target_tmp, ...)
verify_bundle(target_tmp, ...)
shutil.move(target_tmp, target_dir)
```

---

### F2 [CRITICAL] Stale CSVs from dropped symbols silently enter the next bundle

**File**: `Makefile:42`, `cmd/atlas/export_ohlcv.go:76-79`, `scripts/qlib_eval/build_data.py:124`

`qlib_csv` (QLIB_CSV_DIR) is never cleaned between runs. `os.MkdirAll` + `os.Create` only overwrites files it actually writes. If SIGNAL_SYMBOLS shrinks (a symbol is dropped), the old per-instrument CSV remains. `dump_bin` ingests it, producing a stale instrument in `atlas_cn`. `verify_bundle` checks only the `⊆` direction:

```python
missing = sorted(set(expected_instruments) - set(instruments))  # line 124
```

No check for the `⊇` direction (spurious extras pass silently). `make signal-eval` will evaluate against a bundle containing a symbol the user dropped.

**Suggested direction**: (a) Delete `QLIB_CSV_DIR` before each export in the Makefile recipe (`rm -rf $(QLIB_CSV_DIR) && mkdir -p $(QLIB_CSV_DIR)`), OR (b) add `--clean-out-dir` flag to `export-ohlcv`. Also add reciprocal check to `verify_bundle`: error if bundle instruments are a strict superset of `expected_instruments`.

---

### F3 [WARNING] No warm-up lead-in: SIGNAL_FROM is used as both signal start and data start

**File**: `Makefile:7,42`, spec architecture section

`--from $(SIGNAL_FROM)` (default `2021-01-01`) is passed to both `export-ohlcv` and `export-signals`. Horizon-60 signals at the start of the period have no lead-in data. Signals at the tail lose horizon windows because today is the data end. The README acknowledges this as "越界计 NA" (line 155) but this is silent — no warning is emitted.

**Suggested direction**: Document (or enforce in `build_data.py`) a minimum data span. A warning at `build_data.py` main() when `end - start < 365 days` would surface this operationally. Consider recommending SIGNAL_FROM be set at least 90 calendar days before the first signal of interest.

---

### F4 [WARNING] Hard-coded absolute path for DEFAULT_QLIB_SCRIPTS breaks portability and CI

**File**: `scripts/qlib_eval/build_data.py:17`

```python
DEFAULT_QLIB_SCRIPTS = "/Users/zuowei/workspace/python/qlib/scripts"
```

Any second developer or CI runner silently defaults to a non-existent path. The failure is a `FileNotFoundError` on `dump_bin.py` at subprocess launch — not a clear error message identifying the cause.

**Suggested direction**: Change default to `None` (or `os.environ.get("QLIB_SCRIPTS_DIR")`). Raise a clear `ValueError` when absent:
```python
if not Path(scripts_dir).exists():
    raise ValueError(
        f"dump_bin scripts dir not found: {scripts_dir!r}\n"
        "Set --qlib-scripts or QLIB_SCRIPTS_DIR env, or: pip install pyqlib"
    )
```

---

### F5 [WARNING] dump_bin arg contract validated only by mock — no live signal on arg rename

**File**: `scripts/qlib_eval/tests/test_build_data.py:85`, `build_data.py:60-68`

`--data_path` (vs old `--csv_path`) is asserted via `cmd.index("--data_path")` in the mock test. If the qlib fork updates and renames the argument again, tests pass but runtime crashes. The `--exclude_fields symbol,date` coupling has the same property.

**Suggested direction**: Add a lightweight integration test (`pytest -m integration`, already mentioned in plan but deferred) that calls `dump_bin.py --help` and asserts `--data_path` in output. Document the qlib commit SHA pinned in `requirements.txt` or a comment in `build_data.py`.

---

### F6 [WARNING] evaluate.py DEFAULT_QLIB_DIR still points to cn_data — direct invocation silently reads stale data

**File**: `scripts/qlib_eval/evaluate.py:28`

```python
DEFAULT_QLIB_DIR = "~/.qlib/qlib_data/cn_data"
```

The Makefile always passes `--qlib-dir $(QLIB_DIR)` (now atlas_cn), so the Makefile path is safe. But a user who runs `python evaluate.py --signals signals.csv` directly gets silent 2021-2026 signal loss — the exact regression this feature was built to fix. README warns about this (line 88-89) but the warning is easy to miss.

**Suggested direction**: Change `DEFAULT_QLIB_DIR` in `evaluate.py` to `"~/.qlib/qlib_data/atlas_cn"` to align with the Makefile default, eliminating the divergence. This is a one-line fix.

---

### F7 [SUGGESTION] verify_bundle instrument case comparison — normalize both sides

**File**: `scripts/qlib_eval/build_data.py:149,124`

```python
expected = {p.stem.upper() for p in _data_csvs(csv_dir)}  # stems uppercased
```

If dump_bin ever emits mixed-case or lowercase instrument names under a different qlib version, the set-difference check would report false missing instruments.

**Suggested direction**: Normalize both sides:
```python
missing = sorted(set(expected_instruments) - {k.upper() for k in instruments})
```

---

## What Is Genuinely Well-Designed

- **Dependency injection** (`ohlcvDeps` Go, mock-subprocess Python): clean, fast, zero-network tests. The golden CSV test is byte-exact and column-order-sensitive — it would catch a column swap a statistical test would not.
- **Benchmark as hard-error vs. degrading**: correctly placed in the core layer (`executeExportOHLCV` immediate return, not deferred to summary). `TestExportOHLCV_BenchmarkFailureIsFatal` correctly asserts this.
- **Three-form symbol contract** (`000300.SH` → `SH000300` → `sh000300.csv`): documented in spec, enforced by cross-language contract test (Go line 89-92 cross-references Python authority). The right way to handle a polyglot contract.
- **`verify_bundle` is read-only by design and tested**: `test_verify_bundle_is_read_only` uses a directory digest before/after, preventing the common mistake of having verification mutate what it verifies.
- **C1-1 BLOCKER enforcement**: explicit `--symbols $(SIGNAL_SYMBOLS)` in the Makefile recipe with a `test_qlib_data_target_flags` Makefile test. The plan comment at `Makefile:36-39` is unusually thorough and correctly documents the silent-failure mode it prevents.
- **cron failure is loud**: `dump_bin` failure exits non-zero and propagates stderr (`build_data.py:73-75`). The cron example redirects `2>&1`. Cron logs will catch it.

---

## Required Actions Before Accepting

| Priority | Finding | Action |
|----------|---------|--------|
| CRITICAL | F1: no atomic bundle swap | Implement temp-dir + mv in `build_data.py:main()` |
| CRITICAL | F2: stale CSVs persist | Clean `QLIB_CSV_DIR` before export; add `⊇` check to `verify_bundle` |
| WARNING  | F4: hard-coded absolute path | Default to `None`/env-var, raise clear error |
| WARNING  | F6: evaluate.py DEFAULT_QLIB_DIR | Change to `atlas_cn` (one-line) |
| WARNING  | F3: warm-up gap | Add min-span warning in `build_data.py:main()` |
| WARNING  | F5: mock-only arg contract | Note qlib version; add integration marker |
| SUGGESTION | F7: case normalization | One-liner in `verify_bundle` |
