package syncer

import (
	"context"
	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils/metrics"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/hashicorp/go-multierror"
	gwsyncer "github.com/solo-io/gloo/projects/gateway/pkg/syncer"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

type translatorSyncer struct {
	translator translator.Translator
	sanitizer  sanitizer.XdsSanitizer
	xdsCache   envoycache.SnapshotCache
	reporter   reporter.StatusReporter

	syncerExtensions []TranslatorSyncerExtension
	settings         *v1.Settings
	statusMetrics    metrics.ConfigStatusMetrics
	gatewaySyncer    *gwsyncer.TranslatorSyncer
	proxyClient      v1.ProxyClient
	writeNamespace   string

	identity leaderelector.Identity

	// used for debugging purposes only
	latestSnap *v1snap.ApiSnapshot
}

func NewTranslatorSyncer(
	translator translator.Translator,
	xdsCache envoycache.SnapshotCache,
	sanitizer sanitizer.XdsSanitizer,
	reporter reporter.StatusReporter,
	devMode bool,
	extensions []TranslatorSyncerExtension,
	settings *v1.Settings,
	statusMetrics metrics.ConfigStatusMetrics,
	gatewaySyncer *gwsyncer.TranslatorSyncer,
	proxyClient v1.ProxyClient,
	writeNamespace string,
	identity leaderelector.Identity,
) v1snap.ApiSyncer {
	s := &translatorSyncer{
		translator:       translator,
		xdsCache:         xdsCache,
		reporter:         reporter,
		syncerExtensions: extensions,
		sanitizer:        sanitizer,
		settings:         settings,
		statusMetrics:    statusMetrics,
		gatewaySyncer:    gatewaySyncer,
		proxyClient:      proxyClient,
		writeNamespace:   writeNamespace,
		identity:         identity,
	}
	if devMode {
		// TODO(ilackarms): move this somewhere else?
		go func() {
			_ = s.ServeXdsSnapshots()
		}()
	}

	return s
}

func (s *translatorSyncer) Sync(ctx context.Context, snap *v1snap.ApiSnapshot) error {
	logger := contextutils.LoggerFrom(ctx)
	reports := make(reporter.ResourceReports)
	var multiErr *multierror.Error

	// If gateway controller is enabled, run the gateway translation to generate proxies.
	// Use the ProxyClient interface to persist them either to an in-memory store or etcd as configured at startup.
	if s.gatewaySyncer != nil {
		logger.Debugf("getting proxies from gateway translation")
		if err := s.translateProxies(ctx, snap); err != nil {
			multiErr = multierror.Append(multiErr, eris.Wrapf(err, "translating proxies"))
		}
	}

	// Execute the EnvoySyncer
	// This will update the xDS SnapshotCache for each entry that corresponds to a Proxy in the API Snapshot
	s.syncEnvoy(ctx, snap, reports)

	// Execute the SyncerExtensions
	// Each of these are responsible for updating a single entry in the SnapshotCache
	for _, syncerExtension := range s.syncerExtensions {
		intermediateReports := make(reporter.ResourceReports)
		syncerExtension.Sync(ctx, snap, s.settings, s.xdsCache, intermediateReports)
		reports.Merge(intermediateReports)
	}

	if s.identity.IsLeader() {
		// Only leaders will write reports
		if err := s.reporter.WriteReports(ctx, reports, nil); err != nil {
			logger.Debugf("Failed writing report for proxies: %v", err)
			multiErr = multierror.Append(multiErr, eris.Wrapf(err, "writing reports"))
		}
		// Update resource status metrics
		for resource, report := range reports {
			status := s.reporter.StatusFromReport(report, nil)
			s.statusMetrics.SetResourceStatus(ctx, resource, status)
		}
	} else {
		logger.Debugf("Not a leader, skipping status and metrics reporting")
	}

	//After reports are written for proxies, save in gateway syncer (previously gw watched for status changes to proxies)
	if s.gatewaySyncer != nil {
		s.gatewaySyncer.UpdateProxies(ctx)
	}
	return multiErr.ErrorOrNil()
}
func (s *translatorSyncer) translateProxies(ctx context.Context, snap *v1snap.ApiSnapshot) error {
	err := s.gatewaySyncer.Sync(ctx, snap)
	proxyList, err := s.proxyClient.List(s.writeNamespace, clients.ListOpts{})
	snap.Proxies = proxyList
	return err
}
