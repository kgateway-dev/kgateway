package syncer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap/zapcore"

	"github.com/hashicorp/go-multierror"
	"github.com/solo-io/gloo/pkg/utils/syncutil"
	"github.com/solo-io/gloo/projects/gateway/pkg/reconciler"
	"github.com/solo-io/go-utils/hashutils"
	"go.uber.org/zap"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"github.com/solo-io/solo-kit/pkg/errors"
)

type translatorSyncer struct {
	writeNamespace  string
	reporter        reporter.Reporter
	proxyClient     gloov1.ProxyClient
	gwClient        v1.GatewayClient
	vsClient        v1.VirtualServiceClient
	proxyReconciler reconciler.ProxyReconciler
	translator      translator.Translator
	statusSyncer    statusSyncer
	labels          map[string]string
}

func NewTranslatorSyncer(ctx context.Context, writeNamespace string, proxyClient gloov1.ProxyClient, proxyReconciler reconciler.ProxyReconciler, gwClient v1.GatewayClient, vsClient v1.VirtualServiceClient, reporter reporter.Reporter, translator translator.Translator) v1.ApiSyncer {
	t := &translatorSyncer{
		writeNamespace:  writeNamespace,
		reporter:        reporter,
		proxyClient:     proxyClient,
		gwClient:        gwClient,
		vsClient:        vsClient,
		proxyReconciler: proxyReconciler,
		translator:      translator,
		statusSyncer:    newStatusSyncer(writeNamespace, proxyClient, reporter),
		labels: map[string]string{
			"created_by": "gateway",
		},
	}

	go t.statusSyncer.watchProxies(ctx)
	go t.statusSyncer.syncStatusOnInterval(ctx)
	return t
}

// TODO (ilackarms): make sure that sync happens if proxies get updated as well; may need to resync
func (s *translatorSyncer) Sync(ctx context.Context, snap *v1.ApiSnapshot) error {
	ctx = contextutils.WithLogger(ctx, "translatorSyncer")

	logger := contextutils.LoggerFrom(ctx)
	logger.Debugw("begin sync", zap.Any("snapshot", snap.Stringer()))
	snapHash := hashutils.MustHash(snap)
	logger.Infof("begin sync %v (%v virtual services, %v gateways, %v route tables)", snapHash,
		len(snap.VirtualServices), len(snap.Gateways), len(snap.RouteTables))
	defer logger.Infof("end sync %v", snapHash)

	// stringify-ing the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	if contextutils.GetLogLevel() == zapcore.DebugLevel {
		logger.Debug(syncutil.StringifySnapshot(snap))
	}

	gatewaysByProxy := utils.GatewaysByProxyName(snap.Gateways)

	desiredProxies := make(reconciler.GeneratedProxies)

	for proxyName, gatewayList := range gatewaysByProxy {
		proxy, reports := s.translator.Translate(ctx, proxyName, s.writeNamespace, snap, gatewayList)
		if proxy != nil {
			logger.Infof("desired proxy %v", proxy.Metadata.Ref())
			proxy.Metadata.Labels = s.labels
			desiredProxies[proxy] = reports
		}
	}

	return s.reconcile(ctx, desiredProxies)
}

func (s *translatorSyncer) reconcile(ctx context.Context, desiredProxies reconciler.GeneratedProxies) error {
	if err := s.proxyReconciler.ReconcileProxies(ctx, desiredProxies, s.writeNamespace, s.labels); err != nil {
		return err
	}

	// repeat for all resources
	s.statusSyncer.setCurrentProxies(desiredProxies)
	return nil
}

type reportsAndStatus struct {
	Status  *core.Status
	Reports reporter.ResourceReports
}
type statusSyncer struct {
	proxyToLastStatus       map[core.ResourceRef]reportsAndStatus
	currentGeneratedProxies map[core.ResourceRef]struct{}
	mapLock                 sync.RWMutex
	reporter                reporter.Reporter

	proxyClient    gloov1.ProxyWatcher
	writeNamespace string
}

