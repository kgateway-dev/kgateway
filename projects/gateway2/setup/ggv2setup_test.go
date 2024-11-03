package setup_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	envoycluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_service_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	envoy_service_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	envoy_service_route_v3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	"github.com/golang/protobuf/jsonpb"

	discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	"github.com/go-logr/zapr"
	"github.com/solo-io/gloo/projects/gateway2/extensions"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	ggv2setup "github.com/solo-io/gloo/projects/gateway2/setup"
	ggv2utils "github.com/solo-io/gloo/projects/gateway2/utils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	gloosetup "github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/slices"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

func getAssetsDir(t *testing.T) string {
	assets := ""
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// set default if not user provided
		out, err := exec.Command("sh", "-c", "make -sC $(dirname $(go env GOMOD))/projects/gateway2 envtest-path").CombinedOutput()
		t.Log("out:", string(out))
		if err != nil {
			t.Fatalf("failed to get assets dir: %v", err)
		}
		assets = strings.TrimSpace(string(out))
	}
	return assets
}

// testingWriter is a WriteSyncer that writes logs to testing.T.
type testingWriter struct {
	t *testing.T
}

func (w *testingWriter) Write(p []byte) (n int, err error) {
	w.t.Log(string(p)) // Write the log to testing.T
	return len(p), nil
}

func (w *testingWriter) Sync() error {
	return nil
}

// NewTestLogger creates a zap.Logger that writes to testing.T.
func NewTestLogger(t *testing.T) *zap.Logger {
	writer := &testingWriter{t: t}

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
		zapcore.AddSync(writer),
		zapcore.DebugLevel, // Adjust log level as needed
	)

	return zap.New(core, zap.AddCaller())
}

func TestSomething(t *testing.T) {
	os.Setenv("POD_NAMESPACE", "default")
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "crds"),
			filepath.Join("..", "..", "..", "install", "helm", "gloo", "crds"),
			filepath.Join("testdata", "istiocrds"),
		},
		ErrorIfCRDPathMissing: true,
		// set assets dir so we can run without the makefile
		BinaryAssetsDirectory: getAssetsDir(t),
		// web hook to add cluster ips to services
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Cleanup(cancel)
	logger := NewTestLogger(t)
	defer logger.Sync()
	//	t.Cleanup(func() { _ = logger.Sync() })
	log.SetLogger(zapr.NewLogger(logger))

	ctx = contextutils.WithExistingLogger(ctx, logger.Sugar())

	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("failed to get assets dir: %v", err)
	}
	client, err := istiokube.NewCLIClient(istiokube.NewClientConfigForRestConfig(cfg))
	if err != nil {
		t.Fatalf("failed to get init kube client: %v", err)
	}
	istiokube.EnableCrdWatcher(client)

	// apply yaml to the cluster

	f := "scenario1.yaml"
	fext := filepath.Ext(f)
	fpre := strings.TrimSuffix(f, fext)
	fout := fpre + "-out" + fext
	// read the out file
	write := false
	ya, err := os.ReadFile("testdata/" + fout)
	// if not exist
	if os.IsNotExist(err) {
		write = true
		err = nil
	}
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	var expectedXdsDump xdsDump
	err = expectedXdsDump.FromYaml(ya)
	if err != nil {
		t.Fatalf("failed to read yaml: %v", err)
	}

	err = client.ApplyYAMLFiles("default", "testdata/"+f)
	if err != nil {
		t.Fatalf("failed to apply yaml: %v", err)
	}

	// get settings:
	uniqueClientCallbacks, builder := krtcollections.NewUniquelyConnectedClients()
	setupOpts := bootstrap.NewSetupOpts(xds.NewAdsSnapshotCache(ctx), uniqueClientCallbacks)
	addr := &net.TCPAddr{
		IP:   net.IPv4zero,
		Port: int(0),
	}
	controlPlane := gloosetup.NewControlPlane(ctx, setupOpts.Cache, grpc.NewServer(), addr, bootstrap.KubernetesControlPlaneConfig{}, uniqueClientCallbacks, true)
	xds.SetupEnvoyXds(controlPlane.GrpcServer, controlPlane.XDSServer, controlPlane.SnapshotCache)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cant listen %v", err)
	}
	xdsPort := lis.Addr().(*net.TCPAddr).Port
	setupOpts.SetXdsAddress("localhost", int32(xdsPort))

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-controlPlane.GrpcService.Ctx.Done()
		controlPlane.GrpcServer.Stop()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := controlPlane.GrpcServer.Serve(lis); err != nil {
			panic("xds grpc server failed to start")
		}
	}()

	setupOpts.ProxyReconcileQueue = ggv2utils.NewAsyncQueue[gloov1.ProxyList]()
	wg.Add(1)
	go func() {
		defer wg.Done()
		ggv2setup.StartGGv2WithConfig(ctx, setupOpts, cfg, builder, extensions.NewK8sGatewayExtensions,
			registry.GetPluginRegistryFactory,
			types.NamespacedName{Name: "default", Namespace: "default"},
		)
	}()
	// now create a gateway, node ,pod,

	// now pretend to be the proxy

	//	conn, err := grpc.NewClient(fmt.Sprintf("localhost:%d", xdsPort), grpc.WithTransportCredentials(insecure.NewCredentials()))
	//	if err != nil {
	//		t.Fatalf("failed to connect to xds server: %v", err)
	//	}

	//give ggv2 time to initialize so we don't get
	// "ggv2 not initialized" error
	time.Sleep(time.Second)

	dump := getXdsDump(t, ctx, xdsPort)
	cla := getreviews(dump.Endpoints)
	t.Log("got cla", cla)

	if write {
		t.Logf("writing out file")
		// serialize xdsDump to yaml
		d, err := dump.ToYaml()
		if err != nil {
			t.Fatalf("failed to serialize xdsDump: %v", err)
		}
		os.WriteFile("testdata/"+fout, d, 0644)
		t.Fatal("wrote out file - nothing to test")
		return
	}
	expectedXdsDump.Compare(t, dump)

	if len(cla.Endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %v", len(cla.Endpoints))
	}
}

