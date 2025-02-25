package deployer_test

import (
	"context"
	"fmt"
	"slices"

	envoy_config_bootstrap "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	jsonpb "google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	infextv1a1 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha1"
	api "sigs.k8s.io/gateway-api/apis/v1"

	gw2_v1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/deployer"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/internal/version"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"

	// TODO BML tests in this suite fail if this no-op import is not imported first.
	//
	// I know, I know, you're reading this, and you're skeptical. I can feel it.
	// Don't take my word for it.
	//
	// There is some import within this package that this suite relies on. Chasing that down is
	// *hard* tho due to the import tree, and best done in a followup.
	// _ "github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/translator"
	//
	// The above TODO is a result of proto types being registered for free somewhere through
	// the translator import. What we really need is to register all proto types, which is
	// "correctly" available to use via `envoyinit`; note that the autogeneration of these types
	// is currently broken. see: https://github.com/kgateway-dev/kgateway/issues/10491
	_ "github.com/kgateway-dev/kgateway/v2/internal/envoyinit/hack/filter_types"
)

// testBootstrap implements resources.Resource in order to use protoutils.UnmarshalYAML
// this is hacky but it seems more stable/concise than map-casting all the way down
// to the field we need.

func unmarshalYaml(data []byte, into proto.Message) error {
	jsn, err := yaml.YAMLToJSON(data)
	if err != nil {
		return err
	}

	var j jsonpb.UnmarshalOptions

	return j.Unmarshal(jsn, into)
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

func (objs *clientObjects) getEnvoyConfig(namespace, name string) *envoy_config_bootstrap.Bootstrap {
	cm := objs.findConfigMap(namespace, name).Data
	var bootstrapCfg envoy_config_bootstrap.Bootstrap
	err := unmarshalYaml([]byte(cm["envoy.yaml"]), &bootstrapCfg)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return &bootstrapCfg
}

// containMapElements produces a matcher that will only match if all provided map elements
// are completely accounted for. The actual value is expected to not be nil or empty since
// there are other, more appropriate matchers for those cases.
func containMapElements[keyT comparable, valT any](m map[keyT]valT) types.GomegaMatcher {
	subMatchers := []types.GomegaMatcher{
		Not(BeNil()),
		Not(BeEmpty()),
	}
	for k, v := range m {
		subMatchers = append(subMatchers, HaveKeyWithValue(k, v))
	}
	return And(subMatchers...)
}

var _ = Describe("Deployer", func() {
	const (
		defaultNamespace = "default"
	)
	var (
		d *deployer.Deployer

		defaultGatewayClass = func() *api.GatewayClass {
			return &api.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: wellknown.GatewayClassName,
				},
				Spec: api.GatewayClassSpec{
					ControllerName: wellknown.GatewayControllerName,
					ParametersRef: &api.ParametersReference{
						Group:     gw2_v1alpha1.GroupName,
						Kind:      gw2_v1alpha1.GatewayParametersKind,
						Name:      wellknown.DefaultGatewayParametersName,
						Namespace: ptr.To(api.Namespace(defaultNamespace)),
					},
				},
			}
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

		// Note that this is NOT meant to reflect the actual defaults defined in install/helm/gloo/templates/43-gatewayparameters.yaml
		defaultGatewayParams = func() *gw2_v1alpha1.GatewayParameters {
			return &gw2_v1alpha1.GatewayParameters{
				TypeMeta: metav1.TypeMeta{
					Kind: gw2_v1alpha1.GatewayParametersKind,
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
							ExtraLabels: map[string]string{
								"foo-label": "bar-label",
							},
							ExtraAnnotations: map[string]string{
								"foo": "bar",
							},
						},
						ServiceAccount: &gw2_v1alpha1.ServiceAccount{
							ExtraLabels: map[string]string{
								"default-label-key": "default-label-val",
							},
							ExtraAnnotations: map[string]string{
								"default-anno-key": "default-anno-val",
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

		defaultDeploymentName     = defaultGateway().Name
		defaultConfigMapName      = defaultDeploymentName
		defaultServiceName        = defaultDeploymentName
		defaultServiceAccountName = defaultDeploymentName

		selfManagedGatewayParam = func(name string) *gw2_v1alpha1.GatewayParameters {
			return &gw2_v1alpha1.GatewayParameters{
				TypeMeta: metav1.TypeMeta{
					Kind: gw2_v1alpha1.GatewayParametersKind,
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
						Group:     gw2_v1alpha1.GroupName,
						Kind:      gw2_v1alpha1.GatewayParametersKind,
						Name:      wellknown.DefaultGatewayParametersName,
						Namespace: ptr.To(api.Namespace(defaultNamespace)),
					},
				},
			}
			gwParams := &gw2_v1alpha1.GatewayParameters{
				TypeMeta: metav1.TypeMeta{
					Kind: gw2_v1alpha1.GatewayParametersKind,
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
				ControlPlane: deployer.ControlPlaneInfo{
					XdsHost: "something.cluster.local", XdsPort: 1234,
				},
			})
			Expect(err).NotTo(HaveOccurred())

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

			var objs clientObjects
			objs, err = d.GetObjsToDeploy(context.Background(), gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(objs).To(HaveLen(4))
			Expect(objs.findDeployment(defaultNamespace, gw.Name)).ToNot(BeNil())
			Expect(objs.findService(defaultNamespace, gw.Name)).ToNot(BeNil())
			Expect(objs.findConfigMap(defaultNamespace, gw.Name)).ToNot(BeNil())
			Expect(objs.findServiceAccount(defaultNamespace, gw.Name)).ToNot(BeNil())
		})
	})

	Context("aws options", func() {
		stsClusterName := "my_sts_cluster"
		stsUri := "sts.cluster.uri"

		It("passes through aws sts values from settings", func() {
			d, err := deployer.NewDeployer(newFakeClientWithObjs(defaultGatewayClass(), defaultGatewayParams()), &deployer.Inputs{
				ControllerName: wellknown.GatewayControllerName,
				Dev:            false,
				ControlPlane: deployer.ControlPlaneInfo{
					XdsHost: "something.cluster.local", XdsPort: 1234,
				},
				Aws: &deployer.AwsInfo{
					EnableServiceAccountCredentials: true,
					StsClusterName:                  stsClusterName,
					StsUri:                          stsUri,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// run deployer and get the ConfigMap that was created
			var objs clientObjects
			objs, err = d.GetObjsToDeploy(context.Background(), defaultGateway())
			Expect(err).NotTo(HaveOccurred())

			cm := objs.findConfigMap(defaultNamespace, defaultConfigMapName)
			Expect(cm).ToNot(BeNil())
			envoyYaml := cm.Data["envoy.yaml"]

			// check AWS-specific configmap values
			Expect(envoyYaml).To(ContainSubstring(fmt.Sprintf("cluster_name: %s", stsClusterName)))
			Expect(envoyYaml).To(ContainSubstring(fmt.Sprintf("sni: %s", stsUri)))
		})

		It("does not configure aws sts cluster when aws options not set", func() {
			d, err := deployer.NewDeployer(newFakeClientWithObjs(defaultGatewayClass(), defaultGatewayParams()), &deployer.Inputs{
				ControllerName: wellknown.GatewayControllerName,
				Dev:            false,
				ControlPlane: deployer.ControlPlaneInfo{
					XdsHost: "something.cluster.local", XdsPort: 1234,
				},
				Aws: nil,
			})
			Expect(err).NotTo(HaveOccurred())

			// run deployer and get the ConfigMap that was created
			var objs clientObjects
			objs, err = d.GetObjsToDeploy(context.Background(), defaultGateway())
			Expect(err).NotTo(HaveOccurred())

			cm := objs.findConfigMap(defaultNamespace, defaultConfigMapName)
			Expect(cm).ToNot(BeNil())
			envoyYaml := cm.Data["envoy.yaml"]

			// sts cluster should not have been created
			Expect(envoyYaml).NotTo(ContainSubstring(stsClusterName))
			Expect(envoyYaml).NotTo(ContainSubstring(stsUri))
		})

		It("does not configure aws sts cluster when service account credentials not enabled", func() {
			d, err := deployer.NewDeployer(newFakeClientWithObjs(defaultGatewayClass(), defaultGatewayParams()), &deployer.Inputs{
				ControllerName: wellknown.GatewayControllerName,
				Dev:            false,
				ControlPlane: deployer.ControlPlaneInfo{
					XdsHost: "something.cluster.local", XdsPort: 1234,
				},
				Aws: &deployer.AwsInfo{
					EnableServiceAccountCredentials: false,
					StsClusterName:                  stsClusterName,
					StsUri:                          stsUri,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// run deployer and get the ConfigMap that was created
			var objs clientObjects
			objs, err = d.GetObjsToDeploy(context.Background(), defaultGateway())
			Expect(err).NotTo(HaveOccurred())

			cm := objs.findConfigMap(defaultNamespace, defaultConfigMapName)
			Expect(cm).ToNot(BeNil())
			envoyYaml := cm.Data["envoy.yaml"]

			// sts cluster should not have been created
			Expect(envoyYaml).NotTo(ContainSubstring(stsClusterName))
			Expect(envoyYaml).NotTo(ContainSubstring(stsUri))
		})
	})

	Context("special cases", func() {
		var gwc *api.GatewayClass
		BeforeEach(func() {
			gwc = defaultGatewayClass()
			var err error

			d, err = deployer.NewDeployer(newFakeClientWithObjs(gwc, defaultGatewayParams()), &deployer.Inputs{
				ControllerName: wellknown.GatewayControllerName,
				Dev:            false,
				ControlPlane: deployer.ControlPlaneInfo{
					XdsHost: "something.cluster.local", XdsPort: 1234,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should get gvks", func() {
			gvks, err := d.GetGvksToWatch(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(gvks).To(HaveLen(4))
			Expect(gvks).To(ConsistOf(
				wellknown.DeploymentGVK,
				wellknown.ServiceGVK,
				wellknown.ServiceAccountGVK,
				wellknown.ConfigMapGVK,
			))
		})

		It("support segmenting by release", func() {
			d1, err := deployer.NewDeployer(newFakeClientWithObjs(gwc, defaultGatewayParams()), &deployer.Inputs{
				ControllerName: wellknown.GatewayControllerName,
				Dev:            false,
				ControlPlane: deployer.ControlPlaneInfo{
					XdsHost: "something.cluster.local", XdsPort: 1234,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			d2, err := deployer.NewDeployer(newFakeClientWithObjs(gwc, defaultGatewayParams()), &deployer.Inputs{
				ControllerName: wellknown.GatewayControllerName,
				Dev:            false,
				ControlPlane: deployer.ControlPlaneInfo{
					XdsHost: "something.cluster.local", XdsPort: 1234,
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

			var objs1, objs2 clientObjects
			objs1, err = d1.GetObjsToDeploy(context.Background(), gw1)
			Expect(err).NotTo(HaveOccurred())
			Expect(objs1).NotTo(BeEmpty())
			Expect(objs1.findDeployment(defaultNamespace, gw1.Name)).ToNot(BeNil())
			Expect(objs1.findService(defaultNamespace, gw1.Name)).ToNot(BeNil())
			Expect(objs1.findConfigMap(defaultNamespace, gw1.Name)).ToNot(BeNil())
			Expect(objs1.findServiceAccount(defaultNamespace, gw1.Name)).ToNot(BeNil())

			objs2, err = d2.GetObjsToDeploy(context.Background(), gw2)
			Expect(err).NotTo(HaveOccurred())
			Expect(objs2).NotTo(BeEmpty())
			Expect(objs2.findDeployment(defaultNamespace, gw2.Name)).ToNot(BeNil())
			Expect(objs2.findService(defaultNamespace, gw2.Name)).ToNot(BeNil())
			Expect(objs2.findConfigMap(defaultNamespace, gw2.Name)).ToNot(BeNil())
			Expect(objs2.findServiceAccount(defaultNamespace, gw2.Name)).ToNot(BeNil())

			for _, obj := range objs1 {
				Expect(obj.GetName()).To(Equal(gw1.Name))
			}
			for _, obj := range objs2 {
				Expect(obj.GetName()).To(Equal(gw2.Name))
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
			gwpOverrideName       = "gateway-params-override"
			defaultDeployerInputs = func() *deployer.Inputs {
				return &deployer.Inputs{
					ControllerName: wellknown.GatewayControllerName,
					Dev:            false,
					ControlPlane: deployer.ControlPlaneInfo{
						XdsHost: "something.cluster.local", XdsPort: 1234,
					},
				}
			}
			istioEnabledDeployerInputs = func() *deployer.Inputs {
				inp := defaultDeployerInputs()
				inp.IstioIntegrationEnabled = true
				return inp
			}

			defaultGatewayParamsOverride = func() *gw2_v1alpha1.GatewayParameters {
				return &gw2_v1alpha1.GatewayParameters{
					TypeMeta: metav1.TypeMeta{
						Kind: gw2_v1alpha1.GatewayParametersKind,
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
									"override-foo": "override-bar",
								},
								SecurityContext: &corev1.PodSecurityContext{
									RunAsUser:  ptr.To(int64(3)),
									RunAsGroup: ptr.To(int64(4)),
								},
							},
							Service: &gw2_v1alpha1.Service{
								Type:      ptr.To(corev1.ServiceTypeClusterIP),
								ClusterIP: ptr.To("99.99.99.99"),
								ExtraLabels: map[string]string{
									"override-foo-label": "override-bar-label",
								},
								ExtraAnnotations: map[string]string{
									"override-foo": "override-bar",
								},
							},
							ServiceAccount: &gw2_v1alpha1.ServiceAccount{
								ExtraLabels: map[string]string{
									"override-label-key": "override-label-val",
								},
								ExtraAnnotations: map[string]string{
									"override-anno-key": "override-anno-val",
								},
							},
						},
					},
				}
			}
			// this is the result of `defaultGatewayParams` (GatewayClass-level) merged with `defaultGatewayParamsOverride` (Gateway-level)
			mergedGatewayParams = func() *gw2_v1alpha1.GatewayParameters {
				return &gw2_v1alpha1.GatewayParameters{
					TypeMeta: metav1.TypeMeta{
						Kind: gw2_v1alpha1.GatewayParametersKind,
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
									"foo":          "bar",
									"override-foo": "override-bar",
								},
								SecurityContext: &corev1.PodSecurityContext{
									RunAsUser:  ptr.To(int64(3)),
									RunAsGroup: ptr.To(int64(4)),
								},
							},
							Service: &gw2_v1alpha1.Service{
								Type:      ptr.To(corev1.ServiceTypeClusterIP),
								ClusterIP: ptr.To("99.99.99.99"),
								ExtraLabels: map[string]string{
									"foo-label":          "bar-label",
									"override-foo-label": "override-bar-label",
								},
								ExtraAnnotations: map[string]string{
									"foo":          "bar",
									"override-foo": "override-bar",
								},
							},
							ServiceAccount: &gw2_v1alpha1.ServiceAccount{
								ExtraLabels: map[string]string{
									"default-label-key":  "default-label-val",
									"override-label-key": "override-label-val",
								},
								ExtraAnnotations: map[string]string{
									"default-anno-key":  "default-anno-val",
									"override-anno-key": "override-anno-val",
								},
							},
						},
					},
				}
			}
			gatewayParamsOverrideWithSds = func() *gw2_v1alpha1.GatewayParameters {
				return &gw2_v1alpha1.GatewayParameters{
					TypeMeta: metav1.TypeMeta{
						Kind: gw2_v1alpha1.GatewayParametersKind,
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
								Ports: []corev1.ContainerPort{
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
						Kind: gw2_v1alpha1.GatewayParametersKind,
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

			fullyDefinedGatewayParamsWithProbes = func() *gw2_v1alpha1.GatewayParameters {
				params := fullyDefinedGatewayParameters(wellknown.DefaultGatewayParametersName, defaultNamespace)
				params.Spec.Kube.PodTemplate.LivenessProbe = generateLivenessProbe()
				params.Spec.Kube.PodTemplate.ReadinessProbe = generateReadinessProbe()
				params.Spec.Kube.PodTemplate.TerminationGracePeriodSeconds = ptr.To(5)
				params.Spec.Kube.PodTemplate.GracefulShutdown = &gw2_v1alpha1.GracefulShutdownSpec{
					Enabled:          ptr.To(true),
					SleepTimeSeconds: ptr.To(7),
				}
				return params
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

				Expect(dep.Spec.Template.Annotations).To(containMapElements(expectedGwp.PodTemplate.ExtraAnnotations))
				Expect(dep.Spec.Template.Annotations).To(HaveKeyWithValue("prometheus.io/scrape", "true"))
				Expect(dep.Spec.Template.Spec.SecurityContext.RunAsUser).To(Equal(expectedGwp.PodTemplate.SecurityContext.RunAsUser))
				Expect(dep.Spec.Template.Spec.SecurityContext.RunAsGroup).To(Equal(expectedGwp.PodTemplate.SecurityContext.RunAsGroup))

				svc := objs.findService(defaultNamespace, defaultServiceName)
				Expect(svc).ToNot(BeNil())
				Expect(svc.GetAnnotations()).ToNot(BeNil())
				Expect(svc.GetAnnotations()).To(containMapElements(expectedGwp.Service.ExtraAnnotations))
				Expect(svc.GetLabels()).ToNot(BeNil())
				Expect(svc.GetLabels()).To(containMapElements(expectedGwp.Service.ExtraLabels))
				Expect(svc.Spec.Type).To(Equal(*expectedGwp.Service.Type))
				Expect(svc.Spec.ClusterIP).To(Equal(*expectedGwp.Service.ClusterIP))

				sa := objs.findServiceAccount(defaultNamespace, defaultServiceAccountName)
				Expect(sa).ToNot(BeNil())
				Expect(sa.GetAnnotations()).ToNot(BeNil())
				Expect(sa.GetAnnotations()).To(containMapElements(expectedGwp.ServiceAccount.ExtraAnnotations))
				Expect(sa.GetLabels()).ToNot(BeNil())
				Expect(sa.GetLabels()).To(containMapElements(expectedGwp.ServiceAccount.ExtraLabels))

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
			// Check we have Deployment, Envoy ConfigMap, ServiceAccount, Service, AI Stats ConfigMap
			Expect(objs).To(HaveLen(5))
			dep := objs.findDeployment(defaultNamespace, defaultDeploymentName)
			Expect(dep).ToNot(BeNil())
			Expect(dep.Spec.Replicas).ToNot(BeNil())
			Expect(*dep.Spec.Replicas).To(Equal(int32(*expectedGwp.Deployment.Replicas)))

			Expect(dep.Spec.Template.Annotations).To(containMapElements(expectedGwp.PodTemplate.ExtraAnnotations))

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
			Expect(svc.GetAnnotations()).To(containMapElements(expectedGwp.Service.ExtraAnnotations))
			Expect(svc.GetLabels()).ToNot(BeNil())
			Expect(svc.GetLabels()).To(containMapElements(expectedGwp.Service.ExtraLabels))
			Expect(svc.Spec.Type).To(Equal(*expectedGwp.Service.Type))
			Expect(svc.Spec.ClusterIP).To(Equal(*expectedGwp.Service.ClusterIP))

			sa := objs.findServiceAccount(defaultNamespace, defaultServiceAccountName)
			Expect(sa).ToNot(BeNil())
			Expect(sa.GetAnnotations()).ToNot(BeNil())
			Expect(sa.GetAnnotations()).To(containMapElements(expectedGwp.ServiceAccount.ExtraAnnotations))
			Expect(sa.GetLabels()).ToNot(BeNil())
			Expect(sa.GetLabels()).To(containMapElements(expectedGwp.ServiceAccount.ExtraLabels))

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

		fullyDefinedValidationWithProbes := func(objs clientObjects, inp *input) error {
			err := fullyDefinedValidationWithoutRunAsUser(objs, inp)
			if err != nil {
				return err
			}

			dep := objs.findDeployment(defaultNamespace, defaultDeploymentName)
			Expect(*dep.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Equal(int64(5)))

			envoyContainer := dep.Spec.Template.Spec.Containers[0]
			Expect(envoyContainer.LivenessProbe).To(BeEquivalentTo(generateLivenessProbe()))
			Expect(envoyContainer.ReadinessProbe).To(BeEquivalentTo(generateReadinessProbe()))
			Expect(envoyContainer.Lifecycle.PreStop.Exec.Command).To(BeEquivalentTo([]string{
				"/bin/sh",
				"-c",
				"wget --post-data \"\" -O /dev/null 127.0.0.1:19000/healthcheck/fail; sleep 7",
			}))

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
					return validateGatewayParametersPropagation(objs, mergedGatewayParams())
				},
			}),
			Entry("Fully defined GatewayParameters", &input{
				dInputs:    istioEnabledDeployerInputs(),
				gw:         defaultGateway(),
				defaultGwp: fullyDefinedGatewayParams(),
			}, &expectedOutput{
				validationFunc: fullyDefinedValidation,
			}),
			Entry("Fully defined GatewayParameters with probes", &input{
				dInputs:    istioEnabledDeployerInputs(),
				gw:         defaultGateway(),
				defaultGwp: fullyDefinedGatewayParamsWithProbes(),
			}, &expectedOutput{
				validationFunc: fullyDefinedValidationWithProbes,
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
						GatewayClassName: wellknown.GatewayClassName,
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
						GatewayClassName: wellknown.GatewayClassName,
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
					Expect(node).To(HaveKeyWithValue("metadata", map[string]any{
						xds.RoleKey: fmt.Sprintf("%s~%s~%s", wellknown.GatewayApiProxyValue, gw.Namespace, gw.Name),
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
					Expect(node).To(HaveKeyWithValue("metadata", map[string]any{
						xds.RoleKey: fmt.Sprintf("%s~%s~%s", wellknown.GatewayApiProxyValue, gw.Namespace, gw.Name),
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

	Context("Inference Extension endpoint picker", func() {
		const defaultNamespace = "default"

		It("should deploy endpoint picker resources for an InferencePool", func() {
			// Create a fake InferencePool resource.
			pool := &infextv1a1.InferencePool{
				TypeMeta: metav1.TypeMeta{
					Kind:       wellknown.InferencePoolKind,
					APIVersion: fmt.Sprintf("%s/%s", infextv1a1.GroupVersion.Group, infextv1a1.GroupVersion.Version),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pool1",
					Namespace: defaultNamespace,
					UID:       "pool-uid",
				},
			}

			// Initialize a new deployer with InferenceExtension inputs.
			d, err := deployer.NewDeployer(newFakeClientWithObjs(pool), &deployer.Inputs{
				ControllerName:     wellknown.GatewayControllerName,
				InferenceExtension: &deployer.InferenceExtInfo{},
			})
			Expect(err).NotTo(HaveOccurred())

			// Get the endpoint picker objects for the InferencePool.
			objs, err := d.GetEndpointPickerObjs(pool)
			Expect(err).NotTo(HaveOccurred())
			Expect(objs).NotTo(BeEmpty(), "expected non-empty objects for endpoint picker deployment")
			Expect(objs).To(HaveLen(5))

			// Find the child objects.
			var sa *corev1.ServiceAccount
			var role *rbacv1.Role
			var rb *rbacv1.RoleBinding
			var dep *appsv1.Deployment
			var svc *corev1.Service
			for _, obj := range objs {
				switch t := obj.(type) {
				case *corev1.ServiceAccount:
					sa = t
				case *rbacv1.Role:
					role = t
				case *rbacv1.RoleBinding:
					rb = t
				case *appsv1.Deployment:
					dep = t
				case *corev1.Service:
					svc = t
				}
			}
			Expect(sa).NotTo(BeNil(), "expected a ServiceAccount to be rendered")
			Expect(role).NotTo(BeNil(), "expected a Role to be rendered")
			Expect(rb).NotTo(BeNil(), "expected a RoleBinding to be rendered")
			Expect(dep).NotTo(BeNil(), "expected a Deployment to be rendered")
			Expect(svc).NotTo(BeNil(), "expected a Service to be rendered")

			// Check that owner references are set on all rendered objects to the InferencePool.
			for _, obj := range objs {
				ownerRefs := obj.GetOwnerReferences()
				Expect(ownerRefs).To(HaveLen(1))
				ref := ownerRefs[0]
				Expect(ref.Name).To(Equal(pool.Name))
				Expect(ref.UID).To(Equal(pool.UID))
				Expect(ref.Kind).To(Equal(pool.Kind))
				Expect(ref.APIVersion).To(Equal(pool.APIVersion))
				Expect(*ref.Controller).To(BeTrue())
			}

			// Validate that the rendered Deployment and Service have the expected names.
			// (The template hardcodes the names to "inference-gateway-ext-proc".)
			expectedName := fmt.Sprintf("%s-endpoint-picker", pool.Name)
			Expect(sa.Name).To(Equal(expectedName))
			Expect(role.Name).To(Equal(expectedName))
			Expect(rb.Name).To(Equal(expectedName))
			Expect(dep.Name).To(Equal(expectedName))
			Expect(svc.Name).To(Equal(expectedName))

			// Check the container args for the expected poolName.
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
			pickerContainer := dep.Spec.Template.Spec.Containers[0]
			Expect(pickerContainer.Args).To(Equal([]string{
				"-poolName",
				pool.Name,
				"-v",
				"3",
				"-grpcPort",
				"9002",
				"-grpcHealthPort",
				"9003",
			}))
		})
	})
})

// initialize a fake controller-runtime client with the given list of objects
func newFakeClientWithObjs(objs ...client.Object) client.Client {
	scheme := schemes.GatewayScheme()

	// Ensure the rbac types are registered.
	if err := rbacv1.AddToScheme(scheme); err != nil {
		panic(fmt.Sprintf("failed to add rbacv1 scheme: %v", err))
	}

	// Check if any object is an InferencePool, and add its scheme if needed.
	for _, obj := range objs {
		gvk := obj.GetObjectKind().GroupVersionKind()
		if gvk.Kind == wellknown.InferencePoolKind {
			if err := infextv1a1.AddToScheme(scheme); err != nil {
				panic(fmt.Sprintf("failed to add InferenceExtension scheme: %v", err))
			}
			break
		}
	}

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		Build()
}

func fullyDefinedGatewayParameters(name, namespace string) *gw2_v1alpha1.GatewayParameters {
	return &gw2_v1alpha1.GatewayParameters{
		TypeMeta: metav1.TypeMeta{
			Kind: gw2_v1alpha1.GatewayParametersKind,
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
					Tolerations: []corev1.Toleration{{
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
				ServiceAccount: &gw2_v1alpha1.ServiceAccount{
					ExtraLabels: map[string]string{
						"a": "b",
					},
					ExtraAnnotations: map[string]string{
						"c": "d",
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
					Ports: []corev1.ContainerPort{
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

func generateLivenessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"wget",
					"-O",
					"/dev/null",
					"127.0.0.1:19000/server_info",
				},
			},
		},
		InitialDelaySeconds: 3,
		PeriodSeconds:       10,
		FailureThreshold:    3,
	}
}

func generateReadinessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Scheme: "HTTP",
				Port: intstr.IntOrString{
					IntVal: 8082,
				},
				Path: "/envoy-hc",
			},
		},
		InitialDelaySeconds: 5,
		PeriodSeconds:       5,
		FailureThreshold:    2,
	}
}
