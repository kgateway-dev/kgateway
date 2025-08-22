package deployer_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	jsonpb "google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt/krttest"
	"istio.io/istio/pkg/test"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	infextv1a2 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	api "sigs.k8s.io/gateway-api/apis/v1"
	apixv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	gw2_v1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/controller"
	internaldeployer "github.com/kgateway-dev/kgateway/v2/internal/kgateway/deployer"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/httplistenerpolicy"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/internal/version"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/pkg/settings"

	// TODO BML tests in this suite fail if this no-op import is not imported first.
	//
	// I know, I know, you're reading this, and you're skeptical. I can feel it.
	// Don't take my word for it.
	//
	// There is some import within this package that this suite relies on. Chasing that down is
	// *hard* tho due to the import tree, and best done in a followup.
	// _ "github.com/kgateway-dev/kgateway/internal/kgateway/translator/translator.go"
	//
	// The above TODO is a result of proto types being registered for free somewhere through
	// the translator import. What we really need is to register all proto types, which is
	// "correctly" available to use via `envoyinit`; note that the autogeneration of these types
	// is currently broken. see: https://github.com/kgateway-dev/kgateway/issues/10491
	_ "github.com/kgateway-dev/kgateway/v2/pkg/utils/filter_types"
)

const envoyDataKey = "envoy.yaml"

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

