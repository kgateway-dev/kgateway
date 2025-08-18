package setup_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/go-logr/zapr"
	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/grpclog"
	jsonpb "google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/structpb"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	istioslices "istio.io/istio/pkg/slices"
	"istio.io/istio/pkg/test/util/retry"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/proxy_syncer"
	"github.com/kgateway-dev/kgateway/v2/pkg/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/envtestutil"
)

func getAssetsDir(t *testing.T) string {
	var assets string
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// set default if not user provided
		out, err := exec.Command("sh", "-c", "make -sC $(dirname $(go env GOMOD)) envtest-path").CombinedOutput()
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
	sync.RWMutex
	t *testing.T
}

func (w *testingWriter) Write(p []byte) (n int, err error) {
	w.RLock()
	t := w.t // Capture the test context under lock
	w.RUnlock()

	// Check if we have a valid test context before trying to log
	// This prevents races when controller goroutines outlive the test
	if t != nil {
		t.Log(string(p)) // Write the log to testing.T
	}
	// Always return success to avoid breaking the logging chain
	return len(p), nil
}

func (w *testingWriter) Sync() error {
	return nil
}

func (w *testingWriter) set(t *testing.T) {
	w.Lock()
	defer w.Unlock()

	w.t = t
}

var (
	writer = &testingWriter{}
	logger = NewTestLogger()
)

// NewTestLogger creates a zap.Logger which can be used to write to *testing.T
// on each test, set the *testing.T on the writer.
func NewTestLogger() *zap.Logger {
	var core zapcore.Core
	// Only log controller-runtime and gRPC logs if LOG_LEVEL=debug, otherwise they are extremely noisy
	level, err := zapcore.ParseLevel(envutils.GetOrDefault("LOG_LEVEL", "error", false))
	if err != nil {
		panic(fmt.Sprintf("failed to parse LOG_LEVEL: %v", err))
	}
	core = zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
		zapcore.AddSync(writer),
		// Adjust log level as needed
		// if a test assertion fails and logs or too noisy, change to zapcore.FatalLevel
		level,
	)

	return zap.New(core, zap.AddCaller())
}

func init() {
	log.SetLogger(zapr.NewLogger(logger))
	// Use GRPC_GO_LOG_SEVERITY_LEVEL and GRPC_GO_LOG_VERBOSITY_LEVEL env vars to control gRPC logging
	if logger.Level() == zapcore.DebugLevel {
		grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(writer, writer, writer, 100))
	}
}

func TestServiceEntry(t *testing.T) {
	st, err := settings.BuildSettings()
	if err != nil {
		t.Fatalf("can't get settings %v", err)
	}
	st.EnableIstioIntegration = true

	// these exercise applying a DR to a ServiceEntry
	runScenario(t, "testdata/serviceentry/dr", st)
}

func TestDestinationRule(t *testing.T) {
	st, err := settings.BuildSettings()
	st.EnableIstioIntegration = true
	if err != nil {
		t.Fatalf("can't get settings %v", err)
	}
	runScenario(t, "testdata/istio_destination_rule", st)
}

func TestTrafficDistribution(t *testing.T) {
	st, err := settings.BuildSettings()
	if err != nil {
		t.Fatalf("can't get settings %v", err)
	}
	st.EnableIstioIntegration = true

	// these exercise applying a DR to a ServiceEntry
	runScenario(t, "testdata/traffic_distribution", st)
}

func TestWithStandardSettings(t *testing.T) {
	st, err := settings.BuildSettings()
	if err != nil {
		t.Fatalf("can't get settings %v", err)
	}
	runScenario(t, "testdata/standard", st)
}

func TestWithIstioAutomtlsSettings(t *testing.T) {
	st, err := settings.BuildSettings()
	st.EnableIstioIntegration = true
	st.EnableIstioAutoMtls = true
	if err != nil {
		t.Fatalf("can't get settings %v", err)
	}
	runScenario(t, "testdata/istio_mtls", st)
}

func TestWithBindIpv6(t *testing.T) {
	st, err := settings.BuildSettings()
	st.ListenerBindIpv6 = true
	if err != nil {
		t.Fatalf("can't get settings %v", err)
	}
	runScenario(t, "testdata/listenerbind/v6", st)
}

