package consul

import (
	"net"
	"net/url"

	mock_consul2 "github.com/solo-io/gloo/projects/gloo/pkg/plugins/consul/mocks"

	"github.com/golang/mock/gomock"
	consulapi "github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	mock_consul "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul/mocks"
)

var _ = Describe("Resolve", func() {
	var (
		ctrl              *gomock.Controller
		consulWatcherMock *mock_consul.MockConsulWatcher
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(T)

		consulWatcherMock = mock_consul.NewMockConsulWatcher(ctrl)

	})

	AfterEach(func() {
		ctrl.Finish()
	})

	// TODO(kdorosh) test for hostnames resolution..

	It("can resolve consul service addresses that are IPs", func() {
		plug := NewPlugin(consulWatcherMock, nil)

		svcName := "my-svc"
		tag := "tag"
		dc := "dc1"

		us := createTestUpstream(svcName, []string{tag}, []string{dc})

		queryOpts := &consulapi.QueryOptions{Datacenter: dc, RequireConsistent: true}

		consulWatcherMock.EXPECT().Service(svcName, "", queryOpts).Return([]*consulapi.CatalogService{
			{
				ServiceAddress: "5.6.7.8",
				ServicePort:    1234,
			},
			{
				ServiceAddress: "1.2.3.4",
				ServicePort:    1234,
				ServiceTags:    []string{tag},
			},
		}, nil, nil)

		u, err := plug.Resolve(us)
		Expect(err).NotTo(HaveOccurred())

		Expect(u).To(Equal(&url.URL{Scheme: "http", Host: "1.2.3.4:1234"}))
	})

	It("can resolve consul service addresses that are hostnames", func() {

		ips := []net.IPAddr{
			{IP: net.IPv4(2, 1, 0, 10)}, // we will arbitrarily default to the first DNS response
			{IP: net.IPv4(2, 1, 0, 11)},
		}
		mockDnsResolver := mock_consul2.NewMockDnsResolver(ctrl)
		mockDnsResolver.EXPECT().Resolve("test.service.consul").Return(ips, nil).Times(1)

		plug := NewPlugin(consulWatcherMock, mockDnsResolver)

		svcName := "my-svc"
		tag := "tag"
		dc := "dc1"

		us := createTestUpstream(svcName, []string{tag}, []string{dc})

		queryOpts := &consulapi.QueryOptions{Datacenter: dc, RequireConsistent: true}

		consulWatcherMock.EXPECT().Service(svcName, "", queryOpts).Return([]*consulapi.CatalogService{
			{
				ServiceAddress: "5.6.7.8",
				ServicePort:    1234,
			},
			{
				ServiceAddress: "test.service.consul",
				ServicePort:    1234,
				ServiceTags:    []string{tag},
			},
		}, nil, nil)

		u, err := plug.Resolve(us)
		Expect(err).NotTo(HaveOccurred())

		Expect(u).To(Equal(&url.URL{Scheme: "http", Host: "2.1.0.10:1234"}))
	})
})
