package syncer

import (
	"context"
	"fmt"

	"github.com/solo-io/gloo/pkg/utils/syncutil"
	"github.com/solo-io/gloo/projects/gateway/pkg/reconciler"
	"go.uber.org/zap/zapcore"

	"go.uber.org/zap"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/propagator"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"github.com/solo-io/solo-kit/pkg/errors"
)

type translatorSyncer struct {
	writeNamespace  string
	reporter        reporter.Reporter
	propagator      *propagator.Propagator
	proxyClient     gloov1.ProxyClient
	gwClient        v1.GatewayClient
	vsClient        v1.VirtualServiceClient
	proxyReconciler reconciler.ProxyReconciler
	translator      translator.Translator
}

func NewTranslatorSyncer(writeNamespace string, proxyClient gloov1.ProxyClient, proxyReconciler reconciler.ProxyReconciler, gwClient v1.GatewayClient, vsClient v1.VirtualServiceClient, reporter reporter.Reporter, propagator *propagator.Propagator, translator translator.Translator) v1.ApiSyncer {
	return &translatorSyncer{
		writeNamespace:  writeNamespace,
		reporter:        reporter,
		propagator:      propagator,
		proxyClient:     proxyClient,
		gwClient:        gwClient,
		vsClient:        vsClient,
		proxyReconciler: proxyReconciler,
		translator:      translator,
	}
}

// TODO (ilackarms): make sure that sync happens if proxies get updated as well; may need to resync
func (s *translatorSyncer) Sync(ctx context.Context, snap *v1.ApiSnapshot) error {
	ctx = contextutils.WithLogger(ctx, "translatorSyncer")

	logger := contextutils.LoggerFrom(ctx)
	logger.Debugw("begin sync", zap.Any("snapshot", snap.Stringer()))
	logger.Infof("begin sync %v (%v virtual services, %v gateways, %v route tables)", snap.Hash(),
		len(snap.VirtualServices), len(snap.Gateways), len(snap.RouteTables))
	defer logger.Infof("end sync %v", snap.Hash())

	// stringifying the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	if contextutils.GetLogLevel() == zapcore.DebugLevel {
		logger.Debug(syncutil.StringifySnapshot(snap))
	}

	labels := map[string]string{
		"created_by": "gateway",
	}

	gatewaysByProxy := utils.GatewaysByProxyName(snap.Gateways)

	desiredProxies := make(reconciler.GeneratedProxies)

	for proxyName, gatewayList := range gatewaysByProxy {
		proxy, reports := s.translator.Translate(ctx, proxyName, s.writeNamespace, snap, gatewayList)

		if proxy != nil {
			logger.Infof("desired proxy %v", proxy.Metadata.Ref())
			proxy.Metadata.Labels = labels
			desiredProxies[proxy] = reports
		}
	}

	if err := s.proxyReconciler.ReconcileProxies(ctx, desiredProxies, s.writeNamespace, labels); err != nil {
		return err
	}

	// repeat for all resources
	for proxy, reports := range desiredProxies {
		// start propagating for new set of resources
		if err := s.propagateProxyStatus(ctx, proxy, reports); err != nil {
			return err
		}
	}

	return nil
}

func (s *translatorSyncer) propagateProxyStatus(ctx context.Context, proxy *gloov1.Proxy, reports reporter.ResourceReports) error {
	if proxy == nil {
		return nil
	}
	statuses, err := watchProxyStatus(ctx, s.proxyClient, proxy)
	if err != nil {
		return err
	}
	var lastStatus core.Status
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case status := <-statuses:
				if status.Equal(lastStatus) {
					continue
				}
				lastStatus = status
				subresourceStatuses := map[string]*core.Status{
					fmt.Sprintf("%T.%s", proxy, proxy.GetMetadata().Ref().Key()): &status,
				}
				err := s.reporter.WriteReports(ctx, reports, subresourceStatuses)
				if err != nil {
					contextutils.LoggerFrom(ctx).Errorf("err: updating dependent statuses: %v", err)
				}
			}
		}
	}()
	return nil
}

func watchProxyStatus(ctx context.Context, proxyClient gloov1.ProxyClient, proxy *gloov1.Proxy) (<-chan core.Status, error) {
	ctx = contextutils.WithLogger(ctx, "proxy-err-propagator")
	proxies, errs, err := proxyClient.Watch(proxy.Metadata.Namespace, clients.WatchOpts{
		Ctx:      ctx,
		Selector: proxy.Metadata.Labels,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "creating watch for proxy %v", proxy.Metadata.Ref())
	}
	statuses := make(chan core.Status)
	go func() {
		defer close(statuses)
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-errs:
				if !ok {
					return
				}
				contextutils.LoggerFrom(ctx).Error(err)
			case list, ok := <-proxies:
				if !ok {
					return
				}
				proxy, err := list.Find(proxy.Metadata.Namespace, proxy.Metadata.Name)
				if err != nil {
					contextutils.LoggerFrom(ctx).Error(err)
					continue
				}
				select {
				case <-ctx.Done():
					return
				case statuses <- proxy.Status:
				}
			}
		}
	}()

	return statuses, nil
}