func TestWithBindIpv4(t *testing.T) {
	st, err := settings.BuildSettings()
	st.ListenerBindIpv6 = false
	if err != nil {
		t.Fatalf("can't get settings %v", err)
	}
	runScenario(t, "testdata/listenerbind/v4", st)
}

func TestWithAutoDns(t *testing.T) {
	st, err := settings.BuildSettings()
	if err != nil {
		t.Fatalf("can't get settings %v", err)
	}
	st.DnsLookupFamily = settings.DnsLookupFamilyAuto

	runScenario(t, "testdata/autodns", st)
}

func TestWithInferenceAPI(t *testing.T) {
	st, err := settings.BuildSettings()
	if err != nil {
		t.Fatalf("can't get settings %v", err)
	}
	st.EnableInferExt = true
	st.InferExtAutoProvision = true

	runScenario(t, "testdata/inference_api", st)
}

func TestPolicyUpdate(t *testing.T) {
	st, err := settings.BuildSettings()
	if err != nil {
		t.Fatalf("can't get settings %v", err)
	}
	setupEnvTestAndRun(t, st, func(t *testing.T, ctx context.Context, kdbg *krt.DebugHandler, client istiokube.CLIClient, xdsPort int) {
		client.Kube().CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gwtest"}}, metav1.CreateOptions{})

		err = client.ApplyYAMLContents("gwtest", `kind: Gateway
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: http-gw
  namespace: gwtest
spec:
  gatewayClassName: kgateway
  listeners:
  - protocol: HTTP
    port: 8080
    name: http
    allowedRoutes:
      namespaces:
        from: All`, `apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: transformation
  namespace: gwtest
spec:
  transformation:
    response:
      set:
      - name: x-solo-response
        value: '{{ request_header("x-solo-request") }}'
      remove:
      - x-solo-request`, `apiVersion: gateway.networking.k8s.io/v1beta1
kind: HTTPRoute
metadata:
  name: happypath
  namespace: gwtest
spec:
  parentRefs:
    - name: http-gw
  hostnames:
    - "www.example2.com"
  rules:
    - backendRefs:
        - name: kubernetes
          port: 443
      filters:
      - type: ExtensionRef
        extensionRef:
          group: gateway.kgateway.dev
          kind: TrafficPolicy
          name: transformation`)

		time.Sleep(time.Second / 2)

		err = client.ApplyYAMLContents("gwtest", `apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: transformation
  namespace: gwtest
spec:
  transformation:
    response:
      set:
      - name: x-solo-response
        value: '{{ request_header("x-solo-request123") }}'
      remove:
      - x-solo-request321`)

		time.Sleep(time.Second / 2)

		dumper := newXdsDumper(t, ctx, xdsPort, "http-gw")
		t.Cleanup(dumper.Close)
		t.Cleanup(func() {
			if t.Failed() {
				logKrtState(t, fmt.Sprintf("krt state for failed test: %s", t.Name()), kdbg)
			} else if os.Getenv("KGW_DUMP_KRT_ON_SUCCESS") == "true" {
				logKrtState(t, fmt.Sprintf("krt state for successful test: %s", t.Name()), kdbg)
			}
		})

		dump, err := dumper.Dump(t, ctx)
		if err != nil {
			t.Error(err)
		}
		pfc := dump.Routes[0].GetVirtualHosts()[0].GetRoutes()[0].GetTypedPerFilterConfig()
		if len(pfc) != 1 {
			t.Fatalf("expected 1 filter config, got %d", len(pfc))
		}
		if !bytes.Contains(slices.Collect(maps.Values(pfc))[0].Value, []byte("x-solo-request321")) {
			t.Fatalf("expected filter config to contain x-solo-request321")
		}

		t.Logf("%s finished", t.Name())
	})
}

