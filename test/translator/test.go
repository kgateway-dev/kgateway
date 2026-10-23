package translator

import (
	stdcmp "cmp"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyapikeyauthv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/api_key_auth/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/anypb"
	kubeclient "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	apiserverschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/registry"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/gatewaytls"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/listener"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

type translationResult struct {
	Routes        []*envoyroutev3.RouteConfiguration
	Listeners     []*envoylistenerv3.Listener
	ExtraClusters []*envoyclusterv3.Cluster
	Clusters      []*envoyclusterv3.Cluster
	Endpoints     []*envoyendpointv3.ClusterLoadAssignment
	Secrets       []*envoytlsv3.Secret
	Statuses      *Statuses
}

func (tr *translationResult) MarshalJSON() ([]byte, error) {
	m := protojson.MarshalOptions{
		Indent: "  ",
	}

	// Create a map to hold the marshaled fields
	result := make(map[string]any)

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

	if len(tr.Endpoints) > 0 {
		endpoints, err := marshalProtoMessages(tr.Endpoints, m)
		if err != nil {
			return nil, err
		}
		result["Endpoints"] = endpoints
	}

	if len(tr.Secrets) > 0 {
		secrets, err := marshalProtoMessages(tr.Secrets, m)
		if err != nil {
			return nil, err
		}
		result["Secrets"] = secrets
	}

	// Add statuses if they exist
	if tr.Statuses != nil {
		result["Statuses"] = tr.Statuses
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

	if endpointsData, ok := result["Endpoints"]; ok {
		var endpoints []json.RawMessage
		if err := json.Unmarshal(endpointsData, &endpoints); err != nil {
			return err
		}
		tr.Endpoints = make([]*envoyendpointv3.ClusterLoadAssignment, len(endpoints))
		for i, endpointData := range endpoints {
			endpoint := &envoyendpointv3.ClusterLoadAssignment{}
			if err := m.Unmarshal(endpointData, endpoint); err != nil {
				return err
			}
			tr.Endpoints[i] = endpoint
		}
	}

	if secretsData, ok := result["Secrets"]; ok {
		var secrets []json.RawMessage
		if err := json.Unmarshal(secretsData, &secrets); err != nil {
			return err
		}
		tr.Secrets = make([]*envoytlsv3.Secret, len(secrets))
		for i, secretData := range secrets {
			secret := &envoytlsv3.Secret{}
			if err := m.Unmarshal(secretData, secret); err != nil {
				return err
			}
			tr.Secrets[i] = secret
		}
	}

	// Unmarshal statuses if they exist
	if statusesData, ok := result["Statuses"]; ok {
		tr.Statuses = &Statuses{}
		if err := json.Unmarshal(statusesData, tr.Statuses); err != nil {
			return err
		}
	}

	return nil
}

func marshalProtoMessages[T proto.Message](messages []T, m protojson.MarshalOptions) ([]any, error) {
	var result []any
	for _, msg := range messages {
		data, err := m.Marshal(msg)
		if err != nil {
			return nil, err
		}
		var jsonObj any
		if err := json.Unmarshal(data, &jsonObj); err != nil {
			return nil, err
		}
		result = append(result, jsonObj)
	}
	return result, nil
}

type ExtraPluginsFn func(ctx context.Context, commoncol *collections.CommonCollections, mergeSettingsJSON string) []pluginsdk.Plugin

// ExtraStatusSyncerOptionsFn returns the status pipeline options an extension passes to the
// proxy syncer (setup.WithStatusSyncerOptions). It runs after PluginsFn and before the fake
// client starts, so informer-backed collections it builds from commoncol's client are synced
// before translation.
type ExtraStatusSyncerOptionsFn func(ctx context.Context, commoncol *collections.CommonCollections) []proxy_syncer.StatusSyncerOption

type ExtraConfig struct {
	NewClientFn func(*testing.T, ...client.Object) apiclient.Client
	PluginsFn   ExtraPluginsFn
	// StatusSyncerOptionsFn supplies the extension's status pipeline options. The harness
	// honors the policy target resolvers among them (proxy_syncer.WithPolicyTargetResolver),
	// so policies targeting a missing object of an extension kind report TargetNotFound as
	// they would in the controller; other options are ignored.
	StatusSyncerOptionsFn ExtraStatusSyncerOptionsFn
	Schemes               runtime.SchemeBuilder
	GVKToStructuralSchema map[schema.GroupVersionKind]*apiserverschema.Structural
	IncludeEndpoints      bool
	// RegisterTestBackendPlugin registers the fake "test-backend-plugin" backend kind
	// and one backend of that kind (default/example-svc:80). Set it only for fixtures
	// that route to that kind; its cluster otherwise lands in every golden.
	RegisterTestBackendPlugin bool
}

func NewScheme(extraSchemes runtime.SchemeBuilder) *runtime.Scheme {
	scheme := schemes.GatewayScheme()
	extraSchemes = append(extraSchemes, kgateway.Install)
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
	settingsOpts ...SettingsOpts,
) {
	TestTranslationWithExtraPlugins(t, ctx, inputFiles, outputFile, gwNN, ExtraConfig{}, settingsOpts...)
}

func TestTranslationWithExtraPlugins(
	t *testing.T,
	ctx context.Context,
	inputFiles []string,
	outputFile string,
	gwNN types.NamespacedName,
	extraConfig ExtraConfig,
	settingsOpts ...SettingsOpts,
) {
	scheme := NewScheme(extraConfig.Schemes)
	r := require.New(t)

	tc := TestCase{
		InputFiles: inputFiles,
	}
	results, err := tc.Run(t, ctx, scheme, extraConfig, settingsOpts...)
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
	result.Endpoints = sortEndpoints(result.Endpoints)
	output := &translationResult{
		Routes:        result.Proxy.Routes,
		Listeners:     result.Proxy.Listeners,
		ExtraClusters: result.Proxy.ExtraClusters,
		Clusters:      result.Clusters,
		Endpoints:     result.Endpoints,
		Secrets:       result.Proxy.Secrets,
		Statuses:      buildStatusesFromReports(result.ReportsMap, result.Gateways, result.ListenerSets),
	}
	outputYaml, err := testutils.MarshalAnyYaml(output)
	r.NoErrorf(err, "error marshaling output to YAML; actual result: %s", outputYaml)

	if envutils.IsEnvTruthy("REFRESH_GOLDEN") {
		// create parent directory if it doesn't exist
		dir := filepath.Dir(outputFile)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			r.NoErrorf(err, "error creating directory %s", dir)
		}
		t.Log("REFRESH_GOLDEN is set, writing output file", outputFile)
		r.NoError(writeGoldenFile(outputFile, outputYaml), "error writing output file")
	}

	expected := &translationResult{}
	err = ReadYamlFile(outputFile, expected)
	r.NoError(err, "error reading golden output file")

	r.Emptyf(compareProxy(expected, result.Proxy),
		"unexpected diff in proxy output; actual result: %s", outputYaml)
	r.Emptyf(compareClusters(expected.Clusters, result.Clusters),
		"unexpected diff in clusters output; actual result: %s", outputYaml)
	r.Emptyf(compareEndpoints(expected.Endpoints, result.Endpoints),
		"unexpected diff in endpoints output; actual result: %s", outputYaml)
	r.Emptyf(compareStatuses(expected.Statuses, output.Statuses),
		"unexpected diff in statuses output; actual result: %s", outputYaml)
}

// writeGoldenFile replaces path with data by renaming a temp file over it. Callers
// run as parallel subtests and some share a golden, so a concurrent reader must see
// either the old file or the new one, never a truncated one.
func writeGoldenFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(f.Name(), 0o644); err != nil { //nolint:gosec // G302: Golden test file can be readable
		return err
	}
	return os.Rename(f.Name(), path)
}

