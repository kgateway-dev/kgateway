package check

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	"github.com/hashicorp/go-multierror"
	"github.com/rotisserie/eris"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/common"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	ratelimit "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	rlopts "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	"github.com/solo-io/go-utils/cliutils"
	rlv1alpha1 "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
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
			ctx, cancel := context.WithCancel(opts.Top.Ctx)

			if opts.Check.CheckTimeout != 0 {
				ctx, cancel = context.WithTimeout(opts.Top.Ctx, opts.Check.CheckTimeout)
			}
			defer cancel()

			if !opts.Top.Output.IsTable() && !opts.Top.Output.IsJSON() {
				return errors.New("Invalid output type. Only table (default) and json are supported.")
			}

			printer := printers.P{OutputType: opts.Top.Output}
			printer.CheckResult = printer.NewCheckResult()

			var multiErr *multierror.Error

			// check edge gateway resources
			err := CheckResources(ctx, printer, opts)
			if err != nil {
				multiErr = multierror.Append(multiErr, err)
			}

			// check kubernetes gateway resources
			err = CheckKubeGatewayResources(ctx, printer, opts)
			if err != nil {
				multiErr = multierror.Append(multiErr, err)
			}

			// check gloo fed resources (TODO this should return errors too)
			CheckMulticlusterResources(ctx, printer, opts)

			if multiErr.ErrorOrNil() != nil {
				for _, err := range multiErr.Errors {
					printer.AppendError(fmt.Sprint(err))
				}

				// when output type is table, the returned errors get printed directly as cli output.
				if opts.Top.Output.IsTable() {
					return multiErr.ErrorOrNil()
				}
			} else {
				printer.AppendMessage("No problems detected.")
			}

			// when output type is json, any errors added via printer.AppendError will show up in the output here.
			if opts.Top.Output.IsJSON() {
				printer.PrintChecks(new(bytes.Buffer))
			}

			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddCheckOutputFlag(pflags, &opts.Top.Output)
	flagutils.AddNamespaceFlag(pflags, &opts.Metadata.Namespace)
	flagutils.AddPodSelectorFlag(pflags, &opts.Top.PodSelector)
	flagutils.AddResourceNamespaceFlag(pflags, &opts.Top.ResourceNamespaces)
	flagutils.AddExcludeCheckFlag(pflags, &opts.Top.CheckName)
	flagutils.AddReadOnlyFlag(pflags, &opts.Top.ReadOnly)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func CheckResources(ctx context.Context, printer printers.P, opts *options.Options) error {
	var multiErr *multierror.Error

	err := checkConnection(ctx, opts)
	if err != nil {
		multiErr = multierror.Append(multiErr, err)
		return multiErr
	}

	var deployments *appsv1.DeploymentList
	deploymentsIncluded := doesNotContain(opts.Top.CheckName, constants.Deployments)
	if deploymentsIncluded {
		deployments, err = getAndCheckDeployments(ctx, printer, opts)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}
	// Fetch the gloo deployment name even if check deployments is disabled as it is used in other checks
	customGlooDeploymentName, err = helpers.GetGlooDeploymentName(opts.Top.Ctx, opts.Metadata.GetNamespace())
	if err != nil {
		multiErr = multierror.Append(multiErr, err)
	}

	if included := doesNotContain(opts.Top.CheckName, constants.Pods); included {
		err := checkPods(ctx, printer, opts)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}

	settings, err := common.GetSettings(opts)
	if err != nil {
		multiErr = multierror.Append(multiErr, err)
	}

	namespaces, err := getNamespaces(ctx, settings)
	if err != nil {
		multiErr = multierror.Append(multiErr, err)
	}

	// Intersect resource-namespaces flag args and watched namespaces
	if len(opts.Top.ResourceNamespaces) != 0 {
		newNamespaces := []string{}
		for _, flaggedNamespace := range opts.Top.ResourceNamespaces {
			for _, watchedNamespace := range namespaces {
				if flaggedNamespace == watchedNamespace {
					newNamespaces = append(newNamespaces, watchedNamespace)
				}
			}
		}
		namespaces = newNamespaces
		if len(newNamespaces) == 0 {
			multiErr = multierror.Append(multiErr, eris.New("No namespaces specified are currently being watched (defaulting to '"+opts.Metadata.GetNamespace()+"' namespace)"))
			namespaces = []string{opts.Metadata.GetNamespace()}
		}
	}

	var knownUpstreams []string
	if included := doesNotContain(opts.Top.CheckName, constants.Upstreams); included {
		knownUpstreams, err = checkUpstreams(ctx, printer, opts, namespaces)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}

	if included := doesNotContain(opts.Top.CheckName, constants.UpstreamGroup); included {
		err := checkUpstreamGroups(ctx, printer, opts, namespaces)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}

	var knownAuthConfigs []string
	if included := doesNotContain(opts.Top.CheckName, constants.AuthConfigs); included {
		knownAuthConfigs, err = checkAuthConfigs(ctx, printer, opts, namespaces)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}

	var knownRateLimitConfigs []string
	if included := doesNotContain(opts.Top.CheckName, constants.RateLimitConfigs); included {
		knownRateLimitConfigs, err = checkRateLimitConfigs(ctx, printer, opts, namespaces)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}

	var knownVirtualHostOptions []string
	if included := doesNotContain(opts.Top.CheckName, constants.VirtualHostOptions); included {
		knownVirtualHostOptions, err = checkVirtualHostOptions(ctx, printer, opts, namespaces)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}

	var knownRouteOptions []string
	if included := doesNotContain(opts.Top.CheckName, constants.RouteOptions); included {
		knownRouteOptions, err = checkRouteOptions(ctx, printer, opts, namespaces)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}

	if included := doesNotContain(opts.Top.CheckName, constants.Secrets); included {
		err := checkSecrets(ctx, printer, opts, namespaces)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}

	if included := doesNotContain(opts.Top.CheckName, constants.VirtualServices); included {
		err = checkVirtualServices(ctx, printer, opts, namespaces, knownUpstreams, knownAuthConfigs, knownRateLimitConfigs, knownVirtualHostOptions, knownRouteOptions)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}

	if included := doesNotContain(opts.Top.CheckName, constants.Gateways); included {
		err := checkGateways(ctx, printer, opts, namespaces)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}

	if included := doesNotContain(opts.Top.CheckName, constants.Proxies); included {
		err := checkProxies(ctx, printer, opts, namespaces, opts.Metadata.GetNamespace(), deployments, deploymentsIncluded, settings)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}

	if included := doesNotContain(opts.Top.CheckName, constants.XDSMetrics); included {
		err = checkXdsMetrics(ctx, printer, opts, deployments)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}

	return multiErr.ErrorOrNil()
}

