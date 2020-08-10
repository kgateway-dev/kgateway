package check

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	ratelimit "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	rlopts "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	CrdNotFoundErr = func(crdName string) error {
		return eris.Errorf("%s CRD has not been registered", crdName)
	}
)

// contains method
func doesNotContain(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return false
		}
	}
	return true
}

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
			CheckMulticlusterResources(opts)
			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddNamespaceFlag(pflags, &opts.Metadata.Namespace)
	flagutils.AddExcludecheckFlag(pflags, &opts.Top.CheckName)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func CheckResources(opts *options.Options) (bool, error) {
	err := checkConnection(opts.Metadata.Namespace)
	if err != nil {
		return false, err
	}

	deployments, ok, err := getAndCheckDeployments(opts)
	if !ok || err != nil {
		return ok, err
	}

	includePods := doesNotContain(opts.Top.CheckName, "pods")
	if includePods {
		ok, err := checkPods(opts)
		if !ok || err != nil {
			return ok, err
		}
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

	includeUpstreamGroup := doesNotContain(opts.Top.CheckName, "upstreamgroup")
	if includeUpstreamGroup {
		ok, err := checkUpstreamGroups(namespaces)
		if !ok || err != nil {
			return ok, err
		}
	}

	knownAuthConfigs, ok, err := checkAuthConfigs(namespaces)
	if !ok || err != nil {
		return ok, err
	}

	knownRateLimitConfigs, ok, err := checkRateLimitConfigs(namespaces)
	if !ok || err != nil {
		return ok, err
	}

	includeSecrets := doesNotContain(opts.Top.CheckName, "secrets")
	if includeSecrets {
		ok, err := checkSecrets(namespaces)
		if !ok || err != nil {
			return ok, err
		}
	}

	ok, err = checkVirtualServices(namespaces, knownUpstreams, knownAuthConfigs, knownRateLimitConfigs)
	if !ok || err != nil {
		return ok, err
	}

	includeGateway := doesNotContain(opts.Top.CheckName, "gateways")
	if includeGateway {
		ok, err := checkGateways(namespaces)
		if !ok || err != nil {
			return ok, err
		}
	}

	includeProxy := doesNotContain(opts.Top.CheckName, "proxies")
	if includeProxy {
		ok, err := checkProxies(opts.Top.Ctx, namespaces, opts.Metadata.Namespace, deployments)
		if !ok || err != nil {
			return ok, err
		}
	}

	ok, err = checkGlooePromStats(opts.Top.Ctx, opts.Metadata.Namespace, deployments)
	if !ok || err != nil {
		return ok, err
	}
	return true, nil
}

func getAndCheckDeployments(opts *options.Options) (*appsv1.DeploymentList, bool, error) {
	fmt.Printf("Checking deployments... ")
	client := helpers.MustKubeClient()
	_, err := client.CoreV1().Namespaces().Get(opts.Metadata.Namespace, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("Gloo namespace does not exist\n")
		return nil, false, err
	}
	deployments, err := client.AppsV1().Deployments(opts.Metadata.Namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, false, err
	}
	if len(deployments.Items) == 0 {
		fmt.Printf("Gloo is not installed\n")
		return nil, false, nil
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
				return nil, false, err
			}
		}

		for _, condition := range deployment.Status.Conditions {
			setMessage(condition)
			if condition.Type == appsv1.DeploymentProgressing && condition.Status != corev1.ConditionTrue {
				errorToPrint = fmt.Sprintf("Deployment %s in namespace %s is not progressing!%s\n", deployment.Name, deployment.Namespace, message)
			}

			if errorToPrint != "" {
				fmt.Print(errorToPrint)
				return nil, false, err
			}
		}

		for _, condition := range deployment.Status.Conditions {
			setMessage(condition)
			if condition.Type == appsv1.DeploymentAvailable && condition.Status != corev1.ConditionTrue {
				errorToPrint = fmt.Sprintf("Deployment %s in namespace %s is not available!%s\n", deployment.Name, deployment.Namespace, message)
			}

			if errorToPrint != "" {
				fmt.Print(errorToPrint)
				return nil, false, err
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
	return deployments, true, nil
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
	client := helpers.MustNamespacedSettingsClient(opts.Metadata.GetNamespace())
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
	var knownUpstreams []string
	for _, ns := range namespaces {
		upstreams, err := helpers.MustNamespacedUpstreamClient(ns).List(ns, clients.ListOpts{})
		if err != nil {
			return nil, false, err
		}
		for _, upstream := range upstreams {
			if upstream.Status.GetState() == core.Status_Rejected {
				fmt.Printf("Found rejected upstream: %s\n", renderMetadata(upstream.GetMetadata()))
				fmt.Printf("Reason: %s\n", upstream.Status.Reason)
				return nil, false, nil
			}
			if upstream.Status.GetState() == core.Status_Warning {
				fmt.Printf("Found upstream with warnings: %s\n", renderMetadata(upstream.GetMetadata()))
				fmt.Printf("Reason: %s\n", upstream.Status.Reason)
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
	for _, ns := range namespaces {
		upstreamGroups, err := helpers.MustNamespacedUpstreamGroupClient(ns).List(ns, clients.ListOpts{})
		if err != nil {
			return false, err
		}
		for _, upstreamGroup := range upstreamGroups {
			if upstreamGroup.Status.GetState() == core.Status_Rejected {
				fmt.Printf("Found rejected upstream group: %s\n", renderMetadata(upstreamGroup.GetMetadata()))
				fmt.Printf("Reason: %s\n", upstreamGroup.Status.Reason)
				return false, nil
			}
			if upstreamGroup.Status.GetState() == core.Status_Warning {
				fmt.Printf("Found upstream group with warnings: %s\n", renderMetadata(upstreamGroup.GetMetadata()))
				fmt.Printf("Reason: %s\n", upstreamGroup.Status.Reason)
				return false, nil
			}
		}
	}
	fmt.Printf("OK\n")
	return true, nil
}

func checkAuthConfigs(namespaces []string) ([]string, bool, error) {
	fmt.Printf("Checking auth configs... ")
	var knownAuthConfigs []string
	for _, ns := range namespaces {
		authConfigs, err := helpers.MustNamespacedAuthConfigClient(ns).List(ns, clients.ListOpts{})
		if err != nil {
			return nil, false, err
		}
		for _, authConfig := range authConfigs {
			if authConfig.Status.GetState() == core.Status_Rejected {
				fmt.Printf("Found rejected auth config: %s\n", renderMetadata(authConfig.GetMetadata()))
				fmt.Printf("Reason: %s\n", authConfig.Status.Reason)
				return nil, false, nil
			}
			if authConfig.Status.GetState() == core.Status_Warning {
				fmt.Printf("Found auth config with warnings: %s\n", renderMetadata(authConfig.GetMetadata()))
				fmt.Printf("Reason: %s\n", authConfig.Status.Reason)
				return nil, false, nil
			}
			knownAuthConfigs = append(knownAuthConfigs, renderMetadata(authConfig.GetMetadata()))
		}
	}
	fmt.Printf("OK\n")
	return knownAuthConfigs, true, nil
}

func checkRateLimitConfigs(namespaces []string) ([]string, bool, error) {
	fmt.Printf("Checking rate limit configs... ")
	var knownConfigs []string
	for _, ns := range namespaces {

		rlcClient, err := helpers.RateLimitConfigClient([]string{ns})
		if err != nil {
			if isCrdNotFoundErr(err) {
				// Just warn. If the CRD is required, the check would have failed on the crashing gloo/gloo-ee pod.
				fmt.Printf("WARN: %s\n", CrdNotFoundErr(ratelimit.RateLimitConfigCrd.KindName).Error())
				return nil, true, nil
			}
			return nil, false, err
		}

		configs, err := rlcClient.List(ns, clients.ListOpts{})
		if err != nil {
			return nil, false, err
		}
		for _, config := range configs {
			if config.Status.GetState() == v1alpha1.RateLimitConfigStatus_REJECTED {
				fmt.Printf("Found rejected rate limit config: %s\n", renderMetadata(config.GetMetadata()))
				fmt.Printf("Reason: %s\n", config.Status.Message)
				return nil, false, nil
			}
			knownConfigs = append(knownConfigs, renderMetadata(config.GetMetadata()))
		}
	}
	fmt.Printf("OK\n")
	return knownConfigs, true, nil
}

func checkVirtualServices(namespaces, knownUpstreams, knownAuthConfigs, knownRateLimitConfigs []string) (bool, error) {
	fmt.Printf("Checking virtual services... ")
	for _, ns := range namespaces {
		virtualServices, err := helpers.MustNamespacedVirtualServiceClient(ns).List(ns, clients.ListOpts{})
		if err != nil {
			return false, err
		}
		for _, virtualService := range virtualServices {
			if virtualService.Status.GetState() == core.Status_Rejected {
				fmt.Printf("Found rejected virtual service: %s\n", renderMetadata(virtualService.GetMetadata()))
				fmt.Printf("Reason: %s\n", virtualService.Status.GetReason())
				return false, nil
			}
			if virtualService.Status.GetState() == core.Status_Warning {
				fmt.Printf("Found virtual service with warnings: %s\n", renderMetadata(virtualService.GetMetadata()))
				fmt.Printf("Reason: %s\n", virtualService.Status.GetReason())
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

			// Check references to auth configs
			isAuthConfigRefValid := func(knownConfigs []string, ref *core.ResourceRef) bool {
				// If the virtual service points to a specific, non-existent authconfig, it is not valid.
				if ref != nil && !cliutils.Contains(knownConfigs, renderRef(ref)) {
					fmt.Printf("Virtual service references unknown auth config:\n")
					fmt.Printf("  Virtual service: %s\n", renderMetadata(virtualService.GetMetadata()))
					fmt.Printf("  Auth Config: %s\n", renderRef(ref))
					return false
				}
				return true
			}
			// Check virtual host options
			if !isAuthConfigRefValid(knownAuthConfigs, virtualService.GetVirtualHost().GetOptions().GetExtauth().GetConfigRef()) {
				return false, nil
			}
			// Check route options
			for _, route := range virtualService.GetVirtualHost().GetRoutes() {
				if !isAuthConfigRefValid(knownAuthConfigs, route.GetOptions().GetExtauth().GetConfigRef()) {
					return false, nil
				}
				// Check weighted destination options
				for _, weightedDest := range route.GetRouteAction().GetMulti().GetDestinations() {
					if !isAuthConfigRefValid(knownAuthConfigs, weightedDest.GetOptions().GetExtauth().GetConfigRef()) {
						return false, nil
					}
				}
			}

			// Check references to rate limit configs
			isRateLimitConfigRefValid := func(knownConfigs []string, ref *rlopts.RateLimitConfigRef) bool {
				resourceRef := &core.ResourceRef{
					Name:      ref.Name,
					Namespace: ref.Namespace,
				}
				if !cliutils.Contains(knownConfigs, renderRef(resourceRef)) {
					fmt.Printf("Virtual service references unknown rate limit config:\n")
					fmt.Printf("  Virtual service: %s\n", renderMetadata(virtualService.GetMetadata()))
					fmt.Printf("  Rate Limit Config: %s\n", renderRef(resourceRef))
					return false
				}
				return true
			}
			// Check virtual host options
			for _, ref := range virtualService.GetVirtualHost().GetOptions().GetRateLimitConfigs().GetRefs() {
				if !isRateLimitConfigRefValid(knownRateLimitConfigs, ref) {
					return false, nil
				}
			}
			// Check route options
			for _, route := range virtualService.GetVirtualHost().GetRoutes() {
				for _, ref := range route.GetOptions().GetRateLimitConfigs().GetRefs() {
					if !isRateLimitConfigRefValid(knownRateLimitConfigs, ref) {
						return false, nil
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
	for _, ns := range namespaces {
		gateways, err := helpers.MustNamespacedGatewayClient(ns).List(ns, clients.ListOpts{})
		if err != nil {
			return false, err
		}
		for _, gateway := range gateways {
			if gateway.Status.GetState() == core.Status_Rejected {
				fmt.Printf("Found rejected gateway: %s\n", renderMetadata(gateway.GetMetadata()))
				fmt.Printf("Reason: %s\n", gateway.Status.Reason)
				return false, nil
			}
			if gateway.Status.GetState() == core.Status_Warning {
				fmt.Printf("Found gateway with warnings: %s\n", renderMetadata(gateway.GetMetadata()))
				fmt.Printf("Reason: %s\n", gateway.Status.Reason)
				return false, nil
			}
		}
	}
	fmt.Printf("OK\n")
	return true, nil
}

func checkProxies(ctx context.Context, namespaces []string, glooNamespace string, deployments *appsv1.DeploymentList) (bool, error) {
	fmt.Printf("Checking proxies... ")
	for _, ns := range namespaces {
		proxies, err := helpers.MustNamespacedProxyClient(ns).List(ns, clients.ListOpts{})
		if err != nil {
			return false, err
		}
		for _, proxy := range proxies {
			if proxy.Status.GetState() == core.Status_Rejected {
				fmt.Printf("Found rejected proxy: %s\n", renderMetadata(proxy.GetMetadata()))
				fmt.Printf("Reason: %s\n", proxy.Status.Reason)
				return false, nil
			}
			if proxy.Status.GetState() == core.Status_Warning {
				fmt.Printf("Found proxy with warnings: %s\n", renderMetadata(proxy.GetMetadata()))
				fmt.Printf("Reason: %s\n", proxy.Status.Reason)
				return false, nil
			}
		}
	}

	return checkProxiesPromStats(ctx, glooNamespace, deployments)
}

func checkSecrets(namespaces []string) (bool, error) {
	fmt.Printf("Checking secrets... ")
	client := helpers.MustSecretClientWithOptions(5*time.Second, namespaces)

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

// Checks whether the cluster that the kubeconfig points at is available
// The timeout for the kubernetes client is set to a low value to notify the user of the failure
func checkConnection(ns string) error {
	client, err := helpers.GetKubernetesClientWithTimeout(5 * time.Second)
	if err != nil {
		return eris.Wrapf(err, "Could not get kubernetes client")
	}
	_, err = client.CoreV1().Namespaces().Get(ns, metav1.GetOptions{})
	if err != nil {
		return eris.Wrapf(err, "Could not communicate with kubernetes cluster")
	}
	return nil
}

func isCrdNotFoundErr(err error) bool {
	for {
		if statusErr, ok := err.(*errors.StatusError); ok {
			if errors.IsNotFound(err) &&
				statusErr.ErrStatus.Details != nil &&
				statusErr.ErrStatus.Details.Kind == ratelimit.RateLimitConfigCrd.Plural {
				return true
			}
			return false
		}

		// This works for "github.com/pkg/errors"-based errors as well
		if wrappedErr := eris.Unwrap(err); wrappedErr != nil {
			err = wrappedErr
			continue
		}
		return false
	}
}
