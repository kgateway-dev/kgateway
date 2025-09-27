package setup_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/agentgateway/agentgateway/go/api"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/test/util/retry"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/test/envtestutil"
)

func TestAgentgateway(t *testing.T) {
	st, err := envtestutil.BuildSettings()
	st.EnableAgentgateway = true
	st.EnableInferExt = true

	if err != nil {
		t.Fatalf("can't get settings %v", err)
	}

	// Use the runScenario approach to test agent gateway scenarios
	runAgentgatewayScenario(t, "testdata/agentgateway", st)
}

func runAgentgatewayScenario(t *testing.T, scenarioDir string, globalSettings *apisettings.Settings) {
	setupEnvTestAndRun(t, globalSettings, func(t *testing.T, ctx context.Context, kdbg *krt.DebugHandler, client istiokube.CLIClient, xdsPort int) {
		// list all yamls in test data
		files, err := os.ReadDir(scenarioDir)
		if err != nil {
			t.Fatalf("failed to read dir: %v", err)
		}
		for _, f := range files {
			// run tests with the yaml files (agentgateway dumps json output)
			if strings.HasSuffix(f.Name(), ".yaml") {
				if os.Getenv("TEST_PREFIX") != "" && !strings.HasPrefix(f.Name(), os.Getenv("TEST_PREFIX")) {
					continue
				}
				fullpath := filepath.Join(scenarioDir, f.Name())
				t.Run(strings.TrimSuffix(f.Name(), ".yaml"), func(t *testing.T) {
					writer.set(t)
					t.Cleanup(func() {
						writer.set(nil)
					})
					testAgentgatewayScenario(t, ctx, kdbg, client, xdsPort, fullpath)
				})
			}
		}
	})
}

func testAgentgatewayScenario(
	t *testing.T,
	ctx context.Context,
	kdbg *krt.DebugHandler,
	client istiokube.CLIClient,
	xdsPort int,
	f string,
) {
	fext := filepath.Ext(f)
	fpre := strings.TrimSuffix(f, fext)
	t.Logf("running agent gateway scenario for test file: %s", f)

	// read the out file
	fout := fpre + "-out.json"
	write := false
	_, err := os.ReadFile(fout)
	// if not exist
	if os.IsNotExist(err) {
		write = true
		err = nil
	}
	if os.Getenv("REFRESH_GOLDEN") == "true" {
		write = true
	}
	if err != nil {
		t.Fatalf("failed to read file %s: %v", fout, err)
	}

	const gwname = "http-gw-for-test"
	testgwname := "http-" + filepath.Base(fpre)
	testyamlbytes, err := os.ReadFile(f)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	// change the gw name, so we could potentially run multiple tests in parallel (though currently
	// it has other issues, so we don't run them in parallel)
	testyaml := strings.ReplaceAll(string(testyamlbytes), gwname, testgwname)

	yamlfile := filepath.Join(t.TempDir(), "test.yaml")
	os.WriteFile(yamlfile, []byte(testyaml), 0o600)

	err = client.ApplyYAMLFiles("", yamlfile)

	t.Cleanup(func() {
		// always delete yamls, even if there was an error applying them; to prevent test pollution.
		err := client.DeleteYAMLFiles("", yamlfile)
		if err != nil {
			t.Fatalf("failed to delete yaml: %v", err)
		}
		t.Log("deleted yamls", t.Name())
	})

	if err != nil {
		t.Fatalf("failed to apply yaml: %v", err)
	}
	t.Log("applied yamls", t.Name())

	// wait at least a second before the first check
	// to give the CP time to process
	time.Sleep(time.Second)

	t.Cleanup(func() {
		if t.Failed() {
			logKrtState(t, fmt.Sprintf("krt state for failed test: %s", t.Name()), kdbg)
		} else if os.Getenv("KGW_DUMP_KRT_ON_SUCCESS") == "true" {
			logKrtState(t, fmt.Sprintf("krt state for successful test: %s", t.Name()), kdbg)
		}
	})

	// Use retry to wait for the agent gateway to be ready
	retry.UntilSuccessOrFail(t, func() error {
		dumper := newAgentgatewayXdsDumper(t, ctx, xdsPort, testgwname, "gwtest")
		defer dumper.Close()
		dump := dumper.DumpAgentgateway(t, ctx)
		if len(dump.Resources) == 0 {
			return fmt.Errorf("timed out waiting for agent gateway resources")
		}

		if write {
			t.Logf("writing out file")
			// Use proto dump instead of manual YAML writing
			dumpProtoToJSON(t, dump, fpre)
			return fmt.Errorf("wrote out file - nothing to test")
		}

		// Output the config dump
		t.Logf("Agent Gateway Config Dump for %s:", testgwname)
		t.Logf("Total resources: %d", len(dump.Resources))

		// Count different types of resources
		var bindCount, listenerCount, routeCount, tcpCount, policyCount, worklodCount, serviceCount int
		for _, resource := range dump.Resources {
			switch resource.GetKind().(type) {
			case *api.Resource_Bind:
				bindCount++
				t.Logf("AgwBind resource: %+v", resource.GetBind())
			case *api.Resource_Listener:
				listenerCount++
				t.Logf("Listener resource: %+v", resource.GetListener())
			case *api.Resource_Route:
				routeCount++
				t.Logf("Route resource: %+v", resource.GetRoute())
			case *api.Resource_TcpRoute:
				tcpCount++
				t.Logf("TcpRoute resource: %+v", resource.GetTcpRoute())
			case *api.Resource_Policy:
				policyCount++
				t.Logf("Policy resource: %+v", resource.GetPolicy())
			}
		}
		t.Logf("Resource counts - Binds: %d, Listeners: %d, Routes: %d, TcpRoutes: %d, Policy: %d", bindCount, listenerCount, routeCount, tcpCount, policyCount)

		for _, resource := range dump.Addresses {
			switch resource.GetType().(type) {
			case *api.Address_Workload:
				worklodCount++
				t.Logf("workload resource: %+v", resource.GetWorkload())
			case *api.Address_Service:
				serviceCount++
				t.Logf("service resource: %+v", resource.GetService())
			}
		}
		t.Logf("Address counts - Workload: %d, Service: %d", worklodCount, serviceCount)

		if !write {
			// Read expected values from out.json file and compare
			expectedDump, err := readExpectedDump(t, fout)
			t.Logf("expected dump: %+v", expectedDump)
			if err != nil {
				return fmt.Errorf("failed to read expected dump from %s: %v", fout, err)
			}

			// Compare actual vs expected dump
			if err := compareDumps(dump, expectedDump); err != nil {
				t.Logf("actual dump: %+v", dump)
				t.Logf("comparison failed: %v", err)
				return fmt.Errorf("dump comparison failed: %v", err)
			}
		}

		return nil
	}, retry.Converge(2), retry.BackoffDelay(2*time.Second), retry.Timeout(10*time.Second))

	t.Logf("%s finished", t.Name())
}

