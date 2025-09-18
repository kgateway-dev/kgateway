package envtestutil

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/yaml"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/controller"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/setup"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

var setupLogging = sync.Once{}

type postStartFunc func(t *testing.T, ctx context.Context, client istiokube.CLIClient) func(ctx context.Context, commoncol *common.CommonCollections, mergeSettingsJSON string) []pluginsdk.Plugin

func RunController(
	t *testing.T,
	logger *zap.Logger,
	globalSettings *settings.Settings,
	testEnv *envtest.Environment,
	postStart postStartFunc,
	yamlFilesToApply [][]string,
	validator validator.Validator,
	run func(t *testing.T,
		ctx context.Context,
		kdbg *krt.DebugHandler,
		client istiokube.CLIClient,
		xdsPort int,
	),
) {
	if globalSettings == nil {
		st, err := settings.BuildSettings()
		if err != nil {
			t.Fatalf("failed to get settings %v", err)
		}
		globalSettings = st
	}
	// Always set once instead of each time to avoid races
	logLevel := globalSettings.LogLevel
	globalSettings.LogLevel = ""
	setupLogging.Do(func() {
		setup.SetupLogging(logLevel)
	})

	// Enable this if you want api server logs and audit logs.
	if os.Getenv("DEBUG_APISERVER") == "true" {
		addApiServerLogs(t, testEnv)
	}
	var wg sync.WaitGroup
	t.Cleanup(wg.Wait)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ctx = contextutils.WithExistingLogger(ctx, logger.Sugar())

	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("failed to get assets dir: %v", err)
	}
	t.Cleanup(func() { testEnv.Stop() })

	kubeconfig := GenerateKubeConfiguration(t, cfg)
	t.Log("kubeconfig:", kubeconfig)

	client, err := istiokube.NewCLIClient(istiokube.NewClientConfigForRestConfig(cfg))
	if err != nil {
		t.Fatalf("failed to init kube client: %v", err)
	}
	istiokube.EnableCrdWatcher(client)

	var extraPlugins func(ctx context.Context, commoncol *common.CommonCollections, mergeSettingsJSON string) []pluginsdk.Plugin
	if postStart != nil {
		extraPlugins = postStart(t, ctx, client)
	}

	for _, yamlFileWithNs := range yamlFilesToApply {
		ns := yamlFileWithNs[0]
		yamlFile := yamlFileWithNs[1]
		err = client.ApplyYAMLFiles(ns, yamlFile)
		if err != nil {
			t.Fatalf("failed to apply yaml: %v", err)
		}
		err = ApplyPodStatusFromFile(ctx, client, ns, yamlFile)
		if err != nil {
			t.Fatalf("failed to apply pod status: %v", err)
		}
	}

	krtDbg := new(krt.DebugHandler)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("can't listen %v", err)
	}

	s, err := setup.New(
		setup.WithGlobalSettings(globalSettings),
		setup.WithRestConfig(cfg),
		setup.WithExtraPlugins(extraPlugins),
		setup.WithKrtDebugger(krtDbg),
		setup.WithXDSListener(l),
		setup.WithControllerManagerOptions(
			func(ctx context.Context) *ctrl.Options {
				return &ctrl.Options{
					BaseContext:      func() context.Context { return ctx },
					Scheme:           runtime.NewScheme(),
					PprofBindAddress: "127.0.0.1:9099",
					// if you change the port here, also change the port "health" in the helmchart.
					HealthProbeBindAddress: ":9093",
					Controller: config.Controller{
						// 	// see https://github.com/kubernetes-sigs/controller-runtime/issues/2937
						// 	// in short, our tests reuse the same name (reasonably so) and the controller-runtime
						// 	// package does not reset the stack of controller names between tests, so we disable
						// 	// the name validation here.
						SkipNameValidation: ptr.To(true),
					},
				}
			}),
		setup.WithExtraManagerConfig([]func(ctx context.Context, mgr manager.Manager, objectFilter kubetypes.DynamicObjectFilter) error{
			func(ctx context.Context, mgr manager.Manager, objectFilter kubetypes.DynamicObjectFilter) error {
				return controller.AddToScheme(mgr.GetScheme())
			},
		}...),
		setup.WithValidator(validator),
	)
	if err != nil {
		t.Fatalf("error setting up kgateway %v", err)
	}

	// start kgateway
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.Start(ctx); err != nil {
			log.Fatalf("error starting kgateway %v", err)
		}
	}()

	xdsPort := l.Addr().(*net.TCPAddr).Port
	t.Log("running tests, xds port:", xdsPort)
	run(t, ctx, krtDbg, client, xdsPort)
	t.Log("controller done. shutting down. xds port:", xdsPort)
}

