package check

import (
	"context"
	"errors"
	"fmt"
	"time"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"

	"github.com/hashicorp/go-multierror"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
		Long:  "usage: glooctl check [-o FORMAT]",
		RunE: func(cmd *cobra.Command, args []string) error {

			if opts.Top.Output.IsTable() || opts.Top.Output.IsJSON() {

				checkResponse, err := CheckResources(opts)

				if opts.Top.Output.IsTable() {

					if err != nil {
						// Not returning error here because this shouldn't propagate as a standard CLI error, which prints usage.
						return err
					} else {
						fmt.Printf("No problems detected.\n")
					}
				} else {
					printers.PrintChecks(checkResponse, opts.Top.Output)
				}

				CheckMulticlusterResources(opts)
			} else {
				fmt.Printf("Error: Invalid Output Type - Only TABLE (DEFAULT) and JSON Supported!")
			}
			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddOutputFlag(pflags, &opts.Top.Output)
	flagutils.AddNamespaceFlag(pflags, &opts.Metadata.Namespace)
	flagutils.AddExcludecheckFlag(pflags, &opts.Top.CheckName)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func CheckResources(opts *options.Options) (printers.CheckResponse, error) {
	var checkResponse printers.CheckResponse
	var multiErr *multierror.Error

	err := checkConnection(opts.Top.Ctx, opts.Metadata.GetNamespace())
	if err != nil {
		multiErr = multierror.Append(multiErr, err)
		return checkResponse, multiErr
	}

	var deploymentsStatus = printers.CheckStatus{Name: "deployments"}
	deployments, err, errCount := getAndCheckDeployments(opts)
	if err != nil {
		multiErr = multierror.Append(multiErr, err)
		deploymentsStatus.Status = fmt.Sprint(errCount) + " Errors!"
	} else {
		deploymentsStatus.Status = "OK"
	}
	checkResponse.Resource = append(checkResponse.Resource, deploymentsStatus)

	var podsStatus = printers.CheckStatus{Name: "pods"}
	includePods := doesNotContain(opts.Top.CheckName, "pods")
	if includePods {
		err, errCount := checkPods(opts)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
			podsStatus.Status = fmt.Sprint(errCount) + " Errors!"
		} else {
			podsStatus.Status = "OK"
		}
	}
	checkResponse.Resource = append(checkResponse.Resource, podsStatus)

	settings, err := getSettings(opts)
	if err != nil {
		multiErr = multierror.Append(multiErr, err)
	}

	namespaces, err := getNamespaces(opts.Top.Ctx, settings)
	if err != nil {
		multiErr = multierror.Append(multiErr, err)
	}

	var upstreamsStatus = printers.CheckStatus{Name: "upstreams"}
	knownUpstreams, err, errCount := checkUpstreams(opts, namespaces)
	if err != nil {
		multiErr = multierror.Append(multiErr, err)
		upstreamsStatus.Status = fmt.Sprint(errCount) + " Errors!"
	} else {
		upstreamsStatus.Status = "OK"
	}
	checkResponse.Resource = append(checkResponse.Resource, upstreamsStatus)

	var upstreamGroupsStatus = printers.CheckStatus{Name: "upstream groups"}
	includeUpstreamGroup := doesNotContain(opts.Top.CheckName, "upstreamgroup")
	if includeUpstreamGroup {
		err, errCount := checkUpstreamGroups(opts, namespaces)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
			upstreamGroupsStatus.Status = fmt.Sprint(errCount) + " Errors!"
		} else {
			upstreamGroupsStatus.Status = "OK"
		}
		checkResponse.Resource = append(checkResponse.Resource, upstreamGroupsStatus)
	}

	var authConfigsStatus = printers.CheckStatus{Name: "auth configs"}
	knownAuthConfigs, err, errCount := checkAuthConfigs(opts, namespaces)
	if err != nil {
		multiErr = multierror.Append(multiErr, err)
		authConfigsStatus.Status = fmt.Sprint(errCount) + " Errors!"
	} else {
		authConfigsStatus.Status = "OK"
	}
	checkResponse.Resource = append(checkResponse.Resource, authConfigsStatus)

	var rateLimitConfigsStatus = printers.CheckStatus{Name: "rate limit configs"}
	knownRateLimitConfigs, err, errCount := checkRateLimitConfigs(opts, namespaces)
	if err != nil {
		multiErr = multierror.Append(multiErr, err)
		rateLimitConfigsStatus.Status = fmt.Sprint(errCount) + " Errors!"
	} else {
		rateLimitConfigsStatus.Status = "OK"
	}
	checkResponse.Resource = append(checkResponse.Resource, rateLimitConfigsStatus)

	var virtualHostOptionsStatus = printers.CheckStatus{Name: "VirtualHostOptions"}
	knownVirtualHostOptions, err, errCount := checkVirtualHostOptions(opts, namespaces)
	if err != nil {
		multiErr = multierror.Append(multiErr, err)
		virtualHostOptionsStatus.Status = fmt.Sprint(errCount) + " Errors!"
	} else {
		virtualHostOptionsStatus.Status = "OK"
	}
	checkResponse.Resource = append(checkResponse.Resource, virtualHostOptionsStatus)

	var routeOptionsStatus = printers.CheckStatus{Name: "RouteOptions"}
	knownRouteOptions, err, errCount := checkRouteOptions(opts, namespaces)
	if err != nil {
		multiErr = multierror.Append(multiErr, err)
		routeOptionsStatus.Status = fmt.Sprint(errCount) + " Errors!"
	} else {
		routeOptionsStatus.Status = "OK"
	}
	checkResponse.Resource = append(checkResponse.Resource, routeOptionsStatus)

	includeSecrets := doesNotContain(opts.Top.CheckName, "secrets")
	if includeSecrets {
		var secretsStatus = printers.CheckStatus{Name: "secrets"}
		err, errCount := checkSecrets(opts, namespaces)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
			secretsStatus.Status = fmt.Sprint(errCount) + " Errors!"
		} else {
			secretsStatus.Status = "OK"
		}
		checkResponse.Resource = append(checkResponse.Resource, secretsStatus)
	}

	var virtualServicesStatus = printers.CheckStatus{Name: "virtual services"}
	err, errCount = checkVirtualServices(opts, namespaces, knownUpstreams, knownAuthConfigs, knownRateLimitConfigs, knownVirtualHostOptions, knownRouteOptions)
	if err != nil {
		multiErr = multierror.Append(multiErr, err)
		virtualServicesStatus.Status = fmt.Sprint(errCount) + " Errors!"
	} else {
		virtualServicesStatus.Status = "OK"
	}
	checkResponse.Resource = append(checkResponse.Resource, virtualServicesStatus)

	includeGateway := doesNotContain(opts.Top.CheckName, "gateways")
	if includeGateway {
		var gatewaysStatus = printers.CheckStatus{Name: "gateways"}
		err, errCount := checkGateways(opts, namespaces)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
			gatewaysStatus.Status = fmt.Sprint(errCount) + " Errors!"
		} else {
			gatewaysStatus.Status = "OK"
		}
		checkResponse.Resource = append(checkResponse.Resource, gatewaysStatus)
	}

	includeProxy := doesNotContain(opts.Top.CheckName, "proxies")
	if includeProxy {
		var proxiesStatus = printers.CheckStatus{Name: "proxies"}
		err, errCount := checkProxies(opts, namespaces, opts.Metadata.GetNamespace(), deployments)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
			proxiesStatus.Status = fmt.Sprint(errCount) + " Errors!"
		} else {
			proxiesStatus.Status = "OK"
		}
		checkResponse.Resource = append(checkResponse.Resource, proxiesStatus)
	}

	includePrometheusStatsCheck := doesNotContain(opts.Top.CheckName, "xds-metrics")
	if includePrometheusStatsCheck {
		err = checkXdsMetrics(opts.Top.Ctx, opts.Metadata.GetNamespace(), deployments)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}

	for _, e := range multiErr.Errors {
		checkResponse.Errors = append(checkResponse.Errors, fmt.Sprint(e))
	}

	return checkResponse, multiErr.ErrorOrNil()
}

