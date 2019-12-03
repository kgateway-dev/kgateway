package reconciler

import (
	"context"
	"sort"

	"go.uber.org/zap"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/projects/gateway/pkg/reporting"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	"github.com/solo-io/gloo/projects/gateway/pkg/utils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"github.com/solo-io/solo-kit/pkg/errors"
)

type GeneratedProxies map[*gloov1.Proxy]reporter.ResourceReports

type ProxyReconciler interface {
	ReconcileProxies(ctx context.Context, proxiesToWrite GeneratedProxies, writeNamespace string, labels map[string]string) error
}

type proxyReconciler struct {
	proxyValidator validation.ProxyValidationServiceClient
	baseReconciler gloov1.ProxyReconciler
}

func NewProxyReconciler(proxyValidator validation.ProxyValidationServiceClient, proxyClient gloov1.ProxyClient) *proxyReconciler {
	return &proxyReconciler{proxyValidator: proxyValidator, baseReconciler: gloov1.NewProxyReconciler(proxyClient)}
}

const proxyValidationErrMsg = "internal err: communication with proxy validation (gloo) failed"

func (s *proxyReconciler) ReconcileProxies(ctx context.Context, proxiesToWrite GeneratedProxies, writeNamespace string, labels map[string]string) error {
	if err := s.addProxyValidationResults(ctx, proxiesToWrite); err != nil {
		return errors.Wrapf(err, "failed to add proxy validation results to reports")
	}

	proxiesToWrite, err := stripInvalidListenersAndVirtualHosts(ctx, proxiesToWrite)
	if err != nil {
		return err
	}

	var allProxies gloov1.ProxyList
	for proxy := range proxiesToWrite {
		allProxies = append(allProxies, proxy)
	}

	sort.SliceStable(allProxies, func(i, j int) bool {
		return allProxies[i].Metadata.Less(allProxies[j].Metadata)
	})

	if err := s.baseReconciler.Reconcile(writeNamespace, allProxies, transitionFunc(proxiesToWrite), clients.ListOpts{
		Ctx:      ctx,
		Selector: labels,
	}); err != nil {
		return err
	}

	return nil
}

func forEachListener(proxy *gloov1.Proxy, reports reporter.ResourceReports, fn func(*gloov1.Listener, bool)) error {
	for _, lis := range proxy.Listeners {
		accepted, err := reporting.AllSourcesAccepted(reports, lis)
		if err != nil {
			return err
		}

		fn(lis, accepted)
	}
	return nil
}

func forEachVhost(lis *gloov1.Listener, reports reporter.ResourceReports, fn func(*gloov1.VirtualHost, bool)) error {
	if httpListenerType, ok := lis.ListenerType.(*gloov1.Listener_HttpListener); ok {

		for _, vhost := range httpListenerType.HttpListener.GetVirtualHosts() {
			accepted, err := reporting.AllSourcesAccepted(reports, vhost)
			if err != nil {
				return err
			}

			fn(vhost, accepted)
		}
	}
	return nil
}

// validate generated proxies and add reports for the owner resources
// this function makes a gRPC call to gloo validation server
func (s *proxyReconciler) addProxyValidationResults(ctx context.Context, proxiesToWrite GeneratedProxies) error {

	logger := contextutils.LoggerFrom(ctx)

	if s.proxyValidator == nil {
		logger.Warnf("proxy validation is not configured, skipping proxy validation check")
		return nil
	}

	for proxy, reports := range proxiesToWrite {

		proxyRpt, err := s.proxyValidator.ValidateProxy(ctx, &validation.ProxyValidationServiceRequest{
			Proxy: proxy,
		})
		if err != nil {
			return errors.Wrapf(err, proxyValidationErrMsg)
		}

		if validateErr := reports.ValidateStrict(); validateErr != nil {
			logger.Warnw("Proxy had invalid config", zap.Any("proxy", proxy.Metadata.Ref()), zap.Error(validateErr))
		}

		// add the proxy validation result to the existing resource reports
		if err := reporting.AddProxyValidationResult(reports, proxy, proxyRpt.GetProxyReport()); err != nil {
			//should never happen
			return err
		}
	}

	return nil
}