func (objs *clientObjects) getEnvoyConfig(namespace, name string) *envoybootstrapv3.Bootstrap {
	cm := objs.findConfigMap(namespace, name).Data
	var bootstrapCfg envoybootstrapv3.Bootstrap
	err := unmarshalYaml([]byte(cm[envoyDataKey]), &bootstrapCfg)
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
		defaultGatewayClass = func() *api.GatewayClass {
			return &api.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: wellknown.DefaultGatewayClassName,
				},
				Spec: api.GatewayClassSpec{
					ControllerName: wellknown.DefaultGatewayControllerName,
				},
			}
		}

		defaultGatewayClassWithParamsRef = func() *api.GatewayClass {
			gwc := defaultGatewayClass()
			gwc.Spec.ParametersRef = &api.ParametersReference{
				Group:     gw2_v1alpha1.GroupName,
				Kind:      api.Kind(wellknown.GatewayParametersGVK.Kind),
				Name:      wellknown.DefaultGatewayParametersName,
				Namespace: ptr.To(api.Namespace(defaultNamespace)),
			}
			return gwc
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
					APIVersion: "gateway.networking.k8s.io",
				},
				Spec: api.GatewaySpec{
					GatewayClassName: wellknown.DefaultGatewayClassName,
					Listeners: []api.Listener{{
						Name: "listener-1",
						Port: 80,
					}},
				},
			}
		}

		highPortGateway = func() *api.Gateway {
			gw := defaultGateway()
			gw.Spec.Listeners = []api.Listener{{
				Name: "listener-1",
				Port: 8088,
			}}
			return gw
		}

		// Note that this is NOT meant to reflect the actual defaults defined in install/helm/kgateway/templates/gatewayparameters.yaml
		defaultGatewayParams = func() *gw2_v1alpha1.GatewayParameters {
			return &gw2_v1alpha1.GatewayParameters{
				TypeMeta: metav1.TypeMeta{
					Kind: wellknown.GatewayParametersGVK.Kind,
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
							ExternalTrafficPolicy: ptr.To(string(corev1.ServiceExternalTrafficPolicyTypeLocal)),
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
							RoutePrefixRewrite:      ptr.To("/stats/prometheus?usedonly"),
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
					Kind: wellknown.GatewayParametersGVK.Kind,
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

		agentGatewayParam = func(name string) *gw2_v1alpha1.GatewayParameters {
			return &gw2_v1alpha1.GatewayParameters{
				TypeMeta: metav1.TypeMeta{
					Kind: wellknown.GatewayParametersGVK.Kind,
					// The parsing expects GROUP/VERSION format in this field
					APIVersion: gw2_v1alpha1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: defaultNamespace,
					UID:       "1237",
				},
				Spec: gw2_v1alpha1.GatewayParametersSpec{
					Kube: &gw2_v1alpha1.KubernetesProxyConfig{
						AgentGateway: &gw2_v1alpha1.AgentGateway{
							Enabled: ptr.To(true),
							Image: &gw2_v1alpha1.Image{
								Tag: ptr.To("0.4.0"),
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: ptr.To(int64(333)),
							},
							Resources: &corev1.ResourceRequirements{
								Limits: corev1.ResourceList{"cpu": resource.MustParse("101m")},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "test",
									Value: "value",
								},
							},
						},
						PodTemplate: &gw2_v1alpha1.Pod{
							TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
								{
									MaxSkew:           1,
									TopologyKey:       "kubernetes.io/hostname",
									WhenUnsatisfiable: corev1.DoNotSchedule,
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{"app": "test"},
									},
								},
							},
							Tolerations: []corev1.Toleration{
								{
									Key:      "test-key",
									Operator: corev1.TolerationOpEqual,
									Value:    "test-value",
									Effect:   corev1.TaintEffectNoSchedule,
								},
							},
							Affinity: &corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{
													{
														Key:      "kubernetes.io/os",
														Operator: corev1.NodeSelectorOpIn,
														Values:   []string{"linux"},
													},
												},
											},
										},
									},
								},
							},
							NodeSelector: map[string]string{
								"kubernetes.io/arch": "amd64",
							},
						},
					},
				},
			}
		}
	)

	Context("default case", func() {
		It("should work with empty params", func() {
			gwc := &api.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: wellknown.DefaultGatewayClassName,
				},
				Spec: api.GatewayClassSpec{
					ControllerName: wellknown.DefaultGatewayControllerName,
					ParametersRef: &api.ParametersReference{
						Group:     gw2_v1alpha1.GroupName,
						Kind:      api.Kind(wellknown.GatewayParametersGVK.Kind),
						Name:      wellknown.DefaultGatewayParametersName,
						Namespace: ptr.To(api.Namespace(defaultNamespace)),
					},
				},
			}
			gwParams := &gw2_v1alpha1.GatewayParameters{
				TypeMeta: metav1.TypeMeta{
					Kind: wellknown.GatewayParametersGVK.Kind,
					// The parsing expects GROUP/VERSION format in this field
					APIVersion: gw2_v1alpha1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      wellknown.DefaultGatewayParametersName,
					Namespace: defaultNamespace,
					UID:       "1237",
				},
			}

			gw := &api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: defaultNamespace,
					UID:       "1235",
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Gateway",
					APIVersion: "gateway.networking.k8s.io",
				},
				Spec: api.GatewaySpec{
					GatewayClassName: wellknown.DefaultGatewayClassName,
				},
			}

			gwp := internaldeployer.NewGatewayParameters(newFakeClientWithObjs(gwc, gwParams), &deployer.Inputs{
				CommonCollections: newCommonCols(GinkgoT(), gwc, gw),
				Dev:               false,
				ControlPlane: deployer.ControlPlaneInfo{
					XdsHost: "something.cluster.local",
					XdsPort: 1234,
				},
				ImageInfo: &deployer.ImageInfo{
					Registry: "foo",
					Tag:      "bar",
				},
				GatewayClassName:         wellknown.DefaultGatewayClassName,
				WaypointGatewayClassName: wellknown.DefaultWaypointClassName,
				AgentGatewayClassName:    wellknown.DefaultAgentGatewayClassName,
			})
			chart, err := internaldeployer.LoadGatewayChart()
			Expect(err).NotTo(HaveOccurred())
			d := deployer.NewDeployer(wellknown.DefaultGatewayControllerName, newFakeClientWithObjs(gwc, gwParams), chart,
				gwp,
				internaldeployer.GatewayReleaseNameAndNamespace)

			var objs clientObjects
			objs, err = d.GetObjsToDeploy(context.Background(), gw)
			Expect(err).NotTo(HaveOccurred())
			objs = d.SetNamespaceAndOwner(gw, objs)
			Expect(objs).To(HaveLen(4))
			Expect(objs.findDeployment(defaultNamespace, gw.Name)).ToNot(BeNil())
			Expect(objs.findService(defaultNamespace, gw.Name)).ToNot(BeNil())
			Expect(objs.findConfigMap(defaultNamespace, gw.Name)).ToNot(BeNil())
			Expect(objs.findServiceAccount(defaultNamespace, gw.Name)).ToNot(BeNil())
		})
	})

	Context("self managed gateway", func() {
		var (
			d   *deployer.Deployer
			gwp *gw2_v1alpha1.GatewayParameters
			gwc *api.GatewayClass
		)
		BeforeEach(func() {
			gwp = selfManagedGatewayParam("self-managed-gateway-params")
			gwc = &api.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: wellknown.DefaultGatewayClassName,
				},
				Spec: api.GatewayClassSpec{
					ControllerName: wellknown.DefaultGatewayControllerName,
					ParametersRef: &api.ParametersReference{
						Group:     gw2_v1alpha1.GroupName,
						Kind:      api.Kind(wellknown.GatewayParametersGVK.Kind),
						Name:      gwp.GetName(),
						Namespace: ptr.To(api.Namespace(defaultNamespace)),
					},
				},
			}
		})

		It("deploys nothing", func() {
			gw := &api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: defaultNamespace,
				},
				Spec: api.GatewaySpec{
					GatewayClassName: wellknown.DefaultGatewayClassName,
					Infrastructure: &api.GatewayInfrastructure{
						ParametersRef: &api.LocalParametersReference{
							Group: gw2_v1alpha1.GroupName,
							Kind:  api.Kind(wellknown.GatewayParametersGVK.Kind),
							Name:  gwp.GetName(),
						},
					},
					Listeners: []api.Listener{{
						Name: "listener-1",
						Port: 80,
					}},
				},
			}
			var err error
			gwParams := internaldeployer.NewGatewayParameters(newFakeClientWithObjs(gwc, gwp), &deployer.Inputs{
				CommonCollections: newCommonCols(GinkgoT(), gwc, gw),
				Dev:               false,
				ControlPlane: deployer.ControlPlaneInfo{
					XdsHost: "something.cluster.local",
					XdsPort: 1234,
				},
				ImageInfo: &deployer.ImageInfo{
					Registry: "foo",
					Tag:      "bar",
				},
				GatewayClassName:         wellknown.DefaultGatewayClassName,
				WaypointGatewayClassName: wellknown.DefaultWaypointClassName,
				AgentGatewayClassName:    wellknown.DefaultAgentGatewayClassName,
			})
			chart, err := internaldeployer.LoadGatewayChart()
			Expect(err).NotTo(HaveOccurred())
			d = deployer.NewDeployer(wellknown.DefaultGatewayControllerName, newFakeClientWithObjs(gwc, gwp), chart,
				gwParams,
				internaldeployer.GatewayReleaseNameAndNamespace)

			objs, err := d.GetObjsToDeploy(context.Background(), gw)
			Expect(err).NotTo(HaveOccurred())
			objs = d.SetNamespaceAndOwner(gw, objs)
			Expect(objs).To(BeEmpty())
		})
	})

	Context("agentgateway", func() {
		var (
			gwp *gw2_v1alpha1.GatewayParameters
			gwc *api.GatewayClass
		)
		BeforeEach(func() {
			gwp = agentGatewayParam("agent-gateway-params")
			gwc = &api.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "agentgateway",
				},
				Spec: api.GatewayClassSpec{
					ControllerName: wellknown.DefaultGatewayControllerName,
					ParametersRef: &api.ParametersReference{
						Group:     gw2_v1alpha1.GroupName,
						Kind:      api.Kind(wellknown.GatewayParametersGVK.Kind),
						Name:      gwp.GetName(),
						Namespace: ptr.To(api.Namespace(defaultNamespace)),
					},
				},
			}
		})

		It("deploys agentgateway", func() {
			gw := &api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent-gateway",
					Namespace: defaultNamespace,
				},
				Spec: api.GatewaySpec{
					GatewayClassName: "agentgateway",
					Infrastructure: &api.GatewayInfrastructure{
						ParametersRef: &api.LocalParametersReference{
							Group: gw2_v1alpha1.GroupName,
							Kind:  api.Kind(wellknown.GatewayParametersGVK.Kind),
							Name:  gwp.GetName(),
						},
					},
					Listeners: []api.Listener{{
						Name: "listener-1",
						Port: 80,
					}},
				},
			}
			gwParams := internaldeployer.NewGatewayParameters(newFakeClientWithObjs(gwc, gwp), &deployer.Inputs{
				CommonCollections: newCommonCols(GinkgoT(), gwc, gw),
				Dev:               false,
				ControlPlane: deployer.ControlPlaneInfo{
					XdsHost: "something.cluster.local",
					XdsPort: 1234,
				},
				ImageInfo: &deployer.ImageInfo{
					Registry: "foo",
					Tag:      "bar",
				},
				GatewayClassName:         wellknown.DefaultGatewayClassName,
				WaypointGatewayClassName: wellknown.DefaultWaypointClassName,
				AgentGatewayClassName:    wellknown.DefaultAgentGatewayClassName,
			})
			chart, err := internaldeployer.LoadGatewayChart()
			Expect(err).NotTo(HaveOccurred())
			d := deployer.NewDeployer(wellknown.DefaultGatewayControllerName, newFakeClientWithObjs(gwc, gwp), chart,
				gwParams,
				internaldeployer.GatewayReleaseNameAndNamespace)

			var objs clientObjects
			objs, err = d.GetObjsToDeploy(context.Background(), gw)
			Expect(err).NotTo(HaveOccurred())
			objs = d.SetNamespaceAndOwner(gw, objs)
			// check the image is using the agentgateway image
			deployment := objs.findDeployment(defaultNamespace, "agent-gateway")
			Expect(deployment).ToNot(BeNil())
			// check the image uses the override tag
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(ContainSubstring("agentgateway"))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(ContainSubstring("0.4.0"))
			// check security context and resource requirements are correctly set
			expectedSecurityContext := deployment.Spec.Template.Spec.Containers[0].SecurityContext
			Expect(expectedSecurityContext).ToNot(BeNil())
			Expect(expectedSecurityContext.RunAsUser).ToNot(BeNil())
			Expect(*expectedSecurityContext.RunAsUser).To(Equal(int64(333)))
			// check the deployment fields are correctly set
			Expect(deployment.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().Equal(resource.MustParse("101m"))).To(BeTrue())
			Expect(deployment.Spec.Template.Spec.TopologySpreadConstraints[0]).To(Equal(gwp.Spec.Kube.PodTemplate.TopologySpreadConstraints[0]))
			Expect(deployment.Spec.Template.Spec.Tolerations[0]).To(Equal(gwp.Spec.Kube.PodTemplate.Tolerations[0]))
			Expect(deployment.Spec.Template.Spec.NodeSelector).To(Equal(gwp.Spec.Kube.PodTemplate.NodeSelector))
			// check env values are appended to the end of the list
			var testEnvVar corev1.EnvVar
			for _, envVar := range deployment.Spec.Template.Spec.Containers[0].Env {
				if envVar.Name == "test" {
					testEnvVar = envVar
					break
				}
			}
			Expect(testEnvVar.Name).To(Equal("test"))
			Expect(testEnvVar.Value).To(Equal("value"))
			// check the service is using the agentgateway port
			svc := objs.findService(defaultNamespace, "agent-gateway")
			Expect(svc).ToNot(BeNil())
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(80)))
			// check the config map is using the xds address and port
			cm := objs.findConfigMap(defaultNamespace, "agent-gateway")
			Expect(cm).ToNot(BeNil())
		})
	})

	Context("special cases", func() {
		var gwc *api.GatewayClass
		BeforeEach(func() {
			gwc = defaultGatewayClass()
		})

		It("deploys multiple GWs with the same GWP", func() {
			gw1 := &api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: defaultNamespace,
					UID:       "1235",
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Gateway",
					APIVersion: "gateway.networking.k8s.io",
				},
				Spec: api.GatewaySpec{
					GatewayClassName: wellknown.DefaultGatewayClassName,
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
					APIVersion: "gateway.networking.k8s.io",
				},
				Spec: api.GatewaySpec{
					GatewayClassName: wellknown.DefaultGatewayClassName,
				},
			}

			gwParams1 := internaldeployer.NewGatewayParameters(newFakeClientWithObjs(gwc, defaultGatewayParams()), &deployer.Inputs{
				CommonCollections: newCommonCols(GinkgoT(), gwc, gw1, gw2),
				Dev:               false,
				ControlPlane: deployer.ControlPlaneInfo{
					XdsHost: "something.cluster.local",
					XdsPort: 1234,
				},
				ImageInfo: &deployer.ImageInfo{
					Registry: "foo",
					Tag:      "bar",
				},
				GatewayClassName:         wellknown.DefaultGatewayClassName,
				WaypointGatewayClassName: wellknown.DefaultWaypointClassName,
				AgentGatewayClassName:    wellknown.DefaultAgentGatewayClassName,
			})
			chart, err := internaldeployer.LoadGatewayChart()
			Expect(err).NotTo(HaveOccurred())
			d1 := deployer.NewDeployer(wellknown.DefaultGatewayControllerName,
				newFakeClientWithObjs(gwc, defaultGatewayParams()), chart,
				gwParams1,
				internaldeployer.GatewayReleaseNameAndNamespace)

			gwParams2 := internaldeployer.NewGatewayParameters(newFakeClientWithObjs(gwc, defaultGatewayParams()), &deployer.Inputs{
				CommonCollections: newCommonCols(GinkgoT(), gwc, gw1, gw2),
				Dev:               false,
				ControlPlane: deployer.ControlPlaneInfo{
					XdsHost: "something.cluster.local",
					XdsPort: 1234,
				},
				ImageInfo: &deployer.ImageInfo{
					Registry: "foo",
					Tag:      "bar",
				},
				GatewayClassName:         wellknown.DefaultGatewayClassName,
				WaypointGatewayClassName: wellknown.DefaultWaypointClassName,
				AgentGatewayClassName:    wellknown.DefaultAgentGatewayClassName,
			})
			d2 := deployer.NewDeployer(wellknown.DefaultGatewayControllerName, newFakeClientWithObjs(gwc, defaultGatewayParams()), chart,
				gwParams2,
				internaldeployer.GatewayReleaseNameAndNamespace)

			var objs1, objs2 clientObjects
			objs1, err = d1.GetObjsToDeploy(context.Background(), gw1)
			Expect(err).NotTo(HaveOccurred())
			objs1 = d1.SetNamespaceAndOwner(gw1, objs1)
			Expect(objs1).NotTo(BeEmpty())
			Expect(objs1.findDeployment(defaultNamespace, gw1.Name)).ToNot(BeNil())
			Expect(objs1.findService(defaultNamespace, gw1.Name)).ToNot(BeNil())
			Expect(objs1.findConfigMap(defaultNamespace, gw1.Name)).ToNot(BeNil())
			Expect(objs1.findServiceAccount(defaultNamespace, gw1.Name)).ToNot(BeNil())

			objs2, err = d2.GetObjsToDeploy(context.Background(), gw2)
			Expect(err).NotTo(HaveOccurred())
			objs2 = d2.SetNamespaceAndOwner(gw2, objs2)
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

	Context("Gateway API infrastructure field", func() {
		It("rejects invalid group in spec.infrastructure.parametersRef", func() {
			gw := &api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: defaultNamespace,
					UID:       "1235",
				},
				Spec: api.GatewaySpec{
					GatewayClassName: wellknown.DefaultGatewayClassName,
					Infrastructure: &api.GatewayInfrastructure{
						ParametersRef: &api.LocalParametersReference{
							Group: "invalid.group",
							Kind:  api.Kind(wellknown.GatewayParametersGVK.Kind),
							Name:  "test-gwp",
						},
					},
				},
			}

			gwParams := internaldeployer.NewGatewayParameters(newFakeClientWithObjs(defaultGatewayClass()), &deployer.Inputs{
				CommonCollections: newCommonCols(GinkgoT(), defaultGatewayClass(), gw),
				Dev:               false,
				ControlPlane: deployer.ControlPlaneInfo{
					XdsHost: "something.cluster.local",
					XdsPort: 1234,
				},
				ImageInfo: &deployer.ImageInfo{
					Registry: "foo",
					Tag:      "bar",
				},
			})
			chart, err := internaldeployer.LoadGatewayChart()
			Expect(err).NotTo(HaveOccurred())
			d := deployer.NewDeployer(wellknown.DefaultGatewayControllerName,
				newFakeClientWithObjs(defaultGatewayClass()), chart,
				gwParams,
				internaldeployer.GatewayReleaseNameAndNamespace)

			_, err = d.GetObjsToDeploy(context.Background(), gw)
			Expect(err).To(MatchError(ContainSubstring("invalid group invalid.group for GatewayParameters")))
		})

		It("rejects invalid kind in spec.infrastructure.parametersRef", func() {
			gw := &api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: defaultNamespace,
					UID:       "1235",
				},
				Spec: api.GatewaySpec{
					GatewayClassName: wellknown.DefaultGatewayClassName,
					Infrastructure: &api.GatewayInfrastructure{
						ParametersRef: &api.LocalParametersReference{
							Group: gw2_v1alpha1.GroupName,
							Kind:  "InvalidKind",
							Name:  "test-gwp",
						},
					},
				},
			}

			gwParams := internaldeployer.NewGatewayParameters(newFakeClientWithObjs(defaultGatewayClass()), &deployer.Inputs{
				CommonCollections: newCommonCols(GinkgoT(), defaultGatewayClass(), gw),
				Dev:               false,
				ControlPlane: deployer.ControlPlaneInfo{
					XdsHost: "something.cluster.local",
					XdsPort: 1234,
				},
				ImageInfo: &deployer.ImageInfo{
					Registry: "foo",
					Tag:      "bar",
				},
			})
			chart, err := internaldeployer.LoadGatewayChart()
			Expect(err).NotTo(HaveOccurred())
			d := deployer.NewDeployer(
				wellknown.DefaultGatewayControllerName,
				newFakeClientWithObjs(defaultGatewayClass()), chart,
				gwParams,
				internaldeployer.GatewayReleaseNameAndNamespace)

			_, err = d.GetObjsToDeploy(context.Background(), gw)
			Expect(err).To(MatchError(ContainSubstring("invalid kind InvalidKind for GatewayParameters")))
		})
	})

	Context("defaulting", func() {
		var (
			registry string
			tag      string
		)
		BeforeEach(func() {
			registry = "foo"
			tag = "1.2.3"
		})
		When("a GC is created with an empty spec.parametersRef", func() {
			var d *deployer.Deployer
			It("should use the default in-memory GWP", func() {
				var objs clientObjects
				var err error

				gwc := defaultGatewayClass()
				gw := &api.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: defaultNamespace,
						UID:       "1235",
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       "Gateway",
						APIVersion: "gateway.networking.k8s.io",
					},
					Spec: api.GatewaySpec{
						GatewayClassName: wellknown.DefaultGatewayClassName,
						Listeners: []api.Listener{{
							Name: "listener-1",
							Port: 80,
						}},
					},
				}

				gwParams := internaldeployer.NewGatewayParameters(newFakeClientWithObjs(gwc), &deployer.Inputs{
					CommonCollections: newCommonCols(GinkgoT(), gwc, gw),
					Dev:               false,
					ControlPlane: deployer.ControlPlaneInfo{
						XdsHost: "something.cluster.local",
						XdsPort: 1234,
					},
					ImageInfo: &deployer.ImageInfo{
						Registry: registry,
						Tag:      tag,
					},
				})
				chart, err := internaldeployer.LoadGatewayChart()
				Expect(err).NotTo(HaveOccurred())
				d = deployer.NewDeployer(
					wellknown.DefaultGatewayControllerName,
					newFakeClientWithObjs(gwc), chart,
					gwParams,
					internaldeployer.GatewayReleaseNameAndNamespace)

				objs, err = d.GetObjsToDeploy(context.Background(), gw)
				Expect(err).NotTo(HaveOccurred())
				objs = d.SetNamespaceAndOwner(gw, objs)

				By("validating the expected objects are deployed")
				Expect(objs).NotTo(BeEmpty())
				Expect(objs.findDeployment(defaultNamespace, "foo")).NotTo(BeNil())
				Expect(objs.findService(defaultNamespace, "foo")).NotTo(BeNil())
				Expect(objs.findServiceAccount(defaultNamespace, "foo")).NotTo(BeNil())
				Expect(objs.findConfigMap(defaultNamespace, "foo")).NotTo(BeNil())

				By("validating the default values are used")
				Expect(objs.findDeployment(defaultNamespace, "foo").Spec.Template.Spec.Containers[0].Image).To(Equal(fmt.Sprintf("%s/%s:%s", registry, deployer.EnvoyWrapperImage, tag)))
			})
		})

		When("a Gateway has a GWP attached", func() {
			var (
				d   *deployer.Deployer
				gwp *gw2_v1alpha1.GatewayParameters
			)
			BeforeEach(func() {
				gwp = &gw2_v1alpha1.GatewayParameters{
					ObjectMeta: metav1.ObjectMeta{
						Name:      wellknown.DefaultGatewayParametersName,
						Namespace: defaultNamespace,
						UID:       "1237",
					},
					Spec: gw2_v1alpha1.GatewayParametersSpec{
						Kube: &gw2_v1alpha1.KubernetesProxyConfig{
							EnvoyContainer: &gw2_v1alpha1.EnvoyContainer{
								Image: &gw2_v1alpha1.Image{
									Registry: ptr.To("bar"),
									Tag:      ptr.To("2.3.4"),
								},
							},
						},
					},
				}
			})
			It("should deploy the resources with the GWP overrides", func() {
				var objs clientObjects
				var err error

				gw := &api.Gateway{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Gateway",
						APIVersion: "gateway.networking.k8s.io",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: defaultNamespace,
						UID:       "1235",
					},
					Spec: api.GatewaySpec{
						GatewayClassName: wellknown.DefaultGatewayClassName,
						Infrastructure: &api.GatewayInfrastructure{
							ParametersRef: &api.LocalParametersReference{
								Group: gw2_v1alpha1.GroupName,
								Kind:  api.Kind(wellknown.GatewayParametersGVK.Kind),
								Name:  gwp.GetName(),
							},
						},
						Listeners: []api.Listener{{
							Name: "listener-1",
							Port: 80,
						}},
					},
				}
				gwc := defaultGatewayClass()
				gwParams := internaldeployer.NewGatewayParameters(newFakeClientWithObjs(gwc, gwp), &deployer.Inputs{
					CommonCollections: newCommonCols(GinkgoT(), gwc, gw),
					Dev:               false,
					ControlPlane: deployer.ControlPlaneInfo{
						XdsHost: "something.cluster.local",
						XdsPort: 1234,
					},
					ImageInfo: &deployer.ImageInfo{
						Registry: registry,
						Tag:      tag,
					},
				})
				chart, err := internaldeployer.LoadGatewayChart()
				Expect(err).NotTo(HaveOccurred())
				d = deployer.NewDeployer(
					wellknown.DefaultGatewayControllerName,
					newFakeClientWithObjs(gwc, gwp), chart,
					gwParams,
					internaldeployer.GatewayReleaseNameAndNamespace)

				objs, err = d.GetObjsToDeploy(context.Background(), gw)
				Expect(err).NotTo(HaveOccurred())
				objs = d.SetNamespaceAndOwner(gw, objs)

				By("validating the expected objects are deployed")
				Expect(objs).NotTo(BeEmpty())
				Expect(objs.findDeployment(defaultNamespace, "foo")).NotTo(BeNil())
				Expect(objs.findService(defaultNamespace, "foo")).NotTo(BeNil())
				Expect(objs.findServiceAccount(defaultNamespace, "foo")).NotTo(BeNil())
				Expect(objs.findConfigMap(defaultNamespace, "foo")).NotTo(BeNil())

				By("validating the image overrides the default")
				Expect(objs.findDeployment(defaultNamespace, "foo").Spec.Template.Spec.Containers[0].Image).To(Equal(fmt.Sprintf("bar/%s:2.3.4", deployer.EnvoyWrapperImage)))
			})
		})

		When("a Gateway has a minimal GatewayParameters with only overrides", func() {
			var (
				d   *deployer.Deployer
				gwp *gw2_v1alpha1.GatewayParameters
				gwc *api.GatewayClass
			)
			BeforeEach(func() {
				gwp = &gw2_v1alpha1.GatewayParameters{
					TypeMeta: metav1.TypeMeta{
						Kind:       wellknown.GatewayParametersGVK.Kind,
						APIVersion: gw2_v1alpha1.GroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "minimal-gwp",
						Namespace: defaultNamespace,
					},
					Spec: gw2_v1alpha1.GatewayParametersSpec{
						Kube: &gw2_v1alpha1.KubernetesProxyConfig{
							// Only override a few values, rest should be defaulted
							Service: &gw2_v1alpha1.Service{
								Type: ptr.To(corev1.ServiceTypeClusterIP),
							},
							EnvoyContainer: &gw2_v1alpha1.EnvoyContainer{
								Bootstrap: &gw2_v1alpha1.EnvoyBootstrap{
									LogLevel: ptr.To("debug"),
								},
							},
						},
					},
				}
				gwc = defaultGatewayClassWithParamsRef()
				gwc.Spec.ParametersRef.Name = gwp.GetName()
			})
			It("should inherit defaults for non-overridden fields", func() {
				var objs clientObjects
				var err error

				gw := &api.Gateway{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Gateway",
						APIVersion: "gateway.networking.k8s.io",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: defaultNamespace,
						UID:       "1235",
					},
					Spec: api.GatewaySpec{
						GatewayClassName: wellknown.DefaultGatewayClassName,
						Infrastructure: &api.GatewayInfrastructure{
							ParametersRef: &api.LocalParametersReference{
								Group: "gateway.kgateway.dev",
								Kind:  "GatewayParameters",
								Name:  gwp.GetName(),
							},
						},
						Listeners: []api.Listener{{
							Name: "listener-1",
							Port: 80,
						}},
					},
				}
				gwParams := internaldeployer.NewGatewayParameters(newFakeClientWithObjs(gwc, gwp), &deployer.Inputs{
					CommonCollections: newCommonCols(GinkgoT(), gwc, gw),
					Dev:               false,
					ControlPlane: deployer.ControlPlaneInfo{
						XdsHost: "something.cluster.local",
						XdsPort: 1234,
					},
					ImageInfo: &deployer.ImageInfo{
						Registry: registry,
						Tag:      tag,
					},
				})
				chart, err := internaldeployer.LoadGatewayChart()
				Expect(err).NotTo(HaveOccurred())
				d = deployer.NewDeployer(
					wellknown.DefaultGatewayControllerName,
					newFakeClientWithObjs(gwc, gwp), chart,
					gwParams,
					internaldeployer.GatewayReleaseNameAndNamespace)

				objs, err = d.GetObjsToDeploy(context.Background(), gw)
				Expect(err).NotTo(HaveOccurred())
				objs = d.SetNamespaceAndOwner(gw, objs)

				By("validating the expected objects are deployed")
				Expect(objs).NotTo(BeEmpty())
				Expect(objs).To(HaveLen(4))

				By("verifying service type was overridden")
				svc := objs.findService(defaultNamespace, defaultDeploymentName)
				Expect(svc).ToNot(BeNil())
				Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))

				By("verifying deployment inherited default replicas")
				dep := objs.findDeployment(defaultNamespace, defaultDeploymentName)
				Expect(dep).ToNot(BeNil())
				Expect(*dep.Spec.Replicas).To(Equal(int32(1)))

				By("verifying envoy container log level was overridden")
				envoyContainer := dep.Spec.Template.Spec.Containers[0]
				foundLogLevel := false
				for i, arg := range envoyContainer.Args {
					if strings.HasPrefix(arg, "--log-level") {
						Expect(envoyContainer.Args[i+1]).To(Equal("debug"))
						foundLogLevel = true
						break
					}
				}
				Expect(foundLogLevel).To(BeTrue(), "envoy proxy log level not found")

				bootstrapCfg := objs.getEnvoyConfig(defaultNamespace, defaultConfigMapName)
				Expect(bootstrapCfg.StaticResources.Listeners).To(HaveLen(2))
				prometheusListener := bootstrapCfg.StaticResources.Listeners[1]
				port := prometheusListener.Address.GetSocketAddress().PortSpecifier.(*envoycorev3.SocketAddress_PortValue)
				Expect(port.PortValue).To(Equal(uint32(9091)))

				By("verifying image registry and tag were inherited")
				Expect(envoyContainer.Image).To(Equal(fmt.Sprintf("%s/%s:%s", registry, deployer.EnvoyWrapperImage, tag)))
			})
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
			validationFunc func(objs clientObjects, inp *input) error
		}

		var (
			gwpOverrideName = "gateway-params-override"

			defaultGatewayParamsOverride = func() *gw2_v1alpha1.GatewayParameters {
				return &gw2_v1alpha1.GatewayParameters{
					TypeMeta: metav1.TypeMeta{
						Kind: wellknown.GatewayParametersGVK.Kind,
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
								ExternalTrafficPolicy: ptr.To(string(corev1.ServiceExternalTrafficPolicyTypeLocal)),
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
			mergedGatewayParamsNoLowPorts = func() *gw2_v1alpha1.GatewayParameters {
				return &gw2_v1alpha1.GatewayParameters{
					TypeMeta: metav1.TypeMeta{
						Kind: wellknown.GatewayParametersGVK.Kind,
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
								ExternalTrafficPolicy: ptr.To(string(corev1.ServiceExternalTrafficPolicyTypeLocal)),
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
			mergedGatewayParams = func() *gw2_v1alpha1.GatewayParameters {
				gwp := mergedGatewayParamsNoLowPorts()
				gwp.Spec.Kube.PodTemplate.SecurityContext.Sysctls = []corev1.Sysctl{
					{
						Name:  "net.ipv4.ip_unprivileged_port_start",
						Value: "0",
					},
				}
				return gwp
			}
			gatewayParamsOverrideWithSds = func() *gw2_v1alpha1.GatewayParameters {
				return &gw2_v1alpha1.GatewayParameters{
					TypeMeta: metav1.TypeMeta{
						Kind: wellknown.GatewayParametersGVK.Kind,
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
								Ports: []corev1.ContainerPort{{
									Name:          "foo",
									ContainerPort: 80,
								}},
								Tracing: &gw2_v1alpha1.AiExtensionTrace{
									EndPoint: "http://my-otel-collector.svc.cluster.local:4317",
									Sampler: &gw2_v1alpha1.OTelTracesSampler{
										SamplerType: ptr.To(gw2_v1alpha1.OTelTracesSamplerTraceidratio),
										SamplerArg:  ptr.To("0.5"),
									},
									Timeout:  &metav1.Duration{Duration: 100 * time.Second},
									Protocol: ptr.To(gw2_v1alpha1.OTLPTracesProtocolTypeGrpc),
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
						Kind: wellknown.GatewayParametersGVK.Kind,
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

			gwParamsNoPodTemplate = func() *gw2_v1alpha1.GatewayParameters {
				params := fullyDefinedGatewayParameters(wellknown.DefaultGatewayParametersName, defaultNamespace)
				params.Spec.Kube.PodTemplate = nil
				return params
			}

			fullyDefinedGatewayParamsWithUnprivilegedPortStartSysctl = func() *gw2_v1alpha1.GatewayParameters {
				params := fullyDefinedGatewayParameters(wellknown.DefaultGatewayParametersName, defaultNamespace)
				params.Spec.Kube.PodTemplate.SecurityContext.Sysctls = []corev1.Sysctl{
					{
						Name:  "net.ipv4.ip_unprivileged_port_start",
						Value: "100",
					},
				}
				return params
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

			fullyDefinedGatewayParamsWithCustomEnv = func() *gw2_v1alpha1.GatewayParameters {
				params := fullyDefinedGatewayParameters(wellknown.DefaultGatewayParametersName, defaultNamespace)
				params.Spec.Kube.EnvoyContainer.Env = []corev1.EnvVar{
					{
						Name:  "CUSTOM_ENV",
						Value: "abcd",
					},
				}
				return params
			}

			fullyDefinedGatewayParamsWithFloatingUserId = func() *gw2_v1alpha1.GatewayParameters {
				params := fullyDefinedGatewayParameters(wellknown.DefaultGatewayParametersName, defaultNamespace)
				params.Spec.Kube.FloatingUserId = ptr.To(true)
				params.Spec.Kube.PodTemplate.SecurityContext.RunAsUser = nil
				return params
			}

			withGatewayParams = func(gw *api.Gateway, gwpName string) *api.Gateway {
				gw.Spec.Infrastructure = &api.GatewayInfrastructure{
					ParametersRef: &api.LocalParametersReference{
						Group: gw2_v1alpha1.GroupName,
						Kind:  api.Kind(wellknown.GatewayParametersGVK.Kind),
						Name:  gwpName,
					},
				}
				return gw
			}

			highPortGatewayWithGatewayParams = func(gwpName string) *api.Gateway {
				return withGatewayParams(highPortGateway(), gwpName)
			}

			defaultGatewayWithGatewayParams = func(gwpName string) *api.Gateway {
				return withGatewayParams(defaultGateway(), gwpName)
			}

			defaultDeployerInputs = func() *deployer.Inputs {
				return &deployer.Inputs{
					Dev: false,
					ControlPlane: deployer.ControlPlaneInfo{
						XdsHost: "something.cluster.local", XdsPort: 1234,
					},
					ImageInfo: &deployer.ImageInfo{
						Registry: "foo",
						Tag:      "bar",
					},
					GatewayClassName:         wellknown.DefaultGatewayClassName,
					WaypointGatewayClassName: wellknown.DefaultWaypointClassName,
					AgentGatewayClassName:    wellknown.DefaultAgentGatewayClassName,
				}
			}
			istioEnabledDeployerInputs = func() *deployer.Inputs {
				inp := defaultDeployerInputs()
				inp.IstioAutoMtlsEnabled = true
				return inp
			}
			defaultInput = func() *input {
				return &input{
					dInputs:    defaultDeployerInputs(),
					defaultGwp: defaultGatewayParams(),
					gw:         defaultGateway(),
					gwc:        defaultGatewayClassWithParamsRef(),
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

				Expect(dep.Spec.Template.Spec.SecurityContext).To(Equal(expectedGwp.PodTemplate.SecurityContext))

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
				Expect(svc.Spec.ExternalTrafficPolicy).To(Equal(corev1.ServiceExternalTrafficPolicyTypeLocal))

				sa := objs.findServiceAccount(defaultNamespace, defaultServiceAccountName)
				Expect(sa).ToNot(BeNil())
				Expect(sa.GetAnnotations()).ToNot(BeNil())
				Expect(sa.GetAnnotations()).To(containMapElements(expectedGwp.ServiceAccount.ExtraAnnotations))
				Expect(sa.GetLabels()).ToNot(BeNil())
				Expect(sa.GetLabels()).To(containMapElements(expectedGwp.ServiceAccount.ExtraLabels))

				cm := objs.findConfigMap(defaultNamespace, defaultConfigMapName)
				Expect(cm).ToNot(BeNil())
				// This verifies that the cluster name provided to envoy matches the service name used when generating OTel stats
				Expect(cm.Data[envoyDataKey]).To(ContainSubstring(fmt.Sprintf("cluster: %s", httplistenerpolicy.GenerateDefaultServiceName(dep.Name, dep.Namespace))))

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

		// fullyDefinedValidationWithoutRunAsUser doesn't validate "runAsUser" at the container level
		// The entire PodSecurityContext is validated in this function.
		fullyDefinedValidationWithoutRunAsUser := func(objs clientObjects, inp *input) error {
			expectedGwp := inp.defaultGwp.Spec.Kube
			Expect(objs).NotTo(BeEmpty())
			// Check we have Deployment, Envoy ConfigMap, ServiceAccount, Service, AI Stats ConfigMap
			Expect(objs).To(HaveLen(5))
			dep := objs.findDeployment(defaultNamespace, defaultDeploymentName)
			Expect(dep).ToNot(BeNil())
			Expect(dep.Spec.Replicas).ToNot(BeNil())
			Expect(*dep.Spec.Replicas).To(Equal(int32(*expectedGwp.Deployment.Replicas)))

			// Calculate expected PodSecurityContext. The deployer conditionall addes the net.ipv4.ip_unprivileged_port_start=0 sysctl
			// to the default parameters if the gateway uses low ports.

			// There are tests that don't set a pod template, so we need to handle that case
			if expectedGwp.PodTemplate == nil {
				expectedGwp.PodTemplate = &gw2_v1alpha1.Pod{
					SecurityContext: &corev1.PodSecurityContext{},
				}
			}

			expectedPodSecurityContext := expectedGwp.PodTemplate.SecurityContext

			gwUsesLowPorts := false
			for _, listener := range inp.gw.Spec.Listeners {
				if listener.Port < 1024 {
					gwUsesLowPorts = true
					break
				}
			}
			if gwUsesLowPorts {
				expectedPodSecurityContext.Sysctls = []corev1.Sysctl{{
					Name:  "net.ipv4.ip_unprivileged_port_start",
					Value: "0",
				}}
			}
			// assert pod level security context
			Expect(dep.Spec.Template.Spec.SecurityContext).To(Equal(expectedPodSecurityContext))

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
			Expect(svc.Spec.ExternalTrafficPolicy).To(Equal(corev1.ServiceExternalTrafficPolicyTypeLocal))

			sa := objs.findServiceAccount(defaultNamespace, defaultServiceAccountName)
			Expect(sa).ToNot(BeNil())
			Expect(sa.GetAnnotations()).ToNot(BeNil())
			Expect(sa.GetAnnotations()).To(containMapElements(expectedGwp.ServiceAccount.ExtraAnnotations))
			Expect(sa.GetLabels()).ToNot(BeNil())
			Expect(sa.GetLabels()).To(containMapElements(expectedGwp.ServiceAccount.ExtraLabels))

			cm := objs.findConfigMap(defaultNamespace, defaultConfigMapName)
			Expect(cm).ToNot(BeNil())

			aiTracingConfig := objs.findConfigMap(defaultNamespace, defaultConfigMapName+"-ai-otel-config")
			Expect(aiTracingConfig).ToNot(BeNil())

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

			deployment := objs.findDeployment(defaultNamespace, defaultDeploymentName)

			Expect(deployment.Spec.Template.Spec.TopologySpreadConstraints).To(Equal(expectedGwp.PodTemplate.TopologySpreadConstraints))
			Expect(deployment.Spec.Template.Spec.Tolerations).To(Equal(expectedGwp.PodTemplate.Tolerations))
			Expect(deployment.Spec.Template.Spec.Affinity).To(Equal(expectedGwp.PodTemplate.Affinity))
			Expect(deployment.Spec.Template.Spec.NodeSelector).To(Equal(expectedGwp.PodTemplate.NodeSelector))

			return nil
		}

		validateRunAsUser := func(objs clientObjects, inp *input) {
			expectedGwp := inp.defaultGwp.Spec.Kube
			dep := objs.findDeployment(defaultNamespace, defaultDeploymentName)
			Expect(dep.Spec.Template.Spec.SecurityContext.RunAsUser).To(Equal(expectedGwp.PodTemplate.SecurityContext.RunAsUser))

			sdsContainer := dep.Spec.Template.Spec.Containers[1]
			Expect(sdsContainer.SecurityContext.RunAsUser).To(Equal(expectedGwp.SdsContainer.SecurityContext.RunAsUser))

			istioContainer := dep.Spec.Template.Spec.Containers[2]
			Expect(istioContainer.SecurityContext.RunAsUser).To(Equal(expectedGwp.Istio.IstioProxyContainer.SecurityContext.RunAsUser))
		}

		fullyDefinedValidation := func(objs clientObjects, inp *input) error {
			err := fullyDefinedValidationWithoutRunAsUser(objs, inp)
			if err != nil {
				return err
			}

			validateRunAsUser(objs, inp)

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

		fullyDefinedValidationCustomEnv := func(objs clientObjects, inp *input) error {
			err := fullyDefinedValidationWithoutRunAsUser(objs, inp)
			if err != nil {
				return err
			}

			envoyContainer := objs.findDeployment(defaultNamespace, defaultDeploymentName).Spec.Template.Spec.Containers[0]
			Expect(envoyContainer.Env).To(ContainElement(corev1.EnvVar{
				Name:      "CUSTOM_ENV",
				Value:     "abcd",
				ValueFrom: nil,
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
				case deployer.SdsContainerName:
					sdsContainer = container
					foundSds = true
				case deployer.IstioContainerName:
					istioProxyContainer = container
					foundIstioProxy = true
				case deployer.KgatewayContainerName:
					gwContainer = container
					foundGw = true
				case deployer.KgatewayAIContainerName:
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

			By("validating the override values are set in the istio container")
			Expect(istioProxyContainer.Env).To(ContainElement(corev1.EnvVar{
				Name:      "CA_ADDR",
				Value:     *inp.overrideGwp.Spec.Kube.Istio.IstioProxyContainer.IstioDiscoveryAddress,
				ValueFrom: nil,
			}))
			Expect(istioProxyContainer.Env).To(ContainElement(corev1.EnvVar{
				Name:      "ISTIO_META_MESH_ID",
				Value:     *inp.overrideGwp.Spec.Kube.Istio.IstioProxyContainer.IstioMetaMeshId,
				ValueFrom: nil,
			}))
			Expect(istioProxyContainer.Env).To(ContainElement(corev1.EnvVar{
				Name:      "ISTIO_META_CLUSTER_ID",
				Value:     *inp.overrideGwp.Spec.Kube.Istio.IstioProxyContainer.IstioMetaClusterId,
				ValueFrom: nil,
			}))

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
					Expect(err.Error()).To(ContainSubstring(expectedErr.Error()))
					return true
				}
				Expect(err).NotTo(HaveOccurred())
				return false
			}

			inputObjs := []client.Object{defaultGateway(), defaultGatewayClass()}
			if inp.gw != nil {
				inputObjs = append(inputObjs, inp.gw)
			}
			if inp.gwc != nil {
				inputObjs = append(inputObjs, inp.gwc)
			}
			if inp.dInputs != nil {
				inp.dInputs.CommonCollections = newCommonCols(GinkgoT(), inputObjs...)
			}

			// run break-glass setup
			if inp.arbitrarySetup != nil {
				inp.arbitrarySetup()
			}

			// Catch nil objs so the fake client doesn't choke
			gwc := inp.gwc
			if gwc == nil {
				gwc = defaultGatewayClassWithParamsRef()
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

			gwParams := internaldeployer.NewGatewayParameters(newFakeClientWithObjs(gwc, defaultGwp, overrideGwp), inp.dInputs)
			chart, err := internaldeployer.LoadGatewayChart()
			Expect(err).NotTo(HaveOccurred())
			d := deployer.NewDeployer(wellknown.DefaultGatewayControllerName, newFakeClientWithObjs(gwc, defaultGwp, overrideGwp), chart,
				gwParams,
				internaldeployer.GatewayReleaseNameAndNamespace)

			objs, err := d.GetObjsToDeploy(context.Background(), inp.gw)
			if checkErr(err, expected.getObjsErr) {
				return
			}
			objs = d.SetNamespaceAndOwner(inp.gw, objs)

			// handle custom test validation func
			Expect(expected.validationFunc(objs, inp)).NotTo(HaveOccurred())
		},
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
			Entry("high port gateway", &input{
				dInputs:     defaultDeployerInputs(),
				gw:          highPortGatewayWithGatewayParams(gwpOverrideName),
				defaultGwp:  defaultGatewayParams(),
				overrideGwp: defaultGatewayParamsOverride(),
			}, &expectedOutput{
				validationFunc: func(objs clientObjects, inp *input) error {
					return validateGatewayParametersPropagation(objs, mergedGatewayParamsNoLowPorts())
				},
			}),
			Entry("Fully defined GatewayParameters", &input{
				dInputs:    istioEnabledDeployerInputs(),
				gw:         defaultGateway(),
				defaultGwp: fullyDefinedGatewayParams(),
			}, &expectedOutput{
				validationFunc: fullyDefinedValidation,
			}),
			Entry("Fully defined GatewayParameters with ip_unprivileged_port_start sysctl already defined", &input{
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
			Entry("Fully defined GatewayParameters with unprivileged port start sysctl", &input{
				dInputs:    istioEnabledDeployerInputs(),
				gw:         defaultGateway(),
				defaultGwp: fullyDefinedGatewayParamsWithUnprivilegedPortStartSysctl(),
			}, &expectedOutput{
				validationFunc: fullyDefinedValidation,
			}),
			Entry("Fully defined GatewayParameters with no pod template", &input{
				dInputs:    istioEnabledDeployerInputs(),
				gw:         defaultGateway(),
				defaultGwp: gwParamsNoPodTemplate(),
			}, &expectedOutput{
				validationFunc: fullyDefinedValidation,
			}),
			Entry("Fully defined GatewayParameters with custom env vars", &input{
				dInputs:    istioEnabledDeployerInputs(),
				gw:         defaultGateway(),
				defaultGwp: fullyDefinedGatewayParamsWithCustomEnv(),
			}, &expectedOutput{
				validationFunc: fullyDefinedValidationCustomEnv,
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
						APIVersion: "gateway.networking.k8s.io",
					},
					Spec: api.GatewaySpec{
						GatewayClassName: wellknown.DefaultGatewayClassName,
					},
				},
				defaultGwp: defaultGatewayParams(),
			}, &expectedOutput{
				validationFunc: func(objs clientObjects, inp *input) error {
					Expect(objs).NotTo(BeEmpty())
					return nil
				},
			}),
			Entry("no port offset", defaultInput(), &expectedOutput{
				validationFunc: func(objs clientObjects, inp *input) error {
					svc := objs.findService(defaultNamespace, defaultServiceName)
					Expect(svc).NotTo(BeNil())

					port := svc.Spec.Ports[0]
					Expect(port.Port).To(Equal(int32(80)))
					Expect(port.TargetPort.IntVal).To(Equal(int32(80)))
					Expect(port.NodePort).To(Equal(int32(0)))
					return nil
				},
			}),
			Entry("static NodePort", &input{
				dInputs:    defaultDeployerInputs(),
				gw:         defaultGatewayWithGatewayParams(gwpOverrideName),
				defaultGwp: defaultGatewayParams(),
				overrideGwp: &gw2_v1alpha1.GatewayParameters{
					ObjectMeta: metav1.ObjectMeta{
						Name:      gwpOverrideName,
						Namespace: defaultNamespace,
					},
					Spec: gw2_v1alpha1.GatewayParametersSpec{
						Kube: &gw2_v1alpha1.KubernetesProxyConfig{
							Service: &gw2_v1alpha1.Service{
								Type: ptr.To(corev1.ServiceTypeNodePort),
								Ports: []gw2_v1alpha1.Port{
									{
										Port:     80,
										NodePort: ptr.To(uint16(30000)),
									},
								},
							},
						},
					},
				},
			}, &expectedOutput{
				validationFunc: func(objs clientObjects, inp *input) error {
					svc := objs.findService(defaultNamespace, defaultServiceName)
					Expect(svc).NotTo(BeNil())

					port := svc.Spec.Ports[0]
					Expect(port.Port).To(Equal(int32(80)))
					Expect(port.TargetPort.IntVal).To(Equal(int32(80)))
					Expect(port.NodePort).To(Equal(int32(30000)))
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
						APIVersion: "gateway.networking.k8s.io",
					},
					Spec: api.GatewaySpec{
						GatewayClassName: wellknown.DefaultGatewayClassName,
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
					Expect(port.TargetPort.IntVal).To(Equal(int32(80)))
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

					envoyYaml := cm.Data[envoyDataKey]
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
				gwc:         defaultGatewayClassWithParamsRef(),
			}, &expectedOutput{
				validationFunc: func(objs clientObjects, inp *input) error {
					gw := defaultGatewayWithGatewayParams(gwpOverrideName)
					Expect(objs).NotTo(BeEmpty())

					cm := objs.findConfigMap(defaultNamespace, defaultConfigMapName)
					Expect(cm).NotTo(BeNil())

					envoyYaml := cm.Data[envoyDataKey]
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
				getObjsErr: deployer.GatewayParametersError,
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
			Entry("OmitReplicas is true", &input{
				dInputs: defaultDeployerInputs(),
				gw:      defaultGateway(),
				defaultGwp: &gw2_v1alpha1.GatewayParameters{
					TypeMeta: metav1.TypeMeta{
						Kind:       wellknown.GatewayParametersGVK.Kind,
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
								OmitReplicas: ptr.To(true),
							},
						},
					},
				},
				overrideGwp: &gw2_v1alpha1.GatewayParameters{},
			}, &expectedOutput{
				validationFunc: func(objs clientObjects, inp *input) error {
					deployment := objs.findDeployment(defaultNamespace, defaultServiceName)
					Expect(deployment).NotTo(BeNil())
					Expect(deployment.Spec.Replicas).To(BeNil())
					return nil
				},
			}),
			Entry("have replicas set", &input{
				dInputs: defaultDeployerInputs(),
				gw:      defaultGateway(),
				defaultGwp: &gw2_v1alpha1.GatewayParameters{
					TypeMeta: metav1.TypeMeta{
						Kind:       wellknown.GatewayParametersGVK.Kind,
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
								Replicas: ptr.To(uint32(3)),
							},
						},
					},
				},
				overrideGwp: &gw2_v1alpha1.GatewayParameters{},
			}, &expectedOutput{
				validationFunc: func(objs clientObjects, inp *input) error {
					deployment := objs.findDeployment(defaultNamespace, defaultServiceName)
					Expect(deployment).NotTo(BeNil())
					Expect(*deployment.Spec.Replicas).To(Equal(int32(3)))
					return nil
				},
			}),
			Entry("replicas and omitReplicas aren't set (default)", &input{
				dInputs: defaultDeployerInputs(),
				gw:      defaultGateway(),
				defaultGwp: &gw2_v1alpha1.GatewayParameters{
					TypeMeta: metav1.TypeMeta{
						Kind:       wellknown.GatewayParametersGVK.Kind,
						APIVersion: gw2_v1alpha1.GroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      wellknown.DefaultGatewayParametersName,
						Namespace: defaultNamespace,
						UID:       "1237",
					},
					Spec: gw2_v1alpha1.GatewayParametersSpec{
						Kube: &gw2_v1alpha1.KubernetesProxyConfig{
							Deployment: &gw2_v1alpha1.ProxyDeployment{},
						},
					},
				},
				overrideGwp: &gw2_v1alpha1.GatewayParameters{},
			}, &expectedOutput{
				validationFunc: func(objs clientObjects, inp *input) error {
					deployment := objs.findDeployment(defaultNamespace, defaultServiceName)
					Expect(deployment).NotTo(BeNil())
					Expect(*deployment.Spec.Replicas).To(Equal(int32(1))) // default replicas is 1
					return nil
				},
			}),
		)
	})

	Context("Inference Extension endpoint picker", func() {
		const defaultNamespace = "default"

		It("should deploy endpoint picker resources for an InferencePool when autoProvision is enabled", func() {
			// Create a fake InferencePool resource.
			pool := &infextv1a2.InferencePool{
				TypeMeta: metav1.TypeMeta{
					Kind:       wellknown.InferencePoolKind,
					APIVersion: fmt.Sprintf("%s/%s", infextv1a2.GroupVersion.Group, infextv1a2.GroupVersion.Version),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pool1",
					Namespace: defaultNamespace,
					UID:       "pool-uid",
				},
			}

			// Initialize a new deployer with InferenceExtension inputs.
			ie := &internaldeployer.InferenceExtension{}
			chart, err := internaldeployer.LoadInferencePoolChart()
			Expect(err).NotTo(HaveOccurred())
			cli := newFakeClientWithObjs(pool)
			d := deployer.NewDeployer(wellknown.DefaultGatewayControllerName, cli, chart,
				ie,
				internaldeployer.InferenceExtensionReleaseNameAndNamespace)

			// Simulate reconciliation so that the pool gets its finalizer added.
			err = controller.EnsureFinalizer(context.Background(), cli, pool)
			Expect(err).NotTo(HaveOccurred())

			// Check that the pool itself has the finalizer set.
			Expect(pool.GetFinalizers()).To(ContainElement(wellknown.InferencePoolFinalizer))

			// Get the endpoint picker objects for the InferencePool.
			objs, err := d.GetObjsToDeploy(context.Background(), pool)
			Expect(err).NotTo(HaveOccurred())
			objs = d.SetNamespaceAndOwner(pool, objs)

			Expect(objs).NotTo(BeEmpty(), "expected non-empty objects for endpoint picker deployment")
			Expect(objs).To(HaveLen(4))

			// Find the child objects.
			var sa *corev1.ServiceAccount
			var crb *rbacv1.ClusterRoleBinding
			var dep *appsv1.Deployment
			var svc *corev1.Service
			for _, obj := range objs {
				switch t := obj.(type) {
				case *corev1.ServiceAccount:
					sa = t
				case *rbacv1.ClusterRoleBinding:
					crb = t
				case *appsv1.Deployment:
					dep = t
				case *corev1.Service:
					svc = t
				}
			}
			Expect(sa).NotTo(BeNil(), "expected a ServiceAccount to be rendered")
			Expect(crb).NotTo(BeNil(), "expected a ClusterRoleBinding to be rendered")
			Expect(dep).NotTo(BeNil(), "expected a Deployment to be rendered")
			Expect(svc).NotTo(BeNil(), "expected a Service to be rendered")

			// Check that owner references are set on all rendered objects to the InferencePool.
			for _, obj := range objs {
				gvk := obj.GetObjectKind().GroupVersionKind()
				if deployer.IsNamespaced(gvk) {
					ownerRefs := obj.GetOwnerReferences()
					Expect(ownerRefs).To(HaveLen(1))
					ref := ownerRefs[0]
					Expect(ref.Name).To(Equal(pool.Name))
					Expect(ref.UID).To(Equal(pool.UID))
					Expect(ref.Kind).To(Equal(pool.Kind))
					Expect(ref.APIVersion).To(Equal(pool.APIVersion))
					Expect(*ref.Controller).To(BeTrue())
				}
			}

			// Validate that the rendered Deployment and Service have the expected names.
			expectedName := fmt.Sprintf("%s-endpoint-picker", pool.Name)
			Expect(sa.Name).To(Equal(expectedName))
			Expect(crb.Name).To(Equal(expectedName))
			Expect(dep.Name).To(Equal(expectedName))
			Expect(svc.Name).To(Equal(expectedName))

			// Check the container args for the expected poolName.
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
			pickerContainer := dep.Spec.Template.Spec.Containers[0]
			Expect(pickerContainer.Args).To(Equal([]string{
				"-poolName",
				pool.Name,
				"-v",
				"4",
				"-grpcPort",
				"9002",
				"-grpcHealthPort",
				"9003",
			}))
		})
	})

	Context("with listener sets", func() {
		var (
			listenerSetPort int32 = 4567
			listenerPort    int32 = 1234
		)

		It("exposes all necessary ports", func() {
			allNamespaces := api.NamespacesFromAll
			gw := &api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: defaultNamespace,
					UID:       "1235",
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Gateway",
					APIVersion: "gateway.networking.k8s.io",
				},
				Spec: api.GatewaySpec{
					GatewayClassName: wellknown.DefaultGatewayClassName,
					AllowedListeners: &api.AllowedListeners{
						Namespaces: &api.ListenerNamespaces{
							From: &allNamespaces,
						},
					},
					Listeners: []api.Listener{
						{
							Name: "gateway-listener",
							Port: api.PortNumber(listenerPort),
						},
					},
				},
			}

			ls := &apixv1a1.XListenerSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ls",
					Namespace: defaultNamespace,
				},
				Spec: apixv1a1.ListenerSetSpec{
					Listeners: []apixv1a1.ListenerEntry{
						{
							Name: "listenerset-listener",
							Port: apixv1a1.PortNumber(listenerSetPort),
						},
					},
					ParentRef: apixv1a1.ParentGatewayReference{
						Kind:      (*apixv1a1.Kind)(&gw.Kind),
						Group:     (*apixv1a1.Group)(&gw.APIVersion),
						Name:      apixv1a1.ObjectName(gw.Name),
						Namespace: (*apixv1a1.Namespace)(&gw.Namespace),
					},
				},
			}

			gwParams := internaldeployer.NewGatewayParameters(
				newFakeClientWithObjs(defaultGatewayClass(), defaultGatewayParams()), &deployer.Inputs{
					CommonCollections: newCommonCols(GinkgoT(), defaultGatewayClass(), gw, ls),
					Dev:               false,
					ControlPlane: deployer.ControlPlaneInfo{
						XdsHost: "something.cluster.local", XdsPort: 1234,
					},
					ImageInfo: &deployer.ImageInfo{
						Registry: "foo",
						Tag:      "bar",
					},
				})
			chart, err := internaldeployer.LoadGatewayChart()
			Expect(err).NotTo(HaveOccurred())
			d := deployer.NewDeployer(
				wellknown.DefaultGatewayControllerName,
				newFakeClientWithObjs(defaultGatewayClass(), defaultGatewayParams()),
				chart, gwParams, internaldeployer.GatewayReleaseNameAndNamespace)

			var objs clientObjects
			objs, err = d.GetObjsToDeploy(context.Background(), gw)
			Expect(err).NotTo(HaveOccurred())
			objs = d.SetNamespaceAndOwner(gw, objs)

			Expect(objs).To(HaveLen(4))
			Expect(objs.findConfigMap(defaultNamespace, gw.Name)).ToNot(BeNil())
			Expect(objs.findServiceAccount(defaultNamespace, gw.Name)).ToNot(BeNil())

			servicePorts := objs.findService(defaultNamespace, gw.Name).Spec.Ports
			Expect(servicePorts[0].Name).To(Equal(fmt.Sprintf("listener-%d", listenerPort)))
			Expect(servicePorts[0].Port).To(Equal(listenerPort))
			Expect(servicePorts[1].Name).To(Equal(fmt.Sprintf("listener-%d", listenerSetPort)))
			Expect(servicePorts[1].Port).To(Equal(listenerSetPort))

			deploymentPorts := objs.findDeployment(defaultNamespace, gw.Name).Spec.Template.Spec.Containers[0].Ports
			Expect(deploymentPorts[0].Name).To(Equal(fmt.Sprintf("listener-%d", listenerPort)))
			Expect(deploymentPorts[0].ContainerPort).To(Equal(listenerPort))
			Expect(deploymentPorts[1].Name).To(Equal(fmt.Sprintf("listener-%d", listenerSetPort)))
			Expect(deploymentPorts[1].ContainerPort).To(Equal(listenerSetPort))
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
			if err := infextv1a2.Install(scheme); err != nil {
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
			Kind: wellknown.GatewayParametersGVK.Kind,
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
					TopologySpreadConstraints: []corev1.TopologySpreadConstraint{{
						MaxSkew:           1,
						TopologyKey:       "pod-topology-spread-constraint-topology-key",
						WhenUnsatisfiable: "pod-topology-spread-constraint-when-unsatisfiable",
						LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"pod-topology-spread-constraint-label-selector": "foo"}},
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
					ExternalTrafficPolicy: ptr.To(string(corev1.ServiceExternalTrafficPolicyTypeLocal)),
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
					Tracing: &gw2_v1alpha1.AiExtensionTrace{
						EndPoint: "http://my-otel-collector.svc.cluster.local:4317",
						Sampler: &gw2_v1alpha1.OTelTracesSampler{
							SamplerType: ptr.To(gw2_v1alpha1.OTelTracesSamplerTraceidratio),
							SamplerArg:  ptr.To("0.5"),
						},
						Timeout:  &metav1.Duration{Duration: 100 * time.Second},
						Protocol: ptr.To(gw2_v1alpha1.OTLPTracesProtocolTypeGrpc),
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

func newCommonCols(t test.Failer, initObjs ...client.Object) *common.CommonCollections {
	ctx := context.Background()
	var anys []any
	for _, obj := range initObjs {
		anys = append(anys, obj)
	}
	mock := krttest.NewMock(t, anys)

	policies := krtcollections.NewPolicyIndex(krtutil.KrtOptions{}, extensionsplug.ContributesPolicies{}, settings.Settings{})
	kubeRawGateways := krttest.GetMockCollection[*api.Gateway](mock)
	kubeRawListenerSets := krttest.GetMockCollection[*apixv1a1.XListenerSet](mock)
	gatewayClasses := krttest.GetMockCollection[*api.GatewayClass](mock)
	nsCol := krtcollections.NewNamespaceCollectionFromCol(ctx, krttest.GetMockCollection[*corev1.Namespace](mock), krtutil.KrtOptions{})

	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
	gateways := krtcollections.NewGatewayIndex(krtopts, wellknown.DefaultGatewayControllerName, policies, kubeRawGateways, kubeRawListenerSets, gatewayClasses, nsCol)

	commonCols := &common.CommonCollections{
		GatewayIndex: gateways,
	}

	for !kubeRawGateways.HasSynced() || !kubeRawListenerSets.HasSynced() || !gatewayClasses.HasSynced() {
		time.Sleep(time.Second / 10)
	}

	gateways.Gateways.WaitUntilSynced(ctx.Done())
	return commonCols
}

var _ = Describe("DeployObjs", func() {
	var (
		ns   = "test-ns"
		name = "test-obj"
		ctx  = context.Background()
	)

	var getDeployer = func(fc *fakeClient) *deployer.Deployer {
		chart, err := internaldeployer.LoadGatewayChart()
		Expect(err).ToNot(HaveOccurred())
		return deployer.NewDeployer(wellknown.DefaultGatewayControllerName, fc, chart,
			nil,
			internaldeployer.GatewayReleaseNameAndNamespace)
	}

	It("skips patch if object is unchanged", func() {
		patched := false
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Data: map[string]string{"foo": "bar"}}
		fc := &fakeClient{
			Client: newFakeClientWithObjs(cm.DeepCopy()),
			patchFunc: func(_ context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
				Fail("should not be called")
				return errors.New("should not be called")
			},
		}
		d := getDeployer(fc)

		err := d.DeployObjs(ctx, []client.Object{cm})
		Expect(err).ToNot(HaveOccurred())
		Expect(patched).To(BeFalse())
	})

	It("skips patch only changed is object status", func() {
		patched := false
		pod1 := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "test", Image: "test:latest"}}}, Status: corev1.PodStatus{Phase: corev1.PodPending}}
		pod2 := pod1.DeepCopy()

		// obj to deploy won't have a status set.
		pod2.Status = corev1.PodStatus{}
		fc := &fakeClient{
			Client: newFakeClientWithObjs(pod1.DeepCopy()),
			patchFunc: func(_ context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
				Fail("should not be called")
				return errors.New("should not be called")
			},
		}
		d := getDeployer(fc)

		err := d.DeployObjs(ctx, []client.Object{pod2})
		Expect(err).ToNot(HaveOccurred())
		Expect(patched).To(BeFalse())
	})

	It("patches if object is different", func() {
		patched := false
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Data: map[string]string{"foo": "bar"}}
		fc := &fakeClient{
			Client: newFakeClientWithObjs(cm.DeepCopy()),
			patchFunc: func(_ context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
				patched = true
				return nil
			},
		}
		cm.Data = map[string]string{"foo": "bar", "bar": "baz"}
		d := getDeployer(fc)
		err := d.DeployObjs(ctx, []client.Object{cm})
		Expect(err).ToNot(HaveOccurred())
		Expect(patched).To(BeTrue())
	})

	It("patches if object does not exist (IsNotFound)", func() {
		patched := false
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
		fc := &fakeClient{
			Client: newFakeClientWithObjs(),
			patchFunc: func(_ context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
				patched = true
				return nil
			},
		}
		d := getDeployer(fc)
		err := d.DeployObjs(ctx, []client.Object{cm})
		Expect(err).ToNot(HaveOccurred())
		Expect(patched).To(BeTrue())
	})

	It("patches if Get returns a non-IsNotFound error", func() {
		patched := false
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
		fc := &fakeClient{
			Client: newFakeClientWithObjs(cm.DeepCopy()),
			getFunc: func(_ context.Context, _ client.ObjectKey, _ client.Object) error {
				return fmt.Errorf("some random error")
			},
			patchFunc: func(_ context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
				patched = true
				return nil
			},
		}
		d := getDeployer(fc)
		err := d.DeployObjs(ctx, []client.Object{cm})
		Expect(err).ToNot(HaveOccurred())
		Expect(patched).To(BeTrue())
	})
})

type fakeClient struct {
	client.Client
	getFunc   func(ctx context.Context, key client.ObjectKey, obj client.Object) error
	patchFunc func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error
}

func (f *fakeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if f.getFunc != nil {
		return f.getFunc(ctx, key, obj)
	}
	return f.Client.Get(ctx, key, obj)
}
func (f *fakeClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if f.patchFunc != nil {
		return f.patchFunc(ctx, obj, patch, opts...)
	}
	return f.Client.Patch(ctx, obj, patch, opts...)
}
