package consul

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"time"

	"strings"

	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("EndpointController", func() {
	Describe("controller", func() {
		It("watches consul services and returns endpoints", func() {
			cfg := api.DefaultConfig()
			eds, err := NewEndpointController(cfg, false)
			Expect(err).NotTo(HaveOccurred())

			ch := make(chan struct{})
			defer close(ch)
			go eds.Run(ch)

			consul, err := api.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())

			svc1a := newConsulSvc("svc1", nil, "1.2.3.4", 1234)
			svc1b := newConsulSvc("svc1", nil, "2.3.4.5", 2345)
			svc2 := newConsulSvc("svc2", nil, "3.4.5.6", 3456)
			svc3notags := newConsulSvc("svc3", []string{}, "4.5.6.7", 3456)
			svc3sometags := newConsulSvc("svc3", []string{"a"}, "5.6.7.8", 3456)
			svc3alltags := newConsulSvc("svc3", []string{"a", "b"}, "6.7.8.9", 3456)

			us1 := newUpstreamFromSvc(svc1a)
			us2 := newUpstreamFromSvc(svc2)
			us3noTags := newUpstreamFromSvc(svc3notags)
			us3someTags := newUpstreamFromSvc(svc3sometags)
			us3allTags := newUpstreamFromSvc(svc3alltags)

			go eds.TrackUpstreams([]*v1.Upstream{us1, us2, us3noTags, us3someTags, us3allTags})

			err = consul.Agent().ServiceRegister(svc1a)
			Expect(err).NotTo(HaveOccurred())
			err = consul.Agent().ServiceRegister(svc1b)
			Expect(err).NotTo(HaveOccurred())
			err = consul.Agent().ServiceRegister(svc2)
			Expect(err).NotTo(HaveOccurred())
			err = consul.Agent().ServiceRegister(svc3notags)
			Expect(err).NotTo(HaveOccurred())
			err = consul.Agent().ServiceRegister(svc3sometags)
			Expect(err).NotTo(HaveOccurred())
			err = consul.Agent().ServiceRegister(svc3alltags)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() (endpointdiscovery.EndpointGroups, error) {
				select {
				case eps := <-eds.Endpoints():
					return eps, nil
				case err := <-eds.Error():
					return nil, err
				case <-time.After(time.Second * 5):
					return nil, errors.New("timed out")
				}
			}).Should(Equal(endpointdiscovery.EndpointGroups{
				"upstream-for-svc2": {
					{
						Address: "3.4.5.6",
						Port:    3456,
					},
				},
				"upstream-for-svc3-a": {
					{
						Address: "5.6.7.8",
						Port:    3456,
					},
					{
						Address: "6.7.8.9",
						Port:    3456,
					},
				},
				"upstream-for-svc3": {
					{
						Address: "4.5.6.7",
						Port:    3456,
					},
					{
						Address: "5.6.7.8",
						Port:    3456,
					},
					{
						Address: "6.7.8.9",
						Port:    3456,
					},
				},
				"upstream-for-svc3-a-b": {
					{
						Address: "6.7.8.9",
						Port:    3456,
					},
				},
				"upstream-for-svc1": {
					{
						Address: "1.2.3.4",
						Port:    1234,
					},
					{
						Address: "2.3.4.5",
						Port:    2345,
					},
				},
			}))
		})
	})
})

func newConsulSvc(name string, tags []string, address string, port int) *api.AgentServiceRegistration {
	return &api.AgentServiceRegistration{
		ID:      helpers.RandString(4),
		Name:    name,
		Tags:    tags,
		Port:    port,
		Address: address,
	}
}

func newUpstreamFromSvc(svc *api.AgentServiceRegistration) *v1.Upstream {
	name := "upstream-for-" + svc.Name
	if len(svc.Tags) > 0 {
		name += "-" + strings.Join(svc.Tags, "-")
	}
	return &v1.Upstream{
		Name: name,
		Type: UpstreamTypeConsul,
		Spec: EncodeUpstreamSpec(UpstreamSpec{
			ServiceName: svc.Name,
			ServiceTags: svc.Tags,
		}),
	}
}