// strips any listeners and virtualhosts who are created from an errored virtual service / gateway
// check the vs/gateway for the listener/virtualhost by looking at their metadata.sources
// check the error on the vs/gateway by searching through the resource reports
// this function must be called before reconciling the proxies
func stripInvalidListenersAndVirtualHosts(ctx context.Context, proxiesToWrite GeneratedProxies) (GeneratedProxies, error) {
	strippedProxies := GeneratedProxies{}
	logger := contextutils.LoggerFrom(ctx)

	for proxy, reports := range proxiesToWrite {

		// clone because mutations occur
		proxy := resources.Clone(proxy).(*gloov1.Proxy)

		var validListeners []*gloov1.Listener

		if err := forEachListener(proxy, reports, func(listener *gloov1.Listener, accepted bool) {
			if accepted {
				validListeners = append(validListeners, listener)
			} else {
				logger.Warnw("stripping invalid listener from proxy", zap.Any("proxy", proxy.Metadata.Ref()), zap.String("listener", listener.Name))
			}
		}); err != nil {
			return nil, err
		}

		for _, lis := range proxy.Listeners {

			if httpListenerType, ok := lis.ListenerType.(*gloov1.Listener_HttpListener); ok {
				var validVhosts []*gloov1.VirtualHost

				if err := forEachVhost(lis, reports, func(vhost *gloov1.VirtualHost, accepted bool) {
					if accepted {
						validVhosts = append(validVhosts, vhost)
					} else {
						logger.Warnw("stripping invalid virtualhost from proxy", zap.Any("proxy", proxy.Metadata.Ref()), zap.String("listener", lis.Name), zap.String("virtual host", vhost.Name))
					}
				}); err != nil {
					return nil, err
				}

				sort.SliceStable(validVhosts, func(i, j int) bool {
					return validVhosts[i].Name < validVhosts[j].Name
				})

				httpListenerType.HttpListener.VirtualHosts = validVhosts
			}
		}

		sort.SliceStable(validListeners, func(i, j int) bool {
			return validListeners[i].Name < validListeners[j].Name
		})

		proxy.Listeners = validListeners

		// update the map with the copy
		strippedProxies[proxy] = reports
	}

	return strippedProxies, nil
}

// this function is called by the base reconciler to update an existing proxy
// persists listeners and virtual hosts from the existing proxy
// it is necessary to call this transition function *after*
// stripping invalid virtual hosts / listeners from the desired proxy,
// else we will wind up with both an invalid and valid version of the same listener/vhost on our proxy
// which is invalid and will be rejected by Envoy
func transitionFunc(proxiesToWrite GeneratedProxies) gloov1.TransitionProxyFunc {
	return func(original, desired *gloov1.Proxy) (b bool, e error) {
		// if any listeners from the old proxy were rejected in the new reports, preserve those
		if err := forEachListener(original, proxiesToWrite[desired], func(listener *gloov1.Listener, accepted bool) {
			// old listener was rejected, preserve it on the desired proxy
			if !accepted {
				desired.Listeners = append(desired.Listeners, listener)
			}
		}); err != nil {
			// should never happen
			return false, err
		}

		// preserve previous vhosts if new vservice was errored
		for _, desiredListener := range desired.Listeners {

			desiredHttpListener := desiredListener.GetHttpListener()
			if desiredHttpListener == nil {
				continue
			}

			// find the original listener by its name
			// if it does not exist in the original, skip
			var originalListener *gloov1.Listener
			for _, origLis := range original.Listeners {
				if origLis.Name == desiredListener.Name {
					originalListener = origLis
					break
				}
			}
			if originalListener == nil {
				continue
			}

			// find any rejected vhosts on the original listener and copy them over
			if err := forEachVhost(originalListener, proxiesToWrite[desired], func(vhost *gloov1.VirtualHost, accepted bool) {
				// old vhost was rejected, preserve it on the desired proxy
				if !accepted {
					desiredHttpListener.VirtualHosts = append(desiredHttpListener.VirtualHosts, vhost)
				}
			}); err != nil {
				// should never happen
				return false, err
			}

			sort.SliceStable(desiredHttpListener.VirtualHosts, func(i, j int) bool {
				return desiredHttpListener.VirtualHosts[i].Name < desiredHttpListener.VirtualHosts[j].Name
			})

		}

		sort.SliceStable(desired.Listeners, func(i, j int) bool {
			return desired.Listeners[i].Name < desired.Listeners[j].Name
		})

		return utils.TransitionFunction(original, desired)
	}
}
