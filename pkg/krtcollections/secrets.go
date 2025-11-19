package krtcollections

import (
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

type From struct {
	schema.GroupKind
	Namespace string
}

type SecretIndex struct {
	secrets   map[schema.GroupKind]krt.Collection[ir.Secret]
	refgrants *RefGrantIndex
}

func NewSecretIndex(secrets map[schema.GroupKind]krt.Collection[ir.Secret], refgrants *RefGrantIndex) *SecretIndex {
	return &SecretIndex{secrets: secrets, refgrants: refgrants}
}

func (s *SecretIndex) HasSynced() bool {
	if !s.refgrants.HasSynced() {
		return false
	}
	for _, col := range s.secrets {
		if !col.HasSynced() {
			return false
		}
	}
	return true
}

// TODO: comment seems out of date; method is already public and does refgrant logic
// if we want to make this function public, make it do ref grants
func (s *SecretIndex) GetSecret(kctx krt.HandlerContext, from From, secretRef gwv1.SecretObjectReference) (*ir.Secret, error) {
	secretKind := "Secret"
	secretGroup := ""
	toNs := strOr(secretRef.Namespace, from.Namespace)
	if secretRef.Group != nil {
		secretGroup = string(*secretRef.Group)
	}
	if secretRef.Kind != nil {
		secretKind = string(*secretRef.Kind)
	}
	gk := schema.GroupKind{Group: secretGroup, Kind: secretKind}

	to := ir.ObjectSource{
		Group:     secretGroup,
		Kind:      secretKind,
		Namespace: toNs,
		Name:      string(secretRef.Name),
	}
	col := s.secrets[gk]
	if col == nil {
		return nil, ErrUnknownBackendKind
	}

	if !s.refgrants.ReferenceAllowed(kctx, from.GroupKind, from.Namespace, to) {
		return nil, ErrMissingReferenceGrant
	}
	secret := krt.FetchOne(kctx, col, krt.FilterKey(to.ResourceName()))
	if secret == nil {
		return nil, &NotFoundError{NotFoundObj: to}
	}
	return secret, nil
}