func getAndCheckDeployments(ctx context.Context, printer printers.P, opts *options.Options) (*appsv1.DeploymentList, error) {
	printer.AppendCheck("Checking Deployments... ")
	client, err := helpers.GetKubernetesClient(opts.Top.KubeContext)
	if err != nil {
		errMessage := "error getting KubeClient"
		fmt.Println(errMessage)
		return nil, fmt.Errorf(errMessage+": %v", err)
	}
	_, err = client.CoreV1().Namespaces().Get(ctx, opts.Metadata.GetNamespace(), metav1.GetOptions{})
	if err != nil {
		errMessage := "Gloo namespace does not exist"
		fmt.Println(errMessage)
		return nil, fmt.Errorf(errMessage)
	}
	deployments, err := client.AppsV1().Deployments(opts.Metadata.GetNamespace()).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(deployments.Items) == 0 {
		errMessage := "Gloo is not installed"
		fmt.Println(errMessage)
		return nil, fmt.Errorf(errMessage)
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
		printer.AppendStatus("Deployments", fmt.Sprintf("%v Errors!", multiErr.Len()))
		return nil, multiErr
	}
	printer.AppendStatus("Deployments", "OK")
	return deployments, nil
}

func checkPods(ctx context.Context, printer printers.P, opts *options.Options) error {
	printer.AppendCheck("Checking Pods... ")
	client, err := helpers.GetKubernetesClient(opts.Top.KubeContext)
	if err != nil {
		return err
	}
	pods, err := client.CoreV1().Pods(opts.Metadata.GetNamespace()).List(ctx, metav1.ListOptions{
		LabelSelector: opts.Top.PodSelector,
	})
	if err != nil {
		return err
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
			case corev1.PodReadyToStartContainers:
				// This condition was introduced in k8s 1.29. Skip it since completed jobs have Status=False for this condition
			default:
				fmt.Printf("Note: Unhandled pod condition %s\n", condition.Type)
			}

			if errorToPrint != "" {
				multiErr = multierror.Append(multiErr, fmt.Errorf(errorToPrint))
			}
		}
	}
	if multiErr != nil {
		printer.AppendStatus("Pods", fmt.Sprintf("%v Errors!", multiErr.Len()))
		return multiErr
	}
	if len(pods.Items) == 0 {
		printer.AppendMessage("Warning: The provided label selector (" + opts.Top.PodSelector + ") applies to no pods")
	} else {
		printer.AppendStatus("Pods", "OK")
	}
	return nil
}

