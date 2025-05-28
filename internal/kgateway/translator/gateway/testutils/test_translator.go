package testutils

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned/fake"
	common "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
)

type AssertReports func(gwNN types.NamespacedName, reportsMap reports.ReportMap)

type translationResult struct {
	Routes        []*envoy_config_route_v3.RouteConfiguration
	Listeners     []*envoy_config_listener_v3.Listener
	ExtraClusters []*clusterv3.Cluster
	Clusters      []*clusterv3.Cluster `json:"Clusters,omitempty"`
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

func TestTranslation(
	t test.Failer,
	ctx context.Context,
	inputFiles []string,
	outputFile string,
	gwNN types.NamespacedName,
	assertReports AssertReports,
) {
	results, err := TestCase{
		InputFiles: inputFiles,
	}.Run(t, ctx)
	Expect(err).NotTo(HaveOccurred())
	// TODO allow expecting multiple gateways in the output (map nns -> outputFile?)
	Expect(results).To(HaveLen(1))
	Expect(results).To(HaveKey(gwNN))
	result := results[gwNN]

	//// do a json round trip to normalize the output (i.e. things like omit empty)
	//b, _ := json.Marshal(result.Proxy)
	//var proxy ir.GatewayIR
	//Expect(json.Unmarshal(b, &proxy)).NotTo(HaveOccurred())
	if os.Getenv("UPDATE_OUTPUTS") == "1" {
		output := &translationResult{
			Routes:        result.Proxy.Routes,
			Listeners:     result.Proxy.Listeners,
			ExtraClusters: result.Proxy.ExtraClusters,
			Clusters:      result.Clusters,
		}

		d, err := MarshalAnyYaml(output)
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		// create parent directory if it doesn't exist
		dir := filepath.Dir(outputFile)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		os.WriteFile(outputFile, d, 0o644)
	}

	Expect(CompareProxy(outputFile, result.Proxy)).To(BeEmpty())
	Expect(CompareClusters(outputFile, result.Clusters)).To(BeEmpty())

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

func CompareProxy(expectedFile string, actualProxy *irtranslator.TranslationResult) (string, error) {

	expectedProxy, err := ReadProxyFromFile(expectedFile)
	if err != nil {
		return "", err
	}
	return cmp.Diff(expectedProxy, actualProxy, protocmp.Transform(), cmpopts.EquateNaNs()), nil
}

func CompareClusters(expectedFile string, actualClusters []*clusterv3.Cluster) (string, error) {
	expectedOutput := &translationResult{}
	if err := ReadYamlFile(expectedFile, expectedOutput); err != nil {
		return "", err
	}

	// Sort both expected and actual clusters by name
	sort.Slice(expectedOutput.Clusters, func(i, j int) bool {
		return expectedOutput.Clusters[i].Name < expectedOutput.Clusters[j].Name
	})
	sort.Slice(actualClusters, func(i, j int) bool {
		return actualClusters[i].Name < actualClusters[j].Name
	})

	return cmp.Diff(expectedOutput.Clusters, actualClusters, protocmp.Transform(), cmpopts.EquateNaNs()), nil
}

func ReadYamlFile(file string, out interface{}) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	return UnmarshalAnyYaml(data, out)
}

func AreReportsSuccess(gwNN types.NamespacedName, reportsMap reports.ReportMap) error {
	for nns, routeReport := range reportsMap.HTTPRoutes {
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
			if c.Status != metav1.ConditionTrue {
				return fmt.Errorf("condition not accepted for gw %v condition: %v", nns, c)
			}
		}
	}

	return nil
}

var _ extensionsplug.GetBackendForRefPlugin = testBackendPlugin{}.GetBackendForRefPlugin

type testBackendPlugin struct{}

// GetBackendForRef implements query.BackendRefResolver.
func (tp testBackendPlugin) GetBackendForRefPlugin(kctx krt.HandlerContext, key ir.ObjectSource, port int32) *ir.BackendObjectIR {
	if key.Kind != "test-backend-plugin" {
		return nil
	}
	// doesn't matter as long as its not nil
	return &ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "test",
			Kind:      "test-backend-plugin",
			Namespace: "test-backend-plugin-ns",
			Name:      "test-backend-plugin-us",
		},
	}
}

func (tc TestCase) Run(t test.Failer, ctx context.Context) (map[types.NamespacedName]ActualTestResult, error) {
	var (
		anyObjs []runtime.Object
		ourObjs []runtime.Object
	)
	for _, file := range tc.InputFiles {
		objs, err := LoadFromFiles(ctx, file)
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
					anyObjs = append(anyObjs, objs[i])
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
	} {
		clienttest.MakeCRD(t, cli, crd)
	}
	defer cli.Shutdown()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// ensure classes used in tests exist and point at our controller
	gwClasses := append(wellknown.BuiltinGatewayClasses.UnsortedList(), "example-gateway-class")
	for _, className := range gwClasses {
		cli.GatewayAPI().GatewayV1().GatewayClasses().Create(ctx, &gwv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: string(className),
			},
			Spec: gwv1.GatewayClassSpec{
				ControllerName: wellknown.GatewayControllerName,
			},
		}, metav1.CreateOptions{})
	}

	krtOpts := krtutil.KrtOptions{
		Stop: ctx.Done(),
	}

	st, err := settings.BuildSettings()
	if err != nil {
		return nil, err
	}
	commoncol := common.NewCommonCollections(
		ctx,
		krtOpts,
		cli,
		ourCli,
		nil,
		wellknown.GatewayControllerName,
		logr.Discard(),
		*st,
	)

	plugins := registry.Plugins(ctx, commoncol)
	// TODO: consider moving the common code to a util that both proxy syncer and this test call
	plugins = append(plugins, krtcollections.NewBuiltinPlugin(ctx))
	extensions := registry.MergePlugins(plugins...)

	gk := schema.GroupKind{
		Group: "",
		Kind:  "test-backend-plugin",
	}
	extensions.ContributesPolicies[gk] = extensionsplug.PolicyPlugin{
		Name:             "test-backend-plugin",
		GetBackendForRef: testBackendPlugin{}.GetBackendForRefPlugin,
	}

	commoncol.InitPlugins(ctx, extensions)

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

	time.Sleep(1 * time.Second)

	results := make(map[types.NamespacedName]ActualTestResult)

	for _, gw := range commoncol.GatewayIndex.Gateways.List() {
		gwNN := types.NamespacedName{
			Namespace: gw.Namespace,
			Name:      gw.Name,
		}

		xdsSnap, reportsMap := translator.TranslateGateway(krt.TestingDummyContext{}, ctx, gw)

		act, _ := MarshalAnyYaml(xdsSnap)
		fmt.Fprintf(ginkgo.GinkgoWriter, "actual result:\n %s \n", act)

		actual := ActualTestResult{
			Proxy:      xdsSnap,
			ReportsMap: reportsMap,
		}
		results[gwNN] = actual

		ucc := ir.NewUniqlyConnectedClient("test", "test", nil, ir.PodLocality{})
		var clusters []*clusterv3.Cluster
		for _, col := range commoncol.BackendIndex.BackendsWithPolicy() {
			for _, backend := range col.List() {
				if backend.ObjectSource.Kind != "Backend" {
					continue
				}
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