type TestCase struct {
	InputFiles []string
}

type ActualTestResult struct {
	Proxy         *irtranslator.TranslationResult
	ReportsMap    reports.ReportMap
	Gateways      map[types.NamespacedName]*gwv1.Gateway
	ListenerSets  map[types.NamespacedName]*gwv1.ListenerSet
	PolicyPlugins map[schema.GroupKind]pluginsdk.PolicyPlugin
	Clusters      []*envoyclusterv3.Cluster
	Endpoints     []*envoyendpointv3.ClusterLoadAssignment
}

func compareProxy(expected *translationResult, actualProxy *irtranslator.TranslationResult) string {
	expectedProxy := &irtranslator.TranslationResult{
		Routes:        expected.Routes,
		Listeners:     expected.Listeners,
		ExtraClusters: expected.ExtraClusters,
		Secrets:       expected.Secrets,
	}

	// Sort credentials by client name to ensure deterministic comparison
	credentialSortFn := func(x, y *envoyapikeyauthv3.Credential) bool {
		return x.Client < y.Client
	}

	return cmp.Diff(sortProxy(expectedProxy), sortProxy(actualProxy), protocmp.Transform(), protocmp.SortRepeated(credentialSortFn), cmpopts.EquateNaNs())
}

func sortProxy(proxy *irtranslator.TranslationResult) *irtranslator.TranslationResult {
	if proxy == nil {
		return nil
	}

	slices.SortFunc(proxy.Listeners, func(a, b *envoylistenerv3.Listener) int {
		return stdcmp.Compare(a.GetName(), b.GetName())
	})
	slices.SortFunc(proxy.Routes, func(a, b *envoyroutev3.RouteConfiguration) int {
		return stdcmp.Compare(a.GetName(), b.GetName())
	})
	slices.SortFunc(proxy.ExtraClusters, func(a, b *envoyclusterv3.Cluster) int {
		return stdcmp.Compare(a.GetName(), b.GetName())
	})
	slices.SortFunc(proxy.Secrets, func(a, b *envoytlsv3.Secret) int {
		return stdcmp.Compare(a.GetName(), b.GetName())
	})

	// Sort credentials in routes to ensure deterministic output
	// This is to avoid local changes every time the test is run with REFRESH_GOLDEN=true
	for _, routeConfig := range proxy.Routes {
		sortCredentialsInRouteConfiguration(routeConfig)
	}

	return proxy
}