func getNamespaces(ctx context.Context, settings *v1.Settings) ([]string, error) {
	if settings.GetWatchNamespaces() != nil {
		return settings.GetWatchNamespaces(), nil
	}
	return helpers.GetNamespaces(ctx)
}

func checkUpstreams(ctx context.Context, printer printers.P, _ *options.Options, namespaces []string) ([]string, error) {
	printer.AppendCheck("Checking Upstreams... ")
	var knownUpstreams []string
	var multiErr *multierror.Error
	client, err := helpers.UpstreamClient(ctx, namespaces)
	for _, ns := range namespaces {
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
			continue
		}
		upstreams, err := client.List(ns, clients.ListOpts{})
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
			continue
		}
		for _, upstream := range upstreams {
			if upstream.GetNamespacedStatuses() != nil {
				namespacedStatuses := upstream.GetNamespacedStatuses()
				for reporter, status := range namespacedStatuses.GetStatuses() {
					switch status.GetState() {
					case core.Status_Rejected:
						errMessage := fmt.Sprintf("Found rejected upstream by '%s': %s ", reporter, renderMetadata(upstream.GetMetadata()))
						errMessage += fmt.Sprintf("(Reason: %s)", status.GetReason())
						multiErr = multierror.Append(multiErr, errors.New(errMessage))
					case core.Status_Warning:
						errMessage := fmt.Sprintf("Found upstream with warnings by '%s': %s ", reporter, renderMetadata(upstream.GetMetadata()))
						errMessage += fmt.Sprintf("(Reason: %s)", status.GetReason())
						multiErr = multierror.Append(multiErr, errors.New(errMessage))
					}
				}
			} else {
				errMessage := fmt.Sprintf("Found upstream with no status: %s\n", renderMetadata(upstream.GetMetadata()))
				multiErr = multierror.Append(multiErr, errors.New(errMessage))
			}
			knownUpstreams = append(knownUpstreams, renderMetadata(upstream.GetMetadata()))
		}
	}
	if multiErr != nil {
		printer.AppendStatus("Upstreams", fmt.Sprintf("%v Errors!", multiErr.Len()))
		return knownUpstreams, multiErr
	}
	printer.AppendStatus("Upstreams", "OK")
	return knownUpstreams, nil
}

