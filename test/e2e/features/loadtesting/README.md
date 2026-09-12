# KGateway Load Testing Framework

This directory contains the KGateway load testing framework that implements performance tests based on the gateway-api-bench methodology. The framework focuses on **control plane performance** testing rather than data plane traffic.

## Overview

The load testing framework provides:

- **Attached Routes Test**: Measures Gateway API route attachment performance
- **Controller Restart Measurement**: Measures kgateway controller rollout recovery while baseline routes already exist
- **VCluster Simulation**: Creates fake cluster resources to simulate production-scale environments
- **Scale-Aware Testing**: Automatically adjusts thresholds based on route count (1000 vs 5000+ routes)
- **Performance Monitoring**: Tracks setup time, teardown time, and status propagation

## Prerequisites

Before running the load tests, you need a properly configured Kubernetes cluster with:

- Gateway API CRDs installed
- KGateway controller deployed

## 🚀 Setup Options

```bash
# Navigate to the project root
cd <project-root>

# Create a kind cluster with registry (if you don't have one already)
ctlptl create cluster kind --name kind-kind --registry=ctlptl-registry

# Build KGateway images and load them into the cluster
# This also installs Gateway API CRDs and KGateway
VERSION=1.0.0-ci1 CLUSTER_NAME=kind make kind-build-and-load

# Start Tilt for development workflow
tilt up
```

### Verification

After setup, verify your cluster is ready:

```bash
# Check if Gateway API CRDs are installed
kubectl get crd gateways.gateway.networking.k8s.io

# Check if KGateway controller is running
kubectl get pods -n kgateway-system

# Verify cluster context
kubectl config current-context
```

## Running Tests

### Command Line

```bash
# Run baseline test (1000 routes)
make run-load-tests-baseline

# Run production test (5000 routes)
make run-load-tests-production

# Run all load tests (baseline + production)
make run-load-tests

# Run against an already-installed strict-validation controller
VALIDATION_MODE=strict make run-load-tests
```

### VS Code Debug Configuration

Add this configuration to your `.vscode/launch.json` file:

```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "AttachedRoutes Load Test",
            "type": "go",
            "request": "launch",
            "mode": "test",
            "program": "${workspaceFolder}/test/e2e/tests/kgateway_test.go",
            "args": [
                "-test.run",
                "^TestKgateway$/^AttachedRoutes$",
                "-test.v"
            ],
            "env": {
                "SKIP_INSTALL": "true",
                "CLUSTER_NAME": "kind",
                "INSTALL_NAMESPACE": "kgateway-system"
            }
        },
        {
            "name": "AttachedRoutes Baseline Only",
            "type": "go",
            "request": "launch",
            "mode": "test",
            "program": "${workspaceFolder}/test/e2e/tests/kgateway_test.go",
            "args": [
                "-test.run",
                "^TestKgateway$/^AttachedRoutes$/^TestAttachedRoutesBaseline$",
                "-test.v"
            ],
            "env": {
                "SKIP_INSTALL": "true",
                "CLUSTER_NAME": "kind",
                "INSTALL_NAMESPACE": "kgateway-system"
            }
        },
        {
            "name": "AttachedRoutes Production Only",
            "type": "go",
            "request": "launch",
            "mode": "test",
            "program": "${workspaceFolder}/test/e2e/tests/kgateway_test.go",
            "args": [
                "-test.run",
                "^TestKgateway$/^AttachedRoutes$/^TestAttachedRoutesProduction$",
                "-test.v"
            ],
            "env": {
                "SKIP_INSTALL": "true",
                "CLUSTER_NAME": "kind",
                "INSTALL_NAMESPACE": "kgateway-system"
            }
        }
    ]
}
```

#### Environment Variables Explained:

