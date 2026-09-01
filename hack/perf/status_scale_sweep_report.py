#!/usr/bin/env python3
"""Report a scale sweep: one table per metric, scales down the rows, trees across.

The question a sweep answers is whether a cost grows with route count, so each
cell also shows the ratio to the first tree (treated as the reference) at that
scale.
"""

import argparse
import glob
import json
import os
import re
import statistics
import sys

METRICS = [
    ("load cpu, create+settle (s)", lambda r: r["create"]["cpuSeconds"] + r["settle"]["cpuSeconds"], "{:.2f}"),
    ("load alloc, create+settle (MiB)", lambda r: (r["create"]["allocBytes"] + r["settle"]["allocBytes"]) / 2**20, "{:.0f}"),
    ("convergence cpu, incl idle (s)", lambda r: r["create"]["cpuSeconds"] + r["settle"]["cpuSeconds"] + r["idle"]["cpuSeconds"], "{:.2f}"),
    ("convergence alloc, incl idle (MiB)", lambda r: (r["create"]["allocBytes"] + r["settle"]["allocBytes"] + r["idle"]["allocBytes"]) / 2**20, "{:.0f}"),
    ("settle cpu (s)", lambda r: r["settle"]["cpuSeconds"], "{:.2f}"),
    # settle alone ends when writes stop, which is not when work stops; settle+idle is
    # the honest cost of converging.
    ("idle cpu after writes stop (s)", lambda r: r["idle"]["cpuSeconds"], "{:.2f}"),
    ("settle+idle cpu (s)", lambda r: r["settle"]["cpuSeconds"] + r["idle"]["cpuSeconds"], "{:.2f}"),
    ("settle+idle alloc (MiB)", lambda r: (r["settle"]["allocBytes"] + r["idle"]["allocBytes"]) / 2**20, "{:.0f}"),
    ("settle alloc (MiB)", lambda r: r["settle"]["allocBytes"] / 2**20, "{:.0f}"),
    ("churn cpu, constant churn (s)", lambda r: r["churnNeutral"]["cpuSeconds"] + r["churnStatus"]["cpuSeconds"], "{:.2f}"),
    ("churn alloc, constant churn (MiB)", lambda r: (r["churnNeutral"]["allocBytes"] + r["churnStatus"]["allocBytes"]) / 2**20, "{:.0f}"),
    ("live heap (MiB)", lambda r: r["steadyHeap"]["heapAllocBytes"] / 2**20, "{:.0f}"),
    ("heap span inuse, secondary (MiB)", lambda r: r["steadyHeap"]["heapInuseBytes"] / 2**20, "{:.0f}"),
    ("peak rss, secondary (MiB)", lambda r: r["maxRSSBytes"] / 2**20, "{:.0f}"),
    ("route writes per route", lambda r: (r["create"]["routeStatusWrites"] + r["settle"]["routeStatusWrites"]) / r["config"]["routes"], "{:.2f}"),
	("convergence wall (s)", lambda r: r["load"]["convergenceWallSeconds"], "{:.2f}"),
	("p95 route status staleness (s)", lambda r: r["load"]["p95StalenessSeconds"], "{:.2f}"),
	("max route status staleness (s)", lambda r: r["load"]["maxStalenessSeconds"], "{:.2f}"),
	("successful status write qps", lambda r: r["load"]["writeQPS"], "{:.1f}"),
	("status write conflicts", lambda r: r["load"]["conflicts"], "{:.0f}"),
    ("gateway writes, batching-sensitive", lambda r: r["create"]["gatewayStatusWrites"] + r["settle"]["gatewayStatusWrites"], "{:.0f}"),
]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", required=True)
    ap.add_argument("--ref", default="main",
                    help="reference tree for ratios (default: main, if present)")
    args = ap.parse_args()

    scales = []
    for d in glob.glob(os.path.join(args.out, "scale-*")):
        m = re.match(r"scale-(\d+)$", os.path.basename(d))
        if m:
            scales.append(int(m.group(1)))
    scales.sort()
    if not scales:
        print(f"no scale-* directories under {args.out}", file=sys.stderr)
        return 1

    # Tree order is taken from the largest complete scale so the reference column
    # is stable even if a big scale partly failed.
    sides = []
    for scale in scales:
        for d in sorted(glob.glob(os.path.join(args.out, f"scale-{scale}", "*"))):
            side = os.path.basename(d)
            if side not in sides and glob.glob(os.path.join(d, "rep*", "result-*.json")):
                sides.append(side)

    data = {}
    for scale in scales:
        for side in sides:
            runs = []
            for p in sorted(glob.glob(os.path.join(args.out, f"scale-{scale}", side, "rep*", "result-*.json"))):
                with open(p) as f:
                    runs.append(json.load(f))
            if runs:
                data[(scale, side)] = runs

    # Ratios are only meaningful against the baseline being argued about, so put the
    # reference tree first rather than taking whatever sorted() returned.
    ref = args.ref if args.ref in sides else sides[0]
    sides = [ref] + [s for s in sides if s != ref]
    print(f"scale sweep: reference tree = {ref}; cells are median (ratio vs {ref} at that scale)")
    print(f"reps per cell: " + ", ".join(
        f"{scale}: " + "/".join(str(len(data.get((scale, s), []))) for s in sides) for scale in scales))
    print()

    for label, fn, fmt in METRICS:
        print(label)
        header = f"  {'routes':>8}" + "".join(f"{s:>22}" for s in sides)
        print(header)
        print("  " + "-" * (len(header) - 2))
        def vals(runs):
            out = []
            for r in runs or []:
                try:
                    out.append(fn(r))
                except (KeyError, TypeError):
                    pass
            return out

        for scale in scales:
            row = f"  {scale:>8}"
            ref_vals = vals(data.get((scale, ref)))
            ref_med = statistics.median(ref_vals) if ref_vals else None
            for side in sides:
                side_vals = vals(data.get((scale, side)))
                if not side_vals:
                    row += f"{'-':>22}"
                    continue
                med = statistics.median(side_vals)
                cell = fmt.format(med)
                if side != ref and ref_med:
                    cell += f" ({med / ref_med:.2f}x)"
                row += f"{cell:>22}"
            print(row)
        # Growth factor across the scale range makes O(N) vs flat obvious.
        if len(scales) > 1:
            growth = f"  {'growth':>8}"
            for side in sides:
                lo, hi = vals(data.get((scales[0], side))), vals(data.get((scales[-1], side)))
                if not lo or not hi:
                    growth += f"{'-':>22}"
                    continue
                lo_med = statistics.median(lo)
                hi_med = statistics.median(hi)
                growth += f"{(hi_med / lo_med if lo_med else float('nan')):>21.2f}x"
            print(growth + f"   <- {scales[0]} -> {scales[-1]} routes")
        print()
    return 0


if __name__ == "__main__":
    sys.exit(main())
