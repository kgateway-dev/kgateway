package helpers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/solo-io/go-utils/threadsafe"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	kgatewayAdminCli "github.com/kgateway-dev/kgateway/v2/pkg/utils/controllerutils/admincli"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envoyutils/admincli"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/portforward"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
)

// StandardKgatewayDumpOnFail creates a dump of the kubernetes state and certain envoy data from
// the admin interface when a test fails.
// Look at `KubeDumpOnFail` && `EnvoyDumpOnFail` for more details
func StandardKgatewayDumpOnFail(outLog io.Writer, kubectlCli *kubectl.Cli, outDir string, namespaces []string) func() {
	return func() {
		fmt.Printf("Test failed. Dumping state from %s...\n", strings.Join(namespaces, ", "))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		// only wipe at the start of the dump
		wipeOutDir(outDir)

		KubeDumpOnFail(ctx, kubectlCli, outLog, outDir, namespaces)()
		ControllerDumpOnFail(ctx, kubectlCli, outLog, outDir, namespaces)()
		EnvoyDumpOnFail(ctx, kubectlCli, outLog, outDir, namespaces)()

		fmt.Printf("Test failed. Logs and cluster state are available in %s\n", outDir)
	}
}

// KubeDumpOnFail creates a small dump of the kubernetes state when a test fails.
// This is useful for debugging test failures.
// The dump includes:
// - docker state
// - process state
// - kubernetes state
// - logs from all pods in the given namespaces
// - yaml representations of all kgateway CRs in the given namespaces
func KubeDumpOnFail(ctx context.Context, kubectlCli *kubectl.Cli, outLog io.Writer, outDir string,
	namespaces []string) func() {
	return func() {
		setupOutDir(outDir)

		recordDockerState(fileAtPath(filepath.Join(outDir, "docker-state.log")))
		recordProcessState(fileAtPath(filepath.Join(outDir, "process-state.log")))
		recordKubeState(ctx, kubectlCli, fileAtPath(filepath.Join(outDir, "kube-state.log")))

		recordKubeDump(outDir, namespaces...)

		fmt.Printf("Finished dumping kubernetes state\n")
	}
}

func recordDockerState(f *os.File) {
	defer f.Close()

	dockerCmd := exec.Command("docker", "ps")

	dockerState := &bytes.Buffer{}

	dockerCmd.Stdout = dockerState
	dockerCmd.Stderr = dockerState
	err := dockerCmd.Run()
	if err != nil {
		f.WriteString("*** Unable to get docker state ***. Reason: " + err.Error() + " \n")
		return
	}
	f.WriteString("*** Docker state ***\n")
	f.WriteString(dockerState.String() + "\n")
	f.WriteString("*** End Docker state ***\n")
}

func recordProcessState(f *os.File) {
	defer f.Close()

	psCmd := exec.Command("ps", "-auxf")

	psState := &bytes.Buffer{}

	psCmd.Stdout = psState
	psCmd.Stderr = psState
	err := psCmd.Run()
	if err != nil {
		f.WriteString("unable to get process state. Reason: " + err.Error() + " \n")
		return
	}
	f.WriteString("*** Process state ***\n")
	f.WriteString(psState.String() + "\n")
	f.WriteString("*** End Process state ***\n")
}

