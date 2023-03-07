package printers

import (
	"context"
	"fmt"
	"strings"

	gloostatusutils "github.com/solo-io/gloo/pkg/utils/statusutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("getAggregateVirtualServiceStatus", func() {
	var (
		thing1    = "thing1"
		thing2    = "thing2"
		namespace = "gloo-system"

		ctx    context.Context
		cancel context.CancelFunc

		statusClient resources.StatusClient
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		statusClient = gloostatusutils.GetStatusClientForNamespace(defaults.GlooSystem)
	})

	AfterEach(func() {
		cancel()
	})

	It("handles Pending resource state", func() {
		vs := &v1.VirtualService{}
		statusClient.SetStatus(vs, &core.Status{
			State:      core.Status_Pending,
			ReportedBy: "gloo",
		})
		Expect(getAggregateVirtualServiceStatus(ctx, vs, namespace)).To(Equal(core.Status_Pending.String()))

		// range through all possible sub resource states
		for subResourceStatusString, subResourceStatusInt := range core.Status_State_value {
			subResourceStatusState := core.Status_State(subResourceStatusInt)
			namespacedStatus := statusClient.GetStatus(vs)
			namespacedStatus.SubresourceStatuses = map[string]*core.Status{
				thing1: {
					State:  subResourceStatusState,
					Reason: "any reason",
				},
			}
			By(fmt.Sprintf("subresource: %v", subResourceStatusString))
			Expect(getAggregateVirtualServiceStatus(ctx, vs, namespace)).To(Equal(core.Status_Pending.String()))
		}
	})

	It("handles Accepted resource state", func() {
		vs := &v1.VirtualService{}
		statusClient.SetStatus(vs, &core.Status{
			State:      core.Status_Accepted,
			ReportedBy: "gloo",
		})
		Expect(getAggregateVirtualServiceStatus(ctx, vs, namespace)).To(Equal(core.Status_Accepted.String()))

		// range through all possible sub resource states
		for subResourceStatusString, subResourceStatusInt := range core.Status_State_value {
			subResourceStatusState := core.Status_State(subResourceStatusInt)
			By(fmt.Sprintf("subresource: %v", subResourceStatusString))
			status := statusClient.GetStatus(vs)
			status.SubresourceStatuses = map[string]*core.Status{
				thing1: {
					State:      subResourceStatusState,
					Reason:     "any reason",
					ReportedBy: "gloo",
				},
			}
			vs.SetNamespacedStatuses(&core.NamespacedStatuses{
				Statuses: map[string]*core.Status{
					"gloo-system": status,
				},
			})

			if subResourceStatusString == core.Status_Accepted.String() {
				Expect(getAggregateVirtualServiceStatus(ctx, vs, namespace)).To(Equal(core.Status_Accepted.String()))
			} else {
				Expect(getAggregateVirtualServiceStatus(ctx, vs, namespace)).To(Equal(core.Status_Accepted.String() + "\n" + genericSubResourceMessage(thing1, subResourceStatusString)))
			}
		}
	})

	It("handles simple non-Pending and non-Accepted resource states", func() {
		// range through all possible resource states
		for resourceStatusString, resourceStatusInt := range core.Status_State_value {
			resourceStatusState := core.Status_State(resourceStatusInt)
			// check all values other than accepted and pending
			if resourceStatusString != core.Status_Accepted.String() && resourceStatusString != core.Status_Pending.String() {
				By(fmt.Sprintf("resource: %v", resourceStatusString))
				vs := &v1.VirtualService{}
				statusClient.SetStatus(vs, &core.Status{
					State:      resourceStatusState,
					ReportedBy: "gloo",
				})
				Expect(getAggregateVirtualServiceStatus(ctx, vs, namespace)).To(Equal(resourceStatusString))
			}
		}
	})

	It("handles non-Pending and non-Accepted state - sub resources accepted", func() {
		// range through all possible resource states
		for resourceStatusString, resourceStatusInt := range core.Status_State_value {
			resourceStatusState := core.Status_State(resourceStatusInt)
			// check all values other than accepted and pending
			if resourceStatusString != core.Status_Accepted.String() && resourceStatusString != core.Status_Pending.String() {
				By(fmt.Sprintf("resource: %v, one subresource accepted", resourceStatusString))
				subStatuses := map[string]*core.Status{
					thing1: {
						State: core.Status_Accepted,
					},
				}
				vs := &v1.VirtualService{}
				statusClient.SetStatus(vs, &core.Status{
					State:               resourceStatusState,
					SubresourceStatuses: subStatuses,
					ReportedBy:          "gloo",
				})
				Expect(getAggregateVirtualServiceStatus(ctx, vs, namespace)).To(Equal(resourceStatusString))

				By(fmt.Sprintf("resource: %v, two subresources accepted", resourceStatusString))
				subStatuses = map[string]*core.Status{
					thing1: {
						State: core.Status_Accepted,
					},
					thing2: {
						State: core.Status_Accepted,
					},
				}
				namespacedStatus := statusClient.GetStatus(vs)
				namespacedStatus.SubresourceStatuses = subStatuses
				Expect(getAggregateVirtualServiceStatus(ctx, vs, namespace)).To(Equal(resourceStatusString))
			}
		}
	})

	It("handles non-Pending and non-Accepted state - sub resources rejected", func() {
		reasonUntracked := "some reason that does not match a known criteria"
		// range through all possible resource states
		for resourceStatusString, resourceStatusInt := range core.Status_State_value {
			resourceStatusState := core.Status_State(resourceStatusInt)
			// check all values other than accepted and pending
			if resourceStatusString != core.Status_Accepted.String() && resourceStatusString != core.Status_Pending.String() {
				By(fmt.Sprintf("resource: %v, one subresource rejected", resourceStatusString))
				subStatuses := map[string]*core.Status{
					thing1: {
						State:  core.Status_Rejected,
						Reason: reasonUntracked,
					},
				}
				vs := &v1.VirtualService{}
				statusClient.SetStatus(vs, &core.Status{
					State:               resourceStatusState,
					SubresourceStatuses: subStatuses,
					ReportedBy:          "gloo",
				})
				out := getAggregateVirtualServiceStatus(ctx, vs, namespace)
				Expect(out).To(Equal(resourceStatusString + "\n" + genericErrorFormat(thing1, core.Status_Rejected.String(), reasonUntracked)))

				By(fmt.Sprintf("resource: %v, two subresources rejected", resourceStatusString))
				subStatuses = map[string]*core.Status{
					thing1: {
						State:  core.Status_Rejected,
						Reason: reasonUntracked,
					},
					thing2: {
						State:  core.Status_Rejected,
						Reason: reasonUntracked,
					},
				}
				namespacedStatus := statusClient.GetStatus(vs)
				namespacedStatus.SubresourceStatuses = subStatuses
				out = getAggregateVirtualServiceStatus(ctx, vs, namespace)
				Expect(out).To(HavePrefix(resourceStatusString + "\n"))
				// Use regex because order does not matter
				Expect(out).To(MatchRegexp(genericErrorFormat(thing1, core.Status_Rejected.String(), reasonUntracked)))
				Expect(out).To(MatchRegexp(genericErrorFormat(thing2, core.Status_Rejected.String(), reasonUntracked)))
			}
		}
	})

	It("handles non-Pending and non-Accepted state - sub resources errored in known way", func() {
		erroredResourceIdentifier := "some_errored_resource_id"
		reasonUpstreamList := fmt.Sprintf("%v: %v", strings.TrimSpace(gloov1.UpstreamListErrorTag), erroredResourceIdentifier)
		// range through all possible resource states
		for resourceStatusString, resourceStatusInt := range core.Status_State_value {
			resourceStatusState := core.Status_State(resourceStatusInt)
			// check all values other than accepted and pending
			if resourceStatusString != core.Status_Accepted.String() && resourceStatusString != core.Status_Pending.String() {
				By(fmt.Sprintf("resource: %v, one subresource rejected", resourceStatusString))
				subStatuses := map[string]*core.Status{
					thing1: {
						State:  core.Status_Rejected,
						Reason: reasonUpstreamList,
					},
				}
				vs := &v1.VirtualService{}
				statusClient.SetStatus(vs, &core.Status{
					State:               resourceStatusState,
					SubresourceStatuses: subStatuses,
					ReportedBy:          "gloo",
				})
				out := getAggregateVirtualServiceStatus(ctx, vs, namespace)
				Expect(out).To(Equal(resourceStatusString + "\n" + subResourceErrorFormat(erroredResourceIdentifier)))

				By(fmt.Sprintf("resource: %v, one subresource accepted and one rejected", resourceStatusString))
				subStatuses = map[string]*core.Status{
					thing1: {
						State:  core.Status_Rejected,
						Reason: reasonUpstreamList,
					},
					thing2: {
						State: core.Status_Accepted,
					},
				}
				namespacedStatus := statusClient.GetStatus(vs)
				namespacedStatus.SubresourceStatuses = subStatuses
				out = getAggregateVirtualServiceStatus(ctx, vs, namespace)
				Expect(out).To(HavePrefix(resourceStatusString + "\n"))
				Expect(out).To(MatchRegexp(reasonUpstreamList))

				By(fmt.Sprintf("resource: %v, two subresources rejected", resourceStatusString))
				subStatuses = map[string]*core.Status{
					thing1: {
						State:  core.Status_Rejected,
						Reason: reasonUpstreamList,
					},
					thing2: {
						State:  core.Status_Rejected,
						Reason: reasonUpstreamList,
					},
				}
				namespacedStatus = statusClient.GetStatus(vs)
				namespacedStatus.SubresourceStatuses = subStatuses
				out = getAggregateVirtualServiceStatus(ctx, vs, namespace)
				Expect(out).To(HavePrefix(resourceStatusString + "\n"))
				// Use regex because order does not matter
				Expect(out).To(MatchRegexp(genericErrorFormat(thing1, core.Status_Rejected.String(), reasonUpstreamList)))
				Expect(out).To(MatchRegexp(genericErrorFormat(thing2, core.Status_Rejected.String(), reasonUpstreamList)))
			}
		}
	})
})