func getAndCheckDeployments(opts *options.Options) (*appsv1.DeploymentList, error, int) {
	if opts.Top.Output.IsTable() {
		fmt.Printf("Checking deployments... ")
	}

	client := helpers.MustKubeClient()
	_, err := client.CoreV1().Namespaces().Get(opts.Top.Ctx, opts.Metadata.GetNamespace(), metav1.GetOptions{})
	if err != nil {
		errMessage := "Gloo namespace does not exist"
		fmt.Println(errMessage)
		return nil, fmt.Errorf(errMessage), 0
	}
	deployments, err := client.AppsV1().Deployments(opts.Metadata.GetNamespace()).List(opts.Top.Ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err, 0
	}
	if len(deployments.Items) == 0 {
		errMessage := "Gloo is not installed"
		fmt.Println(errMessage)
		return nil, fmt.Errorf(errMessage), 0
	}
	var multiErr *multierror.Error
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
				err := fmt.Errorf("Deployment %s in namespace %s failed to create pods!%s", deployment.Name, deployment.Namespace, message)
				multiErr = multierror.Append(multiErr, err)
			}
		}

		for _, condition := range deployment.Status.Conditions {
			setMessage(condition)
			if condition.Type == appsv1.DeploymentProgressing && condition.Status != corev1.ConditionTrue {
				err := fmt.Errorf("Deployment %s in namespace %s is not progressing!%s", deployment.Name, deployment.Namespace, message)
				multiErr = multierror.Append(multiErr, err)
			}
		}

		for _, condition := range deployment.Status.Conditions {
			setMessage(condition)
			if condition.Type == appsv1.DeploymentAvailable && condition.Status != corev1.ConditionTrue {
				err := fmt.Errorf("Deployment %s in namespace %s is not available!%s", deployment.Name, deployment.Namespace, message)
				multiErr = multierror.Append(multiErr, err)
			}

		}

		for _, condition := range deployment.Status.Conditions {
			if condition.Type != appsv1.DeploymentAvailable &&
				condition.Type != appsv1.DeploymentReplicaFailure &&
				condition.Type != appsv1.DeploymentProgressing {
				err := fmt.Errorf("Deployment %s has an unhandled deployment condition %s", deployment.Name, condition.Type)
				multiErr = multierror.Append(multiErr, err)
			}
		}
	}
	if multiErr != nil {
		fmt.Printf("%v Errors!\n", multiErr.Len())
		return nil, multiErr, multiErr.Len()
	}

	if opts.Top.Output.IsTable() {
		fmt.Printf("OK\n")
	}

	return deployments, nil, 0
}