func checkUpstreamGroups(ctx context.Context, printer printers.P, _ *options.Options, namespaces []string) error {
	printer.AppendCheck("Checking UpstreamGroups... ")
	var multiErr *multierror.Error
	upstreamGroupClient, err := helpers.UpstreamGroupClient(ctx, namespaces)
	for _, ns := range namespaces {
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
			continue
		}
		upstreamGroups, err := upstreamGroupClient.List(ns, clients.ListOpts{})
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
			continue
		}
		if err != nil {
			return err
		}
		for _, upstreamGroup := range upstreamGroups {
			if upstreamGroup.GetNamespacedStatuses() != nil {
				namespacedStatuses := upstreamGroup.GetNamespacedStatuses()
				for reporter, status := range namespacedStatuses.GetStatuses() {
					switch status.GetState() {
					case core.Status_Rejected:
						errMessage := fmt.Sprintf("Found rejected upstream group by '%s': %s ", reporter, renderMetadata(upstreamGroup.GetMetadata()))
						errMessage += fmt.Sprintf("(Reason: %s)", status.GetReason())
						multiErr = multierror.Append(multiErr, errors.New(errMessage))
					case core.Status_Warning:
						errMessage := fmt.Sprintf("Found upstream group with warnings by '%s': %s ", reporter, renderMetadata(upstreamGroup.GetMetadata()))
						errMessage += fmt.Sprintf("(Reason: %s)", status.GetReason())
						multiErr = multierror.Append(multiErr, errors.New(errMessage))
					}
				}
			} else {
				errMessage := fmt.Sprintf("Found upstream group with no status: %s\n", renderMetadata(upstreamGroup.GetMetadata()))
				multiErr = multierror.Append(multiErr, errors.New(errMessage))
			}
		}
	}
	if multiErr != nil {
		printer.AppendStatus("UpstreamGroups", fmt.Sprintf("%v Errors!", multiErr.Len()))
		return multiErr
	}
	printer.AppendStatus("UpstreamGroups", "OK")
	return nil
}

func checkAuthConfigs(ctx context.Context, printer printers.P, _ *options.Options, namespaces []string) ([]string, error) {
	printer.AppendCheck("Checking AuthConfigs... ")
	var knownAuthConfigs []string
	var multiErr *multierror.Error
	authConfigClient, err := helpers.AuthConfigClient(ctx, namespaces)
	for _, ns := range namespaces {
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
			continue
		}
		authConfigs, err := authConfigClient.List(ns, clients.ListOpts{})
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
			continue
		}
		for _, authConfig := range authConfigs {
			if authConfig.GetNamespacedStatuses() != nil {
				namespacedStatuses := authConfig.GetNamespacedStatuses()
				for reporter, status := range namespacedStatuses.GetStatuses() {
					switch status.GetState() {
					case core.Status_Rejected:
						errMessage := fmt.Sprintf("Found rejected auth config by '%s': %s ", reporter, renderMetadata(authConfig.GetMetadata()))
						errMessage += fmt.Sprintf("(Reason: %s)", status.GetReason())
						multiErr = multierror.Append(multiErr, errors.New(errMessage))
					case core.Status_Warning:
						errMessage := fmt.Sprintf("Found auth config with warnings by '%s': %s ", reporter, renderMetadata(authConfig.GetMetadata()))
						errMessage += fmt.Sprintf("(Reason: %s)", status.GetReason())
						multiErr = multierror.Append(multiErr, errors.New(errMessage))
					}
				}
			} else {
				errMessage := fmt.Sprintf("Found auth config with no status: %s\n", renderMetadata(authConfig.GetMetadata()))
				multiErr = multierror.Append(multiErr, errors.New(errMessage))
			}
			knownAuthConfigs = append(knownAuthConfigs, renderMetadata(authConfig.GetMetadata()))
		}
	}
	if multiErr != nil {
		printer.AppendStatus("AuthConfigs", fmt.Sprintf("%v Errors!", multiErr.Len()))
		return knownAuthConfigs, multiErr
	}
	printer.AppendStatus("AuthConfigs", "OK")
	return knownAuthConfigs, nil
}

