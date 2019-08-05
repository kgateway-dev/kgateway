package knative

import (
	"reflect"

	"github.com/knative/serving/pkg/apis/networking/v1alpha1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
)

type Ingress v1alpha1.Ingress

func (p *Ingress) GetMetadata() core.Metadata {
	return kubeutils.FromKubeMeta(p.ObjectMeta)
}

func (p *Ingress) SetMetadata(meta core.Metadata) {
	p.ObjectMeta = kubeutils.ToKubeMeta(meta)
}

func (p *Ingress) Equal(that interface{}) bool {
	return reflect.DeepEqual(p, that)
}

func (p *Ingress) Clone() *Ingress {
	ci := v1alpha1.Ingress(*p)
	copy := ci.DeepCopy()
	newCi := Ingress(*copy)
	return &newCi
}