func getreviews(clas []*envoyendpoint.ClusterLoadAssignment) *envoyendpoint.ClusterLoadAssignment {
	for _, cla := range clas {
		if strings.Contains(cla.ClusterName, "reviews") {
			return cla
		}
	}
	return nil
}

func getXdsDump(t *testing.T, ctx context.Context, xdsPort int) xdsDump {
	conn, err := grpc.NewClient(fmt.Sprintf("localhost:%d", xdsPort), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to connect to xds server: %v", err)
	}
	defer conn.Close()

	clusters := getclusters(t, ctx, conn)
	clusterServiceNames := slices.MapFilter(clusters, func(c *envoycluster.Cluster) *string {
		if c.GetEdsClusterConfig() != nil {
			if c.GetEdsClusterConfig().GetServiceName() != "" {
				s := c.GetEdsClusterConfig().GetServiceName()
				return &s
			}
			return &c.Name
		}
		return nil
	})

	listeners := getlisteners(t, ctx, conn)
	var routenames []string
	for _, l := range listeners {
		routenames = append(routenames, getroutesnames(l)...)
	}
	return xdsDump{
		Clusters:  clusters,
		Listeners: listeners,
		Endpoints: getendpoints(t, ctx, conn, clusterServiceNames),
		Routes:    getroutes(t, ctx, conn, routenames),
	}
}

type xdsDump struct {
	Clusters  []*envoycluster.Cluster
	Listeners []*envoylistener.Listener
	Endpoints []*envoyendpoint.ClusterLoadAssignment
	Routes    []*envoy_config_route_v3.RouteConfiguration
}