func runScenario(t *testing.T, scenarioDir string, globalSettings *settings.Settings) {
	setupEnvTestAndRun(t, globalSettings, func(t *testing.T, ctx context.Context, kdbg *krt.DebugHandler, client istiokube.CLIClient, xdsPort int) {
		// list all yamls in test data
		files, err := os.ReadDir(scenarioDir)
		if err != nil {
			t.Fatalf("failed to read dir: %v", err)
		}
		for _, f := range files {
			// run tests with the yaml files (but not -out.yaml files)/s
			if strings.HasSuffix(f.Name(), ".yaml") && !strings.HasSuffix(f.Name(), "-out.yaml") {
				if os.Getenv("TEST_PREFIX") != "" && !strings.HasPrefix(f.Name(), os.Getenv("TEST_PREFIX")) {
					continue
				}
				fullpath := filepath.Join(scenarioDir, f.Name())
				t.Run(strings.TrimSuffix(f.Name(), ".yaml"), func(t *testing.T) {
					writer.set(t)
					t.Cleanup(func() {
						writer.set(nil)
					})
					// sadly tests can't run yet in parallel, as kgateway will add all the k8s services as clusters. this means
					// that we get test pollution.
					// once we change it to only include the ones in the proxy, we can re-enable this
					//				t.Parallel()
					testScenario(t, ctx, kdbg, client, xdsPort, fullpath)
				})
			}
		}
	})
}

func setupEnvTestAndRun(t *testing.T, globalSettings *settings.Settings, run func(t *testing.T,
	ctx context.Context,
	kdbg *krt.DebugHandler,
	client istiokube.CLIClient,
	xdsPort int,
),
) {
	proxy_syncer.UseDetailedUnmarshalling = true
	writer.set(t)

	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "crds"),
			filepath.Join("..", "..", "..", "install", "helm", "kgateway-crds", "templates"),
			filepath.Join("testdata", "istio_crds_setup"),
		},
		ErrorIfCRDPathMissing: true,
		// set assets dir so we can run without the makefile
		BinaryAssetsDirectory: getAssetsDir(t),
		// This often hangs (for unknown reasons); we don't need cleanup so just kill it almost instantly
		ControlPlaneStopTimeout: time.Millisecond,
		// web hook to add cluster ips to services
	}
	envtestutil.RunController(t, logger, globalSettings, testEnv,
		nil,
		[][]string{
			{"default", "testdata/setup_yaml/setup.yaml"},
			{"gwtest", "testdata/setup_yaml/pods.yaml"},
		},
		run)
}

func testScenario(
	t *testing.T,
	ctx context.Context,
	kdbg *krt.DebugHandler,
	client istiokube.CLIClient,
	xdsPort int,
	f string,
) {
	fext := filepath.Ext(f)
	fpre := strings.TrimSuffix(f, fext)
	t.Logf("running scenario for test file: %s", f)

	// read the out file
	fout := fpre + "-out" + fext
	write := false
	ya, err := os.ReadFile(fout)
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

	var expectedXdsDump xdsDump
	err = expectedXdsDump.FromYaml(ya)
	if err != nil {
		t.Fatalf("failed to read yaml %s: %v", fout, err)
	}
	const gwname = "http-gw-for-test"
	testgwname := "http-" + filepath.Base(fpre)
	testyamlbytes, err := os.ReadFile(f)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	// change the gw name, so we could potentially run multiple tests in parallel (tough currently
	// it has other issues, so we don't run them in parallel)
	testyaml := strings.ReplaceAll(string(testyamlbytes), gwname, testgwname)

	yamlfile := filepath.Join(t.TempDir(), "test.yaml")
	os.WriteFile(yamlfile, []byte(testyaml), 0o644)

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

	t.Cleanup(func() {
		if t.Failed() {
			logKrtState(t, fmt.Sprintf("krt state for failed test: %s", t.Name()), kdbg)
		} else if os.Getenv("KGW_DUMP_KRT_ON_SUCCESS") == "true" {
			logKrtState(t, fmt.Sprintf("krt state for successful test: %s", t.Name()), kdbg)
		}
	})

	retry.UntilSuccessOrFail(t, func() error {
		dumper := newXdsDumper(t, ctx, xdsPort, testgwname)
		defer dumper.Close()
		dump, err := dumper.Dump(t, ctx)
		if err != nil {
			return err
		}
		if len(dump.Listeners) == 0 {
			return fmt.Errorf("timed out waiting for listeners")
		}
		if write {
			t.Logf("writing out file")
			// serialize xdsDump to yaml
			d, err := dump.ToYaml()
			if err != nil {
				return fmt.Errorf("failed to serialize xdsDump: %v", err)
			}
			os.WriteFile(fout, d, 0o644)
			return fmt.Errorf("wrote out file - nothing to test")
		}
		return dump.Compare(expectedXdsDump)
	}, retry.Converge(2), retry.Timeout(10*time.Second))
	t.Logf("%s finished", t.Name())
}

