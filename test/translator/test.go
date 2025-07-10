package translator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"istio.io/istio/pkg/config/schema/gvr"
	kubeclient "istio.io/istio/pkg/kube"

	"istio.io/istio/pkg/kube/kclient/clienttest"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/registry"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/listener"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	common "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/pkg/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
)

type AssertReports func(gwNN types.NamespacedName, reportsMap reports.ReportMap)

type translationResult struct {
	Routes        []*envoy_config_route_v3.RouteConfiguration
	Listeners     []*envoy_config_listener_v3.Listener
	ExtraClusters []*clusterv3.Cluster
	Clusters      []*clusterv3.Cluster
}

func (tr *translationResult) MarshalJSON() ([]byte, error) {
	m := protojson.MarshalOptions{
		Indent: "  ",
	}

	// Create a map to hold the marshaled fields
	result := make(map[string]interface{})

	// Marshal each field using protojson
	if len(tr.Routes) > 0 {
		routes, err := marshalProtoMessages(tr.Routes, m)
		if err != nil {
			return nil, err
		}
		result["Routes"] = routes
	}

	if len(tr.Listeners) > 0 {
		listeners, err := marshalProtoMessages(tr.Listeners, m)
		if err != nil {
			return nil, err
		}
		result["Listeners"] = listeners
	}

	if len(tr.ExtraClusters) > 0 {
		clusters, err := marshalProtoMessages(tr.ExtraClusters, m)
		if err != nil {
			return nil, err
		}
		result["ExtraClusters"] = clusters
	}

	if len(tr.Clusters) > 0 {
		clusters, err := marshalProtoMessages(tr.Clusters, m)
		if err != nil {
			return nil, err
		}
		result["Clusters"] = clusters
	}

	// Marshal the result map to JSON
	return json.Marshal(result)
}

func (tr *translationResult) UnmarshalJSON(data []byte) error {
	m := protojson.UnmarshalOptions{}

	// Create a map to hold the unmarshaled fields
	result := make(map[string]json.RawMessage)

	// Unmarshal the JSON data into the map
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}

	// Unmarshal each field using protojson
	if routesData, ok := result["Routes"]; ok {
		var routes []json.RawMessage
		if err := json.Unmarshal(routesData, &routes); err != nil {
			return err
		}
		tr.Routes = make([]*envoy_config_route_v3.RouteConfiguration, len(routes))
		for i, routeData := range routes {
			route := &envoy_config_route_v3.RouteConfiguration{}
			if err := m.Unmarshal(routeData, route); err != nil {
				return err
			}
			tr.Routes[i] = route
		}
	}

	if listenersData, ok := result["Listeners"]; ok {
		var listeners []json.RawMessage
		if err := json.Unmarshal(listenersData, &listeners); err != nil {
			return err
		}
		tr.Listeners = make([]*envoy_config_listener_v3.Listener, len(listeners))
		for i, listenerData := range listeners {
			listener := &envoy_config_listener_v3.Listener{}
			if err := m.Unmarshal(listenerData, listener); err != nil {
				return err
			}
			tr.Listeners[i] = listener
		}
	}

	if clustersData, ok := result["ExtraClusters"]; ok {
		var clusters []json.RawMessage
		if err := json.Unmarshal(clustersData, &clusters); err != nil {
			return err
		}
		tr.ExtraClusters = make([]*clusterv3.Cluster, len(clusters))
		for i, clusterData := range clusters {
			cluster := &clusterv3.Cluster{}
			if err := m.Unmarshal(clusterData, cluster); err != nil {
				return err
			}
			tr.ExtraClusters[i] = cluster
		}
	}

	if clustersData, ok := result["Clusters"]; ok {
		var clusters []json.RawMessage
		if err := json.Unmarshal(clustersData, &clusters); err != nil {
			return err
		}
		tr.Clusters = make([]*clusterv3.Cluster, len(clusters))
		for i, clusterData := range clusters {
			cluster := &clusterv3.Cluster{}
			if err := m.Unmarshal(clusterData, cluster); err != nil {
				return err
			}
			tr.Clusters[i] = cluster
		}
	}

	return nil
}

