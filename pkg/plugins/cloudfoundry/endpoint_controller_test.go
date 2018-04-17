package cloudfoundry_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	. "github.com/solo-io/gloo/pkg/plugins/cloudfoundry"
)

var _ = Describe("EndpointController", func() {

	var fakeClient *FakeIstioClient
	var endpointDiscovery endpointdiscovery.Interface
	var cancel context.CancelFunc
	var upstream *v1.Upstream
	const hostname = "hostname"

	BeforeEach(func() {
		fakeClient = &FakeIstioClient{}
		ctx, cncl := context.WithCancel(context.Background())
		cancel = cncl
		// check frequently for eventually to work
		endpointDiscovery = NewEndpointDiscovery(ctx, fakeClient, time.Second/1000)
		upstream = &v1.Upstream{
			Name: HostnameToUpstreamName(hostname),
			Type: UpstreamTypeCF,
			Spec: EncodeUpstreamSpec(UpstreamSpec{
				Hostname: hostname,
			}),
		}
	})

	AfterEach(func() {
		cancel()
	})

	It("should discover some endpoints", func() {
		fakeClient.FakeResponse = FakeResponse(hostname, "address", 1337)
		endpointDiscovery.TrackUpstreams([]*v1.Upstream{upstream})
		go endpointDiscovery.Run(nil)
		Eventually(endpointDiscovery.Endpoints()).Should(Receive())
	})

	It("should error when client fails", func() {
		fakeClient.FakeResponseError = errors.New("fake")
		endpointDiscovery.TrackUpstreams([]*v1.Upstream{upstream})
		go endpointDiscovery.Run(nil)
		Eventually(endpointDiscovery.Error()).Should(Receive())
	})

	It("should ignore unknown upstreams", func() {
		fakeClient.FakeResponse = FakeResponse(hostname, "address", 1337)
		upstream.Type = "not a real type"
		endpointDiscovery.TrackUpstreams([]*v1.Upstream{upstream})
		go endpointDiscovery.Run(nil)
		Consistently(endpointDiscovery.Endpoints(), "1s").ShouldNot(Receive())
		Consistently(endpointDiscovery.Error(), "1s").ShouldNot(Receive())
	})

	It("should only report the upstream once", func() {
		fakeClient.FakeResponse = FakeResponse(hostname, "address", 1337)
		endpointDiscovery.TrackUpstreams([]*v1.Upstream{upstream})
		go endpointDiscovery.Run(nil)
		Eventually(endpointDiscovery.Endpoints()).Should(Receive())
		Consistently(endpointDiscovery.Endpoints(), "1s").ShouldNot(Receive())
		Consistently(endpointDiscovery.Error(), "1s").ShouldNot(Receive())
	})

	It("should report changes", func() {

		fakeClient.FakeResponse = FakeResponse(hostname, "address", 1337)
		endpointDiscovery.TrackUpstreams([]*v1.Upstream{upstream})
		go endpointDiscovery.Run(nil)

		expected := endpointdiscovery.EndpointGroups{}
		expected[upstream.Name] = []endpointdiscovery.Endpoint{{Address: "address", Port: 1337}}
		Eventually(endpointDiscovery.Endpoints()).Should(Receive(Equal(expected)))

		fakeClient.SetFakeResponse(hostname, "address2", 1337)
		expected = endpointdiscovery.EndpointGroups{}
		expected[upstream.Name] = []endpointdiscovery.Endpoint{{Address: "address2", Port: 1337}}
		Eventually(endpointDiscovery.Endpoints()).Should(Receive(Equal(expected)))

	})

})