// sortCredentialsInRouteConfiguration sorts API key auth credentials within route configurations
func sortCredentialsInRouteConfiguration(routeConfig *envoyroutev3.RouteConfiguration) {
	if routeConfig == nil {
		return
	}

	for _, vh := range routeConfig.GetVirtualHosts() {
		// Sort credentials in route-level typedPerFilterConfig
		for _, route := range vh.GetRoutes() {
			sortCredentialsInRoute(route)
		}

		// Sort credentials in virtual host-level typedPerFilterConfig
		if vh.GetTypedPerFilterConfig() != nil {
			if config, ok := vh.GetTypedPerFilterConfig()["envoy.filters.http.api_key_auth"]; ok {
				sortCredentialsInAny(config)
			}
		}
	}

	// Sort credentials in route configuration-level typedPerFilterConfig
	if routeConfig.GetTypedPerFilterConfig() != nil {
		if config, ok := routeConfig.GetTypedPerFilterConfig()["envoy.filters.http.api_key_auth"]; ok {
			sortCredentialsInAny(config)
		}
	}
}

// sortCredentialsInRoute sorts API key auth credentials in a route's typedPerFilterConfig
func sortCredentialsInRoute(route *envoyroutev3.Route) {
	if route == nil || route.GetTypedPerFilterConfig() == nil {
		return
	}

	if config, ok := route.GetTypedPerFilterConfig()["envoy.filters.http.api_key_auth"]; ok {
		sortCredentialsInAny(config)
	}
}

// sortCredentialsInAny sorts credentials in an ApiKeyAuthPerRoute config stored as anypb.Any
func sortCredentialsInAny(config *anypb.Any) {
	if config == nil {
		return
	}

	// Unmarshal to ApiKeyAuthPerRoute
	apiKeyAuth := &envoyapikeyauthv3.ApiKeyAuthPerRoute{}
	if err := config.UnmarshalTo(apiKeyAuth); err != nil {
		// Not an ApiKeyAuthPerRoute, skip
		return
	}

	// Sort credentials by client name
	if len(apiKeyAuth.Credentials) > 0 {
		slices.SortFunc(apiKeyAuth.Credentials, func(a, b *envoyapikeyauthv3.Credential) int {
			return stdcmp.Compare(a.Client, b.Client)
		})

		// Marshal back to Any and update the config
		a, err := utils.MessageToAny(apiKeyAuth)
		if err == nil {
			config.TypeUrl = a.TypeUrl
			config.Value = a.Value
		}
	}
}

func compareClusters(expectedClusters, actualClusters []*envoyclusterv3.Cluster) string {
	// Sort both expected and actual clusters by name and compare
	return cmp.Diff(sortClusters(expectedClusters), sortClusters(actualClusters), protocmp.Transform(), cmpopts.EquateNaNs())
}

func sortClusters(clusters []*envoyclusterv3.Cluster) []*envoyclusterv3.Cluster {
	if len(clusters) == 0 {
		return clusters
	}
	slices.SortFunc(clusters, func(a, b *envoyclusterv3.Cluster) int {
		return stdcmp.Compare(a.GetName(), b.GetName())
	})
	// Deliberately no sorting inside a load assignment. Locality order within a CLA
	// is an invariant of the translation, not an artifact: endpoints.sortedLocalities
	// exists so an inline CLA - part of the cluster's content hash, and therefore of
	// the CDS version - does not get republished and re-warmed on every recompute.
	// Re-sorting here would make that regression invisible to every golden.
	return clusters
}

