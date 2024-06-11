package httplisteneroptions

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	httplisoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/httplisteneroptions/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	corev1 "github.com/solo-io/skv2/pkg/api/core.skv2.solo.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("HttpListenerOptions Plugin", func() {
	Describe("Attaching HttpListenerOptions via policy attachment", func() {
		var (
			deps   []client.Object
			plugin *plugin
			ctx    context.Context

			listenerCtx     *plugins.ListenerContext
			outputListener  *v1.Listener
			expectedOptions *v1.HttpListenerOptions
		)
		BeforeEach(func() {
			ctx = context.Background()

			listenerCtx = &plugins.ListenerContext{
				Gateway: &gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gw",
						Namespace: "default",
					},
				},
				GwListener: &gwv1.Listener{
					Name: "test-listener",
				},
			}

			outputListener = &v1.Listener{
				ListenerType: &v1.Listener_AggregateListener{
					AggregateListener: &v1.AggregateListener{
						HttpFilterChains: []*v1.AggregateListener_HttpFilterChain{{}},
						HttpResources:    &v1.AggregateListener_HttpResources{},
					},
				},
			}

			expectedOptions = &v1.HttpListenerOptions{
				HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{
					ServerName: wrapperspb.String("unit-test v4.19"),
				},
			}
		})
		JustBeforeEach(func() {
			fakeClient := testutils.BuildIndexedFakeClient(deps, gwquery.IterateIndices, httplisoptquery.IterateIndices)
			gwQueries := testutils.BuildGatewayQueriesWithClient(fakeClient)
			plugin = NewPlugin(gwQueries, fakeClient)
		})

		When("HttpListenerOptions exist in the same namespace and are attached correctly", func() {
			BeforeEach(func() {
				deps = []client.Object{attachedHttpListenerOption()}
			})
			It("correctly adds buffer limit", func() {
				err := plugin.ApplyListenerPlugin(ctx, listenerCtx, outputListener)
				Expect(err).ToNot(HaveOccurred())
				aggListener := outputListener.GetAggregateListener()
				Expect(aggListener).ToNot(BeNil())
				httpOptionsMap := aggListener.GetHttpResources().GetHttpOptions()
				Expect(httpOptionsMap).ToNot(BeNil())
				hfcs := aggListener.GetHttpFilterChains()
				Expect(hfcs).To(HaveLen(1))
				httpOpts := httpOptionsMap[hfcs[0].GetHttpOptionsRef()]
				Expect(proto.Equal(httpOpts, expectedOptions)).To(BeTrue())
			})
		})

		When("HttpListenerOptions exist in the same namespace and are attached correctly with section name", func() {
			BeforeEach(func() {
				deps = []client.Object{attachedHttpListenerOptionWithSectionName()}
			})
			It("correctly adds buffer limit", func() {
				err := plugin.ApplyListenerPlugin(ctx, listenerCtx, outputListener)
				Expect(err).ToNot(HaveOccurred())
				aggListener := outputListener.GetAggregateListener()
				Expect(aggListener).ToNot(BeNil())
				httpOptionsMap := aggListener.GetHttpResources().GetHttpOptions()
				Expect(httpOptionsMap).ToNot(BeNil())
				hfcs := aggListener.GetHttpFilterChains()
				Expect(hfcs).To(HaveLen(1))
				httpOpts := httpOptionsMap[hfcs[0].GetHttpOptionsRef()]
				Expect(proto.Equal(httpOpts, expectedOptions)).To(BeTrue())
			})
		})

		When("HttpListenerOptions exist in the same namespace and are attached correctly but omit the namespace in targetRef", func() {
			BeforeEach(func() {
				deps = []client.Object{attachedHttpListenerOptionOmitNamespace()}
			})
			It("correctly adds buffer limit", func() {
				err := plugin.ApplyListenerPlugin(ctx, listenerCtx, outputListener)
				Expect(err).ToNot(HaveOccurred())
				aggListener := outputListener.GetAggregateListener()
				Expect(aggListener).ToNot(BeNil())
				httpOptionsMap := aggListener.GetHttpResources().GetHttpOptions()
				Expect(httpOptionsMap).ToNot(BeNil())
				hfcs := aggListener.GetHttpFilterChains()
				Expect(hfcs).To(HaveLen(1))
				httpOpts := httpOptionsMap[hfcs[0].GetHttpOptionsRef()]
				Expect(proto.Equal(httpOpts, expectedOptions)).To(BeTrue())
			})
		})

		When("HttpListenerOptions exist in the same namespace but are not attached correctly", func() {
			BeforeEach(func() {
				deps = []client.Object{nonAttachedHttpListenerOption()}
			})
			It("does not add buffer limit", func() {
				err := plugin.ApplyListenerPlugin(ctx, listenerCtx, outputListener)
				Expect(err).ToNot(HaveOccurred())
				aggListener := outputListener.GetAggregateListener()
				Expect(aggListener).ToNot(BeNil())
				httpOptionsMap := aggListener.GetHttpResources().GetHttpOptions()
				Expect(httpOptionsMap).To(BeNil())
				hfcs := aggListener.GetHttpFilterChains()
				Expect(hfcs).To(HaveLen(1))
				httpOpts := httpOptionsMap[hfcs[0].GetHttpOptionsRef()]
				Expect(httpOpts).To(BeNil())
			})
		})

		When("HttpListenerOptions exist in a different namespace than the provided listenerCtx", func() {
			BeforeEach(func() {
				deps = []client.Object{attachedHttpListenerOption()}
				listenerCtx.Gateway.SetNamespace("bad-namespace")
			})
			It("does not add buffer limit", func() {
				err := plugin.ApplyListenerPlugin(ctx, listenerCtx, outputListener)
				Expect(err).ToNot(HaveOccurred())
				aggListener := outputListener.GetAggregateListener()
				Expect(aggListener).ToNot(BeNil())
				httpOptionsMap := aggListener.GetHttpResources().GetHttpOptions()
				Expect(httpOptionsMap).To(BeNil())
				hfcs := aggListener.GetHttpFilterChains()
				Expect(hfcs).To(HaveLen(1))
				httpOpts := httpOptionsMap[hfcs[0].GetHttpOptionsRef()]
				Expect(httpOpts).To(BeNil())
			})
		})
	})

})

