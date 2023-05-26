package consul

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/solo-io/gloo/test/gomega/matchers"

	"github.com/golang/protobuf/proto"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	mock_consul2 "github.com/solo-io/gloo/projects/gloo/pkg/plugins/consul/mocks"
	proto_matchers "github.com/solo-io/solo-kit/test/matchers"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	mock_consul "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul/mocks"

	. "github.com/solo-io/gloo/projects/gloo/constants"

	"github.com/golang/mock/gomock"
	consulapi "github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
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

			testService *consulapi.CatalogService

			upstreamsToTrack      v1.UpstreamList
			consulServiceSnapshot []*consul.ServiceMeta
			serviceMetaProducer   chan []*consul.ServiceMeta
			errorProducer         chan error

			// first DNS query
			expectedEndpointsFirstAttempt v1.EndpointList
			// second DNS query
			expectedEndpointsSecondAttempt v1.EndpointList
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())

			serviceMetaProducer = make(chan []*consul.ServiceMeta)
			errorProducer = make(chan error)

			upstreamsToTrack = v1.UpstreamList{
				createTestUpstream(svc1, svc1, []string{primary, secondary, canary}, []string{dc1, dc2, dc3}),
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
			consulWatcherMock.EXPECT().WatchServices(gomock.Any(), dataCenters, consulplugin.ConsulConsistencyModes_DefaultMode, gomock.Any()).Return(serviceMetaProducer, errorProducer).Times(1)
			testService = createTestService(buildHostname(svc1, dc2), dc2, svc1, "c", []string{primary, secondary, canary}, 3456, 100)

			expectedEndpointsFirstAttempt = v1.EndpointList{
				createExpectedEndpoint(buildEndpointName("2.1.0.10", testService), svc1, testService.Address, "2.1.0.10", "100", writeNamespace, 3456, map[string]string{
					ConsulTagKeyPrefix + primary:    yes,
					ConsulTagKeyPrefix + secondary:  yes,
					ConsulTagKeyPrefix + canary:     yes,
					ConsulDataCenterKeyPrefix + dc1: no,
					ConsulDataCenterKeyPrefix + dc2: yes,
					ConsulDataCenterKeyPrefix + dc3: no,
				}),
			}

			expectedEndpointsSecondAttempt = v1.EndpointList{
				createExpectedEndpoint(buildEndpointName("2.1.0.11", testService), svc1, testService.Address, "2.1.0.11", "100", writeNamespace, 3456, map[string]string{
					ConsulTagKeyPrefix + primary:    yes,
					ConsulTagKeyPrefix + secondary:  yes,
					ConsulTagKeyPrefix + canary:     yes,
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

		Context("blocking EDS queries", func() {

			var (
				svc2 = "svc-2"
			)

			Context("svc1 gets updated endpoints", func() {

				BeforeEach(func() {
					testSvcCp := *testService
					testSvcCpPtr := &testSvcCp // copy so no data races with test pollution (goroutines still reading this value as the next test writes it)
					consulWatcherMock.EXPECT().Service(svc1, gomock.Any(), gomock.Any()).DoAndReturn(
						func(service, tag string, q *consulapi.QueryOptions) ([]*consulapi.CatalogService, *consulapi.QueryMeta, error) {
							if q.Datacenter == dc2 {
								return []*consulapi.CatalogService{testSvcCpPtr}, &consulapi.QueryMeta{LastIndex: 1}, nil
							}
							return []*consulapi.CatalogService{}, &consulapi.QueryMeta{LastIndex: 1}, nil
						}).Times(100) // busy loop that eventually ends
				})

				It("blocking queries happypath", func() {

					// queue up an update to the catalog svc
					testServiceUpdated := createTestService(buildHostname(svc1, dc2), dc2, svc1, "c", []string{primary, secondary, canary}, 3457, 100) // port updated
					consulWatcherMock.EXPECT().Service(svc1, gomock.Any(), gomock.Any()).DoAndReturn(
						func(service, tag string, q *consulapi.QueryOptions) ([]*consulapi.CatalogService, *consulapi.QueryMeta, error) {
							if q.Datacenter == dc2 {
								return []*consulapi.CatalogService{testServiceUpdated}, &consulapi.QueryMeta{LastIndex: 2}, nil
							}
							return []*consulapi.CatalogService{}, &consulapi.QueryMeta{LastIndex: 2}, nil
						}).AnyTimes()

					initialIps := []net.IPAddr{{IP: net.IPv4(2, 1, 0, 10)}}
					mockDnsResolver := mock_consul2.NewMockDnsResolver(ctrl)
					mockDnsResolver.EXPECT().Resolve(gomock.Any(), gomock.Any()).Do(func(context.Context, string) {
						fmt.Fprint(GinkgoWriter, "Initial resolve called.")
					}).Return(initialIps, nil).AnyTimes() // once for each consul service

					eds := NewPlugin(consulWatcherMock, mockDnsResolver, nil)
					eds.Init(plugins.InitParams{
						Settings: &v1.Settings{
							ConsulDiscovery: &v1.Settings_ConsulUpstreamDiscoveryConfiguration{
								EdsBlockingQueries: &wrapperspb.BoolValue{Value: true},
							},
						},
					})

					endpointsChan, errorChan, err := eds.WatchEndpoints(writeNamespace, upstreamsToTrack, clients.WatchOpts{Ctx: ctx})

					Expect(err).NotTo(HaveOccurred())

					// Simulate the initial read when starting watch
					serviceMetaProducer <- consulServiceSnapshot
					// use select instead of eventually for easier debugging.
					select {
					case err := <-errorChan:
						Expect(err).NotTo(HaveOccurred())
						Fail("err chan closed prematurely")
					case endpointsReceived := <-endpointsChan:
						Expect(endpointsReceived).To(matchers.BeEquivalentToDiff(expectedEndpointsFirstAttempt))
					case <-time.After(time.Second):
						Fail("timeout waiting for endpoints")
					}

					// simulate an error
					failErr := eris.New("fail")
					errorProducer <- failErr
					select {
					case err := <-errorChan:
						Expect(err).To(MatchError(ContainSubstring(failErr.Error())))
					case <-time.After(time.Second):
						Fail("timeout waiting for error")
					}

					// use select instead of eventually for easier debugging.
					select {
					case err := <-errorChan:
						Expect(err).NotTo(HaveOccurred())
						Fail("err chan closed prematurely")
					case endpointsReceived := <-endpointsChan:
						Expect(expectedEndpointsFirstAttempt).To(HaveLen(1))
						// we updated port from 3456 to 3457, so that's what we expect now
						expectedEndpoint := expectedEndpointsFirstAttempt[0].Clone().(*v1.Endpoint)
						expectedEndpoint.Metadata.Name = "2-1-0-10-svc-1-c-3457"
						expectedEndpoint.Port = 3457
						expectedEndpoints := v1.EndpointList{expectedEndpoint}
						Expect(endpointsReceived).To(matchers.BeEquivalentToDiff(expectedEndpoints))
					case <-time.After(time.Second):
						Fail("timeout waiting for endpoints")
					}

					// Cancel and verify that all the channels have been closed
					cancel()
					Eventually(endpointsChan).Should(BeClosed())
					Eventually(errorChan).Should(BeClosed())
				})
			})

			Context("svc1 gets removed from services catalog", func() {

				BeforeEach(func() {
					testSvcCp := *testService
					testSvcCpPtr := &testSvcCp // copy so no data races with test pollution (goroutines still reading this value as the next test writes it)

					consulWatcherMock.EXPECT().Service(svc1, gomock.Any(), gomock.Any()).DoAndReturn(
						func(service, tag string, q *consulapi.QueryOptions) ([]*consulapi.CatalogService, *consulapi.QueryMeta, error) {
							if q.Datacenter == dc2 {
								return []*consulapi.CatalogService{testSvcCpPtr}, &consulapi.QueryMeta{LastIndex: 1}, nil
							}
							return []*consulapi.CatalogService{}, &consulapi.QueryMeta{LastIndex: 1}, nil
						}).AnyTimes()
				})

				It("blocking queries cancel other watches", func() {
					// queue up an update to the catalog svc
					testServiceUpdated := createTestService(buildHostname(svc1, dc2), dc2, svc1, "c", []string{primary, secondary, canary}, 3457, 100) // port updated
					consulWatcherMock.EXPECT().Service(svc1, gomock.Any(), gomock.Any()).DoAndReturn(
						func(service, tag string, q *consulapi.QueryOptions) ([]*consulapi.CatalogService, *consulapi.QueryMeta, error) {
							if q.Datacenter == dc2 {
								return []*consulapi.CatalogService{testServiceUpdated}, &consulapi.QueryMeta{LastIndex: 2}, nil
							}
							return []*consulapi.CatalogService{}, &consulapi.QueryMeta{LastIndex: 2}, nil
						}).AnyTimes()

					initialIps := []net.IPAddr{{IP: net.IPv4(2, 1, 0, 10)}}
					mockDnsResolver := mock_consul2.NewMockDnsResolver(ctrl)
					mockDnsResolver.EXPECT().Resolve(gomock.Any(), gomock.Any()).Do(func(context.Context, string) {
						fmt.Fprint(GinkgoWriter, "Initial resolve called.")
					}).Return(initialIps, nil).AnyTimes() // once for each consul service

					eds := NewPlugin(consulWatcherMock, mockDnsResolver, nil)
					eds.Init(plugins.InitParams{
						Settings: &v1.Settings{
							ConsulDiscovery: &v1.Settings_ConsulUpstreamDiscoveryConfiguration{
								EdsBlockingQueries: &wrapperspb.BoolValue{Value: true},
							},
						},
					})

					endpointsChan, errorChan, err := eds.WatchEndpoints(writeNamespace, upstreamsToTrack, clients.WatchOpts{Ctx: ctx})
					Expect(err).NotTo(HaveOccurred())

					// Simulate the initial read when starting watch
					serviceMetaProducer <- consulServiceSnapshot
					// use select instead of eventually for easier debugging.
					select {
					case err := <-errorChan:
						Expect(err).NotTo(HaveOccurred())
						Fail("err chan closed prematurely")
					case endpointsReceived := <-endpointsChan:
						Expect(endpointsReceived).ToNot(BeEmpty())
					case <-time.After(time.Second):
						Fail("timeout waiting for endpoints")
					}

					consulServiceSnapshot = []*consul.ServiceMeta{
						{
							Name:        svc2,
							DataCenters: []string{dc1, dc2},
							Tags:        []string{primary, secondary},
						},
					}

					consulWatcherMock.EXPECT().Service(svc2, gomock.Any(), gomock.Any()).DoAndReturn(
						func(service, tag string, q *consulapi.QueryOptions) ([]*consulapi.CatalogService, *consulapi.QueryMeta, error) {
							if q.Datacenter == dc2 {
								return []*consulapi.CatalogService{}, &consulapi.QueryMeta{LastIndex: 1}, nil
							}
							return []*consulapi.CatalogService{}, &consulapi.QueryMeta{LastIndex: 1}, nil
						}).AnyTimes()
					serviceMetaProducer <- consulServiceSnapshot // removed svc1 and added svc2; this means we will close watch on svc1 and open a new one on svc2

					// Provide time to let the snapshot be processed
					Consistently(endpointsChan).ShouldNot(BeClosed())
					Consistently(errorChan).ShouldNot(BeClosed())

					// Cancel and verify that all the channels have been closed
					cancel()
					Eventually(endpointsChan).Should(BeClosed())
					Eventually(errorChan).Should(BeClosed())
				})

			})

		})

		Context("non-blocking EDS queries", func() {

			BeforeEach(func() {
				consulWatcherMock.EXPECT().Service(svc1, gomock.Any(), gomock.Any()).DoAndReturn(
					func(service, tag string, q *consulapi.QueryOptions) ([]*consulapi.CatalogService, *consulapi.QueryMeta, error) {
						if q.Datacenter == dc2 {
							return []*consulapi.CatalogService{testService}, &consulapi.QueryMeta{LastIndex: 1}, nil
						}
						return []*consulapi.CatalogService{}, &consulapi.QueryMeta{LastIndex: 1}, nil
					}).Times(3) // once for each datacenter
			})

			It("handles DNS updates even if consul services are unchanged", func() {
				initialIps := []net.IPAddr{{IP: net.IPv4(2, 1, 0, 10)}}
				mockDnsResolver := mock_consul2.NewMockDnsResolver(ctrl)
				mockDnsResolver.EXPECT().Resolve(gomock.Any(), gomock.Any()).Do(func(context.Context, string) {
					fmt.Fprint(GinkgoWriter, "Initial resolve called.")
				}).Return(initialIps, nil).Times(1) // once for each consul service

				updatedIps := []net.IPAddr{{IP: net.IPv4(2, 1, 0, 11)}}
				// once for each consul service x 2 because we will let the test run through the EDS DNS poller twice
				// the first poll, DNS will have changed and we expect to receive new endpoints on the channel
				// the second poll, DNS will resolve to the same thing and we do not expect to receive new endpoints
				mockDnsResolver.EXPECT().Resolve(gomock.Any(), gomock.Any()).Do(func(context.Context, string) {
					fmt.Fprint(GinkgoWriter, "Updated resolve called.")
				}).Return(updatedIps, nil).AnyTimes()

				duration := durationpb.New(time.Microsecond * 100) // 100,000 ns or 0.1 ms
				eds := NewPlugin(consulWatcherMock, mockDnsResolver, duration)

				endpointsChan, errorChan, err := eds.WatchEndpoints(writeNamespace, upstreamsToTrack, clients.WatchOpts{Ctx: ctx})

				Expect(err).NotTo(HaveOccurred())

				// Simulate the initial read when starting watch
				serviceMetaProducer <- consulServiceSnapshot
				// use select instead of eventually for easier debugging.
				select {
				case err := <-errorChan:
					Expect(err).NotTo(HaveOccurred())
					Fail("err chan closed prematurely")
				case endpointsReceived := <-endpointsChan:
					Expect(endpointsReceived).To(matchers.BeEquivalentToDiff(expectedEndpointsFirstAttempt))
				case <-time.After(time.Second):
					Fail("timeout waiting for endpoints")
				}

				// simulate an error
				failErr := eris.New("fail")
				errorProducer <- failErr
				select {
				case err := <-errorChan:
					Expect(err).To(MatchError(ContainSubstring(failErr.Error())))
				case <-time.After(time.Second):
					Fail("timeout waiting for error")
				}

				// Simulate an update to DNS entries via the mock DNS resolver expects set up above
				pollingInterval := duration.AsDuration() + (duration.AsDuration() / 5)
				totalInterval := duration.AsDuration() * 3

				// use select instead of eventually for easier debugging.
				select {
				case err := <-errorChan:
					Expect(err).NotTo(HaveOccurred())
					Fail("err chan closed prematurely")
				case endpointsReceived := <-endpointsChan:
					Expect(endpointsReceived).To(matchers.BeEquivalentToDiff(expectedEndpointsSecondAttempt))
				case <-time.After(totalInterval):
					Fail("timeout waiting for endpoints")
				}

				// ensure we don't receive anything else on channel even though we receive more DNS queries
				Consistently(endpointsChan, totalInterval, pollingInterval).ShouldNot(Receive())

				// Cancel and verify that all the channels have been closed
				cancel()
				Eventually(endpointsChan).Should(BeClosed())
				Eventually(errorChan).Should(BeClosed())
			})
		})

	})

	Describe("endpoints watch - not idiomatic (do not copy)", func() {

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

			serviceMetaProducer = make(chan []*consul.ServiceMeta)
			errorProducer = make(chan error)

			upstreamsToTrack = v1.UpstreamList{
				createTestFilteredUpstream(svc1, svc1, nil, []string{}, []string{dc1, dc2, dc3}),
				createTestFilteredUpstream(svc1+primary, svc1, []string{primary}, []string{primary}, []string{dc1, dc2, dc3}),
				createTestFilteredUpstream(svc1+secondary, svc1, []string{secondary}, []string{secondary}, []string{dc1, dc2, dc3}),
				createTestFilteredUpstream(svc1+canary, svc1, []string{canary}, []string{canary}, []string{dc1, dc2, dc3}),
				createTestFilteredUpstream(svc2+primary, svc2, []string{primary}, []string{primary}, []string{dc1, dc2}),
				createTestFilteredUpstream(svc2+secondary, svc2, []string{secondary}, []string{secondary}, []string{dc1, dc2}),
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
			consulWatcherMock.EXPECT().WatchServices(gomock.Any(), dataCenters, consulplugin.ConsulConsistencyModes_DefaultMode, gomock.Any()).Return(serviceMetaProducer, errorProducer).Times(1)

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
							return []*consulapi.CatalogService{
								createTestService("1.1.0.1", dc1, svc1, "a", []string{primary}, 1234, 100),
								createTestService("1.1.0.2", dc1, svc1, "b", []string{primary}, 1234, 100),
							}, nil, nil
						case dc2:
							return []*consulapi.CatalogService{
								createTestService("2.1.0.10", dc2, svc1, "c", []string{secondary}, 3456, 100),
								createTestService("2.1.0.11", dc2, svc1, "d", []string{secondary}, 4567, 100),
							}, nil, nil
						case dc3:
							services := []*consulapi.CatalogService{
								createTestService("3.1.0.99", dc3, svc1, "e", []string{secondary, canary}, 9999, 100),
							}
							// Simulate the addition of a service instance. "> 5" because the first 5 attempts are a
							// result of the first snapshot (1 invocation for every service:dataCenter pair)
							if currentAttempt > 5 {
								services = append(services, createTestService("3.1.0.3", dc3, svc1, "e1", []string{canary}, 1234, 100))
							}
							return services, nil, nil
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
				createExpectedEndpoint("1-1-0-1-svc-1-a-1234", "svc-1,svc-1primary", "", "1.1.0.1", "100", writeNamespace, 1234, map[string]string{
					ConsulTagKeyPrefix + primary:    yes,
					ConsulTagKeyPrefix + secondary:  no,
					ConsulTagKeyPrefix + canary:     no,
					ConsulDataCenterKeyPrefix + dc1: yes,
					ConsulDataCenterKeyPrefix + dc2: no,
					ConsulDataCenterKeyPrefix + dc3: no,
				}),
				createExpectedEndpoint("1-1-0-2-svc-1-b-1234", "svc-1,svc-1primary", "", "1.1.0.2", "100", writeNamespace, 1234, map[string]string{
					ConsulTagKeyPrefix + primary:    yes,
					ConsulTagKeyPrefix + secondary:  no,
					ConsulTagKeyPrefix + canary:     no,
					ConsulDataCenterKeyPrefix + dc1: yes,
					ConsulDataCenterKeyPrefix + dc2: no,
					ConsulDataCenterKeyPrefix + dc3: no,
				}),
				createExpectedEndpoint("2-1-0-10-svc-1-c-3456", "svc-1,svc-1secondary", "", "2.1.0.10", "100", writeNamespace, 3456, map[string]string{
					ConsulTagKeyPrefix + primary:    no,
					ConsulTagKeyPrefix + secondary:  yes,
					ConsulTagKeyPrefix + canary:     no,
					ConsulDataCenterKeyPrefix + dc1: no,
					ConsulDataCenterKeyPrefix + dc2: yes,
					ConsulDataCenterKeyPrefix + dc3: no,
				}),
				createExpectedEndpoint("2-1-0-11-svc-1-d-4567", "svc-1,svc-1secondary", "", "2.1.0.11", "100", writeNamespace, 4567, map[string]string{
					ConsulTagKeyPrefix + primary:    no,
					ConsulTagKeyPrefix + secondary:  yes,
					ConsulTagKeyPrefix + canary:     no,
					ConsulDataCenterKeyPrefix + dc1: no,
					ConsulDataCenterKeyPrefix + dc2: yes,
					ConsulDataCenterKeyPrefix + dc3: no,
				}),
				createExpectedEndpoint("3-1-0-99-svc-1-e-9999", "svc-1,svc-1secondary,svc-1canary", "", "3.1.0.99", "100", writeNamespace, 9999, map[string]string{
					ConsulTagKeyPrefix + primary:    no,
					ConsulTagKeyPrefix + secondary:  yes,
					ConsulTagKeyPrefix + canary:     yes,
					ConsulDataCenterKeyPrefix + dc1: no,
					ConsulDataCenterKeyPrefix + dc2: no,
					ConsulDataCenterKeyPrefix + dc3: yes,
				}),

				// 4 endpoints for service 2
				createExpectedEndpoint("1-2-0-1-svc-2-a2-8080", "svc-2primary", "", "1.2.0.1", "100", writeNamespace, 8080, map[string]string{
					ConsulTagKeyPrefix + primary:    yes,
					ConsulTagKeyPrefix + secondary:  no,
					ConsulDataCenterKeyPrefix + dc1: yes,
					ConsulDataCenterKeyPrefix + dc2: no,
				}),
				createExpectedEndpoint("1-2-0-2-svc-2-b2-8080", "svc-2primary", "", "1.2.0.2", "100", writeNamespace, 8080, map[string]string{
					ConsulTagKeyPrefix + primary:    yes,
					ConsulTagKeyPrefix + secondary:  no,
					ConsulDataCenterKeyPrefix + dc1: yes,
					ConsulDataCenterKeyPrefix + dc2: no,
				}),
				createExpectedEndpoint("2-2-0-10-svc-2-c2-8088", "svc-2secondary", "", "2.2.0.10", "100", writeNamespace, 8088, map[string]string{
					ConsulTagKeyPrefix + primary:    no,
					ConsulTagKeyPrefix + secondary:  yes,
					ConsulDataCenterKeyPrefix + dc1: no,
					ConsulDataCenterKeyPrefix + dc2: yes,
				}),
				createExpectedEndpoint("2-2-0-11-svc-2-d2-8088", "svc-2secondary", "", "2.2.0.11", "100", writeNamespace, 8088, map[string]string{
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
				createExpectedEndpoint("3-1-0-3-svc-1-e1-1234", "svc-1,svc-1canary", "", "3.1.0.3", "100", writeNamespace, 1234, map[string]string{
					ConsulTagKeyPrefix + primary:    no,
					ConsulTagKeyPrefix + secondary:  no,
					ConsulTagKeyPrefix + canary:     yes,
					ConsulDataCenterKeyPrefix + dc1: no,
					ConsulDataCenterKeyPrefix + dc2: no,
					ConsulDataCenterKeyPrefix + dc3: yes,
				}),
			)
			sort.SliceStable(expectedEndpointsSecondAttempt, func(i, j int) bool {
				return expectedEndpointsSecondAttempt[i].Metadata.Name < expectedEndpointsSecondAttempt[j].Metadata.Name
			})
		})

		AfterEach(func() {
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
					case err := <-errorChan:
						Expect(err).NotTo(HaveOccurred())
						Fail("err chan closed prematurely")
					case <-errRoutineCtx.Done():
						return nil
					}
				}
			})

			// Simulate the initial read when starting watch
			serviceMetaProducer <- consulServiceSnapshot
			var asProtos []proto.Message
			for _, v := range expectedEndpointsFirstAttempt {
				asProtos = append(asProtos, v)
			}
			Eventually(endpointsChan).Should(Receive(proto_matchers.ConsistOfProtos(asProtos...)))

			// Wait for error monitoring routine to stop, we want to simulate an error
			errRoutineCancel()
			_ = eg.Wait()

			errorProducer <- eris.New("fail")
			Eventually(errorChan).Should(Receive())

			// Simulate an update to the services
			// We use the same metadata snapshot because what changed is the service spec
			serviceMetaProducer <- consulServiceSnapshot
			Eventually(endpointsChan).Should(Receive(matchers.BeEquivalentToDiff(expectedEndpointsSecondAttempt)))

			// Cancel and verify that all the channels have been closed
			cancel()
			Eventually(endpointsChan).Should(BeClosed())
			Eventually(errorChan).Should(BeClosed())
		})

	})

	Describe("unit tests", func() {
		It("generates unique endpoint names", func() {

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
				}, {
					ID:         "12341234-1234-1234-1234-123412341234",
					Node:       "ip-1.2.3.4",
					Address:    "1.2.3.4",
					Datacenter: "test-dns",
					TaggedAddresses: map[string]string{
						"lan": "1.2.3.4",
						"wan": "1.2.3.4",
					},
					ServiceID:      "foo",
					ServiceName:    "foo",
					ServiceAddress: "1.2.3.4",
					ServiceTags:    []string{"ftp", "http"},
					ServicePort:    1237,
				},
			}

			twoIps := []net.IPAddr{{IP: net.IPv4(2, 1, 0, 10)}, {IP: net.IPv4(2, 1, 0, 11)}}
			mockDnsResolver := mock_consul2.NewMockDnsResolver(ctrl)
			mockDnsResolver.EXPECT().Resolve(gomock.Any(), gomock.Any()).Return(twoIps, nil).Times(1)

			trackedServiceToUpstreams := make(map[string][]*v1.Upstream)
			for _, svc := range svcs {
				trackedServiceToUpstreams[svc.ServiceName] = []*v1.Upstream{
					{
						Metadata: &core.Metadata{
							Name:      "n",
							Namespace: "n",
						},
						UpstreamType: &v1.Upstream_Consul{
							Consul: &consulplugin.UpstreamSpec{
								ServiceName:  "foo",
								InstanceTags: []string{"http"},
							},
						},
					}, {
						Metadata: &core.Metadata{
							Name:      "n1",
							Namespace: "n",
						},
						UpstreamType: &v1.Upstream_Consul{
							Consul: &consulplugin.UpstreamSpec{
								ServiceName:  "foo",
								InstanceTags: []string{"http", "ftp"},
							},
						},
					}, {
						Metadata: &core.Metadata{
							Name:      "n2",
							Namespace: "n",
						},
						UpstreamType: &v1.Upstream_Consul{
							Consul: &consulplugin.UpstreamSpec{
								ServiceName: "foo",
							},
						},
					},
				}
			}

			// make sure the we have a correct number of generated endpoints:
			endpoints := buildEndpointsFromSpecs(context.TODO(), writeNamespace, mockDnsResolver, svcs, trackedServiceToUpstreams)
			endpontNames := map[string]bool{}
			for _, endpoint := range endpoints {
				fmt.Fprintf(GinkgoWriter, "%s%v\n", "endpoint: ", endpoint)
				endpontNames[endpoint.GetMetadata().Name] = true

				Expect(endpoint.Upstreams).To(proto_matchers.ContainProto(&core.ResourceRef{
					Name:      "n2",
					Namespace: "n",
				}))
				switch endpoint.GetPort() {
				case 1235:
					// 1235 is the http endpoint above
					Expect(endpoint.Upstreams).To(HaveLen(2))
					Expect(endpoint.Upstreams).To(proto_matchers.ContainProto(&core.ResourceRef{
						Name:      "n",
						Namespace: "n",
					}))
				case 1237:
					// 1237 is the ftp,http endpoint above
					Expect(endpoint.Upstreams).To(HaveLen(3))
					Expect(endpoint.Upstreams).To(proto_matchers.ContainProto(&core.ResourceRef{
						Name:      "n1",
						Namespace: "n",
					}))
					Expect(endpoint.Upstreams).To(proto_matchers.ContainProto(&core.ResourceRef{
						Name:      "n",
						Namespace: "n",
					}))
				default:
					Expect(endpoint.Upstreams).To(HaveLen(1))
				}
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
				ServiceTags: []string{"tag-1", "tag-3", "http"},
				ModifyIndex: 9876,
			}
			upstream := createTestFilteredUpstream("my-svc", "my-svc", []string{"tag-1", "tag-3"}, []string{"http"}, []string{"dc-1", "dc-2"})
			// add another upstream so to test that tag2 is in the labels.
			upstream2 := createTestFilteredUpstream("my-svc-2", "my-svc", []string{"tag-2"}, []string{"serf"}, []string{"dc-1", "dc-2"})

			endpoints, err := buildEndpoints(context.TODO(), writeNamespace, nil, consulService, v1.UpstreamList{upstream, upstream2})
			Expect(err).To(BeNil())
			Expect(endpoints).To(HaveLen(1))
			Expect(endpoints[0]).To(matchers.BeEquivalentToDiff(&v1.Endpoint{
				Metadata: &core.Metadata{
					Namespace: writeNamespace,
					Name:      "127-0-0-1-my-svc-my-svc-0-1234",
					Labels: map[string]string{
						ConsulTagKeyPrefix + "tag-1":       ConsulEndpointMetadataMatchTrue,
						ConsulTagKeyPrefix + "tag-2":       ConsulEndpointMetadataMatchFalse,
						ConsulTagKeyPrefix + "tag-3":       ConsulEndpointMetadataMatchTrue,
						ConsulDataCenterKeyPrefix + "dc-1": ConsulEndpointMetadataMatchTrue,
						ConsulDataCenterKeyPrefix + "dc-2": ConsulEndpointMetadataMatchFalse,
					},
					ResourceVersion: "9876",
				},
				Upstreams: []*core.ResourceRef{upstream.Metadata.Ref()},
				Address:   "127.0.0.1",
				Port:      1234,
			}))
		})

		It("generates the correct endpoint for a given Consul service -- propagates hostname", func() {
			consulService := &consulapi.CatalogService{
				ServiceID:   "my-svc-0",
				ServiceName: "my-svc",
				Address:     "hostname.foo.com",
				ServicePort: 1234,
				Datacenter:  "dc-1",
				ServiceTags: []string{"tag-1", "tag-3", "http"},
				ModifyIndex: 9876,
			}
			upstream := createTestFilteredUpstream("my-svc", "my-svc", []string{"tag-1", "tag-3"}, []string{"http"}, []string{"dc-1", "dc-2"})

			// we have to put all the mock expects before the test starts or else the test may have data races
			initialIps := []net.IPAddr{{IP: net.IPv4(127, 0, 0, 1)}}
			mockDnsResolver := mock_consul2.NewMockDnsResolver(ctrl)
			mockDnsResolver.EXPECT().Resolve(gomock.Any(), gomock.Any()).Do(func(context.Context, string) {
				fmt.Fprint(GinkgoWriter, "Initial resolve called.")
			}).Return(initialIps, nil).Times(1) // once for each consul service

			endpoints, err := buildEndpoints(context.TODO(), writeNamespace, mockDnsResolver, consulService, v1.UpstreamList{upstream})
			Expect(err).To(BeNil())
			Expect(endpoints).To(HaveLen(1))
			Expect(endpoints[0]).To(matchers.BeEquivalentToDiff(&v1.Endpoint{
				Metadata: &core.Metadata{
					Namespace: writeNamespace,
					Name:      "127-0-0-1-my-svc-my-svc-0-1234",
					Labels: map[string]string{
						ConsulTagKeyPrefix + "tag-1":       ConsulEndpointMetadataMatchTrue,
						ConsulTagKeyPrefix + "tag-3":       ConsulEndpointMetadataMatchTrue,
						ConsulDataCenterKeyPrefix + "dc-1": ConsulEndpointMetadataMatchTrue,
						ConsulDataCenterKeyPrefix + "dc-2": ConsulEndpointMetadataMatchFalse,
					},
					ResourceVersion: "9876",
				},
				Upstreams:   []*core.ResourceRef{upstream.Metadata.Ref()},
				Address:     "127.0.0.1",
				Port:        1234,
				Hostname:    "hostname.foo.com",
				HealthCheck: &v1.HealthCheckConfig{Hostname: "hostname.foo.com"},
			}))
		})

		It("uses the previous IP addresses if DNS resolution fails", func() {
			consulService := &consulapi.CatalogService{
				ServiceID:   "my-svc-0",
				ServiceName: "my-svc",
				Address:     "my.address.io",
				ServicePort: 1234,
				Datacenter:  "dc-1",
				ServiceTags: []string{"tag-1", "http"},
				ModifyIndex: 9876,
			}

			initialIps := []net.IPAddr{{IP: net.IPv4(127, 0, 0, 1)}}
			mockDnsResolver := mock_consul2.NewMockDnsResolver(ctrl)
			mockDnsResolverWithFallback := NewDnsResolverWithFallback(mockDnsResolver)
			mockDnsResolver.EXPECT().Resolve(gomock.Any(), gomock.Any()).Do(func(context.Context, string) {
				fmt.Fprint(GinkgoWriter, "Initial resolve called.")
			}).Return(initialIps, nil).Times(1)

			upstream := createTestFilteredUpstream("my-svc", "my-svc", []string{"tag-1"}, []string{"http"}, []string{"dc-1", "dc-2"})

			// Initial call should be successfull
			endpoints, err := buildEndpoints(context.TODO(), writeNamespace, mockDnsResolverWithFallback, consulService, v1.UpstreamList{upstream})
			Expect(err).To(BeNil())
			Expect(endpoints).To(HaveLen(1))
			Expect(endpoints[0]).To(matchers.BeEquivalentToDiff(&v1.Endpoint{
				Metadata: &core.Metadata{
					Namespace: writeNamespace,
					Name:      "127-0-0-1-my-svc-my-svc-0-1234",
					Labels: map[string]string{
						ConsulTagKeyPrefix + "tag-1":       ConsulEndpointMetadataMatchTrue,
						ConsulDataCenterKeyPrefix + "dc-1": ConsulEndpointMetadataMatchTrue,
						ConsulDataCenterKeyPrefix + "dc-2": ConsulEndpointMetadataMatchFalse,
					},
					ResourceVersion: "9876",
				},
				Upstreams:   []*core.ResourceRef{upstream.Metadata.Ref()},
				Address:     "127.0.0.1",
				Port:        1234,
				Hostname:    "my.address.io",
				HealthCheck: &v1.HealthCheckConfig{Hostname: "my.address.io"},
			}))

			failErr := eris.New("fail")
			mockDnsResolver.EXPECT().Resolve(gomock.Any(), gomock.Any()).Do(func(context.Context, string) {
				fmt.Fprint(GinkgoWriter, "Errored resolve called.")
			}).Return(nil, failErr).Times(1)

			// Following call should also be successfull despite the error
			endpoints, err = buildEndpoints(context.TODO(), writeNamespace, mockDnsResolverWithFallback, consulService, v1.UpstreamList{upstream})
			Expect(err).To(BeNil())
			Expect(endpoints).To(HaveLen(1))
			Expect(endpoints[0]).To(matchers.BeEquivalentToDiff(&v1.Endpoint{
				Metadata: &core.Metadata{
					Namespace: writeNamespace,
					Name:      "127-0-0-1-my-svc-my-svc-0-1234",
					Labels: map[string]string{
						ConsulTagKeyPrefix + "tag-1":       ConsulEndpointMetadataMatchTrue,
						ConsulDataCenterKeyPrefix + "dc-1": ConsulEndpointMetadataMatchTrue,
						ConsulDataCenterKeyPrefix + "dc-2": ConsulEndpointMetadataMatchFalse,
					},
					ResourceVersion: "9876",
				},
				Upstreams:   []*core.ResourceRef{upstream.Metadata.Ref()},
				Address:     "127.0.0.1",
				Port:        1234,
				Hostname:    "my.address.io",
				HealthCheck: &v1.HealthCheckConfig{Hostname: "my.address.io"},
			}))
		})

	})
})

