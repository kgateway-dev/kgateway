package deployer_test

import (
	"context"
	"fmt"
	"slices"

	"github.com/solo-io/gloo/pkg/schemes"

	envoy_config_bootstrap "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/solo-io/gloo/pkg/version"
	gw2_v1alpha1 "github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/deployer"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	api "sigs.k8s.io/gateway-api/apis/v1"
)

// testBootstrap implements resources.Resource in order to use protoutils.UnmarshalYAML
// this is hacky but it seems more stable/concise than map-casting all the way down
// to the field we need.
type testBootstrap struct {
	envoy_config_bootstrap.Bootstrap
}

func (t *testBootstrap) SetMetadata(meta *core.Metadata) {}

func (t *testBootstrap) Equal(_ any) bool {
	return false
}

func (t *testBootstrap) GetMetadata() *core.Metadata {
	return nil
}

type clientObjects []client.Object

func (objs *clientObjects) findDeployment(namespace, name string) *appsv1.Deployment {
	for _, obj := range *objs {
		if dep, ok := obj.(*appsv1.Deployment); ok {
			if dep.Name == name && dep.Namespace == namespace {
				return dep
			}
		}
	}
	return nil
}

func (objs *clientObjects) findServiceAccount(namespace, name string) *corev1.ServiceAccount {
	for _, obj := range *objs {
		if sa, ok := obj.(*corev1.ServiceAccount); ok {
			if sa.Name == name && sa.Namespace == namespace {
				return sa
			}
		}
	}
	return nil
}

func (objs *clientObjects) findService(namespace, name string) *corev1.Service {
	for _, obj := range *objs {
		if svc, ok := obj.(*corev1.Service); ok {
			if svc.Name == name && svc.Namespace == namespace {
				return svc
			}
		}
	}
	return nil
}

func (objs *clientObjects) findConfigMap(namespace, name string) *corev1.ConfigMap {
	for _, obj := range *objs {
		if cm, ok := obj.(*corev1.ConfigMap); ok {
			if cm.Name == name && cm.Namespace == namespace {
				return cm
			}
		}
	}
	return nil
}

func (objs *clientObjects) getEnvoyConfig(namespace, name string) *testBootstrap {
	cm := objs.findConfigMap(namespace, name).Data
	var bootstrapCfg testBootstrap
	err := protoutils.UnmarshalYAML([]byte(cm["envoy.yaml"]), &bootstrapCfg)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return &bootstrapCfg
}

