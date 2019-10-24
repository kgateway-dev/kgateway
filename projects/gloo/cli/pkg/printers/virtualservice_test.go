package printers

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("getStatus", func() {
	var (
		thing1 = "thing1"
		thing2 = "thing2"
	)
	It("handles non-Accepted resource state", func() {
		// range through all possible resource states
		for resourceStatusString, resourceStatusInt := range core.Status_State_value {
			resourceStatusState := core.Status_State(resourceStatusInt)
			// check all values other than accepted
			if resourceStatusString != core.Status_Accepted.String() {
				By(fmt.Sprintf("resource: %v, subresource: %v", resourceStatusString, "not present"))
				vs := &v1.VirtualService{
					Status: core.Status{
						State: resourceStatusState,
					},
				}
				Expect(getStatus(vs)).To(Equal(resourceStatusString))

				// range through all possible sub resource states
				for subResourceStatusString, subResourceStatusInt := range core.Status_State_value {
					subResourceStatusState := core.Status_State(subResourceStatusInt)
					vs.Status.SubresourceStatuses = map[string]*core.Status{
						thing1: {
							State:  subResourceStatusState,
							Reason: "any reason",
						},
					}
					By(fmt.Sprintf("resource: %v, subresource: %v", resourceStatusString, subResourceStatusString))
					Expect(getStatus(vs)).To(Equal(resourceStatusString))
				}
			}
		}
	})
	It("handles simple Accepted state", func() {
		vs := &v1.VirtualService{
			Status: core.Status{
				State: core.Status_Accepted,
			},
		}
		Expect(getStatus(vs)).To(Equal(core.Status_Accepted.String()))
	})
	It("handles Accepted state - sub resources accepted", func() {
		By("one accepted")
		subStatuses := map[string]*core.Status{
			thing1: {
				State: core.Status_Accepted,
			},
		}
		vs := &v1.VirtualService{
			Status: core.Status{
				State:               core.Status_Accepted,
				SubresourceStatuses: subStatuses,
			},
		}
		Expect(getStatus(vs)).To(Equal(core.Status_Accepted.String()))

		By("two accepted")
		subStatuses = map[string]*core.Status{
			thing1: {
				State: core.Status_Accepted,
			},
			thing2: {
				State: core.Status_Accepted,
			},
		}
		vs.Status.SubresourceStatuses = subStatuses
		Expect(getStatus(vs)).To(Equal(core.Status_Accepted.String()))
	})
	It("handles Accepted state - sub resources rejected", func() {
		reasonUntracked := "some reason that does not match a known criteria"
		By("one rejected")
		subStatuses := map[string]*core.Status{
			thing1: {
				State:  core.Status_Rejected,
				Reason: reasonUntracked,
			},
		}
		vs := &v1.VirtualService{
			Status: core.Status{
				State:               core.Status_Accepted,
				SubresourceStatuses: subStatuses,
			},
		}
		out := getStatus(vs)
		Expect(out).To(Equal(genericErrorFormat(thing1, core.Status_Rejected.String(), reasonUntracked)))

		By("two rejected")
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
		vs.Status.SubresourceStatuses = subStatuses
		out = getStatus(vs)
		// Use regex because order does not matter
		Expect(out).To(MatchRegexp(genericErrorFormat(thing1, core.Status_Rejected.String(), reasonUntracked)))
		Expect(out).To(MatchRegexp(genericErrorFormat(thing2, core.Status_Rejected.String(), reasonUntracked)))
	})

	It("handles Accepted state - sub resources errored in known way", func() {
		erroredResourceIdentifier := "some_errored_resource_id"
		reasonUpstreamList := fmt.Sprintf("%v: %v", strings.TrimSpace(gloov1.UpstreamListErrorTag), erroredResourceIdentifier)
		By("one rejected")
		subStatuses := map[string]*core.Status{
			thing1: {
				State:  core.Status_Rejected,
				Reason: reasonUpstreamList,
			},
		}
		vs := &v1.VirtualService{
			Status: core.Status{
				State:               core.Status_Accepted,
				SubresourceStatuses: subStatuses,
			},
		}
		out := getStatus(vs)
		Expect(out).To(Equal(subResourceErrorFormat(erroredResourceIdentifier)))

		By("one accepted, one rejected")
		subStatuses = map[string]*core.Status{
			thing1: {
				State:  core.Status_Rejected,
				Reason: reasonUpstreamList,
			},
			thing2: {
				State: core.Status_Accepted,
			},
		}
		vs.Status.SubresourceStatuses = subStatuses
		out = getStatus(vs)
		Expect(out).To(MatchRegexp(reasonUpstreamList))

		By("two rejected")
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
		vs.Status.SubresourceStatuses = subStatuses
		out = getStatus(vs)
		// Use regex because order does not matter
		Expect(out).To(MatchRegexp(genericErrorFormat(thing1, core.Status_Rejected.String(), reasonUpstreamList)))
		Expect(out).To(MatchRegexp(genericErrorFormat(thing2, core.Status_Rejected.String(), reasonUpstreamList)))
	})
})
