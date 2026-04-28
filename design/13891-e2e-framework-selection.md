# EP-13891: E2E Framework Selection

* Issue: [13891](https://github.com/kgateway-dev/kgateway/issues/13891)
* Parent epic: [Modernize and improve kgateway end-to-end testing](https://github.com/kgateway-dev/kgateway/issues/13783)
* Reference PRs:
  * [#13782 — fast e2e tests](https://github.com/kgateway-dev/kgateway/pull/13782)
  * [#13890 — basicrouting POC using sigs/e2e-framework](https://github.com/kgateway-dev/kgateway/pull/13890)
  * [#12981 — Good tests](https://github.com/kgateway-dev/kgateway/issues/12981)
  * [#12993 — initial fast e2e attempt](https://github.com/kgateway-dev/kgateway/pull/12993)

## Background

kgateway maintains a large suite of end-to-end (e2e) tests that exercise the full path from Kubernetes Gateway API resources through the kgateway control plane to the dataplane proxies (Envoy and agentgateway). These tests live under [`test/e2e/`](../test/e2e/) and use a custom framework built on top of [`testify/suite`](https://pkg.go.dev/github.com/stretchr/testify/suite).

The framework has accumulated significant capability over time, but it is also showing strain. It is slow, idiosyncratic, and difficult for new contributors to learn. The parent epic asks us to evaluate whether kgateway should keep evolving the existing framework or migrate (in whole or in part) to one of the established alternatives in the Kubernetes ecosystem.

This design document compares three candidates, recommends a path forward, and proposes a migration strategy.

## Goals

* Compare the existing kgateway custom e2e framework against the [`sigs.k8s.io/e2e-framework`](https://github.com/kubernetes-sigs/e2e-framework) project and the Gateway API conformance framework.
* Reach consensus with kgateway maintainers on which framework new e2e tests should be written against.
* Define a pattern that can be applied to migrate or modernize existing tests.
* Give contributors a clear reference for choosing the right framework for a given test.

## Non-Goals

* Migrate every existing e2e test as part of this proposal. The migration scope is defined later in the epic.
* Replace the Gateway API conformance test runner. The conformance suite is consumed as-is from upstream and is not the same surface as kgateway's feature e2e tests.
* Change the way kgateway is installed for tests (Helm-from-local-chart). That concern is orthogonal to framework choice.
* Replace unit tests, gateway translator tests, or load tests. This document is scoped to functional e2e.

## Frameworks Under Consideration

### Framework A — Current kgateway custom framework

Location: [`test/e2e/`](../test/e2e/), with the suite-level base in [`test/e2e/tests/base/base_suite.go`](../test/e2e/tests/base/base_suite.go).

The framework is built around three central abstractions:

* `TestInstallation` ([`test/e2e/test.go`](../test/e2e/test.go)) — bundles a runtime context, cluster context, install context, an `Actions` provider (Helm, kubectl, curl wrappers), an `Assertions` provider (Gomega-based helpers), and a per-test failure dump directory.
* `BaseTestingSuite` — embeds `testify/suite.Suite` and wires the test lifecycle (`SetupSuite`, `BeforeTest`, `AfterTest`, `TearDownSuite`) to manifest application, image pre-pulling, dynamic resource discovery, and Gateway API version gating.
* `SuiteRunner` ([`test/e2e/suite.go`](../test/e2e/suite.go)) — registers and runs a set of `testify` suites against a single `TestInstallation`.

Tests are organized as `features/<area>/suite.go` and registered in `tests/<entrypoint>_tests.go`. Each test method on a suite struct is treated as a Go subtest.

Strengths:

* **Tight kgateway integration.** Helm install flow, failure dump, image pre-pull, dynamic proxy resource awaiting, and Gateway API version/channel guards are all built in.
* **Mature assertion library.** `assertions.Provider` exposes `AssertEventualCurlResponse`, `AssertEventuallyConsistentCurlResponse`, `EventuallyGatewayAddress`, etc. — purpose-built for this product.
* **Manifest-first authoring.** `TestCase{Manifests: []string{...}}` matches how users actually drive kgateway (`kubectl apply -f`).
* **Failure forensics.** On failure the framework dumps namespace state, controller logs, and resource descriptions to a per-test directory. This is invaluable in CI.
* **Persistence flags.** `PERSIST_INSTALL`, `FAIL_FAST_AND_PERSIST`, and `SKIP_INSTALL` enable iterative local debugging without paying the install cost on every run.

Weaknesses (these are the cracks the epic calls out):

* **Slow per-test cycle.** Each suite re-applies its setup manifests and waits for pods. With pre-pull, `EventuallyObjectsExist`, dynamic resource discovery, and `EventuallyPodsRunning`, a single test can spend tens of seconds on setup before it ever issues a curl.
* **Heavy abstraction.** A new contributor has to learn `TestInstallation`, `BaseTestingSuite`, `TestCase`, `Setup`/`SetupByVersion`, `Actions.*`, `AssertionsT(t).*`, the `SuiteRunner`, and the difference between `Setup` and per-test `TestCases` before they can write a basic routing test. The actual test method is often the smallest part of the file.
* **Testify suite sharp edges.** `testify/suite` runs methods named `TestX` by reflection; misnamed methods silently don't run. Subtests share the suite's `*testing.T` unless the suite carefully threads the per-test `T` (kgateway works around this with `AssertionsT(t)` after [past confusion with the deprecated `Assertions`](../test/e2e/test.go)).
* **Coupled installation and execution.** A `TestInstallation` is per-entrypoint, so testing kgateway under multiple Helm value sets requires a new `*_test.go` file *and* a new GitHub Actions invocation. This is the "1:1:1 relationship" called out in [`test/e2e/README.md`](../test/e2e/README.md).
* **Bespoke knowledge.** None of this transfers to other Kubernetes projects. Reviewers from outside the project pay a learning tax.

### Framework B — `sigs.k8s.io/e2e-framework`

Upstream project: <https://github.com/kubernetes-sigs/e2e-framework>. A POC migration of the basicrouting tests lives at [`test/e2e_sigs/`](../test/e2e_sigs/) ([#13890](https://github.com/kgateway-dev/kgateway/pull/13890)).

The framework provides a Go-test-native programming model:

* `env.Environment` is the single object that owns lifecycle. `TestMain` constructs it, optionally registers `Setup` / `Finish` steps (e.g., create a kind cluster, install CRDs, deploy the controller), and calls `testenv.Run(m)`.
* Tests are plain `func TestX(t *testing.T)` functions. They build one or more `features.Feature` values using a fluent builder (`features.New(name).WithLabel(...).Setup(...).Assess(...).Teardown(...).Feature()`) and execute them with `testenv.Test(t, feat)`.
* `envconf.Config` carries the cluster client, namespace, kubeconfig path, and CLI flags — it is threaded into every step's closure.
* `envfuncs` provides reusable building blocks (`CreateCluster`, `CreateNamespace`, `LoadDockerImageToCluster`, etc.) that can be composed into `Setup`/`Finish` chains.

Strengths:

* **Standard Go test idioms.** No reflection-based suite runner. `go test -run TestGatewayWithRoute -v ./test/e2e_sigs/features/basicrouting` works the way every Go developer expects.
* **Composable lifecycle.** Each feature owns its own `Setup` / `Assess` / `Teardown`. Per-feature state lives in the `context.Context` returned from each step, so there is no shared mutable suite struct.
* **Labels and feature gates.** `WithLabel("type", "smoke")` plus `--feature` / `--labels` flags let CI pick subsets without restructuring code.
* **Familiar to the community.** The framework is used by Crossplane, kueue, and several other CNCF projects. New contributors who have written tests for those projects will be immediately productive.
* **Light dependency surface.** The package is small, focused, and stable. No reflection magic on test method names.

Weaknesses:

* **No batteries for kgateway-specific concerns.** Helm-install-from-local-chart, failure dumps, image pre-pull, dynamic proxy resource discovery, and Gateway API version gating do not exist out of the box. We would have to port these or pay the cost in flake and triage.
* **Manifest application is bring-your-own.** The framework gives you a controller-runtime client and `decoder.DecodeEachFile` helpers, but the polished "apply this YAML, then wait for the dynamically created proxy Deployment, then await pods running" flow we have in `BaseTestingSuite.ApplyManifests` is not provided.
* **No `testify/suite`-style fixtures.** If a group of tests genuinely shares expensive setup, you end up either using package-level `TestMain` setup (cluster-wide) or threading state through `context.Context` manually. Mid-grain "suite" fixtures are awkward.
* **Per-feature setup overhead.** The fluent `Setup -> Assess -> Teardown` per feature can mean re-doing work that was previously amortized at the `SetupSuite` level, unless we are deliberate about which steps live at `TestMain` vs. per-feature.
* **Assertion style.** The framework intentionally takes no opinion on assertions. Tests in the wider community use a mix of `t.Fatal`, `require`, and `gomega`. The basicrouting POC pulls in Gomega via [`assertions/assertions.go`](../test/e2e_sigs/assertions/assertions.go) — that pattern works but is something we own, not something the framework gives us.

### Framework C — Gateway API conformance framework

Location (vendored at the project root for inspection): [`gateway-api/conformance/`](../gateway-api/conformance/). Upstream lives at `sigs.k8s.io/gateway-api/conformance/`.

* `ConformanceTestSuite` ([`gateway-api/conformance/utils/suite/suite.go`](../gateway-api/conformance/utils/suite/suite.go)) is the runner. It is constructed once per `TestMain` with the `GatewayClassName`, `ControllerName`, base manifests, and supported features, then `suite.Run(t, tests)` iterates the registered `ConformanceTest` values.
* Each test is a `ConformanceTest` struct: `ShortName`, `Description`, required `Features`, `Manifests`, and a single `Test` function. Tests register themselves through `init()` blocks into a global `ConformanceTests` slice.
* A rich helper library lives under `gateway-api/conformance/utils/`: `kubernetes.GatewayAndHTTPRoutesMustBeAccepted`, `http.MakeRequestAndExpectEventuallyConsistentResponse`, the `RoundTripper` abstraction, `tlog`, etc.
* The framework knows how to gate tests by supported features and to skip provisional tests; it produces a structured conformance report.

Strengths:

* **Authoritative for Gateway API behavior.** When the question is "does kgateway conform to the spec for HTTPRoute path matching?", this is exactly the right tool. We already run it via `make conformance` / `make all-conformance`.
* **Battle-tested helpers.** `MakeRequestAndExpectEventuallyConsistentResponse`, `GatewayMustHaveAddress`, accepted-status checks — all built specifically around the Gateway API surface.
* **Feature gating by spec feature.** `Features: []features.FeatureName{...}` directly maps to the Gateway API feature catalog. Skipping is principled, not ad hoc.

Weaknesses:

* **Tests are tied to upstream definitions.** A `ConformanceTest` lives inside `sigs.k8s.io/gateway-api/conformance/tests/`. We cannot easily add kgateway-specific tests (e.g., for `TrafficPolicy`, `BackendConfigPolicy`, AI extensions, ExtAuth, ExtProc) into that catalog. Out-of-tree conformance tests are possible but contort the framework's intent.
* **Single-shape testing.** The framework expects "given this Gateway/HTTPRoute, does the implementation behave per spec?" It does not have first-class support for "given these kgateway Helm values, install kgateway, then test feature X" or "test against multiple installations in one CI job." Those concerns are out of scope.
* **No install management.** The conformance runner expects a Gateway API implementation to already be running. Helm install, upgrade, uninstall, persistence flags, and failure dumps are kgateway-specific and not provided.
* **Helpers are useful but not portable.** The `roundtripper`, `http`, `grpc`, `tls` packages are excellent for conformance but live in `internal-ish` paths that we should not depend on from product e2e tests; they evolve with the spec, not with kgateway's needs.

## Comparison

| Concern | Current kgateway | sigs/e2e-framework | Gateway API conformance |
|---|---|---|---|
| Authoring model | `testify/suite` + `BaseTestingSuite` | `func TestX` + `features.Feature` builder | `ConformanceTest` struct registered via `init()` |
| Helm install / uninstall | Built-in | Bring your own (or call Helm in a `Setup` envfunc) | Out of scope |
| Manifest apply + await | `ApplyManifests` with image pre-pull and dynamic resource discovery | `decoder.*` helpers; rest is bring your own | Per-test `Manifests` list + `kubernetes.*` helpers |
| Image pre-pull for flake reduction | Yes ([`base_suite.go:429`](../test/e2e/tests/base/base_suite.go#L429)) | No | No |
| Failure dumps | Yes (`PerTestPreFailHandler`) | No (build it) | No |
| Gateway API version gating | Yes (`MinGwApiVersion`/`MaxGwApiVersion` per channel) | No (build it) | Implicit via `Features` registry |
| Persistence/iterative debug flags | `PERSIST_INSTALL`, `FAIL_FAST_AND_PERSIST`, `SKIP_INSTALL` | None built-in | None |
| Filtering tests | Go test `-run` regex | `-run` + `--feature` / `--labels` flags | `--run-test`, supported-features set, skip lists |
| Assertion style | Gomega (`Eventually`) + Testify (`Require`) | Caller's choice (POC uses Gomega) | Custom helpers + `t.Fatal` / `require` |
| Parallelism story | Subtests in one suite share state; cross-suite parallelism is via `go test` parallelism only | `t.Parallel()` works naturally; features are independent | Single suite, sequential by default |
| Familiarity for new contributors | Low (kgateway-specific) | Medium (used across CNCF) | Medium (Gateway API community) |
| Coupling to install layout | High (1:1:1) | Low (lifecycle is composable) | High (single static install) |
| Speed of a minimal test | Slow — full `BaseTestingSuite` cycle | Fast — only what the feature needs | Fast — but only for spec tests |

## Comparison: same test in each framework

The basicrouting "Gateway with Route" test is implemented in two of the three frameworks today, which makes a side-by-side comparison concrete.

### Current framework — [`test/e2e/features/basicrouting/suite.go`](../test/e2e/features/basicrouting/suite.go)

```go
type testingSuite struct {
    *base.BaseTestingSuite
    localGateway common.Gateway
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
    return &testingSuite{
        base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
        common.Gateway{},
    }
}

func (s *testingSuite) SetupSuite() {
    s.BaseTestingSuite.SetupSuite()
    address := s.TestInstallation.Assertions.EventuallyGatewayAddress(s.Ctx, "gateway", "default")
    s.localGateway = common.Gateway{
        NamespacedName: types.NamespacedName{Name: "gateway", Namespace: "default"},
        Address:        address,
    }
}

func (s *testingSuite) TestGatewayWithRoute() { s.assertSuccessfulResponse() }
```

The suite needs two registration files (`suite.go` plus `tests/kgateway_tests.go`), a `TestCase` map, and a `BaseTestingSuite` embedding before the actual assertion runs.

### sigs/e2e-framework POC — [`test/e2e_sigs/features/basicrouting/routing_test.go`](../test/e2e_sigs/features/basicrouting/routing_test.go)

```go
func TestGatewayWithRoute(t *testing.T) {
    gomega.RegisterTestingT(t)
    var gatewayAddress string

    feat := features.New("Gateway with Route").
        Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
            gw := &gwv1.Gateway{}
            require.NoError(t, cfg.Client().Resources().Get(ctx, "test-gateway", "kgateway-test", gw))
            gatewayAddress = gw.Status.Addresses[0].Value
            return ctx
        }).
        Assess("successful response on all listeners", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
            for _, port := range []int{8080, 80} {
                assertions.AssertSuccessfulResponse(t, gatewayAddress, port)
            }
            return ctx
        }).
        Feature()

    testenv.Test(t, feat)
}
```

A standard Go test function, a fluent feature, and a small `assertions` package. The cost: the POC currently assumes resources are pre-applied (the README documents this explicitly), so what we save on framework ceremony we pay back in fixture management — until we port `ApplyManifests` and the install flow into reusable `envfunc`s.

### Gateway API conformance — for reference

```go
var HTTPRouteSimpleSameNamespace = confsuite.ConformanceTest{
    ShortName:   "HTTPRouteSimpleSameNamespace",
    Manifests:   []string{"tests/httproute-simple-same-namespace.yaml"},
    Features:    []features.FeatureName{features.SupportGateway, features.SupportHTTPRoute},
    Test: func(t *testing.T, suite *confsuite.ConformanceTestSuite) {
        gwAddr := kubernetes.GatewayAndHTTPRoutesMustBeAccepted(t, suite.Client, suite.TimeoutConfig, suite.ControllerName, ...)
        http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, gwAddr, ...)
    },
}
```

Compact and readable, but the test is part of the upstream catalog. There is no good place to express "and assert that kgateway's `TrafficPolicy` overrides this behavior."

## Recommendation

We recommend the following layered approach.

1. **Keep using the Gateway API conformance framework, unchanged, for spec conformance.** It is the right tool for that job, and we already run it. Nothing in this proposal changes the conformance pipeline.
2. **Adopt `sigs.k8s.io/e2e-framework` as the framework for new kgateway feature e2e tests, starting in [`test/e2e_sigs/`](../test/e2e_sigs/).** The basicrouting POC ([#13890](https://github.com/kgateway-dev/kgateway/pull/13890)) demonstrates that the authoring experience is dramatically lighter than the current framework, that filtering and per-test runs work as Go developers expect, and that we can carry forward our existing assertion idioms by wrapping Gomega in a small package.
3. **Treat the existing custom framework as legacy but supported.** It is not deleted. It continues to host the bulk of e2e coverage during the migration. New tests should default to the sigs framework; existing tests are migrated opportunistically and when a feature area is being substantially reworked.

The reasoning, in order of weight:

* The single biggest authoring complaint is the heavy abstraction stack a contributor has to internalize before writing a useful test. The sigs framework collapses that to "write a Go test, optionally build a `Feature`."
* Speed comes from doing less per test, not from a faster framework. The sigs framework forces us to be explicit about what setup belongs at `TestMain` (cluster-wide), what belongs at `Feature.Setup`, and what belongs in the assess step. That explicitness *is* the speedup that [#12993](https://github.com/kgateway-dev/kgateway/pull/12993) was reaching for.
* Adopting a community framework lowers the cost of cross-project review and onboarding. kgateway is the only consumer of its current framework; a contributor coming from Crossplane, kueue, or any other `sigs.k8s.io/e2e-framework` user does not pay a kgateway-specific learning tax.
* The Gateway API conformance framework cannot host kgateway-specific feature tests without contortion. We need a second framework for those tests regardless of what we choose, so the question is only "the existing custom one or sigs/e2e-framework."

We explicitly do not recommend a "burn down the custom framework" migration. The existing framework's investments — failure dumps, image pre-pull, version gating, persistence flags, the assertion provider — are real assets. We extract them into reusable building blocks (envfuncs, helper packages) as we migrate, and we delete the custom framework only when there is nothing left depending on it.

## Implementation Plan

The migration is staged so each step is reviewable in isolation and so kgateway is never left in a state where some feature area is half-migrated with no clear owner.

### Phase 1 — Foundations (this PR's follow-up)

* Promote `test/e2e_sigs/` from a basicrouting-only POC to the canonical home for sigs-framework tests.
* Consolidate the POC into the main module (drop the inner `go.mod`, use the repo `go.mod`) so it participates in `make analyze`, `make verify`, and CI.
* Establish reusable helper packages under `test/e2e_sigs/`:
  * `assertions/` — Gomega-backed HTTP and Kubernetes assertions (already started).
  * `envfuncs/` (proposed) — `InstallKgatewayFromLocalChart`, `UninstallKgateway`, `WaitForGatewayReady`, `DumpClusterStateOnFailure`, `PrePullImages`. These wrap the existing logic in [`test/e2e/test.go`](../test/e2e/test.go) and [`test/e2e/tests/base/base_suite.go`](../test/e2e/tests/base/base_suite.go) so the sigs runner can call them directly.
  * `manifests/` (proposed) — a thin port of `ApplyManifests` that handles dynamic proxy resource awaiting and namespace-skip-on-delete behavior.
* Wire `test/e2e_sigs/` into `.github/workflows/e2e.yaml` so it runs on every PR.

### Phase 2 — Migrate a representative slice

* Pick a small set of feature suites to migrate first: `basicrouting`, `cors`, `header_modifiers`. These exercise the common patterns (apply manifests, curl, assert) without pulling in Istio, ExtAuth, or rate-limit dependencies.
* For each, write the new test under `test/e2e_sigs/features/<area>/` *alongside* the existing `test/e2e/features/<area>/` suite. Run both in CI for one release cycle.
* Once we have confidence (no flake regressions, no coverage gaps), delete the duplicate from `test/e2e/`.

### Phase 3 — Document and broaden

* Update [`devel/testing/e2e-framework.md`](../devel/testing/e2e-framework.md) to describe the layered model: conformance for spec, sigs framework for kgateway features, custom framework as legacy.
* Update [`devel/testing/writing-tests.md`](../devel/testing/writing-tests.md) with a "writing a new e2e test" walkthrough that defaults to the sigs framework.
* Continue migrating feature areas opportunistically. Owners of a feature area decide when their tests move; the framework choice is *forward-looking* (new tests default to sigs framework) but not coercive.

### Phase 4 — Decommission

* When `test/e2e/features/` is empty (or contains only legitimately custom-framework-only tests, e.g., upgrade tests that genuinely need ordered suites), retire the `BaseTestingSuite` and `SuiteRunner` machinery.
* The retained `TestInstallation` concept may live on, repackaged as a sigs-framework `envfunc`-friendly helper.

## Alternatives Considered

### Alternative 1 — Keep the custom framework, optimize it

We could focus all effort on making the existing framework faster (the [#12993](https://github.com/kgateway-dev/kgateway/pull/12993) approach) without adopting a new framework. This is the lowest-risk option. We rejected it because the speed problem is downstream of the abstraction problem: the framework is slow because it does many things implicitly per test. Optimizing those implicit steps still leaves us with a kgateway-specific framework that no other project understands.

### Alternative 2 — Move kgateway feature tests onto the Gateway API conformance framework

We could try to fit kgateway-specific tests into the conformance framework's `ConformanceTest` shape, perhaps via an out-of-tree extension package. We rejected it because the conformance framework is opinionated about its scope (spec conformance) and stretching it to host installation lifecycle, multi-install scenarios, and policy-CRD-specific behavior would fight the framework rather than use it.

### Alternative 3 — Adopt Ginkgo / a different framework entirely

Ginkgo has its own community and is used by upstream Kubernetes e2e. The project's CLAUDE.md guidance explicitly says new code should *avoid* Ginkgo, citing reduced clarity. We respect that constraint.

### Alternative 4 — Maintain two frameworks indefinitely

We could leave existing tests in the custom framework forever and write new tests in the sigs framework. We rejected this as the long-term outcome but accept it as the medium-term one (Phase 2-3 above). Two frameworks indefinitely is a maintenance tax we should pay only during migration.

## Open Questions

* **Multi-install scenarios.** Some current entrypoints (`automtls_istio_test.go`, `multiple_installs_test.go`) compose multiple `TestInstallation`s in one process. The sigs framework supports this via multiple `env.Environment` values, but the ergonomics need a worked example before we commit. Tracked as part of Phase 1.
* **Failure dump parity.** The current `PerTestPreFailHandler` is rich. Ported as an `envfunc`, it works. Whether it can be triggered cleanly from `Feature.Teardown` on a *failed* assess (rather than always) is the open detail. The sigs framework exposes a `Failed` flag on `Feature` runs that we believe is sufficient.
* **Migration cadence.** Do we batch migrations by feature area or by file? Phase 2 assumes "by feature area" — confirm with maintainers.
* **agentgateway dataplane.** Conformance and feature tests exist for both Envoy and agentgateway dataplanes. Both should run against the sigs framework once Phase 1 lands; no framework-level concern is anticipated, but we should validate on a real agentgateway suite before declaring victory.

## Approval

This document is the design artifact for issue [#13891](https://github.com/kgateway-dev/kgateway/issues/13891). Approval from kgateway maintainers on:

* the recommendation to adopt `sigs.k8s.io/e2e-framework` for new feature e2e tests,
* the layered model that retains the Gateway API conformance framework for spec testing,
* the staged migration plan (Phases 1-4),

is the gating outcome of this design phase. Implementation begins in Phase 1 once the design is approved.
