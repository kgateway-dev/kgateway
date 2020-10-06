package consul

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Conversions", func() {

	It("correctly generates the name for the fake upstream", func() {
		Expect(fakeUpstreamName("my-consul-service")).To(Equal(UpstreamNamePrefix + "my-consul-service"))
	})

	It("correctly detects upstreams derived from Kubernetes services", func() {
		Expect(IsConsulUpstream(UpstreamNamePrefix + "my-service")).To(BeTrue())
		Expect(IsConsulUpstream("my-" + UpstreamNamePrefix + "service")).To(BeFalse())
		Expect(IsConsulUpstream("consul:my-service-8080")).To(BeFalse())
	})

	It("correctly converts a list of services to upstreams", func() {
		servicesWithDataCenters := []*ServiceMeta{
			{Name: "svc-1", DataCenters: []string{"dc1", "dc2"}},
			{Name: "svc-2", DataCenters: []string{"dc1", "dc3", "dc4"}},
		}

		usList := toUpstreamList(defaults.GlooSystem, servicesWithDataCenters, nil)
		usList.Sort()

		Expect(usList).To(HaveLen(2))

		Expect(usList[0].Metadata.Name).To(Equal(UpstreamNamePrefix + "svc-1"))
		Expect(usList[0].Metadata.Namespace).To(Equal(defaults.GlooSystem))
		Expect(usList[0].GetConsul()).NotTo(BeNil())
		Expect(usList[0].GetConsul().ServiceName).To(Equal("svc-1"))
		Expect(usList[0].GetConsul().DataCenters).To(ConsistOf("dc1", "dc2"))

		Expect(usList[1].Metadata.Name).To(Equal(UpstreamNamePrefix + "svc-2"))
		Expect(usList[1].Metadata.Namespace).To(Equal(defaults.GlooSystem))
		Expect(usList[1].GetConsul()).NotTo(BeNil())
		Expect(usList[1].GetConsul().ServiceName).To(Equal("svc-2"))
		Expect(usList[1].GetConsul().DataCenters).To(ConsistOf("dc1", "dc3", "dc4"))
	})

	It("adds TLS to upstreams that have the TLS tag", func() {
		servicesWithDataCenters := []*ServiceMeta{
			{Name: "svc-1", DataCenters: []string{"dc1", "dc2"}, Tags: []string{"glooUseTls"}},
		}

		usList := toUpstreamList(defaults.GlooSystem, servicesWithDataCenters, &v1.Settings_ConsulUpstreamDiscoveryConfiguration{
			TlsTagName:       "glooUseTls",
			UseTlsTagging:    true,
			SplitTlsServices: false,
			RootCa: &core.ResourceRef{
				Namespace: "rootNs",
				Name:      "rootName",
			},
		})
		usList.Sort()

		Expect(usList).To(HaveLen(1))

		Expect(usList[0].Metadata.Name).To(Equal(UpstreamNamePrefix + "svc-1-tls"))
		Expect(usList[0].Metadata.Namespace).To(Equal(defaults.GlooSystem))
		Expect(usList[0].GetConsul()).NotTo(BeNil())
		Expect(usList[0].GetConsul().ServiceName).To(Equal("svc-1"))
		Expect(usList[0].GetConsul().DataCenters).To(ConsistOf("dc1", "dc2"))
		Expect(usList[0].GetConsul().InstanceTags).To(BeEmpty())
		Expect(usList[0].GetConsul().InstanceBlacklistTags).To(BeEmpty())
		Expect(usList[0].GetSslConfig()).NotTo(BeNil())
		Expect(usList[0].GetSslConfig().GetSslSecrets()).NotTo(BeNil())
		Expect(usList[0].GetSslConfig().GetSecretRef().Namespace).To(Equal("rootNs"))
		Expect(usList[0].GetSslConfig().GetSecretRef().Name).To(Equal("rootName"))
	})

	It("splits upstreams that have the TLS tag when service-splitting is on", func() {
		servicesWithDataCenters := []*ServiceMeta{
			{Name: "svc-1", DataCenters: []string{"dc1", "dc2"}, Tags: []string{"glooUseTls"}},
		}

		usList := toUpstreamList(defaults.GlooSystem, servicesWithDataCenters, &v1.Settings_ConsulUpstreamDiscoveryConfiguration{
			TlsTagName:       "glooUseTls",
			UseTlsTagging:    true,
			SplitTlsServices: true,
			RootCa: &core.ResourceRef{
				Namespace: "rootNs",
				Name:      "rootName",
			},
		})
		usList.Sort()

		Expect(usList).To(HaveLen(2))

		Expect(usList[0].Metadata.Name).To(Equal(UpstreamNamePrefix + "svc-1"))
		Expect(usList[0].Metadata.Namespace).To(Equal(defaults.GlooSystem))
		Expect(usList[0].GetConsul()).NotTo(BeNil())
		Expect(usList[0].GetConsul().ServiceName).To(Equal("svc-1"))
		Expect(usList[0].GetConsul().DataCenters).To(ConsistOf("dc1", "dc2"))
		Expect(usList[0].GetConsul().InstanceBlacklistTags).To(ConsistOf("glooUseTls"))
		Expect(usList[0].GetConsul().InstanceTags).To(BeNil())
		Expect(usList[0].GetSslConfig()).To(BeNil())

		Expect(usList[1].Metadata.Name).To(Equal(UpstreamNamePrefix + "svc-1-tls"))
		Expect(usList[1].Metadata.Namespace).To(Equal(defaults.GlooSystem))
		Expect(usList[1].GetConsul()).NotTo(BeNil())
		Expect(usList[1].GetConsul().ServiceName).To(Equal("svc-1"))
		Expect(usList[1].GetConsul().DataCenters).To(ConsistOf("dc1", "dc2"))
		Expect(usList[1].GetConsul().InstanceTags).To(ConsistOf("glooUseTls"))
		Expect(usList[1].GetConsul().InstanceBlacklistTags).To(BeNil())
		Expect(usList[1].GetSslConfig()).NotTo(BeNil())
	})

	It("correctly consolidates service information from different data centers", func() {
		input := []*dataCenterServicesTuple{
			{
				dataCenter: "dc-1",
				services: map[string][]string{
					"svc-1": {"tag-1", "tag-2"},
					"svc-2": {"tag-2"},
					"svc-3": {},
				},
			},
			{
				dataCenter: "dc-2",
				services: map[string][]string{
					"svc-1": {"tag-3"},
					"svc-2": {},
					"svc-4": nil,
				},
			},
		}

		result := toServiceMetaSlice(input)

		Expect(result).To(ConsistOf(
			[]*ServiceMeta{
				{
					Name:        "svc-1",
					DataCenters: []string{"dc-1", "dc-2"},
					Tags:        []string{"tag-1", "tag-2", "tag-3"},
				},
				{
					Name:        "svc-2",
					DataCenters: []string{"dc-1", "dc-2"},
					Tags:        []string{"tag-2"},
				},
				{
					Name:        "svc-3",
					DataCenters: []string{"dc-1"},
					Tags:        nil,
				},
				{
					Name:        "svc-4",
					DataCenters: []string{"dc-2"},
					Tags:        nil,
				},
			},
		))

	})
})
