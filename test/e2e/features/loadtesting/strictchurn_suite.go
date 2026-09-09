//go:build e2e

package loadtesting

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

// StrictChurn is a control-plane convergence test for the per-client xDS
// pipeline under a #14184-inspired load shape: strict validation (every
// translated cluster pays an external envoy invocation), a few hundred
// backends, sustained route/Service churn including a dangling backend
// reference, and a controller restart mid-churn. It asserts the anti-starvation
// properties at the traffic level:
//
//   - stable routes keep serving throughout churn and across the restart
//     (connected clients are never stranded on stale or withheld config);
//   - a gateway Envoy rolled mid-churn becomes Ready within a bound (a fresh
//     xDS client's first snapshot is never withheld indefinitely);
//   - new config still converges after the churn stops (a route created
//     post-churn becomes routable within a bound).
//
// To probe the first-connect grace period specifically, set
// KGW_XDS_FIRST_CONNECT_DELAY in the test environment: the suite forwards it
// to the controller deployment for the duration of the run (and restores the
// original value on teardown). `KGW_XDS_FIRST_CONNECT_DELAY=0` runs the raw
// reconnect race the delay exists to narrow; large values probe the other
// boundary (warm clients must keep serving through the delay and rolled Envoys
// must still beat gatewayRolloutBound).
//
// The suite mutates the controller deployment (KGW_VALIDATION_MODE=STRICT,
// plus the optional delay override) and restores it on teardown, so it must
// not run before suites that assume a steady controller. It is registered with
// the suite runner, excluded from the shared CI e2e clusters, and run once at
// the end of each nightly load-test lane. Run it locally via
// `make run-load-tests-strict-churn`.
//
// Honesty note: this suite is a regression pin for the anti-starvation
// properties, not a reproducer of the #14184 wedge. Verified empirically
// (2026-07-10): it passes unchanged against a gated (#13868) build — with the
// strict-validation cache and per-client fan-out fixes on main, per-client
// derivation at this scale completes in microseconds, so the gate's defer
// state never persists for any reference shape a laptop fixture can construct
// (dangling, selector-matches-nothing, selectorless, ExternalName, and
// garbage-CA TLS were all probed; errored clusters are exempt from the gates
// by design, and every kube backend yields a CDS row plus an explicit —
// possibly empty — CLA). Reproducing the field wedge needs production-scale
// derivation lag: the transient defer window stretched by fan-out x config
// size x churn, self-amplified by crashloop reconnects.
//
// A second experiment (KGW_VALIDATOR_MODE=BINARY, 5000 backends, 8 gateways,
// 240s bounds) reproduced the #14184 SYMPTOM — every gateway pod crashlooping
// on its startup probe, "gateways are unable to spawn" — but on BOTH builds
// identically: uncached validation saturates shared translation upstream of
// publication policy, so fresh clients get nothing within the probe window on
// any build (the gated build logged 40 transient defers against 3670 snapshot
// sets — the gate was not the constraint). Saturation is therefore not a
// discriminator either; the gate's marginal harm only separates when
// translation is slow-but-flowing, a regime this fixture cannot reach on one
// machine.

// Scale and timing knobs. The scale is sized so a full per-client fan-out is
// hundreds of strict validations per connected client — large enough that a
// wedged or backlogged pipeline fails the convergence bounds, small enough to
// run on a laptop kind cluster in a few minutes.
const (
	strictChurnCycles  = 8
	churnCycleInterval = 5 * time.Second
	// endpointChurnInterval paces the background EndpointSlice rewriter that
	// keeps the fleet's endpoints from ever quiescing during phases 4-5.
	endpointChurnInterval = 250 * time.Millisecond
	// Defaults for the assertion bounds below, overridable via env for
	// scaled experiments (e.g. KGW_VALIDATOR_MODE=BINARY at thousands of
	// backends pushes translation past the laptop-calibrated defaults on
	// every build; measuring time-to-converge then needs longer bounds).
	defaultRolloutBoundSeconds     = 180
	defaultStableBoundSeconds      = 30
	defaultConvergenceBoundSeconds = 90

	stableRouteHost    = "stable.strict-churn.example.com"
	postChurnRouteHost = "postchurn.strict-churn.example.com"

	// nginxBackendService is the shared nginx fixture the TestKgateway harness
	// deploys (common.SetupSharedNginxBackend) into common.SharedNginxNamespace.
	nginxBackendService = "nginx"
	nginxBackendPort    = 8080

	referenceGrantName = "strict-churn-grant"
)

