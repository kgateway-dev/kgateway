# EP: Rerun Only the Failed TestKgateway Subtest in E2E CI

## Background

Our E2E CI shards run `make e2e-test`, which invokes gotestsum with
`--rerun-fails=$(GO_TEST_RETRIES)` (see `Makefile`, target `go-test`). Each
shard selects its suites with a `-run` filter passed through
`GO_TEST_USER_ARGS`, for example:

```
-run '^TestKgateway$/^OAuth$|^TestKgateway$/^TrafficPolicyStatus$'
```

gotestsum's retry mechanism replaces the original `-run` value with a regex
built from the names of the tests it saw fail:

1. If a leaf subtest fails (e.g. `TestKgateway/OAuth/TestSomething`),
   gotestsum reruns exactly that path. This is the behavior we want, and it
   already works.
2. If the **parent** `TestKgateway` fails directly — which happens whenever
   suite setup fails before any subtest starts (helm install error, CRD
   ownership conflict, cluster not ready) — gotestsum's rerun regex is
   `-run '^TestKgateway$'`. The shard's subtest filter is **gone**, so the
   rerun executes *every* suite registered in `KubeGatewaySuiteRunner`
   (LeaderElection ~8m, Deployer ~5.5m, EC2, TCPRoute, ...). That full-suite
   rerun routinely blows past the shard's `-timeout 25m`, panics, and
   produces a dozen "debris" failures unrelated to the original flake.

We have observed this cascade repeatedly in CI triage: a transient setup
failure becomes a 25-minute timeout panic with 13+ misleading failures.

A related hazard: when a helper calls `FailNow` on a parent `*testing.T`,
Go `Goexit`s the whole test subtree; subtests that never started produce no
test events, so gotestsum cannot rerun them and may exit green without
having executed them (false green). See golang/go#58129 and
gotestyourself/gotestsum#274, #341.

## Motivation

- Retries should never widen scope: a shard must only ever run the suites it
  was assigned, on the first run and on every rerun.
- A transient setup flake should cost one extra setup + the assigned suites,
  not a full-suite run and a timeout panic.
- CI triage should not have to distinguish "real failures" from "runaway
  rerun debris."

### Goals

- A failed leaf subtest is rerun as exactly that subtest (status quo,
  preserved).
- A failed `TestKgateway` parent (setup failure) is rerun with the shard's
  original filter, never `^TestKgateway$` alone.
- No change to how developers run tests locally.

### Non-goals

- Fixing individual setup flakes (e.g. CRD ownership races) — those are
  separate work; this proposal bounds their blast radius.
- Changing the sharding scheme or moving suites to top-level tests.
- Solving the gotestsum false-green problem in full generality (mitigated
  here, tracked upstream).

## Implementation Details

The core insight is that gotestsum rewrites `-run`, so any fix relying on
`-run` alone is fragile. Instead, the test binary itself enforces the shard
scope via an environment variable that survives reruns.

### 1. Shard scope enforced in the suite runner (primary change)

Add an optional env var, `E2E_SUITE_FILTER`, honored by the `SuiteRunner`
implementations in `test/e2e/suite.go`. It contains the shard's suite names
(comma-separated, e.g. `OAuth,TrafficPolicyStatus`). When set, `Run(...)`
skips any registered suite not in the list:

```go
func (u *suites) Run(ctx context.Context, t *testing.T, ti *TestInstallation) {
    allowed := suiteFilterFromEnv() // nil => run everything
    for testName, newSuite := range u.tests {
        if allowed != nil && !allowed[testName] {
            continue // not t.Skip: avoid emitting thousands of skip events
        }
        t.Run(testName, func(t *testing.T) { ... })
    }
}
```

The same check applies to `orderedSuites.Run`. Top-level tests that are not
`TestKgateway` (e.g. `TestKgatewayWaypoint`, `TestAPIValidation`) are
unaffected: they are selected purely by `-run` at the top level, and a
top-level name rerun (`^TestAPIValidation$`) is already correctly scoped.

With this in place, even when gotestsum reruns `-run '^TestKgateway$'`, the
binary only executes the shard's suites. The rerun cost of a setup failure
becomes setup + assigned suites — the same cost as the original attempt.

### 2. CI wiring

In `.github/workflows/e2e.yaml` (and the nightly matrix), add a
`suite-filter` field alongside `go-test-run-regex` for shards that select
`TestKgateway` subtests:

