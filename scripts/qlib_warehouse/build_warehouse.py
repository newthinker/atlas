"""CLI: build the SQLite warehouse from qlib_csv_* directories.

Two modes:
  - single-market:  --csv-dir DIR --market M --source S [--fundamentals-dir F]
  - multi-market:   --add MARKET SOURCE CSV_DIR  (repeatable)
                    rebuilds the whole DB from every group whose dir has rows;
                    missing/empty dirs are skipped with a warning (not fatal),
                    so a partial day (e.g. only US refreshed yet) still works.
Both modes write one atomic DB so markets coexist after either schedule runs.
"""
import argparse
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import List, Optional

from . import ingest, writer


def _build_groups(add_specs):
    """Parse each (market, source, csv_dir); skip missing/empty dirs."""
    groups = []
    for market, source, csv_dir in add_specs:
        d = Path(csv_dir)
        if not d.is_dir():
            print(f"skip {market}: csv-dir not found: {d}", file=sys.stderr)
            continue
        rows = ingest.parse_dir(d, market)
        if not rows:
            print(f"skip {market}: no rows in {d}", file=sys.stderr)
            continue
        groups.append({"rows": rows, "market": market, "source": source})
        print(f"ingested {market}: {len(rows)} rows from {d}")
    return groups


def main(argv: Optional[List[str]] = None) -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", required=True)
    ap.add_argument("--dumped-at", default=None)
    # single-market mode
    ap.add_argument("--csv-dir")
    ap.add_argument("--market")
    ap.add_argument("--source")
    ap.add_argument("--fundamentals-dir", default=None)
    # multi-market mode (repeatable)
    ap.add_argument("--add", nargs=3, metavar=("MARKET", "SOURCE", "CSV_DIR"),
                    action="append", default=[])
    args = ap.parse_args(argv)

    dumped_at = args.dumped_at or datetime.now(timezone.utc).isoformat()

    if args.add:
        groups = _build_groups(args.add)
        if not groups:
            print("no rows parsed from any --add group", file=sys.stderr)
            return 2
        writer.write_groups(args.db, groups, dumped_at)
        total = sum(len(g["rows"]) for g in groups)
        print(f"wrote {total} rows ({len(groups)} markets) to {args.db}")
        return 0

    # single-market mode
    if not (args.csv_dir and args.market and args.source):
        print("single mode requires --csv-dir --market --source (or use --add)",
              file=sys.stderr)
        return 1
    csv_dir = Path(args.csv_dir)
    if not csv_dir.is_dir():
        print(f"csv-dir not found: {csv_dir}", file=sys.stderr)
        return 1
    rows = ingest.parse_dir(csv_dir, args.market)
    if not rows:
        print(f"no rows parsed from {csv_dir}", file=sys.stderr)
        return 2

    from . import fundamentals  # local import to keep ingest path lazy
    funds = None
    if args.fundamentals_dir:
        fdir = Path(args.fundamentals_dir)
        if not fdir.is_dir():
            print(f"fundamentals-dir not found: {fdir}", file=sys.stderr)
            return 3
        funds = fundamentals.parse_dir(fdir)

    writer.write(args.db, rows, market=args.market, source=args.source,
                 dumped_at=dumped_at, fundamentals=funds)
    print(f"wrote {len(rows)} rows to {args.db}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