func attachedHttpListenerOption() *solokubev1.HttpListenerOption {
	return &solokubev1.HttpListenerOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy",
			Namespace: "default",
		},
		Spec: sologatewayv1.HttpListenerOption{
			TargetRefs: []*corev1.PolicyTargetReferenceWithSectionName{
				{
					Group:     gwv1.GroupVersion.Group,
					Kind:      wellknown.GatewayKind,
					Name:      "gw",
					Namespace: wrapperspb.String("default"),
				},
			},
			Options: &v1.HttpListenerOptions{
				HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{
					ServerName: wrapperspb.String("unit-test v4.19"),
				},
			},
		},
	}
}
func attachedHttpListenerOptionWithSectionName() *solokubev1.HttpListenerOption {
	listOpt := attachedHttpListenerOption()
	listOpt.Spec.TargetRefs[0].SectionName = &wrapperspb.StringValue{
		Value: "test-listener",
	}
	return listOpt
}

func attachedHttpListenerOptionOmitNamespace() *solokubev1.HttpListenerOption {
	listOpt := attachedHttpListenerOption()
	listOpt.Spec.TargetRefs[0].Namespace = nil
	return listOpt
}

func nonAttachedHttpListenerOption() *solokubev1.HttpListenerOption {
	listOpt := attachedHttpListenerOption()
	listOpt.ObjectMeta.Name = "bad-policy"
	listOpt.Spec.TargetRefs[0].Name = "bad-gw"
	return listOpt
}
