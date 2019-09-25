package metricsservice

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes/typed/core/v1"
)

type Storage interface {
	ReceiveMetrics(ctx context.Context, envoyInstanceId string, newMetrics *EnvoyMetrics) error
	GetUsage(ctx context.Context) (*GlobalUsage, error)
}

type EnvoyMetrics struct {
	HttpRequests   uint64
	TcpConnections uint64
	Uptime         time.Duration
}

type EnvoyUsage struct {
	EnvoyMetrics    *EnvoyMetrics
	LastRecordedAt  time.Time
	FirstRecordedAt time.Time
	Active          bool // whether or not we believe this envoy to be active
}

type GlobalUsage struct {
	EnvoyIdToUsage map[string]*EnvoyUsage
}

type configMapStorage struct {
	configMapClient     k8s.ConfigMapInterface
	podNamespace        string
	currentTimeProvider CurrentTimeProvider

	// we may be receiving metrics from several envoys at the same time
	// be sure to lock appropriately to prevent data loss
	mutex sync.RWMutex
}

var _ Storage = &configMapStorage{}

const (
	metricsConfigMapName = "gloo-usage"
	usageDataKey         = "USAGE_DATA"

	// allow this much time between what we estimate for envoy's uptime and what it actually reports
	uptimeDiffThreshold = time.Second * 1

	// envoy should do a stats push every five seconds
	// if we go ten cycles without a stats push, then consider that envoy inactive
	envoyExpiryDuration = time.Second * 50
)

type CurrentTimeProvider func() time.Time

//go:generate mockgen -destination mocks/mock_config_map_client.go -package mocks k8s.io/client-go/kubernetes/typed/core/v1 ConfigMapInterface

func NewConfigMapStorage(podNamespace string, configMapClient k8s.ConfigMapInterface) Storage {
	return &configMapStorage{
		configMapClient:     configMapClient,
		podNamespace:        podNamespace,
		currentTimeProvider: time.Now,
		mutex:               sync.RWMutex{},
	}
}

// visible for testing
// provide a way to get the current time to make unit tests easier to write and more deterministic
func newConfigMapStorageWithTime(podNamespace string, configMapClient k8s.ConfigMapInterface, currentTimeProvider CurrentTimeProvider) Storage {
	return &configMapStorage{
		configMapClient:     configMapClient,
		podNamespace:        podNamespace,
		currentTimeProvider: currentTimeProvider,
		mutex:               sync.RWMutex{},
	}
}

// Record a new set of metrics for the given envoy instance id
// The envoy instance id template is set in the gateway proxy configmap: `envoy.yaml`.node.id
func (s *configMapStorage) ReceiveMetrics(ctx context.Context, envoyInstanceId string, newMetrics *EnvoyMetrics) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	existingUsage, configMap, err := s.getExistingUsage(ctx)
	if err != nil {
		return err
	}

	return s.writeUsage(ctx, existingUsage, envoyInstanceId, newMetrics, configMap)
}

func (s *configMapStorage) GetUsage(ctx context.Context) (*GlobalUsage, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	existingUsage, _, err := s.getExistingUsage(ctx)
	if err != nil {
		return nil, err
	}

	return existingUsage, nil
}

func (s *configMapStorage) writeUsage(ctx context.Context, existingGlobalUsage *GlobalUsage, envoyInstanceId string, newMetrics *EnvoyMetrics, configMap *corev1.ConfigMap) error {
	now := s.currentTimeProvider()
	var dataToWrite GlobalUsage

	if existingGlobalUsage == nil {
		dataToWrite = GlobalUsage{
			EnvoyIdToUsage: map[string]*EnvoyUsage{
				envoyInstanceId: {
					EnvoyMetrics:    newMetrics,
					LastRecordedAt:  now,
					FirstRecordedAt: now,
				},
			},
		}
	} else {
		// make sure the map is the same at first
		dataToWrite.EnvoyIdToUsage = existingGlobalUsage.EnvoyIdToUsage

		oldUsage, ok := existingGlobalUsage.EnvoyIdToUsage[envoyInstanceId]
		var mergedMetrics *EnvoyMetrics

		// if envoy has restarted since the first time we logged any of its metrics, it will be reporting numbers for
		// requests/connections that are unrelated to what we've already recorded, so we have to add it together with what we've already seen
		if ok && s.hasEnvoyRestartedSinceFirstLog(ctx, oldUsage, newMetrics) {
			mergedMetrics = &EnvoyMetrics{
				HttpRequests:   oldUsage.EnvoyMetrics.HttpRequests + newMetrics.HttpRequests,
				TcpConnections: oldUsage.EnvoyMetrics.TcpConnections + newMetrics.TcpConnections,
				Uptime:         newMetrics.Uptime, // reset the uptime to the newer uptime - to ensure that we keep merging the stats in this way
			}
		} else {
			// otherwise, we've seen a continuous stream of metrics, and the metrics being recorded now are
			// actually correct as they are- so just record them as-is
			mergedMetrics = newMetrics
		}

		firstRecordedTime := now
		if ok {
			firstRecordedTime = oldUsage.FirstRecordedAt
		}

		dataToWrite.EnvoyIdToUsage[envoyInstanceId] = &EnvoyUsage{
			EnvoyMetrics:    mergedMetrics,
			LastRecordedAt:  now,
			FirstRecordedAt: firstRecordedTime,
		}
	}

	// mark an envoy as inactive after a certain amount of time without a stats ping
	for _, v := range dataToWrite.EnvoyIdToUsage {
		v.Active = now.Sub(v.LastRecordedAt) <= envoyExpiryDuration
	}

	bytes, err := json.Marshal(dataToWrite)
	if err != nil {
		return err
	}
	configMap.Data = map[string]string{usageDataKey: string(bytes)}

	_, err = s.configMapClient.Update(configMap)
	return err
}

func (s *configMapStorage) hasEnvoyRestartedSinceFirstLog(ctx context.Context, oldUsage *EnvoyUsage, newMetrics *EnvoyMetrics) bool {
	// if envoy has not restarted, then its uptime should be roughly:
	// (the current time) minus (the time we first received metrics from envoy)
	expectedUptime := s.currentTimeProvider().Sub(oldUsage.FirstRecordedAt)
	actualUptime := newMetrics.Uptime

	uptimeDiff := expectedUptime - actualUptime

	// envoy has restarted if the difference between the expected uptime and the actual uptime
	// is positive - within a small epsilon to account for things like a slow startup
	return uptimeDiff >= uptimeDiffThreshold
}

// returns the old usage, the config map it came from, and any error
// the config map is nil if and only if an error occurs
// the old usage is nil if it has not been written yet or if there was an error reading it
func (s *configMapStorage) getExistingUsage(ctx context.Context) (*GlobalUsage, *corev1.ConfigMap, error) {
	cm, err := s.configMapClient.Get(metricsConfigMapName, v1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}

	usageJson, ok := cm.Data[usageDataKey]

	if !ok || usageJson == "" {
		return nil, cm, nil
	}

	usage := &GlobalUsage{}

	err = json.Unmarshal([]byte(usageJson), &usage)
	if err != nil {
		return nil, nil, err
	}

	return usage, cm, nil
}