func checkPods(opts *options.Options) (error, int) {
	if opts.Top.Output.IsTable() {
		fmt.Printf("Checking pods... ")
	}

	client := helpers.MustKubeClient()
	pods, err := client.CoreV1().Pods(opts.Metadata.GetNamespace()).List(opts.Top.Ctx, metav1.ListOptions{})
	if err != nil {
		return err, 1
	}
	var multiErr *multierror.Error
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
					errorToPrint = fmt.Sprintf("Pod %s in namespace %s is not yet scheduled!%s", pod.Name, pod.Namespace, message)
				}
			case corev1.PodReady:
				if conditionNotMet {
					errorToPrint = fmt.Sprintf("Pod %s in namespace %s is not ready!%s", pod.Name, pod.Namespace, message)
				}
			case corev1.PodInitialized:
				if conditionNotMet {
					errorToPrint = fmt.Sprintf("Pod %s in namespace %s is not yet initialized!%s", pod.Name, pod.Namespace, message)
				}
			case corev1.PodReasonUnschedulable:
				if conditionNotMet {
					errorToPrint = fmt.Sprintf("Pod %s in namespace %s is unschedulable!%s", pod.Name, pod.Namespace, message)
				}
			case corev1.ContainersReady:
				if conditionNotMet {
					errorToPrint = fmt.Sprintf("Not all containers in pod %s in namespace %s are ready!%s", pod.Name, pod.Namespace, message)
				}
			default:
				fmt.Printf("Note: Unhandled pod condition %s", condition.Type)
			}

			if errorToPrint != "" {
				multiErr = multierror.Append(multiErr, fmt.Errorf(errorToPrint))
			}
		}
	}
	if multiErr != nil {
		fmt.Printf("%v Errors!\n", multiErr.Len())
		return multiErr, multiErr.Len()
	}

	if opts.Top.Output.IsTable() {
		fmt.Printf("OK\n")
	}

	return nil, 0
}

func getSettings(opts *options.Options) (*v1.Settings, error) {
	client := helpers.MustNamespacedSettingsClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
	return client.Read(opts.Metadata.GetNamespace(), defaults.SettingsName, clients.ReadOpts{})
}

func getNamespaces(ctx context.Context, settings *v1.Settings) ([]string, error) {
	if settings.GetWatchNamespaces() != nil {
		return settings.GetWatchNamespaces(), nil
	}

	return helpers.GetNamespaces(ctx)
}