// dumpProtoToJSON dumps the agentgateway resources to JSON format
func dumpProtoToJSON(t *testing.T, dump agentGwDump, fpre string) {
	jsonFile := fpre + "-out.json"

	// Sort resources and addresses for consistent ordering
	sortedResources := make([]*api.Resource, len(dump.Resources))
	copy(sortedResources, dump.Resources)
	sortResources(sortedResources)

	sortedAddresses := make([]*api.Address, len(dump.Addresses))
	copy(sortedAddresses, dump.Addresses)
	sortAddresses(sortedAddresses)

	// Create a structured dump map
	dumpMap := map[string]interface{}{
		"resources": sortedResources,
		"addresses": sortedAddresses,
	}

	// Marshal to JSON using standard JSON marshaling to maintain consistency
	jsonData, err := json.MarshalIndent(dumpMap, "", "  ")
	if err != nil {
		t.Logf("failed to marshal to JSON: %v", err)
		return
	}

	err = os.WriteFile(jsonFile, jsonData, 0o600)
	if err != nil {
		t.Logf("failed to write JSON file: %v", err)
		return
	}

	t.Logf("wrote JSON dump to: %s", jsonFile)
}

// sortResources sorts resources by type and key for consistent ordering
func sortResources(resources []*api.Resource) {
	sort.Slice(resources, func(i, j int) bool {
		// First sort by resource type
		typeI := getResourceType(resources[i])
		typeJ := getResourceType(resources[j])
		if typeI != typeJ {
			return typeI < typeJ
		}

		// Then sort by key within the same type
		keyI := getResourceKey(resources[i])
		keyJ := getResourceKey(resources[j])
		return keyI < keyJ
	})
}

// sortAddresses sorts addresses by type and identifier for consistent ordering
func sortAddresses(addresses []*api.Address) {
	sort.Slice(addresses, func(i, j int) bool {
		// First sort by address type
		typeI := getAddressType(addresses[i])
		typeJ := getAddressType(addresses[j])
		if typeI != typeJ {
			return typeI < typeJ
		}

		// Then sort by identifier within the same type
		idI := getAddressIdentifier(addresses[i])
		idJ := getAddressIdentifier(addresses[j])
		return idI < idJ
	})
}

