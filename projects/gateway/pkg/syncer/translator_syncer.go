package syncer

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"

	"github.com/solo-io/gloo/projects/gateway/pkg/utils/metrics"
	gloo_translator "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/errors"
	"go.uber.org/zap/zapcore"

	"github.com/hashicorp/go-multierror"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	"github.com/solo-io/gloo/pkg/utils/syncutil"
	"github.com/solo-io/gloo/projects/gateway/pkg/reconciler"
	"github.com/solo-io/go-utils/hashutils"
	"go.uber.org/zap"

	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/compress"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

type TranslatorSyncer struct {
	writeNamespace     string
	reporter           reporter.Reporter
	proxyReconciler    reconciler.ProxyReconciler
	translator         translator.Translator
	statusSyncer       statusSyncer
	proxyStatusMaxSize string
}

var (
	// labels used to uniquely identify Proxies that are managed by the Gloo controllers
	proxyLabelsToWrite = map[string]string{
		"created_by": "gloo-gateway-translator",
	}

	// Previously, proxies would be identified with:
	//   created_by: gateway
	// Now, proxies are identified with:
	//   created_by: gloo-gateway-translator
	//
	// We need to ensure that users can successfully upgrade from versions
	// where the previous labels were used, to versions with the new labels.
	// Therefore, we watch Proxies with a superset of the old and new labels, and persist Proxies with new labels.
	//
	// This is only required for backwards compatibility.
	// Once users have upgraded to a version with new labels, we can delete this code and read/write the same labels.
	proxyLabelSelectorOptions = clients.ListOpts{
		ExpressionSelector: "created_by in (gloo-gateway-translator, gateway)",
	}
)

func NewTranslatorSyncer(ctx context.Context, writeNamespace string, proxyWatcher gloov1.ProxyClient, proxyReconciler reconciler.ProxyReconciler, reporter reporter.StatusReporter, translator translator.Translator, statusClient resources.StatusClient, statusMetrics metrics.ConfigStatusMetrics, identity leaderelector.Identity) *TranslatorSyncer {
	t := &TranslatorSyncer{
		writeNamespace:  writeNamespace,
		reporter:        reporter,
		proxyReconciler: proxyReconciler,
		translator:      translator,
		statusSyncer:    newStatusSyncer(writeNamespace, proxyWatcher, reporter, statusClient, statusMetrics, identity),
	}
	if pxStatusSizeEnv := os.Getenv("PROXY_STATUS_MAX_SIZE_BYTES"); pxStatusSizeEnv != "" {
		t.proxyStatusMaxSize = pxStatusSizeEnv
	}
	go t.statusSyncer.syncStatusOnEmit(ctx)
	return t
}

// TODO (ilackarms): make sure that sync happens if proxies get updated as well; may need to resync
func (s *TranslatorSyncer) Sync(ctx context.Context, snap *gloov1snap.ApiSnapshot) error {
	ctx = contextutils.WithLogger(ctx, "TranslatorSyncer")
	logger := contextutils.LoggerFrom(ctx)

	snapHash := hashutils.MustHash(snap)
	logger.Infof("begin sync %v (%v virtual services, %v gateways, %v route tables)", snapHash,
		len(snap.VirtualServices), len(snap.Gateways), len(snap.RouteTables))
	defer logger.Infof("end sync %v", snapHash)

	// stringify-ing the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	if contextutils.GetLogLevel() == zapcore.DebugLevel {
		logger.Debug(syncutil.StringifySnapshot(snap))
	}

	desiredProxies, invalidProxies := s.GeneratedDesiredProxies(ctx, snap)

	return s.reconcile(ctx, desiredProxies, invalidProxies)
}

