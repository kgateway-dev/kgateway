package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sync"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/gloo/test/samples"
	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"go.opencensus.io/stats/view"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syamlutil "sigs.k8s.io/yaml"
)

var _ = Describe("Validator", func() {
	var (
		t  translator.Translator
		vf ValidatorFunc
		ns string
		v  *validator
	)

	BeforeEach(func() {
		t = translator.NewDefaultTranslator(translator.Opts{})
		ns = "my-namespace"
		v = NewValidator(NewValidatorConfig(t, vf, ns, false, false))
		mValidConfig = utils.MakeGauge("validation.gateway.solo.io/valid_config", "A boolean indicating whether gloo config is valid")
	})

	It("returns error before sync called", func() {
		_, err := v.ValidateVirtualService(nil, nil, false)
		Expect(err).To(testutils.HaveInErrorChain(NotReadyErr))
		err = v.Sync(context.Background(), &gloov1snap.ApiSnapshot{})
		Expect(err).NotTo(HaveOccurred())
	})

	It("has mValidConfig=1 after Sync is called with valid snapshot", func() {
		err := v.Sync(context.TODO(), &gloov1snap.ApiSnapshot{})
		Expect(err).NotTo(HaveOccurred())

		rows, err := view.RetrieveData("validation.gateway.solo.io/valid_config")
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).NotTo(BeEmpty())
		Expect(rows[0].Data.(*view.LastValueData).Value).To(BeEquivalentTo(1))
	})

	It("has mValidConfig=0 after Sync is called with invalid snapshot", func() {
		us := samples.SimpleUpstream()
		snap := samples.GatewayToGlooSnapshot(samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns))
		snap.Gateways.Each(func(element *gatewayv1.Gateway) {
			http, ok := element.GatewayType.(*gatewayv1.Gateway_HttpGateway)
			if !ok {
				return
			}
			http.HttpGateway.VirtualServiceExpressions = &gatewayv1.VirtualServiceSelectorExpressions{
				Expressions: []*gatewayv1.VirtualServiceSelectorExpressions_Expression{
					{
						Key:      "a",
						Operator: gatewayv1.VirtualServiceSelectorExpressions_Expression_Equals,
						Values:   []string{"b", "c"},
					},
				},
			}
		})
		err := v.Sync(context.TODO(), snap)
		Expect(err).To(HaveOccurred())

		rows, err := view.RetrieveData("validation.gateway.solo.io/valid_config")
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).NotTo(BeEmpty())
		Expect(rows[0].Data.(*view.LastValueData).Value).To(BeEquivalentTo(0))
	})

	Context("validating gloo resources", func() {
		Context("upstreams", func() {
			It("accepts an upstream when validation succeeds", func() {
				v.validationFunc = acceptProxy

				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				reports, err := v.ValidateUpstream(context.TODO(), us, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
				proxyReport := (*reports.ProxyReports)[0]
				warnings := validationutils.GetProxyWarning(proxyReport)
				errors := validationutils.GetProxyError(proxyReport)
				Expect(warnings).To(BeEmpty())
				Expect(errors).NotTo(HaveOccurred())
			})
			It("rejects an upstream when validation fails", func() {
				v.validationFunc = failProxy

				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				reports, err := v.ValidateUpstream(context.TODO(), us, false)
				Expect(err).To(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
				proxyReport := (*reports.ProxyReports)[0]
				errors := validationutils.GetProxyError(proxyReport)
				Expect(errors).To(HaveOccurred())
			})
			It("accepts an upstream when there is a validation warning and allowWarnings is true", func() {
				v.validationFunc = warnProxy
				v.allowWarnings = true

				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				reports, err := v.ValidateUpstream(context.TODO(), us, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
				proxyReport := (*reports.ProxyReports)[0]
				warnings := validationutils.GetProxyWarning(proxyReport)
				Expect(warnings).NotTo(BeEmpty())
			})
			It("rejects an upstream when there is a validation warning and allowWarnings is false", func() {
				v.validationFunc = warnProxy
				v.allowWarnings = false

				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				reports, err := v.ValidateUpstream(context.TODO(), us, false)
				Expect(err).To(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
				proxyReport := (*reports.ProxyReports)[0]
				warnings := validationutils.GetProxyWarning(proxyReport)
				Expect(warnings).NotTo(BeEmpty())
			})

			It("accepts an upstream deletion when validation succeeds", func() {
				v.validationFunc = acceptProxy

				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				ref := us.GetMetadata().Ref()
				err = v.ValidateDeleteUpstream(context.TODO(), ref, false)
				Expect(err).NotTo(HaveOccurred())
			})
			It("rejects an upstream deletion when validation fails", func() {
				v.validationFunc = failProxy

				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				ref := us.GetMetadata().Ref()
				err = v.ValidateDeleteUpstream(context.TODO(), ref, false)
				Expect(err).To(HaveOccurred())
			})
			It("accepts an upstream deletion when there is a validation warning and allowWarnings is true", func() {
				v.validationFunc = warnProxy
				v.allowWarnings = true

				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				ref := us.GetMetadata().Ref()
				err = v.ValidateDeleteUpstream(context.TODO(), ref, false)
				Expect(err).NotTo(HaveOccurred())
			})
			It("rejects an upstream deletion when there is a validation warning and allowWarnings is false", func() {
				v.validationFunc = warnProxy
				v.allowWarnings = false

				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				ref := us.GetMetadata().Ref()
				err = v.ValidateDeleteUpstream(context.TODO(), ref, false)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("secrets", func() {
			It("accepts a secret deletion when validation succeeds", func() {
				v.validationFunc = acceptProxy

				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				secret := &gloov1.Secret{
					Metadata: &core.Metadata{
						Name:      "secret",
						Namespace: "namespace",
					},
				}
				ref := secret.GetMetadata().Ref()
				err = v.ValidateDeleteSecret(context.TODO(), ref, false)
				Expect(err).NotTo(HaveOccurred())
			})
			It("rejects a secret deletion when validation fails", func() {
				v.validationFunc = failProxy

				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				secret := &gloov1.Secret{
					Metadata: &core.Metadata{
						Name:      "secret",
						Namespace: "namespace",
					},
				}
				ref := secret.GetMetadata().Ref()
				err = v.ValidateDeleteSecret(context.TODO(), ref, false)
				Expect(err).To(HaveOccurred())
			})
			It("accepts a secret deletion when there is a validation warning and allowWarnings is true", func() {
				v.validationFunc = warnProxy
				v.allowWarnings = true

				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				secret := &gloov1.Secret{
					Metadata: &core.Metadata{
						Name:      "secret",
						Namespace: "namespace",
					},
				}
				ref := secret.GetMetadata().Ref()
				err = v.ValidateDeleteSecret(context.TODO(), ref, false)
				Expect(err).NotTo(HaveOccurred())
			})
			It("rejects a secret deletion when there is a validation warning and allowWarnings is false", func() {
				v.validationFunc = warnProxy
				v.allowWarnings = false

				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				secret := &gloov1.Secret{
					Metadata: &core.Metadata{
						Name:      "secret",
						Namespace: "namespace",
					},
				}
				ref := secret.GetMetadata().Ref()
				err = v.ValidateDeleteSecret(context.TODO(), ref, false)
				Expect(err).To(HaveOccurred())
			})
		})
	})
	Context("validating a route table", func() {
		Context("proxy validation accepted", func() {
			It("accepts the rt", func() {
				v.validationFunc = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewayToGlooSnapshot(samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns))
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateRouteTable(context.TODO(), snap.RouteTables[0], false)
				Expect(err).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
			})

			It("accepts the rt and returns proxies each time", func() {
				v.validationFunc = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewayToGlooSnapshot(samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns))
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateRouteTable(context.TODO(), snap.RouteTables[0], false)
				Expect(err).NotTo(HaveOccurred())
				proxyReports := *(reports.ProxyReports)
				Expect(proxyReports).To(HaveLen(1))
				Expect(reports.GetProxies()).To(HaveLen(1))
				Expect(reports.GetProxies()[0]).To(ContainSubstring("listener-::-8080"))

				// repeat to ensure any hashing doesn't short circuit returning the proxies
				reports, err = v.ValidateRouteTable(context.TODO(), snap.RouteTables[0], false)
				Expect(err).NotTo(HaveOccurred())

				proxyReports = *(reports.ProxyReports)
				Expect(proxyReports).To(HaveLen(1))
				Expect(reports.GetProxies()).To(HaveLen(1))
				Expect(reports.GetProxies()[0]).To(ContainSubstring("listener-::-8080"))
			})
		})

		Context("proxy validation returns error", func() {
			It("rejects the rt", func() {
				v.validationFunc = failProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewayToGlooSnapshot(samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns))
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// change something to change the hash
				snap.RouteTables[0].Metadata.Labels = map[string]string{"change": "my mind"}

				reports, err := v.ValidateRouteTable(context.TODO(), snap.RouteTables[0], false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to validate Proxy with Gloo validation server"))
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
			})

			Context("allowWarnings=false", func() {
				BeforeEach(func() {
					v = NewValidator(NewValidatorConfig(t, acceptProxy, ns, true, false))
				})
				It("rejects a vs with missing route table ref", func() {
					v.validationFunc = warnProxy
					us := samples.SimpleUpstream()
					snap := samples.GatewayToGlooSnapshot(samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns))
					err := v.Sync(context.TODO(), snap)
					Expect(err).NotTo(HaveOccurred())

					// change something to change the hash
					snap.RouteTables[0].Metadata.Labels = map[string]string{"change": "my mind"}

					reports, err := v.ValidateRouteTable(context.TODO(), snap.RouteTables[0], false)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Route Warning: InvalidDestinationWarning. Reason: you should try harder next time"))
					Expect(*(reports.ProxyReports)).To(HaveLen(1))
				})
			})
		})

		Context("route table rejected", func() {
			It("rejects the rt", func() {
				badRoute := &gatewayv1.Route{
					Action: &gatewayv1.Route_DelegateAction{
						DelegateAction: &gatewayv1.DelegateAction{
							DelegationType: &gatewayv1.DelegateAction_Ref{
								Ref: &core.ResourceRef{
									Name:      "invalid",
									Namespace: "name",
								},
							},
						},
					},
				}

				// validate proxy should never be called
				v.validationFunc = nil
				us := samples.SimpleUpstream()
				snap := samples.GatewayToGlooSnapshot(samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns))
				rt := snap.RouteTables[0].DeepCopyObject().(*gatewayv1.RouteTable)
				rt.Routes = append(rt.Routes, badRoute)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateRouteTable(context.TODO(), rt, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(*(reports.ProxyReports)).To(HaveLen(0))
			})
		})

		Context("route table delegation with selectors", func() {
			It("accepts route table with valid prefix", func() {
				v.validationFunc = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewayToGlooSnapshot(samples.GatewaySnapshotWithDelegateSelector(us.Metadata.Ref(), ns))
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				rt := samples.RouteTableWithLabelsAndPrefix("route2", ns, "/foo/2", map[string]string{"pick": "me"})
				_, err = v.ValidateRouteTable(context.TODO(), rt, false)
				Expect(err).NotTo(HaveOccurred())
			})

			It("rejects route table with invalid prefix", func() {
				v.validationFunc = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewayToGlooSnapshot(samples.GatewaySnapshotWithDelegateSelector(us.Metadata.Ref(), ns))
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// the prefix doesn't start with the parent's prefix so validation will fail
				rt := samples.RouteTableWithLabelsAndPrefix("route2", ns, "/not", map[string]string{"pick": "me"})
				_, err = v.ValidateRouteTable(context.TODO(), rt, false)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("delete a route table", func() {
		Context("has parents", func() {
			It("rejects deletion", func() {
				v.validationFunc = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewayToGlooSnapshot(samples.GatewaySnapshotWithDelegateChain(us.Metadata.Ref(), ns))
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				err = v.ValidateDeleteRouteTable(context.TODO(), snap.RouteTables[1].Metadata.Ref(), false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(
					RouteTableDeleteErr(nil, []*core.ResourceRef{snap.RouteTables[0].Metadata.Ref()}).Error()),
				)
			})
		})
		Context("has no parents", func() {
			It("deletes safely", func() {
				v.validationFunc = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewayToGlooSnapshot(samples.GatewaySnapshotWithDelegateChain(us.Metadata.Ref(), ns))
				// break the parent chain
				snap.RouteTables[1].Routes = nil
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				ref := snap.RouteTables[2].Metadata.Ref()
				err = v.ValidateDeleteRouteTable(context.TODO(), ref, false)
				Expect(err).NotTo(HaveOccurred())

				// ensure route table was removed from validator internal snapshot
				_, err = v.latestSnapshot.RouteTables.Find(ref.Strings())
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("validating a virtual service", func() {

		Context("proxy validation returns error", func() {
			It("rejects the vs", func() {
				v.validationFunc = failProxy
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// change something to change the hash
				snap.VirtualServices[0].Metadata.Labels = map[string]string{"change": "my mind"}

				reports, err := v.ValidateVirtualService(context.TODO(), snap.VirtualServices[0], false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to validate Proxy with Gloo validation server"))
				Expect(*(reports.ProxyReports)).To(HaveLen(1))

			})
		})
		Context("proxy validation accepted", func() {
			It("accepts the vs", func() {
				v.validationFunc = acceptProxy
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateVirtualService(context.TODO(), snap.VirtualServices[0], false)
				Expect(err).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
			})

			It("accepts the vs and returns proxies each time", func() {
				v.validationFunc = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewayToGlooSnapshot(samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns))
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateVirtualService(context.TODO(), snap.VirtualServices[0], false)
				Expect(err).NotTo(HaveOccurred())
				proxyReports := *(reports.ProxyReports)
				Expect(proxyReports).To(HaveLen(1))
				Expect(reports.GetProxies()).To(HaveLen(1))
				Expect(reports.GetProxies()[0]).To(ContainSubstring("listener-::-8080"))

				// repeat to ensure any hashing doesn't short circuit returning the proxies
				reports, err = v.ValidateVirtualService(context.TODO(), snap.VirtualServices[0], false)
				Expect(err).NotTo(HaveOccurred())
				proxyReports = *(reports.ProxyReports)
				Expect(proxyReports).To(HaveLen(1))
				Expect(reports.GetProxies()).To(HaveLen(1))
				Expect(reports.GetProxies()[0]).To(ContainSubstring("listener-::-8080"))
			})
		})
		Context("no gateways for virtual service", func() {
			It("accepts the vs", func() {
				v.validationFunc = failProxy
				snap := samples.SimpleGlooSnapshot(ns)
				snap.Gateways.Each(func(element *gatewayv1.Gateway) {
					switch gatewayType := element.GetGatewayType().(type) {
					case *gatewayv1.Gateway_HttpGateway:
						gatewayType.HttpGateway.VirtualServiceSelector = map[string]string{"nobody": "hastheselabels"}
					case *gatewayv1.Gateway_HybridGateway:
						for _, matchedGateway := range gatewayType.HybridGateway.GetMatchedGateways() {
							if httpGateway := matchedGateway.GetHttpGateway(); httpGateway != nil {
								httpGateway.VirtualServiceSelector = map[string]string{"nobody": "hastheselabels"}
							}
						}
					}
				})
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateVirtualService(context.TODO(), snap.VirtualServices[0], false)
				Expect(err).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(0))
			})
		})
		Context("invalid selector expression for virtual service", func() {
			It("rejects the vs", func() {
				v.validationFunc = failProxy
				snap := samples.SimpleGlooSnapshot(ns)
				snap.Gateways.Each(func(element *gatewayv1.Gateway) {
					http, ok := element.GatewayType.(*gatewayv1.Gateway_HttpGateway)
					if !ok {
						return
					}
					http.HttpGateway.VirtualServiceExpressions = &gatewayv1.VirtualServiceSelectorExpressions{
						Expressions: []*gatewayv1.VirtualServiceSelectorExpressions_Expression{
							{
								Key:      "a",
								Operator: gatewayv1.VirtualServiceSelectorExpressions_Expression_Equals,
								Values:   []string{"b", "c"},
							},
						},
					}
				})
				err := v.Sync(context.TODO(), snap)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("expression is invalid"))
			})
		})
		Context("virtual service rejected", func() {
			It("rejects the vs", func() {
				badRoute := &gatewayv1.Route{
					Action: &gatewayv1.Route_DelegateAction{

						DelegateAction: &gatewayv1.DelegateAction{
							DelegationType: &gatewayv1.DelegateAction_Ref{
								Ref: &core.ResourceRef{
									Name:      "invalid",
									Namespace: "name",
								},
							},
						},
					},
				}

				// validate proxy should never be called
				v.validationFunc = nil
				snap := samples.SimpleGlooSnapshot(ns)
				vs := snap.VirtualServices[0].DeepCopyObject().(*gatewayv1.VirtualService)
				vs.VirtualHost.Routes = append(vs.VirtualHost.Routes, badRoute)

				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateVirtualService(context.TODO(), vs, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(*(reports.ProxyReports)).To(HaveLen(0))
			})
		})
		Context("valid config gauge", func() {
			BeforeEach(func() {
				// reset the value before each test
				utils.Measure(context.TODO(), mValidConfig, -1)
			})
			It("returns 1 when there are no validation errors", func() {
				v.validationFunc = acceptProxy

				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				_, err = v.ValidateVirtualService(context.TODO(), snap.VirtualServices[0], false)
				Expect(err).NotTo(HaveOccurred())

				rows, err := view.RetrieveData("validation.gateway.solo.io/valid_config")
				Expect(err).NotTo(HaveOccurred())
				Expect(rows).NotTo(BeEmpty())
				Expect(rows[0].Data.(*view.LastValueData).Value).To(BeEquivalentTo(1))
			})
			It("returns 0 when there are validation errors", func() {
				v.validationFunc = failProxy

				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				_, err = v.ValidateVirtualService(context.TODO(), snap.VirtualServices[0], false)
				Expect(err).To(HaveOccurred())

				rows, err := view.RetrieveData("validation.gateway.solo.io/valid_config")
				Expect(err).NotTo(HaveOccurred())
				Expect(rows).NotTo(BeEmpty())
				Expect(rows[0].Data.(*view.LastValueData).Value).To(BeEquivalentTo(0))
			})
			It("returns 0 when there are validation warnings and allowWarnings is false", func() {
				v.allowWarnings = false
				v.validationFunc = warnProxy

				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				_, err = v.ValidateVirtualService(context.TODO(), snap.VirtualServices[0], false)
				Expect(err).To(HaveOccurred())

				rows, err := view.RetrieveData("validation.gateway.solo.io/valid_config")
				Expect(err).NotTo(HaveOccurred())
				Expect(rows).NotTo(BeEmpty())
				Expect(rows[0].Data.(*view.LastValueData).Value).To(BeEquivalentTo(0))
			})
			It("returns 1 when there are validation warnings and allowWarnings is true", func() {
				v.allowWarnings = true
				v.validationFunc = warnProxy

				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				_, err = v.ValidateVirtualService(context.TODO(), snap.VirtualServices[0], false)
				Expect(err).NotTo(HaveOccurred())

				rows, err := view.RetrieveData("validation.gateway.solo.io/valid_config")
				Expect(err).NotTo(HaveOccurred())
				Expect(rows).NotTo(BeEmpty())
				Expect(rows[0].Data.(*view.LastValueData).Value).To(BeEquivalentTo(1))
			})
			It("does not affect metrics when dryRun is true", func() {
				v.validationFunc = failProxy

				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// Metric should be valid after successful Sync
				rows, err := view.RetrieveData("validation.gateway.solo.io/valid_config")
				Expect(err).NotTo(HaveOccurred())
				Expect(rows).NotTo(BeEmpty())
				Expect(rows[0].Data.(*view.LastValueData).Value).To(BeEquivalentTo(1))

				// Run a failed validation
				_, err = v.ValidateVirtualService(context.TODO(), snap.VirtualServices[0], true)
				Expect(err).To(HaveOccurred())

				// The metric should still be valid, since dryRun was true
				rows, err = view.RetrieveData("validation.gateway.solo.io/valid_config")
				Expect(err).NotTo(HaveOccurred())
				Expect(rows).NotTo(BeEmpty())
				Expect(rows[0].Data.(*view.LastValueData).Value).To(BeEquivalentTo(1))
			})
		})
		Context("dry-run", func() {
			It("accepts the vs and rejects the second", func() {
				v.validationFunc = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// create a virtual service to validate, should pass validation as a prior one should
				// already be in the validation snapshot cache with a different domain
				samples.AddVsToSnap(snap, us.GetMetadata().Ref(), "ns")
				vs2 := &gatewayv1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs2)
				vs2.Metadata.Name = "vs2"

				reports, err := v.ValidateVirtualService(context.TODO(), vs2, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))

				// create another virtual service to validate, should fail validation as a prior one should
				// already be in the validation snapshot cache with the same domain (as dry-run before was false)
				vs3 := &gatewayv1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs3)
				vs3.Metadata.Name = "vs3"

				reports, err = v.ValidateVirtualService(context.TODO(), vs3, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(err.Error()).To(ContainSubstring("domain conflict: the following"))
				Expect(*(reports.ProxyReports)).To(HaveLen(0))
			})

			It("accepts the vs and accepts the second because of dry-run", func() {
				v.validationFunc = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				samples.AddVsToSnap(snap, us.GetMetadata().Ref(), "ns")
				// create a virtual service to validate, should pass validation as a prior one should
				// already be in the validation snapshot cache with a different domain
				vs2 := &gatewayv1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs2)
				vs2.Metadata.Name = "vs2"

				reports, err := v.ValidateVirtualService(context.TODO(), vs2, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))

				// create another virtual service to validate, should pass validation as a prior one should not
				// already be in the validation snapshot cache (as dry-run was true)
				vs3 := &gatewayv1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs3)
				vs3.Metadata.Name = "vs3"

				reports, err = v.ValidateVirtualService(context.TODO(), vs3, true)
				Expect(err).ToNot(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
			})
		})
	})

	Context("delete a virtual service", func() {
		Context("has parent gateways", func() {
			It("rejects deletion", func() {
				v.validationFunc = acceptProxy
				snap := samples.SimpleGlooSnapshot(ns)
				ref := snap.VirtualServices[0].Metadata.Ref()
				snap.Gateways.Each(func(element *gatewayv1.Gateway) {
					switch gatewayType := element.GetGatewayType().(type) {
					case *gatewayv1.Gateway_HttpGateway:
						gatewayType.HttpGateway.VirtualServices = []*core.ResourceRef{ref}
					case *gatewayv1.Gateway_HybridGateway:
						for _, matchedGateway := range gatewayType.HybridGateway.GetMatchedGateways() {
							if httpGateway := matchedGateway.GetHttpGateway(); httpGateway != nil {
								httpGateway.VirtualServices = []*core.ResourceRef{ref}
							}
						}
					}
					http, ok := element.GatewayType.(*gatewayv1.Gateway_HttpGateway)
					if !ok {
						return
					}
					http.HttpGateway.VirtualServices = []*core.ResourceRef{ref}
				})
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				err = v.ValidateDeleteVirtualService(context.TODO(), ref, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(
					VirtualServiceDeleteErr([]*core.ResourceRef{
						{Name: defaults.GatewayProxyName, Namespace: ns},
						{Name: defaults.GatewayProxyName + "-ssl", Namespace: ns},
						{Name: defaults.GatewayProxyName + "-hybrid", Namespace: ns},
					}).Error()))
			})
		})
		Context("has no parent gateways", func() {
			It("deletes safely", func() {
				v.validationFunc = acceptProxy
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				ref := snap.VirtualServices[0].Metadata.Ref()
				err = v.ValidateDeleteVirtualService(context.TODO(), ref, false)
				Expect(err).NotTo(HaveOccurred())

				// ensure vs was removed from validator internal snapshot
				_, err = v.latestSnapshot.VirtualServices.Find(ref.Strings())
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("validating a gateway", func() {

		Context("proxy validation returns error", func() {
			It("rejects the gw", func() {
				v.validationFunc = failProxy
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// change something to change the hash
				snap.Gateways[0].Metadata.Labels = map[string]string{"change": "my mind"}

				reports, err := v.ValidateGateway(context.TODO(), snap.Gateways[0], false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to validate Proxy with Gloo validation server"))
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
			})
		})
		Context("proxy validation accepted", func() {
			It("accepts the gw", func() {
				v.validationFunc = acceptProxy
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateGateway(context.TODO(), snap.Gateways[0], false)
				Expect(err).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
			})

			It("accepts the gateway and returns proxies each time", func() {
				v.validationFunc = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewayToGlooSnapshot(samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns))
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateGateway(context.TODO(), snap.Gateways[0], false)
				Expect(err).NotTo(HaveOccurred())
				proxyReports := *(reports.ProxyReports)
				Expect(proxyReports).To(HaveLen(1))
				Expect(reports.GetProxies()).To(HaveLen(1))
				Expect(reports.GetProxies()[0]).To(ContainSubstring("listener-::-8080"))

				// repeat to ensure any hashing doesn't short circuit returning the proxies
				reports, err = v.ValidateGateway(context.TODO(), snap.Gateways[0], false)
				Expect(err).NotTo(HaveOccurred())
				proxyReports = *(reports.ProxyReports)
				Expect(proxyReports).To(HaveLen(1))
				Expect(reports.GetProxies()).To(HaveLen(1))
				Expect(reports.GetProxies()[0]).To(ContainSubstring("listener-::-8080"))
			})
		})
		Context("gw rejected", func() {
			It("rejects the gw", func() {
				badRef := &core.ResourceRef{}

				// validate proxy should never be called
				v.validationFunc = nil
				snap := samples.SimpleGlooSnapshot(ns)
				gw := snap.Gateways[0].DeepCopyObject().(*gatewayv1.Gateway)

				gw.GatewayType.(*gatewayv1.Gateway_HttpGateway).HttpGateway.VirtualServices = append(gw.GatewayType.(*gatewayv1.Gateway_HttpGateway).HttpGateway.VirtualServices, badRef)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateGateway(context.TODO(), gw, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(*(reports.ProxyReports)).To(HaveLen(0))
			})
		})
	})

	Context("validating a list of virtual services", func() {

		toUnstructuredList := func(vss ...*gatewayv1.VirtualService) *unstructured.UnstructuredList {

			var objs []unstructured.Unstructured
			for _, vs := range vss {
				kubeRes, _ := gatewayv1.VirtualServiceCrd.KubeResource(vs)
				bytes, err := json.Marshal(kubeRes)
				Expect(err).ToNot(HaveOccurred())
				mapFromVs := map[string]interface{}{}

				// NOTE: This is not the default golang yaml.Unmarshal, because that implementation
				// does not unmarshal into a map[string]interface{}; it unmarshals the file into a map[interface{}]interface{}
				// https://github.com/go-yaml/yaml/issues/139
				err = k8syamlutil.Unmarshal(bytes, &mapFromVs)
				Expect(err).ToNot(HaveOccurred())

				obj := unstructured.Unstructured{Object: mapFromVs}
				objs = append(objs, obj)
			}

			return &unstructured.UnstructuredList{
				Object: map[string]interface{}{
					"kind":    "List",
					"version": "v1",
				},
				Items: objs,
			}
		}

		Context("proxy validation returns error", func() {
			It("rejects the vs list", func() {
				v.validationFunc = failProxy
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// change something to change the hash
				snap.VirtualServices[0].Metadata.Labels = map[string]string{"change": "my mind"}
				vsList := toUnstructuredList(snap.VirtualServices[0])

				reports, err := v.ValidateList(context.TODO(), vsList, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to validate Proxy with Gloo validation server"))
				Expect(*(reports.ProxyReports)).To(HaveLen(1))

			})
		})

		Context("proxy validation accepted", func() {

			It("accepts the vs list", func() {
				v.validationFunc = acceptProxy
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, merr := v.ValidateList(context.TODO(), toUnstructuredList(snap.VirtualServices[0]), false)
				Expect(merr.ErrorOrNil()).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
			})

			It("accepts the multi vs list", func() {
				v.validationFunc = acceptProxy
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				vs1 := &gatewayv1.VirtualService{}
				snap.VirtualServices[0].DeepCopyInto(vs1)
				vs1.Metadata.Name = "vs1"
				vs1.VirtualHost.Domains = []string{"example.vs1.com"}
				vs2 := &gatewayv1.VirtualService{}
				snap.VirtualServices[0].DeepCopyInto(vs2)
				vs2.Metadata.Name = "vs2"
				vs2.VirtualHost.Domains = []string{"example.vs2.com"}

				reports, merr := v.ValidateList(context.TODO(), toUnstructuredList(vs1, vs2), false)
				Expect(merr.ErrorOrNil()).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(2))
			})

			It("rejects the multi vs list with overlapping domains", func() {
				v.validationFunc = acceptProxy
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				vs1 := &gatewayv1.VirtualService{}
				snap.VirtualServices[0].DeepCopyInto(vs1)
				vs1.Metadata.Name = "vs1"

				vs2 := &gatewayv1.VirtualService{}
				snap.VirtualServices[0].DeepCopyInto(vs2)
				vs2.Metadata.Name = "vs2"

				reports, err := v.ValidateList(context.TODO(), toUnstructuredList(vs1, vs2), false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(err.Error()).To(ContainSubstring("domain conflict: the following"))
				Expect(*(reports.ProxyReports)).To(HaveLen(0))
			})

			It("accepts the vs list and returns proxies each time", func() {
				v.validationFunc = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewayToGlooSnapshot(samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns))
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, merr := v.ValidateList(context.TODO(), toUnstructuredList(snap.VirtualServices[0]), false)
				Expect(merr.ErrorOrNil()).NotTo(HaveOccurred())
				proxyReports := *(reports.ProxyReports)
				Expect(proxyReports).To(HaveLen(1))
				Expect(reports.GetProxies()).To(HaveLen(1))
				Expect(reports.GetProxies()[0]).To(ContainSubstring("listener-::-8080"))

				// repeat to ensure any hashing doesn't short circuit returning the proxies
				reports, merr = v.ValidateList(context.TODO(), toUnstructuredList(snap.VirtualServices[0]), false)
				Expect(merr.ErrorOrNil()).NotTo(HaveOccurred())
				proxyReports = *(reports.ProxyReports)
				Expect(proxyReports).To(HaveLen(1))
				Expect(reports.GetProxies()).To(HaveLen(1))
				Expect(reports.GetProxies()[0]).To(ContainSubstring("listener-::-8080"))
			})
		})

		Context("unmarshal errors", func() {
			It("doesn't mask other errors when there's an unmarshal error in a list", func() {

				v.validationFunc = acceptProxy
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				ul := &unstructured.UnstructuredList{}
				jsonBytes, err := ioutil.ReadFile("fixtures/unmarshal-err.json")
				Expect(err).ToNot(HaveOccurred())
				err = ul.UnmarshalJSON(jsonBytes)
				Expect(err).ToNot(HaveOccurred())
				reports, merr := v.ValidateList(context.TODO(), ul, false)
				Expect(merr).To(HaveOccurred())
				Expect(merr.Errors).To(HaveLen(3))
				Expect(merr.Errors[0]).To(MatchError(ContainSubstring("route table gloo-system.i-dont-exist-rt missing")))
				Expect(merr.Errors[1]).To(MatchError(ContainSubstring("virtual service [gloo-system.invalid-vs-2] does not specify a virtual host")))
				Expect(merr.Errors[2]).To(MatchError(ContainSubstring("parsing resource from crd spec testproxy1-rt in namespace gloo-system into *v1.RouteTable")))
				Expect(merr.Errors[2]).To(MatchError(ContainSubstring("unknown field \"matcherss\" in gateway.solo.io.Route")))
				Expect(*(reports.ProxyReports)).To(HaveLen(0))

			})
		})

		Context("virtual service list rejected", func() {
			It("rejects the vs list", func() {
				badRoute := &gatewayv1.Route{
					Action: &gatewayv1.Route_DelegateAction{

						DelegateAction: &gatewayv1.DelegateAction{
							DelegationType: &gatewayv1.DelegateAction_Ref{
								Ref: &core.ResourceRef{
									Name:      "invalid",
									Namespace: "name",
								},
							},
						},
					},
				}

				// validate proxy should never be called
				v.validationFunc = nil
				snap := samples.SimpleGlooSnapshot(ns)
				vs := snap.VirtualServices[0].DeepCopyObject().(*gatewayv1.VirtualService)
				vs.VirtualHost.Routes = append(vs.VirtualHost.Routes, badRoute)
				vsList := toUnstructuredList(vs)

				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateList(context.TODO(), vsList, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(*(reports.ProxyReports)).To(HaveLen(0))
			})
		})

		Context("dry-run", func() {
			It("accepts the vs and rejects the second", func() {
				v.validationFunc = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// create a virtual service to validate, should pass validation as a prior one should
				// already be in the validation snapshot cache with a different domain
				samples.AddVsToSnap(snap, us.GetMetadata().Ref(), "ns")

				vs2 := &gatewayv1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs2)
				vs2.Metadata.Name = "vs2"

				reports, merr := v.ValidateList(context.TODO(), toUnstructuredList(vs2), false)
				Expect(merr.ErrorOrNil()).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))

				// create another virtual service to validate, should fail validation as a prior one should
				// already be in the validation snapshot cache with the same domain (as dry-run before was false)
				vs3 := &gatewayv1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs3)
				vs3.Metadata.Name = "vs3"

				reports, merr = v.ValidateList(context.TODO(), toUnstructuredList(vs3), false)
				Expect(merr.ErrorOrNil()).To(HaveOccurred())
				Expect(merr.ErrorOrNil().Error()).To(ContainSubstring("could not render proxy"))
				Expect(merr.ErrorOrNil().Error()).To(ContainSubstring("domain conflict: the following"))
				Expect(*(reports.ProxyReports)).To(HaveLen(0))
			})

			It("accepts the vs and accepts the second because of dry-run", func() {
				v.validationFunc = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				samples.AddVsToSnap(snap, us.GetMetadata().Ref(), "ns")
				// create a virtual service to validate, should pass validation as a prior one should
				// already be in the validation snapshot cache with a different domain
				vs2 := &gatewayv1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs2)
				vs2.Metadata.Name = "vs2"

				reports, merr := v.ValidateList(context.TODO(), toUnstructuredList(vs2), true)
				Expect(merr.ErrorOrNil()).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))

				// create another virtual service to validate, should pass validation as a prior one should not
				// already be in the validation snapshot cache (as dry-run was true)
				vs3 := &gatewayv1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs3)
				vs3.Metadata.Name = "vs3"

				reports, merr = v.ValidateList(context.TODO(), toUnstructuredList(vs3), true)
				Expect(merr.ErrorOrNil()).ToNot(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
			})

		})

	})

	Context("validating concurrent scenario", func() {

		var (
			resultMap       sync.Map
			numberOfWorkers int
		)

		BeforeEach(func() {
			resultMap = sync.Map{}
			numberOfWorkers = 100
		})

		validateVirtualServiceWorker := func(vsToDuplicate *gatewayv1.VirtualService, name string) error {
			// duplicate the vs with a different name
			workerVirtualService := &gatewayv1.VirtualService{}
			vsToDuplicate.DeepCopyInto(workerVirtualService)
			workerVirtualService.Metadata.Name = "vs2-" + name

			_, err := v.ValidateVirtualService(context.TODO(), workerVirtualService, false)

			if err != nil {
				// worker errors are stored in the resultMap
				resultMap.Store(name, err.Error())
			}

			return nil
		}

		It("accepts only 1 vs when multiple are written concurrently", func() {
			v.validationFunc = acceptProxy
			us := samples.SimpleUpstream()
			snap := samples.SimpleGlooSnapshot(ns)
			err := v.Sync(context.TODO(), snap)
			Expect(err).NotTo(HaveOccurred())

			samples.AddVsToSnap(snap, us.GetMetadata().Ref(), "ns")
			vsToDuplicate := snap.VirtualServices[1]

			// start workers
			errorGroup := errgroup.Group{}
			for i := 0; i < numberOfWorkers; i++ {
				workerName := fmt.Sprintf("worker #%d", i)
				errorGroup.Go(func() error {
					defer GinkgoRecover()
					return validateVirtualServiceWorker(vsToDuplicate, workerName)
				})
			}

			// wait for all workers to complete
			err = errorGroup.Wait()
			Expect(err).NotTo(HaveOccurred())

			// aggregate the error messages from all the workers
			var errMessages []string
			resultMap.Range(func(name, value interface{}) bool {
				errMessages = append(errMessages, value.(string))
				return true // continue
			})

			// Expect 1 worker to have successfully completed and all others to have failed
			Expect(len(errMessages)).To(Equal(numberOfWorkers - 1))
		})

	})
})

type mockValidationClient struct {
	validate func(ctx context.Context, in *validation.GlooValidationServiceRequest, opts ...grpc.CallOption) (*validation.GlooValidationServiceResponse, error)
}

func (c *mockValidationClient) NotifyOnResync(ctx context.Context, in *validation.NotifyOnResyncRequest, opts ...grpc.CallOption) (validation.GlooValidationService_NotifyOnResyncClient, error) {
	return nil, nil
}

func (c *mockValidationClient) Validate(ctx context.Context, in *validation.GlooValidationServiceRequest, opts ...grpc.CallOption) (*validation.GlooValidationServiceResponse, error) {
	if c.validate == nil {
		Fail("Validate was called unexpectedly")
	}
	return c.validate(ctx, in, opts...)
}

func acceptProxy(ctx context.Context, in *validation.GlooValidationServiceRequest) (*validation.GlooValidationServiceResponse, error) {
	var proxies []*gloov1.Proxy
	if in.Proxy != nil {
		proxies = []*gloov1.Proxy{in.Proxy}
	} else {
		proxies = samples.SimpleGlooSnapshot("gloo-system").Proxies
	}

	var validationReports []*validation.ValidationReport
	for _, proxy := range proxies {
		proxyReport := validationutils.MakeReport(proxy)
		validationReports = append(validationReports, &validation.ValidationReport{
			Proxy:       proxy,
			ProxyReport: proxyReport,
		})
	}
	return &validation.GlooValidationServiceResponse{
		ValidationReports: validationReports,
	}, nil
}

func failProxy(ctx context.Context, in *validation.GlooValidationServiceRequest) (*validation.GlooValidationServiceResponse, error) {
	var proxies []*gloov1.Proxy
	if in.Proxy != nil {
		proxies = []*gloov1.Proxy{in.Proxy}
	} else {
		proxies = samples.SimpleGlooSnapshot("gloo-system").Proxies
	}

	var validationReports []*validation.ValidationReport
	for _, proxy := range proxies {
		proxyReport := validationutils.MakeReport(proxy)
		validationutils.AppendListenerError(proxyReport.ListenerReports[0], validation.ListenerReport_Error_SSLConfigError, "you should try harder next time")

		validationReports = append(validationReports, &validation.ValidationReport{
			Proxy:       proxy,
			ProxyReport: proxyReport,
		})
	}
	return &validation.GlooValidationServiceResponse{
		ValidationReports: validationReports,
	}, nil
}

func warnProxy(ctx context.Context, in *validation.GlooValidationServiceRequest) (*validation.GlooValidationServiceResponse, error) {
	var proxies []*gloov1.Proxy
	if in.Proxy != nil {
		proxies = []*gloov1.Proxy{in.Proxy}
	} else {
		proxies = samples.SimpleGlooSnapshot("gloo-system").Proxies
	}

	var validationReports []*validation.ValidationReport
	for _, proxy := range proxies {
		proxyReport := validationutils.MakeReport(proxy)
		validationutils.AppendRouteWarning(proxyReport.ListenerReports[0].GetHttpListenerReport().GetVirtualHostReports()[0].GetRouteReports()[0], validation.RouteReport_Warning_InvalidDestinationWarning, "you should try harder next time")

		validationReports = append(validationReports, &validation.ValidationReport{
			Proxy:       proxy,
			ProxyReport: proxyReport,
		})
	}
	return &validation.GlooValidationServiceResponse{
		ValidationReports: validationReports,
	}, nil
}
