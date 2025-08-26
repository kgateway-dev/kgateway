package loadtesting

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
)

// Timeout constants for the AttachedRoutes test suite
const (
	// General operation timeouts
	gatewayReadinessTimeout      = 60 * time.Second
	routeConditionTimeout        = 60 * time.Second
	translationCompletionTimeout = 5 * time.Minute

	// Cleanup and sleep intervals
	cleanupSleepInterval    = 2 * time.Second
	monitoringSleepInterval = 500 * time.Millisecond

	// Polling intervals
	translationPollingInterval = 5 * time.Second
	gatewayPollingInterval     = 1 * time.Second
	teardownPollingInterval    = 100 * time.Millisecond

	// Performance threshold timeouts
	baselineMaxUserTime       = 30 * time.Second
	baselineMaxTeardownTime   = 10 * time.Second
	productionMaxUserTime     = 90 * time.Second
	productionMaxTeardownTime = 20 * time.Second
	largeScaleMaxUserTime     = 2 * time.Minute
	largeScaleMaxTeardownTime = 30 * time.Second
)

var _ e2e.NewSuiteFunc = NewAttachedRoutesSuite

func NewAttachedRoutesSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &AttachedRoutesSuite{
		LoadTestingSuite: LoadTestingSuite{
			Suite:            suite.Suite{},
			ctx:              ctx,
			testInstallation: testInst,
		},
	}
}

type AttachedRoutesSuite struct {
	LoadTestingSuite
	loadTestManager *LoadTestManager
}

func (s *AttachedRoutesSuite) SetupSuite() {
	testTimestamp := time.Now().UnixNano()
	testNamespace := fmt.Sprintf("kgateway-loadtest-%d", testTimestamp)
	s.loadTestManager = NewLoadTestManager(s.ctx, s.testInstallation, testNamespace)
}

func (s *AttachedRoutesSuite) TearDownSuite() {
	if s.loadTestManager != nil {
		s.loadTestManager.CleanupAll()
	}
}

func (s *AttachedRoutesSuite) BeforeTest(suiteName, testName string) {
	testTimestamp := time.Now().UnixNano()
	s.loadTestManager.testNamespace = fmt.Sprintf("kgateway-loadtest-%d", testTimestamp)
	s.forceCleanupTestResources()
}

func (s *AttachedRoutesSuite) AfterTest(suiteName, testName string) {
	if s.loadTestManager != nil {
		s.loadTestManager.CleanupAll()
		// Reset state
		s.loadTestManager.createdResources = []client.Object{}
		s.loadTestManager.createdGateways = []*gwv1.Gateway{}
		s.loadTestManager.createdRoutes = []*gwv1.HTTPRoute{}
	}
}

func (s *AttachedRoutesSuite) forceCleanupTestResources() {
	deleteOptions := &client.DeleteOptions{
		GracePeriodSeconds: func() *int64 { i := int64(0); return &i }(),
		PropagationPolicy:  func() *metav1.DeletionPropagation { p := metav1.DeletePropagationBackground; return &p }(),
	}

	// Clean up gateways
	gatewayList := &gwv1.GatewayList{}
	if err := s.testInstallation.ClusterContext.Client.List(s.ctx, gatewayList, &client.ListOptions{
		Namespace: LoadTestNamespace,
	}); err == nil {
		testGateways := []string{"test-gateway", "gw-1", "gw-2", "gw-3"}
		for _, gateway := range gatewayList.Items {
			for _, testGW := range testGateways {
				if gateway.Name == testGW {
					s.testInstallation.ClusterContext.Client.Delete(s.ctx, &gateway, deleteOptions)
					break
				}
			}
		}
	}

	// Clean up routes
	routeList := &gwv1.HTTPRouteList{}
	if err := s.testInstallation.ClusterContext.Client.List(s.ctx, routeList, &client.ListOptions{
		Namespace: LoadTestNamespace,
	}); err == nil {
		for _, route := range routeList.Items {
			s.testInstallation.ClusterContext.Client.Delete(s.ctx, &route, deleteOptions)
		}
	}

	time.Sleep(cleanupSleepInterval)
}

func (s *AttachedRoutesSuite) TestAttachedRoutesBaseline() {
	s.T().Log("=== AttachedRoutes Performance Test: Baseline (1000 routes) ===")
	s.runTestWithConfig(1000)
}

func (s *AttachedRoutesSuite) TestAttachedRoutesProduction() {
	s.T().Log("=== AttachedRoutes Performance Test: Production Scale (5000 routes) ===")
	s.runTestWithConfig(5000)
}