// This replaced a watch on the proxy CR from when the gloo and gateway pods were separate
// Now it is called at the end of the gloo translation loop after statuses have been set for proxies
// This is where we update statuses on gateway types based on the proxy statuses
func (s *TranslatorSyncer) UpdateProxies(ctx context.Context) {
	s.statusSyncer.handleUpdatedProxies(ctx)
}
func (s *TranslatorSyncer) GeneratedDesiredProxies(ctx context.Context, snap *gloov1snap.ApiSnapshot) (reconciler.GeneratedProxies, reconciler.InvalidProxies) {
	logger := contextutils.LoggerFrom(ctx)
	gatewaysByProxyName := utils.GatewaysByProxyName(snap.Gateways)

	desiredProxies := make(reconciler.GeneratedProxies)
	invalidProxies := make(reconciler.InvalidProxies)
	for proxyName, gatewayList := range gatewaysByProxyName {
		proxy, reports := s.translator.Translate(ctx, proxyName, snap, gatewayList)
		if proxy != nil {

			if s.shouldCompresss(ctx) {
				compress.SetShouldCompressed(proxy)
			}
			if s.proxyStatusMaxSize != "" {
				if err := compress.SetMaxStatusSizeBytes(proxy, s.proxyStatusMaxSize); err != nil {
					logger.Warnf("Could not parse the maximum status size for the proxy, statuses will not be truncated. Setting %s error: %v",
						s.proxyStatusMaxSize, err)
				}
			}
			logger.Infof("desired proxy %v", proxy.GetMetadata().Ref())
			proxy.GetMetadata().Labels = proxyLabelsToWrite
			desiredProxies[proxy] = reports
		} else {
			// We were unable to create a proxy
			// Ensure that reports for that proxy are propagated to the relevant gateway resources
			invalidProxyRef := &core.ResourceRef{
				Name:      proxyName,
				Namespace: s.writeNamespace,
			}
			invalidProxies[invalidProxyRef] = reports
		}
	}
	return desiredProxies, invalidProxies
}

func (s *TranslatorSyncer) shouldCompresss(ctx context.Context) bool {
	return settingsutil.MaybeFromContext(ctx).GetGateway().GetCompressedProxySpec()
}
func (s *TranslatorSyncer) reconcile(ctx context.Context, desiredProxies reconciler.GeneratedProxies, invalidProxies reconciler.InvalidProxies) error {
	if err := s.proxyReconciler.ReconcileProxies(ctx, desiredProxies, s.writeNamespace, proxyLabelSelectorOptions); err != nil {
		return err
	}

	// repeat for all resources
	s.statusSyncer.setCurrentProxies(desiredProxies, invalidProxies)
	s.statusSyncer.forceSync()
	return nil
}

type reportsAndStatus struct {
	Status  *core.Status
	Reports reporter.ResourceReports
}
type statusSyncer struct {
	proxyToLastStatus       map[string]reportsAndStatus
	inputResourceLastStatus map[resources.InputResource]*core.Status
	currentGeneratedProxies []*core.ResourceRef
	mapLock                 sync.RWMutex
	reporter                reporter.StatusReporter

	proxyClient             gloov1.ProxyClient
	writeNamespace          string
	statusClient            resources.StatusClient
	statusMetrics           metrics.ConfigStatusMetrics
	syncNeeded              chan struct{}
	previousProxyStatusHash uint64

	identity leaderelector.Identity
}

func newStatusSyncer(writeNamespace string, proxyClient gloov1.ProxyClient, reporter reporter.StatusReporter, statusClient resources.StatusClient, statusMetrics metrics.ConfigStatusMetrics, identity leaderelector.Identity) statusSyncer {
	return statusSyncer{
		proxyToLastStatus:       map[string]reportsAndStatus{},
		currentGeneratedProxies: nil,
		reporter:                reporter,
		proxyClient:             proxyClient,
		writeNamespace:          writeNamespace,
		statusClient:            statusClient,
		statusMetrics:           statusMetrics,
		syncNeeded:              make(chan struct{}, 1),
		identity:                identity,
	}
}

