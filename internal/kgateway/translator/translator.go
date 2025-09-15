package translator

import (
	"context"
	"log/slog"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"istio.io/istio/pkg/kube/krt"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/endpoints"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query"
	gwtranslator "github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/gateway"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

var logger = logging.New("translator")

// Combines all the translators needed for xDS translation.
type CombinedTranslator struct {
	extensions extensionsplug.Plugin
	commonCols *common.CommonCollections

	waitForSync []cache.InformerSynced

	gwtranslator      extensionsplug.KGwTranslator
	irtranslator      *irtranslator.Translator
	backendTranslator *irtranslator.BackendTranslator
	endpointPlugins   []extensionsplug.EndpointPlugin

	logger *slog.Logger
}

func NewCombinedTranslator(
	ctx context.Context,
	extensions extensionsplug.Plugin,
	commonCols *common.CommonCollections,
) *CombinedTranslator {
	var endpointPlugins []extensionsplug.EndpointPlugin
	for _, ext := range extensions.ContributesPolicies {
		if ext.PerClientProcessEndpoints != nil {
			endpointPlugins = append(endpointPlugins, ext.PerClientProcessEndpoints)
		}
	}
	return &CombinedTranslator{
		commonCols:      commonCols,
		extensions:      extensions,
		endpointPlugins: endpointPlugins,
		logger:          logger,
		waitForSync:     []cache.InformerSynced{extensions.HasSynced},
	}
}

func (s *CombinedTranslator) Init(ctx context.Context) {
	queries := query.NewData(s.commonCols)

	listenerTranslatorConfig := gwtranslator.TranslatorConfig{}
	listenerTranslatorConfig.ListenerTranslatorConfig.ListenerBindIpv6 = s.commonCols.Settings.ListenerBindIpv6

	s.gwtranslator = gwtranslator.NewTranslator(queries, listenerTranslatorConfig)
	s.irtranslator = &irtranslator.Translator{
		ContributedPolicies:  s.extensions.ContributesPolicies,
		RouteReplacementMode: s.commonCols.Settings.RouteReplacementMode,
		Validator:            validator.New(), // TODO: define this once for RDS and TP plugin.
	}
	s.backendTranslator = &irtranslator.BackendTranslator{
		ContributedBackends: make(map[schema.GroupKind]ir.BackendInit),
		ContributedPolicies: s.extensions.ContributesPolicies,
		CommonCols:          s.commonCols,
	}
	for k, up := range s.extensions.ContributesBackends {
		s.backendTranslator.ContributedBackends[k] = up.BackendInit
	}

	s.waitForSync = append(s.waitForSync,
		s.commonCols.HasSynced,
		s.extensions.HasSynced,
	)
}

func (s *CombinedTranslator) HasSynced() bool {
	for _, sync := range s.waitForSync {
		if !sync() {
			return false
		}
	}
	return true
}

// buildProxy performs translation of a kube Gateway -> GatewayIR
func (s *CombinedTranslator) buildProxy(kctx krt.HandlerContext, ctx context.Context, gw ir.Gateway, r reports.Reporter) *ir.GatewayIR {
	stopwatch := utils.NewTranslatorStopWatch("CombinedTranslator")
	stopwatch.Start()

	var gatewayTranslator extensionsplug.KGwTranslator = s.gwtranslator
	if s.extensions.ContributesGwTranslator != nil {
		maybeGatewayTranslator := s.extensions.ContributesGwTranslator(gw.Obj)
		if maybeGatewayTranslator != nil {
			gatewayTranslator = maybeGatewayTranslator
		}
	}
	proxy := gatewayTranslator.Translate(kctx, ctx, &gw, r)
	if proxy == nil {
		return nil
	}

	duration := stopwatch.Stop(ctx)
	logger.Debug("translated proxy", "namespace", gw.Namespace, "name", gw.Name, "duration", duration.String())

	return proxy
}

func (s *CombinedTranslator) GetUpstreamTranslator() *irtranslator.BackendTranslator {
	return s.backendTranslator
}

// ctx needed for logging; remove once we refactor logging.
func (s *CombinedTranslator) TranslateGateway(kctx krt.HandlerContext, ctx context.Context, gw ir.Gateway) (*irtranslator.TranslationResult, reports.ReportMap) {
	rm := reports.NewReportMap()
	r := reports.NewReporter(&rm)
	logger.Debug("translating Gateway", "resource_ref", gw.ResourceName(), "resource_version", gw.Obj.GetResourceVersion())

	gwir := s.buildProxy(kctx, ctx, gw, r)
	if gwir == nil {
		return nil, reports.ReportMap{}
	}

	// we are recomputing xds snapshots as proxies have changed, signal that we need to sync xds with these new snapshots
	xdsSnap := s.irtranslator.Translate(ctx, *gwir, r)

	return &xdsSnap, rm
}

func (s *CombinedTranslator) TranslateEndpoints(kctx krt.HandlerContext, ucc ir.UniqlyConnectedClient, ep ir.EndpointsForBackend) (*envoyendpointv3.ClusterLoadAssignment, uint64) {
	epInputs := endpoints.EndpointsInputs{
		EndpointsForBackend: ep,
	}
	var hash uint64
	for _, processEndpoints := range s.endpointPlugins {
		additionalHash := processEndpoints(kctx, context.TODO(), ucc, &epInputs)
		hash ^= additionalHash
	}
	return endpoints.PrioritizeEndpoints(s.logger, ucc, epInputs), hash
}