func (s *AttachedRoutesSuite) runTestWithConfig(routeCount int) {
	config := &AttachedRoutesConfig{
		Gateways:    []string{"test-gateway"},
		Routes:      routeCount,
		GracePeriod: GetConfig(routeCount).GracePeriod,
		SingleNode:  true,
		BatchSize:   GetOptimalBatchSize(routeCount),
	}

	results := s.runIncrementalRouteTestWithSimulation(config)
	s.validateIncrementalPerformanceThresholds(results)
	s.reportIncrementalResults(results)
}

func (s *AttachedRoutesSuite) runIncrementalRouteTestWithSimulation(config *AttachedRoutesConfig) *TestResults {
	startTime := time.Now()
	results := &TestResults{
		TestType:     "IncrementalRouteAdditionWithSimulation",
		StartTime:    startTime,
		RouteCount:   config.Routes + 1,
		GatewayCount: len(config.Gateways),
		Watchers:     make(map[types.NamespacedName]*Watcher),
	}

	// Setup phases
	s.setupSimulation(config, results)
	s.setupInfrastructure()
	s.createAndWaitForGateways(config)
	s.createBaselineRoutes(config)
	s.waitForTranslationCompletion(config.Gateways, config.Routes, translationCompletionTimeout)
	s.verifyRouteValid(config.Gateways[0])

	// Monitoring and incremental test
	monitorCtx, cancelMonitor := context.WithCancel(s.ctx)
	defer cancelMonitor()

	s.startAttachedRoutesWatchers(monitorCtx, config.Gateways, results)
	time.Sleep(monitoringSleepInterval)

	// Measure incremental route performance
	s.measureIncrementalRoutePerformance(config, results)

	// Collect final metrics
	results.KGatewayMetrics = s.loadTestManager.CollectKGatewayMetrics()
	results.SimulatedCluster = s.loadTestManager.GetSimulationMetrics()

	cancelMonitor()
	time.Sleep(monitoringSleepInterval)

	results.EndTime = time.Now()
	results.Duration = results.EndTime.Sub(results.StartTime)

	for _, watcher := range results.Watchers {
		results.TotalWrites += len(watcher.Samples)
	}

	s.T().Logf("Final monitoring results: %d total status updates captured during incremental test", results.TotalWrites)
	return results
}

func (s *AttachedRoutesSuite) setupSimulation(config *AttachedRoutesConfig, results *TestResults) {
	s.T().Logf("Phase 1: Setting up cluster simulation for %d baseline routes", config.Routes)
	simulationStart := time.Now()
	err := s.loadTestManager.SetupSimulation(config.Routes, "incremental-attachedroutes")
	s.Require().NoError(err, "Should setup cluster simulation")

	simMetrics := s.loadTestManager.GetSimulationMetrics()
	s.T().Logf("Simulation created in %v: %d nodes, %d services, %d endpoints (memory: %d bytes)",
		time.Since(simulationStart), simMetrics.Nodes, simMetrics.FakeServices, simMetrics.FakePods, simMetrics.MemoryUsage)
}

func (s *AttachedRoutesSuite) setupInfrastructure() {
	s.T().Log("Phase 2: Setting up test infrastructure")
	err := s.loadTestManager.SetupTestInfrastructure()
	s.Require().NoError(err, "Should setup test infrastructure")
}

func (s *AttachedRoutesSuite) createAndWaitForGateways(config *AttachedRoutesConfig) {
	s.T().Log("Phase 3: Creating test gateways")
	err := s.loadTestManager.CreateGateways(config.Gateways)
	s.Require().NoError(err, "Should create gateways")

	err = s.loadTestManager.WaitForGatewayReadiness(gatewayReadinessTimeout)
	s.Require().NoError(err, "Gateways should be ready")
}

func (s *AttachedRoutesSuite) createBaselineRoutes(config *AttachedRoutesConfig) {
	s.T().Logf("Phase 5: Creating %d baseline routes in batches", config.Routes)
	baselineStart := time.Now()
	err := s.loadTestManager.CreateRoutesBatched(config)
	s.Require().NoError(err, "Should create baseline routes")
	s.T().Logf("Baseline routes created in: %v", time.Since(baselineStart))
}