- `SKIP_INSTALL=true`: Skips KGateway installation (assumes it's already installed)
- `CLUSTER_NAME=kind`: Targets the kind cluster named "kind"
- `INSTALL_NAMESPACE=kgateway-system`: Specifies where KGateway is installed

The nightly load-test action runs the full attached-routes suite twice: once
with `validation.level=standard` and once with `validation.level=strict`.
Each attached-routes run restarts the controller after creating baseline routes
and emits a `startup_benchmark_result` log line with the Gateway API CRD
version/channel, validation mode, controller image, rollout generation,
duration, and failure diagnostics.

## Test Types and Metrics

### Baseline Test (1000 routes)

- **Purpose**: Tests performance with moderate scale
- **Thresholds**: Setup <30s, Teardown <10s
- **Batch Size**: 100 routes per batch

### Production Test (5000 routes)

- **Purpose**: Tests performance at production scale
- **Thresholds**: Setup <90s, Teardown <20s
- **Batch Size**: 500 routes per batch

### Controller Restart Measurement

- **Purpose**: Measures the time from a controller rollout restart to a fully ready new deployment generation after baseline routes are present
- **Failure Signal**: Fails if the controller deployment does not become ready within 5 minutes
- **Diagnostics**: Records deployment status, controller pod state, recent pod events, and installed Gateway API metadata

### Key Metrics Measured

- **Setup Time**: Time to add 1 incremental route to existing baseline
- **Route Ready Time**: Time until route accepts traffic
- **Teardown Time**: Time to remove 1 route
- **Controller Restart Time**: Time for the controller deployment to roll out after baseline resources are created
- **Total Writes**: Number of status updates during test
- **Resource Usage**: CPU, memory, and API call metrics

### StrictChurn Test (per-client xDS convergence)

`strictchurn_suite.go` is a convergence/liveness test for the per-client xDS
pipeline under its worst-case shape (#14184) rather than a latency benchmark:

- **Setup**: Enables `KGW_VALIDATION_MODE=STRICT` on the controller (restored on
  teardown), creates ~200 simulated backends and baseline routes across two
  gateways, plus one stable route to a real nginx backend.
- **Load**: Cycles of Service+EndpointSlice+HTTPRoute create/delete, a
  background rewriter that keeps the simulated fleet's EndpointSlices moving
  continuously, persistent dangling and starved backend references, two
  gateway Envoy rolls, and a controller restart mid-churn.
- **Assertions**: The stable route answers 200 at every checkpoint (connected
  proxies are never stranded on stale or withheld config); rolled gateway
  Envoys become Ready within a bound (a fresh xDS client's first snapshot is
  never withheld indefinitely); a route created after churn becomes routable
  within a bound (publication liveness).

To probe the xDS first-connect grace period, set `KGW_XDS_FIRST_CONNECT_DELAY`
in the environment: the suite forwards it to the controller deployment for the
duration of the run. `0` exercises the raw reconnect race the delay narrows;
large values verify warm clients keep serving through the delay and rolled
Envoys still beat the rollout bound. Scale past the laptop-friendly defaults
with `KGW_LOADTEST_BACKENDS` / `KGW_LOADTEST_ROUTES`.

Because the suite mutates the controller deployment, it is registered with the
suite runner but excluded from the shared CI e2e clusters. The nightly
load-test action runs it once per load-test cluster, after the standard and
strict AttachedRoutes runs. Run it locally with
`make run-load-tests-strict-churn`.

### XdsCost Benchmark (per-client xDS control-plane cost)

`xdscost_suite.go` is StrictChurn's measurement sibling. StrictChurn asks "did
the fleet stay served under churn"; XdsCost asks "what did each kind of change
cost the controller", which is the question that separates competing per-client
CDS topologies. It is a benchmark, not an assertion suite: it has no pass/fail
thresholds, and it is meant to be run twice against two builds and diffed.

It measures the controller from the outside, scraping the controller's own
`/metrics` (port 9092) before and after each change. `pkg/metrics` registers the
Go and process collectors, so `process_cpu_seconds_total`,
`go_memstats_alloc_bytes_total`, `go_memstats_heap_inuse_bytes` and
`process_resident_memory_bytes` are measurements of the real running binary.

Three phases, one per event the topologies price differently:

- **EdsChurn** rewrites one simulated Service's EndpointSlice. Those are EDS
  clusters, so the endpoints flow through the separate per-client EDS pipeline
  and never reach the cluster base. This is the control phase: it should cost
  about the same on any CDS topology, and it shows how much of an
  endpoint-churn bill is actually CDS.
- **BaseChurn** edits one static `Backend`'s host. A static Backend becomes a
  STATIC cluster with an inline `ClusterLoadAssignment`, so its endpoints are
  folded into the cluster's base version: the edit is a base change and every
  connected client's CDS payload is rebuilt. This is the frequent event wherever
  endpoints live in the backend object rather than in EndpointSlices
  (ServiceEntry with inline endpoints, DNS and static Backends).
- **Reconnect** deletes one gateway's Envoy pod, so the replacement arrives as a
  new xDS client.

Client fan-out is the number of **Gateways**, not Envoy replicas: a
`UniquelyConnectedClient` is keyed by role, namespace, labels and locality, so on
a single-node cluster every replica of one Gateway collapses into one client.

Each phase reports, per change: controller CPU milliseconds, megabytes
allocated, xDS syncs and snapshot transforms, and convergence latency measured
to the last xDS sync for that change. Each phase emits an `xds_cost_result` JSON
line and the run emits `xds_cost_summary`, so two builds are compared by
diffing output. On a build that carries them, the sparse-CDS deferral metrics
(`kgateway_xds_snapshot_cluster_deferrals_total`) are picked up automatically and
reported per change; on a build without them they read zero.

```bash
# Requires an existing cluster with kgateway installed.
KGW_BENCH_LABEL=my-build make run-xds-cost-bench

# Scale the fleet and pick the validation mode.
KGW_BENCH_LABEL=my-build \
KGW_BENCH_GATEWAYS=8 \
KGW_BENCH_STATIC_BACKENDS=300 \
KGW_BENCH_EDS_ROUTES=200 \
KGW_BENCH_ITERATIONS=15 \
KGW_BENCH_VALIDATION=STRICT \
KGW_BENCH_OUT=/tmp/xdscost.jsonl \
  make run-xds-cost-bench
```

Like StrictChurn it mutates the controller deployment (it pins
`KGW_VALIDATION_MODE` for the run and restores it on teardown), so it is
hard-gated behind `KGW_ENABLE_XDS_COST=true`, which the make target sets.

### XdsFleet Benchmark (fleet-scale capacity ladder)

`xdsfleet_suite.go` is XdsCost at production fan-out. XdsCost prices individual
changes on a fleet a laptop can host; XdsFleet answers the different question of
**how many clients the controller survives** at a realistic shape — thousands of
Services and hundreds of Gateways — which is what separates designs that only
look different in a microbenchmark.

Getting there needs two substitutions, because 800 Gateways with two proxies
each is 1600 Envoy pods that no single machine can run:

- **Gateways are real, their proxies are not.** A `GatewayParameters` pins the
  proxy Deployment to zero replicas, so the control plane does the full
  translation for every Gateway without any Envoy running.
- **xDS streams are synthetic.** The suite opens gRPC ADS streams carrying each
  Gateway's role in node metadata. This requires `KGW_XDS_AUTH=false`, which the
  suite sets: with auth on, the server takes identity from a ServiceAccount JWT
  on the gRPC metadata and a stream without a real pod's token is rejected.
  Identity then comes from the node metadata role, which changes how the role is
  derived but not what is translated for it.

With pod-locality xDS on — the default, and how the suite runs unless
`KGW_FLEET_POD_LOCALITY=false` — every stream must resolve to a real Pod object,
so the suite creates fake Nodes across `KGW_FLEET_ZONES` and binds a fake Pod per
stream. That also makes client identity **per-pod**: `HashLabels` hashes every
augmented label including `kubernetes.io/hostname`, so two proxies of one Gateway
on different nodes are two clients. Client count is
`gateways × min(replicas, zones)`, not `gateways`.

The controller is capped at `KGW_FLEET_MEMORY_LIMIT` (8Gi by default) so that
running out of memory is a clean container restart, which the suite detects via
`restartCount` and reports as `xds_fleet_verdict` with the client count that
killed it. Each wave emits `xds_fleet_wave` with heap, RSS, CPU, allocations,
resource count and transforms, so two builds are compared by diffing the ladder.

```bash
# Requires an existing cluster with kgateway installed.
# Production shape: 400 Gateways x 2 proxies, 50 Gateways per wave.
KGW_BENCH_LABEL=my-build \
KGW_FLEET_SERVICES=6000 \
KGW_FLEET_GATEWAYS=400 \
KGW_FLEET_INLINE_BACKENDS=20 \
KGW_FLEET_STREAMS_PER_GATEWAY=2 \
KGW_FLEET_ZONES=3 \
KGW_FLEET_POD_LOCALITY=true \
KGW_FLEET_WAVES=8 \
KGW_FLEET_MEMORY_LIMIT=8Gi \
KGW_BENCH_OUT=/tmp/xdsfleet.jsonl \
  make run-xds-fleet-bench

# A/B one setting without touching this file: comma-separated KEY=VALUE
# controller environment, snapshotted and restored like the rest.
KGW_FLEET_EXTRA_ENV="KGW_XDS_SHARE_IDENTICAL_RESOURCES=false" make run-xds-fleet-bench
```

Comparing builds is the normal use, and `hack/xds-fleet-ab.sh` does the whole
ladder over several arms: it drains the fleet between arms, installs each arm's
chart and image, and prints the arms side by side at the end. An arm is
`<label>:<chart-dir>:<image-tag>[:<KEY=VALUE,...>]`, so one PR against its own
base is:

```bash
hack/xds-fleet-ab.sh /tmp/ab.jsonl \
  base:/path/to/base/install/helm/kgateway:base-ci1 \
  mypr:/path/to/pr/install/helm/kgateway:pr-ci1
```

**Read the ladder, not a single heap number.** Heap flattens against the cap, so
two builds can report the same heap while one is comfortable and the other is
about to die. The client count each build survives is the number that
discriminates; wave granularity (`KGW_FLEET_GATEWAYS / KGW_FLEET_WAVES`) sets how
precisely you can locate it.

**Resources per client is `425 + services`** in every run measured so far: every
Service becomes a cluster and an endpoint assignment in every client's snapshot
regardless of what that Gateway routes to. Varying `KGW_FLEET_SERVICES` while
holding the Gateway count fixed is therefore the cheapest way to measure what
scoping discovery to referenced backends would be worth, without building it.

Four things cost real time when working on this suite:

- `kgateway_xds_snapshot_syncs_total` only advances for resource kinds the
  metrics layer start-times, so it stays flat for Backend and EndpointSlice
  edits. Use `kgateway_xds_snapshot_transforms_total` as the convergence signal.
- The `xds_snapshot` metrics are label-vecs and are absent entirely until a
  Gateway exists.
- `go test` caches a successful run and replays it under different environment
  variables. The make targets pass `-count=1`; keep it.
- `kubectl port-forward` refuses new connections past roughly 60, which is why
  the suite batches 100 gRPC streams per connection.

Teardown force-deletes the fake pods (`--grace-period=0`) and deletes the
per-run fake Nodes. Both matter: pods bound to fake Nodes have no kubelet to
confirm deletion, so the namespace otherwise hangs in `Terminating` and poisons
the next run, and Nodes are cluster-scoped, so leaked ones silently reuse stale
zone labels and invalidate a locality experiment.

## Framework Architecture

### Components

- **`types.go`**: Data structures and the batching configuration for the baseline and production tests
- **`vcluster_simulator.go`**: Simulates fake cluster resources (nodes, services, endpoints)
- **`loadtest_manager.go`**: Orchestrates test execution and resource management
- **`attachedroutes_suite.go`**: Implements the Attached Routes performance test

### Test Methodology

1. Create simulated cluster with appropriate scale
2. Set up test infrastructure (namespaces, services, gateways)
3. Create baseline routes (1000 or 5000) in batches
4. Wait for all routes to be attached to gateways
5. Restart the controller and wait for the restarted deployment to become ready
6. **Start stopwatch** → Add 1 incremental route
7. Measure time until route is ready and status propagates
8. **Stop stopwatch** → Record performance metrics
9. Measure teardown time for incremental route cleanup

## The "Attached Routes" Test Methodology

This test follows a specific methodology inspired by gateway-api-bench and implements the exact approach recommended by the KGateway team for measuring real-world Gateway API performance.

### Phase-by-Phase Breakdown

1. **Simulation Setup**: Create fake cluster with appropriate scale using VCluster simulator
2. **Infrastructure Setup**: Set up test namespaces and backend services
3. **Gateway Creation**: Create and wait for gateways to be ready
4. **Baseline Routes**: Create N routes (1000 or 5000) in batches with grace periods
5. **Translation Wait**: Wait for all routes to be "attached" to gateways (status propagation)
6. **Route Validation**: Verify baseline routes are accepted and valid
7. **Monitoring Start**: Begin watching gateway status changes with event-driven handlers
8. **STOPWATCH START**: Create 1 additional incremental route
9. **Route Ready**: Curl the gateway until it returns 200 (real traffic validation)
10. **STOPWATCH STOP**: Record "User Time" - the actual time users would experience
11. **Teardown**: Delete the incremental route and measure teardown time

### Why This Methodology

- **Incremental Testing**: Adding 1 route to 1000 existing routes tests real-world scenarios
- **Control Plane Focus**: Measures how fast KGateway processes configuration changes
- **Real Traffic Validation**: Uses curl probing instead of unreliable metrics
- **Status Propagation**: Tests how quickly gateway status reflects route changes
- **Realistic Load**: Tests performance under production-like conditions with simulated backends