// Fleet scale, overridable to push past the laptop-friendly defaults when
// hunting load-dependent behavior (e.g. KGW_LOADTEST_BACKENDS=800).
// KGW_LOADTEST_GATEWAYS matters beyond raw load: each Gateway is its own xDS
// role and therefore its own unique client, and on a single-node cluster
// (where every pod shares one locality) distinct Gateways are the ONLY way to
// multiply unique-client fan-out — the axis that stretched the per-client
// derivation window in the #14184 field reports.
var (
	strictChurnBackends = envScale("KGW_LOADTEST_BACKENDS", 200)
	strictChurnRoutes   = envScale("KGW_LOADTEST_ROUTES", 200)
	strictChurnGateways = churnGatewayNames(envScale("KGW_LOADTEST_GATEWAYS", 2))

	// gatewayRolloutBound is how long a rolling-restarted gateway Envoy has to
	// come back Ready. A fresh Envoy is a brand-new xDS client with no
	// fallback config: if its first snapshot is withheld indefinitely, the
	// pod never reports Ready and the rollout wedges ("gateways are unable to
	// spawn"). Generous enough to absorb one full per-client rebuild on a
	// healthy control plane.
	gatewayRolloutBound = fmt.Sprintf("%ds", envScale("KGW_LOADTEST_ROLLOUT_BOUND_SECONDS", defaultRolloutBoundSeconds))
	// stableRouteBound is how quickly the stable route must answer 200 at
	// every checkpoint during churn. It was reachable before churn started,
	// so any sustained failure means the data plane lost working config.
	stableRouteBound = time.Duration(envScale("KGW_LOADTEST_STABLE_BOUND_SECONDS", defaultStableBoundSeconds)) * time.Second
	// convergenceBound is how quickly a route created after the churn stops
	// must become routable end to end. This is the liveness property: the
	// per-client pipeline still publishes after churn plus a restart.
	convergenceBound = time.Duration(envScale("KGW_LOADTEST_CONVERGENCE_BOUND_SECONDS", defaultConvergenceBoundSeconds)) * time.Second
)

func churnGatewayNames(n int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("churn-gw-%d", i+1)
	}
	return names
}

func envScale(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// controllerAppName returns the app.kubernetes.io/name of the controller under
// test. Defaults to the OSS controller; override with
// KGW_LOADTEST_CONTROLLER_APP (e.g. for a downstream distribution) to point
// the deployment lookup at a differently-labeled control plane. The gateway
// class is not parameterized — create a GatewayClass named "kgateway" bound to
// the distribution's controllerName instead.
func controllerAppName() string {
	if app := os.Getenv("KGW_LOADTEST_CONTROLLER_APP"); app != "" {
		return app
	}
	return "kgateway"
}

func controllerSelector() string {
	return fmt.Sprintf("%s=%s", testdefaults.WellKnownAppLabel, controllerAppName())
}

var _ e2e.NewSuiteFunc = NewStrictChurnSuite

func NewStrictChurnSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &StrictChurnSuite{
		LoadTestingSuite: LoadTestingSuite{
			Suite:            suite.Suite{},
			ctx:              ctx,
			testInstallation: testInst,
		},
	}
}

type StrictChurnSuite struct {
	LoadTestingSuite
	loadTestManager       *LoadTestManager
	installNamespace      string
	controllerDeployment  string
	controllerContainer   string
	originalControllerEnv map[string]*corev1.EnvVar
}