func createTestUpstream(usptreamName, svcName string, tags, dataCenters []string) *v1.Upstream {
	return createTestFilteredUpstream(usptreamName, svcName, tags, nil, dataCenters)
}
func createTestFilteredUpstream(usptreamName, svcName string, tags, instancetags, dataCenters []string) *v1.Upstream {
	return &v1.Upstream{
		Metadata: &core.Metadata{
			Name:      "consul-svc:" + usptreamName,
			Namespace: "",
		},
		UpstreamType: &v1.Upstream_Consul{
			Consul: &consulplugin.UpstreamSpec{
				ServiceName:  svcName,
				SubsetTags:   tags,
				InstanceTags: instancetags,
				DataCenters:  dataCenters,
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

func createExpectedEndpoint(name, usname, hostname, ipAddress, version, ns string, port uint32, labels map[string]string) *v1.Endpoint {
	var healthCheckConfig *v1.HealthCheckConfig
	if hostname != "" {
		healthCheckConfig = &v1.HealthCheckConfig{Hostname: hostname}
	}

	ep := &v1.Endpoint{
		Metadata: &core.Metadata{
			Namespace:       ns,
			Name:            name,
			Labels:          labels,
			ResourceVersion: version,
		},

		Address:     ipAddress,
		Port:        port,
		Hostname:    hostname,
		HealthCheck: healthCheckConfig,
	}

	for _, svc := range strings.Split(usname, ",") {
		ep.Upstreams = append(ep.Upstreams, &core.ResourceRef{
			Name:      "consul-svc:" + svc,
			Namespace: "",
		})
	}

	return ep
}

func buildHostname(svc, dc string) string {
	return svc + ".service." + dc + ".consul"
}