func recordKubeState(ctx context.Context, kubectlCli *kubectl.Cli, f *os.File) {
	defer f.Close()
	kubeState, err := kubectlCli.RunCommandWithOutput(ctx, "get", "all", "-A", "-o", "wide")
	if err != nil {
		f.WriteString(fmt.Sprintf("*** Unable to get kube state ***\nStdout: %s\nReason: %v", err, kubeState))
		return
	}

	resourcesToGet := []string{
		// Kubernetes resources
		"secrets",
		// Kube GW API resources
		"backendlbpolicies.gateway.networking.k8s.io",
		"backendtlspolicies.gateway.networking.k8s.io",
		"gatewayclasses.gateway.networking.k8s.io",
		"gateways.gateway.networking.k8s.io",
		"grpcroutes.gateway.networking.k8s.io",
		"httproutes.gateway.networking.k8s.io",
		"referencegrants.gateway.networking.k8s.io",
		"tcproutes.gateway.networking.k8s.io",
		"tlsroutes.gateway.networking.k8s.io",
		"udproutes.gateway.networking.k8s.io",
		// kgateway resources
		"backends.gateway.kgateway.dev",
		"directresponses.gateway.kgateway.dev",
		"gatewayextensions.gateway.kgateway.dev",
		"gatewayparameters.gateway.kgateway.dev",
		"httplistenerpolicies.gateway.kgateway.dev",
		"trafficpolicies.gateway.kgateway.dev",
	}

	kubeResources, err := kubectlCli.RunCommandWithOutput(ctx, "get", strings.Join(resourcesToGet, ","), "-A", "-owide")
	if err != nil {
		f.WriteString("*** Unable to get kube resources ***. Reason: " + err.Error() + " \n")
		return
	}

	// Describe everything to identify the reason for issues such as Pods, LoadBalancers stuck in pending state
	// (insufficient resources, unable to acquire an IP), etc.
	// Ie: More context around the output of the previous command `kubectl get all -A`
	kubeDescribe, err := kubectlCli.RunCommandWithOutput(ctx, "describe", "all", "-A")
	if err != nil {
		f.WriteString("*** Unable to get kube describe ***. Reason: " + err.Error() + " \n")
		return
	}

	kubeEndpointsState, err := kubectlCli.RunCommandWithOutput(ctx, "get", "endpoints", "-A")
	if err != nil {
		f.WriteString("*** Unable to get endpoint state ***. Reason: " + err.Error() + " \n")
		return
	}

	f.WriteString("*** Kube state ***\n")
	f.WriteString(string(kubeState) + "\n")
	f.WriteString(string(kubeResources) + "\n")
	f.WriteString(string(kubeDescribe) + "\n")
	f.WriteString(string(kubeEndpointsState) + "\n")

	f.WriteString("*** End Kube state ***\n")
}

func recordKubeDump(outDir string, namespaces ...string) {
	// for each namespace, create a namespace directory that contains...
	for _, ns := range namespaces {
		// ...a pod logs subdirectoy
		if err := recordPods(filepath.Join(outDir, ns, "_pods"), ns); err != nil {
			fmt.Printf("error recording pod logs: %f, \n", err)
		}

		// ...and a subdirectory for each kgateway CRD with non-zero resources
		if err := recordCRs(filepath.Join(outDir, ns), ns); err != nil {
			fmt.Printf("error recording pod logs: %f, \n", err)
		}
	}
}

// recordPods records logs from each pod to <output-dir>/$namespace/pods/$pod.log
func recordPods(podDir, namespace string) error {
	pods, _, err := kubeList(namespace, "pod")
	if err != nil {
		return err
	}

	var errs []error

	for _, pod := range pods {
		if err := os.MkdirAll(podDir, os.ModePerm); err != nil {
			return err
		}

		logs, errOutput, err := kubeLogs(namespace, pod)
		// store any error running the log command to return later
		// the error represents the cause of the failure, and should be bubbled up
		// we will still try to get logs for other pods even if this one returns an error
		if err != nil {
			errs = append(errs, err)
		}
		// write any log output to the standard file
		if logs != "" {
			f := fileAtPath(filepath.Join(podDir, pod+".log"))
			f.WriteString(logs)
			f.Close()
		}
		// write any error output to the error file
		// this will consist of the combined stdout and stderr of the command
		if errOutput != "" {
			f := fileAtPath(filepath.Join(podDir, pod+"-error.log"))
			f.WriteString(errOutput)
			f.Close()
		}
	}

	return errors.Join(errs...)
}

// recordCRs records all unique CRs floating about to <output-dir>/$namespace/$crd/$cr.yaml
func recordCRs(namespaceDir string, namespace string) error {
	crds, _, err := kubeList(namespace, "crd")
	if err != nil {
		return err
	}

	// record all unique CRs floating about
	for _, crd := range crds {
		// consider all installed CRDs that are kgateway-managed
		if !strings.Contains(crd, "kgateway.dev") {
			continue
		}

		// if there are any existing CRs corresponding to this CRD
		crs, _, err := kubeList(namespace, crd)
		if err != nil {
			return err
		}
		if len(crs) == 0 {
			continue
		}
		crdDir := filepath.Join(namespaceDir, crd)
		if err := os.MkdirAll(crdDir, os.ModePerm); err != nil {
			return err
		}

		// we record each one in its own .yaml representation
		for _, cr := range crs {
			f := fileAtPath(filepath.Join(crdDir, cr+".yaml"))
			errF := fileAtPath(filepath.Join(crdDir, cr+"-error.log"))

			crDetails, errOutput, err := kubeGet(namespace, crd, cr)

			if crDetails != "" {
				f.WriteString(crDetails)
				f.Close()
			}
			if errOutput != "" {
				errF.WriteString(errOutput)
				errF.Close()
			}

			return err
		}
	}

	return nil
}

