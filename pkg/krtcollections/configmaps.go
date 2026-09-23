package krtcollections

import (
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

type ConfigMapIndex struct {
	configmaps krt.Collection[*corev1.ConfigMap]
	refgrants  *RefGrantIndex
}

func NewConfigMapIndex(configmaps krt.Collection[*corev1.ConfigMap], refgrants *RefGrantIndex) *ConfigMapIndex {
	return &ConfigMapIndex{configmaps: configmaps, refgrants: refgrants}
}

func (c *ConfigMapIndex) HasSynced() bool {
	if !c.refgrants.HasSynced() {
		return false
	}
	return c.configmaps.HasSynced()
}

// Collection returns the underlying ConfigMap collection for direct access.
// This is needed for cases where reference grant validation is not required
// or is handled elsewhere.
func (c *ConfigMapIndex) Collection() krt.Collection[*corev1.ConfigMap] {
	return c.configmaps
}

// GetConfigMap retrieves a ConfigMap from the index, validating reference grants to ensure
// the source object is allowed to reference the target ConfigMap. Returns an error if
// reference grants are missing, or the ConfigMap is not found.
func (c *ConfigMapIndex) GetConfigMap(kctx krt.HandlerContext, from From, configMapRef gwv1.ObjectReference) (*corev1.ConfigMap, error) {
	toNs := strOr(configMapRef.Namespace, from.Namespace)

	to := ir.ObjectSource{
		Group:     "",
		Kind:      "ConfigMap",
		Namespace: toNs,
		Name:      string(configMapRef.Name),
	}

	if !c.refgrants.ReferenceAllowed(kctx, from.GroupKind, from.Namespace, to) {
		return nil, ErrMissingReferenceGrant
	}

	nn := types.NamespacedName{
		Namespace: toNs,
		Name:      string(configMapRef.Name),
	}
	cmPtr := krt.FetchOne(kctx, c.configmaps, krt.FilterObjectName(nn))
	if cmPtr == nil {
		return nil, &NotFoundError{NotFoundObj: to}
	}

	return *cmPtr, nil
}