func checkRateLimitConfigs(ctx context.Context, printer printers.P, _ *options.Options, namespaces []string) ([]string, error) {
	printer.AppendCheck("Checking RateLimitConfigs... ")
	var knownConfigs []string
	var multiErr *multierror.Error
	rlcClient, err := helpers.RateLimitConfigClient(ctx, namespaces)
	for _, ns := range namespaces {
		if err != nil {
			if isCrdNotFoundErr(ratelimit.RateLimitConfigCrd, err) {
				// Just warn. If the CRD is required, the check would have failed on the crashing gloo/gloo-ee pod.
				fmt.Printf("WARN: %s\n", CrdNotFoundErr(ratelimit.RateLimitConfigCrd.KindName).Error())
				return nil, nil
			}
			return nil, err
		}

		configs, err := rlcClient.List(ns, clients.ListOpts{})
		if err != nil {
			return nil, err
		}
		for _, config := range configs {
			if config.Status.GetState() == rlv1alpha1.RateLimitConfigStatus_REJECTED {
				errMessage := fmt.Sprintf("Found rejected rate limit config: %s ", renderMetadata(config.GetMetadata()))
				errMessage += fmt.Sprintf("(Reason: %s)", config.Status.GetMessage())
				multiErr = multierror.Append(multiErr, errors.New(errMessage))
			}

			knownConfigs = append(knownConfigs, renderMetadata(config.GetMetadata()))
		}
	}

	if multiErr != nil {
		printer.AppendStatus("RateLimitConfigs", fmt.Sprintf("%v Errors!", multiErr.Len()))
		return knownConfigs, multiErr
	}

	printer.AppendStatus("RateLimitConfigs", "OK")
	return knownConfigs, nil
}

func checkVirtualHostOptions(ctx context.Context, printer printers.P, _ *options.Options, namespaces []string) ([]string, error) {
	printer.AppendCheck("Checking VirtualHostOptions... ")
	var knownVhOpts []string
	var multiErr *multierror.Error
	vhoptClient, err := helpers.VirtualHostOptionClient(ctx, namespaces)
	for _, ns := range namespaces {
		if err != nil {
			if isCrdNotFoundErr(gatewayv1.VirtualHostOptionCrd, err) {
				// Just warn. If the CRD is required, the check would have failed on the crashing gloo/gloo-ee pod.
				fmt.Printf("WARN: %s\n", CrdNotFoundErr(gatewayv1.VirtualHostOptionCrd.KindName).Error())
				return nil, nil
			}
			return nil, err
		}
		vhOpts, err := vhoptClient.List(ns, clients.ListOpts{})
		if err != nil {
			return nil, err
		}
		for _, vhOpt := range vhOpts {
			if vhOpt.GetNamespacedStatuses() != nil {
				namespacedStatuses := vhOpt.GetNamespacedStatuses()
				for reporter, status := range namespacedStatuses.GetStatuses() {
					switch status.GetState() {
					case core.Status_Rejected:
						errMessage := fmt.Sprintf("Found rejected VirtualHostOption by '%s': %s ", reporter, renderMetadata(vhOpt.GetMetadata()))
						errMessage += fmt.Sprintf("(Reason: %s)", status.GetReason())
						multiErr = multierror.Append(multiErr, errors.New(errMessage))
					case core.Status_Warning:
						errMessage := fmt.Sprintf("Found VirtualHostOption with warnings by '%s': %s ", reporter, renderMetadata(vhOpt.GetMetadata()))
						errMessage += fmt.Sprintf("(Reason: %s)", status.GetReason())
						multiErr = multierror.Append(multiErr, errors.New(errMessage))
					}
				}
			} else {
				errMessage := fmt.Sprintf("Found VirtualHostOption with no status: %s\n", renderMetadata(vhOpt.GetMetadata()))
				multiErr = multierror.Append(multiErr, errors.New(errMessage))
			}
			knownVhOpts = append(knownVhOpts, renderMetadata(vhOpt.GetMetadata()))
		}
	}
	if multiErr != nil {
		printer.AppendStatus("VirtualHostOptions", fmt.Sprintf("%v Errors!", multiErr.Len()))
		return knownVhOpts, multiErr
	}
	printer.AppendStatus("VirtualHostOptions", "OK")
	return knownVhOpts, nil
}

