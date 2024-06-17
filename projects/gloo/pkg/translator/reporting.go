package translator

import (
	"errors"
	"fmt"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/go-utils/contextutils"
)

func reportWeightedDestinationPluginProcessingError(
	params plugins.RouteParams,
	routeReport *validationapi.RouteReport,
	routeName string,
	weightedClusterName string,
	plugin plugins.WeightedDestinationPlugin,
	err error,
) {
	message := fmt.Sprintf("invalid weighted cluster [%s] while processing plugin %s: %s", weightedClusterName, plugin.Name(), err.Error())
	doReportErr := func() {
		validation.AppendRouteError(routeReport,
			validationapi.RouteReport_Error_ProcessingError,
			err.Error(),
			routeName,
		)
	}

	reportPluginProcessingError(params.Params, err, message, doReportErr)
}

func reportRoutePluginProcessingError(
	params plugins.RouteParams,
	routeReport *validationapi.RouteReport,
	in *v1.Route,
	out *envoy_config_route_v3.Route,
	plugin plugins.RoutePlugin,
	err error,
) {
	message := fmt.Sprintf("%T: %v", plugin, err.Error())
	doReportErr := func() {
		// Let's check if the incoming v1.Route has metadata to track the 'source' object
		// that created it. If we do have this metadata, include it with
		// the route error, so that other components can easily find it
		if staticMetadata := in.GetMetadataStatic(); staticMetadata != nil {
			validation.AppendRouteErrorWithMetadata(routeReport,
				validationapi.RouteReport_Error_ProcessingError,
				message,
				out.GetName(),
				staticMetadata,
			)
		}

		// Otherwise with no metadata, report the error without any source info
		validation.AppendRouteError(routeReport,
			validationapi.RouteReport_Error_ProcessingError,
			message,
			out.GetName(),
		)
	}

	reportPluginProcessingError(params.Params, err, message, doReportErr)
}

func reportRouteActionProcessingError(
	routeReport *validationapi.RouteReport,
	out *envoy_config_route_v3.Route,
	err error,
) {
	doReportErr := func() {
		validation.AppendRouteError(routeReport,
			validationapi.RouteReport_Error_ProcessingError,
			err.Error(),
			out.GetName(),
		)
	}

	doReportWarning := func() {
		validation.AppendRouteWarning(routeReport,
			validationapi.RouteReport_Warning_InvalidDestinationWarning,
			err.Error(),
		)
	}

	reportPluginProcessingErrorOrWarning(err, doReportErr, doReportWarning)
}

func reportRouteActionPluginProcessingError(
	params plugins.RouteActionParams,
	routeReport *validationapi.RouteReport,
	out *envoy_config_route_v3.Route,
	plugin plugins.RouteActionPlugin,
	err error,
) {
	message := fmt.Sprintf("invalid route [%s] while processing plugin %s: %s", params.Route.GetName(), plugin.Name(), err.Error())
	doReportErr := func() {
		validation.AppendRouteError(routeReport,
			validationapi.RouteReport_Error_ProcessingError,
			err.Error(),
			out.GetName(),
		)
	}

	reportPluginProcessingError(params.Params, err, message, doReportErr)
}

func reportVirtualHostPluginProcessingError(
	params plugins.VirtualHostParams,
	virtualHost *v1.VirtualHost,
	vhostReport *validationapi.VirtualHostReport,
	vhostPlugin plugins.VirtualHostPlugin,
	err error,
) {
	message := fmt.Sprintf("invalid virtual host [%s] while processing plugin %s: %s", virtualHost.GetName(), vhostPlugin.Name(), err.Error())
	doReportErr := func() {
		// Check if the incoming v1.VirtualHost has metadata to track the 'source' object
		// that created it. If we do have this metadata, include it with
		// the virtual host error, so that other components can easily find it
		if staticMetadata := virtualHost.GetMetadataStatic(); staticMetadata != nil {
			validation.AppendVirtualHostErrorWithMetadata(
				vhostReport,
				validationapi.VirtualHostReport_Error_ProcessingError,
				message,
				staticMetadata,
			)
		}

		// Otherwise with no metadata, report the error without any source info
		validation.AppendVirtualHostError(
			vhostReport,
			validationapi.VirtualHostReport_Error_ProcessingError,
			message,
		)
	}

	reportPluginProcessingError(params.Params, err, message, doReportErr)
}

// reportPluginProcessingError should only be used by components that do not support appending warnings on reports
// Ideally we would append this err on the report as a Warning
// To do so requires modifying an internal Proto API:
// https://github.com/solo-io/gloo/blob/76a49fddacf8a7d26d4bf8dd3b21525a8efe73bd/projects/gloo/api/grpc/validation/gloo_validation.proto#L4
// This API is a legacy of having separate Gloo and Gateway pods and is cumbersome to update
// We take a short-cut, defined below
func reportPluginProcessingError(
	params plugins.Params,
	err error,
	message string,
	doReportErr func(),
) {
	doReportWarning := func() {
		if params.Settings.GetGateway().GetValidation().GetAllowWarnings().GetValue() {
			// If warnings are allowed in our Webhook, this means that warnings on resources should be accepted
			// Since there is no mechanism to report a warning, we swallow it and log
			contextutils.LoggerFrom(params.Ctx).Warnf(fmt.Sprintf("%s. allowWarnings=true so resource will be accepted", message))
		} else {
			// If warnings are not allowed, this means that warnings on resources should be rejected
			// Since there is no mechanism to report a warning, we report it as an error so it is rejected
			doReportErr()
		}
	}

	reportPluginProcessingErrorOrWarning(err, doReportErr, doReportWarning)
}

// reportPluginProcessingErrorOrWarning captures the error that is returned by a plugin, and executes an action with that error
// Most often it will place that error on a report, which is consumed by other components to make validation and status reporting decisions.
// This function has some complex logic, with some technical debt, so we intentionally split it off from other
// code to more easily isolate and test changes to it
func reportPluginProcessingErrorOrWarning(
	err error,
	doReportErr func(),
	doReportWarning func(),
) {
	var configurationError plugins.ConfigurationError
	isConfigurationError := errors.As(err, &configurationError)

	if isConfigurationError && configurationError.IsWarning() {
		doReportWarning()
		return
	}

	// This handles the following cases:
	//	- A plugin does not return a ConfigurationError. Since this is a new concept, this will be a common case, and we
	//		fallback to the legacy behavior, to always report an error
	//	- A plugin returns a ConfigurationError, but explicitly defines it to NOT be a warning.
	doReportErr()
}
