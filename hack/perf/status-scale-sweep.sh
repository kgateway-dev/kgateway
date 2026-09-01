#!/usr/bin/env bash
# Sweep the scale footprint across several route counts and several trees, to test
# whether a cost grows with route count or stays flat.
#
# Churn stays constant (same rounds x routes patched) while total routes grow, so
# churn cost rising with scale means per-change work is O(all routes).
#
# Usage:
#   hack/perf/status-scale-sweep.sh \
#     --tree main=/path/to/main-worktree \
#     --tree fragments=/path/to/other-worktree \
#     --scales 1000,5000,10000 --reps 2 --out _output/scaleperf/sweep
set -euo pipefail

SCALES="1000,5000,10000"
REPS=2
GATEWAYS=10
SERVICES=20
CHURN_ROUNDS=3
CHURN_ROUTES=100
OUT=""
QUIET="5s"
TIMEOUT="30m"
WRITE_LATENCY="${WRITE_LATENCY:-0s}"
GOMAXPROCS_VAL="${GOMAXPROCS:-4}"
GOGC_VAL="${GOGC:-100}"
declare -a TREE_SPECS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --tree) TREE_SPECS+=("$2"); shift 2 ;;
        --scales) SCALES="$2"; shift 2 ;;
        --reps) REPS="$2"; shift 2 ;;
        --gateways) GATEWAYS="$2"; shift 2 ;;
        --services) SERVICES="$2"; shift 2 ;;
        --churn-rounds) CHURN_ROUNDS="$2"; shift 2 ;;
        --churn-routes) CHURN_ROUTES="$2"; shift 2 ;;
        --quiet) QUIET="$2"; shift 2 ;;
        --timeout) TIMEOUT="$2"; shift 2 ;;
		--write-latency) WRITE_LATENCY="$2"; shift 2 ;;
        --out) OUT="$2"; shift 2 ;;
        *) echo "unknown option: $1" >&2; exit 1 ;;
    esac
done

if [[ ${#TREE_SPECS[@]} -lt 2 ]]; then
    echo "need at least two --tree name=path arguments" >&2
    exit 1
fi

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"
OUT="${OUT:-$ROOT/_output/scaleperf/sweep}"
mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)"

# The harness must be byte-identical everywhere, and it is untracked on the
# measured refs, so copy it into each tree up front.
for spec in "${TREE_SPECS[@]}"; do
    tree="${spec#*=}"
    mkdir -p "$tree/test/perf/statusscale"
    cp "$ROOT/test/perf/statusscale/"*.go "$tree/test/perf/statusscale/"
    echo "==> $(echo "$spec" | cut -d= -f1): $tree ($(cd "$tree" && git rev-parse --short HEAD))"
    (cd "$tree" && go test -tags e2e -count=1 -run XXX_NONE ./test/perf/statusscale/ >/dev/null)
done
echo "==> scales: $SCALES; reps: $REPS; churn held constant at ${CHURN_ROUNDS}x${CHURN_ROUTES}"
echo "==> out: $OUT"

run_one() {
    local side="$1" tree="$2" routes="$3" rep="$4"
    local rep_out="$OUT/scale-$routes/$side/rep$rep"
    mkdir -p "$rep_out"
    local started
    started="$(date +%s)"
    if (
        cd "$tree"
        env \
            GOMAXPROCS="$GOMAXPROCS_VAL" GOGC="$GOGC_VAL" \
            SCALEPERF=1 SCALEPERF_LABEL="$side" \
            SCALEPERF_GATEWAYS="$GATEWAYS" SCALEPERF_ROUTES="$routes" \
            SCALEPERF_SERVICES="$SERVICES" \
            SCALEPERF_CHURN_ROUNDS="$CHURN_ROUNDS" SCALEPERF_CHURN_ROUTES="$CHURN_ROUTES" \
            SCALEPERF_QUIET="$QUIET" SCALEPERF_TIMEOUT="$TIMEOUT" \
			SCALEPERF_WRITE_LATENCY="$WRITE_LATENCY" \
            SCALEPERF_OUT="$rep_out" \
            go test -tags e2e -count=1 -timeout 90m -run TestScaleFootprint ./test/perf/statusscale/ \
            >"$rep_out/test.log" 2>&1
    ); then
        echo "    ok   $side routes=$routes rep=$rep ($(( $(date +%s) - started ))s)"
    else
        # Keep going: a failure at a large scale should not discard smaller scales.
        echo "    FAIL $side routes=$routes rep=$rep - $rep_out/test.log" >&2
        tail -5 "$rep_out/test.log" >&2 || true
    fi
}

IFS=',' read -ra SCALE_LIST <<< "$SCALES"
for routes in "${SCALE_LIST[@]}"; do
    echo "--> scale $routes"
    for rep in $(seq 1 "$REPS"); do
        # Reverse the tree order on even reps so machine drift cannot
        # systematically favor the first tree.
        if (( rep % 2 == 1 )); then
            for spec in "${TREE_SPECS[@]}"; do
                run_one "${spec%%=*}" "${spec#*=}" "$routes" "$rep"
            done
        else
            for ((i=${#TREE_SPECS[@]}-1; i>=0; i--)); do
                spec="${TREE_SPECS[$i]}"
                run_one "${spec%%=*}" "${spec#*=}" "$routes" "$rep"
            done
        fi
    done
done

echo
python3 "$ROOT/hack/perf/status_scale_sweep_report.py" --out "$OUT"
