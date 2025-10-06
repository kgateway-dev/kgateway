package destrule

import (
	"fmt"
	"slices"

	"google.golang.org/protobuf/proto"
	"istio.io/api/networking/v1alpha3"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

type NsWithHostname struct {
	Ns       string
	Hostname string
}

var _ fmt.Stringer = NsWithHostname{}

// needed as index key..
func (n NsWithHostname) String() string {
	return fmt.Sprintf("%s/%s", n.Ns, n.Hostname)
}

type DestinationRuleIndex struct {
	Destrules  krt.Collection[DestinationRuleWrapper]
	ByHostname krt.Index[NsWithHostname, DestinationRuleWrapper]
}
type DestinationRuleWrapper struct {
	*networkingclient.DestinationRule
}

// important for FilterSelects below
func (s DestinationRuleWrapper) GetLabelSelector() map[string]string {
	return s.Spec.GetWorkloadSelector().GetMatchLabels()
}

func (c DestinationRuleWrapper) ResourceName() string {
	return krt.Named{Namespace: c.Namespace, Name: c.Name}.ResourceName()
}

func (c DestinationRuleWrapper) String() string {
	return c.ResourceName()
}

var _ krt.Equaler[DestinationRuleWrapper] = new(DestinationRuleWrapper)

func (c DestinationRuleWrapper) Equals(k DestinationRuleWrapper) bool {
	// we only care if the spec changed..
	return proto.Equal(&c.Spec, &k.Spec)
}

func NewDestRuleIndex(istioClient kube.Client, krtopts *krtutil.KrtOptions) DestinationRuleIndex {
	destRuleClient := kclient.NewDelayedInformer[*networkingclient.DestinationRule](
		istioClient, gvr.DestinationRule, kubetypes.StandardInformer,
		kclient.Filter{ObjectFilter: istioClient.ObjectFilter()},
	)
	rawDestrules := krt.WrapClient(destRuleClient, krtopts.ToOptions("DestinationRules")...)
	destrules := krt.NewCollection(rawDestrules, func(kctx krt.HandlerContext, dr *networkingclient.DestinationRule) *DestinationRuleWrapper {
		return &DestinationRuleWrapper{dr}
	})
	return DestinationRuleIndex{
		Destrules:  destrules,
		ByHostname: newDestruleIndex(destrules),
	}
}

const exportAllNs = "*"

func newDestruleIndex(destRuleCollection krt.Collection[DestinationRuleWrapper]) krt.Index[NsWithHostname, DestinationRuleWrapper] {
	idx := krtpkg.UnnamedIndex(destRuleCollection, func(d DestinationRuleWrapper) []NsWithHostname {
		exportTo := d.Spec.GetExportTo()
		if len(exportTo) == 0 {
			return []NsWithHostname{{
				Ns:       exportAllNs,
				Hostname: d.Spec.GetHost(),
			}}
		}
		var keys []NsWithHostname
		for _, ns := range exportTo {
			if ns == "." {
				ns = d.Namespace
			}
			keys = append(keys, NsWithHostname{
				Ns:       ns,
				Hostname: d.Spec.GetHost(),
			})
		}

		return keys
	})
	return idx
}

func (d *DestinationRuleIndex) FetchDestRulesFor(kctx krt.HandlerContext, proxyNs string, hostname string, podLabels map[string]string) *DestinationRuleWrapper {
	if hostname == "" {
		return nil
	}

	key := NsWithHostname{
		Ns:       exportAllNs,
		Hostname: hostname,
	}
	destrules := krt.Fetch(kctx, d.Destrules, krt.FilterIndex(d.ByHostname, key), krt.FilterSelects(podLabels))
	if len(destrules) == 0 {
		key := NsWithHostname{
			Ns:       proxyNs,
			Hostname: hostname,
		}
		destrules = krt.Fetch(kctx, d.Destrules, krt.FilterIndex(d.ByHostname, key), krt.FilterSelects(podLabels))
	}
	if len(destrules) == 0 {
		return nil
	}
	// use oldest. TODO -  we need to merge them.
	oldestDestRule := slices.MinFunc(destrules, func(i DestinationRuleWrapper, j DestinationRuleWrapper) int {
		return i.CreationTimestamp.Time.Compare(j.CreationTimestamp.Time)
	})
	return &oldestDestRule
}

func getLocalityLbSetting(trafficPolicy *v1alpha3.TrafficPolicy) *v1alpha3.LocalityLoadBalancerSetting {
	if trafficPolicy == nil {
		return nil
	}
	localityLb := trafficPolicy.GetLoadBalancer().GetLocalityLbSetting()
	if localityLb != nil {
		if localityLb.GetEnabled() != nil && !localityLb.GetEnabled().Value {
			return nil
		}
	}
	return localityLb
}

func getTrafficPolicy(destrule *DestinationRuleWrapper, port uint32) *v1alpha3.TrafficPolicy {
	trafficPolicy := destrule.Spec.GetTrafficPolicy()
	if trafficPolicy == nil {
		return nil
	}

	for _, portlevel := range trafficPolicy.GetPortLevelSettings() {
		if portlevel.GetPort() != nil {
			if portlevel.GetPort().GetNumber() == port {
				return convertPortLevel(portlevel)
			}
		}
	}
	return trafficPolicy
}

func convertPortLevel(portlevel *v1alpha3.TrafficPolicy_PortTrafficPolicy) *v1alpha3.TrafficPolicy {
	return &v1alpha3.TrafficPolicy{
		ConnectionPool:   portlevel.GetConnectionPool(),
		LoadBalancer:     portlevel.GetLoadBalancer(),
		OutlierDetection: portlevel.GetOutlierDetection(),
		Tls:              portlevel.GetTls(),
	}
}
