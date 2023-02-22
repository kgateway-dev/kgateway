package syncer

import (
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/solo-kit/api/external/kubernetes/namespace"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/projects/discovery/pkg/fds"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
)

var disabledLabels = map[string]string{FdsLabelKey: disabledLabelValue}
var enabledLabels = map[string]string{FdsLabelKey: enabledLabelValue}
var _ = Describe("selectUpstreamsForDiscovery", func() {
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

	disabledUs1 := makeKubeUpstream("disabledUs1", disabledNs.Name, nil)
	disabledUs2 := makeKubeUpstream("disabledUs2", enabledNs.Name, disabledLabels)
	disabledUs3 := makeKubeUpstream("disabledUs3", disabledKubePublicNs.Name, nil)
	disabledAwsUs1 := makeAwsUpstream("disabledAwsUs1", disabledNs.Name, nil)
	disabledAwsUs2 := makeAwsUpstream("disabledAwsUs2", enabledNs.Name, disabledLabels)
	enabledUs1 := makeKubeUpstream("enabledUs1", enabledNs.Name, nil)
	enabledUs2 := makeKubeUpstream("enabledUs2", enabledKubeSystemNs.Name, nil)
	enabledAwsUs1 := makeAwsUpstream("enabledAwsUs1", enabledNs.Name, nil)
	enabledAwsUs2 := makeAwsUpstream("enabledAwsUs2", disabledNs.Name, enabledLabels)
	enabledAwsUs3 := makeAwsUpstream("enabledAwsUs3", "other-namespace", enabledLabels)
	explicitlyEnabledUs1 := makeKubeUpstream("explicitlyEnabledUs1", explicitlyEnabledNs.Name, nil)
	explicitlyEnabledUs2 := makeKubeUpstream("explicitlyEnabledUs2", enabledNs.Name, enabledLabels)

	usList := gloov1.UpstreamList{disabledUs1, disabledUs2, disabledUs3, enabledUs1, enabledUs2, explicitlyEnabledUs1, explicitlyEnabledUs2, disabledAwsUs1, enabledAwsUs3, disabledAwsUs2, enabledAwsUs1, enabledAwsUs2}

	var filtered gloov1.UpstreamList

	Context("blacklist mode", func() {
		BeforeEach(func() {
			filtered = selectUpstreamsForDiscovery(gloov1.Settings_DiscoveryOptions_BLACKLIST, usList, nsList)
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
			filtered = selectUpstreamsForDiscovery(gloov1.Settings_DiscoveryOptions_WHITELIST, usList, nsList)
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
		It("includes AWS upstreams as if they were in blacklist mode", func() {
			Expect(filtered).To(ContainElement(enabledAwsUs1))
			Expect(filtered).To(ContainElement(enabledAwsUs2))
			Expect(filtered).To(ContainElement(enabledAwsUs3))
			Expect(filtered).NotTo(ContainElement(disabledAwsUs1))
			Expect(filtered).NotTo(ContainElement(disabledAwsUs2))
		})
	})

	Context("RunFDS", func() {
		var opts bootstrap.Opts
		BeforeEach(func() {
			opts = bootstrap.Opts{
				Settings: &gloov1.Settings{
					Metadata: &core.Metadata{
						Name:      "test-settings",
						Namespace: "gloo-system",
					},
					Discovery: &gloov1.Settings_DiscoveryOptions{
						UdsOptions: &gloov1.Settings_DiscoveryOptions_UdsOptions{
							Enabled: &wrappers.BoolValue{Value: false},
						},
					},
				},
			}
		})
		It("returns an error when both UDS and FDS are disabled", func() {
			opts.Settings.GetDiscovery().FdsMode = gloov1.Settings_DiscoveryOptions_DISABLED
			Expect(RunFDS(opts)).To(HaveOccurred())
		})
		It("excludes nil discovery factories from the array", func() {
			discoveryFactoryFunc := func(opts bootstrap.Opts) fds.FunctionDiscoveryFactory {
				return nil
			}
			discoveryFactorySliceFunc := func(opts bootstrap.Opts) []fds.FunctionDiscoveryFactory {
				return nil
			}
			Expect(GetFunctionDiscoveriesWithExtensionsAndRegistry(opts, discoveryFactorySliceFunc, Extensions{
				DiscoveryFactoryFuncs: []func(opts bootstrap.Opts) fds.FunctionDiscoveryFactory{discoveryFactoryFunc},
			})).To(HaveLen(0))

		})
	})
})

func makeKubeUpstream(name, namespace string, labels map[string]string) *gloov1.Upstream {
	us := gloov1.NewUpstream("gloo-system", name)
	us.UpstreamType = &gloov1.Upstream_Kube{
		Kube: &kubeplugin.UpstreamSpec{ServiceNamespace: namespace},
	}
	us.DiscoveryMetadata = &gloov1.DiscoveryMetadata{
		Labels: labels,
	}
	return us
}

func makeAwsUpstream(name, namespace string, labels map[string]string) *gloov1.Upstream {
	us := gloov1.NewUpstream(namespace, name)
	us.UpstreamType = &gloov1.Upstream_Aws{
		Aws: &aws.UpstreamSpec{
			Region: "test-region",
		},
	}
	us.DiscoveryMetadata = &gloov1.DiscoveryMetadata{
		Labels: labels,
	}
	return us
}
