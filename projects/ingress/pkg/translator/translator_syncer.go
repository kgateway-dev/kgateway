package translator

import (
	"context"

	"github.com/solo-io/gloo/pkg/utils/syncutil"
	"github.com/solo-io/go-utils/hashutils"
	"go.uber.org/zap/zapcore"

	"github.com/solo-io/gloo/projects/gateway/pkg/utils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/ingress/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

type translatorSyncer struct {
	writeNamespace      string
	writeErrs           chan error
	proxyClient         gloov1.ProxyClient
	ingressClient       v1.IngressClient
	proxyReconciler     gloov1.ProxyReconciler
	requireIngressClass bool

	// support custom ingress class.
	// only relevant when requireIngressClass is true.
	// defaults to 'gloo'
	customIngressClass string
}

func NewSyncer(writeNamespace string, proxyClient gloov1.ProxyClient, ingressClient v1.IngressClient, writeErrs chan error, requireIngressClass bool, customIngressClass string) v1.TranslatorSyncer {
	return &translatorSyncer{
		writeNamespace:      writeNamespace,
		writeErrs:           writeErrs,
		proxyClient:         proxyClient,
		ingressClient:       ingressClient,
		proxyReconciler:     gloov1.NewProxyReconciler(proxyClient),
		requireIngressClass: requireIngressClass,
		customIngressClass:  customIngressClass,
	}
}

// TODO (ilackarms): make sure that sync happens if proxies get updated as well; may need to resync
func (s *translatorSyncer) Sync(ctx context.Context, snap *v1.TranslatorSnapshot) error {
	ctx = contextutils.WithLogger(ctx, "translatorSyncer")

	snapHash := hashutils.MustHash(snap)
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("begin sync %v (%v ingresses)", snapHash,
		len(snap.Ingresses))
	defer logger.Infof("end sync %v", snapHash)

	// stringifying the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	if contextutils.GetLogLevel() == zapcore.DebugLevel {
		logger.Debug(syncutil.StringifySnapshot(snap))
	}

	proxy := translateProxy(ctx, s.writeNamespace, snap, s.requireIngressClass, s.customIngressClass)

	labels := map[string]string{
		"created_by": "ingress",
	}

	var desiredResources gloov1.ProxyList
	if proxy != nil {
		logger.Infof("creating proxy %v", proxy.GetMetadata().Ref())
		proxy.GetMetadata().Labels = labels
		desiredResources = gloov1.ProxyList{proxy}
	}

	if err := s.proxyReconciler.Reconcile(s.writeNamespace, desiredResources, utils.TransitionFunction, clients.ListOpts{
		Ctx:      ctx,
		Selector: labels,
	}); err != nil {
		return err
	}

	return nil
}
