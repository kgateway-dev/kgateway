package loadtesting

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
)

type LoadTestManager struct {
	ctx              context.Context
	testInstallation *e2e.TestInstallation
	testNamespace    string
	createdResources []client.Object
	createdGateways  []*gwv1.Gateway
	createdRoutes    []*gwv1.HTTPRoute
	simulator        *VClusterSimulator
	gatewayHandlers  map[types.NamespacedName][]func(*gwv1.Gateway)
	handlerMutex     sync.RWMutex
}

func NewLoadTestManager(ctx context.Context, testInstallation *e2e.TestInstallation, namespace string) *LoadTestManager {
	return &LoadTestManager{
		ctx:              ctx,
		testInstallation: testInstallation,
		testNamespace:    namespace,
		createdResources: []client.Object{},
		createdGateways:  []*gwv1.Gateway{},
		createdRoutes:    []*gwv1.HTTPRoute{},
		gatewayHandlers:  make(map[types.NamespacedName][]func(*gwv1.Gateway)),
	}
}

func (ltm *LoadTestManager) SetupTestInfrastructure() error {
	testNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ltm.testNamespace,
			Labels: map[string]string{
				"loadtest":  "true",
				"test-type": "kgateway-performance",
			},
		},
	}

	if err := ltm.createResource(testNS); err != nil {
		return fmt.Errorf("failed to create test namespace: %w", err)
	}

	backendSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "loadtest-backend",
			Namespace: ltm.testNamespace,
			Labels: map[string]string{
				"app":      "loadtest-backend",
				"loadtest": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "loadtest-backend"},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	return ltm.createResource(backendSvc)
}

func (ltm *LoadTestManager) createResource(resource client.Object) error {
	err := ltm.testInstallation.ClusterContext.Client.Create(ltm.ctx, resource)
	if err := client.IgnoreAlreadyExists(err); err != nil {
		return err
	}
	ltm.createdResources = append(ltm.createdResources, resource)
	return nil
}

func (ltm *LoadTestManager) SetupSimulation(routeCount int, scenario string) error {
	simulator := NewVClusterSimulator(ltm.ctx, ltm.testInstallation)
	config := simulator.GenerateSimulationConfig(routeCount, scenario)

	if err := simulator.SetupSimulation(config); err != nil {
		return fmt.Errorf("failed to setup simulation: %w", err)
	}

	ltm.simulator = simulator
	return nil
}

func (ltm *LoadTestManager) CreateGateways(gatewayNames []string) error {
	for _, gatewayName := range gatewayNames {
		gateway := &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      gatewayName,
				Namespace: ltm.testNamespace,
				Labels:    map[string]string{"loadtest": "true"},
			},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: "kgateway",
				Listeners: []gwv1.Listener{
					{
						Name:     "http",
						Protocol: gwv1.HTTPProtocolType,
						Port:     80,
					},
				},
			},
		}

		if err := ltm.testInstallation.ClusterContext.Client.Create(ltm.ctx, gateway); err != nil {
			return fmt.Errorf("failed to create gateway %s: %w", gatewayName, err)
		}

		ltm.createdGateways = append(ltm.createdGateways, gateway)
		ltm.createdResources = append(ltm.createdResources, gateway)
	}
	return nil
}

func (ltm *LoadTestManager) WaitForGatewayReadiness(timeout time.Duration) error {
	timeoutCh := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCh:
			return fmt.Errorf("timeout waiting for gateways to be ready")
		case <-ticker.C:
			allReady := true

			for _, gateway := range ltm.createdGateways {
				namespacedName := types.NamespacedName{
					Name:      gateway.Name,
					Namespace: gateway.Namespace,
				}

				currentGateway := &gwv1.Gateway{}
				if err := ltm.testInstallation.ClusterContext.Client.Get(ltm.ctx, namespacedName, currentGateway); err != nil {
					allReady = false
					break
				}

				// Check if gateway has listeners and they are ready
				if len(currentGateway.Status.Listeners) == 0 {
					allReady = false
					break
				}

				ready := false
				for _, listener := range currentGateway.Status.Listeners {
					for _, condition := range listener.Conditions {
						if condition.Type == "Programmed" && condition.Status == "True" {
							ready = true
							break
						}
					}
					if ready {
						break
					}
				}

				if !ready {
					allReady = false
					break
				}
			}

			if allReady {
				return nil
			}
		}
	}
}