func (s *StrictChurnSuite) SetupSuite() {
	// Hard opt-in gate, independent of -run regexes: this suite mutates the
	// controller deployment (strict validation env, a mid-test restart), so a
	// broad invocation like the nightly's unanchored `^TestKgateway` matcher
	// must not pick it up implicitly. `make run-load-tests-strict-churn` sets
	// the variable.
	if os.Getenv("KGW_ENABLE_STRICT_CHURN") != "true" {
		s.T().Skip("StrictChurn mutates the controller deployment; set KGW_ENABLE_STRICT_CHURN=true (or use `make run-load-tests-strict-churn`) to run it")
	}
	s.loadTestManager = NewLoadTestManager(s.ctx, s.testInstallation,
		fmt.Sprintf("kgateway-strictchurn-%d", time.Now().UnixNano()))
	s.installNamespace = s.testInstallation.Metadata.InstallNamespace

	name, err := s.resolveControllerDeployment()
	s.Require().NoError(err, "should find the kgateway controller deployment in %s", s.installNamespace)
	s.controllerDeployment = name

	// The curl pod issues the data-plane probes; the harness's shared nginx
	// (nginx-shared, applied once by TestKgateway's base setup) is the real
	// backend behind the stable and post-churn routes (the simulated services
	// have unroutable endpoints, so they can't answer traffic).
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.CurlPodManifest)
	s.Require().NoError(err, "should apply curl pod")
	s.testInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx,
		testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{LabelSelector: testdefaults.CurlPodLabelSelector})

	envExprs := []string{"KGW_VALIDATION_MODE=STRICT"}
	if d := os.Getenv("KGW_XDS_FIRST_CONNECT_DELAY"); d != "" {
		envExprs = append(envExprs, "KGW_XDS_FIRST_CONNECT_DELAY="+d)
	}
	// KGW_VALIDATOR_MODE=BINARY disables the strict-validation verdict cache,
	// restoring the #14184-era per-call validation cost on every rebuild —
	// the field-shaped amplifier of the per-client derivation window.
	if m := os.Getenv("KGW_VALIDATOR_MODE"); m != "" {
		envExprs = append(envExprs, "KGW_VALIDATOR_MODE="+m)
	}
	s.Require().NoError(s.snapshotControllerEnv(envExprs), "should snapshot controller env before overriding it")
	s.T().Logf("Setting controller env on deployment/%s in %s: %v", s.controllerDeployment, s.installNamespace, envExprs)
	s.Require().NoError(s.setControllerEnv(envExprs...))
}

func (s *StrictChurnSuite) TearDownSuite() {
	// SetupSuite skipped (opt-in gate) before creating anything: do not touch
	// the cluster — the curl/nginx manifests may be in use by other suites.
	if s.loadTestManager == nil {
		return
	}
	// Restore the controller before anything else so a failed run doesn't
	// leave the install in strict mode for whoever uses the cluster next.
	if s.originalControllerEnv != nil {
		if err := s.restoreControllerEnv(); err != nil {
			s.T().Logf("WARNING: failed to restore controller env: %v", err)
		}
	}
	if s.loadTestManager != nil {
		if err := s.loadTestManager.CleanupAll(); err != nil {
			s.T().Logf("Warning: failed to cleanup load test namespaces: %v", err)
		}
	}
	// The nginx-shared ReferenceGrant outlives the per-run namespaces
	// (nginx-shared belongs to the harness), so delete it explicitly.
	_ = s.testInstallation.ClusterContext.Client.Delete(s.ctx, &gwv1b1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      referenceGrantName,
			Namespace: common.SharedNginxNamespace,
		},
	})
	// The shared nginx backend belongs to the harness, not this suite — do
	// not delete it. The curl pod is per-suite (siblings re-apply it).
	_ = s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, testdefaults.CurlPodManifest)
}

func (s *StrictChurnSuite) resolveControllerDeployment() (string, error) {
	out, _, err := s.testInstallation.ClusterContext.Cli.Execute(s.ctx,
		"get", "deploy", "-n", s.installNamespace,
		"-l", controllerSelector(),
		"-o", "jsonpath={.items[0].metadata.name}")
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(out)
	if name == "" {
		return "", fmt.Errorf("no deployment matching %q in %s", controllerSelector(), s.installNamespace)
	}
	return name, nil
}

func (s *StrictChurnSuite) snapshotControllerEnv(envExprs []string) error {
	deployment := &appsv1.Deployment{}
	if err := s.testInstallation.ClusterContext.Client.Get(s.ctx, types.NamespacedName{
		Namespace: s.installNamespace,
		Name:      s.controllerDeployment,
	}, deployment); err != nil {
		return err
	}

	names := make(map[string]struct{}, len(envExprs))
	for _, expr := range envExprs {
		name, _, _ := strings.Cut(expr, "=")
		names[name] = struct{}{}
	}

	originals := make(map[string]*corev1.EnvVar, len(names))
	for name := range names {
		originals[name] = nil
	}
	container, err := findControllerContainer(deployment, "")
	if err != nil {
		return err
	}
	s.controllerContainer = container.Name
	for _, env := range container.Env {
		if _, ok := names[env.Name]; ok {
			originals[env.Name] = env.DeepCopy()
		}
	}
	s.originalControllerEnv = originals
	return nil
}