func checkUpstreams(opts *options.Options, namespaces []string) ([]string, error, int) {
	if opts.Top.Output.IsTable() {
		fmt.Printf("Checking upstreams... ")
	}

	var knownUpstreams []string
	var multiErr *multierror.Error
	for _, ns := range namespaces {
		upstreams, err := helpers.MustNamespacedUpstreamClient(opts.Top.Ctx, ns).List(ns, clients.ListOpts{})
		if err != nil {
			return nil, err, 1
		}
		for _, upstream := range upstreams {
			if upstream.GetStatus().GetState() == core.Status_Rejected {
				errMessage := fmt.Sprintf("Found rejected upstream: %s ", renderMetadata(upstream.GetMetadata()))
				errMessage += fmt.Sprintf("(Reason: %s)", upstream.GetStatus().GetReason())
				multiErr = multierror.Append(multiErr, errors.New(errMessage))
			}
			if upstream.GetStatus().GetState() == core.Status_Warning {
				errMessage := fmt.Sprintf("Found upstream with warnings: %s ", renderMetadata(upstream.GetMetadata()))
				errMessage += fmt.Sprintf("(Reason: %s)", upstream.GetStatus().GetReason())
				multiErr = multierror.Append(multiErr, errors.New(errMessage))
			}
			knownUpstreams = append(knownUpstreams, renderMetadata(upstream.GetMetadata()))
		}
	}
	if multiErr != nil {
		fmt.Printf("%v Errors!\n", multiErr.Len())
		return nil, multiErr, multiErr.Len()
	}
	if opts.Top.Output.IsTable() {
		fmt.Printf("OK\n")
	}

	return knownUpstreams, nil, 0
}

func checkUpstreamGroups(opts *options.Options, namespaces []string) (error, int) {
	if opts.Top.Output.IsTable() {
		fmt.Printf("Checking upstream groups... ")
	}

	var multiErr *multierror.Error
	for _, ns := range namespaces {
		upstreamGroups, err := helpers.MustNamespacedUpstreamGroupClient(opts.Top.Ctx, ns).List(ns, clients.ListOpts{})
		if err != nil {
			return err, 1
		}
		for _, upstreamGroup := range upstreamGroups {
			if upstreamGroup.GetStatus().GetState() == core.Status_Rejected {
				errMessage := fmt.Sprintf("Found rejected upstream group: %s ", renderMetadata(upstreamGroup.GetMetadata()))
				errMessage += fmt.Sprintf("(Reason: %s)", upstreamGroup.GetStatus().GetReason())
				multiErr = multierror.Append(multiErr, errors.New(errMessage))
			}
			if upstreamGroup.GetStatus().GetState() == core.Status_Warning {
				errMessage := fmt.Sprintf("Found upstream group with warnings: %s ", renderMetadata(upstreamGroup.GetMetadata()))
				errMessage += fmt.Sprintf("(Reason: %s)", upstreamGroup.GetStatus().GetReason())
				multiErr = multierror.Append(multiErr, errors.New(errMessage))
			}
		}
	}
	if multiErr != nil {
		fmt.Printf("%v Errors!\n", multiErr.Len())
		return multiErr, multiErr.Len()
	}

	if opts.Top.Output.IsTable() {
		fmt.Printf("OK\n")
	}

	return nil, 0
}

func checkAuthConfigs(opts *options.Options, namespaces []string) ([]string, error, int) {
	if opts.Top.Output.IsTable() {
		fmt.Printf("Checking auth configs... ")
	}

	var knownAuthConfigs []string
	var multiErr *multierror.Error
	for _, ns := range namespaces {
		authConfigs, err := helpers.MustNamespacedAuthConfigClient(opts.Top.Ctx, ns).List(ns, clients.ListOpts{})
		if err != nil {
			return nil, err, 1
		}
		for _, authConfig := range authConfigs {
			if authConfig.GetStatus().GetState() == core.Status_Rejected {
				errMessage := fmt.Sprintf("Found rejected auth config: %s ", renderMetadata(authConfig.GetMetadata()))
				errMessage += fmt.Sprintf("(Reason: %s)", authConfig.GetStatus().GetReason())
				multiErr = multierror.Append(multiErr, errors.New(errMessage))
			} else if authConfig.GetStatus().GetState() == core.Status_Warning {
				errMessage := fmt.Sprintf("Found auth config with warnings: %s ", renderMetadata(authConfig.GetMetadata()))
				errMessage += fmt.Sprintf("(Reason: %s)", authConfig.GetStatus().GetReason())
				multiErr = multierror.Append(multiErr, errors.New(errMessage))
			}
			knownAuthConfigs = append(knownAuthConfigs, renderMetadata(authConfig.GetMetadata()))
		}
	}
	if multiErr != nil {
		fmt.Printf("%v Errors!\n", multiErr.Len())
		return nil, multiErr, multiErr.Len()
	}

	if opts.Top.Output.IsTable() {
		fmt.Printf("OK\n")
	}

	return knownAuthConfigs, nil, 0
}

