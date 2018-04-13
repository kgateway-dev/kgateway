package detector_test

import (
	"sync/atomic"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	. "github.com/solo-io/gloo/internal/function-discovery/detector"
	"github.com/solo-io/gloo/internal/function-discovery/resolver"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("Marker", func() {
	Context("happy path", func() {
		It("marks the upstream with the service info", func() {
			resolve := resolver.NewResolver(nil)
			marker := NewMarker([]Interface{
				&mockDetector{id: "failing", triesBeforeSucceding: 50},
				&mockDetector{id: "succeeding", triesBeforeSucceding: 3},
			}, resolve)
			us := helpers.NewTestUpstream2()
			svcInfo, annotations, err := marker.DetectFunctionalUpstream(us)
			Expect(err).NotTo(HaveOccurred())
			Expect(svcInfo).To(Equal(&v1.ServiceInfo{Type: "mock_service"}))
			Expect(annotations).To(Equal(map[string]string{"foo": "bar"}))
			Expect(atomic.LoadInt32(&totalTriesCounter)).To(BeNumerically(">=", 5))
		})
	})
})

var totalTriesCounter int32

type mockDetector struct {
	id                   string
	triesBeforeSucceding int
}

func (d *mockDetector) DetectFunctionalService(_ *v1.Upstream, addr string) (*v1.ServiceInfo, map[string]string, error) {
	atomic.AddInt32(&totalTriesCounter, 1)
	d.triesBeforeSucceding--
	if d.triesBeforeSucceding > 0 {
		return nil, nil, errors.Errorf("mock[%s]: failed detection", d.id)
	}
	return &v1.ServiceInfo{Type: "mock_service"}, map[string]string{"foo": "bar"}, nil
}