func (s *StrictChurnSuite) restoreControllerEnv() error {
	deployment := &appsv1.Deployment{}
	if err := s.testInstallation.ClusterContext.Client.Get(s.ctx, types.NamespacedName{
		Namespace: s.installNamespace,
		Name:      s.controllerDeployment,
	}, deployment); err != nil {
		return err
	}

	before := deployment.DeepCopy()
	container, err := findControllerContainer(deployment, s.controllerContainer)
	if err != nil {
		return err
	}
	restoreControllerEnvVars(container, s.originalControllerEnv)

	if err := s.testInstallation.ClusterContext.Client.Patch(s.ctx, deployment, client.MergeFrom(before)); err != nil {
		return err
	}
	return s.testInstallation.Actions.Kubectl().DeploymentRolloutStatus(s.ctx,
		s.controllerDeployment, "-n", s.installNamespace, "--timeout=120s")
}

func restoreControllerEnvVars(container *corev1.Container, originals map[string]*corev1.EnvVar) {
	restored := make(map[string]struct{}, len(originals))
	env := make([]corev1.EnvVar, 0, len(container.Env))
	for _, current := range container.Env {
		original, tracked := originals[current.Name]
		if !tracked {
			env = append(env, current)
			continue
		}
		restored[current.Name] = struct{}{}
		if original != nil {
			env = append(env, *original.DeepCopy())
		}
	}
	for name, original := range originals {
		if original == nil {
			continue
		}
		if _, ok := restored[name]; !ok {
			env = append(env, *original.DeepCopy())
		}
	}
	container.Env = env
}

func findControllerContainer(deployment *appsv1.Deployment, name string) (*corev1.Container, error) {
	for i := range deployment.Spec.Template.Spec.Containers {
		if deployment.Spec.Template.Spec.Containers[i].Name == name ||
			(name == "" && deployment.Spec.Template.Spec.Containers[i].Name == "controller") {
			return &deployment.Spec.Template.Spec.Containers[i], nil
		}
	}
	if name != "" {
		return nil, fmt.Errorf("deployment/%s has no container named %q", deployment.Name, name)
	}
	if len(deployment.Spec.Template.Spec.Containers) == 0 {
		return nil, fmt.Errorf("deployment/%s has no containers", deployment.Name)
	}
	return &deployment.Spec.Template.Spec.Containers[0], nil
}

// setControllerEnv applies `kubectl set env` expressions (KEY=VALUE or KEY-)
// to the controller deployment in a single call and waits for the resulting
// rollout.
func (s *StrictChurnSuite) setControllerEnv(envExprs ...string) error {
	args := append([]string{
		"set", "env", "-n", s.installNamespace,
		"deployment/" + s.controllerDeployment,
		"--containers=" + s.controllerContainer,
	}, envExprs...)
	if err := s.testInstallation.Actions.Kubectl().RunCommand(s.ctx, args...); err != nil {
		return err
	}
	return s.testInstallation.Actions.Kubectl().DeploymentRolloutStatus(s.ctx,
		s.controllerDeployment, "-n", s.installNamespace, "--timeout=120s")
}

