package deployer_test

import (
	"context"
	"fmt"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gateway2/controller/scheme"
	"github.com/solo-io/gloo/projects/gateway2/deployer"
	gw2_v1alpha1 "github.com/solo-io/gloo/projects/gateway2/pkg/api/gateway.gloo.solo.io/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	api "sigs.k8s.io/gateway-api/apis/v1"
)

type clientObjects []client.Object

func (objs *clientObjects) deployment() *appsv1.Deployment {
	for _, obj := range *objs {
		if dep, ok := obj.(*appsv1.Deployment); ok {
			return dep
		}
	}
	return nil
}
func (objs *clientObjects) serviceAccount() *corev1.ServiceAccount {
	for _, obj := range *objs {
		if sa, ok := obj.(*corev1.ServiceAccount); ok {
			return sa
		}
	}
	return nil
}
func (objs *clientObjects) service() *corev1.Service {
	for _, obj := range *objs {
		if svc, ok := obj.(*corev1.Service); ok {
			return svc
		}
	}
	return nil
}
func (objs *clientObjects) configMap() *corev1.ConfigMap {
	for _, obj := range *objs {
		if cm, ok := obj.(*corev1.ConfigMap); ok {
			return cm
		}
	}
	return nil
}

var _ = Describe("Deployer", func() {
	var (
		d *deployer.Deployer

		gwc *api.GatewayClass
	)
	BeforeEach(func() {
		var err error

		gwc = &api.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: wellknown.GatewayClassName,
			},
			Spec: api.GatewayClassSpec{
				ControllerName: wellknown.GatewayControllerName,
			},
		}
		d, err = deployer.NewDeployer(newFakeClientWithObjs(gwc), &deployer.Inputs{
			ControllerName: wellknown.GatewayControllerName,
			Dev:            false,
			ControlPlane: bootstrap.ControlPlane{
				Kube: bootstrap.KubernetesControlPlaneConfig{XdsHost: "something.cluster.local", XdsPort: 1234},
			},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should get gvks", func() {
		gvks, err := d.GetGvksToWatch(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(gvks).NotTo(BeEmpty())
	})

	It("support segmenting by release", func() {
		d1, err := deployer.NewDeployer(newFakeClientWithObjs(gwc), &deployer.Inputs{
			ControllerName: wellknown.GatewayControllerName,
			Dev:            false,
			ControlPlane: bootstrap.ControlPlane{
				Kube: bootstrap.KubernetesControlPlaneConfig{XdsHost: "something.cluster.local", XdsPort: 1234},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		d2, err := deployer.NewDeployer(newFakeClientWithObjs(gwc), &deployer.Inputs{
			ControllerName: wellknown.GatewayControllerName,
			Dev:            false,
			ControlPlane: bootstrap.ControlPlane{
				Kube: bootstrap.KubernetesControlPlaneConfig{XdsHost: "something.cluster.local", XdsPort: 1234},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		gw1 := &api.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
				UID:       "1235",
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Gateway",
				APIVersion: "gateway.solo.io/v1beta1",
			},
			Spec: api.GatewaySpec{
				GatewayClassName: wellknown.GatewayClassName,
			},
		}

		gw2 := &api.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bar",
				Namespace: "default",
				UID:       "1235",
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Gateway",
				APIVersion: "gateway.solo.io/v1beta1",
			},
			Spec: api.GatewaySpec{
				GatewayClassName: wellknown.GatewayClassName,
			},
		}

		objs1, err := d1.GetObjsToDeploy(context.Background(), gw1)
		Expect(err).NotTo(HaveOccurred())
		Expect(objs1).NotTo(BeEmpty())
		objs2, err := d2.GetObjsToDeploy(context.Background(), gw2)
		Expect(err).NotTo(HaveOccurred())
		Expect(objs2).NotTo(BeEmpty())

		for _, obj := range objs1 {
			Expect(obj.GetName()).To(Equal("gloo-proxy-foo"))
		}
		for _, obj := range objs2 {
			Expect(obj.GetName()).To(Equal("gloo-proxy-bar"))
		}

	})

	Context("Single gwc and gw", func() {
		type input struct {
			dInputs        *deployer.Inputs
			gwc            *api.GatewayClass
			gw             *api.Gateway
			gwp            *gw2_v1alpha1.GatewayParameters
			glooSvc        *corev1.Service
			arbitrarySetup func()
		}

		type expectedOutput struct {
			err            error
			validationFunc func(objs clientObjects) error
		}

		var (
			defaultGwpName = "default-gateway-params"
			defaultGlooSvc = func() *corev1.Service {
				return &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gloo",
						Namespace: "gloo-system",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name: "grpc-xds",
								Port: 1234,
							},
						},
					},
				}
			}
			defaultGatewayClass = func() *api.GatewayClass {
				return &api.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: wellknown.GatewayClassName,
					},
					Spec: api.GatewayClassSpec{
						ControllerName: wellknown.GatewayControllerName,
					},
				}
			}
			defaultDeployerInputs = func() *deployer.Inputs {
				return &deployer.Inputs{
					ControllerName: wellknown.GatewayControllerName,
					Dev:            false,
				}
			}
			defaultGateway = func() *api.Gateway {
				return &api.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "default",
						UID:       "1235",
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       "Gateway",
						APIVersion: "gateway.solo.io/v1beta1",
					},
					Spec: api.GatewaySpec{
						GatewayClassName: wellknown.GatewayClassName,
						Listeners: []api.Listener{
							{
								Name: "listener-1",
								Port: 80,
							},
						},
					},
				}
			}
			defaultGatewayParams = func() *gw2_v1alpha1.GatewayParameters {
				return &gw2_v1alpha1.GatewayParameters{
					TypeMeta: metav1.TypeMeta{
						Kind: gw2_v1alpha1.GatewayParametersGVK.Kind,
						// The parsing expects GROUP/VERSION format in this field
						APIVersion: fmt.Sprintf("%s/%s", gw2_v1alpha1.GatewayParametersGVK.Group, gw2_v1alpha1.GatewayParametersGVK.Version),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      defaultGwpName,
						Namespace: "default",
						UID:       "1236",
					},
					Spec: gw2_v1alpha1.GatewayParametersSpec{
						ProxyConfig: &gw2_v1alpha1.ProxyConfig{
							EnvironmentType: &gw2_v1alpha1.ProxyConfig_Kube{
								Kube: &gw2_v1alpha1.KubernetesProxyConfig{
									WorkloadType: &gw2_v1alpha1.KubernetesProxyConfig_Deployment{
										Deployment: &gw2_v1alpha1.ProxyDeployment{
											Replicas: &wrappers.UInt32Value{Value: 3},
										},
									},
									EnvoyContainer: &gw2_v1alpha1.EnvoyContainer{
										Bootstrap: &gw2_v1alpha1.EnvoyBootstrap{
											LogLevel:          "debug",
											ComponentLogLevel: "router:info,listener:warn",
										},
									},
								},
							},
						},
					},
				}
			}
			defaultGatewayWithGatewayParams = func(gwpName string) *api.Gateway {
				gw := defaultGateway()
				gw.Annotations = map[string]string{
					wellknown.GatewayParametersAnnotationName: gwpName,
				}

				return gw
			}
			defaultInput = func() *input {
				return &input{
					dInputs: defaultDeployerInputs(),
					gwc:     defaultGatewayClass(),
					glooSvc: defaultGlooSvc(),
					gw:      defaultGateway(),
					gwp:     defaultGatewayParams(),
				}
			}
		)
		DescribeTable("create and validate objs", func(inp *input, expected *expectedOutput) {
			// run break-glass setup
			if inp.arbitrarySetup != nil {
				inp.arbitrarySetup()
			}

			d, err := deployer.NewDeployer(newFakeClientWithObjs(inp.gwc, inp.glooSvc, inp.gwp), inp.dInputs)
			// We don't have any interesting error cases in the NewDeployer to test for but if we get
			// some then we will need to handle those outside the table or be more clever about expected
			// errors
			Expect(err).NotTo(HaveOccurred())

			objs, err := d.GetObjsToDeploy(context.Background(), inp.gw)
			if expected.err != nil {
				Expect(err).To(MatchError(expected.err))
				// return here since we matched our expected error
				return
			} else {
				Expect(err).NotTo(HaveOccurred())
			}

			// handle custom test validation func
			Expect(expected.validationFunc(objs)).NotTo(HaveOccurred())
		},
			Entry("No GatewayParameters", &input{
				dInputs: defaultDeployerInputs(),
				gwc:     defaultGatewayClass(),
				glooSvc: defaultGlooSvc(),
				gw:      defaultGateway(),
				gwp:     &gw2_v1alpha1.GatewayParameters{},
			}, &expectedOutput{
				validationFunc: func(objs clientObjects) error {
					Expect(objs).NotTo(BeEmpty())
					// Check we have Deployment, ConfigMap, ServiceAccount, Service
					// TODO(jbohanon): validate everything??
					Expect(objs).To(HaveLen(4))
					return nil
				},
			}),
			Entry("GatewayParameters overrides", &input{
				dInputs: defaultDeployerInputs(),
				gwc:     defaultGatewayClass(),
				glooSvc: defaultGlooSvc(),
				gw:      defaultGatewayWithGatewayParams(defaultGwpName),
				gwp:     defaultGatewayParams(),
			}, &expectedOutput{
				err: nil,
				validationFunc: func(objs clientObjects) error {
					Expect(objs).NotTo(BeEmpty())
					// Check we have Deployment, ConfigMap, ServiceAccount, Service
					Expect(objs).To(HaveLen(4))
					return nil
				},
			}),
			Entry("correct deployment with sds enabled", &input{
				dInputs: &deployer.Inputs{
					Dev:            false,
					ControllerName: "foo",
					IstioValues: bootstrap.IstioValues{
						SDSEnabled: true,
					},
				},
				gwc:     defaultGatewayClass(),
				glooSvc: defaultGlooSvc(),
				gw:      defaultGateway(),
				gwp:     defaultGatewayParams(),
			}, &expectedOutput{
				validationFunc: func(objs clientObjects) error {
					Expect(objs.deployment().Spec.Template.Spec.Containers).To(HaveLen(3))
					return nil
				},
			}),
			Entry("no listeners on gateway", &input{
				dInputs: defaultDeployerInputs(),
				gwc:     defaultGatewayClass(),
				glooSvc: defaultGlooSvc(),
				gw: &api.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "default",
						UID:       "1235",
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       "Gateway",
						APIVersion: "gateway.solo.io/v1beta1",
					},
					Spec: api.GatewaySpec{
						GatewayClassName: "gloo-gateway",
					},
				},
				gwp: defaultGatewayParams(),
			}, &expectedOutput{
				validationFunc: func(objs clientObjects) error {
					Expect(objs).NotTo(BeEmpty())
					return nil
				},
			}),
			Entry("port offset", defaultInput(), &expectedOutput{
				validationFunc: func(objs clientObjects) error {
					svc := objs.service()
					Expect(svc).NotTo(BeNil())

					port := svc.Spec.Ports[0]
					Expect(port.Port).To(Equal(int32(80)))
					Expect(port.TargetPort.IntVal).To(Equal(int32(8080)))
					return nil
				},
			}),
			Entry("duplicate ports", &input{
				dInputs: defaultDeployerInputs(),
				gwc:     defaultGatewayClass(),
				glooSvc: defaultGlooSvc(),
				gw: &api.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "default",
						UID:       "1235",
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       "Gateway",
						APIVersion: "gateway.solo.io/v1beta1",
					},
					Spec: api.GatewaySpec{
						GatewayClassName: "gloo-gateway",
						Listeners: []api.Listener{
							{
								Name: "listener-1",
								Port: 80,
							},
							{
								Name: "listener-2",
								Port: 80,
							},
						},
					},
				},
				gwp: defaultGatewayParams(),
			}, &expectedOutput{
				validationFunc: func(objs clientObjects) error {
					svc := objs.service()
					Expect(svc).NotTo(BeNil())

					Expect(svc.Spec.Ports).To(HaveLen(1))
					port := svc.Spec.Ports[0]
					Expect(port.Port).To(Equal(int32(80)))
					Expect(port.TargetPort.IntVal).To(Equal(int32(8080)))
					return nil
				},
			}),
			Entry("propagates version.Version to deployment", &input{
				dInputs:        defaultDeployerInputs(),
				gwc:            defaultGatewayClass(),
				glooSvc:        defaultGlooSvc(),
				gw:             defaultGateway(),
				arbitrarySetup: func() { version.Version = "testversion" },
				gwp:            defaultGatewayParams(),
			}, &expectedOutput{
				validationFunc: func(objs clientObjects) error {
					dep := objs.deployment()
					Expect(dep).NotTo(BeNil())
					Expect(dep.Spec.Template.Spec.Containers).NotTo(BeEmpty())
					for _, c := range dep.Spec.Template.Spec.Containers {
						Expect(c.Image).To(HaveSuffix(":testversion"))
					}
					return nil
				},
			}),
			Entry("object owner refs are set", defaultInput(), &expectedOutput{
				validationFunc: func(objs clientObjects) error {
					Expect(objs).NotTo(BeEmpty())

					gw := defaultGateway()

					for _, obj := range objs {
						ownerRefs := obj.GetOwnerReferences()
						Expect(ownerRefs).To(HaveLen(1))
						Expect(ownerRefs[0].Name).To(Equal(gw.Name))
						Expect(ownerRefs[0].UID).To(Equal(gw.UID))
						Expect(ownerRefs[0].Kind).To(Equal(gw.Kind))
						Expect(ownerRefs[0].APIVersion).To(Equal(gw.APIVersion))
						Expect(*ownerRefs[0].Controller).To(BeTrue())
					}
					return nil
				},
			}),
			Entry("envoy yaml is valid", defaultInput(), &expectedOutput{
				validationFunc: func(objs clientObjects) error {
					gw := defaultGateway()
					Expect(objs).NotTo(BeEmpty())

					cm := objs.configMap()
					Expect(cm).NotTo(BeNil())

					envoyYaml := cm.Data["envoy.yaml"]
					Expect(envoyYaml).NotTo(BeEmpty())

					// make sure it's valid yaml
					var envoyConfig map[string]any
					err := yaml.Unmarshal([]byte(envoyYaml), &envoyConfig)
					Expect(err).NotTo(HaveOccurred(), "envoy config is not valid yaml: %s", envoyYaml)

					// make sure the envoy node metadata looks right
					node := envoyConfig["node"].(map[string]any)
					Expect(node).To(HaveKeyWithValue("metadata", map[string]any{
						"gateway": map[string]any{
							"name":      gw.Name,
							"namespace": gw.Namespace,
						},
					}))
					return nil
				},
			}),
			// Entry("no gateway class", &input{
			// 	dInputs: defaultDeployerInputs(),
			// 	gwc:     defaultGatewayClass(),
			// 	glooSvc: defaultGlooSvc(),
			// 	gw: &api.Gateway{
			// 		ObjectMeta: metav1.ObjectMeta{
			// 			Name:      "foo",
			// 			Namespace: "default",
			// 			UID:       "1235",
			// 		},
			// 		TypeMeta: metav1.TypeMeta{
			// 			Kind:       "Gateway",
			// 			APIVersion: "gateway.solo.io/v1beta1",
			// 		},
			// 		Spec: api.GatewaySpec{
			// 			Listeners: []api.Listener{
			// 				{
			// 					Name: "listener-1",
			// 					Port: 80,
			// 				},
			// 			},
			// 		},
			// 	},
			// }, &expectedOutput{
			// 	err: deployer.NoGatewayClassError,
			// }),
			// Entry("failed to get gateway class", &input{
			// 	dInputs: defaultDeployerInputs(),
			// 	gwc:     defaultGatewayClass(),
			// 	glooSvc: defaultGlooSvc(),
			// 	gw: &api.Gateway{
			// 		ObjectMeta: metav1.ObjectMeta{
			// 			Name:      "foo",
			// 			Namespace: "default",
			// 			UID:       "1235",
			// 		},
			// 		TypeMeta: metav1.TypeMeta{
			// 			Kind:       "Gateway",
			// 			APIVersion: "gateway.solo.io/v1beta1",
			// 		},
			// 		Spec: api.GatewaySpec{
			// 			GatewayClassName: "wrong-gatewayclass",
			// 			Listeners: []api.Listener{
			// 				{
			// 					Name: "listener-1",
			// 					Port: 80,
			// 				},
			// 			},
			// 		},
			// 	},
			// }, &expectedOutput{
			// 	err: deployer.GetGatewayClassError,
			// }),
			Entry("failed to get GatewayParameters", &input{
				dInputs: defaultDeployerInputs(),
				gwc:     defaultGatewayClass(),
				glooSvc: defaultGlooSvc(),
				gw:      defaultGatewayWithGatewayParams("bad-gwp"),
				gwp:     &gw2_v1alpha1.GatewayParameters{},
			}, &expectedOutput{
				err: deployer.GetGatewayParametersError,
			}),
		)
	})
})

// initialize a fake controller-runtime client with the given list of objects
func newFakeClientWithObjs(objs ...client.Object) client.Client {
	s := scheme.NewScheme()
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}