func checkRateLimitConfigs(opts *options.Options, namespaces []string) ([]string, error, int) {
	if opts.Top.Output.IsTable() {
		fmt.Printf("Checking rate limit configs... ")
	}

	var knownConfigs []string
	var multiErr *multierror.Error
	for _, ns := range namespaces {

		rlcClient, err := helpers.RateLimitConfigClient(opts.Top.Ctx, []string{ns})
		if err != nil {
			if isCrdNotFoundErr(ratelimit.RateLimitConfigCrd, err) {
				// Just warn. If the CRD is required, the check would have failed on the crashing gloo/gloo-ee pod.
				fmt.Printf("WARN: %s\n", CrdNotFoundErr(ratelimit.RateLimitConfigCrd.KindName).Error())
				return nil, nil, 0
			}
			return nil, err, 1
		}

		configs, err := rlcClient.List(ns, clients.ListOpts{})
		if err != nil {
			return nil, err, 1
		}
		for _, config := range configs {
			if config.Status.GetState() == v1alpha1.RateLimitConfigStatus_REJECTED {
				errMessage := fmt.Sprintf("Found rejected rate limit config: %s ", renderMetadata(config.GetMetadata()))
				errMessage += fmt.Sprintf("(Reason: %s)", config.Status.GetMessage())
				multiErr = multierror.Append(multiErr, fmt.Errorf(errMessage))
			}
			knownConfigs = append(knownConfigs, renderMetadata(config.GetMetadata()))
		}
	}

	if multiErr != nil {
		fmt.Printf("%v Errors!\n", multiErr.Len())
		return nil, multiErr, multiErr.Len()
	}

	if opts.Top.Output.IsTable() {
		fmt.Printf("OK\n")
	}

	return knownConfigs, nil, 0
}

func checkVirtualHostOptions(opts *options.Options, namespaces []string) ([]string, error, int) {
	if opts.Top.Output.IsTable() {
		fmt.Printf("Checking VirtualHostOptions... ")
	}

	var knownVhOpts []string
	var multiErr *multierror.Error
	for _, ns := range namespaces {
		vhoptClient, err := helpers.VirtualHostOptionClient(opts.Top.Ctx, []string{ns})
		if err != nil {
			if isCrdNotFoundErr(gatewayv1.VirtualHostOptionCrd, err) {
				// Just warn. If the CRD is required, the check would have failed on the crashing gloo/gloo-ee pod.
				fmt.Printf("WARN: %s\n", CrdNotFoundErr(gatewayv1.VirtualHostOptionCrd.KindName).Error())
				return nil, nil, 0
			}
			return nil, err, 1
		}
		vhOpts, err := vhoptClient.List(ns, clients.ListOpts{})
		if err != nil {
			return nil, err, 1
		}
		for _, vhOpt := range vhOpts {
			if vhOpt.GetStatus().GetState() == core.Status_Rejected {
				errMessage := fmt.Sprintf("Found rejected VirtualHostOption: %s ", renderMetadata(vhOpt.GetMetadata()))
				errMessage += fmt.Sprintf("(Reason: %s)", vhOpt.GetStatus().GetReason())
				multiErr = multierror.Append(multiErr, errors.New(errMessage))
			} else if vhOpt.GetStatus().GetState() == core.Status_Warning {
				errMessage := fmt.Sprintf("Found VirtualHostOption with warnings: %s ", renderMetadata(vhOpt.GetMetadata()))
				errMessage += fmt.Sprintf("(Reason: %s)", vhOpt.GetStatus().GetReason())
				multiErr = multierror.Append(multiErr, errors.New(errMessage))
			}
			knownVhOpts = append(knownVhOpts, renderMetadata(vhOpt.GetMetadata()))
		}
	}
	if multiErr != nil {
		fmt.Printf("%v Errors!\n", multiErr.Len())
		return nil, multiErr, multiErr.Len()
	}

	if opts.Top.Output.IsTable() {
		fmt.Printf("OK\n")
	}

	return knownVhOpts, nil, 0
}