func (s *AttachedRoutesSuite) measureIncrementalRoutePerformance(config *AttachedRoutesConfig, results *TestResults) {
	s.T().Log("Phase 9: === STARTING STOPWATCH === Creating 1 incremental HTTPRoute")
	stopwatchStart := time.Now()

	incrementalRoute := s.createSingleIncrementalRoute(config.Gateways[0])
	s.Require().NotNil(incrementalRoute, "Should create incremental route")

	s.T().Log("Phase 10: Probing gateway until incremental route returns 200")
	routeReadyTime := s.curlGatewayUntilReady(config.Gateways[0], config.Routes)

	stopwatchEnd := time.Now()
	userTime := stopwatchEnd.Sub(stopwatchStart)
	s.T().Logf("Phase 11: === STOPWATCH STOPPED === User Time: %v (incremental route)", userTime)

	results.SetupTime = userTime
	results.RouteReadyTime = routeReadyTime

	// Measure teardown
	s.T().Log("Phase 12: Measuring teardown time for incremental route")
	teardownStart := time.Now()
	err := s.deleteIncrementalRoute(incrementalRoute)
	s.Require().NoError(err, "Should delete incremental route")

	results.TeardownTime = s.waitForRouteTeardown(config.Gateways, config.Routes, teardownStart)
	s.T().Logf("=== TEARDOWN COMPLETE === Teardown Time: %v", results.TeardownTime)
}

func (s *AttachedRoutesSuite) startAttachedRoutesWatchers(ctx context.Context, gateways []string, results *TestResults) {
	for _, gatewayName := range gateways {
		namespacedName := types.NamespacedName{
			Name:      gatewayName,
			Namespace: s.loadTestManager.testNamespace,
		}

		watcher := &Watcher{
			Name:    namespacedName,
			Last:    0,
			Samples: []Sample{},
		}
		results.Watchers[namespacedName] = watcher
		go s.watchAttachedRoutesWithEvents(ctx, namespacedName, watcher)
	}
}

func (s *AttachedRoutesSuite) watchAttachedRoutesWithEvents(ctx context.Context, gatewayName types.NamespacedName, watcher *Watcher) {
	var mu sync.Mutex
	eventCount := 0

	s.T().Logf("Starting event-driven monitoring for gateway: %s", gatewayName.String())

	handler := func(gateway *gwv1.Gateway) {
		mu.Lock()
		defer mu.Unlock()

		eventCount++
		if len(gateway.Status.Listeners) == 0 {
			return
		}

		attachedRoutes := int(gateway.Status.Listeners[0].AttachedRoutes)
		s.T().Logf("Gateway event #%d: %s AttachedRoutes=%d (ResourceVersion=%s)",
			eventCount, gatewayName.String(), attachedRoutes, gateway.ResourceVersion)

		watcher.Last = attachedRoutes
		if watcher.Samples == nil {
			watcher.Samples = []Sample{}
		}

		watcher.Samples = append(watcher.Samples, Sample{
			Time:           time.Now(),
			AttachedRoutes: attachedRoutes,
		})

		s.T().Logf("Status update recorded: AttachedRoutes=%d (total samples: %d)",
			attachedRoutes, len(watcher.Samples))
	}

	s.T().Logf("Registering event handler for gateway: %s", gatewayName.String())
	s.loadTestManager.RegisterGatewayHandler(gatewayName, handler)

	<-ctx.Done()

	s.T().Logf("Event monitoring stopped for gateway: %s (total events: %d, recorded samples: %d)",
		gatewayName.String(), eventCount, len(watcher.Samples))

	s.loadTestManager.DeregisterGatewayHandler(gatewayName, handler)
}

func (s *AttachedRoutesSuite) waitForTranslationCompletion(gateways []string, expectedRoutes int, timeout time.Duration) bool {
	s.T().Logf("Waiting for translation completion: expecting %d routes", expectedRoutes)

	timeoutCh := time.After(timeout)
	ticker := time.NewTicker(translationPollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCh:
			s.T().Log("Timeout waiting for translation completion")
			return false
		case <-ticker.C:
			if s.checkAllGatewaysReady(gateways, expectedRoutes) {
				s.T().Log("Translation completed: all baseline routes are attached")
				return true
			}
		}
	}
}

func (s *AttachedRoutesSuite) checkAllGatewaysReady(gateways []string, expectedRoutes int) bool {
	for _, gatewayName := range gateways {
		namespacedName := types.NamespacedName{
			Name:      gatewayName,
			Namespace: s.loadTestManager.testNamespace,
		}

		gateway := &gwv1.Gateway{}
		if err := s.testInstallation.ClusterContext.Client.Get(s.ctx, namespacedName, gateway); err != nil {
			return false
		}

		if len(gateway.Status.Listeners) == 0 {
			return false
		}

		attachedRoutes := int(gateway.Status.Listeners[0].AttachedRoutes)
		s.T().Logf("Gateway %s: AttachedRoutes=%d (expecting %d)", gatewayName, attachedRoutes, expectedRoutes)

		if attachedRoutes < expectedRoutes {
			return false
		}
	}
	return true
}