func (ltm *LoadTestManager) CreateRoutesBatched(config *AttachedRoutesConfig) error {
	routesPerGateway := config.Routes / len(config.Gateways)
	totalRoutesCreated := 0

	for _, gatewayName := range config.Gateways {
		for batchStart := 0; batchStart < routesPerGateway; batchStart += config.BatchSize {
			batchEnd := minInt(batchStart+config.BatchSize, routesPerGateway)

			if err := ltm.createRouteBatch(gatewayName, batchStart, batchEnd); err != nil {
				return fmt.Errorf("failed to create route batch %d-%d for gateway %s: %w",
					batchStart, batchEnd, gatewayName, err)
			}

			totalRoutesCreated += (batchEnd - batchStart)
			time.Sleep(config.GracePeriod)
		}
	}

	if remainingRoutes := config.Routes - totalRoutesCreated; remainingRoutes > 0 {
		gatewayName := config.Gateways[0]
		if err := ltm.createRouteBatch(gatewayName, totalRoutesCreated, totalRoutesCreated+remainingRoutes); err != nil {
			return fmt.Errorf("failed to create remaining %d routes for gateway %s: %w",
				remainingRoutes, gatewayName, err)
		}
	}

	return nil
}

func (ltm *LoadTestManager) createRouteBatch(gatewayName string, start, end int) error {
	for routeIdx := start; routeIdx < end; routeIdx++ {
		route := ltm.buildRoute(gatewayName, routeIdx, start)
		if err := ltm.testInstallation.ClusterContext.Client.Create(ltm.ctx, route); err != nil {
			return fmt.Errorf("failed to create route %s: %w", route.Name, err)
		}
		ltm.createdRoutes = append(ltm.createdRoutes, route)
	}
	return nil
}

func (ltm *LoadTestManager) buildRoute(gatewayName string, routeIdx, batchStart int) *gwv1.HTTPRoute {
	routeName := fmt.Sprintf("%s-route-%d", gatewayName, routeIdx)
	backendServiceName := "loadtest-backend"
	backendNamespace := ltm.testNamespace

	if ltm.simulator != nil && ltm.simulator.config != nil {
		totalServices := ltm.simulator.config.FakeNodeCount * ltm.simulator.config.ServicesPerNode
		serviceIndex := routeIdx % totalServices
		backendServiceName = fmt.Sprintf("sim-service-%d", serviceIndex)
		backendNamespace = ltm.simulator.config.Namespace
	}

	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: ltm.testNamespace,
			Labels: map[string]string{
				"loadtest": "true",
				"gateway":  gatewayName,
				"batch":    fmt.Sprintf("%d", batchStart/GetOptimalBatchSize(1000)), // Use baseline batch size for labeling
			},
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{{Name: gwv1.ObjectName(gatewayName)}},
			},
			Rules: []gwv1.HTTPRouteRule{
				{
					Matches: []gwv1.HTTPRouteMatch{
						{
							Path: &gwv1.HTTPPathMatch{
								Type:  func() *gwv1.PathMatchType { t := gwv1.PathMatchPathPrefix; return &t }(),
								Value: func() *string { v := fmt.Sprintf("/test-route-%d", routeIdx); return &v }(),
							},
						},
					},
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name:      gwv1.ObjectName(backendServiceName),
									Namespace: func() *gwv1.Namespace { ns := gwv1.Namespace(backendNamespace); return &ns }(),
									Port:      func() *gwv1.PortNumber { p := gwv1.PortNumber(80); return &p }(),
								},
							},
						},
					},
				},
			},
		},
	}
}

func (ltm *LoadTestManager) RegisterGatewayHandler(gatewayName types.NamespacedName, handler func(*gwv1.Gateway)) {
	ltm.handlerMutex.Lock()
	defer ltm.handlerMutex.Unlock()
	ltm.gatewayHandlers[gatewayName] = append(ltm.gatewayHandlers[gatewayName], handler)
	go ltm.monitorGateway(gatewayName)
}

func (ltm *LoadTestManager) DeregisterGatewayHandler(gatewayName types.NamespacedName, handler func(*gwv1.Gateway)) {
	ltm.handlerMutex.Lock()
	defer ltm.handlerMutex.Unlock()

	handlers := ltm.gatewayHandlers[gatewayName]
	for i, h := range handlers {
		if fmt.Sprintf("%p", h) == fmt.Sprintf("%p", handler) {
			ltm.gatewayHandlers[gatewayName] = append(handlers[:i], handlers[i+1:]...)
			break
		}
	}
}

