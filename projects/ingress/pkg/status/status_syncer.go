package status

import (
	"context"
	"net"
	"sort"

	"github.com/solo-io/gloo/pkg/utils/syncutil"
	"go.uber.org/zap/zapcore"

	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/projects/ingress/pkg/api/ingress"
	"github.com/solo-io/gloo/projects/ingress/pkg/api/service"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	kubev1 "k8s.io/api/core/v1"

	v1 "github.com/solo-io/gloo/projects/ingress/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

type statusSyncer struct {
	ingressClient v1.IngressClient
}

func NewSyncer(ingressClient v1.IngressClient) v1.StatusSyncer {
	return &statusSyncer{
		ingressClient: ingressClient,
	}
}

// TODO (ilackarms): make sure that sync happens if proxies get updated as well; may need to resync
func (s *statusSyncer) Sync(ctx context.Context, snap *v1.StatusSnapshot) error {
	ctx = contextutils.WithLogger(ctx, "statusSyncer")

	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("begin sync %v (%v ingresses, %v services)", snap.Hash(),
		len(snap.Ingresses), len(snap.Services))
	defer logger.Infof("end sync %v", snap.Hash())
	services := snap.Services

	// stringifying the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	if contextutils.GetLogLevel() == zapcore.DebugLevel {
		logger.Debug(syncutil.StringifySnapshot(snap))
	}

	lbStatus, err := getLbStatus(services)
	if err != nil {
		return err
	}

	for _, ing := range snap.Ingresses {
		kubeIngress, err := ingress.ToKube(ing)
		if err != nil {
			return errors.Wrapf(err, "internal error: converting proto ingress to kube ingress")
		}
		kubeIngress.Status.LoadBalancer.Ingress = lbStatus

		updatedIngress, err := ingress.FromKube(kubeIngress)
		if err != nil {
			return errors.Wrapf(err, "internal error: converting back to proto ingress from kube ingress")
		}

		if proto.Equal(updatedIngress.KubeIngressStatus, ing.KubeIngressStatus) {
			continue
		}
		if _, err := s.ingressClient.Write(updatedIngress, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true}); err != nil {
			return errors.Wrapf(err, "writing updated status to kubernetes")
		}
		logger.Infof("updated ingress %v with status %v", ing.Metadata.Ref(), lbStatus)
	}

	return nil
}

func getLbStatus(services v1.KubeServiceList) ([]kubev1.LoadBalancerIngress, error) {
	switch len(services) {
	case 0:
		return nil, nil
	case 1:
		kubeSvc, err := service.ToKube(services[0])
		if err != nil {
			return nil, errors.Wrapf(err, "internal error: converting proto svc to kube service")
		}
		return ingressStatusFromAddrs(serviceAddrs(kubeSvc)), nil
	}
	return nil, errors.Errorf("failed to get lb status: expected 1 ingress service, found %v", func() []core.ResourceRef {
		var refs []core.ResourceRef
		for _, svc := range services {
			refs = append(refs, svc.Metadata.Ref())
		}
		return refs
	}())
}

func serviceAddrs(svc *kubev1.Service) []string {
	if svc.Spec.Type == kubev1.ServiceTypeExternalName {
		return []string{svc.Spec.ExternalName}
	}
	var addrs []string

	for _, ip := range svc.Status.LoadBalancer.Ingress {
		if ip.IP == "" {
			addrs = append(addrs, ip.Hostname)
		} else {
			addrs = append(addrs, ip.IP)
		}
	}
	addrs = append(addrs, svc.Spec.ExternalIPs...)

	return addrs
}

func ingressStatusFromAddrs(addrs []string) []kubev1.LoadBalancerIngress {
	var lbi []kubev1.LoadBalancerIngress
	for _, ep := range addrs {
		if net.ParseIP(ep) == nil {
			lbi = append(lbi, kubev1.LoadBalancerIngress{Hostname: ep})
		} else {
			lbi = append(lbi, kubev1.LoadBalancerIngress{IP: ep})
		}
	}

	sort.SliceStable(lbi, func(a, b int) bool {
		return lbi[a].IP < lbi[b].IP
	})

	return lbi
}
