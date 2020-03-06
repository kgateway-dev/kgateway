package check

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/solo-io/gloo/pkg/cliutil"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.CHECK_COMMAND.Use,
		Short: constants.CHECK_COMMAND.Short,
		RunE: func(cmd *cobra.Command, args []string) error {
			ok, err := CheckResources(opts)
			if err != nil {
				// Not returning error here because this shouldn't propagate as a standard CLI error, which prints usage.
				fmt.Printf("Error!\n")
				fmt.Printf("%s\n", err.Error())
				os.Exit(1)
			} else if !ok {
				fmt.Printf("Problems detected!\n")
				os.Exit(1)
			} else {
				fmt.Printf("No problems detected.\n")
			}
			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddNamespaceFlag(pflags, &opts.Metadata.Namespace)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func CheckResources(opts *options.Options) (bool, error) {
	err := checkConnection()
	if err != nil {
		return false, err
	}
	ok, err := checkDeployments(opts)
	if !ok || err != nil {
		return ok, err
	}
	ok, err = checkPods(opts)
	if !ok || err != nil {
		return ok, err
	}
	settings, err := getSettings(opts)
	if err != nil {
		return false, err
	}

	namespaces, err := getNamespaces(settings)
	if err != nil {
		return false, err
	}

	knownUpstreams, ok, err := checkUpstreams(namespaces)
	if !ok || err != nil {
		return ok, err
	}

	ok, err = checkUpstreamGroups(namespaces)
	if !ok || err != nil {
		return ok, err
	}

	ok, err = checkSecrets(namespaces)
	if !ok || err != nil {
		return ok, err
	}

	ok, err = checkVirtualServices(namespaces, knownUpstreams)
	if !ok || err != nil {
		return ok, err
	}

	ok, err = checkGateways(namespaces)
	if !ok || err != nil {
		return ok, err
	}

	ok, err = checkProxies(namespaces, opts.Metadata.Namespace)
	if !ok || err != nil {
		return ok, err
	}

	return true, nil
}

func checkDeployments(opts *options.Options) (bool, error) {
	fmt.Printf("Checking deployments... ")
	client := helpers.MustKubeClient()
	_, err := client.CoreV1().Namespaces().Get(opts.Metadata.Namespace, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("Gloo namespace does not exist\n")
		return false, err
	}
	deployments, err := client.AppsV1().Deployments(opts.Metadata.Namespace).List(metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	if len(deployments.Items) == 0 {
		fmt.Printf("Gloo is not installed\n")
		return false, nil
	}

	var errorToPrint string
	var message string
	setMessage := func(c appsv1.DeploymentCondition) {
		if c.Message != "" {
			message = fmt.Sprintf(" Message: %s", c.Message)
		}
	}

	for _, deployment := range deployments.Items {
		// possible condition types listed at https://godoc.org/k8s.io/api/apps/v1#DeploymentConditionType
		// check for each condition independently because multiple conditions will be True and DeploymentReplicaFailure
		// tends to provide the most explicit error message.
		for _, condition := range deployment.Status.Conditions {
			setMessage(condition)
			if condition.Type == appsv1.DeploymentReplicaFailure && condition.Status == corev1.ConditionTrue {
				errorToPrint = fmt.Sprintf("Deployment %s in namespace %s failed to create pods!%s\n", deployment.Name, deployment.Namespace, message)
			}
			if errorToPrint != "" {
				fmt.Print(errorToPrint)
				return false, err
			}
		}

		for _, condition := range deployment.Status.Conditions {
			setMessage(condition)
			if condition.Type == appsv1.DeploymentProgressing && condition.Status != corev1.ConditionTrue {
				errorToPrint = fmt.Sprintf("Deployment %s in namespace %s is not progressing!%s\n", deployment.Name, deployment.Namespace, message)
			}

			if errorToPrint != "" {
				fmt.Print(errorToPrint)
				return false, err
			}
		}

		for _, condition := range deployment.Status.Conditions {
			setMessage(condition)
			if condition.Type == appsv1.DeploymentAvailable && condition.Status != corev1.ConditionTrue {
				errorToPrint = fmt.Sprintf("Deployment %s in namespace %s is not available!%s\n", deployment.Name, deployment.Namespace, message)
			}

			if errorToPrint != "" {
				fmt.Print(errorToPrint)
				return false, err
			}
		}

		for _, condition := range deployment.Status.Conditions {
			if condition.Type != appsv1.DeploymentAvailable &&
				condition.Type != appsv1.DeploymentReplicaFailure &&
				condition.Type != appsv1.DeploymentProgressing {
				fmt.Printf("Note: Unhandled deployment condition %s", condition.Type)
			}
		}
	}
	fmt.Printf("OK\n")
	return true, nil
}

func checkPods(opts *options.Options) (bool, error) {
	fmt.Printf("Checking pods... ")
	client := helpers.MustKubeClient()
	pods, err := client.CoreV1().Pods(opts.Metadata.Namespace).List(metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	for _, pod := range pods.Items {
		for _, condition := range pod.Status.Conditions {
			var errorToPrint string
			var message string

			if condition.Message != "" {
				message = fmt.Sprintf(" Message: %s", condition.Message)
			}

			// if condition is not met and the pod is not completed
			conditionNotMet := condition.Status != corev1.ConditionTrue && condition.Reason != "PodCompleted"

			// possible condition types listed at https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-conditions
			switch condition.Type {
			case corev1.PodScheduled:
				if conditionNotMet {
					errorToPrint = fmt.Sprintf("Pod %s in namespace %s is not yet scheduled!%s\n", pod.Name, pod.Namespace, message)
				}
			case corev1.PodReady:
				if conditionNotMet {
					errorToPrint = fmt.Sprintf("Pod %s in namespace %s is not ready!%s\n", pod.Name, pod.Namespace, message)
				}
			case corev1.PodInitialized:
				if conditionNotMet {
					errorToPrint = fmt.Sprintf("Pod %s in namespace %s is not yet initialized!%s\n", pod.Name, pod.Namespace, message)
				}
			case corev1.PodReasonUnschedulable:
				if conditionNotMet {
					errorToPrint = fmt.Sprintf("Pod %s in namespace %s is unschedulable!%s\n", pod.Name, pod.Namespace, message)
				}
			case corev1.ContainersReady:
				if conditionNotMet {
					errorToPrint = fmt.Sprintf("Not all containers in pod %s in namespace %s are ready!%s\n", pod.Name, pod.Namespace, message)
				}
			default:
				fmt.Printf("Note: Unhandled pod condition %s", condition.Type)
			}

			if errorToPrint != "" {
				fmt.Print(errorToPrint)
				return false, err
			}
		}
	}
	fmt.Printf("OK\n")
	return true, nil
}

func getSettings(opts *options.Options) (*v1.Settings, error) {
	client := helpers.MustSettingsClient()
	return client.Read(opts.Metadata.Namespace, defaults.SettingsName, clients.ReadOpts{})
}

func getNamespaces(settings *v1.Settings) ([]string, error) {
	if settings.WatchNamespaces != nil {
		return settings.WatchNamespaces, nil
	}

	return helpers.GetNamespaces()
}

func checkUpstreams(namespaces []string) ([]string, bool, error) {
	fmt.Printf("Checking upstreams... ")
	client := helpers.MustUpstreamClient()
	var knownUpstreams []string
	for _, ns := range namespaces {
		upstreams, err := client.List(ns, clients.ListOpts{})
		if err != nil {
			return nil, false, err
		}
		for _, upstream := range upstreams {
			if upstream.Status.GetState() == core.Status_Rejected {
				fmt.Printf("Found rejected upstream: %s\n", renderMetadata(upstream.GetMetadata()))
				fmt.Printf("Reason: %s", upstream.Status.Reason)
				return nil, false, nil
			}
			knownUpstreams = append(knownUpstreams, renderMetadata(upstream.GetMetadata()))
		}
	}
	fmt.Printf("OK\n")
	return knownUpstreams, true, nil
}

func checkUpstreamGroups(namespaces []string) (bool, error) {
	fmt.Printf("Checking upstream groups... ")
	client := helpers.MustUpstreamGroupClient()
	for _, ns := range namespaces {
		upstreamGroups, err := client.List(ns, clients.ListOpts{})
		if err != nil {
			return false, err
		}
		for _, upstreamGroup := range upstreamGroups {
			if upstreamGroup.Status.GetState() == core.Status_Rejected {
				fmt.Printf("Found rejected upstream group: %s\n", renderMetadata(upstreamGroup.GetMetadata()))
				fmt.Printf("Reason: %s", upstreamGroup.Status.Reason)
				return false, nil
			}
		}
	}
	fmt.Printf("OK\n")
	return true, nil
}

func checkVirtualServices(namespaces, knownUpstreams []string) (bool, error) {
	fmt.Printf("Checking virtual services... ")
	client := helpers.MustVirtualServiceClient()
	for _, ns := range namespaces {
		virtualServices, err := client.List(ns, clients.ListOpts{})
		if err != nil {
			return false, err
		}
		for _, virtualService := range virtualServices {
			if virtualService.Status.GetState() == core.Status_Rejected {
				fmt.Printf("Found rejected virtual service: %s\n", renderMetadata(virtualService.GetMetadata()))
				fmt.Printf("Reason: %s", virtualService.Status.GetReason())
				return false, nil
			}
			for _, route := range virtualService.GetVirtualHost().GetRoutes() {
				if route.GetRouteAction() != nil {
					if route.GetRouteAction().GetSingle() != nil {
						us := route.GetRouteAction().GetSingle()
						if us.GetUpstream() != nil {
							if !cliutils.Contains(knownUpstreams, renderRef(us.GetUpstream())) {
								fmt.Printf("Virtual service references unknown upstream:\n")
								fmt.Printf("  Virtual service: %s\n", renderMetadata(virtualService.GetMetadata()))
								fmt.Printf("  Upstream: %s\n", renderRef(us.GetUpstream()))
								return false, nil
							}
						}
					}
				}
			}
		}
	}
	fmt.Printf("OK\n")
	return true, nil
}

func checkGateways(namespaces []string) (bool, error) {
	fmt.Printf("Checking gateways... ")
	client := helpers.MustGatewayClient()
	for _, ns := range namespaces {
		gateways, err := client.List(ns, clients.ListOpts{})
		if err != nil {
			return false, err
		}
		for _, gateway := range gateways {
			if gateway.Status.GetState() == core.Status_Rejected {
				fmt.Printf("Found rejected gateway: %s\n", renderMetadata(gateway.GetMetadata()))
				fmt.Printf("Reason: %s", gateway.Status.Reason)
				return false, nil
			}
		}
	}
	fmt.Printf("OK\n")
	return true, nil
}

func checkProxies(namespaces []string, glooNamespace string) (bool, error) {
	fmt.Printf("Checking proxies... ")
	client := helpers.MustProxyClient()
	for _, ns := range namespaces {
		proxies, err := client.List(ns, clients.ListOpts{})
		if err != nil {
			return false, err
		}
		for _, proxy := range proxies {
			if proxy.Status.GetState() == core.Status_Rejected {
				fmt.Printf("Found rejected proxy: %s\n", renderMetadata(proxy.GetMetadata()))
				fmt.Printf("Reason: %s", proxy.Status.Reason)
				return false, nil
			}
		}
	}
	// check if any proxy instances are out of sync with the Gloo control plane
	errMessage := "Problem while checking for out of sync proxies"

	// port-forward gateway-proxy deployment and get prometheus metrics
	freePort, err := cliutil.GetFreePort()
	if err != nil {
		fmt.Println(errMessage)
		return false, err
	}
	localPort := strconv.Itoa(freePort)
	adminPort := strconv.Itoa(int(defaults.EnvoyAdminPort))
	promStatsPath := "/stats/prometheus"
	stats, portFwdCmd, err := cliutil.PortForwardGet(glooNamespace, "deploy/gateway-proxy",
		localPort, adminPort, false, promStatsPath)
	if portFwdCmd.Process != nil {
		defer portFwdCmd.Process.Release()
		defer portFwdCmd.Process.Kill()
	}
	if err != nil {
		fmt.Println(errMessage)
		return false, err
	}

	if strings.TrimSpace(stats) == "" {
		fmt.Println(errMessage+": could not find any metrics at", promStatsPath, "endpoint of the gateway-proxy deployment")
		return false, nil
	}

	if !strings.Contains(stats, "envoy_control_plane_connected_state{} 1") {
		fmt.Println("Your gateway-proxy is out of sync with the Gloo control plane and is not receiving valid gloo config. " +
			"You may want to try looking at your gloo or gateway-proxy logs or using the `glooctl debug log` command.")
		return false, nil
	}

	// wait for metrics to update
	time.Sleep(time.Millisecond * 250)

	// gather metrics again
	res, err := http.Get("http://localhost:" + localPort + promStatsPath)
	if err != nil {
		fmt.Println(errMessage)
		return false, err
	}
	if res.StatusCode != 200 {
		fmt.Println(errMessage+": received unexpected status code", res.StatusCode, "from", promStatsPath, "endpoint of the gateway-proxy deployment")
		return false, nil
	}
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(errMessage)
		return false, err
	}
	res.Body.Close()
	newStats := string(b)

	if strings.TrimSpace(newStats) == "" {
		fmt.Println(errMessage+": could not find any metrics at", promStatsPath, "endpoint of the gateway-proxy deployment")
		return false, nil
	}

	desiredMetricsSegments := []string{"update_attempt", "update_rejected", "update_failure"}
	statsMap := parseMetrics(stats, desiredMetricsSegments, promStatsPath)
	newStatsMap := parseMetrics(newStats, desiredMetricsSegments, promStatsPath)

	if reflect.DeepEqual(newStatsMap, statsMap) {
		fmt.Printf("OK\n")
		return true, nil
	}

	for k, oldVal := range statsMap {
		if newVal, ok := newStatsMap[k]; ok && strings.Contains(k, "attempt") && newVal > oldVal {
			// at least one attempt for this counter- check if any were rejected or failed
			rejectedMetric := strings.Replace(k, "attempt", "rejected", -1)
			newRejected, newOk := newStatsMap[rejectedMetric]
			oldRejected, oldOk := statsMap[rejectedMetric]
			if newOk && oldOk && newRejected > oldRejected {
				fmt.Printf("An update to your gateway-proxy deployment was rejected due to schema/validation errors. The %v metric increased.\n"+
					"You may want to try using the `glooctl proxy logs` or `glooctl debug logs` commands.\n", rejectedMetric)
				return false, nil
			}
			failureMetric := strings.Replace(k, "attempt", "failure", -1)
			newFailure, newOk := newStatsMap[failureMetric]
			oldFailure, oldOk := statsMap[failureMetric]
			if newOk && oldOk && newFailure > oldFailure {
				fmt.Printf("An update to your gateway-proxy deployment was rejected due to network errors. The %v metric increased.\n"+
					"You may want to try using the `glooctl proxy logs` or `glooctl debug logs` commands.\n", failureMetric)
				return false, nil
			}
		}
	}

	fmt.Printf("OK\n")
	return true, nil
}

func checkSecrets(namespaces []string) (bool, error) {
	fmt.Printf("Checking secrets... ")
	client := helpers.MustSecretClient()
	for _, ns := range namespaces {
		_, err := client.List(ns, clients.ListOpts{})
		if err != nil {
			return false, err
		}
		// currently this would only find syntax errors
	}
	fmt.Printf("OK\n")
	return true, nil
}

func renderMetadata(metadata core.Metadata) string {
	return renderNamespaceName(metadata.Namespace, metadata.Name)
}

func renderRef(ref *core.ResourceRef) string {
	return renderNamespaceName(ref.Namespace, ref.Name)
}

func renderNamespaceName(namespace, name string) string {
	return fmt.Sprintf("%s %s", namespace, name)
}

// parseMetrics parses prometheus metrics and returns a map from the metric name and labels to its value.
// It expects to only look for int values!
func parseMetrics(stats string, desiredMetricSegments []string, promStatsPath string) map[string]int {
	statsMap := make(map[string]int)
	statsLines := strings.Split(stats, "\n")
	for _, line := range statsLines {
		trimLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimLine, "#") || trimLine == "" {
			continue // Ignore comments, help text, type info, empty lines (https://github.com/prometheus/docs/blob/master/content/docs/instrumenting/exposition_formats.md#comments-help-text-and-type-information)
		}
		desiredMetric := false
		for _, s := range desiredMetricSegments {
			if strings.Contains(trimLine, s) {
				desiredMetric = true
			}
		}
		if desiredMetric {
			pieces := strings.Fields(trimLine) // split by white spaces
			metric := strings.Join(pieces[0:len(pieces)-1], "")
			metricVal, err := strconv.Atoi(pieces[len(pieces)-1])
			if err != nil {
				fmt.Printf("Found an unexpected format in metrics at %v endpoint of the gateway-proxy deployment. "+
					"Expected %v metric to have an int value but got value %v.\nContinuing check...", promStatsPath, metric, pieces[len(pieces)-1])
				continue
			}
			statsMap[metric] = metricVal
		}
	}
	return statsMap
}

// Checks whether the cluster that the kubeconfig points at is available
// The timeout for the kubernetes client is set to a low value to notify the user of the failure
func checkConnection() error {
	client, err := helpers.GetKubernetesClientWithTimeout(5 * time.Second)
	if err != nil {
		return eris.Wrapf(err, "Could not get kubernetes client")
	}
	_, err = client.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		return eris.Wrapf(err, "Could not communicate with kubernetes cluster")
	}
	return nil
}