func (s *statusSyncer) setCurrentProxies(desiredProxies reconciler.GeneratedProxies, invalidProxies reconciler.InvalidProxies) {
	s.mapLock.Lock()
	defer s.mapLock.Unlock()
	// clear out the status map
	s.inputResourceLastStatus = make(map[resources.InputResource]*core.Status)
	s.currentGeneratedProxies = nil

	// List of refs to proxies
	// This includes both proxies that are valid and invalid
	proxyReportsByRef := make(map[*core.ResourceRef]reporter.ResourceReports)
	for proxy, reports := range desiredProxies {
		ref := proxy.GetMetadata().Ref()
		proxyReportsByRef[ref] = reports
	}
	for ref, reports := range invalidProxies {
		proxyReportsByRef[ref] = reports
	}

	// Tech Debt:  we've identified that it's not useful to have both two parallel data structures
	//		proxyToLastStatus       map[string]reportsAndStatus
	// 		currentGeneratedProxies []*core.ResourceRef
	// floating around.  Historically, they were there to envorce an alphabetical processing of
	//  `proxyToLastStatus`.  See https://github.com/solo-io/gloo/issues/5812 for more details.
	for proxyRef, reports := range proxyReportsByRef {
		refKey := gloo_translator.UpstreamToClusterName(proxyRef)
		if _, ok := s.proxyToLastStatus[refKey]; !ok {
			s.proxyToLastStatus[refKey] = reportsAndStatus{}
		}
		current := s.proxyToLastStatus[refKey]
		// These reports are for gateway resources: VirtualServices, RouteTables and Gateways
		current.Reports = reports
		s.proxyToLastStatus[refKey] = current
		s.currentGeneratedProxies = append(s.currentGeneratedProxies, proxyRef)
	}

	// To ensure that reports are generated in the same order, we sort the proxies.
	// Without this sorting, the same resources could produce a report with
	// the warnings/errors in a different order. This would cause statuses to be
	// updated unnecessarily.
	sort.SliceStable(s.currentGeneratedProxies, func(i, j int) bool {
		refi := s.currentGeneratedProxies[i]
		refj := s.currentGeneratedProxies[j]
		if refi.GetNamespace() != refj.GetNamespace() {
			return refi.GetNamespace() < refj.GetNamespace()
		}
		return refi.GetName() < refj.GetName()
	})
}

func (s *statusSyncer) handleUpdatedProxies(ctx context.Context) {
	logger := contextutils.LoggerFrom(ctx)
	proxyList, err := s.proxyClient.List(s.writeNamespace, clients.ListOpts{
		Ctx: ctx,
	})
	if err != nil {
		// We don't return errors from this because we don't want to fail the rest of gloo translation
		logger.Errorw("Error reading updated proxies, statuses may be out of date.", err)
		return
	}
	currentHash, err := s.hashStatuses(proxyList)
	if err != nil {
		logger.DPanicw("error while hashing, this should never happen", zap.Error(err))
	}
	// We use hashing here to be compatible with the memory client used in
	// the local e2e; it fires a watch update too all watch object, on any change,
	// this means that setting by the status of a virtual service we will get another
	// proxyList from the channel. This results in excessive CPU usage in CI.
	if currentHash != s.previousProxyStatusHash {
		logger.Debugw("proxy list updated", "len(proxyList)", len(proxyList), "currentHash", currentHash, "previousProxyStatusHash", s.previousProxyStatusHash)
		s.previousProxyStatusHash = currentHash
		s.setStatuses(proxyList)
		s.forceSync()
	}
}

func (s *statusSyncer) hashStatuses(proxyList gloov1.ProxyList) (uint64, error) {
	statuses := make([]interface{}, 0, len(proxyList))
	for _, proxy := range proxyList {

		statuses = append(statuses, s.statusClient.GetStatus(proxy))
	}
	return hashutils.HashAllSafe(nil, statuses...)
}

func (s *statusSyncer) setStatuses(list gloov1.ProxyList) {
	s.mapLock.Lock()
	defer s.mapLock.Unlock()
	for _, proxy := range list {
		ref := proxy.GetMetadata().Ref()
		refKey := gloo_translator.UpstreamToClusterName(ref)
		status := s.statusClient.GetStatus(proxy)
		if current, ok := s.proxyToLastStatus[refKey]; ok {
			current.Status = status
			s.proxyToLastStatus[refKey] = current
		} else {
			s.proxyToLastStatus[refKey] = reportsAndStatus{
				Status: status,
			}
		}
	}
}

func (s *statusSyncer) forceSync() {
	select {
	case s.syncNeeded <- struct{}{}:
	default:
	}
}

func (s *statusSyncer) syncStatusOnEmit(ctx context.Context) error {
	var retryChan <-chan time.Time

	doSync := func() {
		err := s.syncStatus(ctx)
		if err != nil {
			contextutils.LoggerFrom(ctx).Debugw("failed to sync status; will try again shortly.", "error", err)
			retryChan = time.After(time.Second)
		} else {
			retryChan = nil
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-retryChan:
			doSync()
		case <-s.syncNeeded:
			doSync()
		}
	}
}

