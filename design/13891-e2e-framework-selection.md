# EP-13891: E2E Testing Framework Evaluation

* Issue: [13891](https://github.com/kgateway-dev/kgateway/issues/13891)
* Parent epic: [Modernize and improve kgateway end-to-end testing](https://github.com/kgateway-dev/kgateway/issues/13351)
* Reference:
  * [#12981 — Good tests](https://github.com/kgateway-dev/kgateway/issues/12981)
  * [#12993 — initial fast e2e attempt](https://github.com/kgateway-dev/kgateway/pull/12993)

<!-- toc -->

- [Background](#background)
- [Motivation](#motivation)
- [Goals](#goals)
- [Non-Goals](#non-goals)
- [Implementation Details](#implementation-details)
  - [Frameworks Under Consideration](#frameworks-under-consideration)
    - [Current kgateway custom framework](#current-kgateway-custom-framework)
    - [`sigs.k8s.io/e2e-framework`](#sigsk8sioe2e-framework)
    - [Gateway API conformance framework](#gateway-api-conformance-framework)
  - [Comparison](#comparison)
  - [Same test in each framework](#same-test-in-each-framework)
    - [Gateway API conformance framework](#gateway-api-conformance-framework-1)
    - [Current framework](#current-framework)
    - [sigs/e2e-framework POC](#sigse2e-framework-poc)
  - [Recommendation](#recommendation)
  - [Improvement Plan](#improvement-plan)
  - [Test Plan](#test-plan)
- [Alternatives](#alternatives)
  - [Alternative 1 — Adopt the Gateway API conformance framework](#alternative-1--adopt-the-gateway-api-conformance-framework)
  - [Alternative 2 — Adopt `sigs.k8s.io/e2e-framework`](#alternative-2--adopt-sigsk8sioe2e-framework)
- [Approval](#approval)

<!-- /toc -->

## Background

kgateway maintains a large suite of end-to-end (e2e) tests that exercise the full path from Kubernetes Gateway API resources through the kgateway control plane to the dataplane proxies (Envoy). These tests live under [`test/e2e/`](../test/e2e/) and use a custom framework built on top of [`testify/suite`](https://pkg.go.dev/github.com/stretchr/testify/suite).

The framework has accumulated significant capability over time, but it has also become slow to run and has a learning curve. 

## Motivation

The current custom framework is **slow** (per-test setup — manifest application, image pre-pull, dynamic resource discovery, `EventuallyPodsRunning` — dominates execution time) and **heavy on framework artifacts** (a new test requires a `TestCase` map, a `BaseTestingSuite` embedding, a `SetupSuite` method, and registration in a central `tests/*_tests.go` file before the first assertion is written — the scaffolding often outweighs the test logic).

This document evaluates whether kgateway should keep evolving the existing framework or migrate to one of the established alternatives in the Kubernetes ecosystem. The complementary write-up in [#12981 ("Good tests")](https://github.com/kgateway-dev/kgateway/issues/12981) and the prototype in [#12993 ("fast e2e tests")](https://github.com/kgateway-dev/kgateway/pull/12993) make a concrete case for what a faster, simpler test loop should look like.

The conclusion is to keep the custom framework and invest in incremental improvements. The reasoning is in [Recommendation](#recommendation); the rejected alternatives and their tradeoffs are in [Alternatives](#alternatives).

## Goals

These are the potential areas for improvement. 

* **Speed & efficiency.** Improve execution speed and resource efficiency.
* **Accessibility.** Easy to read and write. Minimize the kgateway-specific structures a contributor has to learn before writing their first test. 
* **Reliability & reproducibility.** Same result locally and in CI. Deterministic setup and teardown. No cross-test state leakage.
* **Maintenance burden.** Lean on upstream/community-maintained test code rather than carrying kgateway-specific equivalents in-repo.


## Non-Goals

* Full migration plan/scope of considered frameworks. 
* Update kgateway install for new framework. Orthogonal to framework choice.
* POCs assume gateway is already installed, will be addressed in full design if framework is adopted. 
* Replace unit tests, gateway translator tests, or load tests. This document is scoped to functional e2e.
* Design the full integration of the chosen framework into the kgateway codebase. The scope here is validating functional fit at the POC level.

## Implementation Details


### Frameworks Under Consideration

#### Current kgateway custom framework

Location: [`test/e2e/`](../test/e2e/), with the suite-level base in [`test/e2e/tests/base/base_suite.go`](../test/e2e/tests/base/base_suite.go).

The framework is built around three central abstractions:

* `TestInstallation` ([`test/e2e/test.go`](../test/e2e/test.go)) — bundles a runtime context, cluster context, install context, an `Actions` provider (Helm, kubectl, curl wrappers), an `Assertions` provider (Gomega-based helpers), and a per-test failure dump directory.
* `BaseTestingSuite` — embeds `testify/suite.Suite` and wires the test lifecycle (`SetupSuite`, `BeforeTest`, `AfterTest`, `TearDownSuite`) to manifest application, image pre-pulling, dynamic resource discovery, and Gateway API version gating.
* `SuiteRunner` ([`test/e2e/suite.go`](../test/e2e/suite.go)) — registers and runs a set of `testify` suites against a single `TestInstallation`.

Tests are organized as `features/<area>/suite.go` and registered in `tests/<entrypoint>_tests.go`. Each test method on a suite struct is treated as a Go subtest.

**Strengths**:

* **Tight kgateway integration.** Helm install flow, failure dump, image pre-pull, dynamic proxy resource awaiting, and Gateway API version/channel guards are all built in.
* **Mature assertion library.** `assertions.Provider` exposes `AssertEventualCurlResponse`, `AssertEventuallyConsistentCurlResponse`, `EventuallyGatewayAddress`, etc. — purpose-built for this product.
* **Manifest-first authoring.** `TestCase{Manifests: []string{...}}` matches how users actually drive kgateway (`kubectl apply -f`).
* **Failure forensics.** On failure the framework dumps namespace state, controller logs, and resource descriptions to a per-test directory. This is invaluable in CI.
* **Persistence flags.** `PERSIST_INSTALL`, `FAIL_FAST_AND_PERSIST`, and `SKIP_INSTALL` enable iterative local debugging without paying the install cost on every run.

**Weaknesses** 

* **Slow per-test cycle.** Each suite re-applies its setup manifests and waits for pods. With pre-pull, `EventuallyObjectsExist`, dynamic resource discovery, and `EventuallyPodsRunning`, a single test can spend tens of seconds on setup before it ever issues a curl.
* **Heavy abstraction.** A new contributor has to learn `TestInstallation`, `BaseTestingSuite`, `TestCase`, `Setup`/`SetupByVersion`, `Actions.*`, `AssertionsT(t).*`, the `SuiteRunner`, and the difference between `Setup` and per-test `TestCases` before they can write a basic routing test. The actual test method is often the smallest part of the file.
* **Coupled installation and execution.** A `TestInstallation` is per-entrypoint, so testing kgateway under multiple Helm value sets requires a new `*_test.go` file *and* a new GitHub Actions invocation. This is the "1:1:1 relationship" called out in [`test/e2e/README.md`](../test/e2e/README.md).
* **Bespoke knowledge.** None of this transfers to other Kubernetes projects. Reviewers from outside the project pay a learning tax.

#### `sigs.k8s.io/e2e-framework`

Upstream project: [`sigs.k8s.io/e2e-framework`](https://github.com/kubernetes-sigs/e2e-framework). A POC migration of the basicrouting tests lives at [`test/e2e_sigs_framework/`](https://github.com/kgateway-dev/kgateway/tree/lfx-e2e-framework-poc/test/e2e_sigs_framework).

The framework provides a Go-test-native programming model:

* `env.Environment` is the single object that owns lifecycle. `TestMain` constructs it, optionally registers `Setup` / `Finish` steps (e.g., create a kind cluster, install CRDs, deploy the controller), and calls `testenv.Run(m)`.
* Tests are plain `func TestX(t *testing.T)` functions. They build one or more `features.Feature` values using a fluent builder (`features.New(name).WithLabel(...).Setup(...).Assess(...).Teardown(...).Feature()`) and execute them with `testenv.Test(t, feat)`.
* `envconf.Config` carries the cluster client, namespace, kubeconfig path, and CLI flags — it is threaded into every step's closure.
* `envfuncs` provides reusable building blocks (`CreateCluster`, `CreateNamespace`, `LoadDockerImageToCluster`, etc.) that can be composed into `Setup`/`Finish` chains.

**Strengths**:

* **Standard Go test idioms.** No reflection-based suite runner. `go test -run TestGatewayWithRoute -v ./test/e2e_sigs_framework/features/basicrouting` works the way every Go developer expects.
* **Composable lifecycle.** Each feature owns its own `Setup` / `Assess` / `Teardown`. Per-feature state lives in the `context.Context` returned from each step, so there is no shared mutable suite struct.
* **Labels and feature gates.** `WithLabel("type", "smoke")` plus `--feature` / `--labels` flags let CI pick subsets without restructuring code.
* **Familiar to the community.** The framework is used by Crossplane, kueue, and several other CNCF projects. New contributors who have written tests for those projects will be immediately productive.
* **Light dependency surface.** The package is small, focused, and stable. No reflection magic on test method names.

**Weaknesses:**

* **No support for kgateway-specific concerns.** Helm-install-from-local-chart, failure dumps, image pre-pull, dynamic proxy resource discovery, and Gateway API version gating do not exist out of the box. We would have to port these or pay the cost in flake and triage.
* **Manifest application is bring-your-own.** The framework gives you a controller-runtime client and `decoder.DecodeEachFile` helpers, but the polished "apply this YAML, then wait for the dynamically created proxy Deployment, then await pods running" flow we have in `BaseTestingSuite.ApplyManifests` is not provided.
* **No `testify/suite`-style fixtures.** The framework offers two scopes: cluster-wide setup in `TestMain`, or per-feature setup inside `features.Feature`. There is no built-in scope in between — i.e., a fixture shared by a *group* of related tests (the equivalent of `testify/suite`'s `SetupSuite`). If several tests share expensive setup, you have to either hoist it to `TestMain` (paid by every test in the package) or thread state through `context.Context` by hand.
* **Per-feature setup overhead.** The fluent `Setup -> Assess -> Teardown` per feature can mean re-doing work that was previously amortized at the `SetupSuite` level, unless we are deliberate about which steps live at `TestMain` vs. per-feature.
* **Assertion style.** The framework intentionally takes no opinion on assertions. Tests in the wider community use a mix of `t.Fatal`, `require`, and `gomega`. The basicrouting POC pulls in Gomega via [`assertions/assertions.go`](https://github.com/kgateway-dev/kgateway/blob/lfx-e2e-framework-poc/test/e2e_sigs_framework/assertions/assertions.go) — that pattern works but is something we own, not something the framework gives us.

#### Gateway API conformance framework

Upstream lives at `sigs.k8s.io/gateway-api/conformance/`.

* `ConformanceTestSuite` is the runner. It is constructed once per `TestMain` with the `GatewayClassName`, `ControllerName`, base manifests, and supported features, then `suite.Run(t, tests)` iterates the registered `ConformanceTest` values.
* Each test is a `ConformanceTest` struct: `ShortName`, `Description`, required `Features`, `Manifests`, and a single `Test` function. 
* A rich helper library lives under `gateway-api/conformance/utils/`: `kubernetes.GatewayAndHTTPRoutesMustBeAccepted`, `http.MakeRequestAndExpectEventuallyConsistentResponse`, the `RoundTripper` abstraction, `tlog`, etc.
* The framework knows how to gate tests by supported features and to skip provisional tests; it produces a structured conformance report.

**Strengths:** 

* **Reusable Gateway API testing patterns.** Helpers like 
  `MakeRequestAndExpectEventuallyConsistentResponse` and 
  `GatewayMustHaveAddress` handle common Gateway API validation 
  patterns and reduce boilerplate.

* **Can be extended for kgateway tests.** The framework's `ConformanceTest` 
  struct is simple—a plain Go struct with a `Run(t, suite)` method. 
  We can define kgateway-specific tests using the same machinery.


**Weaknesses:**

* **Helpers are built for Gateway API testing.** For kgateway CRDs (TrafficPolicy, BackendConfigPolicy, ExtAuth), we need custom helpers to check their status and behavior. 
* **No install management.** The conformance runner expects a Gateway API implementation to already be running. Install lifecycle is not part of the conformance test framework and will have to be managed by kgateway-specific code.
* **Incompatible with upstream TLS bootstrap.** The framework's `suite.Setup()` performs TLS bootstrap that interferes with kgateway's install flow. We must manage TLS configuration ourselves.
* **Multi-install scenarios are awkward.** A single `ConformanceTestSuite` value points at one cluster install. Tests that compose multiple installs need either separate Go test binaries (one suite per install) or to remain on the legacy framework as a permitted exception.
* **Helpers follow spec evolution.** The upstream `kubernetes.*`, `http.*`, and `roundtripper` packages evolve with the Gateway API specification. We accept this as the cost of letting the upstream community maintain the helpers — it directly serves the **Maintenance burden** goal. The test framework version is pinned via `go.mod` to match the Gateway API version kgateway targets.

### Comparison

| Concern | Current kgateway | sigs/e2e-framework | Gateway API conformance |
|---|---|---|---|
| Test structure | `testify/suite` + `BaseTestingSuite` | `func TestX` + `features.Feature` builder | `ConformanceTest` value, dispatched by orchestrator |
| Helm install / uninstall | Built-in | Bring your own | Bring your own |
| Manifest apply + await | `ApplyManifests` with image pre-pull | `decoder.*` helpers, rest is bring your own | `Applier.MustApplyWithCleanup` — auto `t.Cleanup` |
| HTTP probe with eventual consistency | `AssertEventuallyConsistentCurlResponse` | None — caller writes one (POC uses `assert.Eventually`) | `http.MakeRequestAndExpectEventuallyConsistentResponse` upstream |
| Gateway/Route readiness helpers | `EventuallyGatewayAddress` | None | `kubernetes.GatewayAndHTTPRoutesMustBeAccepted` upstream |
| Image pre-pull for flake reduction | Yes ([`base_suite.go:429`](../test/e2e/tests/base/base_suite.go#L429)) | No | Not needed once tests share base apply; ported to `common/` if required |
| Failure dumps | Yes (`PerTestPreFailHandler`) | No (build it) | Add as `common/` wrapper around `tc.Run` |
| Gateway API version gating | Yes (`MinGwApiVersion`/`MaxGwApiVersion` per channel) | No (build it) | Implicit via `SupportedFeatures` registry |
| Filtering tests | Go test `-run` regex | `-run` + `--feature` / `--labels` flags | `-run` + `SupportedFeatures` + skip lists |
| Assertion style | Gomega (`Eventually`) + Testify (`Require`) | Caller's choice (POC uses `testify/assert`) | Upstream helpers + `t.Fatal` / `require` |
| Parallelism story | Subtests in one suite share state | `t.Parallel()` works naturally per feature | `t.Parallel()` works at orchestrator level |
| Familiarity for new contributors | Low (kgateway-specific) | Medium (used across CNCF) | High (Gateway API community already uses it for spec tests) |
| Coupling to install layout | High (1:1:1) | Low (lifecycle is composable) | One install per `ConformanceTestSuite` value |
| Speed of a minimal test | Slow — full `BaseTestingSuite` cycle | Fast — only what the feature needs | Fast — single base apply amortized across all features |
| In-repo helper code we own | `BaseTestingSuite`, `assertions.Provider`, `Actions`, `EventuallyPodsRunning`, etc. | Equivalents of all of the above (we own them) | `common/suite.go` setup + `common/kgateway_helpers/` for policy CRDs |

### Same test in each framework

The basicrouting "Gateway with Route" test is implemented in all three frameworks today, which makes a side-by-side comparison concrete. 

#### Gateway API conformance framework

[POC](https://github.com/kgateway-dev/kgateway/tree/lfx-e2e-framework-poc):

The framework needs three pieces wired up: a `TestMain` that constructs the `ConformanceTestSuite`, a `TestX` function that applies the Gateway and backend manifests, and the `ConformanceTest` value that asserts behaviour.

`TestMain` ([`main_test.go`](https://github.com/kgateway-dev/kgateway/blob/lfx-e2e-framework-poc/test/sigs_gateway_api_framework/main_test.go)) constructs the suite once for the package:

```go
var suite *confsuite.ConformanceTestSuite

func TestMain(m *testing.M) {
    if err := common.SetupConformanceSuite(gatewayClassName, []fs.FS{manifestFS}); err != nil {
        fmt.Fprintf(os.Stderr, "failed to set up conformance suite: %v\n", err)
        os.Exit(1)
    }
    suite = common.GetSuite()
    os.Exit(m.Run())
}
```

`common.SetupConformanceSuite` ([`common/suite.go`](https://github.com/kgateway-dev/kgateway/blob/lfx-e2e-framework-poc/test/sigs_gateway_api_framework/common/suite.go)) is an in-repo helper that builds the client, calls `confsuite.NewConformanceTestSuite`, and configures `suite.Applier` directly instead of calling `suite.Setup()` — the upstream `Setup()` performs TLS bootstrap that does not fit kgateway's install flow.


```go
func TestGatewayWithRoute(t *testing.T) {
    common.ApplyBaseManifests(t, []string{"testdata/gateway.yaml", "testdata/backend.yaml"})

    suite.ControllerName = kubernetes.GWCMustHaveAcceptedConditionTrue(
        t, suite.Client, suite.TimeoutConfig, suite.GatewayClassName,
    )

    test := confsuite.ConformanceTest{
        ShortName:   "GatewayWithRoute",
        Description: "An HTTPRoute attached to a Gateway routes requests to the echo backend on each listener port.",
        Features:    []features.FeatureName{features.SupportGateway, features.SupportHTTPRoute},
        Manifests:   []string{"testdata/http-route.yaml"},
        Test: func(t *testing.T, s *confsuite.ConformanceTestSuite) {
            gwNN := types.NamespacedName{Name: gatewayName, Namespace: testNamespace}
            routeNN := types.NamespacedName{Name: routeName, Namespace: testNamespace}

            gwAddr := kubernetes.GatewayAndHTTPRoutesMustBeAccepted(
                t, s.Client, s.TimeoutConfig, s.ControllerName,
                kubernetes.NewGatewayRef(gwNN), routeNN,
            )

            for _, port := range []int{listenerHighPort, listenerLowPort} {
                t.Run("listener_port_"+strconv.Itoa(port), func(t *testing.T) {
                    http.MakeRequestAndExpectEventuallyConsistentResponse(
                        t, s.RoundTripper, s.TimeoutConfig,
                        addressOnPort(gwAddr, port),
                        http.ExpectedResponse{
                            Request:   http.Request{Host: routeHostname, Path: "/"},
                            Response:  http.Response{StatusCode: nethttp.StatusOK},
                            Backend:   echoBackendName,
                            Namespace: testNamespace,
                        },
                    )
                })
            }
        },
    }

    test.Run(t, suite)
}
```

Gateway/HTTPRoute readiness and HTTP probing come from upstream helpers (`kubernetes.GatewayAndHTTPRoutesMustBeAccepted`, `http.MakeRequestAndExpectEventuallyConsistentResponse`). The Gateway resource itself is applied by the in-repo `common.ApplyBaseManifests` helper, and the HTTPRoute manifest listed in `ConformanceTest.Manifests` is auto-applied by `test.Run(t, suite)` with `t.Cleanup` registering teardown. The repo owns `common/suite.go` (the suite constructor and the Applier configuration that replaces `suite.Setup()`) — that is the kgateway-specific glue this approach still requires.

#### Current framework

File: [`test/e2e/features/basicrouting/suite.go`](../test/e2e/features/basicrouting/suite.go)

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
    s.localGateway = common.Gateway{...}
}

func (s *testingSuite) TestGatewayWithRoute() {
    s.assertSuccessfulResponse()
}
```

Three registration touchpoints before the test logic. First, `setup` and `testCases` map in `suite.go`:

```go
var (
    setup = base.TestCase{
        Manifests: []string{"testdata/gateway-with-route.yaml"},
    }
    testCases = map[string]*base.TestCase{
        "TestGatewayWithRoute": {
            Manifests: []string{"testdata/service.yaml"},
        },
        "TestHeadlessService": {
            Manifests: []string{"testdata/headless-service.yaml"},
        },
    }
)
```

Second, explicit registration in `tests/kgateway_tests.go`:

```go
kubeGatewaySuiteRunner.Register("BasicRouting", basicrouting.NewTestingSuite)
kubeGatewaySuiteRunner.Register("Cors", cors.NewTestingSuite)
// ... 50+ more registrations ...
```

The framework requires: a `TestCase` map definition, a `BaseTestingSuite` embedding, a `SetupSuite` method, explicit registration in a central file, and  test method itself.

#### sigs/e2e-framework POC

[POC](https://github.com/kgateway-dev/kgateway/tree/lfx-e2e-framework-poc/test/e2e_sigs_framework):

```go
func TestGatewayWithRoute(t *testing.T) {
    var gatewayAddress string

    feat := features.New("Gateway with Route").
        Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
            // Manifest application: repo-specific code required
            applier := kgateway.NewApplier(cfg.Client())  // Custom helper
            applier.ApplyManifests(ctx, "testdata/gateway-with-route.yaml")
            // Cleanup registered via t.Cleanup (framework-standard)
            
            addr, err := gateway.GetAddress(ctx, cfg, "test-gateway", "kgateway-test")
            if err != nil {
                t.Fatalf("failed to get gateway address: %v", err)
            }
            gatewayAddress = addr
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

The fluent `features.New` API is clean, but manifest application (`kgateway.NewApplier`) and readiness checks (`gateway.GetAddress`) and assertions (`assertions.AssertSuccessfulResponse`) are all in-repo helpers — equivalents of `kubernetes.GatewayAndHTTPRoutesMustBeAccepted` and `http.MakeRequestAndExpectEventuallyConsistentResponse` that we would own indefinitely. The framework provides the lifecycle scaffolding but not the Gateway API–specific machinery. 

### Recommendation

**We keep the existing custom framework and invest in incremental improvements rather than migrating to a new framework.** 

* **The bottleneck is not just the framework.** CI wall-clock is bounded as much or more by environment setup — image build, image pull, cluster provision, kgateway install — as by test execution. Improvements in those areas yield similar speedups with significantly less work than a framework migration would require.

* **Substantial new helper code would be required.** External frameworks provide generic Gateway API helpers, but kgateway-specific behavior would still require a meaningful helper layer in-repo. The "lean on upstream" benefit pays off less than it appears.

* **Incremental refactors are easier to land than a full migration.** The "fast test" pattern from [PR #12993](https://github.com/kgateway-dev/kgateway/pull/12993) is much smaller in scope than a framework migration and is still only partially rolled out. A migration to a new framework would be many times larger and carries the risk of landing partially, leaving the codebase maintaining two frameworks for an extended period.



### Improvement Plan

We propose two refactors. They are independent and can land in any order.

#### 1. Roll out the fast-test pattern from PR #12993

[PR #12993](https://github.com/kgateway-dev/kgateway/pull/12993) (merged) established a faster execution model on the agentgateway feature area, with a measured improvement from 35s to ~1s on `TestAgentgatewayIntegration`. Migrate the remaining suites under [`test/e2e/features/`](../test/e2e/features/) to the same pattern. Concrete cleanups inside that rollout:

* **Stop routing test traffic through an in-cluster curl pod.** Many suites still `kubectl exec` into a curl pod to issue requests, which adds a fixed per-call cost and forces every assertion through a string-matching shell pipeline. Sending requests directly from the test process over the LoadBalancer removes that overhead and gives tests structured access to the response (status, headers, body) via Go matchers — both faster and more expressive.
* **Consolidate backend deployments onto the defaults package.** Suites have grown a habit of deploying their own backend (httpbin, nginx, echo) instead of reusing the canonical ones the defaults package already provides. Collapsing onto shared backends cuts duplicate pod start-up and image-pull cost, removes inconsistency across suites (different images asserting the same thing), and makes future backend changes a single edit instead of a sweep.
* **Reuse the shared BaseGateway instead of installing a per-suite Gateway.** A large fraction of suites install their own Gateway even though they only need a plain HTTP listener that the shared BaseGateway already provides. Attaching those suites' HTTPRoutes to the BaseGateway removes the per-suite Gateway apply, the wait for it to be programmed, and the corresponding Envoy Deployment — directly attacking the dominant per-test setup cost.

#### 2. Remove dead manifests, helpers, and assertion variants

[`test/e2e/`](../test/e2e/) has accumulated test YAML, helpers, and assertion variants that are no longer referenced. Audit and remove. 



## Alternatives

We investigated both alternative frameworks below. We are not adopting either now; the immediate work is improving the current framework (see [Proposal](#proposal)). Of the two, the Gateway API conformance framework is the stronger option if kgateway later chooses to migrate.

### Alternative 1 — Adopt the Gateway API conformance framework

We prototyped adopting `sigs.k8s.io/gateway-api/conformance` as the single framework for kgateway end-to-end tests, with new tests written as `confsuite.ConformanceTest` values and existing tests migrated to that shape over time. The framework provides built-in Gateway API helpers (`kubernetes.GatewayAndHTTPRoutesMustBeAccepted`, `http.MakeRequestAndExpectEventuallyConsistentResponse`), a simple `ConformanceTest` struct as the core abstraction, and community familiarity — Gateway API conformance tests already use this shape.

We didn't choose it because:

* **The bottleneck is environment setup, not the framework.** CI wall-clock is bounded as much or more by image build, image pull, cluster provision, and kgateway install as by test execution. A framework migration would not address the dominant cost.
* **Substantial new helper code would still be required.** The framework's upstream helpers cover generic Gateway API testing, but kgateway-specific behavior needs a meaningful helper layer in-repo regardless.
* **Full migration risks landing partially.** The "fast test" pattern from [PR #12993](https://github.com/kgateway-dev/kgateway/pull/12993) is a much smaller scope and is still only partially rolled out. A framework migration is many times larger and risks leaving the codebase maintaining two frameworks for an extended period.

### Alternative 2 — Adopt `sigs.k8s.io/e2e-framework`

We prototyped this in the [feature branch](https://github.com/kgateway-dev/kgateway/tree/lfx-e2e-framework-poc/test/e2e_sigs_framework), with code under `test/e2e_sigs_framework/`. The framework gives a clean Go-test-native programming model and composable lifecycle steps (`env.Environment` with `Setup`/`Finish` hooks). It is used by Crossplane, kueue, and several other CNCF projects.

We didn't choose it because:

* **It provides only the lifecycle scaffolding.** No assertion library, no Gateway API readiness helpers, no manifest applier, no eventual-consistency HTTP probe. The POC's `assertions/assertions.go` and `common/gateway/gateway.go` are the start of an in-repo equivalent of what `sigs.k8s.io/gateway-api/conformance/utils/` already provides. Adopting this framework means owning the implementation of that functionality.
* **The same maintenance-burden concern applies.** Even more so than the conformance framework, `sigs.k8s.io/e2e-framework` provides only generic lifecycle scaffolding — no manifest applier, no Gateway API readiness helpers, no eventual-consistency HTTP probe. We would own all of that in-repo indefinitely.
* **Two frameworks is a tax.** Running the upstream Gateway API conformance suite for spec coverage *and* `sigs.k8s.io/e2e-framework` for kgateway-specific tests means contributors switch mental models depending on which directory they are in.

The POC is preserved in the [feature branch](https://github.com/kgateway-dev/kgateway/tree/lfx-e2e-framework-poc) as a reference for the comparison.


## Approval