func GenerateKubeConfiguration(t *testing.T, restconfig *rest.Config) string {
	clusters := make(map[string]*clientcmdapi.Cluster)
	authinfos := make(map[string]*clientcmdapi.AuthInfo)
	contexts := make(map[string]*clientcmdapi.Context)

	clusterName := "cluster"
	clusters[clusterName] = &clientcmdapi.Cluster{
		Server:                   restconfig.Host,
		CertificateAuthorityData: restconfig.CAData,
	}
	authinfos[clusterName] = &clientcmdapi.AuthInfo{
		ClientKeyData:         restconfig.KeyData,
		ClientCertificateData: restconfig.CertData,
	}
	contexts[clusterName] = &clientcmdapi.Context{
		Cluster:   clusterName,
		Namespace: "default",
		AuthInfo:  clusterName,
	}

	clientConfig := clientcmdapi.Config{
		Kind:       "Config",
		APIVersion: "v1",
		Clusters:   clusters,
		Contexts:   contexts,
		// current context must be mgmt cluster for now, as the api server doesn't have context configurable.
		CurrentContext: "cluster",
		AuthInfos:      authinfos,
	}
	// create temp file
	tmpfile := filepath.Join(t.TempDir(), "kubeconfig")
	err := clientcmd.WriteToFile(clientConfig, tmpfile)
	if err != nil {
		t.Fatalf("failed to write kubeconfig: %v", err)
	}

	return tmpfile
}

func addApiServerLogs(t *testing.T, testEnv *envtest.Environment) {
	apiserverOut := new(bytes.Buffer)
	apiserverErr := new(bytes.Buffer)

	testEnv.ControlPlane = envtest.ControlPlane{
		APIServer: &envtest.APIServer{
			Out: apiserverOut,
			Err: apiserverErr,
		},
	}
	policy := policyFile()
	t.Cleanup(func() {
		os.Remove(policy)
	})
	args := testEnv.ControlPlane.APIServer.Configure()
	args.Append("audit-log-path", "-")
	args.Append("audit-policy-file", policy)
	t.Cleanup(func() {
		t.Log("apiserver out:", apiserverOut.String())
		t.Log("apiserver err:", apiserverErr.String())
	})
}

// ApplyPodStatusFromFile reads a YAML file, looks for Pod resources with a Status set,
// and patches their status into the cluster. Skips any Pods not found or lacking a status.
// This is needed because the other places that apply yaml will only apply spec.
// We now have tests (ServiceEntry) that rely on IPs from Pod status instead of EndpointSlice.
func ApplyPodStatusFromFile(ctx context.Context, c istiokube.CLIClient, defaultNs, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading YAML file %q: %w", filePath, err)
	}

	docs := bytes.Split(data, []byte("\n---\n"))

	for _, doc := range docs {
		doc = bytes.TrimSpace(doc)
		if len(doc) == 0 {
			continue
		}

		pod := &corev1.Pod{}
		if err := yaml.Unmarshal(doc, pod); err != nil {
			continue
		}
		if pod.Kind != "Pod" {
			continue
		}

		// Skip if there's no status to patch
		if pod.Status.PodIP == "" && len(pod.Status.PodIPs) == 0 && pod.Status.Phase == "" {
			continue
		}

		ns := pod.Namespace
		if ns == "" {
			ns = defaultNs
		}

		podClient := c.Kube().CoreV1().Pods(ns)

		// Retrieve the existing Pod
		existingPod, err := podClient.Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to retrieve existing Pod %s/%s: %w", ns, pod.Name, err)
		}

		// Update the in-memory status
		existingPod.Status = pod.Status

		// Persist the new status
		_, err = podClient.UpdateStatus(ctx, existingPod, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update status for Pod %s/%s: %w", ns, pod.Name, err)
		}
	}

	return nil
}

func policyFile() string {
	p := `apiVersion: audit.k8s.io/v1
kind: Policy
rules:
- level: Metadata
`
	// write to temp file:
	f, err := os.CreateTemp("", "policy.yaml")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	_, err = f.WriteString(p)
	if err != nil {
		panic(err)
	}
	return f.Name()
}