func (s *StrictChurnSuite) TestStrictChurnConvergence() {
	s.T().Log("=== StrictChurn: strict validation + route/Service churn + controller restart ===")

	// Phase 1: scale fixture — fake Services/EndpointSlices and baseline routes.
	s.Require().NoError(s.loadTestManager.SetupSimulation(strictChurnBackends, "strict-churn"),
		"should setup cluster simulation")
	s.Require().NoError(s.loadTestManager.SetupTestInfrastructure(), "should setup test infrastructure")
	s.createReferenceGrants()

	s.Require().NoError(s.loadTestManager.CreateGateways(strictChurnGateways), "should create gateways")
	s.Require().NoError(s.loadTestManager.WaitForGatewayReadiness(gatewayReadinessTimeout), "gateways should be ready")

	config := &AttachedRoutesConfig{
		Gateways:    strictChurnGateways,
		Routes:      strictChurnRoutes,
		GracePeriod: GetConfig(strictChurnRoutes).GracePeriod,
		BatchSize:   GetOptimalBatchSize(strictChurnRoutes),
	}
	s.Require().NoError(s.loadTestManager.CreateRoutesBatched(config), "should create baseline routes")
	s.waitForAttachedRoutes(strictChurnGateways[0], strictChurnRoutes/len(strictChurnGateways), translationCompletionTimeout)

	// Phase 2: a stable route to a real backend, verified working before any
	// churn. This is the route that must never break.
	s.createRouteToNginx("stable-route", strictChurnGateways[0], stableRouteHost)
	s.assertRouteServes(strictChurnGateways[0], stableRouteHost, stableRouteBound)
	s.T().Log("Stable route serving 200 — starting churn")

	// Phase 3: two permanently-unhealthy references that persist through the
	// run, attached to the stable route's gateway. The dangling route's
	// Service never exists (kgateway replaces the route with a 500 direct
	// response); the starved route's Service exists but its selector matches
	// no pods, so its cluster never has a ready endpoint. Neither shape
	// wedges the #13868 gates on its own (the per-client CDS row and an
	// empty CLA both materialize within milliseconds, so the gate's defer is
	// transient — verified against a gated build); they are here as permanent
	// sources of not-fully-ready reference state under which publication must
	// keep flowing for everything else.
	s.createDanglingRoute(strictChurnGateways[0])
	s.createStarvedRoute(strictChurnGateways[0])

	// Phase 4: churn — create/delete a Service+EndpointSlice and a route each
	// cycle, restart the controller halfway through, and require the stable
	// route back at 200 before the next cycle. A background churner also
	// rewrites the simulated fleet's EndpointSlices continuously: real
	// clusters never quiesce, and an all-or-nothing consistency gate that
	// requires a globally-consistent instant will never find one while
	// endpoints keep moving. (This is the load shape that separates a
	// bounded-wait control plane from a wedged one; quiet-endpoint runs
	// converge even on builds with the unbounded gate.)
	churnerCtx, stopChurner := context.WithCancel(s.ctx)
	defer stopChurner()
	s.startEndpointChurner(churnerCtx)
	for cycle := range strictChurnCycles {
		s.runChurnCycle(cycle)

		// Client identity churn: roll each gateway's Envoy once mid-run. A
		// rolled pod reconnects as a brand-new xDS client whose first
		// snapshot must be built from scratch while the fleet churns —
		// completion is asserted after the loop (gatewayRolloutBound). The
		// schedule spreads rolls across cycles (gcd(3, strictChurnCycles)=1,
		// so up to strictChurnCycles gateways land on distinct cycles; for
		// the default two gateways this is the original cycles 2 and 5).
		for i, gw := range strictChurnGateways {
			if cycle == (2+i*3)%strictChurnCycles {
				s.rollGateway(gw)
			}
		}

		if cycle == strictChurnCycles/2 {
			s.T().Logf("Cycle %d: restarting controller mid-churn", cycle)
			s.Require().NoError(s.testInstallation.Actions.Kubectl().RestartDeploymentAndWait(s.ctx,
				s.controllerDeployment, "-n", s.installNamespace), "controller should restart cleanly mid-churn")
		}

		s.assertRouteServes(strictChurnGateways[0], stableRouteHost, stableRouteBound)
		time.Sleep(churnCycleInterval)
	}

	// Phase 4b: every rolled gateway must finish its rollout — the freshly
	// started Envoy only reports Ready once it has received config, so a
	// control plane that withholds a new client's first snapshot indefinitely
	// fails here with a wedged rollout.
	for _, gw := range strictChurnGateways {
		s.Require().NoError(s.testInstallation.Actions.Kubectl().DeploymentRolloutStatus(s.ctx,
			gw, "-n", s.loadTestManager.testNamespace, "--timeout="+gatewayRolloutBound),
			"gateway %s rollout must complete: a fresh Envoy client must receive its first xDS snapshot", gw)
	}
	s.T().Log("Route churn + gateway rolls complete — asserting convergence under live endpoint churn")

	// Phase 5: liveness — config created while the fleet's endpoints are
	// still moving must flow to the data plane within the bound. A control
	// plane that waits for a globally-consistent instant never finds one on
	// a busy fleet; unconditional publication must deliver regardless.
	s.createRouteToNginx("postchurn-route", strictChurnGateways[1], postChurnRouteHost)
	s.assertRouteServes(strictChurnGateways[1], postChurnRouteHost, convergenceBound)
	stopChurner()

	s.T().Log("=== StrictChurn PASSED: stable traffic held, gateway rolls completed, post-churn config converged ===")
}

