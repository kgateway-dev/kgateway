#!/usr/bin/env python3
"""Aggregate test/perf/statusscale result JSON files into an A/B comparison.

Reports the median of each metric per side plus the per-rep spread, so a small
median delta can be judged against the run-to-run noise instead of being read as
a result on its own.
"""

import argparse
import glob
import json
import os
import statistics
import sys

# (label, json path, unit, lower_is_better). None marks an informational metric
# for which the report must not infer better/worse.
METRICS = [
    ("live heap (HeapAlloc)", ("steadyHeap", "heapAllocBytes"), "bytes", True),
    ("live heap growth (steady-baseline)", ("__live_growth",), "bytes", True),
    ("steady heap objects", ("steadyHeap", "heapObjects"), "count", True),
    ("heap span inuse (secondary)", ("steadyHeap", "heapInuseBytes"), "bytes", None),
    ("heap span growth (secondary)", ("__inuse_growth",), "bytes", None),
    ("goroutines", ("steadyHeap", "goroutines"), "count", True),
    ("peak rss (secondary)", ("maxRSSBytes",), "bytes", None),
    ("total alloc (whole run)", ("totalAllocBytes",), "bytes", True),
    # Work crosses the create/settle boundary because controllers process objects
    # while the fixture is still being submitted. The sums are the stable load signal.
    ("load alloc (create+settle)", ("__load_alloc",), "bytes", True),
    ("load cpu (create+settle)", ("__load_cpu",), "seconds", True),
    ("convergence alloc (+post-write idle)", ("__convergence_alloc",), "bytes", True),
    ("convergence cpu (+post-write idle)", ("__convergence_cpu",), "seconds", True),
    ("settle alloc", ("settle", "allocBytes"), "bytes", True),
    ("idle alloc after writes stop", ("idle", "allocBytes"), "bytes", True),
    ("neutral churn alloc", ("churnNeutral", "allocBytes"), "bytes", True),
    ("status churn alloc", ("churnStatus", "allocBytes"), "bytes", True),
    ("settle cpu", ("settle", "cpuSeconds"), "seconds", True),
    ("idle cpu after writes stop", ("idle", "cpuSeconds"), "seconds", True),
    # Writes must be summed over create+settle: a faster build lands more of them
    # while objects are still being created, so a per-phase count alone is an
    # attribution artifact rather than a real difference.
    ("load route status writes (create+settle)", ("__load_route_writes",), "count", True),
    ("route status writes per route", ("__writes_per_route",), "ratio", True),
    ("load gw writes (batching-sensitive)", ("__load_gw_writes",), "count", None),
    ("settle route writes (phase-sensitive)", ("settle", "routeStatusWrites"), "count", None),
    ("neutral churn cpu", ("churnNeutral", "cpuSeconds"), "seconds", True),
    ("neutral churn route writes (1/patch expected)", ("churnNeutral", "routeStatusWrites"), "count", True),
    ("neutral churn gw writes (0 expected)", ("churnNeutral", "gatewayStatusWrites"), "count", True),
    ("status churn cpu", ("churnStatus", "cpuSeconds"), "seconds", True),
    ("status churn route writes", ("churnStatus", "routeStatusWrites"), "count", True),
]


def dig(obj, path):
    if path == ("__live_growth",):
        return obj["steadyHeap"]["heapAllocBytes"] - obj["baselineHeap"]["heapAllocBytes"]
    if path == ("__inuse_growth",):
        return obj["steadyHeap"]["heapInuseBytes"] - obj["baselineHeap"]["heapInuseBytes"]
    if path == ("__load_alloc",):
        return obj["create"]["allocBytes"] + obj["settle"]["allocBytes"]
    if path == ("__load_cpu",):
        return obj["create"]["cpuSeconds"] + obj["settle"]["cpuSeconds"]
    if path == ("__convergence_alloc",):
        return dig(obj, ("__load_alloc",)) + obj["idle"]["allocBytes"]
    if path == ("__convergence_cpu",):
        return dig(obj, ("__load_cpu",)) + obj["idle"]["cpuSeconds"]
    if path == ("__load_route_writes",):
        return obj["create"]["routeStatusWrites"] + obj["settle"]["routeStatusWrites"]
    if path == ("__load_gw_writes",):
        return obj["create"]["gatewayStatusWrites"] + obj["settle"]["gatewayStatusWrites"]
    if path == ("__writes_per_route",):
        return dig(obj, ("__load_route_writes",)) / obj["config"]["routes"]
    for key in path:
        obj = obj[key]
    return obj