func marshalProtoMessages[T proto.Message](messages []T, m protojson.MarshalOptions) ([]interface{}, error) {
	var result []interface{}
	for _, msg := range messages {
		data, err := m.Marshal(msg)
		if err != nil {
			return nil, err
		}
		var jsonObj interface{}
		if err := json.Unmarshal(data, &jsonObj); err != nil {
			return nil, err
		}
		result = append(result, jsonObj)
	}
	return result, nil
}

type ExtraPluginsFn func(ctx context.Context, commoncol *common.CommonCollections) []pluginsdk.Plugin

func NewScheme(extraSchemes runtime.SchemeBuilder) *runtime.Scheme {
	scheme := schemes.GatewayScheme()
	extraSchemes = append(extraSchemes, v1alpha1.Install)
	if err := extraSchemes.AddToScheme(scheme); err != nil {
		log.Fatalf("failed to add extra schemes to scheme: %v", err)
	}
	return scheme
}

func TestTranslation(
	t test.Failer,
	ctx context.Context,
	inputFiles []string,
	outputFile string,
	gwNN types.NamespacedName,
	assertReports AssertReports,
	settingsOpts ...SettingsOpts,
) {
	TestTranslationWithExtraPlugins(t, ctx, inputFiles, outputFile, gwNN, assertReports, nil, nil, nil, settingsOpts...)
}

func TestTranslationWithExtraPlugins(
	t test.Failer,
	ctx context.Context,
	inputFiles []string,
	outputFile string,
	gwNN types.NamespacedName,
	assertReports AssertReports,
	extraPluginsFn ExtraPluginsFn,
	extraSchemes runtime.SchemeBuilder,
	extraGroups []string,
	settingsOpts ...SettingsOpts,
) {
	scheme := NewScheme(extraSchemes)

	results, err := TestCase{
		InputFiles: inputFiles,
	}.Run(t, ctx, scheme, extraPluginsFn, extraGroups, settingsOpts...)
	Expect(err).NotTo(HaveOccurred())
	// TODO allow expecting multiple gateways in the output (map nns -> outputFile?)
	Expect(results).To(HaveLen(1))
	Expect(results).To(HaveKey(gwNN))
	result := results[gwNN]

	//// do a json round trip to normalize the output (i.e. things like omit empty)
	//b, _ := json.Marshal(result.Proxy)
	//var proxy ir.GatewayIR
	//Expect(json.Unmarshal(b, &proxy)).NotTo(HaveOccurred())

	// sort the output and print it
	result.Proxy = sortProxy(result.Proxy)
	result.Clusters = sortClusters(result.Clusters)
	output := &translationResult{
		Routes:        result.Proxy.Routes,
		Listeners:     result.Proxy.Listeners,
		ExtraClusters: result.Proxy.ExtraClusters,
		Clusters:      result.Clusters,
	}
	outputYaml, err := MarshalAnyYaml(output)
	fmt.Fprintf(ginkgo.GinkgoWriter, "actual result:\n %s \nerror: %v", outputYaml, err)
	Expect(err).NotTo(HaveOccurred())

	if envutils.IsEnvTruthy("REFRESH_GOLDEN") {
		// create parent directory if it doesn't exist
		dir := filepath.Dir(outputFile)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		os.WriteFile(outputFile, outputYaml, 0o644)
	}

	Expect(compareProxy(outputFile, result.Proxy)).To(BeEmpty())
	Expect(compareClusters(outputFile, result.Clusters)).To(BeEmpty())

	if assertReports != nil {
		assertReports(gwNN, result.ReportsMap)
	} else {
		Expect(AreReportsSuccess(gwNN, result.ReportsMap)).NotTo(HaveOccurred())
	}
}

type TestCase struct {
	InputFiles []string
}

type ActualTestResult struct {
	Proxy      *irtranslator.TranslationResult
	Clusters   []*clusterv3.Cluster
	ReportsMap reports.ReportMap
}

