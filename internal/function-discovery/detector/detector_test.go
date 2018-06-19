package detector_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	. "github.com/solo-io/gloo/internal/function-discovery/detector"
	"github.com/solo-io/gloo/internal/function-discovery/resolver"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/static"
	"github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("Marker", func() {
	Context("happy path", func() {
		It("marks the upstream with the service info", func() {
			resolve := resolver.NewResolver(nil, nil)
			marker := NewMarker([]Interface{
				&mockDetector{id: "failing", triesBeforeSucceding: 50},
				&mockDetector{id: "succeeding", triesBeforeSucceding: 3},
			}, resolve)
			us := helpers.NewTestUpstream2()
			us.Spec = static.EncodeUpstreamSpec(static.UpstreamSpec{
				Hosts: []static.Host{
					{Addr: "localhost", Port: 8000},
				},
			})
			svcInfo, annotations, err := marker.DetectFunctionalServiceType(us)
			Expect(err).NotTo(HaveOccurred())
			Expect(svcInfo).To(Equal(&v1.ServiceInfo{Type: "mock_service"}))
			Expect(annotations).To(Equal(map[string]string{"foo": "bar"}))
			Expect(totalTries).To(BeNumerically(">=", 5))
		})
	})
})

var totalTries int

type mockDetector struct {
	id                   string
	triesBeforeSucceding int
}

func (d *mockDetector) DetectFunctionalService(_ *v1.Upstream, addr string) (*v1.ServiceInfo, map[string]string, error) {
	totalTries++
	d.triesBeforeSucceding--
	if d.triesBeforeSucceding > 0 {
		return nil, nil, errors.Errorf("mock[%s]: failed detection", d.id)
	}
	return &v1.ServiceInfo{Type: "mock_service"}, map[string]string{"foo": "bar"}, nil
}
