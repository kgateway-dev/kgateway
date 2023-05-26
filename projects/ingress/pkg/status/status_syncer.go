package status

import (
	"context"
	"net"
	"sort"

	networkv1 "k8s.io/api/networking/v1"

	"github.com/solo-io/gloo/pkg/utils/syncutil"
	"github.com/solo-io/go-utils/hashutils"
	"go.uber.org/zap/zapcore"

	"github.com/golang/protobuf/proto"
	errors "github.com/rotisserie/eris"
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
	snapHash := hashutils.MustHash(snap)
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("begin sync %v (%v ingresses, %v services)", snapHash,
		len(snap.Ingresses), len(snap.Services))
	defer logger.Infof("end sync %v", snapHash)
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

		if proto.Equal(updatedIngress.GetKubeIngressStatus(), ing.GetKubeIngressStatus()) {
			continue
		}
		if _, err := s.ingressClient.Write(updatedIngress, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true}); err != nil {
			return errors.Wrapf(err, "writing updated status to kubernetes")
		}
		logger.Infof("updated ingress %v with status %v", ing.GetMetadata().Ref(), lbStatus)
	}

	return nil
}

func getLbStatus(services v1.KubeServiceList) ([]networkv1.IngressLoadBalancerIngress, error) {
	switch len(services) {
	case 0:
		return nil, nil
	case 1:
		kubeSvc, err := service.ToKube(services[0])
		if err != nil {
			return nil, errors.Wrapf(err, "internal error: converting proto svc to kube service")
		}

		kubeSvcRef := services[0].GetMetadata().Ref()
		kubeSvcAddrs, err := serviceAddrs(kubeSvc, kubeSvcRef)
		if err != nil {
			return nil, errors.Wrapf(err, "internal err: extracting service addrs from kube service")
		}

		return ingressStatusFromAddrs(kubeSvcAddrs), nil
	}
	return nil, errors.Errorf("failed to get lb status: expected 1 ingress service, found %v", func() []*core.ResourceRef {
		var refs []*core.ResourceRef
		for _, svc := range services {
			refs = append(refs, svc.GetMetadata().Ref())
		}
		return refs
	}())
}

func serviceAddrs(svc *kubev1.Service, kubeSvcRef *core.ResourceRef) ([]string, error) {
	if svc.Spec.Type == kubev1.ServiceTypeExternalName {

		// Remove the possibility of using localhost in ExternalNames as endpoints
		svcIp := net.ParseIP(svc.Spec.ExternalName)
		if svc.Spec.ExternalName == "localhost" || (svcIp != nil && svcIp.IsLoopback()) {
			return nil, errors.Errorf("Invalid attempt to use localhost name %s, in %v", svc.Spec.ExternalName, kubeSvcRef)
		}
		return []string{svc.Spec.ExternalName}, nil
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

	return addrs, nil
}

func ingressStatusFromAddrs(addrs []string) []networkv1.IngressLoadBalancerIngress {
	var lbi []networkv1.IngressLoadBalancerIngress
	for _, ep := range addrs {
		if net.ParseIP(ep) == nil {
			lbi = append(lbi, networkv1.IngressLoadBalancerIngress{Hostname: ep})
		} else {
			lbi = append(lbi, networkv1.IngressLoadBalancerIngress{IP: ep})
		}
	}

	sort.SliceStable(lbi, func(a, b int) bool {
		return lbi[a].IP < lbi[b].IP
	})

	return lbi
}
