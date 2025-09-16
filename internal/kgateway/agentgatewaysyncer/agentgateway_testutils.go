package agentgatewaysyncer

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
	"time"

	"github.com/agentgateway/agentgateway/go/api"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/onsi/ginkgo/v2"
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

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/registry"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/listener"
	krtinternal "github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	agentgatewayplugins "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/pkg/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
	"github.com/kgateway-dev/kgateway/v2/test/translator"
)

type AssertReports func(gwNN types.NamespacedName, reportsMap reports.ReportMap)

type translationResult struct {
	Routes    []*api.Route
	TCPRoutes []*api.TCPRoute
	Listeners []*api.Listener
	Binds     []*api.Bind
	Backends  []*api.Backend
	Policies  []*api.Policy
	Addresses []*api.Address
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

	if len(tr.TCPRoutes) > 0 {
		tcproutes, err := marshalProtoMessages(tr.TCPRoutes, m)
		if err != nil {
			return nil, err
		}
		result["TCPRoutes"] = tcproutes
	}

	if len(tr.Listeners) > 0 {
		listeners, err := marshalProtoMessages(tr.Listeners, m)
		if err != nil {
			return nil, err
		}
		result["Listeners"] = listeners
	}

	if len(tr.Binds) > 0 {
		binds, err := marshalProtoMessages(tr.Binds, m)
		if err != nil {
			return nil, err
		}
		result["Binds"] = binds
	}

	if len(tr.Addresses) > 0 {
		addresses, err := marshalProtoMessages(tr.Addresses, m)
		if err != nil {
			return nil, err
		}
		result["Addresses"] = addresses
	}
	if len(tr.Backends) > 0 {
		backends, err := marshalProtoMessages(tr.Backends, m)
		if err != nil {
			return nil, err
		}
		result["Backends"] = backends
	}

	if len(tr.Policies) > 0 {
		policies, err := marshalProtoMessages(tr.Policies, m)
		if err != nil {
			return nil, err
		}
		result["Policies"] = policies
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
		tr.Routes = make([]*api.Route, len(routes))
		for i, routeData := range routes {
			route := &api.Route{}
			if err := m.Unmarshal(routeData, route); err != nil {
				return err
			}
			tr.Routes[i] = route
		}
	}

	if tcpRoutesData, ok := result["TCPRoutes"]; ok {
		var tcproutes []json.RawMessage
		if err := json.Unmarshal(tcpRoutesData, &tcproutes); err != nil {
			return err
		}
		tr.TCPRoutes = make([]*api.TCPRoute, len(tcproutes))
		for i, tcprouteData := range tcproutes {
			tcproute := &api.TCPRoute{}
			if err := m.Unmarshal(tcprouteData, tcproute); err != nil {
				return err
			}
			tr.TCPRoutes[i] = tcproute
		}
	}

	if listenersData, ok := result["Listeners"]; ok {
		var listeners []json.RawMessage
		if err := json.Unmarshal(listenersData, &listeners); err != nil {
			return err
		}
		tr.Listeners = make([]*api.Listener, len(listeners))
		for i, listenerData := range listeners {
			listener := &api.Listener{}
			if err := m.Unmarshal(listenerData, listener); err != nil {
				return err
			}
			tr.Listeners[i] = listener
		}
	}

	if bindsData, ok := result["Binds"]; ok {
		var binds []json.RawMessage
		if err := json.Unmarshal(bindsData, &binds); err != nil {
			return err
		}
		tr.Binds = make([]*api.Bind, len(binds))
		for i, bindData := range binds {
			bind := &api.Bind{}
			if err := m.Unmarshal(bindData, bind); err != nil {
				return err
			}
			tr.Binds[i] = bind
		}
	}

	if backendsData, ok := result["Backends"]; ok {
		var backends []json.RawMessage
		if err := json.Unmarshal(backendsData, &backends); err != nil {
			return err
		}
		tr.Backends = make([]*api.Backend, len(backends))
		for i, backendData := range backends {
			backend := &api.Backend{}
			if err := m.Unmarshal(backendData, backend); err != nil {
				return err
			}
			tr.Backends[i] = backend
		}
	}

	if policiesData, ok := result["Policies"]; ok {
		var policies []json.RawMessage
		if err := json.Unmarshal(policiesData, &policies); err != nil {
			return err
		}
		tr.Policies = make([]*api.Policy, len(policies))
		for i, policyData := range policies {
			policy := &api.Policy{}
			if err := m.Unmarshal(policyData, policy); err != nil {
				return err
			}
			tr.Policies[i] = policy
		}
	}

	if addressesData, ok := result["Addresses"]; ok {
		var addresses []json.RawMessage
		if err := json.Unmarshal(addressesData, &addresses); err != nil {
			return err
		}
		tr.Addresses = make([]*api.Address, len(addresses))
		for i, addressData := range addresses {
			address := &api.Address{}
			if err := m.Unmarshal(addressData, address); err != nil {
				return err
			}
			tr.Addresses[i] = address
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
	r.NoError(err)
	// TODO allow expecting multiple gateways in the output (map nns -> outputFile?)
	r.Len(results, 1)
	r.Contains(results, gwNN)
	result := results[gwNN]

	// TODO: do a json round trip to normalize the output (i.e. things like omit empty)

	// sort the output and print it
	var routes []*api.Route
	var tcproutes []*api.TCPRoute
	var listeners []*api.Listener
	var binds []*api.Bind
	var backends []*api.Backend
	var policies []*api.Policy
	var addresses []*api.Address

	// Extract agentgateway API types from ADPResources
	for _, adpRes := range result.Resources {
		for _, item := range adpRes.ResourceConfig.Items {
			resourceWrapper := item.Resource.(*envoyResourceWithCustomName)
			res := resourceWrapper.Message.(*api.Resource)
			switch r := res.Kind.(type) {
			case *api.Resource_Route:
				routes = append(routes, r.Route)
			case *api.Resource_TcpRoute:
				tcproutes = append(tcproutes, r.TcpRoute)
			case *api.Resource_Listener:
				listeners = append(listeners, r.Listener)
			case *api.Resource_Bind:
				binds = append(binds, r.Bind)
			case *api.Resource_Backend:
				backends = append(backends, r.Backend)
			case *api.Resource_Policy:
				policies = append(policies, r.Policy)
			}
		}
		for _, item := range adpRes.AddressConfig.Items {
			resourceWrapper := item.Resource.(*envoyResourceWithCustomName)
			res := resourceWrapper.Message.(*api.Address)
			addresses = append(addresses, res)
		}
	}

	output := &translationResult{
		Routes:    routes,
		TCPRoutes: tcproutes,
		Listeners: listeners,
		Binds:     binds,
		Backends:  backends,
		Policies:  policies,
		Addresses: addresses,
	}
	output = sortTranslationResult(output)
	outputYaml, err := translator.MarshalAnyYaml(output)
	fmt.Fprintf(ginkgo.GinkgoWriter, "actual result:\n %s \nerror: %v", outputYaml, err)
	r.NoError(err)

	if envutils.IsEnvTruthy("REFRESH_GOLDEN") {
		// create parent directory if it doesn't exist
		dir := filepath.Dir(outputFile)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			r.NoError(err)
		}
		os.WriteFile(outputFile, outputYaml, 0o644)
	}

	diff, err := compareProxy(outputFile, output)
	r.Empty(diff)
	r.NoError(err)

	if assertReports != nil {
		assertReports(gwNN, result.ReportsMap)
	} else {
		r.NoError(AreReportsSuccess(result.ReportsMap))
	}
}

type TestCase struct {
	InputFiles []string
}

type ActualTestResult struct {
	Resources  []agentGwXdsResources
	ReportsMap reports.ReportMap
}

func compareProxy(expectedFile string, actualProxy *translationResult) (string, error) {
	expectedOutput := &translationResult{}
	if err := ReadYamlFile(expectedFile, expectedOutput); err != nil {
		return "", err
	}

	return cmp.Diff(sortTranslationResult(expectedOutput), sortTranslationResult(actualProxy), protocmp.Transform(), cmpopts.EquateNaNs()), nil
}

func sortTranslationResult(tr *translationResult) *translationResult {
	if tr == nil {
		return nil
	}

	// Sort routes by name
	sort.Slice(tr.Routes, func(i, j int) bool {
		return tr.Routes[i].GetKey() < tr.Routes[j].GetKey()
	})

	sort.Slice(tr.TCPRoutes, func(i, j int) bool {
		return tr.TCPRoutes[i].GetKey() < tr.TCPRoutes[j].GetKey()
	})

	// Sort listeners by name
	sort.Slice(tr.Listeners, func(i, j int) bool {
		return tr.Listeners[i].GetKey() < tr.Listeners[j].GetKey()
	})

	// Sort binds by name
	sort.Slice(tr.Binds, func(i, j int) bool {
		return tr.Binds[i].GetKey() < tr.Binds[j].GetKey()
	})

	// Sort backends by name
	sort.Slice(tr.Backends, func(i, j int) bool {
		return tr.Backends[i].GetName() < tr.Backends[j].GetName()
	})

	// Sort policies by name
	sort.Slice(tr.Policies, func(i, j int) bool {
		return tr.Policies[i].GetName() < tr.Policies[j].GetName()
	})

	// Sort addresses
	sort.Slice(tr.Addresses, func(i, j int) bool {
		return tr.Addresses[i].String() < tr.Addresses[j].String()
	})

	return tr
}

func ReadYamlFile(file string, out interface{}) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	return translator.UnmarshalAnyYaml(data, out)
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

func AreReportsSuccess(reportsMap reports.ReportMap) error {
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
			if c.Type == listener.GatewayConditionAttachedListenerSets {
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
	gvkToStructuralSchema, err := translator.GetStructuralSchemas(
		filepath.Join(testutils.GitRootDirectory(), translator.CRDPath))
	if err != nil {
		return nil, fmt.Errorf("error getting structural schemas: %w", err)
	}

	for _, file := range tc.InputFiles {
		objs, err := translator.LoadFromFiles(file, scheme, gvkToStructuralSchema)
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
		wellknown.DefaultAgentGatewayClassName,
	}
	for _, className := range gwClasses {
		cli.GatewayAPI().GatewayV1().GatewayClasses().Create(ctx, &gwv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: className,
			},
			Spec: gwv1.GatewayClassSpec{
				ControllerName: wellknown.DefaultGatewayControllerName,
			},
		}, metav1.CreateOptions{})
	}

	krtOpts := krtinternal.KrtOptions{
		Stop: ctx.Done(),
	}

	settings, err := settings.BuildSettings()
	// enable agent gateway translation
	settings.EnableAgentGateway = true
	settings.EnableInferExt = true
	if err != nil {
		return nil, err
	}
	for _, opt := range settingsOpts {
		// overwrite any additional settings
		opt(settings)
	}

	commoncol, err := collections.NewCommonCollections(
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
	proxySyncerPlugins := proxySyncerPluginFactory(ctx, commoncol, wellknown.DefaultAgentGatewayClassName, extraPluginsFn, *settings)
	commoncol.InitPlugins(ctx, proxySyncerPlugins, *settings)

	cli.RunAndWait(ctx.Done())
	results := make(map[types.NamespacedName]ActualTestResult)

	// Create AgwCollections with the necessary input collections
	agwCollections, err := agentgatewayplugins.NewAgwCollections(
		commoncol,
		"istio-system",
		"Kubernetes",
	)
	if err != nil {
		return nil, err
	}
	agwMergedPlugins := agentGatewayPluginFactory(ctx, agwCollections)
	kubeclient.WaitForCacheSync("tlsroutes", ctx.Done(), agwCollections.TLSRoutes.HasSynced)
	kubeclient.WaitForCacheSync("tcproutes", ctx.Done(), agwCollections.TCPRoutes.HasSynced)
	kubeclient.WaitForCacheSync("httproutes", ctx.Done(), agwCollections.HTTPRoutes.HasSynced)
	kubeclient.WaitForCacheSync("grpcroutes", ctx.Done(), agwCollections.GRPCRoutes.HasSynced)
	kubeclient.WaitForCacheSync("backends", ctx.Done(), agwCollections.Backends.HasSynced)
	kubeclient.WaitForCacheSync("trafficpolicies", ctx.Done(), agwCollections.TrafficPolicies.HasSynced)
	kubeclient.WaitForCacheSync("infpool", ctx.Done(), agwCollections.InferencePools.HasSynced)
	kubeclient.WaitForCacheSync("secrets", ctx.Done(), agwCollections.Secrets.HasSynced)

	// Instead of calling full Init(), manually initialize just what we need for testing
	// to avoid race conditions with XDS collection building
	agentGwSyncer := NewAgentGwSyncer(
		wellknown.DefaultGatewayControllerName,
		wellknown.DefaultAgentGatewayClassName,
		cli,
		nil, // mgr not needed for test
		agwCollections,
		agwMergedPlugins,
		nil,  // xdsCache not needed for test
		true, // enableInferExt
	)
	agentGwSyncer.translator.Init()

	gatewayClasses := GatewayClassesCollection(agwCollections.GatewayClasses, krtOpts)
	refGrants := BuildReferenceGrants(ReferenceGrantsCollection(agwCollections.ReferenceGrants, krtOpts))
	gateways := agentGwSyncer.buildGatewayCollection(gatewayClasses, refGrants, krtOpts)

	// Build ADP resources and addresses collections
	adpResourcesCollection, _ := agentGwSyncer.buildADPResources(gateways, refGrants, krtOpts)
	_, adpBackendsCollection := agentGwSyncer.newADPBackendCollection(agwCollections.Backends, krtOpts)
	addressesCollection := agentGwSyncer.buildAddressCollections(krtOpts)

	// Wait for collections to sync
	kubeclient.WaitForCacheSync("adp-resources", ctx.Done(), adpResourcesCollection.HasSynced)
	kubeclient.WaitForCacheSync("addresses", ctx.Done(), addressesCollection.HasSynced)

	// build final proxy xds result
	agentGwSyncer.buildXDSCollection(adpResourcesCollection, adpBackendsCollection, addressesCollection, krtOpts)
	kubeclient.WaitForCacheSync("xds", ctx.Done(), agentGwSyncer.xDS.HasSynced)

	time.Sleep(500 * time.Millisecond) // Allow collections to populate

	for _, gw := range commoncol.GatewayIndex.Gateways.List() {
		gwNN := types.NamespacedName{
			Namespace: gw.Namespace,
			Name:      gw.Name,
		}

		// Collect results for this gateway
		var xdsResult []agentGwXdsResources

		// Create a test context for fetching from collections
		testCtx := krt.TestingDummyContext{}

		// Fetch xds resources for this gateway
		allResources := krt.Fetch(testCtx, agentGwSyncer.xDS)
		for _, resource := range allResources {
			if resource.NamespacedName == gwNN {
				xdsResult = append(xdsResult, resource)
			}
		}

		// Get the reports from the collected resources
		reportsMap := reports.NewReportMap()
		for _, resource := range allResources {
			// Merge reports from all resources for this gateway
			for gwKey, gwReport := range resource.reports.Gateways {
				reportsMap.Gateways[gwKey] = gwReport
			}
			for lsKey, lsReport := range resource.reports.ListenerSets {
				reportsMap.ListenerSets[lsKey] = lsReport
			}
			for routeKey, routeReport := range resource.reports.HTTPRoutes {
				reportsMap.HTTPRoutes[routeKey] = routeReport
			}
			for routeKey, routeReport := range resource.reports.GRPCRoutes {
				reportsMap.GRPCRoutes[routeKey] = routeReport
			}
			for routeKey, routeReport := range resource.reports.TCPRoutes {
				reportsMap.TCPRoutes[routeKey] = routeReport
			}
			for routeKey, routeReport := range resource.reports.TLSRoutes {
				reportsMap.TLSRoutes[routeKey] = routeReport
			}
		}

		actual := ActualTestResult{
			Resources:  allResources,
			ReportsMap: reportsMap,
		}
		results[gwNN] = actual
	}

	return results, nil
}

func proxySyncerPluginFactory(ctx context.Context, commoncol *collections.CommonCollections, name string, extraPluginsFn ExtraPluginsFn, globalSettings settings.Settings) pluginsdk.Plugin {
	plugins := registry.Plugins(ctx, commoncol, wellknown.DefaultAgentGatewayClassName, globalSettings)

	var extraPlugs []pluginsdk.Plugin
	if extraPluginsFn != nil {
		extraPlugins := extraPluginsFn(ctx, commoncol)
		extraPlugs = append(extraPlugs, extraPlugins...)
	}
	plugins = append(plugins, extraPlugs...)
	mergedPlugins := registry.MergePlugins(plugins...)
	for i, plug := range extraPlugs {
		kubeclient.WaitForCacheSync(fmt.Sprintf("extra-%d", i), ctx.Done(), plug.HasSynced)
	}
	return mergedPlugins
}

// agentGatewayPluginFactory is a factory function that returns the agent gateway plugins
// It is based on agentGatewayPluginFactory(cfg)(ctx, cfg.AgwCollections) in start.go
func agentGatewayPluginFactory(ctx context.Context, agwCollections *agentgatewayplugins.AgwCollections) agentgatewayplugins.AgentgatewayPlugin {
	agwPlugins := agentgatewayplugins.Plugins(agwCollections)
	mergedPlugins := agentgatewayplugins.MergePlugins(agwPlugins...)
	for i, plug := range agwPlugins {
		kubeclient.WaitForCacheSync(fmt.Sprintf("plugin-%d", i), ctx.Done(), plug.HasSynced)
	}
	return mergedPlugins
}
