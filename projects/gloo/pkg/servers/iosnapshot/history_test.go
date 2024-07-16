package iosnapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway2/controller/scheme"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	crdv1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
	apiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var _ = Describe("History", func() {

	var (
		ctx context.Context

		clientBuilder *fake.ClientBuilder
		xdsCache      cache.SnapshotCache
		history       History
	)

	BeforeEach(func() {
		ctx = context.Background()

		clientBuilder = fake.NewClientBuilder().WithScheme(scheme.NewScheme())
		xdsCache = &xds.MockXdsCache{}
		history = NewHistory(xdsCache)
	})

	Context("GetInputSnapshot", func() {

		It("returns ApiSnapshot without sensitive data", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				Secrets: v1.SecretList{
					{Metadata: &core.Metadata{Name: "secret-east", Namespace: defaults.GlooSystem}},
					{Metadata: &core.Metadata{Name: "secret-west", Namespace: defaults.GlooSystem}},
				},
				Artifacts: v1.ArtifactList{
					{Metadata: &core.Metadata{Name: "artifact-east", Namespace: defaults.GlooSystem}},
					{Metadata: &core.Metadata{Name: "artifact-west", Namespace: defaults.GlooSystem}},
				},
			})

			inputSnapshotBytes, err := history.GetInputSnapshot(ctx)
			Expect(err).NotTo(HaveOccurred())

			returnedResources := []crdv1.Resource{}
			err = json.Unmarshal(inputSnapshotBytes, &returnedResources)
			Expect(err).NotTo(HaveOccurred())

			Expect(containsResourceType(returnedResources, v1.SecretGVK)).To(BeFalse(), "input snapshot should not contain secrets")
			Expect(containsResourceType(returnedResources, v1.ArtifactGVK)).To(BeFalse(), "input snapshot should not contain artifacts")
		})

		It("returns ApiSnapshot without Proxies", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				Proxies: v1.ProxyList{
					{Metadata: &core.Metadata{Name: "proxy-east", Namespace: defaults.GlooSystem}},
					{Metadata: &core.Metadata{Name: "proxy-west", Namespace: defaults.GlooSystem}},
				},
				Upstreams: v1.UpstreamList{
					{Metadata: &core.Metadata{Name: "upstream-east", Namespace: defaults.GlooSystem}},
					{Metadata: &core.Metadata{Name: "upstream-west", Namespace: defaults.GlooSystem}},
				},
			})

			inputSnapshotBytes, err := history.GetInputSnapshot(ctx)
			Expect(err).NotTo(HaveOccurred())

			returnedResources := []crdv1.Resource{}
			err = json.Unmarshal(inputSnapshotBytes, &returnedResources)
			Expect(err).NotTo(HaveOccurred())

			// proxies should not be included in input snapshot
			Expect(containsResourceType(returnedResources, v1.ProxyGVK)).To(BeFalse(), "input snapshot should not contain proxies")

			// upstreams should be included in input snapshot
			Expect(containsResource(returnedResources, v1.UpstreamGVK, defaults.GlooSystem, "upstream-east")).
				To(BeTrue(), fmt.Sprintf("input snapshot should contain upstream %s", "upstream-east"))
			Expect(containsResource(returnedResources, v1.UpstreamGVK, defaults.GlooSystem, "upstream-west")).
				To(BeTrue(), fmt.Sprintf("input snapshot should contain upstream %s", "upstream-west"))
		})

		It("includes Kubernetes Gateway resources in all namespaces", func() {
			clientObjects := []client.Object{
				&apiv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-gw",
						Namespace: "a",
					},
				},
				&apiv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-http-route",
						Namespace: "b",
					},
				},
				&apiv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-gw-class",
						Namespace: "c",
					},
				},
				&apiv1beta1.ReferenceGrant{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-ref-grant",
						Namespace: "d",
					},
				},
			}
			setClientOnHistory(ctx, history, clientBuilder.WithObjects(clientObjects...))

			inputSnapshotBytes, err := history.GetInputSnapshot(ctx)
			Expect(err).NotTo(HaveOccurred())

			returnedResources := []crdv1.Resource{}
			err = json.Unmarshal(inputSnapshotBytes, &returnedResources)
			Expect(err).NotTo(HaveOccurred())

			Expect(containsResource(returnedResources, wellknown.GatewayGVK, "a", "kube-gw")).
				To(BeTrue(), fmt.Sprintf("input snapshot should contain gateway %s.%s", "a", "kube-gw"))
			Expect(containsResource(returnedResources, wellknown.GatewayClassGVK, "c", "kube-gw-class")).
				To(BeTrue(), fmt.Sprintf("input snapshot should contain gatewayclass %s.%s", "c", "kube-gw-class"))
			Expect(containsResource(returnedResources, wellknown.HTTPRouteGVK, "b", "kube-http-route")).
				To(BeTrue(), fmt.Sprintf("input snapshot should contain httproute %s.%s", "b", "kube-http-route"))
			Expect(containsResource(returnedResources, wellknown.ReferenceGrantGVK, "d", "kube-ref-grant")).
				To(BeTrue(), fmt.Sprintf("input snapshot should contain referencegrant %s.%s", "d", "kube-ref-grant"))
		})

	})

	Context("GetProxySnapshot", func() {

		It("returns ApiSnapshot with _only_ Proxies", func() {
			setSnapshotOnHistory(ctx, history, &v1snap.ApiSnapshot{
				Proxies: v1.ProxyList{
					{Metadata: &core.Metadata{Name: "proxy-east", Namespace: defaults.GlooSystem}},
					{Metadata: &core.Metadata{Name: "proxy-west", Namespace: defaults.GlooSystem}},
				},
				Upstreams: v1.UpstreamList{
					{Metadata: &core.Metadata{Name: "upstream-east", Namespace: defaults.GlooSystem}},
					{Metadata: &core.Metadata{Name: "upstream-west", Namespace: defaults.GlooSystem}},
				},
			})

			proxySnapshotBytes, err := history.GetProxySnapshot(ctx)
			Expect(err).NotTo(HaveOccurred())

			returnedResources := []crdv1.Resource{}
			err = json.Unmarshal(proxySnapshotBytes, &returnedResources)
			Expect(err).NotTo(HaveOccurred())

			Expect(containsResource(returnedResources, v1.ProxyGVK, defaults.GlooSystem, "proxy-east")).
				To(BeTrue(), fmt.Sprintf("proxy snapshot should contain proxy %s", "proxy-east"))
			Expect(containsResource(returnedResources, v1.ProxyGVK, defaults.GlooSystem, "proxy-west")).
				To(BeTrue(), fmt.Sprintf("proxy snapshot should contain proxy %s", "proxy-west"))

			Expect(containsResourceType(returnedResources, v1.UpstreamGVK)).To(BeFalse(), "proxy snapshot should not contain upstreams")
		})

	})

})