// getResourceType returns a string representation of the resource type for sorting
func getResourceType(resource *api.Resource) string {
	switch resource.GetKind().(type) {
	case *api.Resource_Bind:
		return "bind"
	case *api.Resource_Listener:
		return "listener"
	case *api.Resource_Route:
		return "route"
	case *api.Resource_TcpRoute:
		return "tcproute"
	case *api.Resource_Policy:
		return "policy"
	default:
		return "unknown"
	}
}

// getResourceKey returns the key for a resource for sorting
func getResourceKey(resource *api.Resource) string {
	switch x := resource.GetKind().(type) {
	case *api.Resource_Bind:
		return x.Bind.GetKey()
	case *api.Resource_Listener:
		return x.Listener.GetKey()
	case *api.Resource_Route:
		return x.Route.GetKey()
	case *api.Resource_TcpRoute:
		return x.TcpRoute.GetKey()
	case *api.Resource_Policy:
		return x.Policy.GetName()
	default:
		return ""
	}
}

// getAddressType returns a string representation of the address type for sorting
func getAddressType(address *api.Address) string {
	switch address.GetType().(type) {
	case *api.Address_Workload:
		return "workload"
	case *api.Address_Service:
		return "service"
	default:
		return "unknown"
	}
}

// getAddressIdentifier returns an identifier for an address for sorting
func getAddressIdentifier(address *api.Address) string {
	switch x := address.GetType().(type) {
	case *api.Address_Workload:
		return x.Workload.GetName()
	case *api.Address_Service:
		return x.Service.GetHostname()
	default:
		return ""
	}
}

// readExpectedDump reads and parses the expected dump from a JSON file
func readExpectedDump(t *testing.T, filename string) (agentGwDump, error) {
	var dump agentGwDump

	data, err := os.ReadFile(filename)
	if err != nil {
		return dump, fmt.Errorf("failed to read file: %v", err)
	}

	// Parse the JSON structure using a custom approach that matches the actual JSON format
	var jsonData map[string]interface{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return dump, fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	// Create unmarshaler for protobuf types
	unmarshaler := protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}

	// Parse resources
	if resourcesData, ok := jsonData["resources"].([]interface{}); ok {
		for _, r := range resourcesData {
			if resourceMap, ok := r.(map[string]interface{}); ok {
				resource := &api.Resource{}

				// Parse Kind field
				if kindData, ok := resourceMap["Kind"].(map[string]interface{}); ok {
					if bindData, ok := kindData["Bind"].(map[string]interface{}); ok {
						bindJSON, err := json.Marshal(bindData)
						if err != nil {
							t.Logf("failed to marshal bind data: %v", err)
							continue
						}
						bind := &api.Bind{}
						if err := unmarshaler.Unmarshal(bindJSON, bind); err != nil {
							t.Logf("failed to unmarshal bind: %v", err)
							continue
						}
						resource.Kind = &api.Resource_Bind{Bind: bind}
					} else if listenerData, ok := kindData["Listener"].(map[string]interface{}); ok {
						listenerJSON, err := json.Marshal(listenerData)
						if err != nil {
							t.Logf("failed to marshal listener data: %v", err)
							continue
						}
						listener := &api.Listener{}
						if err := unmarshaler.Unmarshal(listenerJSON, listener); err != nil {
							t.Logf("failed to unmarshal listener: %v", err)
							continue
						}
						resource.Kind = &api.Resource_Listener{Listener: listener}
					} else if routeData, ok := kindData["Route"].(map[string]interface{}); ok {
						routeJSON, err := json.Marshal(routeData)
						if err != nil {
							t.Logf("failed to marshal route data: %v", err)
							continue
						}
						route := &api.Route{}
						if err := unmarshaler.Unmarshal(routeJSON, route); err != nil {
							t.Logf("failed to unmarshal route: %v", err)
							continue
						}
						resource.Kind = &api.Resource_Route{Route: route}
					} else if tcprouteData, ok := kindData["TcpRoute"].(map[string]interface{}); ok {
						tcprouteJSON, err := json.Marshal(tcprouteData)
						if err != nil {
							t.Logf("failed to marshal tcp route data: %v", err)
							continue
						}
						tcproute := &api.TCPRoute{}
						if err := unmarshaler.Unmarshal(tcprouteJSON, tcproute); err != nil {
							t.Logf("failed to unmarshal route: %v", err)
							continue
						}
						resource.Kind = &api.Resource_TcpRoute{TcpRoute: tcproute}
					} else if policyData, ok := kindData["Policy"].(map[string]interface{}); ok {
						policyJSON, err := json.Marshal(policyData)
						if err != nil {
							t.Logf("failed to marshal policy data: %v", err)
							continue
						}
						policy := &api.Policy{}
						if err := unmarshaler.Unmarshal(policyJSON, policy); err != nil {
							t.Logf("failed to unmarshal policy: %v", err)
							continue
						}
						resource.Kind = &api.Resource_Policy{Policy: policy}
					}
				}

				if resource.Kind != nil {
					dump.Resources = append(dump.Resources, resource)
				}
			}
		}
	}

	// Parse addresses
	if addressesData, ok := jsonData["addresses"].([]interface{}); ok {
		for _, a := range addressesData {
			if addressMap, ok := a.(map[string]interface{}); ok {
				address := &api.Address{}

				// Parse Type field
				if typeData, ok := addressMap["Type"].(map[string]interface{}); ok {
					if serviceData, ok := typeData["Service"].(map[string]interface{}); ok {
						serviceJSON, err := json.Marshal(serviceData)
						if err != nil {
							t.Logf("failed to marshal service data: %v", err)
							continue
						}
						service := &api.Service{}
						if err := unmarshaler.Unmarshal(serviceJSON, service); err != nil {
							t.Logf("failed to unmarshal service: %v", err)
							continue
						}
						address.Type = &api.Address_Service{Service: service}
					} else if workloadData, ok := typeData["Workload"].(map[string]interface{}); ok {
						workloadJSON, err := json.Marshal(workloadData)
						if err != nil {
							t.Logf("failed to marshal workload data: %v", err)
							continue
						}
						workload := &api.Workload{}
						if err := unmarshaler.Unmarshal(workloadJSON, workload); err != nil {
							t.Logf("failed to unmarshal workload: %v", err)
							continue
						}
						address.Type = &api.Address_Workload{Workload: workload}
					}
				}

				if address.Type != nil {
					dump.Addresses = append(dump.Addresses, address)
				}
			}
		}
	}

	return dump, nil
}

