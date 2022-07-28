package setup

import (
	"context"
	"errors"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var (
	// These could theoretically be passed in as arguments to make the cleanup resources by label functionality more generic
	// Currently there isn't a clear use case for that and defining the values here feels most readable
	gatewayLabelValue = "gloo-gateway-translator"
	createdByLabelKey = "created_by"
)

func deleteUnusedProxies(ctx context.Context, namespace string, proxyClient v1.ProxyClient) error {
	currentProxies, err := proxyClient.List(namespace, clients.ListOpts{Ctx: ctx})
	if err != nil {
		return err
	}
	deleteErrs := make([]error, 0)
	for _, proxy := range currentProxies {
		if val, ok := proxy.GetMetadata().GetLabels()[createdByLabelKey]; ok && val == gatewayLabelValue {
			err = proxyClient.Delete(namespace, proxy.GetMetadata().GetName(), clients.DeleteOpts{Ctx: ctx})
			// continue to clean up other proxies
			if err != nil {
				deleteErrs = append(deleteErrs, err)
			}
		}
	}
	if len(deleteErrs) == 0 {
		return nil
	}
	// Concatenate error messages from all the failed deletes
	allErrs := ""
	for _, err := range deleteErrs {
		allErrs += err.Error()
	}
	return errors.New(allErrs)
}
func DoProxyCleanup(ctx context.Context, settings *v1.Settings, proxyClient v1.ProxyClient, namespace string) error {
	//Do not clean up proxies if all the resources are held in memory or if proxies are being persisted
	if settings.GetConfigSource() == nil || settings.GetGateway().GetPersistProxySpec().GetValue() {
		return nil
	}

	return deleteUnusedProxies(ctx, namespace, proxyClient)
}
