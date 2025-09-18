package loadtesting

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
)

const (
	// MemoryFootprintPerService represents the estimated memory footprint per service in bytes
	MemoryFootprintPerService = 1024
)

type VClusterSimulator struct {
	ctx              context.Context
	testInstallation *e2e.TestInstallation
	config           *VClusterConfig
	createdResources []client.Object
	metrics          *SimulationMetrics
}

type SimulationMetrics struct {
	TotalFakeNodes     int           `json:"totalFakeNodes"`
	TotalFakeServices  int           `json:"totalFakeServices"`
	TotalFakeEndpoints int           `json:"totalFakeEndpoints"`
	SetupDuration      time.Duration `json:"setupDuration"`
	MemoryFootprint    int64         `json:"memoryFootprint"`
	APICallsGenerated  int64         `json:"apiCallsGenerated"`
}

func NewVClusterSimulator(ctx context.Context, testInstallation *e2e.TestInstallation) *VClusterSimulator {
	return &VClusterSimulator{
		ctx:              ctx,
		testInstallation: testInstallation,
		createdResources: []client.Object{},
		metrics:          &SimulationMetrics{},
	}
}

func (vcs *VClusterSimulator) GenerateSimulationConfig(routeCount int, scenario string) *VClusterConfig {
	var fakeNodeCount, servicesPerNode int

	if routeCount <= BaselineRoutes {
		fakeNodeCount = maxInt(2, routeCount/200)
		servicesPerNode = maxInt(20, routeCount/(fakeNodeCount*3))
	} else {
		fakeNodeCount = maxInt(3, routeCount/100)
		servicesPerNode = maxInt(15, routeCount/(fakeNodeCount*2))
	}

	timestamp := time.Now().UnixNano()
	return &VClusterConfig{
		SimulationName:  fmt.Sprintf("sim-%s-%d", scenario, timestamp),
		Namespace:       fmt.Sprintf("vcluster-sim-%s-%d", scenario, timestamp),
		FakeNodeCount:   fakeNodeCount,
		ServicesPerNode: servicesPerNode,
		PodsPerService:  2,
	}
}

func (vcs *VClusterSimulator) SetupSimulation(config *VClusterConfig) error {
	startTime := time.Now()
	vcs.config = config

	steps := []func() error{
		vcs.createSimulationNamespace,
		vcs.createFakeNodes,
		vcs.createFakeServicesWithEndpoints,
	}

	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}

	vcs.calculateMetrics(time.Since(startTime))
	return nil
}

func (vcs *VClusterSimulator) createSimulationNamespace() error {
	if err := vcs.waitForNamespaceTermination(); err != nil {
		return err
	}

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: vcs.config.Namespace,
			Labels: map[string]string{
				"vcluster-simulation": "true",
				"test-scenario":       vcs.config.SimulationName,
				"loadtest":            "true",
			},
		},
	}

	return vcs.createResource(namespace)
}

func (vcs *VClusterSimulator) waitForNamespaceTermination() error {
	existingNS := &corev1.Namespace{}
	err := vcs.testInstallation.ClusterContext.Client.Get(vcs.ctx, client.ObjectKey{Name: vcs.config.Namespace}, existingNS)

	if err != nil || existingNS.Status.Phase != corev1.NamespaceTerminating {
		return nil
	}

	fmt.Printf("Waiting for namespace %s to finish terminating...\n", vcs.config.Namespace)
	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for namespace %s to terminate", vcs.config.Namespace)
		case <-ticker.C:
			err := vcs.testInstallation.ClusterContext.Client.Get(vcs.ctx, client.ObjectKey{Name: vcs.config.Namespace}, existingNS)
			if client.IgnoreNotFound(err) == nil {
				return nil
			}
		}
	}
}

func (vcs *VClusterSimulator) createResource(resource client.Object) error {
	err := vcs.testInstallation.ClusterContext.Client.Create(vcs.ctx, resource)
	if err := client.IgnoreAlreadyExists(err); err != nil {
		return err
	}
	vcs.createdResources = append(vcs.createdResources, resource)
	return nil
}

func (vcs *VClusterSimulator) createFakeNodes() error {
	for i := 0; i < vcs.config.FakeNodeCount; i++ {
		node := vcs.buildFakeNode(i)
		if err := vcs.createResource(node); err != nil {
			return fmt.Errorf("failed to create fake node %s: %w", node.Name, err)
		}
	}
	return nil
}

func (vcs *VClusterSimulator) buildFakeNode(index int) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("sim-node-%s-%d", vcs.config.SimulationName, index),
			Labels: map[string]string{
				"vcluster-simulation":     "true",
				"simulation":              vcs.config.SimulationName,
				"node.kubernetes.io/role": "worker",
				"beta.kubernetes.io/arch": "amd64",
				"beta.kubernetes.io/os":   "linux",
			},
		},
		Spec: corev1.NodeSpec{Unschedulable: true},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			Addresses: []corev1.NodeAddress{{
				Type:    corev1.NodeInternalIP,
				Address: fmt.Sprintf("10.0.%d.%d", index/255, index%255),
			}},
			Capacity:    vcs.getNodeResources(),
			Allocatable: vcs.getNodeResources(),
		},
	}
}

