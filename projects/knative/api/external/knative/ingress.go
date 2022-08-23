package knative

import (
	"reflect"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
)

type Ingress v1alpha1.Ingress

func (p *Ingress) GetMetadata() *core.Metadata {
	return kubeutils.FromKubeMeta(p.ObjectMeta, true)
}

func (p *Ingress) SetMetadata(meta *core.Metadata) {
	p.ObjectMeta = kubeutils.ToKubeMeta(meta)
}

func (p *Ingress) Equal(that interface{}) bool {
	return reflect.DeepEqual(p, that)
}

func (p *Ingress) Clone() *Ingress {
	ing := v1alpha1.Ingress(*p)
	copy := ing.DeepCopy()
	newIng := Ingress(*copy)
	return &newIng
}

func (p *Ingress) IsPublic() bool {
	// by default, ingresses are public if they have no rules saying otherwise
	isPublic := true
	for _, ingressRule := range p.Spec.Rules {
		// if there is _any_ ingress rule, it is not public
		isPublic = false

		// ...unless we match a configured IngressVisibilityExternalIP
		if ingressRule.Visibility == "" || ingressRule.Visibility == v1alpha1.IngressVisibilityExternalIP {
			return true
		}
	}
	return isPublic
}
