package translator

import (
	"context"

	"github.com/solo-io/gloo/projects/clusteringress/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/utils/contextutils"
)

type translatorSyncer struct {
	writeNamespace       string
	writeErrs            chan error
	proxyClient          gloov1.ProxyClient
	clusterIngressClient v1.ClusterIngressClient
	proxyReconciler      gloov1.ProxyReconciler
}

func NewSyncer(writeNamespace string, proxyClient gloov1.ProxyClient, ingressClient v1.ClusterIngressClient, writeErrs chan error) v1.TranslatorSyncer {
	return &translatorSyncer{
		writeNamespace:       writeNamespace,
		writeErrs:            writeErrs,
		proxyClient:          proxyClient,
		clusterIngressClient: ingressClient,
		proxyReconciler:      gloov1.NewProxyReconciler(proxyClient),
	}
}

// TODO (ilackarms): make sure that sync happens if proxies get updated as well; may need to resync
func (s *translatorSyncer) Sync(ctx context.Context, snap *v1.TranslatorSnapshot) error {
	ctx = contextutils.WithLogger(ctx, "translatorSyncer")

	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("begin sync %v (%v cluster ingresses)", snap.Hash(),
		len(snap.Clusteringresses.List()))
	defer logger.Infof("end sync %v", snap.Hash())
	logger.Debugf("%v", snap)

	proxy, err := translateProxy(s.writeNamespace, snap)
	if err != nil {
		logger.Warnf("snapshot %v was rejected due to invalid config: %v\n"+
			"knative ingress proxy will not be updated.", snap.Hash(), err)
		return err
	}

	labels := map[string]string{
		"created_by": "knative",
	}

	var desiredResources gloov1.ProxyList
	if proxy != nil {
		logger.Infof("creating proxy %v", proxy.Metadata.Ref())
		proxy.Metadata.Labels = labels
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
