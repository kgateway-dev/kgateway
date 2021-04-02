package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sync"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/gloo/test/samples"
	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syamlutil "sigs.k8s.io/yaml"
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
		err = v.Sync(context.Background(), &gatewayv1.ApiSnapshot{})
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

			Context("allowWarnings=false", func() {
				BeforeEach(func() {
					v = NewValidator(NewValidatorConfig(t, vc, ns, true, false))
				})
				It("rejects a vs with missing route table ref", func() {
					vc.validateProxy = warnProxy
					us := samples.SimpleUpstream()
					snap := samples.GatewaySnapshotWithDelegates(us.Metadata.Ref(), ns)
					err := v.Sync(context.TODO(), snap)
					Expect(err).NotTo(HaveOccurred())

					// change something to change the hash
					snap.RouteTables[0].Metadata.Labels = map[string]string{"change": "my mind"}

					proxyReports, err := v.ValidateRouteTable(context.TODO(), snap.RouteTables[0], false)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Route Warning: InvalidDestinationWarning. Reason: you should try harder next time"))
					Expect(proxyReports).To(HaveLen(1))
				})
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
				Expect(err.Error()).To(ContainSubstring(
					RouteTableDeleteErr(nil, []*core.ResourceRef{snap.RouteTables[0].Metadata.Ref()}).Error()),
				)
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
					}).Error()))
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
				badRef := &core.ResourceRef{}

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
				proxyReports, merr := v.ValidateList(context.TODO(), toUnstructuredList(snap.VirtualServices[0]), false)
				Expect(merr.ErrorOrNil()).NotTo(HaveOccurred())
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

				proxyReports, merr := v.ValidateList(context.TODO(), toUnstructuredList(vs1, vs2), false)
				Expect(merr.ErrorOrNil()).NotTo(HaveOccurred())
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
				proxyReports, merr := v.ValidateList(context.TODO(), toUnstructuredList(snap.VirtualServices[0]), false)
				Expect(merr.ErrorOrNil()).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))
				Expect(proxyReports).To(HaveKey(ContainSubstring("listener-::-8080")))

				// repeat to ensure any hashing doesn't short circuit returning the proxies
				proxyReports, merr = v.ValidateList(context.TODO(), toUnstructuredList(snap.VirtualServices[0]), false)
				Expect(merr.ErrorOrNil()).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))
				Expect(proxyReports).To(HaveKey(ContainSubstring("listener-::-8080")))
			})
		})

		Context("unmarshal errors", func() {
			It("doesn't mask other errors when there's an unmarshal error in a list", func() {

				vc.validateProxy = acceptProxy
				us := samples.SimpleUpstream()
				snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				ul := &unstructured.UnstructuredList{}
				jsonBytes, err := ioutil.ReadFile("fixtures/unmarshal-err.json")
				Expect(err).ToNot(HaveOccurred())
				err = ul.UnmarshalJSON(jsonBytes)
				Expect(err).ToNot(HaveOccurred())
				proxyReports, merr := v.ValidateList(context.TODO(), ul, false)
				Expect(merr).To(HaveOccurred())
				Expect(merr.Errors).To(HaveLen(3))
				Expect(merr.Errors[0]).To(MatchError(ContainSubstring("route table gloo-system.i-dont-exist-rt missing")))
				Expect(merr.Errors[1]).To(MatchError(ContainSubstring("virtual service [gloo-system.invalid-vs-2] does not specify a virtual host")))
				Expect(merr.Errors[2]).To(MatchError(ContainSubstring("parsing resource from crd spec testproxy1-rt in namespace gloo-system into *v1.RouteTable")))
				Expect(merr.Errors[2]).To(MatchError(ContainSubstring("unknown field \"matcherss\" in gateway.solo.io.Route")))
				Expect(proxyReports).To(HaveLen(0))

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

				proxyReports, merr := v.ValidateList(context.TODO(), toUnstructuredList(vs2), false)
				Expect(merr.ErrorOrNil()).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))

				// create another virtual service to validate, should fail validation as a prior one should
				// already be in the validation snapshot cache with the same domain (as dry-run before was false)
				vs3 := &gatewayv1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs3)
				vs3.Metadata.Name = "vs3"

				proxyReports, merr = v.ValidateList(context.TODO(), toUnstructuredList(vs3), false)
				Expect(merr.ErrorOrNil()).To(HaveOccurred())
				Expect(merr.ErrorOrNil().Error()).To(ContainSubstring("could not render proxy"))
				Expect(merr.ErrorOrNil().Error()).To(ContainSubstring("domain conflict: the following"))
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

				proxyReports, merr := v.ValidateList(context.TODO(), toUnstructuredList(vs2), true)
				Expect(merr.ErrorOrNil()).NotTo(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))

				// create another virtual service to validate, should pass validation as a prior one should not
				// already be in the validation snapshot cache (as dry-run was true)
				vs3 := &gatewayv1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs3)
				vs3.Metadata.Name = "vs3"

				proxyReports, merr = v.ValidateList(context.TODO(), toUnstructuredList(vs3), true)
				Expect(merr.ErrorOrNil()).ToNot(HaveOccurred())
				Expect(proxyReports).To(HaveLen(1))
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
			vc.validateProxy = acceptProxy
			us := samples.SimpleUpstream()
			snap := samples.SimpleGatewaySnapshot(us.Metadata.Ref(), ns)
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

func warnProxy(ctx context.Context, in *validation.ProxyValidationServiceRequest, opts ...grpc.CallOption) (*validation.ProxyValidationServiceResponse, error) {
	rpt := validationutils.MakeReport(in.Proxy)
	validationutils.AppendRouteWarning(rpt.ListenerReports[0].GetHttpListenerReport().GetVirtualHostReports()[0].GetRouteReports()[0], validation.RouteReport_Warning_InvalidDestinationWarning, "you should try harder next time")
	return &validation.ProxyValidationServiceResponse{ProxyReport: rpt}, nil
}

func communicationErr(ctx context.Context, in *validation.ProxyValidationServiceRequest, opts ...grpc.CallOption) (*validation.ProxyValidationServiceResponse, error) {
	return nil, eris.Errorf("communication no good")
}
