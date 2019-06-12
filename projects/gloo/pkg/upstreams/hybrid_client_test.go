package upstreams_test

import (
	"context"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"time"
)

var _ = Describe("HybridUpstreams", func() {

	var (
		ctx            context.Context
		cancel         context.CancelFunc
		svcClient      skkube.ServiceClient
		baseUsClient   v1.UpstreamClient
		hybridClient   v1.UpstreamClient
		watchNamespace = "watched-ns"
		err            error

		existingFakeUpstreamName = upstreams.ServiceUpstreamNamePrefix + "svc-1-8081"

		// Results in 5 upstreams being created, 2 real, 3 service-derived (one of which is in a different namespace)
		writeResources = func() {
			opts := clients.WriteOpts{Ctx: ctx}
			_, err = svcClient.Write(getService("svc-1", watchNamespace, []int32{8080, 8081}), opts)
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(10 * time.Millisecond) // give the event some time to propagate

			_, err = baseUsClient.Write(getUpstream("us-1", watchNamespace, "svc-3", watchNamespace, 1234), opts)
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(10 * time.Millisecond) // give the event some time to propagate

			_, err = svcClient.Write(getService("svc-2", watchNamespace, []int32{9001}), opts)
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(10 * time.Millisecond) // give the event some time to propagate

			_, err = svcClient.Write(getService("svc-3", "other-namespace", []int32{9999}), opts)
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(10 * time.Millisecond) // give the event some time to propagate
		}
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		inMemoryFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}

		svcClient, err = skkube.NewServiceClient(inMemoryFactory)
		Expect(err).NotTo(HaveOccurred())

		baseUsClient, err = v1.NewUpstreamClient(inMemoryFactory)
		Expect(err).NotTo(HaveOccurred())

		hybridClient, err = upstreams.NewHybridUpstreamClient(baseUsClient, svcClient)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cancel()
	})

	Context("read operations", func() {

		It("is able to look up a service-derived upstream", func() {
			writeResources()

			us, err := hybridClient.Read(watchNamespace, existingFakeUpstreamName, clients.ReadOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())
			Expect(us.UpstreamSpec.GetKube()).NotTo(BeNil())
			Expect(us.UpstreamSpec.GetKube().ServiceName).To(Equal("svc-1"))
		})

		It("fails when looking up a non-existent service-derived upstream", func() {
			writeResources()

			_, err := hybridClient.Read(watchNamespace, upstreams.ServiceUpstreamNamePrefix+"svc-1-9999", clients.ReadOpts{Ctx: ctx})
			Expect(err).To(HaveOccurred())
		})

		It("correctly lists real and service-derived upstreams", func() {
			writeResources()

			list, err := hybridClient.List(watchNamespace, clients.ListOpts{})
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(HaveLen(4))
		})

		It("correctly aggregates watches on real and service-derived upstreams", func() {
			usChan, errChan, initErr := hybridClient.Watch(watchNamespace, clients.WatchOpts{Ctx: ctx})
			Expect(initErr).NotTo(HaveOccurred())

			// Helper to read from channels
			consumer := newWatchConsumer()
			consumer.collect(ctx, usChan, errChan)

			writeResources()

			Expect(consumer.upstreamLists).To(HaveLen(3))
			Expect(consumer.upstreamLists[0]).To(HaveLen(2))
			Expect(consumer.upstreamLists[1]).To(HaveLen(3))
			Expect(consumer.upstreamLists[2]).To(HaveLen(4))
		})
	})

	Context("write operations", func() {

		It("does not write service-derived upstreams", func() {
			fakeUs := getUpstream(upstreams.ServiceUpstreamNamePrefix+"us-1", watchNamespace, "svc-3", watchNamespace, 1234)
			_, err := hybridClient.Write(fakeUs, clients.WriteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())

			list, err := hybridClient.List(watchNamespace, clients.ListOpts{})
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(HaveLen(0))
		})

		It("writes real upstreams", func() {
			realUs := getUpstream("us-1", watchNamespace, "svc-3", watchNamespace, 1234)
			_, err := hybridClient.Write(realUs, clients.WriteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())

			list, err := hybridClient.List(watchNamespace, clients.ListOpts{})
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(HaveLen(1))
		})

		It("does not delete service-derived upstreams", func() {
			writeResources()

			err := hybridClient.Delete(watchNamespace, existingFakeUpstreamName, clients.DeleteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())

			list, err := hybridClient.List(watchNamespace, clients.ListOpts{})
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(HaveLen(4))
		})

		It("deletes real upstreams", func() {
			writeResources()

			err := hybridClient.Delete(watchNamespace, "us-1", clients.DeleteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())

			list, err := hybridClient.List(watchNamespace, clients.ListOpts{})
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(HaveLen(3))
		})

	})
})

type watchConsumer struct {
	upstreamLists []v1.UpstreamList
	errors        []error
}

func newWatchConsumer() *watchConsumer {
	return &watchConsumer{}
}

func (c *watchConsumer) collect(ctx context.Context, usChan <-chan v1.UpstreamList, errorChan <-chan error) *watchConsumer {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case usList, ok := <-usChan:
				if ok {
					c.upstreamLists = append(c.upstreamLists, usList)
				}
			case err, ok := <-errorChan:
				if ok {
					c.errors = append(c.errors, err)
				}
			}
		}
	}()
	return c
}
