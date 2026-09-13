#!/usr/bin/env bash
# A/B the control plane's scale footprint between the current worktree and a base
# ref, using test/perf/statusscale. Reps alternate A/B then B/A so machine drift
# affects both sides equally, and the identical harness file is copied into the base
# worktree so both sides run the same measurement code.
#
# Usage:
#   hack/perf/status-scale-compare.sh [--base main] [--reps 3] [--routes 1000] ...
#   hack/perf/status-scale-compare.sh --control   # base vs base: the noise floor
set -euo pipefail

BASE_REF="main"
CANDIDATE_REF=""
CANDIDATE_TREE_IN=""
REPS=3
GATEWAYS=10
ROUTES=1000
SERVICES=20
CHURN_ROUNDS=3
CHURN_ROUTES=100
OUT=""
CONTROL=0
GOMAXPROCS_VAL="${GOMAXPROCS:-4}"
GOGC_VAL="${GOGC:-100}"
QUIET="5s"
TIMEOUT="10m"

usage() {
    sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'
    cat <<'EOF'

Options:
  --base <ref>          baseline git ref (default: main)
  --candidate-ref <ref> measure this ref instead of the current worktree
  --candidate-tree <d>  measure this existing directory as-is (uncommitted work
                        included); the harness is copied into it
  --reps <n>            measurement reps per side (default: 3)
  --gateways <n>        Gateways to create (default: 10)
  --routes <n>          HTTPRoutes to create (default: 1000)
  --services <n>        Services to create (default: 20)
  --churn-rounds <n>    churn rounds per churn phase (default: 3)
  --churn-routes <n>    routes patched per churn round (default: 100)
  --quiet <dur>         idle period that counts as converged (default: 5s)
  --timeout <dur>       per-phase timeout (default: 10m)
  --out <dir>           output dir (default: _output/scaleperf)
  --control             run base vs base to measure the noise floor
  -h, --help            show this help
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --base) BASE_REF="$2"; shift 2 ;;
        --candidate-ref) CANDIDATE_REF="$2"; shift 2 ;;
        --candidate-tree) CANDIDATE_TREE_IN="$2"; shift 2 ;;
        --reps) REPS="$2"; shift 2 ;;
        --gateways) GATEWAYS="$2"; shift 2 ;;
        --routes) ROUTES="$2"; shift 2 ;;
        --services) SERVICES="$2"; shift 2 ;;
        --churn-rounds) CHURN_ROUNDS="$2"; shift 2 ;;
        --churn-routes) CHURN_ROUTES="$2"; shift 2 ;;
        --quiet) QUIET="$2"; shift 2 ;;
        --timeout) TIMEOUT="$2"; shift 2 ;;
        --out) OUT="$2"; shift 2 ;;
        --control) CONTROL=1; shift ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown option: $1" >&2; usage; exit 1 ;;
    esac
done

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"
OUT="${OUT:-$ROOT/_output/scaleperf}"
mkdir -p "$OUT"
# Measured runs cd into a worktree, so a relative --out would resolve there.
OUT="$(cd "$OUT" && pwd)"
# Worktrees live outside OUT so repeated comparisons reuse them (and their build
# caches) instead of recreating one per output directory.
TREES="$ROOT/_output/scaleperf-trees"
mkdir -p "$TREES"

CANDIDATE_SHA="$(git rev-parse --short HEAD)"
CANDIDATE_NAME="$(git rev-parse --abbrev-ref HEAD)"
BASE_SHA="$(git rev-parse --short "$BASE_REF")"

if [[ -n "$(git status --porcelain -- ':!_output' 2>/dev/null)" ]]; then
    echo "note: working tree is dirty; the candidate side measures the working tree as-is" >&2
fi

# Base runs in a detached worktree so the current checkout is untouched.
BASE_TREE="$TREES/base-$BASE_SHA"
if [[ ! -d "$BASE_TREE" ]]; then
    echo "==> creating base worktree at $BASE_TREE ($BASE_REF @ $BASE_SHA)"
    git worktree add --detach "$BASE_TREE" "$BASE_REF" >/dev/null
fi
# The harness must be byte-identical on both sides.
mkdir -p "$BASE_TREE/test/perf/statusscale"
cp "$ROOT/test/perf/statusscale/"*.go "$BASE_TREE/test/perf/statusscale/"

# An explicit directory: measured exactly as it sits, uncommitted work included.
if [[ -n "$CANDIDATE_TREE_IN" ]]; then
    CANDIDATE_TREE="$(cd "$CANDIDATE_TREE_IN" && pwd)"
    CANDIDATE_LABEL="candidate"
    mkdir -p "$CANDIDATE_TREE/test/perf/statusscale"
    cp "$ROOT/test/perf/statusscale/"*.go "$CANDIDATE_TREE/test/perf/statusscale/"
    CANDIDATE_DESC="$(cd "$CANDIDATE_TREE" && git rev-parse --short HEAD) tree $CANDIDATE_TREE"