// logKrtState logs the krt state with a message
func logKrtState(t *testing.T, msg string, kdbg *krt.DebugHandler) {
	t.Helper()
	j, err := kdbg.MarshalJSON()
	if err != nil {
		t.Logf("failed to marshal krt state: %v", err)
	} else {
		t.Logf("%s: %s", msg, string(j))
	}
}

type xdsDumper struct {
	conn      *grpc.ClientConn
	adsClient envoy_service_discovery_v3.AggregatedDiscoveryService_StreamAggregatedResourcesClient
	dr        *envoy_service_discovery_v3.DiscoveryRequest
	cancel    context.CancelFunc
}

func (x xdsDumper) Close() {
	if x.conn != nil {
		x.conn.Close()
	}
	if x.adsClient != nil {
		x.adsClient.CloseSend()
	}
	if x.cancel != nil {
		x.cancel()
	}
}

func newXdsDumper(t *testing.T, ctx context.Context, xdsPort int, gwname string) xdsDumper {
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
						"role": structpb.NewStringValue(fmt.Sprintf("kgateway-kube-gateway-api~%s~%s", "gwtest", gwname)),
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

func (x xdsDumper) Dump(t *testing.T, ctx context.Context) (xdsDump, error) {
	dr := proto.Clone(x.dr).(*envoy_service_discovery_v3.DiscoveryRequest)
	dr.TypeUrl = "type.googleapis.com/envoy.config.cluster.v3.Cluster"
	x.adsClient.Send(dr)
	dr = proto.Clone(x.dr).(*envoy_service_discovery_v3.DiscoveryRequest)
	dr.TypeUrl = "type.googleapis.com/envoy.config.listener.v3.Listener"
	x.adsClient.Send(dr)

	var clusters []*envoyclusterv3.Cluster
	var listeners []*envoylistenerv3.Listener
	var errs error

	// run this in parallel with a 5s timeout
	done := make(chan struct{})
	go func() {
		defer close(done)
		sent := 2
		for i := 0; i < sent; i++ {
			dresp, err := x.adsClient.Recv()
			if err != nil {
				errs = errors.Join(errs, fmt.Errorf("failed to get response from xds server: %v", err))
			}
			t.Logf("got response: %s len: %d", dresp.GetTypeUrl(), len(dresp.GetResources()))
			if dresp.GetTypeUrl() == "type.googleapis.com/envoy.config.cluster.v3.Cluster" {
				for _, anyCluster := range dresp.GetResources() {
					var cluster envoyclusterv3.Cluster
					if err := anyCluster.UnmarshalTo(&cluster); err != nil {
						errs = errors.Join(errs, fmt.Errorf("failed to unmarshal cluster: %v", err))
					}
					clusters = append(clusters, &cluster)
				}
			} else if dresp.GetTypeUrl() == "type.googleapis.com/envoy.config.listener.v3.Listener" {
				needMoreListerners := false
				for _, anyListener := range dresp.GetResources() {
					var listener envoylistenerv3.Listener
					if err := anyListener.UnmarshalTo(&listener); err != nil {
						errs = errors.Join(errs, fmt.Errorf("failed to unmarshal listener: %v", err))
					}
					listeners = append(listeners, &listener)
					needMoreListerners = needMoreListerners || (len(getroutesnames(&listener)) == 0)
				}
				if len(listeners) == 0 {
					needMoreListerners = true
				}

				if needMoreListerners {
					// no routes on listener.. request another listener snapshot, after
					// the control plane processes the listeners
					sent += 1
					listeners = nil
					dr = proto.Clone(x.dr).(*envoy_service_discovery_v3.DiscoveryRequest)
					dr.TypeUrl = "type.googleapis.com/envoy.config.listener.v3.Listener"
					dr.VersionInfo = dresp.GetVersionInfo()
					dr.ResponseNonce = dresp.GetNonce()
					x.adsClient.Send(dr)
				}
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		// don't fatal yet as we want to dump the state while still connected
		errs = errors.Join(errs, fmt.Errorf("timed out waiting for listener/cluster xds dump"))
		return xdsDump{}, errs
	}
	if len(listeners) == 0 {
		errs = errors.Join(errs, fmt.Errorf("no listeners found"))
		return xdsDump{}, errs
	}
	t.Logf("xds: found %d listeners and %d clusters", len(listeners), len(clusters))

	clusterServiceNames := istioslices.MapFilter(clusters, func(c *envoyclusterv3.Cluster) *string {
		if c.GetEdsClusterConfig() != nil {
			if c.GetEdsClusterConfig().GetServiceName() != "" {
				s := c.GetEdsClusterConfig().GetServiceName()
				if s == "" {
					s = c.GetName()
				}
				return &s
			}
			return &c.Name
		}
		return nil
	})

	var routenames []string
	for _, l := range listeners {
		routenames = append(routenames, getroutesnames(l)...)
	}

	dr = proto.Clone(x.dr).(*envoy_service_discovery_v3.DiscoveryRequest)
	dr.ResourceNames = routenames
	dr.TypeUrl = "type.googleapis.com/envoy.config.route.v3.RouteConfiguration"
	x.adsClient.Send(dr)
	dr = proto.Clone(x.dr).(*envoy_service_discovery_v3.DiscoveryRequest)
	dr.TypeUrl = "type.googleapis.com/envoy.config.endpoint.v3.ClusterLoadAssignment"
	dr.ResourceNames = clusterServiceNames
	x.adsClient.Send(dr)

	var endpoints []*envoyendpointv3.ClusterLoadAssignment
	var routes []*envoyroutev3.RouteConfiguration

	done = make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 2; i++ {
			dresp, err := x.adsClient.Recv()
			if err != nil {
				errs = errors.Join(errs, fmt.Errorf("failed to get response from xds server: %v", err))
			}
			t.Logf("got response: %s len: %d", dresp.GetTypeUrl(), len(dresp.GetResources()))
			if dresp.GetTypeUrl() == "type.googleapis.com/envoy.config.route.v3.RouteConfiguration" {
				for _, anyRoute := range dresp.GetResources() {
					var route envoyroutev3.RouteConfiguration
					if err := anyRoute.UnmarshalTo(&route); err != nil {
						errs = errors.Join(errs, fmt.Errorf("failed to unmarshal route: %v", err))
					}
					routes = append(routes, &route)
				}
			} else if dresp.GetTypeUrl() == "type.googleapis.com/envoy.config.endpoint.v3.ClusterLoadAssignment" {
				for _, anyCla := range dresp.GetResources() {
					var cla envoyendpointv3.ClusterLoadAssignment
					if err := anyCla.UnmarshalTo(&cla); err != nil {
						errs = errors.Join(errs, fmt.Errorf("failed to unmarshal cla: %v", err))
					}
					// remove kube endpoints, as with envtests we will get random ports, so we cant assert on them
					if !strings.Contains(cla.ClusterName, "kubernetes") {
						endpoints = append(endpoints, &cla)
					}
				}
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		// don't fatal yet as we want to dump the state while still connected
		errs = errors.Join(errs, fmt.Errorf("timed out waiting for routes/cla xds dump"))
		return xdsDump{}, errs
	}

	t.Logf("found %d routes and %d endpoints", len(routes), len(endpoints))
	xdsDump := xdsDump{
		Clusters:  clusters,
		Listeners: listeners,
		Endpoints: endpoints,
		Routes:    routes,
	}
	return xdsDump, errs
}

type xdsDump struct {
	Clusters  []*envoyclusterv3.Cluster
	Listeners []*envoylistenerv3.Listener
	Endpoints []*envoyendpointv3.ClusterLoadAssignment
	Routes    []*envoyroutev3.RouteConfiguration
}

func (x *xdsDump) Compare(other xdsDump) error {
	var errs error

	if len(x.Clusters) != len(other.Clusters) {
		errs = errors.Join(errs, fmt.Errorf("expected %v clusters, got %v", len(other.Clusters), len(x.Clusters)))
	}

	if len(x.Listeners) != len(other.Listeners) {
		errs = errors.Join(errs, fmt.Errorf("expected %v listeners, got %v", len(other.Listeners), len(x.Listeners)))
	}
	if len(x.Endpoints) != len(other.Endpoints) {
		errs = errors.Join(errs, fmt.Errorf("expected %v endpoints, got %v", len(other.Endpoints), len(x.Endpoints)))
	}
	if len(x.Routes) != len(other.Routes) {
		errs = errors.Join(errs, fmt.Errorf("expected %v routes, got %v", len(other.Routes), len(x.Routes)))
	}

	clusterset := map[string]*envoyclusterv3.Cluster{}
	for _, c := range x.Clusters {
		clusterset[c.Name] = c
	}
	for _, otherc := range other.Clusters {
		ourc := clusterset[otherc.Name]
		if ourc == nil {
			errs = errors.Join(errs, fmt.Errorf("cluster %v not found", otherc.Name))
			continue
		}
		ourCla := ourc.LoadAssignment
		otherCla := otherc.LoadAssignment
		if err := compareCla(ourCla, otherCla); err != nil {
			errs = errors.Join(errs, fmt.Errorf("cluster %v: %w", otherc.Name, err))
		}

		// don't proto.Equal the LoadAssignment
		ourc.LoadAssignment = nil
		otherc.LoadAssignment = nil
		if !proto.Equal(otherc, ourc) {
			errs = errors.Join(errs, fmt.Errorf("cluster %v not equal: got: %s, expected: %s", otherc.Name, ourc.String(), otherc.String()))
		}
		ourc.LoadAssignment = ourCla
		otherc.LoadAssignment = otherCla
	}
	listenerset := map[string]*envoylistenerv3.Listener{}
	for _, c := range x.Listeners {
		listenerset[c.Name] = c
	}
	for _, c := range other.Listeners {
		otherc := listenerset[c.Name]
		if otherc == nil {
			errs = errors.Join(errs, fmt.Errorf("listener %v not found", c.Name))
			continue
		}
		if !proto.Equal(c, otherc) {
			errs = errors.Join(errs, fmt.Errorf("listener %v not equal", c.Name))
		}
	}
	routeset := map[string]*envoyroutev3.RouteConfiguration{}
	for _, c := range x.Routes {
		routeset[c.Name] = c
	}
	for _, c := range other.Routes {
		otherc := routeset[c.Name]
		if otherc == nil {
			errs = errors.Join(errs, fmt.Errorf("route %v not found", c.Name))
			continue
		}

		// Ignore VirtualHost ordering
		vhostFn := func(x, y *envoyroutev3.VirtualHost) bool { return x.Name < y.Name }
		if diff := cmp.Diff(c, otherc, protocmp.Transform(), protocmp.SortRepeated(vhostFn)); diff != "" {
			errs = errors.Join(errs, fmt.Errorf("route %v not equal!\ndiff:\b%s\n", c.Name, diff))
		}
	}

	epset := map[string]*envoyendpointv3.ClusterLoadAssignment{}
	for _, c := range x.Endpoints {
		epset[c.ClusterName] = c
	}
	for _, c := range other.Endpoints {
		otherc := epset[c.ClusterName]
		if err := compareCla(c, otherc); err != nil {
			errs = errors.Join(errs, fmt.Errorf("endpoint %v: %w", c.ClusterName, err))
		}
	}

	return errs
}

func compareCla(c, otherc *envoyendpointv3.ClusterLoadAssignment) error {
	if (c == nil) != (otherc == nil) {
		if c == nil {
			return fmt.Errorf("cluster is nil")
		}
		return fmt.Errorf("ep %v not found", c.ClusterName)
	}
	if c == nil || otherc == nil {
		return nil
	}
	ep1 := flattenendpoints(c)
	ep2 := flattenendpoints(otherc)
	if !equalset(ep1, ep2) {
		return fmt.Errorf("ep list %v not equal: %v %v", c.ClusterName, ep1, ep2)
	}
	ce := c.Endpoints
	ocd := otherc.Endpoints
	c.Endpoints = nil
	otherc.Endpoints = nil
	if !proto.Equal(c, otherc) {
		return fmt.Errorf("ep %v not equal", c.ClusterName)
	}
	c.Endpoints = ce
	otherc.Endpoints = ocd
	return nil
}

func equalset(a, b []*envoyendpointv3.LocalityLbEndpoints) bool {
	if len(a) != len(b) {
		return false
	}
	for _, v := range a {
		if istioslices.FindFunc(b, func(e *envoyendpointv3.LocalityLbEndpoints) bool {
			return proto.Equal(v, e)
		}) == nil {
			return false
		}
	}
	return true
}

func flattenendpoints(v *envoyendpointv3.ClusterLoadAssignment) []*envoyendpointv3.LocalityLbEndpoints {
	var flat []*envoyendpointv3.LocalityLbEndpoints
	for _, e := range v.Endpoints {
		for _, l := range e.LbEndpoints {
			flatbase := proto.Clone(e).(*envoyendpointv3.LocalityLbEndpoints)
			flatbase.LbEndpoints = []*envoyendpointv3.LbEndpoint{l}
			flat = append(flat, flatbase)
		}
	}
	return flat
}

func (x *xdsDump) FromYaml(ya []byte) error {
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
		r, err := anyJsonRoundTrip[envoyclusterv3.Cluster](c)
		if err != nil {
			return err
		}
		x.Clusters = append(x.Clusters, r)
	}
	for _, c := range jsonM["endpoints"] {
		r, err := anyJsonRoundTrip[envoyendpointv3.ClusterLoadAssignment](c)
		if err != nil {
			return err
		}
		x.Endpoints = append(x.Endpoints, r)
	}
	for _, c := range jsonM["listeners"] {
		r, err := anyJsonRoundTrip[envoylistenerv3.Listener](c)
		if err != nil {
			return err
		}
		x.Listeners = append(x.Listeners, r)
	}
	for _, c := range jsonM["routes"] {
		r, err := anyJsonRoundTrip[envoyroutev3.RouteConfiguration](c)
		if err != nil {
			return err
		}
		x.Routes = append(x.Routes, r)
	}
	return nil
}

func anyJsonRoundTrip[T any, PT interface {
	proto.Message
	*T
}](c any) (PT, error) {
	var ju jsonpb.UnmarshalOptions
	jb, err := json.Marshal(c)
	var zero PT
	if err != nil {
		return zero, err
	}
	var r T
	var pr PT = &r
	err = ju.Unmarshal(jb, pr)
	return pr, err
}

func sortResource[T fmt.Stringer](resources []T) []T {
	// clone the slice
	resources = append([]T(nil), resources...)
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].String() < resources[j].String()
	})
	return resources
}