// setSnapshotOnHistory sets the ApiSnapshot on the history, and blocks until it has been processed
// This is a utility method to help developers write tests, without having to worry about the asynchronous
// nature of the `Set` API on the History
func setSnapshotOnHistory(ctx context.Context, history History, snap *v1snap.ApiSnapshot) {
	snap.Gateways = append(snap.Gateways, &gatewayv1.Gateway{
		// We append a custom Gateway to the Snapshot, and then use that object
		// to verify the Snapshot has been processed
		Metadata: &core.Metadata{Name: "gw-signal", Namespace: defaults.GlooSystem},
	})

	history.SetApiSnapshot(snap)

	eventuallyInputSnapshotContainsResource(ctx, history, gatewayv1.GatewayGVK, defaults.GlooSystem, "gw-signal")
}

// setClientOnHistory sets the Kubernetes Client on the history, and blocks until it has been processed
// This is a utility method to help developers write tests, without having to worry about the asynchronous
// nature of the `Set` API on the History
func setClientOnHistory(ctx context.Context, history History, builder *fake.ClientBuilder) {
	gwSignalObject := &apiv1.Gateway{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-signal",
			Namespace: defaults.GlooSystem,
		},
	}

	history.SetKubeGatewayClient(builder.WithObjects(gwSignalObject).Build())

	eventuallyInputSnapshotContainsResource(ctx, history, wellknown.GatewayGVK, defaults.GlooSystem, "gw-signal")
}

// check that the input snapshot eventually contains a resource with the given gvk, namespace, and name
func eventuallyInputSnapshotContainsResource(
	ctx context.Context,
	history History,
	gvk schema.GroupVersionKind,
	namespace string,
	name string) {
	Eventually(func(g Gomega) {
		inputSnapshotBytes, err := history.GetInputSnapshot(ctx)
		g.Expect(err).NotTo(HaveOccurred())

		returnedResources := []crdv1.Resource{}
		err = json.Unmarshal(inputSnapshotBytes, &returnedResources)
		g.Expect(err).NotTo(HaveOccurred())

		g.Expect(containsResource(returnedResources, gvk, namespace, name)).To(BeTrue())
	}).
		WithPolling(time.Millisecond*100).
		WithTimeout(time.Second*5).
		Should(Succeed(), fmt.Sprintf("snapshot should eventually contain resource %v %s.%s", gvk, namespace, name))
}

// return true if the list of resources contains a resource with the given gvk, namespace, and name
func containsResource(
	resources []crdv1.Resource,
	gvk schema.GroupVersionKind,
	namespace string,
	name string) bool {
	return slices.ContainsFunc(resources, func(res crdv1.Resource) bool {
		return areGvksEqual(res.GroupVersionKind(), gvk) &&
			res.GetName() == name &&
			res.GetNamespace() == namespace
	})
}

// return true if the list of resources contains any resource with the given gvk
func containsResourceType(
	resources []crdv1.Resource,
	gvk schema.GroupVersionKind) bool {
	return slices.ContainsFunc(resources, func(res crdv1.Resource) bool {
		return areGvksEqual(res.GroupVersionKind(), gvk)
	})
}

func areGvksEqual(a, b schema.GroupVersionKind) bool {
	return a.Group == b.Group &&
		a.Version == b.Version &&
		a.Kind == b.Kind
}
