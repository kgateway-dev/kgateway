package validation

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syamlutil "sigs.k8s.io/yaml"

	"github.com/solo-io/go-utils/testutils"

	"github.com/rotisserie/eris"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/gloo/test/samples"
	"google.golang.org/grpc"
)

var _ = Describe("Validator", func() {
	var (
		t  translator.Translator
		vc *mockValidationClient
		ns string
		v  *validator
	)
	BeforeEach(func() {
		t = translator.NewDefaultTranslator(translator.Opts{})
		vc = &mockValidationClient{}
		ns = "my-namespace"
		v = NewValidator(NewValidatorConfig(t, vc, ns, false, false))
	})
	It("returns error before sync called", func() {
		_, err := v.ValidateVirtualService(nil, nil, false)
		Expect(err).To(testutils.HaveInErrorChain(NotReadyErr))
		err = v.Sync(nil, &gatewayv1.ApiSnapshot{})
		Expect(err).NotTo(HaveOccurred())
	})

	Context("validating a route table", func() {
		Context("proxy validation accepted", func() {
			It("accepts the rt", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateRouteTable(context.TODO(), snap.RouteTables[0], false)
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))
			})

			It("accepts the rt and returns proxies each time", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateRouteTable(context.TODO(), snap.RouteTables[0], false)
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))
				Expect(proxyReports).To(HaveKey(ContainSubstring("listener-::-8080")))

				// repeat to ensure any hashing doesn't short circuit returning the proxies
				proxyReports, err = v.ValidateRouteTable(context.TODO(), snap.RouteTables[0], false)
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))
				Expect(proxyReports).To(HaveKey(ContainSubstring("listener-::-8080")))
			})
		})

		Context("proxy validation returns error", func() {
			It("rejects the rt", func() {
				vc.validateProxy = failProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// change something to change the hash
				snap.RouteTables[0].Metadata.Labels = map[string]string{"change": "my mind"}

				proxyReports, err := v.ValidateRouteTable(context.TODO(), snap.RouteTables[0], false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to validate Proxy with Gloo validation server"))
				Expect(proxyReports).To(HaveLen(1))
			})
		})

		Context("proxy validation fails (bad connection)", func() {
			Context("ignoreProxyValidation=false", func() {
				It("rejects the rt", func() {
					vc.validateProxy = communicationErr
					us := samples.SimpleUpstream()
					snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
					err := v.Sync(context.TODO(), snap)
					Expect(err).NotTo(HaveOccurred())

					// change something to change the hash
					snap.RouteTables[0].Metadata.Labels = map[string]string{"change": "my mind"}

					proxyReports, err := v.ValidateRouteTable(context.TODO(), snap.RouteTables[0], false)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to communicate with Gloo Proxy validation server"))
					Expect(proxyReports).To(BeEmpty())
				})
			})
			Context("ignoreProxyValidation=true", func() {
				It("accepts the rt", func() {
					vc.validateProxy = communicationErr
					v = NewValidator(NewValidatorConfig(t, vc, ns, true, false))
					us := samples.SimpleUpstream()
					snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
					err := v.Sync(context.TODO(), snap)
					Expect(err).NotTo(HaveOccurred())
					proxyReports, err := v.ValidateRouteTable(context.TODO(), snap.RouteTables[0], false)
					Expect(err).NotTo(HaveOccurred())
					Expect(proxyReports).To(HaveLen(0))
				})
			})
			Context("allowWarnings=true", func() {
				BeforeEach(func() {
					v = NewValidator(NewValidatorConfig(t, vc, ns, true, true))
				})
				It("accepts a vs with missing route table ref", func() {
					vc.validateProxy = communicationErr
					err := v.Sync(context.TODO(), &gatewayv1.ApiSnapshot{})
					Expect(err).NotTo(HaveOccurred())
					vs, _ := samples.LinkedRouteTablesWithVirtualService("vs", "ns")
					proxyReports, err := v.ValidateVirtualService(context.TODO(), vs, false)
					Expect(err).NotTo(HaveOccurred())
					Expect(proxyReports).To(HaveLen(0))
				})
				It("accepts a rt with missing route table ref", func() {
					vc.validateProxy = communicationErr
					err := v.Sync(context.TODO(), &gatewayv1.ApiSnapshot{})
					Expect(err).NotTo(HaveOccurred())
					_, rts := samples.LinkedRouteTablesWithVirtualService("vs", "ns")
					proxyReports, err := v.ValidateRouteTable(context.TODO(), rts[1], false)
					Expect(err).NotTo(HaveOccurred())
					Expect(proxyReports).To(HaveLen(0))
				})
				It("accepts delete leaf route table", func() {
					vc.validateProxy = communicationErr
					us := samples.SimpleUpstream()
					snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
					err := v.Sync(context.TODO(), snap)
					Expect(err).NotTo(HaveOccurred())

					ref := snap.RouteTables[len(snap.RouteTables)-1].Metadata.Ref()

					err = v.ValidateDeleteRouteTable(context.TODO(), ref, false)
					Expect(err).NotTo(HaveOccurred())
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
				vc.validateProxy = nil
				us := samples.SimpleUpstream()
				snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
				rt := snap.RouteTables[0].DeepCopyObject().(*gatewayv1.RouteTable)
				rt.Routes = append(rt.Routes, badRoute)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateRouteTable(context.TODO(), rt, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(proxyReports).To(HaveLen(0))
			})
		})
	})

	Context("delete a route table", func() {
		Context("has parents", func() {
			It("rejects deletion", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewaySnapshotWithDelegateChain(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				err = v.ValidateDeleteRouteTable(context.TODO(), snap.RouteTables[1].Metadata.Ref(), false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Deletion blocked because active Routes delegate to this Route Table. " +
					"Remove delegate actions to this route table from the virtual services: [] and the route tables: [{node-0 my-namespace}], then try again"))
			})
		})
		Context("has no parents", func() {
			It("deletes safely", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewaySnapshotWithDelegateChain(us.Metadata.Ref(), ns)
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
				vc.validateProxy = failProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// change something to change the hash
				snap.VirtualServices[0].Metadata.Labels = map[string]string{"change": "my mind"}

				proxyReports, err := v.ValidateVirtualService(context.TODO(), snap.VirtualServices[0], false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to validate Proxy with Gloo validation server"))
				Expect(proxyReports).To(HaveLen(1))

			})
		})
		Context("proxy validation accepted", func() {
			It("accepts the vs", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateVirtualService(context.TODO(), snap.VirtualServices[0], false)
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))
			})

			It("accepts the vs and returns proxies each time", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateVirtualService(context.TODO(), snap.VirtualServices[0], false)
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))
				Expect(proxyReports).To(HaveKey(ContainSubstring("listener-::-8080")))

				// repeat to ensure any hashing doesn't short circuit returning the proxies
				proxyReports, err = v.ValidateVirtualService(context.TODO(), snap.VirtualServices[0], false)
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))
				Expect(proxyReports).To(HaveKey(ContainSubstring("listener-::-8080")))
			})
		})
		Context("no gateways for virtual service", func() {
			It("accepts the vs", func() {
				vc.validateProxy = failProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				snap.Gateways.Each(func(element *gatewayv1.Gateway) {
					http, ok := element.GatewayType.(*gatewayv1.Gateway_HttpGateway)
					if !ok {
						return
					}
					http.HttpGateway.VirtualServiceSelector = map[string]string{"nobody": "hastheselabels"}

				})
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateVirtualService(context.TODO(), snap.VirtualServices[0], false)
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(0))
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
				vc.validateProxy = nil
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				vs := snap.VirtualServices[0].DeepCopyObject().(*gatewayv1.VirtualService)
				vs.VirtualHost.Routes = append(vs.VirtualHost.Routes, badRoute)

				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateVirtualService(context.TODO(), vs, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(proxyReports).To(HaveLen(0))
			})
		})

		Context("dry-run", func() {
			It("accepts the vs and rejects the second", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// create a virtual service to validate, should pass validation as a prior one should
				// already be in the validation snapshot cache with a different domain
				samples.AddVsToSnap(snap, us.GetMetadata().Ref(), "ns")
				vs2 := &gatewayv1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs2)
				vs2.Metadata.Name = "vs2"

				proxyReports, err := v.ValidateVirtualService(context.TODO(), vs2, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))

				// create another virtual service to validate, should fail validation as a prior one should
				// already be in the validation snapshot cache with the same domain (as dry-run before was false)
				vs3 := &gatewayv1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs3)
				vs3.Metadata.Name = "vs3"

				proxyReports, err = v.ValidateVirtualService(context.TODO(), vs3, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(err.Error()).To(ContainSubstring("domain conflict: the following"))
				Expect(proxyReports).To(HaveLen(0))
			})

			It("accepts the vs and accepts the second because of dry-run", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				samples.AddVsToSnap(snap, us.GetMetadata().Ref(), "ns")
				// create a virtual service to validate, should pass validation as a prior one should
				// already be in the validation snapshot cache with a different domain
				vs2 := &gatewayv1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs2)
				vs2.Metadata.Name = "vs2"

				proxyReports, err := v.ValidateVirtualService(context.TODO(), vs2, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))

				// create another virtual service to validate, should pass validation as a prior one should not
				// already be in the validation snapshot cache (as dry-run was true)
				vs3 := &gatewayv1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs3)
				vs3.Metadata.Name = "vs3"

				proxyReports, err = v.ValidateVirtualService(context.TODO(), vs3, true)
				Expect(err).ToNot(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))
			})
		})
	})

	Context("delete a virtual service", func() {
		Context("has parent gateways", func() {
			It("rejects deletion", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				ref := snap.VirtualServices[0].Metadata.Ref()
				snap.Gateways.Each(func(element *gatewayv1.Gateway) {
					http, ok := element.GatewayType.(*gatewayv1.Gateway_HttpGateway)
					if !ok {
						return
					}
					http.HttpGateway.VirtualServices = []core.ResourceRef{ref}
				})
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				err = v.ValidateDeleteVirtualService(context.TODO(), ref, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("Deletion blocked because active Gateways reference this Virtual Service. "+
					"Remove refs to this virtual service from the gateways: [{%s my-namespace} {%s-ssl my-namespace}], "+
					"then try again", defaults.GatewayProxyName, defaults.GatewayProxyName)))
			})
		})
		Context("has no parent gateways", func() {
			It("deletes safely", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
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
				vc.validateProxy = failProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// change something to change the hash
				snap.Gateways[0].Metadata.Labels = map[string]string{"change": "my mind"}

				proxyReports, err := v.ValidateGateway(context.TODO(), snap.Gateways[0], false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to validate Proxy with Gloo validation server"))
				Expect(proxyReports).To(HaveLen(1))
			})
		})
		Context("proxy validation accepted", func() {
			It("accepts the gw", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateGateway(context.TODO(), snap.Gateways[0], false)
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))
			})

			It("accepts the gateway and returns proxies each time", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateGateway(context.TODO(), snap.Gateways[0], false)
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))
				Expect(proxyReports).To(HaveKey(ContainSubstring("listener-::-8080")))

				// repeat to ensure any hashing doesn't short circuit returning the proxies
				proxyReports, err = v.ValidateGateway(context.TODO(), snap.Gateways[0], false)
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))
				Expect(proxyReports).To(HaveKey(ContainSubstring("listener-::-8080")))
			})
		})
		Context("gw rejected", func() {
			It("rejects the gw", func() {
				badRef := core.ResourceRef{}

				// validate proxy should never be called
				vc.validateProxy = nil
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				gw := snap.Gateways[0].DeepCopyObject().(*gatewayv1.Gateway)

				gw.GatewayType.(*gatewayv1.Gateway_HttpGateway).HttpGateway.VirtualServices = append(gw.GatewayType.(*gatewayv1.Gateway_HttpGateway).HttpGateway.VirtualServices, badRef)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateGateway(context.TODO(), gw, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(proxyReports).To(HaveLen(0))
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
				vc.validateProxy = failProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// change something to change the hash
				snap.VirtualServices[0].Metadata.Labels = map[string]string{"change": "my mind"}
				vsList := toUnstructuredList(snap.VirtualServices[0])

				proxyReports, err := v.ValidateList(context.TODO(), vsList, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to validate Proxy with Gloo validation server"))
				Expect(proxyReports).To(HaveLen(1))

			})
		})

		Context("proxy validation accepted", func() {

			It("accepts the vs list", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateList(context.TODO(), toUnstructuredList(snap.VirtualServices[0]), false)
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))
			})

			It("accepts the multi vs list", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
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

				proxyReports, err := v.ValidateList(context.TODO(), toUnstructuredList(vs1, vs2), false)
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(2))
			})

			It("rejects the multi vs list with overlapping domains", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				vs1 := &gatewayv1.VirtualService{}
				snap.VirtualServices[0].DeepCopyInto(vs1)
				vs1.Metadata.Name = "vs1"

				vs2 := &gatewayv1.VirtualService{}
				snap.VirtualServices[0].DeepCopyInto(vs2)
				vs2.Metadata.Name = "vs2"

				proxyReports, err := v.ValidateList(context.TODO(), toUnstructuredList(vs1, vs2), false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(err.Error()).To(ContainSubstring("domain conflict: the following"))
				Expect(proxyReports).To(HaveLen(0))
			})

			It("accepts the vs list and returns proxies each time", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateList(context.TODO(), toUnstructuredList(snap.VirtualServices[0]), false)
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))
				Expect(proxyReports).To(HaveKey(ContainSubstring("listener-::-8080")))

				// repeat to ensure any hashing doesn't short circuit returning the proxies
				proxyReports, err = v.ValidateList(context.TODO(), toUnstructuredList(snap.VirtualServices[0]), false)
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))
				Expect(proxyReports).To(HaveKey(ContainSubstring("listener-::-8080")))
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
				vc.validateProxy = nil
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				vs := snap.VirtualServices[0].DeepCopyObject().(*gatewayv1.VirtualService)
				vs.VirtualHost.Routes = append(vs.VirtualHost.Routes, badRoute)
				vsList := toUnstructuredList(vs)

				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				proxyReports, err := v.ValidateList(context.TODO(), vsList, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(proxyReports).To(HaveLen(0))
			})
		})

		Context("dry-run", func() {
			It("accepts the vs and rejects the second", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// create a virtual service to validate, should pass validation as a prior one should
				// already be in the validation snapshot cache with a different domain
				samples.AddVsToSnap(snap, us.GetMetadata().Ref(), "ns")

				vs2 := &gatewayv1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs2)
				vs2.Metadata.Name = "vs2"

				proxyReports, err := v.ValidateList(context.TODO(), toUnstructuredList(vs2), false)
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))

				// create another virtual service to validate, should fail validation as a prior one should
				// already be in the validation snapshot cache with the same domain (as dry-run before was false)
				vs3 := &gatewayv1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs3)
				vs3.Metadata.Name = "vs3"

				proxyReports, err = v.ValidateList(context.TODO(), toUnstructuredList(vs3), false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(err.Error()).To(ContainSubstring("domain conflict: the following"))
				Expect(proxyReports).To(HaveLen(0))
			})

			It("accepts the vs and accepts the second because of dry-run", func() {
				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				samples.AddVsToSnap(snap, us.GetMetadata().Ref(), "ns")
				// create a virtual service to validate, should pass validation as a prior one should
				// already be in the validation snapshot cache with a different domain
				vs2 := &gatewayv1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs2)
				vs2.Metadata.Name = "vs2"

				proxyReports, err := v.ValidateList(context.TODO(), toUnstructuredList(vs2), true)
				Expect(err).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))

				// create another virtual service to validate, should pass validation as a prior one should not
				// already be in the validation snapshot cache (as dry-run was true)
				vs3 := &gatewayv1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs3)
				vs3.Metadata.Name = "vs3"

				proxyReports, err = v.ValidateList(context.TODO(), toUnstructuredList(vs3), true)
				Expect(err).ToNot(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))
			})

		})

	})

})

