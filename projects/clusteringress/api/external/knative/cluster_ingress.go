package knative

import (
	"reflect"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	"knative.dev/serving/pkg/apis/networking/v1alpha1"
)

type ClusterIngress v1alpha1.Ingress

func (p *ClusterIngress) GetMetadata() core.Metadata {
	return kubeutils.FromKubeMeta(p.ObjectMeta)
}

func (p *ClusterIngress) SetMetadata(meta core.Metadata) {
	p.ObjectMeta = kubeutils.ToKubeMeta(meta)
}

func (p *ClusterIngress) Equal(that interface{}) bool {
	return reflect.DeepEqual(p, that)
}

func (p *ClusterIngress) Clone() *ClusterIngress {
	ci := v1alpha1.Ingress(*p)
	ciCopy := ci.DeepCopy()
	newCi := ClusterIngress(*ciCopy)
	return &newCi
}