func (s *AttachedRoutesSuite) verifyRouteValid(gatewayName string) error {
	s.T().Logf("Verifying routes are valid for gateway: %s", gatewayName)

	routeList := &gwv1.HTTPRouteList{}
	if err := s.testInstallation.ClusterContext.Client.List(s.ctx, routeList, &client.ListOptions{
		Namespace: s.loadTestManager.testNamespace,
	}); err != nil {
		return fmt.Errorf("failed to list routes: %w", err)
	}

	if len(routeList.Items) == 0 {
		return fmt.Errorf("no routes found")
	}

	route := &routeList.Items[0]
	if len(route.Status.Parents) == 0 {
		return fmt.Errorf("route has no parent status")
	}

	parentStatus := route.Status.Parents[0]
	for _, condition := range parentStatus.Conditions {
		if condition.Type == "Accepted" && condition.Status == "True" {
			s.T().Logf("Route %s is valid and accepted", route.Name)
			return nil
		}
	}

	return fmt.Errorf("no routes are accepted")
}

func (s *AttachedRoutesSuite) createSingleIncrementalRoute(gatewayName string) *gwv1.HTTPRoute {
	s.T().Log("Creating single incremental HTTPRoute")
	routeName := fmt.Sprintf("incremental-route-%d", time.Now().UnixNano())

	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: s.loadTestManager.testNamespace,
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{{Name: gwv1.ObjectName(gatewayName)}},
			},
			Hostnames: []gwv1.Hostname{gwv1.Hostname(fmt.Sprintf("%s.example.com", routeName))},
			Rules: []gwv1.HTTPRouteRule{{
				Matches: []gwv1.HTTPRouteMatch{{
					Path: &gwv1.HTTPPathMatch{
						Type:  &[]gwv1.PathMatchType{gwv1.PathMatchPathPrefix}[0],
						Value: &[]string{fmt.Sprintf("/%s", routeName)}[0],
					},
				}},
				BackendRefs: []gwv1.HTTPBackendRef{{
					BackendRef: gwv1.BackendRef{
						BackendObjectReference: gwv1.BackendObjectReference{
							Name: gwv1.ObjectName("simulated-service-1"),
							Port: &[]gwv1.PortNumber{80}[0],
						},
					},
				}},
			}},
		},
	}

	err := s.testInstallation.ClusterContext.Client.Create(s.ctx, route)
	s.Require().NoError(err, "Should create incremental route")

	s.T().Logf("Created incremental route: %s", routeName)
	return route
}

func (s *AttachedRoutesSuite) curlGatewayUntilReady(gatewayName string, baselineRoutes int) time.Duration {
	s.T().Log("Probing gateway until incremental route returns 200")
	return s.waitForGatewayCondition(gatewayName, baselineRoutes+1, "ready")
}

func (s *AttachedRoutesSuite) waitForRouteTeardown(gateways []string, expectedCount int, teardownStart time.Time) time.Duration {
	s.T().Log("Waiting for incremental route teardown")
	return s.waitForAllGatewaysCondition(gateways, expectedCount, teardownStart, "teardown")
}

func (s *AttachedRoutesSuite) waitForGatewayCondition(gatewayName string, expectedCount int, conditionType string) time.Duration {
	probeStart := time.Now()
	timeout := time.After(routeConditionTimeout)
	ticker := time.NewTicker(gatewayPollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			s.T().Logf("Timeout waiting for gateway %s condition", conditionType)
			return time.Since(probeStart)
		case <-ticker.C:
			namespacedName := types.NamespacedName{
				Name:      gatewayName,
				Namespace: s.loadTestManager.testNamespace,
			}

			gateway := &gwv1.Gateway{}
			if err := s.testInstallation.ClusterContext.Client.Get(s.ctx, namespacedName, gateway); err != nil {
				continue
			}

			if len(gateway.Status.Listeners) == 0 {
				continue
			}

			attachedRoutes := int(gateway.Status.Listeners[0].AttachedRoutes)
			s.T().Logf("Probing: AttachedRoutes=%d (expecting %d)", attachedRoutes, expectedCount)

			if attachedRoutes >= expectedCount {
				readyTime := time.Since(probeStart)
				s.T().Logf("Gateway condition %s met! Time: %v", conditionType, readyTime)
				return readyTime
			}
		}
	}
}