// kubeLogs runs $(kubectl -n $namespace logs $pod --all-containers) and returns the string result
func kubeLogs(namespace string, pod string) (string, string, error) {
	args := []string{"-n", namespace, "logs", pod, "--all-containers"}
	return kubeExecute(args)
}

// kubeGet runs $(kubectl -n $namespace get $kubeType $name -oyaml) and returns the string result
func kubeGet(namespace string, kubeType string, name string) (string, string, error) {
	args := []string{"-n", namespace, "get", kubeType, name, "-oyaml"}
	return kubeExecute(args)
}

func kubeExecute(args []string) (string, string, error) {
	cli := kubectl.NewCli().WithReceiver(ginkgo.GinkgoWriter)

	var outLocation threadsafe.Buffer
	runError := cli.Command(context.Background(), args...).WithStdout(&outLocation).Run()
	if runError != nil {
		return outLocation.String(), runError.OutputString(), runError.Cause()
	}

	return outLocation.String(), "", nil
}

// kubeList runs $(kubectl -n $namespace $target) and returns a slice of kubernetes object names
func kubeList(namespace string, target string) ([]string, string, error) {
	args := []string{"-n", namespace, "get", target}
	lines, errContent, err := kubeExecute(args)
	if err != nil {
		return nil, errContent, err
	}

	var toReturn []string
	for _, line := range strings.Split(strings.TrimSuffix(lines, "\n"), "\n") {
		if strings.HasPrefix(line, "NAME") || strings.HasPrefix(line, "No resources found") {
			continue // skip header line and cases where there are no resources
		}
		if split := strings.Split(line, " "); len(split) > 1 {
			toReturn = append(toReturn, split[0])
		}
	}
	return toReturn, "", nil
}

// ControllerDumpOnFail creates a small dump of the controller state when a test fails.
// This is useful for debugging test failures.
// The dump includes:
// - controller logs
// - controller metrics
// - controller xds snapshot
// - controller krt snapshot
func ControllerDumpOnFail(ctx context.Context, kubectlCli *kubectl.Cli, outLog io.Writer,
	outDir string, namespaces []string) func() {
	return func() {
		for _, ns := range namespaces {
			controllerPodNames, err := kubectlCli.GetPodsInNsWithLabel(ctx, ns, "kgateway=kgateway")
			if err != nil {
				fmt.Printf("error fetching controller pod names: %f\n", err)
				continue
			}

			if len(controllerPodNames) == 0 {
				fmt.Printf("no kgateway=kgateway pods found in namespace %s\n", ns)
				continue
			}

			fmt.Printf("found controller pods: %s\n", strings.Join(controllerPodNames, ", "))

			namespaceOutDir := filepath.Join(outDir, ns)
			setupOutDir(namespaceOutDir)

			for _, podName := range controllerPodNames {
				writeControllerLog(ctx, namespaceOutDir, ns, podName, kubectlCli)
				writeMetricsLog(ctx, namespaceOutDir, ns, podName, kubectlCli)

				// Open a port-forward to the controller pod's admin port
				portForwarder, err := kubectlCli.StartPortForward(ctx,
					portforward.WithPod(podName, ns),
					portforward.WithPorts(int(wellknown.KgatewayAdminPort), int(wellknown.KgatewayAdminPort)),
				)
				if err != nil {
					fmt.Printf("error starting port forward to controller admin port: %f\n", err)
				}

				defer func() {
					portForwarder.Close()
					portForwarder.WaitForStop()
				}()

				adminClient := kgatewayAdminCli.NewClient().
					WithReceiver(io.Discard).
					WithCurlOptions(
						curl.WithRetries(3, 0, 10),
						curl.WithPort(int(wellknown.KgatewayAdminPort)),
					)

				krtSnapshotFile := fileAtPath(filepath.Join(namespaceOutDir, fmt.Sprintf("%s.krt_snapshot.log", podName)))
				err = adminClient.KrtSnapshotCmd(ctx).WithStdout(krtSnapshotFile).Run().Cause()
				if err != nil {
					fmt.Printf("error running krt snapshot command: %f\n", err)
				}

				xdsSnapshotFile := fileAtPath(filepath.Join(namespaceOutDir, fmt.Sprintf("%s.xds_snapshot.log", podName)))
				err = adminClient.XdsSnapshotCmd(ctx).WithStdout(xdsSnapshotFile).Run().Cause()
				if err != nil {
					fmt.Printf("error running xds snapshot command: %f\n", err)
				}

				fmt.Printf("finished dumping controller state\n")
			}
		}
	}
}