// runChurnCycle creates this cycle's Service+EndpointSlice+route, then deletes
// the previous cycle's, so the controller continuously sees backend and route
// add/remove events without the resource count growing.
func (s *StrictChurnSuite) runChurnCycle(cycle int) {
	simNS := s.loadTestManager.simulator.config.Namespace
	svcName := fmt.Sprintf("churn-svc-%d", cycle)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: simNS,
			Labels:    map[string]string{"loadtest": "true", "churn": "true"},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": svcName},
			Ports: []corev1.ServicePort{{
				Name: "http", Port: 80, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP,
			}},
		},
	}
	port := int32(8080)
	portName := "http"
	protocol := corev1.ProtocolTCP
	eps := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: simNS,
			Labels: map[string]string{
				"kubernetes.io/service-name": svcName,
				"loadtest":                   "true",
				"churn":                      "true",
			},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{{
			Addresses: []string{fmt.Sprintf("10.250.0.%d", (cycle%250)+1)},
		}},
		Ports: []discoveryv1.EndpointPort{{Name: &portName, Port: &port, Protocol: &protocol}},
	}
	route := s.buildChurnRoute(fmt.Sprintf("churn-route-%d", cycle), strictChurnGateways[cycle%len(strictChurnGateways)],
		fmt.Sprintf("churn-%d.strict-churn.example.com", cycle), svcName, simNS)

	s.Require().NoError(s.testInstallation.ClusterContext.Client.Create(s.ctx, svc), "churn service create")
	s.Require().NoError(s.testInstallation.ClusterContext.Client.Create(s.ctx, eps), "churn endpointslice create")
	s.Require().NoError(s.testInstallation.ClusterContext.Client.Create(s.ctx, route), "churn route create")

	// Delete the previous cycle's resources (kept alive one cycle so the
	// controller always overlaps an add with a remove).
	if cycle > 0 {
		prev := cycle - 1
		prevSvc := fmt.Sprintf("churn-svc-%d", prev)
		_ = s.testInstallation.Actions.Kubectl().RunCommand(s.ctx,
			"delete", "httproute", fmt.Sprintf("churn-route-%d", prev),
			"-n", s.loadTestManager.testNamespace, "--ignore-not-found", "--wait=false")
		_ = s.testInstallation.Actions.Kubectl().RunCommand(s.ctx,
			"delete", "service", prevSvc, "-n", simNS, "--ignore-not-found", "--wait=false")
		_ = s.testInstallation.Actions.Kubectl().RunCommand(s.ctx,
			"delete", "endpointslice", prevSvc, "-n", simNS, "--ignore-not-found", "--wait=false")
	}
	// The last cycle's leftovers are deleted by namespace teardown.
}

func (s *StrictChurnSuite) createDanglingRoute(gateway string) {
	route := s.buildChurnRoute("dangling-route", gateway,
		"dangling.strict-churn.example.com", "no-such-service", s.loadTestManager.testNamespace)
	s.Require().NoError(s.testInstallation.ClusterContext.Client.Create(s.ctx, route), "dangling route create")
	s.loadTestManager.createdRoutes = append(s.loadTestManager.createdRoutes, route)
}

