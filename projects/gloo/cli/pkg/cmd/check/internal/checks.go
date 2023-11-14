package internal

import (
	"context"
	"fmt"
	"reflect"

	"github.com/hashicorp/go-multierror"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"golang.org/x/exp/slices"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwclient "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
)

// Checks whether the cluster that the kubeconfig points at is available
// The timeout for the kubernetes client is set to a low value to notify the user of the failure
func CheckConnection(ctx context.Context, _ printers.P, opts *options.Options) error {
	client, err := helpers.GetKubernetesClient(opts.Top.KubeContext)
	if err != nil {
		return eris.Wrapf(err, "Could not get kubernetes client")
	}
	_, err = client.CoreV1().Namespaces().Get(ctx, opts.Top.Namespace, metav1.GetOptions{})
	if err != nil {
		return eris.Wrapf(err, "Could not communicate with kubernetes cluster")
	}
	return nil
}

// func CheckControlPlane(ctx context.Context, printer printers.P, opts *options.Options) error {}

func CheckDeployments(ctx context.Context, printer printers.P, opts *options.Options) error {
	printer.AppendCheck("Checking deployments... ")
	client, err := helpers.GetKubernetesClient(opts.Top.KubeContext)
	if err != nil {
		errMessage := "error getting KubeClient"
		fmt.Println(errMessage)
		return fmt.Errorf(errMessage+": %v", err)
	}
	_, err = client.CoreV1().Namespaces().Get(ctx, opts.Top.Namespace, metav1.GetOptions{})
	if err != nil {
		errMessage := "Gloo namespace does not exist"
		fmt.Println(errMessage)
		return fmt.Errorf(errMessage)
	}
	deployments, err := client.AppsV1().Deployments(opts.Top.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	if len(deployments.Items) == 0 {
		errMessage := "Gloo is not installed"
		fmt.Println(errMessage)
		return fmt.Errorf(errMessage)
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
	if multiErr.ErrorOrNil() != nil {
		printer.AppendStatus("deployments", fmt.Sprintf("%v Errors!", multiErr.Len()))
		return multiErr.ErrorOrNil()
	}
	printer.AppendStatus("deployments", "OK")
	return nil
}

func CheckGatewayClass(ctx context.Context, printer printers.P, opts *options.Options) error {
	printer.AppendCheck("Checking GatewayClass... ")
	cfg := config.GetConfigOrDie()
	cli := gwclient.NewForConfigOrDie(cfg)

	gc, err := cli.GatewayV1().GatewayClasses().Get(ctx, "gloo-gateway", metav1.GetOptions{})
	if err != nil {
		errMessage := "Could not find solo GatewayClass gloo-gateway"
		fmt.Println(errMessage)
		return fmt.Errorf(errMessage)
	}

	expectedConditions := []expectedCondition[gwv1.GatewayClassConditionType]{
		{condition: gwv1.GatewayClassConditionStatusAccepted, status: metav1.ConditionTrue},
		{condition: gwv1.GatewayClassConditionStatusSupportedVersion, status: metav1.ConditionTrue},
	}

	multierr := &multierror.Error{}
	processConditions(
		fmt.Sprintf("GatewayClass %s", gc.Name),
		multierr,
		expectedConditions,
		gc.Status.Conditions,
		gc.Generation,
	)

	if multierr.ErrorOrNil() != nil {
		printer.AppendStatus("GatewayClass", fmt.Sprintf("%v Errors!", multierr.Len()))
		return multierr.ErrorOrNil()
	}
	printer.AppendStatus("GatewayClass", "OK")
	return nil
}

func CheckGatewys(ctx context.Context, printer printers.P, opts *options.Options) error {
	printer.AppendCheck("Checking Gateways... ")
	cfg := config.GetConfigOrDie()
	cli := gwclient.NewForConfigOrDie(cfg)

	// Maybe this will be all NS?
	gwList, err := cli.GatewayV1().Gateways("").List(ctx, metav1.ListOptions{})
	if err != nil {
		errMessage := "could not list Gateways"
		fmt.Println(errMessage)
		return fmt.Errorf(errMessage)
	}

	multierr := &multierror.Error{}
	for _, gw := range gwList.Items {
		// Pike until go 1.22
		gw := gw

		if gw.Spec.GatewayClassName != "gloo-gateway" {
			// #not_my_gateway
			continue
		}

		expectedConditions := []expectedCondition[gwv1.GatewayConditionType]{
			{condition: gwv1.GatewayConditionAccepted, status: metav1.ConditionTrue},
			{condition: gwv1.GatewayConditionProgrammed, status: metav1.ConditionTrue},
		}

		processConditions(
			fmt.Sprintf("Gateway %s.%s", gw.Namespace, gw.Name),
			multierr,
			expectedConditions,
			gw.Status.Conditions,
			gw.Generation,
		)

		// Go through listeners and process them

		for spec_idx := range gw.Spec.Listeners {

			status_idx := slices.IndexFunc(gw.Status.Listeners, func(ls gwv1.ListenerStatus) bool {
				return ls.Name == gw.Spec.Listeners[spec_idx].Name
			})

			// No status found for listener, that's an error
			if status_idx == -1 {
				multierror.Append(multierr, fmt.Errorf("no status found for listener %s", gw.Spec.Listeners[spec_idx].Name))
				continue
			}

			expectedConditions := []expectedCondition[gwv1.ListenerConditionType]{
				{condition: gwv1.ListenerConditionProgrammed, status: metav1.ConditionTrue},
				{condition: gwv1.ListenerConditionConflicted, status: metav1.ConditionFalse},
				{condition: gwv1.ListenerConditionAccepted, status: metav1.ConditionTrue},
				{condition: gwv1.ListenerConditionResolvedRefs, status: metav1.ConditionTrue},
			}

			processConditions(
				fmt.Sprintf("Listener %s.%s.%s", gw.Namespace, gw.Name, gw.Spec.Listeners[spec_idx].Name),
				multierr,
				expectedConditions,
				gw.Status.Listeners[status_idx].Conditions,
				gw.Generation,
			)

		}
	}

	if multierr.ErrorOrNil() != nil {
		printer.AppendStatus("Gateway", fmt.Sprintf("%v Errors!", multierr.Len()))
		return multierr.ErrorOrNil()
	}
	printer.AppendStatus("Gateway", "OK")
	return nil
}

func CheckHTTPRoutes(ctx context.Context, printer printers.P, opts *options.Options) error {
	printer.AppendCheck("Checking HTTPRoutes... ")
	cfg := config.GetConfigOrDie()
	cli := gwclient.NewForConfigOrDie(cfg)

	// Maybe this will be all NS?
	httpRouteList, err := cli.GatewayV1().HTTPRoutes("").List(ctx, metav1.ListOptions{})
	if err != nil {
		errMessage := "Could not list HTTPRoutes"
		fmt.Println(errMessage)
		return fmt.Errorf(errMessage)
	}

	multierr := &multierror.Error{}
	for route_idx := range httpRouteList.Items {
		// For each parent_ref in the spec, check that one exists in the status.

		for parent_idx := range httpRouteList.Items[route_idx].Spec.ParentRefs {
			parent_ref := httpRouteList.Items[route_idx].Spec.ParentRefs[parent_idx]
			child_idx := slices.IndexFunc(
				httpRouteList.Items[route_idx].Status.Parents,
				func(status_parent gwv1.RouteParentStatus) bool {
					// Return the idx if we find the parent_ref in the status
					return reflect.DeepEqual(status_parent.ParentRef, parent_ref)
				},
			)

			if child_idx == -1 {
				// Error, parent was not recorded in the status
				multierr = multierror.Append(multierr,
					fmt.Errorf("unable to find matching status for ParentRef %+v", parent_ref),
				)
			} else {
				// Check the validity of the status
				expectedConditions := []expectedCondition[gwv1.RouteConditionType]{
					{condition: gwv1.RouteConditionAccepted, status: metav1.ConditionTrue},
					{condition: gwv1.RouteConditionResolvedRefs, status: metav1.ConditionTrue},
				}
				processConditions(
					fmt.Sprintf("HTTPRoute %s.%s.%s", httpRouteList.Items[route_idx].Namespace, httpRouteList.Items[route_idx].Name, parent_ref.Name),
					multierr,
					expectedConditions,
					httpRouteList.Items[route_idx].Status.Parents[parent_idx].Conditions,
					httpRouteList.Items[route_idx].Generation,
				)
			}
		}
	}

	if multierr.ErrorOrNil() != nil {
		printer.AppendStatus("HTTPRoute", fmt.Sprintf("%v Errors!", multierr.Len()))
		return multierr.ErrorOrNil()
	}
	printer.AppendStatus("HTTPRoute", "OK")
	return nil

}

type expectedCondition[T ~string] struct {
	condition T
	status    metav1.ConditionStatus
}

func processConditions[T ~string](
	parent string,
	multierr *multierror.Error,
	expectedConditions []expectedCondition[T],
	conditions []metav1.Condition,
	generation int64,
) {
	for idx := range expectedConditions {
		condition := meta.FindStatusCondition(conditions, string(expectedConditions[idx].condition))
		if condition == nil {
			multierr = multierror.Append(multierr, fmt.Errorf(
				"%s status (%s) was not found, most likely an error or has not reconciled yet",
				parent, string(expectedConditions[idx].condition),
			))
		} else if condition.ObservedGeneration != generation {
			// Hasn't reconciled yet
			multierr = multierror.Append(multierr, fmt.Errorf(
				"status (%s) is not up to date with the object's Generation",
				parent, string(expectedConditions[idx].condition),
			))
		} else {
			switch condition.Status {
			case expectedConditions[idx].status: // We're good here
			default: //ruh roh
				multierr = multierror.Append(multierr, fmt.Errorf(
					"%s status (%s) is not set to expected (%s). Reason: %s, Message: %s",
					parent,
					string(expectedConditions[idx].condition),
					string(expectedConditions[idx].status),
					condition.Reason,
					condition.Message,
				))
			}
		}
	}
}
