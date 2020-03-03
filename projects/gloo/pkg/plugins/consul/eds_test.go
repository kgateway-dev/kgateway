package consul

import (
	"context"
	"net"
	"sort"
	"sync/atomic"
	"time"

	mock_consul2 "github.com/solo-io/gloo/projects/gloo/pkg/plugins/consul/mocks"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	mock_consul "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul/mocks"

	. "github.com/solo-io/gloo/projects/gloo/constants"

	"github.com/golang/mock/gomock"
	consulapi "github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	consulplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"golang.org/x/sync/errgroup"
)

var _ = Describe("Consul EDS", func() {

	var (
		ctrl *gomock.Controller
	)

	const writeNamespace = defaults.GlooSystem
	BeforeEach(func() {
		ctrl = gomock.NewController(T)
	})

	AfterEach(func() {
		ctrl.Finish()
	})
	Describe("endpoints watch 2 - more idiomatic", func() {

		var (
			ctx               context.Context
			cancel            context.CancelFunc
			consulWatcherMock *mock_consul.MockConsulWatcher

			// Data center names
			dc1         = "dc-1"
			dc2         = "dc-2"
			dc3         = "dc-3"
			dataCenters = []string{dc1, dc2, dc3}

			// Service names
			svc1 = "svc-1"

			// Tag names
			primary   = "primary"
			secondary = "secondary"
			canary    = "canary"
			yes       = ConsulEndpointMetadataMatchTrue
			no        = ConsulEndpointMetadataMatchFalse

			upstreamsToTrack      v1.UpstreamList
			consulServiceSnapshot []*consul.ServiceMeta
			serviceMetaProducer   chan []*consul.ServiceMeta
			errorProducer         chan error

			expectedEndpointsFirstAttempt  v1.EndpointList
			expectedEndpointsSecondAttempt v1.EndpointList
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			ctrl = gomock.NewController(T)

			serviceMetaProducer = make(chan []*consul.ServiceMeta)
			errorProducer = make(chan error)

			upstreamsToTrack = v1.UpstreamList{
				createTestUpstream(svc1, []string{primary, secondary, canary}, []string{dc1, dc2, dc3}),
			}

			consulServiceSnapshot = []*consul.ServiceMeta{
				{
					Name:        svc1,
					DataCenters: []string{dc1, dc2, dc3},
					Tags:        []string{primary, secondary, canary},
				},
			}

			consulWatcherMock = mock_consul.NewMockConsulWatcher(ctrl)
			consulWatcherMock.EXPECT().DataCenters().Return(dataCenters, nil).Times(1)
			consulWatcherMock.EXPECT().WatchServices(gomock.Any(), dataCenters).Return(serviceMetaProducer, errorProducer).Times(1)

			consulWatcherMock.EXPECT().Service(svc1, "", gomock.Any()).Return([]*consulapi.CatalogService{
				createTestService(buildHostname(svc1, dc2), dc2, svc1, "c", []string{secondary}, 3456, 100),
			}, nil, nil).Times(3) // once for each datacenter

			expectedEndpointsFirstAttempt = v1.EndpointList{
				createExpectedEndpoint(svc1, "c", "2.1.0.10", "100", writeNamespace, 3456, map[string]string{
					ConsulTagKeyPrefix + primary:    no,
					ConsulTagKeyPrefix + secondary:  yes,
					ConsulTagKeyPrefix + canary:     no,
					ConsulDataCenterKeyPrefix + dc1: no,
					ConsulDataCenterKeyPrefix + dc2: yes,
					ConsulDataCenterKeyPrefix + dc3: no,
				}),
			}

			expectedEndpointsSecondAttempt = v1.EndpointList{
				createExpectedEndpoint(svc1, "c", "2.1.0.11", "100", writeNamespace, 3456, map[string]string{
					ConsulTagKeyPrefix + primary:    no,
					ConsulTagKeyPrefix + secondary:  yes,
					ConsulTagKeyPrefix + canary:     no,
					ConsulDataCenterKeyPrefix + dc1: no,
					ConsulDataCenterKeyPrefix + dc2: yes,
					ConsulDataCenterKeyPrefix + dc3: no,
				}),
			}
		})

		AfterEach(func() {

			if cancel != nil {
				cancel()
			}

			close(serviceMetaProducer)
			close(errorProducer)
		})

		It("handles DNS updates even if consul services are unchanged", func() {

			// we have to put all the mock expects before the test starts or else the test may have data races
			initialIps := []net.IPAddr{{IP: net.IPv4(2, 1, 0, 10)}}
			mockDnsResolver := mock_consul2.NewMockDnsResolver(ctrl)
			mockDnsResolver.EXPECT().Resolve(gomock.Any()).Return(initialIps, nil).Times(1) // once for each consul service

			updatedIps := []net.IPAddr{{IP: net.IPv4(2, 1, 0, 11)}}
			// once for each consul service x 2 because we will let the test run through the EDS DNS poller twice
			// the first poll, DNS will have changed and we expect to receive new endpoints on the channel
			// the second poll, DNS will resolve to the same thing and we do not expect to receive new endpoints
			mockDnsResolver.EXPECT().Resolve(gomock.Any()).Return(updatedIps, nil).Times(2)

			eds := NewPlugin(consulWatcherMock, mockDnsResolver, nil)

			endpointsChan, errorChan, err := eds.WatchEndpoints(writeNamespace, upstreamsToTrack, clients.WatchOpts{Ctx: ctx})

			Expect(err).NotTo(HaveOccurred())

			// Monitors error channel until we cancel its context
			errRoutineCtx, errRoutineCancel := context.WithCancel(ctx)
			eg := errgroup.Group{}
			eg.Go(func() error {
				defer GinkgoRecover()
				for {
					select {
					default:
						Consistently(errorChan).ShouldNot(Receive())
					case <-errRoutineCtx.Done():
						return nil
					}
				}
			})

			// Simulate the initial read when starting watch
			serviceMetaProducer <- consulServiceSnapshot
			Eventually(endpointsChan).Should(Receive(BeEquivalentTo(expectedEndpointsFirstAttempt)))

			// Wait for error monitoring routine to stop, we want to simulate an error
			errRoutineCancel()
			_ = eg.Wait()

			errorProducer <- eris.New("fail")
			Eventually(errorChan).Should(Receive())

			// Simulate an update to DNS entries
			// by default we poll DNS every 5s for updates
			pollingInterval := DefaultDnsPollingInterval + time.Second
			Eventually(endpointsChan, pollingInterval, "1s").Should(Receive(BeEquivalentTo(expectedEndpointsSecondAttempt)))

			// ensure we don't receive anything else on channel even though we receive more DNS queries
			Consistently(endpointsChan, pollingInterval, "1s").ShouldNot(Receive())

			// Cancel and verify that all the channels have been closed
			cancel()
			Eventually(endpointsChan).Should(BeClosed())
			Eventually(errorChan).Should(BeClosed())
		})

	})

	Describe("endpoints watch - not idiomatic (do not copy)", func() {

		var (
			ctx               context.Context
			cancel            context.CancelFunc
			ctrl              *gomock.Controller
			consulWatcherMock *mock_consul.MockConsulWatcher

			// Data center names
			dc1         = "dc-1"
			dc2         = "dc-2"
			dc3         = "dc-3"
			dataCenters = []string{dc1, dc2, dc3}

			// Service names
			svc1 = "svc-1"
			svc2 = "svc-2"

			// Tag names
			primary   = "primary"
			secondary = "secondary"
			canary    = "canary"
			yes       = ConsulEndpointMetadataMatchTrue
			no        = ConsulEndpointMetadataMatchFalse

			upstreamsToTrack      v1.UpstreamList
			consulServiceSnapshot []*consul.ServiceMeta
			serviceMetaProducer   chan []*consul.ServiceMeta
			errorProducer         chan error

			expectedEndpointsFirstAttempt,
			expectedEndpointsSecondAttempt v1.EndpointList
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			ctrl = gomock.NewController(T)

			serviceMetaProducer = make(chan []*consul.ServiceMeta)
			errorProducer = make(chan error)

			upstreamsToTrack = v1.UpstreamList{
				createTestUpstream(svc1, []string{primary, secondary, canary}, []string{dc1, dc2, dc3}),
				createTestUpstream(svc2, []string{primary, secondary}, []string{dc1, dc2}),
			}

			consulServiceSnapshot = []*consul.ServiceMeta{
				{
					Name:        svc1,
					DataCenters: []string{dc1, dc2, dc3},
					Tags:        []string{primary, secondary, canary},
				},
				{
					Name:        svc2,
					DataCenters: []string{dc1, dc2},
					Tags:        []string{primary, secondary},
				},
			}

			consulWatcherMock = mock_consul.NewMockConsulWatcher(ctrl)
			consulWatcherMock.EXPECT().DataCenters().Return(dataCenters, nil).Times(1)
			consulWatcherMock.EXPECT().WatchServices(gomock.Any(), dataCenters).Return(serviceMetaProducer, errorProducer).Times(1)

			// The Service function gets always invoked with the same parameters for same service. This makes it
			// impossible to mock in an idiomatic way. Just use a single match on everything and use the DoAndReturn
			// function to react based on the context.

			// The above is not true, the service name and query params (with datacenter) are different, we can rewrite
			// this in a more idiomatic way in the future.
			attempt := uint32(0)
			consulWatcherMock.EXPECT().Service(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(service, tag string, q *consulapi.QueryOptions) ([]*consulapi.CatalogService, *consulapi.QueryMeta, error) {
					currentAttempt := atomic.AddUint32(&attempt, 1)
					switch service {
					case svc1:
						switch q.Datacenter {
						case dc1:
							services := []*consulapi.CatalogService{
								createTestService("1.1.0.1", dc1, svc1, "a", []string{primary}, 1234, 100),
								createTestService("1.1.0.2", dc1, svc1, "b", []string{primary}, 1234, 100),
							}
							// Simulate the addition of a service instance. "> 5" because the first 5 attempts are a
							// result of the first snapshot (1 invocation for every service:dataCenter pair)
							if currentAttempt > 5 {
								services = append(services, createTestService("1.1.0.3", dc1, svc1, "b1", []string{primary, canary}, 1234, 100))
							}
							return services, nil, nil
						case dc2:
							return []*consulapi.CatalogService{
								createTestService("2.1.0.10", dc2, svc1, "c", []string{secondary}, 3456, 100),
								createTestService("2.1.0.11", dc2, svc1, "d", []string{secondary}, 4567, 100),
							}, nil, nil
						case dc3:
							return []*consulapi.CatalogService{
								createTestService("3.1.0.99", dc3, svc1, "e", []string{secondary, canary}, 9999, 100),
							}, nil, nil
						}
					case svc2:
						switch q.Datacenter {
						case dc1:
							return []*consulapi.CatalogService{
								createTestService("1.2.0.1", dc1, svc2, "a2", []string{primary}, 8080, 100),
								createTestService("1.2.0.2", dc1, svc2, "b2", []string{primary}, 8080, 100),
							}, nil, nil
						case dc2:
							return []*consulapi.CatalogService{
								createTestService("2.2.0.10", dc2, svc2, "c2", []string{secondary}, 8088, 100),
								createTestService("2.2.0.11", dc2, svc2, "d2", []string{secondary}, 8088, 100),
							}, nil, nil
						}
					}
					return nil, &consulapi.QueryMeta{}, eris.New("you screwed up the test")
				},
			).AnyTimes()

			expectedEndpointsFirstAttempt = v1.EndpointList{
				// 5 endpoints for service 1
				createExpectedEndpoint(svc1, "a", "1.1.0.1", "100", writeNamespace, 1234, map[string]string{
					ConsulTagKeyPrefix + primary:    yes,
					ConsulTagKeyPrefix + secondary:  no,
					ConsulTagKeyPrefix + canary:     no,
					ConsulDataCenterKeyPrefix + dc1: yes,
					ConsulDataCenterKeyPrefix + dc2: no,
					ConsulDataCenterKeyPrefix + dc3: no,
				}),
				createExpectedEndpoint(svc1, "b", "1.1.0.2", "100", writeNamespace, 1234, map[string]string{
					ConsulTagKeyPrefix + primary:    yes,
					ConsulTagKeyPrefix + secondary:  no,
					ConsulTagKeyPrefix + canary:     no,
					ConsulDataCenterKeyPrefix + dc1: yes,
					ConsulDataCenterKeyPrefix + dc2: no,
					ConsulDataCenterKeyPrefix + dc3: no,
				}),
				createExpectedEndpoint(svc1, "c", "2.1.0.10", "100", writeNamespace, 3456, map[string]string{
					ConsulTagKeyPrefix + primary:    no,
					ConsulTagKeyPrefix + secondary:  yes,
					ConsulTagKeyPrefix + canary:     no,
					ConsulDataCenterKeyPrefix + dc1: no,
					ConsulDataCenterKeyPrefix + dc2: yes,
					ConsulDataCenterKeyPrefix + dc3: no,
				}),
				createExpectedEndpoint(svc1, "d", "2.1.0.11", "100", writeNamespace, 4567, map[string]string{
					ConsulTagKeyPrefix + primary:    no,
					ConsulTagKeyPrefix + secondary:  yes,
					ConsulTagKeyPrefix + canary:     no,
					ConsulDataCenterKeyPrefix + dc1: no,
					ConsulDataCenterKeyPrefix + dc2: yes,
					ConsulDataCenterKeyPrefix + dc3: no,
				}),
				createExpectedEndpoint(svc1, "e", "3.1.0.99", "100", writeNamespace, 9999, map[string]string{
					ConsulTagKeyPrefix + primary:    no,
					ConsulTagKeyPrefix + secondary:  yes,
					ConsulTagKeyPrefix + canary:     yes,
					ConsulDataCenterKeyPrefix + dc1: no,
					ConsulDataCenterKeyPrefix + dc2: no,
					ConsulDataCenterKeyPrefix + dc3: yes,
				}),

				// 4 endpoints for service 2
				createExpectedEndpoint(svc2, "a2", "1.2.0.1", "100", writeNamespace, 8080, map[string]string{
					ConsulTagKeyPrefix + primary:    yes,
					ConsulTagKeyPrefix + secondary:  no,
					ConsulDataCenterKeyPrefix + dc1: yes,
					ConsulDataCenterKeyPrefix + dc2: no,
				}),
				createExpectedEndpoint(svc2, "b2", "1.2.0.2", "100", writeNamespace, 8080, map[string]string{
					ConsulTagKeyPrefix + primary:    yes,
					ConsulTagKeyPrefix + secondary:  no,
					ConsulDataCenterKeyPrefix + dc1: yes,
					ConsulDataCenterKeyPrefix + dc2: no,
				}),
				createExpectedEndpoint(svc2, "c2", "2.2.0.10", "100", writeNamespace, 8088, map[string]string{
					ConsulTagKeyPrefix + primary:    no,
					ConsulTagKeyPrefix + secondary:  yes,
					ConsulDataCenterKeyPrefix + dc1: no,
					ConsulDataCenterKeyPrefix + dc2: yes,
				}),
				createExpectedEndpoint(svc2, "d2", "2.2.0.11", "100", writeNamespace, 8088, map[string]string{
					ConsulTagKeyPrefix + primary:    no,
					ConsulTagKeyPrefix + secondary:  yes,
					ConsulDataCenterKeyPrefix + dc1: no,
					ConsulDataCenterKeyPrefix + dc2: yes,
				}),
			}

			// Sort using the same criteria as EDS, this makes it easier to compare actual to expected results
			sort.SliceStable(expectedEndpointsFirstAttempt, func(i, j int) bool {
				return expectedEndpointsFirstAttempt[i].Metadata.Name < expectedEndpointsFirstAttempt[j].Metadata.Name
			})

			expectedEndpointsSecondAttempt = append(
				expectedEndpointsFirstAttempt.Clone(),
				createExpectedEndpoint(svc1, "b1", "1.1.0.3", "100", writeNamespace, 1234, map[string]string{
					ConsulTagKeyPrefix + primary:    yes,
					ConsulTagKeyPrefix + secondary:  no,
					ConsulTagKeyPrefix + canary:     yes,
					ConsulDataCenterKeyPrefix + dc1: yes,
					ConsulDataCenterKeyPrefix + dc2: no,
					ConsulDataCenterKeyPrefix + dc3: no,
				}),
			)
			sort.SliceStable(expectedEndpointsSecondAttempt, func(i, j int) bool {
				return expectedEndpointsSecondAttempt[i].Metadata.Name < expectedEndpointsSecondAttempt[j].Metadata.Name
			})
		})

		AfterEach(func() {
			ctrl.Finish()

			if cancel != nil {
				cancel()
			}

			close(serviceMetaProducer)
			close(errorProducer)
		})

		It("works as expected", func() {
			eds := NewPlugin(consulWatcherMock, nil, nil)

			endpointsChan, errorChan, err := eds.WatchEndpoints(writeNamespace, upstreamsToTrack, clients.WatchOpts{Ctx: ctx})

			Expect(err).NotTo(HaveOccurred())

			// Monitors error channel until we cancel its context
			errRoutineCtx, errRoutineCancel := context.WithCancel(ctx)
			eg := errgroup.Group{}
			eg.Go(func() error {
				defer GinkgoRecover()
				for {
					select {
					default:
						Consistently(errorChan).ShouldNot(Receive())
					case <-errRoutineCtx.Done():
						return nil
					}
				}
			})

			// Simulate the initial read when starting watch
			serviceMetaProducer <- consulServiceSnapshot
			Eventually(endpointsChan).Should(Receive(BeEquivalentTo(expectedEndpointsFirstAttempt)))

			// Wait for error monitoring routine to stop, we want to simulate an error
			errRoutineCancel()
			_ = eg.Wait()

			errorProducer <- eris.New("fail")
			Eventually(errorChan).Should(Receive())

			// Simulate an update to the services
			// We use the same metadata snapshot because what changed is the service spec
			serviceMetaProducer <- consulServiceSnapshot
			Eventually(endpointsChan).Should(Receive(BeEquivalentTo(expectedEndpointsSecondAttempt)))

			// Cancel and verify that all the channels have been closed
			cancel()
			Eventually(endpointsChan).Should(BeClosed())
			Eventually(errorChan).Should(BeClosed())
		})

	})

	Describe("unit tests", func() {
		FIt("generates unique endpoint names", func() {

			svcs := []*consulapi.CatalogService{
				{
					ID:         "12341234-1234-1234-1234-123412341234",
					Node:       "ip-1.2.3.4",
					Address:    "1.2.3.4",
					Datacenter: "test",
					TaggedAddresses: map[string]string{
						"lan": "1.2.3.4",
						"wan": "1.2.3.4",
					},
					// test with two services having the same services id. this can happen.
					ServiceID:      "foo",
					ServiceName:    "foo",
					ServiceAddress: "1.2.3.4",
					ServiceTags:    []string{"serf"},
					ServicePort:    1234,
				}, {
					ID:         "12341234-1234-1234-1234-123412341234",
					Node:       "ip-1.2.3.4",
					Address:    "1.2.3.4",
					Datacenter: "test",
					TaggedAddresses: map[string]string{
						"lan": "1.2.3.4",
						"wan": "1.2.3.4",
					},
					ServiceID:      "foo",
					ServiceName:    "foo",
					ServiceAddress: "1.2.3.4",
					ServiceTags:    []string{"http"},
					ServicePort:    1235,
				}, {
					ID:         "12341234-1234-1234-1234-123412341234",
					Node:       "ip-1.2.3.4",
					Address:    "test.com",
					Datacenter: "test-dns",
					TaggedAddresses: map[string]string{
						"lan": "1.2.3.4",
						"wan": "1.2.3.4",
					},
					ServiceID:      "foo",
					ServiceName:    "foo",
					ServiceAddress: "test.com",
					ServiceTags:    []string{"ftp"},
					ServicePort:    1236,
				},
			}

			twoIps := []net.IPAddr{{IP: net.IPv4(2, 1, 0, 10)}, {IP: net.IPv4(2, 1, 0, 11)}}
			mockDnsResolver := mock_consul2.NewMockDnsResolver(ctrl)
			mockDnsResolver.EXPECT().Resolve(gomock.Any()).Return(twoIps, nil).Times(1)

			trackedServiceToUpstreams := make(map[string][]*v1.Upstream)
			for _, svc := range svcs {
				trackedServiceToUpstreams[svc.ServiceName] = []*v1.Upstream{}
			}

			// make sure the we have a correct number of generated endpoints:

			endpoints := buildEndpointsFromSpecs(context.TODO(), writeNamespace, mockDnsResolver, svcs, trackedServiceToUpstreams)
			endpontNames := map[string]bool{}
			for _, endpoint := range endpoints {
				endpontNames[endpoint.GetMetadata().Name] = true
			}
			Expect(endpontNames).To(HaveLen(len(svcs) + (len(twoIps) - 1)))
		})
	})
	Describe("unit tests", func() {

		It("generates the correct endpoint for a given Consul service", func() {
			consulService := &consulapi.CatalogService{
				ServiceID:   "my-svc-0",
				ServiceName: "my-svc",
				Address:     "127.0.0.1",
				ServicePort: 1234,
				Datacenter:  "dc-1",
				ServiceTags: []string{"tag-1", "tag-3"},
				ModifyIndex: 9876,
			}
			upstream := createTestUpstream("my-svc", []string{"tag-1", "tag-2", "tag-3"}, []string{"dc-1", "dc-2"})

			endpoints, err := buildEndpoints(writeNamespace, nil, consulService, v1.UpstreamList{upstream})
			Expect(err).To(BeNil())

			Expect(endpoints).To(ConsistOf(&v1.Endpoint{
				Metadata: core.Metadata{
					Namespace: writeNamespace,
					Name:      "my-svc-my-svc-0",
					Labels: map[string]string{
						ConsulTagKeyPrefix + "tag-1":       ConsulEndpointMetadataMatchTrue,
						ConsulTagKeyPrefix + "tag-2":       ConsulEndpointMetadataMatchFalse,
						ConsulTagKeyPrefix + "tag-3":       ConsulEndpointMetadataMatchTrue,
						ConsulDataCenterKeyPrefix + "dc-1": ConsulEndpointMetadataMatchTrue,
						ConsulDataCenterKeyPrefix + "dc-2": ConsulEndpointMetadataMatchFalse,
					},
					ResourceVersion: "9876",
				},
				Upstreams: []*core.ResourceRef{utils.ResourceRefPtr(upstream.Metadata.Ref())},
				Address:   "127.0.0.1",
				Port:      1234,
			}))
		})
	})
})

