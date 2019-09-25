package metricsservice

import (
	"context"
	"encoding/json"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/solo-io/gloo/projects/metrics/pkg/metricsservice/mocks"
	k8sv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metrics", func() {
	Describe("Config map storage", func() {
		var (
			mockCtrl               *gomock.Controller
			configMapClient        *mocks.MockConfigMapInterface
			podNamespace           = "test-namespace"
			envoyInstanceId        = "gateway-proxy-v2-84585498d7-lfw6g.gloo-system"
			tenSecondUptimeMetrics = &EnvoyMetrics{
				HttpRequests:   100,
				TcpConnections: 0,
				Uptime:         time.Second * 10,
			}
			twentySecondUptimeMetrics = &EnvoyMetrics{
				HttpRequests:   101,
				TcpConnections: 0,
				Uptime:         time.Second * 20,
			}
			threeSecondUptimeMetrics = &EnvoyMetrics{
				HttpRequests:   6,
				TcpConnections: 9,
				Uptime:         time.Second * 3,
			}
			currentTime             = time.Date(2019, 4, 20, 16, 20, 0, 0, time.UTC)
			testCurrentTimeProvider = func() time.Time {
				return currentTime
			}
		)

		BeforeEach(func() {
			mockCtrl = gomock.NewController(GinkgoT())
			configMapClient = mocks.NewMockConfigMapInterface(mockCtrl)
		})

		AfterEach(func() {
			mockCtrl.Finish()
		})

		It("can write new metrics to a config map", func() {
			metricsStorage := newConfigMapStorageWithTime(podNamespace, configMapClient, testCurrentTimeProvider)
			emptyConfigMap := &k8sv1.ConfigMap{
				Data: map[string]string{},
			}

			configMapClient.EXPECT().
				Get(metricsConfigMapName, v1.GetOptions{}).
				Return(emptyConfigMap, nil)

			dataToWrite := GlobalUsage{
				EnvoyIdToUsage: map[string]*EnvoyUsage{
					envoyInstanceId: {
						EnvoyMetrics:    tenSecondUptimeMetrics,
						LastRecordedAt:  currentTime,
						FirstRecordedAt: currentTime,
						Active:          true,
					},
				},
			}
			bytes, err := json.Marshal(dataToWrite)
			Expect(err).NotTo(HaveOccurred())

			configMapWithMetrics := &k8sv1.ConfigMap{
				Data: map[string]string{
					usageDataKey: string(bytes),
				},
			}
			configMapClient.EXPECT().Update(configMapWithMetrics).Return(nil, nil)

			err = metricsStorage.ReceiveMetrics(context.TODO(), envoyInstanceId, tenSecondUptimeMetrics)
			Expect(err).NotTo(HaveOccurred())
		})

		It("can update the config map", func() {
			metricsStorage := newConfigMapStorageWithTime(podNamespace, configMapClient, testCurrentTimeProvider)
			existingMetrics := tenSecondUptimeMetrics
			existingUsage := &GlobalUsage{
				EnvoyIdToUsage: map[string]*EnvoyUsage{
					envoyInstanceId: {
						EnvoyMetrics:   existingMetrics,
						LastRecordedAt: currentTime,

						// the time api is janky- this says that we first recorded this twenty seconds ago
						// this should indicate that envoy has NOT restarted, since the new metric we're about to record
						// has an uptime of 20 seconds on it
						FirstRecordedAt: currentTime.Add(time.Duration(-20) * time.Second),
						Active:          true,
					},
				},
			}
			bytes, err := json.Marshal(existingUsage)
			Expect(err).NotTo(HaveOccurred())

			existingConfigMap := &k8sv1.ConfigMap{
				Data: map[string]string{usageDataKey: string(bytes)},
			}

			configMapClient.EXPECT().
				Get(metricsConfigMapName, v1.GetOptions{}).
				Return(existingConfigMap, nil)

			newMetrics := twentySecondUptimeMetrics

			updatedUsage := &GlobalUsage{
				EnvoyIdToUsage: map[string]*EnvoyUsage{
					envoyInstanceId: {
						// the metrics should be just the new metrics
						EnvoyMetrics:   newMetrics,
						LastRecordedAt: currentTime,

						// the time api is janky- this says that we first recorded this twenty seconds ago
						FirstRecordedAt: currentTime.Add(time.Duration(-20) * time.Second),
						Active:          true,
					},
				},
			}
			bytes, err = json.Marshal(updatedUsage)
			Expect(err).NotTo(HaveOccurred())
			updatedConfigMap := &k8sv1.ConfigMap{
				Data: map[string]string{usageDataKey: string(bytes)},
			}
			configMapClient.EXPECT().Update(updatedConfigMap).Return(nil, nil)

			err = metricsStorage.ReceiveMetrics(context.TODO(), envoyInstanceId, newMetrics)
			Expect(err).NotTo(HaveOccurred())
		})

		It("marks envoys as inactive after a certain amount of time without a stats push", func() {
			metricsStorage := newConfigMapStorageWithTime(podNamespace, configMapClient, testCurrentTimeProvider)
			existingMetrics := twentySecondUptimeMetrics
			oldUsage := &EnvoyUsage{
				EnvoyMetrics:    existingMetrics,
				LastRecordedAt:  currentTime.Add(time.Duration(-2) * envoyExpiryDuration),
				FirstRecordedAt: currentTime.Add(time.Duration(-21) * envoyExpiryDuration),
				Active:          true,
			}
			existingUsage := &GlobalUsage{
				EnvoyIdToUsage: map[string]*EnvoyUsage{
					envoyInstanceId: oldUsage,
				},
			}
			bytes, err := json.Marshal(existingUsage)
			Expect(err).NotTo(HaveOccurred())

			existingConfigMap := &k8sv1.ConfigMap{
				Data: map[string]string{usageDataKey: string(bytes)},
			}

			configMapClient.EXPECT().
				Get(metricsConfigMapName, v1.GetOptions{}).
				Return(existingConfigMap, nil)

			newEnvoyId := envoyInstanceId + "-different-envoy-id"
			newUsage := &EnvoyUsage{
				EnvoyMetrics:    threeSecondUptimeMetrics,
				LastRecordedAt:  currentTime,
				FirstRecordedAt: currentTime,
				Active:          true,
			}

			// this one should be no longer active
			inactiveEnvoy := *oldUsage
			inactiveEnvoy.Active = false

			updatedUsage := &GlobalUsage{
				EnvoyIdToUsage: map[string]*EnvoyUsage{
					envoyInstanceId: &inactiveEnvoy,
					newEnvoyId:      newUsage,
				},
			}
			bytes, err = json.Marshal(updatedUsage)
			Expect(err).NotTo(HaveOccurred())
			updatedConfigMap := &k8sv1.ConfigMap{
				Data: map[string]string{usageDataKey: string(bytes)},
			}
			configMapClient.EXPECT().Update(updatedConfigMap).Return(nil, nil)

			err = metricsStorage.ReceiveMetrics(context.TODO(), newEnvoyId, threeSecondUptimeMetrics)
			Expect(err).NotTo(HaveOccurred())
		})

		It("can merge metrics after envoy restarts", func() {
			metricsStorage := newConfigMapStorageWithTime(podNamespace, configMapClient, testCurrentTimeProvider)
			existingMetrics := twentySecondUptimeMetrics
			existingUsage := &GlobalUsage{
				EnvoyIdToUsage: map[string]*EnvoyUsage{
					envoyInstanceId: {
						EnvoyMetrics:   existingMetrics,
						LastRecordedAt: currentTime,

						// the time api is janky- this says that we first recorded this twenty seconds ago
						// this should indicate that envoy has NOT restarted, since the new metric we're about to record
						// has an uptime of 20 seconds on it
						FirstRecordedAt: currentTime.Add(time.Duration(-20) * time.Second),
						Active:          true,
					},
				},
			}
			bytes, err := json.Marshal(existingUsage)
			Expect(err).NotTo(HaveOccurred())

			existingConfigMap := &k8sv1.ConfigMap{
				Data: map[string]string{usageDataKey: string(bytes)},
			}

			configMapClient.EXPECT().
				Get(metricsConfigMapName, v1.GetOptions{}).
				Return(existingConfigMap, nil)

			updatedUsage := &GlobalUsage{
				EnvoyIdToUsage: map[string]*EnvoyUsage{
					envoyInstanceId: {
						// the metrics should be the combination of the two metrics we've recorded
						EnvoyMetrics: &EnvoyMetrics{
							HttpRequests:   twentySecondUptimeMetrics.HttpRequests + threeSecondUptimeMetrics.HttpRequests,
							TcpConnections: twentySecondUptimeMetrics.TcpConnections + threeSecondUptimeMetrics.TcpConnections,
							Uptime:         threeSecondUptimeMetrics.Uptime,
						},
						LastRecordedAt:  currentTime,
						FirstRecordedAt: currentTime.Add(time.Duration(-20) * time.Second),
						Active:          true,
					},
				},
			}
			bytes, err = json.Marshal(updatedUsage)
			Expect(err).NotTo(HaveOccurred())
			updatedConfigMap := &k8sv1.ConfigMap{
				Data: map[string]string{usageDataKey: string(bytes)},
			}
			configMapClient.EXPECT().Update(updatedConfigMap).Return(nil, nil)

			err = metricsStorage.ReceiveMetrics(context.TODO(), envoyInstanceId, threeSecondUptimeMetrics)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