def load_side(out_dir, side):
    runs = []
    for path in sorted(glob.glob(os.path.join(out_dir, side, "rep*", "result-*.json"))):
        with open(path) as f:
            data = json.load(f)
        data["__path"] = path
        runs.append(data)
    return runs


def fmt(value, unit):
    if unit == "bytes":
        v = float(value)
        sign = "-" if v < 0 else ""
        v = abs(v)
        for suffix, scale in (("GiB", 1 << 30), ("MiB", 1 << 20), ("KiB", 1 << 10)):
            if v >= scale:
                return f"{sign}{v / scale:.2f}{suffix}"
        return f"{sign}{v:.0f}B"
    if unit == "seconds":
        return f"{float(value):.2f}s"
    if unit == "ratio":
        return f"{float(value):.2f}x"
    return f"{value:g}" if isinstance(value, float) else str(value)


def spread(values, unit):
    if len(values) < 2:
        return ""
    lo, hi = min(values), max(values)
    med = statistics.median(values)
    pct = (hi - lo) / med * 100 if med else 0.0
    return f"[{fmt(lo, unit)}..{fmt(hi, unit)}] {pct:.1f}%"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", required=True)
    ap.add_argument("--candidate", default="candidate")
    ap.add_argument("--candidate-desc", default="candidate")
    ap.add_argument("--base-desc", default="base")
    args = ap.parse_args()

    base = load_side(args.out, "base")
    cand = load_side(args.out, args.candidate)
    if not base or not cand:
        print(f"no results found under {args.out} (base: {len(base)}, {args.candidate}: {len(cand)})",
              file=sys.stderr)
        return 1

    cfg = cand[0]["config"]
    print("=" * 150)
    print(f"control plane scale footprint: {args.candidate_desc}  vs  {args.base_desc}")
    print(f"scale: {cfg['gateways']} gateways, {cfg['routes']} routes, {cfg['services']} services; "
          f"churn {cfg['churnRounds']} rounds x {cfg['churnRoutes']} routes")
    print(f"reps: {len(cand)} candidate, {len(base)} base; "
          f"GOMAXPROCS={cand[0]['gomaxprocs']} GOGC={cand[0]['gogc']} "
          f"{cand[0]['goos']}/{cand[0]['goarch']}")
    print("=" * 150)

    header = (f"{'metric':43} {'base':>12} {'candidate':>12} {'delta':>12} {'delta %':>9}  "
              f"{'base spread':>24}  {'candidate spread':>24}")
    print(header)
    print("-" * len(header))

    for label, path, unit, lower_better in METRICS:
        try:
            b_vals = [dig(r, path) for r in base]
            c_vals = [dig(r, path) for r in cand]
        except (KeyError, TypeError):
            continue
        b_med = statistics.median(b_vals)
        c_med = statistics.median(c_vals)
        delta = c_med - b_med
        pct = (delta / b_med * 100) if b_med else float("nan")
        marker = ""
        if lower_better is not None and b_med and abs(pct) >= 5:
            improved = delta < 0 if lower_better else delta > 0
            marker = "  <-- better" if improved else "  <-- WORSE"
        print(f"{label:43} {fmt(b_med, unit):>12} {fmt(c_med, unit):>12} "
              f"{fmt(delta, unit):>12} {pct:>8.1f}%  {spread(b_vals, unit):>24}  "
              f"{spread(c_vals, unit):>24}{marker}")

    print()
    print("per-rep live heap (HeapAlloc):")
    for side, runs in (("base", base), (args.candidate, cand)):
        vals = ", ".join(fmt(dig(r, ("steadyHeap", "heapAllocBytes")), "bytes") for r in runs)
        print(f"  {side:>10}: {vals}")

    # Point at the median reps' heap profiles: the profile diff is what explains a
    # heap delta, and it is the part reviewers actually check.
    def median_profile(runs):
        ordered = sorted(runs, key=lambda r: dig(r, ("steadyHeap", "heapAllocBytes")))
        return ordered[len(ordered) // 2].get("heapProfile", "")

    b_prof, c_prof = median_profile(base), median_profile(cand)
    if b_prof and c_prof and os.path.exists(b_prof) and os.path.exists(c_prof):
        print()
        print("attribute the heap delta (median reps):")
        print(f"  go tool pprof -top -nodecount=25 -diff_base={b_prof} {c_prof}")
        print(f"  go tool pprof -http=: -diff_base={b_prof} {c_prof}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
