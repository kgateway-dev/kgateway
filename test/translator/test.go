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
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"istio.io/istio/pkg/config/schema/gvr"
	kubeclient "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient/clienttest"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwxv1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/registry"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/listener"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	common "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/pkg/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
)

type AssertReports func(gwNN types.NamespacedName, reportsMap reports.ReportMap)

type translationResult struct {
	Routes        []*envoyroutev3.RouteConfiguration
	Listeners     []*envoylistenerv3.Listener
	ExtraClusters []*envoyclusterv3.Cluster
	Clusters      []*envoyclusterv3.Cluster
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
		tr.Routes = make([]*envoyroutev3.RouteConfiguration, len(routes))
		for i, routeData := range routes {
			route := &envoyroutev3.RouteConfiguration{}
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
		tr.Listeners = make([]*envoylistenerv3.Listener, len(listeners))
		for i, listenerData := range listeners {
			listener := &envoylistenerv3.Listener{}
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
		tr.ExtraClusters = make([]*envoyclusterv3.Cluster, len(clusters))
		for i, clusterData := range clusters {
			cluster := &envoyclusterv3.Cluster{}
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
		tr.Clusters = make([]*envoyclusterv3.Cluster, len(clusters))
		for i, clusterData := range clusters {
			cluster := &envoyclusterv3.Cluster{}
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
	t *testing.T,
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
	t *testing.T,
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
	r := require.New(t)

	results, err := TestCase{
		InputFiles: inputFiles,
	}.Run(t, ctx, scheme, extraPluginsFn, extraGroups, settingsOpts...)
	r.NoError(err, "error running test case")
	r.Len(results, 1, "expected exactly one gateway in the results")
	r.Contains(results, gwNN)
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
	r.NoErrorf(err, "error marshaling output to YAML; actual result: %s", outputYaml)

	if envutils.IsEnvTruthy("REFRESH_GOLDEN") {
		// create parent directory if it doesn't exist
		dir := filepath.Dir(outputFile)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			r.NoErrorf(err, "error creating directory %s", dir)
		}
		t.Log("REFRESH_GOLDEN is set, writing output file", outputFile)
		os.WriteFile(outputFile, outputYaml, 0o644)
	}

	gotProxy, err := compareProxy(outputFile, result.Proxy)
	r.Emptyf(gotProxy, "unexpected diff in proxy output; actual result: %s", outputYaml)
	r.NoError(err, "error comparing proxy output")

	gotClusters, err := compareClusters(outputFile, result.Clusters)
	r.Emptyf(gotClusters, "unexpected diff in clusters output; actual result: %s", outputYaml)
	r.NoError(err, "error comparing clusters output")

	if assertReports != nil {
		assertReports(gwNN, result.ReportsMap)
	} else {
		r.NoError(AreReportsSuccess(gwNN, result.ReportsMap), "expected status reports to not have errors")
	}
}

type TestCase struct {
	InputFiles []string
}

type ActualTestResult struct {
	Proxy      *irtranslator.TranslationResult
	Clusters   []*envoyclusterv3.Cluster
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

func compareClusters(expectedFile string, actualClusters []*envoyclusterv3.Cluster) (string, error) {
	expectedOutput := &translationResult{}
	if err := ReadYamlFile(expectedFile, expectedOutput); err != nil {
		return "", err
	}

	// Sort both expected and actual clusters by name and compare
	return cmp.Diff(sortClusters(expectedOutput.Clusters), sortClusters(actualClusters), protocmp.Transform(), cmpopts.EquateNaNs()), nil
}

func sortClusters(clusters []*envoyclusterv3.Cluster) []*envoyclusterv3.Cluster {
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
	for nns := range reportsMap.HTTPRoutes {
		if route != nil && nns != *route {
			continue
		}
		r := gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nns.Name,
				Namespace: nns.Namespace,
			},
		}
		status := reportsMap.BuildRouteStatus(context.Background(), &r, wellknown.DefaultGatewayClassName)

		for ref, parentRefReport := range status.Parents {
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

func GetPolicyStatusError(
	reportsMap reports.ReportMap,
	policy *reporter.PolicyKey,
) error {
	for key := range reportsMap.Policies {
		if policy != nil && *policy != key {
			continue
		}
		status := reportsMap.BuildPolicyStatus(context.Background(), key, wellknown.DefaultGatewayControllerName, gwv1a2.PolicyStatus{})
		for ancestor, report := range status.Ancestors {
			for _, c := range report.Conditions {
				if c.Status != metav1.ConditionTrue {
					return fmt.Errorf("condition error for policy: %v, ancestor ref: %v, condition: %v", key, ancestor, c)
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

	for nns := range reportsMap.TCPRoutes {
		r := gwv1a2.TCPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nns.Name,
				Namespace: nns.Namespace,
			},
		}
		status := reportsMap.BuildRouteStatus(context.Background(), &r, wellknown.DefaultGatewayClassName)

		for ref, parentRefReport := range status.Parents {
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

	for nns := range reportsMap.TLSRoutes {
		r := gwv1a2.TLSRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nns.Name,
				Namespace: nns.Namespace,
			},
		}
		status := reportsMap.BuildRouteStatus(context.Background(), &r, wellknown.DefaultGatewayClassName)

		for ref, parentRefReport := range status.Parents {
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

	for nns := range reportsMap.GRPCRoutes {
		r := gwv1.GRPCRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nns.Name,
				Namespace: nns.Namespace,
			},
		}
		status := reportsMap.BuildRouteStatus(context.Background(), &r, wellknown.DefaultGatewayClassName)

		for ref, parentRefReport := range status.Parents {
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

	for nns := range reportsMap.Gateways {
		g := gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nns.Name,
				Namespace: nns.Namespace,
			},
		}
		status := reportsMap.BuildGWStatus(context.Background(), g)
		for _, c := range status.Conditions {
			if c.Type == listener.AttachedListenerSetsConditionType {
				// A gateway might or might not have AttachedListenerSets so skip this condition
				continue
			}
			if c.Status != metav1.ConditionTrue {
				return fmt.Errorf("condition not accepted for gw %v condition: %v", nns, c)
			}
		}
	}

	for ls := range reportsMap.ListenerSets {
		l := gwxv1.XListenerSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ls.Name,
				Namespace: ls.Namespace,
			},
		}
		status := reportsMap.BuildListenerSetStatus(context.Background(), l)
		for _, c := range status.Conditions {
			if c.Status != metav1.ConditionTrue {
				return fmt.Errorf("condition not accepted for listenerSet %s condition: %v", ls, c)
			}
		}
	}

	err = GetPolicyStatusError(reportsMap, nil)
	if err != nil {
		return err
	}

	return nil
}

type SettingsOpts func(*settings.Settings)

func (tc TestCase) Run(
	t *testing.T,
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
	r := require.New(t)

	for _, file := range tc.InputFiles {
		objs, err := LoadFromFiles(file, scheme)
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
		Backends: krt.NewStaticCollection(nil, []ir.BackendObjectIR{
			testBackend,
		}),
		BackendInit: ir.BackendInit{
			InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
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
		var clusters []*envoyclusterv3.Cluster
		for _, col := range commoncol.BackendIndex.BackendsWithPolicy() {
			for _, backend := range col.List() {
				cluster, err := translator.GetUpstreamTranslator().TranslateBackend(krt.TestingDummyContext{}, ucc, backend)
				r.NoErrorf(err, "error translating backend %s", backend.GetName())
				clusters = append(clusters, cluster)
			}
		}
		r := results[gwNN]
		r.Clusters = clusters
		results[gwNN] = r
	}

	return results, nil
}
