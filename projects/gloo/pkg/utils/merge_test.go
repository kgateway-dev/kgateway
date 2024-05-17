package utils

import (
	"time"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rltypes "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/retries"
)

var _ = Describe("Merge", func() {
	It("merges top-level route options fields", func() {
		dst := &v1.RouteOptions{
			PrefixRewrite: &wrappers.StringValue{Value: "preserve-me"},
			Retries: &retries.RetryPolicy{
				RetryOn:    "5XX",
				NumRetries: 0, // should not overwrite this field
			},
		}
		d := prototime.DurationToProto(time.Minute)
		src := &v1.RouteOptions{
			Timeout: d,
			Retries: &retries.RetryPolicy{
				RetryOn:    "5XX",
				NumRetries: 3, // do not overwrite 0 value above
				// do not merge PerTryTimeout because it doesn't exist in dst
				PerTryTimeout: d,
			},
			RatelimitBasic: &ratelimit.IngressRateLimit{
				AuthorizedLimits: &rltypes.RateLimit{
					Unit:            1,
					RequestsPerUnit: 2,
				},
			},
		}
		expected := &v1.RouteOptions{
			PrefixRewrite: &wrappers.StringValue{Value: "preserve-me"},
			Timeout:       d,
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

		actual, overwrote := ShallowMergeRouteOptions(dst, src)
		Expect(actual).To(Equal(expected))
		Expect(overwrote).To(BeTrue())
	})
})
