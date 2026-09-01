#!/usr/bin/env bash
# Run a single scale measurement in a named tree, with absolute paths. Verifies the
# result file exists before reporting success, so a skipped or misdirected run can't
# look like a pass.
#
# Assumes it has the machine to itself: a concurrent build or another measurement
# will contend for CPU and invalidate the numbers.
#
# Usage: hack/perf/run-one-scale.sh <label> <tree> <routes> <rep> <out-root>
set -euo pipefail

LABEL="$1"; TREE="$2"; ROUTES="$3"; REP="$4"; OUT_ROOT="$5"

TREE="$(cd "$TREE" && pwd)"
OUT="$(cd "$(dirname "$OUT_ROOT")" && pwd)/$(basename "$OUT_ROOT")/scale-$ROUTES/$LABEL/rep$REP"
mkdir -p "$OUT"

(
    cd "$TREE"
    env \
        GOMAXPROCS="${GOMAXPROCS:-4}" GOGC="${GOGC:-100}" \
        SCALEPERF=1 SCALEPERF_LABEL="$LABEL" \
        SCALEPERF_GATEWAYS="${GATEWAYS:-10}" SCALEPERF_ROUTES="$ROUTES" \
        SCALEPERF_SERVICES="${SERVICES:-20}" \
        SCALEPERF_CHURN_ROUNDS="${CHURN_ROUNDS:-3}" SCALEPERF_CHURN_ROUTES="${CHURN_ROUTES:-100}" \
        SCALEPERF_QUIET="${QUIET:-5s}" SCALEPERF_TIMEOUT="${TIMEOUT:-30m}" \
		SCALEPERF_WRITE_LATENCY="${WRITE_LATENCY:-0s}" \
        SCALEPERF_OUT="$OUT" \
        SCALEPERF_MEMPROFILERATE="${SCALEPERF_MEMPROFILERATE:-0}" \
        go test -tags e2e -count=1 -timeout 90m -run TestScaleFootprint ./test/perf/statusscale/ \
        >"$OUT/test.log" 2>&1
)

if [[ ! -f "$OUT/result-$LABEL.json" ]]; then
    echo "FAIL $LABEL routes=$ROUTES rep=$REP: no result file (skipped or misdirected) - $OUT/test.log" >&2
    exit 1
fi
echo "ok $LABEL routes=$ROUTES rep=$REP -> $OUT/result-$LABEL.json"