// compareDumps compares two dumps and returns an error if they differ
func compareDumps(actual, expected agentGwDump) error {
	// Sort both dumps for comparison
	sortedActual := agentGwDump{
		Resources: make([]*api.Resource, len(actual.Resources)),
		Addresses: make([]*api.Address, len(actual.Addresses)),
	}
	copy(sortedActual.Resources, actual.Resources)
	copy(sortedActual.Addresses, actual.Addresses)
	sortResources(sortedActual.Resources)
	sortAddresses(sortedActual.Addresses)

	sortedExpected := agentGwDump{
		Resources: make([]*api.Resource, len(expected.Resources)),
		Addresses: make([]*api.Address, len(expected.Addresses)),
	}
	copy(sortedExpected.Resources, expected.Resources)
	copy(sortedExpected.Addresses, expected.Addresses)
	sortResources(sortedExpected.Resources)
	sortAddresses(sortedExpected.Addresses)

	// Compare resources
	if len(sortedActual.Resources) != len(sortedExpected.Resources) {
		return fmt.Errorf("resource count mismatch: actual=%d, expected=%d", len(sortedActual.Resources), len(sortedExpected.Resources))
	}

	for i, actualRes := range sortedActual.Resources {
		expectedRes := sortedExpected.Resources[i]
		if getResourceType(actualRes) != getResourceType(expectedRes) {
			return fmt.Errorf("resource type mismatch at index %d: actual=%s, expected=%s", i, getResourceType(actualRes), getResourceType(expectedRes))
		}
		if getResourceKey(actualRes) != getResourceKey(expectedRes) {
			return fmt.Errorf("resource key mismatch at index %d: actual=%s, expected=%s", i, getResourceKey(actualRes), getResourceKey(expectedRes))
		}
	}

	// Compare addresses
	if len(sortedActual.Addresses) != len(sortedExpected.Addresses) {
		return fmt.Errorf("address count mismatch: actual=%d, expected=%d", len(sortedActual.Addresses), len(sortedExpected.Addresses))
	}

	for i, actualAddr := range sortedActual.Addresses {
		expectedAddr := sortedExpected.Addresses[i]
		if getAddressType(actualAddr) != getAddressType(expectedAddr) {
			return fmt.Errorf("address type mismatch at index %d: actual=%s, expected=%s", i, getAddressType(actualAddr), getAddressType(expectedAddr))
		}
		if getAddressIdentifier(actualAddr) != getAddressIdentifier(expectedAddr) {
			return fmt.Errorf("address identifier mismatch at index %d: actual=%s, expected=%s", i, getAddressIdentifier(actualAddr), getAddressIdentifier(expectedAddr))
		}
	}

	return nil
}

