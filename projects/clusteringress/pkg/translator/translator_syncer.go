package translator

import (
	"context"
	"github.com/knative/serving/pkg/apis/networking/v1alpha1"
	"github.com/solo-io/gloo/projects/clusteringress/pkg/api/clusteringress"
	"github.com/solo-io/go-utils/errors"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"time"

	v1 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

type translatorSyncer struct {
	proxyAddress  string
	writeNamespace       string
	writeErrs            chan error
	proxyClient          gloov1.ProxyClient
	clusterIngressClient v1.ClusterIngressClient
	proxyReconciler      gloov1.ProxyReconciler
}

func NewSyncer(writeNamespace, proxyAddress string, proxyClient gloov1.ProxyClient, ingressClient v1.ClusterIngressClient, writeErrs chan error) v1.TranslatorSyncer {
	return &translatorSyncer{
		proxyAddress:  proxyAddress,
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
	logger.Infof("begin sync %v (%v cluster ingresses, %v upstreams, %v secrets)", snap.Hash(),
		len(snap.Clusteringresses),
		len(snap.Upstreams),
		len(snap.Secrets),
	)
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

	if err := s.propagateProxyStatus(ctx, proxy, snap.Clusteringresses); err != nil {
		return errors.Wrapf(err, "failed to propagate proxy status "+
			"to clusteringress objects")
	}

	return nil
}

// propagate to all clusteringresses the status of the proxy
func (s *translatorSyncer) propagateProxyStatus(ctx context.Context, proxy *gloov1.Proxy, clusterIngresses v1.ClusterIngressList) error {
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
			return errors.Errorf("timed out waiting for proxy status to be updated")
		case <-ticker:
			// poll the proxy for an accepted or rejected status
			updatedProxy, err := s.proxyClient.Read(proxy.Metadata.Namespace, proxy.Metadata.Name, clients.ReadOpts{Ctx: ctx})
			if err != nil {
				return err
			}
			switch updatedProxy.Status.State {
			case core.Status_Pending:
				continue
			case core.Status_Rejected:
				contextutils.LoggerFrom(ctx).Errorf("proxy was rejected by gloo: %v",
					updatedProxy.Status.Reason)
				return nil
			case core.Status_Accepted:
				return s.markClusterIngressesReady(ctx, clusterIngresses)
			}
		}
	}
}

func (s *translatorSyncer) markClusterIngressesReady(ctx context.Context, clusterIngresses v1.ClusterIngressList) error {
	var updatedClusterIngresses v1.ClusterIngressList
	for _, intermediaryCi := range clusterIngresses {
		// convert the proto representation to the kube type
		ci, err := clusteringress.ToKube(intermediaryCi)
		if err != nil {
			return err
		}
		if ci.Status.IsReady() {
			continue
		}
		ci.Status.InitializeConditions()
		ci.Status.MarkNetworkConfigured()
		ci.Status.MarkLoadBalancerReady([]v1alpha1.LoadBalancerIngressStatus{
			{DomainInternal: s.proxyAddress},
		})
		ci.Status.ObservedGeneration = ci.Generation

		// convert it back and store it
		intermediaryCi, err = clusteringress.FromKube(ci)
		if err != nil {
			return err
		}
		updatedClusterIngresses = append(updatedClusterIngresses, intermediaryCi)
	}
	for _, ci := range updatedClusterIngresses {
		if _, err := s.clusterIngressClient.Write(ci, clients.WriteOpts{OverwriteExisting: true, Ctx: ctx}); err != nil {
			contextutils.LoggerFrom(ctx).Errorf("failed to update ClusterIngress %v status", ci.Metadata.Name)
		}
	}
	return nil
}
