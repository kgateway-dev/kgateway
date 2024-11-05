package proxy_syncer

import (
	"fmt"
	"slices"

	"google.golang.org/protobuf/proto"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
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

var _ krt.Equaler[DestinationRuleWrapper] = new(DestinationRuleWrapper)

func (c DestinationRuleWrapper) Equals(k DestinationRuleWrapper) bool {
	// we only care if the spec changed..
	return proto.Equal(&c.Spec, &k.Spec)
}

func NewDestRuleIndex(istioClient kube.Client) DestinationRuleIndex {
	destRuleClient := kclient.NewDelayedInformer[*networkingclient.DestinationRule](istioClient, gvr.DestinationRule, kubetypes.StandardInformer, kclient.Filter{})
	rawDestrules := krt.WrapClient(destRuleClient, krt.WithName("DestinationRules"))
	destrules := krt.NewCollection(rawDestrules, func(kctx krt.HandlerContext, dr *networkingclient.DestinationRule) *DestinationRuleWrapper {
		return &DestinationRuleWrapper{dr}
	})
	return DestinationRuleIndex{
		Destrules:  destrules,
		ByHostname: newDestruleIndex(destrules),
	}
}

func NewEmptyDestRuleIndex() DestinationRuleIndex {
	destrules := krt.NewStaticCollection[DestinationRuleWrapper](nil)
	return DestinationRuleIndex{
		Destrules:  destrules,
		ByHostname: newDestruleIndex(destrules),
	}
}

func newDestruleIndex(destRuleCollection krt.Collection[DestinationRuleWrapper]) krt.Index[NsWithHostname, DestinationRuleWrapper] {
	idx := krt.NewIndex(destRuleCollection, func(d DestinationRuleWrapper) []NsWithHostname {
		return []NsWithHostname{{
			Ns:       d.Namespace,
			Hostname: d.Spec.GetHost(),
		}}
	})
	return idx
}

func (d *DestinationRuleIndex) FetchDestRulesFor(kctx krt.HandlerContext, proxyNs string, hostname string, podLabels map[string]string) *DestinationRuleWrapper {
	if hostname == "" {
		return nil
	}

	key := NsWithHostname{
		Ns:       proxyNs,
		Hostname: hostname,
	}
	destrules := krt.Fetch(kctx, d.Destrules, krt.FilterIndex(d.ByHostname, key), krt.FilterSelects(podLabels))
	if len(destrules) == 0 {
		return nil
	}
	// use oldest. TODO -  we need to merge them.
	oldestDestRule := slices.MinFunc(destrules, func(i DestinationRuleWrapper, j DestinationRuleWrapper) int {
		return i.CreationTimestamp.Time.Compare(j.CreationTimestamp.Time)
	})
	return &oldestDestRule
}
