package agentgatewaysyncer

import (
	gocmp "cmp"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/agentgateway/agentgateway/go/api"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"istio.io/istio/pilot/pkg/config/kube/crd"
	"istio.io/istio/pilot/test/util"
	"istio.io/istio/pkg/config/schema/gvk"
	kubeclient "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient/clienttest"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/pkg/consts"
	"sigs.k8s.io/yaml"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/agentgatewaysyncer/status"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/registry"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	agwplugins "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	agwtranslator "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
	translatortest "github.com/kgateway-dev/kgateway/v2/test/translator"
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

type ExtraPluginsFn func(ctx context.Context, commoncol *collections.CommonCollections) []pluginsdk.Plugin

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
	expectedStatusFile string,
	gwNN types.NamespacedName,
	settingsOpts ...SettingsOpts,
) {
	TestTranslationWithExtraPlugins(t, ctx, inputFiles, outputFile, expectedStatusFile, gwNN, nil, nil, nil, settingsOpts...)
}

func TestTranslationWithExtraPlugins(
	t *testing.T,
	ctx context.Context,
	inputFiles []string,
	outputFile string,
	expectedStatusFile string,
	gwNN types.NamespacedName,
	extraPluginsFn ExtraPluginsFn,
	extraSchemes runtime.SchemeBuilder,
	extraGVRs []schema.GroupVersionResource,
	settingsOpts ...SettingsOpts,
) {
	scheme := NewScheme(extraSchemes)
	r := require.New(t)

	results, err := TestCase{
		InputFiles: inputFiles,
	}.Run(t, ctx, scheme, extraPluginsFn, extraGVRs, settingsOpts...)
	r.NoError(err)

	// TODO: do a json round trip to normalize the output (i.e. things like omit empty)

	// sort the output and print it
	var routes []*api.Route
	var tcproutes []*api.TCPRoute
	var listeners []*api.Listener
	var binds []*api.Bind
	var backends []*api.Backend
	var policies []*api.Policy
	var addresses []*api.Address

	// Extract agentgateway API types from AgwResources
	for _, res := range results.Resources {
		if !(res.Gateway == gwNN || res.Gateway == (types.NamespacedName{})) {
			continue
		}
		switch r := res.Resource.Kind.(type) {
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
	for _, item := range results.Addresses {
		addresses = append(addresses, item.IntoProto())
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
	outputYaml, err := testutils.MarshalAnyYaml(output)
	fmt.Fprintf(ginkgo.GinkgoWriter, "actual result:\n %s \nerror: %v", outputYaml, err)
	r.NoError(err)

	if envutils.IsEnvTruthy("REFRESH_GOLDEN") {
		// create parent directory if it doesn't exist
		dir := filepath.Dir(outputFile)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			r.NoError(err)
		}
		os.WriteFile(outputFile, outputYaml, 0o600)
	}

	diff, err := compareProxy(outputFile, output)
	r.Empty(diff)
	r.NoError(err)

	outputStatus := results.Status.Dump()

	util.CompareContent(t, []byte(outputStatus), expectedStatusFile)
}

type TestCase struct {
	InputFiles []string
}

type ActualTestResult struct {
	Resources []ir.AgwResource
	Addresses []Address
	Status    *TestStatusQueue
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
	return testutils.UnmarshalAnyYaml(data, out)
}

type SettingsOpts func(*apisettings.Settings)

func (tc TestCase) Run(
	t *testing.T,
	ctx context.Context,
	scheme *runtime.Scheme,
	extraPluginsFn ExtraPluginsFn,
	extraGVRs []schema.GroupVersionResource,
	settingsOpts ...SettingsOpts,
) (ActualTestResult, error) {
	var (
		anyObjs []runtime.Object
		ourObjs []runtime.Object
	)
	gvkToStructuralSchema, err := testutils.GetStructuralSchemas(
		filepath.Join(testutils.GitRootDirectory(), testutils.CRDPath))
	if err != nil {
		return ActualTestResult{}, fmt.Errorf("error getting structural schemas: %w", err)
	}

	for _, file := range tc.InputFiles {
		objs, err := testutils.LoadFromFiles(file, scheme, gvkToStructuralSchema)
		if err != nil {
			return ActualTestResult{}, err
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
					for _, gvr := range extraGVRs {
						if strings.Contains(apiversion, gvr.Group) {
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

	ourCli := fake.NewSimpleClientset(ourObjs...)
	cli := kubeclient.NewFakeClient(anyObjs...)
	allGVRs := append(translatortest.AllCRDs, extraGVRs...)
	for _, gvr := range allGVRs {
		clienttest.MakeCRDWithAnnotations(t, cli, gvr, map[string]string{
			consts.BundleVersionAnnotation: consts.BundleVersion,
		})
	}
	defer cli.Shutdown()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// ensure classes used in tests exist and point at our controller
	gwClasses := []string{
		wellknown.DefaultAgwClassName,
	}
	for _, className := range gwClasses {
		cli.GatewayAPI().GatewayV1().GatewayClasses().Create(ctx, &gwv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: className,
			},
			Spec: gwv1.GatewayClassSpec{
				ControllerName: wellknown.DefaultAgwControllerName,
			},
		}, metav1.CreateOptions{})
	}

	krtDebugger := new(krt.DebugHandler)
	dumpOnFailure(t, krtDebugger)
	krtOpts := krtutil.KrtOptions{
		Stop:     ctx.Done(),
		Debugger: krtDebugger,
	}

	settings, err := apisettings.BuildSettings()
	if err != nil {
		return ActualTestResult{}, err
	}
	// enable agent gateway translation
	settings.EnableAgentgateway = true
	settings.EnableInferExt = true
	for _, opt := range settingsOpts {
		// overwrite any additional settings
		opt(settings)
	}

	commoncol, err := collections.NewCommonCollections(
		ctx,
		krtOpts,
		cli,
		ourCli,
		wellknown.DefaultGatewayControllerName,
		wellknown.DefaultAgwControllerName,
		*settings,
	)
	if err != nil {
		return ActualTestResult{}, err
	}
	proxySyncerPlugins := proxySyncerPluginFactory(ctx, commoncol, extraPluginsFn, *settings)
	commoncol.InitPlugins(ctx, proxySyncerPlugins, *settings)

	// Create AgwCollections with the necessary input collections
	agwCollections, err := agwplugins.NewAgwCollections(
		commoncol,
		wellknown.DefaultAgwControllerName,
		"istio-system",
		"Kubernetes",
	)
	if err != nil {
		return ActualTestResult{}, err
	}

	cli.RunAndWait(ctx.Done())

	agwMergedPlugins := agwPluginFactory(ctx, agwCollections)
	kubeclient.WaitForCacheSync("tlsroutes", ctx.Done(), agwCollections.TLSRoutes.HasSynced)
	kubeclient.WaitForCacheSync("tcproutes", ctx.Done(), agwCollections.TCPRoutes.HasSynced)
	kubeclient.WaitForCacheSync("httproutes", ctx.Done(), agwCollections.HTTPRoutes.HasSynced)
	kubeclient.WaitForCacheSync("grpcroutes", ctx.Done(), agwCollections.GRPCRoutes.HasSynced)
	kubeclient.WaitForCacheSync("backends", ctx.Done(), agwCollections.Backends.HasSynced)
	kubeclient.WaitForCacheSync("agentgatewaypolicies", ctx.Done(), agwCollections.AgentgatewayPolicies.HasSynced)
	kubeclient.WaitForCacheSync("infpool", ctx.Done(), agwCollections.InferencePools.HasSynced)
	kubeclient.WaitForCacheSync("secrets", ctx.Done(), agwCollections.Secrets.HasSynced)

	// Instead of calling full Init(), manually initialize just what we need for testing
	// to avoid race conditions with XDS collection building
	agentGwSyncer := NewAgwSyncer(
		wellknown.DefaultAgwControllerName,
		cli,
		agwCollections,
		agwMergedPlugins,
		nil,
	)
	agentGwSyncer.translator.Init()
	gatewayClasses := agwtranslator.GatewayClassesCollection(agwCollections.GatewayClasses, krtOpts)
	refGrants := agwtranslator.BuildReferenceGrants(agwtranslator.ReferenceGrantsCollection(agwCollections.ReferenceGrants, krtOpts))
	_, listenerSets := agentGwSyncer.buildListenerSetCollection(gatewayClasses, refGrants, krtOpts)
	_, gateways := agentGwSyncer.buildGatewayCollection(gatewayClasses, listenerSets, refGrants, krtOpts)

	// Build ADP resources and addresses collections
	agwResourcesCollection, _, _ := agentGwSyncer.buildAgwResources(gateways, refGrants, krtOpts)
	addressesCollection := agentGwSyncer.buildAddressCollections(krtOpts)

	// Wait for collections to sync
	kubeclient.WaitForCacheSync("agw-resources", ctx.Done(), agwResourcesCollection.HasSynced)
	kubeclient.WaitForCacheSync("addresses", ctx.Done(), addressesCollection.HasSynced)

	// build final proxy xds result
	agentGwSyncer.buildXDSCollection(agwResourcesCollection, addressesCollection, krtOpts)

	sq := &TestStatusQueue{
		state: map[status.Resource]any{},
	}
	// Normally we don't care to block on status being written, but here we need to since we want to test output
	statusSynced := agentGwSyncer.StatusCollections().SetQueue(sq)
	for _, st := range statusSynced {
		st.WaitUntilSynced(ctx.Done())
	}

	return ActualTestResult{
		Resources: agwResourcesCollection.List(),
		Addresses: addressesCollection.List(),
		Status:    sq,
	}, nil
}

var _ status.WorkerQueue = &TestStatusQueue{}

type TestStatusQueue struct {
	mu    sync.Mutex
	state map[status.Resource]any
}

func (t *TestStatusQueue) Push(target status.Resource, data any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state[target] = data
}

func (t *TestStatusQueue) Run(ctx context.Context) {
}

var timestampRegex = regexp.MustCompile(`lastTransitionTime:.*`)

func (t *TestStatusQueue) Dump() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	sb := strings.Builder{}
	objs := []crd.IstioKind{}
	for k, v := range t.state {
		statusj, _ := json.Marshal(v)
		obj := crd.IstioKind{
			TypeMeta: metav1.TypeMeta{
				Kind:       k.Kind,
				APIVersion: k.GroupVersion().String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      k.Name,
				Namespace: k.Namespace,
			},
			Spec:   nil,
			Status: ptr.Of(json.RawMessage(statusj)),
		}
		objs = append(objs, obj)
	}
	slices.SortFunc(objs, func(a, b crd.IstioKind) int {
		ord := []string{gvk.GatewayClass.Kind, gvk.Gateway.Kind, gvk.HTTPRoute.Kind, gvk.GRPCRoute.Kind, gvk.TLSRoute.Kind, gvk.TCPRoute.Kind}
		if r := gocmp.Compare(slices.Index(ord, a.Kind), slices.Index(ord, b.Kind)); r != 0 {
			return r
		}
		if r := a.CreationTimestamp.Time.Compare(b.CreationTimestamp.Time); r != 0 {
			return r
		}
		if r := gocmp.Compare(a.Namespace, b.Namespace); r != 0 {
			return r
		}
		return gocmp.Compare(a.Name, b.Name)
	})
	for _, obj := range objs {
		b, err := yaml.Marshal(obj)
		if err != nil {
			panic(err.Error())
		}
		// Replace parts that are not stable
		b = timestampRegex.ReplaceAll(b, []byte("lastTransitionTime: fake"))
		sb.WriteString(string(b))
		sb.WriteString("---\n")
	}
	return sb.String()
}

func proxySyncerPluginFactory(ctx context.Context, commoncol *collections.CommonCollections, extraPluginsFn ExtraPluginsFn, globalSettings apisettings.Settings) pluginsdk.Plugin {
	plugins := registry.Plugins(ctx, commoncol, wellknown.DefaultAgwClassName, globalSettings, nil)

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

// agwPluginFactory is a factory function that returns the agent gateway plugins
// It is based on agwPluginFactory(cfg)(ctx, cfg.AgwCollections) in start.go
func agwPluginFactory(ctx context.Context, agwCollections *agwplugins.AgwCollections) agwplugins.AgwPlugin {
	agwPlugins := agwplugins.Plugins(agwCollections)
	mergedPlugins := agwplugins.MergePlugins(agwPlugins...)
	for i, plug := range agwPlugins {
		kubeclient.WaitForCacheSync(fmt.Sprintf("plugin-%d", i), ctx.Done(), plug.HasSynced)
	}
	return mergedPlugins
}

func dumpOnFailure(t *testing.T, debugger *krt.DebugHandler) {
	t.Cleanup(func() {
		if t.Failed() {
			b, _ := yaml.Marshal(debugger)
			t.Log(string(b))
		}
	})
}
