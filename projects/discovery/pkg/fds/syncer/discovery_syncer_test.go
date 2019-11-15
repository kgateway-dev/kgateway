package syncer

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/solo-kit/api/external/kubernetes/namespace"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var disabledLabels = map[string]string{FdsLabelKey: disbledLabelValue}
var enabledLabels = map[string]string{FdsLabelKey: enbledLabelValue}
var _ = Describe("filterUpstreamsForDiscovery", func() {
	disabledNs := &kubernetes.KubeNamespace{KubeNamespace: namespace.KubeNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "explicitly-disabled-ns",
			Labels: disabledLabels,
		},
	}}
	enabledNs := &kubernetes.KubeNamespace{KubeNamespace: namespace.KubeNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "implicitly-enabled-ns",
		},
	}}
	enabledKubeSystemNs := &kubernetes.KubeNamespace{KubeNamespace: namespace.KubeNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "kube-system",
			Labels: enabledLabels,
		},
	}}
	disabledKubePublicNs := &kubernetes.KubeNamespace{KubeNamespace: namespace.KubeNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-public",
		},
	}}
	explicitlyEnabledNs := &kubernetes.KubeNamespace{KubeNamespace: namespace.KubeNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "explicitly-enabled",
			Labels: enabledLabels,
		},
	}}
	nsList := kubernetes.KubeNamespaceList{disabledNs, enabledNs, enabledKubeSystemNs, disabledKubePublicNs, explicitlyEnabledNs}

	disabledUs1 := makeUpstream("disabledUs1", disabledNs.Name, nil)
	disabledUs2 := makeUpstream("disabledUs2", enabledNs.Name, disabledLabels)
	disabledUs3 := makeUpstream("disabledUs3", disabledKubePublicNs.Name, nil)
	enabledUs1 := makeUpstream("enabledUs1", enabledNs.Name, nil)
	enabledUs2 := makeUpstream("enabledUs2", enabledKubeSystemNs.Name, nil)
	explicitlyEnabledUs1 := makeUpstream("explicitlyEnabledUs1", explicitlyEnabledNs.Name, nil)
	explicitlyEnabledUs2 := makeUpstream("explicitlyEnabledUs2", enabledNs.Name, enabledLabels)

	usList := gloov1.UpstreamList{disabledUs1, disabledUs2, disabledUs3, enabledUs1, enabledUs2, explicitlyEnabledUs1, explicitlyEnabledUs2}

	var filtered gloov1.UpstreamList

	Context("blacklist mode", func() {
		BeforeEach(func() {
			filtered = filterUpstreamsForDiscovery(gloov1.Settings_DiscoveryOptions_BLACKLIST, usList, nsList)
		})

		It("excludes upstreams whose namespace has the disabled label", func() {
			Expect(filtered).NotTo(ContainElement(disabledUs1))
		})
		It("excludes upstreams who have the disabled label", func() {
			Expect(filtered).NotTo(ContainElement(disabledUs2))
		})
		It("excludes upstreams whose namespace is kube-system", func() {
			Expect(filtered).NotTo(ContainElement(disabledUs3))
		})
		It("includes upstreams in namespaces without disabled label", func() {
			Expect(filtered).To(ContainElement(enabledUs1))
			Expect(filtered).To(ContainElement(explicitlyEnabledUs2))
		})
		It("includes upstreams in namespaces with enabled label", func() {
			Expect(filtered).To(ContainElement(explicitlyEnabledUs1))
		})
		It("includes upstreams in enabled kube-system when enabled", func() {
			Expect(filtered).To(ContainElement(enabledUs2))
		})
	})

	Context("whitelist mode", func() {
		BeforeEach(func() {
			filtered = filterUpstreamsForDiscovery(gloov1.Settings_DiscoveryOptions_WHITELIST, usList, nsList)
		})

		It("excludes upstreams whose namespace has the disabled label", func() {
			Expect(filtered).NotTo(ContainElement(disabledUs1))
		})
		It("excludes upstreams who have the disabled label", func() {
			Expect(filtered).NotTo(ContainElement(disabledUs2))
		})
		It("excludes upstreams whose namespace is kube-system", func() {
			Expect(filtered).NotTo(ContainElement(disabledUs3))
		})
		It("excludes upstreams in namespaces without disabled label", func() {
			Expect(filtered).NotTo(ContainElement(enabledUs1))
		})
		It("includes explicitly enabled upstreams", func() {
			Expect(filtered).To(ContainElement(enabledUs2))
		})
		It("includes upstreams from explicitly enabled namespaces", func() {
			Expect(filtered).To(ContainElement(enabledUs2))
		})
		It("includes upstreams in namespaces with enabled label", func() {
			Expect(filtered).To(ContainElement(explicitlyEnabledUs1))
			Expect(filtered).To(ContainElement(explicitlyEnabledUs2))
		})
	})
})

func makeUpstream(name, namespace string, labels map[string]string) *gloov1.Upstream {
	us := gloov1.NewUpstream("gloo-system", name)
	us.UpstreamType = &gloov1.Upstream_Kube{
		Kube: &kubeplugin.UpstreamSpec{ServiceNamespace: namespace},
	}
	us.Metadata.Labels = labels
	return us
}
