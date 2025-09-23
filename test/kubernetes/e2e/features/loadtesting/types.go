package loadtesting

import (
	"context"
	"time"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
)

type LoadTestingSuite struct {
	suite.Suite
	ctx              context.Context
	testInstallation *e2e.TestInstallation
}

type AttachedRoutesConfig struct {
	Gateways    []string      `yaml:"gateways"`
	Routes      int           `yaml:"routes"`
	GracePeriod time.Duration `yaml:"gracePeriod"`
	SingleNode  bool          `yaml:"singleNode"`
	BatchSize   int           `yaml:"batchSize"`
}

type VClusterConfig struct {
	SimulationName  string `yaml:"simulationName"`
	Namespace       string `yaml:"namespace"`
	FakeNodeCount   int    `yaml:"fakeNodeCount"`
	ServicesPerNode int    `yaml:"servicesPerNode"`
	PodsPerService  int    `yaml:"podsPerService"`
}

type Sample struct {
	Time           time.Time `json:"time"`
	AttachedRoutes int       `json:"attachedRoutes"`
}

type Watcher struct {
	Name    types.NamespacedName `json:"name"`
	Last    int                  `json:"last"`
	Samples []Sample             `json:"samples"`
}

type TestResults struct {
	TestType     string                            `json:"testType"`
	StartTime    time.Time                         `json:"startTime"`
	EndTime      time.Time                         `json:"endTime"`
	Duration     time.Duration                     `json:"duration"`
	RouteCount   int                               `json:"routeCount"`
	GatewayCount int                               `json:"gatewayCount"`
	Watchers     map[types.NamespacedName]*Watcher `json:"watchers"`

	SetupTime        time.Duration   `json:"setupTime"`
	TeardownTime     time.Duration   `json:"teardownTime"`
	RouteReadyTime   time.Duration   `json:"routeReadyTime"`
	TotalWrites      int             `json:"totalWrites"`
	KGatewayMetrics  KGatewayMetrics `json:"kgatewayMetrics"`
	SimulatedCluster VClusterMetrics `json:"simulatedCluster"`
}

// BaseMetrics contains common metrics fields shared across different metric types
type BaseMetrics struct {
	CPUUsage    float64 `json:"cpuUsage"`
	MemoryUsage int64   `json:"memoryUsage"`
}

type KGatewayMetrics struct {
	BaseMetrics
	CollectionTransformsTotal   int64         `json:"collectionTransformsTotal"`
	CollectionTransformDuration time.Duration `json:"collectionTransformDuration"`
	CollectionResources         int64         `json:"collectionResources"`
	StatusSyncerResources       int64         `json:"statusSyncerResources"`
}

type VClusterMetrics struct {
	BaseMetrics
	Nodes             int     `json:"nodes"`
	FakeNamespaces    int     `json:"fakeNamespaces"`
	FakeServices      int     `json:"fakeServices"`
	FakePods          int     `json:"fakePods"`
	BackendSimulated  bool    `json:"backendSimulated"`
	APICallsPerSecond float64 `json:"apiCallsPerSecond"`
}

// ThresholdConfig consolidates all threshold and configuration values for a test scenario
type ThresholdConfig struct {
	SetupThreshold    time.Duration
	TeardownThreshold time.Duration
	BatchSize         int
	GracePeriod       time.Duration
}

const (
	LoadTestNamespace = "kgateway-loadtest"
	BaselineRoutes    = 1000
	ProductionRoutes  = 5000
)

var (
	// BaselineConfig contains configuration values for baseline/smaller tests
	BaselineConfig = ThresholdConfig{
		SetupThreshold:    30 * time.Second,
		TeardownThreshold: 30 * time.Second,
		BatchSize:         100,
		GracePeriod:       100 * time.Millisecond,
	}

	// ProductionConfig contains configuration values for production/larger tests
	ProductionConfig = ThresholdConfig{
		SetupThreshold:    120 * time.Second,
		TeardownThreshold: 120 * time.Second,
		BatchSize:         500,
		GracePeriod:       100 * time.Millisecond,
	}
)

// getOptimalValue is a generic helper that returns production or baseline values based on route count
func getOptimalValue[T any](routeCount int, productionValue, baselineValue T) T {
	if routeCount >= ProductionRoutes {
		return productionValue
	}
	return baselineValue
}

// GetConfig returns the appropriate configuration based on route count
func GetConfig(routeCount int) ThresholdConfig {
	return getOptimalValue(routeCount, ProductionConfig, BaselineConfig)
}

// GetOptimalBatchSize returns the optimal batch size based on route count
func GetOptimalBatchSize(routeCount int) int {
	return GetConfig(routeCount).BatchSize
}

// GetOptimalSetupThreshold returns the optimal setup threshold based on route count
func GetOptimalSetupThreshold(routeCount int) time.Duration {
	return GetConfig(routeCount).SetupThreshold
}

// GetOptimalTeardownThreshold returns the optimal teardown threshold based on route count
func GetOptimalTeardownThreshold(routeCount int) time.Duration {
	return GetConfig(routeCount).TeardownThreshold
}