func newAgentgatewayXdsDumper(t *testing.T, ctx context.Context, xdsPort int, gwname, gwnamespace string) xdsDumper {
	conn, err := grpc.NewClient(fmt.Sprintf("localhost:%d", xdsPort),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithIdleTimeout(time.Second*10),
	)
	if err != nil {
		t.Fatalf("failed to connect to xds server: %v", err)
	}

	d := xdsDumper{
		conn: conn,
		dr: &envoy_service_discovery_v3.DiscoveryRequest{
			Node: &envoycorev3.Node{
				Id: "gateway.gwtest",
				Metadata: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"role": structpb.NewStringValue(fmt.Sprintf("%s~%s", gwnamespace, gwname)),
					},
				},
			},
		},
	}

	ads := envoy_service_discovery_v3.NewAggregatedDiscoveryServiceClient(d.conn)
	ctx, cancel := context.WithTimeout(ctx, time.Second*30) // long timeout - just in case. we should never reach it.
	adsClient, err := ads.StreamAggregatedResources(ctx)
	if err != nil {
		t.Fatalf("failed to get ads client: %v", err)
	}
	d.adsClient = adsClient
	d.cancel = cancel

	return d
}

type agentGwDump struct {
	Resources []*api.Resource
	Addresses []*api.Address
}

func (x xdsDumper) DumpAgentgateway(t *testing.T, ctx context.Context) agentGwDump {
	// get resources
	resources := x.GetResources(t, ctx)
	addresses := x.GetAddress(t, ctx)

	return agentGwDump{
		Resources: resources,
		Addresses: addresses,
	}
}

func (x xdsDumper) GetResources(t *testing.T, ctx context.Context) []*api.Resource {
	dr := proto.Clone(x.dr).(*envoy_service_discovery_v3.DiscoveryRequest)
	dr.TypeUrl = translator.TargetTypeResourceUrl
	x.adsClient.Send(dr)
	var resources []*api.Resource
	// run this in parallel with a 5s timeout
	done := make(chan struct{})
	go func() {
		defer close(done)
		sent := 1
		for i := 0; i < sent; i++ {
			dresp, err := x.adsClient.Recv()
			if err != nil {
				t.Errorf("failed to get response from xds server: %v", err)
			}
			t.Logf("got response: %s len: %d", dresp.GetTypeUrl(), len(dresp.GetResources()))
			if dresp.GetTypeUrl() == translator.TargetTypeResourceUrl {
				for _, anyResource := range dresp.GetResources() {
					var resource api.Resource
					if err := anyResource.UnmarshalTo(&resource); err != nil {
						t.Errorf("failed to unmarshal resource: %v", err)
					}
					resources = append(resources, &resource)
				}
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		// don't fatal yet as we want to dump the state while still connected
		t.Error("timed out waiting for resources for agent gateway xds dump")
		return nil
	}
	if len(resources) == 0 {
		t.Error("no resources found")
		return nil
	}
	t.Logf("xds: found %d resources", len(resources))
	return resources
}

func (x xdsDumper) GetAddress(t *testing.T, ctx context.Context) []*api.Address {
	dr := proto.Clone(x.dr).(*envoy_service_discovery_v3.DiscoveryRequest)
	dr.TypeUrl = translator.TargetTypeAddressUrl
	x.adsClient.Send(dr)
	var address []*api.Address
	// run this in parallel with a 5s timeout
	done := make(chan struct{})
	go func() {
		defer close(done)
		sent := 1
		for i := 0; i < sent; i++ {
			dresp, err := x.adsClient.Recv()
			if err != nil {
				t.Errorf("failed to get response from xds server: %v", err)
			}
			t.Logf("got address response: %s len: %d", dresp.GetTypeUrl(), len(dresp.GetResources()))
			if dresp.GetTypeUrl() == translator.TargetTypeAddressUrl {
				for _, anyResource := range dresp.GetResources() {
					var resource api.Address
					if err := anyResource.UnmarshalTo(&resource); err != nil {
						t.Errorf("failed to unmarshal resource: %v", err)
					}
					address = append(address, &resource)
				}
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		// don't fatal yet as we want to dump the state while still connected
		t.Error("timed out waiting for address resources for agent gateway xds dump")
		return nil
	}
	if len(address) == 0 {
		t.Error("no address resources found")
		return nil
	}
	t.Logf("xds: found %d address resources", len(address))
	return address
}
