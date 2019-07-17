package consul

import (
	"context"
	"sort"
	"sync/atomic"

	"github.com/golang/mock/gomock"
	consulapi "github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	consulplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	"github.com/solo-io/go-utils/errors"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"golang.org/x/sync/errgroup"
)

var _ = Describe("Consul EDS", func() {

	Describe("endpoints watch", func() {

		var (
			ctx               context.Context
			cancel            context.CancelFunc
			ctrl              *gomock.Controller
			consulWatcherMock *consul.MockConsulWatcher

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
			yes       = EndpointTagMatchTrue
			no        = EndpointTagMatchFalse

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
				createUpstream(svc1, []string{primary, secondary, canary}, []string{dc1, dc2, dc3}),
				createUpstream(svc2, []string{primary, secondary}, []string{dc1, dc2}),
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

			consulWatcherMock = consul.NewMockConsulWatcher(ctrl)
			consulWatcherMock.EXPECT().DataCenters().Return(dataCenters, nil).Times(1)
			consulWatcherMock.EXPECT().WatchServices(gomock.Any(), dataCenters).Return(serviceMetaProducer, errorProducer).Times(1)

			// The Service function gets always invoked with the same parameters for same service. This makes it
			// impossible to mock in an idiomatic way. Just use a single match on everything and use the DoAndReturn
			// function to react based on the context.
			attempt := uint32(0)
			consulWatcherMock.EXPECT().Service(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(service, tag string, q *consulapi.QueryOptions) ([]*consulapi.CatalogService, *consulapi.QueryMeta, error) {
					currentAttempt := atomic.AddUint32(&attempt, 1)
					switch service {
					case svc1:
						switch q.Datacenter {
						case dc1:
							services := []*consulapi.CatalogService{
								createService("1.1.0.1", dc1, svc1, "a", []string{primary}, 1234, 100),
								createService("1.1.0.2", dc1, svc1, "b", []string{primary}, 1234, 100),
							}
							// Simulate the addition of a service instance. "> 5" because the first 5 attempts are a
							// result of the first snapshot (1 invocation for every service:dataCenter pair)
							if currentAttempt > 5 {
								services = append(services, createService("1.1.0.3", dc1, svc1, "b2", []string{primary, canary}, 1234, 100))
							}
							return services, nil, nil
						case dc2:
							return []*consulapi.CatalogService{
								createService("2.1.0.10", dc2, svc1, "c", []string{secondary}, 3456, 100),
								createService("2.1.0.11", dc2, svc1, "d", []string{secondary}, 4567, 100),
							}, nil, nil
						case dc3:
							return []*consulapi.CatalogService{
								createService("3.1.0.99", dc3, svc1, "e", []string{secondary, canary}, 9999, 100),
							}, nil, nil
						}
					case svc2:
						switch q.Datacenter {
						case dc1:
							return []*consulapi.CatalogService{
								createService("1.2.0.1", dc1, svc2, "a", []string{primary}, 8080, 100),
								createService("1.2.0.2", dc1, svc2, "b", []string{primary}, 8080, 100),
							}, nil, nil
						case dc2:
							return []*consulapi.CatalogService{
								createService("2.2.0.10", dc2, svc2, "c", []string{secondary}, 8088, 100),
								createService("2.2.0.11", dc2, svc2, "d", []string{secondary}, 8088, 100),
							}, nil, nil
						}
					}
					return nil, &consulapi.QueryMeta{}, errors.New("you screwed up the test")
				},
			).AnyTimes()

			expectedEndpointsFirstAttempt = v1.EndpointList{
				// 5 endpoints for service 1
				createExpectedEndpoint(svc1, "a", "1.1.0.1", "100", 1234, map[string]string{
					primary:            yes,
					secondary:          no,
					canary:             no,
					DataCenterLabelKey: dc1,
				}),
				createExpectedEndpoint(svc1, "b", "1.1.0.2", "100", 1234, map[string]string{
					primary:            yes,
					secondary:          no,
					canary:             no,
					DataCenterLabelKey: dc1,
				}),
				createExpectedEndpoint(svc1, "c", "2.1.0.10", "100", 3456, map[string]string{
					primary:            no,
					secondary:          yes,
					canary:             no,
					DataCenterLabelKey: dc2,
				}),
				createExpectedEndpoint(svc1, "d", "2.1.0.11", "100", 4567, map[string]string{
					primary:            no,
					secondary:          yes,
					canary:             no,
					DataCenterLabelKey: dc2,
				}),
				createExpectedEndpoint(svc1, "e", "3.1.0.99", "100", 9999, map[string]string{
					primary:            no,
					secondary:          yes,
					canary:             yes,
					DataCenterLabelKey: dc3,
				}),

				// 4 endpoints for service 2
				createExpectedEndpoint(svc2, "a", "1.2.0.1", "100", 8080, map[string]string{
					primary:            yes,
					secondary:          no,
					DataCenterLabelKey: dc1,
				}),
				createExpectedEndpoint(svc2, "b", "1.2.0.2", "100", 8080, map[string]string{
					primary:            yes,
					secondary:          no,
					DataCenterLabelKey: dc1,
				}),
				createExpectedEndpoint(svc2, "c", "2.2.0.10", "100", 8088, map[string]string{
					primary:            no,
					secondary:          yes,
					DataCenterLabelKey: dc2,
				}),
				createExpectedEndpoint(svc2, "d", "2.2.0.11", "100", 8088, map[string]string{
					primary:            no,
					secondary:          yes,
					DataCenterLabelKey: dc2,
				}),
			}

			// Sort using the same criteria as EDS, this makes it easier to compare actual to expected results
			sort.SliceStable(expectedEndpointsFirstAttempt, func(i, j int) bool {
				return expectedEndpointsFirstAttempt[i].Metadata.Name < expectedEndpointsFirstAttempt[j].Metadata.Name
			})

			expectedEndpointsSecondAttempt = append(
				expectedEndpointsFirstAttempt.Clone(),
				createExpectedEndpoint(svc1, "b2", "1.1.0.3", "100", 1234, map[string]string{
					primary:            yes,
					secondary:          no,
					canary:             yes,
					DataCenterLabelKey: dc1,
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
			eds := NewPlugin(consulWatcherMock)

			endpointsChan, errorChan, err := eds.WatchEndpoints("", upstreamsToTrack, clients.WatchOpts{Ctx: ctx})

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

			errorProducer <- errors.New("fail")
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
			upstream := createUpstream("my-svc", []string{"tag-1", "tag-2", "tag-3"}, []string{"dc-1", "dc-2"})

			endpoint := createEndpoint(consulService, v1.UpstreamList{upstream})

			Expect(endpoint).To(BeEquivalentTo(&v1.Endpoint{
				Metadata: core.Metadata{
					Namespace: "",
					Name:      "my-svc_my-svc-0",
					Labels: map[string]string{
						"tag-1":            EndpointTagMatchTrue,
						"tag-2":            EndpointTagMatchFalse,
						"tag-3":            EndpointTagMatchTrue,
						DataCenterLabelKey: "dc-1",
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

func createUpstream(svcName string, tags, dataCenters []string) *v1.Upstream {
	return &v1.Upstream{
		Metadata: core.Metadata{
			Name:      "consul-svc:" + svcName,
			Namespace: "",
		},
		UpstreamSpec: &v1.UpstreamSpec{
			UpstreamType: &v1.UpstreamSpec_Consul{
				Consul: &consulplugin.UpstreamSpec{
					ServiceName: svcName,
					ServiceTags: tags,
					DataCenters: dataCenters,
				},
			},
		},
	}
}

func createService(address, dc, name, id string, tags []string, port int, lastIndex uint64) *consulapi.CatalogService {
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

func createExpectedEndpoint(name, id, address, version string, port uint32, labels map[string]string) *v1.Endpoint {
	if id != "" {
		id = "_" + id
	}
	return &v1.Endpoint{
		Metadata: core.Metadata{
			Namespace:       "",
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
