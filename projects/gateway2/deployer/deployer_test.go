package deployer_test

import (
	"context"

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
			dpc            *gw2_v1alpha1.DataPlaneConfig
			glooSvc        *corev1.Service
			arbitrarySetup func()
		}

		type expectedOutput struct {
			newDeployerErr error
			convertErr     error
			objs           clientObjects
			validationFunc func(objs clientObjects) error
		}

		var (
			apiNamespace   api.Namespace = "gloo-system"
			defaultGlooSvc               = func() *corev1.Service {
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
						Name: "gloo-gateway",
					},
					Spec: api.GatewayClassSpec{
						ControllerName: "solo.io/gloo-gateway",
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
						GatewayClassName: "gloo-gateway",
						Listeners: []api.Listener{
							{
								Name: "listener-1",
								Port: 80,
							},
						},
					},
				}
			}
			defaultDataPlaneConfig = func() *gw2_v1alpha1.DataPlaneConfig {
				return &gw2_v1alpha1.DataPlaneConfig{
					TypeMeta: metav1.TypeMeta{
						Kind:       "DataPlaneConfig",
						APIVersion: "gateway.solo.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gloo-data-plane",
						Namespace: "gloo-system",
						UID:       "1236",
					},
					Spec: gw2_v1alpha1.DataPlaneConfigSpec{
						ProxyConfig: &gw2_v1alpha1.ProxyConfig{
							EnvironmentType: &gw2_v1alpha1.ProxyConfig_Kube{
								Kube: &gw2_v1alpha1.KubernetesProxyConfig{
									WorkloadType: nil,
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
			defaultInput = func() *input {
				return &input{
					dInputs: defaultDeployerInputs(),
					gwc:     defaultGatewayClass(),
					glooSvc: defaultGlooSvc(),
					gw:      defaultGateway(),
					dpc:     defaultDataPlaneConfig(),
				}
			}
		)
		DescribeTable("create and validate objs", func(inp *input, expected *expectedOutput) {
			// run break-glass setup
			if inp.arbitrarySetup != nil {
				inp.arbitrarySetup()
			}

			checkErr := func(err, expectedErr error) (shouldReturn bool) {
				if expectedErr != nil {
					Expect(err).To(MatchError(expectedErr))
					return true
				}
				Expect(err).NotTo(HaveOccurred())
				return false
			}

			d, err := deployer.NewDeployer(newFakeClientWithObjs(inp.gwc, inp.glooSvc), inp.dInputs)
			if checkErr(err, expected.newDeployerErr) {
				return
			}

			objs, err := d.GetObjsToDeploy(context.Background(), inp.gw)
			if checkErr(err, expected.convertErr) {
				return
			}

			if expected.objs != nil {
				// handle object comparisons
				Expect(objs).NotTo(BeEmpty())
				Expect(objs).To(HaveLen(len(expected.objs)))
				for i := range objs {
					// Not sure if this will work with interfaces
					Expect(objs[i]).To(BeIdenticalTo(expected.objs[i]))
				}
			} else {
				// handle custom test validation func
				Expect(expected.validationFunc(objs)).NotTo(HaveOccurred())
			}
		},
			// DO_NOT_SUBMIT until adding some validation here
			Entry("happypath", &input{
				dInputs: defaultDeployerInputs(),
				gwc:     defaultGatewayClass(),
				glooSvc: defaultGlooSvc(),
				gw:      defaultGateway(),
			}, &expectedOutput{
				newDeployerErr: nil,
				convertErr:     nil,
				objs:           nil,
				validationFunc: func(objs clientObjects) error {
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
			}, &expectedOutput{
				newDeployerErr: nil,
				convertErr:     nil,
				objs:           nil,
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
			Entry("no gloo svc", &input{
				dInputs: defaultDeployerInputs(),
				gwc:     defaultGatewayClass(),
				glooSvc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "whoopsie",
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
				},
				gw: defaultGateway(),
			}, &expectedOutput{
				convertErr: deployer.NoGlooSvcFoundError,
			}),
			Entry("no xds port", &input{
				dInputs: defaultDeployerInputs(),
				gwc:     defaultGatewayClass(),
				glooSvc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gloo",
						Namespace: "gloo-system",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name: "whoopsie",
								Port: 1234,
							},
						},
					},
				},
				gw: defaultGateway(),
			}, &expectedOutput{
				convertErr: deployer.NoXdsPortFoundError,
			}),
			Entry("no gateway class", &input{
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
						Listeners: []api.Listener{
							{
								Name: "listener-1",
								Port: 80,
							},
						},
					},
				},
			}, &expectedOutput{
				convertErr: deployer.NoGatewayClassError,
			}),
			Entry("failed to get gateway class", &input{
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
						GatewayClassName: "wrong-gatewayclass",
						Listeners: []api.Listener{
							{
								Name: "listener-1",
								Port: 80,
							},
						},
					},
				},
			}, &expectedOutput{
				convertErr: deployer.GetGatewayClassError,
			}),
			Entry("unsupported parametersRef Kind; bad group", &input{
				dInputs: defaultDeployerInputs(),
				gwc: &api.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gloo-gateway",
					},
					Spec: api.GatewayClassSpec{
						ControllerName: "solo.io/gloo-gateway",
						ParametersRef: &api.ParametersReference{
							Group: "foo",
							Kind:  api.Kind(gw2_v1alpha1.DataPlaneConfigGVK.Kind),
							Name:  "foo",
						},
					},
				},
				glooSvc: defaultGlooSvc(),
				gw:      defaultGateway(),
			}, &expectedOutput{
				convertErr: deployer.UnsupportedParametersRefKind,
			}),
			Entry("unsupported parametersRef Kind; bad kind", &input{
				dInputs: defaultDeployerInputs(),
				gwc: &api.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gloo-gateway",
					},
					Spec: api.GatewayClassSpec{
						ControllerName: "solo.io/gloo-gateway",
						ParametersRef: &api.ParametersReference{
							Group: api.Group(gw2_v1alpha1.DataPlaneConfigGVK.Group),
							Kind:  "foo",
							Name:  "foo",
						},
					},
				},
				glooSvc: defaultGlooSvc(),
				gw:      defaultGateway(),
			}, &expectedOutput{
				convertErr: deployer.UnsupportedParametersRefKind,
			}),
			Entry("failed to get dataplaneconfig", &input{
				dInputs: defaultDeployerInputs(),
				gwc: &api.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gloo-gateway",
					},
					Spec: api.GatewayClassSpec{
						ControllerName: "solo.io/gloo-gateway",
						ParametersRef: &api.ParametersReference{
							Group:     api.Group(gw2_v1alpha1.DataPlaneConfigGVK.Group),
							Kind:      api.Kind(gw2_v1alpha1.DataPlaneConfigGVK.Kind),
							Name:      "foo",
							Namespace: &apiNamespace,
						},
					},
				},
				glooSvc: defaultGlooSvc(),
				gw:      defaultGateway(),
				dpc:     defaultDataPlaneConfig(),
			}, &expectedOutput{
				convertErr: deployer.GetDataPlaneConfigError,
			}),
		)
	})

})

// initialize a fake controller-runtime client with the given list of objects
func newFakeClientWithObjs(objs ...client.Object) client.Client {
	s := scheme.NewScheme()
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}