func checkRouteOptions(ctx context.Context, printer printers.P, _ *options.Options, namespaces []string) ([]string, error) {
	printer.AppendCheck("Checking RouteOptions... ")
	var knownRouteOpts []string
	var multiErr *multierror.Error
	routeOptionClient, err := helpers.RouteOptionClient(ctx, namespaces)
	for _, ns := range namespaces {
		if err != nil {
			if isCrdNotFoundErr(gatewayv1.RouteOptionCrd, err) {
				// Just warn. If the CRD is required, the check would have failed on the crashing gloo/gloo-ee pod.
				fmt.Printf("WARN: %s\n", CrdNotFoundErr(gatewayv1.RouteOptionCrd.KindName).Error())
				return nil, nil
			}
			return nil, err
		}
		routeOptions, err := routeOptionClient.List(ns, clients.ListOpts{})
		if err != nil {
			return nil, err
		}
		for _, routeOpt := range routeOptions {
			if routeOpt.GetNamespacedStatuses() != nil {
				namespacedStatuses := routeOpt.GetNamespacedStatuses()
				for reporter, status := range namespacedStatuses.GetStatuses() {
					switch status.GetState() {
					case core.Status_Rejected:
						errMessage := fmt.Sprintf("Found rejected RouteOption by '%s': %s ", reporter, renderMetadata(routeOpt.GetMetadata()))
						errMessage += fmt.Sprintf("(Reason: %s)", status.GetReason())
						multiErr = multierror.Append(multiErr, errors.New(errMessage))
					case core.Status_Warning:
						errMessage := fmt.Sprintf("Found RouteOption with warnings by '%s': %s ", reporter, renderMetadata(routeOpt.GetMetadata()))
						errMessage += fmt.Sprintf("(Reason: %s)", status.GetReason())
						multiErr = multierror.Append(multiErr, errors.New(errMessage))
					}
				}
			} else {
				errMessage := fmt.Sprintf("Found RouteOption with no status: %s\n", renderMetadata(routeOpt.GetMetadata()))
				multiErr = multierror.Append(multiErr, errors.New(errMessage))
			}
			knownRouteOpts = append(knownRouteOpts, renderMetadata(routeOpt.GetMetadata()))
		}
	}
	if multiErr != nil {
		printer.AppendStatus("RouteOptions", fmt.Sprintf("%v Errors!", multiErr.Len()))
		return knownRouteOpts, multiErr
	}
	printer.AppendStatus("RouteOptions", "OK")
	return knownRouteOpts, nil
}

