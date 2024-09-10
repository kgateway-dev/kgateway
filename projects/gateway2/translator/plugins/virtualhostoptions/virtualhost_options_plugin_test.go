package virtualhostoptions

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/solo-io/gloo/pkg/utils/statusutils"
	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/utils"
	vhoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/virtualhostoptions/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/retries"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	corev1 "github.com/solo-io/skv2/pkg/api/core.skv2.solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("VirtualHostOptions Plugin", func() {
	Describe("Attaching VirtualHostOptions via policy attachment", func() {
		var (
			deps   []client.Object
			plugin *plugin
			ctx    context.Context

			listenerCtx     *plugins.ListenerContext
			outputListener  *v1.Listener
			expectedOptions *v1.VirtualHostOptions
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
						HttpResources: &v1.AggregateListener_HttpResources{
							VirtualHosts: map[string]*v1.VirtualHost{"foo": {}},
						},
					},
				},
			}

			expectedOptions = &v1.VirtualHostOptions{
				Retries: &retries.RetryPolicy{
					RetryOn:    "5xx",
					NumRetries: 5,
				},
			}
		})
		JustBeforeEach(func() {
			fakeClient := testutils.BuildIndexedFakeClient(deps, gwquery.IterateIndices, vhoptquery.IterateIndices)
			gwQueries := testutils.BuildGatewayQueriesWithClient(fakeClient)
			resourceClientFactory := &factory.MemoryResourceClientFactory{
				Cache: memory.NewInMemoryResourceCache(),
			}

			vhOptionClient, _ := sologatewayv1.NewVirtualHostOptionClient(ctx, resourceClientFactory)
			statusClient := statusutils.GetStatusClientForNamespace("gloo-system")
			statusReporter := reporter.NewReporter(defaults.KubeGatewayReporter, statusClient, vhOptionClient.BaseClient())
			plugin = NewPlugin(gwQueries, fakeClient, vhOptionClient, statusReporter)
		})
		When("outListener is not an AggregateListener", func() {
			BeforeEach(func() {
				outputListener = &v1.Listener{
					ListenerType: &v1.Listener_HybridListener{
						HybridListener: &v1.HybridListener{},
					},
				}
			})
			It("produces expected error", func() {
				err := plugin.ApplyListenerPlugin(ctx, &plugins.ListenerContext{}, outputListener)
				Expect(err).To(MatchError(utils.ErrUnexpectedListenerType))
			})
		})

		When("VirtualHostOptions exist in the same namespace and are attached correctly", func() {
			BeforeEach(func() {
				deps = []client.Object{attachedVirtualHostOption()}
			})
			It("correctly adds retry", func() {
				plugin.ApplyListenerPlugin(ctx, listenerCtx, outputListener)

				for _, vh := range outputListener.GetAggregateListener().HttpResources.VirtualHosts {
					Expect(proto.Equal(vh.GetOptions(), expectedOptions)).To(BeTrue())
				}
			})
		})

		When("Multiple VirtualHostOptions attaching to the same listener", func() {
			BeforeEach(func() {
				first := attachedVirtualHostOptionAfterT("first", 0)

				second := attachedVirtualHostOptionAfterT("second", 1*time.Hour)
				second.Spec.Options.Retries.NumRetries = 10
				second.Spec.Options.IncludeRequestAttemptCount = wrapperspb.Bool(true)

				third := attachedVirtualHostOptionAfterT("third", 2*time.Hour)
				third.Spec.Options.HeaderManipulation = &headers.HeaderManipulation{
					RequestHeadersToRemove: []string{"third"},
				}

				deps = []client.Object{first, second, third}
			})
			It("correctly use merged options in priority order from oldest to newest", func() {
				plugin.ApplyListenerPlugin(ctx, listenerCtx, outputListener)

				for _, vh := range outputListener.GetAggregateListener().HttpResources.VirtualHosts {
					// From first
					Expect(vh.GetOptions().GetRetries().GetNumRetries()).To(BeNumerically("==", 5))
					// From second
					Expect(vh.GetOptions().GetIncludeRequestAttemptCount().GetValue()).To(BeTrue())
					// From third
					Expect(vh.GetOptions().GetHeaderManipulation().GetRequestHeadersToRemove()).To(ConsistOf("third"))
				}
			})
		})

		When("VirtualHostOptions exist in the same namespace and are attached correctly with section name", func() {
			BeforeEach(func() {
				deps = []client.Object{attachedVirtualHostOptionWithSectionName()}
			})
			It("correctly adds retry", func() {
				plugin.ApplyListenerPlugin(ctx, listenerCtx, outputListener)

				for _, vh := range outputListener.GetAggregateListener().HttpResources.VirtualHosts {
					Expect(proto.Equal(vh.GetOptions(), expectedOptions)).To(BeTrue())
				}
			})
		})

		When("VirtualHostOptions exist in the same namespace and are attached correctly but omit the namespace in targetRef", func() {
			BeforeEach(func() {
				deps = []client.Object{attachedVirtualHostOptionOmitNamespace()}
			})
			It("correctly adds retry", func() {
				plugin.ApplyListenerPlugin(ctx, listenerCtx, outputListener)

				for _, vh := range outputListener.GetAggregateListener().HttpResources.VirtualHosts {
					Expect(proto.Equal(vh.GetOptions(), expectedOptions)).To(BeTrue())
				}
			})
		})

		When("VirtualHostOptions exist in the same namespace but are not attached correctly", func() {
			BeforeEach(func() {
				deps = []client.Object{nonAttachedVirtualHostOption()}
			})
			It("does not add faultinjection", func() {
				plugin.ApplyListenerPlugin(ctx, listenerCtx, outputListener)

				for _, vh := range outputListener.GetAggregateListener().HttpResources.VirtualHosts {
					Expect(vh.GetOptions()).To(BeNil())
				}
			})
		})

		When("VirtualHostOptions exist in a different namespace than the provided listenerCtx", func() {
			BeforeEach(func() {
				deps = []client.Object{attachedVirtualHostOption()}
				listenerCtx.Gateway.SetNamespace("bad-namespace")
			})
			It("does not add retry", func() {
				plugin.ApplyListenerPlugin(ctx, listenerCtx, outputListener)

				for _, vh := range outputListener.GetAggregateListener().HttpResources.VirtualHosts {
					Expect(vh.GetOptions()).To(BeNil())
				}
			})
		})
	})
})