func (x *xdsDump) Compare(t *testing.T, other xdsDump) {
	if len(x.Clusters) != len(other.Clusters) {
		t.Fatalf("expected %v clusters, got %v", len(other.Clusters), len(x.Clusters))
	}

	if len(x.Listeners) != len(other.Listeners) {
		t.Fatalf("expected %v listeners, got %v", len(other.Listeners), len(x.Listeners))
	}
	if len(x.Endpoints) != len(other.Endpoints) {
		t.Fatalf("expected %v endpoints, got %v", len(other.Endpoints), len(x.Endpoints))
	}
	if len(x.Routes) != len(other.Routes) {
		t.Fatalf("expected %v routes, got %v", len(other.Routes), len(x.Routes))
	}

	clusterset := map[string]*envoycluster.Cluster{}
	for _, c := range x.Clusters {
		clusterset[c.Name] = c
	}
	for _, c := range other.Clusters {
		otherc := clusterset[c.Name]
		if otherc == nil {
			t.Fatalf("cluster %v not found", c.Name)
		}
		if !proto.Equal(c, otherc) {
			t.Fatalf("cluster %v not equal", c.Name)
		}
	}
	listenerset := map[string]*envoylistener.Listener{}
	for _, c := range x.Listeners {
		listenerset[c.Name] = c
	}
	for _, c := range other.Listeners {
		otherc := listenerset[c.Name]
		if otherc == nil {
			t.Fatalf("listener %v not found", c.Name)
		}
		if !proto.Equal(c, otherc) {
			t.Fatalf("listener %v not equal", c.Name)
		}
	}
	routeset := map[string]*envoy_config_route_v3.RouteConfiguration{}
	for _, c := range x.Routes {
		routeset[c.Name] = c
	}
	for _, c := range other.Routes {
		otherc := routeset[c.Name]
		if otherc == nil {
			t.Fatalf("route %v not found", c.Name)
		}
		if !proto.Equal(c, otherc) {
			t.Fatalf("route %v not equal", c.Name)
		}
	}

	// note: with endpoints we might have order issues; so may need custom equals
	epset := map[string]*envoyendpoint.ClusterLoadAssignment{}
	for _, c := range x.Endpoints {
		epset[c.ClusterName] = c
	}
	for _, c := range other.Endpoints {
		otherc := epset[c.ClusterName]
		if otherc == nil {
			t.Fatalf("ep %v not found", c.ClusterName)
		}
		if !proto.Equal(c, otherc) {
			t.Fatalf("ep %v not equal", c.ClusterName)
		}
	}

}

func (x *xdsDump) FromYaml(ya []byte) error {
	var ju jsonpb.Unmarshaler

	ya, err := yaml.YAMLToJSON(ya)
	if err != nil {
		return err
	}

	jsonM := map[string][]any{}
	err = json.Unmarshal(ya, &jsonM)
	if err != nil {
		return err
	}
	for _, c := range jsonM["clusters"] {
		jb, err := json.Marshal(c)
		if err != nil {
			return err
		}
		var cluster envoycluster.Cluster
		ju.Unmarshal(bytes.NewReader(jb), &cluster)
		x.Clusters = append(x.Clusters, &cluster)
	}
	for _, c := range jsonM["endpoints"] {
		jb, err := json.Marshal(c)
		if err != nil {
			return err
		}
		var r envoyendpoint.ClusterLoadAssignment
		ju.Unmarshal(bytes.NewReader(jb), &r)
		x.Endpoints = append(x.Endpoints, &r)
	}
	for _, c := range jsonM["listeners"] {
		jb, err := json.Marshal(c)
		if err != nil {
			return err
		}
		var r envoylistener.Listener
		ju.Unmarshal(bytes.NewReader(jb), &r)
		x.Listeners = append(x.Listeners, &r)
	}
	for _, c := range jsonM["routes"] {
		jb, err := json.Marshal(c)
		if err != nil {
			return err
		}
		var r envoy_config_route_v3.RouteConfiguration
		ju.Unmarshal(bytes.NewReader(jb), &r)
		x.Routes = append(x.Routes, &r)
	}
	return nil
}