// extractCurrentReports massages several asynchronously set `statusSyncer` variables into formats consumable by `syncStatus`
func (s *statusSyncer) extractCurrentReports() (reporter.ResourceReports, map[resources.InputResource]map[string]*core.Status, map[resources.InputResource]*core.Status) {
	var nilProxy *gloov1.Proxy
	allReports := reporter.ResourceReports{}
	inputResourceBySubresourceStatuses := map[resources.InputResource]map[string]*core.Status{}
	var localInputResourceLastStatus map[resources.InputResource]*core.Status

	s.mapLock.RLock()
	defer s.mapLock.RUnlock()
	// grab a local copy of the map. it only updated here, and the variable is cleared under the lock; so is safe.
	localInputResourceLastStatus = s.inputResourceLastStatus

	var refKeys []string
	for _, ref := range s.currentGeneratedProxies {
		refKeys = append(refKeys, gloo_translator.UpstreamToClusterName(ref))
	}

	// iterate over proxyToLastStatus by alphabetical ordering of keys
	for _, refKey := range refKeys {
		reportsAndStatus, ok := s.proxyToLastStatus[refKey]
		if !ok {
			continue
		}

		// merge reports that share an inputResource
		for inputResource, newReport := range reportsAndStatus.Reports {
			if existingReport, ok := allReports[inputResource]; ok {
				// combine `existingStatus` and `newReport`
				if newReport.Errors != nil {
					existingReport.Errors = multierror.Append(existingReport.Errors, newReport.Errors)
				}
				if newReport.Warnings != nil {
					existingReport.Warnings = append(existingReport.Warnings, newReport.Warnings...)
				}
				allReports[inputResource] = existingReport
			} else {
				// add `newStatus` to allReports
				allReports[inputResource] = newReport
			}

			if reportsAndStatus.Status != nil {
				// add the proxy status as well if we have it
				status := *reportsAndStatus.Status
				if _, ok := inputResourceBySubresourceStatuses[inputResource]; !ok {
					inputResourceBySubresourceStatuses[inputResource] = map[string]*core.Status{}
				}
				inputResourceBySubresourceStatuses[inputResource][fmt.Sprintf("%T.%s", nilProxy, refKey)] = &status
			}
		}
	}

	return allReports, inputResourceBySubresourceStatuses, localInputResourceLastStatus
}

func (s *statusSyncer) syncStatus(ctx context.Context) error {
	allReports, inputResourceBySubresourceStatuses, localInputResourceLastStatus := s.extractCurrentReports()

	var errs error
	for inputResource, subresourceStatuses := range allReports {
		// write reports may update the status, so clone the object
		clonedInputResource := resources.Clone(inputResource).(resources.InputResource)
		// set the last known status on the input resource.
		// this may be different than the status on the snapshot, as the snapshot doesn't get updated
		// on status changes.
		if status, ok := localInputResourceLastStatus[inputResource]; ok {
			s.statusClient.SetStatus(clonedInputResource, status)
		}

		reports := reporter.ResourceReports{clonedInputResource: subresourceStatuses}
		currentStatuses := inputResourceBySubresourceStatuses[inputResource]

		if s.identity.IsLeader() {
			// Only leaders will write reports
			//
			// while tempting to write statuses in parallel to increase performance, we should actually first consider recommending the user tunes k8s qps/burst:
			// https://github.com/solo-io/gloo/blob/a083522af0a4ce22f4d2adf3a02470f782d5a865/projects/gloo/api/v1/settings.proto#L337-L350
			if err := s.reporter.WriteReports(ctx, reports, currentStatuses); err != nil {
				// add TEMPORARY wrap to our WriteReports error that we should remove in Gloo Edge ~v1.16.0+.
				// to get the status performance improvements, we need to make the assumption that the user has the latest CRDs installed.
				// if a user forgets the error message is very confusing (invalid request during kubectl patch);
				// this should help them understand what's going on in case they did not read the changelog.
				wrappedErr := errors.Wrapf(err, "failed to write reports for %v;"+
					"did you make sure your CRDs have been updated since v1.12.17? (i.e. `status` and `status.statuses` fields exist on your CR)", inputResource.GetMetadata().Ref().Key())
				errs = multierror.Append(errs, wrappedErr)
			} else {
				// The inputResource's status was successfully written, update the cache and metric with that status
				status := s.reporter.StatusFromReport(subresourceStatuses, currentStatuses)
				localInputResourceLastStatus[inputResource] = status
			}
		} else {
			contextutils.LoggerFrom(ctx).Debug("Not a leader, skipping reports writing")
		}

		status := s.reporter.StatusFromReport(subresourceStatuses, currentStatuses)
		s.statusMetrics.SetResourceStatus(ctx, inputResource, status)
	}
	return errs
}
