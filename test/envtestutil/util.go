package envtestutil

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/controller"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/setup"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
)

func RunController(t *testing.T, logger *zap.Logger, globalSettings *settings.Settings, testEnv *envtest.Environment,
	postStart func(t *testing.T, ctx context.Context, client istiokube.CLIClient) func(ctx context.Context, commoncol *common.CommonCollections) []pluginsdk.Plugin,
	yamlFilesToApply [][]string,
	run func(t *testing.T,
		ctx context.Context,
		kdbg *krt.DebugHandler,
		client istiokube.CLIClient,
		xdsPort int,
	)) {
	if globalSettings == nil {
		st, err := settings.BuildSettings()
		if err != nil {
			t.Fatalf("failed to get settings %v", err)
		}
		globalSettings = st
	}

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
		t.Fatalf("failed to get init kube client: %v", err)
	}
	var extraPlugins func(ctx context.Context, commoncol *common.CommonCollections) []pluginsdk.Plugin
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
		err = applyPodStatusFromFile(ctx, client, ns, yamlFile)
		if err != nil {
			t.Fatalf("failed to apply pod status: %v", err)
		}
	}

	// setup xDS server:
	uniqueClientCallbacks, builder := krtcollections.NewUniquelyConnectedClients()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("can't listen %v", err)
	}
	xdsPort := lis.Addr().(*net.TCPAddr).Port
	snapCache, grpcServer := setup.NewControlPlaneWithListener(ctx, lis, uniqueClientCallbacks)
	t.Cleanup(func() { grpcServer.Stop() })

	setupOpts := &controller.SetupOpts{
		Cache:          snapCache,
		KrtDebugger:    new(krt.DebugHandler),
		GlobalSettings: globalSettings,
	}

	// start kgateway
	wg.Add(1)
	go func() {
		defer wg.Done()
		setup.StartKgatewayWithConfig(ctx, setupOpts, cfg, builder, extraPlugins)
	}()
	// give kgateway time to initialize so we don't get
	// "kgateway not initialized" error
	// this means that it attaches the pod collection to the unique client set collection.
	time.Sleep(time.Second)
	run(t, ctx, setupOpts.KrtDebugger, client, xdsPort)
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

// applyPodStatusFromFile reads a YAML file, looks for Pod resources with a Status set,
// and patches their status into the cluster. Skips any Pods not found or lacking a status.
// This is needed because the other places that apply yaml will only apply spec.
// We now have tests (ServiceEntry) that rely on IPs from Pod status instead of EndpointSlice.
func applyPodStatusFromFile(ctx context.Context, c istiokube.CLIClient, defaultNs, filePath string) error {
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