func compareProxy(expectedFile string, actualProxy *irtranslator.TranslationResult) (string, error) {
	expectedProxy, err := ReadProxyFromFile(expectedFile)
	if err != nil {
		return "", err
	}

	return cmp.Diff(sortProxy(expectedProxy), sortProxy(actualProxy), protocmp.Transform(), cmpopts.EquateNaNs()), nil
}

func sortProxy(proxy *irtranslator.TranslationResult) *irtranslator.TranslationResult {
	if proxy == nil {
		return nil
	}

	sort.Slice(proxy.Listeners, func(i, j int) bool {
		return proxy.Listeners[i].GetName() < proxy.Listeners[j].GetName()
	})
	sort.Slice(proxy.Routes, func(i, j int) bool {
		return proxy.Routes[i].GetName() < proxy.Routes[j].GetName()
	})
	sort.Slice(proxy.ExtraClusters, func(i, j int) bool {
		return proxy.ExtraClusters[i].GetName() < proxy.ExtraClusters[j].GetName()
	})

	return proxy
}

func compareClusters(expectedFile string, actualClusters []*clusterv3.Cluster) (string, error) {
	expectedOutput := &translationResult{}
	if err := ReadYamlFile(expectedFile, expectedOutput); err != nil {
		return "", err
	}

	// Sort both expected and actual clusters by name and compare
	return cmp.Diff(sortClusters(expectedOutput.Clusters), sortClusters(actualClusters), protocmp.Transform(), cmpopts.EquateNaNs()), nil
}

func sortClusters(clusters []*clusterv3.Cluster) []*clusterv3.Cluster {
	if len(clusters) == 0 {
		return clusters
	}

	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].GetName() < clusters[j].GetName()
	})

	return clusters
}

func ReadYamlFile(file string, out interface{}) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	return UnmarshalAnyYaml(data, out)
}

func GetHTTPRouteStatusError(
	reportsMap reports.ReportMap,
	route *types.NamespacedName,
) error {
	for nns, routeReport := range reportsMap.HTTPRoutes {
		if route != nil && nns != *route {
			continue
		}
		for ref, parentRefReport := range routeReport.Parents {
			for _, c := range parentRefReport.Conditions {
				// most route conditions true is good, except RouteConditionPartiallyInvalid
				if c.Type == string(gwv1.RouteConditionPartiallyInvalid) && c.Status != metav1.ConditionFalse {
					return fmt.Errorf("condition error for httproute: %v ref: %v condition: %v", nns, ref, c)
				} else if c.Status != metav1.ConditionTrue {
					return fmt.Errorf("condition error for httproute: %v ref: %v condition: %v", nns, ref, c)
				}
			}
		}
	}
	return nil
}

func AreReportsSuccess(gwNN types.NamespacedName, reportsMap reports.ReportMap) error {
	err := GetHTTPRouteStatusError(reportsMap, nil)
	if err != nil {
		return err
	}

	for nns, routeReport := range reportsMap.TCPRoutes {
		for ref, parentRefReport := range routeReport.Parents {
			for _, c := range parentRefReport.Conditions {
				// most route conditions true is good, except RouteConditionPartiallyInvalid
				if c.Type == string(gwv1.RouteConditionPartiallyInvalid) && c.Status != metav1.ConditionFalse {
					return fmt.Errorf("condition error for tcproute: %v ref: %v condition: %v", nns, ref, c)
				} else if c.Status != metav1.ConditionTrue {
					return fmt.Errorf("condition error for tcproute: %v ref: %v condition: %v", nns, ref, c)
				}
			}
		}
	}

	for nns, routeReport := range reportsMap.TLSRoutes {
		for ref, parentRefReport := range routeReport.Parents {
			for _, c := range parentRefReport.Conditions {
				// most route conditions true is good, except RouteConditionPartiallyInvalid
				if c.Type == string(gwv1.RouteConditionPartiallyInvalid) && c.Status != metav1.ConditionFalse {
					return fmt.Errorf("condition error for tlsroute: %v ref: %v condition: %v", nns, ref, c)
				} else if c.Status != metav1.ConditionTrue {
					return fmt.Errorf("condition error for tlsroute: %v ref: %v condition: %v", nns, ref, c)
				}
			}
		}
	}

	for nns, routeReport := range reportsMap.GRPCRoutes {
		for ref, parentRefReport := range routeReport.Parents {
			for _, c := range parentRefReport.Conditions {
				// most route conditions true is good, except RouteConditionPartiallyInvalid
				if c.Type == string(gwv1.RouteConditionPartiallyInvalid) && c.Status != metav1.ConditionFalse {
					return fmt.Errorf("condition error for grpcroute: %v ref: %v condition: %v", nns, ref, c)
				} else if c.Status != metav1.ConditionTrue {
					return fmt.Errorf("condition error for grpcroute: %v ref: %v condition: %v", nns, ref, c)
				}
			}
		}
	}

	for nns, gwReport := range reportsMap.Gateways {
		for _, c := range gwReport.GetConditions() {
			if c.Type == listener.AttachedListenerSetsConditionType {
				// A gateway might or might not have AttachedListenerSets so skip this condition
				continue
			}
			if c.Status != metav1.ConditionTrue {
				return fmt.Errorf("condition not accepted for gw %v condition: %v", nns, c)
			}
		}
	}

	return nil
}

