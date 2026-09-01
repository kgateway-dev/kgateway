# Control plane scale footprint

`TestScaleFootprint` runs the whole kgateway control plane in-process against
envtest, loads it with N Gateways and M HTTPRoutes, and measures what the process
costs at that scale. It exists to A/B two builds of the control plane — typically
a branch against `main` — not to assert a threshold, so it is skipped unless
`SCALEPERF` is set.

Everything is measured inside the test process. The envtest apiserver and etcd run
as separate processes, so their CPU is excluded and the numbers reflect control
plane work plus the test's own client work.

A run assumes it has the machine to itself. Nothing enforces that: a concurrent
build, a second measurement, or another agent driving the same harness will
contend for CPU and quietly invalidate the numbers. Check that the machine is idle
before starting a sweep, and establish the noise floor with the `--control` run
described under "Comparing two branches" before trusting any delta.

## What it measures

| Metric | Meaning |
| --- | --- |
| `baselineHeap` | post-GC live heap after startup, before the fixture exists |
| `steadyHeap` | post-GC live heap after every route carries our status and post-write work has gone idle |
| `create` | creating the fixture; CPU here is mostly the test's own client work |
| `settle` | from "objects exist" to "all routes have status and nothing is being written"; work can move across the create/settle boundary |
| `idle` | control-plane work that continues after writes stop; included in convergence totals and before the steady heap snapshot |
| `churnNeutral` | patching route hostnames: translation re-runs and only `observedGeneration` moves in the status, so exactly one route write per patch is correct. Anything above that, or any Gateway write at all, is churn |
| `churnStatus` | flipping a route's `backendRef` between a real and a missing Service, which forces a genuine `ResolvedRefs` transition each round; measures the cost of writes that must happen |
| `maxRSSBytes` | peak RSS of the test process (secondary — peak, not steady, and includes envtest client machinery) |
| `load.convergenceWallSeconds` | wall time from the start of fixture creation until every route has status and the watch is quiet |
| `load.routeWritesPerRoute` | watch-observed HTTPRoute status writes during initial convergence divided by route count |
| `load.writeQPS` | successful Gateway and HTTPRoute status writes divided by the active interval from the first write attempt to the last completion |
| `load.conflicts` | HTTP 409 responses from Gateway and HTTPRoute status writes during initial convergence |
| `load.p95StalenessSeconds` / `maxStalenessSeconds` | time from each HTTPRoute create submission to its first matching status event on the watch stream |

Both heap snapshots are taken after two forced collections. `HeapAlloc` is the
primary steady-memory metric because it measures live allocations. `HeapInuse`
measures allocator spans, includes unused space within spans, and can be bimodal;
it and peak RSS are reported as secondary diagnostics.

For load cost, compare `create + settle` CPU and allocation. Controllers process
objects while the fixture is still being submitted, so either phase alone is
sensitive to scheduling. The report also shows `create + settle + idle` as the
full convergence cost. Wall time is retained in JSON for diagnostics but omitted
from comparison tables: every convergence wait contains a configured quiet
interval, which dominates small performance differences.

Status writes are counted from a watch on HTTPRoutes and Gateways: a `MODIFIED`
event whose `metadata.generation` is unchanged from the previously observed
version is a status write, and a generation bump is one of the harness's own spec
patches. That makes the write counts independent of either build's metric names.

Always compare write counts summed over `create` **and** `settle`. A build that
converges faster lands more of its writes while objects are still being created,
so a single phase's count can look like a large reduction when the total is
unchanged or worse. `status_scale_report.py` reports the summed count and the
writes-per-route ratio for this reason.

Load-time Gateway write counts are informational, not a redundant-write score.
`AttachedRoutes` legitimately changes as routes arrive, and nondeterministic
translation batching can substantially change the number of intermediate
Gateway statuses. Gateway writes during neutral churn remain actionable because
the churn does not change Gateway status.

## Running one measurement