// EnvoyDumpOnFail creates a small dump of the envoy admin interface when a test fails.
// This is useful for debugging test failures.
// The dump includes:
// - config dump
// - stats
// - clusters
// - listeners
func EnvoyDumpOnFail(ctx context.Context, kubectlCli *kubectl.Cli, _ io.Writer, outDir string, namespaces []string) func() {
	return func() {
		for _, ns := range namespaces {
			proxies := []string{}

			kubeGatewayProxies, err := kubectlCli.GetPodsInNsWithLabel(ctx, ns, "kgateway=kube-gateway")
			if err != nil {
				fmt.Printf("error fetching kube-gateway proxies: %f\n", err)
			} else {
				proxies = append(proxies, kubeGatewayProxies...)
			}

			if len(proxies) == 0 {
				fmt.Printf("no proxies found in namespace %s\n", ns)
				continue
			}

			fmt.Printf("found proxies: %s\n", strings.Join(proxies, ", "))

			envoyOutDir := filepath.Join(outDir, ns)
			setupOutDir(envoyOutDir)

			for _, proxy := range proxies {
				adminCli, shutdown, err := admincli.NewPortForwardedClient(ctx,
					fmt.Sprintf("pod/%s", proxy), ns)
				if err != nil {
					fmt.Printf("error creating admin cli: %f\n", err)
					continue
				}

				defer shutdown()

				configDumpFile := fileAtPath(filepath.Join(envoyOutDir, fmt.Sprintf("%s.config.log", proxy)))
				err = adminCli.ConfigDumpCmd(ctx, nil).WithStdout(configDumpFile).Run().Cause()
				if err != nil {
					fmt.Printf("error running config dump command: %f\n", err)
				}

				statsFile := fileAtPath(filepath.Join(envoyOutDir, fmt.Sprintf("%s.stats.log", proxy)))
				err = adminCli.StatsCmd(ctx, nil).WithStdout(statsFile).Run().Cause()
				if err != nil {
					fmt.Printf("error running stats command: %f\n", err)
				}

				clustersFile := fileAtPath(filepath.Join(envoyOutDir, fmt.Sprintf("%s.clusters.log", proxy)))
				err = adminCli.ClustersCmd(ctx).WithStdout(clustersFile).Run().Cause()
				if err != nil {
					fmt.Printf("error running clusters command: %f\n", err)
				}

				listenersFile := fileAtPath(filepath.Join(envoyOutDir, fmt.Sprintf("%s.listeners.log", proxy)))
				err = adminCli.ListenersCmd(ctx).WithStdout(listenersFile).Run().Cause()
				if err != nil {
					fmt.Printf("error running listeners command: %f\n", err)
				}

				fmt.Printf("finished dumping envoy state\n")
			}
		}
	}
}

func wipeOutDir(outDir string) {
	err := os.RemoveAll(outDir)
	if err != nil {
		fmt.Printf("error wiping out directory: %f\n", err)
	}
}

// setupOutDir forcibly deletes/creates the output directory
func setupOutDir(outdir string) {
	err := os.MkdirAll(outdir, os.ModePerm)
	if err != nil {
		fmt.Printf("error creating log directory: %f\n", err)
	}
}

// fileAtPath creates a file at the given path, and returns the file object
func fileAtPath(path string) *os.File {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		fmt.Printf("unable to openfile: %f\n", err)
	}
	return f
}

func writeControllerLog(ctx context.Context, outDir string, ns string, podName string, kubectlCli *kubectl.Cli) {
	// Get the kgateway controller logs
	controllerLogsFile := fileAtPath(filepath.Join(outDir, fmt.Sprintf("%s.controller.log", podName)))
	controllerLogsCmd := kubectlCli.WithReceiver(controllerLogsFile).Command(ctx,
		"-n", ns, "logs", podName, "-c", KgatewayContainerName, "--tail=1000")
	err := controllerLogsCmd.Run().Cause()
	if err != nil {
		fmt.Printf("error running controller logs for %s in %s command: %v\n", podName, ns, err)
	}
}

func writeMetricsLog(ctx context.Context, outDir string, ns string, podName string, kubectlCli *kubectl.Cli) {
	// Using an ephemeral debug pod fetch the metrics from the kgateway controller
	metricsFile := fileAtPath(filepath.Join(outDir, fmt.Sprintf("%s.metrics.log", podName)))
	metricsCmd := kubectlCli.Command(ctx, "debug", "-n", ns,
		"-it", "--image=curlimages/curl:7.83.1", podName, "--",
		"curl", "http://localhost:9091/metrics")
	err := metricsCmd.WithStdout(metricsFile).WithStderr(metricsFile).Run().Cause()
	if err != nil {
		fmt.Printf("error running metrics command: %f\n", err)
	}
}
