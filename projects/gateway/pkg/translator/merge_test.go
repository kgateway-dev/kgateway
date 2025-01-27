package translator

import (
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	rltypes "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/retries"
)

var _ = Describe("Merge", func() {
	It("merges top-level virtualhost options fields", func() {
		dst := &v1.VirtualHostOptions{
			HeaderManipulation: &headers.HeaderManipulation{
				ResponseHeadersToRemove: []string{"header1"},
			},
			Retries: &retries.RetryPolicy{
				RetryOn:    "5XX",
				NumRetries: 0, // should not overwrite this field
			},
		}
		src := &v1.VirtualHostOptions{
			HeaderManipulation: &headers.HeaderManipulation{
				RequestHeadersToRemove: []string{"header1"},
			},
			Retries: &retries.RetryPolicy{
				RetryOn:    "5XX",
				NumRetries: 3, // do not overwrite 0 value above
				// do not merge PerTryTimeout because it doesn't exist in dst
				PerTryTimeout: &duration.Duration{
					Seconds: 2,
				},
			},
			RatelimitBasic: &ratelimit.IngressRateLimit{
				AuthorizedLimits: &rltypes.RateLimit{
					Unit:            1,
					RequestsPerUnit: 2,
				},
			},
		}
		expected := &v1.VirtualHostOptions{
			HeaderManipulation: &headers.HeaderManipulation{
				ResponseHeadersToRemove: []string{"header1"},
			},
			Retries: &retries.RetryPolicy{
				RetryOn:    "5XX",
				NumRetries: 0,
			},
			RatelimitBasic: &ratelimit.IngressRateLimit{
				AuthorizedLimits: &rltypes.RateLimit{
					Unit:            1,
					RequestsPerUnit: 2,
				},
			},
		}

		actual, _ := utils.ShallowMergeVirtualHostOptions(dst, src)
		Expect(actual).To(Equal(expected))
	})
})