func createTestUpstream(svcName string, tags, dataCenters []string) *v1.Upstream {
	return &v1.Upstream{
		Metadata: core.Metadata{
			Name:      "consul-svc:" + svcName,
			Namespace: "",
		},
		UpstreamType: &v1.Upstream_Consul{
			Consul: &consulplugin.UpstreamSpec{
				ServiceName: svcName,
				ServiceTags: tags,
				DataCenters: dataCenters,
			},
		},
	}
}

func createTestService(address, dc, name, id string, tags []string, port int, lastIndex uint64) *consulapi.CatalogService {
	return &consulapi.CatalogService{
		ServiceName: name,
		ServiceID:   id,
		Address:     address,
		Datacenter:  dc,
		ServiceTags: tags,
		ServicePort: port,
		ModifyIndex: lastIndex,
	}
}

func createExpectedEndpoint(name, id, address, version, ns string, port uint32, labels map[string]string) *v1.Endpoint {
	if id != "" {
		id = "-" + id
	}
	return &v1.Endpoint{
		Metadata: core.Metadata{
			Namespace:       ns,
			Name:            name + id,
			Labels:          labels,
			ResourceVersion: version,
		},
		Upstreams: []*core.ResourceRef{
			{
				Name:      "consul-svc:" + name,
				Namespace: "",
			},
		},
		Address: address,
		Port:    port,
	}
}

func buildHostname(svc, dc string) string {
	return svc + ".service." + dc + ".consul"
}