type mockValidationClient struct {
	validateProxy func(ctx context.Context, in *validation.ProxyValidationServiceRequest, opts ...grpc.CallOption) (*validation.ProxyValidationServiceResponse, error)
}

func (c *mockValidationClient) NotifyOnResync(ctx context.Context, in *validation.NotifyOnResyncRequest, opts ...grpc.CallOption) (validation.ProxyValidationService_NotifyOnResyncClient, error) {
	return nil, nil
}

func (c *mockValidationClient) ValidateProxy(ctx context.Context, in *validation.ProxyValidationServiceRequest, opts ...grpc.CallOption) (*validation.ProxyValidationServiceResponse, error) {
	if c.validateProxy == nil {
		Fail("validateProxy was called unexpectedly")
	}
	return c.validateProxy(ctx, in, opts...)
}

func acceptProxy(ctx context.Context, in *validation.ProxyValidationServiceRequest, opts ...grpc.CallOption) (*validation.ProxyValidationServiceResponse, error) {
	return &validation.ProxyValidationServiceResponse{ProxyReport: validationutils.MakeReport(in.Proxy)}, nil
}

func failProxy(ctx context.Context, in *validation.ProxyValidationServiceRequest, opts ...grpc.CallOption) (*validation.ProxyValidationServiceResponse, error) {
	rpt := validationutils.MakeReport(in.Proxy)
	validationutils.AppendListenerError(rpt.ListenerReports[0], validation.ListenerReport_Error_SSLConfigError, "you should try harder next time")
	return &validation.ProxyValidationServiceResponse{ProxyReport: rpt}, nil
}

func communicationErr(ctx context.Context, in *validation.ProxyValidationServiceRequest, opts ...grpc.CallOption) (*validation.ProxyValidationServiceResponse, error) {
	return nil, eris.Errorf("communication no good")
}