func (x *xdsDump) ToYaml() ([]byte, error) {
	jsonM := map[string][]any{}
	for _, c := range sortResource(x.Clusters) {
		roundtrip, err := protoJsonRoundTrip(c)
		if err != nil {
			return nil, err
		}
		jsonM["clusters"] = append(jsonM["clusters"], roundtrip)
	}
	for _, c := range sortResource(x.Listeners) {
		roundtrip, err := protoJsonRoundTrip(c)
		if err != nil {
			return nil, err
		}
		jsonM["listeners"] = append(jsonM["listeners"], roundtrip)
	}
	for _, c := range sortResource(x.Endpoints) {
		roundtrip, err := protoJsonRoundTrip(c)
		if err != nil {
			return nil, err
		}
		jsonM["endpoints"] = append(jsonM["endpoints"], roundtrip)
	}
	for _, c := range sortResource(x.Routes) {
		roundtrip, err := protoJsonRoundTrip(c)
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

func protoJsonRoundTrip(c proto.Message) (any, error) {
	var j jsonpb.MarshalOptions
	s, err := j.Marshal(c)
	if err != nil {
		return nil, err
	}
	var roundtrip any
	err = json.Unmarshal(s, &roundtrip)
	if err != nil {
		return nil, err
	}
	return roundtrip, nil
}

func getroutesnames(l *envoylistenerv3.Listener) []string {
	var routes []string
	for _, fc := range l.GetFilterChains() {
		for _, filter := range fc.GetFilters() {
			suffix := string((&envoyhttp.HttpConnectionManager{}).ProtoReflect().Descriptor().FullName())
			if strings.HasSuffix(filter.GetTypedConfig().GetTypeUrl(), suffix) {
				var hcm envoyhttp.HttpConnectionManager
				switch config := filter.GetConfigType().(type) {
				case *envoylistenerv3.Filter_TypedConfig:
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