```bash
SCALEPERF=1 \
  SCALEPERF_LABEL=mybranch \
  SCALEPERF_GATEWAYS=10 \
  SCALEPERF_ROUTES=1000 \
  SCALEPERF_OUT=/tmp/scaleperf \
  go test -tags e2e -count=1 -timeout 60m -run TestScaleFootprint ./test/perf/statusscale/
```

Results land in `$SCALEPERF_OUT` as `result-<label>.json` and
`heap-<label>.pb.gz`, and a single `SCALEPERF_JSON {...}` line is printed to
stdout for scripted collection.

| Variable | Default | Purpose |
| --- | --- | --- |
| `SCALEPERF` | unset | must be set or the test skips |
| `SCALEPERF_LABEL` | `unlabeled` | names the output files |
| `SCALEPERF_GATEWAYS` | 10 | Gateways created |
| `SCALEPERF_ROUTES` | 1000 | HTTPRoutes created, spread over the Gateways |
| `SCALEPERF_SERVICES` | 20 | backend Services created |
| `SCALEPERF_CHURN_ROUNDS` | 3 | rounds in each churn phase |
| `SCALEPERF_CHURN_ROUTES` | 100 | routes patched per round |
| `SCALEPERF_QUIET` | `3s` | idle period on the watch that counts as converged |
| `SCALEPERF_TIMEOUT` | `5m` | per-phase timeout |
| `SCALEPERF_PARALLEL` | 8 | concurrent creates/patches |
| `SCALEPERF_WRITE_LATENCY` | `0s` | latency injected before each Gateway and HTTPRoute status request reaches the API server |
| `SCALEPERF_OUT` | test temp dir | output directory |
| `SCALEPERF_IDLE_CPUPROFILE` | unset | set to any value to CPU-profile the idle phase to `cpuidle-<label>.pprof` |
| `KGW_LOG_LEVEL` | `error` (forced) | log I/O is CPU; raise it only when debugging |

## Attributing the idle phase

At high route counts most CPU lands in the post-write idle phase, and the whole-run
heap profile cannot separate it from load. `SCALEPERF_IDLE_CPUPROFILE=1` writes a CPU
profile covering only that phase:

```bash
SCALEPERF_IDLE_CPUPROFILE=1 ... go test -tags e2e -run TestScaleFootprint ./test/perf/statusscale/
go tool pprof -top -cum -nodecount=200 cpuidle-<label>.pprof
```

Read it with `-cum` and filter to application frames: the flat view is dominated by
runtime GC and map internals, which are a symptom of the allocation rather than its
source. Profiling costs a few percent, so a run with it enabled is for attribution and
should not be compared against runs without it.

## Comparing two branches

```bash
hack/perf/status-scale-compare.sh --base main --reps 3 --routes 1000
```

The driver creates a detached worktree for the base ref, copies this harness into
it so both sides run byte-identical measurement code, alternates run order
(candidate/base, then base/candidate) so machine drift does not consistently
favor one side, and prints median comparisons with both sides' per-rep spreads.

Establish the noise floor before trusting any delta:

```bash
hack/perf/status-scale-compare.sh --base main --reps 3 --control
```

That runs `main` against `main`. If the control shows a 6% heap gap, a 6% branch
delta is noise.

## Attributing a heap delta

A number alone is weak; the profile diff shows the mechanism. The driver prints
the exact command for the median reps:

```bash
go tool pprof -top -nodecount=25 -diff_base=<base>/heap-base.pb.gz <cand>/heap-candidate.pb.gz
```

## Caveats

- Sweep the scale (0 / 1000 / 5000 routes). A win from removing per-object
  duplication grows with object count; a fixed offset does not.
- `GOGC`, `GOMAXPROCS` and machine load all move steady-state heap. The driver
  pins the first two; you have to handle the third.
- Peak RSS and `HeapInuse` are noisy for Go processes. Treat post-GC `HeapAlloc`
  as the primary live-memory number.
- Gateways use `selfManaged` GatewayParameters, so the deployer creates nothing
  per Gateway. This isolates translation and status, and understates the footprint
  of a real install.
- Services have no selector and envtest populates no EndpointSlices, so endpoint
  translation is not exercised.