```yaml
- cluster-name: 'cluster-seven'
  go-test-args: '-timeout=25m'
  go-test-run-regex: '^TestAPIValidation$$|^TestKgateway$$/^OAuth$$|^TestKgateway$$/^TrafficPolicyStatus$$|^TestKgateway$$/^XdsStarvation$$'
  suite-filter: 'OAuth,TrafficPolicyStatus,XdsStarvation'
```

`.github/actions/kubernetes-e2e-tests/action.yaml` grows an optional
`suite-filter` input and exports `E2E_SUITE_FILTER` into the `make e2e-test`
environment.

To avoid the regex and the filter drifting apart, add a small CI lint step
(shell/jq in `hack/ci/`) that derives the expected suite list from
`go-test-run-regex` and fails if `suite-filter` disagrees. Longer term the
action's existing TODO ("accept this as a list of strings and build the
regex") lets us generate both from one list and delete the lint.

### 3. Guardrail against silent under-execution

If `E2E_SUITE_FILTER` names a suite that is not registered (typo, renamed
suite), `Run` fails the parent test with an explicit error listing the
unknown names. This keeps a stale filter from silently skipping coverage.

### 4. Keep `--rerun-fails-max-failures` bounded (already present in nightly)

No change needed to gotestsum flags. Leaf-subtest reruns continue to work
natively; the env filter only constrains which suites the parent will
consider.

## Test Plan

- Unit tests for the filter parsing and for `suites.Run`/`orderedSuites.Run`
  skipping behavior (table-driven, no Ginkgo), including the unknown-suite
  guardrail.
- Local verification: run
  `E2E_SUITE_FILTER=BasicRouting make e2e-test TEST_PKG=./test/e2e/tests GO_TEST_USER_ARGS='-run ^TestKgateway$'`
  and confirm only `BasicRouting` executes.
- CI simulation: on a branch, inject a one-shot setup failure (fail the
  first helm install attempt via a test hook), set `GO_TEST_RETRIES=2`, and
  confirm the rerun executes only the shard's suites and stays well under
  the 25m timeout.
- The regex/filter consistency lint runs in the existing workflow-lint job
  (`make lint-actions` step or adjacent).

## Alternatives

### A. Wrapper script owning retries (drop `--rerun-fails` for e2e)

A retry loop in `hack/` that reruns the whole shard command on failure
(shard filter preserved because we reuse the original `-run`). Optionally
smarter: parse gotestsum's `--jsonfile` to compute the failed-leaf set and
rerun only those, falling back to the original regex when only the parent
failed.

Rejected as primary: it reimplements gotestsum's retry logic, loses
`--rerun-fails-abort-on-data-race` semantics, and the naive form reruns
passing suites too. The jsonfile-parsing form is strictly more code than
the env filter for the same outcome. It remains a reasonable fallback if we
ever need retry logic gotestsum cannot express.

### B. `--rerun-fails-run-root-test`

Makes the problem worse: it forces reruns to the *root* test, i.e. all of
`TestKgateway`, which is exactly the failure mode we are eliminating.

### C. Promote every suite to a top-level `Test*` function

Would make gotestsum reruns naturally shard-scoped (root == suite) and
would also parallelize setup, but it is a large restructuring: per-suite
installs or a shared-installation redesign, changes to every shard regex,
and a different cost model per shard. Worth considering independently; not
required to fix the rerun-scope bug.

### D. Upstream gotestsum change

Teach gotestsum to intersect its rerun regex with the user's original
`-run` value. Cleanest conceptually, but we do not control release timing,
and the env-filter fix is small and local. We can still file the upstream
issue (related: gotestyourself/gotestsum#274) and drop our filter later.

## Open Questions

1. Should `E2E_SUITE_FILTER` also gate the nightly matrix's larger shards,
   or only the PR e2e workflow initially? (Proposal: both — nightly is
   where retries are most expensive.)
2. Skipped-not-listed suites emit no `t.Skip` events, so gotestsum's counts
   change slightly between a first run and a rerun. Is that acceptable for
   our Slack/nightly reporting, or should we emit a single summary log line
   listing filtered suites?
3. Do we want the consistency lint to *generate* `go-test-run-regex` from a
   suite list instead of checking agreement, per the existing TODO in
   `.github/actions/kubernetes-e2e-tests/action.yaml`?