func (s *AttachedRoutesSuite) waitForAllGatewaysCondition(gateways []string, expectedCount int, startTime time.Time, conditionType string) time.Duration {
	timeout := time.After(routeConditionTimeout)
	ticker := time.NewTicker(teardownPollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			s.T().Logf("Timeout waiting for %s condition", conditionType)
			return time.Since(startTime)
		case <-ticker.C:
			allReady := true

			for _, gatewayName := range gateways {
				namespacedName := types.NamespacedName{
					Name:      gatewayName,
					Namespace: s.loadTestManager.testNamespace,
				}

				gateway := &gwv1.Gateway{}
				if err := s.testInstallation.ClusterContext.Client.Get(s.ctx, namespacedName, gateway); err != nil {
					allReady = false
					break
				}

				if len(gateway.Status.Listeners) == 0 {
					allReady = false
					break
				}

				attachedRoutes := int(gateway.Status.Listeners[0].AttachedRoutes)
				if attachedRoutes > expectedCount {
					allReady = false
					break
				}
			}

			if allReady {
				conditionTime := time.Since(startTime)
				s.T().Logf("%s condition complete: %v", conditionType, conditionTime)
				return conditionTime
			}
		}
	}
}

func (s *AttachedRoutesSuite) deleteIncrementalRoute(route *gwv1.HTTPRoute) error {
	s.T().Logf("Deleting incremental route: %s", route.Name)

	deleteOptions := &client.DeleteOptions{
		GracePeriodSeconds: func() *int64 { i := int64(0); return &i }(),
		PropagationPolicy:  func() *metav1.DeletionPropagation { p := metav1.DeletePropagationBackground; return &p }(),
	}

	if err := s.testInstallation.ClusterContext.Client.Delete(s.ctx, route, deleteOptions); err != nil && client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete incremental route: %w", err)
	}

	s.T().Logf("Successfully deleted incremental route")
	return nil
}

func (s *AttachedRoutesSuite) validateIncrementalPerformanceThresholds(results *TestResults) {
	s.Require().Greater(len(results.Watchers), 0, "At least one watcher should have recorded data")

	// Scale-aware performance thresholds based on the number of existing routes
	baselineRoutes := results.RouteCount - 1 // Subtract the incremental route
	var maxUserTime time.Duration
	var maxTeardownTime time.Duration

	switch {
	case baselineRoutes <= 1000:
		// Baseline test
		maxUserTime = baselineMaxUserTime
		maxTeardownTime = baselineMaxTeardownTime
	case baselineRoutes <= 5000:
		// Production scale test: allow more time due to complexity
		maxUserTime = productionMaxUserTime
		maxTeardownTime = productionMaxTeardownTime
	default:
		maxUserTime = largeScaleMaxUserTime
		maxTeardownTime = largeScaleMaxTeardownTime
	}

	s.T().Logf("Performance validation: %d baseline routes, maxUserTime=%v, maxTeardownTime=%v",
		baselineRoutes, maxUserTime, maxTeardownTime)

	s.Require().Less(results.SetupTime, maxUserTime,
		"Incremental route addition time should be reasonable for %d baseline routes (expected <%v)",
		baselineRoutes, maxUserTime)

	s.Require().Less(results.TeardownTime, maxTeardownTime,
		"Incremental route teardown should be fast for %d baseline routes (expected <%v)",
		baselineRoutes, maxTeardownTime)
}

func (s *AttachedRoutesSuite) reportIncrementalResults(results *TestResults) {
	s.T().Log("=== KEY METRICS (Following Recommended Methodology) ===")
	s.T().Logf("Setup Time (Stopwatch): %v", results.SetupTime)
	s.T().Logf("Route Ready Time: %v", results.RouteReadyTime)
	s.T().Logf("Teardown Time: %v", results.TeardownTime)
	s.T().Logf("Total Writes: %d", results.TotalWrites)

	s.T().Log("=== DETAILED RESULTS ===")
	s.T().Logf("Test Type: %s", results.TestType)
	s.T().Logf("Total Duration: %v", results.Duration)
	s.T().Logf("Routes Tested: %d", results.RouteCount)
	s.T().Logf("Gateway Count: %d", results.GatewayCount)

	if results.SimulatedCluster.BackendSimulated {
		sim := results.SimulatedCluster
		s.T().Log("=== CLUSTER SIMULATION METRICS ===")
		s.T().Logf("Simulated Nodes: %d", sim.Nodes)
		s.T().Logf("Simulated Services: %d", sim.FakeServices)
		s.T().Logf("Simulated Endpoints: %d", sim.FakePods)
		s.T().Logf("Memory Usage: %.2f MB", float64(sim.MemoryUsage)/1024/1024)
		s.T().Logf("CPU Usage: %.2f%%", sim.CPUUsage*100)
		s.T().Logf("API Calls/sec: %.2f", sim.APICallsPerSecond)
	}

	s.T().Log("=== INCREMENTAL ROUTE TEST COMPLETE ===")
}