func compareEndpoints(expectedEndpoints, actualEndpoints []*envoyendpointv3.ClusterLoadAssignment) string {
	return cmp.Diff(sortEndpoints(expectedEndpoints), sortEndpoints(actualEndpoints), protocmp.Transform(), cmpopts.EquateNaNs())
}

// sortEndpoints orders the CLAs themselves; the locality groups inside each one are
// left exactly as translated, for the reason given in sortClusters.
func sortEndpoints(endpoints []*envoyendpointv3.ClusterLoadAssignment) []*envoyendpointv3.ClusterLoadAssignment {
	slices.SortFunc(endpoints, func(a, b *envoyendpointv3.ClusterLoadAssignment) int {
		return stdcmp.Compare(a.GetClusterName(), b.GetClusterName())
	})
	return endpoints
}

func ReadYamlFile(file string, out any) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	return testutils.UnmarshalAnyYaml(data, out)
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
		status := reportsMap.BuildRouteStatus(&r, wellknown.DefaultGatewayClassName)

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

const (
	// settlePolls is how many consecutive unchanged snapshots count as quiescence.
	settlePolls = 3
	// settleInterval is the gap between snapshots. Every scenario pays at least
	// settlePolls * settleInterval, so keep it small: the point is to yield to
	// queued krt work, not to wait out a fixed duration.
	settleInterval = 5 * time.Millisecond
	settleTimeout  = 10 * time.Second
)

// waitForEndpointsToSettle waits for the endpoint IR the goldens are built from to
// quiesce, and returns the settled view. Endpoint IRs depend dynamically on pods and
// EndpointSlices, and HasSynced on every collection does not imply the queue is empty:
// a dependency-triggered recompute can still be in flight, so snapshotting right after
// HasSynced races with it and yields nondeterministic endpoint output.
//
// krt offers no drain barrier to wait on instead, and this is not only a test problem:
// proxy_syncer gates its first snapshot on the same HasSynced signals, which is why it
// carries both the deferral in perclient.go and the first-connect delay in
// pkg/krtcollections/uniqueclients.go to keep a proxy's first watch out of that window.
// Polling here mirrors that gap rather than hiding one.
//
// The caller translates the returned snapshot itself rather than recomputing it, so the
// endpoints in the golden are exactly the ones that were observed to be stable. See
// finalEndpointsSnapshot for why the snapshot is the policy-attached view: settling on
// the raw plugin-contributed collection instead would miss both a backend arriving in
// BackendIndex and a policy attaching to one, neither of which is visible in a raw
// endpoint row.
func waitForEndpointsToSettle(
	t *testing.T,
	ctx context.Context,
	snapshot func() map[string]*ir.EndpointsForBackend,
) map[string]*ir.EndpointsForBackend {
	t.Helper()
	prev := snapshot()
	stable := 0
	timeout := time.After(settleTimeout)
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("context ended while waiting for endpoints to settle: %v", ctx.Err())
		case <-timeout:
			t.Fatalf("endpoints collection did not settle within %s", settleTimeout)
		case <-time.After(settleInterval):
		}
		cur := snapshot()
		if endpointsEqual(prev, cur) {
			stable++
			if stable >= settlePolls {
				return cur
			}
		} else {
			stable = 0
		}
		prev = cur
	}
}

// finalEndpointsSnapshot returns the endpoint IR that the endpoint goldens are
// translated from, keyed by backend resource name: every policy-carrying backend's raw
// endpoints projected onto the policy-attached backend, exactly as proxy_syncer's
// FinalBackendEndpoints collection does it.
func finalEndpointsSnapshot(commoncol *collections.CommonCollections) map[string]*ir.EndpointsForBackend {
	out := make(map[string]*ir.EndpointsForBackend)
	for _, col := range commoncol.BackendIndex.BackendsWithPolicy() {
		for _, backend := range col.List() {
			raw := krt.FetchOne(krt.TestingDummyContext{}, commoncol.Endpoints, krt.FilterKey(backend.ResourceName()))
			if ep := proxy_syncer.FinalBackendEndpoints(backend, raw); ep != nil {
				out[backend.ResourceName()] = ep
			}
		}
	}
	return out
}

func endpointsEqual(a, b map[string]*ir.EndpointsForBackend) bool {
	if len(a) != len(b) {
		return false
	}
	for name, ea := range a {
		eb, ok := b[name]
		if !ok || !ea.Equals(*eb) {
			return false
		}
	}
	return true
}

