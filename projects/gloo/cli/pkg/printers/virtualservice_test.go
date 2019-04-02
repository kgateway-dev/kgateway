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

var _ = Describe("getVirtualServiceStatus", func() {
	var (
		thing1 = "thing1"
		thing2 = "thing2"
	)

	It("handles Pending state", func() {
		vs := &v1.VirtualService{
			Status: core.Status{
				State: core.Status_Pending,
			},
		}
		Expect(getVirtualServiceStatus(vs)).To(Equal(core.Status_Pending.String()))
	})
	It("handles Rejected state", func() {
		vs := &v1.VirtualService{
			Status: core.Status{
				State: core.Status_Rejected,
			},
		}
		Expect(getVirtualServiceStatus(vs)).To(Equal(core.Status_Rejected.String()))
	})
	It("handles simple Accepted state", func() {
		vs := &v1.VirtualService{
			Status: core.Status{
				State: core.Status_Accepted,
			},
		}
		Expect(getVirtualServiceStatus(vs)).To(Equal(core.Status_Accepted.String()))
	})
	It("handles Accepted state - sub resources accepted", func() {
		By("one accepted")
		subStatuses := map[string]*core.Status{
			thing1: &core.Status{
				State: core.Status_Accepted,
			},
		}
		vs := &v1.VirtualService{
			Status: core.Status{
				State:               core.Status_Accepted,
				SubresourceStatuses: subStatuses,
			},
		}
		Expect(getVirtualServiceStatus(vs)).To(Equal(core.Status_Accepted.String()))

		By("two accepted")
		subStatuses = map[string]*core.Status{
			thing1: &core.Status{
				State: core.Status_Accepted,
			},
			thing2: &core.Status{
				State: core.Status_Accepted,
			},
		}
		vs.Status.SubresourceStatuses = subStatuses
		Expect(getVirtualServiceStatus(vs)).To(Equal(core.Status_Accepted.String()))
	})
	It("handles Accepted state - sub resources rejected", func() {
		reasonUntracked := "some reason that does not match a known criteria"
		By("one rejected")
		subStatuses := map[string]*core.Status{
			thing1: &core.Status{
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
		out := getVirtualServiceStatus(vs)
		Expect(out).To(MatchRegexp(thing1))
		Expect(out).To(MatchRegexp(core.Status_Rejected.String()))
		Expect(out).To(MatchRegexp(reasonUntracked))
		Expect(out).To(MatchRegexp(regexStringFromList([]string{
			thing1,
			core.Status_Rejected.String(),
			reasonUntracked})))

		By("two rejected")
		subStatuses = map[string]*core.Status{
			thing1: &core.Status{
				State:  core.Status_Rejected,
				Reason: reasonUntracked,
			},
			thing2: &core.Status{
				State:  core.Status_Rejected,
				Reason: reasonUntracked,
			},
		}
		vs.Status.SubresourceStatuses = subStatuses
		out = getVirtualServiceStatus(vs)
		Expect(joinParagraph(out)).To(MatchRegexp(regexStringFromList([]string{
			thing1,
			core.Status_Rejected.String(),
			reasonUntracked})))
		// Make separate calls because order does not matter
		Expect(joinParagraph(out)).To(MatchRegexp(regexStringFromList([]string{
			thing2,
			core.Status_Rejected.String(),
			reasonUntracked})))
	})

	It("handles Accepted state - sub resources errored in known way", func() {
		erroredResourceIdentifier := "some_errored_resource_id"
		reasonUpstreamList := fmt.Sprintf("%v: %v", gloov1.UpstreamListErrorTag, erroredResourceIdentifier)
		By("one rejected")
		subStatuses := map[string]*core.Status{
			thing1: &core.Status{
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
		out := getVirtualServiceStatus(vs)
		Expect(out).To(MatchRegexp(reasonUpstreamList))

		By("one accepted, one rejected")
		subStatuses = map[string]*core.Status{
			thing1: &core.Status{
				State:  core.Status_Rejected,
				Reason: reasonUpstreamList,
			},
			thing2: &core.Status{
				State: core.Status_Accepted,
			},
		}
		vs.Status.SubresourceStatuses = subStatuses
		out = getVirtualServiceStatus(vs)
		Expect(out).To(MatchRegexp(reasonUpstreamList))

		By("two rejected")
		subStatuses = map[string]*core.Status{
			thing1: &core.Status{
				State:  core.Status_Rejected,
				Reason: reasonUpstreamList,
			},
			thing2: &core.Status{
				State:  core.Status_Rejected,
				Reason: reasonUpstreamList,
			},
		}
		vs.Status.SubresourceStatuses = subStatuses
		out = getVirtualServiceStatus(vs)
		Expect(joinParagraph(out)).To(MatchRegexp(regexStringFromList([]string{
			thing1,
			core.Status_Rejected.String(),
			reasonUpstreamList})))
		// Make separate calls because order does not matter
		Expect(joinParagraph(out)).To(MatchRegexp(regexStringFromList([]string{
			thing2,
			core.Status_Rejected.String(),
			reasonUpstreamList})))
	})
})

// Helpers

func joinParagraph(para string) string {
	return strings.Replace(para, "\n", " ", -1)
}
func regexStringFromList(list []string) string {
	return strings.Join(list, ".*")
}
