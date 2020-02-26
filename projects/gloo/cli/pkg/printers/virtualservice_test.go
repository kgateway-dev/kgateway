package printers

import (
	"fmt"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
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
		namespace = "gloo-system"
	)
	It("handles Pending resource state", func() {
		vs := &v1.VirtualService{
			Status: core.Status{
				State: core.Status_Pending,
			},
		}
		Expect(getStatus(vs, namespace)).To(Equal(core.Status_Pending.String()))

		// range through all possible sub resource states
		for subResourceStatusString, subResourceStatusInt := range core.Status_State_value {
			subResourceStatusState := core.Status_State(subResourceStatusInt)
			vs.Status.SubresourceStatuses = map[string]*core.Status{
				thing1: {
					State:  subResourceStatusState,
					Reason: "any reason",
				},
			}
			By(fmt.Sprintf("subresource: %v", subResourceStatusString))
			Expect(getStatus(vs, namespace)).To(Equal(core.Status_Pending.String()))
		}
	})
	It("handles Accepted resource state", func() {
		vs := &v1.VirtualService{
			Status: core.Status{
				State: core.Status_Accepted,
			},
		}
		Expect(getStatus(vs, namespace)).To(Equal(core.Status_Accepted.String()))

		// get settings
		settingsClient := helpers.MustSettingsClient()
		settings, _ := settingsClient.Read(namespace, defaults.SettingsName, clients.ReadOpts{})
		replaceInvalidRoutes := settings.Gloo.InvalidConfigPolicy.ReplaceInvalidRoutes

		// range through all possible sub resource states
		for subResourceStatusString, subResourceStatusInt := range core.Status_State_value {
			subResourceStatusState := core.Status_State(subResourceStatusInt)
			By(fmt.Sprintf("subresource: %v", subResourceStatusString))
			vs.Status.SubresourceStatuses = map[string]*core.Status{
				thing1: {
					State:  subResourceStatusState,
					Reason: "any reason",
				},
			}

			// check route replacement off case
			settings.Gloo.InvalidConfigPolicy.ReplaceInvalidRoutes = false
			settingsClient.Write(settings, clients.WriteOpts{OverwriteExisting: true})

			if subResourceStatusString == core.Status_Accepted.String() {
				Expect(getStatus(vs, namespace)).To(Equal(core.Status_Accepted.String()))
			} else {
				Expect(getStatus(vs, namespace)).To(Equal(core.Status_Accepted.String() + "\n" + genericSubResourceMessage(thing1, subResourceStatusString)))
			}

			// check route replacement on case
			settings.Gloo.InvalidConfigPolicy.ReplaceInvalidRoutes = true
			settingsClient.Write(settings, clients.WriteOpts{OverwriteExisting: true})

			if subResourceStatusString == core.Status_Accepted.String() {
				Expect(getStatus(vs, namespace)).To(Equal(core.Status_Accepted.String()))
			}
		}

		// return replaceInvalidRoutes value in settings to its original value
		settings.Gloo.InvalidConfigPolicy.ReplaceInvalidRoutes = replaceInvalidRoutes
		settingsClient.Write(settings, clients.WriteOpts{OverwriteExisting: true})
	})
	It("handles simple non-Pending and non-Accepted resource states", func() {
		// range through all possible resource states
		for resourceStatusString, resourceStatusInt := range core.Status_State_value {
			resourceStatusState := core.Status_State(resourceStatusInt)
			// check all values other than accepted and pending
			if resourceStatusString != core.Status_Accepted.String() && resourceStatusString != core.Status_Pending.String() {
				By(fmt.Sprintf("resource: %v", resourceStatusString))
				vs := &v1.VirtualService{
					Status: core.Status{
						State: resourceStatusState,
					},
				}
				Expect(getStatus(vs, namespace)).To(Equal(resourceStatusString))
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
				vs := &v1.VirtualService{
					Status: core.Status{
						State:               resourceStatusState,
						SubresourceStatuses: subStatuses,
					},
				}
				Expect(getStatus(vs, namespace)).To(Equal(resourceStatusString))

				By(fmt.Sprintf("resource: %v, two subresources accepted", resourceStatusString))
				subStatuses = map[string]*core.Status{
					thing1: {
						State: core.Status_Accepted,
					},
					thing2: {
						State: core.Status_Accepted,
					},
				}
				vs.Status.SubresourceStatuses = subStatuses
				Expect(getStatus(vs, namespace)).To(Equal(resourceStatusString))
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
				vs := &v1.VirtualService{
					Status: core.Status{
						State:               resourceStatusState,
						SubresourceStatuses: subStatuses,
					},
				}
				out := getStatus(vs, namespace)
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
				vs.Status.SubresourceStatuses = subStatuses
				out = getStatus(vs, namespace)
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
				vs := &v1.VirtualService{
					Status: core.Status{
						State:               resourceStatusState,
						SubresourceStatuses: subStatuses,
					},
				}
				out := getStatus(vs, namespace)
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
				vs.Status.SubresourceStatuses = subStatuses
				out = getStatus(vs, namespace)
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
				vs.Status.SubresourceStatuses = subStatuses
				out = getStatus(vs, namespace)
				Expect(out).To(HavePrefix(resourceStatusString + "\n"))
				// Use regex because order does not matter
				Expect(out).To(MatchRegexp(genericErrorFormat(thing1, core.Status_Rejected.String(), reasonUpstreamList)))
				Expect(out).To(MatchRegexp(genericErrorFormat(thing2, core.Status_Rejected.String(), reasonUpstreamList)))
			}
		}
	})
})