func newStatusSyncer(writeNamespace string, proxyClient gloov1.ProxyWatcher, reporter reporter.Reporter) statusSyncer {
	return statusSyncer{
		proxyToLastStatus:       map[core.ResourceRef]reportsAndStatus{},
		currentGeneratedProxies: map[core.ResourceRef]struct{}{},
		reporter:                reporter,
		proxyClient:             proxyClient,
		writeNamespace:          writeNamespace,
	}
}

func (s *statusSyncer) setCurrentProxies(desiredProxies reconciler.GeneratedProxies) {
	// add an remove things from the map
	s.mapLock.Lock()
	defer s.mapLock.Unlock()
	newCurrentGeneratedProxies := map[core.ResourceRef]struct{}{}
	for proxy, reports := range desiredProxies {
		// start propagating for new set of resources
		ref := proxy.GetMetadata().Ref()
		if _, ok := s.proxyToLastStatus[ref]; !ok {
			s.proxyToLastStatus[ref] = reportsAndStatus{}
		}
		current := s.proxyToLastStatus[ref]
		current.Reports = reports
		s.proxyToLastStatus[ref] = current
		newCurrentGeneratedProxies[ref] = struct{}{}
	}
	s.currentGeneratedProxies = newCurrentGeneratedProxies
}

// run this in the background
func (s *statusSyncer) watchProxies(ctx context.Context) error {
	ctx = contextutils.WithLogger(ctx, "proxy-err-propagator")
	proxies, errs, err := s.proxyClient.Watch(s.writeNamespace, clients.WatchOpts{
		Ctx: ctx,
	})
	if err != nil {
		return errors.Wrapf(err, "creating watch for proxies in %v", s.writeNamespace)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-errs:
			if !ok {
				return nil
			}
			contextutils.LoggerFrom(ctx).Error(err)
		case list, ok := <-proxies:
			if !ok {
				return nil
			}
			s.setStatuses(list)
		}
	}
}

func (s *statusSyncer) setStatuses(list gloov1.ProxyList) {
	s.mapLock.Lock()
	defer s.mapLock.Unlock()
	for _, proxy := range list {
		ref := proxy.Metadata.Ref()
		status := proxy.Status
		if current, ok := s.proxyToLastStatus[ref]; ok {
			current.Status = &status
			s.proxyToLastStatus[ref] = current
		} else {
			s.proxyToLastStatus[ref] = reportsAndStatus{
				Status: &status,
			}
		}
	}
}

// run this on a timer
func (s *statusSyncer) syncStatusOnInterval(ctx context.Context) error {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			err := s.syncStatus(ctx)
			if err != nil {
				contextutils.LoggerFrom(ctx).Debugw("failed to sync status; will try again shortly.", "error", err)
			}
		}
	}
}

func (s *statusSyncer) syncStatus(ctx context.Context) error {
	var nilProxy *gloov1.Proxy
	allReports := reporter.ResourceReports{}
	subresourceStatuses := map[resources.InputResource]map[string]*core.Status{}
	func() {
		s.mapLock.RLock()
		defer s.mapLock.RUnlock()
		for ref, reportsAndStatus := range s.proxyToLastStatus {
			_, inDesiredProxies := s.currentGeneratedProxies[ref]
			if !inDesiredProxies {
				continue
			}
			// merge all the reports for the vs from all the proxies.
			for k, v := range reportsAndStatus.Reports {
				if reportsAndStatus.Status != nil {
					// add the proxy status as well if we have it
					status := *reportsAndStatus.Status
					if _, ok := subresourceStatuses[k]; !ok {
						subresourceStatuses[k] = map[string]*core.Status{}
					}
					subresourceStatuses[k][fmt.Sprintf("%T.%s", nilProxy, ref.Key())] = &status
				}
				if report, ok := allReports[k]; ok {
					if v.Errors != nil {
						report.Errors = multierror.Append(report.Errors, v.Errors)
					}
					if v.Warnings != nil {
						report.Warnings = append(report.Warnings, v.Warnings...)
					}
				} else {
					allReports[k] = v
				}
			}
		}
	}()

	if len(subresourceStatuses) == 0 {
		subresourceStatuses = nil
	}
	var errs error
	for k, v := range allReports {
		reports := reporter.ResourceReports{k: v}
		if err := s.reporter.WriteReports(ctx, reports, subresourceStatuses[k]); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	return errs
}