var _ = Describe("Deployer", func() {
	const (
		defaultNamespace = "default"
	)
	var (
		d *deployer.Deployer

		// Note that this is NOT meant to reflect the actual defaults defined in install/helm/gloo/templates/43-gatewayparameters.yaml
		defaultGatewayParams = func() *gw2_v1alpha1.GatewayParameters {
			return &gw2_v1alpha1.GatewayParameters{
				TypeMeta: metav1.TypeMeta{
					Kind: "GatewayParameters",
					// The parsing expects GROUP/VERSION format in this field
					APIVersion: gw2_v1alpha1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      wellknown.DefaultGatewayParametersName,
					Namespace: defaultNamespace,
					UID:       "1237",
				},
				Spec: gw2_v1alpha1.GatewayParametersSpec{
					Kube: &gw2_v1alpha1.KubernetesProxyConfig{
						Deployment: &gw2_v1alpha1.ProxyDeployment{
							Replicas: ptr.To(uint32(2)),
						},
						EnvoyContainer: &gw2_v1alpha1.EnvoyContainer{
							Bootstrap: &gw2_v1alpha1.EnvoyBootstrap{
								LogLevel: ptr.To("debug"),
								ComponentLogLevels: map[string]string{
									"router":   "info",
									"listener": "warn",
								},
							},
							Image: &gw2_v1alpha1.Image{
								Registry:   ptr.To("scooby"),
								Repository: ptr.To("dooby"),
								Tag:        ptr.To("doo"),
								PullPolicy: ptr.To(corev1.PullAlways),
							},
						},
						PodTemplate: &gw2_v1alpha1.Pod{
							ExtraAnnotations: map[string]string{
								"foo": "bar",
							},
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:  ptr.To(int64(1)),
								RunAsGroup: ptr.To(int64(2)),
							},
						},
						Service: &gw2_v1alpha1.Service{
							Type:      ptr.To(corev1.ServiceTypeClusterIP),
							ClusterIP: ptr.To("99.99.99.99"),
							ExtraAnnotations: map[string]string{
								"foo": "bar",
							},
						},
						Stats: &gw2_v1alpha1.StatsConfig{
							Enabled:                 ptr.To(true),
							RoutePrefixRewrite:      ptr.To("/stats/prometheus"),
							EnableStatsRoute:        ptr.To(true),
							StatsRoutePrefixRewrite: ptr.To("/stats"),
						},
					},
				},
			}
		}

		selfManagedGatewayParam = func(name string) *gw2_v1alpha1.GatewayParameters {
			return &gw2_v1alpha1.GatewayParameters{
				TypeMeta: metav1.TypeMeta{
					Kind: "GatewayParameters",
					// The parsing expects GROUP/VERSION format in this field
					APIVersion: gw2_v1alpha1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: defaultNamespace,
					UID:       "1237",
				},
				Spec: gw2_v1alpha1.GatewayParametersSpec{
					SelfManaged: &gw2_v1alpha1.SelfManagedGateway{},
				},
			}
		}
	)

	Context("default case", func() {

		It("should work with empty params", func() {
			gwc := &api.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: wellknown.GatewayClassName,
				},
				Spec: api.GatewayClassSpec{
					ControllerName: wellknown.GatewayControllerName,
					ParametersRef: &api.ParametersReference{
						Group:     "gateway.gloo.solo.io",
						Kind:      "GatewayParameters",
						Name:      wellknown.DefaultGatewayParametersName,
						Namespace: ptr.To(api.Namespace(defaultNamespace)),
					},
				},
			}
			gwParams := &gw2_v1alpha1.GatewayParameters{
				TypeMeta: metav1.TypeMeta{
					Kind: "GatewayParameters",
					// The parsing expects GROUP/VERSION format in this field
					APIVersion: gw2_v1alpha1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      wellknown.DefaultGatewayParametersName,
					Namespace: defaultNamespace,
					UID:       "1237",
				},
			}
			d, err := deployer.NewDeployer(newFakeClientWithObjs(gwc, gwParams), &deployer.Inputs{
				ControllerName: wellknown.GatewayControllerName,
				Dev:            false,
				ControlPlane: bootstrap.ControlPlane{
					Kube: bootstrap.KubernetesControlPlaneConfig{XdsHost: "something.cluster.local", XdsPort: 1234},
				},
			})

			gw := &api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: defaultNamespace,
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

			Expect(err).NotTo(HaveOccurred())
			gvks, err := d.GetObjsToDeploy(context.Background(), gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(gvks).NotTo(BeEmpty())
		})
	})

	Context("special cases", func() {
		var gwc *api.GatewayClass
		BeforeEach(func() {
			gwc = &api.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: wellknown.GatewayClassName,
				},
				Spec: api.GatewayClassSpec{
					ControllerName: wellknown.GatewayControllerName,
					ParametersRef: &api.ParametersReference{
						Group:     "gateway.gloo.solo.io",
						Kind:      "GatewayParameters",
						Name:      wellknown.DefaultGatewayParametersName,
						Namespace: ptr.To(api.Namespace(defaultNamespace)),
					},
				},
			}
			var err error

			d, err = deployer.NewDeployer(newFakeClientWithObjs(gwc, defaultGatewayParams()), &deployer.Inputs{
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
			d1, err := deployer.NewDeployer(newFakeClientWithObjs(gwc, defaultGatewayParams()), &deployer.Inputs{
				ControllerName: wellknown.GatewayControllerName,
				Dev:            false,
				ControlPlane: bootstrap.ControlPlane{
					Kube: bootstrap.KubernetesControlPlaneConfig{XdsHost: "something.cluster.local", XdsPort: 1234},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			d2, err := deployer.NewDeployer(newFakeClientWithObjs(gwc, defaultGatewayParams()), &deployer.Inputs{
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
					Namespace: defaultNamespace,
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
					Namespace: defaultNamespace,
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

			proxyName := func(name string) string {
				return fmt.Sprintf("gloo-proxy-%s", name)
			}
			var objs1, objs2 clientObjects
			objs1, err = d1.GetObjsToDeploy(context.Background(), gw1)
			Expect(err).NotTo(HaveOccurred())
			Expect(objs1).NotTo(BeEmpty())
			Expect(objs1.findDeployment(defaultNamespace, proxyName(gw1.Name))).ToNot(BeNil())
			Expect(objs1.findService(defaultNamespace, proxyName(gw1.Name))).ToNot(BeNil())
			Expect(objs1.findConfigMap(defaultNamespace, proxyName(gw1.Name))).ToNot(BeNil())
			// Expect(objs1.findServiceAccount("default")).ToNot(BeNil())
			objs2, err = d2.GetObjsToDeploy(context.Background(), gw2)
			Expect(err).NotTo(HaveOccurred())
			Expect(objs2).NotTo(BeEmpty())
			Expect(objs2.findDeployment(defaultNamespace, proxyName(gw2.Name))).ToNot(BeNil())
			Expect(objs2.findService(defaultNamespace, proxyName(gw2.Name))).ToNot(BeNil())
			Expect(objs2.findConfigMap(defaultNamespace, proxyName(gw2.Name))).ToNot(BeNil())
			// Expect(objs2.findServiceAccount("default")).ToNot(BeNil())

			for _, obj := range objs1 {
				Expect(obj.GetName()).To(Equal("gloo-proxy-foo"))
			}
			for _, obj := range objs2 {
				Expect(obj.GetName()).To(Equal("gloo-proxy-bar"))
			}
		})
	})

	Context("Single gwc and gw", func() {
		type input struct {
			dInputs        *deployer.Inputs
			gw             *api.Gateway
			defaultGwp     *gw2_v1alpha1.GatewayParameters
			overrideGwp    *gw2_v1alpha1.GatewayParameters
			gwc            *api.GatewayClass
			arbitrarySetup func()
		}

		type expectedOutput struct {
			getObjsErr     error
			newDeployerErr error
			validationFunc func(objs clientObjects, inp *input) error
		}

		var (
			gwpOverrideName       = "default-gateway-params"
			defaultDeployerInputs = func() *deployer.Inputs {
				return &deployer.Inputs{
					ControllerName: wellknown.GatewayControllerName,
					Dev:            false,
					ControlPlane: bootstrap.ControlPlane{
						Kube: bootstrap.KubernetesControlPlaneConfig{XdsHost: "something.cluster.local", XdsPort: 1234},
					},
				}
			}
			istioEnabledDeployerInputs = func() *deployer.Inputs {
				inp := defaultDeployerInputs()
				inp.IstioValues = bootstrap.IstioValues{
					IntegrationEnabled: true,
				}
				return inp
			}
			defaultGateway = func() *api.Gateway {
				return &api.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: defaultNamespace,
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
			defaultGatewayClass = func() *api.GatewayClass {
				return &api.GatewayClass{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name: wellknown.GatewayClassName,
					},
					Spec: api.GatewayClassSpec{
						ControllerName: wellknown.GatewayControllerName,
						ParametersRef: &api.ParametersReference{
							Group:     "gateway.gloo.solo.io",
							Kind:      "GatewayParameters",
							Name:      wellknown.DefaultGatewayParametersName,
							Namespace: ptr.To(api.Namespace(defaultNamespace)),
						},
					},
				}
			}
			defaultGatewayParamsOverride = func() *gw2_v1alpha1.GatewayParameters {
				return &gw2_v1alpha1.GatewayParameters{
					TypeMeta: metav1.TypeMeta{
						Kind: "GatewayParameters",
						// The parsing expects GROUP/VERSION format in this field
						APIVersion: gw2_v1alpha1.GroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      gwpOverrideName,
						Namespace: defaultNamespace,
						UID:       "1236",
					},
					Spec: gw2_v1alpha1.GatewayParametersSpec{
						Kube: &gw2_v1alpha1.KubernetesProxyConfig{
							Deployment: &gw2_v1alpha1.ProxyDeployment{
								Replicas: ptr.To(uint32(3)),
							},
							EnvoyContainer: &gw2_v1alpha1.EnvoyContainer{
								Bootstrap: &gw2_v1alpha1.EnvoyBootstrap{
									LogLevel: ptr.To("debug"),
									ComponentLogLevels: map[string]string{
										"router":   "info",
										"listener": "warn",
									},
								},
								Image: &gw2_v1alpha1.Image{
									Registry:   ptr.To("foo"),
									Repository: ptr.To("bar"),
									Tag:        ptr.To("bat"),
									PullPolicy: ptr.To(corev1.PullAlways),
								},
							},
							PodTemplate: &gw2_v1alpha1.Pod{
								ExtraAnnotations: map[string]string{
									"foo": "bar",
								},
								SecurityContext: &corev1.PodSecurityContext{
									RunAsUser:  ptr.To(int64(3)),
									RunAsGroup: ptr.To(int64(4)),
								},
							},
							Service: &gw2_v1alpha1.Service{
								Type:      ptr.To(corev1.ServiceTypeClusterIP),
								ClusterIP: ptr.To("99.99.99.99"),
								ExtraAnnotations: map[string]string{
									"foo": "bar",
								},
							},
						},
					},
				}
			}
			gatewayParamsOverrideWithSds = func() *gw2_v1alpha1.GatewayParameters {
				return &gw2_v1alpha1.GatewayParameters{
					TypeMeta: metav1.TypeMeta{
						Kind: "GatewayParameters",
						// The parsing expects GROUP/VERSION format in this field
						APIVersion: gw2_v1alpha1.GroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      gwpOverrideName,
						Namespace: defaultNamespace,
						UID:       "1236",
					},
					Spec: gw2_v1alpha1.GatewayParametersSpec{
						Kube: &gw2_v1alpha1.KubernetesProxyConfig{
							SdsContainer: &gw2_v1alpha1.SdsContainer{
								Image: &gw2_v1alpha1.Image{
									Registry:   ptr.To("foo"),
									Repository: ptr.To("bar"),
									Tag:        ptr.To("baz"),
								},
							},
							Istio: &gw2_v1alpha1.IstioIntegration{
								IstioProxyContainer: &gw2_v1alpha1.IstioContainer{
									Image: &gw2_v1alpha1.Image{
										Registry:   ptr.To("scooby"),
										Repository: ptr.To("dooby"),
										Tag:        ptr.To("doo"),
									},
									IstioDiscoveryAddress: ptr.To("can't"),
									IstioMetaMeshId:       ptr.To("be"),
									IstioMetaClusterId:    ptr.To("overridden"),
								},
							},
							AiExtension: &gw2_v1alpha1.AiExtension{
								Enabled: ptr.To(true),
								Image: &gw2_v1alpha1.Image{
									Registry:   ptr.To("foo"),
									Repository: ptr.To("bar"),
									Tag:        ptr.To("baz"),
								},
								Ports: []*corev1.ContainerPort{
									{
										Name:          "foo",
										ContainerPort: 80,
									},
								},
							},
						},
					},
				}
			}
			gatewayParamsOverrideWithSdsAndFloatingUserId = func() *gw2_v1alpha1.GatewayParameters {
				params := gatewayParamsOverrideWithSds()
				params.Spec.Kube.FloatingUserId = ptr.To(true)
				return params
			}
			gatewayParamsOverrideWithoutStats = func() *gw2_v1alpha1.GatewayParameters {
				return &gw2_v1alpha1.GatewayParameters{
					TypeMeta: metav1.TypeMeta{
						Kind: "GatewayParameters",
						// The parsing expects GROUP/VERSION format in this field
						APIVersion: gw2_v1alpha1.GroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      gwpOverrideName,
						Namespace: defaultNamespace,
						UID:       "1236",
					},
					Spec: gw2_v1alpha1.GatewayParametersSpec{

						Kube: &gw2_v1alpha1.KubernetesProxyConfig{
							Stats: &gw2_v1alpha1.StatsConfig{
								Enabled:          ptr.To(false),
								EnableStatsRoute: ptr.To(false),
							},
						},
					},
				}
			}
			fullyDefinedGatewayParams = func() *gw2_v1alpha1.GatewayParameters {
				return fullyDefinedGatewayParameters(wellknown.DefaultGatewayParametersName, defaultNamespace)
			}

			fullyDefinedGatewayParamsWithFloatingUserId = func() *gw2_v1alpha1.GatewayParameters {
				params := fullyDefinedGatewayParameters(wellknown.DefaultGatewayParametersName, defaultNamespace)
				params.Spec.Kube.FloatingUserId = ptr.To(true)
				return params
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
					dInputs:    defaultDeployerInputs(),
					gw:         defaultGateway(),
					defaultGwp: defaultGatewayParams(),
					gwc:        defaultGatewayClass(),
				}
			}
			defaultDeploymentName     = fmt.Sprintf("gloo-proxy-%s", defaultGateway().Name)
			defaultConfigMapName      = defaultDeploymentName
			defaultServiceName        = defaultDeploymentName
			defaultServiceAccountName = defaultDeploymentName

			validateGatewayParametersPropagation = func(objs clientObjects, gwp *gw2_v1alpha1.GatewayParameters) error {
				expectedGwp := gwp.Spec.Kube
				Expect(objs).NotTo(BeEmpty())
				// Check we have Deployment, ConfigMap, ServiceAccount, Service
				Expect(objs).To(HaveLen(4))
				dep := objs.findDeployment(defaultNamespace, defaultDeploymentName)
				Expect(dep).ToNot(BeNil())
				Expect(dep.Spec.Replicas).ToNot(BeNil())
				Expect(*dep.Spec.Replicas).To(Equal(int32(*expectedGwp.Deployment.Replicas)))
				expectedImage := fmt.Sprintf("%s/%s",
					*expectedGwp.EnvoyContainer.Image.Registry,
					*expectedGwp.EnvoyContainer.Image.Repository,
				)
				Expect(dep.Spec.Template.Spec.Containers[0].Image).To(ContainSubstring(expectedImage))
				if expectedTag := expectedGwp.EnvoyContainer.Image.Tag; *expectedTag != "" {
					Expect(dep.Spec.Template.Spec.Containers[0].Image).To(ContainSubstring(":" + *expectedTag))
				} else {
					Expect(dep.Spec.Template.Spec.Containers[0].Image).To(ContainSubstring(":" + version.Version))
				}
				Expect(dep.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(Equal(*expectedGwp.EnvoyContainer.Image.PullPolicy))
				Expect(dep.Spec.Template.Annotations).To(matchers.ContainMapElements(expectedGwp.PodTemplate.ExtraAnnotations))
				Expect(dep.Spec.Template.Annotations).To(HaveKeyWithValue("prometheus.io/scrape", "true"))
				Expect(dep.Spec.Template.Spec.SecurityContext.RunAsUser).To(Equal(expectedGwp.PodTemplate.SecurityContext.RunAsUser))
				Expect(dep.Spec.Template.Spec.SecurityContext.RunAsGroup).To(Equal(expectedGwp.PodTemplate.SecurityContext.RunAsGroup))

				svc := objs.findService(defaultNamespace, defaultServiceName)
				Expect(svc).ToNot(BeNil())
				Expect(svc.GetAnnotations()).ToNot(BeNil())
				Expect(svc.Annotations).To(matchers.ContainMapElements(expectedGwp.Service.ExtraAnnotations))
				Expect(svc.Spec.Type).To(Equal(*expectedGwp.Service.Type))
				Expect(svc.Spec.ClusterIP).To(Equal(*expectedGwp.Service.ClusterIP))

				sa := objs.findServiceAccount(defaultNamespace, defaultServiceAccountName)
				Expect(sa).ToNot(BeNil())

				cm := objs.findConfigMap(defaultNamespace, defaultConfigMapName)
				Expect(cm).ToNot(BeNil())

				logLevelsMap := expectedGwp.EnvoyContainer.Bootstrap.ComponentLogLevels
				levels := []types.GomegaMatcher{}
				for k, v := range logLevelsMap {
					levels = append(levels, ContainSubstring(fmt.Sprintf("%s:%s", k, v)))
				}

				argsMatchers := []interface{}{
					"--log-level",
					*expectedGwp.EnvoyContainer.Bootstrap.LogLevel,
					"--component-log-level",
					And(levels...),
				}

				Expect(objs.findDeployment(defaultNamespace, defaultDeploymentName).Spec.Template.Spec.Containers[0].Args).To(ContainElements(
					argsMatchers...,
				))
				return nil
			}
		)

		// fullyDefinedValidationWithoutRunAsUser doesn't check "runAsUser"
		fullyDefinedValidationWithoutRunAsUser := func(objs clientObjects, inp *input) error {
			expectedGwp := inp.defaultGwp.Spec.Kube
			Expect(objs).NotTo(BeEmpty())
			// Check we have Deployment, ConfigMap, ServiceAccount, Service
			Expect(objs).To(HaveLen(4))
			dep := objs.findDeployment(defaultNamespace, defaultDeploymentName)
			Expect(dep).ToNot(BeNil())
			Expect(dep.Spec.Replicas).ToNot(BeNil())
			Expect(*dep.Spec.Replicas).To(Equal(int32(*expectedGwp.Deployment.Replicas)))

			Expect(dep.Spec.Template.Annotations).To(matchers.ContainMapElements(expectedGwp.PodTemplate.ExtraAnnotations))

			// assert envoy container
			expectedEnvoyImage := fmt.Sprintf("%s/%s",
				*expectedGwp.EnvoyContainer.Image.Registry,
				*expectedGwp.EnvoyContainer.Image.Repository,
			)
			envoyContainer := dep.Spec.Template.Spec.Containers[0]
			Expect(envoyContainer.Image).To(ContainSubstring(expectedEnvoyImage))
			if expectedTag := expectedGwp.EnvoyContainer.Image.Tag; *expectedTag != "" {
				Expect(envoyContainer.Image).To(ContainSubstring(":" + *expectedTag))
			} else {
				Expect(envoyContainer.Image).To(ContainSubstring(":" + version.Version))
			}
			Expect(envoyContainer.ImagePullPolicy).To(Equal(*expectedGwp.EnvoyContainer.Image.PullPolicy))
			Expect(envoyContainer.Resources.Limits.Cpu()).To(Equal(expectedGwp.EnvoyContainer.Resources.Limits.Cpu()))
			Expect(envoyContainer.Resources.Requests.Cpu()).To(Equal(expectedGwp.EnvoyContainer.Resources.Requests.Cpu()))

			// assert sds container
			expectedSdsImage := fmt.Sprintf("%s/%s",
				*expectedGwp.SdsContainer.Image.Registry,
				*expectedGwp.SdsContainer.Image.Repository,
			)
			sdsContainer := dep.Spec.Template.Spec.Containers[1]
			Expect(sdsContainer.Image).To(ContainSubstring(expectedSdsImage))
			if expectedTag := expectedGwp.SdsContainer.Image.Tag; *expectedTag != "" {
				Expect(sdsContainer.Image).To(ContainSubstring(":" + *expectedTag))
			} else {
				Expect(sdsContainer.Image).To(ContainSubstring(":" + version.Version))
			}
			Expect(sdsContainer.ImagePullPolicy).To(Equal(*expectedGwp.SdsContainer.Image.PullPolicy))
			Expect(sdsContainer.Resources.Limits.Cpu()).To(Equal(expectedGwp.SdsContainer.Resources.Limits.Cpu()))
			Expect(sdsContainer.Resources.Requests.Cpu()).To(Equal(expectedGwp.SdsContainer.Resources.Requests.Cpu()))
			idx := slices.IndexFunc(sdsContainer.Env, func(e corev1.EnvVar) bool {
				return e.Name == "LOG_LEVEL"
			})
			Expect(idx).ToNot(Equal(-1))
			Expect(sdsContainer.Env[idx].Value).To(Equal(*expectedGwp.SdsContainer.Bootstrap.LogLevel))

			// assert istio container
			istioExpectedImage := fmt.Sprintf("%s/%s",
				*expectedGwp.Istio.IstioProxyContainer.Image.Registry,
				*expectedGwp.Istio.IstioProxyContainer.Image.Repository,
			)
			istioContainer := dep.Spec.Template.Spec.Containers[2]
			Expect(istioContainer.Image).To(ContainSubstring(istioExpectedImage))
			if expectedTag := expectedGwp.Istio.IstioProxyContainer.Image.Tag; *expectedTag != "" {
				Expect(istioContainer.Image).To(ContainSubstring(":" + *expectedTag))
			} else {
				Expect(istioContainer.Image).To(ContainSubstring(":" + version.Version))
			}
			Expect(istioContainer.ImagePullPolicy).To(Equal(*expectedGwp.Istio.IstioProxyContainer.Image.PullPolicy))
			Expect(istioContainer.Resources.Limits.Cpu()).To(Equal(expectedGwp.Istio.IstioProxyContainer.Resources.Limits.Cpu()))
			Expect(istioContainer.Resources.Requests.Cpu()).To(Equal(expectedGwp.Istio.IstioProxyContainer.Resources.Requests.Cpu()))
			// TODO: assert on istio args (e.g. log level, istio meta fields, etc)

			// assert AI extension container
			expectedAIExtension := fmt.Sprintf("%s/%s",
				*expectedGwp.AiExtension.Image.Registry,
				*expectedGwp.AiExtension.Image.Repository,
			)
			aiExt := dep.Spec.Template.Spec.Containers[3]
			Expect(aiExt.Image).To(ContainSubstring(expectedAIExtension))
			Expect(aiExt.Ports).To(HaveLen(len(expectedGwp.AiExtension.Ports)))

			// assert Service
			svc := objs.findService(defaultNamespace, defaultServiceName)
			Expect(svc).ToNot(BeNil())
			Expect(svc.GetAnnotations()).ToNot(BeNil())
			Expect(svc.Annotations).To(matchers.ContainMapElements(expectedGwp.Service.ExtraAnnotations))
			Expect(svc.Spec.Type).To(Equal(*expectedGwp.Service.Type))
			Expect(svc.Spec.ClusterIP).To(Equal(*expectedGwp.Service.ClusterIP))

			sa := objs.findServiceAccount(defaultNamespace, defaultServiceAccountName)
			Expect(sa).ToNot(BeNil())

			cm := objs.findConfigMap(defaultNamespace, defaultConfigMapName)
			Expect(cm).ToNot(BeNil())

			logLevelsMap := expectedGwp.EnvoyContainer.Bootstrap.ComponentLogLevels
			levels := []types.GomegaMatcher{}
			for k, v := range logLevelsMap {
				levels = append(levels, ContainSubstring(fmt.Sprintf("%s:%s", k, v)))
			}

			argsMatchers := []interface{}{
				"--log-level",
				*expectedGwp.EnvoyContainer.Bootstrap.LogLevel,
				"--component-log-level",
				And(levels...),
			}

			Expect(objs.findDeployment(defaultNamespace, defaultDeploymentName).Spec.Template.Spec.Containers[0].Args).To(ContainElements(
				argsMatchers...,
			))
			return nil
		}

		fullyDefinedValidation := func(objs clientObjects, inp *input) error {
			err := fullyDefinedValidationWithoutRunAsUser(objs, inp)
			if err != nil {
				return err
			}

			expectedGwp := inp.defaultGwp.Spec.Kube
			dep := objs.findDeployment(defaultNamespace, defaultDeploymentName)
			Expect(dep.Spec.Template.Spec.SecurityContext.RunAsUser).To(Equal(expectedGwp.PodTemplate.SecurityContext.RunAsUser))

			sdsContainer := dep.Spec.Template.Spec.Containers[1]
			Expect(sdsContainer.SecurityContext.RunAsUser).To(Equal(expectedGwp.SdsContainer.SecurityContext.RunAsUser))

			istioContainer := dep.Spec.Template.Spec.Containers[2]
			Expect(istioContainer.SecurityContext.RunAsUser).To(Equal(expectedGwp.Istio.IstioProxyContainer.SecurityContext.RunAsUser))

			return nil
		}

		fullyDefinedValidationFloatingUserId := func(objs clientObjects, inp *input) error {
			err := fullyDefinedValidationWithoutRunAsUser(objs, inp)
			if err != nil {
				return err
			}

			// Security contexts may be nil if unsetting runAsUser results in the a nil-equivalent object
			// This is fine, as it leaves the runAsUser value undet as desired
			dep := objs.findDeployment(defaultNamespace, defaultDeploymentName)
			if dep.Spec.Template.Spec.SecurityContext != nil {
				Expect(dep.Spec.Template.Spec.SecurityContext.RunAsUser).To(BeNil())
			}

			envoyContainer := dep.Spec.Template.Spec.Containers[0]
			if envoyContainer.SecurityContext != nil {
				Expect(envoyContainer.SecurityContext.RunAsUser).To(BeNil())
			}

			sdsContainer := dep.Spec.Template.Spec.Containers[1]
			if sdsContainer.SecurityContext != nil {
				Expect(sdsContainer.SecurityContext.RunAsUser).To(BeNil())
			}

			istioContainer := dep.Spec.Template.Spec.Containers[2]
			if istioContainer.SecurityContext != nil {
				Expect(istioContainer.SecurityContext.RunAsUser).To(BeNil())
			}

			return nil
		}

		generalAiAndSdsValidationFunc := func(objs clientObjects, inp *input, expectNullRunAsUser bool) error {
			containers := objs.findDeployment(defaultNamespace, defaultDeploymentName).Spec.Template.Spec.Containers
			Expect(containers).To(HaveLen(4))
			var foundGw, foundSds, foundIstioProxy, foundAIExtension bool
			var sdsContainer, istioProxyContainer, aiContainer, gwContainer corev1.Container
			for _, container := range containers {
				switch container.Name {
				case "sds":
					sdsContainer = container
					foundSds = true
				case "istio-proxy":
					istioProxyContainer = container
					foundIstioProxy = true
				case "gloo-gateway":
					gwContainer = container
					foundGw = true
				case "gloo-ai-extension":
					aiContainer = container
					foundAIExtension = true
				default:
					Fail("unknown container name " + container.Name)
				}
			}
			Expect(foundGw).To(BeTrue())
			Expect(foundSds).To(BeTrue())
			Expect(foundIstioProxy).To(BeTrue())
			Expect(foundAIExtension).To(BeTrue())

			if expectNullRunAsUser {
				if sdsContainer.SecurityContext != nil {
					Expect(sdsContainer.SecurityContext.RunAsUser).To(BeNil())
				}

				if gwContainer.SecurityContext != nil {
					Expect(gwContainer.SecurityContext.RunAsUser).To(BeNil())
				}

				if istioProxyContainer.SecurityContext != nil {
					Expect(istioProxyContainer.SecurityContext.RunAsUser).To(BeNil())
				}

				if aiContainer.SecurityContext != nil {
					Expect(aiContainer.SecurityContext.RunAsUser).To(BeNil())
				}
			}

			bootstrapCfg := objs.getEnvoyConfig(defaultNamespace, defaultConfigMapName)
			clusters := bootstrapCfg.GetStaticResources().GetClusters()
			Expect(clusters).ToNot(BeNil())
			Expect(clusters).To(ContainElement(HaveField("Name", "gateway_proxy_sds")))

			sdsImg := inp.overrideGwp.Spec.Kube.SdsContainer.Image
			Expect(sdsContainer.Image).To(Equal(fmt.Sprintf("%s/%s:%s", *sdsImg.Registry, *sdsImg.Repository, *sdsImg.Tag)))
			istioProxyImg := inp.overrideGwp.Spec.Kube.Istio.IstioProxyContainer.Image
			Expect(istioProxyContainer.Image).To(Equal(fmt.Sprintf("%s/%s:%s", *istioProxyImg.Registry, *istioProxyImg.Repository, *istioProxyImg.Tag)))

			return nil
		}

		aiAndSdsValidationFunc := func(objs clientObjects, inp *input) error {
			return generalAiAndSdsValidationFunc(objs, inp, false) // false: don't expect null runAsUser
		}

		aiSdsAndFloatingUserIdValidationFunc := func(objs clientObjects, inp *input) error {
			return generalAiAndSdsValidationFunc(objs, inp, true) // true: don't expect null runAsUser
		}

		DescribeTable("create and validate objs", func(inp *input, expected *expectedOutput) {
			checkErr := func(err, expectedErr error) (shouldReturn bool) {
				GinkgoHelper()
				if expectedErr != nil {
					Expect(err).To(MatchError(expectedErr))
					return true
				}
				Expect(err).NotTo(HaveOccurred())
				return false
			}

			// run break-glass setup
			if inp.arbitrarySetup != nil {
				inp.arbitrarySetup()
			}

			// Catch nil objs so the fake client doesn't choke
			gwc := inp.gwc
			if gwc == nil {
				gwc = defaultGatewayClass()
			}

			// default these to empty objects so we can test behavior when one or both
			// resources don't exist
			defaultGwp := inp.defaultGwp
			if defaultGwp == nil {
				defaultGwp = &gw2_v1alpha1.GatewayParameters{}
			}
			overrideGwp := inp.overrideGwp
			if overrideGwp == nil {
				overrideGwp = &gw2_v1alpha1.GatewayParameters{}
			}

			d, err := deployer.NewDeployer(newFakeClientWithObjs(gwc, defaultGwp, overrideGwp), inp.dInputs)
			if checkErr(err, expected.newDeployerErr) {
				return
			}

			objs, err := d.GetObjsToDeploy(context.Background(), inp.gw)
			if checkErr(err, expected.getObjsErr) {
				return
			}

			// handle custom test validation func
			Expect(expected.validationFunc(objs, inp)).NotTo(HaveOccurred())
		},
			Entry("No GatewayParameters falls back on default GatewayParameters", &input{
				dInputs:    defaultDeployerInputs(),
				gw:         defaultGateway(),
				defaultGwp: defaultGatewayParams(),
			}, &expectedOutput{
				validationFunc: func(objs clientObjects, inp *input) error {
					return validateGatewayParametersPropagation(objs, defaultGatewayParams())
				},
			}),
			Entry("GatewayParameters overrides", &input{
				dInputs:     defaultDeployerInputs(),
				gw:          defaultGatewayWithGatewayParams(gwpOverrideName),
				defaultGwp:  defaultGatewayParams(),
				overrideGwp: defaultGatewayParamsOverride(),
			}, &expectedOutput{
				validationFunc: func(objs clientObjects, inp *input) error {
					return validateGatewayParametersPropagation(objs, inp.overrideGwp)
				},
			}),
			Entry("Fully defined GatewayParameters", &input{
				dInputs:    istioEnabledDeployerInputs(),
				gw:         defaultGateway(),
				defaultGwp: fullyDefinedGatewayParams(),
			}, &expectedOutput{
				validationFunc: fullyDefinedValidation,
			}),
			Entry("Fully defined GatewayParameters with floating user id", &input{
				dInputs:    istioEnabledDeployerInputs(),
				gw:         defaultGateway(),
				defaultGwp: fullyDefinedGatewayParamsWithFloatingUserId(),
			}, &expectedOutput{
				validationFunc: fullyDefinedValidationFloatingUserId,
			}),
			Entry("correct deployment with sds and AI extension enabled", &input{
				dInputs:     istioEnabledDeployerInputs(),
				gw:          defaultGatewayWithGatewayParams(gwpOverrideName),
				defaultGwp:  defaultGatewayParams(),
				overrideGwp: gatewayParamsOverrideWithSds(),
			}, &expectedOutput{
				validationFunc: aiAndSdsValidationFunc,
			}),
			Entry("correct deployment with sds, AI extension, and floatinguUserId enabled", &input{
				dInputs:     istioEnabledDeployerInputs(),
				gw:          defaultGatewayWithGatewayParams(gwpOverrideName),
				defaultGwp:  defaultGatewayParams(),
				overrideGwp: gatewayParamsOverrideWithSdsAndFloatingUserId(),
			}, &expectedOutput{
				validationFunc: aiSdsAndFloatingUserIdValidationFunc,
			}),
			Entry("no listeners on gateway", &input{
				dInputs: defaultDeployerInputs(),
				gw: &api.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: defaultNamespace,
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
				defaultGwp: defaultGatewayParams(),
			}, &expectedOutput{
				validationFunc: func(objs clientObjects, inp *input) error {
					Expect(objs).NotTo(BeEmpty())
					return nil
				},
			}),
			Entry("port offset", defaultInput(), &expectedOutput{
				validationFunc: func(objs clientObjects, inp *input) error {
					svc := objs.findService(defaultNamespace, defaultServiceName)
					Expect(svc).NotTo(BeNil())

					port := svc.Spec.Ports[0]
					Expect(port.Port).To(Equal(int32(80)))
					Expect(port.TargetPort.IntVal).To(Equal(int32(8080)))
					return nil
				},
			}),
			Entry("duplicate ports", &input{
				dInputs: defaultDeployerInputs(),
				gw: &api.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: defaultNamespace,
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
				defaultGwp: defaultGatewayParams(),
			}, &expectedOutput{
				validationFunc: func(objs clientObjects, inp *input) error {
					svc := objs.findService(defaultNamespace, defaultServiceName)
					Expect(svc).NotTo(BeNil())

					Expect(svc.Spec.Ports).To(HaveLen(1))
					port := svc.Spec.Ports[0]
					Expect(port.Port).To(Equal(int32(80)))
					Expect(port.TargetPort.IntVal).To(Equal(int32(8080)))
					return nil
				},
			}),
			Entry("object owner refs are set", defaultInput(), &expectedOutput{
				validationFunc: func(objs clientObjects, inp *input) error {
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
				validationFunc: func(objs clientObjects, inp *input) error {
					gw := defaultGateway()
					Expect(objs).NotTo(BeEmpty())

					cm := objs.findConfigMap(defaultNamespace, defaultConfigMapName)
					Expect(cm).NotTo(BeNil())

					envoyYaml := cm.Data["envoy.yaml"]
					Expect(envoyYaml).NotTo(BeEmpty())

					// make sure it's valid yaml
					var envoyConfig map[string]any
					err := yaml.Unmarshal([]byte(envoyYaml), &envoyConfig)
					Expect(err).NotTo(HaveOccurred(), "envoy config is not valid yaml: %s", envoyYaml)

					// make sure the envoy node metadata looks right
					node := envoyConfig["node"].(map[string]any)
					proxyName := fmt.Sprintf("%s-%s", gw.Namespace, gw.Name)
					Expect(node).To(HaveKeyWithValue("metadata", map[string]any{
						xds.RoleKey: fmt.Sprintf("%s~%s~%s", glooutils.GatewayApiProxyValue, gw.Namespace, proxyName),
					}))

					// make sure the stats listener is enabled
					staticResources := envoyConfig["static_resources"].(map[string]any)
					listeners := staticResources["listeners"].([]interface{})
					var prometheusListener map[string]any
					for _, lis := range listeners {
						lis := lis.(map[string]any)
						lisName := lis["name"]
						if lisName == "prometheus_listener" {
							prometheusListener = lis
							break
						}
					}
					Expect(prometheusListener).NotTo(BeNil())

					return nil
				},
			}),
			Entry("envoy yaml is valid with stats disabled", &input{
				dInputs:     defaultDeployerInputs(),
				gw:          defaultGatewayWithGatewayParams(gwpOverrideName),
				defaultGwp:  defaultGatewayParams(),
				overrideGwp: gatewayParamsOverrideWithoutStats(),
				gwc:         defaultGatewayClass(),
			}, &expectedOutput{
				validationFunc: func(objs clientObjects, inp *input) error {
					gw := defaultGatewayWithGatewayParams(gwpOverrideName)
					Expect(objs).NotTo(BeEmpty())

					cm := objs.findConfigMap(defaultNamespace, defaultConfigMapName)
					Expect(cm).NotTo(BeNil())

					envoyYaml := cm.Data["envoy.yaml"]
					Expect(envoyYaml).NotTo(BeEmpty())

					// make sure it's valid yaml
					var envoyConfig map[string]any
					err := yaml.Unmarshal([]byte(envoyYaml), &envoyConfig)
					Expect(err).NotTo(HaveOccurred(), "envoy config is not valid yaml: %s", envoyYaml)

					// make sure the envoy node metadata looks right
					node := envoyConfig["node"].(map[string]any)
					proxyName := fmt.Sprintf("%s-%s", gw.Namespace, gw.Name)
					Expect(node).To(HaveKeyWithValue("metadata", map[string]any{
						xds.RoleKey: fmt.Sprintf("%s~%s~%s", glooutils.GatewayApiProxyValue, gw.Namespace, proxyName),
					}))

					// make sure the stats listener is enabled
					staticResources := envoyConfig["static_resources"].(map[string]any)
					listeners := staticResources["listeners"].([]interface{})
					var prometheusListener map[string]any
					for _, lis := range listeners {
						lis := lis.(map[string]any)
						lisName := lis["name"]
						if lisName == "prometheus_listener" {
							prometheusListener = lis
							break
						}
					}
					Expect(prometheusListener).To(BeNil())

					return nil
				},
			}),
			Entry("failed to get GatewayParameters", &input{
				dInputs:    defaultDeployerInputs(),
				gw:         defaultGatewayWithGatewayParams("bad-gwp"),
				defaultGwp: defaultGatewayParams(),
			}, &expectedOutput{
				getObjsErr: deployer.GetGatewayParametersError,
			}),
			Entry("nil inputs to NewDeployer", &input{
				dInputs:    nil,
				gw:         defaultGateway(),
				defaultGwp: defaultGatewayParams(),
			}, &expectedOutput{
				newDeployerErr: deployer.NilDeployerInputsErr,
			}),
			Entry("No GatewayParameters override but default is self-managed; should not deploy gateway", &input{
				dInputs:    defaultDeployerInputs(),
				gw:         defaultGateway(),
				defaultGwp: selfManagedGatewayParam(wellknown.DefaultGatewayParametersName),
			}, &expectedOutput{
				validationFunc: func(objs clientObjects, inp *input) error {
					Expect(objs).To(BeEmpty())
					return nil
				},
			}),
			Entry("Self-managed GatewayParameters override; should not deploy gateway", &input{
				dInputs:     defaultDeployerInputs(),
				gw:          defaultGatewayWithGatewayParams("self-managed"),
				defaultGwp:  defaultGatewayParams(),
				overrideGwp: selfManagedGatewayParam("self-managed"),
			}, &expectedOutput{
				validationFunc: func(objs clientObjects, inp *input) error {
					Expect(objs).To(BeEmpty())
					return nil
				},
			}),
		)
	})
})

// initialize a fake controller-runtime client with the given list of objects
func newFakeClientWithObjs(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(schemes.DefaultScheme()).
		WithObjects(objs...).
		Build()
}

func fullyDefinedGatewayParameters(name, namespace string) *gw2_v1alpha1.GatewayParameters {
	return &gw2_v1alpha1.GatewayParameters{
		TypeMeta: metav1.TypeMeta{
			Kind: "GatewayParameters",
			// The parsing expects GROUP/VERSION format in this field
			APIVersion: gw2_v1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "1236",
		},
		Spec: gw2_v1alpha1.GatewayParametersSpec{
			Kube: &gw2_v1alpha1.KubernetesProxyConfig{
				Deployment: &gw2_v1alpha1.ProxyDeployment{
					Replicas: ptr.To[uint32](3),
				},
				EnvoyContainer: &gw2_v1alpha1.EnvoyContainer{
					Bootstrap: &gw2_v1alpha1.EnvoyBootstrap{
						LogLevel: ptr.To("debug"),
						ComponentLogLevels: map[string]string{
							"router":   "info",
							"listener": "warn",
						},
					},
					Image: &gw2_v1alpha1.Image{
						Registry:   ptr.To("foo"),
						Repository: ptr.To("bar"),
						Tag:        ptr.To("bat"),
						PullPolicy: ptr.To(corev1.PullAlways),
					},
					SecurityContext: &corev1.SecurityContext{
						RunAsUser: ptr.To(int64(111)),
					},
					Resources: &corev1.ResourceRequirements{
						Limits:   corev1.ResourceList{"cpu": resource.MustParse("101m")},
						Requests: corev1.ResourceList{"cpu": resource.MustParse("103m")},
					},
				},
				SdsContainer: &gw2_v1alpha1.SdsContainer{
					Image: &gw2_v1alpha1.Image{
						Registry:   ptr.To("sds-registry"),
						Repository: ptr.To("sds-repository"),
						Tag:        ptr.To("sds-tag"),
						Digest:     ptr.To("sds-digest"),
						PullPolicy: ptr.To(corev1.PullAlways),
					},
					SecurityContext: &corev1.SecurityContext{
						RunAsUser: ptr.To(int64(222)),
					},
					Resources: &corev1.ResourceRequirements{
						Limits:   corev1.ResourceList{"cpu": resource.MustParse("201m")},
						Requests: corev1.ResourceList{"cpu": resource.MustParse("203m")},
					},
					Bootstrap: &gw2_v1alpha1.SdsBootstrap{
						LogLevel: ptr.To("debug"),
					},
				},
				PodTemplate: &gw2_v1alpha1.Pod{
					ExtraAnnotations: map[string]string{
						"pod-anno": "foo",
					},
					ExtraLabels: map[string]string{
						"pod-label": "foo",
					},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser: ptr.To(int64(333)),
					},
					ImagePullSecrets: []corev1.LocalObjectReference{{
						Name: "pod-image-pull-secret",
					}},
					NodeSelector: map[string]string{
						"pod-node-selector": "foo",
					},
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      "pod-affinity-nodeAffinity-required-expression-key",
										Operator: "pod-affinity-nodeAffinity-required-expression-operator",
										Values:   []string{"foo"},
									}},
									MatchFields: []corev1.NodeSelectorRequirement{{
										Key:      "pod-affinity-nodeAffinity-required-field-key",
										Operator: "pod-affinity-nodeAffinity-required-field-operator",
										Values:   []string{"foo"},
									}},
								}},
							},
						},
					},
					Tolerations: []*corev1.Toleration{{
						Key:               "pod-toleration-key",
						Operator:          "pod-toleration-operator",
						Value:             "pod-toleration-value",
						Effect:            "pod-toleration-effect",
						TolerationSeconds: ptr.To(int64(1)),
					}},
				},
				Service: &gw2_v1alpha1.Service{
					Type:      ptr.To(corev1.ServiceTypeClusterIP),
					ClusterIP: ptr.To("99.99.99.99"),
					ExtraAnnotations: map[string]string{
						"service-anno": "foo",
					},
					ExtraLabels: map[string]string{
						"service-label": "foo",
					},
				},
				Istio: &gw2_v1alpha1.IstioIntegration{
					IstioProxyContainer: &gw2_v1alpha1.IstioContainer{
						Image: &gw2_v1alpha1.Image{
							Registry:   ptr.To("istio-registry"),
							Repository: ptr.To("istio-repository"),
							Tag:        ptr.To("istio-tag"),
							Digest:     ptr.To("istio-digest"),
							PullPolicy: ptr.To(corev1.PullAlways),
						},
						SecurityContext: &corev1.SecurityContext{
							RunAsUser: ptr.To(int64(444)),
						},
						Resources: &corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("301m")},
							Requests: corev1.ResourceList{"cpu": resource.MustParse("303m")},
						},
						LogLevel:              ptr.To("debug"),
						IstioDiscoveryAddress: ptr.To("istioDiscoveryAddress"),
						IstioMetaMeshId:       ptr.To("istioMetaMeshId"),
						IstioMetaClusterId:    ptr.To("istioMetaClusterId"),
					},
				},
				AiExtension: &gw2_v1alpha1.AiExtension{
					Enabled: ptr.To(true),
					Ports: []*corev1.ContainerPort{
						{
							Name:          "foo",
							ContainerPort: 80,
						},
					},
					Image: &gw2_v1alpha1.Image{
						Registry:   ptr.To("ai-extension-registry"),
						Repository: ptr.To("ai-extension-repository"),
						Tag:        ptr.To("ai-extension-tag"),
						Digest:     ptr.To("ai-extension-digest"),
						PullPolicy: ptr.To(corev1.PullAlways),
					},
				},
			},
		},
	}
}
