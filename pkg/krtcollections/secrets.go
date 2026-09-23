package krtcollections

import (
	"fmt"
	"slices"
	"strings"

	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// From identifies the resource that holds a cross-namespace reference, for the
// purpose of evaluating ReferenceGrants against it. GroupKind is the identity a
// ReferenceGrant has to name in from.group and from.kind to permit the reference.
type From struct {
	schema.GroupKind
	Namespace string
}

type SecretIndex struct {
	secrets     map[schema.GroupKind]krt.Collection[ir.Secret]
	byNamespace map[schema.GroupKind]krt.Index[string, ir.Secret]
	refgrants   *RefGrantIndex
}

func NewSecretIndex(secrets map[schema.GroupKind]krt.Collection[ir.Secret], refgrants *RefGrantIndex) *SecretIndex {
	byNamespace := make(map[schema.GroupKind]krt.Index[string, ir.Secret], len(secrets))
	for gk, col := range secrets {
		byNamespace[gk] = krt.NewNamespaceIndex(col)
	}
	return &SecretIndex{secrets: secrets, byNamespace: byNamespace, refgrants: refgrants}
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

// GetSecret retrieves a secret from the index, validating reference grants to ensure
// the source object is allowed to reference the target secret. Returns an error if
// the secret kind is unknown, reference grants are missing, or the secret is not found.
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

	to := ir.ObjectSource{
		Group:     secretGroup,
		Kind:      secretKind,
		Namespace: toNs,
		Name:      string(secretRef.Name),
	}
	col := s.secrets[schema.GroupKind{Group: secretGroup, Kind: secretKind}]
	if col == nil {
		// should never happen
		return nil, fmt.Errorf("internal error looking up secret %s", to.NamespacedName())
	}

	if !s.refgrants.ReferenceAllowed(kctx, from.GroupKind, from.Namespace, to) {
		return nil, &MissingReferenceGrantError{From: from, To: to}
	}
	secret := krt.FetchOne(kctx, col, krt.FilterKey(to.ResourceName()))
	if secret == nil {
		return nil, &NotFoundError{NotFoundObj: to}
	}
	return secret, nil
}

// GetSecretWithoutRefGrant retrieves a secret from a given namespace.
// This is a convenience function for same-namespace secret lookups where ReferenceGrant
// validation is not needed (same-namespace references are always allowed).
// Warning: this function does not validate the ReferenceGrant, so it is not safe to use for cross-namespace references.
func (s *SecretIndex) GetSecretWithoutRefGrant(kctx krt.HandlerContext, secretName, ns string) (*ir.Secret, error) {
	secretRef := gwv1.SecretObjectReference{
		Name: gwv1.ObjectName(secretName),
	}
	// ReferenceGrant check will always pass for same-namespace,
	// so GroupKind doesn't matter
	from := From{
		Namespace: ns,
	}
	return s.GetSecret(kctx, from, secretRef)
}

// SelectorNoMatchError reports a label selector that matched no resource the
// referrer may reference.
//
// It is the only error a selector with no permitted match returns. Which resources
// carry the labels in a namespace that has not granted the referrer access is not
// observable, so "nothing matched" and "something matched but is not granted" must
// read the same: telling them apart would let a referrer probe label values across
// the cluster. The message names the namespaces searched and the grant identity that
// widens the search, which is enough to fix either case.
type SelectorNoMatchError struct {
	From        From
	To          schema.GroupKind
	MatchLabels map[string]string

	// AnyNamespace is set when ReferenceGrants are not enforced, so every namespace
	// was searched.
	AnyNamespace bool
}

func (e *SelectorNoMatchError) Error() string {
	selector := labels.SelectorFromSet(e.MatchLabels).String()
	if e.AnyNamespace {
		return fmt.Sprintf("no %ss matching %q in any namespace", e.To.Kind, selector)
	}
	return fmt.Sprintf(
		"no %ss matching %q in namespace %q or in namespaces with a ReferenceGrant allowing %ss to be referenced from group %q kind %q in namespace %q",
		e.To.Kind, selector, e.From.Namespace, e.To.Kind, e.From.Group, e.From.Kind, e.From.Namespace,
	)
}

// GetSecretsBySelector retrieves the secrets matching matchLabels that from may
// reference: those in from's namespace, and those in namespaces whose ReferenceGrants
// permit it. Only those namespaces are searched, so a secret the referrer cannot
// reference never influences the result.
//
// It returns a SelectorNoMatchError when no permitted secret matches.
func (s *SecretIndex) GetSecretsBySelector(
	kctx krt.HandlerContext,
	from From,
	secretGK schema.GroupKind,
	matchLabels map[string]string,
) ([]ir.Secret, error) {
	col := s.secrets[secretGK]
	if col == nil {
		return nil, ErrUnknownBackendKind
	}

	matches := krt.FilterGeneric(func(obj any) bool {
		secret := obj.(ir.Secret)
		if secret.Obj == nil {
			return false
		}
		objLabels := secret.Obj.GetLabels()
		if objLabels == nil {
			return false
		}
		for key, value := range matchLabels {
			if objLabels[key] != value {
				return false
			}
		}
		return true
	})

	grantingNamespaces, all := s.refgrants.GrantingNamespaces(kctx, from.GroupKind, from.Namespace, secretGK)
	var allowedSecrets []ir.Secret
	if all {
		allowedSecrets = krt.Fetch(kctx, col, matches)
	} else {
		byNamespace := s.byNamespace[secretGK]
		for _, ns := range append(grantingNamespaces, from.Namespace) {
			for _, secret := range krt.Fetch(kctx, col, krt.FilterIndex(byNamespace, ns), matches) {
				// A grant can be limited to named secrets, so a granting namespace
				// does not by itself permit every secret in it.
				if ns != from.Namespace && !s.refgrants.ReferenceAllowed(kctx, from.GroupKind, from.Namespace, secret.ObjectSource) {
					continue
				}
				allowedSecrets = append(allowedSecrets, secret)
			}
		}
	}

	if len(allowedSecrets) == 0 {
		return nil, &SelectorNoMatchError{From: from, To: secretGK, MatchLabels: matchLabels, AnyNamespace: all}
	}
	// Order by namespace/name so the translated config is stable across fetches.
	slices.SortFunc(allowedSecrets, func(a, b ir.Secret) int {
		return strings.Compare(a.ResourceName(), b.ResourceName())
	})
	return allowedSecrets, nil
}