func attachedVirtualHostOption() *solokubev1.VirtualHostOption {
	return &solokubev1.VirtualHostOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy",
			Namespace: "default",
		},
		Spec: sologatewayv1.VirtualHostOption{
			TargetRefs: []*corev1.PolicyTargetReferenceWithSectionName{
				{
					Group:     gwv1.GroupVersion.Group,
					Kind:      wellknown.GatewayKind,
					Name:      "gw",
					Namespace: wrapperspb.String("default"),
				},
			},
			Options: &v1.VirtualHostOptions{
				Retries: &retries.RetryPolicy{
					RetryOn:    "5xx",
					NumRetries: 5,
				},
			},
		},
	}
}

func attachedVirtualHostOptionAfterT(name string, d time.Duration) *solokubev1.VirtualHostOption {
	return &solokubev1.VirtualHostOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now().Add(d)),
		},
		Spec: sologatewayv1.VirtualHostOption{
			TargetRefs: []*corev1.PolicyTargetReferenceWithSectionName{
				{
					Group:     gwv1.GroupVersion.Group,
					Kind:      wellknown.GatewayKind,
					Name:      "gw",
					Namespace: wrapperspb.String("default"),
				},
			},
			Options: &v1.VirtualHostOptions{
				Retries: &retries.RetryPolicy{
					RetryOn:    "5xx",
					NumRetries: 5,
				},
			},
		},
	}
}

func attachedVirtualHostOptionWithSectionName() *solokubev1.VirtualHostOption {
	vhOpt := attachedVirtualHostOption()
	vhOpt.Spec.TargetRefs[0].SectionName = &wrapperspb.StringValue{
		Value: "test-listener",
	}
	return vhOpt
}

func attachedVirtualHostOptionOmitNamespace() *solokubev1.VirtualHostOption {
	vhOpt := attachedVirtualHostOption()
	vhOpt.Spec.TargetRefs[0].Namespace = nil
	return vhOpt
}

func nonAttachedVirtualHostOption() *solokubev1.VirtualHostOption {
	vhOpt := attachedVirtualHostOption()
	vhOpt.ObjectMeta.Name = "bad-policy"
	vhOpt.Spec.TargetRefs[0].Name = "bad-gw"
	return vhOpt
}