func checkRouteOptions(opts *options.Options, namespaces []string) ([]string, error, int) {
	if opts.Top.Output.IsTable() {
		fmt.Printf("Checking RouteOptions... ")
	}

	var knownVhOpts []string
	var multiErr *multierror.Error
	for _, ns := range namespaces {
		routeOptionClient, err := helpers.RouteOptionClient(opts.Top.Ctx, []string{ns})
		if err != nil {
			if isCrdNotFoundErr(gatewayv1.RouteOptionCrd, err) {
				// Just warn. If the CRD is required, the check would have failed on the crashing gloo/gloo-ee pod.
				fmt.Printf("WARN: %s\n", CrdNotFoundErr(gatewayv1.RouteOptionCrd.KindName).Error())
				return nil, nil, 0
			}
			return nil, err, 1
		}
		vhOpts, err := routeOptionClient.List(ns, clients.ListOpts{})
		if err != nil {
			return nil, err, 1
		}
		for _, routeOpt := range vhOpts {
			if routeOpt.GetStatus().GetState() == core.Status_Rejected {
				errMessage := fmt.Sprintf("Found rejected RouteOption: %s ", renderMetadata(routeOpt.GetMetadata()))
				errMessage += fmt.Sprintf("(Reason: %s)", routeOpt.GetStatus().GetReason())
				multiErr = multierror.Append(multiErr, errors.New(errMessage))
			} else if routeOpt.GetStatus().GetState() == core.Status_Warning {
				errMessage := fmt.Sprintf("Found RouteOption with warnings: %s ", renderMetadata(routeOpt.GetMetadata()))
				errMessage += fmt.Sprintf("(Reason: %s)", routeOpt.GetStatus().GetReason())
				multiErr = multierror.Append(multiErr, errors.New(errMessage))
			}
			knownVhOpts = append(knownVhOpts, renderMetadata(routeOpt.GetMetadata()))
		}
	}
	if multiErr != nil {
		fmt.Printf("%v Errors!\n", multiErr.Len())
		return nil, multiErr, multiErr.Len()
	}

	if opts.Top.Output.IsTable() {
		fmt.Printf("OK\n")
	}

	return knownVhOpts, nil, 0
}

