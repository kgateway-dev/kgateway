package consul_test

import (
	"context"
	"time"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	consulplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/consul"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/golang/mock/gomock"
	consulapi "github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rotisserie/eris"
	. "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	. "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul/mocks"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("ClientWrapper", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
		ctrl   *gomock.Controller
		client *MockClientWrapper
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		ctrl = gomock.NewController(T)
		client = NewMockClientWrapper(ctrl)
	})

	AfterEach(func() {
		if cancel != nil {
			cancel()
		}
		ctrl.Finish()
	})

	Describe("list operation", func() {

		BeforeEach(func() {
			client.EXPECT().DataCenters().Return([]string{"dc1", "dc2"}, nil).Times(1)
		})

		Context("default consistency mode", func() {
			BeforeEach(func() {
				dcServices := []dataCenterServicesTuple{{
					DataCenter: "dc1",
					Services:   map[string][]string{"svc-1": {"tag-1", "tag-2"}, "svc-2": {"tag-2"}},
				}, {
					DataCenter: "dc2",
					Services:   map[string][]string{"svc-1": {"tag-1"}, "svc-3": {}},
				}}

				setupDatacenterServices(ctx, client, &consulapi.QueryOptions{RequireConsistent: false, AllowStale: false, UseCache: true}, &dcServices)
			})

			It("returns the expected upstreams", func() {
				usClient := NewConsulUpstreamClient(NewConsulWatcherFromClient(client), nil)

				upstreams, err := usClient.List(defaults.GlooSystem, clients.ListOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())

				Expect(upstreams).To(HaveLen(3))
				Expect(upstreams).To(ConsistOf(
					CreateUpstreamsFromService(&ServiceMeta{Name: "svc-1", DataCenters: []string{"dc1", "dc2"}, Tags: []string{"tag-1", "tag-2"}}, nil)[0],
					CreateUpstreamsFromService(&ServiceMeta{Name: "svc-2", DataCenters: []string{"dc1"}, Tags: []string{"tag-2"}}, nil)[0],
					CreateUpstreamsFromService(&ServiceMeta{Name: "svc-3", DataCenters: []string{"dc2"}}, nil)[0],
				))
			})
		})

		Context("non-default consistency mode", func() {
			It("returns the expected upstreams using stale consistency mode", func() {
				dcServices := []dataCenterServicesTuple{{
					DataCenter: "dc1",
					Services:   map[string][]string{"svc-1": {"tag-1", "tag-2"}, "svc-2": {"tag-2"}},
				}, {
					DataCenter: "dc2",
					Services:   map[string][]string{"svc-1": {"tag-1"}, "svc-3": {}},
				}}

				setupDatacenterServices(ctx, client, &consulapi.QueryOptions{RequireConsistent: false, AllowStale: true, UseCache: true}, &dcServices)
				usClient := NewConsulUpstreamClient(
					NewConsulWatcherFromClient(client),
					&v1.Settings_ConsulUpstreamDiscoveryConfiguration{
						ConsistencyMode: consulplugin.ConsulConsistencyModes_StaleMode,
					})

				upstreams, err := usClient.List(defaults.GlooSystem, clients.ListOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())

				Expect(upstreams).To(HaveLen(3))
				Expect(upstreams).To(ConsistOf(
					CreateUpstreamsFromService(&ServiceMeta{Name: "svc-1", DataCenters: []string{"dc1", "dc2"}, Tags: []string{"tag-1", "tag-2"}}, &v1.Settings_ConsulUpstreamDiscoveryConfiguration{ConsistencyMode: consulplugin.ConsulConsistencyModes_StaleMode})[0],
					CreateUpstreamsFromService(&ServiceMeta{Name: "svc-2", DataCenters: []string{"dc1"}, Tags: []string{"tag-2"}}, &v1.Settings_ConsulUpstreamDiscoveryConfiguration{ConsistencyMode: consulplugin.ConsulConsistencyModes_StaleMode})[0],
					CreateUpstreamsFromService(&ServiceMeta{Name: "svc-3", DataCenters: []string{"dc2"}}, &v1.Settings_ConsulUpstreamDiscoveryConfiguration{ConsistencyMode: consulplugin.ConsulConsistencyModes_StaleMode})[0],
				))
			})

			It("returns the expected upstreams using Consul's default consistency mode", func() {
				dcServices := []dataCenterServicesTuple{{
					DataCenter: "dc1",
					Services:   map[string][]string{"svc-1": {"tag-1", "tag-2"}, "svc-2": {"tag-2"}},
				}, {
					DataCenter: "dc2",
					Services:   map[string][]string{"svc-1": {"tag-1"}, "svc-3": {}},
				}}

				setupDatacenterServices(ctx, client, &consulapi.QueryOptions{RequireConsistent: false, AllowStale: false, UseCache: true}, &dcServices)

				usClient := NewConsulUpstreamClient(
					NewConsulWatcherFromClient(client),
					&v1.Settings_ConsulUpstreamDiscoveryConfiguration{
						ConsistencyMode: consulplugin.ConsulConsistencyModes_DefaultMode,
					})

				upstreams, err := usClient.List(defaults.GlooSystem, clients.ListOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())

				Expect(upstreams).To(HaveLen(3))
				Expect(upstreams).To(ConsistOf(
					CreateUpstreamsFromService(&ServiceMeta{Name: "svc-1", DataCenters: []string{"dc1", "dc2"}, Tags: []string{"tag-1", "tag-2"}}, &v1.Settings_ConsulUpstreamDiscoveryConfiguration{ConsistencyMode: consulplugin.ConsulConsistencyModes_DefaultMode})[0],
					CreateUpstreamsFromService(&ServiceMeta{Name: "svc-2", DataCenters: []string{"dc1"}, Tags: []string{"tag-2"}}, &v1.Settings_ConsulUpstreamDiscoveryConfiguration{ConsistencyMode: consulplugin.ConsulConsistencyModes_DefaultMode})[0],
					CreateUpstreamsFromService(&ServiceMeta{Name: "svc-3", DataCenters: []string{"dc2"}}, &v1.Settings_ConsulUpstreamDiscoveryConfiguration{ConsistencyMode: consulplugin.ConsulConsistencyModes_DefaultMode})[0],
				))
			})

		})
	})

	Describe("watch operation", func() {

		Context("no errors occur", func() {

			BeforeEach(func() {
				client.EXPECT().DataCenters().Return([]string{"dc1", "dc2"}, nil).Times(1)

				// ----------- Data center 1 -----------
				dc1 := "dc1"

				// Initial call, no delay
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter: dc1,
					WaitIndex:  0,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(100, []string{"svc-1"}, 0)).Times(1)

				// Second call simulates blocking query that returns with updated resources
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter: dc1,
					WaitIndex:  100,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(200, []string{"svc-1", "svc-2"}, 100*time.Millisecond)).Times(1)

				// Expect any number of subsequent invocations and return same resource version (last index)
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter: dc1,
					WaitIndex:  200,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(200, []string{"svc-1", "svc-2"}, 200*time.Millisecond)).AnyTimes()

				// ----------- Data center 2 -----------
				dc2 := "dc2"

				// Initial call, no delay
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter: dc2,
					WaitIndex:  0,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(100, []string{}, 0)).Times(1)

				// Second call simulates blocking query that returns with updated resources
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter: dc2,
					WaitIndex:  100,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(250, []string{"svc-1", "svc-3"}, 200*time.Millisecond)).Times(1)

				// Expect any number of subsequent invocations and return same resource version (last index)
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter: dc2,
					WaitIndex:  250,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(250, []string{"svc-1", "svc-3"}, 200*time.Millisecond)).AnyTimes()

			})

			It("correctly reacts to service updates", func() {
				usClient := NewConsulUpstreamClient(NewConsulWatcherFromClient(client), nil)

				upstreamChan, errChan, err := usClient.Watch(defaults.GlooSystem, clients.WatchOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())

				Eventually(upstreamChan, 500*time.Millisecond).Should(Receive(ConsistOf(
					CreateUpstreamsFromService(&ServiceMeta{Name: "svc-1", DataCenters: []string{"dc1", "dc2"}}, nil)[0],
					CreateUpstreamsFromService(&ServiceMeta{Name: "svc-2", DataCenters: []string{"dc1"}}, nil)[0],
					CreateUpstreamsFromService(&ServiceMeta{Name: "svc-3", DataCenters: []string{"dc2"}}, nil)[0],
				)))

				// WARNING: the following Consistently exposes a brief 100ms window into an
				// unbuffered channel. If messages are sent on that channel before this call,
				// they will not cause a failure here. Consider using a buffered channel and/or
				// explicitly setting duration (default 100ms) and interval (default 10ms)
				Eventually(errChan, DefaultConsistentlyDuration, DefaultConsistentlyPollInterval).ShouldNot(Receive())

				// Cancel and verify that all the channels have been closed
				cancel()
				Eventually(upstreamChan, DefaultEventuallyTimeout, DefaultEventuallyPollInterval).Should(BeClosed())
				Eventually(errChan, DefaultEventuallyTimeout, DefaultEventuallyPollInterval).Should(BeClosed())
			})
		})

		Context("a transient error occurs while contacting the Consul agent", func() {

			BeforeEach(func() {

				dc1 := "dc1"
				client.EXPECT().DataCenters().Return([]string{dc1}, nil).Times(1)

				// Initial call, no delay
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter: dc1,
					WaitIndex:  0,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(100, []string{"svc-1"}, 0)).Times(1)

				// We need this to react differently on the same expectation
				attemptNum := 0

				// Simulate failure
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter: dc1,
					WaitIndex:  100,
				}).WithContext(ctx)).DoAndReturn(
					func(q *consulapi.QueryOptions) (map[string][]string, *consulapi.QueryMeta, error) {
						time.Sleep(50 * time.Millisecond)

						attemptNum++

						// Simulate failure on the first attempt
						if attemptNum == 1 {
							return nil, nil, eris.New("flake")
						}

						return map[string][]string{"svc-1": nil, "svc-2": nil}, &consulapi.QueryMeta{LastIndex: 200}, nil
					},
				).Times(2)

				// Expect any number of subsequent invocations and return same resource version (last index)
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter: dc1,
					WaitIndex:  200,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(200, []string{"svc-1", "svc-3"}, 200*time.Millisecond)).AnyTimes()
			})

			It("can recover from the error", func() {
				usClient := NewConsulUpstreamClient(NewConsulWatcherFromClient(client), nil)

				upstreamChan, errChan, err := usClient.Watch(defaults.GlooSystem, clients.WatchOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())

				// The retry delay in the consul client is 100ms
				Eventually(upstreamChan, 300*time.Millisecond).Should(Receive(ConsistOf(
					CreateUpstreamsFromService(&ServiceMeta{Name: "svc-1", DataCenters: []string{"dc1"}}, nil)[0],
					CreateUpstreamsFromService(&ServiceMeta{Name: "svc-2", DataCenters: []string{"dc1"}}, nil)[0],
				)))

				// WARNING: the following Consistently exposes a brief 100ms window into an
				// unbuffered channel. If messages are sent on that channel before this call,
				// they will not cause a failure here. Consider using a buffered channel and/or
				// explicitly setting duration (default 100ms) and interval (default 10ms)
				Eventually(errChan, DefaultConsistentlyDuration, DefaultConsistentlyPollInterval).ShouldNot(Receive())

				// Cancel and verify that all the channels have been closed
				cancel()
				Eventually(upstreamChan, DefaultEventuallyTimeout, DefaultEventuallyPollInterval).Should(BeClosed())
				Eventually(errChan, DefaultEventuallyTimeout, DefaultEventuallyPollInterval).Should(BeClosed())
			})
		})

		Context("services do not change during the lifetime of the watch", func() {

			BeforeEach(func() {
				dc1 := "dc1"
				client.EXPECT().DataCenters().Return([]string{dc1}, nil).Times(1)

				// Initial call, no delay
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter: dc1,
					WaitIndex:  0,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(100, []string{"svc-1"}, 0)).Times(1)

				// Expect any number of subsequent invocations and return same resource version (last index)
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter: dc1,
					WaitIndex:  100,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(100, []string{"svc-1"}, 100*time.Millisecond)).AnyTimes()
			})

			It("publishes a single event", func() {
				usClient := NewConsulUpstreamClient(NewConsulWatcherFromClient(client), nil)

				upstreamChan, errChan, err := usClient.Watch(defaults.GlooSystem, clients.WatchOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())

				// Give the watch some time to start
				time.Sleep(50 * time.Millisecond)

				// We get the expected message
				Expect(upstreamChan).Should(Receive(ConsistOf(CreateUpstreamsFromService(&ServiceMeta{Name: "svc-1", DataCenters: []string{"dc1"}}, nil)[0])))

				// We don't get any further messages
				// WARNING: the following Consistently exposes a brief 100ms window into an
				// unbuffered channel. If messages are sent on that channel before this call,
				// they will not cause a failure here. Consider using a buffered channel and/or
				// explicitly setting duration (default 100ms) and interval (default 10ms)
				Eventually(upstreamChan, DefaultConsistentlyDuration, DefaultConsistentlyPollInterval).ShouldNot(Receive())

				// WARNING: the following Consistently exposes a brief 100ms window into an
				// unbuffered channel. If messages are sent on that channel before this call,
				// they will not cause a failure here. Consider using a buffered channel and/or
				// explicitly setting duration (default 100ms) and interval (default 10ms)
				Eventually(errChan, DefaultConsistentlyDuration, DefaultConsistentlyPollInterval).ShouldNot(Receive())

				// Cancel and verify that all the channels have been closed
				cancel()
				Eventually(upstreamChan, DefaultEventuallyTimeout, DefaultEventuallyPollInterval).Should(BeClosed())
				Eventually(errChan, DefaultEventuallyTimeout, DefaultEventuallyPollInterval).Should(BeClosed())
			})
		})
	})
})

// Represents the signature of the Services function
type svcQueryFunc func(q *consulapi.QueryOptions) (map[string][]string, *consulapi.QueryMeta, error)

func returnWithDelay(newIndex uint64, services []string, delay time.Duration) svcQueryFunc {

	svcMap := make(map[string][]string, len(services))
	for _, svc := range services {
		svcMap[svc] = nil
	}

	return func(q *consulapi.QueryOptions) (map[string][]string, *consulapi.QueryMeta, error) {
		time.Sleep(delay)
		return svcMap, &consulapi.QueryMeta{LastIndex: newIndex}, nil
	}
}

type dataCenterServicesTuple struct {
	DataCenter string
	Services   map[string][]string
}

func setupDatacenterServices(ctx context.Context, client *MockClientWrapper, queryOptions *consulapi.QueryOptions, returns *[]dataCenterServicesTuple) {
	for _, r := range *returns {
		client.EXPECT().Services((&consulapi.QueryOptions{
			Datacenter:        r.DataCenter,
			RequireConsistent: queryOptions.RequireConsistent,
			AllowStale:        queryOptions.AllowStale,
		}).WithContext(ctx)).Return(r.Services, nil, nil).Times(1)
	}
}
