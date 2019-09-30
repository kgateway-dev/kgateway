package translator

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/retries"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/transformation"
)

var _ = Describe("MergeRoutePlugins", func() {
	It("merges top-level route plugins fields", func() {
		dst := &v1.RoutePlugins{
			PrefixRewrite: &transformation.PrefixRewrite{
				PrefixRewrite: "preserve-me",
			},
			Retries: &retries.RetryPolicy{
				RetryOn:    "5XX",
				NumRetries: 0, // should not overwrite this field
			},
		}
		d := time.Minute
		src := &v1.RoutePlugins{
			Timeout: &d,
			Retries: &retries.RetryPolicy{
				RetryOn:    "5XX",
				NumRetries: 3, // do not overwrite 0 value above
			},
			RatelimitBasic: &ratelimit.IngressRateLimit{
				AuthorizedLimits: &ratelimit.RateLimit{
					Unit:            1,
					RequestsPerUnit: 2,
				},
			},
		}
		expected := &v1.RoutePlugins{
			PrefixRewrite: &transformation.PrefixRewrite{
				PrefixRewrite: "preserve-me",
			},
			Timeout: &d,
			Retries: &retries.RetryPolicy{
				RetryOn:    "5XX",
				NumRetries: 0,
			},
			RatelimitBasic: &ratelimit.IngressRateLimit{
				AuthorizedLimits: &ratelimit.RateLimit{
					Unit:            1,
					RequestsPerUnit: 2,
				},
			},
		}

		actual, err := mergeRoutePlugins(dst, src)
		Expect(err).NotTo(HaveOccurred())
		Expect(actual).To(Equal(expected))
	})
})