// startEndpointChurner continuously rewrites simulated EndpointSlices, one
// every endpointChurnInterval, rotating through the fleet. Each write is a
// real input change (the endpoint IP moves), so every referenced backend's
// per-client endpoint translation keeps re-running for every connected
// client — the steady-state load shape of a production fleet where pods are
// always rolling somewhere.
func (s *StrictChurnSuite) startEndpointChurner(ctx context.Context) {
	cfg := s.loadTestManager.simulator.config
	simNS := cfg.Namespace
	// The simulator sizes the fleet itself: the number of sim-service-*
	// EndpointSlices is FakeNodeCount*ServicesPerNode, NOT strictChurnBackends.
	// Rotate over what actually exists or the churner runs off the end of the
	// fleet and stops producing churn.
	totalSimServices := cfg.FakeNodeCount * cfg.ServicesPerNode
	s.Require().Positive(totalSimServices, "simulation must have services to churn")
	go func() {
		ticker := time.NewTicker(endpointChurnInterval)
		defer ticker.Stop()
		attempts, writes := 0, 0
		for {
			select {
			case <-ctx.Done():
				s.T().Logf("Endpoint churner stopped after %d EndpointSlice rewrites (%d attempts)", writes, attempts)
				return
			case <-ticker.C:
			}
			idx := attempts % totalSimServices
			ip := fmt.Sprintf("10.246.%d.%d", (attempts/250)%200, attempts%250+1)
			// Advance every tick regardless of outcome so one unpatchable
			// slice can never wedge the rotation.
			attempts++
			patch := fmt.Sprintf(`[{"op":"replace","path":"/endpoints/0/addresses/0","value":%q}]`, ip)
			eps := &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("sim-service-%d", idx), Namespace: simNS,
			}}
			if err := s.testInstallation.ClusterContext.Client.Patch(ctx, eps,
				client.RawPatch(types.JSONPatchType, []byte(patch))); err != nil {
				if ctx.Err() != nil {
					return
				}
				s.T().Logf("endpoint churn patch failed (continuing): %v", err)
				continue
			}
			writes++
		}
	}()
}

// rollGateway triggers a rolling restart of a gateway's Envoy deployment
// (named after the Gateway by the deployer) without waiting — completion is
// asserted later against gatewayRolloutBound.
func (s *StrictChurnSuite) rollGateway(gateway string) {
	s.T().Logf("Rolling gateway %s (new xDS client identity)", gateway)
	s.Require().NoError(s.testInstallation.Actions.Kubectl().RestartDeployment(s.ctx,
		gateway, "-n", s.loadTestManager.testNamespace), "should roll gateway %s", gateway)
}

// createStarvedRoute creates a Service whose selector matches no pods plus a
// route referencing it: the backendRef resolves (the Service exists), so the
// gateway's referenced-cluster set permanently contains an EDS cluster that
// will never have a ready endpoint.
func (s *StrictChurnSuite) createStarvedRoute(gateway string) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "starved-svc",
			Namespace: s.loadTestManager.testNamespace,
			Labels:    map[string]string{"loadtest": "true"},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "starved-no-such-pod"},
			Ports: []corev1.ServicePort{{
				Name: "http", Port: 80, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP,
			}},
		},
	}
	s.Require().NoError(s.loadTestManager.createResource(svc), "starved service create")
	route := s.buildChurnRoute("starved-route", gateway,
		"starved.strict-churn.example.com", "starved-svc", s.loadTestManager.testNamespace)
	s.Require().NoError(s.testInstallation.ClusterContext.Client.Create(s.ctx, route), "starved route create")
	s.loadTestManager.createdRoutes = append(s.loadTestManager.createdRoutes, route)
}

func (s *StrictChurnSuite) createRouteToNginx(name, gateway, hostname string) {
	route := s.buildChurnRoute(name, gateway, hostname,
		nginxBackendService, common.SharedNginxNamespace)
	s.Require().NoError(s.testInstallation.ClusterContext.Client.Create(s.ctx, route), "route %s create", name)
	s.loadTestManager.createdRoutes = append(s.loadTestManager.createdRoutes, route)
}

func (s *StrictChurnSuite) buildChurnRoute(name, gateway, hostname, backendSvc, backendNS string) *gwv1.HTTPRoute {
	pathType := gwv1.PathMatchPathPrefix
	pathValue := "/"
	ns := gwv1.Namespace(backendNS)
	port := gwv1.PortNumber(80)
	if backendNS == common.SharedNginxNamespace {
		port = gwv1.PortNumber(nginxBackendPort)
	}
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.loadTestManager.testNamespace,
			Labels:    map[string]string{"loadtest": "true"},
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{{Name: gwv1.ObjectName(gateway)}},
			},
			Hostnames: []gwv1.Hostname{gwv1.Hostname(hostname)},
			Rules: []gwv1.HTTPRouteRule{{
				Matches: []gwv1.HTTPRouteMatch{{
					Path: &gwv1.HTTPPathMatch{Type: &pathType, Value: &pathValue},
				}},
				BackendRefs: []gwv1.HTTPBackendRef{{
					BackendRef: gwv1.BackendRef{
						BackendObjectReference: gwv1.BackendObjectReference{
							Name:      gwv1.ObjectName(backendSvc),
							Namespace: &ns,
							Port:      &port,
						},
					},
				}},
			}},
		},
	}
}