func checkVirtualServices(ctx context.Context, printer printers.P, _ *options.Options, namespaces, knownUpstreams, knownAuthConfigs, knownRateLimitConfigs, knownVirtualHostOptions, knownRouteOptions []string) error {
	printer.AppendCheck("Checking VirtualServices... ")
	var multiErr *multierror.Error

	virtualServiceClient, err := helpers.VirtualServiceClient(ctx, namespaces)
	for _, ns := range namespaces {
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
			continue
		}
		virtualServices, err := virtualServiceClient.List(ns, clients.ListOpts{})
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
			continue
		}
		for _, virtualService := range virtualServices {
			if virtualService.GetNamespacedStatuses() != nil {
				namespacedStatuses := virtualService.GetNamespacedStatuses()
				for reporter, status := range namespacedStatuses.GetStatuses() {
					switch status.GetState() {
					case core.Status_Rejected:
						errMessage := fmt.Sprintf("Found rejected virtual service by '%s': %s ", reporter, renderMetadata(virtualService.GetMetadata()))
						errMessage += fmt.Sprintf("(Reason: %s)", status.GetReason())
						multiErr = multierror.Append(multiErr, fmt.Errorf(errMessage))
					case core.Status_Warning:
						errMessage := fmt.Sprintf("Found virtual service with warnings by '%s': %s ", reporter, renderMetadata(virtualService.GetMetadata()))
						errMessage += fmt.Sprintf("(Reason: %s)", status.GetReason())
						multiErr = multierror.Append(multiErr, fmt.Errorf(errMessage))
					}
				}
			} else {
				errMessage := fmt.Sprintf("Found virtual service with no status: %s\n", renderMetadata(virtualService.GetMetadata()))
				multiErr = multierror.Append(multiErr, errors.New(errMessage))
			}

			for _, route := range virtualService.GetVirtualHost().GetRoutes() {
				if route.GetRouteAction() != nil {
					if route.GetRouteAction().GetSingle() != nil {
						us := route.GetRouteAction().GetSingle()
						if us.GetUpstream() != nil {
							if !cliutils.Contains(knownUpstreams, renderRef(us.GetUpstream())) {
								// TODO warning message if using rejected or warning upstream
								errMessage := "Virtual service references unknown upstream: "
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
					// TODO: Virtual service references rejected or warning auth config
					errMessage := "Virtual service references unknown auth config:\n"
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
						errMessage := "Virtual service references unknown VirtualHostOption:\n"
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
					// TODO: check if references rate limit config with error or warning
					errMessage := "Virtual service references unknown rate limit config:\n"
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
		printer.AppendStatus("VirtualServices", fmt.Sprintf("%v Errors!", multiErr.Len()))
		return multiErr
	}
	printer.AppendStatus("VirtualServices", "OK")
	return nil
}

func checkGateways(ctx context.Context, printer printers.P, _ *options.Options, namespaces []string) error {
	printer.AppendCheck("Checking Gateways... ")
	var multiErr *multierror.Error
	gatewayClient, err := helpers.GatewayClient(ctx, namespaces)
	for _, ns := range namespaces {
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
			continue
		}
		gateways, err := gatewayClient.List(ns, clients.ListOpts{})
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
			continue
		}
		for _, gateway := range gateways {
			if gateway.GetNamespacedStatuses() != nil {
				namespacedStatuses := gateway.GetNamespacedStatuses()
				for reporter, status := range namespacedStatuses.GetStatuses() {
					switch status.GetState() {
					case core.Status_Rejected:
						errMessage := fmt.Sprintf("Found rejected gateway by '%s': %s\n", reporter, renderMetadata(gateway.GetMetadata()))
						errMessage += fmt.Sprintf("Reason: %s\n", status.GetReason())
						multiErr = multierror.Append(multiErr, fmt.Errorf(errMessage))
					case core.Status_Warning:
						errMessage := fmt.Sprintf("Found gateway with warnings by '%s': %s\n", reporter, renderMetadata(gateway.GetMetadata()))
						errMessage += fmt.Sprintf("Reason: %s\n", status.GetReason())
						multiErr = multierror.Append(multiErr, fmt.Errorf(errMessage))
					}
				}
			} else {
				errMessage := fmt.Sprintf("Found gateway with no status: %s\n", renderMetadata(gateway.GetMetadata()))
				multiErr = multierror.Append(multiErr, errors.New(errMessage))
			}
		}
	}

	if multiErr != nil {
		printer.AppendStatus("Gateways", fmt.Sprintf("%v Errors!", multiErr.Len()))
		return multiErr
	}

	printer.AppendStatus("Gateways", "OK")
	return nil
}

func checkProxies(ctx context.Context, printer printers.P, opts *options.Options, namespaces []string, glooNamespace string, deployments *appsv1.DeploymentList, deploymentsIncluded bool, settings *v1.Settings) error {
	printer.AppendCheck("Checking Proxies... ")
	if !deploymentsIncluded {
		printer.AppendStatus("proxies", "Skipping proxies because deployments were excluded")
		return nil
	}
	if deployments == nil {
		fmt.Println("Skipping due to an error in checking deployments")
		return fmt.Errorf("proxy check was skipped due to an error in checking deployments")
	}
	var multiErr *multierror.Error
	for _, ns := range namespaces {
		proxies, err := common.ListProxiesFromSettings(ns, opts, settings)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
			continue
		}
		for _, proxy := range proxies {
			if proxy.GetNamespacedStatuses() != nil {
				namespacedStatuses := proxy.GetNamespacedStatuses()
				for reporter, status := range namespacedStatuses.GetStatuses() {
					switch status.GetState() {
					case core.Status_Rejected:
						errMessage := fmt.Sprintf("Found rejected proxy by '%s': %s\n", reporter, renderMetadata(proxy.GetMetadata()))
						errMessage += fmt.Sprintf("Reason: %s\n", status.GetReason())
						multiErr = multierror.Append(multiErr, fmt.Errorf(errMessage))
					case core.Status_Warning:
						errMessage := fmt.Sprintf("Found proxy with warnings by '%s': %s\n", reporter, renderMetadata(proxy.GetMetadata()))
						errMessage += fmt.Sprintf("Reason: %s\n", status.GetReason())
						multiErr = multierror.Append(multiErr, fmt.Errorf(errMessage))
					}
				}
			} else {
				// Proxy has no status. We want to warn users that something is causing the Proxy to not be processed by the ControlPlane
				translatorValue := utils.GetTranslatorValue(proxy.GetMetadata())
				if translatorValue == utils.GatewayApiProxyValue {
					// This proxy was created by the k8s Gateway translation
					// That feature does not yet support propagating statuses onto the Proxy CR, so we ignore it

				} else {
					// This proxy was created by the Edge Gateway translation
					// That feature does support propagating statuses on the Proxy CR, so if a status is not there, we should error
					errMessage := fmt.Sprintf("Found proxy with no status: %s\n", renderMetadata(proxy.GetMetadata()))
					multiErr = multierror.Append(multiErr, errors.New(errMessage))
				}
			}
		}
	}
	err, warnings := checkProxiesPromStats(ctx, opts, glooNamespace, deployments)
	if err != nil {
		multiErr = multierror.Append(multiErr, err)
	}

	if multiErr != nil {
		printer.AppendStatus("Proxies", fmt.Sprintf("%v Errors!", multiErr.Len()))
		return multiErr
	}
	printer.AppendStatus("Proxies", "OK")
	if warnings != nil && warnings.Len() != 0 {
		for _, warning := range warnings.Errors {
			printer.AppendMessage(warning.Error())
		}
	}
	return nil
}

func checkSecrets(ctx context.Context, printer printers.P, _ *options.Options, namespaces []string) error {
	printer.AppendCheck("Checking Secrets... ")
	var multiErr *multierror.Error
	client, err := helpers.GetSecretClient(ctx, namespaces)
	if err != nil {
		multiErr = multierror.Append(multiErr, err)
		printer.AppendStatus("secrets", fmt.Sprintf("%v Errors!", multiErr.Len()))
		return multiErr
	}

	for _, ns := range namespaces {
		_, err := client.List(ns, clients.ListOpts{})
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
		// currently this would only find syntax errors
	}
	if multiErr != nil {
		printer.AppendStatus("Secrets", fmt.Sprintf("%v Errors!", multiErr.Len()))
		return multiErr
	}
	printer.AppendStatus("Secrets", "OK")
	return nil
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
func checkConnection(ctx context.Context, opts *options.Options) error {
	client, err := helpers.GetKubernetesClient(opts.Top.KubeContext)
	if err != nil {
		return eris.Wrapf(err, "Could not get kubernetes client")
	}
	_, err = client.CoreV1().Namespaces().Get(ctx, opts.Metadata.GetNamespace(), metav1.GetOptions{})
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
