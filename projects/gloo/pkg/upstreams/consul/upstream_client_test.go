package consul

import (
	"context"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/golang/mock/gomock"
	consulapi "github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/go-utils/errors"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("ConsulClient", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
		ctrl   *gomock.Controller
		client *MockConsulClient
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		ctrl = gomock.NewController(T)
		client = NewMockConsulClient(ctrl)
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

			client.EXPECT().Services((&consulapi.QueryOptions{
				Datacenter:        "dc1",
				RequireConsistent: true,
			}).WithContext(ctx)).Return(
				map[string][]string{
					"svc-1": {"tag-1", "tag-2"},
					"svc-2": {"tag-2"},
				},
				nil,
				nil,
			).Times(1)

			client.EXPECT().Services((&consulapi.QueryOptions{
				Datacenter:        "dc2",
				RequireConsistent: true,
			}).WithContext(ctx)).Return(
				map[string][]string{
					"svc-1": {"tag-1"},
					"svc-3": {},
				},
				nil,
				nil,
			).Times(1)
		})

		It("returns the expected upstreams", func() {
			usClient := NewConsulUpstreamClient(NewConsulWatcherFromClient(client))

			upstreams, err := usClient.List(defaults.GlooSystem, clients.ListOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())

			Expect(upstreams).To(HaveLen(3))
			Expect(upstreams).To(ConsistOf(
				ToUpstream(&ServiceMeta{Name: "svc-1", DataCenters: []string{"dc1", "dc2"}, Tags: []string{"tag-1", "tag-2"}}),
				ToUpstream(&ServiceMeta{Name: "svc-2", DataCenters: []string{"dc1"}, Tags: []string{"tag-2"}}),
				ToUpstream(&ServiceMeta{Name: "svc-3", DataCenters: []string{"dc2"}}),
			))
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
					Datacenter:        dc1,
					RequireConsistent: true,
					WaitIndex:         0,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(100, []string{"svc-1"}, 0)).Times(1)

				// Second call simulates blocking query that returns with updated resources
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter:        dc1,
					RequireConsistent: true,
					WaitIndex:         100,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(200, []string{"svc-1", "svc-2"}, 100*time.Millisecond)).Times(1)

				// Expect any number of subsequent invocations and return same resource version (last index)
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter:        dc1,
					RequireConsistent: true,
					WaitIndex:         200,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(200, []string{"svc-1", "svc-2"}, 200*time.Millisecond)).AnyTimes()

				// ----------- Data center 2 -----------
				dc2 := "dc2"

				// Initial call, no delay
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter:        dc2,
					RequireConsistent: true,
					WaitIndex:         0,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(100, []string{}, 0)).Times(1)

				// Second call simulates blocking query that returns with updated resources
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter:        dc2,
					RequireConsistent: true,
					WaitIndex:         100,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(250, []string{"svc-1", "svc-3"}, 200*time.Millisecond)).Times(1)

				// Expect any number of subsequent invocations and return same resource version (last index)
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter:        dc2,
					RequireConsistent: true,
					WaitIndex:         250,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(250, []string{"svc-1", "svc-3"}, 200*time.Millisecond)).AnyTimes()

			})

			It("correctly reacts to service updates", func() {
				usClient := NewConsulUpstreamClient(NewConsulWatcherFromClient(client))

				upstreamChan, errChan, err := usClient.Watch(defaults.GlooSystem, clients.WatchOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())

				Eventually(upstreamChan, 500*time.Millisecond).Should(Receive(ConsistOf(
					ToUpstream(&ServiceMeta{Name: "svc-1", DataCenters: []string{"dc1", "dc2"}}),
					ToUpstream(&ServiceMeta{Name: "svc-2", DataCenters: []string{"dc1"}}),
					ToUpstream(&ServiceMeta{Name: "svc-3", DataCenters: []string{"dc2"}}),
				)))

				Consistently(errChan).ShouldNot(Receive())

				// Cancel and verify that all the channels have been closed
				cancel()
				Eventually(upstreamChan).Should(BeClosed())
				Eventually(errChan).Should(BeClosed())
			})
		})

		// TODO(marco): temporarily disable, want to see if there are other failures in CI while I investigate
		XContext("a transient error occurs while contacting the Consul agent", func() {

			BeforeEach(func() {

				dc1 := "dc1"
				client.EXPECT().DataCenters().Return([]string{dc1}, nil).Times(1)

				// Initial call, no delay
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter:        dc1,
					RequireConsistent: true,
					WaitIndex:         0,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(100, []string{"svc-1"}, 0)).Times(1)

				// We need this to react differently on the same expectation
				attemptNum := 0

				// Simulate failure
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter:        dc1,
					RequireConsistent: true,
					WaitIndex:         100,
				}).WithContext(ctx)).DoAndReturn(
					func(q *consulapi.QueryOptions) (map[string][]string, *consulapi.QueryMeta, error) {
						time.Sleep(50 * time.Millisecond)

						attemptNum++

						// Simulate failure on the first attempt
						if attemptNum == 1 {
							return nil, nil, errors.New("flake")
						}

						return map[string][]string{"svc-1": nil, "svc-2": nil}, &consulapi.QueryMeta{LastIndex: 200}, nil
					},
				).Times(2)

				// Expect any number of subsequent invocations and return same resource version (last index)
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter:        dc1,
					RequireConsistent: true,
					WaitIndex:         200,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(200, []string{"svc-1", "svc-3"}, 200*time.Millisecond)).AnyTimes()
			})

			It("can recover from the error", func() {
				usClient := NewConsulUpstreamClient(NewConsulWatcherFromClient(client))

				upstreamChan, errChan, err := usClient.Watch(defaults.GlooSystem, clients.WatchOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())

				// The retry delay in the consul client is 100ms
				Eventually(upstreamChan, 300*time.Millisecond).Should(Receive(ConsistOf(
					ToUpstream(&ServiceMeta{Name: "svc-1", DataCenters: []string{"dc1"}}),
					ToUpstream(&ServiceMeta{Name: "svc-2", DataCenters: []string{"dc1"}}),
				)))

				Consistently(errChan).ShouldNot(Receive())

				// Cancel and verify that all the channels have been closed
				cancel()
				Eventually(upstreamChan).Should(BeClosed())
				Eventually(errChan).Should(BeClosed())
			})
		})

		Context("services do not change during the lifetime of the watch", func() {

			BeforeEach(func() {
				dc1 := "dc1"
				client.EXPECT().DataCenters().Return([]string{dc1}, nil).Times(1)

				// Initial call, no delay
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter:        dc1,
					RequireConsistent: true,
					WaitIndex:         0,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(100, []string{"svc-1"}, 0)).Times(1)

				// Expect any number of subsequent invocations and return same resource version (last index)
				client.EXPECT().Services((&consulapi.QueryOptions{
					Datacenter:        dc1,
					RequireConsistent: true,
					WaitIndex:         100,
				}).WithContext(ctx)).DoAndReturn(returnWithDelay(100, []string{"svc-1"}, 100*time.Millisecond)).AnyTimes()
			})

			It("publishes a single event", func() {
				usClient := NewConsulUpstreamClient(NewConsulWatcherFromClient(client))

				upstreamChan, errChan, err := usClient.Watch(defaults.GlooSystem, clients.WatchOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())

				// Give the watch some time to start
				time.Sleep(50 * time.Millisecond)

				// We get the expected message
				Expect(upstreamChan).Should(Receive(ConsistOf(ToUpstream(&ServiceMeta{Name: "svc-1", DataCenters: []string{"dc1"}}))))

				// We don't get any further messages
				Consistently(upstreamChan).ShouldNot(Receive())

				Consistently(errChan).ShouldNot(Receive())

				// Cancel and verify that all the channels have been closed
				cancel()
				Eventually(upstreamChan).Should(BeClosed())
				Eventually(errChan).Should(BeClosed())
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
