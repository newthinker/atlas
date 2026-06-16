"""CLI: build the SQLite warehouse from a qlib_csv_* directory."""
import argparse
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import List, Optional

from . import ingest, writer


def main(argv: Optional[List[str]] = None) -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--csv-dir", required=True)
    ap.add_argument("--market", required=True)
    ap.add_argument("--source", required=True)
    ap.add_argument("--db", required=True)
    ap.add_argument("--dumped-at", default=None)
    args = ap.parse_args(argv)

    csv_dir = Path(args.csv_dir)
    if not csv_dir.is_dir():
        print(f"csv-dir not found: {csv_dir}", file=sys.stderr)
        return 1

    rows = ingest.parse_dir(csv_dir)
    if not rows:
        print(f"no rows parsed from {csv_dir}", file=sys.stderr)
        return 2

    dumped_at = args.dumped_at or datetime.now(timezone.utc).isoformat()
    writer.write(args.db, rows, market=args.market, source=args.source,
                 dumped_at=dumped_at)
    print(f"wrote {len(rows)} rows to {args.db}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
