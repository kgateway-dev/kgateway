package internal

import (
	"context"
	"fmt"
	"reflect"

	"github.com/hashicorp/go-multierror"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"golang.org/x/exp/slices"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwclient "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
)

func CheckGatewayClass(ctx context.Context, printer printers.P, opts *options.Options) error {
	printer.AppendCheck("Checking Kubernetes GatewayClasses... ")
	cfg := config.GetConfigOrDie()
	cli := gwclient.NewForConfigOrDie(cfg)

	gc, err := cli.GatewayV1().GatewayClasses().Get(ctx, wellknown.GatewayClassName, metav1.GetOptions{})
	if err != nil {
		errMessage := fmt.Sprintf("Could not find solo GatewayClass %s", wellknown.GatewayClassName)
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
		printer.AppendStatus("Kubernetes GatewayClasses", fmt.Sprintf("%v Errors!", multierr.Len()))
		return multierr.ErrorOrNil()
	}
	printer.AppendStatus("Kubernetes GatewayClasses", "OK")
	return nil
}

func CheckGateways(ctx context.Context, printer printers.P, opts *options.Options) error {
	printer.AppendCheck("Checking Kubernetes Gateways... ")
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

		if gw.Spec.GatewayClassName != wellknown.GatewayClassName {
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
		printer.AppendStatus("Kubernetes Gateways", fmt.Sprintf("%v Errors!", multierr.Len()))
		return multierr.ErrorOrNil()
	}
	printer.AppendStatus("Kubernetes Gateways", "OK")
	return nil
}

func CheckHTTPRoutes(ctx context.Context, printer printers.P, opts *options.Options) error {
	printer.AppendCheck("Checking Kubernetes HTTPRoutes... ")
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
		printer.AppendStatus("Kubernetes HTTPRoutes", fmt.Sprintf("%v Errors!", multierr.Len()))
		return multierr.ErrorOrNil()
	}
	printer.AppendStatus("Kubernetes HTTPRoutes", "OK")
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
				"%s status (%s) is not up to date with the object's Generation",
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