func checkVirtualServices(opts *options.Options, namespaces, knownUpstreams, knownAuthConfigs, knownRateLimitConfigs, knownVirtualHostOptions, knownRouteOptions []string) (error, int) {
	if opts.Top.Output.IsTable() {
		fmt.Printf("Checking virtual services... ")
	}

	var multiErr *multierror.Error

	for _, ns := range namespaces {
		virtualServices, err := helpers.MustNamespacedVirtualServiceClient(opts.Top.Ctx, ns).List(ns, clients.ListOpts{})
		if err != nil {
			return err, 1
		}
		for _, virtualService := range virtualServices {
			if virtualService.GetStatus().GetState() == core.Status_Rejected {
				errMessage := fmt.Sprintf("Found rejected virtual service: %s ", renderMetadata(virtualService.GetMetadata()))
				errMessage += fmt.Sprintf("(Reason: %s)", virtualService.GetStatus().GetReason())
				multiErr = multierror.Append(multiErr, fmt.Errorf(errMessage))
			}
			if virtualService.GetStatus().GetState() == core.Status_Warning {
				errMessage := fmt.Sprintf("Found virtual service with warnings: %s ", renderMetadata(virtualService.GetMetadata()))
				errMessage += fmt.Sprintf("(Reason: %s)", virtualService.GetStatus().GetReason())
				multiErr = multierror.Append(multiErr, fmt.Errorf(errMessage))
			}
			for _, route := range virtualService.GetVirtualHost().GetRoutes() {
				if route.GetRouteAction() != nil {
					if route.GetRouteAction().GetSingle() != nil {
						us := route.GetRouteAction().GetSingle()
						if us.GetUpstream() != nil {
							if !cliutils.Contains(knownUpstreams, renderRef(us.GetUpstream())) {
								//TODO warning message if using rejected or warning upstream
								errMessage := fmt.Sprintf("Virtual service references unknown upstream: ")
								errMessage += fmt.Sprintf("(Virtual service: %s", renderMetadata(virtualService.GetMetadata()))
								errMessage += fmt.Sprintf(" | Upstream: %s)", renderRef(us.GetUpstream()))
								multiErr = multierror.Append(multiErr, fmt.Errorf(errMessage))
							}
						}
					}
				}
			}

			// Check references to auth configs
			isAuthConfigRefValid := func(knownConfigs []string, ref *core.ResourceRef) error {
				// If the virtual service points to a specific, non-existent authconfig, it is not valid.
				if ref != nil && !cliutils.Contains(knownConfigs, renderRef(ref)) {
					//TODO: Virtual service references rejected or warning auth config
					errMessage := fmt.Sprintf("Virtual service references unknown auth config:\n")
					errMessage += fmt.Sprintf("  Virtual service: %s\n", renderMetadata(virtualService.GetMetadata()))
					errMessage += fmt.Sprintf("  Auth Config: %s\n", renderRef(ref))
					return fmt.Errorf(errMessage)
				}
				return nil
			}
			isOptionsRefValid := func(knownOptions []string, refs []*core.ResourceRef) error {
				// If the virtual host points to a specifc, non-existent VirtualHostOption, it is not valid.
				for _, ref := range refs {
					if ref != nil && !cliutils.Contains(knownOptions, renderRef(ref)) {
						errMessage := fmt.Sprintf("Virtual service references unknown VirtualHostOption:\n")
						errMessage += fmt.Sprintf("  Virtual service: %s\n", renderMetadata(virtualService.GetMetadata()))
						errMessage += fmt.Sprintf("  VirtualHostOption: %s\n", renderRef(ref))
						return fmt.Errorf(errMessage)
					}
				}
				return nil
			}
			// Check virtual host options
			if err := isAuthConfigRefValid(knownAuthConfigs, virtualService.GetVirtualHost().GetOptions().GetExtauth().GetConfigRef()); err != nil {
				multiErr = multierror.Append(multiErr, err)
			}
			vhDelegateOptions := virtualService.GetVirtualHost().GetOptionsConfigRefs().GetDelegateOptions()
			if err := isOptionsRefValid(knownVirtualHostOptions, vhDelegateOptions); err != nil {
				multiErr = multierror.Append(multiErr, err)
			}

			// Check route options
			for _, route := range virtualService.GetVirtualHost().GetRoutes() {
				if err := isAuthConfigRefValid(knownAuthConfigs, route.GetOptions().GetExtauth().GetConfigRef()); err != nil {
					multiErr = multierror.Append(multiErr, err)
				}
				if err := isOptionsRefValid(knownRouteOptions, route.GetOptionsConfigRefs().GetDelegateOptions()); err != nil {
					multiErr = multierror.Append(multiErr, err)
				}
				// Check weighted destination options
				for _, weightedDest := range route.GetRouteAction().GetMulti().GetDestinations() {
					if err := isAuthConfigRefValid(knownAuthConfigs, weightedDest.GetOptions().GetExtauth().GetConfigRef()); err != nil {
						multiErr = multierror.Append(multiErr, err)
					}
				}
			}

			// Check references to rate limit configs
			isRateLimitConfigRefValid := func(knownConfigs []string, ref *rlopts.RateLimitConfigRef) error {
				resourceRef := &core.ResourceRef{
					Name:      ref.GetName(),
					Namespace: ref.GetNamespace(),
				}
				if !cliutils.Contains(knownConfigs, renderRef(resourceRef)) {
					//TODO: check if references rate limit config with error or warning
					errMessage := fmt.Sprintf("Virtual service references unknown rate limit config:\n")
					errMessage += fmt.Sprintf("  Virtual service: %s\n", renderMetadata(virtualService.GetMetadata()))
					errMessage += fmt.Sprintf("  Rate Limit Config: %s\n", renderRef(resourceRef))
					return fmt.Errorf(errMessage)
				}
				return nil
			}
			// Check virtual host options
			for _, ref := range virtualService.GetVirtualHost().GetOptions().GetRateLimitConfigs().GetRefs() {
				if err := isRateLimitConfigRefValid(knownRateLimitConfigs, ref); err != nil {
					multiErr = multierror.Append(multiErr, err)
				}
			}
			// Check route options
			for _, route := range virtualService.GetVirtualHost().GetRoutes() {
				for _, ref := range route.GetOptions().GetRateLimitConfigs().GetRefs() {
					if err := isRateLimitConfigRefValid(knownRateLimitConfigs, ref); err != nil {
						multiErr = multierror.Append(multiErr, err)
					}
				}
			}
		}
	}

	if multiErr != nil {
		fmt.Printf("%v Errors!\n", multiErr.Len())
		return multiErr, multiErr.Len()
	}

	if opts.Top.Output.IsTable() {
		fmt.Printf("OK\n")
	}

	return nil, 0
}

func checkGateways(opts *options.Options, namespaces []string) (error, int) {
	if opts.Top.Output.IsTable() {
		fmt.Printf("Checking gateways... ")
	}

	var multiErr *multierror.Error
	for _, ns := range namespaces {
		gateways, err := helpers.MustNamespacedGatewayClient(opts.Top.Ctx, ns).List(ns, clients.ListOpts{})
		if err != nil {
			return err, 1
		}
		for _, gateway := range gateways {
			if gateway.GetStatus().GetState() == core.Status_Rejected {
				errMessage := fmt.Sprintf("Found rejected gateway: %s\n", renderMetadata(gateway.GetMetadata()))
				errMessage += fmt.Sprintf("Reason: %s\n", gateway.GetStatus().GetReason())
				multiErr = multierror.Append(multiErr, fmt.Errorf(errMessage))
			}
			if gateway.GetStatus().GetState() == core.Status_Warning {
				errMessage := fmt.Sprintf("Found gateway with warnings: %s\n", renderMetadata(gateway.GetMetadata()))
				errMessage += fmt.Sprintf("Reason: %s\n", gateway.GetStatus().GetReason())
				multiErr = multierror.Append(multiErr, fmt.Errorf(errMessage))
			}
		}
	}

	if multiErr != nil {
		fmt.Printf("%v Errors!\n", multiErr.Len())
		return multiErr, multiErr.Len()
	}

	if opts.Top.Output.IsTable() {
		fmt.Printf("OK\n")
	}

	return nil, 0
}

