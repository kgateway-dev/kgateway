package translator

import (
	"context"
	"time"

	v1machinery "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/pkg/utils/syncutil"
	"github.com/solo-io/go-utils/hashutils"
	"go.uber.org/zap/zapcore"

	"github.com/rotisserie/eris"
	v1alpha1 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/external/knative"
	v1 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	knativev1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	knativeclient "knative.dev/networking/pkg/client/clientset/versioned/typed/networking/v1alpha1"
)

type translatorSyncer struct {
	proxyAddress    string
	writeNamespace  string
	writeErrs       chan error
	proxyClient     gloov1.ProxyClient
	proxyReconciler gloov1.ProxyReconciler
	ingressClient   knativeclient.IngressesGetter
}

func NewSyncer(proxyAddress, writeNamespace string, proxyClient gloov1.ProxyClient, ingressClient knativeclient.IngressesGetter, writeErrs chan error) v1.TranslatorSyncer {
	return &translatorSyncer{
		proxyAddress:    proxyAddress,
		writeNamespace:  writeNamespace,
		writeErrs:       writeErrs,
		proxyClient:     proxyClient,
		ingressClient:   ingressClient,
		proxyReconciler: gloov1.NewProxyReconciler(proxyClient),
	}
}

// TODO (ilackarms): make sure that sync happens if proxies get updated as well; may need to resync
func (s *translatorSyncer) Sync(ctx context.Context, snap *v1.TranslatorSnapshot) error {
	ctx = contextutils.WithLogger(ctx, "translatorSyncer")

	snapHash := hashutils.MustHash(snap)
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("begin sync %v (%v cluster ingresses )", snapHash,
		len(snap.Clusteringresses),
	)
	defer logger.Infof("end sync %v", snapHash)

	// stringifying the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	if contextutils.GetLogLevel() == zapcore.DebugLevel {
		logger.Debug(syncutil.StringifySnapshot(snap))
	}

	proxy, err := translateProxy(ctx, s.writeNamespace, snap)
	if err != nil {
		logger.Warnf("snapshot %v was rejected due to invalid config: %v\n"+
			"knative ingress proxy will not be updated.", snapHash, err)
		return err
	}

	labels := map[string]string{
		"created_by": "knative",
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

	if err := s.propagateProxyStatus(ctx, proxy, snap.Clusteringresses); err != nil {
		return eris.Wrapf(err, "failed to propagate proxy status "+
			"to clusteringress objects")
	}

	return nil
}

// propagate to all clusteringresses the status of the proxy
func (s *translatorSyncer) propagateProxyStatus(ctx context.Context, proxy *gloov1.Proxy, clusterIngresses v1alpha1.ClusterIngressList) error {
	if proxy == nil {
		return nil
	}
	timeout := time.After(time.Second * 30)
	ticker := time.Tick(time.Second / 2)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timeout:
			return eris.Errorf("timed out waiting for proxy status to be updated")
		case <-ticker:
			// poll the proxy for an accepted or rejected status
			updatedProxy, err := s.proxyClient.Read(
				proxy.GetMetadata().GetNamespace(),
				proxy.GetMetadata().GetName(),
				clients.ReadOpts{Ctx: ctx},
			)
			if err != nil {
				return err
			}
			switch updatedProxy.GetStatus().GetState() {
			case core.Status_Pending:
				continue
			case core.Status_Rejected:
				contextutils.LoggerFrom(ctx).Errorf("proxy was rejected by gloo: %v",
					updatedProxy.GetStatus().GetReason())
				return nil
			case core.Status_Accepted:
				return s.markClusterIngressesReady(ctx, clusterIngresses)
			}
		}
	}
}

func (s *translatorSyncer) markClusterIngressesReady(ctx context.Context, clusterIngresses v1alpha1.ClusterIngressList) error {
	var updatedClusterIngresses []*knativev1alpha1.Ingress
	for _, wrappedCi := range clusterIngresses {
		ci := knativev1alpha1.Ingress(wrappedCi.ClusterIngress)
		if ci.Status.ObservedGeneration == ci.ObjectMeta.Generation {
			continue
		}
		ci.Status.InitializeConditions()
		ci.Status.MarkNetworkConfigured()
		lb := []knativev1alpha1.LoadBalancerIngressStatus{
			{DomainInternal: s.proxyAddress},
		}
		ci.Status.MarkLoadBalancerReady(lb, lb)
		ci.Status.ObservedGeneration = ci.Generation
		updatedClusterIngresses = append(updatedClusterIngresses, &ci)
	}
	for _, ci := range updatedClusterIngresses {
		if _, err := s.ingressClient.Ingresses(ci.Namespace).UpdateStatus(ctx, ci, v1machinery.UpdateOptions{}); err != nil {
			contextutils.LoggerFrom(ctx).Errorf("failed to update ClusterIngress %v status with error %v", ci.Name, err)
		}
	}
	return nil
}