# A worktree candidate: either the control (base vs base) or an explicit ref. Both
# get the harness copied in, since it is untracked on the measured ref.
elif [[ "$CONTROL" == "1" || -n "$CANDIDATE_REF" ]]; then
    if [[ "$CONTROL" == "1" ]]; then
        CANDIDATE_REF="$BASE_REF"
        CANDIDATE_LABEL="control"
    else
        CANDIDATE_LABEL="candidate"
    fi
    CAND_SHA="$(git rev-parse --short "$CANDIDATE_REF")"
    CANDIDATE_TREE="$TREES/cand-$CANDIDATE_LABEL-$CAND_SHA"
    if [[ ! -d "$CANDIDATE_TREE" ]]; then
        echo "==> creating candidate worktree at $CANDIDATE_TREE ($CANDIDATE_REF @ $CAND_SHA)"
        git worktree add --detach "$CANDIDATE_TREE" "$CANDIDATE_REF" >/dev/null
    fi
    mkdir -p "$CANDIDATE_TREE/test/perf/statusscale"
    cp "$ROOT/test/perf/statusscale/"*.go "$CANDIDATE_TREE/test/perf/statusscale/"
    CANDIDATE_DESC="$CANDIDATE_REF @ $CAND_SHA"
    [[ "$CONTROL" == "1" ]] && CANDIDATE_DESC="$CANDIDATE_DESC (control)"
else
    CANDIDATE_TREE="$ROOT"
    CANDIDATE_LABEL="candidate"
    CANDIDATE_DESC="$CANDIDATE_NAME @ $CANDIDATE_SHA"
fi

echo "==> candidate: $CANDIDATE_DESC"
echo "==> base:      $BASE_REF @ $BASE_SHA"
echo "==> scale:     ${GATEWAYS} gateways, ${ROUTES} routes, ${SERVICES} services"
echo "==> churn:     ${CHURN_ROUNDS} rounds x ${CHURN_ROUTES} routes (neutral + status)"
echo "==> reps:      $REPS per side, alternating A/B order; GOMAXPROCS=$GOMAXPROCS_VAL GOGC=$GOGC_VAL"
echo "==> out:       $OUT"

# Warm both build caches first so compile time never lands inside a measured run.
for tree in "$CANDIDATE_TREE" "$BASE_TREE"; do
    (cd "$tree" && go test -tags e2e -count=1 -run XXX_NONE ./test/perf/statusscale/ >/dev/null)
done

run_one() {
    local tree="$1" side="$2" rep="$3"
    local rep_out="$OUT/$side/rep$rep"
    mkdir -p "$rep_out"
    echo "--> $side rep $rep"
    (
        cd "$tree"
        env \
            GOMAXPROCS="$GOMAXPROCS_VAL" \
            GOGC="$GOGC_VAL" \
            SCALEPERF=1 \
            SCALEPERF_LABEL="$side" \
            SCALEPERF_GATEWAYS="$GATEWAYS" \
            SCALEPERF_ROUTES="$ROUTES" \
            SCALEPERF_SERVICES="$SERVICES" \
            SCALEPERF_CHURN_ROUNDS="$CHURN_ROUNDS" \
            SCALEPERF_CHURN_ROUTES="$CHURN_ROUTES" \
            SCALEPERF_QUIET="$QUIET" \
            SCALEPERF_TIMEOUT="$TIMEOUT" \
            SCALEPERF_OUT="$rep_out" \
            SCALEPERF_MEMPROFILERATE="${SCALEPERF_MEMPROFILERATE:-0}" \
            go test -tags e2e -count=1 -timeout 60m -run TestScaleFootprint ./test/perf/statusscale/ \
            >"$rep_out/test.log" 2>&1
    ) || { echo "    FAILED - see $rep_out/test.log" >&2; tail -20 "$rep_out/test.log" >&2; exit 1; }
}

for rep in $(seq 1 "$REPS"); do
    # Alternate A/B then B/A so a consistent thermal or background-load drift
    # cannot systematically favor the side that always runs first.
    if (( rep % 2 == 1 )); then
        run_one "$CANDIDATE_TREE" "$CANDIDATE_LABEL" "$rep"
        run_one "$BASE_TREE" "base" "$rep"
    else
        run_one "$BASE_TREE" "base" "$rep"
        run_one "$CANDIDATE_TREE" "$CANDIDATE_LABEL" "$rep"
    fi
done

echo
python3 "$ROOT/hack/perf/status_scale_report.py" \
    --out "$OUT" \
    --candidate "$CANDIDATE_LABEL" \
    --candidate-desc "$CANDIDATE_DESC" \
    --base-desc "$BASE_REF @ $BASE_SHA"