func (x *xdsDump) ToYaml() ([]byte, error) {
	var j jsonpb.Marshaler
	jsonM := map[string][]any{}
	for _, c := range x.Clusters {
		s, err := j.MarshalToString(c)
		if err != nil {
			return nil, err
		}
		var roundtrip any
		err = json.Unmarshal([]byte(s), &roundtrip)
		if err != nil {
			return nil, err
		}
		jsonM["clusters"] = append(jsonM["clusters"], roundtrip)
	}
	for _, c := range x.Listeners {
		s, err := j.MarshalToString(c)
		if err != nil {
			return nil, err
		}
		var roundtrip any
		err = json.Unmarshal([]byte(s), &roundtrip)
		if err != nil {
			return nil, err
		}
		jsonM["listeners"] = append(jsonM["listeners"], roundtrip)
	}
	for _, c := range x.Endpoints {
		s, err := j.MarshalToString(c)
		if err != nil {
			return nil, err
		}
		var roundtrip any
		err = json.Unmarshal([]byte(s), &roundtrip)
		if err != nil {
			return nil, err
		}
		jsonM["endpoints"] = append(jsonM["endpoints"], roundtrip)
	}
	for _, c := range x.Routes {
		s, err := j.MarshalToString(c)
		if err != nil {
			return nil, err
		}
		var roundtrip any
		err = json.Unmarshal([]byte(s), &roundtrip)
		if err != nil {
			return nil, err
		}
		jsonM["routes"] = append(jsonM["routes"], roundtrip)
	}

	bytes, err := json.Marshal(jsonM)
	if err != nil {
		return nil, err
	}

	ya, err := yaml.JSONToYAML(bytes)
	if err != nil {
		return nil, err
	}
	return ya, nil

}

func getclusters(t *testing.T, ctx context.Context, conn *grpc.ClientConn) []*envoycluster.Cluster {

	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	dr := &discovery_v3.DiscoveryRequest{Node: &envoycore.Node{
		Id: "reviews-1.default",
		Metadata: &structpb.Struct{
			Fields: map[string]*structpb.Value{"role": {Kind: &structpb.Value_StringValue{StringValue: fmt.Sprintf("gloo-kube-gateway-api~%s~%s-%s", "default", "default", "http")}}}},
	}}

	cds := envoy_service_cluster_v3.NewClusterDiscoveryServiceClient(conn)

	//give ggv2 time to initialize so we don't get
	// "ggv2 not initialized" error

	epcli, err := cds.StreamClusters(ctx)
	if err != nil {
		t.Fatalf("failed to get eds client: %v", err)
	}
	epcli.Send(dr)
	dresp, err := epcli.Recv()
	if err != nil {
		t.Fatalf("failed to get response from xds server: %v", err)
	}
	var clusters []*envoycluster.Cluster
	for _, anyCluster := range dresp.GetResources() {

		var cluster envoycluster.Cluster
		if err := anyCluster.UnmarshalTo(&cluster); err != nil {
			t.Fatalf("failed to unmarshal cluster: %v", err)
		}
		clusters = append(clusters, &cluster)
	}
	return clusters
}

func getroutesnames(l *envoylistener.Listener) []string {
	var routes []string
	for _, fc := range l.GetFilterChains() {
		for _, filter := range fc.GetFilters() {
			suffix := string((&envoyhttp.HttpConnectionManager{}).ProtoReflect().Descriptor().FullName())
			if strings.HasSuffix(filter.GetTypedConfig().GetTypeUrl(), suffix) {
				var hcm envoyhttp.HttpConnectionManager
				switch config := filter.GetConfigType().(type) {
				case *envoylistener.Filter_TypedConfig:
					if err := config.TypedConfig.UnmarshalTo(&hcm); err == nil {
						rds := hcm.GetRds().GetRouteConfigName()
						if rds != "" {
							routes = append(routes, rds)
						}
					}
				}
			}
		}
	}
	return routes
}

