package listeneroptions

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
	lisoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/listeneroptions/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	corev1 "github.com/solo-io/skv2/pkg/api/core.skv2.solo.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("ListenerOptions Plugin", func() {
	Describe("Attaching ListenerOptions via policy attachment", func() {
		var (
			deps   []client.Object
			plugin *plugin
			ctx    context.Context

			listenerCtx     *plugins.ListenerContext
			outputListener  *v1.Listener
			expectedOptions *v1.ListenerOptions
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

			outputListener = &v1.Listener{}

			expectedOptions = &v1.ListenerOptions{
				PerConnectionBufferLimitBytes: &wrapperspb.UInt32Value{
					Value: uint32(419),
				},
			}
		})
		JustBeforeEach(func() {
			fakeClient := testutils.BuildIndexedFakeClient(deps, gwquery.IterateIndices, lisoptquery.IterateIndices)
			gwQueries := testutils.BuildGatewayQueriesWithClient(fakeClient)
			plugin = NewPlugin(gwQueries, fakeClient)
		})

		When("ListenerOptions exist in the same namespace and are attached correctly", func() {
			BeforeEach(func() {
				deps = []client.Object{attachedListenerOption()}
			})
			It("correctly adds buffer limit", func() {
				err := plugin.ApplyListenerPlugin(ctx, listenerCtx, outputListener)
				Expect(err).ToNot(HaveOccurred())
				Expect(proto.Equal(outputListener.GetOptions(), expectedOptions)).To(BeTrue())
			})
		})

		When("ListenerOptions exist in the same namespace and are attached correctly with section name", func() {
			BeforeEach(func() {
				deps = []client.Object{attachedListenerOptionWithSectionName()}
			})
			It("correctly adds buffer limit", func() {
				err := plugin.ApplyListenerPlugin(ctx, listenerCtx, outputListener)
				Expect(err).ToNot(HaveOccurred())
				Expect(proto.Equal(outputListener.GetOptions(), expectedOptions)).To(BeTrue())
			})
		})

		When("ListenerOptions exist in the same namespace and are attached correctly but omit the namespace in targetRef", func() {
			BeforeEach(func() {
				deps = []client.Object{attachedListenerOptionOmitNamespace()}
			})
			It("correctly adds buffer limit", func() {
				err := plugin.ApplyListenerPlugin(ctx, listenerCtx, outputListener)
				Expect(err).ToNot(HaveOccurred())
				Expect(proto.Equal(outputListener.GetOptions(), expectedOptions)).To(BeTrue())
			})
		})

		When("ListenerOptions exist in the same namespace but are not attached correctly", func() {
			BeforeEach(func() {
				deps = []client.Object{nonAttachedListenerOption()}
			})
			It("does not add buffer limit", func() {
				err := plugin.ApplyListenerPlugin(ctx, listenerCtx, outputListener)
				Expect(err).ToNot(HaveOccurred())
				Expect(outputListener.GetOptions()).To(BeNil())
			})
		})

		When("ListenerOptions exist in a different namespace than the provided listenerCtx", func() {
			BeforeEach(func() {
				deps = []client.Object{attachedListenerOption()}
				listenerCtx.Gateway.SetNamespace("bad-namespace")
			})
			It("does not add buffer limit", func() {
				err := plugin.ApplyListenerPlugin(ctx, listenerCtx, outputListener)
				Expect(err).ToNot(HaveOccurred())
				Expect(outputListener.GetOptions()).To(BeNil())
			})
		})
	})

})

func attachedListenerOption() *solokubev1.ListenerOption {
	return &solokubev1.ListenerOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy",
			Namespace: "default",
		},
		Spec: sologatewayv1.ListenerOption{
			TargetRefs: []*corev1.PolicyTargetReferenceWithSectionName{
				{
					Group:     gwv1.GroupVersion.Group,
					Kind:      wellknown.GatewayKind,
					Name:      "gw",
					Namespace: wrapperspb.String("default"),
				},
			},
			Options: &v1.ListenerOptions{
				PerConnectionBufferLimitBytes: &wrapperspb.UInt32Value{
					Value: uint32(419),
				},
			},
		},
	}
}
func attachedListenerOptionWithSectionName() *solokubev1.ListenerOption {
	listOpt := attachedListenerOption()
	listOpt.Spec.TargetRefs[0].SectionName = &wrapperspb.StringValue{
		Value: "test-listener",
	}
	return listOpt
}

func attachedListenerOptionOmitNamespace() *solokubev1.ListenerOption {
	listOpt := attachedListenerOption()
	listOpt.Spec.TargetRefs[0].Namespace = nil
	return listOpt
}

func nonAttachedListenerOption() *solokubev1.ListenerOption {
	listOpt := attachedListenerOption()
	listOpt.ObjectMeta.Name = "bad-policy"
	listOpt.Spec.TargetRefs[0].Name = "bad-gw"
	return listOpt
}