func GetPolicyStatusError(
	reportsMap reports.ReportMap,
	policy *reporter.PolicyKey,
) error {
	for key := range reportsMap.Policies {
		if policy != nil && *policy != key {
			continue
		}
		status := buildPolicyStatus(reportsMap, key, gwv1.PolicyStatus{})
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
		status := reportsMap.BuildRouteStatus(&r, wellknown.DefaultGatewayClassName)

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
		r := gwv1.TLSRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nns.Name,
				Namespace: nns.Namespace,
			},
		}
		status := reportsMap.BuildRouteStatus(&r, wellknown.DefaultGatewayClassName)

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
		status := reportsMap.BuildRouteStatus(&r, wellknown.DefaultGatewayClassName)

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
		status := reportsMap.BuildGWStatus(g, nil)
		for _, c := range status.Conditions {
			if c.Type == listener.GatewayConditionAttachedListenerSets {
				// A gateway might or might not have AttachedListenerSets so skip this condition
				continue
			}
			if c.Status != metav1.ConditionTrue {
				return fmt.Errorf("condition not accepted for gw %v condition: %v", nns, c)
			}
		}
	}

	for gvk, listenerSetsForGVK := range reportsMap.ListenerSets {
		for ls := range listenerSetsForGVK {
			l := gwv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ls.Name,
					Namespace: ls.Namespace,
				},
			}
			l.SetGroupVersionKind(gvk)
			status := reportsMap.BuildListenerSetStatus(l)
			for _, c := range status.Conditions {
				if c.Status != metav1.ConditionTrue {
					return fmt.Errorf("condition not accepted for listenerSet %s condition: %v", ls, c)
				}
			}
		}
	}

	err = GetPolicyStatusError(reportsMap, nil)
	if err != nil {
		return err
	}

	return nil
}

type SettingsOpts func(*apisettings.Settings)