func getlisteners(t *testing.T, ctx context.Context, conn *grpc.ClientConn) []*envoylistener.Listener {

	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	dr := &discovery_v3.DiscoveryRequest{Node: &envoycore.Node{
		Id: "reviews-1.default",
		Metadata: &structpb.Struct{
			Fields: map[string]*structpb.Value{"role": {Kind: &structpb.Value_StringValue{StringValue: fmt.Sprintf("gloo-kube-gateway-api~%s~%s-%s", "default", "default", "http")}}}},
	}}

	ds := envoy_service_listener_v3.NewListenerDiscoveryServiceClient(conn)

	//give ggv2 time to initialize so we don't get
	// "ggv2 not initialized" error

	epcli, err := ds.StreamListeners(ctx)
	if err != nil {
		t.Fatalf("failed to get eds client: %v", err)
	}
	epcli.Send(dr)
	dresp, err := epcli.Recv()
	if err != nil {
		t.Fatalf("failed to get response from xds server: %v", err)
	}
	var resources []*envoylistener.Listener
	for _, anyResource := range dresp.GetResources() {

		var resource envoylistener.Listener
		if err := anyResource.UnmarshalTo(&resource); err != nil {
			t.Fatalf("failed to unmarshal resource: %v", err)
		}
		resources = append(resources, &resource)
	}
	return resources
}

func getendpoints(t *testing.T, ctx context.Context, conn *grpc.ClientConn, clusterServiceNames []string) []*envoyendpoint.ClusterLoadAssignment {

	eds := envoy_service_endpoint_v3.NewEndpointDiscoveryServiceClient(conn)
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	epcli, err := eds.StreamEndpoints(ctx)
	if err != nil {
		t.Fatalf("failed to get eds client: %v", err)
	}
	dr := &discovery_v3.DiscoveryRequest{Node: &envoycore.Node{
		Id: "reviews-1.default",
		Metadata: &structpb.Struct{
			Fields: map[string]*structpb.Value{"role": {Kind: &structpb.Value_StringValue{StringValue: fmt.Sprintf("gloo-kube-gateway-api~%s~%s-%s", "default", "default", "http")}}}},
	},
		ResourceNames: clusterServiceNames,
	}
	epcli.Send(dr)
	dresp, err := epcli.Recv()
	if err != nil {
		t.Fatalf("failed to get response from xds server: %v", err)
	}
	var clas []*envoyendpoint.ClusterLoadAssignment
	for _, anyCluster := range dresp.GetResources() {

		var cla envoyendpoint.ClusterLoadAssignment
		if err := anyCluster.UnmarshalTo(&cla); err != nil {
			t.Fatalf("failed to unmarshal cluster: %v", err)
		}
		clas = append(clas, &cla)
	}
	return clas
}

func getroutes(t *testing.T, ctx context.Context, conn *grpc.ClientConn, rosourceNames []string) []*envoy_config_route_v3.RouteConfiguration {

	eds := envoy_service_route_v3.NewRouteDiscoveryServiceClient(conn)
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	epcli, err := eds.StreamRoutes(ctx)
	if err != nil {
		t.Fatalf("failed to get eds client: %v", err)
	}
	dr := &discovery_v3.DiscoveryRequest{Node: &envoycore.Node{
		Id: "reviews-1.default",
		Metadata: &structpb.Struct{
			Fields: map[string]*structpb.Value{"role": {Kind: &structpb.Value_StringValue{StringValue: fmt.Sprintf("gloo-kube-gateway-api~%s~%s-%s", "default", "default", "http")}}}},
	},
		ResourceNames: rosourceNames,
	}
	epcli.Send(dr)
	dresp, err := epcli.Recv()
	if err != nil {
		t.Fatalf("failed to get response from xds server: %v", err)
	}
	var clas []*envoy_config_route_v3.RouteConfiguration
	for _, anyCluster := range dresp.GetResources() {

		var cla envoy_config_route_v3.RouteConfiguration
		if err := anyCluster.UnmarshalTo(&cla); err != nil {
			t.Fatalf("failed to unmarshal cluster: %v", err)
		}
		clas = append(clas, &cla)
	}
	return clas
}
