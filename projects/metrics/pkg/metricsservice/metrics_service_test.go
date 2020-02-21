package metricsservice_test

import (
	"context"
	"time"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	v2 "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v2"
	"github.com/gogo/protobuf/proto"
	_go "github.com/prometheus/client_model/go"
	"github.com/solo-io/gloo/projects/metrics/pkg/metricsservice"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/metrics/pkg/metricsservice/mocks"
)

var _ = Describe("Metrics service", func() {
	var (
		mockCtrl       *gomock.Controller
		metricsService *metricsservice.Server
		storage        *mocks.MockStorageClient
		timeProvider   metricsservice.CurrentTimeProvider
		metricsStream  *mocks.MockMetricsService_StreamMetricsServer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		timeProvider = func() time.Time {
			return time.Date(2019, 4, 20, 16, 20, 0, 0, time.UTC)
		}
		usageMerger := metricsservice.NewUsageMerger(timeProvider)
		storage = mocks.NewMockStorageClient(mockCtrl)
		metricsStream = mocks.NewMockMetricsService_StreamMetricsServer(mockCtrl)
		metricsHandler := metricsservice.NewDefaultMetricsHandler(storage, usageMerger)
		metricsService = metricsservice.NewServer(metricsservice.Options{Ctx: context.TODO()}, metricsHandler)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("can receive and store new metrics", func() {
		envoyInstanceId := "my-envoy-id"

		metrics := &v2.StreamMetricsMessage{
			Identifier: &v2.StreamMetricsMessage_Identifier{
				Node: &envoycore.Node{Id: envoyInstanceId},
			},
			EnvoyMetrics: []*_go.MetricFamily{
				{
					Name: proto.String("http.http.downstream_rq_total"),
					Metric: []*_go.Metric{
						{
							Counter: &_go.Counter{
								Value: proto.Float64(123),
							},
						},
					},
				},
				{
					Name: proto.String(metricsservice.ServerUptime),
					Metric: []*_go.Metric{
						{
							Gauge: &_go.Gauge{Value: proto.Float64(2)},
						},
					},
				},
			},
		}
		metricsStream.EXPECT().
			Recv().
			Return(metrics, nil)
		storage.EXPECT().
			GetUsage().
			Return(nil, nil)

		expectedUsage := &metricsservice.GlobalUsage{
			EnvoyIdToUsage: map[string]*metricsservice.EnvoyUsage{
				envoyInstanceId: {
					EnvoyMetrics: &metricsservice.EnvoyMetrics{
						HttpRequests:   123,
						TcpConnections: 0,
						Uptime:         time.Second * 2,
					},
					LastRecordedAt:  timeProvider(),
					FirstRecordedAt: timeProvider(),
					Active:          true,
				},
			},
		}

		storage.EXPECT().
			RecordUsage(expectedUsage).
			Return(nil)

		err := metricsService.StreamMetrics(metricsStream)
		Expect(err).NotTo(HaveOccurred())
	})

	It("can receive and update metrics", func() {
		envoyInstanceId := "my-envoy-id"

		metrics := &v2.StreamMetricsMessage{
			Identifier: &v2.StreamMetricsMessage_Identifier{
				Node: &envoycore.Node{Id: envoyInstanceId},
			},
			EnvoyMetrics: []*_go.MetricFamily{
				{
					Name: proto.String("http.http.downstream_rq_total"),
					Metric: []*_go.Metric{
						{
							Counter: &_go.Counter{Value: proto.Float64(123)},
						},
					},
				},
				{
					Name: proto.String(metricsservice.ServerUptime),
					Metric: []*_go.Metric{
						{
							Gauge: &_go.Gauge{Value: proto.Float64(10)},
						},
					},
				},
			},
		}
		metricsStream.EXPECT().
			Recv().
			Return(metrics, nil)

		existingUsage := &metricsservice.GlobalUsage{
			EnvoyIdToUsage: map[string]*metricsservice.EnvoyUsage{
				envoyInstanceId: {
					EnvoyMetrics: &metricsservice.EnvoyMetrics{
						HttpRequests:   100,
						TcpConnections: 0,
						Uptime:         time.Second * 5,
					},
					LastRecordedAt:  timeProvider().Add(time.Second * -5),
					FirstRecordedAt: timeProvider().Add(time.Second * -5),
					Active:          true,
				},
			},
		}
		storage.EXPECT().
			GetUsage().
			Return(existingUsage, nil)

		expectedUsage := &metricsservice.GlobalUsage{
			EnvoyIdToUsage: map[string]*metricsservice.EnvoyUsage{
				envoyInstanceId: {
					EnvoyMetrics: &metricsservice.EnvoyMetrics{
						HttpRequests:   123,
						TcpConnections: 0,
						Uptime:         time.Second * 10,
					},
					LastRecordedAt:  timeProvider(),
					FirstRecordedAt: timeProvider().Add(time.Second * -5),
					Active:          true,
				},
			},
		}

		storage.EXPECT().
			RecordUsage(expectedUsage).
			Return(nil)

		err := metricsService.StreamMetrics(metricsStream)
		Expect(err).NotTo(HaveOccurred())
	})
})
