package translator

import (
	"context"
	"time"

	"github.com/solo-io/gloo/pkg/utils/syncutil"
	"github.com/solo-io/go-utils/hashutils"
	"go.uber.org/zap/zapcore"

	"golang.org/x/sync/errgroup"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1alpha1 "github.com/solo-io/gloo/projects/knative/pkg/api/external/knative"
	v1 "github.com/solo-io/gloo/projects/knative/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	v1machinery "k8s.io/apimachinery/pkg/apis/meta/v1"
	knativev1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	knativeclient "knative.dev/networking/pkg/client/clientset/versioned/typed/networking/v1alpha1"
)

type translatorSyncer struct {
	externalProxyAddress string
	internalProxyAddress string
	writeNamespace       string
	writeErrs            chan error
	proxyClient          gloov1.ProxyClient
	proxyReconciler      gloov1.ProxyReconciler
	ingressClient        knativeclient.IngressesGetter
	requireIngressClass  bool

	// injection for testing
	translateProxy func(ctx context.Context, proxyName, proxyNamespace string, ingresses v1alpha1.IngressList) (*gloov1.Proxy, error)
}

func NewSyncer(externalProxyAddress, internalProxyAddress, writeNamespace string, proxyClient gloov1.ProxyClient, ingressClient knativeclient.IngressesGetter, writeErrs chan error, requireIngressClass bool) v1.TranslatorSyncer {
	return &translatorSyncer{
		externalProxyAddress: externalProxyAddress,
		internalProxyAddress: internalProxyAddress,
		writeNamespace:       writeNamespace,
		writeErrs:            writeErrs,
		proxyClient:          proxyClient,
		ingressClient:        ingressClient,
		proxyReconciler:      gloov1.NewProxyReconciler(proxyClient),
		requireIngressClass:  requireIngressClass,
		translateProxy:       translateProxy,
	}
}

const (
	externalProxyName = "knative-external-proxy"
	internalProxyName = "knative-internal-proxy"
)

// enforce ingress class if requirement is set
func (s *translatorSyncer) shouldProcess(ingress *v1alpha1.Ingress) bool {
	if !s.requireIngressClass {
		return true
	}
	if len(ingress.Annotations) == 0 {
		return false
	}
	return ingress.Annotations[ingressClassAnnotation] == glooIngressClass
}

func (s *translatorSyncer) Sync(ctx context.Context, snap *v1.TranslatorSnapshot) error {
	ctx = contextutils.WithLogger(ctx, "translatorSyncer")

	snapHash := hashutils.MustHash(snap)
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("begin sync %v (%v knative ingresses)", snapHash,
		len(snap.Ingresses),
	)
	defer logger.Infof("end sync %v", snapHash)

	// stringifying the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	if contextutils.GetLogLevel() == zapcore.DebugLevel {
		logger.Debug(syncutil.StringifySnapshot(snap))
	}

	// split ingresses by their visibility, create a proxy for each
	var externalIngresses, internalIngresses v1alpha1.IngressList

	for _, ing := range snap.Ingresses {
		if !s.shouldProcess(ing) {
			continue
		}

		if ing.IsPublic() {
			externalIngresses = append(externalIngresses, ing)
		}
		internalIngresses = append(internalIngresses, ing)
	}

	externalProxy, err := s.translateProxy(ctx, externalProxyName, s.writeNamespace, externalIngresses)
	if err != nil {
		logger.Warnf("snapshot %v was rejected due to invalid config: %v\n"+
			"knative ingress externalProxy will not be updated.", snapHash, err)
		return err
	}

	internalProxy, err := s.translateProxy(ctx, internalProxyName, s.writeNamespace, internalIngresses)
	if err != nil {
		logger.Warnf("snapshot %v was rejected due to invalid config: %v\n"+
			"knative ingress externalProxy will not be updated.", snapHash, err)
		return err
	}

	labels := map[string]string{
		"created_by": "gloo-knative-translator",
	}

	var desiredResources gloov1.ProxyList
	if externalProxy != nil {
		logger.Infof("creating external proxy %v", externalProxy.GetMetadata().Ref())
		externalProxy.GetMetadata().Labels = labels
		desiredResources = append(desiredResources, externalProxy)
	}

	if internalProxy != nil {
		logger.Infof("creating internal proxy %v", internalProxy.GetMetadata().Ref())
		internalProxy.GetMetadata().Labels = labels
		desiredResources = append(desiredResources, internalProxy)
	}

	if err := s.proxyReconciler.Reconcile(s.writeNamespace, desiredResources, utils.TransitionFunction, clients.ListOpts{
		Ctx:      ctx,
		Selector: labels,
	}); err != nil {
		return err
	}

	g := &errgroup.Group{}
	g.Go(func() error {
		if err := s.propagateProxyStatus(ctx, externalProxy, externalIngresses); err != nil {
			return eris.Wrapf(err, "failed to propagate external proxy status "+
				"to ingress objects")
		}
		return nil
	})
	g.Go(func() error {
		if err := s.propagateProxyStatus(ctx, internalProxy, internalIngresses); err != nil {
			return eris.Wrapf(err, "failed to propagate internal proxy status "+
				"to ingress objects")
		}
		return nil
	})

	return nil
}

// propagate to all ingresses the status of the proxy
func (s *translatorSyncer) propagateProxyStatus(ctx context.Context, proxy *gloov1.Proxy, ingresses v1alpha1.IngressList) error {
	if proxy == nil {
		return nil
	}
	ticker := time.Tick(time.Second / 2)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker:
			// poll the proxy for an accepted or rejected status
			updatedProxy, err := s.proxyClient.Read(proxy.GetMetadata().GetNamespace(), proxy.GetMetadata().GetName(), clients.ReadOpts{Ctx: ctx})
			if err != nil {
				return err
			}
			switch updatedProxy.GetStatus().GetState() {
			case core.Status_Pending:
				continue
			case core.Status_Rejected:
				contextutils.LoggerFrom(ctx).Errorf("proxy was rejected by gloo: %v",
					updatedProxy.GetStatus().GetReason())
				continue
			case core.Status_Accepted:
				return s.markIngressesReady(ctx, ingresses)
			}
		}
	}
}

func (s *translatorSyncer) markIngressesReady(ctx context.Context, ingresses v1alpha1.IngressList) error {
	var updatedIngresses []*knativev1alpha1.Ingress
	for _, wrappedCi := range ingresses {
		ci := knativev1alpha1.Ingress(wrappedCi.Ingress)
		if ci.Status.ObservedGeneration == ci.ObjectMeta.Generation {
			continue
		}
		ci.Status.InitializeConditions()
		ci.Status.MarkNetworkConfigured()
		externalLbStatus := []knativev1alpha1.LoadBalancerIngressStatus{
			{DomainInternal: s.externalProxyAddress},
		}
		internalLbStatus := []knativev1alpha1.LoadBalancerIngressStatus{
			{DomainInternal: s.internalProxyAddress},
		}
		ci.Status.MarkLoadBalancerReady(externalLbStatus, internalLbStatus)
		ci.Status.ObservedGeneration = ci.Generation
		updatedIngresses = append(updatedIngresses, &ci)
	}
	for _, ingress := range updatedIngresses {
		if _, err := s.ingressClient.Ingresses(ingress.Namespace).UpdateStatus(ctx, ingress, v1machinery.UpdateOptions{}); err != nil {
			contextutils.LoggerFrom(ctx).Errorf("failed to update Ingress %v status with error %v", ingress.Name, err)
		}
	}
	return nil
}