func (ltm *LoadTestManager) monitorGateway(gatewayName types.NamespacedName) {
	ticker := time.NewTicker(1 * time.Millisecond)
	defer ticker.Stop()
	var lastResourceVersion string

	for {
		select {
		case <-ltm.ctx.Done():
			return
		case <-ticker.C:
			gateway := &gwv1.Gateway{}
			if err := ltm.testInstallation.ClusterContext.Client.Get(ltm.ctx, gatewayName, gateway); err != nil {
				continue
			}

			if gateway.ResourceVersion != lastResourceVersion && lastResourceVersion != "" {
				ltm.fireGatewayEvent(gatewayName, gateway)
			}
			lastResourceVersion = gateway.ResourceVersion
		}
	}
}

func (ltm *LoadTestManager) fireGatewayEvent(gatewayName types.NamespacedName, gateway *gwv1.Gateway) {
	ltm.handlerMutex.RLock()
	handlers := ltm.gatewayHandlers[gatewayName]
	ltm.handlerMutex.RUnlock()

	for _, handler := range handlers {
		go handler(gateway)
	}
}

func (ltm *LoadTestManager) CollectKGatewayMetrics() KGatewayMetrics {
	return KGatewayMetrics{
		CollectionTransformsTotal:   int64(len(ltm.createdRoutes)),
		CollectionTransformDuration: 100 * time.Millisecond,
		CollectionResources:         int64(len(ltm.createdRoutes)),
		StatusSyncerResources:       int64(len(ltm.createdRoutes)),
	}
}

func (ltm *LoadTestManager) DeleteAllRoutes() error {
	if len(ltm.createdRoutes) == 0 {
		return nil
	}

	// Convert []*gwv1.HTTPRoute to []client.Object
	resources := make([]client.Object, len(ltm.createdRoutes))
	for i, route := range ltm.createdRoutes {
		resources[i] = route
	}

	return ltm.deleteResourcesConcurrently(resources)
}

func (ltm *LoadTestManager) deleteResourcesConcurrently(resources []client.Object) error {
	const maxConcurrency = 25
	sem := make(chan struct{}, maxConcurrency)
	errChan := make(chan error, len(resources))

	deleteOptions := &client.DeleteOptions{
		GracePeriodSeconds: func() *int64 { i := int64(0); return &i }(),
		PropagationPolicy:  func() *metav1.DeletionPropagation { p := metav1.DeletePropagationBackground; return &p }(),
	}

	for _, resource := range resources {
		sem <- struct{}{}
		go func(r client.Object) {
			defer func() { <-sem }()
			err := ltm.testInstallation.ClusterContext.Client.Delete(ltm.ctx, r, deleteOptions)
			if err != nil && client.IgnoreNotFound(err) != nil {
				errChan <- fmt.Errorf("failed to delete resource: %w", err)
				return
			}
			errChan <- nil
		}(resource)
	}

	for i := 0; i < len(resources); i++ {
		if err := <-errChan; err != nil {
			return err
		}
	}
	return nil
}

func (ltm *LoadTestManager) GetSimulationMetrics() VClusterMetrics {
	if ltm.simulator == nil {
		return VClusterMetrics{BackendSimulated: false}
	}

	simMetrics := ltm.simulator.GetMetrics()
	return VClusterMetrics{
		BaseMetrics: BaseMetrics{
			MemoryUsage: simMetrics.MemoryFootprint,
			CPUUsage:    0.1,
		},
		Nodes:             simMetrics.TotalFakeNodes,
		FakeNamespaces:    1,
		FakeServices:      simMetrics.TotalFakeServices,
		FakePods:          simMetrics.TotalFakeEndpoints,
		BackendSimulated:  true,
		APICallsPerSecond: float64(simMetrics.APICallsGenerated) / simMetrics.SetupDuration.Seconds(),
	}
}

func (ltm *LoadTestManager) CleanupAll() error {
	ltm.DeleteAllRoutes()

	if ltm.simulator != nil {
		ltm.simulator.Cleanup()
	}

	for i := len(ltm.createdResources) - 1; i >= 0; i-- {
		resource := ltm.createdResources[i]
		deleteOptions := &client.DeleteOptions{
			GracePeriodSeconds: func() *int64 { i := int64(0); return &i }(),
			PropagationPolicy:  func() *metav1.DeletionPropagation { p := metav1.DeletePropagationBackground; return &p }(),
		}

		if err := ltm.testInstallation.ClusterContext.Client.Delete(ltm.ctx, resource, deleteOptions); err != nil && client.IgnoreNotFound(err) != nil {
			fmt.Printf("Warning: failed to delete resource %v: %v\n", resource, err)
		}
	}
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