// createReferenceGrants permits the test namespace's routes to reference
// Services in the simulation and nginx namespaces. Without these, every
// cross-namespace backendRef is RefNotPermitted and no clusters are built —
// which would silence the strict-validation load this scenario exists to apply.
//
// The grants are UPSERTED, not created-and-ignore-exists: the nginx-shared
// grant has a fixed name in a namespace the harness owns (CleanupAll never
// deletes it), so a leftover grant from a previous run permits only that
// run's — long deleted — test namespace. Silently keeping it starves every
// route of the current run into a steady 500 (RefNotPermitted -> route
// replacement), which is indistinguishable at the data plane from the xDS
// starvation this suite exists to detect.
func (s *StrictChurnSuite) createReferenceGrants() {
	for _, targetNS := range []string{
		s.loadTestManager.simulator.config.Namespace,
		common.SharedNginxNamespace,
	} {
		grant := &gwv1b1.ReferenceGrant{
			ObjectMeta: metav1.ObjectMeta{
				Name:      referenceGrantName,
				Namespace: targetNS,
				Labels:    map[string]string{"loadtest": "true"},
			},
			Spec: gwv1b1.ReferenceGrantSpec{
				From: []gwv1b1.ReferenceGrantFrom{{
					Group:     gwv1b1.Group(gwv1.GroupName),
					Kind:      "HTTPRoute",
					Namespace: gwv1b1.Namespace(s.loadTestManager.testNamespace),
				}},
				To: []gwv1b1.ReferenceGrantTo{{
					Group: "",
					Kind:  "Service",
				}},
			},
		}
		s.Require().NoError(s.upsertReferenceGrant(grant),
			"should create ReferenceGrant in %s", targetNS)
	}
}

func (s *StrictChurnSuite) upsertReferenceGrant(grant *gwv1b1.ReferenceGrant) error {
	err := s.testInstallation.ClusterContext.Client.Create(s.ctx, grant)
	if err == nil || !apierrors.IsAlreadyExists(err) {
		return err
	}
	existing := &gwv1b1.ReferenceGrant{}
	if err := s.testInstallation.ClusterContext.Client.Get(s.ctx,
		client.ObjectKeyFromObject(grant), existing); err != nil {
		return err
	}
	existing.Spec = grant.Spec
	existing.Labels = grant.Labels
	return s.testInstallation.ClusterContext.Client.Update(s.ctx, existing)
}

// assertRouteServes requires the given hostname to answer 200 with the nginx
// body through the gateway's in-cluster Service within the bound.
func (s *StrictChurnSuite) assertRouteServes(gateway, hostname string, bound time.Duration) {
	s.testInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{
				Name: gateway, Namespace: s.loadTestManager.testNamespace,
			})),
			curl.WithHostHeader(hostname),
			curl.WithPath("/"),
			curl.WithPort(80),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(testdefaults.NginxResponse),
		},
		bound, 2*time.Second,
	)
}

// waitForAttachedRoutes polls a gateway's listener status until at least
// minRoutes report attached.
func (s *StrictChurnSuite) waitForAttachedRoutes(gateway string, minRoutes int, timeout time.Duration) {
	s.Require().Eventually(func() bool {
		gw := &gwv1.Gateway{}
		if err := s.testInstallation.ClusterContext.Client.Get(s.ctx,
			types.NamespacedName{Namespace: s.loadTestManager.testNamespace, Name: gateway}, gw); err != nil {
			return false
		}
		if len(gw.Status.Listeners) == 0 {
			return false
		}
		attached := int(gw.Status.Listeners[0].AttachedRoutes)
		s.T().Logf("Gateway %s: AttachedRoutes=%d (waiting for >=%d)", gateway, attached, minRoutes)
		return attached >= minRoutes
	}, timeout, translationPollingInterval, "baseline routes should attach to %s", gateway)
}
