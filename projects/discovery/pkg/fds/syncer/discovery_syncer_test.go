package syncer

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/kubernetes"
	"github.com/solo-io/solo-kit/api/external/kubernetes/namespace"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var disabledLabels = map[string]string{FdsLabelKey: disbledLabelValue}
var enabledLabels = map[string]string{FdsLabelKey: enbledLabelValue}
var _ = Describe("filterUpstreamsForDiscovery", func() {
	disabledNs := &kubernetes.KubeNamespace{KubeNamespace: namespace.KubeNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "disabled-ns",
			Labels: disabledLabels,
		},
	}}
	enabledNs := &kubernetes.KubeNamespace{KubeNamespace: namespace.KubeNamespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "enabled-ns",
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
	nsList := kubernetes.KubeNamespaceList{disabledNs, enabledNs, enabledKubeSystemNs}

	disabledUs1 := makeUpstream("a", disabledNs.Name, nil)
	disabledUs2 := makeUpstream("b", enabledNs.Name, disabledLabels)
	disabledUs3 := makeUpstream("c", disabledKubePublicNs.Name, nil)
	enabledUs1 := makeUpstream("d", enabledNs.Name, nil)
	enabledUs2 := makeUpstream("e", enabledKubeSystemNs.Name, nil)

	usList := gloov1.UpstreamList{disabledUs1, disabledUs2, disabledUs3, enabledUs1, enabledUs2}

	filtered := filterUpstreamsForDiscovery(usList, nsList)

	It("excludes upstreams whose namespace has the disabled label", func() {
		Expect(filtered).NotTo(ContainElement(disabledUs1))
	})
	It("excludes upstreams who have the disabled label", func() {
		Expect(filtered).NotTo(ContainElement(disabledUs2))
	})
	It("excludes upstreams whose namespace is kube-system", func() {
		Expect(filtered).NotTo(ContainElement(disabledUs3))
	})
	It("includes upstreams in enabled namespaces", func() {
		Expect(filtered).To(ContainElement(enabledUs1))
	})
	It("includes upstreams in enabled kube-system when enabled", func() {
		Expect(filtered).To(ContainElement(enabledUs2))
	})
})

func makeUpstream(name, namespace string, labels map[string]string) *gloov1.Upstream {
	us := gloov1.NewUpstream("gloo-system", name)
	us.UpstreamSpec = &gloov1.UpstreamSpec{UpstreamType: &gloov1.UpstreamSpec_Kube{
		Kube: &kubeplugin.UpstreamSpec{ServiceNamespace: namespace},
	}}
	us.Metadata.Labels = labels
	return us
}
