package create_test

import (
	"context"
	"sort"

	"github.com/solo-io/solo-kit/test/matchers"

	"github.com/golang/protobuf/ptypes/wrappers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/create"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("UpstreamGroup", func() {

	var (
		expectedDest []*v1.WeightedDestination
		ctx          context.Context
		cancel       context.CancelFunc
	)

	BeforeEach(func() {
		helpers.UseMemoryClients()
		ctx, cancel = context.WithCancel(context.Background())
		us := &v1.Upstream{
			UpstreamType: &v1.Upstream_Aws{
				Aws: &aws.UpstreamSpec{
					Region: "test-region",
					SecretRef: &core.ResourceRef{
						Namespace: "gloo-system",
						Name:      "test-aws-us",
					},
				},
			},
			Metadata: &core.Metadata{
				Namespace: "gloo-system",
				Name:      "us1",
			},
		}
		_, err := helpers.MustUpstreamClient(ctx).Write(us, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
		us.Metadata.Name = "us2"
		_, err = helpers.MustUpstreamClient(ctx).Write(us, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		expectedDest = []*v1.WeightedDestination{
			{
				Destination: &v1.Destination{
					DestinationType: &v1.Destination_Upstream{
						Upstream: &core.ResourceRef{
							Namespace: "gloo-system",
							Name:      "us1",
						},
					},
				},
				Weight: &wrappers.UInt32Value{Value: 1},
			},
			{
				Destination: &v1.Destination{
					DestinationType: &v1.Destination_Upstream{
						Upstream: &core.ResourceRef{
							Namespace: "gloo-system",
							Name:      "us2",
						},
					},
				},
				Weight: &wrappers.UInt32Value{Value: 3},
			},
		}
	})

	getUpstreamGroup := func(name string) *v1.UpstreamGroup {
		ug, err := helpers.MustUpstreamGroupClient(ctx).Read("gloo-system", name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		return ug
	}

	AfterEach(func() { cancel() })

	Context("Empty args and flags", func() {
		It("should give clear error message", func() {
			err := testutils.Glooctl("create upstreamgroup")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(create.EmptyUpstreamGroupCreateError))
		})
	})

	Context("Invalid format", func() {
		It("should error when namespaced upstreams have invalid format", func() {
			err := testutils.Glooctl("create upstreamgroup test --namespace gloo-system --weighted-upstreams us1=2,us2=3")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid format: provide namespaced upstream names"))
		})
	})

	Context("It works", func() {
		It("should work", func() {
			err := testutils.Glooctl("create upstreamgroup test --namespace gloo-system --weighted-upstreams gloo-system.us1=1,gloo-system.us2=3")
			Expect(err).NotTo(HaveOccurred())

			ug := getUpstreamGroup("test")
			sort.SliceStable(ug.Destinations, func(i, j int) bool {
				return ug.Destinations[i].Weight.GetValue() < ug.Destinations[j].Weight.GetValue()
			})
			for index, actualDestination := range ug.Destinations {
				Expect(actualDestination).To(matchers.MatchProto(expectedDest[index]))
			}
		})

		It("should bump weights to at least 1", func() {
			err := testutils.Glooctl("create upstreamgroup test --namespace gloo-system --weighted-upstreams gloo-system.us1=0,gloo-system.us2=3")
			Expect(err).NotTo(HaveOccurred())

			ug := getUpstreamGroup("test")
			sort.SliceStable(ug.Destinations, func(i, j int) bool {
				return ug.Destinations[i].Weight.GetValue() < ug.Destinations[j].Weight.GetValue()
			})
			for index, actualDestination := range ug.Destinations {
				Expect(actualDestination).To(matchers.MatchProto(expectedDest[index]))
			}
		})

		It("can print as kube yaml in dry-run", func() {
			out, err := testutils.GlooctlOut("create upstreamgroup test --namespace gloo-system --weighted-upstreams gloo-system.us1=2 --dry-run")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring(`apiVersion: gloo.solo.io/v1
kind: UpstreamGroup
metadata:
  creationTimestamp: null
  name: test
  namespace: gloo-system
spec:
  destinations:
  - destination:
      upstream:
        name: us1
        namespace: gloo-system
    weight: 2
status: {}
`))
		})
	})
})