func (vcs *VClusterSimulator) getNodeResources() corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewQuantity(4, resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(16*1024*1024*1024, resource.BinarySI),
		corev1.ResourcePods:   *resource.NewQuantity(110, resource.DecimalSI),
	}
}

func (vcs *VClusterSimulator) createFakeServicesWithEndpoints() error {
	serviceIndex := 0

	for nodeIndex := 0; nodeIndex < vcs.config.FakeNodeCount; nodeIndex++ {
		for serviceOnNode := 0; serviceOnNode < vcs.config.ServicesPerNode; serviceOnNode++ {
			if err := vcs.createServiceWithEndpoints(serviceIndex, nodeIndex); err != nil {
				return err
			}
			serviceIndex++
		}
	}
	return nil
}

func (vcs *VClusterSimulator) createServiceWithEndpoints(serviceIndex, nodeIndex int) error {
	serviceName := fmt.Sprintf("sim-service-%d", serviceIndex)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: vcs.config.Namespace,
			Labels: map[string]string{
				"vcluster-simulation": "true",
				"simulation":          vcs.config.SimulationName,
				"sim-node":            fmt.Sprintf("sim-node-%s-%d", vcs.config.SimulationName, nodeIndex),
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": serviceName},
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
				Protocol:   corev1.ProtocolTCP,
			}},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	if err := vcs.createResource(service); err != nil {
		return fmt.Errorf("failed to create service %s: %w", serviceName, err)
	}

	endpoints := vcs.buildFakeEndpoints(serviceName, serviceIndex)
	if err := vcs.createResource(endpoints); err != nil {
		return fmt.Errorf("failed to create endpoints %s: %w", serviceName, err)
	}

	return nil
}

func (vcs *VClusterSimulator) buildFakeEndpoints(serviceName string, serviceIndex int) *corev1.Endpoints {
	var addresses []corev1.EndpointAddress

	for podReplica := 0; podReplica < vcs.config.PodsPerService; podReplica++ {
		uniquePodID := (serviceIndex * vcs.config.PodsPerService) + podReplica
		subnet := (uniquePodID / 254) % 255
		host := (uniquePodID % 254) + 1

		addresses = append(addresses, corev1.EndpointAddress{
			IP: fmt.Sprintf("10.244.%d.%d", subnet, host),
			TargetRef: &corev1.ObjectReference{
				Kind:      "Pod",
				Name:      fmt.Sprintf("%s-pod-%d", serviceName, podReplica),
				Namespace: vcs.config.Namespace,
			},
		})
	}

	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: vcs.config.Namespace,
			Labels: map[string]string{
				"vcluster-simulation": "true",
				"simulation":          vcs.config.SimulationName,
			},
		},
		Subsets: []corev1.EndpointSubset{{
			Addresses: addresses,
			Ports: []corev1.EndpointPort{{
				Name:     "http",
				Port:     8080,
				Protocol: corev1.ProtocolTCP,
			}},
		}},
	}
}

func (vcs *VClusterSimulator) calculateMetrics(setupDuration time.Duration) {
	totalServices := vcs.config.FakeNodeCount * vcs.config.ServicesPerNode
	totalEndpoints := totalServices * vcs.config.PodsPerService

	vcs.metrics = &SimulationMetrics{
		TotalFakeNodes:     vcs.config.FakeNodeCount,
		TotalFakeServices:  totalServices,
		TotalFakeEndpoints: totalEndpoints,
		SetupDuration:      setupDuration,
		MemoryFootprint:    int64(totalServices * MemoryFootprintPerService),
		APICallsGenerated:  int64(vcs.config.FakeNodeCount + (totalServices * 2)),
	}
}

func (vcs *VClusterSimulator) GetMetrics() SimulationMetrics {
	if vcs.metrics == nil {
		return SimulationMetrics{}
	}
	return *vcs.metrics
}

func (vcs *VClusterSimulator) Cleanup() error {
	deleteOptions := &client.DeleteOptions{
		GracePeriodSeconds: func() *int64 { i := int64(0); return &i }(),
		PropagationPolicy:  func() *metav1.DeletionPropagation { p := metav1.DeletePropagationBackground; return &p }(),
	}

	for i := len(vcs.createdResources) - 1; i >= 0; i-- {
		resource := vcs.createdResources[i]
		if err := vcs.testInstallation.ClusterContext.Client.Delete(vcs.ctx, resource, deleteOptions); err != nil && client.IgnoreNotFound(err) != nil {
			fmt.Printf("Warning: failed to delete simulation resource %v: %v\n", resource, err)
		}
	}

	time.Sleep(2 * time.Second)
	return nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