func (tc TestCase) Run(
	t *testing.T,
	ctx context.Context,
	scheme *runtime.Scheme,
	extraConfig ExtraConfig,
	settingsOpts ...SettingsOpts,
) (map[types.NamespacedName]ActualTestResult, error) {
	r := require.New(t)

	// If GVKToStructuralSchema is provided, use it; otherwise load from our CRDs
	gvkToStructuralSchema := extraConfig.GVKToStructuralSchema
	if len(gvkToStructuralSchema) == 0 {
		var err error
		gvkToStructuralSchema, err = testutils.GetStructuralSchemasForAllCharts()
		r.NoError(err, "error getting structural schemas")
	}

	var allObjs []client.Object
	var fakeNow time.Time
	for _, file := range tc.InputFiles {
		objs, err := testutils.LoadFromFiles(file, scheme, gvkToStructuralSchema)
		if err != nil {
			return nil, err
		}
		// add a creation timestamp to each object to ensure consistent application of policy
		for _, obj := range objs {
			if secret, ok := obj.(*corev1.Secret); ok && len(secret.StringData) > 0 {
				if secret.Data == nil {
					secret.Data = make(map[string][]byte, len(secret.StringData))
				}
				for key, value := range secret.StringData {
					secret.Data[key] = []byte(value)
				}
			}
			fakeNow = fakeNow.Add(time.Second)
			obj.SetCreationTimestamp(metav1.NewTime(fakeNow))
		}
		allObjs = append(allObjs, objs...)
	}

	var fakeClient apiclient.Client
	if extraConfig.NewClientFn != nil {
		fakeClient = extraConfig.NewClientFn(t, allObjs...)
	} else {
		fakeClient = fake.NewClient(t, allObjs...)
	}
	defer fakeClient.Shutdown()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// ensure classes used in tests exist and point at our controller
	gwClasses := []string{
		wellknown.DefaultGatewayClassName,
		wellknown.DefaultWaypointClassName,
		"example-gateway-class",
	}
	for _, className := range gwClasses {
		fakeClient.GatewayAPI().GatewayV1().GatewayClasses().Create(ctx, &gwv1.GatewayClass{
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

	settings, err := apisettings.BuildSettings()
	if err != nil {
		return nil, err
	}
	for _, opt := range settingsOpts {
		opt(settings)
	}

	commoncol, err := collections.NewCommonCollections(
		ctx,
		krtOpts,
		fakeClient,
		wellknown.DefaultGatewayControllerName,
		*settings,
	)
	if err != nil {
		return nil, err
	}

	v := validator.NewDocker()
	plugins := registry.Plugins(ctx, commoncol, *settings, v)
	// TODO: consider moving the common code to a util that both proxy syncer and this test call
	plugins = append(plugins, krtcollections.NewBuiltinPlugin(ctx))

	var extraPlugs []pluginsdk.Plugin
	if extraConfig.PluginsFn != nil {
		extraPlugins := extraConfig.PluginsFn(ctx, commoncol, settings.PolicyMerge)
		extraPlugs = append(extraPlugs, extraPlugins...)
	}
	plugins = append(plugins, extraPlugs...)
	extensions := registry.MergePlugins(plugins...)

	var statusSyncerOpts []proxy_syncer.StatusSyncerOption
	if extraConfig.StatusSyncerOptionsFn != nil {
		statusSyncerOpts = extraConfig.StatusSyncerOptionsFn(ctx, commoncol)
	}

	// A fake backend kind for fixtures that route to a plugin-contributed backend
	// (inputs/backends/basic.yaml). Opt-in: the backend it contributes shows up in
	// BackendsWithPolicy() for every gateway, so registering it unconditionally would
	// put a cluster that belongs to the harness into every golden in every suite.
	if extraConfig.RegisterTestBackendPlugin {
		gk := schema.GroupKind{
			Group: "",
			Kind:  "test-backend-plugin",
		}
		extensions.ContributesPolicies[gk] = pluginsdk.PolicyPlugin{
			Name: "test-backend-plugin",
		}
		testBackend := ir.NewBackendObjectIR(ir.ObjectSource{
			Kind:      "test-backend-plugin",
			Namespace: "default",
			Name:      "example-svc",
		}, 80, "", "")
		extensions.ContributesBackends[gk] = pluginsdk.BackendPlugin{
			Backends: krt.NewStaticCollection(nil, []ir.BackendObjectIR{
				testBackend,
			}),
			BackendInit: ir.BackendInit{
				InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					return nil
				},
			},
		}
	}

	commoncol.InitPlugins(ctx, extensions, *settings)

	translator := translator.NewCombinedTranslator(ctx, extensions, commoncol, v)
	translator.Init(ctx)

	fakeClient.RunAndWait(ctx.Done())
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
	// Settling has to come after every HasSynced above: commoncol.Endpoints is a join
	// over the endpoint collections contributed by all plugins, extra ones included, so
	// quiescence measured before a plugin syncs can be quiescence of a partial join.
	var settledEndpoints map[string]*ir.EndpointsForBackend
	if extraConfig.IncludeEndpoints {
		settledEndpoints = waitForEndpointsToSettle(t, ctx, func() map[string]*ir.EndpointsForBackend {
			return finalEndpointsSnapshot(commoncol)
		})
	}

	results := make(map[types.NamespacedName]ActualTestResult)
	queries := query.NewData(commoncol)

	// Build a map of all gateways by NamespacedName for status building
	gatewayMap := make(map[types.NamespacedName]*gwv1.Gateway)
	for _, gw := range commoncol.GatewayIndex.Gateways.List() {
		gwNN := types.NamespacedName{
			Namespace: gw.Namespace,
			Name:      gw.Name,
		}
		gatewayMap[gwNN] = gw.Obj
	}

	// Build a map of all ListenerSets by nn for status building. We extract these
	// from the loaded input objects since they're not directly available via InitCollections()
	// (i.e. no dedicated KRT collection).
	listenerSetMap := make(map[types.NamespacedName]*gwv1.ListenerSet)
	for _, obj := range allObjs {
		if ls, ok := obj.(*gwv1.ListenerSet); ok {
			listenerSetMap[client.ObjectKeyFromObject(ls)] = ls
		}
	}

	// Status reports read translation errors from the proxy syncer's status
	// rows; build them for one test client.
	ucc := ir.NewUniquelyConnectedClient("test", "test", nil, ir.PodLocality{})
	perClientClusters := proxy_syncer.NewPerClientEnvoyClusters(ctx, krtOpts, translator.GetBackendTranslator(),
		krt.JoinCollection(commoncol.BackendIndex.BackendsWithPolicy(), krtOpts.ToOptions("FinalBackends")...),
		krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{ucc}, krtOpts.ToOptions("UniqueClients")...))
	perClientClusters.StatusClusters().WaitUntilSynced(ctx.Done())
	statusRows := perClientClusters.StatusClusters().List()

	for _, gw := range commoncol.GatewayIndex.Gateways.List() {
		xdsSnap, reportsMap := translator.TranslateGateway(krt.TestingDummyContext{}, ctx, gw)

		// Backend policies (e.g. BackendConfigPolicy) use a different reporting pipeline than gateway policies.
		// Gateway policies (ListenerPolicy, TrafficPolicy) are reported during gateway translation via the
		// standard reporter mechanism. Backend policies are processed differently - they don't use the reporter
		// during translation, instead their reports are generated separately by GenerateBackendPolicyReport().
		// We need to merge both report types to capture all policy statuses for golden file testing.
		var backendIRs []*ir.BackendObjectIR
		for _, col := range commoncol.BackendIndex.BackendsWithPolicyRequiringStatus() {
			backendIRs = append(backendIRs, col.List()...)
		}
		backendPolicyReports := proxy_syncer.GenerateBackendPolicyReport(backendIRs, statusRows)

		// Merge gateway reports with backend policy reports. A policy can appear in both
		// (BackendTLSPolicy reports Gateway ancestors from translation and target ancestors
		// from the backend path), so union the ancestors rather than replacing the report.
		mergedReports := reportsMap
		for key, backendReport := range backendPolicyReports.Policies {
			if existing, ok := mergedReports.Policies[key]; ok && existing != nil && backendReport != nil {
				maps.Copy(existing.Ancestors, backendReport.Ancestors)
				continue
			}
			mergedReports.Policies[key] = backendReport
		}

		// Policies whose targetRefs do not resolve are reported by a third producer in the proxy
		// syncer (see proxy_syncer/policy_target_status.go). A policy may have both a Gateway
		// ancestor from translation and a TargetNotFound ancestor, so merge by ancestor rather
		// than replacing the policy's report.
		mergedReports.MergePolicyReports(proxy_syncer.GeneratePolicyTargetReports(commoncol, extensions, statusSyncerOpts...))

		// Backend Accepted conditions are generated outside gateway translation
		// (see proxy_syncer's backendStatusContributions); reproduce them here so
		// golden files capture Backend statuses.
		var kgwBackends []ir.BackendObjectIR
		var kgwExtraConditions []ir.BackendObjectStatus
		if kgwBackendPlugin, ok := extensions.ContributesBackends[wellknown.BackendGVK.GroupKind()]; ok {
			if kgwBackendPlugin.Backends != nil {
				kgwBackends = kgwBackendPlugin.Backends.List()
			}
			if kgwBackendPlugin.ExtraConditions != nil {
				kgwExtraConditions = kgwBackendPlugin.ExtraConditions.List()
			}
		}
		backendStatusReports := proxy_syncer.GenerateBackendStatusReport(kgwBackends, statusRows, kgwExtraConditions)
		maps.Copy(mergedReports.Backends, backendStatusReports.Backends)

		gwNN := types.NamespacedName{
			Namespace: gw.Namespace,
			Name:      gw.Name,
		}
		actual := ActualTestResult{
			Proxy:         xdsSnap,
			ReportsMap:    mergedReports,
			Gateways:      gatewayMap,
			ListenerSets:  listenerSetMap,
			PolicyPlugins: extensions.ContributesPolicies,
		}
		results[gwNN] = actual

		ctx := context.Background()
		t := translator.GetBackendTranslator()
		var clusters []*envoyclusterv3.Cluster
		var endpointAssignments []*envoyendpointv3.ClusterLoadAssignment
		referencedClusters := extractRouteConfigurationClusterNames(xdsSnap.Routes)
		// appendEndpoints translates ep for the cluster just appended. Preserve empty
		// assignments: publishing them is required to clear stale endpoints from Envoy.
		appendEndpoints := func(ep *ir.EndpointsForBackend) {
			if ep == nil {
				return
			}
			resolved := translator.ResolveEndpoints(krt.TestingDummyContext{}, ucc, *ep)
			if cla := translator.BuildClusterLoadAssignment(ucc, resolved); cla != nil {
				endpointAssignments = append(endpointAssignments, cla)
			}
		}
		for _, col := range commoncol.BackendIndex.BackendsWithPolicy() {
			for _, backend := range col.List() {
				// Errored translations (including strict-mode validation failures) are
				// skipped rather than failing the test: snapshotPerClient omits errored
				// clusters from CDS, so the golden output must omit them too.
				cluster, base, err := translateBackendForGolden(ctx, krt.TestingDummyContext{}, t, ucc, backend)
				if err != nil {
					continue
				}
				if cluster != nil {
					clusters = append(clusters, cluster)
					// Optionally translate EDS assignments so selected goldens cover endpoint
					// derivation, for every backend kind that has endpoint IR - Services,
					// ServiceEntries, InferencePools.
					if extraConfig.IncludeEndpoints && cluster.GetType() == envoyclusterv3.Cluster_EDS {
						// settledEndpoints is the policy-attached endpoint IR (see
						// finalEndpointsSnapshot), not the raw commoncol.Endpoints rows: those
						// predate policy attachment, so the endpoint plugins would run against
						// empty AttachedPolicies and all be no-ops here.
						appendEndpoints(settledEndpoints[backend.ResourceName()])
					}
				}
				// snapshotPerClient merges generated resources into LDS and SDS.
				xdsSnap.Listeners = append(xdsSnap.Listeners, base.Listeners...)
				xdsSnap.Secrets = append(xdsSnap.Secrets, base.Secrets...)
			}
		}
		if clientCertificate, err := gatewaytls.ResolveForGateway(krt.TestingDummyContext{}, ctx, queries, &gw); err == nil && clientCertificate != nil {
			for _, col := range commoncol.BackendIndex.BackendsWithPolicy() {
				for _, backend := range col.List() {
					clone := backend.CloneForGatewayBackendClientCertificate(gw.ObjectSource, clientCertificate)
					if _, ok := referencedClusters[clone.ClusterName()]; !ok {
						continue
					}

					cluster, base, err := translateBackendForGolden(ctx, krt.TestingDummyContext{}, t, ucc, &clone)
					if err != nil {
						continue
					}
					if cluster == nil {
						continue
					}
					clusters = append(clusters, cluster)
					// The variant's EDS cluster needs its own CLA. Build it the way
					// proxy_syncer's newGatewayBackendVariantEndpoints does - the base
					// backend's endpoints under the clone's identity - then project it
					// through FinalBackendEndpoints like every other backend.
					if extraConfig.IncludeEndpoints && cluster.GetType() == envoyclusterv3.Cluster_EDS {
						if base := settledEndpoints[backend.ResourceName()]; base != nil {
							variant := ir.NewEndpointsForBackend(clone)
							variant.ReuseEndpointsFrom(base)
							appendEndpoints(proxy_syncer.FinalBackendEndpoints(&clone, variant))
						}
					}
					xdsSnap.Listeners = append(xdsSnap.Listeners, base.Listeners...)
					xdsSnap.Secrets = append(xdsSnap.Secrets, base.Secrets...)
				}
			}
		}
		r := results[gwNN]
		r.Clusters = clusters
		r.Endpoints = endpointAssignments
		results[gwNN] = r
	}

	return results, nil
}

// translateBackendForGolden selects the base or per-client cluster using the
// same translation contract as snapshot assembly, and returns the base, which
// carries the generated resources. Errored translations return no cluster
// because snapshotPerClient excludes errored rows from CDS.
func translateBackendForGolden(
	ctx context.Context,
	kctx krt.HandlerContext,
	backendTranslator *irtranslator.BackendTranslator,
	ucc ir.UniquelyConnectedClient,
	backend *ir.BackendObjectIR,
) (*envoyclusterv3.Cluster, *irtranslator.BaseCluster, error) {
	base := backendTranslator.TranslateBackendBase(krt.TestingDummyContext{}, ctx, backend)
	if base.Error != nil {
		return nil, nil, base.Error
	}
	perClient, err := backendTranslator.ApplyPerClient(kctx, ctx, ucc, backend, base)
	if err != nil {
		return nil, nil, err
	}
	if perClient != nil {
		return perClient, base, nil
	}
	return base.Cluster, base, nil
}

func extractRouteConfigurationClusterNames(routeConfigs []*envoyroutev3.RouteConfiguration) map[string]struct{} {
	clusterNames := make(map[string]struct{})
	for _, routeConfig := range routeConfigs {
		for _, virtualHost := range routeConfig.GetVirtualHosts() {
			for _, route := range virtualHost.GetRoutes() {
				switch action := route.GetAction().(type) {
				case *envoyroutev3.Route_Route:
					if action.Route == nil {
						continue
					}
					switch clusterSpecifier := action.Route.GetClusterSpecifier().(type) {
					case *envoyroutev3.RouteAction_Cluster:
						if clusterSpecifier.Cluster != "" {
							clusterNames[clusterSpecifier.Cluster] = struct{}{}
						}
					case *envoyroutev3.RouteAction_WeightedClusters:
						if clusterSpecifier.WeightedClusters == nil {
							continue
						}
						for _, weightedCluster := range clusterSpecifier.WeightedClusters.GetClusters() {
							if weightedCluster.GetName() != "" {
								clusterNames[weightedCluster.GetName()] = struct{}{}
							}
						}
					}
				}
			}
		}
	}
	return clusterNames
}