type SettingsOpts func(*settings.Settings)

func (tc TestCase) Run(
	t test.Failer,
	ctx context.Context,
	scheme *runtime.Scheme,
	extraPluginsFn ExtraPluginsFn,
	extraGroups []string,
	settingsOpts ...SettingsOpts,
) (map[types.NamespacedName]ActualTestResult, error) {
	var (
		anyObjs []runtime.Object
		ourObjs []runtime.Object
	)
	for _, file := range tc.InputFiles {
		objs, err := LoadFromFiles(ctx, file, scheme)
		if err != nil {
			return nil, err
		}
		for i := range objs {
			switch obj := objs[i].(type) {
			case *gwv1.Gateway:
				anyObjs = append(anyObjs, obj)

			default:
				apiversion := reflect.ValueOf(obj).Elem().FieldByName("TypeMeta").FieldByName("APIVersion").String()
				if strings.Contains(apiversion, v1alpha1.GroupName) {
					ourObjs = append(ourObjs, obj)
				} else {
					external := false
					for _, group := range extraGroups {
						if strings.Contains(apiversion, group) {
							external = true
							break
						}
					}
					if !external {
						anyObjs = append(anyObjs, objs[i])
					}
				}
			}
		}
	}

	ourCli := fake.NewClientset(ourObjs...)
	cli := kubeclient.NewFakeClient(anyObjs...)
	for _, crd := range []schema.GroupVersionResource{
		gvr.KubernetesGateway_v1,
		gvr.GatewayClass,
		gvr.HTTPRoute_v1,
		gvr.GRPCRoute,
		gvr.Service,
		gvr.Pod,
		gvr.TCPRoute,
		gvr.TLSRoute,
		gvr.ServiceEntry,
		gvr.WorkloadEntry,
		gvr.AuthorizationPolicy,
		wellknown.XListenerSetGVR,
		wellknown.BackendTLSPolicyGVR,
	} {
		clienttest.MakeCRD(t, cli, crd)
	}
	defer cli.Shutdown()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// ensure classes used in tests exist and point at our controller
	gwClasses := []string{
		wellknown.DefaultGatewayClassName,
		wellknown.DefaultWaypointClassName,
		"example-gateway-class",
	}
	for _, className := range gwClasses {
		cli.GatewayAPI().GatewayV1().GatewayClasses().Create(ctx, &gwv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: string(className),
			},
			Spec: gwv1.GatewayClassSpec{
				ControllerName: wellknown.DefaultGatewayControllerName,
			},
		}, metav1.CreateOptions{})
	}

	krtOpts := krtutil.KrtOptions{
		Stop: ctx.Done(),
	}

	settings, err := settings.BuildSettings()
	if err != nil {
		return nil, err
	}
	for _, opt := range settingsOpts {
		opt(settings)
	}

	commoncol, err := common.NewCommonCollections(
		ctx,
		krtOpts,
		cli,
		ourCli,
		nil,
		wellknown.DefaultGatewayControllerName,
		logr.Discard(),
		*settings,
	)
	if err != nil {
		return nil, err
	}

	plugins := registry.Plugins(ctx, commoncol, wellknown.DefaultWaypointClassName)
	// TODO: consider moving the common code to a util that both proxy syncer and this test call
	plugins = append(plugins, krtcollections.NewBuiltinPlugin(ctx))

	var extraPlugs []pluginsdk.Plugin
	if extraPluginsFn != nil {
		extraPlugins := extraPluginsFn(ctx, commoncol)
		extraPlugs = append(extraPlugs, extraPlugins...)
	}
	plugins = append(plugins, extraPlugs...)
	extensions := registry.MergePlugins(plugins...)

	// needed for the Plugin Backend test (backend-plugin/gateway.yaml)
	gk := schema.GroupKind{
		Group: "",
		Kind:  "test-backend-plugin",
	}
	extensions.ContributesPolicies[gk] = extensionsplug.PolicyPlugin{
		Name: "test-backend-plugin",
	}
	testBackend := ir.NewBackendObjectIR(ir.ObjectSource{
		Kind:      "test-backend-plugin",
		Namespace: "default",
		Name:      "example-svc",
	}, 80, "")
	extensions.ContributesBackends[gk] = extensionsplug.BackendPlugin{
		Backends: krt.NewStaticCollection([]ir.BackendObjectIR{
			testBackend,
		}),
		BackendInit: ir.BackendInit{
			InitBackend: func(ctx context.Context, in ir.BackendObjectIR, out *clusterv3.Cluster) *ir.EndpointsForBackend {
				return nil
			},
		},
	}

	commoncol.InitPlugins(ctx, extensions, *settings)

	translator := translator.NewCombinedTranslator(ctx, extensions, commoncol)
	translator.Init(ctx)

	cli.RunAndWait(ctx.Done())
	commoncol.GatewayIndex.Gateways.WaitUntilSynced(ctx.Done())

	kubeclient.WaitForCacheSync("routes", ctx.Done(), commoncol.Routes.HasSynced)
	kubeclient.WaitForCacheSync("extensions", ctx.Done(), extensions.HasSynced)
	kubeclient.WaitForCacheSync("commoncol", ctx.Done(), commoncol.HasSynced)
	kubeclient.WaitForCacheSync("translator", ctx.Done(), translator.HasSynced)
	kubeclient.WaitForCacheSync("backends", ctx.Done(), commoncol.BackendIndex.HasSynced)
	kubeclient.WaitForCacheSync("endpoints", ctx.Done(), commoncol.Endpoints.HasSynced)
	for i, plug := range extraPlugs {
		kubeclient.WaitForCacheSync(fmt.Sprintf("extra-%d", i), ctx.Done(), plug.HasSynced)
	}

	time.Sleep(1 * time.Second)

	results := make(map[types.NamespacedName]ActualTestResult)

	for _, gw := range commoncol.GatewayIndex.Gateways.List() {
		gwNN := types.NamespacedName{
			Namespace: gw.Namespace,
			Name:      gw.Name,
		}

		xdsSnap, reportsMap := translator.TranslateGateway(krt.TestingDummyContext{}, ctx, gw)

		actual := ActualTestResult{
			Proxy:      xdsSnap,
			ReportsMap: reportsMap,
		}
		results[gwNN] = actual

		ucc := ir.NewUniqlyConnectedClient("test", "test", nil, ir.PodLocality{})
		var clusters []*clusterv3.Cluster
		for _, col := range commoncol.BackendIndex.BackendsWithPolicy() {
			for _, backend := range col.List() {
				cluster, err := translator.GetUpstreamTranslator().TranslateBackend(krt.TestingDummyContext{}, ucc, backend)
				Expect(err).NotTo(HaveOccurred())
				clusters = append(clusters, cluster)
			}
		}
		r := results[gwNN]
		r.Clusters = clusters
		results[gwNN] = r
	}

	return results, nil
}