func checkProxies(opts *options.Options, namespaces []string, glooNamespace string, deployments *appsv1.DeploymentList) (error, int) {
	if opts.Top.Output.IsTable() {
		fmt.Printf("Checking proxies... ")
	}

	if deployments == nil {
		fmt.Println("Skipping due to an error in checking deployments")
		return fmt.Errorf("proxy check was skipped due to an error in checking deployments"), 1
	}
	var multiErr *multierror.Error
	for _, ns := range namespaces {
		proxies, err := helpers.MustNamespacedProxyClient(opts.Top.Ctx, ns).List(ns, clients.ListOpts{})
		if err != nil {
			return err, 1
		}
		for _, proxy := range proxies {
			if proxy.GetStatus().GetState() == core.Status_Rejected {
				errMessage := fmt.Sprintf("Found rejected proxy: %s\n", renderMetadata(proxy.GetMetadata()))
				errMessage += fmt.Sprintf("Reason: %s\n", proxy.GetStatus().GetReason())
				multiErr = multierror.Append(multiErr, fmt.Errorf(errMessage))
			}
			if proxy.GetStatus().GetState() == core.Status_Warning {
				errMessage := fmt.Sprintf("Found proxy with warnings: %s\n", renderMetadata(proxy.GetMetadata()))
				errMessage += fmt.Sprintf("Reason: %s\n", proxy.GetStatus().GetReason())
				multiErr = multierror.Append(multiErr, fmt.Errorf(errMessage))
			}
		}
	}

	if err := checkProxiesPromStats(opts.Top.Ctx, glooNamespace, deployments); err != nil {
		multiErr = multierror.Append(multiErr, err)
	}

	if multiErr != nil {
		fmt.Printf("%v Errors!\n", multiErr.Len())
		return multiErr, multiErr.Len()
	}

	if opts.Top.Output.IsTable() {
		fmt.Printf("OK\n")
	}

	return nil, 0
}

func checkSecrets(opts *options.Options, namespaces []string) (error, int) {
	if opts.Top.Output.IsTable() {
		fmt.Printf("Checking secrets... ")
	}

	var multiErr *multierror.Error
	client := helpers.MustSecretClientWithOptions(opts.Top.Ctx, 5*time.Second, namespaces)

	for _, ns := range namespaces {
		_, err := client.List(ns, clients.ListOpts{})
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
		// currently this would only find syntax errors
	}
	if multiErr != nil {
		fmt.Printf("%v Errors!\n", multiErr.Len())
		return multiErr, multiErr.Len()
	}

	if opts.Top.Output.IsTable() {
		fmt.Printf("OK\n")
	}

	return nil, 0
}

func renderMetadata(metadata *core.Metadata) string {
	return renderNamespaceName(metadata.GetNamespace(), metadata.GetName())
}

func renderRef(ref *core.ResourceRef) string {
	return renderNamespaceName(ref.GetNamespace(), ref.GetName())
}

func renderNamespaceName(namespace, name string) string {
	return fmt.Sprintf("%s %s", namespace, name)
}

// Checks whether the cluster that the kubeconfig points at is available
// The timeout for the kubernetes client is set to a low value to notify the user of the failure
func checkConnection(ctx context.Context, ns string) error {
	client, err := helpers.GetKubernetesClientWithTimeout(5 * time.Second)
	if err != nil {
		return eris.Wrapf(err, "Could not get kubernetes client")
	}
	_, err = client.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	if err != nil {
		return eris.Wrapf(err, "Could not communicate with kubernetes cluster")
	}
	return nil
}

func isCrdNotFoundErr(crd crd.Crd, err error) bool {
	for {
		if statusErr, ok := err.(*apierrors.StatusError); ok {
			if apierrors.IsNotFound(err) &&
				statusErr.ErrStatus.Details != nil &&
				statusErr.ErrStatus.Details.Kind == crd.Plural {
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
