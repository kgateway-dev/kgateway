package upstreams_test

import (
	"context"
	"fmt"
	"time"

	"github.com/rotisserie/eris"
	mock_consul "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul/mocks"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	. "github.com/solo-io/gloo/test/gomega"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
)

var _ = Describe("Hybrid Upstream Client", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
		ctrl   *gomock.Controller

		svcClient                skkube.ServiceClient
		baseUsClient             v1.UpstreamClient
		mockInternalConsulClient *mock_consul.MockClientWrapper

		hybridClient v1.UpstreamClient

		watchNamespace = "watched-ns"
		err            error
		usIndex        = 0

		// Results in 5 upstreams being created, 1 real, 4 service-derived (one of which is in a different namespace)
		writeAnotherUpstream = func() {
			usIndex++
			// Real upstream
			_, err = baseUsClient.Write(getUpstream(fmt.Sprintf("us-%d", usIndex), watchNamespace, "svc-3", watchNamespace, 1234), clients.WriteOpts{Ctx: ctx})
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
		}
		writeResources = func() {
			opts := clients.WriteOpts{Ctx: ctx}
			writeAnotherUpstream()
			// Kubernetes services
			_, err = svcClient.Write(getService("svc-1", watchNamespace, []int32{8080, 8081}), opts)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			_, err = svcClient.Write(getService("svc-2", watchNamespace, []int32{9001}), opts)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			_, err = svcClient.Write(getService("svc-3", "other-namespace", []int32{9999}), opts)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
		}
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		ctrl = gomock.NewController(T)

		inMemoryFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}

		baseUsClient, err = v1.NewUpstreamClient(ctx, inMemoryFactory)
		Expect(err).NotTo(HaveOccurred())

		svcClient, err = skkube.NewServiceClient(ctx, inMemoryFactory)
		Expect(err).NotTo(HaveOccurred())

		mockInternalConsulClient = mock_consul.NewMockClientWrapper(ctrl)
		mockInternalConsulClient.EXPECT().DataCenters().Return([]string{"dc1"}, nil).AnyTimes()
		mockInternalConsulClient.EXPECT().Services(gomock.Any()).Return(
			map[string][]string{"svc-1": {}},
			&api.QueryMeta{LastIndex: 100},
			nil,
		).AnyTimes()

		// In certain tests we override this, so we need to default to nil before each test
		upstreams.TimerOverride = nil
	})

	JustBeforeEach(func() {
		hybridClient, err = upstreams.NewHybridUpstreamClient(
			baseUsClient,
			svcClient,
			consul.NewConsulWatcherFromClient(mockInternalConsulClient),
			nil,
		)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cancel()
		ctrl.Finish()
	})

	It("correctly lists real and service-derived upstreams", func() {
		writeResources()

		list, err := hybridClient.List(watchNamespace, clients.ListOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())
		Expect(list).To(HaveLen(4))
	})

	It("correctly aggregates watches on all underlying upstream sources", func() {
		usChan, errChan, initErr := hybridClient.Watch(watchNamespace, clients.WatchOpts{Ctx: ctx})
		Expect(initErr).NotTo(HaveOccurred())

		writeResources()

		Eventually(func() (v1.UpstreamList, error) {
			select {
			case list := <-usChan:
				return list, nil
			case <-time.After(500 * time.Millisecond):
				return nil, eris.Errorf("timed out waiting for next upstream list")
			}
		}, "3s").Should(HaveLen(4))
		// WARNING: the following Consistently exposes a brief 100ms window into an
		// unbuffered channel. If messages are sent on that channel before this call,
		// they will not cause a failure here. Consider using a buffered channel and/or
		// explicitly setting duration (default 100ms) and interval (default 10ms)
		Consistently(errChan, DefaultConsistentlyDuration, DefaultConsistentlyPollingInterval).Should(Not(Receive()))

		cancel()
		Eventually(usChan, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(BeClosed())
		Eventually(errChan, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(BeClosed())
	})

	It("successfully sends even if polled sporadically", func() {
		timerC := make(chan time.Time, 1)
		upstreams.TimerOverride = timerC
		usChan, _, err := hybridClient.Watch(watchNamespace, clients.WatchOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())

		// get the initial list
		Eventually(usChan, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(Receive())

		writeResources()
		// give it time to propagate to watch goroutine <-collectUpstreamsChan
		time.Sleep(time.Second / 10)
		timerC <- time.Now()
		// give it time to propagate to watch goroutine <-timerC
		time.Sleep(time.Second / 10)
		// do a **single** poll.
		Expect(usChan).To(Receive(HaveLen(4)))

		for i := 0; i < 5; i++ {
			// add another upstream give time to process and try again
			writeAnotherUpstream()
			time.Sleep(time.Second / 10)
			timerC <- time.Now()
			time.Sleep(time.Second / 10)
			// do a **single** poll.
			Expect(usChan).To(Receive(HaveLen(4 + (i + 1))))
		}

	})

	Context("Sleep client", func() {

		BeforeEach(func() {
			baseUsClient = sleepyClient{UpstreamClient: baseUsClient}
		})

		It("correctly returns a full snapshot even if watch is delayed", func() {
			writeResources()

			usChan, errChan, initErr := hybridClient.Watch(watchNamespace, clients.WatchOpts{Ctx: ctx})
			Expect(initErr).NotTo(HaveOccurred())

			Eventually(func() (v1.UpstreamList, error) {
				select {
				case list := <-usChan:
					return list, nil
				case <-time.After(500 * time.Millisecond):
					return nil, eris.Errorf("timed out waiting for next upstream list")
				}
			}, "3s").Should(HaveLen(4))

			// WARNING: the following Consistently exposes a brief 100ms window into an
			// unbuffered channel. If messages are sent on that channel before this call,
			// they will not cause a failure here. Consider using a buffered channel and/or
			// explicitly setting duration (default 100ms) and interval (default 10ms)
			Consistently(errChan, DefaultConsistentlyDuration, DefaultConsistentlyPollingInterval).Should(Not(Receive()))

			cancel()
			Eventually(usChan, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(BeClosed())
			Eventually(errChan, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(BeClosed())
		})
	})

	Context("kubernetes client is nil", func() {

		BeforeEach(func() {
			writeResources()

			// We need the svc client to write resources. When we are done, set it to nil
			svcClient = nil
		})

		It("does not list upstreams derived from Kubernetes services", func() {
			list, err := hybridClient.List(watchNamespace, clients.ListOpts{})
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(HaveLen(1))
		})

	})
})

type sleepyClient struct {
	v1.UpstreamClient
}

func (s sleepyClient) Watch(namespace string, opts clients.WatchOpts) (<-chan v1.UpstreamList, <-chan error, error) {
	c, e, err := s.UpstreamClient.Watch(namespace, opts)
	if err != nil {
		return c, e, err
	}

	var delayedC chan v1.UpstreamList

	go func() {
		for e := range c {
			time.Sleep(time.Second)
			delayedC <- e
		}
	}()

	return delayedC, e, err
}
